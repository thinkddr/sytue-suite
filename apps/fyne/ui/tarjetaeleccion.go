// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/ChoiceCard.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TarjetaEleccion es `ChoiceCard.tsx`: una opción de un grupo de radios que dice la
// CONSECUENCIA de elegirla —título, una o dos frases y, si hace falta, una nota—.
// Elegida, el chevron de la izquierda pasa a ser un tic.
//
// Medida en producción en «Tus datos» del Hub (`dense`, 588 de ancho): 59,64 de alto =
// 1 de borde + 10,5 de relleno + 19,25 del título + 1,75 + 15,64 de la descripción +
// 10,5 + 1; el icono de 18 en x = 11,5 y el texto en x = 40.
//
// ponytail: un grupo de radios del navegador se recorre con las FLECHAS; aquí cada
// tarjeta es su propia parada de tabulación y se elige con Espacio o Intro. Llega a
// todo con el teclado; lo que falta es el atajo. Se hace cuando un grupo lo pida, con
// un `AlMover` como el de `Boton`.
type TarjetaEleccion struct {
	widget.BaseWidget

	Titulo      string
	Descripcion string
	// Nota es el `footnote`: la línea menor de debajo, en `sytue-meta`.
	Nota string
	// Insignia es el `badge`: la etiqueta que va detrás del título («Lo más habitual»),
	// en la misma fila si cabe y debajo si no (`flex flex-wrap items-center gap-2`).
	Insignia     string
	TonoInsignia Tono
	// Elegida es `selected`. El estado es de quien pinta el grupo, como en la web.
	Elegida bool
	// Icono es el `icon` de la web: el de sin elegir. El cero es su `chevronRight` por
	// defecto; compartir en Drive pone el globo («Solo ver») y el más («Ver y añadir»).
	Icono iconos.Icono
	// Densa es `dense`: `p-3` en vez de `p-4`.
	Densa bool
	// Inerte es `disabled`: atenuada y sin responder.
	Inerte bool
	// Ancho es al que se reparten los textos y el que se declara, como `Parrafo.A`: el
	// alto depende de él.
	Ancho    float32
	AlElegir func()

	encima, enfocado bool
}

var (
	_ fyne.Widget       = (*TarjetaEleccion)(nil)
	_ fyne.Tappable     = (*TarjetaEleccion)(nil)
	_ fyne.Focusable    = (*TarjetaEleccion)(nil)
	_ fyne.Accessible   = (*TarjetaEleccion)(nil)
	_ desktop.Hoverable = (*TarjetaEleccion)(nil)
)

// Medidas de ChoiceCard.tsx.
const (
	// LadoIconoEleccion es el `<Icon size={18}>`, y DesplazaIconoEleccion su `mt-0.5`.
	LadoIconoEleccion     float32 = 18
	DesplazaIconoEleccion float32 = 0.5 * 0.25 * tema.Rem
	// HuecoTarjetaEleccion es el `gap-3` entre el icono y los textos.
	HuecoTarjetaEleccion float32 = 3 * 0.25 * tema.Rem
	// MargenDescEleccion es el `mt-0.5` de la descripción y MargenNotaEleccion el
	// `mt-1` de la nota.
	MargenDescEleccion float32 = 0.5 * 0.25 * tema.Rem
	MargenNotaEleccion float32 = 1 * 0.25 * tema.Rem
)

// descEleccion es el `text-sm leading-snug` de la descripción: 1,375 del tamaño.
var descEleccion = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}

// NuevaTarjetaEleccion crea la opción.
func NuevaTarjetaEleccion(titulo, descripcion string, alElegir func()) *TarjetaEleccion {
	c := &TarjetaEleccion{Titulo: titulo, Descripcion: descripcion, AlElegir: alElegir}
	c.ExtendBaseWidget(c)
	return c
}

// A fija el ancho de reparto, para encadenarlo.
func (c *TarjetaEleccion) A(ancho float32) *TarjetaEleccion { c.Ancho = ancho; return c }

func (c *TarjetaEleccion) relleno() float32 {
	if c.Densa {
		return 3 * 0.25 * tema.Rem
	}
	return 4 * 0.25 * tema.Rem
}

// cabe es lo que le queda a los textos: la tarjeta menos borde, relleno, icono y hueco.
func (c *TarjetaEleccion) cabe(ancho float32) float32 {
	return max(ancho-2*(tema.Borde+c.relleno())-LadoIconoEleccion-HuecoTarjetaEleccion, 0)
}

// HuecoInsignia es el `gap-2` de la fila del título: entre él y la insignia, y entre
// filas cuando la insignia no cabe.
var HuecoInsignia = tema.Espacio(2)

