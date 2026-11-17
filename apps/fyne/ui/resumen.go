// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Resumen es `ReviewSummary` de `@sytue/ui`: «esto es lo que vas a aplicar», la lista
// clave → valor del último paso de un asistente o de cualquier confirmación. Cada fila
// puede llevar su «Editar», que devuelve a donde se rellenó ese dato.
//
// La web: `<dl>` con `border rounded-md bg-surface`, `divide-y` en `border-subtle`,
// filas `px-3 py-2.5 gap-3`; la clave en `text-sm text-muted`, el valor en `text-sm`
// (`font-mono text-xs` si es técnico) y, desde `sm:`, una rejilla de
// `minmax(0, 11rem) 1fr auto`. Por debajo es un `flex` en el que la clave mide su texto.
//
// Había DOS en el Hub —`resumen` en `bloques.go` y `resumenAlta` en
// `bienvenida_pasos.go`, uno sin «Editar» y otro con él—, que es justo lo que §2.8
// prohíbe: este es el de la suite.
//
// ponytail: la web alinea las filas por la línea base (`items-baseline`) y aquí van por
// arriba, salvo el puntal del «Editar» (BajoEditarResumen), que sí se cuenta porque
// cambia el alto de la fila. Lo que queda solo se nota en el valor monoespaciado, que es
// `text-xs` junto a la clave de `text-sm`: queda 1,75 px más arriba que en el navegador.
type Resumen struct {
	widget.BaseWidget

	Filas []FilaResumen
	// RotuloEditar es el «Editar» de cada fila que lo lleva (`ui.review.edit`).
	RotuloEditar string
	// Ventana es el ancho de la VENTANA, que es el que decide el `sm:`. Cero es ancha.
	Ventana float32

	claves, valores []*Parrafo
	editar          []*Enlace
	filetes         []*canvas.Rectangle
}

// FilaResumen es un dato del resumen.
type FilaResumen struct {
	Clave, Valor string
	// Mono es el valor técnico (una URL, un identificador): `font-mono text-xs`.
	Mono bool
	// AlEditar devuelve a donde se rellenó el dato. Nil: la fila no lleva «Editar».
	AlEditar func()
}

var (
	_ fyne.Widget     = (*Resumen)(nil)
	_ fyne.Accessible = (*Resumen)(nil)
)

// Medidas de ReviewSummary.tsx.
const (
	// MargenResumenX y MargenResumenY son el `px-3 py-2.5` de cada fila.
	MargenResumenX float32 = 3 * 0.25 * tema.Rem
	MargenResumenY float32 = 2.5 * 0.25 * tema.Rem
	// HuecoResumen es el `gap-3` entre columnas.
	HuecoResumen float32 = 3 * 0.25 * tema.Rem
	// ClaveResumen es el tope de la columna de la clave desde `sm:` (`minmax(0, 11rem)`).
	ClaveResumen float32 = 11 * tema.Rem
	// BajoEditarResumen es lo que baja el texto de una fila CON «Editar»: el botón vive
	// en un `<dd>` sin clase de texto, cuyo puntal es el de `text-base`, y al alinear la
	// fila por la línea base (`items-baseline`) ese puntal empuja hacia abajo todo lo
	// demás. Medido en la web en el resumen de las reglas de Mail: 11,75 − 8,75.
	BajoEditarResumen float32 = 3
)

// NuevoResumen crea el resumen con el «Editar» del catálogo común: escrito a mano salía
// en un solo idioma en los veintiséis.
func NuevoResumen(t *i18n.Translator, filas ...FilaResumen) *Resumen {
	r := &Resumen{Filas: filas}
	if t != nil {
		r.RotuloEditar = t.T("ui.review.edit", nil)
	}
	r.ExtendBaseWidget(r)
	return r
}

// AccessibilityRole: la lista es un contenedor; se leen sus filas.
func (r *Resumen) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel: la lista no tiene nombre propio, como el `<dl>` de la web.
func (r *Resumen) AccessibilityLabel() string { return "" }

// ancha es el `sm:` de la ventana.
func (r *Resumen) ancha() bool { return r.Ventana == 0 || r.Ventana >= CorteSM }

