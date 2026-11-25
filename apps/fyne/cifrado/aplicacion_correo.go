// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// AplicacionCorreo es Sytue Mail nativo eligiendo por cuenta entre la bóveda
// directa y Matrix como la web (`PrivateMailGate.tsx`, ADR 0087):
//
//   - autoridad de la bóveda o cuenta nueva → la puerta de la bóveda y el correo
//     en modo de bóveda;
//   - autoridad de Matrix confirmada → «Pasa tu correo a la nueva bóveda»: vaciar y
//     pasar, o seguir por ahora con el anterior (en esta ejecución);
//   - ante la duda → el correo de Matrix de siempre, con la salida «¿No puedes abrir
//     tu correo?» mientras no está abierto;
//   - Matrix retirada en Rooms (ADR 0087, entrega 5: `storage_matrix_retired`) → la
//     misma pantalla SIN «seguir con el anterior»; y desde que Rooms lo dijo (se
//     recuerda en las preferencias de la app), la duda ya no abre Matrix sino un
//     «Reintentar». El anfitrión lo sabe solo por la respuesta de Rooms.
//
// Vaciar y pasar abre una bóveda NUEVA (pqvault.Reset) y registra su clave en
// Sytue ID con la marca de traspaso. Si Sytue ID pide identificarse otra vez, se
// vuelve a entrar (Suite.Reidentificar) y se registra la MISMA bóveda: nunca se
// vacía dos veces. Lo pedido vive en la ejecución, como el `sessionStorage` de la
// pestaña en la web; en cuanto la autoridad es de la bóveda, se olvida.

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// AplicacionCorreoConfig es el correo privado de la app Mail.
type AplicacionCorreoConfig struct {
	// Matrix es la app de Matrix de siempre, con Product "mail". Su CanStart lo pone
	// este anfitrión; el que traiga se respeta además.
	Matrix PrivateApplicationConfig[*MailApplicationRepository]
	// MontarBoveda monta la vista sobre el correo de la bóveda, ya preparado. Es el
	// Mount de Matrix con la cuenta de la bóveda.
	MontarBoveda func(context.Context, Cuenta, *MailApplicationRepository) (ApplicationView, error)
	// PasarABoveda es la pantalla «Pasa tu correo a la nueva bóveda» (apps/mail):
	// `confirmar`, ya con la casilla marcada, vacía y pasa; `seguir` sigue con Matrix.
	// Con `seguir` nil (Matrix retirada) no hay vuelta atrás: la pantalla no ofrece
	// seguir y dice lo que se pierde.
	PasarABoveda func(confirmar, seguir func()) fyne.CanvasObject
	// SinComprobar es el aviso «no se ha podido comprobar…» con `reintentar`
	// (apps/mail): la duda cuando Rooms ya dijo que Matrix está retirada.
	SinComprobar func(reintentar func()) fyne.CanvasObject
	// OfertaTraspaso es la salida «¿No puedes abrir tu correo?» debajo de Matrix
	// (apps/mail); `confirmar` llega ya con la casilla marcada.
	OfertaTraspaso func(confirmar func()) fyne.CanvasObject
}

// AplicacionCorreo elige por cuenta entre la bóveda y Matrix. Sus métodos se
// llaman desde el hilo de la interfaz.
type AplicacionCorreo struct {
	host   *anfitrionCorreo
	matrix *PrivateApplication[*MailApplicationRepository]
}

