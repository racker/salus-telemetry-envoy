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

package ambassador

import (
	"context"
	"crypto/tls"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"github.com/racker/salus-telemetry-envoy/agents"
	"github.com/racker/salus-telemetry-envoy/auth"
	"github.com/racker/salus-telemetry-envoy/config"
	"github.com/racker/salus-telemetry-protocol/telemetry_edge"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"net"
	"strings"
	"time"
)

const (
	EnvoyIdHeader = "x-envoy-id"
)

type EgressConnection interface {
	Start(ctx context.Context, supportedAgents []telemetry_edge.AgentType)
	PostLogEvent(agentType telemetry_edge.AgentType, jsonContent string)
	PostMetric(metric *telemetry_edge.Metric)
	PostTestMonitorResults(results *telemetry_edge.TestMonitorResults)
}

type IdGenerator interface {
	Generate() string
}

type StandardIdGenerator struct{}

func NewIdGenerator() IdGenerator {
	return &StandardIdGenerator{}
}

func (g *StandardIdGenerator) Generate() string {
	return uuid.NewV1().String()
}

type NetworkDialOptionCreator interface {
	Create(string) grpc.DialOption
}

type StandardNetworkDialOptionCreator struct{}

func NewNetworkDialOptionCreator() NetworkDialOptionCreator {
	return &StandardNetworkDialOptionCreator{}
}

// create a dial option for the network parameter, (tcp4 or 6)
func (g *StandardNetworkDialOptionCreator) Create(network string) grpc.DialOption {
	return grpc.WithContextDialer(
		func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		})
}

type StandardEgressConnection struct {
	Address           string
	TlsDisabled       bool
	GrpcCallLimit     time.Duration
	KeepAliveInterval time.Duration

	client            telemetry_edge.TelemetryAmbassadorClient
	envoyId           string
	ctx               context.Context
	agentsRunner      agents.Router
	detachChan        chan<- struct{}
	grpcTlsDialOption grpc.DialOption
	certificate       *tls.Certificate
	supportedAgents   []telemetry_edge.AgentType
	idGenerator       IdGenerator
	labels            map[string]string
	resourceId        string
	// outgoingContext is used by gRPC client calls to build the final call context
	outgoingContext          context.Context
	networkDialOptionCreator NetworkDialOptionCreator
	attached                 bool
}

func init() {
	viper.SetDefault(config.AmbassadorAddress, "localhost:6565")
	viper.SetDefault("grpc.callLimit", 30*time.Second)
	viper.SetDefault("ambassador.keepAliveInterval", 45*time.Second)
}

// NewEgressConnection creates the component that manages connections out to a Salus Ambassador.
// The detachChan provides a signalling mechanism to the agent runner to let is know when an
// attachment to an Ambassador was terminated.
func NewEgressConnection(agentsRunner agents.Router, detachChan chan<- struct{}, idGenerator IdGenerator,
	networkDialOptionCreator NetworkDialOptionCreator) (EgressConnection, error) {
	resourceId := viper.GetString(config.ResourceId)
	if resourceId == "" {
		return nil, errors.Errorf("Envoy configuration is missing %s", config.ResourceId)
	}

	connection := &StandardEgressConnection{
		Address:                  viper.GetString(config.AmbassadorAddress),
		TlsDisabled:              viper.GetBool("tls.disabled"),
		GrpcCallLimit:            viper.GetDuration("grpc.callLimit"),
		KeepAliveInterval:        viper.GetDuration("ambassador.keepAliveInterval"),
		agentsRunner:             agentsRunner,
		detachChan:               detachChan,
		idGenerator:              idGenerator,
		resourceId:               resourceId,
		networkDialOptionCreator: networkDialOptionCreator,
	}

	var err error
	connection.grpcTlsDialOption, err = connection.loadTlsDialOption()
	if err != nil {
		return nil, err
	}

	connection.labels, err = config.ComputeLabels()
	if err != nil {
		return nil, err
	}

	return connection, nil
}

