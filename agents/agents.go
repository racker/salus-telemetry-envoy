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
	"archive/tar"
	"compress/gzip"
	"context"
	"github.com/pkg/errors"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

const (
	agentsSubpath     = "agents"
	configsDirSubpath = "config.d"
	currentVerLink    = "CURRENT"
	binSubpath        = "bin"
	dirPerms          = 0755
	configFilePerms   = 0600
)

// SpecificAgentRunner manages the lifecyle and configuration of a single type of agent
type SpecificAgentRunner interface {
	// Load gets called after viper's configuration has been populated and before any other use.
	Load(agentBasePath string) error
	SetCommandHandler(handler CommandHandler)
	// EnsureRunningState is called after installation of an agent and after each call to ProcessConfig.
	// In the latter case, applyConfigs will be passed as true to indicate the runner should take
	// actions to reload configuration into an agent, if running.
	// It must ensure the agent process is running if configs and executable are available
	// It must also ensure that that the process is stopped if no configuration remains
	EnsureRunningState(ctx context.Context, applyConfigs bool)
	ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error
	// Stop should stop the agent's process, if running
	Stop()
}

// Router routes external agent operations to the respective SpecificAgentRunner instance
type Router interface {
	// Start ensures that when the ctx is done, then the managed SpecificAgentRunner instances will be stopped
	Start(ctx context.Context)
	ProcessInstall(install *telemetry_edge.EnvoyInstructionInstall)
	ProcessConfigure(configure *telemetry_edge.EnvoyInstructionConfigure)
}

type noAppliedConfigsError struct{}

func (e *noAppliedConfigsError) Error() string {
	return "no configurations were applied"
}

// IsNoAppliedConfigs tests if an error returned by a SpecificAgentRunner's ProcessConfig indicates no configs were applied
func IsNoAppliedConfigs(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*noAppliedConfigsError)
	return ok
}

func init() {
	viper.SetDefault("agents.dataPath", "data-telemetry-envoy")
}

func downloadExtractTarGz(outputPath, url string, exePath string) error {

	log.WithField("file", url).Debug("downloading agent")
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to download agent")
	}
	//noinspection GoUnhandledErrorResult
	defer resp.Body.Close()

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return errors.Wrap(err, "unable to ungzip agent download")
	}

	_, exeFilename := path.Split(exePath)
	binOutPath := path.Join(outputPath, binSubpath)
	err = os.Mkdir(binOutPath, dirPerms)
	if err != nil {
		return errors.Wrap(err, "unable to create bin directory")
	}

	processedExe := false
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if header.Name == exePath {

			file, err := os.OpenFile(path.Join(binOutPath, exeFilename), os.O_RDWR|os.O_CREATE, os.FileMode(header.Mode))
			if err != nil {
				log.WithError(err).Error("unable to open file for writing")
				continue
			}

			_, err = io.Copy(file, tarReader)
			if err != nil {
				_ = file.Close()
				log.WithError(err).Error("unable to write to file")
				continue
			} else {
				processedExe = true
			}
		}
	}

	if !processedExe {
		return errors.New("failed to find/process agent executable")
	}

	return nil
}

func fileExists(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	} else if err != nil {
		log.WithError(err).Warn("failed to stat file")
		return false
	} else {
		return true
	}
}

// handleContentConfigurationOp handles agent config operations that work with content simply written to
// the file named by configInstancePath
// Returns true if the configuration was applied
func handleContentConfigurationOp(op *telemetry_edge.ConfigurationOp, configInstancePath string, conversion Conversion) bool {
	switch op.GetType() {
	case telemetry_edge.ConfigurationOp_CREATE, telemetry_edge.ConfigurationOp_MODIFY:

		var finalConfig []byte
		var err error

		switch conversion {
		case ConversionNone:
			finalConfig = []byte(op.GetContent())
		case ConversionJsonToTelegrafToml:
			finalConfig, err = ConvertJsonToTelegrafToml(op.GetContent(), op.ExtraLabels)
			if err != nil {
				log.WithError(err).WithField("op", op).Warn("failed to convert config blob to TOML")
				return false
			}
		}

		err = ioutil.WriteFile(configInstancePath, finalConfig, configFilePerms)
		if err != nil {
			log.WithError(err).WithField("op", op).Warn("failed to process telegraf config operation")
		} else {
			return true
		}

	case telemetry_edge.ConfigurationOp_REMOVE:
		err := os.Remove(configInstancePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.WithField("op", op).Warn("did not need to remove since already removed")
				return true
			} else {
				log.WithError(err).WithField("op", op).Warn("failed to remove config instance file")
			}
		} else {
			return true
		}
	}

	return false
}
