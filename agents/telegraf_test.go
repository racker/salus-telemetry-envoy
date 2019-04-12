/*
 *    Copyright 2018 Rackspace US, Inc.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 *
 */

package agents_test

import (
	"context"
	"github.com/petergtz/pegomock"
	"github.com/racker/telemetry-envoy/agents"
	"github.com/racker/telemetry-envoy/agents/matchers"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTelegrafRunner_ProcessConfig_CreateModify(t *testing.T) {
	tests := []struct {
		opType telemetry_edge.ConfigurationOp_Type
	}{
		{opType: telemetry_edge.ConfigurationOp_CREATE},
		{opType: telemetry_edge.ConfigurationOp_MODIFY},
	}

	for _, tt := range tests {
		t.Run(tt.opType.String(), func(t *testing.T) {
			pegomock.RegisterMockTestingT(t)

			dataPath, err := ioutil.TempDir("", "telegraf_test")
			require.NoError(t, err)
			defer os.RemoveAll(dataPath)

			runner := &agents.TelegrafRunner{}
			viper.Set(config.IngestTelegrafJsonBind, "localhost:8094")
			err = runner.Load(dataPath)
			require.NoError(t, err)

			commandHandler := NewMockCommandHandler()
			runner.SetCommandHandler(commandHandler)

			configure := &telemetry_edge.EnvoyInstructionConfigure{
				AgentType: telemetry_edge.AgentType_TELEGRAF,
				Operations: []*telemetry_edge.ConfigurationOp{
					{
						Id:      "a-b-c",
						Type:    tt.opType,
						Content: "{\"type\":\"mem\"}",
					},
				},
			}
			err = runner.ProcessConfig(configure)
			require.NoError(t, err)

			var files, mainConfigs, instanceConfigs int
			err = filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
				if !info.IsDir() {
					files++
				}
				if filepath.Base(path) == "telegraf.conf" {
					mainConfigs++
					content, err := ioutil.ReadFile(path)
					require.NoError(t, err)

					assert.Contains(t, string(content), "outputs.socket_writer")
					assert.Contains(t, string(content), "address = \"tcp://localhost:8094\"")
				} else if filepath.Base(path) == "a-b-c.conf" {
					instanceConfigs++
					content, err := ioutil.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, "[inputs]\n\n  [[inputs.mem]]\n", string(content))

					assert.Equal(t, "config.d", filepath.Base(filepath.Dir(path)))
				}
				return nil
			})
			require.NoError(t, err)
			assert.NotZero(t, files)
			assert.Equal(t, 1, mainConfigs)
			assert.Equal(t, 1, instanceConfigs)

		})
	}
}

func TestTelegrafRunner_EnsureRunning_NoConfig(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	dataPath, err := ioutil.TempDir("", "test_agents")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	mockCommandHandler := NewMockCommandHandler()

	telegrafRunner := &agents.TelegrafRunner{}
	telegrafRunner.SetCommandHandler(mockCommandHandler)
	viper.Set(config.IngestTelegrafJsonBind, "localhost:8094")
	err = telegrafRunner.Load(dataPath)
	require.NoError(t, err)

	ctx := context.Background()
	telegrafRunner.EnsureRunningState(ctx, false)

	mockCommandHandler.VerifyWasCalled(pegomock.Never()).
		StartAgentCommand(matchers.AnyPtrToAgentsAgentRunningContext(), matchers.AnyTelemetryEdgeAgentType(),
			pegomock.AnyString(), matchers.AnyTimeDuration())
	mockCommandHandler.VerifyWasCalledOnce().
		Stop(matchers.AnyPtrToAgentsAgentRunningContext())
}

