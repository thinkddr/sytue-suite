// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// El asistente externo reutiliza el SDK. Ni los servidores ni las credenciales
// se deducen de una instalación vieja de Suite o de una URL de la forja.
type controlClientesExternos struct {
	v            *Vista
	pantalla     *ClientesExternos
	coordenadas  func(context.Context) (api.Coordenadas, error)
	crear        func(context.Context, Plataforma) (string, error)
	abrir        func(Plataforma) error
	montar       func(context.Context, string) error
	perfilNativo func(context.Context) (func() error, error)
	perfil       func(context.Context, string) (string, error)
	identidad    func() auth.IdentitySnapshot
	mostrada     auth.IdentitySnapshot
	preparar     func(context.Context) context.Context
	contexto     context.Context
	cancelarVida context.CancelFunc

	revision uint64
	cancelar context.CancelFunc
}

func nuevoControlClientesExternos(v *Vista) *controlClientesExternos {
	c := &controlClientesExternos{v: v, pantalla: NuevosClientesExternos(v.t)}
	c.pantalla.AlCopiar = func(valor, aviso string) {
		if c.vigente(c.revision) {
			v.copiar(valor, aviso)
		}
	}
	c.pantalla.AlAbrirURL = func(url string) {
		if c.vigente(c.revision) {
			v.AbrirURL(url)
		}
	}
	c.pantalla.AlReintentar = v.RefrescarClientesExternos
	c.pantalla.AlCambiarPaso = c.cancelarOperacion
	return c
}

// ConAutorizacionClientesExternos captura una intención al mostrar la cuenta, antes
// de solicitar tokens. Los clics posteriores heredan esa intención, no la recapturan.
func (v *Vista) ConAutorizacionClientesExternos(identidad func() auth.IdentitySnapshot,
	preparar func(context.Context) context.Context,
) *Vista {
	v.controlClientes.identidad, v.controlClientes.preparar = identidad, preparar
	return v
}

func (c *controlClientesExternos) cancelarOperacion() {
	if c.cancelar != nil {
		c.cancelar()
		c.cancelar = nil
	}
	c.revision++
}

func (c *controlClientesExternos) limpiar() {
	c.cancelarOperacion()
	if c.cancelarVida != nil {
		c.cancelarVida()
		c.cancelarVida = nil
	}
	c.contexto = nil
	c.pantalla.Cliente = Cliente{}
	c.pantalla.Montando, c.pantalla.FalloMontaje = false, false
	c.pantalla.Correo, c.pantalla.Plataforma = "", ""
	c.pantalla.Coordenadas = api.Coordenadas{}
	c.pantalla.Estado = Cargando
	c.pantalla.Ir(0)
}

func (c *controlClientesExternos) autorizacionVigente() bool {
	if c.contexto == nil || c.contexto.Err() != nil {
		return false
	}
	intent := auth.RequestAuthorizationIntent(c.contexto)
	return intent == nil || intent.Current()
}

func (c *controlClientesExternos) vigente(revision uint64) bool {
	return c.v.destino == AClientesExternos && c.revision == revision && c.autorizacionVigente() &&
		(c.identidad == nil || c.identidad() == c.mostrada)
}

// RefrescarClientesExternos vuelve a pedir datos por entrada o reintento explícito.
func (v *Vista) RefrescarClientesExternos() {
	v.enUI(func() { v.controlClientes.cargar(context.Background()) })
}

// RefrescarSesionClientesExternos filtra eventos de apariencia y presencia. Una
// adopción, cierre o cambio de servidor sí cancela y limpia la intención anterior.
func (v *Vista) RefrescarSesionClientesExternos() {
	v.enUI(func() {
		c := v.controlClientes
		if c.identidad == nil || c.identidad() == c.mostrada &&
			(v.destino != AClientesExternos || c.autorizacionVigente()) {
			return
		}
		c.cargar(context.Background())
	})
}

func (c *controlClientesExternos) cargar(padre context.Context) {
	v := c.v
	c.limpiar()
	if c.identidad != nil {
		c.mostrada = c.identidad()
	}
	if v.destino != AClientesExternos {
		return
	}
	if c.coordenadas == nil || v.cuenta == nil {
		c.pantalla.Estado = Fallo
		return
	}
	// Una recarga es una nueva intención y vida de pantalla. Conserva los valores
	// del llamador, sin heredar el vencimiento de la operación ya terminada.
	vida := context.WithoutCancel(padre)
	if c.preparar != nil {
		vida = c.preparar(vida)
	}
	vida, c.cancelarVida = context.WithCancel(vida)
	c.contexto = vida
	revision, identidad := c.revision, c.mostrada
	ctx, cancelar := context.WithTimeout(vida, 15*time.Second)
	c.cancelar = cancelar
	go func() {
		defer cancelar()
		cuenta, err := v.cuenta.Reclamos(ctx)
		var datos api.Coordenadas
		if err == nil && cuenta.Sub != "" && cuenta.Email != "" {
			datos, err = c.coordenadas(ctx)
		}
		if err == nil && (cuenta.Sub == "" || cuenta.Email == "" || datos.Domain == "") {
			err = errors.New("hub: faltan la identidad o las coordenadas del cliente")
		}
		if err == nil && c.identidad != nil && cuenta.Sub != identidad.Subject {
			err = errors.New("hub: la identidad ha cambiado durante la carga")
		}
		if err == nil {
			err = ctx.Err()
		}
		v.enUI(func() { c.recibirCarga(revision, cuenta.Email, datos, err) })
	}()
}

