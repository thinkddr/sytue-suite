// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"time"
	"unicode/utf16"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
)

// Los cursores de los demás en el folio: lo que en la web es la capa de `PageCanvas.tsx`
// (un caret fino, la selección translúcida y la etiqueta con el nombre, en el color de la
// persona) y el mapeo de sus offsets de texto a posiciones del cursor.
//
// Un cursor viaja como offsets del TEXTO VISIBLE del folio, el `textContent` de la web, en
// unidades UTF-16: aquí los bloques se recorren en orden y cada runa cuenta lo que cuenta
// en el navegador. Un salto de línea (`<br>`) y un objeto (una imagen) no son texto: cero.

// unidadesRuna es lo que aporta al `textContent` cada runa de un bloque.
func unidadesRuna(r rune) int {
	switch {
	case r == '\n' || r == rico.ObjetoRuna:
		return 0
	case r > 0xFFFF:
		return 2
	}
	return 1
}

// textoVisible es el `textContent` del folio en unidades UTF-16: sobre lo que se miden
// los offsets de un cursor y lo que compara el anclaje de un cursor sin puntos.
func (f *Folio) textoVisible() []uint16 {
	var out []uint16
	for _, b := range f.bloques {
		for _, r := range b.Texto() {
			if unidadesRuna(r) > 0 {
				out = utf16.AppendRune(out, r)
			}
		}
	}
	return out
}

// offsetDe es el offset de texto de una posición del cursor (`textOffsetIn` de la web).
func (f *Folio) offsetDe(q punto) int {
	off := 0
	for i, b := range f.bloques {
		runas := []rune(b.Texto())
		if i == q.b {
			for _, r := range runas[:min(max(q.p, 0), len(runas))] {
				off += unidadesRuna(r)
			}
			return off
		}
		for _, r := range runas {
			off += unidadesRuna(r)
		}
	}
	return off
}

// puntoDeOffset es `pointAtOffset` de la web: la primera posición de un bloque de texto
// donde cabe el offset; en una frontera entre bloques, el final del de arriba. false si
// el offset se sale del texto, y entonces no se pinta (contenido distinto entre pares).
func (f *Folio) puntoDeOffset(off int) (punto, bool) {
	return f.puntoDeOffsetCerca(off, -1)
}

// puntoDeOffsetCerca es puntoDeOffset eligiendo, cuando el offset cae en una frontera
// (el final de un párrafo, un párrafo vacío y el principio del siguiente son el mismo
// offset), la posición del bloque más cercano a `cerca`; -1, la primera, como la web.
func (f *Folio) puntoDeOffsetCerca(off, cerca int) (punto, bool) {
	if off < 0 {
		return punto{}, false
	}
	mejor, hay := punto{}, false
	resto := off
	for i, b := range f.bloques {
		runas := []rune(b.Texto())
		largo := 0
		for _, r := range runas {
			largo += unidadesRuna(r)
		}
		if f.deTexto(i) && resto <= largo {
			p, n := 0, 0
			for p < len(runas) && n < resto && n+unidadesRuna(runas[p]) <= resto {
				n += unidadesRuna(runas[p])
				p++
			}
			q := punto{i, p}
			if !hay || absInt(i-cerca) < absInt(mejor.b-cerca) {
				mejor, hay = q, true
			}
			if cerca < 0 || resto < largo || i >= cerca {
				break // no hay frontera, o ya no se acerca más
			}
		}
		resto -= largo
		if resto < 0 {
			break
		}
	}
	return mejor, hay
}

// cursorEnOffsets pone el cursor propio en estos offsets de texto, si caen en el folio,
// lo más cerca posible del bloque en el que estaba. Es lo que hace que, tras fusionar lo
// de otro, el cursor siga a SUS caracteres.
func (f *Folio) cursorEnOffsets(off coedicion.Offsets) {
	cur, okc := f.puntoDeOffsetCerca(off.Focus, f.cur.b)
	anc, oka := f.puntoDeOffsetCerca(off.Anchor, f.anc.b)
	if !okc || !oka {
		return
	}
	if cur.b != f.cur.b {
		f.ed = nil
	}
	f.cur, f.anc = cur, anc
	f.pag.Refresh()
}

