// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type testStoragePanel struct {
	device               *factoryDevice
	opened, verified     bool
	starts, shows, hides int
	onReady              func(context.Context)
	bootstrap            func(context.Context) (mx.Bootstrap, error)
	life                 context.Context
}

func (p *testStoragePanel) port() storagePanel[*factoryDevice] {
	return storagePanel[*factoryDevice]{
		start: func(ctx context.Context, _, _ string) { p.life = ctx; p.starts++; p.opened = true },
		client: func(ctx context.Context) (*factoryDevice, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !p.opened {
				return nil, mx.ErrLoginRequired
			}
			return p.device, nil
		},
		ready: func() bool { return p.opened && p.verified && !p.device.status.RecoverySetupPending },
		show:  func() { p.shows++ },
		hide: func() bool {
			if p.device.status.RecoverySetupPending {
				return false
			}
			p.hides++
			return true
		},
		bind: func(ready func(context.Context), bootstrap func(context.Context) (mx.Bootstrap, error)) {
			p.onReady, p.bootstrap = ready, bootstrap
		},
	}
}

type storagePanelsFixture struct {
	core           *storagePanels[*factoryDevice]
	source, target *testStoragePanel
	ctx            context.Context
	session        *auth.Session
	queue          chan func()
	ready          int
	failed         []error
	stateCalls     int
	last           *Resources[*factoryDevice]
}

func newStoragePanelsFixture(t *testing.T, phase string) *storagePanelsFixture {
	t.Helper()
	f, ctx, session, devices := factoryFixture(t)
	x := &storagePanelsFixture{source: &testStoragePanel{device: devices.Legacy}, target: &testStoragePanel{device: devices.Isolated}, ctx: ctx, session: session, queue: make(chan func(), 8)}
	scope := devices.Isolated.binding.Scope
	state := storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: phase, SourceRooms: []string{devices.Legacy.binding.Target.RoomID}}
	if phase != "legacy" {
		state.Epoch = "6eb2be9b-88a8-4a40-b3c0-7b59ed7354dd"
	}
	if phase == "isolated" {
		state.Receipt = &storage.Reference{RoomID: devices.Isolated.binding.Target.RoomID, EventID: "$complete:rooms.example.test"}
	}
	x.core = &storagePanels[*factoryDevice]{
		legacy: x.source.port(), isolated: x.target.port(), hasLegacy: true, factory: f,
		state: func(context.Context, storage.Scope) (storage.IdentityCutoverState, error) {
			x.stateCalls++
			return state, nil
		},
		bootstrap: func(_ context.Context, version storage.IdentityVersion) (mx.Bootstrap, error) {
			return mx.Bootstrap{Actor: scope.Subject, Tenant: scope.Tenant, StorageProduct: scope.Product, StorageIdentityVersion: version}, nil
		},
		dispatch: func(f func()) { x.queue <- f },
		ready: func(ctx context.Context, r *Resources[*factoryDevice]) {
			if err := r.Check(ctx); err != nil {
				t.Error("Ready received stale resources", err)
			}
			x.ready++
			x.last = r
		}, failed: func(err error) { x.failed = append(x.failed, err) },
	}
	t.Cleanup(func() {
		x.core.invalid.Store(true)
		if x.core.cancel != nil {
			x.core.cancel()
		}
	})
	return x
}

func (x *storagePanelsFixture) take(t *testing.T) func() {
	t.Helper()
	select {
	case f := <-x.queue:
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("storage panels did not dispatch")
		return nil
	}
}

func (x *storagePanelsFixture) connect(t *testing.T) {
	t.Helper()
	if err := x.core.start(x.ctx); err != nil {
		t.Fatal(err)
	}
	x.take(t)()
	if x.core.metadata.Phase != "isolated" {
		x.source.verified = true
		x.source.onReady(x.ctx)
	}
	x.target.verified = true
	x.target.onReady(x.ctx)
	x.take(t)()
}

