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
	"github.com/petergtz/pegomock"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSupportedAgents(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	runner1 := NewMockSpecificAgentRunner()
	agents.RegisterAgentRunnerForTesting(telemetry_edge.AgentType_FILEBEAT, runner1)

	runner2 := NewMockSpecificAgentRunner()
	agents.RegisterAgentRunnerForTesting(telemetry_edge.AgentType_TELEGRAF, runner2)

	supportedAgents := agents.SupportedAgents()
	assert.ElementsMatch(t, supportedAgents, []telemetry_edge.AgentType{
		telemetry_edge.AgentType_TELEGRAF,
		telemetry_edge.AgentType_FILEBEAT,
	})
}
