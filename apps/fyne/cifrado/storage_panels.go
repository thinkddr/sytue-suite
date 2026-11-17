// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// StoragePanelsConfig opts one native presenter into a separate destination
// recovery panel. Existing Panel defaults and unrelated applications are intact.
// State and Bootstrap use the presenter's bound Rooms API. Ready runs on the UI
// thread only after Factory authenticated both clients, recovery and owner keys.
type StoragePanelsConfig struct {
	Legacy, Isolated *Panel
	Factory          Factory[*mx.Client]
	State            func(context.Context, storage.Scope) (storage.IdentityCutoverState, error)
	Bootstrap        func(context.Context, storage.IdentityVersion) (mx.Bootstrap, error)
	Ready            func(context.Context, *Resources[*mx.Client])
	Failed           func(error)
	Dispatch         func(func())
}

// StoragePanels controls two UI panels; it never closes or logs out either SDK.
// Hosts drain repositories before their own final panel/device teardown.
type StoragePanels struct{ core *storagePanels[*mx.Client] }

type panelDevice interface {
	StorageDevice
	comparable
}

type storagePanel[D panelDevice] struct {
	start  func(context.Context, string, string)
	client func(context.Context) (D, error)
	ready  func() bool
	show   func()
	hide   func() bool
	bind   func(func(context.Context), func(context.Context) (mx.Bootstrap, error))
	attach func(*ApplicationSession, func(error))
}

type storagePanels[D panelDevice] struct {
	legacy, isolated storagePanel[D]
	hasLegacy        bool
	factory          Factory[D]
	state            func(context.Context, storage.Scope) (storage.IdentityCutoverState, error)
	bootstrap        func(context.Context, storage.IdentityVersion) (mx.Bootstrap, error)
	ready            func(context.Context, *Resources[D])
	failed           func(error)
	dispatch         func(func())
	life             context.Context
	cancel           context.CancelFunc
	intent           *auth.AuthorizationIntent
	application      atomic.Pointer[ApplicationSession]
	scope            storage.Scope
	metadata         storage.IdentityCutoverState
	started          bool
	invalid          atomic.Bool
	targetStarted    bool
	busy             bool
	announced        bool
	panelsReady      bool
}

func NewStoragePanels(cfg StoragePanelsConfig) (*StoragePanels, error) {
	if cfg.Isolated == nil || cfg.Legacy == cfg.Isolated || !storage.Product(cfg.Factory.Product) || !cfg.Factory.configured() || cfg.State == nil || cfg.Bootstrap == nil || cfg.Ready == nil {
		return nil, ErrPrivateResources
	}
	if configure := cfg.Factory.configurePanel; configure != nil {
		if cfg.Legacy != nil {
			if err := configure(cfg.Legacy); err != nil {
				return nil, err
			}
		}
		if err := configure(cfg.Isolated); err != nil {
			return nil, err
		}
	}
	dispatch := cfg.Dispatch
	if dispatch == nil {
		dispatch = fyne.Do
	}
	p := &storagePanels[*mx.Client]{hasLegacy: cfg.Legacy != nil, factory: cfg.Factory, state: cfg.State, bootstrap: cfg.Bootstrap, ready: cfg.Ready, failed: cfg.Failed, dispatch: dispatch}
	if cfg.Legacy != nil {
		p.legacy = nativeStoragePanel(cfg.Legacy)
	}
	p.isolated = nativeStoragePanel(cfg.Isolated)
	return &StoragePanels{core: p}, nil
}

func nativeStoragePanel(panel *Panel) storagePanel[*mx.Client] {
	return storagePanel[*mx.Client]{
		start: panel.Iniciar, client: panel.Cliente, ready: panel.Verificado, show: panel.AbrirPanel, hide: panel.Ocultar, attach: panel.BindWorkers,
		bind: func(ready func(context.Context), bootstrap func(context.Context) (mx.Bootstrap, error)) {
			panel.AlLista, panel.Bootstrap = ready, bootstrap
		},
	}
}

