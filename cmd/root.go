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

package cmd

import (
	"fmt"
	"github.com/racker/telemetry-envoy/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"strings"
)

var (
	cfgFile string
	debug   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "telemetry-envoy",
	Short: "The Telemetry Envoy application",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(versionInfoIn VersionInfo) {
	versionInfo = versionInfoIn

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.telemetry-envoy.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug output")

	rootCmd.PersistentFlags().Int("perf-test-port", 0, "Enable perf test mode port")
	viper.BindPFlag(config.PerfTestPort, rootCmd.PersistentFlags().Lookup("perf-test-port"))
	rootCmd.PersistentFlags().String("resource-id", "", "Identifier of the resource where this Envoy is running")
	viper.BindPFlag(config.ResourceId, rootCmd.PersistentFlags().Lookup("resource-id"))

	rootCmd.PersistentFlags().String("data-path", config.DefaultAgentsDataPath,
		"Data directory where Envoy stores downloaded agents and write agent configs")
	viper.BindPFlag(config.AgentsDataPath, rootCmd.PersistentFlags().Lookup("data-path"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	// log to stdout since most service wrappers and container orchestration expects that
	log.SetOutput(os.Stdout)

	// Configure logging thresholds
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("telemetry-envoy")

		// main production location
		viper.AddConfigPath("/etc/salus")
		// mostly for developer convenience
		viper.AddConfigPath("$HOME/.salus")
		// otherwise fallback to looking in current directory to ease double-click launching
		viper.AddConfigPath(".")
	}

	replacer := strings.NewReplacer(".", "_", "-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("ENVOY")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.WithField("file", viper.ConfigFileUsed()).Info("Using config file")
	} else if cfgFile != "" {
		log.WithError(err).Fatal("Failed to read config file")
	} else {
		log.WithError(err).Debug("Unable to locate config file")
	}
}
