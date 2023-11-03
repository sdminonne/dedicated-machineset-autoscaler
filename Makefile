DIR := ${CURDIR}

BIN_DIR=bin

GO_GCFLAGS ?= -gcflags=all='-N -l'
GO=GOFLAGS=-mod=vendor go
GO_BUILD_RECIPE=CGO_ENABLED=1 $(GO) build $(GO_GCFLAGS)

all: build

build: autoscaler

# Build binary
.PHONY: autoscaler
autoscaler:
	$(GO_BUILD_RECIPE) -o $(BIN_DIR)/autoscaler ./cmd/autoscaler

# test
.PHONY: test
test:
	${GO} test ./...

.PHONY: cover
cover:
	${GO} test -coverprofile=coverage.out ./...

.PHONY: view-cover
view-cover: cover
	${GO}  tool cover -html=coverage.out