func TestStoragePanelsKeepsSourceAliveUntilDestinationRecoveryAcknowledged(t *testing.T) {
	for _, phase := range []string{"legacy", "freezing", "frozen"} {
		t.Run(phase, func(t *testing.T) {
			x := newStoragePanelsFixture(t, phase)
			if err := x.core.start(x.ctx); err != nil {
				t.Fatal(err)
			}
			x.take(t)()
			if x.source.starts != 1 || x.target.starts != 0 {
				t.Fatal("destination bypassed source readiness")
			}
			x.source.verified = true
			x.source.onReady(x.ctx)
			if x.target.starts != 1 || x.source.hides != 1 || x.source.device.closes != 0 {
				t.Fatal("destination setup replaced or closed source")
			}
			x.target.verified = true
			x.target.device.status.RecoverySetupPending = true
			x.target.onReady(x.ctx)
			if x.ready != 0 || x.source.device.preparations+x.target.device.preparations != 0 {
				t.Fatal("unacknowledged recovery prepared/froze app")
			}
			x.target.device.status.RecoverySetupPending = false
			x.target.onReady(x.ctx)
			x.take(t)()
			if x.ready != 1 || x.last == nil || !x.last.Devices.RequireLegacy || x.last.Devices.Legacy != x.source.device || x.last.Devices.Isolated != x.target.device {
				t.Fatal("verified pair lost source", x.ready)
			}
			if x.source.device.closes+x.target.device.closes != 0 {
				t.Fatal("ready revoked a borrowed SDK")
			}
			x.core.retry()
			x.take(t)()
			if x.ready != 2 || x.source.starts != 1 || x.target.starts != 1 || x.stateCalls != 1 {
				t.Fatal("retry opened another identity or reselected source", x.ready, x.stateCalls)
			}
		})
	}
}

func TestStoragePanelsAlreadyIsolatedNeverOpensLegacy(t *testing.T) {
	x := newStoragePanelsFixture(t, "isolated")
	x.core.hasLegacy = false
	x.connect(t)
	if x.ready != 1 || x.source.starts != 0 || x.target.starts != 1 || x.last.Devices.RequireLegacy || x.last.Devices.Legacy != nil {
		t.Fatal("isolated account opened legacy", x.ready)
	}
}

func TestStoragePanelsBorrowsExistingSourceWithoutRestart(t *testing.T) {
	x := newStoragePanelsFixture(t, "frozen")
	x.source.opened, x.source.verified = true, true
	if err := x.core.start(x.ctx); err != nil {
		t.Fatal(err)
	}
	x.take(t)()
	if x.source.starts != 0 || x.source.device.closes != 0 || x.target.starts != 1 {
		t.Fatal("warm source was restarted before destination was ready")
	}
	x.target.verified = true
	x.target.onReady(x.ctx)
	x.take(t)()
	if x.ready != 1 {
		t.Fatal("warm source did not complete verified pair")
	}
}

func TestStoragePanelsPeerRefreshDuringRecoveryPreservesBothDevices(t *testing.T) {
	x := newStoragePanelsFixture(t, "frozen")
	if err := x.core.start(x.ctx); err != nil {
		t.Fatal(err)
	}
	x.take(t)()
	x.source.verified = true
	x.source.onReady(x.ctx)
	application := x.core.application.Load()
	tokens := factoryTokens("alice")
	tokens.Refresh = "synthetic-peer-refresh"
	factoryAdopt(t, application.witness, x.session, tokens)
	if auth.RequestAuthorizationIntent(x.ctx).Current() {
		t.Fatal("short action was not replaced by peer refresh")
	}
	x.target.verified = true
	x.target.onReady(x.ctx)
	x.take(t)()
	if x.ready != 1 || x.source.starts != 1 || x.target.starts != 1 || x.source.device.closes+x.target.device.closes != 0 {
		t.Fatal("peer refresh restarted or lost verified native devices")
	}
}

