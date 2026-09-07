# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o manager main.go

# Use Red Hat Universal Base Image (UBI) micro as minimal certified base image
FROM registry.access.redhat.com/ubi9/ubi-micro:latest
ARG VERSION="v5.2.0"
WORKDIR /
COPY --from=builder /workspace/manager .
COPY LICENSE /licenses/LICENSE

LABEL name="frappe-operator" \
      vendor="Vyogo Technologies" \
      version="${VERSION}" \
      release="1" \
      summary="Kubernetes Operator for Frappe and ERPNext" \
      description="The Frappe Operator brings FrappeApps like ERPNext natively into Kubernetes and OpenShift."

USER 65532:65532

ENTRYPOINT ["/manager"]