func (c *controlClientesExternos) recibirCarga(revision uint64, correo string, datos api.Coordenadas, err error) {
	if c.revision != revision || c.v.destino != AClientesExternos {
		return
	}
	if !c.vigente(revision) {
		c.pantalla.Estado = Fallo
		return
	}
	c.pantalla.Estado = EstadoDe(err)
	if err == nil {
		c.pantalla.Coordenadas, c.pantalla.Correo, c.pantalla.Apex = datos, correo, datos.Domain
	}
}

// iniciar no reproduce una intención invalidada: recarga y exige otro clic.
func (c *controlClientesExternos) iniciar(p Plataforma) (context.Context, uint64, uint64, bool) {
	w := c.pantalla
	if w.Estado != Listo || w.Paso != 1 || w.Plataforma != p {
		return nil, 0, 0, false
	}
	if !c.vigente(c.revision) {
		c.cargar(context.Background())
		return nil, 0, 0, false
	}
	c.cancelarOperacion()
	ctx, cancelar := context.WithTimeout(c.contexto, 30*time.Second)
	c.cancelar = cancelar
	return ctx, c.revision, w.revision, true
}

func (c *controlClientesExternos) resultadoVigente(ctx context.Context, revision, paso uint64, p Plataforma) bool {
	if c.v.destino != AClientesExternos || c.revision != revision || paso != c.pantalla.revision ||
		c.pantalla.Paso != 1 || c.pantalla.Plataforma != p {
		return false
	}
	if !c.vigente(revision) {
		// A different call may have renewed the Session. Reload without replaying
		// the write; the person must choose and click again with the new witness.
		c.cargar(ctx)
		return false
	}
	return true
}

func (c *controlClientesExternos) configurar(p Plataforma) {
	w := c.pantalla
	if c.crear == nil || w.Cliente.Creando || (p != Outlook && p != Thunderbird && p != Android) {
		return
	}
	ctx, revision, paso, ok := c.iniciar(p)
	if !ok {
		return
	}
	w.Cliente = Cliente{Plataforma: p, Creando: true}
	w.Refresh()
	cancelar, abrir := c.cancelar, c.abrir
	go func() {
		clave, err := c.crear(ctx, p)
		c.v.enUI(func() {
			if !c.resultadoVigente(ctx, revision, paso, p) {
				cancelar()
				return
			}
			// La copia sigue en UI. La apertura puede esperar al SO y no bloquea
			// esta cola; no se inicia hasta que la clave ya se ha copiado.
			w.Cliente = configurarCliente(ctx, p,
				func(context.Context, Plataforma) (string, error) { return clave, err },
				func(clave string) { c.v.copiar(clave, "devices.copied.password") }, nil)
			if w.Cliente.Fallo || abrir == nil {
				cancelar()
				return
			}
			w.Cliente.Creando = true
			go func() {
				abierto := abrirClienteVigente(ctx, p, abrir)
				c.v.enUI(func() {
					defer cancelar()
					if !c.resultadoVigente(ctx, revision, paso, p) {
						return
					}
					w.Cliente.Creando, w.Cliente.Abierto = false, abierto
				})
			}()
		})
	}()
}

// AbrirPrograma puede bloquear hasta que responda el SO. Este paso sólo usa
// contexto e intención capturados, sin leer widgets desde la gorutina.
func abrirClienteVigente(ctx context.Context, p Plataforma, abrir func(Plataforma) error) bool {
	if ctx.Err() != nil {
		return false
	}
	if i := auth.RequestAuthorizationIntent(ctx); i != nil && !i.Current() {
		return false
	}
	if abrir == nil || abrir(p) != nil {
		return false
	}
	return ctx.Err() == nil
}

func (c *controlClientesExternos) perfilApple(p Plataforma) {
	w := c.pantalla
	if w.PerfilCreando || (p != IPhone && p != Mac) {
		return
	}
	nativo := p == w.EsteApple && c.perfilNativo != nil
	if !nativo && c.perfil == nil {
		return
	}
	ctx, revision, paso, ok := c.iniciar(p)
	if !ok {
		return
	}
	w.PerfilCreando, w.PerfilFallo = true, false
	w.Refresh()
	cancelar := c.cancelar
	go func() {
		var aplicar func() error
		var err error
		if nativo {
			aplicar, err = c.perfilNativo(ctx)
		} else {
			var enlace string
			enlace, err = c.perfil(ctx, "Apple · "+string(p))
			if err == nil {
				u, fallo := url.Parse(enlace)
				if fallo != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
					err = errors.New("hub: el enlace de perfil no es HTTPS")
				}
			}
			aplicar = func() error {
				if err := ctx.Err(); err != nil {
					return err
				}
				c.v.AbrirURL(enlace)
				return nil
			}
		}
		c.v.enUI(func() {
			defer cancelar()
			if !c.resultadoVigente(ctx, revision, paso, p) {
				return
			}
			if err == nil {
				err = ctx.Err()
			}
			if err == nil && aplicar != nil {
				err = aplicar()
			}
			w.PerfilCreando, w.PerfilFallo = false, err != nil || aplicar == nil
		})
	}()
}

