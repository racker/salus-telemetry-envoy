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
	"log"
	"net"
	"sync"
)

const (
	TelegrafJsonListener = "telegraf-json"
	LumberjackListener   = "lumberjack"
)

var listenerAddresses sync.Map

func RegisterListenerAddress(name string, address string) {
	// The given address is relative to the binding, so it might contain the special "all interfaces"
	// address of 0.0.0.0. As such, we need to pick apart and rebuild an address suitable for
	// telling local agents how to connect to the given bound address.
	// This will allow a server without an envoy to send its metrics to a
	// different server's envoy/ingestor.

	_, port, err := net.SplitHostPort(address)
	if err != nil {
		log.Panic(err)
	}

	listenerAddresses.Store(name, "127.0.0.1:"+port)
}

func GetListenerAddress(name string) string {
	if address, ok := listenerAddresses.Load(name); ok {
		return address.(string)
	} else {
		return ""
	}
}
