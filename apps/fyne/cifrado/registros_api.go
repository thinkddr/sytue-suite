// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// Las apps nativas de registros igual que la web (ADR 0086, «clientes nativos»): en
// la web, calendar, contacts, erp, bases, cast, hub y las ofimáticas pasan la puerta
// de la bóveda y leen y escriben por la API de su servicio. Aquí, mientras la fuente
// del servidor de esa cuenta siga `legacy`, la app nativa hace lo mismo: monta su
// vista sobre la API con la sesión de Sytue ID, sin Matrix y sin congelar nada.
//
// Si la fuente ya NO es `legacy`, los datos de esa cuenta se copiaron a Matrix desde
// un dispositivo nativo y el servidor ya no admite escrituras en la fuente (el
// disparador `sytue_private_storage_guard` no deja volver a `legacy`). Ahí, y solo
// ahí, la app sigue por Matrix como hasta ahora: es donde están sus datos.

import (
	"context"
	"errors"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// FuenteLegacy es el modo de la fuente del servidor que todavía es la de la web.
const FuenteLegacy = "legacy"

// Cuenta es lo que una vista de registros necesita de la cuenta que la monta, venga
// de la API (CuentaAPI) o de Matrix (*Resources).
type Cuenta interface {
	Context() context.Context
	PrivateStorageScope() storage.Scope
	HTTP() api.Session
	Check(context.Context) error
}

var _ Cuenta = (*Resources[*mx.Client])(nil)

// ModoFuente lee el modo de la fuente de un producto en su servidor (`legacy`,
// `building`, `frozen` o `private`).
type ModoFuente func(context.Context, api.Session, storage.Scope) (string, error)

// AplicacionRegistrosConfig es una app de registros nativa: la de Matrix de siempre
// y, al lado, cómo montar su vista sobre la API y cómo saber cuál toca.
type AplicacionRegistrosConfig[R PrivateRepository] struct {
	// Matrix es la configuración de la app de Matrix. Su CanStart lo pone esta app.
	Matrix PrivateApplicationConfig[R]
	Modo   ModoFuente
	// API monta la vista sobre la API del servidor, como la web.
	API func(Cuenta) (ApplicationView, error)
	// Soltar devuelve la fuente del servidor a legacy («soltar la fuente», ADR
	// 0086): la salida del panel de Matrix, abra o no. Nil: sin salida.
	// SoltarEn(servicio) es la de todos los servicios de registros.
	Soltar SoltarFuente
}

// SoltarFuente pide al servidor volver a los datos de antes de congelar.
type SoltarFuente func(context.Context, api.Session, storage.Scope) error

// SoltarEn es «soltar la fuente» contra la raíz de un servicio (Suite.URL).
func SoltarEn(servicio string) SoltarFuente {
	return func(ctx context.Context, session api.Session, scope storage.Scope) error {
		return api.SoltarFuentePrivada(ctx, servicio, session, scope)
	}
}

// AplicacionRegistros elige por cuenta entre la API y Matrix. Sus métodos se llaman
// desde el hilo de la interfaz, como los de PrivateApplication.
type AplicacionRegistros[R PrivateRepository] struct {
	host   *registros
	matrix *PrivateApplication[R]
}

// NuevaAplicacionRegistros monta la app; Start la arranca.
func NuevaAplicacionRegistros[R PrivateRepository](cfg AplicacionRegistrosConfig[R]) (*AplicacionRegistros[R], error) {
	suite := cfg.Matrix.Suite
	if suite == nil || suite.Sesion == nil || cfg.Modo == nil || cfg.API == nil || cfg.Matrix.Translate == nil || cfg.Matrix.CanStart != nil {
		return nil, ErrPrivateResources
	}
	host := &registros{
		product:   cfg.Matrix.Product,
		translate: cfg.Matrix.Translate,
		identity: func() (auth.IdentitySnapshot, bool) {
			return suite.Sesion.SnapshotIdentity(), suite.Estado().Dentro()
		},
		session: func(owner auth.IdentitySnapshot) api.Session {
			return nuevaSesionDeCuenta(suite.Sesion, owner)
		},
		modo:          cfg.Modo,
		montar:        cfg.API,
		devolver:      cfg.Soltar,
		reidentificar: suite.Reidentificar,
		dispatch:      fyne.Do,
		slot:          container.NewStack(),
	}
	matrixCfg := cfg.Matrix
	matrixCfg.CanStart = host.matrixPuedeEmpezar
	if cfg.Soltar != nil {
		matrixCfg.SoltarFuente = host.soltarFuente
	}
	matrix, err := NewPrivateApplication(matrixCfg)
	if err != nil {
		return nil, err
	}
	host.matrix = matrix
	app := &AplicacionRegistros[R]{host: host, matrix: matrix}
	suite.AlCambiar(func(sesion.Estado) { fyne.Do(host.adoptar) })
	return app, nil
}

func (a *AplicacionRegistros[R]) Objeto() fyne.CanvasObject { return a.host.slot }

// Start arranca la app de Matrix (que, sin decidir, se queda esperando) y decide.
func (a *AplicacionRegistros[R]) Start() {
	a.matrix.Start()
	a.host.adoptar()
}

// Show es el Show de la app de Matrix cuando la cuenta va por Matrix; por la API la
// vista ya está a la vista, y sin decidir no hay nada que enseñar todavía.
func (a *AplicacionRegistros[R]) Show() {
	if a.host.vaPorMatrix() {
		a.matrix.Show()
	}
}

// Retry reintenta la app de Matrix cuando la cuenta va por Matrix: lo que pide un
// anfitrión cuando algo de lo que depende ya está listo (Photos, al montarse el
// Drive de sus originales). Por la API, o sin decidir, no hay nada que reintentar.
func (a *AplicacionRegistros[R]) Retry() {
	if a.host.vaPorMatrix() {
		a.matrix.Retry()
	}
}

// Close desmonta la vista de la API y cierra la de Matrix.
func (a *AplicacionRegistros[R]) Close() {
	a.host.cerrar()
	a.matrix.Close()
}

// appMatrix es lo que el anfitrión usa de PrivateApplication (sustituible en pruebas).
type appMatrix interface {
	Objeto() fyne.CanvasObject
	Retry()
	Retire()
}

// destino es lo que se decidió para una cuenta.
type destino uint8

const (
	destinoSinDecidir destino = iota
	destinoAPI
	destinoMatrix
	destinoFallo
)

type registros struct {
	product   string
	translate func(string) string
	identity  func() (auth.IdentitySnapshot, bool)
	session   func(auth.IdentitySnapshot) api.Session
	modo      ModoFuente
	montar    func(Cuenta) (ApplicationView, error)
	devolver  SoltarFuente
	// reidentificar vuelve a entrar en Sytue ID pidiendo identificarse otra vez.
	reidentificar func(context.Context) error
	dispatch      func(func())
	matrix        appMatrix
	slot          *fyne.Container

	owner   auth.IdentitySnapshot
	inside  bool
	life    context.Context
	stop    context.CancelFunc
	cerrado bool
	// mu guarda destino y para: los lee CanStart de la app de Matrix.
	mu      sync.Mutex
	destino destino
	para    auth.IdentitySnapshot
	vista   *ApplicationView
	cuenta  *CuentaAPI
}

// matrixPuedeEmpezar es el CanStart de la app de Matrix: solo arranca para la cuenta
// a la que se decidió mandarla a Matrix. Sin esto, una cuenta en `legacy` abriría
// Matrix y su primera importación congelaría la fuente que la web sigue usando.
func (h *registros) matrixPuedeEmpezar() bool {
	owner, inside := h.identity()
	h.mu.Lock()
	defer h.mu.Unlock()
	return inside && !h.cerrado && h.destino == destinoMatrix && h.para == owner
}

func (h *registros) vaPorMatrix() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.destino == destinoMatrix
}

