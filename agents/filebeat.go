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

package agents

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"text/template"
	"time"
)

const (
	filebeatMainConfigFilename = "filebeat.yml"
	filebeatExeName            = "filebeat"
)

type filebeatMainConfigData struct {
	ConfigsPath       string
	LumberjackAddress string
}

var filebeatMainConfigTmpl = template.Must(template.New("filebeatMain").Parse(`
filebeat.config.inputs:
  enabled: true
  path: {{.ConfigsPath}}/*.yml
  reload.enabled: true
  reload.period: 5s
output.logstash:
  hosts: ["{{.LumberjackAddress}}"]
`))

type FilebeatRunner struct {
	basePath       string
	running        *AgentRunningContext
	commandHandler CommandHandler
}

func (fbr *FilebeatRunner) PurgeConfig() error {
	return purgeConfigsDirectory(fbr.basePath)
}

func init() {
	registerSpecificAgentRunner(telemetry_edge.AgentType_FILEBEAT, &FilebeatRunner{})
}

func (fbr *FilebeatRunner) Load(agentBasePath string) error {
	fbr.basePath = agentBasePath
	return nil
}

func (fbr *FilebeatRunner) SetCommandHandler(handler CommandHandler) {
	fbr.commandHandler = handler
}

func (fbr *FilebeatRunner) PostInstall(string) error {
	return nil
}

func (fbr *FilebeatRunner) EnsureRunningState(ctx context.Context, _ bool) {
	log.Debug("ensuring filebeat is in correct running state")

	if !hasConfigsAndExeReady(fbr.basePath, filebeatExeName, ".yml") {
		log.Debug("filebeat not runnable due to some missing paths and files")
		fbr.commandHandler.Stop(fbr.running)
		return
	}

	if fbr.running.IsRunning() {
		log.Debug("filebeat is already running")
		// filebeat is configured to auto-reload config changes, so nothing extra needed
		return
	}

	runningContext := fbr.commandHandler.CreateContext(ctx,
		telemetry_edge.AgentType_FILEBEAT,
		buildRelativeExePath(filebeatExeName), fbr.basePath,
		"run",
		"--path.config", "./",
		"--path.data", "data",
		"--path.logs", "logs")

	err := fbr.commandHandler.StartAgentCommand(runningContext, telemetry_edge.AgentType_FILEBEAT, "", 0)
	if err != nil {
		log.WithError(err).
			WithField("agentType", telemetry_edge.AgentType_FILEBEAT).
			Warn("failed to start agent")
		return
	}

	go fbr.commandHandler.WaitOnAgentCommand(ctx, fbr, runningContext)

	fbr.running = runningContext
	log.Info("started filebeat")
}

func (fbr *FilebeatRunner) Stop() {
	fbr.commandHandler.Stop(fbr.running)
	fbr.running = nil
}

func (fbr *FilebeatRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {
	configsPath, err := ensureConfigsDir(fbr.basePath)
	if err != nil {
		return err
	}

	mainConfigPath := path.Join(fbr.basePath, filebeatMainConfigFilename)
	if !fileExists(mainConfigPath) {
		err = fbr.createMainConfig(mainConfigPath)
		if err != nil {
			return errors.Wrap(err, "failed to create main filebeat config")
		}
	}

	applied := 0
	for _, op := range configure.GetOperations() {
		log.WithField("op", op).Debug("processing filebeat config operation")

		configInstancePath := filepath.Join(configsPath, fmt.Sprintf("%s.yml", op.GetId()))

		if handleContentConfigurationOp(op, configInstancePath, ConversionNone) {
			applied++
		}
	}

	if applied == 0 {
		return &noAppliedConfigsError{}
	}

	return nil
}

func (fbr *FilebeatRunner) createMainConfig(mainConfigPath string) error {

	log.WithField("path", mainConfigPath).Debug("creating main filebeat config file")

	file, err := os.OpenFile(mainConfigPath, os.O_CREATE|os.O_RDWR, configFilePerms)
	if err != nil {
		return errors.Wrap(err, "unable to open main filebeat config file")
	}
	defer file.Close()

	data := filebeatMainConfigData{
		ConfigsPath:       configsDirSubpath,
		LumberjackAddress: config.GetListenerAddress(config.LumberjackListener),
	}

	err = filebeatMainConfigTmpl.Execute(file, data)
	if err != nil {
		return errors.Wrap(err, "failed to execute filebeat main config template")
	}

	return nil
}

func (fbr *FilebeatRunner) ProcessTestMonitor(string, string, time.Duration) (*telemetry_edge.TestMonitorResults, error) {
	return nil, errors.New("Test monitor not supported by filebeat agent")
}
