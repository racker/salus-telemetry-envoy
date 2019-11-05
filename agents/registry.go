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

package agents

import "github.com/racker/salus-telemetry-protocol/telemetry_edge"

var specificAgentRunners = make(map[telemetry_edge.AgentType]SpecificAgentRunner)

func registerSpecificAgentRunner(agentType telemetry_edge.AgentType, runner SpecificAgentRunner) {
	specificAgentRunners[agentType] = runner
}

func SupportedAgents() []telemetry_edge.AgentType {
	types := make([]telemetry_edge.AgentType, 0, len(specificAgentRunners))

	for agentType := range specificAgentRunners {
		types = append(types, agentType)
	}

	return types
}
