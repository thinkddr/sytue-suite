// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// La puerta nativa de la bóveda post-cuántica directa (ADR 0086/0087): la de
// `PqVaultGate.tsx` en Fyne. Abre la bóveda con la clave de desbloqueo de Sytue ID
// que la app ya tiene en el dispositivo (la misma para todas las apps de la suite:
// un inicio de sesión desbloquea todas) y, si no abre, nunca deja a la persona
// atrapada: volver a entrar en Sytue ID, o vaciar los datos cifrados con
// confirmación y empezar con la bóveda nueva.

import (
	"context"
	"errors"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// EstadoBoveda es lo que enseña la puerta.
type EstadoBoveda uint8

const (
	// BovedaAbriendo: abriendo, creando o vaciando.
	BovedaAbriendo EstadoBoveda = iota
	// BovedaEntrar: este dispositivo no tiene la clave de desbloqueo.
	BovedaEntrar
	// BovedaNoAbre: la clave no abre la bóveda y la bóveda guarda datos.
	BovedaNoAbre
	// BovedaFallo: cualquier otro fallo al abrir (red, servidor, envoltorio roto).
	BovedaFallo
	// BovedaLista: abierta y entregada a la app.
	BovedaLista
	// BovedaCerrada: la puerta se cerró o la cuenta cambió.
	BovedaCerrada
)

// PuertaBovedaConfig une la puerta a la sesión y a la app.
type PuertaBovedaConfig struct {
	Scope    storage.Scope
	Server   pqvault.Server
	Registry pqvault.Registry
	// Clave devuelve una copia de la clave de desbloqueo de la sesión, o nil.
	Clave func() []byte
	// Entrar vuelve a entrar en Sytue ID para traer la clave a este dispositivo.
	Entrar func(context.Context) error
	// Lista recibe la bóveda abierta, en el hilo de la interfaz. Desde entonces es
	// de la app: la cierra ella.
	Lista func(context.Context, *pqvault.Vault)
	// Intencion ata cada operación a la cuenta de la puerta: devuelve el contexto
	// con una intención de autorización recién capturada, o ErrAuthorizationChanged
	// si la sesión ya es de otra persona. Se captura otra vez tras volver a entrar,
	// que renueva la sesión. Nil: el contexto tal cual (pruebas).
	Intencion func(context.Context) (context.Context, error)
	// Translate traduce una clave de los catálogos comunes.
	Translate func(string) string
	// Dispatch lleva al hilo de la interfaz (fyne.Do si nil).
	Dispatch func(func())
	// AntesDeVaciar, si está, corre antes de vaciar la bóveda (fuera del hilo de la
	// interfaz): Drive suelta ahí su fuente congelada, para que la bóveda nueva tenga
	// de dónde importar. Si falla, no se vacía nada.
	AntesDeVaciar func(context.Context) error
	// abrir y vaciar se sustituyen en pruebas.
	abrir, vaciar func(context.Context, pqvault.Config) (*pqvault.Vault, error)
}

// PuertaBoveda es la puerta. Sus métodos se llaman desde el hilo de la interfaz.
type PuertaBoveda struct {
	cfg  PuertaBovedaConfig
	life context.Context
	stop context.CancelFunc

	mu          sync.Mutex
	estado      EstadoBoveda
	ocupada     bool
	confirmando bool
	cerrada     bool
	// vaciarAlEntrar: un «vaciar» confirmado pidió iniciar sesión antes de tocar la
	// bóveda (sin la clave de desbloqueo no se puede sellar la nueva). Al volver a
	// entrar se termina de vaciar en vez de abrir la bóveda de antes, que volvería a
	// fallar. Nunca se pone si la bóveda ya se vació.
	vaciarAlEntrar bool
	vista          *vistaBoveda
}

// NuevaPuertaBoveda crea la puerta; Iniciar la abre.
func NuevaPuertaBoveda(cfg PuertaBovedaConfig) (*PuertaBoveda, error) {
	if !cfg.Scope.Valid() || cfg.Server == nil || cfg.Clave == nil || cfg.Lista == nil || cfg.Translate == nil {
		return nil, ErrPrivateResources
	}
	if cfg.Dispatch == nil {
		cfg.Dispatch = fyne.Do
	}
	if cfg.abrir == nil {
		cfg.abrir = pqvault.Open
	}
	if cfg.vaciar == nil {
		cfg.vaciar = pqvault.Reset
	}
	p := &PuertaBoveda{cfg: cfg}
	p.vista = nuevaVistaBoveda(p)
	return p, nil
}

// NuevaPuertaBovedaDeSesion es la puerta de un producto con la sesión de la suite:
// la clave de desbloqueo que comparten las apps del dispositivo, los clientes de
// Drive y Sytue ID con el token de la sesión, y volver a entrar en Sytue ID.
func NuevaPuertaBovedaDeSesion(suite *sesion.Suite, product string, translate func(string) string, lista func(context.Context, *pqvault.Vault)) (*PuertaBoveda, error) {
	if suite == nil || suite.Sesion == nil || !storage.Product(product) {
		return nil, ErrPrivateResources
	}
	owner := suite.Sesion.SnapshotIdentity()
	return NuevaPuertaBoveda(PuertaBovedaConfig{
		Scope:    storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: product},
		Server:   api.NewDrive(suite.URL("drive"), sesionBovedaDeCuenta{session: suite.Sesion, owner: owner}, nil),
		Registry: api.NewAccount(suite.URL("id"), sesionBovedaDeCuenta{session: suite.Sesion, owner: owner}, nil),
		Clave: func() []byte {
			clave, _ := suite.Sesion.Desbloqueo()
			return clave
		},
		Entrar: func(ctx context.Context) error { return suite.EntrarParaDesbloquear(ctx, "") },
		Intencion: func(ctx context.Context) (context.Context, error) {
			ctx = suite.Sesion.WithAuthorizationIntent(ctx)
			if auth.RequestAuthorizationIntent(ctx).Identity() != owner {
				return nil, auth.ErrAuthorizationChanged
			}
			return ctx, nil
		},
		Lista:     lista,
		Translate: translate,
	})
}

