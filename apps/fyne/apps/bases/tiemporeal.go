// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Coedición en tiempo real, lado de la app (contrato v1 del servicio), el par de
// lib/tiemporeal.ts de la web: el flujo
// `GET /bases/{id}/events` dice QUÉ ha cambiado y aquí se decide qué volver a pedir
// (planDe, pura y probada como la de la web); el latido `PUT /bases/{id}/presence` dice a
// los demás que esta ventana sigue en la base y en qué celda. El flujo se reabre solo,
// con espera creciente y desde el último cursor, y se deja de reintentar cuando el
// servicio dice que ya no hay acceso.

const (
	// cadaLatido es el `setInterval(latir, 10_000)` de la web: el servicio da por ida una
	// ventana que deja de latir.
	cadaLatido = 10 * time.Second
	// esperaAvisoFoco es el respiro antes de decir la celda con el foco: una flecha
	// mantenida no manda una petición por celda.
	esperaAvisoFoco = 300 * time.Millisecond
	// esperaRecarga junta varias cosas seguidas (una importación, un borrado en
	// cascada) en UNA recarga.
	esperaRecarga = 400 * time.Millisecond
	// topeReconexion es la espera máxima entre dos intentos de reabrir el flujo.
	topeReconexion = 30 * time.Second
)

// tipoPlan es qué hacer ante un cambio que llega por el flujo.
type tipoPlan int

const (
	planNada tipoPlan = iota
	// planRegistro: volver a pedir ese registro y sustituirlo (o añadirlo, si es nuevo).
	planRegistro
	planQuitar
	// planEstructura: recargar base, tablas y campos (y con ello el rol).
	planEstructura
	planComentarios
)

// plan es el `Plan` de la web.
type plan struct {
	tipo tipoPlan
	// id es el registro (planRegistro, planQuitar, planComentarios).
	id    string
	nuevo bool
}

// contextoPlan es el `Contexto` de la web.
type contextoPlan struct {
	// tabla es la que se está mirando.
	tabla string
	// propia dice si una operación de celda la ha lanzado ESTA ventana: su eco no se
	// vuelve a pedir.
	propia func(op string) bool
}

// planDe decide qué pedir de nuevo ante un cambio. Lo de otra tabla no toca lo que se ve.
func planDe(c api.BasesChange, ctx contextoPlan) plan {
	accion := c.Action
	empieza := func(prefijo string) bool { return strings.HasPrefix(accion, prefijo) }
	switch {
	case accion == "access" || accion == "base.update" || empieza("table.") || empieza("field."):
		return plan{tipo: planEstructura}
	case empieza("view."):
		if c.TableID == ctx.tabla {
			return plan{tipo: planEstructura}
		}
		return plan{}
	case c.TableID != ctx.tabla || c.RecordID == "":
		return plan{}
	case empieza("comment."):
		return plan{tipo: planComentarios, id: c.RecordID}
	case c.OpID != "" && ctx.propia != nil && ctx.propia(c.OpID):
		return plan{}
	}
	switch accion {
	case "record.delete":
		return plan{tipo: planQuitar, id: c.RecordID}
	case "record.create":
		return plan{tipo: planRegistro, id: c.RecordID, nuevo: true}
	case "record.update", "record.link":
		return plan{tipo: planRegistro, id: c.RecordID}
	}
	return plan{}
}

// nombreDe es cómo se enseña a alguien presente: su nombre o el principio de su `sub`.
func nombreDe(p api.BasesPresence) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Sub[:min(len(p.Sub), 8)]
}

// presentesPorCelda es quién está en cada celda de una tabla (clave `registro:campo`),
// sin la propia ventana.
func presentesPorCelda(presentes []api.BasesPresence, tabla, miConn string) map[string][]string {
	out := map[string][]string{}
	for _, p := range presentes {
		if p.Conn == miConn || p.TableID != tabla || p.RecordID == "" {
			continue
		}
		clave := p.RecordID + ":" + p.FieldID
		nombre := nombreDe(p)
		if !slices.Contains(out[clave], nombre) {
			out[clave] = append(out[clave], nombre)
		}
	}
	return out
}

