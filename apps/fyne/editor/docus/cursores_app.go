// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

// Los cursores por el canal en vivo, el lado de la app: lo que en la web reparten
// `collab.ts` (mandar el propio con su ritmo, guardar el último de cada par y podarlo con
// la lista de presentes) y el efecto de `PageCanvas.tsx` que los sitúa en el texto de
// AHORA. Son presencia efímera: sin canal no se manda ni se pinta ninguno.

// pausaCursor es el CURSOR_THROTTLE_MS de `collab.ts`: el primer movimiento sale ya y los
// que llegan dentro de la pausa se juntan en uno, el último, al cumplirse.
const pausaCursor = 80 * time.Millisecond

// parAjeno es el último cursor de otra persona: tal como llegó y dónde está ahora, con el
// texto sobre el que esa posición era cierta (para recolocarlo si no trae puntos).
type parAjeno struct {
	pres  api.DocPresencia
	pos   api.Cursor
	off   coedicion.Offsets
	texto []uint16
}

// claveCursor es `cursorPosKey`: la misma posición, la misma clave, y la etiqueta sigue
// desvaneciéndose; otra, y vuelve a verse.
func claveCursor(c api.Cursor) string {
	return "d" + strconv.Itoa(c.Anchor) + ":" + strconv.Itoa(c.Focus)
}

// presencia es `applyPresence`: la lista de presentes poda los cursores de quien se ha
// ido, y la difusión de un cursor lo pone o lo cambia.
func (a *App) presencia(ab *abierto, m api.DocMensaje) {
	cambio := false
	if m.Presence != nil {
		presentes := map[string]bool{}
		for _, p := range m.Presence {
			presentes[p.Sub] = true
		}
		for id := range ab.pares {
			if !presentes[id] {
				delete(ab.pares, id)
				cambio = true
			}
		}
	}
	if c := m.Cursor; c != nil && c.Cursor.Kind == "doc" {
		if ab.pares == nil {
			ab.pares = map[string]*parAjeno{}
		}
		pos := *c.Cursor
		ab.pares[c.Sub] = &parAjeno{pres: *c, pos: pos, off: coedicion.Offsets{Anchor: pos.Anchor, Focus: pos.Focus}}
		cambio = true
	}
	if cambio {
		a.reubicarPares(ab)
	}
}

// canalCerrado: lo que mandaba el canal deja de valer. Los cursores ajenos se quitan
// (`clearPeerCursors`): quedarían quietos donde estaban.
func (a *App) canalCerrado(ab *abierto) {
	ab.canalAbierto, ab.canal = false, nil
	if len(ab.pares) > 0 {
		ab.pares = nil
		a.reubicarPares(ab)
	}
}

// nombreDePar es `peerName` de state/cursors.ts: el nombre de un colaborador, o
// «Colaborador» (`header.peerUnnamed`) si su presencia no lo trae. Sin la reserva, su cursor
// salía sin etiqueta.
func (a *App) nombreDePar(nombre string) string {
	if n := strings.TrimSpace(nombre); n != "" {
		return n
	}
	return a.t("header.peerUnnamed")
}

// reubicarPares sitúa cada cursor ajeno en el folio de ahora: por los caracteres a los que
// se ancla si los trae y la secuencia coincide con el folio; si no, por lo que ha
// cambiado el texto desde la última vez que se situó (coedicion.AnclarOffset).
func (a *App) reubicarPares(ab *abierto) {
	f := ab.folio
	texto := f.textoVisible()
	lista := make([]ParCursor, 0, len(ab.pares))
	for id, p := range ab.pares {
		exacto := false
		if p.pos.Puntos != nil && ab.co != nil {
			if off, ok := ab.co.OffsetsDePuntos(len(texto), *p.pos.Puntos); ok {
				p.off, exacto = off, true
			}
		}
		if !exacto && p.texto != nil {
			p.off = coedicion.Offsets{
				Anchor: coedicion.AnclarOffset(p.off.Anchor, p.texto, texto),
				Focus:  coedicion.AnclarOffset(p.off.Focus, p.texto, texto),
			}
		}
		p.texto = texto
		lista = append(lista, ParCursor{
			ID: id, Nombre: a.nombreDePar(p.pres.Name), Color: p.pres.Color,
			Anchor: p.off.Anchor, Focus: p.off.Focus, Clave: claveCursor(p.pos),
		})
	}
	// En orden fijo: con dos en el mismo sitio, el de encima no cambia en cada repintado.
	slices.SortFunc(lista, func(x, y ParCursor) int { return strings.Compare(x.ID, y.ID) })
	f.PonerPares(lista)
}

