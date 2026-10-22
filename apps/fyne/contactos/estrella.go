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
// «Sin fondo» se pinta con el `bg` de la ficha, que es lo que se ve a través: `ui.Caja`
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
		caja: ui.NuevaCaja(tema.Color("bg", v), ui.BordeSobre("border", "bg", v), tema.Forma("radius-control")),
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
	r.caja.Pintar(tema.Color("bg", v), ui.BordeSobre("border", "bg", v))
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
	r.Layout(r.b.Size())
	r.glifo.Objeto().Refresh()
}
