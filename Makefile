DOCKER_USERNAME=lucachot
CTRL_IMAGE_NAME=basic-sched
GO_FLAGS=

ifdef RACE
	GO_FLAGS += -race
endif

TAG ?= latest

ifeq ($(SCHED), CTL)
BINARY = cmd/central_sched/main.go
BINARY += msg
OUTPUT = bin/central
DOCKER_NAME = central-sched
endif

ifeq ($(SCHED), RMT)
BINARY = cmd/remote_sched/main.go
BINARY += msg
OUTPUT = bin/remote
DOCKER_NAME = remote-sched
endif

ifeq ($(SCHED), AGG)
BINARY = cmd/aggregator/main.go
BINARY += msg
OUTPUT = bin/agg
DOCKER_NAME = aggregator
endif

ifeq ($(SCHED), PROBE)
BINARY = test/main.go
BINARY += msg
OUTPUT = bin/agg-probe
DOCKER_NAME = agg-probe
endif


all: build push

msg: src/message/message.proto
	protoc --go_out=. --go_opt=paths=source_relative \
	--go-grpc_out=. --go-grpc_opt=paths=source_relative $<

compile: ${BINARY}
	go build ${GO_FLAGS} -o ${OUTPUT} $<

build: compile
	docker build -t ${DOCKER_USERNAME}/${DOCKER_NAME}:${TAG} -f docker/${DOCKER_NAME} .

push:
	docker push ${DOCKER_USERNAME}/${DOCKER_NAME}:${TAG}

