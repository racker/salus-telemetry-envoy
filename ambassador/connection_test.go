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

package ambassador_test

import (
	"context"
	"github.com/petergtz/pegomock"
	"github.com/phayes/freeport"
	"github.com/racker/telemetry-envoy/ambassador"
	"github.com/racker/telemetry-envoy/config"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netContext "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
	"strconv"
	"testing"
	"time"
)

type TestingAmbassadorService struct {
	idViaAttach       string
	idViaKeepAlive    string
	idViaPostMetric   string
	idViaPostLogEvent string

	done       chan struct{}
	attaches   chan *telemetry_edge.EnvoySummary
	keepAlives chan *telemetry_edge.KeepAliveRequest
	logs       chan *telemetry_edge.LogEvent
	metrics    chan *telemetry_edge.PostedMetric
}

func NewTestingAmbassadorService(done chan struct{}) *TestingAmbassadorService {
	return &TestingAmbassadorService{
		done:       done,
		attaches:   make(chan *telemetry_edge.EnvoySummary, 1),
		keepAlives: make(chan *telemetry_edge.KeepAliveRequest, 1),
		logs:       make(chan *telemetry_edge.LogEvent, 1),
		metrics:    make(chan *telemetry_edge.PostedMetric, 1),
	}
}

func (s *TestingAmbassadorService) AttachEnvoy(summary *telemetry_edge.EnvoySummary, resp telemetry_edge.TelemetryAmbassador_AttachEnvoyServer) error {
	if md, ok := metadata.FromIncomingContext(resp.Context()); ok {
		s.idViaAttach = md.Get(ambassador.EnvoyIdHeader)[0]
	}
	s.attaches <- summary
	<-s.done
	return nil
}

func (s *TestingAmbassadorService) KeepAlive(ctx netContext.Context, req *telemetry_edge.KeepAliveRequest) (*telemetry_edge.KeepAliveResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.idViaKeepAlive = md.Get(ambassador.EnvoyIdHeader)[0]
	}
	s.keepAlives <- req
	return &telemetry_edge.KeepAliveResponse{}, nil
}

func (s *TestingAmbassadorService) PostLogEvent(ctx netContext.Context, log *telemetry_edge.LogEvent) (*telemetry_edge.PostLogEventResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.idViaPostLogEvent = md.Get(ambassador.EnvoyIdHeader)[0]
	}
	s.logs <- log
	return &telemetry_edge.PostLogEventResponse{}, nil
}

func (s *TestingAmbassadorService) PostMetric(ctx netContext.Context, metric *telemetry_edge.PostedMetric) (*telemetry_edge.PostMetricResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.idViaPostMetric = md.Get(ambassador.EnvoyIdHeader)[0]
	}
	s.metrics <- metric
	return &telemetry_edge.PostMetricResponse{}, nil
}

func TestStandardEgressConnection_Start(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	ambassadorPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	ambassadorAddr := net.JoinHostPort("localhost", strconv.Itoa(ambassadorPort))
	listener, err := net.Listen("tcp", ambassadorAddr)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	var done = make(chan struct{}, 1)
	defer close(done)
	ambassadorService := NewTestingAmbassadorService(done)
	telemetry_edge.RegisterTelemetryAmbassadorServer(grpcServer, ambassadorService)

	go grpcServer.Serve(listener)

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "ourResourceId")
	viper.Set(config.Zone, "myZone")
	viper.Set(config.AmbassadorAddress, ambassadorAddr)
	viper.Set("tls.disabled", true)
	viper.Set("ambassador.keepAliveInterval", 1*time.Millisecond)

	detachChan := make(chan struct{}, 1)
	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		ambassador.NewNetworkDialOptionCreator())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go egressConnection.Start(ctx, []telemetry_edge.AgentType{telemetry_edge.AgentType_TELEGRAF})
	defer cancel()

	select {
	case summary := <-ambassadorService.attaches:
		assert.Equal(t, "ourResourceId", summary.ResourceId)
		assert.Equal(t, "id-1", ambassadorService.idViaAttach)
		assert.Equal(t, "myZone", summary.Zone)
	case <-time.After(500 * time.Millisecond):
		t.Error("did not see attachment in time")
	}

	select {
	case <-ambassadorService.keepAlives:
		// good
		assert.Equal(t, "id-1", ambassadorService.idViaKeepAlive)
	case <-time.After(100 * time.Millisecond):
		t.Error("did not see keep alive in time")
	}
}

