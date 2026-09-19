package v2ray

import (
	"context"
	"fmt"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// All v2ray transports are stubbed out in the agent build: the generated node
// configs never set a transport, and the variants (http/websocket/grpc/quic)
// drag net/http and HTTP/2 into the binary.
func NewServerTransport(ctx context.Context, logger logger.ContextLogger, options option.V2RayTransportOptions, tlsConfig tls.ServerConfig, handler adapter.V2RayServerTransportHandler) (adapter.V2RayServerTransport, error) {
	if options.Type == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("v2ray transport %q is not compiled into this build", options.Type)
}

func NewClientTransport(ctx context.Context, dialer N.Dialer, serverAddr M.Socksaddr, options option.V2RayTransportOptions, tlsConfig tls.Config) (adapter.V2RayClientTransport, error) {
	if options.Type == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("v2ray transport %q is not compiled into this build", options.Type)
}
