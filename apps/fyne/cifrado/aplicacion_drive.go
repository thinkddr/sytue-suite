// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// AplicacionDrive es el Drive privado de una app nativa que elige por cuenta
// entre la bóveda directa y Matrix (drive_boveda.go). Es lo que usan Photos y PDF,
// que consumen el repositorio de Drive; la app Drive monta lo mismo con sus
// propias piezas (cmd/drive). Como AplicacionRegistros: la app de Matrix de
// siempre, con su CanStart en manos de este anfitrión, y al lado la bóveda.

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// AplicacionDriveConfig es el Drive privado de una app.
type AplicacionDriveConfig struct {
	// Matrix es la app de Matrix de siempre, con Product "drive". Su CanStart lo
	// pone este anfitrión; el que traiga se respeta además (Puede).
	Matrix PrivateApplicationConfig[*privateDrive.Repository]
	// MontarBoveda monta la vista sobre el repositorio de la bóveda, ya
	// inicializado. Es el Mount de Matrix con la cuenta de la bóveda.
	MontarBoveda func(context.Context, Cuenta, *privateDrive.Repository) (ApplicationView, error)
	// AlDecidir, si está, sabe dónde va la cuenta en cuanto se decide (en el hilo de
	// la interfaz): la ofimática no abre el Drive de Matrix, solo lo dice.
	AlDecidir func(AlmacenDrive)
}

// AplicacionDrive elige por cuenta entre la bóveda y Matrix. Sus métodos se
// llaman desde el hilo de la interfaz.
type AplicacionDrive struct {
	host   *anfitrionDrive
	matrix *PrivateApplication[*privateDrive.Repository]
}

// NuevaAplicacionDrive monta la app; Start la arranca.
func NuevaAplicacionDrive(cfg AplicacionDriveConfig) (*AplicacionDrive, error) {
	suite := cfg.Matrix.Suite
	if suite == nil || suite.Sesion == nil || cfg.Matrix.Product != "drive" || cfg.Matrix.Translate == nil || cfg.MontarBoveda == nil {
		return nil, ErrPrivateResources
	}
	fabrica, err := NuevaFabricaBoveda(suite, "drive")
	if err != nil {
		return nil, err
	}
	translate := cfg.Matrix.Translate
	host := &anfitrionDrive{
		translate: translate,
		identity: func() (auth.IdentitySnapshot, bool) {
			return suite.Sesion.SnapshotIdentity(), suite.Estado().Dentro()
		},
		puede: cfg.Matrix.CanStart,
		elegir: func(ctx context.Context, owner auth.IdentitySnapshot) (AlmacenDrive, error) {
			return ElegirAlmacenDrive(ctx, ConsultaDriveDeSesion(suite, owner))
		},
		puerta: func(lista func(context.Context, *pqvault.Vault)) (puertaDrive, error) {
			return PuertaDriveDeSesion(suite, translate, lista)
		},
		abrir: func(ctx context.Context, vault *pqvault.Vault) (*RecursosBoveda, repositorioBoveda, error) {
			recursos, repository, err := AbrirDriveBoveda(ctx, fabrica, suite.URL("drive"), vault)
			if err != nil {
				return nil, nil, err
			}
			return recursos, repository, nil
		},
		montar:    cfg.MontarBoveda,
		alDecidir: cfg.AlDecidir,
		estado:    cfg.Matrix.State,
		dispatch:  fyne.Do,
		slot:      container.NewStack(),
	}
	matrixCfg := cfg.Matrix
	matrixCfg.CanStart = host.matrixPuedeEmpezar
	matrix, err := NewPrivateApplication(matrixCfg)
	if err != nil {
		return nil, err
	}
	host.matrix = matrix
	suite.AlCambiar(func(sesion.Estado) { fyne.Do(host.adoptar) })
	return &AplicacionDrive{host: host, matrix: matrix}, nil
}

func (a *AplicacionDrive) Objeto() fyne.CanvasObject { return a.host.slot }

// Start arranca la app de Matrix (que, sin decidir, se queda esperando) y decide.
func (a *AplicacionDrive) Start() {
	a.matrix.Start()
	a.host.adoptar()
}

// Retry vuelve a intentar lo que toque: Matrix si la cuenta va por Matrix; si no,
// decidir (o abrir la puerta) otra vez.
func (a *AplicacionDrive) Retry() { a.host.reintentar() }

// EnMatrix dice si la cuenta se decidió mandar a Matrix.
func (a *AplicacionDrive) EnMatrix() bool { return a.host.va(DriveEnMatrix) }

