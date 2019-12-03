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

package ingest_test

import (
	"context"
	"fmt"
	"github.com/petergtz/pegomock"
	"github.com/phayes/freeport"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/ingest"
	"github.com/racker/telemetry-envoy/ingest/matchers"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

func TestPerfTestIngestor(t *testing.T) {
	pegomock.RegisterMockTestingT(t)
	mockEgressConnection := NewMockEgressConnection()
	port, err := freeport.GetFreePort()
	require.NoError(t, err)
	viper.Set(config.PerfTestPort, port)
	ingestor := &ingest.PerfTestIngestor{}
	err = ingestor.Bind(mockEgressConnection)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go ingestor.Start(ctx)
	defer cancel()

	// allow for ingestor to bind and webserver to start
	time.Sleep(5 * time.Millisecond)

	args := mockEgressConnection.VerifyWasCalledEventually(
		pegomock.Times(2),
		4*time.Second,
	).PostMetric(matchers.AnyPtrToTelemetryEdgeMetric()).GetAllCapturedArguments()

	require.Len(t, args, 2)
	assert.Equal(t, "perfTestMetric",
		args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Name)
	assert.Equal(t, "perfTestTag",
		args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Tags["test_tag"])
	assert.Equal(t, float64(-2.0),
		args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Fvalues["result_code"])
	assert.Equal(t, "success",
		args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Svalues["result_type"])

	metricsPerMinute := 10
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/?metricsPerMinute=%d", port, metricsPerMinute))
	require.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("metricsPerMinute set to %d", metricsPerMinute), string(body))
}
