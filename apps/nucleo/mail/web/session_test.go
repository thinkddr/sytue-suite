// SPDX-License-Identifier: Apache-2.0
package web

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

type boundaryPort struct {
	id             string
	identity       webport.Identity
	current, calls int
}

func (p *boundaryPort) ID() string { return p.id }
func (p *boundaryPort) Current(ctx context.Context) (webport.Identity, error) {
	p.current++
	return p.identity, ctx.Err()
}

func (p *boundaryPort) Call(context.Context, storage.Scope, webport.Method, []byte) ([]byte, error) {
	p.calls++
	return nil, webport.ErrRejected
}

func TestManagerRejectsLegacyCanonicalBeforeAnySourceOrHTTPAndRetiredIDsCannotReopen(t *testing.T) {
	manager, err := NewManager(webport.NewSources(), webport.NewPolicies())
	if err != nil {
		t.Fatal(err)
	}
	port := &boundaryPort{id: strings.Repeat("a", 64), identity: webport.Identity{Version: 1, Scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}, Binding: webport.Binding{MatrixUserID: "@legacy:matrix.test", DeviceID: "DEVICE"}}}
	if _, err = manager.Open(t.Context(), Ports{Isolated: port}); !errors.Is(err, webport.ErrRejected) || port.calls != 0 {
		t.Fatal("legacy fallback became the canonical Mail application", err, port.calls)
	}
	beforeRetry := port.current
	if _, retryErr := manager.Open(t.Context(), Ports{Isolated: port}); !errors.Is(retryErr, webport.ErrRejected) || port.current <= beforeRetry || port.calls != 0 {
		t.Fatal("failed construction retained canonical reservation", retryErr)
	}
	if err = manager.ClosePort(t.Context(), port.ID()); err != nil {
		t.Fatal(err)
	}
	before := port.current
	if _, err = manager.Open(t.Context(), Ports{Isolated: port}); !errors.Is(err, webport.ErrRejected) || port.current != before {
		t.Fatal("retired capability reopened", err)
	}
}

func TestManagerRejectsRoleAliasingAndUnknownSessionWithoutFabricatingPendingZero(t *testing.T) {
	manager, _ := NewManager(webport.NewSources(), webport.NewPolicies())
	port := &boundaryPort{id: strings.Repeat("a", 64)}
	if _, err := manager.Open(t.Context(), Ports{Isolated: port, LegacyReader: port}); !errors.Is(err, webport.ErrRejected) || port.current != 0 {
		t.Fatal("one capability used for two roles", err)
	}
	if _, err := manager.Open(t.Context(), Ports{Isolated: port, MigrationWriter: &boundaryPort{id: strings.Repeat("b", 64)}}); !errors.Is(err, webport.ErrRejected) || port.current != 0 {
		t.Fatal("unbound writer admitted", err)
	}
	if _, err := manager.Lookup(t.Context(), strings.Repeat("c", 64), port.ID()); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("missing engine fabricated empty pending state", err)
	}
}

type constructionBarrierPort struct {
	id                         string
	entered, canceled, release chan struct{}
}

func (p *constructionBarrierPort) ID() string { return p.id }
func (p *constructionBarrierPort) Current(ctx context.Context) (webport.Identity, error) {
	close(p.entered)
	<-ctx.Done()
	close(p.canceled)
	<-p.release
	return webport.Identity{}, ctx.Err()
}

func (p *constructionBarrierPort) Call(context.Context, storage.Scope, webport.Method, []byte) ([]byte, error) {
	return nil, webport.ErrRejected
}

func TestManagerRetirementWaitsForCanceledConstructionInsteadOfDiscardingItsLifetime(t *testing.T) {
	manager, _ := NewManager(webport.NewSources(), webport.NewPolicies())
	port := &constructionBarrierPort{id: strings.Repeat("d", 64), entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	opened := make(chan error, 1)
	go func() { _, err := manager.Open(t.Context(), Ports{Isolated: port}); opened <- err }()
	select {
	case <-port.entered:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	retired := make(chan error, 1)
	go func() { retired <- manager.ClosePort(t.Context(), port.ID()) }()
	select {
	case <-port.canceled:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	select {
	case err := <-retired:
		t.Fatal("retirement acknowledged before actual constructor IO joined", err)
	default:
	}
	close(port.release)
	if err := <-opened; err == nil {
		t.Fatal("canceled constructor became a live engine")
	}
	if err := <-retired; err != nil {
		t.Fatal(err)
	}
}

func TestMailDomainCommandsCannotBecomeAnHTTPFallbackOrCarryMIMEInReadOperations(t *testing.T) {
	for _, action := range []string{"https://other.invalid/send", "/api/v1/messages", "constructor", "Mailboxes"} {
		if _, _, err := invoke(t.Context(), nil, Call{Action: action}, nil); !errors.Is(err, webport.ErrRejected) {
			t.Fatal("unlisted domain operation admitted", action, err)
		}
	}
	if _, _, err := invoke(t.Context(), nil, Call{Action: "mailboxes"}, []byte("private original")); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("read action accepted an untracked plaintext payload", err)
	}
}

func TestMailSessionNeverExposesItsRepositoryOrAnUnboundRunner(t *testing.T) {
	typ := reflect.TypeOf((*Session)(nil))
	for _, name := range []string{"Repository", "Value", "Run", "Product", "Resources", "Ports"} {
		if _, exists := typ.MethodByName(name); exists {
			t.Fatal("Mail host retained raw domain capability", name)
		}
	}
}
