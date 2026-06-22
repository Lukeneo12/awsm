BINARY := awsm
PKG := ./...
GOCACHE ?= $(TMPDIR)awsm-go-build

.PHONY: build install test cover vet fmt clean

build:
	go build -o bin/$(BINARY) .

install:
	go install .

test:
	go test $(PKG)

cover:
	go test $(PKG) -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

vet:
	go vet $(PKG)

fmt:
	gofmt -l -w .

clean:
	rm -rf bin coverage.out
