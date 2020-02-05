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
	"bufio"
	"context"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-envoy/lineproto"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
)

const (
	lineProtoMetricsChanSize = 100
)

func init() {
	viper.SetDefault(config.IngestLineProtocolBind, "localhost:8194")

	registerIngestor(&LineProtocol{})
}

type LineProtocol struct {
	listener   net.Listener
	egressConn ambassador.EgressConnection
	// metrics channel allows for buffering between the ingestion from a metrics producer and
	// posting to the ambassador
	metrics chan *telemetry_edge.NameTagValueMetric
}

func (l *LineProtocol) Bind() error {
	address := viper.GetString(config.IngestLineProtocolBind)
	// check if ingest is disabled via absent config
	if address == "" {
		return nil
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.Wrap(err, "failed to bind line protocol listener")
	}

	config.RegisterListenerAddress(config.LineProtocolListener, listener.Addr().String())
	l.listener = listener

	log.WithField("address", address).Debug("listening for line protocol")
	return nil
}

func (l *LineProtocol) Start(ctx context.Context, connection ambassador.EgressConnection) {
	if l.listener == nil {
		log.Debug("telegraf json ingest is disabled")
		return
	}
	l.egressConn = connection

	l.metrics = make(chan *telemetry_edge.NameTagValueMetric, lineProtoMetricsChanSize)

	go l.acceptConnections()

	for {
		select {
		case <-ctx.Done():
			l.listener.Close()
			return

		case metric := <-l.metrics:
			l.sendMetric(metric)
		}
	}
}

func (l *LineProtocol) acceptConnections() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			// errors during accept usually just mean the listener is closed
			log.WithError(err).Debug("error while accepting line protocol connections")
			return
		}

		go l.handleConnection(conn)
	}
}

func (l *LineProtocol) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		metrics, err := lineproto.ParseInfluxLineProtocolMetrics(scanner.Bytes())
		if err != nil {
			log.WithError(err).
				WithField("line", scanner.Text()).
				Error("failed to parse ingested line")
			continue
		}

		for _, metric := range metrics {
			l.metrics <- metric
		}
	}
}

func (l *LineProtocol) sendMetric(metric *telemetry_edge.NameTagValueMetric) {
	outMetric := &telemetry_edge.Metric{
		Variant: &telemetry_edge.Metric_NameTagValue{
			NameTagValue: metric,
		},
	}

	l.egressConn.PostMetric(outMetric)
}
