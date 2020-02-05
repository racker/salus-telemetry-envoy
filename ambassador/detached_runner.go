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
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/telemetry-envoy/agents"
	log "github.com/sirupsen/logrus"
	"os"
)

// DetachedRunner can be used to drive an agents.Router with installation and configuration
// instructions loaded from a JSON file conveying the DetachedInstructions protobuf message.
type DetachedRunner struct {
	agentsRouter agents.Router
	unmarshaler  *jsonpb.Unmarshaler
}

func NewDetachedRunner(agentsRouter agents.Router) *DetachedRunner {
	return &DetachedRunner{
		agentsRouter: agentsRouter,
		unmarshaler: &jsonpb.Unmarshaler{
			AllowUnknownFields: true, // for forward compatibility
		},
	}
}

func (r *DetachedRunner) Load(instructionsFilePath string) error {
	log.Println("hitting the Load call")
	instructionsFile, err := os.Open(instructionsFilePath)
	if err != nil {
		return errors.Wrap(err, "Unable to open instructions file")
	}
	defer instructionsFile.Close()

	var detachedInstructions telemetry_edge.DetachedInstructions
	log.Println("unmarshalling: ", instructionsFile)
	err = r.unmarshaler.Unmarshal(instructionsFile, &detachedInstructions)
	if err != nil {
		showExampleDetachedInstructions()

		return errors.Wrap(err, "Unable to unmarshal instructions")
	}

	if len(detachedInstructions.GetInstructions()) == 0 {
		showExampleDetachedInstructions()

		return errors.New("Instructions file was empty")
	}

	for _, instruction := range detachedInstructions.GetInstructions() {
		if instruction.GetInstall() != nil {
			r.agentsRouter.ProcessInstall(instruction.GetInstall())
		} else if instruction.GetConfigure() != nil {
			log.Println("ProcessConfigure")
			r.agentsRouter.ProcessConfigure(instruction.GetConfigure())
		} else {
			log.WithField("instruction", instruction).Warn("Unexpected instruction type")
		}
	}

	return nil
}

func showExampleDetachedInstructions() {
	marshaler := jsonpb.Marshaler{
		Indent: "  ",
	}

	instructions := &telemetry_edge.DetachedInstructions{
		Instructions: []*telemetry_edge.EnvoyInstruction{
			{
				Details: &telemetry_edge.EnvoyInstruction_Install{
					Install: &telemetry_edge.EnvoyInstructionInstall{
						Agent: &telemetry_edge.Agent{
							Type:    telemetry_edge.AgentType_TELEGRAF,
							Version: "1.11.0",
						},
						Url: "https://homebrew.bintray.com/bottles/telegraf-1.11.0.high_sierra.bottle.tar.gz",
						Exe: "telegraf/1.11.0/bin/telegraf",
					},
				},
			},
			{
				Details: &telemetry_edge.EnvoyInstruction_Configure{
					Configure: &telemetry_edge.EnvoyInstructionConfigure{
						AgentType: telemetry_edge.AgentType_TELEGRAF,
						Operations: []*telemetry_edge.ConfigurationOp{
							{
								Id:      "monitor-1",
								Type:    telemetry_edge.ConfigurationOp_CREATE,
								Content: "{\"type\": \"cpu\", \"percpu\": false, \"totalcpu\": true}",
								ExtraLabels: map[string]string{
									"detached": "true",
								},
								Interval: 10,
							},
						},
					},
				},
			},
		},
	}

	output, err := marshaler.MarshalToString(instructions)
	if err != nil {
		log.WithError(err).Fatal("Unable to marshal example instructions")
	} else {
		log.Print("Example instructions json")
		fmt.Println(output)
	}
}
