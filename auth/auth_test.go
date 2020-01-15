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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"github.com/racker/salus-telemetry-envoy/auth"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func verifyCertPoolSubject(t *testing.T, expected string, pool *x509.CertPool) {
	// go from DES/ASN.1 encoded subject -> RDN sequence -> pkix name
	var caSubjectRDN pkix.RDNSequence
	_, err := asn1.Unmarshal(pool.Subjects()[0], &caSubjectRDN)
	require.NoError(t, err)
	var caSubject pkix.Name
	caSubject.FillFromRDNSequence(&caSubjectRDN)
	assert.Equal(t, expected, caSubject.CommonName)
}

func verifyCertSubject(t *testing.T, expected string, certificate *tls.Certificate) {
	parsedCertificate, err := x509.ParseCertificate(certificate.Certificate[0])
	require.NoError(t, err)
	assert.Equal(t, expected, parsedCertificate.Subject.CommonName)
}

func TestLoadCertificates_NoConfig(t *testing.T) {
	_, _, err := auth.LoadCertificates()
	assert.Error(t, err)
}

func TestLoadCertificates_Provided_MissingFile(t *testing.T) {

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBufferString(`
tls:
  provided:
    cert: testdata/a.pem
    key: testdata/b.pem
`))
	require.NoError(t, err)

	_, _, err = auth.LoadCertificates()
	assert.Error(t, err)
}

func TestLoadCertificates_Provided_BadCaFile(t *testing.T) {

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBufferString(`
tls:
  provided:
    cert: testdata/client.pem
    key: testdata/client-key.pem
    ca: testdata/bad_cert.pem
`))
	require.NoError(t, err)

	_, _, err = auth.LoadCertificates()
	assert.Error(t, err)
}

func TestLoadCertificates_Provided_BadCertFile(t *testing.T) {

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
tls:
  provided:
    cert: testdata/bad_cert.pem
    key: testdata/client-key.pem
    ca: testdata/ca.pem
`))
	require.NoError(t, err)

	_, _, err = auth.LoadCertificates()
	assert.Error(t, err)
}

func TestLoadCertificates_Provided_Normal(t *testing.T) {
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBufferString(`
tls:
  provided:
    cert: testdata/client.pem
    key: testdata/client-key.pem
    ca: testdata/ca.pem
`))
	require.NoError(t, err)

	certificate, pool, err := auth.LoadCertificates()
	require.NoError(t, err)

	require.NotNil(t, certificate)
	require.NotNil(t, pool)

	verifyCertSubject(t, "aaaaaa", certificate)

	verifyCertPoolSubject(t, "dev-rmii-ambassador-ca", pool)
}

func TestAppendUrlPath(t *testing.T) {
	var tests = []struct {
		name     string
		base     string
		path     string
		expected string
	}{
		{name: "typical", base: "http://localhost:8080", path: "/one/two", expected: "http://localhost:8080/one/two"},
		{name: "relative but no prior", base: "http://localhost:8080", path: "one/two", expected: "http://localhost:8080/one/two"},
		{name: "relative with prior no slash", base: "http://localhost:8080/here", path: "one/two", expected: "http://localhost:8080/one/two"},
		{name: "relative with prior and slash", base: "http://localhost:8080/here/", path: "one/two", expected: "http://localhost:8080/here/one/two"},
		{name: "absolute", base: "http://localhost:8080/here", path: "/one/two", expected: "http://localhost:8080/one/two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := auth.AppendUrlPath(tt.base, tt.path)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
