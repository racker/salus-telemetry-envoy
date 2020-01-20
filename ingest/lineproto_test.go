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

package ingest_test

import (
	"context"
	"github.com/petergtz/pegomock"
	"github.com/phayes/freeport"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-envoy/ingest"
	"github.com/racker/salus-telemetry-envoy/ingest/matchers"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestLineProtocol_Start(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	mockEgressConnection := NewMockEgressConnection()
	port, err := freeport.GetFreePort()
	require.NoError(t, err)

	ingestor := &ingest.LineProtocol{}
	addr := net.JoinHostPort("localhost", strconv.Itoa(port))
	viper.Set(config.IngestLineProtocolBind, addr)
	err = ingestor.Bind()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go ingestor.Start(ctx, mockEgressConnection)
	defer cancel()

	// allow for ingestor to bind and accept connections
	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte(`packages,system=rpm,package=tzdata,arch=noarch version="2019a-1.el8" 1136214245000000000
`))
	require.NoError(t, err)

	args := mockEgressConnection.VerifyWasCalledEventually(
		pegomock.Times(1),
		500*time.Millisecond,
	).
		PostMetric(matchers.AnyPtrToTelemetryEdgeMetric()).GetAllCapturedArguments()

	assert.IsType(t, (*telemetry_edge.Metric_NameTagValue)(nil), args[0].Variant)
	metric := args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue

	assert.Equal(t, "packages", metric.Name)
	assert.Equal(t, map[string]string{
		"system":  "rpm",
		"package": "tzdata",
		"arch":    "noarch",
	}, metric.Tags)
	assert.Equal(t, map[string]string{
		"version": "2019a-1.el8",
	}, metric.Svalues)
	assert.Equal(t, int64(1136214245000), metric.Timestamp)
}