func TestTelegrafRunner_EnsureRunningState_FullSequence(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	dataPath, err := ioutil.TempDir("", "TestTelegrafRunner_EnsureRunningState_NeedsReload")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	// touch the file telegraf "exe" in the bin directory
	binPath := path.Join(dataPath, "CURRENT", "bin")
	err = os.MkdirAll(binPath, 0755)
	require.NoError(t, err)
	binFileOut, err := os.Create(path.Join(binPath, "telegraf"))
	require.NoError(t, err)
	binFileOut.Close()

	commandHandler := NewMockCommandHandler()
	ctx := context.Background()

	telegrafRunner := &agents.TelegrafRunner{}
	telegrafRunner.SetCommandHandler(commandHandler)
	viper.Set(config.IngestTelegrafJsonBind, "localhost:8094")
	err = telegrafRunner.Load(dataPath)
	require.NoError(t, err)

	///////////////////////
	// TEST CREATE
	createConfig := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_TELEGRAF,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:      "1",
				Content: "{\"type\":\"mem\"}",
				Type:    telemetry_edge.ConfigurationOp_CREATE,
			},
		},
	}
	err = telegrafRunner.ProcessConfig(createConfig)
	require.NoError(t, err)
	configs, err := ioutil.ReadDir(path.Join(dataPath, "config.d"))
	require.NoError(t, err)
	assert.Len(t, configs, 1)

	runningContext := agents.CreatePreRunningAgentRunningContext()

	pegomock.When(commandHandler.CreateContext(
		matchers.AnyContextContext(),
		matchers.AnyTelemetryEdgeAgentType(),
		pegomock.AnyString(),
		pegomock.AnyString(),
		pegomock.AnyString(), pegomock.AnyString(),
		pegomock.AnyString(), pegomock.AnyString())).
		ThenReturn(runningContext)

	telegrafRunner.EnsureRunningState(ctx, true)

	commandHandler.VerifyWasCalledOnce().
		CreateContext(matchers.AnyContextContext(),
			matchers.EqTelemetryEdgeAgentType(telemetry_edge.AgentType_TELEGRAF),
			pegomock.EqString("CURRENT/bin/telegraf"),
			pegomock.EqString(dataPath),
			pegomock.AnyString(), pegomock.AnyString(),
			pegomock.AnyString(), pegomock.AnyString())

	commandHandler.VerifyWasCalled(pegomock.Never()).
		Signal(matchers.AnyPtrToAgentsAgentRunningContext(), matchers.EqSyscallSignal(syscall.SIGHUP))

	///////////////////////
	// TEST MODIFY
	modifyConfig := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_TELEGRAF,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:      "1",
				Content: "{\"type\":\"mem\"}",
				Type:    telemetry_edge.ConfigurationOp_MODIFY,
			},
		},
	}
	err = telegrafRunner.ProcessConfig(modifyConfig)
	require.NoError(t, err)
	configs, err = ioutil.ReadDir(path.Join(dataPath, "config.d"))
	require.NoError(t, err)
	assert.Len(t, configs, 1)

	telegrafRunner.EnsureRunningState(ctx, true)

	commandHandler.VerifyWasCalledOnce().
		Signal(matchers.AnyPtrToAgentsAgentRunningContext(), matchers.EqSyscallSignal(syscall.SIGHUP))

	///////////////////////
	// TEST REMOVE
	removeConfig := &telemetry_edge.EnvoyInstructionConfigure{
		AgentType: telemetry_edge.AgentType_TELEGRAF,
		Operations: []*telemetry_edge.ConfigurationOp{
			{
				Id:   "1",
				Type: telemetry_edge.ConfigurationOp_REMOVE,
			},
		},
	}
	err = telegrafRunner.ProcessConfig(removeConfig)
	require.NoError(t, err)
	configs, err = ioutil.ReadDir(path.Join(dataPath, "config.d"))
	require.NoError(t, err)
	assert.Len(t, configs, 0)

	telegrafRunner.EnsureRunningState(ctx, true)

	commandHandler.VerifyWasCalledOnce().
		Stop(matchers.EqPtrToAgentsAgentRunningContext(runningContext))
}

func TestTelegrafRunner_EnsureRunning_MissingExe(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	dataPath, err := ioutil.TempDir("", "test_agents")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	mainConfigFile, err := os.Create(path.Join(dataPath, "telegraf.conf"))
	require.NoError(t, err)
	mainConfigFile.Close()

	err = os.Mkdir(path.Join(dataPath, "config.d"), 0755)
	require.NoError(t, err)

	specificConfigFile, err := os.Create(path.Join(dataPath, "config.d", "123.conf"))
	require.NoError(t, err)
	specificConfigFile.Close()

	mockCommandHandler := NewMockCommandHandler()

	telegrafRunner := &agents.TelegrafRunner{}
	telegrafRunner.SetCommandHandler(mockCommandHandler)
	viper.Set(config.IngestTelegrafJsonBind, "localhost:8094")
	err = telegrafRunner.Load(dataPath)
	require.NoError(t, err)

	ctx := context.Background()
	telegrafRunner.EnsureRunningState(ctx, false)

	mockCommandHandler.VerifyWasCalled(pegomock.Never()).
		StartAgentCommand(matchers.AnyPtrToAgentsAgentRunningContext(), matchers.AnyTelemetryEdgeAgentType(),
			pegomock.AnyString(), matchers.AnyTimeDuration())
	mockCommandHandler.VerifyWasCalledOnce().
		Stop(matchers.AnyPtrToAgentsAgentRunningContext())
}