// sesionBovedaDeCuenta es la sesión de la suite atada a la cuenta de la bóveda.
// La bóveda se queda con su servidor mientras la app la usa, también para
// peticiones sin intención de autorización en el contexto (las del repositorio):
// cada una captura la suya y, si la sesión ya es de otra persona, falla en vez de
// llevar el token de la cuenta nueva a la bóveda de la anterior.
type sesionBovedaDeCuenta struct {
	session *auth.Session
	owner   auth.IdentitySnapshot
}

func (s sesionBovedaDeCuenta) intencion(ctx context.Context) (context.Context, error) {
	if auth.RequestAuthorizationIntent(ctx) == nil {
		ctx = s.session.WithAuthorizationIntent(ctx)
	}
	if auth.RequestAuthorizationIntent(ctx).Identity() != s.owner {
		return nil, auth.ErrAuthorizationChanged
	}
	return ctx, nil
}

func (s sesionBovedaDeCuenta) AccessToken(ctx context.Context) (string, error) {
	token, _, err := s.AccessTokenScoped(ctx)
	return token, err
}

func (s sesionBovedaDeCuenta) AccessTokenScoped(ctx context.Context) (string, *auth.AuthorizationScope, error) {
	ctx, err := s.intencion(ctx)
	if err != nil {
		return "", nil, err
	}
	return s.session.AccessTokenScoped(ctx)
}

func (s sesionBovedaDeCuenta) Invalidate(rechazado string) {
	if s.session.SnapshotIdentity() == s.owner {
		s.session.Invalidate(rechazado)
	}
}

// Objeto es la vista de la puerta.
func (p *PuertaBoveda) Objeto() fyne.CanvasObject { return p.vista.raiz }

// Estado es lo que enseña ahora.
func (p *PuertaBoveda) Estado() EstadoBoveda {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.estado
}

