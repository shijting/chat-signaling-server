PROTOC ?= $(shell which protoc)

PROTOS := $(shell find -name '*.proto')
PROTO_GEN_PB := $(subst .proto,.pb.go,$(PROTOS))
PROTO_GEN_GRPC := $(subst .proto,_grpc.pb.go,$(PROTOS))

GO_SRC := $(shell find -name '*.go')

.PHONY: all
all: build

.PHONY: build
build: bin/signal-server

bin/%: $(GO_SRC) $(PROTO_GEN_PB) $(PROTO_GEN_GRPC)
	go build -o $@ ./$<

%_grpc.pb.go: %.proto
	protoc --go-grpc_out=. --go-grpc_opt=paths=source_relative $<

%.pb.go: %.proto
	protoc --go_out=. --go_opt=paths=source_relative $<