// avisarCursor llama a AlCursor (diciendo si ha cambiado el documento) si el cursor, la selección o el documento han cambiado
// desde la última vez. Lo llama programarFormato, que es por donde pasa cada movimiento.
func (f *Folio) avisarCursor() {
	if f.AlCursor == nil || !f.conCursor {
		return
	}
	ahora := estadoCursor{f.cur, f.anc, f.cambios}
	if ahora == f.avisado {
		return
	}
	editado := ahora.cambios != f.avisado.cambios
	f.avisado = ahora
	f.AlCursor(editado)
}

// estadoCursor es lo que, al cambiar, se avisa por AlCursor.
type estadoCursor struct {
	cur, anc punto
	cambios  int
}

// ParCursor es el cursor de otra persona listo para pintar: quién es (su `sub`, su nombre
// y el token de su color) y dónde está, en offsets del texto visible de AHORA. Clave
// reinicia el desvanecido de la etiqueta: cambia cuando la persona se mueve.
type ParCursor struct {
	ID, Nombre, Color string
	Anchor, Focus     int
	Clave             string
}

// parFolio es un ParCursor ya situado en el folio.
type parFolio struct {
	ParCursor
	anc, cur punto
	etiqueta *etiquetaPar
}

// Cómo se ven los cursores ajenos, de `PageCanvas.tsx` y `editor.css`: un caret de 2 px,
// la selección al 18 % y la etiqueta 15 px por encima del caret, de hasta 160 px, que se
// queda entera el 55 % de 2,6 s y luego se desvanece (`sytue-peer-label-fade`).
const (
	anchoCaretPar    = 2
	opacidadSelPar   = 0.18
	alzaEtiquetaPar  = 15
	maxEtiquetaPar   = 160
	duracionEtiqueta = 2600 * time.Millisecond
	etiquetaEntera   = 0.55
)

// opacidadEtiqueta es la de la etiqueta en el instante t (0…1, ya con el `ease-out`).
func opacidadEtiqueta(t float32) float32 {
	if t <= etiquetaEntera {
		return 1
	}
	return max(0, 1-(t-etiquetaEntera)/(1-etiquetaEntera))
}

// etiquetaPar son los objetos de una etiqueta: se conservan entre repintados para que el
// desvanecido siga sin rehacer la página en cada fotograma.
type etiquetaPar struct {
	fondo *canvas.Rectangle
	texto *canvas.Text
	anim  *fyne.Animation
	alfa  float32
}

func (e *etiquetaPar) parar() {
	if e.anim != nil {
		e.anim.Stop()
		e.anim = nil
	}
}

// aplicarAlfa tiñe la etiqueta con su opacidad y la repinta.
func (e *etiquetaPar) aplicarAlfa(fondo, tinta color.NRGBA) {
	fondo.A = uint8(float32(fondo.A) * e.alfa)
	tinta.A = uint8(float32(tinta.A) * e.alfa)
	e.fondo.FillColor, e.texto.Color = fondo, tinta
}

// PonerPares cambia los cursores ajenos que se pintan. Los que no caen en el texto de
// ahora no se pintan; la etiqueta de quien se ha movido vuelve a verse entera.
func (f *Folio) PonerPares(pares []ParCursor) {
	antes := map[string]*parFolio{}
	for i := range f.pares {
		antes[f.pares[i].ID] = &f.pares[i]
	}
	nuevos := make([]parFolio, 0, len(pares))
	for _, p := range pares {
		cur, okc := f.puntoDeOffset(p.Focus)
		anc, oka := f.puntoDeOffset(p.Anchor)
		if !okc || !oka {
			continue
		}
		pf := parFolio{ParCursor: p, anc: anc, cur: cur}
		if previo, ok := antes[p.ID]; ok {
			pf.etiqueta = previo.etiqueta
			delete(antes, p.ID)
			if previo.Clave != p.Clave || previo.Color != p.Color {
				f.desvanecer(&pf)
			}
		} else {
			f.desvanecer(&pf)
		}
		nuevos = append(nuevos, pf)
	}
	for _, p := range antes {
		if p.etiqueta != nil {
			p.etiqueta.parar()
		}
	}
	f.pares = nuevos
	f.pag.Refresh()
}

