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

package ambassador

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
)

type stdoutEgressConnection struct {
	marshaler *jsonpb.Marshaler
}

// NewStdoutEgressConnection creates a "stubbed" version of EgressConnection that simply outputs
// posted metrics and logs rather than actually connecting to an Ambasssador. This is useful for
// testing in a detached mode.
func NewStdoutEgressConnection() EgressConnection {
	return &stdoutEgressConnection{
		marshaler: &jsonpb.Marshaler{},
	}
}

func (e *stdoutEgressConnection) Start(ctx context.Context, supportedAgents []telemetry_edge.AgentType) {
}

func (e *stdoutEgressConnection) PostLogEvent(agentType telemetry_edge.AgentType, jsonContent string) {
	fmt.Println(jsonContent)
}

func (e *stdoutEgressConnection) PostMetric(metric *telemetry_edge.Metric) {
	content, err := e.marshaler.MarshalToString(metric)
	if err != nil {
		log.WithError(err).
			WithField("metric", metric).
			Warn("Unable to marshal metric")
	}

	fmt.Println(content)
}

func (e *stdoutEgressConnection) PostTestMonitorResults(results *telemetry_edge.TestMonitorResults) {
	content, err := e.marshaler.MarshalToString(results)
	if err != nil {
		log.WithError(err).
			WithField("results", results).
			Warn("Unable to marshal test monitor results")
	}

	fmt.Println(content)
}
