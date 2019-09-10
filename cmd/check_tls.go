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
	"crypto/tls"
	"github.com/racker/telemetry-envoy/auth"
	"github.com/racker/telemetry-envoy/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var checkTlsCmd = &cobra.Command{
	Use:   "check-tls",
	Short: "Obtains client certificates and verifies TLS connectivity to Ambassador",
	Run: func(cmd *cobra.Command, args []string) {

		certificate, certPool, err := auth.LoadCertificates()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to acquire client certificates")
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{*certificate},
			RootCAs:      certPool,
		}

		conn, err := tls.Dial("tcp", viper.GetString(config.AmbassadorAddress), tlsConfig)
		if err != nil {
			logrus.WithError(err).
				WithField("ambassador", config.AmbassadorAddress).
				Fatal("Failed to connect to ambassador")
		}
		_ = conn.Close()

		logrus.
			WithField("ambassador", config.AmbassadorAddress).
			Info("Successfully authenticated and connected to Ambassador")
	},
}

func init() {
	rootCmd.AddCommand(checkTlsCmd)

	checkTlsCmd.Flags().String("ambassador", "", "Ambassador host:port to verify")
	if err := viper.BindPFlag(config.AmbassadorAddress, checkTlsCmd.Flag("ambassador")); err != nil {
		logrus.WithError(err).Fatal("viper setup failed")
	}
}