// filaTitulo dice cómo va la fila del título: su alto, si la insignia cabe al lado de
// un título de una línea, y cuánto mide ella.
func (c *TarjetaEleccion) filaTitulo(titulo []string, cabe float32) (alto float32, alLado bool, ins fyne.Size) {
	alto = float32(len(titulo)) * tema.TextoBase.Linea
	if c.Insignia == "" {
		return alto, false, fyne.Size{}
	}
	ins = MedidaInsignia(c.Insignia)
	if len(titulo) == 1 && Medir(titulo[0], tema.TextoBase, SemiNegrita).Width+HuecoInsignia+ins.Width <= cabe {
		return max(alto, ins.Height), true, ins
	}
	return alto + HuecoInsignia + ins.Height, false, ins
}

// lineas reparte los tres textos al ancho de la tarjeta.
func (c *TarjetaEleccion) lineas(ancho float32) (titulo, desc, nota []string) {
	cabe := c.cabe(ancho)
	titulo = Repartir(c.Titulo, tema.TextoBase, SemiNegrita, cabe)
	if c.Descripcion != "" {
		desc = Repartir(c.Descripcion, descEleccion, Normal, cabe)
	}
	if c.Nota != "" {
		nota = Repartir(c.Nota, tema.TextoXS, Normal, cabe)
	}
	return titulo, desc, nota
}

// AltoTarjetaEleccion es lo que ocupa a ese ancho. Es la ÚNICA cuenta: el renderer
// coloca recorriendo lo mismo.
func AltoTarjetaEleccion(c *TarjetaEleccion, ancho float32) float32 {
	titulo, desc, nota := c.lineas(ancho)
	alto, _, _ := c.filaTitulo(titulo, c.cabe(ancho))
	if len(desc) > 0 {
		alto += MargenDescEleccion + float32(len(desc))*descEleccion.Linea
	}
	if len(nota) > 0 {
		alto += MargenNotaEleccion + float32(len(nota))*tema.TextoXS.Linea
	}
	return 2*(tema.Borde+c.relleno()) + max(alto, DesplazaIconoEleccion+LadoIconoEleccion)
}

// MinSize declara el ancho de reparto y el alto que le corresponde.
func (c *TarjetaEleccion) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(c.Ancho, AltoTarjetaEleccion(c, c.Ancho))
}

// Tapped elige. Como un radio, pulsar la ya elegida no hace nada.
func (c *TarjetaEleccion) Tapped(*fyne.PointEvent) {
	if c.Inerte {
		return
	}
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		enfocar(cv, c)
	}
	if c.Elegida || c.AlElegir == nil {
		return
	}
	c.AlElegir()
}

// FocusGained enciende el aro.
func (c *TarjetaEleccion) FocusGained() { c.enfocado = true; c.Refresh() }

// FocusLost lo apaga.
func (c *TarjetaEleccion) FocusLost() { c.enfocado = false; c.Refresh() }

// TypedRune elige con la barra espaciadora, como el radio nativo que lleva dentro.
func (c *TarjetaEleccion) TypedRune(r rune) {
	if r == ' ' {
		c.Tapped(nil)
	}
}

// TypedKey elige también con Intro.
func (c *TarjetaEleccion) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
		c.Tapped(nil)
		return
	}
	SubirEscape(c, k)
}

// MouseIn, MouseMoved y MouseOut llevan el `hover:border-border-strong`.
func (c *TarjetaEleccion) MouseIn(*desktop.MouseEvent) { c.encima = true; c.Refresh() }

// MouseMoved no cambia nada.
func (c *TarjetaEleccion) MouseMoved(*desktop.MouseEvent) {}

// MouseOut lo quita.
func (c *TarjetaEleccion) MouseOut() { c.encima = false; c.Refresh() }

// AccessibilityLabel es el título con su consecuencia.
func (c *TarjetaEleccion) AccessibilityLabel() string {
	if c.Descripcion == "" {
		return c.Titulo
	}
	return c.Titulo + ". " + c.Descripcion
}

// AccessibilityRole: el mismo techo que `Eleccion` —Fyne 2.8 no tiene rol de radio—.
func (c *TarjetaEleccion) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

// CreateRenderer compone la tarjeta. Los objetos se crean UNA vez y cada Refresh los
// repinta: rehacerlos al pasar el puntero le quitaría el foco a quien usa el teclado.
func (c *TarjetaEleccion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaEleccionRenderer{
		c:       c,
		aro:     NuevoAro(tema.RadioLG, ConHueco, v),
		caja:    NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioLG),
		tic:     iconos.Trazo(iconos.Tic, LadoIconoEleccion, "primary-text", v),
		chevron: iconos.Trazo(c.icono(), LadoIconoEleccion, "text-subtle", v),
		titulo:  NuevoParrafo("", tema.TextoBase, SemiNegrita, "text", v),
		ins:     NuevaInsignia("", c.TonoInsignia),
		desc:    NuevoParrafo("", descEleccion, Normal, "text-muted", v),
		nota:    NuevoParrafo("", tema.TextoXS, Normal, "text-subtle", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.tic.Objeto(), r.chevron.Objeto(), r.titulo, r.desc, r.nota, r.ins)
	r.Refresh()
	return r
}