// Ocultar dismisses verified UI without cancelling sync or closing its keys.
// A setup still waiting for its password stays open: nothing is shown or saved
// until the key is wrapped (ADR 0082).
func (c *Panel) Ocultar() bool {
	if c.diag.Paso == PasoProteger || c.status.RecoverySetupPending {
		return false
	}
	if c.dialog != nil {
		c.dialog.Cerrar()
		c.dialog = nil
	}
	return true
}

// Start is called on the UI thread once per presenter and authorization intent.
func (p *StoragePanels) Start(ctx context.Context) error { return p.core.start(ctx) }

// Retry keeps the same clients and epoch; it never opens another account.
func (p *StoragePanels) Retry() { p.core.retry() }

func (p *StoragePanels) Show() { p.core.show() }

// Invalidate suppresses queued callbacks and cancels work, retaining both SDKs
// and their native secrets for the host's drained teardown or later recovery.
func (p *StoragePanels) Invalidate() {
	p.core.invalidate()
}

func (p *storagePanels[D]) invalidate() {
	p.invalid.Store(true)
	if p.cancel != nil {
		p.cancel()
	}
}

// CutoverState is routing metadata, never evidence of encrypted completion.
func (p *StoragePanels) CutoverState() storage.IdentityCutoverState {
	state := p.core.metadata
	state.SourceRooms = slices.Clone(state.SourceRooms)
	if state.Receipt != nil {
		receipt := *state.Receipt
		state.Receipt = &receipt
	}
	return state
}

func (p *storagePanels[D]) start(ctx context.Context) error {
	if p.started {
		return ErrPrivateResources
	}
	intent := auth.RequestAuthorizationIntent(ctx)
	owner := intent.Identity()
	scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: p.factory.Product}
	if !intent.Current() || !scope.Valid() {
		return auth.ErrAuthorizationChanged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.life, p.cancel = context.WithCancel(ctx) //nolint:gosec // G118: this presenter's retained cancel is called by Invalidate; SDK teardown remains with the drained host.
	p.intent, p.scope, p.started = intent, scope, true
	if p.hasLegacy {
		p.bind(p.legacy, storage.LegacyIdentity)
	}
	p.bind(p.isolated, storage.IsolatedIdentity)
	p.loadState()
	return nil
}

func (p *storagePanels[D]) current() bool {
	if !p.started || p.invalid.Load() || p.life.Err() != nil {
		return false
	}
	if session := p.application.Load(); session != nil {
		return session.Check(session.Context()) == nil
	}
	return p.intent.Current()
}

func (p *storagePanels[D]) context() context.Context {
	if session := p.application.Load(); session != nil {
		return session.Context()
	}
	return p.life
}

func (p *storagePanels[D]) bind(panel storagePanel[D], version storage.IdentityVersion) {
	panel.bind(func(context.Context) { p.onReady(version) }, func(ctx context.Context) (mx.Bootstrap, error) {
		if !p.current() {
			return mx.Bootstrap{}, auth.ErrAuthorizationChanged
		}
		bootstrap, err := p.bootstrap(ctx, version)
		if err != nil {
			return mx.Bootstrap{}, err
		}
		if !p.current() || bootstrap.StorageIdentityVersion.Normalized() != version || bootstrap.Actor != p.scope.Subject || bootstrap.Tenant != p.scope.Tenant || bootstrap.StorageProduct != p.scope.Product {
			return mx.Bootstrap{}, ErrPrivateResources
		}
		return bootstrap, nil
	})
}

func (p *storagePanels[D]) loadState() {
	if !p.current() || p.busy {
		return
	}
	p.busy = true
	go func() {
		session := p.application.Load()
		var err error
		if session == nil {
			session, err = p.factory.Capture(p.life)
			if err == nil {
				p.application.Store(session)
			}
		}
		var state storage.IdentityCutoverState
		var requireLegacy bool
		if err == nil {
			state, err = p.state(session.Context(), p.scope)
		}
		if err == nil {
			requireLegacy, err = state.RequiresLegacy(p.scope)
		}
		p.dispatch(func() {
			if !p.current() {
				return
			}
			p.busy = false
			if err != nil || requireLegacy && !p.hasLegacy {
				p.failure()
				return
			}
			p.metadata = state
			p.preparePanels()
		})
	}()
}

