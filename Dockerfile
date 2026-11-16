ARG GO_VERSION
FROM golang:${GO_VERSION}-alpine
RUN apk add --no-cache git make \
 && git config --system --add safe.directory '*' \
 && mkdir -p /cache/mod /cache/build && chmod -R 1777 /cache
ENV GOMODCACHE=/cache/mod GOCACHE=/cache/build \
    CGO_ENABLED=0 GOFLAGS=-buildvcs=false HOME=/tmp IN_DOCKER=1
WORKDIR /src
