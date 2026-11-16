// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/color"
	"strconv"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// «Copiar formato» (la brocha) y «Cambiar mayúsculas y minúsculas» de Inicio › Fuente,
// como `brocha.ts` y `exec('changeCase')` de la web.
//
// La brocha:
//  1. Pulsada con el cursor o una selección en un texto, copia el formato de CARÁCTER de
//     su principio (negrita…, fuente, tamaño, color, resalte) y queda armada: el botón
//     sale pulsado.
//  2. Lo siguiente que se seleccione con el puntero (arrastrar, doble clic, un tirador en
//     el móvil) lo recibe, y la brocha se desarma. Un clic suelto no la gasta.
//  3. Esc la desarma. Pulsarla otra vez pinta la selección de ahora si es otra que la de
//     origen —el camino del teclado: Mayús+flechas y otra vez el botón— y si no, la desarma.
//
// El estilo de párrafo no viaja, como en la web: tiene su galería al lado.

// alternarBrocha es el botón: arma la brocha o, armada, pinta o la desarma.
func (f *Folio) alternarBrocha() {
	if f.brocha != nil {
		if f.haySeleccion() && [2]punto{f.cur, f.anc} != f.brochaDe {
			f.pintarBrocha()
			return
		}
		f.desarmarBrocha()
		return
	}
	if !f.conCursor || !f.deTexto(f.cur.b) {
		return
	}
	a, b := f.orden()
	hasta := b.p
	if a.b != b.b {
		hasta = f.largo(a.b)
	}
	est := f.edicion(a.b).Formato(a.p, max(hasta, a.p))
	f.brocha, f.brochaDe = &est, [2]punto{f.cur, f.anc}
	f.programarFormato()
}

// desarmarBrocha la deja sin pintar nada.
func (f *Folio) desarmarBrocha() {
	if f.brocha == nil {
		return
	}
	f.brocha = nil
	f.programarFormato()
}

// soltarBrocha es el final de un gesto del puntero: con la brocha armada y algo
// seleccionado, pinta.
func (f *Folio) soltarBrocha() {
	if f.brocha != nil && f.haySeleccion() && f.editable() {
		f.pintarBrocha()
	}
}

// pintarBrocha pone lo copiado en la selección, en UN paso de deshacer, y la desarma.
func (f *Folio) pintarBrocha() {
	fuente := *f.brocha
	f.brocha = nil
	props := propsDeBrocha(fuente)
	f.porTramos(func(e *rico.Edicion, d, h int) { e.PegarFormato(d, h, fuente, props) })
}

// propsDeBrocha son las propiedades de CSS de lo copiado, escritas como las escribe
// Chrome con `styleWithCSS` (la web pinta con `execCommand`). PegarFormato solo pone las
// que cambian lo que se ve.
func propsDeBrocha(e rico.Estilo) [][2]string {
	var out [][2]string
	if e.Familia != "" {
		out = append(out, [2]string{"font-family", familiaCSS(e.Familia)})
	}
	if e.Tamano > 0 {
		t := tamanoCSS(pasoDeTamano(e.Tamano))
		if t == "" {
			t = pxTexto(e.Tamano)
		}
		out = append(out, [2]string{"font-size", t})
	}
	out = append(out, [2]string{"color", rgbCSS(e.Color)})
	if e.Fondo.A > 0 {
		out = append(out, [2]string{"background-color", rgbCSS(e.Fondo)})
	}
	return out
}

// rgbCSS escribe un color como lo serializa el navegador: `rgb(r, g, b)`, o `rgba(…)` si
// no es opaco.
func rgbCSS(c color.NRGBA) string {
	rgb := strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B))
	if c.A == 0xff {
		return "rgb(" + rgb + ")"
	}
	return "rgba(" + rgb + ", " + strconv.FormatFloat(float64(c.A)/0xff, 'f', 2, 64) + ")"
}

// cambiarMayusculas es «Cambiar mayúsculas y minúsculas» sobre la selección. Cada bloque
// es un párrafo (frase y palabra nuevas); las runas no cambian de número, así que la
// selección sigue abarcando lo mismo. Sin selección no hace nada, como la web.
func (f *Folio) cambiarMayusculas(valor string) {
	modo, ok := rico.LeerModoMayusculas(valor)
	if !ok || !f.haySeleccion() {
		return
	}
	c := rico.NuevoCambioMayusculas(modo)
	f.porTramos(func(e *rico.Edicion, d, h int) { e.CambiarMayusculas(d, h, c) })
}
