// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/nucleo/saneado"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/portapapeles"
)

// # Pegar con formato
//
// ⌘/Ctrl + V pega lo copiado en Word, LibreOffice o un navegador con su formato, como el
// contenteditable de la web: el HTML del portapapeles del sistema (`portapapeles.HTML`,
// el mismo que lee el redactor de Mail) se pasa por la regla de saneado del documento
// (`apps/nucleo/saneado`, la que aplica el servicio al guardar), se reduce a lo que el
// folio entiende (`rico.ReducirPegado`) y entra en el cursor en un solo paso de deshacer.
// Sin HTML, con ⌘/Ctrl + Mayús + V, o donde el formato no cabe —el encabezado y el pie,
// una caja de otro lienzo, el control de cambios, varios bloques dentro de una celda—, se
// pega el texto, como hasta ahora.
//
// El HTML se lee, se sanea y se reduce fuera del hilo de la interfaz: en X11, si quien
// copió es esta misma ventana, su bucle tiene que poder contestar; y un documento de Word
// copiado entero son megas, que sanearlos ahí congelaría la ventana.

// leerHTML es portapapeles.HTML. Las pruebas lo cambian por el HTML que copiaría otra
// aplicación.
func (f *Folio) leerHTML() (string, error) {
	if f.LeerHTML != nil {
		return f.LeerHTML()
	}
	return portapapeles.HTML()
}

func (f *Folio) lanzar(hacer func()) {
	if f.Lanzar != nil {
		f.Lanzar(hacer)
		return
	}
	go hacer()
}

// pegar es ⌘/Ctrl + V: con formato si el portapapeles lo trae y aquí cabe; si no, el texto.
func (f *Folio) pegar(texto string) {
	if !f.editable() {
		return
	}
	if f.caja != nil || f.enRegion() || f.rastreando() {
		f.pegarTexto(texto)
		return
	}
	f.lanzar(func() {
		h, err := f.leerHTML()
		var p rico.Pegado
		if err == nil {
			p = rico.ReducirPegado(saneado.HTML(h))
		}
		fyne.Do(func() {
			if !f.editable() {
				return // pasó a solo lectura mientras se leía
			}
			if err != nil || !f.pegarHTML(p) {
				f.pegarTexto(texto)
			}
		})
	})
}

// pegarTexto es pegar sin formato: lo de siempre, con lo que la caja de otro lienzo haga
// con lo pegado.
func (f *Folio) pegarTexto(texto string) {
	if f.caja != nil && f.caja.AlPegar != nil {
		texto = f.caja.AlPegar(texto)
	}
	f.escribir(texto)
}

// pegarHTML pega un HTML de otra aplicación, ya saneado y reducido, con su formato, y dice
// si lo ha hecho. No lo hace si no queda nada que pegar, si lo que queda no se entendería
// (el último cerrojo: el folio seguiría siendo editable después) o si son bloques dentro
// de una celda, donde InsertarHTML los llevaría detrás de la tabla.
func (f *Folio) pegarHTML(p rico.Pegado) bool {
	if p.HTML == "" || !rico.ParseCon(p.HTML, f.hoja()).Editable() {
		return false
	}
	enCelda := f.cur.b < len(f.bloques) && f.bloques[f.cur.b].Celda != nil
	if !p.EnLinea && enCelda {
		return false
	}
	f.antesDeCambiar(false)
	f.borrarSeleccion()
	if p.EnLinea {
		e := f.edicion(f.cur.b)
		n := 0
		if e != nil && f.deTexto(f.cur.b) {
			n = e.PegarEnLinea(f.cur.p, p.HTML)
		}
		if n == 0 {
			f.deshacerPegado()
			return false
		}
		f.cur.p += n
		f.anc = f.cur
		f.reestructurado()
	} else {
		l, ok := f.doc.PegarBloques(f.doc.Bloques(), f.cur.b, f.cur.p, p.HTML)
		if !ok {
			f.deshacerPegado()
			return false
		}
		f.irA(l, 0)
		// Al final de lo pegado. La maqueta aún es la de antes: el largo, del modelo.
		if f.cur.b < len(f.bloques) {
			f.cur.p = f.bloques[f.cur.b].Editar().Largo()
		}
		f.anc = f.cur
		f.reestructurado()
	}
	// Lo pegado y lo que había tienen que seguir leyéndose como un documento editable
	// (una lista pegada junto a otra, un título dentro de algo): si no, fuera, y el texto.
	if !rico.ParseCon(f.doc.HTML(), f.hoja()).Editable() {
		f.deshacerPegado()
		return false
	}
	return true
}

// deshacerPegado vuelve a la foto de antes de pegar sin dejarla para rehacer.
func (f *Folio) deshacerPegado() {
	f.deshacer()
	if n := len(f.hist.adelante); n > 0 {
		f.hist.adelante = f.hist.adelante[:n-1]
	}
}
