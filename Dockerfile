FROM golang:bullseye as cache
WORKDIR /src
COPY go.mod go.sum .
RUN  env GOPROXY=https://goproxy.io go mod download

FROM golang:bullseye as build-base
COPY --from=cache /go/pkg/mod /go/pkg/mod
COPY . /src/
WORKDIR /src

FROM build-base as build-server
RUN  go build -o /bin/signal-server ./cmd/signal-server

FROM debian:bullseye as runtime
ENTRYPOINT /bin/signal-server

FROM result as result-updated
RUN  apt update && apt dist-upgrade --yes && apt clean

FROM runtime as result
COPY --from=build-server /bin/signal-server /bin/signal-server

