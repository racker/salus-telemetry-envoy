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

package agents_test

import (
	"fmt"
	"github.com/racker/telemetry-envoy/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestConvertJsonToToml(t *testing.T) {
	// NOTE: the given JSON content is provided by the testdata/TestConvertJsonToToml_{name}.json files
	// NOTE: the expected TOML content is provided by the testdata/TestConvertJsonToToml_{name}.toml files
	tests := []struct {
		name        string
		extraLabels map[string]string
	}{
		{name: "cpu"},
		{name: "disk"},
		{name: "diskio"},
		{name: "mem"},
		{name: "ping", extraLabels: map[string]string{
			"target_tenant": "t-1",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			jsonFile, err := os.Open(path.Join("testdata",
				fmt.Sprintf("TestConvertJsonToToml_%s.json", tc.name)))
			require.NoError(t, err)
			defer jsonFile.Close()
			jsonBytes, err := ioutil.ReadAll(jsonFile)
			require.NoError(t, err)

			result, err := agents.ConvertJsonToTelegrafToml(string(jsonBytes), tc.extraLabels)
			require.NoError(t, err)

			expectedFile, err := os.Open(path.Join("testdata",
				fmt.Sprintf("TestConvertJsonToToml_%s.toml", tc.name)))
			require.NoError(t, err)
			defer expectedFile.Close()
			expected, err := ioutil.ReadAll(expectedFile)
			require.NoError(t, err)

			assert.Equal(t, string(expected), string(result))
		})
	}
}
