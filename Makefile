BINARY_NAME := $(shell basename "$(PWD)")
GOBASE := $(shell pwd)
GOBIN := $(GOBASE)/bin

GOOS := "linux"
GOARCH := "amd64"

all: build

build:
	@echo "  >  Building binary..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(GOBIN)/$(BINARY_NAME) *.go