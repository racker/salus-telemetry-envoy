package agents

import (
	"context"
	"encoding/json"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
	"github.com/pkg/errors"
)

var (
	oracleStartupDuration = 60 * time.Second
)

type OracleRunner struct {
	ingestHost          string
	ingestPort          string
	basePath            string // look at some of the older versions of this file to see how the base path was used to drop per/monitor config files
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


	runningContext := o.commandHandler.CreateContext(ctx,
		telemetry_edge.AgentType_ORACLE,
		filepath.Join(o.exePath(), "salus-oracle-agent"), o.basePath)

	log.Println("Starting Oracle Agent at path: ", o.basePath)

	err := o.commandHandler.StartAgentCommand(runningContext,
		telemetry_edge.AgentType_ORACLE,
		"Succeeded in reconnecting to Envoy", oracleStartupDuration)
	if err != nil {
		log.Fatal("Failed to start the Oracle agent: ", err)
	}
}

func (o *OracleRunner) PostInstall() error {
	log.Println("Oracle Agent PostInstall stub")
	return nil
}

func (o *OracleRunner) PurgeConfig() error {
	o.resolveConfigPath() // walk and delete all files in this DIR
	// need to delete all of the configurations
	log.Println("Oracle Agent PurgeConfig stub")
	return nil
}

func (o *OracleRunner) ProcessConfig(configure *telemetry_edge.EnvoyInstructionConfigure) error {
	log.Println("Oracle Agent ProcessConfig stub")
	for _, element := range configure.GetOperations() {
		//var genericJsonObj *json.RawMessage
		// in sending just the Content I am actually not adding in the interval... That really needs to be sent
		var config OracleConfig
		config.interval = element.Interval
		config.content = element.Content
		/*err := json.Unmarshal([]byte(config), &genericJsonObj)
		if err != nil {
			log.WithError(err).
				WithField("agentType", telemetry_edge.AgentType_Oracle).
				Warn("failed to start agent: Unable to unmarshal config")
			//actually stop the agent
			return err
		}*/
		//output, err := genericJsonObj.MarshalJSON()
		output, err := json.Marshal(config) //into a map of maps and then marshal that out
		if err != nil {
			log.WithError(err).
				WithField("agentType", telemetry_edge.AgentType_TELEGRAF).
				Warn("failed to start agent: unable to write config")
			return err
		}
		log.Println("Configuration Output: ", string(output))
		log.Println("Output ConfigurationPath: ",o.resolveConfigPath())

		err = os.MkdirAll(o.resolveConfigPath(),dirPerms)
		if err != nil {
			log.WithError(err).
				WithField("agentType", telemetry_edge.AgentType_TELEGRAF).
				Warn("failed to start agent")
			return err
		}


		file, err := os.OpenFile(filepath.Join(o.resolveConfigPath(),element.GetId()), os.O_RDWR|os.O_CREATE, os.FileMode(configFilePerms))
		_, err = file.Write(output)
		if err != nil {
			log.WithError(err).
				WithField("agentType", telemetry_edge.AgentType_TELEGRAF).
				Warn("failed to start agent")
			return err
		}
		file.Close()
	}

	return nil
}

func (o *OracleRunner) ProcessTestMonitor(correlationId string, content string, timeout time.Duration) (*telemetry_edge.TestMonitorResults, error) {
	return nil, errors.New("Test monitor not supported by filebeat agent")
}

func (o *OracleRunner) Stop() {
	o.commandHandler.Stop(o.running)
	o.running = nil

}

func (o *OracleRunner) resolveConfigPath() string {
	return filepath.Join(o.basePath,"/config/")
}


func (o *OracleRunner) exePath() string {
	return filepath.Join(currentVerLink, binSubpath)
}

