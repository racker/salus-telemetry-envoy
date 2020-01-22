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
	"github.com/racker/salus-telemetry-envoy/agents/matchers"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackagesAgentRunner_ProcessConfig(t *testing.T) {
	tests := []struct {
		name             string
		ops              []*telemetry_edge.ConfigurationOp
		expectedContents []string
	}{
		{name: "create", ops: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-rpm",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"packages", "includeRpm":true}`,
				Interval: 3600,
			},
			{
				Id:       "monitor-deb",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"packages", "includeDebian":true}`,
				Interval: 3600,
			},
		},
			expectedContents: []string{
				`{"include-rpm":true,"interval": "1h0m0s"}`,
				`{"include-debian":true,"interval":"1h0m0s"}`,
			},
		},
		{name: "modify", ops: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-rpm",
				Type:     telemetry_edge.ConfigurationOp_MODIFY,
				Content:  `{"type":"packages", "includeRpm":true,"includeDebian":true}`,
				Interval: 7200,
			},
			{
				Id:       "monitor-deb",
				Type:     telemetry_edge.ConfigurationOp_MODIFY,
				Content:  `{"type":"packages", "includeDebian":true,"includeRpm":true}`,
				Interval: 7200,
			},
		},
			expectedContents: []string{
				`{"include-rpm":true,"include-debian":true,"interval":"2h0m0s"}`,
				`{"include-debian":true,"include-rpm":true,"interval":"2h0m0s"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataPath, err := ioutil.TempDir("", "pkgagent_test")
			require.NoError(t, err)
			defer os.RemoveAll(dataPath)

			runner := &agents.PackagesAgentRunner{}
			err = runner.Load(dataPath)
			require.NoError(t, err)

			configure := &telemetry_edge.EnvoyInstructionConfigure{
				AgentType:  telemetry_edge.AgentType_PACKAGES,
				Operations: tt.ops,
			}
			err = runner.ProcessConfig(configure)
			require.NoError(t, err)

			found, err := readFilesIntoMap(dataPath, ".json")
			require.NoError(t, err)
			assert.Len(t, found, 2)

			for i, op := range tt.ops {
				content, ok := found[fmt.Sprintf("config.d/%s.json", op.Id)]
				assert.True(t, ok, "op:%+v", op)
				assert.JSONEq(t, tt.expectedContents[i], content, "op:%+v", op)
			}
		})
	}

}

func TestPackagesAgentRunner_ProcessConfig_remove(t *testing.T) {
	dataPath, err := ioutil.TempDir("", "pkgagent_test")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	runner := &agents.PackagesAgentRunner{}
	err = runner.Load(dataPath)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(dataPath, "config.d"), 0755)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(dataPath, "config.d", "monitor-rpm.json"), []byte(`{}`), 0644)
	require.NoError(t, err)

	// sanity check the "config file" above landed in the config area validated below
	found, err := readFilesIntoMap(dataPath, ".json")
	require.NoError(t, err)
	require.Len(t, found, 1)

	configure := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_PACKAGES,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:   "monitor-rpm",
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

