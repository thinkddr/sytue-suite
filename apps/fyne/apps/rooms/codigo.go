// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// En la web: el caso `block` de MessageBody.tsx y `.sytue-code` de la hoja.
//
// Un bloque ``` es una caja `border rounded-md bg-surface-sunken` con su cabecera
// —`border-b px-2.5 py-1`: el lenguaje en `sytue-meta` y «Copiar» en `text-xs`— y el
// código en la letra monoespaciada a `text-sm`, `whitespace-pre` con desplazamiento
// horizontal (`overflow-x-auto px-2.5 py-1.5`).

// caraMono es `--font-mono`: JetBrains Mono, la primera de la pila de la web, que la
// suite lleva embebida en `tema.Mono`.
func caraMono() (fyne.Resource, fyne.TextStyle) { return tema.Mono, fyne.TextStyle{} }

// medirMono es el ancho de un texto en la monoespaciada.
func medirMono(s string, e tema.Escalon) float32 {
	cara, estilo := caraMono()
	m, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, e.Tam, estilo, cara)
	return m.Width
}

// Las medidas del bloque. HOJA.
var (
	// CanalonCodigo es el `px-2.5` de la cabecera y del código.
	CanalonCodigo = tema.Espacio(2.5)
	// RellenoCabeceraCodigo es el `py-1` de la cabecera y RellenoCodigo el `py-1.5`
	// del código.
	RellenoCabeceraCodigo = tema.Espacio(1)
	RellenoCodigo         = tema.Espacio(1.5)
)

// AltoCabeceraCodigo es la cabecera sin su filete: `py-1` y `sytue-meta`.
func AltoCabeceraCodigo() float32 { return 2*RellenoCabeceraCodigo + tema.TextoXS.Linea }

// AltoBloqueCodigo es la caja entera: los dos filetes, la cabecera con el suyo, el
// `py-1.5` y una línea de `text-sm` por renglón.
func AltoBloqueCodigo(texto string) float32 {
	n := len(strings.Split(texto, "\n"))
	return 3*tema.Borde + AltoCabeceraCodigo() + 2*RellenoCodigo + float32(n)*tema.TextoSM.Linea
}

// pintarCodigo arma las piezas de un bloque: la caja, la cabecera y el desplazador con
// el código.
func (c *CuerpoRico) pintarCodigo(b bloqueCodigo, v tema.Variante) []fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("border", v), tema.RadioMD)
	ui.MoverCaja(caja, fyne.NewPos(b.x, b.y), fyne.NewSize(b.ancho, b.alto))
	out := caja.Objetos()
	yCab := b.y + tema.Borde
	filete := canvas.NewRectangle(tema.Color("border", v))
	filete.Move(fyne.NewPos(b.x+tema.Borde, yCab+AltoCabeceraCodigo()))
	filete.Resize(fyne.NewSize(b.ancho-2*tema.Borde, tema.Borde))
	lenguaje := b.lenguaje
	if lenguaje == "" {
		lenguaje = "código"
		if c.T != nil {
			lenguaje = c.T.T("message.code", nil)
		}
	}
	rotulo := ui.Texto(lenguaje, escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	ui.ColocarTexto(rotulo, escalonMeta, b.x+tema.Borde+CanalonCodigo, yCab+RellenoCabeceraCodigo)
	out = append(out, filete, rotulo)
	if c.T != nil {
		copiar := NuevoBotonCopiar(c.T.T("action.copy", nil), func() {
			if c.AlCopiar != nil {
				c.AlCopiar(b.texto)
			}
		})
		m := copiar.MinSize()
		copiar.Move(fyne.NewPos(b.x+b.ancho-tema.Borde-CanalonCodigo-m.Width, yCab+(AltoCabeceraCodigo()-m.Height)/2))
		copiar.Resize(m)
		out = append(out, copiar)
	}
	cara, estilo := caraMono()
	lineas := strings.Split(b.texto, "\n")
	col := container.NewWithoutLayout()
	ancho := float32(0)
	for i, l := range lineas {
		tx := ui.Texto(l, tema.TextoSM, ui.Normal, tema.Color(c.Tinta, v))
		tx.FontSource, tx.TextStyle = cara, estilo
		ui.ColocarTexto(tx, tema.TextoSM, CanalonCodigo, RellenoCodigo+float32(i)*tema.TextoSM.Linea)
		col.Add(tx)
		ancho = max(ancho, medirMono(l, tema.TextoSM))
	}
	alto := 2*RellenoCodigo + float32(len(lineas))*tema.TextoSM.Linea
	col.Resize(fyne.NewSize(ancho+2*CanalonCodigo, alto))
	desplazador := container.NewHScroll(container.New(tamFijo{fyne.NewSize(ancho+2*CanalonCodigo, alto)}, col))
	desplazador.Move(fyne.NewPos(b.x+tema.Borde, yCab+AltoCabeceraCodigo()+tema.Borde))
	desplazador.Resize(fyne.NewSize(b.ancho-2*tema.Borde, alto))
	return append(out, desplazador)
}

// tamFijo declara un mínimo fijo para lo que va dentro de un desplazador.
type tamFijo struct{ fyne.Size }

func (f tamFijo) MinSize([]fyne.CanvasObject) fyne.Size { return f.Size }
func (f tamFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Resize(t)
	}
}

// BotonCopiar es el «Copiar» de la cabecera: `rounded px-1 text-xs text-text-muted
// hover:text-text`, sin caja.
type BotonCopiar struct {
	ui.Pulsable
	Rotulo string
	texto  *canvas.Text
}

var _ desktop.Hoverable = (*BotonCopiar)(nil)

// NuevoBotonCopiar crea el botón.
func NuevoBotonCopiar(rotulo string, al func()) *BotonCopiar {
	b := &BotonCopiar{Rotulo: rotulo}
	b.Nombre, b.AlPulsar = rotulo, al
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es el texto con su `px-1`.
func (b *BotonCopiar) MinSize() fyne.Size {
	return fyne.NewSize(ui.Medir(b.Rotulo, tema.TextoXS, ui.Normal).Width+2*tema.Espacio(1), tema.TextoXS.Linea)
}

func (b *BotonCopiar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.texto = ui.Texto(b.Rotulo, tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	return &botonCopiarRenderer{b: b}
}

type botonCopiarRenderer struct{ b *BotonCopiar }

func (r *botonCopiarRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonCopiarRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.b.texto} }
func (r *botonCopiarRenderer) Destroy()                     {}
func (r *botonCopiarRenderer) Layout(fyne.Size) {
	ui.ColocarTexto(r.b.texto, tema.TextoXS, tema.Espacio(1), 0)
}

func (r *botonCopiarRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := "text-muted"
	if r.b.Encima() || r.b.Enfocado() {
		tinta = "text"
	}
	r.b.texto.Color = tema.Color(tinta, v)
	r.b.texto.Refresh()
}
