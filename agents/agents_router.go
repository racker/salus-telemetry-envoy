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
	"github.com/spf13/viper"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

type StandardAgentsRouter struct {
	DataPath   string
	detachChan <-chan struct{}

	ctx context.Context
	// mutetx is used to ensure ProcessInstall and ProcessConfigure are not called concurrently
	mutetx sync.Mutex
}

// NewAgentsRunner creates the component that manages the configuration and process lifecycle
// of the individual agents supported by the Envoy.
// The detachChan receives a signal when an attachment to an Ambassador is terminated. At that
// point this agents runner will take care of stopping any running agents and purging configuration
// in order to guarantee a consistent state at attachment.
func NewAgentsRunner(detachChan <-chan struct{}) (Router, error) {
	ar := &StandardAgentsRouter{
		DataPath:   viper.GetString(config.AgentsDataPath),
		detachChan: detachChan,
	}

	commandHandler := NewCommandHandler()

	for agentType, runner := range specificAgentRunners {

		agentBasePath := filepath.Join(ar.DataPath, agentsSubpath, agentType.String())

		runner.SetCommandHandler(commandHandler)
		err := runner.Load(agentBasePath)
		if err != nil {
			return nil, errors.Wrapf(err, "loading agent runner: %T", runner)
		}
	}

	err := ar.PurgeAgentConfigs()
	if err != nil {
		return nil, err
	}

	return ar, nil
}

func (ar *StandardAgentsRouter) Start(ctx context.Context) {
	ar.ctx = ctx

	for {
		select {
		case <-ar.ctx.Done():
			ar.stopAll()
			return

		case <-ar.detachChan:
			ar.stopAll()
			err := ar.PurgeAgentConfigs()
			if err != nil {
				log.WithError(err).Warn("failed to purge configs while handling detach")
			}
		}
	}
}

func (ar *StandardAgentsRouter) stopAll() {
	log.Debug("stopping agent runners")
	for _, specific := range specificAgentRunners {
		specific.Stop()
	}
}

func (ar *StandardAgentsRouter) ProcessInstall(install *telemetry_edge.EnvoyInstructionInstall) {
	log.WithField("install", install).Info("processing install instruction")

	agentType := install.Agent.Type
	if _, exists := specificAgentRunners[agentType]; !exists {
		log.WithField("type", agentType).Warn("no specific runner for agent type")
		return
	}

	ar.mutetx.Lock()
	defer ar.mutetx.Unlock()

	agentVersion := install.Agent.Version
	agentBasePath := path.Join(ar.DataPath, agentsSubpath, agentType.String())
	outputPath := path.Join(agentBasePath, agentVersion)

	if !fileExists(outputPath) {
		err := os.MkdirAll(outputPath, dirPerms)
		if err != nil {
			log.WithError(err).WithField("path", outputPath).Error("unable to mkdirs")
			return
		}

		err = downloadExtractTarGz(outputPath, install.Url, install.Exe)
		if err != nil {
			_ = os.RemoveAll(outputPath)
			log.WithError(err).Error("failed to download and extract agent")
			return
		}

		err = specificAgentRunners[agentType].PostInstall(outputPath)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"version": agentVersion,
				"type":    agentType,
			}).Error("failed to post-process agent installation")
			return
		}
	}

	err := ar.ensureCurrentSymlink(agentType, agentBasePath, agentVersion)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"version": agentVersion,
			"type":    agentType,
		}).Error("failed to adjust current agent installation link")
		return
	}

	specificAgentRunners[agentType].EnsureRunningState(ar.ctx, false)
}

func (ar *StandardAgentsRouter) ProcessConfigure(configure *telemetry_edge.EnvoyInstructionConfigure) {
	log.WithField("instruction", configure).Info("processing configure instruction")

	agentType := configure.GetAgentType()
	if specificRunner, exists := specificAgentRunners[agentType]; exists {

		ar.mutetx.Lock()
		defer ar.mutetx.Unlock()

		err := specificRunner.ProcessConfig(configure)
		if err != nil {
			if IsNoAppliedConfigs(err) {
				log.Warn("no configuration was applied")
			} else {
				log.WithError(err).Warn("failed to process agent configuration")
			}
		} else {
			specificRunner.EnsureRunningState(ar.ctx, true)
		}
	} else {
		log.WithField("type", configure.GetAgentType()).Warn("unable to configure unknown agent type")
	}
}

func (ar *StandardAgentsRouter) PurgeAgentConfigs() error {
	for agentType, specificRunner := range specificAgentRunners {
		log.WithField("agentType", agentType).Debug("purging config")
		err := specificRunner.PurgeConfig()
		if err != nil {
			return err
		}
	}
	return nil
}

func (ar *StandardAgentsRouter) ProcessTestMonitor(testMonitor *telemetry_edge.EnvoyInstructionTestMonitor) *telemetry_edge.TestMonitorResults {

	log.WithField("instruction", testMonitor).Info("processing test monitor instruction")

	agentType := testMonitor.GetAgentType()
	if specificRunner, exists := specificAgentRunners[agentType]; exists {

		results, err := specificRunner.ProcessTestMonitor(
			testMonitor.GetCorrelationId(), testMonitor.GetContent(),
			time.Duration(testMonitor.GetTimeout())*time.Second)
		// returned error is a shorthand to create a test monitor results with only errors reported
		if err != nil {
			log.WithError(err).
				WithField("instruction", testMonitor).
				Warn("Unable to process test monitor")

			results = &telemetry_edge.TestMonitorResults{
				CorrelationId: testMonitor.GetCorrelationId(),
				Errors:        []string{err.Error()},
			}
		}

		log.
			WithField("instruction", testMonitor).
			WithField("results", results).
			Debug("returning results of test-monitor")

		return results
	} else {
		log.WithField("type", agentType).Warn("unable to test monitor for unknown agent type")
		return nil
	}
}

func (ar *StandardAgentsRouter) ensureCurrentSymlink(agentType telemetry_edge.AgentType,
	agentBasePath string, agentVersion string) error {

	currentSymlinkPath := path.Join(agentBasePath, currentVerLink)

	// first grab some info about the symlink itself, if it exists
	_, err := os.Lstat(currentSymlinkPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to access current link: %w", err)
		}

		// link not present, so skip down to link creation

	} else {
		// link is present, but maybe it already points to correct directory...
		existingTarget, err := os.Readlink(currentSymlinkPath)
		if err != nil {
			return fmt.Errorf("failed to read current link: %w", err)
		}

		if existingTarget == agentVersion {
			// it was already pointing to desired version, so nothing needs to be done

			log.WithFields(log.Fields{
				"version": agentVersion,
				"type":    agentType,
			}).Debug("current link is already set")
			return nil
		}

		// remove link pointing to previous version
		err = os.Remove(currentSymlinkPath)
		if err != nil {
			return fmt.Errorf("failed to delete current version symlink: %w", err)
		}
	}

	// make sure a previous version is stopped
	specificAgentRunners[agentType].Stop()

	// ...and finally (re-)create the symlink pointing to the desired agent version
	err = os.Symlink(agentVersion, currentSymlinkPath)
	if err != nil {
		return fmt.Errorf("failed to create current version symlink: %w", err)
	}

	return nil
}
