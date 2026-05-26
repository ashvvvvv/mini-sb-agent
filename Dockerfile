FROM golang:1.25-bookworm
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1
RUN go mod download
# Keep TUN support. Keep with_gvisor only for userspace TUN stack; drop DHCP/WireGuard/Tailscale/ACME-related feature tags.
RUN go build -trimpath -tags "with_gvisor with_quic with_utls" -ldflags "-s -w -buildid=" -o /out/mini-sb-agent ./cmd/mini-sb-agent
