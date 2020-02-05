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
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
)

const (
	telegrafJsonMetricChanSize = 10
)

type TelegrafJson struct {
	listener   net.Listener
	metrics    chan *telegrafJsonMetric
	egressConn ambassador.EgressConnection
}

type telegrafJsonMetric struct {
	Timestamp int64
	Name      string
	Tags      map[string]string
	Fields    map[string]interface{}
}

func init() {
	viper.SetDefault(config.IngestTelegrafJsonBind, "localhost:8094")

	registerIngestor(&TelegrafJson{})
}

func (t *TelegrafJson) Bind() error {
	address := viper.GetString(config.IngestTelegrafJsonBind)
	// check if ingest is disabled via absent config
	if address == "" {
		return nil
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.Wrap(err, "failed to bind telegraf json listener")
	}

	config.RegisterListenerAddress(config.TelegrafJsonListener, listener.Addr().String())
	t.listener = listener

	log.WithField("address", address).Debug("listening for telegraf json")
	return nil
}

func (t *TelegrafJson) Start(ctx context.Context, connection ambassador.EgressConnection) {
	if t.listener == nil {
		log.Debug("telegraf json ingest is disabled")
		return
	}
	t.egressConn = connection

	t.metrics = make(chan *telegrafJsonMetric, telegrafJsonMetricChanSize)
	go t.acceptConnections(ctx)

	for {
		select {
		case <-ctx.Done():
			t.listener.Close()
			return

		case m := <-t.metrics:
			t.processMetric(m)
		}
	}
}

func (t *TelegrafJson) acceptConnections(ctx context.Context) {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			// errors during accept usually just mean the listener is closed
			log.WithError(err).Debug("error while accepting telegraf ingest egressConn")
			return
		} else {
			go t.handleConnection(ctx, conn)
		}
	}
}

func (t *TelegrafJson) handleConnection(ctx context.Context, conn net.Conn) {
	log.WithField("addr", conn.RemoteAddr()).Info("handling telegraf json connection")

	defer conn.Close()

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		var m telegrafJsonMetric

		content := scanner.Bytes()
		if len(content) > 0 {
			err := json.Unmarshal(content, &m)
			if err != nil {
				log.WithError(err).WithField("content", string(content)).Warn("failed to decode telegraf json metric")
			} else {
				log.WithField("m", &m).Debug("successfully unmarshaled json metric")
				t.metrics <- &m
			}
		}
	}

	if scanner.Err() != nil {
		log.WithError(scanner.Err()).Warn("failure while reading json lines")
	}
}

func (t *TelegrafJson) processMetric(m *telegrafJsonMetric) {
	log.WithField("m", m).Debug("processing metric")
	fvalues := make(map[string]float64)
	svalues := make(map[string]string)

	for name, value := range m.Fields {
		switch v := value.(type) {
		case float64:
			fvalues[name] = v
		case string:
			svalues[name] = v
		}
	}

	outMetric := &telemetry_edge.Metric{
		Variant: &telemetry_edge.Metric_NameTagValue{
			NameTagValue: &telemetry_edge.NameTagValueMetric{
				Name:      m.Name,
				Timestamp: m.Timestamp,
				Tags:      m.Tags,
				Fvalues:   fvalues,
				Svalues:   svalues,
			},
		},
	}

	t.egressConn.PostMetric(outMetric)
}
