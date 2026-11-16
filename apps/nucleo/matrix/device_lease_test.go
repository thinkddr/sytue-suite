// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type leaseSecrets struct {
	base     *memorySecrets
	reads    atomic.Int32
	deleting chan struct{}
	resume   chan struct{}
}

func (s *leaseSecrets) Read(key string) ([]byte, error)      { s.reads.Add(1); return s.base.Read(key) }
func (s *leaseSecrets) Write(key string, value []byte) error { return s.base.Write(key, value) }
func (s *leaseSecrets) Delete(key string) error {
	if s.deleting != nil {
		close(s.deleting)
		s.deleting = nil
		<-s.resume
	}
	return s.base.Delete(key)
}

func leaseClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func leaseSaved(t *testing.T, c *Client, token string) {
	t.Helper()
	c.session = credentials{Homeserver: c.cfg.Bootstrap.HomeserverURL, UserID: id.UserID(c.cfg.Bootstrap.MatrixUserID), DeviceID: "ARTIFICIAL_DEVICE", AccessToken: token, Subject: c.cfg.Subject, Tenant: c.cfg.Tenant}
	raw, err := json.Marshal(c.session)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	if err = c.cfg.Secrets.Write(c.sessionKey(), raw); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceLeasePrecedesSecretsBrowserAndNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cfg := configFor(t, server)
	secrets := &leaseSecrets{base: cfg.Secrets.(*memorySecrets)}
	cfg.Secrets = secrets
	holder := leaseClient(t, cfg)
	if err := holder.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"open", "desktop_login", "mobile_login"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			defer cancel()
			var err error
			switch kind {
			case "open":
				_, err = Open(ctx, cfg)
			case "desktop_login":
				_, err = Login(ctx, cfg, func(string) error { return errors.New("unexpected browser") })
			case "mobile_login":
				_, err = LoginNative(ctx, cfg, api.NewRooms(server.URL, nil, server.Client()), func(context.Context, api.MatrixNativeAuthorization) (api.MatrixNativeCallback, error) {
					return api.MatrixNativeCallback{}, errors.New("unexpected mobile callback")
				})
			}
			if !errors.Is(err, context.DeadlineExceeded) || secrets.reads.Load() != 0 || calls.Load() != 0 {
				t.Fatalf("lease boundary: err=%v reads=%d requests=%d", err, secrets.reads.Load(), calls.Load())
			}
		})
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := Open(t.Context(), cfg); !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("failed Open retained the lease: %v", err)
		}
	}
}

func TestDeviceLeaseContentionHasABoundedRetry(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := configFor(t, server)
	secrets := &leaseSecrets{base: cfg.Secrets.(*memorySecrets)}
	cfg.Secrets = secrets
	holder := leaseClient(t, cfg)
	if err := holder.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrDeviceInUse) || secrets.reads.Load() != 0 {
		t.Fatal("contention did not return its bounded retry without reading secrets", err)
	}
}

func TestDeviceLeaseIsHeldUntilReadersDrainAndCleanupFinishes(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cfg := configFor(t, server)
	secrets := &leaseSecrets{base: cfg.Secrets.(*memorySecrets)}
	cfg.Secrets = secrets
	holder := leaseClient(t, cfg)
	if err := holder.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	holder.stateMu.RLock()
	closed := make(chan error, 1)
	go func() { closed <- holder.Close() }()
	deadline := time.Now().Add(time.Second)
	for !holder.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !holder.closed.Load() {
		holder.stateMu.RUnlock()
		t.Fatal("close did not begin")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	_, err := Open(ctx, cfg)
	cancel()
	holder.stateMu.RUnlock()
	if !errors.Is(err, context.DeadlineExceeded) || secrets.reads.Load() != 0 {
		t.Fatalf("reader drain released lease early: %v", err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	current := leaseClient(t, cfg)
	if err := current.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	leaseSaved(t, current, "artificial-current")
	secrets.deleting = make(chan struct{})
	secrets.resume = make(chan struct{})
	deleting := secrets.deleting
	loggedOut := make(chan error, 1)
	go func() { loggedOut <- current.Logout(t.Context()) }()
	<-deleting
	before := secrets.reads.Load()
	ctx, cancel = context.WithTimeout(t.Context(), 40*time.Millisecond)
	_, err = Open(ctx, cfg)
	cancel()
	close(secrets.resume)
	if !errors.Is(err, context.DeadlineExceeded) || secrets.reads.Load() != before {
		t.Fatalf("key cleanup released lease early: %v", err)
	}
	if err := <-loggedOut; err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), cfg); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("cleanup did not release empty device: %v", err)
	}
}

func TestClosedDeviceCannotLogOutOrEraseItsReplacement(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = w.Write([]byte(`{}`)) }))
	defer server.Close()
	cfg := configFor(t, server)
	old := leaseClient(t, cfg)
	if err := old.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	leaseSaved(t, old, "artificial-old")
	_ = old.Close()
	next := leaseClient(t, cfg)
	if err := next.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	leaseSaved(t, next, "artificial-replacement")
	_ = next.Close()
	if err := old.Logout(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatalf("stale logout accepted: %v", err)
	}
	saved, err := next.readSession()
	if err != nil || saved != next.session || calls.Load() != 0 {
		t.Fatalf("replacement changed: err=%v requests=%d", err, calls.Load())
	}
}

func TestDeviceLeaseProcessDeathReleasesTheSameAccount(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	cfg := configFor(t, server)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDeviceLeaseProcessHelper$")
	cmd.Env = append(os.Environ(), "SYTUE_MATRIX_LEASE_ROOT="+cfg.StoreDir, "SYTUE_MATRIX_LEASE_URL="+cfg.Bootstrap.HomeserverURL)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "lease held" {
		t.Fatal("helper did not acquire its lease")
	}
	blocked, stop := context.WithTimeout(t.Context(), 40*time.Millisecond)
	_, err = Open(blocked, cfg)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("another process opened the same device: %v", err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	next := leaseClient(t, cfg)
	if err = next.acquireStore(t.Context()); err != nil {
		t.Fatalf("dead process retained lease: %v", err)
	}
	path := filepath.Join(cfg.StoreDir, ".device-leases", next.account+".lock")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private lease permissions: %v", err)
	}
}

func TestDeviceLeaseProcessHelper(t *testing.T) {
	root := os.Getenv("SYTUE_MATRIX_LEASE_ROOT")
	if root == "" {
		t.Skip("subprocess fixture only")
	}
	cfg := Config{Bootstrap: Bootstrap{Actor: "person", Tenant: "org", ControllerUserID: "@rooms_controller:matrix.test", HomeserverURL: os.Getenv("SYTUE_MATRIX_LEASE_URL"), MatrixUserID: "@person:matrix.test", Encryption: string(id.AlgorithmMegolmV1)}, Subject: "person", Tenant: "org", StoreDir: root, Secrets: &memorySecrets{values: map[string][]byte{}}}
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err = client.acquireStore(t.Context()); err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	fmt.Println("lease held")
	<-signals
}