// adoptar sigue a la sesión: otra cuenta (o salir) desmonta lo de la anterior y
// vuelve a decidir. Refrescar el token no cambia nada.
func (h *registros) adoptar() {
	if h.cerrado {
		return
	}
	owner, inside := h.identity()
	if h.life != nil && h.owner == owner && h.inside == inside {
		return
	}
	h.soltar()
	h.owner, h.inside = owner, inside
	if !inside || owner.Subject == "" {
		// Fuera: la de Matrix enseña «Iniciar sesión».
		h.enseñar(h.matrix.Objeto())
		return
	}
	h.life, h.stop = context.WithCancel(context.Background()) //nolint:gosec // G118: soltar y cerrar llaman a stop.
	h.decidir()
}

func (h *registros) decidir() {
	life, owner := h.life, h.owner
	if life == nil {
		return
	}
	h.mu.Lock()
	h.destino, h.para = destinoSinDecidir, auth.IdentitySnapshot{}
	h.mu.Unlock()
	h.enseñarCargando()
	scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: h.product}
	session := h.session(owner)
	go func() {
		modo, err := h.modo(life, session, scope)
		h.dispatch(func() { h.decidido(life, owner, scope, session, modo, err) })
	}()
}

func (h *registros) decidido(life context.Context, owner auth.IdentitySnapshot, scope storage.Scope, session api.Session, modo string, err error) {
	if h.cerrado || life.Err() != nil || h.life != life || h.owner != owner {
		return
	}
	if err != nil {
		h.fallar()
		return
	}
	if modo != FuenteLegacy {
		h.mu.Lock()
		h.destino, h.para = destinoMatrix, owner
		h.mu.Unlock()
		h.enseñar(h.matrix.Objeto())
		h.matrix.Retry()
		return
	}
	cuenta := &CuentaAPI{life: life, scope: scope, session: session, current: func() bool {
		actual, dentro := h.identity()
		return dentro && actual == owner
	}}
	view, err := h.montar(cuenta)
	if err != nil {
		h.fallar()
		return
	}
	h.mu.Lock()
	h.destino, h.para = destinoAPI, owner
	h.mu.Unlock()
	h.vista, h.cuenta = &view, cuenta
	h.enseñar(view.Object)
}

