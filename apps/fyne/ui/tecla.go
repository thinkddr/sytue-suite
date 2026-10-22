// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// AtajoDelSistema escribe un atajo como lo lee quien tiene delante el teclado: tal cual
// en Linux y Windows («Ctrl+G», «Ctrl K») y con los símbolos del Mac en macOS e iOS
// («⌘G», «⌘K»), que es lo que hacen `formatShortcut` del ribbon de Docs y el `⌘K` de
// `GlobalSearch.tsx`. Se escribe SIEMPRE con «Ctrl» y pasa por aquí: una pista que dice
// «Ctrl» en un Mac manda a buscar una tecla que no es la del atajo. Lo vigila
// `TestLasPistasDeAtajoPasanPorAtajoDelSistema`.
func AtajoDelSistema(atajo string) string { return atajoPara(runtime.GOOS, atajo) }

// atajoPara es AtajoDelSistema con el sistema como argumento, para probarlo en Linux.
// Rehacer es la excepción: ⌘⇧Z en un Mac, no ⌘Y (la misma que traduce Docs).
func atajoPara(goos, atajo string) string {
	if goos != "darwin" && goos != "ios" {
		return atajo
	}
	return strings.NewReplacer("Ctrl+Y", "⌘⇧Z", "Ctrl+", "⌘", "Ctrl ", "⌘", "Mayús+", "⇧").Replace(atajo)
}

// Tecla es el `<kbd>` de la suite: la pastilla con el nombre de un atajo.
//
// Es DECORACIÓN, siempre: en la web va con `aria-hidden` porque la tecla la nombra el
// `aria-label` o el `title` del control que la lleva. Quien la pinta tiene que seguir
// nombrando el atajo con palabras, no dejarlo en el dibujo.
//
// # Por qué es configurable y no hay una sola pastilla
//
// Porque la web no tiene una sola: `GlobalSearch.tsx` la pinta como píldora perfilada
// sobre el cromo (`⌘K`, `text-[11px]`, borde `chrome-border`), `ChatApp.tsx` la pinta
// cuadrada, más pequeña y con el borde a 40 % de opacidad (`Ctrl+G`, `text-2xs`), y
// `ShortcutsModal.tsx` la pinta con fondo `bg-subtle` y caja mínima de 24. La paridad
// manda sobre la reutilización (`docs/CONVENTIONS.md` §13.2): lo que se comparte es la
// pieza —caja, texto centrado, medida única—, no las medidas.
//
// ponytail: la web pone `font-mono` en dos de las tres y la suite no embebe ninguna
// monoespaciada (solo los cinco Figtree). Se pinta con la cara que se le pida; el día
// que haya una mono en `apps/nucleo/fuentes`, se cambia aquí y en un sitio.
type Tecla struct {
	widget.BaseWidget

	Texto   string
	Escalon tema.Escalon
	Peso    Peso
	// Fondo, Tinta y Borde son TOKENS. Fondo y Borde vacíos dejan la caja
	// transparente y sin perfil.
	Fondo, Tinta, Borde string
	// OpacidadBorde es el `/40` del borde de Rooms. 0 vale por 1.
	OpacidadBorde float32
	// Relleno es el `px-…` y el `py-…`; Radio, el de la caja.
	RellenoX, RellenoY float32
	Radio              float32
	// Minimo es el `min-w-6 h-6` de la ventana de atajos: con 0 la pastilla mide lo
	// que ocupe su texto con el relleno.
	Minimo fyne.Size
}

var _ fyne.Widget = (*Tecla)(nil)

// NuevaTecla crea la pastilla y la extiende. Los campos se rellenan a continuación:
// cada sitio de la web tiene los suyos y no hay un juego por defecto que valga.
func NuevaTecla(texto string, e tema.Escalon, p Peso) *Tecla {
	t := &Tecla{Texto: texto, Escalon: e, Peso: p, Radio: tema.RadioSM}
	t.ExtendBaseWidget(t)
	return t
}

// MedidaTecla es lo que ocupa: el texto con su relleno y el borde por dentro, nunca
// menos que `Minimo`. Es la única cuenta —la usan `MinSize`, el renderer y el guardián.
func MedidaTecla(t *Tecla) fyne.Size {
	borde := float32(0)
	if t.Borde != "" {
		borde = tema.Borde
	}
	m := fyne.NewSize(
		Medir(t.Texto, t.Escalon, t.Peso).Width+2*(t.RellenoX+borde),
		t.Escalon.Linea+2*(t.RellenoY+borde))
	return fyne.NewSize(max(m.Width, t.Minimo.Width), max(m.Height, t.Minimo.Height))
}

// MinSize es MedidaTecla.
func (t *Tecla) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return MedidaTecla(t)
}

// colores resuelve los tokens de la pastilla: relleno y filete, transparentes si no hay.
func (t *Tecla) colores(v tema.Variante) (relleno, borde color.NRGBA) {
	if t.Fondo != "" {
		relleno = tema.Color(t.Fondo, v)
	}
	if t.Borde != "" {
		borde = tema.Color(t.Borde, v)
		if o := t.OpacidadBorde; o > 0 && o < 1 {
			borde = tokens.ConAlfa(borde, float64(o))
		}
	}
	return relleno, borde
}

// CreateRenderer compone caja y rótulo. Termina en Refresh (regla 7 del paquete).
//
// La caja se construye YA con sus colores, no transparente para pintarla después:
// `Caja` decide al nacer si lleva filete (`borde.A != 0`) y no lo cambia en `Pintar`.
// Nacida transparente, la pastilla de Rooms salía sin borde; lo cazó su guardián.
func (t *Tecla) CreateRenderer() fyne.WidgetRenderer {
	relleno, borde := t.colores(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r := &teclaRenderer{
		t:      t,
		caja:   NuevaCaja(relleno, borde, t.Radio),
		rotulo: Texto(t.Texto, t.Escalon, t.Peso, color.NRGBA{}),
	}
	r.objetos = append(r.caja.Objetos(), r.rotulo)
	r.Refresh()
	return r
}

type teclaRenderer struct {
	t       *Tecla
	caja    *Caja
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *teclaRenderer) MinSize() fyne.Size           { return MedidaTecla(r.t) }
func (r *teclaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *teclaRenderer) Destroy()                     {}

// Layout centra el rótulo en la pastilla: con `Minimo` puesto el texto no empieza en el
// relleno, va al medio (`justify-center` de la ventana de atajos).
func (r *teclaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ancho := Medir(r.t.Texto, r.t.Escalon, r.t.Peso).Width
	ColocarTexto(r.rotulo, r.t.Escalon, (t.Width-ancho)/2, (t.Height-r.t.Escalon.Linea)/2)
}

func (r *teclaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(r.t.colores(v))
	r.rotulo.Text = r.t.Texto
	r.rotulo.Color = tema.Color(r.t.Tinta, v)
	r.rotulo.Refresh()
	r.Layout(r.t.Size())
}
