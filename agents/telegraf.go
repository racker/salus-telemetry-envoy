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
	"github.com/racker/telemetry-envoy/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"text/template"
	"time"
)

const (
	telegrafMainConfigFilename = "telegraf.conf"
)

var telegrafMainConfigTmpl = template.Must(template.New("telegrafMain").Parse(`
[agent]
  interval = "10s"
  omit_hostname = true
[[outputs.socket_writer]]
  address = "tcp://{{.IngestHost}}:{{.IngestPort}}"
  data_format = "json"
  json_timestamp_units = "1ms"
`))

var (
	telegrafStartupDuration = 10 * time.Second
)

type telegrafMainConfigData struct {
	IngestHost string
	IngestPort string
}

type TelegrafRunner struct {
	ingestHost     string
	ingestPort     string
	basePath       string
	running        *AgentRunningContext
	commandHandler CommandHandler
}

func init() {
	registerSpecificAgentRunner(telemetry_edge.AgentType_TELEGRAF, &TelegrafRunner{})
}

func (tr *TelegrafRunner) Load(agentBasePath string) error {
	ingestAddr := viper.GetString(config.IngestTelegrafJsonBind)
	host, port, err := net.SplitHostPort(ingestAddr)
	if err != nil {
		return errors.Wrap(err, "couldn't parse telegraf ingest bind")
	}
	tr.ingestHost = host
	tr.ingestPort = port
	tr.basePath = agentBasePath
	return nil
}

func (tr *TelegrafRunner) SetCommandHandler(handler CommandHandler) {
	tr.commandHandler = handler
}

func (tr *TelegrafRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {
	configsPath := path.Join(tr.basePath, configsDirSubpath)
	err := os.MkdirAll(configsPath, dirPerms)
	if err != nil {
		return errors.Wrapf(err, "failed to create configs path for telegraf: %v", configsPath)
	}

	mainConfigPath := path.Join(tr.basePath, telegrafMainConfigFilename)
	if !fileExists(mainConfigPath) {
		err = tr.createMainConfig(mainConfigPath)
		if err != nil {
			return errors.Wrap(err, "failed to create main telegraf config")
		}
	}

	applied := 0
	for _, op := range configure.GetOperations() {
		log.WithField("op", op).Debug("processing telegraf config operation")

		configInstancePath := filepath.Join(configsPath, fmt.Sprintf("%s.conf", op.GetId()))

		if handleContentConfigurationOp(op, configInstancePath, ConversionJsonToTelegrafToml) {
			applied++
		}
	}

	if applied == 0 {
		return &noAppliedConfigsError{}
	}

	return nil
}

func (tr *TelegrafRunner) EnsureRunningState(ctx context.Context, applyConfigs bool) {
	log.Debug("ensuring telegraf is in correct running state")

	if !tr.hasRequiredPaths() {
		log.Debug("telegraf not runnable due to some missing paths and files, stopping if needed")
		tr.commandHandler.Stop(tr.running)
		return
	}

	if tr.running.IsRunning() {
		log.Debug("telegraf is already running, signaling config reload")
		tr.handleConfigReload()
		return
	}

	runningContext := tr.commandHandler.CreateContext(ctx,
		telemetry_edge.AgentType_TELEGRAF,
		tr.exePath(), tr.basePath,
		"--config", telegrafMainConfigFilename,
		"--config-directory", configsDirSubpath)

	err := tr.commandHandler.StartAgentCommand(runningContext,
		telemetry_edge.AgentType_TELEGRAF,
		"Loaded inputs:", telegrafStartupDuration)
	if err != nil {
		log.WithError(err).
			WithField("agentType", telemetry_edge.AgentType_TELEGRAF).
			Warn("failed to start agent")
		return
	}

	go tr.commandHandler.WaitOnAgentCommand(ctx, tr, runningContext)

	tr.running = runningContext
	log.WithField("pid", runningContext.Pid()).
		WithField("agentType", telemetry_edge.AgentType_TELEGRAF).
		Info("started agent")
}

// exePath returns path to executable relative to baseDir
func (tr *TelegrafRunner) exePath() string {
	return filepath.Join(currentVerLink, binSubpath, "telegraf")
}

func (tr *TelegrafRunner) Stop() {
	tr.commandHandler.Stop(tr.running)
	tr.running = nil
}

func (tr *TelegrafRunner) createMainConfig(mainConfigPath string) error {
	file, err := os.OpenFile(mainConfigPath, os.O_CREATE|os.O_RDWR, configFilePerms)
	if err != nil {
		return errors.Wrap(err, "unable to open main telegraf config file")
	}
	//noinspection GoUnhandledErrorResult
	defer file.Close()

	data := &telegrafMainConfigData{
		IngestHost: tr.ingestHost,
		IngestPort: tr.ingestPort,
	}

	err = telegrafMainConfigTmpl.Execute(file, data)
	if err != nil {
		return errors.Wrap(err, "failed to execute telegraf main config template")
	}

	return nil
}

func (tr *TelegrafRunner) handleConfigReload() {
	if err := tr.commandHandler.Signal(tr.running, syscall.SIGHUP); err != nil {
		log.WithError(err).WithField("pid", tr.running.Pid()).
			Warn("failed to signal agent process")
	}
}

func (tr *TelegrafRunner) hasRequiredPaths() bool {

	mainConfigPath := path.Join(tr.basePath, telegrafMainConfigFilename)
	if !fileExists(mainConfigPath) {
		return false
	}

	configsPath := filepath.Join(tr.basePath, configsDirSubpath)
	if !fileExists(configsPath) {
		log.WithField("path", configsPath).Debug("missing configs path")
		return false
	}

	configsDir, err := os.Open(configsPath)
	if err != nil {
		log.WithError(err).Warn("unable to open configs directory for listing")
		return false
	}
	//noinspection GoUnhandledErrorResult
	defer configsDir.Close()

	names, err := configsDir.Readdirnames(0)
	if err != nil {
		log.WithError(err).WithField("path", configsPath).
			Warn("unable to read files in configs directory")
		return false
	}

	hasConfigs := false
	for _, name := range names {
		if path.Ext(name) == ".conf" {
			hasConfigs = true
		}
	}
	if !hasConfigs {
		log.WithField("path", configsPath).Debug("missing config files")
		return false
	}

	fullExePath := path.Join(tr.basePath, tr.exePath())
	if !fileExists(fullExePath) {
		log.WithField("exe", fullExePath).Debug("missing exe")
		return false
	}

	return true
}