func (c *controlClientesExternos) montarDrive(direccion string) {
	w := c.pantalla
	if c.montar == nil || w.Montando || direccion == "" || direccion != w.Coordenadas.WebDAVURL {
		return
	}
	ctx, revision, paso, ok := c.iniciar(UnidadDrive)
	if !ok {
		return
	}
	w.Montando, w.FalloMontaje = true, false
	w.Refresh()
	cancelar := c.cancelar
	go func() {
		err := c.montar(ctx, direccion)
		c.v.enUI(func() {
			defer cancelar()
			if !c.resultadoVigente(ctx, revision, paso, UnidadDrive) {
				return
			}
			if err == nil {
				err = ctx.Err()
			}
			w.Montando, w.FalloMontaje = false, err != nil
		})
	}()
}

// ConCoordenadasClientesExternos lee las direcciones del despliegue mediante el SDK.
func (v *Vista) ConCoordenadasClientesExternos(f func(context.Context) (api.Coordenadas, error)) *Vista {
	v.controlClientes.coordenadas = f
	return v
}

// ConMontaje da al asistente externo el botón de conectar Drive como disco en este equipo. Solo
// lo pide el escritorio (`escritorio.MontarWebDAV`); sin él, quedan los pasos a mano.
func (v *Vista) ConMontaje(montar func(context.Context, string) error) *Vista {
	v.controlClientes.montar = montar
	v.controlClientes.pantalla.AlMontar = v.controlClientes.montarDrive
	return v
}

// ConPerfilApple prepara la descarga fuera de UI y aplica la instalación sólo
// si la intención y la cuenta que la pidió siguen visibles al recibirla.
func (v *Vista) ConPerfilApple(este Plataforma, preparar func(context.Context) (func() error, error)) *Vista {
	v.controlClientes.perfilNativo = preparar
	v.controlClientes.pantalla.EsteApple = este
	v.controlClientes.pantalla.AlConfigurarApple = func() { v.controlClientes.perfilApple(este) }
	return v
}

// ConEnlacePerfilApple cubre otros dispositivos Apple mediante un enlace de un
// uso creado por POST autenticado. Nunca se abre la antigua URL GET pública.
func (v *Vista) ConEnlacePerfilApple(crear func(context.Context, string) (string, error)) *Vista {
	v.controlClientes.perfil = crear
	v.controlClientes.pantalla.AlEnlacePerfilApple = v.controlClientes.perfilApple
	return v
}

// ConClientes conserva la creación, copia y apertura del programa, con intención vigente.
func (v *Vista) ConClientes(crear func(context.Context, Plataforma) (string, error), abrir func(Plataforma) error) *Vista {
	v.controlClientes.crear, v.controlClientes.abrir = crear, abrir
	v.controlClientes.pantalla.AlConfigurarCliente = v.controlClientes.configurar
	return v
}

// configurarCliente aplica en orden crear, copiar y abrir; nunca copia tras cancelación.
func configurarCliente(ctx context.Context, p Plataforma, crear func(context.Context, Plataforma) (string, error),
	copiar func(string), abrir func(Plataforma) error,
) Cliente {
	if err := ctx.Err(); err != nil {
		return Cliente{Plataforma: p, Fallo: true}
	}
	if i := auth.RequestAuthorizationIntent(ctx); i != nil && !i.Current() {
		return Cliente{Plataforma: p, Fallo: true}
	}
	clave, err := crear(ctx, p)
	if err != nil || clave == "" {
		return Cliente{Plataforma: p, Fallo: true}
	}
	if ctx.Err() != nil {
		return Cliente{Plataforma: p, Fallo: true}
	}
	if i := auth.RequestAuthorizationIntent(ctx); i != nil && !i.Current() {
		return Cliente{Plataforma: p, Fallo: true}
	}
	copiar(clave)
	return Cliente{Plataforma: p, Clave: clave, Abierto: abrir != nil && abrir(p) == nil}
}

// ConDAVx5 abre el alta sin contraseña sólo para la cuenta y las instrucciones visibles.
func (v *Vista) ConDAVx5() *Vista {
	v.controlClientes.pantalla.AlAbrirDAVx5 = func(base string, contactos bool) {
		if !v.controlClientes.vigente(v.controlClientes.revision) || v.controlClientes.pantalla.Paso != 1 || v.controlClientes.pantalla.Plataforma != Android {
			return
		}
		if v.abrir != nil {
			if err := api.AbrirDAVx5(v.abrir, base, v.controlClientes.pantalla.Correo, contactos); err != nil {
				slog.Warn("hub: no se ha podido abrir DAVx⁵", "error", err)
			}
		}
	}
	return v
}
