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
	"github.com/pkg/errors"
	"github.com/syndtr/gocapability/capability"
)

// addNetRawCapabilities enables an executable to use net raw devices, such as for ICMP/ping.
// The operations in this function are a noop when running on a non-Linux system.
func addNetRawCapabilities(exePath string) error {

	c, err := capability.NewFile2(exePath)
	if err != nil {
		return errors.Wrap(err, "failed to open exe")
	}
	err = c.Load()
	if err != nil {
		return errors.Wrap(err, "failed to load capabilities")
	}

	c.Set(capability.EFFECTIVE|capability.PERMITTED|capability.INHERITABLE, capability.CAP_NET_RAW)

	err = c.Apply(capability.CAPS)
	if err != nil {
		return errors.Wrap(err, "failed to apply capabilities")
	}

	return nil
}