// sincronizar crea o actualiza las piezas de cada fila. Los «Editar» sobreviven a los
// cambios de texto: rehacerlos le quitaría el foco a quien acaba de pulsar uno.
func (r *Resumen) sincronizar(v tema.Variante) {
	for len(r.claves) < len(r.Filas) {
		i := len(r.claves)
		r.claves = append(r.claves, NuevoParrafo("", tema.TextoSM, Normal, "text-muted", v))
		r.valores = append(r.valores, NuevoParrafo("", tema.TextoSM, Normal, "text", v))
		e := NuevoEnlace("", tema.TextoSM, SemiNegrita, "primary-text", nil)
		e.Boton = true // es un `<button>` con cara de enlace
		e.AlPulsar = func() {
			if i < len(r.Filas) && r.Filas[i].AlEditar != nil {
				r.Filas[i].AlEditar()
			}
		}
		r.editar = append(r.editar, e)
		if i > 0 {
			r.filetes = append(r.filetes, canvas.NewRectangle(tema.Color("border-subtle", v)))
		}
	}
	n := len(r.Filas)
	r.claves, r.valores, r.editar = r.claves[:n], r.valores[:n], r.editar[:n]
	r.filetes = r.filetes[:max(n-1, 0)]
	for i, f := range r.Filas {
		r.claves[i].Texto = f.Clave
		r.valores[i].Texto = f.Valor
		r.valores[i].Escalon, r.valores[i].Peso = tema.TextoSM, Normal
		if f.Mono {
			r.valores[i].Escalon, r.valores[i].Peso = tema.TextoXS, Mono
		}
		r.editar[i].Texto = r.RotuloEditar
		// El `<span class="sr-only"> {label}</span>`: «Editar Dominio», no cinco
		// «Editar» iguales para el lector.
		r.editar[i].Nombre = r.RotuloEditar + " " + f.Clave
	}
}

// anchoEditar es la tercera columna (`auto`) de la fila i, con su `gap-3`. Cada fila es
// su PROPIA rejilla (`sm:grid` va en la fila, no en el `<dl>`), así que la columna mide
// el «Editar» de esa fila y nada en las que no lo llevan; pero desde `sm:` el hueco hasta
// esa columna vacía sigue ahí, porque la rejilla tiene tres pistas. Por debajo es un
// `flex`: sin «Editar» no hay tercer hijo ni hueco.
func (r *Resumen) anchoEditar(i int) float32 {
	if r.Filas[i].AlEditar != nil {
		return MedidaEnlace(r.editar[i]).Width + HuecoResumen
	}
	if r.ancha() {
		return HuecoResumen
	}
	return 0
}

// columnas son el ancho de la clave y el del valor en la fila i con ese ancho interior.
func (r *Resumen) columnas(i int, interior float32) (clave, valor float32) {
	if r.ancha() {
		clave = min(ClaveResumen, interior)
	} else {
		clave = min(Medir(r.claves[i].Texto, tema.TextoSM, Normal).Width, interior)
	}
	return clave, max(interior-clave-HuecoResumen-r.anchoEditar(i), 0)
}

func (r *Resumen) altoFila(i int, ancho float32) float32 {
	clave, valor := r.columnas(i, ancho-2*(tema.Borde+MargenResumenX))
	h := max(AltoParrafo(r.claves[i], clave), AltoParrafo(r.valores[i], valor))
	if r.Filas[i].AlEditar != nil {
		// La fila crece con el puntal (33,25 → 36,75 en la web): el texto baja y el
		// alto no baja de la línea de `text-base`.
		h = max(BajoEditarResumen+max(h, MedidaEnlace(r.editar[i]).Height), tema.TextoBase.Linea)
	}
	return 2*MargenResumenY + h
}

// bajada es lo que baja el contenido de la fila i (ver BajoEditarResumen).
func (r *Resumen) bajada(i int) float32 {
	if r.Filas[i].AlEditar != nil {
		return BajoEditarResumen
	}
	return 0
}

