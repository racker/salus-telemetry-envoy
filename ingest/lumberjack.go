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
	"encoding/json"
	"github.com/elastic/go-lumber/lj"
	"github.com/elastic/go-lumber/server"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net"
)

type Lumberjack struct {
	egressConn ambassador.EgressConnection
	listener   net.Listener
	server     server.Server
}

const ()

func init() {
	viper.SetDefault(config.IngestLumberjackBind, "")

	registerIngestor(&Lumberjack{})
}

func (l *Lumberjack) Bind() error {
	address := viper.GetString(config.IngestLumberjackBind)
	// check if ingest is disabled via absent config
	if address == "" {
		return nil
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errors.Wrap(err, "failed to bind lumberjack listener")
	}

	config.RegisterListenerAddress(config.LumberjackListener, listener.Addr().String())
	l.listener = listener

	log.WithField("address", address).Debug("listening for lumberjack")
	return nil
}

// Start processes incoming lumberjack batches
func (l *Lumberjack) Start(ctx context.Context, connection ambassador.EgressConnection) {
	if l.listener == nil {
		log.Debug("lumberjack ingest is disabled")
		return
	}
	l.egressConn = connection

	var err error
	l.server, err = server.ListenAndServeWith(func(network, addr string) (net.Listener, error) {
		// provide our own binder so we can intercept and return the listener that was pre-bound
		return l.listener, err
	}, l.listener.Addr().String(), server.V2(true))
	if err != nil {
		log.WithError(err).Fatal("unable to start lumberjack server")
	}

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