func (c *StandardEgressConnection) Start(ctx context.Context, supportedAgents []telemetry_edge.AgentType) {

	log.WithFields(log.Fields{
		"resourceId": c.resourceId,
	}).Debug("Starting connection to Ambassador")

	c.ctx = ctx
	c.supportedAgents = supportedAgents

	for {
		select {
		case <-c.ctx.Done():
			log.Info("Stopping egress connection handling")
			return

		default:
			err := backoff.RetryNotify(c.attach, backoff.WithContext(backoff.NewExponentialBackOff(), c.ctx),
				func(err error, delay time.Duration) {
					log.WithError(err).WithField("delay", delay).
						WithField("envoyId", c.envoyId).
						Warn("delaying until next attempt")

					cause := errors.Cause(err)
					if cause != nil && cause != err {
						if strings.Contains(cause.Error(), "tls: expired certificate") {
							log.Warn("authenticating certificate has expired, reloading certificates")

							var loadErr error
							c.grpcTlsDialOption, loadErr = c.loadTlsDialOption()
							if loadErr != nil {
								log.WithError(loadErr).Warn("failed to reload certificates")
							}
						}
					}
				})
			if err != nil && err.Error() != "closed" {
				log.WithError(err).Warn("failure during retry section")
			}

			c.envoyId = c.idGenerator.Generate()
		}
	}
}

// dialNetwork takes a network, (tcp4 or 6,) and returns a connection to that network
func (c *StandardEgressConnection) dialNetwork(network string, dialTimeoutCtx context.Context) (*grpc.ClientConn, error) {
	networkDialOption := c.networkDialOptionCreator.Create(network)
	return grpc.DialContext(dialTimeoutCtx,
		c.Address,
		c.grpcTlsDialOption,

		grpc.WithBlock(),
		grpc.FailOnNonTempDialError(true),
		networkDialOption,
	)
}

func (c *StandardEgressConnection) attach() error {

	c.envoyId = c.idGenerator.Generate()
	log.
		WithField("ambassadorAddress", c.Address).
		WithField("envoyId", c.envoyId).
		WithField("resourceId", c.resourceId).
		Info("dialing ambassador")
	// use a blocking dial, but fail on non-temp errors so that we can catch connectivity errors here rather than during
	// the attach operation

	// try both IPv6 and IPv4
	dialTimeoutCtxTcp6, dialTimeoutTcp6Cancel := context.WithTimeout(c.ctx, c.GrpcCallLimit)
	defer dialTimeoutTcp6Cancel()

	conn, err := c.dialNetwork("tcp6", dialTimeoutCtxTcp6)
	if err != nil {
		log.WithError(err).Debugf("tcp6 connection failed, trying tcp4...")

		dialTimeoutCtxTcp4, dialTimeoutTcp4Cancel := context.WithTimeout(c.ctx, c.GrpcCallLimit)
		defer dialTimeoutTcp4Cancel()
		conn, err = c.dialNetwork("tcp4", dialTimeoutCtxTcp4)
	}
	if err != nil {
		return errors.Wrap(err, "failed to dial Ambassador")
	}
	//noinspection GoUnhandledErrorResult
	defer conn.Close()

	c.client = telemetry_edge.NewTelemetryAmbassadorClient(conn)
	callMetadata := metadata.Pairs(EnvoyIdHeader, c.envoyId)

	// connCtx creates a scope where the go routines for each connection can all be
	// cancelled in one-shot when an error is reported by any of them. It also inherits
	// from the application context, where a termination signal will also mark the context as "done"
	connCtx, cancelFunc := context.WithCancel(c.ctx)
	// outgoingCtx further extends the context by populating headers that will be passed along
	// with each gRPC call.
	outgoingCtx := metadata.NewOutgoingContext(connCtx, callMetadata)
	c.outgoingContext = outgoingCtx

	envoySummary := &telemetry_edge.EnvoySummary{
		SupportedAgents: c.supportedAgents,
		Labels:          c.labels,
		ResourceId:      c.resourceId,
		Zone:            viper.GetString(config.Zone),
	}

	instructions, err := c.client.AttachEnvoy(outgoingCtx, envoySummary)
	if err != nil {
		return errors.Wrap(err, "failed to attach Envoy")
	}
	log.WithField("summary", envoySummary).Info("attached")

	errChan := make(chan error, 10)

	go c.watchForInstructions(outgoingCtx, errChan, instructions)
	for {
		select {
		case <-outgoingCtx.Done():
			c.attached = false
			log.Debug("closing attach receiver stream")
			err := instructions.CloseSend()
			if err != nil {
				log.WithError(err).Warn("closing send side of instructions stream")
			}
			return errors.New("closed")

		case err := <-errChan:
			c.attached = false
			log.WithError(err).WithField("envoyId", c.envoyId).
				Warn("terminating connection due to error")
			// notify listeners of detachments, if needed
			if c.detachChan != nil {
				c.detachChan <- struct{}{}
			}
			cancelFunc()
		}
	}
}

