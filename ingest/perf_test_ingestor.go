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

package ingest

import (
	"context"
	"fmt"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
	"net/http"
	"strconv"
	"time"
)

type PerfTestIngestor struct {
	egressConn       ambassador.EgressConnection
	metricsPerMinute int
	floatsPerMetric  int
	ticker           *time.Ticker
	newRateC         chan int
}

func init() {
	registerIngestor(&PerfTestIngestor{})
}

func (p *PerfTestIngestor) Bind() error {
	log.Infof("gbjinfo: %d\n", viper.GetInt(config.PerfTestPort))
	if viper.GetInt(config.PerfTestPort) == 0 {
		return nil
	}
	if viper.GetInt(config.PerfTestMetricsPerMinute) == 0 {
		p.metricsPerMinute = 60
		p.floatsPerMetric = 10
	} else {
		p.metricsPerMinute = viper.GetInt(config.PerfTestMetricsPerMinute)
		p.floatsPerMetric = viper.GetInt(config.PerfTestFloatsPerMetric)
	}
	log.Infof("entering perfTest mode.  metricsPerMinute %d, floatsPerMetric %d\n",
		p.metricsPerMinute, p.floatsPerMetric)
	p.newRateC = make(chan int)
	return nil
}

func (p *PerfTestIngestor) Start(ctx context.Context, conn ambassador.EgressConnection) {
	if viper.GetInt(config.PerfTestPort) == 0 {
		return
	}
	p.egressConn = conn

	p.ticker = time.NewTicker(time.Duration(int64(time.Minute) / int64(p.metricsPerMinute)))
	go p.startPerfTestServer()
	for {
		select {
		case <-ctx.Done():
			p.ticker.Stop()
			return
		case p.metricsPerMinute = <-p.newRateC:
			p.ticker.Stop()
			p.ticker = time.NewTicker(time.Duration(int64(time.Minute) / int64(p.metricsPerMinute)))
		case <-p.ticker.C:
			p.processMetric()
		}
	}
}

func (p *PerfTestIngestor) startPerfTestServer() {
	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/", p.handler)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", viper.GetInt(config.PerfTestPort)))
	if err != nil {
		log.Fatalf("couldn't create perf test server")
	}
	log.Info("started perfTest webServer")
	err = http.Serve(listener, serverMux)
	log.Fatalf("perf test server error %v", err)
}

func (p *PerfTestIngestor) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("Only POST's accepted"))
		return
	}
	metricsPerMinuteSet := false
	floatsPerMetricSet := false
	metricsCount := p.metricsPerMinute
	var err error

	if r.PostFormValue("metricsPerMinute") != "" {
		metricsCount, err = strconv.Atoi(r.PostFormValue("metricsPerMinute"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("metricsPerMinute parameter must be an int: " + err.Error()))
			return
		}
		p.newRateC <- metricsCount
		metricsPerMinuteSet = true
	}

	if r.PostFormValue("floatsPerMetric") != "" {
		floatsCount, err := strconv.Atoi(r.PostFormValue("floatsPerMetric"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("floatsPerMetric parameter must be an int: " + err.Error()))
			return
		}
		p.floatsPerMetric = floatsCount
		floatsPerMetricSet = true
	}
	if metricsPerMinuteSet == false && floatsPerMetricSet == false {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("metricsPerMinute or floatsPerMetric parameter required"))
		return
	}
	output := fmt.Sprintf("metricsPerMinute set to %d, floatsPerMetric set to %d\n",
		metricsCount, p.floatsPerMetric)
	log.Infof(output)
	_, _ = w.Write([]byte(output))
	return
}

func (p *PerfTestIngestor) processMetric() {
	fvalues := make(map[string]float64)
	svalues := make(map[string]string)
	tags := make(map[string]string)
	tags["test_tag"] = "perfTestTag"
	svalues["result_type"] = "success"
	for i := 0; i < p.floatsPerMetric; i++ {
		fvalues[fmt.Sprintf("duration_%d", i)] = float64(i)
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
