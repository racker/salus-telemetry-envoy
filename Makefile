OS := $(shell uname -s)
IAmGroot := $(shell whoami)

.PHONY: default
default: generate build

.PHONY: snapshot
snapshot:
	goreleaser release --rm-dist --snapshot

.PHONY: release
release:
	goreleaser release --rm-dist

.PHONY: build
build:
	go build -o telemetry-envoy .

.PHONY: install
install: test
	go install

.PHONY: generate
generate:
	go generate ./telemetry_edge
	go generate ./...

.PHONY: clean
clean:
	rm -f telemetry-envoy
	rm -rf */matchers
	rm -f */mock_*_test.go

.PHONY: test
test: clean generate
	go test ./...

.PHONY: test-verbose
test-verbose: clean generate
	go test -v ./...

test-report-junit: generate
	mkdir -p test-results
	go test -v ./... 2>&1 | tee test-results/go-test.out
	go install -mod=readonly github.com/jstemmer/go-junit-report
	go-junit-report <test-results/go-test.out > test-results/report.xml

.PHONY: coverage
coverage: generate
	go test -cover ./...

.PHONY: coverage-report
coverage-report: generate
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: init-os-specific init-gotools init
init: init-os-specific init-gotools

ifeq (${OS},Darwin)
init-os-specific:
	-brew install protobuf goreleaser
else
ifeq (${OS},Linux)
init-os-specific:
ifeq (${IAmGroot},root)
	apt-get update
	apt-get install -y protobuf-compiler
else
	sudo apt install -y protobuf-compiler
endif
endif
endif

init-gotools:
	go install -mod=readonly github.com/golang/protobuf/protoc-gen-go
	go install -mod=readonly github.com/petergtz/pegomock/pegomock/...