func (c *StandardEgressConnection) PostLogEvent(agentType telemetry_edge.AgentType, jsonContent string) {
	if !c.attached {
		log.WithField("envoyId", c.envoyId).
			WithField("agentType", agentType).
			WithField("content", jsonContent).
			Warn("unable to post log event due to unattached connection")
		return
	}
	callCtx, callCancel := context.WithTimeout(c.outgoingContext, c.GrpcCallLimit)
	defer callCancel()

	log.Debug("posting log event")
	_, err := c.client.PostLogEvent(callCtx, &telemetry_edge.LogEvent{
		AgentType:   agentType,
		JsonContent: jsonContent,
	})
	if err != nil {
		log.WithError(err).Warn("failed to post log event")
	}
}

func (c *StandardEgressConnection) PostMetric(metric *telemetry_edge.Metric) {
	if !c.attached {
		log.WithField("envoyId", c.envoyId).
			WithField("metric", metric).
			Warn("unable to post metric due to unattached connection")
		return
	}
	callCtx, callCancel := context.WithTimeout(c.outgoingContext, c.GrpcCallLimit)
	defer callCancel()

	log.WithField("metric", metric).Debug("posting metric")
	_, err := c.client.PostMetric(callCtx, &telemetry_edge.PostedMetric{
		Metric: metric,
	})
	if err != nil {
		log.WithError(err).
			WithField("metric", metric).
			Warn("failed to post metric")
	}
}

func (c *StandardEgressConnection) PostTestMonitorResults(results *telemetry_edge.TestMonitorResults) {
	callCtx, callCancel := context.WithTimeout(c.outgoingContext, c.GrpcCallLimit)
	defer callCancel()

	log.WithField("results", results).Debug("posting test monitor results")
	_, err := c.client.PostTestMonitorResults(callCtx, results)
	if err != nil {
		log.WithError(err).
			WithField("results", results).
			Warn("failed to post test monitor results")
	}
}

func (c *StandardEgressConnection) sendKeepAlives(ctx context.Context, errChan chan<- error) {
	for {
		select {
		case <-time.After(c.KeepAliveInterval):
			callCtx, callCancel := context.WithTimeout(c.outgoingContext, c.GrpcCallLimit)
			_, err := c.client.KeepAlive(callCtx, &telemetry_edge.KeepAliveRequest{})
			if err != nil {
				errChan <- errors.Wrap(err, "failed to send keep alive")
				callCancel()
				return
			}
			callCancel()

		case <-ctx.Done():
			return
		}
	}
}

func (c *StandardEgressConnection) loadTlsDialOption() (grpc.DialOption, error) {
	if c.TlsDisabled {
		return grpc.WithInsecure(), nil
	}

	certificate, certPool, err := auth.LoadCertificates()
	if err != nil {
		return nil, err
	}
	c.certificate = certificate

	transportCreds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{*certificate},
		RootCAs:      certPool,
	})
	return grpc.WithTransportCredentials(transportCreds), nil
}

func (c *StandardEgressConnection) watchForInstructions(ctx context.Context,
	errChan chan<- error, instructions telemetry_edge.TelemetryAmbassador_AttachEnvoyClient) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
			instruction, err := instructions.Recv()
			if err != nil {
				errChan <- errors.Wrap(err, "failed to receive instruction")
				return
			}

			switch {
			case instruction.GetReady() != nil:
				log.Debug("ambassador is ready")
				c.attached = true
				go c.sendKeepAlives(ctx, errChan)

			case instruction.GetInstall() != nil:
				c.agentsRunner.ProcessInstall(instruction.GetInstall())

			case instruction.GetConfigure() != nil:
				c.agentsRunner.ProcessConfigure(instruction.GetConfigure())

			case instruction.GetTestMonitor() != nil:
				c.processTestMonitor(instruction.GetTestMonitor())

			case instruction.GetRefresh() != nil:
				//TODO
			}
		}
	}
}

func (c *StandardEgressConnection) processTestMonitor(testMonitor *telemetry_edge.EnvoyInstructionTestMonitor) {
	// Test monitors are blocking since they typically require starting a temporary instance of
	// the agent and gathering its results.
	//
	// ...so let it process concurrently and we'll post the return value when it's available.
	//
	// FYI letting the agent code post back to the ambassador code would have introduced a package
	// import cycle.
	go func() {
		// any errors within the operation are stored into the returned results structure
		results := c.agentsRunner.ProcessTestMonitor(testMonitor)
		if results != nil {
			c.PostTestMonitorResults(results)
		} else {
			log.
				WithField("instruction", testMonitor).
				Warn("Unexpected nil test monitor results")
		}
	}()
}