// Show enseña el panel de Matrix cuando la cuenta va por Matrix.
func (a *AplicacionDrive) Show() {
	if a.host.va(DriveEnMatrix) {
		a.matrix.Show()
	}
}

// Close desmonta lo de la bóveda y cierra la app de Matrix.
func (a *AplicacionDrive) Close() {
	a.host.cerrar()
	a.matrix.Close()
}

// puertaDrive es lo que el anfitrión usa de PuertaBoveda (sustituible en pruebas).
type puertaDrive interface {
	Objeto() fyne.CanvasObject
	Iniciar(context.Context)
	// IniciarVaciando arranca vaciando la bóveda (la confirmación ya se pidió).
	IniciarVaciando(context.Context)
	Cerrar()
}

// repositorioBoveda es lo que el anfitrión usa de PreparedDriveRepository.
type repositorioBoveda interface {
	Initialize(context.Context) error
	Close()
	DriveRepository() *privateDrive.Repository
}

// DriveRepository es el repositorio de dominio que envuelve.
func (r *PreparedDriveRepository) DriveRepository() *privateDrive.Repository { return r.Repository }

type anfitrionDrive struct {
	translate func(string) string
	identity  func() (auth.IdentitySnapshot, bool)
	puede     func() bool
	elegir    func(context.Context, auth.IdentitySnapshot) (AlmacenDrive, error)
	puerta    func(func(context.Context, *pqvault.Vault)) (puertaDrive, error)
	abrir     func(context.Context, *pqvault.Vault) (*RecursosBoveda, repositorioBoveda, error)
	montar    func(context.Context, Cuenta, *privateDrive.Repository) (ApplicationView, error)
	alDecidir func(AlmacenDrive)
	estado    func(ApplicationState, error)
	dispatch  func(func())
	matrix    appMatrix
	slot      *fyne.Container

	owner   auth.IdentitySnapshot
	inside  bool
	life    context.Context
	stop    context.CancelFunc
	cerrado bool
	ocupado bool
	vigente puertaDrive
	// fallo es el último fallo al cargar el Drive de la bóveda (para volver a él).
	fallo    error
	recursos *RecursosBoveda
	repo     repositorioBoveda
	vista    *ApplicationView

	// mu guarda destino y para: los lee CanStart de la app de Matrix.
	mu      sync.Mutex
	destino AlmacenDrive
	para    auth.IdentitySnapshot
}

func (h *anfitrionDrive) puedeEmpezar() bool { return h.puede == nil || h.puede() }

// matrixPuedeEmpezar es el CanStart de la app de Matrix: solo para la cuenta que
// se decidió mandar a Matrix. Sin esto, una cuenta cuyo Drive está en la bóveda
// abriría Matrix y escribiría donde la web ya no lee.
func (h *anfitrionDrive) matrixPuedeEmpezar() bool {
	owner, inside := h.identity()
	h.mu.Lock()
	propia := inside && !h.cerrado && h.destino == DriveEnMatrix && h.para == owner
	h.mu.Unlock()
	return propia && h.puedeEmpezar()
}

func (h *anfitrionDrive) va(destino AlmacenDrive) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.destino == destino
}

func (h *anfitrionDrive) adoptar() {
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
		h.enseñar(h.matrix.Objeto())
		return
	}
	h.life, h.stop = context.WithCancel(context.Background()) //nolint:gosec // G118: soltar y cerrar llaman a stop.
	if h.puedeEmpezar() {
		h.decidir()
	} else {
		h.enseñar(h.matrix.Objeto())
	}
}

func (h *anfitrionDrive) reintentar() {
	if h.cerrado || h.ocupado {
		return
	}
	if h.life == nil {
		// Sin arrancar (Photos solo la pide cuando hace falta): adoptar decide.
		h.adoptar()
		return
	}
	h.mu.Lock()
	destino := h.destino
	h.mu.Unlock()
	switch {
	case destino == DriveEnMatrix:
		h.matrix.Retry()
	case destino == DriveEnBoveda && h.repo != nil:
		// Ya montada: nada que reintentar.
	default:
		if h.puedeEmpezar() {
			h.soltarBoveda()
			h.decidir()
		}
	}
}

func (h *anfitrionDrive) decidir() {
	life, owner := h.life, h.owner
	if life == nil || h.ocupado {
		return
	}
	h.mu.Lock()
	h.destino, h.para = 0, auth.IdentitySnapshot{}
	h.mu.Unlock()
	h.ocupado = true
	h.enseñarCargando()
	go func() {
		almacen, err := h.elegir(life, owner)
		h.dispatch(func() { h.decidido(life, owner, almacen, err) })
	}()
}