// Iniciar abre la bóveda en el contexto de la presentación (con su intención de
// autorización: si la cuenta cambia, las peticiones fallan en vez de mezclarse).
func (p *PuertaBoveda) Iniciar(ctx context.Context) {
	p.mu.Lock()
	if p.life != nil || p.cerrada {
		p.mu.Unlock()
		return
	}
	p.life, p.stop = context.WithCancel(ctx) //nolint:gosec // G118: Cerrar llama a stop.
	p.mu.Unlock()
	p.abrir(false) //nolint:contextcheck // abrir usa p.life, que deriva de ctx y vive lo que la puerta.
}

// Reintentar vuelve a abrir con la clave que haya ahora. Abrir es lo contrario de
// vaciar: un vaciado pendiente de entrar se olvida, y habrá que confirmarlo otra vez.
func (p *PuertaBoveda) Reintentar() {
	p.mu.Lock()
	p.vaciarAlEntrar = false
	p.mu.Unlock()
	p.abrir(false)
}

// VolverAEntrar trae la clave de desbloqueo de Sytue ID y vuelve a abrir (o, si un
// «vaciar» confirmado se quedó esperando la clave, termina de vaciar).
func (p *PuertaBoveda) VolverAEntrar() {
	p.mu.Lock()
	vaciar := p.vaciarAlEntrar
	p.vaciarAlEntrar = false
	p.mu.Unlock()
	open := p.cfg.abrir
	if vaciar {
		open = p.vaciarCon
	}
	p.correr(func(life context.Context) (context.Context, *pqvault.Vault, error) {
		ctx, vault, err := life, (*pqvault.Vault)(nil), error(nil)
		if p.cfg.Entrar != nil {
			err = p.cfg.Entrar(life)
		}
		entro := err == nil
		if entro {
			ctx, vault, err = p.abrirCon(life, open)
		}
		// Si no se pudo entrar, o sigue sin clave, el vaciado pendiente sigue
		// pendiente: la bóveda no se tocó.
		var antes errAntesDeVaciar
		if vaciar && (!entro || errors.Is(err, pqvault.ErrSignin) && !errors.As(err, &antes)) {
			p.mu.Lock()
			p.vaciarAlEntrar = true
			p.mu.Unlock()
		}
		return ctx, vault, err
	})
}

// IniciarVaciando arranca la puerta vaciando la bóveda: la app ya pidió y obtuvo la
// confirmación (la bóveda abría, pero lo que guarda no se pudo cargar).
func (p *PuertaBoveda) IniciarVaciando(ctx context.Context) {
	p.mu.Lock()
	if p.life != nil || p.cerrada {
		p.mu.Unlock()
		return
	}
	p.life, p.stop = context.WithCancel(ctx) //nolint:gosec // G118: Cerrar llama a stop.
	p.mu.Unlock()
	p.abrir(true) //nolint:contextcheck // abrir usa p.life, que deriva de ctx y vive lo que la puerta.
}

// vaciarCon es `vaciar` precedido de AntesDeVaciar.
func (p *PuertaBoveda) vaciarCon(ctx context.Context, cfg pqvault.Config) (*pqvault.Vault, error) {
	// Sin la clave no se sella la bóveda nueva: no se suelta nada hasta tenerla.
	if len(cfg.UnlockKey) != recoverywrap.UnlockKeyLen {
		return nil, pqvault.ErrSignin
	}
	if p.cfg.AntesDeVaciar != nil {
		if err := p.cfg.AntesDeVaciar(ctx); err != nil {
			return nil, errAntesDeVaciar{err}
		}
	}
	return p.cfg.vaciar(ctx, cfg)
}

// errAntesDeVaciar envuelve un fallo previo a vaciar: la bóveda sigue como estaba.
type errAntesDeVaciar struct{ err error }

func (e errAntesDeVaciar) Error() string { return e.err.Error() }
func (e errAntesDeVaciar) Unwrap() error { return e.err }

// PedirVaciado enseña la confirmación de vaciar.
func (p *PuertaBoveda) PedirVaciado() {
	p.mu.Lock()
	p.confirmando = true
	p.mu.Unlock()
	p.vista.pintar()
}

