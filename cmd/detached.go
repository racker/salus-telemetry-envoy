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

package cmd

import (
	"context"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/ingest"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"time"
)

const ConfigDetachedInstructions = "detached.instructions"

var detachedCmd = &cobra.Command{
	Use:   "detached",
	Short: "Run the Envoy detached from an Ambassador with instructions provided by a local file",
	Run: func(cmd *cobra.Command, args []string) {
		handleInterrupts(func(ctx context.Context) {

			// send regular logging to stderr since the detached egress will output metrics to stdout
			log.SetOutput(os.Stderr)

			detachChan := make(chan struct{}, 1)

			// bind ingestors like normal
			for _, ingestor := range ingest.Ingestors() {
				err := ingestor.Bind()
				if err != nil {
					log.WithError(err).WithField("ingestor", ingestor).
						Fatal("failed to connect ingestor")
				}
			}

			// create agents runner/router like normal; however, nothing will actually send to the
			// given detached channel
			agentsRunner, err := agents.NewAgentsRunner(detachChan)
			if err != nil {
				log.WithError(err).Fatal("unable to setup agent runner")
			}

			// use a "stub" egress "connection" that outputs metrics to stdout
			connection := ambassador.NewStdoutEgressConnection()

			// ...and start ingestors like normal
			for _, ingestor := range ingest.Ingestors() {
				go ingestor.Start(ctx, connection)
			}

			// start everything like normal
			go agentsRunner.Start(ctx)
			go connection.Start(ctx, agents.SupportedAgents())

			// give the goroutines above a few cycles to actually be ready
			time.Sleep(100 * time.Millisecond)

			// ...but feed instructions to the agents runner from a file
			detachedRunner := ambassador.NewDetachedRunner(agentsRunner)
			err = detachedRunner.Load(viper.GetString(ConfigDetachedInstructions))
			if err != nil {
				log.WithError(err).Fatal("Unable to load detached instructions")
			}
		})

	},
}

func init() {
	rootCmd.AddCommand(detachedCmd)

	detachedCmd.Flags().String("instructions", "",
		"JSON file containing Envoy instructions")
	if err := detachedCmd.MarkFlagRequired("instructions"); err != nil {
		log.WithError(err).Fatal("viper setup of instructions failed")
	}
	if err := viper.BindPFlag(ConfigDetachedInstructions, detachedCmd.Flag("instructions")); err != nil {
		log.WithError(err).Fatal("viper setup of instructions failed")
	}
}
