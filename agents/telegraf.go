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
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"syscall"
	"text/template"
	"time"
)

var telegrafMainConfigTmpl = template.Must(template.New("telegrafMain").Parse(`
[agent]
  interval = "10s"
  flush_interval = "10s"
  flush_jitter = "2s"
  omit_hostname = true
[[outputs.socket_writer]]
  address = "tcp://{{.IngestHost}}:{{.IngestPort}}"
  data_format = "json"
  json_timestamp_units = "1ms"
[[inputs.internal]]
  collect_memstats = false
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
	configServerMux *http.ServeMux
	configServerURL string
	configServerToken string
	configServerHandler http.HandlerFunc
	tomlMainConfig []byte
	// tomlConfigs key is the "bound monitor id", i.e. monitorId_resourceId
	tomlConfigs map[string][]byte
}

func (tr *TelegrafRunner) PurgeConfig() error {
	tr.tomlConfigs = make(map[string][]byte)
	return nil
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
	tr.configServerToken = uuid.NewV4().String()
	tr.configServerHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Token " + tr.configServerToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, err = w.Write(tr.concatConfigs())
		if err != nil {
			log.Errorf("Error writing config page %v", err)
		}
	}

	serverId := uuid.NewV4().String()
	tr.configServerMux = http.NewServeMux()
	tr.configServerMux.Handle("/" + serverId, tr.configServerHandler)

	// Get the next available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return errors.Wrap(err, "couldn't create http listener")
	}
	listenerPort := listener.Addr().(*net.TCPAddr).Port
	tr.configServerURL = fmt.Sprintf("http://localhost:%d/%s", listenerPort, serverId)

	tr.tomlConfigs = make(map[string][]byte)
	mainConfig, err := tr.createMainConfig()
	if err != nil {
		return errors.Wrap(err, "couldn't create main config")
	}

	tr.tomlMainConfig = mainConfig
	go tr.serve(listener)
	return nil
}

func (tr *TelegrafRunner) serve(listener net.Listener) {
	log.Info("started webServer")
	err := http.Serve(listener, tr.configServerMux)
	// Note this is probably not the best way to handle webserver failure
	log.Fatalf("web server error %v", err)
}

func (tr *TelegrafRunner) SetCommandHandler(handler CommandHandler) {
	tr.commandHandler = handler
}

func (tr *TelegrafRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {
	applied := 0
	for _, op := range configure.GetOperations() {
		log.WithField("op", op).Debug("processing telegraf config operation")

		if tr.handleTelegrafConfigurationOp(op) {
			applied++
		}
	}

	if applied == 0 {
		return &noAppliedConfigsError{}
	}

	return nil
}

func (tr *TelegrafRunner) concatConfigs() []byte {
	var configs []byte
	configs = append(configs, tr.tomlMainConfig...)
	// telegraf can only handle one 'inputs' header per file so add exactly one here
	configs = append(configs, []byte("[inputs]")...)
	for _, v := range tr.tomlConfigs {
		// remove the other redundant '[inputs]' headers here
		if bytes.Equal([]byte("[inputs]"),v[0:8]) {
			v = v[8:]
		}
		configs = append(configs, v...)
	}
	return configs
}

func (tr *TelegrafRunner) handleTelegrafConfigurationOp(op *telemetry_edge.ConfigurationOp) bool {
	switch op.GetType() {
	case telemetry_edge.ConfigurationOp_CREATE, telemetry_edge.ConfigurationOp_MODIFY:
		var finalConfig []byte
		var err error
		finalConfig, err = ConvertJsonToTelegrafToml(op.GetContent(), op.ExtraLabels)
		if err != nil {
			log.WithError(err).WithField("op", op).Warn("failed to convert config blob to TOML")
			return false
			}
		tr.tomlConfigs[op.GetId()] = finalConfig
		return true

	case telemetry_edge.ConfigurationOp_REMOVE:
		if _, ok := tr.tomlConfigs[op.GetId()]; ok {
			delete(tr.tomlConfigs, op.GetId())
			return true
		}
		return false
		}
	return false
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
		"--config", tr.configServerURL)

	// telegraf returns the INFLUX_TOKEN in the http config request header
	runningContext.AppendEnv("INFLUX_TOKEN=" + tr.configServerToken)

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

func (tr *TelegrafRunner) createMainConfig() ([]byte, error) {
	data := &telegrafMainConfigData{
		IngestHost: tr.ingestHost,
		IngestPort: tr.ingestPort,
	}
	var b bytes.Buffer

	err := telegrafMainConfigTmpl.Execute(&b, data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute telegraf main config template")
	}

	return b.Bytes(), nil
}

func (tr *TelegrafRunner) handleConfigReload() {
	if err := tr.commandHandler.Signal(tr.running, syscall.SIGHUP); err != nil {
		log.WithError(err).WithField("pid", tr.running.Pid()).
			Warn("failed to signal agent process")
	}
}

func (tr *TelegrafRunner) hasRequiredPaths() bool {
	fullExePath := path.Join(tr.basePath, tr.exePath())
	if !fileExists(fullExePath) {
		log.WithField("exe", fullExePath).Debug("missing exe")
		return false
	}

	return true
}
