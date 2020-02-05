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
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoAppliedConfigs(t *testing.T) {
	var err error
	err = agents.CreateNoAppliedConfigsError()
	assert.True(t, agents.IsNoAppliedConfigs(err))

	assert.False(t, agents.IsNoAppliedConfigs(errors.New("not ours")))
	assert.False(t, agents.IsNoAppliedConfigs(nil))
}

func readFilesIntoMap(path string, suffix string) (map[string]string, error) {
	contents := make(map[string]string)

	err := filepath.Walk(path, func(entryPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), suffix) {
			content, err := ioutil.ReadFile(entryPath)
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(path, entryPath)
			if err != nil {
				return err
			}
			contents[relPath] = string(content)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return contents, nil
}

// createFakeAgentExe creates an empty file simulating the existence of the agent's exe
func createFakeAgentExe(t *testing.T, dataPath string, exeName string) error {
	// touch the "exe" in the bin directory
	binPath := path.Join(dataPath, "CURRENT", "bin")
	err := os.MkdirAll(binPath, 0755)
	require.NoError(t, err)
	binFileOut, err := os.Create(path.Join(binPath, exeName))
	require.NoError(t, err)
	binFileOut.Close()
	return err
}
