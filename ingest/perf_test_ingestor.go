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
)

type PerfTestIngestor struct {
	egressConn ambassador.EgressConnection
}

func init() {
	registerIngestor(&PerfTestIngestor{})
}

func (p *PerfTestIngestor) Bind(conn ambassador.EgressConnection) error {
	p.egressConn = conn
	return nil
}

func (p *PerfTestIngestor) Start(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C:
			p.processMetric()
		}
	}
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