func (c *TarjetaEleccion) icono() iconos.Icono {
	if c.Icono == iconos.Ninguno {
		return iconos.ChevronDerecha
	}
	return c.Icono
}

// tarjetaEleccionRenderer lleva SIEMPRE los dos iconos y los tres textos y enseña lo
// que toca, como `Eleccion`: así la lista de objetos no cambia con el estado.
type tarjetaEleccionRenderer struct {
	c                  *TarjetaEleccion
	aro                *Aro
	caja               *Caja
	tic, chevron       *iconos.Dibujo
	titulo, desc, nota *Parrafo
	ins                *Insignia
	objetos            []fyne.CanvasObject
}

func (r *tarjetaEleccionRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *tarjetaEleccionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaEleccionRenderer) Destroy()                     {}

// tinta es el color de un token con el `opacity-60` de la opción inerte encima.
func (r *tarjetaEleccionRenderer) tinta(token string, v tema.Variante) color.NRGBA {
	c := tema.Color(token, v)
	if r.c.Inerte {
		return atenuarCon(c, 0.6)
	}
	return c
}

// Refresh repinta: `border-primary` elegida, `border-border-strong` al pasar por encima
// y `border-border` en reposo, sobre `bg-surface-raised`; inerte, `bg-bg-subtle`.
func (r *tarjetaEleccionRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := "surface-raised", "border"
	switch {
	case c.Inerte:
		fondo = "bg-subtle"
	case c.Elegida:
		borde = "primary"
	case c.encima:
		borde = "border-strong"
	}
	r.caja.Pintar(r.tinta(fondo, v), r.tinta(borde, v))
	r.aro.Pintar(v)
	r.aro.Ver(c.enfocado)
	r.tic.TenirCon(r.tinta("primary-text", v))
	r.chevron.TenirCon(r.tinta("text-subtle", v))
	r.tic.Ver(c.Elegida)
	r.chevron.Ver(!c.Elegida)
	for _, x := range []struct {
		p     *Parrafo
		texto string
		token string
	}{{r.titulo, c.Titulo, "text"}, {r.desc, c.Descripcion, "text-muted"}, {r.nota, c.Nota, "text-subtle"}} {
		x.p.Texto, x.p.Tinta = x.texto, r.tinta(x.token, v)
		ver(x.p, x.texto != "")
		x.p.Refresh()
	}
	r.ins.Texto, r.ins.Tono = c.Insignia, c.TonoInsignia
	ver(r.ins, c.Insignia != "")
	r.ins.Refresh()
	r.Layout(c.Size())
}

func (r *tarjetaEleccionRenderer) Layout(t fyne.Size) {
	c := r.c
	if t.Width <= 0 {
		t.Width = c.Ancho
	}
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	m := tema.Borde + c.relleno()
	r.tic.Colocar(m, m+DesplazaIconoEleccion, LadoIconoEleccion)
	r.chevron.Colocar(m, m+DesplazaIconoEleccion, LadoIconoEleccion)

	x, y, cabe := m+LadoIconoEleccion+HuecoTarjetaEleccion, m, c.cabe(t.Width)
	for i, p := range []*Parrafo{r.titulo, r.desc, r.nota} {
		if p.Texto == "" {
			continue
		}
		if i > 0 {
			y += []float32{0, MargenDescEleccion, MargenNotaEleccion}[i]
		}
		h := AltoParrafo(p, cabe)
		if i == 0 && c.Insignia != "" {
			fila, alLado, ins := c.filaTitulo(Repartir(c.Titulo, tema.TextoBase, SemiNegrita, cabe), cabe)
			if alLado {
				// `items-center`: título e insignia centrados en la fila.
				p.Move(fyne.NewPos(x, y+(fila-h)/2))
				p.Resize(fyne.NewSize(cabe, h))
				ancho := Medir(c.Titulo, tema.TextoBase, SemiNegrita).Width
				r.ins.Move(fyne.NewPos(x+ancho+HuecoInsignia, y+(fila-ins.Height)/2))
			} else {
				p.Move(fyne.NewPos(x, y))
				p.Resize(fyne.NewSize(cabe, h))
				r.ins.Move(fyne.NewPos(x, y+h+HuecoInsignia))
			}
			r.ins.Resize(ins)
			y += fila
			continue
		}
		p.Move(fyne.NewPos(x, y))
		p.Resize(fyne.NewSize(cabe, h))
		y += h
	}
}
