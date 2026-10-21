// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

// La coedición por carácter de `useDocument.ts` y `collab.ts`: cada guardado lleva las
// ops de la secuencia además del registro, y lo que escriben los demás llega por el
// sondeo de `GET …/ops` y se fusiona en cuanto no hay nada propio en vuelo.
//
// Llega por el canal en vivo (`/documents/{id}/ws`) y, mientras no está abierto, por el
// sondeo, como en la web; la presencia se renueva por HTTP siempre, que es lo único que
// ve también a quien está sin canal.
//
// ponytail: los cursores de los demás aún no se pintan. Tras fusionar, el cursor propio
// se queda en su bloque y su posición, sin correrse con lo que otro haya escrito delante
// en la misma línea.

// SondeoCoedicion es el POLL_MS de la web; cada tres rondas se renueva la presencia, que
// el servicio olvida a los 30 s.
const (
	SondeoCoedicion = 4 * time.Second
	rondasPresencia = 3
)

// iniciarCoedicion arma la secuencia desde el estado que se acaba de leer y echa a andar
// el sondeo. Corre en el hilo de la interfaz.
func (a *App) iniciarCoedicion(ab *abierto, cont api.DocContent) {
	ab.co = coedicion.Nueva(a.actor, coedicion.ElementosDeEstado(cont.Elementos), cont.Value)
	ab.lamport, ab.seq = cont.MaxLamport, cont.Seq
	ab.co.FijarContador(ab.cuenta)
	a.sondear(ab) // ya: la presencia dice si se puede sembrar antes del primer guardado
	a.programarSondeo(ab)
	if a.EnVivo {
		a.abrirCanal(ab)
	}
}

// Reconexión del canal: `1 s, 2 s, 4 s…` hasta 30, el WS_BACKOFF de la web.
const (
	esperaCanal    = time.Second
	esperaCanalMax = 30 * time.Second
)

// abrirCanal mantiene el canal en vivo mientras el documento siga abierto: al cortarse,
// vuelve a llamar con la última seq vista, y entretanto manda el sondeo.
func (a *App) abrirCanal(ab *abierto) {
	c, cancelar := context.WithCancel(context.Background())
	ab.cerrarCanal = cancelar
	id := ab.id
	ab.seqVista.Store(ab.seq)
	go func() {
		espera := esperaCanal
		for c.Err() == nil {
			//nolint:contextcheck // un `role` recarga el documento, y la carga abre su propio plazo
			err := a.Svc.Coeditar(c, id, ab.seqVista.Load(), func(m api.DocMensaje) {
				espera = esperaCanal
				fyne.Do(func() { a.enVivo(ab, m) })
			})
			fyne.Do(func() { ab.canalAbierto = false })
			if c.Err() != nil || err == nil {
				return
			}
			select {
			case <-c.Done():
				return
			case <-time.After(espera):
			}
			espera = min(2*espera, esperaCanalMax)
		}
	}()
}

// enVivo es un mensaje del canal, en el hilo de la interfaz.
func (a *App) enVivo(ab *abierto, m api.DocMensaje) {
	if a.abierto != ab {
		return
	}
	switch m.Type {
	case "welcome":
		ab.canalAbierto = true
	case "ops":
		a.recibir(ab, m.Ops)
	case "role":
		if api.CanWrite(m.Role) && ab.ctx.SoloLectura {
			a.recargarPorRol(ab)
		}
	}
}

func (a *App) programarSondeo(ab *abierto) {
	ab.pararSondeo = a.programar(SondeoCoedicion, func() {
		if a.abierto != ab {
			if ab.cerrarCanal != nil {
				ab.cerrarCanal()
			}
			return
		}
		a.sondear(ab)
		a.programarSondeo(ab)
	})
}

// sondear pide lo nuevo (y, de vez en cuando, quién está) en su rutina y lo entrega en
// el hilo de la interfaz.
func (a *App) sondear(ab *abierto) {
	if a.Svc == nil || ab.sondeando {
		return
	}
	ab.sondeando = true
	ab.rondas++
	id, desde, presencia, sondeo := ab.id, ab.seq, ab.rondas%rondasPresencia == 1, !ab.canalAbierto
	if !presencia && !sondeo {
		ab.sondeando = false
		return // con el canal abierto, esta ronda no hay nada que pedir
	}
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		var ops []api.DocOpSecuenciada
		err := errNoPedida
		if sondeo {
			ops, err = a.Svc.OpsDesde(c, id, desde)
		}
		var miembros []api.DocMiembro
		errP := errNoPedida
		if presencia {
			miembros, errP = a.Svc.Presencia(c, id, a.actor)
		}
		fyne.Do(func() {
			ab.sondeando = false
			if a.abierto != ab {
				return
			}
			if errP == nil {
				ab.presentes = otrasPersonas(miembros, a.actor)
			}
			if err == nil {
				a.recibir(ab, ops)
			}
		})
	})
}

var errNoPedida = errors.New("no se ha pedido en esta ronda")

// otrasPersonas cuenta a los demás presentes, una vez por persona: dos pestañas de la
// misma son una, y la conexión propia no cuenta.
func otrasPersonas(miembros []api.DocMiembro, conn string) int {
	vistas := map[string]bool{}
	for _, m := range miembros {
		if m.Conn != conn {
			vistas[m.Sub] = true
		}
	}
	return len(vistas)
}

