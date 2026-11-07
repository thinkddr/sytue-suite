// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// botonEstrella es el `FavoriteToggle` de ContactDetail.tsx.
//
// NO es un `ui.Boton` secundario, aunque lo parezca: es un `<button>` escrito a mano
// con `grid h-9 w-9 place-items-center rounded-md border` y SIN fondo, así que sobre el
// papel de la ficha se ve el papel, y su borde es `border` y no el `border-strong` del
// secundario. Con el botón del catálogo salía un cuadrado blanco con un borde más
// oscuro.
//
// «Sin fondo» se pinta con el fondo de la hoja (`ui.FondoHoja`: blanca sobre papel),
// que es lo que se ve a través: `ui.Caja`
// es un rectángulo del color del borde con el relleno encima (regla 4 de `ui.go`), así
// que un relleno transparente dejaría ver el borde entero y saldría un cuadrado macizo.
//
//	caja     31,5 × 31,5, `rounded-md`, borde `border`, sin relleno    hoja
//	estrella 18: `warning-text` y rellena si está destacado; si no,     hoja
//	         solo trazo en `text-muted`, y `text` al pasar por encima
type botonEstrella struct {
	ui.Pulsable
	Activo bool
}

func nuevoBotonEstrella(activo bool, rotulo string, alPulsar func()) *botonEstrella {
	b := &botonEstrella{Activo: activo}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// LadoGlifoEstrella es el `StarGlyph` por defecto (18) dentro del botón.
const LadoGlifoEstrella float32 = 18

func (b *botonEstrella) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &estrellaRenderer{
		b:    b,
		caja: ui.NuevaCaja(tema.Color(ui.FondoHoja(), v), ui.BordeSobre("border", ui.FondoHoja(), v), tema.Forma("radius-control")),
		aro:  ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
	}
	r.Refresh()
	return r
}

type estrellaRenderer struct {
	b      *botonEstrella
	caja   *ui.Caja
	aro    *ui.Aro
	glifo  *iconos.Dibujo
	activo bool
}

func (r *estrellaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoEstrellaFicha, LadoEstrellaFicha)
}

func (r *estrellaRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(append(r.caja.Objetos(), r.glifo.Objeto())...)
}

func (r *estrellaRenderer) Destroy() {}

func (r *estrellaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	r.glifo.Colocar((t.Width-LadoGlifoEstrella)/2, (t.Height-LadoGlifoEstrella)/2, LadoGlifoEstrella)
}

func (r *estrellaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// La ruta es la misma; lo que cambia es el relleno, así que el dibujo se rehace
	// solo cuando cambia el estado.
	if r.glifo == nil || r.activo != r.b.Activo {
		r.activo = r.b.Activo
		if r.activo {
			r.glifo = GlifoRelleno(EstrellaD, LadoGlifoEstrella, tema.Color("warning-text", v))
		} else {
			r.glifo = Glifo(EstrellaD, LadoGlifoEstrella, "text-muted", v)
		}
	}
	switch {
	case r.b.Activo:
		r.glifo.TenirCon(tema.Color("warning-text", v))
	case r.b.Encima():
		r.glifo.TenirCon(tema.Color("text", v))
	default:
		r.glifo.TenirCon(tema.Color("text-muted", v))
	}
	r.caja.Pintar(tema.Color(ui.FondoHoja(), v), ui.BordeSobre("border", ui.FondoHoja(), v))
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.Layout(r.b.Size())
	r.glifo.Objeto().Refresh()
}

// estrellaFila es el `FavoriteToggle row` de ContactsApp.tsx: el `IconButton size="sm"`
// del final de la fila, HERMANO del botón que abre la ficha. Es un control de verdad, y
// no una zona del toque de la fila, porque así se llega a él con el tabulador, se pulsa
// con Intro o Espacio y el lector de pantalla lo anuncia con su nombre («Destacar a
// Ana»), que es lo que la web ya hacía y la app nativa no (`docs/design.md` §2.6).
//
// Sin destacar, la estrella en trazo es `opacity-0` en reposo pero sigue en el orden de
// tabulación, y sale al pasar por la fila, en un teléfono, al pasarle el puntero o con
// el foco (`focus-visible:opacity-100`): un control enfocado que no se ve no dice dónde
// está el foco.
//
//	caja     LadoBotonEstrellaFila subido al suelo táctil, sin fondo   hoja
//	estrella LadoEstrellaFila: rellena en `warning-text` si destacado;  hoja
//	         si no, trazo `text-muted`, y `text` al pasar por encima
type estrellaFila struct {
	ui.Pulsable
	Activo bool
	// FilaEncima es el `group-hover` de la fila y el `pointer-coarse` de un teléfono.
	FilaEncima bool
}

func nuevaEstrellaFila() *estrellaFila {
	b := &estrellaFila{}
	b.ExtendBaseWidget(b)
	return b
}

// seVe dice si la estrella se pinta: siempre si está destacado; si no, solo cuando la
// web le quita el `opacity-0`.
func (b *estrellaFila) seVe() bool {
	return b.Activo || b.FilaEncima || b.Encima() || b.Enfocado() || ui.EsTactil()
}

func (b *estrellaFila) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &estrellaFilaRenderer{
		b:       b,
		aro:     ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		relleno: GlifoRelleno(EstrellaD, LadoEstrellaFila, tema.Color("warning-text", v)),
		trazo:   Glifo(EstrellaD, LadoEstrellaFila, "text-muted", v),
	}
	r.objetos = r.aro.Antes(r.relleno.Objeto(), r.trazo.Objeto())
	r.Refresh()
	return r
}

type estrellaFilaRenderer struct {
	b              *estrellaFila
	aro            *ui.Aro
	relleno, trazo *iconos.Dibujo
	objetos        []fyne.CanvasObject
}

func (r *estrellaFilaRenderer) MinSize() fyne.Size {
	l := ui.SueloTactil(LadoBotonEstrellaFila)
	return fyne.NewSize(l, l)
}

func (r *estrellaFilaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *estrellaFilaRenderer) Destroy()                     {}

func (r *estrellaFilaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	x, y := (t.Width-LadoEstrellaFila)/2, (t.Height-LadoEstrellaFila)/2
	r.relleno.Colocar(x, y, LadoEstrellaFila)
	r.trazo.Colocar(x, y, LadoEstrellaFila)
}

func (r *estrellaFilaRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.relleno.TenirCon(tema.Color("warning-text", v))
	tinta := "text-muted"
	if b.Encima() {
		tinta = "text"
	}
	r.trazo.TenirCon(tema.Color(tinta, v))
	r.relleno.Ver(b.Activo)
	r.trazo.Ver(!b.Activo && b.seVe())
	r.aro.Pintar(v)
	r.aro.Ver(b.Enfocado())
	r.Layout(b.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}
