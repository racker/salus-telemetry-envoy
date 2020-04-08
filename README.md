
[![CircleCI branch](https://img.shields.io/circleci/project/github/racker/salus-telemetry-envoy/master.svg)](https://circleci.com/gh/racker/salus-telemetry-envoy)

## Run Configuration

Envoy's `run` sub-command can accept some configuration via command-line arguments and/or all
configuration via a yaml file passed via `--config`. The file **must** be named with a suffix
of `.yaml` or `.yml`.

The following is an example configuration file that can be used as a starting point:

```yaml
# The identifier of the resource where this Envoy is running
# The convention is a type:value, but is not required.
resource_id: "type:value"
# Additional key:value string pairs that will be included with Envoy attachment.
labels:
  #environment: production
# Envoy-token allocated for client certificate retrieval
auth_token: ""
tls:
  auth_service:
    # The URL of the Salus Authentication Service
    url: http://localhost:8182
  # `provided` mode can be used for development/testing since it uses pre-allocated certificates. 
  # NOTE: Remove auth_service config when using this.
  #provided:
    #cert: client.pem
    #key: client-key.pem
    #ca: ca.pem
ambassador:
  # The host:port of the secured gRPC endpoint of the Salus Ambassador
  address: localhost:6565
ingest:
  lumberjack:
    # host:port of where the lumberjack ingestion should bind
    # This is intended for consuming output from filebeat
    # This ingestor can be disabled by setting this to an empty value, but will render the filebeat
    # agent unusable.
    # The port can be set to 0 or left off to allow any available port to be used.
    bind: localhost:5044
  telegraf:
    json:
      # host:port of where the telegraf json ingestion should bind
      # This socket will accept data output by telegraf using the socket_writer plugin and
      # a data_format of json
      # This ingestor can be disabled by setting this to an empty value, but will render the telegraf
      # and other agents unusable.
      # The port can be set to 0 or left off to allow any available port to be used.
      bind: localhost:8094
  lineProtocol:
    # host:port where TCP Influx line protocol ingestion should bind
    # This socket will accept data in the same way as telegraf's socket_listener with a data
    # format of "influx".
    # This ingestor can be disabled by setting this to an empty value, but will render the telegraf
    # and other agents unusable.
    # The port can be set to 0 or left off to allow any available port to be used.
    bind: localhost:8194
agents:
  # Data directory where Envoy stores downloaded agents and write agent configs
  dataPath: /var/lib/telemetry-envoy
  # The amount of time an agent is allowed to gracefully stop after a TERM signal. If the
  # timeout is exceeded, then a KILL signal is sent.
  terminationTimeout: 5s
  # The amount of time to pause before each restart of a failed agent process.
  restartDelay: 1s
  # If not specified by the test-monitor instruction, this is the amount of time an agent is 
  # allowed to run while performing a "test monitor" operation
  testMonitorTimeout: 30s
```

## Development

### Environment Setup

This application uses Go 1.11 modules, so be sure to clone this **outside** of the `$GOPATH`.

Speaking of which, some of the code generator tooling does expect `$GOPATH` to be set and the tools to
be available in `$PATH`. As such, it is recommended to add the following to your shell's
setup file (such as `~/.profile` or `~/.bashrc`):

```
export GOPATH=$HOME/go
export PATH="$PATH:$GOPATH/bin"
```

#### Things to Install

First, install Go 1.11 (or newer). On MacOS you can install with `brew install golang`.

After that you can [install the gRPC compiler tooling for Go](https://grpc.io/docs/quickstart/go.html#install-protocol-buffers-v3) 
and [goreleaser](https://goreleaser.com/). 
On MacOS you can install both by performing a 
```bash
make init
```

#### (Re-)Generating Mock Files

Generated mock files are used for unit testing. If using `make test` the generation of those mock files happens automatically; however, if you need to specifically (re)generate those, you can use `make generate`.

### IntelliJ Run Config

This module is actually a submodule of the [salus-telemetry-bundle] (https://github.com/racker/salus-telemetry-bundle).  The following instructions expect that you have installed that repo with this as a submodule.

When using IntelliJ, install the Go plugin from JetBrains and create a run configuration
by right-clicking on the `main.go` file and choosing the "Create ..." option under the
run options.

Choose "Directory" for the "Run Kind"

For ease of configuration, you'll want to set the working directory of the run configuration
to be the `dev` directory of the `telemetry-bundle` project.

Add the following to the "Program arguments":

```
run --config=envoy-config-provided.yml
```

The `envoy-config-provided.yml` can be replaced with one of the other config files located there depending on
the scenario currently running on your system.

### Running from command-line

First, ensure you have completed the steps in the "Environment Setup" section, then...

Build and install the executable by running:

```bash
make install
```

Ensure you have `$GOPATH/bin` in your `$PATH` in order to reference the executable installed by `make install`.

Go over to the bundle repo's `dev` directory and run the built envoy from there:

```bash
cd ../../dev
telemetry-envoy run --debug --config=envoy-config-provided.yml
```

The `envoy-config-provided.yml` can be replaced with one of the other config files located there depending on
the scenario currently running on your system.

### Running detached from an Ambassador

Envoy includes a sub-command called `detached` that can be used to test the Envoy's agent download and configuration mechanism without connecting to an Ambassador. 

The sub-command replaces the support of the Ambassador with the following mechanisms:

- Ingested telemetry (metrics and logs) is encoded to JSON and output to stdout
- Agent installation and configuration instructions are loaded from a JSON file
- Test-Monitor instructions are executed, but the results are output as an info-level log

The instructions file must be structured for unmarshaling into the `DetachedInstructions` protobuf message, declared in the Telemetry Edge protocol. [This example file](cmd/testdata/instructions.json) installs an instance of telegraf and configures a CPU monitor.

The path to the instructions file is passed via `--instructions`. A data directory path should be provided via `--data-path` since the default is not likely suitable for local development environments.

An example invocation of this sub-command is:

```bash
./telemetry-envoy detached --instructions=cmd/testdata/instructions.json --data-path=data
```

With this sub-command, logs are output to stderr so that file descriptor redirects can be used to separate the envoy logging from the posted telemetry content.

### Running perf test mode
Adding the following to the config file starts the envoy in perf test mode at port 8100, with 10 metrics per minute and 20 floats per metric
```bash
perfTest:
  port: 8100
  metricsPerMinute: 10
  floatsPerMetric: 20
```

The rate of metrics generated can be changed with a rest call like so:
```bash
curl http://localhost:8100/ -d metricsPerMinute=10 -d floatsPerMetric=20
```

### Running stress-connections mode

When the [Go build tag](https://golang.org/pkg/go/build/) `dev` is enabled with `-tags dev`, then the sub-command `stress-connections` is available for use, such as:

```
./telemetry-envoy stress-connections --config=envoy-config-provided.yml \
  --connection-count=5 --metrics-per-minute=20
```

This intended use for this mode is stress-testing and profiling the Envoy-Ambassador connectivity performance. It runs a stripped down Envoy that does the following:
- Skips all creation of ingestors
- Skips all registration of agent runners
- Establishes a configurable number of connections (`--connection-count`) to the Ambassador specified in the given `--config` file. Each connection:
    - Is assigned a resource ID `resource-{index}` where `{index}` starts at zero
    - Gets a fabricated metric posted at the requested rate (`--metrics-per-minute`). The metric is named `stress_connection` and contains a single floating-point field named `duration` with a random value. The metric generator randomly staggers the start time for each connection to ensure activity is evenly spread across connections.

> NOTE: In IntelliJ, the build tag preferences are located in "Languages & Framework > Go > Build Tags & Vendoring". The run config also has an option "Use all custom build tags" that needs to be enabled.

### Locally testing gRPC/proto changes

If making local changes to the gRPC/proto files in `salus-telemetry-protocol`, you'll need to make a temporary change to `go.mod` to reference those. Add the following after the `require` block:

```
replace github.com/racker/salus-telemetry-protocol => ../../libs/protocol
```

**NOTE** be sure to re-generate the `*.pb.go` files in `../../libs/protocol` after making `*.proto` changes.


### Executable build

You can build a `telemetry-envoy` executable by using

```
make build
```

### Cross-platform build

If you need to build an executable for all supported platforms, such as for Linux when
developing on MacOS, use

```
make snapshot
```

### Release packages

To perform a new release, create and push a new tag to GitHub, as shown below. You will want to look at the [the latest release](https://github.com/racker/salus-telemetry-envoy/releases/latest) to determine the next appropriate tag/version to apply using [semantic versioning](https://semver.org/).

```
git tag -a 0.1.1 -m 'Description of major feature/fix for release'
git push --tags
``` 

The packages should now be available at https://github.com/racker/salus-telemetry-envoy/releases.

## Architecture
The envoy operates as a single go process with multiple goroutines handling the subfunctions.  It is diagrammed [here](./doc/envoy.png)

### Connection
Responsible for the initial attachment to the ambassador and all communication with it, including receiving config/install instructions for the agents, and passing back log messages/metrics from the ingestors.

### Router
Recieves config/install instructions from the Ambassador, (through the Connection,) and forwards them to the appropriate agentRunner.

### agentRunner
There is one of these for each agent managed by the envoy.  The runners are responsible for managing the agents, which are invoked as child processes of the envoy.  The runners use a library called the command handler that abstracts out all the common interprocess communication functions required by the runners.

### Ingestors
These are tcp servers that receive the log/metric data from the agents and forward them back to the Connection for transmittal back to the ambassador.

