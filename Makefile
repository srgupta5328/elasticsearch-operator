# Makefile for the Docker image upmcenterprises/elasticsearch-operator
# MAINTAINER: Steve Sloka <slokas@upmc.edu>

.PHONY: all build container push clean test

TAG ?= 0.3.0
PREFIX ?= dtr.predix.io/pcs
pkgs = $(shell go list ./... | grep -v /vendor/ | grep -v /test/)
# go source files, ignore vendor directory
SRC = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

all: container

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -installsuffix cgo -o _output/bin/elasticsearch-operator --ldflags '-w' ./cmd/operator/main.go

container: build
	docker build -t $(PREFIX)/predix-logging:$(TAG) .

push:
	docker push $(PREFIX)/predix-logging:$(TAG)

clean:
	rm -f epredix-logging

format:
	go fmt $(pkgs)

check:
	@go tool vet ${SRC}

helm-package:
	helm package charts/{elasticsearch,elasticsearch-operator,kibana,logstash} -d charts
	helm repo index --merge charts/index.yaml charts

test: clean
	go test $$(go list ./... | grep -v /vendor/)
