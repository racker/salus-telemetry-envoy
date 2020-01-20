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

import (
	"strings"
	"sync"
)

const (
	TelegrafJsonListener = "telegraf-json"
	LumberjackListener   = "lumberjack"
)

var listenerAddresses sync.Map

func RegisterListenerAddress(name string, address string) {
	// The given address is relative to the binding, so if an ingestor has been configured to
	// listen on all interfaces with 0.0.0.0, such as for accepting telemetry from
	// non-envoy-running hosts, then the agent needs to be told the loopback address to dial.
	listenerAddresses.Store(name, strings.Replace(address, "0.0.0.0", "127.0.0.1", 1))
}

func GetListenerAddress(name string) string {
	if address, ok := listenerAddresses.Load(name); ok {
		return address.(string)
	} else {
		return ""
	}
}
