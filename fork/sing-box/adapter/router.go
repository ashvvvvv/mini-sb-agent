package adapter

import (
	"context"
	"net"
	"time"

	"github.com/sagernet/sing-tun"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/x/list"

	"go4.org/netipx"
)

type Router interface {
	Lifecycle
	ConnectionRouter
	PreMatch(metadata InboundContext, context tun.DirectRouteContext, timeout time.Duration) (tun.DirectRouteDestination, error)
	ConnectionRouterEx
	RuleSet(tag string) (RuleSet, bool)
	NeedWIFIState() bool
	Rules() []Rule
	AppendTracker(tracker ConnectionTracker)
	ResetNetwork()
	//for v2bx
	GetCtx() context.Context
}

type ConnectionTracker interface {
	RoutedConnection(ctx context.Context, conn net.Conn, metadata InboundContext, matchedRule Rule, matchOutbound Outbound) net.Conn
	RoutedPacketConnection(ctx context.Context, conn N.PacketConn, metadata InboundContext, matchedRule Rule, matchOutbound Outbound) N.PacketConn
}

// Deprecated: Use ConnectionRouterEx instead.
type ConnectionRouter interface {
	RouteConnection(ctx context.Context, conn net.Conn, metadata InboundContext) error
	RoutePacketConnection(ctx context.Context, conn N.PacketConn, metadata InboundContext) error
}

type ConnectionRouterEx interface {
	ConnectionRouter
	RouteConnectionEx(ctx context.Context, conn net.Conn, metadata InboundContext, onClose N.CloseHandlerFunc)
	RoutePacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata InboundContext, onClose N.CloseHandlerFunc)
}

type RuleSet interface {
	Name() string
	StartContext(ctx context.Context, startContext *HTTPStartContext) error
	PostStart() error
	Metadata() RuleSetMetadata
	ExtractIPSet() []*netipx.IPSet
	IncRef()
	DecRef()
	Cleanup()
	RegisterCallback(callback RuleSetUpdateCallback) *list.Element[RuleSetUpdateCallback]
	UnregisterCallback(element *list.Element[RuleSetUpdateCallback])
	Close() error
	HeadlessRule
}

type RuleSetUpdateCallback func(it RuleSet)

type RuleSetMetadata struct {
	ContainsProcessRule bool
	ContainsWIFIRule    bool
	ContainsIPCIDRRule  bool
}
// HTTPStartContext is a stub in this build: remote HTTP resources (remote
// rule-sets) are not compiled in, so no http.Client is ever created.
type HTTPStartContext struct {
	ctx context.Context
}

func NewHTTPStartContext(ctx context.Context) *HTTPStartContext {
	return &HTTPStartContext{ctx: ctx}
}

func (c *HTTPStartContext) Close() {}
