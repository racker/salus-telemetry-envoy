
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
tls:
  auth_service:
    # The URL of the Salus Authentication Service
    url: http://localhost:8182
    # The provider type to use for authented allocation of client TLS certificates. Further
    # configuration is located at tls.token_providers.<token_provider>
    # Possible options are
    # - keystone_v2 : uses Identity v2 for x-auth-token allocation
    # - static : uses statically provided headers to pass to Salus Authentication Service
    token_provider: keystone_v2
  #Provides client authentication certificates pre-allocated. Remove auth_service config when using this.
  #provided:
    #cert: client.pem
    #key: client-key.pem
    #ca: ca.pem
  token_providers:
    keystone_v2:
      identityServiceUrl: https://identity.api.rackspacecloud.com/v2.0/
      # can also be set by env variable $ENVOY_KEYSTONE_USERNAME
      username: ...
      # can also be set by env variable $ENVOY_KEYSTONE_APIKEY
      apikey: ...
    #Specifies one or more HTTP request headers to pass to authentication service
    #static:
    #  - name: Header-Name
    #    value: headerValue
ambassador:
  # The host:port of the secured gRPC endpoint of the Salus Ambassador
  address: localhost:6565
ingest:
  lumberjack:
    # host:port of where the lumberjack ingestion should bind
    # This is intended for consuming output from filebeat
    bind: localhost:5044
  telegraf:
    json:
      # host:port of where the telegraf json ingestion should bind
      # This socket will accept data output by telegraf using the socket_writer plugin and
      # a data_format of json
      bind: localhost:8094
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

### gRPC code generating

When first setting up the project and after the `telemetry_edge/telemetry-edge.proto` file
is changed, you will need to re-generate the Go source from it using

```
make generate
```

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