func TestStandardEgressConnection_AmbassadorStop(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	ambassadorPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	ambassadorAddr := net.JoinHostPort("localhost", strconv.Itoa(ambassadorPort))
	listener, err := net.Listen("tcp", ambassadorAddr)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	var done = make(chan struct{}, 1)
	ambassadorService := NewTestingAmbassadorService(done)
	telemetry_edge.RegisterTelemetryAmbassadorServer(grpcServer, ambassadorService)

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "ourResourceId")
	viper.Set(config.Zone, "myZone")
	viper.Set(config.AmbassadorAddress, ambassadorAddr)
	viper.Set("tls.disabled", true)
	viper.Set("ambassador.keepAliveInterval", 1*time.Millisecond)

	detachChan := make(chan struct{}, 1)
	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		ambassador.NewNetworkDialOptionCreator())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go egressConnection.Start(ctx, []telemetry_edge.AgentType{telemetry_edge.AgentType_TELEGRAF})
	defer cancel()

	select {
	case summary := <-ambassadorService.attaches:
		assert.Equal(t, "ourResourceId", summary.ResourceId)
		assert.Equal(t, "id-1", ambassadorService.idViaAttach)
		assert.Equal(t, "myZone", summary.Zone)
	case <-time.After(500 * time.Millisecond):
		t.Error("did not see attachment in time")
	}

	// Stop the ambassador
	t.Log("stopping ambassador")
	close(done)
	grpcServer.Stop()

	select {
	case <-detachChan:
		// good
		t.Log("saw detach")
	case <-time.After(500 * time.Millisecond):
		t.Error("did not see detach in time")
	}
}

func TestStandardEgressConnection_MissingResourceId(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "")
	detachChan := make(chan struct{}, 1)
	
	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		ambassador.NewNetworkDialOptionCreator())
	require.Error(t, err)
	require.Nil(t, egressConnection)
}

func TestStandardEgressConnection_PostMetric(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	ambassadorPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	ambassadorAddr := net.JoinHostPort("localhost", strconv.Itoa(ambassadorPort))
	listener, err := net.Listen("tcp", ambassadorAddr)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	done := make(chan struct{}, 1)
	defer close(done)
	ambassadorService := NewTestingAmbassadorService(done)
	telemetry_edge.RegisterTelemetryAmbassadorServer(grpcServer, ambassadorService)

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "ourResourceId")
	viper.Set(config.AmbassadorAddress, ambassadorAddr)
	viper.Set("tls.disabled", true)

	detachChan := make(chan struct{}, 1)
	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		ambassador.NewNetworkDialOptionCreator())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go egressConnection.Start(ctx, []telemetry_edge.AgentType{telemetry_edge.AgentType_TELEGRAF})
	defer cancel()

	select {
	case <-ambassadorService.attaches:
		//continue
	case <-time.After(500 * time.Millisecond):
		t.Log("did not see attachment in time")
		t.FailNow()
	}

	metric := &telemetry_edge.Metric{
		Variant: &telemetry_edge.Metric_NameTagValue{
			NameTagValue: &telemetry_edge.NameTagValueMetric{
				Name: "cpu",
				Tags: map[string]string{
					"cpu": "cpu1",
				},
				Fvalues: map[string]float64{
					"usage": 12.34,
				},
			},
		},
	}
	egressConnection.PostMetric(metric)

	select {
	case postedMetric := <-ambassadorService.metrics:
		assert.Equal(t, "cpu", postedMetric.Metric.GetNameTagValue().Name)
		assert.Equal(t, "id-1", ambassadorService.idViaPostMetric)

	case <-time.After(100 * time.Millisecond):
		t.Error("did not see posted metric in time")
	}
}

