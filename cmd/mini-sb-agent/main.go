package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"mini-sb-agent/counter"
	"mini-sb-agent/panelapi"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/direct"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/tun"
	"github.com/sagernet/sing-box/protocol/vless"
	badjson "github.com/sagernet/sing/common/json"
	N "github.com/sagernet/sing/common/network"
)

type Hook struct {
	byInbound sync.Map
	users     *UserManager
}

func (h *Hook) ResolveUser(user string) string {
	if h.users != nil {
		return h.users.Resolve(user)
	}
	return user
}

func (h *Hook) tc(tag string) *counter.TrafficCounter {
	if tag == "" {
		tag = "default"
	}
	if v, ok := h.byInbound.Load(tag); ok {
		return v.(*counter.TrafficCounter)
	}
	c := counter.NewTrafficCounter()
	v, _ := h.byInbound.LoadOrStore(tag, c)
	return v.(*counter.TrafficCounter)
}
func (h *Hook) RoutedConnection(ctx context.Context, conn net.Conn, m adapter.InboundContext, r adapter.Rule, o adapter.Outbound) net.Conn {
	if m.User == "" {
		return conn
	}
	nodeLimiter, userLimiter := h.limiters(m.User)
	conn = counter.NewConnCounter(conn, h.tc(m.Inbound).GetCounter(h.ResolveUser(m.User)))
	conn = counter.NewRateLimitedConn(conn, nodeLimiter, nodeLimiter)
	conn = counter.NewRateLimitedConn(conn, userLimiter, userLimiter)
	return conn
}
func (h *Hook) RoutedPacketConnection(ctx context.Context, conn N.PacketConn, m adapter.InboundContext, r adapter.Rule, o adapter.Outbound) N.PacketConn {
	if m.User == "" {
		return conn
	}
	nodeLimiter, userLimiter := h.limiters(m.User)
	conn = counter.NewPacketConnCounter(conn, h.tc(m.Inbound).GetCounter(h.ResolveUser(m.User)))
	conn = counter.NewRateLimitedPacketConn(conn, nodeLimiter, nodeLimiter)
	conn = counter.NewRateLimitedPacketConn(conn, userLimiter, userLimiter)
	return conn
}
func (h *Hook) limiters(user string) (*counter.RateLimiter, *counter.RateLimiter) {
	if h.users == nil {
		return nil, nil
	}
	return h.users.Limiters(user)
}
func (h *Hook) Snapshot(reset bool) map[string]map[string][2]int64 {
	out := map[string]map[string][2]int64{}
	h.byInbound.Range(func(k, v any) bool {
		out[k.(string)] = v.(*counter.TrafficCounter).Snapshot(reset)
		return true
	})
	return out
}
func (h *Hook) SnapshotDelta() map[string]map[string][2]int64 {
	out := map[string]map[string][2]int64{}
	h.byInbound.Range(func(k, v any) bool {
		delta := v.(*counter.TrafficCounter).SnapshotDelta()
		if len(delta) > 0 {
			out[k.(string)] = delta
		}
		return true
	})
	return out
}
func (h *Hook) CommitSnapshot(snapshot map[string]map[string][2]int64) {
	for inboundTag, users := range snapshot {
		if v, ok := h.byInbound.Load(inboundTag); ok {
			v.(*counter.TrafficCounter).CommitSnapshot(users)
		}
	}
}
func (h *Hook) RemoveAbsent(active map[string]struct{}) {
	h.byInbound.Range(func(k, v any) bool {
		v.(*counter.TrafficCounter).RemoveAbsent(active)
		return true
	})
}

func minimalContext(parent context.Context) context.Context {
	inbounds := inbound.NewRegistry()
	tun.RegisterInbound(inbounds)
	vless.RegisterInbound(inbounds)
	hysteria2.RegisterInbound(inbounds)

	outbounds := outbound.NewRegistry()
	direct.RegisterOutbound(outbounds)

	return box.Context(parent, inbounds, outbounds, endpoint.NewRegistry(), dns.NewTransportRegistry(), service.NewRegistry())
}

