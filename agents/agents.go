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
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
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
	// PostInstall is invoked after installation of a new agent version and allows the
	// specific agent runner a chance to tweak capabilities assigned to the executable
	PostInstall(agentVersionPath string) error
	PurgeConfig() error
	ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error
	ProcessTestMonitor(correlationId string, content string, timeout time.Duration) (*telemetry_edge.TestMonitorResults, error)
	// Stop should stop the agent's process, if running
	Stop()
}

// Router routes external agent operations to the respective SpecificAgentRunner instance
type Router interface {
	// Start ensures that when the ctx is done, then the managed SpecificAgentRunner instances will be stopped
	Start(ctx context.Context)
	ProcessInstall(install *telemetry_edge.EnvoyInstructionInstall)
	ProcessConfigure(configure *telemetry_edge.EnvoyInstructionConfigure)
	ProcessTestMonitor(testMonitor *telemetry_edge.EnvoyInstructionTestMonitor) *telemetry_edge.TestMonitorResults
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
	viper.SetDefault(config.AgentsDataPath, config.DefaultAgentsDataPath)
	viper.SetDefault(config.AgentsTestMonitorTimeout, config.DefaultAgentsTestMonitorTimeout)
	viper.SetDefault(config.AgentsDefaultMonitoringInterval, config.DefaultAgentsDefaultMonitoringInterval)
	viper.SetDefault(config.AgentsMaxFlushInterval, config.DefaultAgentsMaxFlushInterval)
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
			defer file.Close()

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
			finalConfig, err = ConvertJsonToTelegrafToml(op.GetContent(), op.ExtraLabels, op.Interval)
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

// purgeConfigsDirectory can be used by agent runner's PurgeConfig when its agent is configured
// via config files
func purgeConfigsDirectory(basepath string) error {
	configsPath := path.Join(basepath, configsDirSubpath)
	err := os.RemoveAll(configsPath)
	if err != nil {
		return errors.Wrap(err, "failed to purge configs directory")
	}

	return nil
}

// hasConfigsAndExeReady can be used by agent runners to evaluate readiness of the agent by
// testing for the configs directory, files within that directory, and an installed executable
func hasConfigsAndExeReady(basepath string, exeName string, configFileExtension string) bool {
	curVerPath := filepath.Join(basepath, currentVerLink)
	if !fileExists(curVerPath) {
		log.WithField("path", curVerPath).Debug("missing current version link")
		return false
	}

	configsPath := filepath.Join(basepath, configsDirSubpath)
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
		if path.Ext(name) == configFileExtension {
			hasConfigs = true
			break
		}
	}
	if !hasConfigs {
		log.WithField("path", configsPath).Debug("missing config files")
		return false
	}

	fullExePath := path.Join(basepath, buildRelativeExePath(exeName))
	if !fileExists(fullExePath) {
		log.WithField("exe", fullExePath).Debug("missing exe")
		return false
	}

	return true
}

// buildRelativeExePath creates a relative path to the named executable via the standard
// current symlink and "bin" directory convention
func buildRelativeExePath(exeName string) string {
	return filepath.Join(currentVerLink, binSubpath, exeName)
}

// ensureConfigsDir can be used by agent runners to ensure the standard config subdirectory exists
// Returns the fully resolved path
func ensureConfigsDir(basepath string) (string, error) {
	configsPath := path.Join(basepath, configsDirSubpath)
	err := os.MkdirAll(configsPath, dirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to create configs path %s: %w", configsPath, err)
	}
	return configsPath, err
}

func getBoolFromConfigMap(configMap map[string]interface{}, field string, defaultVal bool) bool {
	if val, exists := configMap[field]; exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		} else {
			return defaultVal
		}
	} else {
		return defaultVal
	}
}

// filterLines will split the given content into lines, pass each line to the given filter
// and only keep those lines when filter returns true. The resulting lines are appended back
// together and returned as a byte slice.
func filterLines(content []byte, filter func(string) bool) []byte {
	bufIn := bytes.NewBuffer(content)
	var bufOut bytes.Buffer

	scanner := bufio.NewScanner(bufIn)
	for scanner.Scan() {
		line := scanner.Text()
		if filter(line) {
			bufOut.WriteString(line + "\n")
		}
	}

	return bufOut.Bytes()
}
