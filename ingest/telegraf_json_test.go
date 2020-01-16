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
	"github.com/petergtz/pegomock"
	"github.com/phayes/freeport"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-envoy/ingest"
	"github.com/racker/salus-telemetry-envoy/ingest/matchers"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"testing"
	"time"
)

func TestTelegrafJson_Start(t *testing.T) {
	tests := []struct {
		name       string
		totalCount int
		verify     func(t *testing.T, args []*telemetry_edge.Metric)
	}{
		{
			name:       "normal",
			totalCount: 9,
			verify: func(t *testing.T, args []*telemetry_edge.Metric) {
				assert.Equal(t, "cpu",
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Name)
				assert.Equal(t, int64(1538794540000),
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Timestamp)
				assert.Equal(t, "cpu0",
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Tags["cpu"])
				assert.Equal(t, float64(8.9),
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Fvalues["usage_user"])
				assert.Equal(t, float64(0),
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Fvalues["usage_steal"])
				assert.Equal(t, "one",
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Svalues["fake"])

				assert.Equal(t, float64(13.686313686313687),
					args[4].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Fvalues["usage_system"])
			},
		},
		{
			name:       "malformed",
			totalCount: 2,
			verify: func(t *testing.T, args []*telemetry_edge.Metric) {
				assert.Equal(t, "cpu",
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Name)
				assert.Equal(t, int64(1538794540000),
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Timestamp)
				assert.Equal(t, "cpu1",
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Tags["cpu"])
				assert.Equal(t, float64(1.1),
					args[0].Variant.(*telemetry_edge.Metric_NameTagValue).NameTagValue.Fvalues["usage_user"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pegomock.RegisterMockTestingT(t)

			mockEgressConnection := NewMockEgressConnection()
			port, err := freeport.GetFreePort()
			require.NoError(t, err)

			ingestor := &ingest.TelegrafJson{}
			addr := net.JoinHostPort("localhost", strconv.Itoa(port))
			viper.Set(config.IngestTelegrafJsonBind, addr)
			err = ingestor.Bind(mockEgressConnection)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			go ingestor.Start(ctx)
			defer cancel()

			// allow for ingestor to bind and accept connections
			time.Sleep(10 * time.Millisecond)

			conn, err := net.Dial("tcp", addr)
			require.NoError(t, err)
			defer conn.Close()

			file, err := os.Open(path.Join("testdata", "telegraf_json", tt.name+".json"))
			require.NoError(t, err)
			defer file.Close()

			written, err := io.Copy(conn, file)
			require.NoError(t, err)
			require.NotZero(t, written)

			args := mockEgressConnection.VerifyWasCalledEventually(
				pegomock.Times(tt.totalCount),
				500*time.Millisecond,
			).
				PostMetric(matchers.AnyPtrToTelemetryEdgeMetric()).GetAllCapturedArguments()

			require.Len(t, args, tt.totalCount)
			for i := 0; i < tt.totalCount; i++ {
				assert.IsType(t, (*telemetry_edge.Metric_NameTagValue)(nil), args[i].Variant, "%d is wrong type", i)
			}

			tt.verify(t, args)
		})
	}
}