// NuevaAplicacionCorreo monta la app; Start la arranca.
func NuevaAplicacionCorreo(cfg AplicacionCorreoConfig) (*AplicacionCorreo, error) {
	suite := cfg.Matrix.Suite
	if suite == nil || suite.Sesion == nil || cfg.Matrix.Product != "mail" || cfg.Matrix.Translate == nil || cfg.MontarBoveda == nil || cfg.PasarABoveda == nil || cfg.OfertaTraspaso == nil || cfg.SinComprobar == nil {
		return nil, ErrPrivateResources
	}
	fabrica, err := NuevaFabricaBoveda(suite, "mail")
	if err != nil {
		return nil, err
	}
	servicios := CorreoBovedaConfig{MailURL: suite.URL("mail"), DriveURL: suite.URL("drive"), RoomsURL: suite.URL("rooms")}
	translate := cfg.Matrix.Translate
	host := &anfitrionCorreo{
		translate: translate,
		identity: func() (auth.IdentitySnapshot, bool) {
			return suite.Sesion.SnapshotIdentity(), suite.Estado().Dentro()
		},
		puede: cfg.Matrix.CanStart,
		elegir: func(ctx context.Context, owner auth.IdentitySnapshot) AlmacenCorreo {
			scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: "mail"}
			return ElegirAlmacenCorreo(ctx, api.NewRooms(suite.URL("rooms"), nuevaSesionDeCuenta(suite.Sesion, owner), nil), scope)
		},
		puerta: func(modo modoPuerta, lista func(context.Context, *pqvault.Vault)) (puertaDrive, error) {
			return puertaCorreoDeSesion(suite, translate, modo, lista)
		},
		reidentificar: suite.Reidentificar,
		intencion: func(ctx context.Context, owner auth.IdentitySnapshot) (context.Context, error) {
			ctx = suite.Sesion.WithAuthorizationIntent(ctx)
			if auth.RequestAuthorizationIntent(ctx).Identity() != owner {
				return nil, auth.ErrAuthorizationChanged
			}
			return ctx, nil
		},
		registrar: func(ctx context.Context, owner auth.IdentitySnapshot, vault *pqvault.Vault, handover bool) pqvault.SigningStatus {
			return vault.RegisterSigningKey(ctx, api.NewAccount(suite.URL("id"), nuevaSesionDeCuenta(suite.Sesion, owner), nil), handover)
		},
		abrir: func(ctx context.Context, vault *pqvault.Vault) (recursosCorreo, repositorioCorreo, error) {
			recursos, repository, err := AbrirCorreoBoveda(ctx, fabrica, servicios, vault)
			if err != nil {
				return nil, nil, err
			}
			return recursos, repository, nil
		},
		montar:            cfg.MontarBoveda,
		vistaPasar:        cfg.PasarABoveda,
		vistaOferta:       cfg.OfertaTraspaso,
		vistaSinComprobar: cfg.SinComprobar,
		retirada:          retiradaEnPreferencias(cfg.Matrix.Application),
		estado:            cfg.Matrix.State,
		dispatch:          fyne.Do,
		slot:              container.NewStack(),
		traspasos:         map[auth.IdentitySnapshot]*marcaTraspaso{},
		seguir:            map[auth.IdentitySnapshot]bool{},
	}
	matrixCfg := cfg.Matrix
	matrixCfg.CanStart = host.matrixPuedeEmpezar
	estado := cfg.Matrix.State
	matrixCfg.State = func(state ApplicationState, err error) {
		host.estadoMatrix(state)
		if estado != nil {
			estado(state, err)
		}
	}
	matrix, err := NewPrivateApplication(matrixCfg)
	if err != nil {
		return nil, err
	}
	host.matrix = matrix
	suite.AlCambiar(func(sesion.Estado) { fyne.Do(host.adoptar) })
	return &AplicacionCorreo{host: host, matrix: matrix}, nil
}

func (a *AplicacionCorreo) Objeto() fyne.CanvasObject { return a.host.slot }

// Start arranca la app de Matrix (que, sin decidir, se queda esperando) y decide.
func (a *AplicacionCorreo) Start() {
	a.matrix.Start()
	a.host.adoptar()
}

// Retry vuelve a intentar lo que toque.
func (a *AplicacionCorreo) Retry() { a.host.reintentar() }

// Close desmonta lo de la bóveda y cierra la app de Matrix.
func (a *AplicacionCorreo) Close() {
	a.host.cerrar()
	a.matrix.Close()
}

// modoPuerta es cómo abre la puerta de la bóveda del correo.
type modoPuerta uint8

const (
	// puertaNormal abre la bóveda que hay.
	puertaNormal modoPuerta = iota
	// puertaTraspaso vacía la bóveda (una nueva) y registra su clave con la marca
	// de traspaso.
	puertaTraspaso
	// puertaTraspasoVaciada: en esta ejecución ya se vació para el traspaso; se
	// abre esa bóveda y se registra con la marca, sin vaciar otra vez.
	puertaTraspasoVaciada
)