func (h *anfitrionDrive) actual(life context.Context, owner auth.IdentitySnapshot) bool {
	if h.cerrado || life.Err() != nil || h.life != life || h.owner != owner {
		return false
	}
	actual, inside := h.identity()
	return inside && actual == owner
}

func (h *anfitrionDrive) decidido(life context.Context, owner auth.IdentitySnapshot, almacen AlmacenDrive, err error) {
	if !h.actual(life, owner) {
		return
	}
	h.ocupado = false
	if err != nil {
		h.fallar(err) //nolint:contextcheck // Soltar la bóveda vive más que esta decisión: espera a la vista y cierra sin cancelar.
		return
	}
	h.mu.Lock()
	h.destino, h.para = almacen, owner
	h.mu.Unlock()
	if h.alDecidir != nil {
		h.alDecidir(almacen)
	}
	if almacen == DriveEnMatrix {
		h.enseñar(h.matrix.Objeto())
		h.matrix.Retry()
		return
	}
	h.abrirPuerta(life, owner, false)
}

func (h *anfitrionDrive) abrirPuerta(life context.Context, owner auth.IdentitySnapshot, vaciar bool) {
	var vigente puertaDrive
	puerta, err := h.puerta(func(ctx context.Context, vault *pqvault.Vault) {
		h.bovedaLista(life, owner, vigente, ctx, vault)
	})
	if err != nil {
		h.fallar(err) //nolint:contextcheck // Soltar la bóveda vive más que esta decisión: espera a la vista y cierra sin cancelar.
		return
	}
	vigente = puerta
	h.vigente = vigente
	h.enseñar(puerta.Objeto())
	if vaciar {
		puerta.IniciarVaciando(life)
		return
	}
	puerta.Iniciar(life)
}

// bovedaLista recibe la bóveda abierta (hilo de la interfaz): abre los recursos y
// el repositorio, y lo inicializa fuera del hilo.
//
//nolint:revive // ctx no va primero: es el de la puerta, distinto de la vida de la cuenta.
func (h *anfitrionDrive) bovedaLista(life context.Context, owner auth.IdentitySnapshot, puerta puertaDrive, ctx context.Context, vault *pqvault.Vault) {
	if !h.actual(life, owner) || puerta == nil || h.vigente != puerta || h.repo != nil || h.ocupado {
		vault.Close()
		return
	}
	h.ocupado = true
	h.enseñarCargando()
	go func() {
		recursos, repo, err := h.abrir(ctx, vault)
		if err == nil {
			// El repositorio vive lo que la cuenta (recursos.Context), no lo que la
			// entrega de la puerta.
			err = repo.Initialize(recursos.Context()) //nolint:contextcheck // ver arriba.
		}
		h.dispatch(func() { h.inicializada(life, owner, puerta, recursos, repo, err) })
	}()
}

func (h *anfitrionDrive) inicializada(life context.Context, owner auth.IdentitySnapshot, puerta puertaDrive, recursos *RecursosBoveda, repo repositorioBoveda, err error) {
	vigente := h.actual(life, owner) && h.vigente == puerta
	if vigente {
		h.ocupado = false
	}
	if err == nil && vigente {
		var view ApplicationView
		view, err = h.montar(recursos.Context(), recursos, repo.DriveRepository()) //nolint:contextcheck // La vista vive lo que la cuenta, no lo que la decisión.
		if err == nil {
			h.recursos, h.repo, h.vista = recursos, repo, &view
			h.enseñar(view.Object)
			if h.estado != nil {
				h.estado(ApplicationReady, nil)
			}
			return
		}
	}
	// Fallo o cuenta vieja: nada queda abierto. El diario guarda lo pendiente.
	if repo != nil {
		go func() { repo.Close(); recursos.Close() }()
	} else if recursos != nil {
		recursos.Close()
	}
	if vigente {
		h.fallarAbierta(err) //nolint:contextcheck // Soltar la bóveda vive más que esta decisión: espera a la vista y cierra sin cancelar.
	}
}

// fallarAbierta es fallar cuando la bóveda sí abrió pero su Drive no se pudo cargar
// (la importación del servidor no cuadra, o la fuente ya no está): además de
// reintentar, deja vaciar, como la web. Sin esta salida, una bóveda recién vaciada
// cuya fuente ya era `private` se quedaba en «reintentar» para siempre.
func (h *anfitrionDrive) fallarAbierta(err error) {
	h.fallo = err
	h.fallar(err)
	vacio, ok := h.slot.Objects[0].(*ui.Vacio)
	if !ok {
		return
	}
	vaciar := ui.NuevoBoton(h.translate("vault.empty"), h.pedirVaciado)
	vaciar.Variante = ui.Secundario
	vacio.AccionSecundaria = vaciar
	vacio.Refresh()
}

