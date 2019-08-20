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
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/telemetry-envoy/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
)

// TelegrafTestConfigRunner encapsulates the process-spawning aspects of handling telegraf's --test
// mode of running a configuration one-shot
type TelegrafTestConfigRunner interface {
	StartTestConfigServer(configToml []byte, configServerErrors chan error, listener net.Listener) io.Closer
	RunCommand(hostPort string, exePath string, basePath string) ([]byte, error)
}

type TelegrafTestConfigRunnerBuilder func(testConfigServerId string, testConfigServerToken string) TelegrafTestConfigRunner

// This builder function variable enables unit test mocking via RegisterTelegrafTestConfigRunnerBuilder
var telegrafTestConfigRunnerBuilder TelegrafTestConfigRunnerBuilder = func(testConfigServerId string, testConfigServerToken string) TelegrafTestConfigRunner {
	return &defaultTelegrafTestConfigRunner{
		testConfigServerId:    testConfigServerId,
		testConfigServerToken: testConfigServerToken,
	}
}

type defaultTelegrafTestConfigRunner struct {
	testConfigServerId    string
	testConfigServerToken string
}

func (tcr *defaultTelegrafTestConfigRunner) StartTestConfigServer(configToml []byte, configServerErrors chan error, listener net.Listener) io.Closer {
	configServerHandlers := http.NewServeMux()
	configServerHandlers.HandleFunc("/"+tcr.testConfigServerId, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Token "+tcr.testConfigServerToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		_, err := w.Write(configToml)
		if err != nil {
			configServerErrors <- errors.Wrap(err, "failed to write config TOML response")
		}
	})
	configServer := &http.Server{Handler: configServerHandlers}
	go func() {
		logrus.Debug("started test monitor config server")
		err := configServer.Serve(listener)
		// err is always non-nil from Serve
		// ...but ignore error due to us closing the server
		if err != http.ErrServerClosed {
			configServerErrors <- err
		}
	}()

	return configServer
}

func (tcr *defaultTelegrafTestConfigRunner) RunCommand(hostPort string, exePath string, basePath string) ([]byte, error) {
	testConfigServerUrl := fmt.Sprintf("http://%s/%s", hostPort, tcr.testConfigServerId)
	cmdCtx, _ := context.WithTimeout(context.Background(), viper.GetDuration(config.AgentsTestMonitorTimeout))
	cmd := exec.CommandContext(cmdCtx, exePath,
		"--test",
		"--config", testConfigServerUrl)
	cmd.Dir = basePath
	cmd.Env = append(os.Environ(), "INFLUX_TOKEN="+tcr.testConfigServerToken)
	cmd.Stderr = nil
	// blocking run of telegraf test
	cmdOut, err := cmd.Output()
	return cmdOut, err
}
