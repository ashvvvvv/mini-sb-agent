package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"mini-sb-agent/counter"
	"mini-sb-agent/panelapi"

	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
)

// TestDirectionalLimitersAreDistinctObjects verifies that DirectionalLimiters
// returns four independent *RateLimiter pointers — nodeRead ≠ nodeWrite and
// userRead ≠ userWrite — so each direction has its own token bucket.
func TestDirectionalLimitersAreDistinctObjects(t *testing.T) {
	um := NewUserManager(100) // 100 Mbps node limit
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 10},
	}); err != nil {
		t.Fatal(err)
	}

	nodeRead, nodeWrite, userRead, userWrite := um.DirectionalLimiters("uuid-1")

	if nodeRead == nil || nodeWrite == nil {
		t.Fatal("node limiters must not be nil when node rate is set")
	}
	if nodeRead == nodeWrite {
		t.Fatal("nodeRead and nodeWrite must be different objects")
	}

	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}
	if userRead == nil || userWrite == nil {
		t.Fatal("user limiters must not be nil when user SpeedLimit is set")
	}
	if userRead == userWrite {
		t.Fatal("userRead and userWrite must be different objects")
	}
}

// TestDirectionalLimitersBudgetsAreIndependent verifies that consuming tokens
// from one direction's limiter does not reduce the budget of the other
// direction. This is the core invariant: a saturated download must not starve
// upload ACK/control packets.
func TestDirectionalLimitersBudgetsAreIndependent(t *testing.T) {
	// Use a low rate so we can observe the bucket being drained.
	um := NewUserManager(0) // no node limiter; test user limiter only
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 1}, // 1 Mbps = 125000 bytes/sec
	}); err != nil {
		t.Fatal(err)
	}
	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}

	_, _, userRead, userWrite := um.DirectionalLimiters("uuid-1")

	// Drain the read limiter's entire burst budget.
	// burst = max(rate * 200ms, 64KiB) = max(125000*0.2, 65536) = max(25000, 65536) = 65536
	// Immediately after creation tokens are 0 but Allow/Wait refills based on
	// elapsed time. We drain by calling Wait which will block (consume) tokens.
	// Instead, let's use Allow to check availability without sleeping.

	// Wait a tiny bit so tokens accumulate, then drain the read side.
	time.Sleep(10 * time.Millisecond) // ~1250 bytes worth

	// Drain read bucket — this should NOT affect write bucket.
	if err := userRead.Wait(65536); err != nil {
		t.Fatal("draining read limiter:", err)
	}

	// Now check the write bucket: it should still allow data through quickly.
	// If read and write shared a bucket, the write side would be drained too.
	start := time.Now()
	if err := userWrite.Wait(1024); err != nil {
		t.Fatal("write limiter after read drain:", err)
	}
	elapsed := time.Since(start)
	// 1024 bytes at 125000 B/s is ~8ms. If shared, we'd need to refill ~64KiB
	// first which would take ~500ms.
	if elapsed > 200*time.Millisecond {
		t.Fatalf("write limiter stalled after read drain: %v (shared bucket?)", elapsed)
	}
}

// TestNodeDirectionalLimitersBudgetsAreIndependent repeats the independence
// check for the node-level limiter pair.
func TestNodeDirectionalLimitersBudgetsAreIndependent(t *testing.T) {
	um := NewUserManager(1) // 1 Mbps node limit
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1"},
	}); err != nil {
		t.Fatal(err)
	}

	nodeRead, nodeWrite, _, _ := um.DirectionalLimiters("uuid-1")
	if nodeRead == nil || nodeWrite == nil {
		t.Fatal("node limiters must not be nil")
	}

	time.Sleep(10 * time.Millisecond)

	// Drain node read bucket.
	if err := nodeRead.Wait(65536); err != nil {
		t.Fatal("draining node read limiter:", err)
	}

	// Node write should still have budget.
	start := time.Now()
	if err := nodeWrite.Wait(1024); err != nil {
		t.Fatal("node write limiter after read drain:", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("node write limiter stalled after read drain: %v (shared bucket?)", elapsed)
	}
}

// TestRoutedConnectionUsesDirectionalLimiters verifies the integration: a
// RoutedConnection wraps with separate read/write limiters so that a large
// write (download to client) does not consume the read (upload) budget.
func TestRoutedConnectionUsesDirectionalLimiters(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 1}, // 1 Mbps
	}); err != nil {
		t.Fatal(err)
	}
	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}

	h := &Hook{users: um}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "test",
		User:    "uuid-1",
		Source:  M.ParseSocksaddr("127.0.0.1:12345"),
	}, nil, nil)

	// Saturate the write side with a background goroutine.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		// Write 500KB — at 1 Mbps (125KB/s) this takes ~4 seconds.
		_, _ = wrapped.Write(make([]byte, 500_000))
	}()

	// Drain data from client side so write can proceed.
	go func() {
		_, _ = io.Copy(io.Discard, client)
	}()

	// While write is in progress, the read side should still be responsive.
	// Send a small "ACK" packet from client, read it through wrapped.
	// If buckets are shared, Read will stall because the write drained tokens.
	readClient, readServer := net.Pipe()
	defer readClient.Close()
	defer readServer.Close()

	wrappedRead := h.RoutedConnection(context.Background(), readServer, adapter.InboundContext{
		Inbound: "test",
		User:    "uuid-1",
		Source:  M.ParseSocksaddr("127.0.0.1:12346"),
	}, nil, nil)

	// Give the first connection's write goroutine a moment to start draining.
	time.Sleep(50 * time.Millisecond)

	// Send a small read payload.
	go func() {
		_, _ = readClient.Write(make([]byte, 64))
	}()

	readBuf := make([]byte, 128)
	readStart := time.Now()
	n, err := wrappedRead.Read(readBuf)
	if err != nil {
		t.Fatal("reading small packet:", err)
	}
	readElapsed := time.Since(readStart)
	if n != 64 {
		t.Fatalf("expected 64 bytes, got %d", n)
	}
	// 64 bytes at 125000 B/s is sub-millisecond. Allow generous 500ms.
	// If shared, the write would have drained the bucket and read would stall.
	if readElapsed > 500*time.Millisecond {
		t.Fatalf("read stalled while write was saturated: %v (shared bucket?)", readElapsed)
	}
}

