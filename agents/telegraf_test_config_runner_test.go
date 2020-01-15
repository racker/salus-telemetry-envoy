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

package agents

import (
	"fmt"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

//noinspection GoUnhandledErrorResult
func TestDefaultTelegrafTestConfigRunner_StartTestConfigServer(t *testing.T) {
	tcr := telegrafTestConfigRunnerBuilder("server-1", "token-1")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	configServerErrors := make(chan error, 2)
	server := tcr.StartTestConfigServer([]byte("[[inputs.cpu]]"), configServerErrors, listener)
	require.NotNil(t, server)
	defer server.Close()

	url := fmt.Sprintf("http://127.0.0.1:%d/%s", listener.Addr().(*net.TCPAddr).Port, "server-1")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Add("Authorization", "Token token-1")

	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	respBody, err := ioutil.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "[[inputs.cpu]]", string(respBody))
	response.Body.Close()
}

func TestDefaultTelegrafTestConfigRunner_RunCommand(t *testing.T) {

	tcr := telegrafTestConfigRunnerBuilder("server-1", "token-1")

	output, err := tcr.RunCommand("127.0.0.1:65432", "./telegraf-test-config",
		"testdata", 1*time.Second)
	require.NoError(t, err)

	assert.Equal(t, "args=--test --config http://127.0.0.1:65432/server-1\nINFLUX_TOKEN=token-1\n",
		string(output))
}

func TestDefaultTelegrafTestConfigRunner_RunCommand_Timeout(t *testing.T) {

	tcr := telegrafTestConfigRunnerBuilder("server-1", "token-1")

	output, err := tcr.RunCommand("127.0.0.1:65432", "./telegraf-test-config-slow",
		"testdata", 100*time.Millisecond)
	assert.Error(t, err)
	assert.Equal(t, "signal: killed", err.Error())

	assert.Equal(t, "", string(output))
}

func TestDefaultTelegrafTestConfigRunner_RunCommand_TimeoutDefault(t *testing.T) {

	tcr := telegrafTestConfigRunnerBuilder("server-1", "token-1")

	// Use a configured-default timeout longer than the script's sleep to tell the difference from
	// previous unit test
	viper.Set(config.AgentsTestMonitorTimeout, 2*time.Second)

	output, err := tcr.RunCommand("127.0.0.1:65432", "./telegraf-test-config-slow",
		"testdata", 0)
	require.NoError(t, err)

	assert.Equal(t, "slow but finished\n", string(output))
}
