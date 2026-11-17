//go:build integration

// SPDX-License-Identifier: Apache-2.0
package pgtest

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// CommitAckProxy drops one successful COMMIT response after PostgreSQL emits
// CommandComplete. It forwards opaque authentication bytes without logging or
// decoding them, and is limited to a disposable loopback, non-TLS fixture.
type CommitAckProxy struct {
	dsn         string
	upstream    string
	listener    net.Listener
	armed       atomic.Bool
	dropped     chan struct{}
	mutex       sync.Mutex
	closing     bool
	connections map[net.Conn]struct{}
	workers     sync.WaitGroup
	closeOnce   sync.Once
}

func NewCommitAckProxy(t *testing.T, dsn string) *CommitAckProxy {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil || u == nil || u.Query().Get("sslmode") != "disable" {
		t.Fatal("COMMIT proxy requires a disposable non-TLS fixture")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && u.Hostname() != "postgres" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("COMMIT proxy requires a loopback fixture")
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("COMMIT proxy listen failed")
	}
	proxy := &CommitAckProxy{upstream: net.JoinHostPort(u.Hostname(), port), listener: listener, dropped: make(chan struct{}, 1), connections: make(map[net.Conn]struct{})}
	u.Host = listener.Addr().String()
	proxy.dsn = u.String()
	proxy.workers.Add(1)
	go func() {
		defer proxy.workers.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			proxy.workers.Add(1)
			go func() { defer proxy.workers.Done(); proxy.forward(t.Context(), client) }()
		}
	}()
	t.Cleanup(proxy.Close)
	return proxy
}
func (p *CommitAckProxy) DSN() string { return p.dsn }
func (p *CommitAckProxy) Arm()        { p.armed.Store(true) }
func (p *CommitAckProxy) WaitDropped(ctx context.Context) error {
	select {
	case <-p.dropped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *CommitAckProxy) track(conn net.Conn) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.closing {
		_ = conn.Close()
		return false
	}
	p.connections[conn] = struct{}{}
	return true
}

func (p *CommitAckProxy) untrack(conn net.Conn) {
	p.mutex.Lock()
	delete(p.connections, conn)
	p.mutex.Unlock()
	_ = conn.Close()
}

func (p *CommitAckProxy) forward(ctx context.Context, client net.Conn) {
	if !p.track(client) {
		return
	}
	defer p.untrack(client)
	upstream, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", p.upstream)
	if err != nil {
		return
	}
	if !p.track(upstream) {
		return
	}
	defer p.untrack(upstream)
	var startup [4]byte
	if _, err := io.ReadFull(client, startup[:]); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(startup[:])
	if length < 8 || length > 1<<20 {
		return
	}
	packet := make([]byte, int(length))
	copy(packet, startup[:])
	if _, err := io.ReadFull(client, packet[4:]); err != nil {
		return
	}
	if _, err := upstream.Write(packet); err != nil {
		return
	}
	frontendDone := make(chan struct{})
	go func() { defer close(frontendDone); _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
	defer func() { _ = client.Close(); _ = upstream.Close(); <-frontendDone }()
	for {
		var header [5]byte
		if _, err := io.ReadFull(upstream, header[:]); err != nil {
			return
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length < 4 || length > 64<<20 {
			return
		}
		payload := make([]byte, int(length)-4)
		if _, err := io.ReadFull(upstream, payload); err != nil {
			return
		}
		if header[0] == 'C' && bytes.Equal(payload, []byte("COMMIT\x00")) && p.armed.CompareAndSwap(true, false) {
			_ = client.Close()
			_ = upstream.Close()
			select {
			case p.dropped <- struct{}{}:
			default:
			}
			return
		}
		if _, err := client.Write(header[:]); err != nil {
			return
		}
		if _, err := client.Write(payload); err != nil {
			return
		}
	}
}

func (p *CommitAckProxy) Close() {
	p.closeOnce.Do(func() {
		p.mutex.Lock()
		p.closing = true
		connections := make([]net.Conn, 0, len(p.connections))
		for conn := range p.connections {
			connections = append(connections, conn)
		}
		p.mutex.Unlock()
		_ = p.listener.Close()
		for _, conn := range connections {
			_ = conn.Close()
		}
		p.workers.Wait()
	})
}
