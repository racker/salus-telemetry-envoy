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
	"fmt"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/telemetry-envoy/ambassador"
	"github.com/racker/telemetry-envoy/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"strconv"
	"time"
)

type PerfTestIngestor struct {
	egressConn               ambassador.EgressConnection
	currentMetricsPerMinute  int64
	previousMetricsPerMinute int64
	currentFloatsPerMetric   int64
	previousFloatsPerMetric  int64
	serverHandler            http.HandlerFunc
	ticker                   *time.Ticker
}

func init() {
	registerIngestor(&PerfTestIngestor{})
}

func (p *PerfTestIngestor) Bind(conn ambassador.EgressConnection) error {
	if viper.GetInt(config.PerfTestPort) == 0 {
		return nil
	}
	log.Info("entering perfTest mode")
	p.egressConn = conn
	p.previousMetricsPerMinute = 0
	p.currentMetricsPerMinute = 60
	p.previousFloatsPerMetric = 0
	p.currentFloatsPerMetric = 10
	p.serverHandler = p.handler
	return nil
}

func (p *PerfTestIngestor) Start(ctx context.Context) {
	if viper.GetInt(config.PerfTestPort) == 0 {
		return
	}

	go p.startPerfTestServer()
	for {
		if p.previousMetricsPerMinute != p.currentMetricsPerMinute {
			if p.ticker != nil {
				p.ticker.Stop()
			}
			p.previousMetricsPerMinute = p.currentMetricsPerMinute
			p.ticker = time.NewTicker(time.Duration(int64(time.Minute) / p.currentMetricsPerMinute))
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

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", viper.GetInt(config.PerfTestPort)))
	if err != nil {
		log.Fatalf("couldn't create perf test server")
	}
	log.Info("started perfTest webServer")
	err = http.Serve(listener, serverMux)
	log.Fatalf("perf test server error %v", err)
}

func (p *PerfTestIngestor) handler(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	metricsPerMinuteSet := false
	floatsPerMetricSet := false

	metricsPerMinuteVals, ok := params["metricsPerMinute"]
	if ok && len(metricsPerMinuteVals) != 0 {
		metricsCount, err := strconv.Atoi(metricsPerMinuteVals[0])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("metricsPerMinute parameter must be an int: " + err.Error()))
			return
		}
		p.currentMetricsPerMinute = int64(metricsCount)
		metricsPerMinuteSet = true
	}

	floatsPerMetricVals, ok := params["floatsPerMetric"]
	if ok && len(floatsPerMetricVals) != 0 {
		floatsCount, err := strconv.Atoi(floatsPerMetricVals[0])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("floatsPerMetric parameter must be an int: " + err.Error()))
			return
		}
		p.currentFloatsPerMetric = int64(floatsCount)
		floatsPerMetricSet = true
	}
	if metricsPerMinuteSet == false && floatsPerMetricSet == false {
		_, _ = w.Write([]byte("metricsPerMinute or floatsPerMetric parameter required"))
		return
	}
	_, _ = w.Write([]byte(fmt.Sprintf("metricsPerMinute set to %d, floatsPerMetric set to %d", p.currentMetricsPerMinute, p.currentFloatsPerMetric)))
	return
}

func (p *PerfTestIngestor) processMetric() {
	fvalues := make(map[string]float64)
	svalues := make(map[string]string)
	tags := make(map[string]string)
	tags["test_tag"] = "perfTestTag"
	svalues["result_type"] = "success"
	for i := 0; int64(i) < p.currentFloatsPerMetric; i++ {
		fvalues[fmt.Sprintf("result_code%d", i)] = -2.0
	}
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