func TestStandardEgressConnection_PostLogEvent(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	ambassadorPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	ambassadorAddr := net.JoinHostPort("localhost", strconv.Itoa(ambassadorPort))
	listener, err := net.Listen("tcp", ambassadorAddr)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	done := make(chan struct{}, 1)
	defer close(done)
	ambassadorService := NewTestingAmbassadorService(done)
	telemetry_edge.RegisterTelemetryAmbassadorServer(grpcServer, ambassadorService)

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "ourResourceId")
	viper.Set(config.AmbassadorAddress, ambassadorAddr)
	viper.Set("tls.disabled", true)
	detachChan := make(chan struct{}, 1)

	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		ambassador.NewNetworkDialOptionCreator())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go egressConnection.Start(ctx, []telemetry_edge.AgentType{telemetry_edge.AgentType_FILEBEAT})
	defer cancel()

	select {
	case <-ambassadorService.attaches:
		//continue
	case <-time.After(500 * time.Millisecond):
		t.Log("did not see attachment in time")
		t.FailNow()
	}

	egressConnection.PostLogEvent(telemetry_edge.AgentType_FILEBEAT, `{"testing":"value"}`)

	select {
	case logEvent := <-ambassadorService.logs:
		assert.Equal(t, telemetry_edge.AgentType_FILEBEAT, logEvent.AgentType)
		assert.Equal(t, `{"testing":"value"}`, logEvent.JsonContent)
		assert.Equal(t, "id-1", ambassadorService.idViaPostLogEvent)

	case <-time.After(100 * time.Millisecond):
		t.Error("did not see posted metric in time")
	}
}


type testNetworkDialOptionCreator struct{networks chan string}
func NewTestNetworkDialOptionCreator(networks chan string) *testNetworkDialOptionCreator {
	return &testNetworkDialOptionCreator{networks: networks}
}

// create a dial option for the network parameter, that returns the parameter on the networks chan
func (t *testNetworkDialOptionCreator) Create(network string) grpc.DialOption {
	return grpc.WithContextDialer(
		func (ctx context.Context, addr string) (net.Conn, error) {
			// send back the network being dialed
			t.networks<-network
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		})
}

func TestStandardEgressConnection_NetworkDialOptionCreator(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	idGenerator := NewMockIdGenerator()
	pegomock.When(idGenerator.Generate()).ThenReturn("id-1")

	ambassadorPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	ambassadorAddr := net.JoinHostPort("localhost", strconv.Itoa(ambassadorPort))
	mockAgentsRunner := NewMockRouter()
	viper.Set(config.ResourceId, "ourResourceId")
	viper.Set("tls.disabled", true)
	viper.Set(config.AmbassadorAddress, ambassadorAddr)
	detachChan := make(chan struct{}, 1)

	networks := make(chan string)
	testNetworkDialOptionCreator := NewTestNetworkDialOptionCreator(networks)

	egressConnection, err := ambassador.NewEgressConnection(mockAgentsRunner, detachChan, idGenerator,
		testNetworkDialOptionCreator)
	require.NoError(t, err)

	ctx, _ := context.WithCancel(context.Background())
	go egressConnection.Start(ctx, []telemetry_edge.AgentType{telemetry_edge.AgentType_FILEBEAT})

	var networksTested []string
	networksExpected :=[]string {"tcp4", "tcp6"}

	// wait for the expected number of messages on the networks chan
	for range networksExpected {
		select {
		case n := <-networks:
			networksTested = append(networksTested, n)
		case <-time.After(500 * time.Millisecond):
			t.Log("did not see networks tested in time")
			t.FailNow()
		}
	}

	assert.ElementsMatch(t, networksExpected, networksTested, "expected to see tcp4 and tcp6")
}

