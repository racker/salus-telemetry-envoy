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

package agents_test

import (
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestFilebeatRunner_ProcessConfig_CreateModify(t *testing.T) {
	tests := []struct {
		opType telemetry_edge.ConfigurationOp_Type
	}{
		{opType: telemetry_edge.ConfigurationOp_CREATE},
		{opType: telemetry_edge.ConfigurationOp_MODIFY},
	}

	for _, tt := range tests {
		t.Run(tt.opType.String(), func(t *testing.T) {
			dataPath, err := ioutil.TempDir("", "filebeat_test")
			require.NoError(t, err)
			defer os.RemoveAll(dataPath)

			runner := &agents.FilebeatRunner{}
			err = runner.Load(dataPath)
			require.NoError(t, err)
			config.RegisterListenerAddress(config.LumberjackListener, "0.0.0.0:5555")

			configure := &telemetry_edge.EnvoyInstructionConfigure{
				AgentType: telemetry_edge.AgentType_FILEBEAT,
				Operations: []*telemetry_edge.ConfigurationOp{
					{
						Id:      "a-b-c",
						Type:    tt.opType,
						Content: "configuration content",
					},
				},
			}
			err = runner.ProcessConfig(configure)
			require.NoError(t, err)

			var files, mainConfigs, instanceConfigs int
			err = filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
				files++
				if filepath.Base(path) == "filebeat.yml" {
					mainConfigs++
					content, err := ioutil.ReadFile(path)
					require.NoError(t, err)

					assert.Contains(t, string(content), "path: config.d/*.yml")
					// confirm listener registration substituted bound host with loopback IP
					assert.Contains(t, string(content), "hosts: [\"127.0.0.1:5555\"]")
				} else if filepath.Base(path) == "a-b-c.yml" {
					instanceConfigs++
					content, err := ioutil.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, "configuration content", string(content))

					assert.Equal(t, "config.d", filepath.Base(filepath.Dir(path)))
				}
				return nil
			})
			require.NoError(t, err)
			assert.NotZero(t, files)
			assert.Equal(t, 1, mainConfigs)
			assert.Equal(t, 1, instanceConfigs)
		})
	}
}
