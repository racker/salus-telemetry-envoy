/*
 * Copyright 2019 Rackspace US, Inc.
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
	"bytes"
	"context"
	"github.com/petergtz/pegomock"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/telemetry-envoy/agents"
	"github.com/racker/telemetry-envoy/agents/matchers"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func TestStandardCommandHandler_StartAgentCommand(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)

	dataPath, err := ioutil.TempDir("", "TestStandardCommandHandler_StartAgentCommand")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	markerPath := path.Join(dataPath, "marker")
	commandHandler := agents.NewCommandHandler()

	runningContext := commandHandler.CreateContext(context.Background(), telemetry_edge.AgentType_TELEGRAF,
		"./telegraf", "testdata", markerPath)

	err = commandHandler.StartAgentCommand(runningContext, telemetry_edge.AgentType_TELEGRAF, "Agent Config:", 1*time.Second)
	require.NoError(t, err)
	defer commandHandler.Stop(runningContext)

	assert.FileExists(t, markerPath)

	assert.NotZero(t, logBuffer.Len())
	assert.Contains(t, logBuffer.String(), "Agent Config:")
}

func TestStandardCommandHandler_StartAgentCommand_NoWaitFor(t *testing.T) {
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)

	dataPath, err := ioutil.TempDir("", "TestStandardCommandHandler_StartAgentCommand_NoWaitFor")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	markerPath := path.Join(dataPath, "marker")
	commandHandler := agents.NewCommandHandler()

	runningContext := commandHandler.CreateContext(context.Background(), telemetry_edge.AgentType_FILEBEAT,
		"./filebeat", "testdata", markerPath)

	err = commandHandler.StartAgentCommand(runningContext, telemetry_edge.AgentType_FILEBEAT, "", 0)
	require.NoError(t, err)
	defer commandHandler.Stop(runningContext)

	sawMarker := make(chan struct{})
	go func() {
		for {
			if strings.Contains(logBuffer.String(), "Created marker file") {
				close(sawMarker)
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()

	select {
	case <-time.After(1 * time.Second):
		t.Fail()
	case <-sawMarker:
		// good
	}

	assert.FileExists(t, markerPath)
}

func TestStandardCommandHandler_Stop_IgnoresTerm(t *testing.T) {
	dataPath, err := ioutil.TempDir("", "TestStandardCommandHandler_Stop_IgnoresTerm")
	require.NoError(t, err)
	defer os.RemoveAll(dataPath)

	commandHandler := agents.NewCommandHandler()

	runningContext := commandHandler.CreateContext(context.Background(), telemetry_edge.AgentType_FILEBEAT,
		"./ignores_sigterm", "testdata")

	err = commandHandler.StartAgentCommand(runningContext, telemetry_edge.AgentType_FILEBEAT, "", 0)
	require.NoError(t, err)

	agents.SetAgentTerminationTimeout(10 * time.Millisecond)
	commandHandler.Stop(runningContext)

	agents.WaitOnAgentRunningContextStopped(t, runningContext, 1*time.Second)
}

func TestStandardCommandHandler_WaitOnAgentCommand(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	agents.SetAgentRestartDelay(1 * time.Millisecond)

	commandHandler := agents.NewCommandHandler()

	ctx := context.Background()
	agentRunner := NewMockSpecificAgentRunner()

	runningContext := commandHandler.CreateContext(context.Background(), telemetry_edge.AgentType_FILEBEAT,
		"./sleep_a_little", "testdata")

	err := agents.RunAgentRunningContext(runningContext)
	require.NoError(t, err)

	commandHandler.WaitOnAgentCommand(ctx, agentRunner, runningContext)

	agentRunner.VerifyWasCalledEventually(
		pegomock.Once(),
		10*time.Millisecond,
	).EnsureRunningState(matchers.AnyContextContext(), pegomock.EqBool(false))
}