// personasPresentes son las personas en la base (una por persona, sin contar la propia
// ventana), en el orden en que llegan.
func personasPresentes(presentes []api.BasesPresence, miConn string) []string {
	vistas := map[string]bool{}
	var out []string
	for _, p := range presentes {
		if p.Conn == miConn || vistas[p.Sub] {
			continue
		}
		vistas[p.Sub] = true
		out = append(out, nombreDe(p))
	}
	return out
}

// opsPropias son las operaciones de celda lanzadas desde esta ventana. Se apuntan en la
// rutina que escribe y se miran en la de la interfaz.
type opsPropias struct {
	mu  sync.Mutex
	ops map[string]bool
}

func (o *opsPropias) anadir(op string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ops == nil {
		o.ops = map[string]bool{}
	}
	o.ops[op] = true
}

func (o *opsPropias) tiene(op string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.ops[op]
}

// enVivo es el tiempo real de una base abierta. Todo se toca en la rutina de la
// interfaz salvo `latido`, que lee también la rutina que late.
type enVivo struct {
	ctx   context.Context
	parar context.CancelFunc
	// presentes es la última lista del servicio; abierto, que ya ha llegado un `hello`.
	presentes   []api.BasesPresence
	abierto     bool
	sinConexion bool
	sinAcceso   bool
	// recarga junta las recargas seguidas; recargaEstructura, si alguna pedía la estructura.
	recarga           *time.Timer
	recargaEstructura bool
	avisoFoco         *time.Timer
	// latir despierta a la rutina que late sin esperar al reloj.
	latir chan struct{}

	mu     sync.Mutex
	latido api.BasesLatido
}

// ponerLatido cambia lo que dice el próximo latido y, con `ya`, lo adelanta.
func (rt *enVivo) ponerLatido(l api.BasesLatido, ya bool) {
	rt.mu.Lock()
	cambia := rt.latido != l
	rt.latido = l
	rt.mu.Unlock()
	if ya && cambia {
		rt.adelantar()
	}
}

func (rt *enVivo) adelantar() {
	select {
	case rt.latir <- struct{}{}:
	default: // ya hay uno pendiente
	}
}

func (rt *enVivo) leerLatido() api.BasesLatido {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.latido
}

// latidoDe es lo que dice el latido de la base: la tabla que se mira y la celda con el
// foco de la rejilla, si alguna vez lo ha tenido (abrir la base no es estar en su
// primera celda).
func (p *Pantalla) latidoDe(b *Base) api.BasesLatido {
	l := api.BasesLatido{Conn: p.conn, TableID: b.Activa}
	if b.Activa != "" && b.rejilla != nil && b.rejilla.tocada && len(b.rejilla.Registros) > 0 {
		if r, c, ok := b.rejilla.actual(); ok {
			l.RecordID, l.FieldID = r.ID, c.ID
		}
	}
	return l
}

// seguir abre el flujo de la base y empieza a latir. Se para con dejarDeSeguir.
func (p *Pantalla) seguir(b *Base) {
	if p.svc == nil {
		return
	}
	ctx, parar := context.WithCancel(context.Background())
	rt := &enVivo{ctx: ctx, parar: parar, latir: make(chan struct{}, 1)}
	rt.latido = p.latidoDe(b)
	b.rt = rt
	id, conn := b.ID, p.conn
	p.vivo(func() {
		cursor := ""
		api.Escuchar(ctx, topeReconexion, func(ctx context.Context) error {
			err := p.svc.Events(ctx, id, conn, cursor, func(c string) { cursor = c }, func(e api.BasesEvento) {
				if e.Tipo == api.BasesAdios && e.Motivo == api.BasesSinAcceso {
					parar() // no se reintenta
				}
				enHilo(func() {
					if p.abierta == b && (ctx.Err() == nil || e.Tipo == api.BasesAdios) {
						//nolint:contextcheck // Lo que pide la pantalla vive lo que la pantalla, no el flujo.
						p.alEvento(b, e)
					}
				})
			})
			if ctx.Err() != nil {
				return err
			}
			var e *api.Error
			sinAcceso := errors.As(err, &e) && (e.Status == http.StatusNotFound || e.Status == http.StatusForbidden)
			if sinAcceso {
				parar()
			}
			//nolint:contextcheck // Lo que pide la pantalla vive lo que la pantalla, no el flujo.
			enHilo(func() {
				if p.abierta != b {
					return
				}
				switch {
				case sinAcceso && rt.abierto:
					p.perderAcceso(b)
				case sinAcceso:
					// No llegó a abrirse: la base no se ve, y eso ya lo dice su carga.
				case !rt.sinConexion:
					rt.sinConexion = true
					p.Repintar()
				}
			})
			return err
		})
	})
	p.vivo(func() { p.latiendo(ctx, id, rt) })
}

