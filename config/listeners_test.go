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

package config_test

import (
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRegisterListenerAddress(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected string
	}{
		{name: "localhost", address: "localhost:1234", expected: "localhost:1234"},
		{name: "bindall", address: "0.0.0.0:1234", expected: "127.0.0.1:1234"},
		{name: "external", address: "10.1.2.3:1234", expected: "10.1.2.3:1234"},
		{name: "bindall_zeroes", address: "10.0.0.0:1234", expected: "10.0.0.0:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.RegisterListenerAddress(tt.name, tt.address)
			address := config.GetListenerAddress(tt.name)
			assert.Equal(t, tt.expected, address)
		})
	}
}