// recibir es el reparto de `collab.ts` para un lote: las ops de la secuencia van a la
// fusión; un `<clave>:lww` ajeno anula la secuencia; el `set` del registro se adopta por
// LWW si el canal por carácter no está en marcha y no hay nada propio sin guardar.
func (a *App) recibir(ab *abierto, ops []api.DocOpSecuenciada) {
	clave := ab.ctx.Tipo.ClaveContenido()
	var chars []coedicion.Op
	var registro *api.DocContent
	anular := false
	for _, o := range ops {
		ab.seq = max(ab.seq, o.Seq)
		ab.seqVista.Store(ab.seq)
		var set struct {
			Kind    string          `json:"kind"`
			Dot     api.DocDot      `json:"dot"`
			Key     string          `json:"key"`
			Value   json.RawMessage `json:"value"`
			Lamport uint64          `json:"lamport"`
		}
		if json.Unmarshal(o.Op, &set) != nil {
			continue
		}
		ab.lamport = max(ab.lamport, set.Lamport)
		if o.Actor == a.actor || set.Dot.Actor == a.actor {
			continue // el eco de lo propio
		}
		if op, ok := coedicion.OpDeSecuencia(o.Op); ok {
			chars = append(chars, op)
			continue
		}
		switch {
		case set.Kind == "set" && set.Key == coedicion.ClaveLWW(clave):
			anular = true
		case set.Kind == "set" && set.Key == clave:
			r := api.DocContent{Dot: set.Dot, Lamport: set.Lamport}
			if json.Unmarshal(set.Value, &r.Value) == nil {
				registro = &r
			}
		}
	}
	if anular {
		ab.co.Anular()
	}
	ab.co.Recibir(chars)
	if !a.fusionar(ab) && registro != nil && !ab.co.Sincronizada() && a.sinNadaPropio(ab) && registro.Lamport > ab.base.Lamport {
		// LWW de toda la vida: lo del otro es más nuevo y aquí no hay nada que perder.
		registro.MaxLamport = ab.lamport
		ab.base, ab.conflicto = *registro, false
		ab.folio.Fusionar(registro.Value)
		ab.guardado = ab.folio.Cambios()
		a.ponerGuardado(ab, editor.Limpio)
	}
}

// sinNadaPropio dice si no hay edición local en vuelo: lo que se ve es lo guardado.
func (a *App) sinNadaPropio(ab *abierto) bool {
	return !ab.guardando && !ab.forzar && ab.folio.Cambios() == ab.guardado
}

// fusionar vuelca al folio lo ajeno recibido, solo sin edición local en vuelo (si no, el
// siguiente diff borraría lo que acaba de llegar), y deja el registro por reescribir: el
// servicio lee de él y todavía tiene la vista del otro.
func (a *App) fusionar(ab *abierto) bool {
	if ab.co == nil || !a.sinNadaPropio(ab) {
		return false
	}
	txt, ok := ab.co.Fusionar()
	if !ok {
		return false
	}
	ab.folio.Fusionar(txt)
	ab.forzar = true
	a.programarGuardado(ab)
	return true
}

// loteCoedicion prepara el guardado con la secuencia (`buildOpsBody`) en el hilo de la
// interfaz y da el envío, que corre en otra rutina con su propio plazo y entrega el resultado con fyne.Do:
// las ops que llevan hasta el HTML, acumuladas hasta que el servidor dice que sí
// —reenviar las mismas es seguro, el CRDT es idempotente—, y el `set` del registro.
// Autoritativo si la secuencia no estaba al día; un lote que no cabe renuncia a ella y
// va autoritativo y sin ops.
func (a *App) loteCoedicion(ab *abierto, html string, n int) func() error {
	co := ab.co
	autoritativo := !co.Sincronizada()
	nuevas := co.OpsLocales(html, ab.lamport, ab.presentes == 0)
	ab.lamport += uint64(len(nuevas))
	ab.pendientes = append(ab.pendientes, nuevas...)
	ab.cuenta = max(ab.cuenta, co.Contador()) + 1
	co.FijarContador(ab.cuenta)
	ab.lamport++
	dot, lamport, chars := api.DocDot{Actor: a.actor, Counter: ab.cuenta}, ab.lamport, ab.pendientes
	id, clave := ab.id, ab.ctx.Tipo.ClaveContenido()
	return func() error {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		_, err := a.Svc.SaveCoedicion(c, id, clave, html, dot, lamport, autoritativo, chars)
		grande := errors.Is(err, api.ErrLoteGrande)
		if grande {
			_, err = a.Svc.SaveCoedicion(c, id, clave, html, dot, lamport, true, nil)
		}
		fyne.Do(func() {
			if grande {
				co.Anular()
			}
			if err == nil || grande {
				ab.pendientes = ab.pendientes[len(chars):]
			}
			a.guardadoHecho(ab, n, api.DocContent{Value: html, Dot: dot, Lamport: lamport, MaxLamport: lamport}, err)
		})
		return err
	}
}

// conCoedicion dice si este guardado va por la secuencia: con ella al día, o sin nadie
// más para sembrarla. Si no, va por el registro con la comprobación de delante.
func (ab *abierto) conCoedicion() bool {
	return ab.co != nil && (ab.co.Sincronizada() || ab.presentes == 0)
}