// latiendo manda el latido cada cadaLatido, y antes si se le adelanta (la celda con el
// foco ha cambiado, se ha abierto el flujo). Sale al parar la base.
func (p *Pantalla) latiendo(ctx context.Context, base string, rt *enVivo) {
	reloj := time.NewTicker(p.cadaLatido)
	defer reloj.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloj.C:
		case <-rt.latir:
		}
		c, cancelar := context.WithTimeout(ctx, p.Plazo)
		_ = p.svc.Heartbeat(c, base, rt.leerLatido())
		cancelar()
	}
}

// dejarDeSeguir cierra el flujo y el latido de una base que se deja (al volver a la
// lista o al abrir otra). Al cerrarse el flujo, el servicio quita su presencia.
func (p *Pantalla) dejarDeSeguir(b *Base) {
	if b == nil || b.rt == nil {
		return
	}
	b.rt.parar()
	if b.rt.recarga != nil {
		b.rt.recarga.Stop()
	}
	if b.rt.avisoFoco != nil {
		b.rt.avisoFoco.Stop()
	}
}

// alEvento atiende un evento del flujo, en la rutina de la interfaz.
func (p *Pantalla) alEvento(b *Base, e api.BasesEvento) {
	rt := b.rt
	switch e.Tipo {
	case api.BasesHola:
		// Lo que cambiara entre la primera carga y la apertura del flujo no lo cuenta
		// nadie: se vuelve a pedir una vez. Al reconectar ya lo cubre el replay.
		if !rt.abierto {
			p.recargar(b, false)
		}
		rt.abierto = true
		rt.adelantar()
		if rt.sinConexion {
			rt.sinConexion = false
			p.Repintar()
		}
	case api.BasesCambio:
		p.aplicarCambio(b, e.Cambio)
	case api.BasesPresencia:
		rt.presentes = e.Presentes
		if b.rejilla != nil {
			b.rejilla.PonerPresentes(presentesPorCelda(rt.presentes, b.Activa, p.conn))
		}
		p.Repintar() // los avatares de la cabecera, y las tarjetas del tablero
	case api.BasesResync:
		p.recargar(b, true)
	case api.BasesAdios:
		if e.Motivo == api.BasesSinAcceso {
			p.perderAcceso(b)
		}
		// `token` o `error`: el servicio cierra y se reabre desde el último cursor.
	}
}

// perderAcceso es el `bye {reason: access}`: se dice y se deja de escuchar.
func (p *Pantalla) perderAcceso(b *Base) {
	rt := b.rt
	rt.sinAcceso, rt.presentes = true, nil
	p.dejarDeSeguir(b)
	if b.rejilla != nil {
		b.rejilla.PonerPresentes(nil)
	}
	p.Repintar()
}

// aplicarCambio vuelve a pedir lo justo de un cambio de otra persona.
func (p *Pantalla) aplicarCambio(b *Base, c api.BasesChange) {
	pl := planDe(c, contextoPlan{tabla: b.Activa, propia: b.propias.tiene})
	switch pl.tipo {
	case planNada:
	case planEstructura:
		p.recargar(b, true)
	case planQuitar:
		b.vent.editar(func(x api.BasesRecord) (api.BasesRecord, bool) { return x, x.ID != pl.id })
		p.ponerRejilla()
		p.Repintar()
	case planComentarios:
		if k := p.comentando; k != nil && k.registro == pl.id && k.d.Abierto() {
			k.cargar()
		}
	case planRegistro:
		// Con una vista (filtro u orden), una búsqueda o más páginas por cargar, un alta
		// puede no tocar aquí o ir a otro sitio: se recarga, o se deja para cuando llegue.
		if pl.nuevo && (b.VistaID != "" || b.lectura.hay() || b.vent.siguiente != "") {
			if b.VistaID != "" || b.lectura.hay() {
				p.recargar(b, false)
			}
			return
		}
		p.releerRegistro(b, pl.id, pl.nuevo)
	}
}