func (m modoPuerta) traspaso() bool { return m != puertaNormal }

// aperturas es cómo abre y cómo vacía la puerta en cada modo. En los de
// traspaso, las dos llevan la marca de traspaso.
func (m modoPuerta) aperturas() (abrir, vaciar func(context.Context, pqvault.Config) (*pqvault.Vault, error)) {
	conMarca := func(open func(context.Context, pqvault.Config) (*pqvault.Vault, error)) func(context.Context, pqvault.Config) (*pqvault.Vault, error) {
		return func(ctx context.Context, cfg pqvault.Config) (*pqvault.Vault, error) {
			cfg.Handover = true
			return open(ctx, cfg)
		}
	}
	switch m {
	case puertaTraspaso:
		return conMarca(pqvault.Reset), conMarca(pqvault.Reset)
	case puertaTraspasoVaciada:
		return conMarca(pqvault.Open), conMarca(pqvault.Reset)
	case puertaNormal:
	}
	return pqvault.Open, pqvault.Reset
}

// puertaCorreoDeSesion es la puerta de la bóveda del correo de la cuenta actual,
// con las aperturas del modo.
func puertaCorreoDeSesion(suite *sesion.Suite, translate func(string) string, modo modoPuerta, lista func(context.Context, *pqvault.Vault)) (*PuertaBoveda, error) {
	puerta, err := NuevaPuertaBovedaDeSesion(suite, "mail", translate, lista)
	if err != nil {
		return nil, err
	}
	puerta.cfg.abrir, puerta.cfg.vaciar = modo.aperturas()
	return puerta, nil
}

// recursosCorreo es lo que el anfitrión usa de RecursosBoveda: la cuenta de la
// vista y cerrar la bóveda.
type recursosCorreo interface {
	Cuenta
	Close()
}

var _ recursosCorreo = (*RecursosBoveda)(nil)

// repositorioCorreo es lo que el anfitrión usa de RepositorioCorreoBoveda.
type repositorioCorreo interface {
	Initialize(context.Context) error
	Close()
	Application() *MailApplicationRepository
}

// marcaTraspaso es un traspaso pedido para una cuenta en esta ejecución.
type marcaTraspaso struct {
	// vaciada: la bóveda nueva ya se abrió; no se vacía otra vez.
	vaciada bool
}

type anfitrionCorreo struct {
	translate     func(string) string
	identity      func() (auth.IdentitySnapshot, bool)
	puede         func() bool
	elegir        func(context.Context, auth.IdentitySnapshot) AlmacenCorreo
	puerta        func(modoPuerta, func(context.Context, *pqvault.Vault)) (puertaDrive, error)
	reidentificar func(context.Context) error
	intencion     func(context.Context, auth.IdentitySnapshot) (context.Context, error)
	registrar     func(context.Context, auth.IdentitySnapshot, *pqvault.Vault, bool) pqvault.SigningStatus
	abrir         func(context.Context, *pqvault.Vault) (recursosCorreo, repositorioCorreo, error)
	montar        func(context.Context, Cuenta, *MailApplicationRepository) (ApplicationView, error)
	vistaPasar    func(confirmar, seguir func()) fyne.CanvasObject
	vistaOferta   func(confirmar func()) fyne.CanvasObject
	// vistaSinComprobar es la duda con Matrix retirada.
	vistaSinComprobar func(reintentar func()) fyne.CanvasObject
	// retirada recuerda, por cuenta, que Rooms dijo que Matrix está retirada.
	retirada *recuerdoRetirada
	estado   func(ApplicationState, error)
	dispatch func(func())
	matrix   appMatrix
	slot     *fyne.Container

	owner   auth.IdentitySnapshot
	inside  bool
	life    context.Context
	stop    context.CancelFunc
	cerrado bool
	ocupado bool
	vigente puertaDrive
	modo    modoPuerta
	// pendiente es la bóveda entregada cuya clave de firma espera a que la persona
	// vuelva a identificarse.
	pendiente *pqvault.Vault
	// reidentificoSolo: ya se fue sola a Sytue ID por la clave de firma de esta
	// cuenta en esta ejecución; si aún falta, se enseña la vista con sus botones.
	reidentificoSolo map[auth.IdentitySnapshot]bool
	recursos         recursosCorreo
	repo             repositorioCorreo
	vista            *ApplicationView
	matrixLista      bool
	oferta           fyne.CanvasObject

	// Lo pedido por cuenta en esta ejecución.
	traspasos map[auth.IdentitySnapshot]*marcaTraspaso
	seguir    map[auth.IdentitySnapshot]bool

	// mu guarda destino y para: los lee CanStart de la app de Matrix.
	mu      sync.Mutex
	destino destinoCorreo
	para    auth.IdentitySnapshot
}