func TestStoragePanelsSuppressesQueuedStateAndReadyAfterABA(t *testing.T) {
	for _, stage := range []string{"state", "ready"} {
		t.Run(stage, func(t *testing.T) {
			x := newStoragePanelsFixture(t, "frozen")
			if err := x.core.start(x.ctx); err != nil {
				t.Fatal(err)
			}
			queued := x.take(t)
			if stage == "ready" {
				queued()
				x.source.verified = true
				x.source.onReady(x.ctx)
				x.target.verified = true
				x.target.onReady(x.ctx)
				queued = x.take(t)
			}
			if application := x.core.application.Load(); application != nil {
				factoryAdopt(t, application.witness, x.session, factoryTokens("bob"))
				factoryAdopt(t, application.witness, x.session, factoryTokens("alice"))
			} else {
				x.session.Sincronizar(factoryTokens("bob"))
				x.session.Sincronizar(factoryTokens("alice"))
			}
			queued()
			if x.ready != 0 || len(x.failed) != 0 {
				t.Fatal("queued callback revived an old account")
			}
			if stage == "state" && x.source.starts+x.target.starts != 0 {
				t.Fatal("stale state started devices")
			}
			if x.source.device.closes+x.target.device.closes != 0 {
				t.Fatal("stale callback destroyed retained keys")
			}
		})
	}
}

func TestStoragePanelsDoesNotExposeFactoryErrorsAndCanRetry(t *testing.T) {
	x := newStoragePanelsFixture(t, "frozen")
	x.target.device.prepareErr = errors.New("sensitive internal failure, never a user notification")
	x.connect(t)
	if x.ready != 0 || len(x.failed) != 1 || !errors.Is(x.failed[0], ErrPrivateResources) {
		t.Fatal("invalid owner keys reached Ready or leaked internal error", x.failed)
	}
	if x.source.device.closes+x.target.device.closes != 0 {
		t.Fatal("failed preparation closed source")
	}
	x.target.device.prepareErr = nil
	x.core.retry()
	x.take(t)()
	if x.ready != 1 || x.stateCalls != 1 || x.source.starts != 1 || x.target.starts != 1 {
		t.Fatal("retry did not reuse retained source")
	}
}

func TestStoragePanelsRejectsMalformedStateAndBootstrapWithoutFallback(t *testing.T) {
	x := newStoragePanelsFixture(t, "legacy")
	x.core.state = func(context.Context, storage.Scope) (storage.IdentityCutoverState, error) {
		return storage.IdentityCutoverState{}, nil
	}
	if err := x.core.start(x.ctx); err != nil {
		t.Fatal(err)
	}
	x.take(t)()
	if len(x.failed) != 1 || x.source.starts+x.target.starts != 0 {
		t.Fatal("malformed phase fell back to legacy")
	}
	x = newStoragePanelsFixture(t, "frozen")
	if err := x.core.start(x.ctx); err != nil {
		t.Fatal(err)
	}
	x.take(t)()
	x.core.bootstrap = func(context.Context, storage.IdentityVersion) (mx.Bootstrap, error) {
		return mx.Bootstrap{Actor: "bob", Tenant: "acme", StorageProduct: "mail", StorageIdentityVersion: storage.IsolatedIdentity}, nil
	}
	if _, err := x.target.bootstrap(x.ctx); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("foreign bootstrap was accepted", err)
	}
	if err := x.core.start(x.ctx); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("same coordinator started another lifecycle", err)
	}
}

func TestNativeHideKeepsVerifiedSourceAndRequiresWrappedRecovery(t *testing.T) {
	c, _ := panelEn(t, 320)
	c.ctx = t.Context()
	c.status = mx.VerificationStatus{HasIdentity: true, Verified: true, RecoverySetupPending: true}
	c.AbrirPanel()
	if c.Ocultar() || c.dialog == nil {
		t.Fatal("unfinished recovery setup was hidden")
	}
	c.status.RecoverySetupPending = false
	c.diag.Paso = PasoProteger
	if c.Ocultar() || c.dialog == nil {
		t.Fatal("unwrapped recovery was hidden")
	}
	c.diag.Paso = PasoListo
	gen := c.generation
	if !c.Ocultar() || c.dialog != nil || c.generation != gen || c.ctx.Err() != nil || !c.Verificado() {
		t.Fatal("hiding verified UI tore down source")
	}
}
