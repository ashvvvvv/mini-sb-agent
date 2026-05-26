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
	"mini-sb-agent/xboard"

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
	userAlias sync.Map
}

func (h *Hook) ResolveUser(user string) string {
	if user == "" {
		return ""
	}
	if v, ok := h.userAlias.Load(user); ok {
		return v.(string)
	}
	return user
}

func (h *Hook) SetUserAliases(users []xboard.User) {
	for _, u := range users {
		if u.ID <= 0 {
			continue
		}
		id := fmt.Sprint(u.ID)
		if u.UUID != "" {
			h.userAlias.Store(u.UUID, id)
		}
		if u.Password != "" {
			h.userAlias.Store(u.Password, id)
		}
		if u.Name != "" {
			h.userAlias.Store(u.Name, id)
		}
	}
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
	return counter.NewConnCounter(conn, h.tc(m.Inbound).GetCounter(h.ResolveUser(m.User)))
}
func (h *Hook) RoutedPacketConnection(ctx context.Context, conn N.PacketConn, m adapter.InboundContext, r adapter.Rule, o adapter.Outbound) N.PacketConn {
	if m.User == "" {
		return conn
	}
	return counter.NewPacketConnCounter(conn, h.tc(m.Inbound).GetCounter(h.ResolveUser(m.User)))
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

type UserMap struct {
	Inbounds map[string][]UserEntry `json:"inbounds"`
}
type UserEntry struct {
	ID       int    `json:"id"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
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

func injectHy2Users(b *box.Box, path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m UserMap
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for tag, list := range m.Inbounds {
		if len(list) == 0 {
			continue
		}
		raw, ok := b.Inbound().Get(tag)
		if !ok {
			return fmt.Errorf("inbound %s not found", tag)
		}
		in, ok := raw.(*hysteria2.Inbound)
		if !ok {
			return fmt.Errorf("inbound %s is not hysteria2", tag)
		}
		users := make([]option.Hysteria2User, 0, len(list))
		ids := make([]int, 0, len(list))
		seen := map[int]bool{}
		for _, u := range list {
			if u.ID <= 0 {
				return fmt.Errorf("inbound %s has invalid user id %d; use >=1", tag, u.ID)
			}
			if seen[u.ID] {
				return fmt.Errorf("inbound %s duplicate user id %d", tag, u.ID)
			}
			seen[u.ID] = true
			if u.Password == "" {
				return fmt.Errorf("inbound %s user id %d empty password", tag, u.ID)
			}
			name := u.Name
			if name == "" {
				name = u.Password
			}
			users = append(users, option.Hysteria2User{Name: name, Password: u.Password})
			ids = append(ids, u.ID)
		}
		if err := in.AddUsers(users, ids); err != nil {
			return err
		}
		log.Printf("injected %d hysteria2 users into %s", len(users), tag)
	}
	return nil
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
	xboardURL := flag.String("xboard-url", "", "Xboard base URL; empty disables panel sync")
	xboardToken := flag.String("xboard-token", "", "Xboard node token")
	xboardNodeID := flag.String("xboard-node-id", "", "Xboard node id")
	xboardNodeType := flag.String("xboard-node-type", "vless", "Xboard node type")
	xboardEvery := flag.Duration("xboard-every", time.Minute, "Xboard sync interval")
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
	h := &Hook{}
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
	if err := injectHy2Users(b, *users); err != nil {
		log.Fatal(err)
	}

	var panel xboard.Panel
	if *xboardURL != "" {
		panel = xboard.NewClient(*xboardURL, *xboardToken, *xboardNodeID, *xboardNodeType)
	} else if *users != "" {
		panel = xboard.LocalUsers{Path: *users}
	}
	if panel != nil {
		syncer := &xboard.Syncer{
			Panel:    panel,
			Snapshot: h.SnapshotDelta,
			Commit:   h.CommitSnapshot,
			Users: func(list []xboard.User) error {
				h.SetUserAliases(list)
				active := make(map[string]struct{}, len(list))
				for _, u := range list {
					if u.ID > 0 {
						active[fmt.Sprint(u.ID)] = struct{}{}
					}
				}
				if len(active) > 0 {
					h.RemoveAbsent(active)
				}
				return nil
			},
			Every: *xboardEvery,
		}
		go syncer.Run(ctx)
	}

	<-ctx.Done()
	if err := b.Close(); err != nil {
		log.Println(err)
	}
}
