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
	"encoding/json"
	"github.com/elastic/go-lumber/lj"
	"github.com/elastic/go-lumber/server"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Lumberjack struct {
	egressConn ambassador.EgressConnection
	server     server.Server
}

const ()

func init() {
	viper.SetDefault(config.IngestLumberjackBind, "localhost:5044")

	registerIngestor(&Lumberjack{})
}

func (l *Lumberjack) Bind(connection ambassador.EgressConnection) error {
	l.egressConn = connection

	address := viper.GetString(config.IngestLumberjackBind)

	var err error
	l.server, err = server.ListenAndServe(address, server.V2(true))
	if err != nil {
		return err
	}

	log.WithField("address", address).Debug("Listening for lumberjack")
	return nil
}

// Start processes incoming lumberjack batches
func (l *Lumberjack) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info("closing lumberjack ingest")
			l.server.Close()
			return

		case batch := <-l.server.ReceiveChan():
			l.processLumberjackBatch(batch)
		}
	}
}

func (l *Lumberjack) processLumberjackBatch(batch *lj.Batch) {
	log.WithField("batchSize", len(batch.Events)).Debug("received lumberjack batch")
	for _, event := range batch.Events {
		eventBytes, err := json.Marshal(event)
		if err != nil {
			log.WithError(err).Warn("couldn't marshal")
		} else {
			log.WithField("event", string(eventBytes)).Debug("lumberjack event")
		}

		l.egressConn.PostLogEvent(telemetry_edge.AgentType_FILEBEAT,
			string(eventBytes))
	}
	batch.ACK()
}