// TestDeletedUserDirectionalLimitersAllClosed verifies that deleting a user
// closes both the rx and tx halves of the limiter pair.
func TestDeletedUserDirectionalLimitersAllClosed(t *testing.T) {
	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 5},
	}); err != nil {
		t.Fatal(err)
	}
	_, _, userRead, userWrite := um.DirectionalLimiters("uuid-1")
	if userRead == nil || userWrite == nil {
		t.Fatal("expected non-nil limiters before delete")
	}

	// Delete by applying empty user list.
	if err := um.ApplyBox(nil, nil); err != nil {
		t.Fatal(err)
	}

	if !userRead.Closed() {
		t.Fatal("userRead limiter not closed after user deletion")
	}
	if !userWrite.Closed() {
		t.Fatal("userWrite limiter not closed after user deletion")
	}
}

// TestSpeedChangeUpdatesBothDirections verifies that a speed limit change
// updates both the rx and tx limiter rates without replacing the objects.
func TestSpeedChangeUpdatesBothDirections(t *testing.T) {
	if !userRateLimitBuildEnabled {
		t.Skip("user rate limiting is compile-time disabled")
	}
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 3},
	}); err != nil {
		t.Fatal(err)
	}
	_, _, rxBefore, txBefore := um.DirectionalLimiters("uuid-1")

	// Update speed.
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 7},
	}); err != nil {
		t.Fatal(err)
	}
	_, _, rxAfter, txAfter := um.DirectionalLimiters("uuid-1")

	// Pointers should be reused (not replaced).
	if rxBefore != rxAfter {
		t.Fatal("rx limiter was replaced on speed change; old connections keep stale limits")
	}
	if txBefore != txAfter {
		t.Fatal("tx limiter was replaced on speed change; old connections keep stale limits")
	}
	// Rates should match the new speed.
	wantRate := mbpsToBytes(7)
	if rxAfter.Rate() != wantRate {
		t.Fatalf("rx rate=%d, want %d", rxAfter.Rate(), wantRate)
	}
	if txAfter.Rate() != wantRate {
		t.Fatalf("tx rate=%d, want %d", txAfter.Rate(), wantRate)
	}
}

// TestNodeLimiterPairSurvivesUserChurn verifies that the node-level rx/tx
// limiter pair is stable across user add/delete cycles.
func TestNodeLimiterPairSurvivesUserChurn(t *testing.T) {
	um := NewUserManager(50) // 50 Mbps node
	nodeRead1, nodeWrite1, _, _ := um.DirectionalLimiters("nobody")

	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1"},
	}); err != nil {
		t.Fatal(err)
	}
	nodeRead2, nodeWrite2, _, _ := um.DirectionalLimiters("uuid-1")

	if err := um.ApplyBox(nil, nil); err != nil {
		t.Fatal(err)
	}
	nodeRead3, nodeWrite3, _, _ := um.DirectionalLimiters("nobody")

	if nodeRead1 != nodeRead2 || nodeRead2 != nodeRead3 {
		t.Fatal("node read limiter was replaced during user churn")
	}
	if nodeWrite1 != nodeWrite2 || nodeWrite2 != nodeWrite3 {
		t.Fatal("node write limiter was replaced during user churn")
	}
}

// Verify the counter.RateLimitedConn exported by RoutedConnection has the
// expected type (not the old single-limiter wrapper which would share).
func TestRoutedConnectionReturnsRateLimitedType(t *testing.T) {
	um := NewUserManager(0)
	if err := um.ApplyBox(nil, []panelapi.User{
		{ID: 1, UUID: "uuid-1", SpeedLimit: 1},
	}); err != nil {
		t.Fatal(err)
	}
	h := &Hook{users: um}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	wrapped := h.RoutedConnection(context.Background(), server, adapter.InboundContext{
		Inbound: "test",
		User:    "uuid-1",
		Source:  M.ParseSocksaddr("127.0.0.1:12345"),
	}, nil, nil)
	if _, ok := wrapped.(*counter.RateLimitedConn); !ok {
		t.Fatalf("expected *counter.RateLimitedConn, got %T", wrapped)
	}
}
