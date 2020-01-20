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
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/racker/salus-telemetry-envoy/agents/matchers"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"
	"time"
)

func TestTelegrafRunner_ProcessConfig_CreateModify(t *testing.T) {
	tests := []struct {
		name             string
		opType           telemetry_edge.ConfigurationOp_Type
		interval         int64
		expectedInterval string
	}{
		{name: "create", opType: telemetry_edge.ConfigurationOp_CREATE},
		{name: "create_with_interval", opType: telemetry_edge.ConfigurationOp_CREATE, interval: 62, expectedInterval: "1m2s"},
		{name: "modify", opType: telemetry_edge.ConfigurationOp_MODIFY},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pegomock.RegisterMockTestingT(t)

			dataPath, err := ioutil.TempDir("", "telegraf_test")
			require.NoError(t, err)
			//noinspection GoUnhandledErrorResult
			defer os.RemoveAll(dataPath)

			runner := &agents.TelegrafRunner{}
			config.RegisterListenerAddress(config.TelegrafJsonListener, "localhost:8094")
			viper.Set(config.AgentsDefaultMonitoringInterval, 30*time.Second)
			viper.Set(config.AgentsMaxFlushInterval, 31*time.Second)
			err = runner.Load(dataPath)
			require.NoError(t, err)

			commandHandler := NewMockCommandHandler()
			runner.SetCommandHandler(commandHandler)

			configure := &telemetry_edge.EnvoyInstructionConfigure{
				AgentType: telemetry_edge.AgentType_TELEGRAF,
				Operations: []*telemetry_edge.ConfigurationOp{
					{
						Id:       "a-b-c",
						Type:     tt.opType,
						Content:  "{\"type\":\"mem\"}",
						Interval: tt.interval,
					},
				},
			}
			err = runner.ProcessConfig(configure)
			require.NoError(t, err)

			content, err := runner.GetCurrentConfig()
			require.NoError(t, err)

			// assert the [agent] level interval
			assert.Contains(t, string(content), "interval = \"30s\"")
			assert.Contains(t, string(content), "flush_interval = \"31s\"")
			assert.Contains(t, string(content), "outputs.socket_writer")
			assert.Contains(t, string(content), "address = \"tcp://localhost:8094\"")
			assert.Contains(t, string(content), "[inputs]\n\n  [[inputs.mem]]\n")
			// optionally assert the per plugin interval
			if tt.expectedInterval != "" {
				assert.Contains(t, string(content), fmt.Sprintf("interval = \"%s\"", tt.expectedInterval))
			}
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
	err = telegrafRunner.PurgeConfig()
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
	content, err := telegrafRunner.GetCurrentConfig()
	require.NoError(t, err)
	assert.Contains(t, string(content), "[inputs]\n\n  [[inputs.mem]]\n")

	runningContext := agents.CreatePreRunningAgentRunningContext()

	pegomock.When(commandHandler.CreateContext(
		matchers.AnyContextContext(),
		matchers.AnyTelemetryEdgeAgentType(),
		pegomock.AnyString(),
		pegomock.AnyString(),
		pegomock.AnyString(), pegomock.AnyString())).
		ThenReturn(runningContext)

	telegrafRunner.EnsureRunningState(ctx, true)

	commandHandler.VerifyWasCalledOnce().
		CreateContext(matchers.AnyContextContext(),
			matchers.EqTelemetryEdgeAgentType(telemetry_edge.AgentType_TELEGRAF),
			pegomock.EqString("CURRENT/bin/telegraf"),
			pegomock.EqString(dataPath),
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
	content, err = telegrafRunner.GetCurrentConfig()
	require.NoError(t, err)
	assert.Contains(t, string(content), "[inputs]\n\n  [[inputs.mem]]\n")

	telegrafRunner.EnsureRunningState(ctx, true)

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
	content, err = telegrafRunner.GetCurrentConfig()
	require.NoError(t, err)
	assert.NotContains(t, string(content), "[inputs]\n\n  [[inputs.mem]]\n")

	telegrafRunner.EnsureRunningState(ctx, true)

	commandHandler.VerifyWasCalled(pegomock.Times(2)).
		Signal(matchers.AnyPtrToAgentsAgentRunningContext(), matchers.EqSyscallSignal(syscall.SIGHUP))
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

func TestTelegrafRunner_Load_WebserverAuth(t *testing.T) {
	dataPath, err := ioutil.TempDir("", "test_agents")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	mockCommandHandler := NewMockCommandHandler()

	telegrafRunner := &agents.TelegrafRunner{}
	telegrafRunner.SetCommandHandler(mockCommandHandler)
	viper.Set(config.IngestTelegrafJsonBind, "localhost:8094")
	err = telegrafRunner.Load(dataPath)
	require.NoError(t, err)

	content, err := telegrafRunner.GetCurrentConfig()
	require.NoError(t, err)
	assert.Contains(t, string(content), "outputs.socket_writer")

	content, statusCode, err := telegrafRunner.GetCurrentConfigWithBadToken()
	require.NoError(t, err)
	assert.Equal(t, statusCode, http.StatusUnauthorized)
	assert.NotContains(t, string(content), "outputs.socket_writer")
	assert.Contains(t, string(content), "unauthorized")
}

func TestTelegrafRunner_ProcessTestMonitor_Normal(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	tcr := NewMockTelegrafTestConfigRunner()
	agents.RegisterTelegrafTestConfigRunnerBuilder(func(testConfigServerId string, testConfigServerToken string) agents.TelegrafTestConfigRunner {
		assert.NotEmpty(t, testConfigServerId)
		assert.NotEmpty(t, testConfigServerToken)
		return tcr
	})

	stubConfigServer := NewMockCloser()
	pegomock.When(
		tcr.StartTestConfigServer(matchers.AnySliceOfByte(), matchers.AnyChanOfError(), matchers.AnyNetListener())).
		ThenReturn(stubConfigServer)

	stdoutFile, err := os.Open("testdata/telegraf_test_config_stdout.txt")
	require.NoError(t, err)
	agentStdout, err := ioutil.ReadAll(stdoutFile)
	require.NoError(t, err)
	_ = stdoutFile.Close()

	pegomock.When(
		tcr.RunCommand(pegomock.AnyString(), pegomock.AnyString(), pegomock.AnyString(), matchers.AnyTimeDuration())).
		ThenReturn(agentStdout, nil)

	telegrafRunner := &agents.TelegrafRunner{}
	results, err := telegrafRunner.ProcessTestMonitor("correlation-1",
		`{"type":"cpu"}`, 3*time.Second)
	require.NoError(t, err)

	require.NotNil(t, results)
	assert.Equal(t, "correlation-1", results.GetCorrelationId())
	assert.Empty(t, results.GetErrors())
	assert.NotEmpty(t, results.GetMetrics())

	expectedMetricsFile, err := os.Open("testdata/expected_telegraf_test_config.txt")
	require.NoError(t, err)
	expectedMetrics, err := ioutil.ReadAll(expectedMetricsFile)
	require.NoError(t, err)
	_ = expectedMetricsFile.Close()
	// rather than do a deep equals on a data structure that would be tedious to populated, we'll
	// compare a verbose dump of the object with %+v
	assert.Equal(t, string(expectedMetrics), fmt.Sprintf("%+v", results.GetMetrics()))

	configToml, _, listener := tcr.VerifyWasCalledOnce().
		StartTestConfigServer(matchers.AnySliceOfByte(), matchers.AnyChanOfError(), matchers.AnyNetListener()).
		GetCapturedArguments()

	assert.Equal(t, "[inputs]\n\n  [[inputs.cpu]]\n", string(configToml))
	assert.NotNil(t, listener)
	//noinspection GoUnhandledErrorResult
	defer listener.Close()

	hostPort, exe, basePath, timeout := tcr.VerifyWasCalledOnce().
		RunCommand(pegomock.AnyString(), pegomock.AnyString(), pegomock.AnyString(), matchers.AnyTimeDuration()).
		GetCapturedArguments()
	assert.Contains(t, hostPort, "127.0.0.1:")
	assert.Equal(t, exe, "CURRENT/bin/telegraf")
	// empty value expected since runner's full config purposely wasn't loaded
	assert.Equal(t, basePath, "")
	assert.Equal(t, 3*time.Second, timeout)

	stubConfigServer.VerifyWasCalledOnce().
		Close()
}

func TestTelegrafRunner_ProcessTestMonitor_Errors(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	tcr := NewMockTelegrafTestConfigRunner()
	agents.RegisterTelegrafTestConfigRunnerBuilder(func(testConfigServerId string, testConfigServerToken string) agents.TelegrafTestConfigRunner {
		assert.NotEmpty(t, testConfigServerId)
		assert.NotEmpty(t, testConfigServerToken)
		return tcr
	})

	stubConfigServer := NewMockCloser()
	pegomock.When(
		tcr.StartTestConfigServer(matchers.AnySliceOfByte(), matchers.AnyChanOfError(), matchers.AnyNetListener())).
		Then(func(params []pegomock.Param) pegomock.ReturnValues {
			configServerErrors := params[1].(chan error)
			configServerErrors <- errors.New("simulated config server error")
			return pegomock.ReturnValues{stubConfigServer}
		})

	cmdError := &exec.ExitError{
		ProcessState: &os.ProcessState{},
		Stderr:       []byte("simulating stderr"),
	}

	pegomock.When(
		tcr.RunCommand(pegomock.AnyString(), pegomock.AnyString(), pegomock.AnyString(), matchers.AnyTimeDuration())).
		ThenReturn(nil, cmdError)

	telegrafRunner := &agents.TelegrafRunner{}
	results, err := telegrafRunner.ProcessTestMonitor("correlation-1",
		`{"type":"cpu"}`, 3*time.Second)
	require.NoError(t, err)

	require.NotNil(t, results)
	assert.Equal(t, "correlation-1", results.GetCorrelationId())
	assert.Empty(t, results.GetMetrics())
	assert.NotEmpty(t, results.GetErrors())
	assert.Equal(t, []string{
		"Command: exit status 0",
		"CommandStderr: simulating stderr",
		"ConfigServer: simulated config server error",
	}, results.GetErrors())

	configToml, _, listener := tcr.VerifyWasCalledOnce().
		StartTestConfigServer(matchers.AnySliceOfByte(), matchers.AnyChanOfError(), matchers.AnyNetListener()).
		GetCapturedArguments()

	assert.Equal(t, "[inputs]\n\n  [[inputs.cpu]]\n", string(configToml))
	assert.NotNil(t, listener)

	hostPort, exe, basePath, _ := tcr.VerifyWasCalledOnce().
		RunCommand(pegomock.AnyString(), pegomock.AnyString(), pegomock.AnyString(), matchers.AnyTimeDuration()).
		GetCapturedArguments()
	assert.Contains(t, hostPort, "127.0.0.1:")
	assert.Equal(t, exe, "CURRENT/bin/telegraf")
	// empty value expected since runner's full config purposely wasn't loaded
	assert.Equal(t, basePath, "")

	stubConfigServer.VerifyWasCalledOnce().
		Close()
}
