// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Kbd.tsx

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

// Tecla es el `<kbd>` de la suite: la pastilla con el nombre de un atajo. Es la `Kbd`
// de `@sytue/ui`.
//
// Es DECORACIÓN, siempre: en la web va con `aria-hidden` porque la tecla la nombra el
// `aria-label` o el `title` del control que la lleva. Quien la pinta tiene que seguir
// nombrando el atajo con palabras, no dejarlo en el dibujo.
//
// # Dos presets, por el fondo en el que se posa
//
// La web tenía una receta de `<kbd>` por sitio y ahora tiene dos tonos de una sola
// medida (`h-5 min-w-5 px-1.5 rounded border text-xs`, cifras tabulares): `surface`
// sobre el papel y `chrome` sobre el cromo. Aquí son TeclaSuperficie y TeclaCromo, y
// las teclas de atajo se crean con ellas.
//
// Los campos siguen abiertos porque la misma pieza —caja, texto centrado, medida
// única— sirve a pastillas que no son teclas (la cuenta de módulos del ERP); una TECLA
// que necesite otros colores o medidas es un tercer preset aquí y un tercer tono en
// `Kbd.tsx`, no unos campos retocados en el sitio que la usa.
//
// La web pinta las teclas con la cara de la suite, no con una mono: Fyne no embebe
// ninguna (solo los cinco Figtree) y la paridad manda.
type Tecla struct {
	widget.BaseWidget

	Texto   string
	Escalon tema.Escalon
	Peso    Peso
	// Fondo, Tinta y Borde son TOKENS. Fondo y Borde vacíos dejan la caja
	// transparente y sin perfil.
	Fondo, Tinta, Borde string
	// Debajo es el token sobre el que se compone Fondo cuando Fondo es un velo
	// translúcido (el `bg-chrome-hover` de un botón del carril): `Caja` pinta el filete
	// DEBAJO del relleno, y un relleno translúcido lo dejaría ver entero.
	Debajo string
	// Relleno es el `px-…` y el `py-…`; Radio, el de la caja.
	RellenoX, RellenoY float32
	Radio              float32
	// Minimo es el `h-5 min-w-5` de los presets: con 0 la pastilla mide lo que ocupe
	// su texto con el relleno.
	Minimo fyne.Size
}

var _ fyne.Widget = (*Tecla)(nil)

// NuevaTecla crea la pastilla desnuda y la extiende; los campos se rellenan a
// continuación. Para una tecla de atajo, TeclaSuperficie o TeclaCromo.
func NuevaTecla(texto string, e tema.Escalon, p Peso) *Tecla {
	t := &Tecla{Texto: texto, Escalon: e, Peso: p, Radio: tema.RadioSM}
	t.ExtendBaseWidget(t)
	return t
}

// LadoTecla es el `h-5 min-w-5` de los dos presets: una letra suelta sale cuadrada.
var LadoTecla = tema.Espacio(5)

// teclaDeAtajo es lo común a los dos presets: `text-xs` normal, `px-1.5`, `rounded`
// (el radio `sm`) y la caja mínima de LadoTecla.
func teclaDeAtajo(texto string) *Tecla {
	t := NuevaTecla(texto, tema.TextoXS, Normal)
	t.RellenoX, t.Radio = tema.Espacio(1.5), tema.RadioSM
	t.Minimo = fyne.NewSize(LadoTecla, LadoTecla)
	return t
}

// TeclaSuperficie es `<Kbd>` (tono `surface`): la tecla sobre el papel —diálogos, pies
// de paleta, barras de herramientas—, con fondo `bg-subtle`, filete `border` y tinta
// `text-muted`.
func TeclaSuperficie(texto string) *Tecla {
	t := teclaDeAtajo(texto)
	t.Fondo, t.Tinta, t.Borde = "bg-subtle", "text-muted", "border"
	return t
}

// TeclaCromo es `<Kbd tone="chrome">`: la tecla sobre la barra o el menú lateral, con
// filete `chrome-border` y tinta `on-chrome-muted`.
//
// La web no le pone fondo y deja ver el cromo. Aquí `Caja` pinta el filete como un
// rectángulo RELLENO debajo del de dentro, así que sin fondo la pastilla saldría maciza
// del color del filete: el fondo es el del carril (TokenCarril), que es lo que hay
// detrás en el menú. Quien la posa sobre otra cosa —el campo del buscador— cambia
// `Fondo` por el token de lo que tiene debajo, y solo ese.
func TeclaCromo(texto string) *Tecla {
	t := teclaDeAtajo(texto)
	t.Fondo, t.Tinta, t.Borde = TokenCarril(), "on-chrome-muted", "chrome-border"
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
		if t.Debajo != "" {
			relleno = tokens.SobreSRGB(relleno, tema.Color(t.Debajo, v))
		}
	}
	if t.Borde != "" {
		borde = tema.Color(t.Borde, v)
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
