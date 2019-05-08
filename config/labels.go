/*
 *    Copyright 2018 Rackspace US, Inc.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 *
 */

package config

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"runtime"
	"strings"
)

// Discoverable label names
const (
	// This prefix will be used to qualify labels that are discovered by the
	// Envoy and differentiate those from the user configured labels.
	DiscoveredPrefix = "discovered"
	PrefixDelimiter  = "_"
)

var (
	SystemPrefixes = []string{DiscoveredPrefix}

	ArchLabel        = DiscoveredLabel("arch")
	BiosVendorLabel  = DiscoveredLabel("bios-vendor")
	BiosVersionLabel = DiscoveredLabel("bios-version")
	HostnameLabel    = DiscoveredLabel("hostname")
	OsLabel          = DiscoveredLabel("os")
	SerialNoLabel    = DiscoveredLabel("serial")
	XenIdLabel       = DiscoveredLabel("xen-id")
)

// ComputeLabels reads any labels specified in the config file.
// It also auto discovers various instance identifiers such as the os type and hostname.
// If an auto-detected label is also specified in the config file, the config file value will be used.
// These labels are passed over to the server-side endpoint.
// All label key and values are lower-case; keys can include a hyphen.
func ComputeLabels() (map[string]string, error) {
	labels := make(map[string]string)

	labels[OsLabel] = runtime.GOOS
	labels[ArchLabel] = runtime.GOARCH

	hostname, err := os.Hostname()
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine hostname label")
	}
	labels[HostnameLabel] = hostname

	xenId, err := GetXenId()
	if err != nil {
		log.WithError(err).Debug("unable to determine xen-id")
	} else {
		labels[XenIdLabel] = xenId
	}

	serial, err := GetSystemSerialNumber()
	if err != nil {
		log.WithError(err).Debug("unable to determine system serial number")
	} else {
		labels[SerialNoLabel] = serial
	}

	biosData, err := GetBiosData()
	if err != nil {
		log.WithError(err).Debug("unable to determine bios data")
	} else {
		for k, v := range biosData {
			labels[k] = v
		}
	}

	configuredLabels := viper.GetStringMapString("labels")
	for k, v := range configuredLabels {
		if ValidateUserLabelName(k) {
			labels[k] = v
		} else {
			return nil, errors.Errorf("configured label '%s' conflicts with a system prefix", k)
		}
	}

	log.WithField("labels", labels).Debug("discovered labels")

	return labels, nil
}

// DiscoveredLabel converts an unqualified label into the qualified, namespaced label name/key
func DiscoveredLabel(label string) string {
	return DiscoveredPrefix + PrefixDelimiter + label
}

// ValidateUserLabelName will check that the given label name does not conflict with a system
// prefix. Returns true if the user's label is valid.
func ValidateUserLabelName(label string) bool {
	for _, prefix := range SystemPrefixes {
		if strings.HasPrefix(label, prefix+PrefixDelimiter) {
			return false
		}
	}
	return true
}