// pedirVaciado enseña la confirmación de vaciar, con su casilla obligatoria.
func (h *anfitrionDrive) pedirVaciado() {
	if h.cerrado || h.ocupado || h.life == nil {
		return
	}
	var accion *ui.Boton
	entiendo := ui.NuevaCasilla(h.translate("vault.emptyUnderstood"), func(marcada bool) {
		accion.Inerte = !marcada
		accion.Refresh()
	})
	accion = ui.NuevoBoton(h.translate("vault.empty"), func() {
		if entiendo.Marcada {
			h.vaciar()
		}
	})
	accion.Variante = ui.Peligro
	accion.Inerte = true
	volver := ui.NuevoBoton(h.translate("action.back"), func() { h.fallarAbierta(h.fallo) })
	volver.Variante = ui.Secundario
	cuerpo := widget.NewLabel(h.translate("vault.emptyBody"))
	cuerpo.Wrapping = fyne.TextWrapWord
	h.enseñar(container.NewVBox(
		widget.NewLabelWithStyle(h.translate("vault.emptyTitle"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		cuerpo, entiendo, container.NewHBox(accion, volver),
	))
}

// vaciar abre la puerta vaciando: suelta la fuente del servidor (AntesDeVaciar) y
// estrena la bóveda, que vuelve a importar el Drive del servidor.
func (h *anfitrionDrive) vaciar() {
	if h.cerrado || h.ocupado || h.life == nil || !h.actual(h.life, h.owner) {
		return
	}
	h.soltarBoveda()
	h.mu.Lock()
	h.destino, h.para = DriveEnBoveda, h.owner
	h.mu.Unlock()
	h.abrirPuerta(h.life, h.owner, true)
}

func (h *anfitrionDrive) fallar(err error) {
	// Sin destino: ni Matrix ni la bóveda hasta volver a decidir.
	h.mu.Lock()
	h.destino, h.para = 0, auth.IdentitySnapshot{}
	h.mu.Unlock()
	h.soltarBoveda()
	// El código corto (drive.import.row_heads, http.409…) debajo del aviso: sin él,
	// en producción no había forma de saber qué comprobación lo rechazó.
	vacio := ui.NuevoVacioError(h.translate("matrix.failed"), avisarFallo("el Drive de la bóveda", err)) // DS-06
	vacio.Accion = ui.NuevoBoton(h.translate("action.retry"), h.reintentar)
	h.enseñar(vacio)
	if h.estado != nil {
		h.estado(ApplicationFailed, err)
	}
}

// soltarBoveda cierra la puerta, desmonta la vista y retira repositorio y
// bóveda. Lo pendiente queda en el diario de la bóveda.
func (h *anfitrionDrive) soltarBoveda() {
	if h.vigente != nil {
		h.vigente.Cerrar()
		h.vigente = nil
	}
	view, repo, recursos := h.vista, h.repo, h.recursos
	h.vista, h.repo, h.recursos = nil, nil, nil
	if view != nil && view.Detach != nil {
		view.Detach()
	}
	if repo == nil && recursos == nil {
		return
	}
	go func() {
		if view != nil && view.Join != nil {
			_ = view.Join(context.Background())
		}
		if repo != nil {
			repo.Close()
		}
		recursos.Close()
	}()
}

func (h *anfitrionDrive) soltar() {
	if h.stop != nil {
		h.stop()
	}
	h.life, h.stop, h.ocupado = nil, nil, false
	h.mu.Lock()
	h.destino, h.para = 0, auth.IdentitySnapshot{}
	h.mu.Unlock()
	h.soltarBoveda()
}

func (h *anfitrionDrive) cerrar() {
	h.soltar()
	h.mu.Lock()
	h.cerrado = true
	h.mu.Unlock()
}

func (h *anfitrionDrive) enseñarCargando() {
	h.enseñar(container.NewBorder(nil, widget.NewProgressBarInfinite(), nil, nil, ui.NuevoVacio(h.translate("state.loading"), "")))
}

func (h *anfitrionDrive) enseñar(object fyne.CanvasObject) {
	h.slot.Objects = []fyne.CanvasObject{object}
	h.slot.Refresh()
	ui.RecolocarBarraSuite(h.slot)
}
