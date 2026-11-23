// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PrivateApplicationConfig supplies domain construction and presentation. Native
// consent, recovery, identity lifetime, retries and source-safe teardown are shared.
type PrivateApplicationConfig[R PrivateRepository] struct {
	Suite       *sesion.Suite
	Application fyne.App
	Window      fyne.Window
	Product     string
	Translate   func(string) string
	Build       func(context.Context, *Resources[*mx.Client]) (ApplicationPlan[R], error)
	// BeforeOpen may obtain explicit root consent before the repository cycle
	// acquires its work lease. Build and Prepare must never wait for user input.
	BeforeOpen func(context.Context, *Resources[*mx.Client], StorageApproval) error
	Mount      func(context.Context, *Resources[*mx.Client], R) (ApplicationView, error)
	State      func(ApplicationState, error)
	CanStart   func() bool
	// SoltarFuente lo pone AplicacionRegistros: la salida «Vaciar y volver a los
	// datos del servidor» del panel cuando la clave no abre (Panel.SoltarFuente).
	SoltarFuente func(context.Context) error
}

type PrivateApplication[R PrivateRepository] struct {
	core  *privateApplication[*mx.Client, R]
	slot  *fyne.Container
	state ApplicationState
}

//nolint:gocyclo // valida cada pieza obligatoria de la configuración antes de montar nada.
func NewPrivateApplication[R PrivateRepository](cfg PrivateApplicationConfig[R]) (*PrivateApplication[R], error) {
	if cfg.Suite == nil || cfg.Application == nil || cfg.Window == nil || !storage.Product(cfg.Product) || cfg.Translate == nil || cfg.Build == nil || cfg.Mount == nil {
		return nil, ErrPrivateResources
	}
	factory, err := NewNativeFactory(cfg.Suite, cfg.Product)
	if err != nil {
		return nil, err
	}
	directory, err := ajustes.Carpeta()
	if err != nil {
		return nil, err
	}
	application := &PrivateApplication[R]{slot: container.NewStack()}
	core := &privateApplication[*mx.Client, R]{}
	var approvals sync.Map
	consent := NewIngressConsent(cfg.Suite.URL("rooms"))
	approvedPublication := func(ctx context.Context, resources *Resources[*mx.Client], approve StorageApproval) error {
		if cfg.BeforeOpen != nil {
			if err := cfg.BeforeOpen(ctx, resources, approve); err != nil {
				return err
			}
		}
		if resources.storagePublicationBinder() == nil {
			return consent.BeforeOpen(ctx, resources, approve)
		}
		return resources.Check(ctx)
	}
	application.core = core
	core.cfg = privateApplicationConfig[*mx.Client, R]{
		identity: func() (auth.IdentitySnapshot, bool) {
			return cfg.Suite.Sesion.SnapshotIdentity(), cfg.Suite.Estado().Dentro()
		},
		capture: cfg.Suite.Sesion.WithAuthorizationIntent, canStart: cfg.CanStart,
		build: cfg.Build, mount: cfg.Mount, dispatch: fyne.Do,
		beforeOpen: nativeStorageBeforeOpen(&approvals, approvedPublication),
		replace: func(object fyne.CanvasObject) {
			application.slot.Objects = []fyne.CanvasObject{object}
			application.slot.Refresh()
		},
	}
	core.cfg.state = func(state ApplicationState, failure error) { application.renderState(cfg, state, failure) }
	core.cfg.setup = func(ctx context.Context, ready func(context.Context, *Resources[*mx.Client]), failed func(error)) (applicationPanels, func(context.Context) error, error) {
		open := func(ctx context.Context, target string) error {
			result := make(chan error, 1)
			fyne.Do(func() { result <- api.AbrirFuera(cfg.Application, target) })
			select {
			case err := <-result:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		panel := func() *Panel {
			value := &Panel{Base: mx.Config{SytueApex: cfg.Suite.Apex(), StoreDir: filepath.Join(directory, cfg.Product+"-matrix"), DeviceName: "Sytue " + cfg.Product + " · " + runtime.GOOS}, Lienzo: cfg.Window.Canvas, Traducir: cfg.Translate, Copiar: func(text string) { cfg.Application.Clipboard().SetContent(text) }}
			value.Abrir = func(target string) error { return open(ctx, target) }
			// NativeFactory applies the supported native broker on mobile/consumer apps.
			// This desktop default carries the same operation context and opens system UI.
			value.Login = func(ctx context.Context, configuration mx.Config) (*mx.Client, error) {
				return mx.Login(ctx, configuration, func(target string) error { return open(ctx, target) })
			}
			value.Avisar = func(string) { failed(ErrPrivateResources) }
			value.SoltarFuente = cfg.SoltarFuente
			return value
		}
		legacy, isolated := panel(), panel()
		var consentResources []*Resources[*mx.Client]
		panels, err := NewStoragePanels(StoragePanelsConfig{
			Legacy: legacy, Isolated: isolated, Factory: factory,
			State: func(ctx context.Context, scope storage.Scope) (storage.IdentityCutoverState, error) {
				bound, err := SessionHTTP(ctx)
				if err != nil {
					return storage.IdentityCutoverState{}, err
				}
				return api.NewRooms(cfg.Suite.URL("rooms"), bound, nil).MatrixStorageMigration(ctx, scope)
			},
			Bootstrap: func(ctx context.Context, version storage.IdentityVersion) (mx.Bootstrap, error) {
				bound, err := SessionHTTP(ctx)
				if err != nil {
					return mx.Bootstrap{}, err
				}
				return api.NewRooms(cfg.Suite.URL("rooms"), bound, nil).MatrixStorageBootstrapForIdentity(ctx, cfg.Product, version)
			}, Ready: func(ctx context.Context, resources *Resources[*mx.Client]) {
				approvals.Store(resources, nativeStorageApproval(resources, isolated))
				consentResources = append(consentResources, resources)
				ready(ctx, resources)
			}, Failed: func(err error) { fyne.Do(func() { failed(err) }) },
		})
		if err != nil {
			return nil, nil, err
		}
		retire := func(ctx context.Context) error {
			started := make(chan struct{})
			//nolint:contextcheck // Each panel cancels its own retained lifetime; ctx below joins both closures after identity cancellation.
			fyne.Do(func() {
				for _, resources := range consentResources {
					approvals.Delete(resources)
				}
				consentResources = nil
				legacy.Cerrar()
				isolated.Cerrar()
				close(started)
			})
			select {
			case <-started:
			case <-ctx.Done():
				return ctx.Err()
			}
			return errors.Join(legacy.Retire(ctx), isolated.Retire(ctx))
		}
		return panels, retire, nil
	}
	cfg.Suite.AlCambiar(func(sesion.Estado) { fyne.Do(core.adopt) })
	return application, nil
}
func (a *PrivateApplication[R]) Objeto() fyne.CanvasObject { return a.slot }
func (a *PrivateApplication[R]) Start()                    { a.core.adopt() }
func (a *PrivateApplication[R]) Retry()                    { a.core.retry() }
func (a *PrivateApplication[R]) Show()                     { a.core.show() }
func (a *PrivateApplication[R]) Close()                    { a.core.close() }

// Retire suelta la cuenta actual sin cerrar la app: AplicacionRegistros la usa
// cuando la cuenta deja de ir por Matrix (soltada la fuente). Volver a Matrix es
// Retry, que la adopta de nuevo.
func (a *PrivateApplication[R]) Retire()                 { a.core.retire() }
func (a *PrivateApplication[R]) State() ApplicationState { return a.state }

func (a *PrivateApplication[R]) renderState(cfg PrivateApplicationConfig[R], state ApplicationState, failure error) {
	a.state = state
	if state != ApplicationReady {
		title, body := cfg.Translate("matrix.title"), cfg.Translate("matrix.subtitle")
		if state == ApplicationFailed {
			title, body = cfg.Translate("matrix.failed"), ""
		}
		view := ui.NuevoVacio(title, body)
		if state == ApplicationPreparing || state == ApplicationRetiring {
			loading := widget.NewProgressBarInfinite()
			a.slot.Objects = []fyne.CanvasObject{container.NewBorder(nil, loading, nil, nil, view)}
		} else {
			label, action := cfg.Translate("matrix.connect"), a.Show
			switch state {
			case ApplicationSignedOut:
				label = cfg.Translate("account.signIn")
				action = func() { go func() { _ = cfg.Suite.Entrar(context.Background()) }() }
			case ApplicationFailed:
				label, action = cfg.Translate("action.retry"), a.Retry
			case ApplicationLocked, ApplicationPreparing, ApplicationReady, ApplicationRetiring:
			}
			view.Accion = ui.NuevoBoton(label, action)
			a.slot.Objects = []fyne.CanvasObject{view}
		}
		a.slot.Refresh()
	}
	if cfg.State != nil {
		cfg.State(state, failure)
	}
}
