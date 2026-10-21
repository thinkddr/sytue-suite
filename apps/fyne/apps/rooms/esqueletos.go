// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los dos esqueletos de Rooms: el del carril mientras llegan los canales y el del
// historial mientras llegan los mensajes. Los bloques son `ui.Esqueleto`, con su pulso;
// lo que es de Rooms es CUÁNTOS hay y con qué geometría, que es la de la fila real a
// la que sustituyen —sin eso el layout salta al llegar los datos—.
//
// Son widgets y no un contenedor por el `aria-busy` con su `sr-only`: en la web los
// bloques son `aria-hidden` y quien anuncia la carga es el contenedor. Aquí lo anuncia
// el `AccessibilityLabel`.

// Medidas del esqueleto del carril (`ChatApp.tsx`): tres `h-9 rounded-md
// bg-sidebar-hover` en una columna `gap-1 px-0.5`. HOJA.
var (
	AltoEsqueletoCanal   = tema.Espacio(9)
	HuecoEsqueletoCanal  = tema.Espacio(1)
	MargenEsqueletoCanal = tema.Espacio(0.5)
	// CuantosEsqueletoCanal son los tres de `[0, 1, 2].map`.
	CuantosEsqueletoCanal = 3
)

// Medidas del esqueleto del historial (`MessageView.tsx`): `px-4 py-3` y cinco filas
// `mt-5 flex gap-3`, cada una con un cuadro de `2rem` y una columna `gap-2 pt-1` con
// dos barras de `0.75rem`: la del nombre de `7rem` y la del texto al 70, 48, 62, 40 y
// 55 % del ancho. HOJA.
var (
	ArribaEsqueletoMensajes          = tema.Espacio(3)
	AireEsqueletoMensaje             = tema.Espacio(5)
	LadoEsqueletoAvatar      float32 = 2 * tema.Rem
	HuecoEsqueletoMensaje            = tema.Espacio(3)
	ArribaEsqueletoTexto             = tema.Espacio(1)
	HuecoEsqueletoTexto              = tema.Espacio(2)
	AltoEsqueletoTexto       float32 = 0.75 * tema.Rem
	AnchoEsqueletoNombre     float32 = 7 * tema.Rem
	PartesEsqueletoMensajes          = []float32{0.70, 0.48, 0.62, 0.40, 0.55}
	altoFilaEsqueletoMensaje         = max(LadoEsqueletoAvatar,
		ArribaEsqueletoTexto+2*AltoEsqueletoTexto+HuecoEsqueletoTexto)
)

// EsqueletoCanales son los tres bloques del carril.
type EsqueletoCanales struct {
	widget.BaseWidget

	// Nombre es lo que se anuncia: «Cargando canales…», ya traducido.
	Nombre string

	bloques []fyne.CanvasObject
}

var _ fyne.Widget = (*EsqueletoCanales)(nil)

