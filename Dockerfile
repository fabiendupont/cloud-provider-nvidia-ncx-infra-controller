# Build the cloud controller manager binary
FROM golang:1.25 AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a \
    -ldflags '-extldflags "-static"' \
    -o nico-cloud-controller-manager \
    cmd/nico-cloud-controller-manager/main.go

# Use distroless as minimal base image
FROM gcr.io/distroless/static-debian12

WORKDIR /

COPY --from=builder /workspace/nico-cloud-controller-manager .

USER 65532:65532

ENTRYPOINT ["/nico-cloud-controller-manager"]
