OS := $(shell uname -s)
IAmGroot := $(shell whoami)

.PHONY: default
default: build

.PHONY: snapshot
snapshot:
	goreleaser release --rm-dist --snapshot

.PHONY: release
release:
	curl -sL https://git.io/goreleaser | bash

.PHONY: build
build:
	CGO_ENABLED=0 go build -o telemetry-envoy .

.PHONY: build-dev
build-dev:
	CGO_ENABLED=0 go build -tags dev -o telemetry-envoy .

.PHONY: install
install: test
	go install

.PHONY: generate
generate: generate-mocks

.PHONY: generate-mocks
generate-mocks:
	go generate ./...

.PHONY: clean
clean:
	rm -f telemetry-envoy
	rm -rf */matchers
	rm -f */mock_*_test.go

.PHONY: test
test: clean generate
	go test ./...

.PHONY: retest
retest:
	go test ./...

.PHONY: test-verbose
test-verbose: clean generate
	go test -v ./...

test-report-junit: generate
	mkdir -p test-results
	go test -v ./... 2>&1 | tee test-results/go-test.out
	go get -mod=readonly github.com/jstemmer/go-junit-report
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
	-brew install goreleaser
else
init-os-specific:
	curl -sfL https://install.goreleaser.com/github.com/goreleaser/goreleaser.sh | sh
endif

init-gotools:
	go install github.com/petergtz/pegomock/...