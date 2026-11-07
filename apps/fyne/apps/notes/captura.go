// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"math"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

// Paridad web: la CAPTURA —client/captura.ts y el `paste` y el `drop` de NoteCanvas.tsx—.
// Lo que se pega o se suelta sobre el cuaderno se vuelve nota: una URL sola, nota de
// enlace; un texto, nota de texto. Solo texto plano, como la web: el portapapeles de Fyne
// no da otra cosa, y lo soltado llega por `soltar.AlSoltar` como `text/plain` o como
// `text/uri-list`, que son los dos tipos que mira la web.

// interpretarCaptura es la nota que crea un texto pegado (apps/nucleo/notas).
var interpretarCaptura = notas.InterpretarCaptura

// interpretarSoltado es `interpretarCaptura(text/plain, text/uri-list)` de la web: el
// primer enlace web de la lista manda sobre el texto, porque el texto de un enlace
// arrastrado suele ser su título y no su dirección.
func interpretarSoltado(texto string, urls []string) (api.NoteInput, bool) {
	for _, u := range urls {
		if in, ok := interpretarCaptura(u); ok && in.Kind == "link" {
			return in, true
		}
	}
	// Una lista sin ningún enlace web (un `mailto:`) queda como texto: es lo que el
	// navegador pone además en `text/plain`, y aquí llega solo la lista.
	if texto == "" {
		texto = strings.Join(urls, "\n")
	}
	return interpretarCaptura(texto)
}

// pegar es Cmd/Ctrl+V sobre el cuaderno. En una lista —un día o un cuaderno en lista—
// cada línea es una nota, como al pegar en la captura (pegarLineas); en el lienzo libre
// y en columnas, un texto pegado es UNA tarjeta, que es lo que se ve allí (NoteCanvas.tsx).
func (p *Pantalla) pegar(s fyne.Shortcut) {
	if texto, ok := p.pegadoSobreCuaderno(s); ok && p.enFilas() && p.pegarLineas(texto) {
		return
	}
	if in, ok := p.captura(s); ok {
		lanzar(func() error { return p.crear(in) })
	}
}

// captura es la nota que crearía ese pegado como UNA tarjeta.
func (p *Pantalla) captura(s fyne.Shortcut) (api.NoteInput, bool) {
	texto, ok := p.pegadoSobreCuaderno(s)
	if !ok {
		return api.NoteInput{}, false
	}
	return interpretarCaptura(texto)
}

// pegadoSobreCuaderno es lo que se pega sobre el cuaderno. Pegar DENTRO de algo —un
// campo, el editor, una tarjeta enfocada, un diálogo— sigue siendo pegar ahí: solo
// cuenta cuando nada tiene el foco.
func (p *Pantalla) pegadoSobreCuaderno(s fyne.Shortcut) (string, bool) {
	pg, ok := s.(*fyne.ShortcutPaste)
	if !ok || pg.Clipboard == nil || p.win == nil || p.vista != VistaCuaderno || p.activo.ID == "" ||
		p.win.Canvas().Focused() != nil || p.win.Canvas().Overlays().Top() != nil || !p.hayRed() {
		return "", false
	}
	return pg.Clipboard.Content(), true
}

// enFilas es el cuaderno pintado en FILAS (`enFilas` de NoteCanvas.tsx): en lista, sea un
// día que se ve como día o un cuaderno corriente; en árbol la nota llena su panel.
func (p *Pantalla) enFilas() bool {
	return p.activo.Vista == Lista && (p.activo.Crudo.DailyDate == "" || p.vistaDiario() == VistaDia)
}

// Soltar es el `drop` del cuaderno (`soltar` de NoteCanvas.tsx): lo soltado se vuelve
// nota, y en el lienzo libre nace DONDE se soltó. Lo llama la ventana con
// `soltar.AlSoltar`, en el hilo de la interfaz; los ficheros no llegan aquí, igual que la
// web solo mira `text/plain` y `text/uri-list`.
func (p *Pantalla) Soltar(texto string, urls []string, pos fyne.Position) {
	if in, ok := p.soltado(texto, urls, pos); ok {
		lanzar(func() error { return p.crear(in) })
	}
}

// soltado es la nota que crearía soltar eso ahí. Solo vale sobre el cuaderno —la zona
// del desplazador, que es la del `onDrop` de la web—, sin nada encima y con red.
func (p *Pantalla) soltado(texto string, urls []string, pos fyne.Position) (api.NoteInput, bool) {
	if p.win == nil || p.desplazador == nil || p.vista != VistaCuaderno || p.activo.ID == "" ||
		p.win.Canvas().Overlays().Top() != nil || !p.hayRed() {
		return api.NoteInput{}, false
	}
	d := fyne.CurrentApp().Driver()
	zona, tam := d.AbsolutePositionForObject(p.desplazador), p.desplazador.Size()
	if pos.X < zona.X || pos.Y < zona.Y || pos.X >= zona.X+tam.Width || pos.Y >= zona.Y+tam.Height {
		return api.NoteInput{}, false
	}
	in, ok := interpretarSoltado(texto, urls)
	if ok && p.activo.Vista == Libre && p.desplazador.Content != nil {
		// `clientX - caja.left` del lienzo, que se mueve con el desplazamiento.
		o := d.AbsolutePositionForObject(p.desplazador.Content)
		in.X = ptr(max(0, int(math.Round(float64(pos.X-o.X)))))
		in.Y = ptr(max(0, int(math.Round(float64(pos.Y-o.Y)))))
	}
	return in, ok
}
