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

package auth

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/url"
)

// TlsConfig is populated from the viper config key "tls"
type TlsConfig struct {
	Disabled bool
	// Provided contains paths to the TLS certificates (PEM files) on the local filesystem.
	// This is useful for local testing or scaled down deployments.
	Provided *struct {
		Cert, Key, Ca string
	}
	AuthService *struct {
		Url string
	} `mapstructure:"auth_service"`
}

type CertProvider interface {
	ProvideCertificates(tlsConfig *TlsConfig) (*tls.Certificate, *x509.CertPool, error)
}

func LoadCertificates() (*tls.Certificate, *x509.CertPool, error) {

	tlsConfig := &TlsConfig{}
	err := viper.UnmarshalKey("tls", tlsConfig)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load tls configuration")
	}

	if tlsConfig.Provided != nil {
		provider := &PreallocatedCertProvider{}
		return provider.ProvideCertificates(tlsConfig)
	}

	if tlsConfig.AuthService != nil {
		provider := &AuthServiceCertProvider{}
		return provider.ProvideCertificates(tlsConfig)
	}

	return nil, nil, errors.New("missing specific tls provider configuration")
}

type PreallocatedCertProvider struct{}

func (p *PreallocatedCertProvider) ProvideCertificates(tlsConfig *TlsConfig) (*tls.Certificate, *x509.CertPool, error) {
	log.WithField("config", tlsConfig.Provided).Debug("loading provided certificates")
	certificate, err := tls.LoadX509KeyPair(
		tlsConfig.Provided.Cert,
		tlsConfig.Provided.Key,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load certificates")
	}

	// load the CA
	certPool := x509.NewCertPool()
	bs, err := ioutil.ReadFile(tlsConfig.Provided.Ca)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read ca cert")
	}

	ok := certPool.AppendCertsFromPEM(bs)
	if !ok {
		return nil, nil, errors.New("failed to process CA cert")
	}

	log.WithField("config", tlsConfig.Provided).Debug("successfully loaded provided certificates")
	return &certificate, certPool, nil
}

func AppendUrlPath(baseUrlStr string, pathStr string) (string, error) {
	baseUrl, err := url.Parse(baseUrlStr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse base URL")
	}

	relUrl, err := url.Parse(pathStr)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse relative URL")
	}

	resolved := baseUrl.ResolveReference(relUrl)

	return resolved.String(), nil
}
