FROM golang:1.25.3-alpine

ENV CGO_ENABLED=0 \
    GOPROXY=off \
    GOSUMDB=off \
    GOCACHE=/tmp/gocache \
    GOPATH=/tmp/gopath

WORKDIR /workspace
USER 65532:65532