func TestPackagesAgentRunner_EnsureRunningState_noApplyConfigs(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	commandHandler := NewMockCommandHandler()
	runningContext := agents.CreatePreRunningAgentRunningContext()
	pegomock.When(commandHandler.CreateContext(
		matchers.AnyContextContext(),
		matchers.AnyTelemetryEdgeAgentType(),
		pegomock.AnyString(), pegomock.AnyString(),
		pegomock.AnyString(), pegomock.AnyString(), // args
		pegomock.AnyString(), pegomock.AnyString(), // more args
	)).ThenReturn(runningContext)

	dataPath, err := ioutil.TempDir("", "pkgagent_test")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	runner := &agents.PackagesAgentRunner{}
	runner.SetCommandHandler(commandHandler)
	config.RegisterListenerAddress(config.LineProtocolListener, "localhost:8899")
	err = runner.Load(dataPath)
	require.NoError(t, err)

	configure := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_PACKAGES,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:       "monitor-rpm",
				Type:     telemetry_edge.ConfigurationOp_CREATE,
				Content:  `{"type":"packages", "includeRpm":true}`,
				Interval: 3600,
			}},
	}
	// it's safe to the real ProcessConfig since it was tested in previous test case
	err = runner.ProcessConfig(configure)
	require.NoError(t, err)

	err = createFakeAgentExe(t, dataPath, "salus-packages-agent")
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
		CreateContext(matchers.AnyContextContext(),
			matchers.EqTelemetryEdgeAgentType(telemetry_edge.AgentType_PACKAGES),
			pegomock.EqString("CURRENT/bin/salus-packages-agent"),
			pegomock.EqString(dataPath),
			pegomock.EqString("--configs"),
			pegomock.EqString("config.d"),
			pegomock.EqString("--line-protocol-to-socket"),
			pegomock.EqString("localhost:8899"))

	// called at steps 1 and 3
	commandHandler.VerifyWasCalled(pegomock.Times(2)).
		StartAgentCommand(matchers.AnyPtrToAgentsAgentRunningContext(),
			matchers.EqTelemetryEdgeAgentType(telemetry_edge.AgentType_PACKAGES),
			pegomock.EqString(""),
			matchers.AnyTimeDuration())

	// called at steps 1 and 3
	commandHandler.VerifyWasCalledEventually(pegomock.Times(2), 1*time.Second).
		WaitOnAgentCommand(matchers.AnyContextContext(),
			matchers.EqAgentsSpecificAgentRunner(runner),
			matchers.EqPtrToAgentsAgentRunningContext(runningContext))

	// called only at start of step 3
	commandHandler.VerifyWasCalled(pegomock.Once()).
		Stop(matchers.AnyPtrToAgentsAgentRunningContext())
}

func TestPackagesAgentRunner_ProcessTestMonitor(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	content := []byte(`Some kind of logging line to ignore
> packages,system=debian,package=sensible-utils,arch=all version="0.0.12" 1579042018775063900
> packages,system=debian,package=sysvinit-utils,arch=amd64 version="2.88dsf-59.10ubuntu1" 1579042018775063900
`)

	commandHandler := NewMockCommandHandler()
	pegomock.When(
		commandHandler.RunToCompletion(matchers.AnyContextContext(),
			pegomock.AnyString(), pegomock.AnyString(), // exe and basePath
			pegomock.AnyString(),                       // arg to console
			pegomock.AnyString(), pegomock.AnyString(), // arg include rpm
			pegomock.AnyString(), pegomock.AnyString(), // arg include debian
		),
	).
		ThenReturn(content, nil)

	runner := &agents.PackagesAgentRunner{}
	runner.SetCommandHandler(commandHandler)

	results, err := runner.ProcessTestMonitor("request-1", `{"include-debian":true}`, 60*time.Second)
	require.NoError(t, err)

	require.NotNil(t, results)
	assert.Equal(t, "request-1", results.CorrelationId)
	assert.Empty(t, results.Errors)
	assert.Len(t, results.Metrics, 2)

	metric := results.Metrics[0].GetNameTagValue()
	assert.Equal(t, "packages", metric.Name)
	assert.Equal(t, int64(1579042018775), metric.Timestamp)
	assert.Equal(t, map[string]string{
		"system":  "debian",
		"package": "sensible-utils",
		"arch":    "all",
	}, metric.Tags)
	assert.Equal(t, map[string]string{
		"version": "0.0.12",
	}, metric.Svalues)

	metric = results.Metrics[1].GetNameTagValue()
	assert.Equal(t, "packages", metric.Name)
	assert.Equal(t, int64(1579042018775), metric.Timestamp)
	assert.Equal(t, map[string]string{
		"system":  "debian",
		"package": "sysvinit-utils",
		"arch":    "amd64",
	}, metric.Tags)
	assert.Equal(t, map[string]string{
		"version": "2.88dsf-59.10ubuntu1",
	}, metric.Svalues)

	commandHandler.VerifyWasCalledOnce().
		RunToCompletion(matchers.AnyContextContext(),
			pegomock.EqString("CURRENT/bin/salus-packages-agent"),
			pegomock.AnyString(),
			pegomock.EqString("--line-protocol-to-console"),
			pegomock.EqString("--include-rpm"),
			pegomock.EqString("false"),
			pegomock.EqString("--include-debian"),
			pegomock.EqString("true"),
		)
}
