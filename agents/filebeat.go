/*
 *    Copyright 2018 Rackspace US, Inc.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 *
 */

package agents

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
	"os"
	"path"
	"path/filepath"
	"text/template"
)

const (
	filebeatMainConfigFilename = "filebeat.yml"
)

type filebeatMainConfigData struct {
	ConfigsPath    string
	LumberjackPort string
}

var filebeatMainConfigTmpl = template.Must(template.New("filebeatMain").Parse(`
filebeat.config.inputs:
  enabled: true
  path: {{.ConfigsPath}}/*.yml
  reload.enabled: true
  reload.period: 5s
output.logstash:
  hosts: ["localhost:{{.LumberjackPort}}"]
`))

type FilebeatRunner struct {
	LumberjackBind string
	basePath       string
	running        *AgentRunningContext
	commandHandler CommandHandler
}

func init() {
	registerSpecificAgentRunner(telemetry_edge.AgentType_FILEBEAT, &FilebeatRunner{})
}

func (fbr *FilebeatRunner) Load(agentBasePath string) error {
	fbr.basePath = agentBasePath
	fbr.LumberjackBind = viper.GetString(config.IngestLumberjackBind)
	return nil
}

func (fbr *FilebeatRunner) SetCommandHandler(handler CommandHandler) {
	fbr.commandHandler = handler
}

func (fbr *FilebeatRunner) EnsureRunningState(ctx context.Context, applyConfigs bool) {
	log.Debug("ensuring filebeat is in correct running state")

	if !fbr.hasRequiredPaths() {
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
		fbr.exePath(), fbr.basePath,
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
	configsPath := path.Join(fbr.basePath, configsDirSubpath)
	err := os.MkdirAll(configsPath, dirPerms)
	if err != nil {
		return errors.Wrapf(err, "failed to create configs path for filebeat: %v", configsPath)
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

	_, port, err := net.SplitHostPort(fbr.LumberjackBind)
	if err != nil {
		return errors.Wrapf(err, "unable to split lumberjack bind info: %v", fbr.LumberjackBind)
	}

	file, err := os.OpenFile(mainConfigPath, os.O_CREATE|os.O_RDWR, configFilePerms)
	if err != nil {
		return errors.Wrap(err, "unable to open main filebeat config file")
	}
	defer file.Close()

	data := filebeatMainConfigData{
		ConfigsPath:    configsDirSubpath,
		LumberjackPort: port,
	}

	err = filebeatMainConfigTmpl.Execute(file, data)
	if err != nil {
		return errors.Wrap(err, "failed to execute filebeat main config template")
	}

	return nil
}

func (fbr *FilebeatRunner) hasRequiredPaths() bool {
	curVerPath := filepath.Join(fbr.basePath, currentVerLink)
	if !fileExists(curVerPath) {
		log.WithField("path", curVerPath).Debug("missing current version link")
		return false
	}

	configsPath := filepath.Join(fbr.basePath, configsDirSubpath)
	if !fileExists(configsPath) {
		log.WithField("path", configsPath).Debug("missing configs path")
		return false
	}

	configsDir, err := os.Open(configsPath)
	if err != nil {
		log.WithError(err).Warn("unable to open configs directory for listing")
		return false
	}
	defer configsDir.Close()

	names, err := configsDir.Readdirnames(0)
	if err != nil {
		log.WithError(err).WithField("path", configsPath).
			Warn("unable to read files in configs directory")
		return false
	}

	hasConfigs := false
	for _, name := range names {
		if path.Ext(name) == ".yml" {
			hasConfigs = true
		}
	}
	if !hasConfigs {
		log.WithField("path", configsPath).Debug("missing config files")
		return false
	}

	fullExePath := path.Join(fbr.basePath, fbr.exePath())
	if !fileExists(fullExePath) {
		log.WithField("exe", fullExePath).Debug("missing exe")
		return false
	}

	return true
}

// exePath returns path to executable relative to baseDir
func (fbr *FilebeatRunner) exePath() string {
	return filepath.Join(currentVerLink, binSubpath, "filebeat")
}