// desvanecer (re)arranca la etiqueta del par: entera y, salvo con menos movimiento, se va
// apagando. La animación cambia solo la etiqueta, no repinta la página.
func (f *Folio) desvanecer(p *parFolio) {
	if p.etiqueta == nil {
		p.etiqueta = &etiquetaPar{fondo: canvas.NewRectangle(color.NRGBA{}), texto: ui.Texto("", tema.Texto2XS, ui.Medio, color.NRGBA{})}
	}
	e := p.etiqueta
	e.parar()
	e.alfa = 1
	if ui.MenosMovimiento() {
		return // `prefers-reduced-motion`: la etiqueta se queda fija
	}
	token := p.Color
	e.anim = fyne.NewAnimation(duracionEtiqueta, func(t float32) {
		e.alfa = opacidadEtiqueta(t)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		e.aplicarAlfa(tema.Color(token, v), tema.Color("neutral-0", v))
		e.fondo.Refresh()
		e.texto.Refresh()
	})
	e.anim.Curve = fyne.AnimationEaseOut
	e.anim.Start()
}

// pintarSeleccionesPares pone la selección de cada par, debajo del texto como la propia.
func (f *Folio) pintarSeleccionesPares(objs []fyne.CanvasObject, ox, oy, z float32, v tema.Variante) []fyne.CanvasObject {
	for _, p := range f.pares {
		if p.anc == p.cur {
			continue
		}
		col := tema.Color(p.Color, v)
		col.A = uint8(float32(col.A) * opacidadSelPar)
		a, b := p.anc, p.cur
		if b.antes(a) {
			a, b = b, a
		}
		for _, rc := range f.rectangulosEntre(a, b) {
			r := canvas.NewRectangle(col)
			r.CornerRadius = z
			r.Move(fyne.NewPos(ox+rc.x*z, oy+rc.y*z))
			r.Resize(fyne.NewSize(rc.w*z, rc.h*z))
			objs = append(objs, r)
		}
	}
	return objs
}

// pintarCaretsPares pone, encima de todo, el caret de cada par y su etiqueta.
func (f *Folio) pintarCaretsPares(objs []fyne.CanvasObject, ox, oy, z float32, v tema.Variante) []fyne.CanvasObject {
	for _, p := range f.pares {
		x, y, alto, ok := f.caretEnMaqueta(p.cur)
		if !ok {
			continue
		}
		col := tema.Color(p.Color, v)
		x, y = ox+x*z, oy+y*z
		caret := canvas.NewRectangle(col)
		caret.Move(fyne.NewPos(x, y))
		caret.Resize(fyne.NewSize(anchoCaretPar*z, alto*z))
		objs = append(objs, caret)
		if e := p.etiqueta; e != nil && p.Nombre != "" {
			// `text-2xs leading-tight font-medium px-1 py-px rounded-sm`, en px de la página.
			es := tema.Escalon{Tam: tema.Texto2XS.Tam * z, Linea: tema.Texto2XS.Tam * 1.25 * z}
			e.texto.TextSize = es.Tam
			ancho := ui.Truncar(e.texto, p.Nombre, es, ui.Medio, (maxEtiquetaPar-8)*z)
			e.fondo.CornerRadius = tema.RadioSM * z
			e.fondo.Move(fyne.NewPos(x, y-alzaEtiquetaPar*z))
			e.fondo.Resize(fyne.NewSize(ancho+8*z, es.Linea+2*z))
			ui.ColocarTexto(e.texto, es, x+4*z, y-alzaEtiquetaPar*z+z)
			e.aplicarAlfa(col, tema.Color("neutral-0", v))
			objs = append(objs, e.fondo, e.texto)
		}
	}
	return objs
}