// MedidaResumen es lo que ocupa el resumen a ese ancho: el borde, las filas y sus
// filetes. Es la única cuenta (regla 2 de ui.go).
func MedidaResumen(r *Resumen, ancho float32) fyne.Size {
	r.sincronizar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	if len(r.Filas) == 0 {
		return fyne.NewSize(ancho, 0)
	}
	h := 2 * tema.Borde
	for i := range r.Filas {
		if i > 0 {
			h += tema.Borde
		}
		h += r.altoFila(i, ancho)
	}
	return fyne.NewSize(ancho, h)
}

// AltoPara es el alto a ese ancho, para `AltoA` y las secciones.
func (r *Resumen) AltoPara(ancho float32) float32 { return MedidaResumen(r, ancho).Height }

// CreateRenderer compone la caja, los filetes y las filas. Termina en Refresh.
func (r *Resumen) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x := &resumenRenderer{r: r, caja: NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)}
	x.Refresh()
	return x
}

type resumenRenderer struct {
	r       *Resumen
	caja    *Caja
	objetos []fyne.CanvasObject
}

// MinSize: el ancho del «Editar» más ancho con los márgenes, y el alto al ancho que
// tenga.
func (x *resumenRenderer) MinSize() fyne.Size {
	r := x.r
	var editar float32
	for i := range r.Filas {
		editar = max(editar, r.anchoEditar(i))
	}
	minimo := 2*(tema.Borde+MargenResumenX) + editar
	return MedidaResumen(r, max(r.Size().Width, minimo))
}
func (x *resumenRenderer) Objects() []fyne.CanvasObject { return x.objetos }
func (x *resumenRenderer) Destroy()                     {}

func (x *resumenRenderer) Layout(t fyne.Size) {
	r := x.r
	alto := MedidaResumen(r, t.Width).Height
	x.caja.Redimensionar(fyne.NewSize(t.Width, alto))
	interior := t.Width - 2*(tema.Borde+MargenResumenX)
	cx := tema.Borde + MargenResumenX
	fy := tema.Borde
	for i := range r.Filas {
		if i > 0 {
			r.filetes[i-1].Move(fyne.NewPos(tema.Borde, fy))
			r.filetes[i-1].Resize(fyne.NewSize(t.Width-2*tema.Borde, tema.Borde))
			fy += tema.Borde
		}
		clave, valor := r.columnas(i, interior)
		y := fy + MargenResumenY + r.bajada(i)
		r.claves[i].Move(fyne.NewPos(cx, y))
		r.claves[i].Resize(fyne.NewSize(clave, AltoParrafo(r.claves[i], clave)))
		r.valores[i].Move(fyne.NewPos(cx+clave+HuecoResumen, y))
		r.valores[i].Resize(fyne.NewSize(valor, AltoParrafo(r.valores[i], valor)))
		if r.Filas[i].AlEditar != nil {
			m := MedidaEnlace(r.editar[i])
			r.editar[i].Move(fyne.NewPos(t.Width-tema.Borde-MargenResumenX-m.Width, y))
			r.editar[i].Resize(m)
		}
		fy += r.altoFila(i, t.Width)
	}
}

// Refresh repinta con el tema en curso y rehace la lista de objetos (las filas pueden
// haber cambiado).
func (x *resumenRenderer) Refresh() {
	r := x.r
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sincronizar(v)
	x.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	x.objetos = append([]fyne.CanvasObject{}, x.caja.Objetos()...)
	for _, f := range r.filetes {
		f.FillColor = tema.Color("border-subtle", v)
		f.Refresh()
		x.objetos = append(x.objetos, f)
	}
	for i, f := range r.Filas {
		r.claves[i].Tinta = tema.Color("text-muted", v)
		r.valores[i].Tinta = tema.Color("text", v)
		r.claves[i].Refresh()
		r.valores[i].Refresh()
		x.objetos = append(x.objetos, r.claves[i], r.valores[i])
		if f.AlEditar != nil {
			r.editar[i].Refresh()
			x.objetos = append(x.objetos, r.editar[i])
		}
	}
	x.Layout(r.Size())
}
