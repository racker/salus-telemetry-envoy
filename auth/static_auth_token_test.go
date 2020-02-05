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

package auth_test

import (
	"github.com/racker/salus-telemetry-envoy/auth"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestStaticAuthTokenProvider_ProvideAuthToken(t *testing.T) {

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
tls:
  token_providers:
    static:
      headers:
      - name: Header1
        value: Value1
      - name: Header2
        value: Value2
`))

	require.NoError(t, err)

	provider, err := auth.NewStaticAuthTokenProvider()
	require.NoError(t, err)
	require.NotNil(t, provider)

	token, err := provider.ProvideAuthToken()
	require.NoError(t, err)
	require.NotNil(t, token)

	assert.Equal(t, "Value1", token.Headers["Header1"])
	assert.Equal(t, "Value2", token.Headers["Header2"])
}