func loadOptions(path string) (option.Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return option.Options{}, err
	}
	ctx := minimalContext(context.Background())
	return badjson.UnmarshalExtendedContext[option.Options](ctx, data)
}

func loadLocalUsers(path string) ([]panelapi.User, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return panelapi.ParseUsers(data)
}

func collectInbounds(b *box.Box) map[string]adapter.Inbound {
	out := make(map[string]adapter.Inbound)
	for _, in := range b.Inbound().Inbounds() {
		out[in.Tag()] = in
	}
	return out
}

func serveStats(ctx context.Context, listen string, h *Hook) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		reset := r.URL.Query().Get("reset") == "1"
		delta := r.URL.Query().Get("delta") == "1"
		w.Header().Set("Content-Type", "application/json")
		if delta {
			json.NewEncoder(w).Encode(h.SnapshotDelta())
			return
		}
		json.NewEncoder(w).Encode(h.Snapshot(reset))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })

	var ln net.Listener
	var err error
	if strings.HasPrefix(listen, "unix:") {
		path := strings.TrimPrefix(listen, "unix:")
		_ = os.Remove(path)
		ln, err = net.Listen("unix", path)
	} else {
		ln, err = net.Listen("tcp", listen)
	}
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Println("stats api", listen)
	return srv.Serve(ln)
}

func main() {
	config := flag.String("config", "config.json", "sing-box config path")
	api := flag.String("api", "unix:/tmp/mini-sb-agent.sock", "local stats API listen addr; empty disables; supports unix:/path.sock")
	users := flag.String("users", "", "local neutral user map json for dynamic protocol users")
	panelURL := flag.String("panel-url", "", "Panel API base URL; empty disables panel sync")
	panelToken := flag.String("panel-token", "", "Panel API node token")
	panelNodeID := flag.String("panel-node-id", "", "Panel API node id")
	panelNodeType := flag.String("panel-node-type", "vless", "Panel API node type")
	panelEvery := flag.Duration("panel-every", time.Minute, "Panel API sync interval")
	nodeRateMbps := flag.Int("node-rate-mbps", 0, "shared node rate limit in Mbps; 0 disables")
	flag.Parse()

	runtime.GOMAXPROCS(1)
	os.Setenv("SING_DNS_PATH", "")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts, err := loadOptions(*config)
	if err != nil {
		log.Fatal(err)
	}
	boxCtx := minimalContext(context.Background())
	b, err := box.New(box.Options{Context: boxCtx, Options: opts})
	if err != nil {
		log.Fatal(err)
	}
	userManager := NewUserManager(*nodeRateMbps)
	h := &Hook{users: userManager}
	b.Router().AppendTracker(h)

	if *api != "" {
		go func() {
			if err := serveStats(ctx, *api, h); err != nil && ctx.Err() == nil {
				log.Println(err)
			}
		}()
	}

	if err := b.Start(); err != nil {
		log.Fatal(err)
	}
	if *users != "" {
		localUsers, err := loadLocalUsers(*users)
		if err != nil {
			log.Fatal(err)
		}
		if err := userManager.ApplyBox(collectInbounds(b), localUsers); err != nil {
			log.Fatal(err)
		}
	}

	var panel panelapi.Panel
	if *panelURL != "" {
		panel = panelapi.NewClient(*panelURL, *panelToken, *panelNodeID, *panelNodeType)
	} else if *users != "" {
		panel = panelapi.LocalUsers{Path: *users}
	}
	if panel != nil {
		syncer := &panelapi.Syncer{
			Panel:    panel,
			Snapshot: h.SnapshotDelta,
			Commit:   h.CommitSnapshot,
			Users: func(list []panelapi.User) error {
				if err := userManager.ApplyBox(collectInbounds(b), list); err != nil {
					return err
				}
				h.RemoveAbsent(userManager.ActiveIDs())
				return nil
			},
			Every: *panelEvery,
		}
		go syncer.Run(ctx)
	}

	<-ctx.Done()
	if err := b.Close(); err != nil {
		log.Println(err)
	}
}
