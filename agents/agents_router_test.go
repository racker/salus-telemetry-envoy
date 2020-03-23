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
	"github.com/petergtz/pegomock"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/racker/salus-telemetry-envoy/agents/matchers"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"
)

func TestAgentsRunner_ProcessInstall(t *testing.T) {
	var tests = []struct {
		name      string
		agentType telemetry_edge.AgentType
		file      string
		version   string
		exe       string
	}{
		{
			name:      "telegraf_linux",
			agentType: telemetry_edge.AgentType_TELEGRAF,
			file:      "telegraf_dot_slash.tgz",
			version:   "1.8.0",
			exe:       "./telegraf/usr/bin/telegraf",
		},
		{
			name:      "filebeat_linux",
			agentType: telemetry_edge.AgentType_FILEBEAT,
			file:      "filebeat_relative.tgz",
			version:   "6.4.1",
			exe:       "filebeat-6.4.1-linux-x86_64/filebeat",
		},
	}

	ts := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer ts.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pegomock.RegisterMockTestingT(t)

			agents.UnregisterAllAgentRunners()

			dataPath, err := ioutil.TempDir("", "test_agents")
			require.NoError(t, err)
			defer os.RemoveAll(dataPath)
			viper.Set(config.AgentsDataPath, dataPath)

			detachChan := make(chan struct{}, 1)
			agentsRunner, err := agents.NewAgentsRunner(detachChan)
			require.NoError(t, err)
			require.NotNil(t, agentsRunner)

			mockSpecificAgentRunner := NewMockSpecificAgentRunner()
			agents.RegisterAgentRunnerForTesting(tt.agentType, mockSpecificAgentRunner)

			install := &telemetry_edge.EnvoyInstructionInstall{
				Url: ts.URL + "/" + tt.file,
				Exe: tt.exe,
				Agent: &telemetry_edge.Agent{
					Version: tt.version,
					Type:    tt.agentType,
				},
			}

			agentsRunner.ProcessInstall(install)

			expectedAgentVersionPath := path.Join(dataPath, "agents", tt.agentType.String(), tt.version)
			mockSpecificAgentRunner.VerifyWasCalledOnce().PostInstall(expectedAgentVersionPath)

			mockSpecificAgentRunner.VerifyWasCalledOnce().EnsureRunningState(matchers.AnyContextContext(), pegomock.EqBool(false))

			_, exeFilename := path.Split(tt.exe)
			assert.FileExists(t, path.Join(dataPath, "agents", tt.agentType.String(), tt.version, "bin", exeFilename))
		})
	}
}

