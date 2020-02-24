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

package config

import "time"

// Viper configuration keys used inter-package
const (
	AgentsDataPath                  = "agents.dataPath"
	AgentsTerminationTimeoutConfig  = "agents.terminationTimeout"
	AgentsRestartDelayConfig        = "agents.restartDelay"
	AgentsTestMonitorTimeout        = "agents.testMonitorTimeout"
	AgentsDefaultMonitoringInterval = "agents.defaultMonitoringInterval"
	AgentsMaxFlushInterval          = "agents.maxFlushInterval"
	IngestLumberjackBind            = "ingest.lumberjack.bind"
	IngestTelegrafJsonBind          = "ingest.telegraf.json.bind"
	IngestLineProtocolBind          = "ingest.lineProtocol.bind"
	AmbassadorAddress               = "ambassador.address"
	ResourceId                      = "resource_id"
	Zone                            = "zone"
	PerfTestPort                    = "perfTest.port"
	PerfTestMetricsPerMinute        = "perfTest.metricsPerMinute"
	PerfTestFloatsPerMetric         = "perfTest.metricsPerMetric"

	DefaultAgentsDataPath                  = "/var/lib/telemetry-envoy"
	DefaultAgentsTestMonitorTimeout        = 30 * time.Second
	DefaultAgentsDefaultMonitoringInterval = 60 * time.Second
	DefaultAgentsMaxFlushInterval          = 60 * time.Second
)
