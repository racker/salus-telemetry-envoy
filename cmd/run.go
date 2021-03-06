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
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Envoy with a secure connection",
	Run: func(cmd *cobra.Command, args []string) {

		handleInterrupts(func(ctx context.Context) {

			detachChan := make(chan struct{}, 1)

			// Bind ingestors before agents created since they need the resolved, bound address
			// of each ingestor
			for _, ingestor := range ingest.Ingestors() {
				err := ingestor.Bind()
				if err != nil {
					log.WithError(err).WithField("ingestor", ingestor).Fatal("failed to connect ingestor")
				}
			}

			agentsRunner, err := agents.NewAgentsRunner(detachChan)
			if err != nil {
				log.WithError(err).Fatal("unable to setup agent runner")
			}

			connection, err := ambassador.NewEgressConnection(agentsRunner, detachChan, ambassador.NewIdGenerator(),
				ambassador.NewNetworkDialOptionCreator())
			if err != nil {
				log.WithError(err).Fatal("unable to setup ambassador connection")
			}

			for _, ingestor := range ingest.Ingestors() {
				go ingestor.Start(ctx, connection)
			}

			go agentsRunner.Start(ctx)
			go connection.Start(ctx, agents.SupportedAgents())
		})
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("ca", "", "Ambassador CA certificate")
	viper.BindPFlag("tls.ca", runCmd.Flag("ca"))

	runCmd.Flags().String("cert", "", "Certificate to use for authentication")
	viper.BindPFlag("tls.cert", runCmd.Flag("cert"))

	runCmd.Flags().String("key", "", "Private key to use for authentication")
	viper.BindPFlag("tls.key", runCmd.Flag("key"))
}