func (h *anfitrionCorreo) puedeEmpezar() bool { return h.puede == nil || h.puede() }

// matrixPuedeEmpezar es el CanStart de la app de Matrix: solo para la cuenta que
// se decidió mandar a Matrix. Sin esto, una cuenta cuyo correo está en la bóveda
// abriría Matrix, cuyo camino ya no abre un ámbito de la bóveda.
func (h *anfitrionCorreo) matrixPuedeEmpezar() bool {
	owner, inside := h.identity()
	h.mu.Lock()
	propia := inside && !h.cerrado && h.destino == correoMatrix && h.para == owner
	h.mu.Unlock()
	return propia && h.puedeEmpezar()
}

func (h *anfitrionCorreo) va(destino destinoCorreo) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.destino == destino
}

func (h *anfitrionCorreo) decidirDestino(destino destinoCorreo, owner auth.IdentitySnapshot) {
	h.mu.Lock()
	h.destino, h.para = destino, owner
	h.mu.Unlock()
}

// adoptar sigue a la sesión: otra cuenta (o salir) suelta lo de la anterior y
// vuelve a decidir. La vida de cada cuenta empieza aquí, no en quien llama.
//
//nolint:contextcheck // Ver arriba.
func (h *anfitrionCorreo) adoptar() {
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

func (h *anfitrionCorreo) reintentar() {
	if h.cerrado || h.ocupado {
		return
	}
	if h.life == nil {
		h.adoptar()
		return
	}
	switch {
	case h.va(correoMatrix):
		h.matrix.Retry()
	case h.repo != nil || h.vigente != nil || h.pendiente != nil:
		// Montado, o la puerta o la identificación ya enseñan su propia salida.
	default:
		if h.puedeEmpezar() {
			h.decidir()
		}
	}
}

// decidir lee la autoridad del correo fuera del hilo de la interfaz.
func (h *anfitrionCorreo) decidir() {
	life, owner := h.life, h.owner
	if life == nil || h.ocupado {
		return
	}
	h.decidirDestino(correoSinDecidir, auth.IdentitySnapshot{})
	h.ocupado = true
	h.enseñarCargando()
	go func() {
		almacen := h.elegir(life, owner)
		h.dispatch(func() { h.decidido(life, owner, almacen) })
	}()
}

func (h *anfitrionCorreo) actual(life context.Context, owner auth.IdentitySnapshot) bool {
	if h.cerrado || life.Err() != nil || h.life != life || h.owner != owner {
		return false
	}
	actual, inside := h.identity()
	return inside && actual == owner
}

func (h *anfitrionCorreo) decidido(life context.Context, owner auth.IdentitySnapshot, almacen AlmacenCorreo) {
	if !h.actual(life, owner) {
		return
	}
	h.ocupado = false
	marca := h.traspasos[owner]
	destino := elegirDestinoCorreo(almacen, marca != nil, h.seguir[owner], h.aprenderRetirada(owner, almacen))
	switch destino {
	case correoBoveda:
		// Con la autoridad de la bóveda, la marca se olvida: nunca se vacía otra vez.
		delete(h.traspasos, owner)
		h.decidirDestino(correoBoveda, owner)
		h.abrirPuerta(life, owner, puertaNormal)
	case correoTraspaso:
		h.decidirDestino(correoTraspaso, owner)
		h.abrirPuerta(life, owner, h.modoTraspaso(owner))
	case correoElegir:
		h.decidirDestino(correoElegir, owner)
		h.enseñar(h.vistaPasar(h.pedirTraspaso, h.seguirConMatrix))
	case correoPasar:
		// Matrix retirada: solo pasar a la bóveda, sin «seguir con el anterior».
		h.decidirDestino(correoPasar, owner)
		h.enseñar(h.vistaPasar(h.pedirTraspaso, nil))
	case correoSinComprobar:
		// No se sabe dónde está el correo y Matrix ya no abriría: ni Matrix ni vaciar.
		h.decidirDestino(correoSinComprobar, owner)
		h.enseñar(h.vistaSinComprobar(h.reintentar))
	case correoMatrix, correoSinDecidir:
		h.irAMatrix(owner)
	}
}

// aprenderRetirada es lo que una respuesta clara de Rooms enseña de la retirada de
// Matrix, y lo que se sabe después: `retirado` se recuerda; Matrix confirmada (el
// conmutador de Rooms apagado) se olvida; la bóveda y la duda no cambian nada.
func (h *anfitrionCorreo) aprenderRetirada(owner auth.IdentitySnapshot, almacen AlmacenCorreo) bool {
	switch almacen {
	case CorreoRetirado:
		h.retirada.recordar(owner, true)
		return true
	case CorreoEnMatrix:
		h.retirada.recordar(owner, false)
		return false
	case CorreoEnBoveda, CorreoDudoso:
	}
	return h.retirada.sabe(owner)
}

func (h *anfitrionCorreo) modoTraspaso(owner auth.IdentitySnapshot) modoPuerta {
	if marca := h.traspasos[owner]; marca != nil && marca.vaciada {
		return puertaTraspasoVaciada
	}
	return puertaTraspaso
}

// pedirTraspaso es «Vaciar y pasar a la bóveda», ya confirmado: desde la pantalla
// de elegir o desde la salida de la puerta de Matrix. Matrix se suelta y se abre
// una bóveda nueva con la marca de traspaso.
func (h *anfitrionCorreo) pedirTraspaso() {
	if h.cerrado || h.life == nil || h.ocupado {
		return
	}
	life, owner := h.life, h.owner
	if actual, inside := h.identity(); !inside || actual != owner {
		return
	}
	if h.traspasos[owner] == nil {
		h.traspasos[owner] = &marcaTraspaso{}
	}
	delete(h.seguir, owner)
	enMatrix := h.va(correoMatrix)
	h.decidirDestino(correoTraspaso, owner)
	if enMatrix {
		h.matrix.Retire()
	}
	h.abrirPuerta(life, owner, h.modoTraspaso(owner))
}

// seguirConMatrix es «Seguir con el correo anterior por ahora»: en esta ejecución
// no se vuelve a proponer pasar a la bóveda.
func (h *anfitrionCorreo) seguirConMatrix() {
	if h.cerrado || h.life == nil || h.ocupado || !h.va(correoElegir) {
		return
	}
	h.seguir[h.owner] = true
	h.irAMatrix(h.owner)
}

func (h *anfitrionCorreo) irAMatrix(owner auth.IdentitySnapshot) {
	h.decidirDestino(correoMatrix, owner)
	h.enseñarMatrix()
	h.matrix.Retry()
}

// estadoMatrix sigue a la app de Matrix: con el correo abierto, la salida del
// traspaso desaparece.
func (h *anfitrionCorreo) estadoMatrix(state ApplicationState) {
	h.matrixLista = state == ApplicationReady
	if h.va(correoMatrix) {
		h.enseñarMatrix()
	}
}

// enseñarMatrix enseña la app de Matrix y, mientras no abre, la salida «¿No
// puedes abrir tu correo?».
func (h *anfitrionCorreo) enseñarMatrix() {
	if h.matrixLista {
		h.enseñar(h.matrix.Objeto())
		return
	}
	if h.oferta == nil {
		h.oferta = h.vistaOferta(h.pedirTraspaso)
	}
	h.enseñar(container.NewBorder(nil, h.oferta, nil, nil, h.matrix.Objeto()))
}

func (h *anfitrionCorreo) abrirPuerta(life context.Context, owner auth.IdentitySnapshot, modo modoPuerta) {
	h.soltarBoveda()
	var vigente puertaDrive
	puerta, err := h.puerta(modo, func(ctx context.Context, vault *pqvault.Vault) {
		h.bovedaLista(life, owner, vigente, ctx, vault)
	})
	if err != nil {
		h.fallar(err) //nolint:contextcheck // Soltar la bóveda vive más que esta decisión: espera a la vista y cierra sin cancelar.
		return
	}
	vigente = puerta
	h.vigente, h.modo = vigente, modo
	h.enseñar(puerta.Objeto())
	puerta.Iniciar(life)
}

// bovedaLista recibe la bóveda abierta (hilo de la interfaz). Si su clave de
// firma ya es la activa en Sytue ID, abre el correo; si Sytue ID pide
// identificarse otra vez, la guarda y lo pide.
//
//nolint:revive // ctx no va primero: es el de la puerta, distinto de la vida de la cuenta.
func (h *anfitrionCorreo) bovedaLista(life context.Context, owner auth.IdentitySnapshot, puerta puertaDrive, ctx context.Context, vault *pqvault.Vault) {
	if !h.actual(life, owner) || puerta == nil || h.vigente != puerta || h.repo != nil || h.ocupado || h.pendiente != nil {
		vault.Close()
		return
	}
	if h.modo.traspaso() {
		// La bóveda nueva ya está abierta: desde aquí no se vacía otra vez.
		if marca := h.traspasos[owner]; marca != nil {
			marca.vaciada = true
		}
	}
	switch vault.SigningStatus() {
	case pqvault.SigningRegistered:
		h.abrirCorreo(life, owner, ctx, vault)
	case pqvault.SigningReauth:
		h.pendiente = vault
		// Sin pedir que pulse nada: se va a identificarse otra vez y se registra la
		// clave (una vez; si no sale, la vista con sus botones).
		if !h.reidentificoSolo[owner] {
			if h.reidentificoSolo == nil {
				h.reidentificoSolo = map[auth.IdentitySnapshot]bool{}
			}
			h.reidentificoSolo[owner] = true
			h.registrarFirma(true) //nolint:contextcheck // Registrar la firma vive con la cuenta (h.life), no con la apertura de la puerta.
			return
		}
		h.pedirIdentificacion()
	case pqvault.SigningMissing, pqvault.SigningUnavailable:
		vault.Close()
		h.fallar(pqvault.ErrSigningKey) //nolint:contextcheck // Soltar la bóveda vive más que esta entrega.
	}
}

// pedirIdentificacion enseña la vista que vuelve a entrar en Sytue ID. Sus botones
// viven con la cuenta (h.life), no con la entrega que la pidió.
//
//nolint:contextcheck // Ver arriba.
func (h *anfitrionCorreo) pedirIdentificacion() {
	h.enseñar(nuevaVistaReidentificar(h.translate, func() { h.registrarFirma(true) }, func() { h.registrarFirma(false) }).raiz)
}

// registrarFirma vuelve a entrar en Sytue ID (si `entrar`) y registra la clave de
// firma de la bóveda pendiente, con la marca de traspaso si la bóveda es de uno.
// La bóveda no se vuelve a abrir ni a vaciar.
func (h *anfitrionCorreo) registrarFirma(entrar bool) {
	vault := h.pendiente
	if h.cerrado || h.life == nil || h.ocupado || vault == nil {
		return
	}
	life, owner, handover := h.life, h.owner, h.modo.traspaso()
	h.ocupado = true
	h.enseñarCargando()
	go func() {
		var err error
		if entrar {
			err = h.reidentificar(life)
		}
		ctx := life
		if err == nil {
			// Volver a entrar renueva la sesión: la intención es otra.
			ctx, err = h.intencion(life, owner)
		}
		status := pqvault.SigningUnavailable
		if err == nil {
			status = h.registrar(ctx, owner, vault, handover)
		}
		h.dispatch(func() { h.firmaRegistrada(life, owner, ctx, vault, status, err) })
	}()
}

//nolint:revive // ctx no va primero: es el de la intención renovada, distinto de la vida de la cuenta.
func (h *anfitrionCorreo) firmaRegistrada(life context.Context, owner auth.IdentitySnapshot, ctx context.Context, vault *pqvault.Vault, status pqvault.SigningStatus, err error) {
	if !h.actual(life, owner) || h.pendiente != vault {
		return
	}
	h.ocupado = false
	switch {
	case err != nil, status == pqvault.SigningReauth:
		// No volvió a entrar (canceló, sin red) o Sytue ID aún no la ve reciente:
		// se sigue pidiendo, con la misma bóveda. Otra cuenta la suelta adoptar.
		h.pedirIdentificacion()
	case status == pqvault.SigningRegistered:
		h.pendiente = nil
		h.abrirCorreo(life, owner, ctx, vault)
	default:
		h.pendiente = nil
		vault.Close()
		h.fallar(pqvault.ErrSigningKey) //nolint:contextcheck // Soltar la bóveda vive más que esta entrega.
	}
}

// abrirCorreo abre los recursos y el correo sobre la bóveda y lo prepara fuera del
// hilo de la interfaz.
//
//nolint:revive // ctx no va primero: es el de la entrega, distinto de la vida de la cuenta.
func (h *anfitrionCorreo) abrirCorreo(life context.Context, owner auth.IdentitySnapshot, ctx context.Context, vault *pqvault.Vault) {
	puerta := h.vigente
	h.ocupado = true
	h.enseñarCargando()
	go func() {
		recursos, repo, err := h.abrir(ctx, vault)
		if err == nil {
			// El correo vive lo que la cuenta (recursos.Context), no lo que la entrega.
			err = repo.Initialize(recursos.Context()) //nolint:contextcheck // ver arriba.
		}
		h.dispatch(func() { h.inicializado(life, owner, puerta, recursos, repo, err) })
	}()
}

func (h *anfitrionCorreo) inicializado(life context.Context, owner auth.IdentitySnapshot, puerta puertaDrive, recursos recursosCorreo, repo repositorioCorreo, err error) {
	vigente := h.actual(life, owner) && h.vigente == puerta
	if vigente {
		h.ocupado = false
	}
	if err == nil && vigente {
		var view ApplicationView
		view, err = h.montar(recursos.Context(), recursos, repo.Application()) //nolint:contextcheck // La vista vive lo que la cuenta.
		if err == nil {
			h.recursos, h.repo, h.vista = recursos, repo, &view
			// El correo abrió en la bóveda: el traspaso, si lo había, está hecho.
			delete(h.traspasos, owner)
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
		h.fallar(err) //nolint:contextcheck // Soltar la bóveda vive más que esta decisión.
	}
}

func (h *anfitrionCorreo) fallar(err error) {
	h.decidirDestino(correoSinDecidir, auth.IdentitySnapshot{})
	h.soltarBoveda()
	vacio := ui.NuevoVacio(h.translate("matrix.failed"), "")
	vacio.Tono = ui.VacioError // DS-06: es un fallo, no una lista vacía
	vacio.Accion = ui.NuevoBoton(h.translate("action.retry"), h.reintentar)
	h.enseñar(vacio)
	if h.estado != nil {
		h.estado(ApplicationFailed, err)
	}
}

// soltarBoveda cierra la puerta y la bóveda pendiente, desmonta la vista y retira
// correo y bóveda. Lo pendiente queda en el diario de la bóveda.
//
//nolint:contextcheck // La retirada vive más que quien la pide: espera a la vista ya cancelada y cierra sin cancelar.
func (h *anfitrionCorreo) soltarBoveda() {
	if h.vigente != nil {
		h.vigente.Cerrar()
		h.vigente = nil
	}
	if h.pendiente != nil {
		h.pendiente.Close()
		h.pendiente = nil
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
		if recursos != nil {
			recursos.Close()
		}
	}()
}

func (h *anfitrionCorreo) soltar() {
	if h.stop != nil {
		h.stop()
	}
	h.life, h.stop, h.ocupado = nil, nil, false
	h.matrixLista, h.oferta = false, nil
	h.decidirDestino(correoSinDecidir, auth.IdentitySnapshot{})
	h.soltarBoveda()
}

func (h *anfitrionCorreo) cerrar() {
	h.soltar()
	h.mu.Lock()
	h.cerrado = true
	h.mu.Unlock()
}

func (h *anfitrionCorreo) enseñarCargando() {
	h.enseñar(container.NewBorder(nil, widget.NewProgressBarInfinite(), nil, nil, ui.NuevoVacio(h.translate("state.loading"), "")))
}

func (h *anfitrionCorreo) enseñar(object fyne.CanvasObject) {
	h.slot.Objects = []fyne.CanvasObject{object}
	h.slot.Refresh()
}
