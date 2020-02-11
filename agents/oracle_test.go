/*
 * Copyright 2020 Rackspace US, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agents_test

import (
	"context"
	"fmt"
	"github.com/petergtz/pegomock"
	"github.com/racker/salus-telemetry-envoy/agents"
	. "github.com/racker/salus-telemetry-envoy/agents/matchers"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOracleAgentRunner_ProcessConfig(t *testing.T) {
	tests := []struct {
		name             string
		ops              []*telemetry_edge.ConfigurationOp
		expectedContents []string
	}{
		{name: "create", ops: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-rman",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"oracle_rman","databaseNames":["RMAN"],"filePath":"./testdata", "errorCodeWhitelist": ["RMAN-1234"]}`,
				Interval: 3600,
			},
			{
				Id:       "monitor-dataguard",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"oracle_dataguard","databaseNames":["dataguard"],"filePath":"./testdata/"}`,
				Interval: 3600,
			},
			{
				Id:       "monitor-tablespace",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"oracle_tablespace","databaseNames":["tablespace"],"filePath":"./testdata/"}`,
				Interval: 3600,
			},
		},
			expectedContents: []string{
				`{"type":"oracle_rman","databaseNames":["RMAN"],"filePath":"./testdata", "errorCodeWhitelist": ["RMAN-1234"], "interval": 3600}`,
				`{"type":"oracle_dataguard","databaseNames":["dataguard"],"filePath":"./testdata/", "interval": 3600}`,
				`{"type":"oracle_tablespace","databaseNames":["tablespace"],"filePath":"./testdata/", "interval": 3600}`,
			},
		},
		{name: "modify", ops: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-rman",
				Type:     telemetry_edge.ConfigurationOp_MODIFY,
				Content:  `{"type":"oracle_rman","databaseNames":["RMAN"],"filePath":"./testdata", "errorCodeWhitelist":  ["ORA-1234"]}`,
				Interval: 7200,
			},
			{
				Id:       "monitor-dataguard",
				Type:     telemetry_edge.ConfigurationOp_MODIFY,
				Content:  `{"type":"oracle_dataguard","databaseNames":["dataguard", "otherDataguard"],"filePath":"./testdata/"}`,
				Interval: 7200,
			},
			{
				Id:       "monitor-tablespace",
				Type:     telemetry_edge.ConfigurationOp_MODIFY,
				Content:  `{"type":"oracle_tablespace","databaseNames":["tablespace", "otherTablespace"],"filePath":"./testdata/"}`,
				Interval: 7200,
			},
		},
			expectedContents: []string{
				`{"type":"oracle_rman","databaseNames":["RMAN"],"filePath":"./testdata", "errorCodeWhitelist":  ["ORA-1234"], "interval": 7200}`,
				`{"type":"oracle_dataguard","databaseNames":["dataguard", "otherDataguard"],"filePath":"./testdata/", "interval": 7200}`,
				`{"type":"oracle_tablespace","databaseNames":["tablespace", "otherTablespace"],"filePath":"./testdata/", "interval": 7200}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath, err := ioutil.TempDir("", "oracleagent_test")
			require.NoError(t, err)
			defer os.RemoveAll(dataPath)

			runner := &agents.OracleRunner{}
			err = runner.Load(dataPath)
			require.NoError(t, err)

			configure := &telemetry_edge.EnvoyInstructionConfigure{
				AgentType:  telemetry_edge.AgentType_ORACLE,
				Operations: tt.ops,
			}
			err = runner.ProcessConfig(configure)
			require.NoError(t, err)

			found, err := readFilesIntoMap(dataPath, ".json")
			require.NoError(t, err)
			assert.Len(t, found, 3)

			for i, op := range tt.ops {
				content, ok := found[fmt.Sprintf("config.d/%s.json", op.Id)]
				assert.True(t, ok, "op:%+v", op)
				assert.JSONEq(t, tt.expectedContents[i], content, "op:%+v", op)
			}
		})
	}

}

func TestOracleAgentRunner_ProcessConfig_remove(t *testing.T) {
	dataPath, err := ioutil.TempDir("", "oracleagent_test")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	runner := &agents.OracleRunner{}
	err = runner.Load(dataPath)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(dataPath, "config.d"), 0755)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(dataPath, "config.d", "monitor-rman.json"), []byte(`{}`), 0644)
	require.NoError(t, err)

	// sanity check the "config file" above landed in the config area validated below
	found, err := readFilesIntoMap(dataPath, ".json")
	require.NoError(t, err)
	require.Len(t, found, 1)

	configure := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_ORACLE,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:   "monitor-rman",
				Type: telemetry_edge.ConfigurationOp_REMOVE,
			},
		},
	}
	err = runner.ProcessConfig(configure)
	require.NoError(t, err)

	found, err = readFilesIntoMap(dataPath, ".json")
	require.NoError(t, err)
	assert.Len(t, found, 0)
}

func TestOracleAgentRunner_EnsureRunningState_ApplyConfigsParameter(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	commandHandler := NewMockCommandHandler()
	runningContext := agents.CreatePreRunningAgentRunningContext()
	pegomock.When(commandHandler.CreateContext(
		AnyContextContext(),
		AnyTelemetryEdgeAgentType(),
		pegomock.AnyString(), pegomock.AnyString(),
		pegomock.AnyString(), pegomock.AnyString(),
	)).ThenReturn(runningContext)

	dataPath, err := ioutil.TempDir("", "oracleagent_test")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	runner := &agents.OracleRunner{}
	runner.SetCommandHandler(commandHandler)

	err = runner.Load(dataPath)
	require.NoError(t, err)

	configure := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_ORACLE,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-dataguard",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"oracle_dataguard","databaseNames":["dataguard"],"filePath":"./testdata/"}`,
				Interval: 3600,
			}},
	}
	// it's safe to call the real ProcessConfig since it was tested in previous test case
	err = runner.ProcessConfig(configure)
	require.NoError(t, err)

	err = createFakeAgentExe(t, dataPath, "salus-oracle-agent")
	require.NoError(t, err)

	ctx := context.Background()

	// Step 1: should cause a normal startup of agent
	runner.EnsureRunningState(ctx, false)

	// Step 2: should result in a no-op since already running and no config changed
	runner.EnsureRunningState(ctx, false)

	// Step 3: should cause a stop and then re-startup of agent
	runner.EnsureRunningState(ctx, true)

	// called at steps 1 and 3
	commandHandler.VerifyWasCalled(pegomock.Times(2)).
		CreateContext(AnyContextContext(),
			EqTelemetryEdgeAgentType(telemetry_edge.AgentType_ORACLE),
			pegomock.EqString("CURRENT/bin/salus-oracle-agent"),
			pegomock.EqString(dataPath),
			pegomock.AnyString(), pegomock.AnyString())

	// called at steps 1 and 3
	commandHandler.VerifyWasCalled(pegomock.Times(2)).
		StartAgentCommand(AnyPtrToAgentsAgentRunningContext(),
			EqTelemetryEdgeAgentType(telemetry_edge.AgentType_ORACLE),
			pegomock.EqString("Succeeded in reconnecting to Envoy"),
			AnyTimeDuration())

	// called at steps 1 and 3
	commandHandler.VerifyWasCalledEventually(pegomock.Times(2), 10*time.Second).
		WaitOnAgentCommand(AnyContextContext(),
			EqAgentsSpecificAgentRunner(runner),
			EqPtrToAgentsAgentRunningContext(runningContext))

	// called only at start of step 3
	commandHandler.VerifyWasCalled(pegomock.Once()).
		Stop(AnyPtrToAgentsAgentRunningContext())
}

func TestOracleAgentRunner_ProcessTestMonitor(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	commandHandler := NewMockCommandHandler()


	runner := &agents.OracleRunner{}
	runner.SetCommandHandler(commandHandler)

	_, err := runner.ProcessTestMonitor("request-1", `{"include-debian":true}`, 60*time.Second)
	require.Error(t, err, "Test monitor not supported by oracle agent")

}