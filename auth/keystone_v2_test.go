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
	"encoding/json"
	"fmt"
	"github.com/oliveagle/jsonpath"
	"github.com/phayes/freeport"
	"github.com/racker/salus-telemetry-envoy/auth"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestKeystoneV2AuthTokenProvider_ProvideAuthToken_Normal(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/tokens", req.URL.Path)

		reqBytes, err := ioutil.ReadAll(req.Body)
		require.NoError(t, err)

		verifyKeystoneV2Request(t, reqBytes)

		respFile, err := os.Open("testdata/keystone_v2_tokens_response.json")
		require.NoError(t, err)
		defer respFile.Close()

		resp.Header().Set("Content-Type", "application/json")
		io.Copy(resp, respFile)
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  token_providers:
    keystone_v2:
      username: user1
      apikey: abc123
      identityServiceUrl: %s
`, ts.URL)))

	authTokenProvider, err := auth.NewKeystoneV2AuthTokenProvider()
	require.NoError(t, err)

	token, err := authTokenProvider.ProvideAuthToken()
	require.NoError(t, err)

	assert.Equal(t, "ThisIsJustForTesting", token.Headers["X-Auth-Token"])
}

func TestKeystoneV2AuthTokenProvider_ProvideAuthToken_BadStatus(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/tokens", req.URL.Path)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

		reqBytes, err := ioutil.ReadAll(req.Body)
		require.NoError(t, err)

		verifyKeystoneV2Request(t, reqBytes)

		resp.Header().Set("Content-Type", "text/plain")
		resp.WriteHeader(500)
		resp.Write([]byte(""))
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  token_providers:
    keystone_v2:
      username: user1
      apikey: abc123
      identityServiceUrl: %s
`, ts.URL)))

	authTokenProvider, err := auth.NewKeystoneV2AuthTokenProvider()
	require.NoError(t, err)

	_, err = authTokenProvider.ProvideAuthToken()
	require.Error(t, err)
}

func TestKeystoneV2AuthTokenProvider_ProvideAuthToken_MalformedResponse(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/tokens", req.URL.Path)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

		reqBytes, err := ioutil.ReadAll(req.Body)
		require.NoError(t, err)

		verifyKeystoneV2Request(t, reqBytes)

		resp.Header().Set("Content-Type", "application/json")
		resp.Write([]byte("not valid json"))
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  token_providers:
    keystone_v2:
      username: user1
      apikey: abc123
      identityServiceUrl: %s
`, ts.URL)))

	authTokenProvider, err := auth.NewKeystoneV2AuthTokenProvider()
	require.NoError(t, err)

	_, err = authTokenProvider.ProvideAuthToken()
	require.Error(t, err)
}

func verifyKeystoneV2Request(t *testing.T, reqBytes []byte) {
	var reqContent interface{}
	err := json.Unmarshal(reqBytes, &reqContent)
	require.NoError(t, err)
	username, err := jsonpath.JsonPathLookup(reqContent, "$.auth.RAX-KSKEY:apiKeyCredentials.username")
	require.NoError(t, err)
	assert.Equal(t, "user1", username)
	apikey, err := jsonpath.JsonPathLookup(reqContent, "$.auth.RAX-KSKEY:apiKeyCredentials.apiKey")
	require.NoError(t, err)
	assert.Equal(t, "abc123", apikey)
}

func TestKeystoneV2AuthTokenProvider_ProvideAuthToken_BadLogin(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/tokens", req.URL.Path)

		resp.Header().Set("Content-Type", "application/json")
		resp.WriteHeader(401)
		resp.Write([]byte(`{
	"unauthorized": {
		"code": 401,
		"message": "Username or api key is invalid."
	}
}`))
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  token_providers:
    keystone_v2:
      username: foo
      apikey: bar
      identityServiceUrl: %s
`, ts.URL)))

	authTokenProvider, err := auth.NewKeystoneV2AuthTokenProvider()
	require.NoError(t, err)

	_, err = authTokenProvider.ProvideAuthToken()
	require.Error(t, err)
}

func TestKeystoneV2AuthTokenProvider_ProvideAuthToken_NoServer(t *testing.T) {
	fakePort, err := freeport.GetFreePort()
	require.NoError(t, err)

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  token_providers:
    keystone_v2:
      username: user1
      apikey: abc123
      identityServiceUrl: http://localhost:%d
`, fakePort)))

	authTokenProvider, err := auth.NewKeystoneV2AuthTokenProvider()
	require.NoError(t, err)

	_, err = authTokenProvider.ProvideAuthToken()
	require.Error(t, err)
}