// CancelarVaciado la esconde.
func (p *PuertaBoveda) CancelarVaciado() {
	p.mu.Lock()
	p.confirmando = false
	p.mu.Unlock()
	p.vista.pintar()
}

// Vaciar borra los datos cifrados de la bóveda y la estrena. Solo con la casilla
// de confirmación marcada: sin ella no hace nada.
func (p *PuertaBoveda) Vaciar(confirmado bool) {
	p.mu.Lock()
	permitido := confirmado && p.confirmando && (p.estado == BovedaNoAbre || p.estado == BovedaFallo)
	p.mu.Unlock()
	if !permitido {
		return
	}
	p.abrir(true)
}

// Cerrar cancela lo que esté en marcha. Una bóveda ya entregada es de la app.
func (p *PuertaBoveda) Cerrar() {
	p.mu.Lock()
	p.cerrada = true
	p.estado = BovedaCerrada
	stop := p.stop
	p.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (p *PuertaBoveda) abrir(reset bool) {
	open := p.cfg.abrir
	if reset {
		open = p.vaciarCon
	}
	p.correr(func(life context.Context) (context.Context, *pqvault.Vault, error) {
		ctx, vault, err := p.abrirCon(life, open)
		// Sin clave de desbloqueo, vaciar no llegó a tocar la bóveda: al volver a
		// entrar se termina. Un fallo de AntesDeVaciar no se arrastra.
		var antes errAntesDeVaciar
		if reset && errors.Is(err, pqvault.ErrSignin) && !errors.As(err, &antes) {
			p.mu.Lock()
			p.vaciarAlEntrar = true
			p.mu.Unlock()
		}
		return ctx, vault, err
	})
}

// abrirCon abre con la clave de la sesión, atado a su cuenta. Devuelve también el
// contexto de esa apertura, que es el que recibe la app con la bóveda.
func (p *PuertaBoveda) abrirCon(life context.Context, open func(context.Context, pqvault.Config) (*pqvault.Vault, error)) (context.Context, *pqvault.Vault, error) {
	ctx := life
	if p.cfg.Intencion != nil {
		var err error
		if ctx, err = p.cfg.Intencion(life); err != nil {
			return life, nil, err
		}
	}
	clave := p.cfg.Clave()
	defer clear(clave)
	vault, err := open(ctx, pqvault.Config{Scope: p.cfg.Scope, Server: p.cfg.Server, Registry: p.cfg.Registry, UnlockKey: clave})
	return ctx, vault, err
}

// correr hace una operación fuera del hilo de la interfaz, de una en una.
func (p *PuertaBoveda) correr(op func(context.Context) (context.Context, *pqvault.Vault, error)) {
	p.mu.Lock()
	if p.life == nil || p.cerrada || p.ocupada || p.estado == BovedaLista {
		p.mu.Unlock()
		return
	}
	p.ocupada, p.estado = true, BovedaAbriendo
	life := p.life
	p.mu.Unlock()
	p.vista.pintar()
	go func() {
		ctx, vault, err := op(life)
		p.cfg.Dispatch(func() { p.terminar(life, ctx, vault, err) })
	}()
}

func (p *PuertaBoveda) terminar(life, ctx context.Context, vault *pqvault.Vault, err error) {
	p.mu.Lock()
	p.ocupada = false
	if p.cerrada || life.Err() != nil || err == nil && ctx.Err() != nil {
		p.mu.Unlock()
		if vault != nil {
			vault.Close()
		}
		return
	}
	switch {
	case err == nil:
		p.estado, p.confirmando = BovedaLista, false
	case errors.Is(err, pqvault.ErrSignin):
		p.estado = BovedaEntrar
	case errors.Is(err, pqvault.ErrWrong):
		p.estado = BovedaNoAbre
	case errors.Is(err, auth.ErrAuthorizationChanged):
		// Otra cuenta: esta puerta ya no es de nadie.
		p.estado, p.cerrada = BovedaCerrada, true
	default:
		p.estado = BovedaFallo
	}
	estado := p.estado
	p.mu.Unlock()
	p.vista.pintar()
	if estado == BovedaLista {
		p.cfg.Lista(ctx, vault)
	}
}

// vistaBoveda pinta la puerta: un texto y los botones que tocan en cada estado,
// y la confirmación de vaciar con su casilla.
type vistaBoveda struct {
	p                          *PuertaBoveda
	raiz                       *fyne.Container
	mensaje                    *widget.Label
	progreso                   *widget.ProgressBarInfinite
	entrar, otraVez, reintenta *widget.Button
	vaciar                     *widget.Button
	confirmar                  *fyne.Container
	entiendo                   *widget.Check
	vaciarYa, cancelar         *widget.Button
}

func nuevaVistaBoveda(p *PuertaBoveda) *vistaBoveda {
	t := p.cfg.Translate
	v := &vistaBoveda{p: p}
	titulo := widget.NewLabelWithStyle(t("matrix.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.mensaje = widget.NewLabel("")
	v.mensaje.Wrapping = fyne.TextWrapWord
	v.progreso = widget.NewProgressBarInfinite()
	v.entrar = widget.NewButton(t("matrix.signIn"), p.VolverAEntrar)
	v.entrar.Importance = widget.HighImportance
	v.otraVez = widget.NewButton(t("vault.signInAgain"), p.VolverAEntrar)
	v.otraVez.Importance = widget.HighImportance
	v.reintenta = widget.NewButton(t("action.retry"), p.Reintentar)
	v.vaciar = widget.NewButton(t("vault.empty"), p.PedirVaciado)
	cuerpo := widget.NewLabel(t("vault.emptyBody"))
	cuerpo.Wrapping = fyne.TextWrapWord
	v.vaciarYa = widget.NewButton(t("vault.empty"), func() { p.Vaciar(v.entiendo.Checked) })
	v.vaciarYa.Importance = widget.DangerImportance
	v.vaciarYa.Disable()
	v.entiendo = widget.NewCheck(t("vault.emptyUnderstood"), func(marcada bool) {
		if marcada {
			v.vaciarYa.Enable()
		} else {
			v.vaciarYa.Disable()
		}
	})
	v.cancelar = widget.NewButton(t("action.cancel"), p.CancelarVaciado)
	v.confirmar = container.NewVBox(
		widget.NewLabelWithStyle(t("vault.emptyTitle"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		cuerpo, v.entiendo, container.NewHBox(v.cancelar, v.vaciarYa),
	)
	v.raiz = container.NewVBox(titulo, v.mensaje, v.progreso, container.NewHBox(v.entrar, v.otraVez, v.vaciar, v.reintenta), v.confirmar)
	v.pintar()
	return v
}

func (v *vistaBoveda) pintar() {
	p, t := v.p, v.p.cfg.Translate
	p.mu.Lock()
	estado, confirmando := p.estado, p.confirmando
	p.mu.Unlock()
	mostrar := func(o fyne.CanvasObject, si bool) {
		if si {
			o.Show()
		} else {
			o.Hide()
		}
	}
	switch estado {
	case BovedaAbriendo:
		v.mensaje.SetText(t("matrix.unlocking"))
	case BovedaEntrar:
		v.mensaje.SetText(t("matrix.signInBody"))
	case BovedaNoAbre:
		v.mensaje.SetText(t("vault.wrongKeyDevice"))
	case BovedaFallo:
		v.mensaje.SetText(t("vault.failed"))
	default:
		v.mensaje.SetText("")
	}
	salida := estado == BovedaNoAbre || estado == BovedaFallo
	mostrar(v.progreso, estado == BovedaAbriendo)
	mostrar(v.entrar, estado == BovedaEntrar)
	mostrar(v.otraVez, salida)
	mostrar(v.vaciar, salida && !confirmando)
	mostrar(v.reintenta, salida || estado == BovedaEntrar)
	mostrar(v.confirmar, salida && confirmando)
	if !confirmando {
		v.entiendo.SetChecked(false)
	}
}