// fusionarFolio pone en el folio lo que deja la coedición y lleva el cursor propio con
// SUS caracteres: por los puntos de la secuencia si los hay, y si no, por lo que ha
// cambiado el texto. Sin esto se quedaba en su bloque y su posición, y lo que otro
// escribía delante en la misma línea lo dejaba en otra palabra.
func (a *App) fusionarFolio(ab *abierto, html string, anclado *coedicion.Puntos) {
	f := ab.folio
	antes := f.textoVisible()
	off := coedicion.Offsets{Anchor: f.offsetDe(f.anc), Focus: f.offsetDe(f.cur)}
	f.Fusionar(html)
	despues := f.textoVisible()
	exacto := false
	if anclado != nil && ab.co != nil {
		if o, ok := ab.co.OffsetsDePuntos(len(despues), *anclado); ok {
			off, exacto = o, true
		}
	}
	if !exacto {
		off = coedicion.Offsets{
			Anchor: coedicion.AnclarOffset(off.Anchor, antes, despues),
			Focus:  coedicion.AnclarOffset(off.Focus, antes, despues),
		}
	}
	if f.conCursor {
		f.cursorEnOffsets(off)
	}
	a.reubicarPares(ab)
}

// cursorMovido es el AlCursor del folio: el cursor propio sale hacia los demás y, si ha
// cambiado el documento, los ajenos se recolocan sobre lo escrito.
func (a *App) cursorMovido(ab *abierto, editado bool) {
	if a.abierto != ab {
		return
	}
	if editado && len(ab.pares) > 0 {
		a.reubicarPares(ab)
	}
	a.emitirCursor(ab)
}

// emitirCursor es `sendCursor`: con el canal abierto, el primer movimiento sale ya y los
// siguientes, dentro de la pausa, se juntan en el último. Sin canal no se manda nada.
func (a *App) emitirCursor(ab *abierto) {
	if ab.canal == nil || !ab.canalAbierto {
		return
	}
	espera := pausaCursor - time.Since(ab.cursorEnviado)
	if espera <= 0 {
		a.enviarCursor(ab)
		return
	}
	if ab.cursorEnEspera {
		return // ya sale uno al cumplirse la pausa, y llevará la posición de entonces
	}
	ab.cursorEnEspera = true
	a.programar(espera, func() {
		ab.cursorEnEspera = false
		if a.abierto == ab {
			a.enviarCursor(ab)
		}
	})
}

// enviarCursor manda la posición de ahora: offsets del texto visible y, si la secuencia
// coincide con el folio, los caracteres a los que se ancla.
func (a *App) enviarCursor(ab *abierto) {
	canal := ab.canal
	if canal == nil || !ab.canalAbierto {
		return
	}
	ab.cursorEnviado = time.Now()
	f := ab.folio
	off := coedicion.Offsets{Anchor: f.offsetDe(f.anc), Focus: f.offsetDe(f.cur)}
	c := api.Cursor{Kind: "doc", Anchor: off.Anchor, Focus: off.Focus}
	if ab.co != nil {
		if p, ok := ab.co.PuntosDeOffsets(len(f.textoVisible()), off); ok {
			c.Puntos = &p
		}
	}
	plazo := a.Plazo
	a.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), plazo)
		defer cancelar()
		_ = canal.Enviar(ctx, c) // efímero: el siguiente movimiento manda la buena
	})
}