// NuevoEsqueletoCanales crea los tres bloques.
func NuevoEsqueletoCanales(nombre string) *EsqueletoCanales {
	e := &EsqueletoCanales{Nombre: nombre}
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityLabel es el `sr-only` con `role="status"`.
func (e *EsqueletoCanales) AccessibilityLabel() string { return e.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (e *EsqueletoCanales) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// MedidaEsqueletoCanales es lo que ocupa de alto. Es la única cuenta.
func MedidaEsqueletoCanales() fyne.Size {
	n := float32(CuantosEsqueletoCanal)
	return fyne.NewSize(0, n*AltoEsqueletoCanal+(n-1)*HuecoEsqueletoCanal)
}

// CreateRenderer monta los bloques, teñidos para el cromo oscuro.
func (e *EsqueletoCanales) CreateRenderer() fyne.WidgetRenderer {
	e.bloques = nil
	for range CuantosEsqueletoCanal {
		e.bloques = append(e.bloques, ui.NuevoEsqueleto(0, AltoEsqueletoCanal).SobreCromo())
	}
	r := &esqCanalesRenderer{e: e}
	r.Refresh()
	return r
}

type esqCanalesRenderer struct{ e *EsqueletoCanales }

func (r *esqCanalesRenderer) MinSize() fyne.Size           { return MedidaEsqueletoCanales() }
func (r *esqCanalesRenderer) Objects() []fyne.CanvasObject { return r.e.bloques }
func (r *esqCanalesRenderer) Destroy()                     {}

func (r *esqCanalesRenderer) Layout(t fyne.Size) {
	y := float32(0)
	for _, b := range r.e.bloques {
		b.Move(fyne.NewPos(MargenEsqueletoCanal, y))
		b.Resize(fyne.NewSize(max(t.Width-2*MargenEsqueletoCanal, 0), AltoEsqueletoCanal))
		y += AltoEsqueletoCanal + HuecoEsqueletoCanal
	}
}

func (r *esqCanalesRenderer) Refresh() { r.Layout(r.e.Size()) }

// EsqueletoMensajes son las cinco filas del historial. El hilo usa la misma pieza con
// dos filas y otros aires (`NuevoEsqueletoHilo`).
type EsqueletoMensajes struct {
	widget.BaseWidget

	// Nombre es lo que se anuncia: «Cargando mensajes…», ya traducido.
	Nombre string
	// Partes es el ancho de cada barra de texto; AnchoNombre, el de la del nombre.
	Partes      []float32
	AnchoNombre float32
	// Arriba es el aire de encima de todo, Aire el de antes de cada fila y Canalon el
	// de los lados. SinAireArriba quita el Aire de la PRIMERA fila, que es lo que hace
	// un `gap` frente a un `mt`.
	Arriba, Aire, Canalon float32
	SinAireArriba         bool

	bloques []fyne.CanvasObject
}

var _ fyne.Widget = (*EsqueletoMensajes)(nil)

// NuevoEsqueletoMensajes crea las cinco filas.
func NuevoEsqueletoMensajes(nombre string) *EsqueletoMensajes {
	e := &EsqueletoMensajes{
		Nombre: nombre, Partes: PartesEsqueletoMensajes, AnchoNombre: AnchoEsqueletoNombre,
		Arriba: ArribaEsqueletoMensajes, Aire: AireEsqueletoMensaje,
	}
	e.ExtendBaseWidget(e)
	return e
}

// aireDe es el aire de antes de la fila i.
func (e *EsqueletoMensajes) aireDe(i int) float32 {
	if i == 0 && e.SinAireArriba {
		return 0
	}
	return e.Aire
}

// AccessibilityLabel es el `sr-only` con `role="status"`.
func (e *EsqueletoMensajes) AccessibilityLabel() string { return e.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (e *EsqueletoMensajes) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// MedidaEsqueletoMensajes es lo que ocupa de alto el del historial, sin el `py-3` de
// abajo, que no empuja nada.
func MedidaEsqueletoMensajes() fyne.Size {
	return NuevoEsqueletoMensajes("").medida()
}

// medida es la única cuenta: la usan MinSize y MedidaEsqueletoMensajes.
func (e *EsqueletoMensajes) medida() fyne.Size {
	alto := e.Arriba
	for i := range e.Partes {
		alto += e.aireDe(i) + altoFilaEsqueletoMensaje
	}
	return fyne.NewSize(0, alto)
}

// CreateRenderer monta un cuadro y dos barras por fila.
func (e *EsqueletoMensajes) CreateRenderer() fyne.WidgetRenderer {
	e.bloques = nil
	for range e.Partes {
		e.bloques = append(e.bloques,
			ui.NuevoEsqueleto(LadoEsqueletoAvatar, LadoEsqueletoAvatar),
			ui.NuevoEsqueleto(e.AnchoNombre, AltoEsqueletoTexto),
			ui.NuevoEsqueleto(0, AltoEsqueletoTexto))
	}
	r := &esqMensajesRenderer{e: e}
	r.Refresh()
	return r
}

type esqMensajesRenderer struct{ e *EsqueletoMensajes }

func (r *esqMensajesRenderer) MinSize() fyne.Size           { return r.e.medida() }
func (r *esqMensajesRenderer) Objects() []fyne.CanvasObject { return r.e.bloques }
func (r *esqMensajesRenderer) Destroy()                     {}

// Layout reparte las filas. La barra del texto es un porcentaje de la columna que
// queda a la derecha del cuadro (`flex-1`), no del historial entero.
func (r *esqMensajesRenderer) Layout(t fyne.Size) {
	e := r.e
	y := e.Arriba
	columna := max(t.Width-2*e.Canalon-LadoEsqueletoAvatar-HuecoEsqueletoMensaje, 0)
	x := e.Canalon + LadoEsqueletoAvatar + HuecoEsqueletoMensaje
	for i, parte := range e.Partes {
		y += e.aireDe(i)
		cuadro, nombre, texto := e.bloques[3*i], e.bloques[3*i+1], e.bloques[3*i+2]
		cuadro.Move(fyne.NewPos(e.Canalon, y))
		cuadro.Resize(fyne.NewSize(LadoEsqueletoAvatar, LadoEsqueletoAvatar))
		nombre.Move(fyne.NewPos(x, y+ArribaEsqueletoTexto))
		nombre.Resize(fyne.NewSize(min(e.AnchoNombre, columna), AltoEsqueletoTexto))
		texto.Move(fyne.NewPos(x, y+ArribaEsqueletoTexto+AltoEsqueletoTexto+HuecoEsqueletoTexto))
		texto.Resize(fyne.NewSize(columna*parte, AltoEsqueletoTexto))
		y += altoFilaEsqueletoMensaje
	}
}

func (r *esqMensajesRenderer) Refresh() { r.Layout(r.e.Size()) }