// soltarFuente es la salida del panel de Matrix cuando la clave no abre: el
// servidor vuelve a los datos de antes de congelar, Matrix se suelta y la cuenta
// va por la API. Si el servicio pide identificarse otra vez, se abre Sytue ID
// (prompt=login, max_age=0) y se repite una vez, para la MISMA cuenta. La persona
// ya lo confirmó en el panel; el servicio exige además la confirmación en el cuerpo.
// Corre fuera del hilo de la interfaz.
func (h *registros) soltarFuente(ctx context.Context) error {
	if h.devolver == nil {
		return ErrPrivateResources
	}
	owner, inside := h.identity()
	h.mu.Lock()
	propia := inside && !h.cerrado && h.destino == destinoMatrix && h.para == owner
	h.mu.Unlock()
	if !propia || owner.Subject == "" {
		return auth.ErrAuthorizationChanged
	}
	scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: h.product}
	err := h.devolver(ctx, h.session(owner), scope)
	if errors.Is(err, api.ErrReautenticacion) && h.reidentificar != nil {
		if errEntrar := h.reidentificar(ctx); errEntrar != nil {
			return errEntrar
		}
		if actual, dentro := h.identity(); !dentro || actual != owner {
			return auth.ErrAuthorizationChanged
		}
		err = h.devolver(ctx, h.session(owner), scope)
	}
	if errors.Is(err, api.ErrFuenteCambiada) {
		// Otro dispositivo congeló otra vez entre leer la época y soltarla: una vez más.
		err = h.devolver(ctx, h.session(owner), scope)
	}
	if err != nil {
		return err
	}
	h.dispatch(func() { h.soltada(owner) }) //nolint:contextcheck // La vista nueva vive con la cuenta, no con esta petición del panel.
	return nil
}

// soltada: la fuente de `owner` ya es legacy. Matrix se suelta y se vuelve a
// decidir, que ahora monta la vista sobre la API.
func (h *registros) soltada(owner auth.IdentitySnapshot) {
	if h.cerrado || h.life == nil || h.owner != owner {
		return
	}
	h.mu.Lock()
	h.destino, h.para = destinoSinDecidir, auth.IdentitySnapshot{}
	h.mu.Unlock()
	h.matrix.Retire()
	h.decidir()
}

