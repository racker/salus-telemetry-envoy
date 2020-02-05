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
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/racker/salus-telemetry-envoy/ambassador"
	"github.com/racker/salus-telemetry-envoy/ambassador/matchers"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestStandardEgressConnection_Start(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	listener, ambassadorAddr, err := setupListener(t)
	//noinspection GoUnhandledErrorResult,GoNilness
	defer listener.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// done simulates an ongoing connection by blocking the attach until test is complete
	var done = make(chan struct{}, 1)
	defer close(done)

	ambassadorService := setupMockAmbassadorServer(done, grpcServer)

	//noinspection GoUnhandledErrorResult
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

	summary, respStream := ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 500*time.Millisecond).
		AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer()).
		GetCapturedArguments()
	assert.Equal(t, "ourResourceId", summary.ResourceId)
	assertIdFromContext(t, "id-1", respStream.Context())
	assert.Equal(t, "myZone", summary.Zone)

	ambassadorService.VerifyWasCalledEventually(pegomock.AtLeast(1), 100*time.Millisecond).
		KeepAlive(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeKeepAliveRequest())
}

func TestStandardEgressConnection_AmbassadorStop(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	listener, ambassadorAddr, err := setupListener(t)
	//noinspection GoUnhandledErrorResult,GoNilness
	defer listener.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// done simulates an ongoing connection by blocking the attach until test is complete
	var done = make(chan struct{}, 1)

	ambassadorService := setupMockAmbassadorServer(done, grpcServer)

	//noinspection GoUnhandledErrorResult
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

	ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 500*time.Millisecond).
		AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer())

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

	listener, ambassadorAddr, err := setupListener(t)
	//noinspection GoUnhandledErrorResult,GoNilness
	defer listener.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// done simulates an ongoing connection by blocking the attach until test is complete
	var done = make(chan struct{}, 1)
	defer close(done)

	ambassadorService := setupMockAmbassadorServer(done, grpcServer)
	pegomock.When(ambassadorService.PostMetric(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgePostedMetric())).
		ThenReturn(&telemetry_edge.PostMetricResponse{}, nil)

	//noinspection GoUnhandledErrorResult
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

	ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 500*time.Millisecond).
		AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer())

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
	t.Log("Posting metric")
	egressConnection.PostMetric(metric)

	t.Log("Verifying server saw posted metric")
	ctx, postedMetric := ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 100*time.Millisecond).
		PostMetric(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgePostedMetric()).
		GetCapturedArguments()

	assert.Equal(t, "cpu", postedMetric.Metric.GetNameTagValue().Name)
	assertIdFromContext(t, "id-1", ctx)
}

func TestStandardEgressConnection_PostTestMonitorResults(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	listener, ambassadorAddr, err := setupListener(t)
	//noinspection GoUnhandledErrorResult,GoNilness
	defer listener.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// done simulates an ongoing connection by blocking the attach until test is complete
	var done = make(chan struct{}, 1)
	defer close(done)

	ambassadorService := setupMockAmbassadorServer(done, grpcServer)
	pegomock.When(ambassadorService.PostTestMonitorResults(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeTestMonitorResults())).
		ThenReturn(&telemetry_edge.PostTestMonitorResultsResponse{}, nil)

	//noinspection GoUnhandledErrorResult
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

	ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 500*time.Millisecond).
		AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer())

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
	results := &telemetry_edge.TestMonitorResults{
		CorrelationId: "test-1",
		Errors:        []string{"simulated error"},
		Metrics:       []*telemetry_edge.Metric{metric},
	}
	egressConnection.PostTestMonitorResults(results)

	ctx, postedResults := ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 100*time.Millisecond).
		PostTestMonitorResults(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeTestMonitorResults()).
		GetCapturedArguments()

	assert.Equal(t, "test-1", postedResults.GetCorrelationId())
	assert.NotEmpty(t, postedResults.GetErrors())
	assert.NotEmpty(t, postedResults.GetMetrics())
	assert.Equal(t, "cpu", postedResults.GetMetrics()[0].GetNameTagValue().Name)
	assertIdFromContext(t, "id-1", ctx)
}

func TestStandardEgressConnection_PostLogEvent(t *testing.T) {
	pegomock.RegisterMockTestingT(t)

	listener, ambassadorAddr, err := setupListener(t)
	//noinspection GoUnhandledErrorResult,GoNilness
	defer listener.Close()

	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()

	// done simulates an ongoing connection by blocking the attach until test is complete
	var done = make(chan struct{}, 1)
	defer close(done)

	ambassadorService := setupMockAmbassadorServer(done, grpcServer)
	pegomock.When(ambassadorService.PostLogEvent(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeLogEvent())).
		ThenReturn(&telemetry_edge.PostLogEventResponse{}, nil)

	//noinspection ALL
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

	ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 500*time.Millisecond).
		AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer())

	egressConnection.PostLogEvent(telemetry_edge.AgentType_FILEBEAT, `{"testing":"value"}`)

	ctx, logEvent := ambassadorService.VerifyWasCalledEventually(pegomock.Once(), 100*time.Millisecond).
		PostLogEvent(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeLogEvent()).
		GetCapturedArguments()
	assert.Equal(t, telemetry_edge.AgentType_FILEBEAT, logEvent.AgentType)
	assert.Equal(t, `{"testing":"value"}`, logEvent.JsonContent)
	assertIdFromContext(t, "id-1", ctx)
}

type testNetworkDialOptionCreator struct{ networks chan string }

func NewTestNetworkDialOptionCreator(networks chan string) *testNetworkDialOptionCreator {
	return &testNetworkDialOptionCreator{networks: networks}
}

// create a dial option for the network parameter, that returns the parameter on the networks chan
func (t *testNetworkDialOptionCreator) Create(network string) grpc.DialOption {
	return grpc.WithContextDialer(
		func(ctx context.Context, addr string) (net.Conn, error) {
			// send back the network being dialed
			t.networks <- network
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
	networksExpected := []string{"tcp4", "tcp6"}

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

func setupListener(t *testing.T) (net.Listener, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ambassadorAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	return listener, ambassadorAddr, err
}

func setupMockAmbassadorServer(done chan struct{}, grpcServer *grpc.Server) *MockTelemetryAmbassadorServer {
	ambassadorService := NewMockTelemetryAmbassadorServer()
	pegomock.When(ambassadorService.AttachEnvoy(matchers.AnyPtrToTelemetryEdgeEnvoySummary(), matchers.AnyTelemetryEdgeTelemetryAmbassadorAttachEnvoyServer())).
		Then(func(params []pegomock.Param) pegomock.ReturnValues {
			<-done
			return nil
		})
	pegomock.When(ambassadorService.KeepAlive(matchers.AnyContextContext(), matchers.AnyPtrToTelemetryEdgeKeepAliveRequest())).
		ThenReturn(&telemetry_edge.KeepAliveResponse{}, nil)

	telemetry_edge.RegisterTelemetryAmbassadorServer(grpcServer, ambassadorService)
	return ambassadorService
}

// assertIdFromContext confirms the gRPC connection populated the metadata with the connecting
// envoy's ID it provided
func assertIdFromContext(t *testing.T, expectedId string, ctx context.Context) {
	md, ok := metadata.FromIncomingContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, expectedId, md.Get(ambassador.EnvoyIdHeader)[0])
}