func (p *storagePanels[D]) preparePanels() {
	if !p.current() || p.busy {
		return
	}
	p.busy = true
	finish := func(err error) {
		if !p.current() {
			return
		}
		p.busy = false
		if err != nil {
			p.failure()
			return
		}
		p.panelsReady = true
		if p.metadata.Phase == "isolated" {
			p.beginTarget()
		} else {
			p.beginPanel(p.legacy, storage.LegacyIdentity)
		}
	}
	attach := func(panel storagePanel[D], after func(error)) {
		if panel.attach == nil {
			after(nil)
		} else {
			panel.attach(p.application.Load(), after)
		}
	}
	if p.metadata.Phase == "isolated" {
		attach(p.isolated, finish)
		return
	}
	attach(p.legacy, func(err error) {
		if err != nil {
			finish(err)
			return
		}
		attach(p.isolated, finish)
	})
}

func (p *storagePanels[D]) beginPanel(panel storagePanel[D], version storage.IdentityVersion) {
	if !p.current() {
		return
	}
	client, err := panel.client(p.context())
	if err == nil && !nilDevice(client) {
		_, tenant := client.Identity()
		if client.NativeConsumer() != p.factory.NativeConsumer || client.StorageIdentityVersion() != version || tenant != p.scope.Tenant {
			p.failure()
			return
		}
		if panel.ready() {
			p.onReady(version)
		} else {
			panel.show()
		}
		return
	}
	if err != nil && !errors.Is(err, mx.ErrLoginRequired) {
		p.failure()
		return
	}
	panel.start(p.context(), p.scope.Subject, p.scope.Tenant)
}

func (p *storagePanels[D]) beginTarget() {
	if !p.targetStarted {
		p.targetStarted = true
		p.beginPanel(p.isolated, storage.IsolatedIdentity)
	}
}

func (p *storagePanels[D]) onReady(version storage.IdentityVersion) {
	if !p.current() || p.metadata.Phase == "" {
		return
	}
	if version == storage.LegacyIdentity {
		if !p.legacy.ready() || !p.legacy.hide() {
			return
		}
		p.beginTarget()
	}
	p.tryReady()
}

func (p *storagePanels[D]) devices() (Devices[D], error) {
	required, err := p.metadata.RequiresLegacy(p.scope)
	devices := Devices[D]{RequireLegacy: required}
	if err != nil {
		return devices, err
	}
	if devices.RequireLegacy {
		if !p.hasLegacy || !p.legacy.ready() {
			return devices, mx.ErrUnverified
		}
		devices.Legacy, err = p.legacy.client(p.context())
		if err != nil {
			return devices, err
		}
	}
	if !p.isolated.ready() {
		return devices, mx.ErrUnverified
	}
	devices.Isolated, err = p.isolated.client(p.context())
	return devices, err
}

func (p *storagePanels[D]) tryReady() {
	if !p.current() || p.busy || p.announced {
		return
	}
	devices, err := p.devices()
	if err != nil {
		return
	}
	p.busy = true
	go func() {
		resources, err := p.factory.open(p.context(), p.application.Load(), devices)
		if err == nil {
			err = resources.Check(p.context())
		}
		p.dispatch(func() {
			if !p.current() {
				return
			}
			p.busy = false
			if err != nil {
				p.failure()
				return
			}
			current, currentErr := p.devices()
			if currentErr != nil || current.Legacy != devices.Legacy || current.Isolated != devices.Isolated || resources.Check(p.context()) != nil {
				p.failure()
				return
			}
			p.announced = true
			p.ready(p.context(), resources)
		})
	}()
}

func (p *storagePanels[D]) retry() {
	if !p.current() || p.busy {
		return
	}
	if p.metadata.Phase == "" {
		p.loadState()
		return
	}
	if !p.panelsReady {
		p.preparePanels()
		return
	}
	p.announced = false
	p.tryReady()
}

func (p *storagePanels[D]) show() {
	if !p.current() || p.metadata.Phase == "" {
		return
	}
	if p.metadata.Phase != "isolated" && !p.legacy.ready() {
		p.legacy.show()
	} else if p.targetStarted {
		p.isolated.show()
	}
}

func (p *storagePanels[D]) failure() {
	if p.current() && p.failed != nil {
		p.failed(ErrPrivateResources)
	}
}
