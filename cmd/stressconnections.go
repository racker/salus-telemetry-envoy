// +build dev

// NOTE: that build tag ^ ensures that end users won't get this dev-only command
// in the published Envoy.

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
	"fmt"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"math/rand"
	"time"
)

var stressConnectionsCmd = &cobra.Command{
	Use:   "stress-connections",
	Short: "Run the Envoy in a connection-only mode with simulated metrics posted",
	Run: func(cmd *cobra.Command, args []string) {
		handleInterrupts(func(ctx context.Context) {
			if viper.GetBool("stress.quiet") {
				log.SetLevel(log.WarnLevel)
			}

			idGenerator := ambassador.NewIdGenerator()

			networkDialOptionCreator := ambassador.NewNetworkDialOptionCreator()

			connectionCount := viper.GetInt("stress.connection-count")
			connectionsDelay := viper.GetDuration("stress.connections-delay")
			resourcePrefix := viper.GetString("stress.resource-prefix")

			// resource ID gets explicitly overridden in each connection below; however,
			// the NewEgressConnection constructor still expects the property to be set
			viper.Set(config.ResourceId, "__unused__")

			connections := make([]ambassador.EgressConnection, connectionCount)
			for i := 0; i < connectionCount; i++ {
				connection, err := ambassador.NewEgressConnection(
					&mockAgentsRouter{},
					nil,
					idGenerator,
					networkDialOptionCreator,
				)
				if err != nil {
					log.Fatal(err)
				}
				resourceId := fmt.Sprintf("%s-%04d", resourcePrefix, i)
				ambassador.SetResourceId(connection, resourceId)

				go connection.Start(ctx, []telemetry_edge.AgentType{
					telemetry_edge.AgentType_TELEGRAF,
				})

				connections[i] = connection

				// simulate a bound monitor generating fabricated metrics
				monitorId := uuid.NewV4().String()
				go sendMetrics(ctx, resourceId, monitorId, connection)

				time.Sleep(connectionsDelay)
			}

		})

	},
}

func init() {
	rootCmd.AddCommand(stressConnectionsCmd)

	stressConnectionsCmd.Flags().Int("connection-count", 5,
		"Number of Ambassador connections to setup")
	viper.BindPFlag("stress.connection-count", stressConnectionsCmd.Flag("connection-count"))

	stressConnectionsCmd.Flags().Duration("connections-delay", 0,
		"Amount of time to wait between the start of each connection. Default/0 allows for a stampede and longer allows for gradual ramp-up.")
	viper.BindPFlag("stress.connections-delay", stressConnectionsCmd.Flag("connections-delay"))

	stressConnectionsCmd.Flags().Int("metrics-per-minute", 20,
		"Number of metrics to post per minute per connection")
	viper.BindPFlag("stress.metrics-per-minute", stressConnectionsCmd.Flag("metrics-per-minute"))

	stressConnectionsCmd.Flags().Bool("stagger-metrics", true,
		"When true, delay the initial metric sent by each connection by a random amount of the interval")
	viper.BindPFlag("stress.stagger-metrics", stressConnectionsCmd.Flag("stagger-metrics"))

	stressConnectionsCmd.Flags().String("resource-prefix", "resource",
		"The prefix of the resources that will be simulated which will be following by dash and an index")
	viper.BindPFlag("stress.resource-prefix", stressConnectionsCmd.Flag("resource-prefix"))

	stressConnectionsCmd.Flags().Bool("quiet", true,
		"Only show warning level logs and above")
	viper.BindPFlag("stress.quiet", stressConnectionsCmd.Flag("quiet"))
}

func sendMetrics(ctx context.Context, resourceId string, monitorId string, connection ambassador.EgressConnection) {
	metricsPerMinute := viper.GetInt("stress.metrics-per-minute")

	interval := time.Minute / time.Duration(metricsPerMinute)

	if viper.GetBool("stress.stagger-metrics") {
		// sleep a random part of one interval
		time.Sleep(time.Duration(
			rand.Int63n(int64(interval)),
		))
	}

	ticker := time.NewTicker(interval)

	for {
		select {
		case <-ctx.Done():
			return

		case ts := <-ticker.C:
			sendMetric(ts, resourceId, monitorId, connection)
		}
	}
}

func sendMetric(ts time.Time, resourceId string, monitorId string, connection ambassador.EgressConnection) {
	fvalues := make(map[string]float64)
	fvalues["duration"] = rand.Float64() * 1000.0
	tags := make(map[string]string)
	// simulate extra labels injected by ambassador during monitor config instruction building
	tags["monitor_id"] = monitorId
	tags["monitor_type"] = "stress"
	// this tag is mostly here so that debug logs reveal what resource would be "posting" each metric
	tags["simulating_resource"] = resourceId
	metric := &telemetry_edge.Metric{
		Variant: &telemetry_edge.Metric_NameTagValue{
			NameTagValue: &telemetry_edge.NameTagValueMetric{
				Name:      "stress_connection",
				Timestamp: ts.Unix() * 1000,
				Tags:      tags,
				Fvalues:   fvalues,
			},
		},
	}
	connection.PostMetric(metric)
}

type mockAgentsRouter struct {
}

func (m *mockAgentsRouter) Start(ctx context.Context) {
}

func (m *mockAgentsRouter) ProcessInstall(install *telemetry_edge.EnvoyInstructionInstall) {
	log.WithField("instruction", install).Info("Ignoring install")
}

func (m *mockAgentsRouter) ProcessConfigure(configure *telemetry_edge.EnvoyInstructionConfigure) {
	log.WithField("instruction", configure).Info("Ignoring configure")
}

func (m *mockAgentsRouter) ProcessTestMonitor(testMonitor *telemetry_edge.EnvoyInstructionTestMonitor) *telemetry_edge.TestMonitorResults {
	log.WithField("testMonitor", testMonitor).Info("Ignoring test-monitor")
	return nil
}
