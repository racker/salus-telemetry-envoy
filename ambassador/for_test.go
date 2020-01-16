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

//go:generate pegomock generate -m github.com/racker/salus-telemetry-envoy/agents Router
//go:generate pegomock generate -m github.com/racker/salus-telemetry-envoy/ambassador IdGenerator
//go:generate pegomock generate -m github.com/racker/salus-telemetry-protocol/telemetry_edge TelemetryAmbassadorServer

package ambassador_test