func (h *registros) fallar() {
	h.mu.Lock()
	h.destino = destinoFallo
	h.mu.Unlock()
	vacio := ui.NuevoVacio(h.translate("error.connection"), "")
	vacio.Accion = ui.NuevoBoton(h.translate("action.retry"), h.decidir)
	h.enseñar(vacio)
}

// soltar desmonta la vista de la API y corta lo que quede en marcha de esa cuenta.
func (h *registros) soltar() {
	if h.stop != nil {
		h.stop()
	}
	h.life, h.stop = nil, nil
	h.mu.Lock()
	h.destino, h.para = destinoSinDecidir, auth.IdentitySnapshot{}
	h.mu.Unlock()
	view := h.vista
	h.vista, h.cuenta = nil, nil
	if view == nil {
		return
	}
	if view.Detach != nil {
		view.Detach()
	}
	if view.Join != nil {
		go func() { _ = view.Join(context.Background()) }()
	}
}

func (h *registros) cerrar() {
	h.soltar()
	h.mu.Lock()
	h.cerrado = true
	h.mu.Unlock()
}

func (h *registros) enseñarCargando() {
	h.enseñar(container.NewBorder(nil, widget.NewProgressBarInfinite(), nil, nil, ui.NuevoVacio(h.translate("state.loading"), "")))
}

func (h *registros) enseñar(object fyne.CanvasObject) {
	h.slot.Objects = []fyne.CanvasObject{object}
	h.slot.Refresh()
	ui.RecolocarBarraSuite(h.slot)
}

// CuentaAPI es la cuenta de una vista montada sobre la API: su vida acaba al
// cambiar de cuenta o salir, y su sesión no da el token de nadie más.
type CuentaAPI struct {
	life    context.Context
	scope   storage.Scope
	session api.Session
	current func() bool
}

func (c *CuentaAPI) Context() context.Context           { return c.life }
func (c *CuentaAPI) PrivateStorageScope() storage.Scope { return c.scope }
func (c *CuentaAPI) HTTP() api.Session                  { return c.session }

// Check falla en cuanto la cuenta deja de ser la de la vista.
func (c *CuentaAPI) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.life.Err(); err != nil {
		return err
	}
	if !c.current() {
		return auth.ErrAuthorizationChanged
	}
	return nil
}

// tokenDeSesion es lo que sesionDeCuenta usa de auth.Session.
type tokenDeSesion interface {
	AccessToken(context.Context) (string, error)
	Invalidate(string)
	SnapshotIdentity() auth.IdentitySnapshot
}

// sesionDeCuenta es la sesión de la suite atada a una cuenta: cada petición captura
// su intención de autorización y la compara con la dueña de la vista, y la sesión
// la vuelve a comprobar bajo su cerrojo al dar el token (AccessTokenScoped). Una
// petición que sale después de cambiar de cuenta falla; nunca lleva el token de la
// cuenta nueva a la vista de la anterior.
type sesionDeCuenta struct {
	session tokenDeSesion
	// capturar devuelve el contexto con la intención y la cuenta que la capturó.
	capturar func(context.Context) (context.Context, auth.IdentitySnapshot)
	owner    auth.IdentitySnapshot
}

func nuevaSesionDeCuenta(session *auth.Session, owner auth.IdentitySnapshot) sesionDeCuenta {
	return sesionDeCuenta{session: session, owner: owner, capturar: func(ctx context.Context) (context.Context, auth.IdentitySnapshot) {
		ctx = session.WithAuthorizationIntent(ctx)
		var identity auth.IdentitySnapshot
		if intent := auth.RequestAuthorizationIntent(ctx); intent != nil {
			identity = intent.Identity()
		}
		return ctx, identity
	}}
}

func (s sesionDeCuenta) AccessToken(ctx context.Context) (string, error) {
	ctx, identity := s.capturar(ctx)
	if identity != s.owner {
		return "", auth.ErrAuthorizationChanged
	}
	return s.session.AccessToken(ctx)
}

func (s sesionDeCuenta) Invalidate(rechazado string) {
	if s.session.SnapshotIdentity() == s.owner {
		s.session.Invalidate(rechazado)
	}
}
