package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
)

const (
	oracleStartupDuration = 60 * time.Second
)

type OracleRunner struct {
	basePath            string
	running             *AgentRunningContext
	commandHandler      CommandHandler
}

type OracleConfig struct {
	interval		int64
	content 		string
}


func init() {
	registerSpecificAgentRunner(telemetry_edge.AgentType_ORACLE, &OracleRunner{})
}


func (o *OracleRunner) Load(agentBasePath string) error {
	o.basePath = agentBasePath

	return nil
}

func (o *OracleRunner) SetCommandHandler(handler CommandHandler) {
	o.commandHandler = handler
}

func (o *OracleRunner) EnsureRunningState(ctx context.Context, applyConfigs bool) {

	if o.running.IsRunning() {

		// oracle agent requires a restart to pick up new configurations
		if applyConfigs {
			o.Stop()
			// ...and fall through to let it start back up again
		} else {
			// was already running and no configs to apply
			return
		}
	}
	runningContext := o.commandHandler.CreateContext(ctx,
		telemetry_edge.AgentType_ORACLE,
		buildRelativeExePath("salus-oracle-agent"), o.basePath,
		"-configs", configsDirSubpath)

	err := o.commandHandler.StartAgentCommand(runningContext,
		telemetry_edge.AgentType_ORACLE,
		"Succeeded in reconnecting to Envoy", oracleStartupDuration)
	if err != nil {
		log.Fatal("Failed to start the Oracle agent: ", err)
	}

	go o.commandHandler.WaitOnAgentCommand(ctx, o, runningContext)

	o.running = runningContext
	log.Info("started oracle agent")
}

func (o *OracleRunner) PostInstall() error {
	return nil
}

func (o *OracleRunner) PurgeConfig() error {
	return purgeConfigsDirectory(o.basePath)
}

func (o *OracleRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {

	configsPath, err := ensureConfigsDir(o.basePath)
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
			err := o.writeConfigFile(configInstancePath, op)
			if err != nil {
				return fmt.Errorf("failed to process oracle agent config file: %w", err)
			} else {
				applied++
			}

		case telemetry_edge.ConfigurationOp_REMOVE:
			err := os.Remove(configInstancePath)
			if err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove oracle agent config file: %w", err)
				}
			} else {
				applied++
			}
		}
	}

	if applied == 0 {
		return &noAppliedConfigsError{}
	}

	return nil
}


func (o *OracleRunner) writeConfigFile(path string, op *telemetry_edge.ConfigurationOp) error {
	var configMap map[string]interface{}
	err := json.Unmarshal([]byte(op.GetContent()), &configMap)
	if err != nil {
		return err
	}

	// add interval
	configMap["interval"] = op.Interval

	outFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer outFile.Close()

	encoder := json.NewEncoder(outFile)
	err = encoder.Encode(configMap)
	if err != nil {
		return err
	}

	return nil
}

func (o *OracleRunner) ProcessTestMonitor(correlationId string, content string, timeout time.Duration) (*telemetry_edge.TestMonitorResults, error) {
	return nil, errors.New("Test monitor not supported by oracle agent")
}

func (o *OracleRunner) Stop() {
	o.commandHandler.Stop(o.running)
	o.running = nil

}

