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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a \
    -ldflags '-extldflags "-static"' \
    -o nvidia-carbide-cloud-controller-manager \
    cmd/nvidia-carbide-cloud-controller-manager/main.go

# Use distroless as minimal base image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/nvidia-carbide-cloud-controller-manager .

USER 65532:65532

ENTRYPOINT ["/nvidia-carbide-cloud-controller-manager"]
