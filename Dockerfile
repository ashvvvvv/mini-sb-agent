FROM golang:1.25-bookworm
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
RUN go mod download
ARG BUILD_TAGS="with_quic with_utls"
# Production NAT nodes use VLESS Reality + Hysteria2 only, so the default omits
# with_gvisor to avoid pulling in the userspace TUN stack and reduce idle RSS.
# Rebuild with --build-arg BUILD_TAGS="with_gvisor with_quic with_utls" if TUN is needed.
RUN go build -trimpath -tags "$BUILD_TAGS" -ldflags "-s -w -buildid=" -o /out/mini-sb-agent ./cmd/mini-sb-agent
