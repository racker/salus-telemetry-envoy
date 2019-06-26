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

package cmd

import (
	"context"
	"github.com/racker/telemetry-envoy/agents"
	"github.com/racker/telemetry-envoy/ambassador"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/ingest"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Envoy with a secure connection",
	Run: func(cmd *cobra.Command, args []string) {

		handleInterrupts(func(ctx context.Context) {

			agentsRunner, err := agents.NewAgentsRunner()
			if err != nil {
				log.WithError(err).Fatal("unable to setup agent runner")
			}

			connection, err := ambassador.NewEgressConnection(agentsRunner, ambassador.NewIdGenerator())
			if err != nil {
				log.WithError(err).Fatal("unable to setup ambassador connection")
			}

			for _, ingestor := range ingest.Ingestors() {
				err := ingestor.Bind(connection)
				if err != nil {
					log.WithError(err).WithField("ingestor", ingestor).Fatal("failed to connect ingestor")
				}
			}

			go agentsRunner.Start(ctx)
			go connection.Start(ctx, agents.SupportedAgents())

			for _, ingestor := range ingest.Ingestors() {
				go ingestor.Start(ctx)
			}
		})
	},
}

func handleInterrupts(body func(ctx context.Context)) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	rootCtx, cancel := context.WithCancel(context.Background())

	body(rootCtx)

	for {
		select {
		case <-signalChan:
			log.Info("cancelling application context")
			cancel()
		case <-rootCtx.Done():
			os.Exit(0)
		}
	}

}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().String("ca", "", "Ambassador CA certificate")
	viper.BindPFlag("tls.ca", runCmd.Flag("ca"))

	runCmd.Flags().String("cert", "", "Certificate to use for authentication")
	viper.BindPFlag("tls.cert", runCmd.Flag("cert"))

	runCmd.Flags().String("key", "", "Private key to use for authentication")
	viper.BindPFlag("tls.key", runCmd.Flag("key"))

	runCmd.Flags().String("resource-id", "", "Identifier of the resource where this Envoy is running")
	viper.BindPFlag(config.ResourceId, runCmd.Flag("resource-id"))

	runCmd.Flags().String("data-path", config.DefaultAgentsDataPath,
		"Data directory where Envoy stores downloaded agents and write agent configs")
	viper.BindPFlag(config.AgentsDataPath, runCmd.Flag("data-path"))
}