func TestAgentsRunner_ProcessInstall_linkHandling(t *testing.T) {
	const agentType = telemetry_edge.AgentType_TELEGRAF
	agentTypeStr := agentType.String()
	const (
		initialVersion = "1.0.0"
		newerVersion   = "2.0.0"
	)

	agents.UnregisterAllAgentRunners()

	ts := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer ts.Close()

	dataPath, err := ioutil.TempDir("", "test_agents")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)
	viper.Set(config.AgentsDataPath, dataPath)

	detachChan := make(chan struct{}, 1)
	agentsRunner, err := agents.NewAgentsRunner(detachChan)
	require.NoError(t, err)
	require.NotNil(t, agentsRunner)

	installInitial := &telemetry_edge.EnvoyInstructionInstall{
		Url: ts.URL + "/" + "telegraf_dot_slash.tgz",
		Exe: "./telegraf/usr/bin/telegraf",
		Agent: &telemetry_edge.Agent{
			Version: initialVersion,
			Type:    agentType,
		},
	}
	installNewer := &telemetry_edge.EnvoyInstructionInstall{
		Url: ts.URL + "/" + "telegraf_dot_slash.tgz",
		Exe: "./telegraf/usr/bin/telegraf",
		Agent: &telemetry_edge.Agent{
			Version: newerVersion,
			Type:    agentType,
		},
	}

	verifyPaths := func(version string) {
		assert.FileExists(t, path.Join(dataPath, "agents", agentTypeStr, "CURRENT", "bin", "telegraf"))
		assert.FileExists(t, path.Join(dataPath, "agents", agentTypeStr, version, "bin", "telegraf"))

		linkTarget, err := os.Readlink(path.Join(dataPath, "agents", agentTypeStr, "CURRENT"))
		require.NoError(t, err)
		assert.Equal(t, version, linkTarget)
	}

	// Sub-test to scope mock
	t.Run("initial", func(t *testing.T) {
		pegomock.RegisterMockTestingT(t)

		mockSpecificAgentRunner := NewMockSpecificAgentRunner()

		agents.RegisterAgentRunnerForTesting(agentType, mockSpecificAgentRunner)

		agentsRunner.ProcessInstall(installInitial)

		mockSpecificAgentRunner.VerifyWasCalledOnce().EnsureRunningState(matchers.AnyContextContext(), pegomock.EqBool(false))

		mockSpecificAgentRunner.VerifyWasCalledOnce().PostInstall(
			path.Join(dataPath, "agents", agentTypeStr, initialVersion))

		mockSpecificAgentRunner.VerifyWasCalledOnce().Stop()

		verifyPaths(initialVersion)
	})

	t.Run("same", func(t *testing.T) {
		pegomock.RegisterMockTestingT(t)

		mockSpecificAgentRunner := NewMockSpecificAgentRunner()

		agents.RegisterAgentRunnerForTesting(agentType, mockSpecificAgentRunner)

		agentsRunner.ProcessInstall(installInitial)

		mockSpecificAgentRunner.VerifyWasCalledOnce().EnsureRunningState(matchers.AnyContextContext(), pegomock.EqBool(false))

		// Verify NOT CALLED
		mockSpecificAgentRunner.VerifyWasCalled(pegomock.Never()).PostInstall(
			path.Join(dataPath, "agents", agentTypeStr, initialVersion))

		// Verify NOT CALLED
		mockSpecificAgentRunner.VerifyWasCalled(pegomock.Never()).Stop()

		verifyPaths(initialVersion)
	})

	t.Run("newer", func(t *testing.T) {
		pegomock.RegisterMockTestingT(t)

		mockSpecificAgentRunner := NewMockSpecificAgentRunner()

		agents.RegisterAgentRunnerForTesting(agentType, mockSpecificAgentRunner)

		agentsRunner.ProcessInstall(installNewer)

		mockSpecificAgentRunner.VerifyWasCalledOnce().EnsureRunningState(matchers.AnyContextContext(), pegomock.EqBool(false))

		mockSpecificAgentRunner.VerifyWasCalledOnce().PostInstall(
			path.Join(dataPath, "agents", agentTypeStr, newerVersion))

		mockSpecificAgentRunner.VerifyWasCalledOnce().Stop()

		verifyPaths(newerVersion)
	})
}

func TestAgentsRunner_Detach(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	agents.UnregisterAllAgentRunners()

	dataPath, err := ioutil.TempDir("", "test_agents")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)
	viper.Set(config.AgentsDataPath, dataPath)

	detachChan := make(chan struct{}, 1)
	agentsRunner, err := agents.NewAgentsRunner(detachChan)
	require.NoError(t, err)
	require.NotNil(t, agentsRunner)

	mockSpecificAgentRunner := NewMockSpecificAgentRunner()
	agents.RegisterAgentRunnerForTesting(telemetry_edge.AgentType_TELEGRAF, mockSpecificAgentRunner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Log("starting agents runner")
	go agentsRunner.Start(ctx)

	t.Log("sending detach signal")
	detachChan <- struct{}{}

	mockSpecificAgentRunner.
		VerifyWasCalledEventually(pegomock.Times(1), 100*time.Millisecond).
		PurgeConfig()
	mockSpecificAgentRunner.
		VerifyWasCalledEventually(pegomock.Times(1), 100*time.Millisecond).
		Stop()
}
