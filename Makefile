SHELL := /bin/bash
CURRENT_PATH = $(shell pwd)
APP_NAME = draconis
export GODEBUG=x509ignoreCN=0

GO_BIN = go
ifneq (${GO},)
	GO_BIN = ${GO}
endif

# build with verison infos
BUILD_CONST_DIR = github.com/axiomesh/axiom-ledger/pkg/repo
BUILD_DATE = $(shell date +%FT%T)
GIT_COMMIT = $(shell git log --pretty=format:'%h' -n 1)
GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
ifeq ($(version),)
	# not specify version: make install
	APP_VERSION = $(shell git describe --abbrev=0 --tag)
	ifeq ($(APP_VERSION),)
		APP_VERSION = dev
	endif
else
	# specify version: make install version=v0.6.1-dev
	APP_VERSION = $(version)
endif

GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildDate=${BUILD_DATE}"
GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildCommit=${GIT_COMMIT}"
GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildBranch=${GIT_BRANCH}"
GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildVersion=${APP_VERSION}"

ifneq ($(secret),)
    # specify version: add a flag
    GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildVersionSecret=$(secret)"
endif

ifneq ($(net),)
    # specify version: add a flag
    GOLDFLAGS += -X "${BUILD_CONST_DIR}.BuildNet=$(net)"
endif


COVERAGE_TEST_PKGS := $(shell ${GO_BIN} list ./... | grep -v 'syncer' | grep -v 'vm' | grep -v 'proof' | grep -v 'repo' | grep -v 'mock_*' | grep -v 'tester' | grep -v 'proto' | grep -v 'cmd'| grep -v 'api')
INTEGRATION_COVERAGE_TEST_PKGS :=$(shell ${GO_BIN} list -f '{{if not .Standard}}{{.ImportPath}}{{end}}' -deps ./... |grep -E 'github.com/axiomesh/axiom-ledger|github.com/axiomesh/axiom-bft|github.com/axiomesh/axiom-kit|github.com/axiomesh/axiom-p2p|github.com/axiomesh/eth-kit' | grep -v 'syncer' | grep -v 'vm' | grep -v 'proof' | grep -v 'repo' | grep -v 'mock_*' | grep -v 'tester' | grep -v 'proto' | grep -v 'cmd'| paste -sd ",")
INTEGRATION_COVERAGE_DIR = $(CURRENT_PATH)/integration

RED=\033[0;31m
GREEN=\033[0;32m
BLUE=\033[0;34m
NC=\033[0m

GOARCH := $(or $(GOARCH),$(shell go env GOARCH))
GOOS := $(or $(GOOS),$(shell go env GOOS))

help: Makefile
	@printf "${BLUE}Choose a command run:${NC}\n"
	@sed -n 's/^##//p' $< | column -t -s ':' | sed -e 's/^/    /'

## make prepare: Preparation before development
prepare:
	${GO_BIN} install go.uber.org/mock/mockgen@main
	${GO_BIN} install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.3
	${GO_BIN} install github.com/fsgo/go_fmt/cmd/gorgeous@latest

## make generate-mock: Run go generate
generate-mock:
	${GO_BIN} generate ./...

## make linter: Run golanci-lint
linter:
	golangci-lint run --timeout=5m --new-from-rev=HEAD~1 -v

## make fmt: Formats go source code
fmt:
	gorgeous -local github.com/axiomesh -mi

## make test: Run go unittest
test:
	${GO_BIN} test -timeout 300s ./... -count=1

## make test-coverage: Test project with cover
test-coverage:
	${GO_BIN} test -timeout 300s -short -coverprofile cover.out -covermode=atomic ${COVERAGE_TEST_PKGS}
	cat cover.out | grep -v "pb.go" >> coverage.txt

## make smoke-test: Run smoke test
smoke-test:
	@mkdir -p ${INTEGRATION_COVERAGE_DIR}
	cd scripts && bash smoke_test.sh -b ${BRANCH} -i ${INTEGRATION_COVERAGE_DIR}
	${GO_BIN} tool covdata textfmt -i=${INTEGRATION_COVERAGE_DIR} -pkg=${INTEGRATION_COVERAGE_TEST_PKGS} -o=profile.txt
	${GO_BIN} tool cover -func=profile.txt

## make build-coverage: Go build the project for integration test
build-coverage:
	@mkdir -p bin
	${GO_BIN} build -cover -ldflags '${GOLDFLAGS}' ./cmd/${APP_NAME}
	@cp -f ./${APP_NAME} bin
	@printf "${GREEN}Build ${APP_NAME} successfully!${NC}\n"

## make build: Go build the project
build:
	@mkdir -p bin
	${GO_BIN} build -ldflags '${GOLDFLAGS}' ./cmd/${APP_NAME}
	@cp -f ./${APP_NAME} bin
	@printf "${GREEN}Build ${APP_NAME} successfully!${NC}\n"

## make install: Go install the project
install:
	${GO_BIN} install -ldflags '${GOLDFLAGS}' ./cmd/${APP_NAME}
	@printf "${GREEN}Install ${APP_NAME} successfully!${NC}\n"

## make cluster: Run cluster including 4 nodes
cluster:build
	cd scripts && bash cluster.sh

## make solo: Run one node in solo mode
solo:build
	cd scripts && bash solo.sh

package:build
	cp -f ${APP_NAME} ./scripts/package/tools/bin/${APP_NAME}
	tar czvf ./${APP_NAME}-${APP_VERSION}-${GOARCH}-${GOOS}.tar.gz -C ./scripts/package/ .

## make precommit: Check code like CI
precommit: fmt test-coverage linter

ABI_FOLDER := ./internal/executor/system/sol
FILES := $(wildcard $(ABI_FOLDER)/*.abi)
CONTRACT_NAMES := $(patsubst $(ABI_FOLDER)/%,%,$(basename $(FILES)))
$(CONTRACT_NAMES):
	@echo "Generating ${ABI_FOLDER}/$@.abi"
	abigen --abi $(ABI_FOLDER)/$@.abi --pkg binding --type $@ --out $(ABI_FOLDER)/binding/$@.go;
	@echo "Generated $(ABI_FOLDER)/binding/$@.go"

## make generate-system-contracts-binding: Generate system contracts abi binding
generate-system-contracts-binding: $(CONTRACT_NAMES)