// releerRegistro pide un registro que ha cambiado y lo sustituye donde está, o lo añade
// al final si es nuevo. El foco y el editor abierto siguen donde estaban (rejilla.Poner).
func (p *Pantalla) releerRegistro(b *Base, id string, nuevo bool) {
	tabla := b.Activa
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := p.svc.GetRecord(ctx, id)
		enHilo(func() {
			if err != nil || p.abierta != b || b.Activa != tabla {
				return
			}
			esta := false
			b.vent.editar(func(x api.BasesRecord) (api.BasesRecord, bool) {
				if x.ID == r.ID {
					esta = true
					return r, true
				}
				return x, true
			})
			if !esta && !nuevo {
				return
			}
			if !esta {
				b.vent.anadir(r)
			}
			p.ponerRejilla()
			p.Repintar()
		})
	})
}

// recargar vuelve a pedir los registros (y con `estructura`, la base y sus tablas), una
// sola vez para varias peticiones seguidas.
func (p *Pantalla) recargar(b *Base, estructura bool) {
	rt := b.rt
	rt.recargaEstructura = rt.recargaEstructura || estructura
	if rt.recarga != nil {
		rt.recarga.Stop()
	}
	rt.recarga = time.AfterFunc(p.esperaRecarga, func() {
		enHilo(func() {
			if p.abierta != b || rt.ctx.Err() != nil {
				return
			}
			conEstructura := rt.recargaEstructura
			rt.recargaEstructura = false
			if conEstructura {
				p.cargarEstructura()
			}
			p.cargarRegistros("inicio")
		})
	})
}

// alEnfocarCelda dice a los demás, con un respiro, en qué celda está ahora el foco.
func (p *Pantalla) alEnfocarCelda(b *Base) {
	rt := b.rt
	if rt == nil || rt.ctx.Err() != nil {
		return
	}
	if rt.avisoFoco != nil {
		rt.avisoFoco.Stop()
	}
	rt.avisoFoco = time.AfterFunc(esperaAvisoFoco, func() {
		enHilo(func() {
			if p.abierta == b && rt.ctx.Err() == nil {
				rt.ponerLatido(p.latidoDe(b), true)
			}
		})
	})
}

// objetoEnVivo es lo que la cabecera de la base dice del tiempo real: el acceso perdido
// o la conexión caída, y los avatares de quienes están.
func (p *Pantalla) objetoEnVivo(b *Base, v tema.Variante) []fyne.CanvasObject {
	rt := b.rt
	if rt == nil {
		return nil
	}
	var hijos []fyne.CanvasObject
	switch {
	case rt.sinAcceso:
		hijos = append(hijos, alerta(p.t("rt.lostAccess"), v))
	case rt.sinConexion:
		hijos = append(hijos, ui.NuevoParrafo(p.t("rt.offline"), tema.TextoSM, ui.Normal, "text-muted", v).A(p.anchoContenido()))
	}
	if personas := personasPresentes(rt.presentes, p.conn); len(personas) > 0 {
		// `flex flex-wrap items-center gap-1`, cada uno un Avatar xs con su nombre.
		avatares := make([]fyne.CanvasObject, len(personas))
		for i, n := range personas {
			avatares[i] = ui.NuevoAvatar(n).Con(ui.AvatarXS)
		}
		// Con muchas personas (o en un teléfono) la fila baja a la siguiente, a su alto.
		hijos = append(hijos, ui.AlAncho(p.anchoContenido(), container.New(ui.FilaAuto{
			Hueco: tema.Espacio(1), Derecha: len(avatares), Partir: true, SaltoFila: tema.Espacio(1),
		}, avatares...)))
	}
	return hijos
}

// enTarjeta es quién está en CUALQUIER celda de un registro, sin repetir (Tablero.tsx).
func (p *Pantalla) enTarjeta(b *Base, registro string) []string {
	if b.rt == nil {
		return nil
	}
	var out []string
	for _, pr := range b.rt.presentes {
		if pr.Conn == p.conn || pr.TableID != b.Activa || pr.RecordID != registro {
			continue
		}
		if n := nombreDe(pr); !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}
