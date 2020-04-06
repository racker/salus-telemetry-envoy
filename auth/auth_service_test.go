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
		assert.Equal(t, "/v1.0/cert", req.URL.Path)
		assert.Equal(t, "Bearer token1", req.Header.Get("Authorization"))

		resp.Header().Set("Content-Type", "application/json")

		respFile, err := os.Open("testdata/auth_service_resp.json")
		require.NoError(t, err)
		defer respFile.Close()

		io.Copy(resp, respFile)
	}))
	defer ts.Close()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
auth_token: token1
tls:
  auth_service:
    url: %s
`, ts.URL)))
	require.NoError(t, err)

	certificate, certPool, err := auth.LoadCertificates()
	require.NoError(t, err)

	verifyCertSubject(t, "dev-ambassador", certificate)
	verifyCertPoolSubject(t, "dev-rmii-ambassador-ca", certPool)
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
`, ts.URL)))
	require.NoError(t, err)

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
`, ts.URL)))
	require.NoError(t, err)

	certificate, certPool, err := auth.LoadCertificates()
	require.Error(t, err)
	assert.Nil(t, certificate)
	assert.Nil(t, certPool)
}
