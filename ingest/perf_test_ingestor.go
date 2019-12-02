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

package ingest

import (
	"context"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/telemetry-envoy/ambassador"
	"time"
	"github.com/racker/telemetry-envoy/config"
	"github.com/spf13/viper"
	"net/http"
	"net"
	log "github.com/sirupsen/logrus"
	"strconv"
	"fmt"
)

type PerfTestIngestor struct {
	egressConn ambassador.EgressConnection
	currentMetricCount int64
	previousMetricCount int64
	serverHandler http.HandlerFunc
	ticker *time.Ticker
}

func init() {
	registerIngestor(&PerfTestIngestor{})
}

func (p *PerfTestIngestor) Bind(conn ambassador.EgressConnection) error {
	if !viper.GetBool(config.PerfTestMode) {
		return nil
	}
	log.Info("entering perfTest mode")
	p.egressConn = conn
	p.previousMetricCount = 0;
	p.currentMetricCount = 5;
	p.serverHandler = p.handler
	return nil
}

func (p *PerfTestIngestor) Start(ctx context.Context) {
	if !viper.GetBool(config.PerfTestMode) {
		return
	}
	
	go p.startPerfTestServer()
	for {
		if (p.previousMetricCount != p.currentMetricCount) {
			if (p.ticker != nil) {
				p.ticker.Stop()
			}
			p.previousMetricCount = p.currentMetricCount
			p.ticker = time.NewTicker(time.Duration(p.currentMetricCount * int64(time.Second)))
		}
		select {
		case <-ctx.Done():
			p.ticker.Stop()
			return

		case <-p.ticker.C:
			p.processMetric()
		}
	}
}

func (p *PerfTestIngestor) startPerfTestServer() {
	serverMux := http.NewServeMux()
	serverMux.Handle("/", p.serverHandler)

	listener, err := net.Listen("tcp", ":8100")
	if err != nil {
		log.Fatalf("couldn't create perf test server")
	}
	log.Info("started perfTest webServer")
	err = http.Serve(listener, serverMux)
	// Note this is probably not the best way to handle webserver failure
	log.Fatalf("perf test server error %v", err)
}
func (p *PerfTestIngestor) handler(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	metricCountVals, ok := params["metricCount"]
	if !ok || len(metricCountVals) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("metricCount parameter required"))
		return
	}
	count, err := strconv.Atoi(metricCountVals[0])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("metricCount parameter must be an int"))
		return
	}
	p.currentMetricCount = int64(count)
	_, _ = w.Write([]byte(fmt.Sprintf("metricCount set to %d", count)))
        return
}

func (p *PerfTestIngestor) processMetric() {
	fvalues := make(map[string]float64)
	svalues := make(map[string]string)
	tags := make(map[string]string)
	tags["test_tag"] = "perfTestTag"
	svalues["result_type"] = "success"
	fvalues["result_code"] = -2.0
	outMetric := &telemetry_edge.Metric{
		Variant: &telemetry_edge.Metric_NameTagValue{
			NameTagValue: &telemetry_edge.NameTagValueMetric{
				Name:      "perfTestMetric",
				Timestamp: time.Now().Unix(),
				Tags:      tags,
				Fvalues:   fvalues,
				Svalues:   svalues,
			},
		},
	}

	p.egressConn.PostMetric(outMetric)
}
