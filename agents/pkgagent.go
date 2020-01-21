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
	"encoding/json"
	"fmt"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-envoy/lineproto"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	packageAgentExeName = "salus-packages-agent"
)

type PackagesAgentRunner struct {
	basePath       string
	commandHandler CommandHandler
	running        *AgentRunningContext
}

func init() {
	registerSpecificAgentRunner(telemetry_edge.AgentType_PACKAGES, &PackagesAgentRunner{})
}

func (pr *PackagesAgentRunner) Load(agentBasePath string) error {
	pr.basePath = agentBasePath
	return nil
}

func (pr *PackagesAgentRunner) SetCommandHandler(handler CommandHandler) {
	pr.commandHandler = handler
}

func (pr *PackagesAgentRunner) PurgeConfig() error {
	return purgeConfigsDirectory(pr.basePath)
}

func (pr *PackagesAgentRunner) PostInstall() error {
	// no adjustments needed
	return nil
}

func (pr *PackagesAgentRunner) EnsureRunningState(ctx context.Context, applyConfigs bool) {
	log.Debug("ensuring package agent is in correct running state")

	if !hasConfigsAndExeReady(pr.basePath, packageAgentExeName, ".json") {
		log.Debug("packages agent not runnable due to some missing paths and files")
		pr.Stop()
		return
	}

	if pr.running.IsRunning() {
		// package agent requires a restart to pick up new configurations
		if applyConfigs {
			pr.Stop()
			// ...and fall through to let it start back up again
		} else {
			// was already running and no configs to apply
			return
		}
	}

	runningContext := pr.commandHandler.CreateContext(ctx,
		telemetry_edge.AgentType_PACKAGES,
		buildRelativeExePath(packageAgentExeName), pr.basePath,
		"--configs", configsDirSubpath,
		"--line-protocol-to-socket", config.GetListenerAddress(config.LineProtocolListener),
	)

	err := pr.commandHandler.StartAgentCommand(runningContext, telemetry_edge.AgentType_PACKAGES, "", 0)
	if err != nil {
		log.WithError(err).
			WithField("agentType", telemetry_edge.AgentType_PACKAGES).
			Warn("failed to start agent")
		return
	}

	go pr.commandHandler.WaitOnAgentCommand(ctx, pr, runningContext)

	pr.running = runningContext
	log.Info("started packages agent")
}

func (pr *PackagesAgentRunner) Stop() {
	pr.commandHandler.Stop(pr.running)
	pr.running = nil
}

func (pr *PackagesAgentRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {
	configsPath, err := ensureConfigsDir(pr.basePath)
	if err != nil {
		return err
	}

	applied := 0
	for _, op := range configure.GetOperations() {
		log.WithField("op", op).Debug("processing config operation")

		configInstancePath := filepath.Join(configsPath, fmt.Sprintf("%s.json", op.GetId()))

		switch op.Type {
		case telemetry_edge.ConfigurationOp_CREATE, telemetry_edge.ConfigurationOp_MODIFY:
			// (re)create file
			err := pr.writeConfigFile(configInstancePath, op)
			if err != nil {
				return fmt.Errorf("failed to process package agent config file: %w", err)
			} else {
				applied++
			}

		case telemetry_edge.ConfigurationOp_REMOVE:
			err := os.Remove(configInstancePath)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove package agent config file: %w", err)
			}
		}
	}

	if applied == 0 {
		return &noAppliedConfigsError{}
	}

	return nil
}

func (pr *PackagesAgentRunner) writeConfigFile(path string, op *telemetry_edge.ConfigurationOp) error {
	var configMap map[string]interface{}
	err := json.Unmarshal([]byte(op.GetContent()), &configMap)
	if err != nil {
		return err
	}

	// remove "type"
	delete(configMap, "type")
	// add interval
	configMap["interval"] = (time.Duration(op.Interval) * time.Second).String()

	outFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer outFile.Close()

	configMap = normalizeKeysToKebabCase(configMap)

	encoder := json.NewEncoder(outFile)
	err = encoder.Encode(configMap)
	if err != nil {
		return err
	}

	return nil
}

func (pr *PackagesAgentRunner) ProcessTestMonitor(correlationId string, content string, timeout time.Duration) (*telemetry_edge.TestMonitorResults, error) {
	var configMap map[string]interface{}
	err := json.Unmarshal([]byte(content), &configMap)
	if err != nil {
		return nil, err
	}

	includeRpm := getBoolFromConfigMap(configMap, "include-rpm", false)
	includeDebian := getBoolFromConfigMap(configMap, "include-debian", false)

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()

	output, err := pr.commandHandler.RunToCompletion(ctx,
		buildRelativeExePath(packageAgentExeName), pr.basePath,
		"--line-protocol-to-console",
		"--include-rpm", strconv.FormatBool(includeRpm),
		"--include-debian", strconv.FormatBool(includeDebian),
	)
	if err != nil {
		return nil, err
	}

	// filter to keep only the metric output lines which will be prefixed in order to avoid
	// attempting to parse any log output
	// See https://github.com/racker/salus-packages-agent/#console
	filtered := filterLines(output, func(line string) bool {
		return strings.HasPrefix(line, "> ")
	})

	parsedMetrics, err := lineproto.ParseInfluxLineProtocolMetrics(filtered)
	if err != nil {
		return nil, err
	}

	// Wrap up the named tag-value metrics into the general metrics type
	metrics := make([]*telemetry_edge.Metric, len(parsedMetrics))
	for i, metric := range parsedMetrics {
		metrics[i] = &telemetry_edge.Metric{
			Variant: &telemetry_edge.Metric_NameTagValue{NameTagValue: metric},
		}
	}

	results := &telemetry_edge.TestMonitorResults{
		CorrelationId: correlationId,
		Errors:        []string{},
		Metrics:       metrics,
	}

	return results, nil
}
