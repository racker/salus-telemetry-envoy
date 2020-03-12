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

package auth_test

import (
	"fmt"
	"github.com/racker/salus-telemetry-envoy/auth"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAuthServiceCertProvider_ProvideCertificates_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/v1.0/tenant/abc123/auth/cert", req.URL.Path)
		assert.Equal(t, "token-1", req.Header.Get("X-Auth-Token"))

		resp.Header().Set("Content-Type", "application/json")

		respFile, err := os.Open("testdata/auth_service_resp.json")
		require.NoError(t, err)
		defer respFile.Close()

		io.Copy(resp, respFile)
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tenant_id: abc123
tls:
  auth_service:
    url: %s
    token_provider: test
`, ts.URL)))
	require.NoError(t, err)

	auth.RegisterAuthTokenProvider("test", func() (auth.AuthTokenProvider, error) {
		return &TestAuthTokenProvider{Header: "X-Auth-Token", Token: "token-1"}, nil
	})

	certificate, certPool, err := auth.LoadCertificates()
	require.NoError(t, err)

	verifyCertSubject(t, "dev-ambassador", certificate)
	verifyCertPoolSubject(t, "dev-rmii-ambassador-ca", certPool)
}

func TestAuthServiceCertProvider_MissingTenantIdConfig(t *testing.T) {
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
tls:
  auth_service:
    token_provider: test
`))
	require.NoError(t, err)

	auth.RegisterAuthTokenProvider("test", func() (auth.AuthTokenProvider, error) {
		return &TestAuthTokenProvider{Header: "X-Auth-Token", Token: "token-1"}, nil
	})

	_, _, err = auth.LoadCertificates()
	require.Error(t, err)
	assert.Equal(t, err.Error(), "tenant_id missing from configuration")
}

func TestAuthServiceCertProvider_ProvideCertificates_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(500)
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  auth_service:
    url: %s
    token_provider: test
`, ts.URL)))
	require.NoError(t, err)

	auth.RegisterAuthTokenProvider("test", func() (auth.AuthTokenProvider, error) {
		return &TestAuthTokenProvider{Header: "X-Auth-Token", Token: "token-1"}, nil
	})

	certificate, certPool, err := auth.LoadCertificates()
	require.Error(t, err)
	assert.Nil(t, certificate)
	assert.Nil(t, certPool)
}

func TestAuthServiceCertProvider_ProvideCertificates_MissingRespFields(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(200)
		resp.Write([]byte(`{"certificate":""}`))
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
tls:
  auth_service:
    url: %s
    token_provider: test
`, ts.URL)))
	require.NoError(t, err)

	auth.RegisterAuthTokenProvider("test", func() (auth.AuthTokenProvider, error) {
		return &TestAuthTokenProvider{Header: "X-Auth-Token", Token: "token-1"}, nil
	})

	certificate, certPool, err := auth.LoadCertificates()
	require.Error(t, err)
	assert.Nil(t, certificate)
	assert.Nil(t, certPool)
}
