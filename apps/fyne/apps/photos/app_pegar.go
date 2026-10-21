// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # Lo que se pega arriba (`position: sticky`)
//
// En la galería, la barra de la selección (`sticky top-0 z-20`) y la cabecera de cada mes
// (`sticky top-0`, o `top-[3.25rem]` bajo la barra) se quedan arriba mientras se recorre:
// «es lo que permite saber dónde estás en una galería de miles». Fyne no tiene `sticky`,
// así que la app pone una COPIA en una capa encima del desplazamiento cuando el original
// ha salido por arriba, y la quita cuando vuelve. La copia de un mes la empuja hacia
// arriba el final de su sección, como hace el navegador con el borde de su contenedor.

// TopeBajoBarra es el `top-[3.25rem]` del mes con la barra de selección puesta.
const TopeBajoBarra = 3.25 * tema.Rem

// pegable es un mes de la galería tal y como quedó en la pantalla.
type pegable struct {
	cabecera, rejilla fyne.CanvasObject
	g                 GrupoMes
}

// estadoPegado es lo que la app recuerda de la última galería pintada, y las copias
// pegadas: se construyen al pegarse y luego solo se mueven, que es lo que pasa a cada
// paso de la rueda.
type estadoPegado struct {
	d          Galeria
	meses      []pegable
	barra      fyne.CanvasObject
	pila       fyne.CanvasObject // el contenedor de los bloques, dentro del marco
	capa       *fyne.Container
	cual       int // el mes pegado ahora, o -1
	copiaMes   fyne.CanvasObject
	copiaBarra fyne.CanvasObject
}

// recoger apunta en la galería que se va a pintar dónde dejarle a la app sus meses y su
// barra. Las copias de antes eran de otros datos: se tiran.
func (a *App) recoger(d *Galeria) {
	p := &a.peg
	p.meses, p.barra, p.cual, p.copiaMes, p.copiaBarra = p.meses[:0], nil, -1, nil, nil
	d.AlMes = func(c, r fyne.CanvasObject, g GrupoMes) {
		p.meses = append(p.meses, pegable{cabecera: c, rejilla: r, g: g})
	}
	d.AlBarra = func(b fyne.CanvasObject) { p.barra = b }
	p.d = *d
}

// pegar pone en la capa la copia de lo que ha salido por arriba. Se llama al desplazarse
// y al repintar.
func (a *App) pegar() {
	p := &a.peg
	if p.capa == nil {
		return
	}
	enGaleria := a.ruta == RutaFotos || a.ruta == RutaFavoritas
	if !enGaleria || p.pila == nil {
		a.despegar()
		return
	}
	s := a.scroll.Offset.Y
	origen := p.pila.Position()
	var objs []fyne.CanvasObject
	tope := float32(0)
	if b := p.barra; b != nil {
		// La barra se pega en cuanto su borde de arriba pasa del borde de la vista.
		if origen.Y+b.Position().Y < s {
			if p.copiaBarra == nil {
				p.copiaBarra = barraSeleccion(a.v, p.d)
			}
			p.copiaBarra.Move(fyne.NewPos(origen.X+b.Position().X, 0))
			p.copiaBarra.Resize(b.Size())
			objs = append(objs, p.copiaBarra)
		} else {
			p.copiaBarra = nil
		}
		tope = TopeBajoBarra
	}
	cual, y := -1, float32(0)
	for i, m := range p.meses {
		arriba := origen.Y + m.cabecera.Position().Y
		fin := origen.Y + m.rejilla.Position().Y + m.rejilla.Size().Height
		if arriba < s+tope && fin > s+tope {
			// El final de la sección empuja la copia hacia arriba.
			cual, y = i, min(tope, fin-s-m.cabecera.Size().Height)
		}
	}
	if cual != p.cual {
		p.cual, p.copiaMes = cual, nil
		if cual >= 0 {
			p.copiaMes = cabeceraMes(a.v, p.meses[cual].g, p.d)
		}
	}
	if p.copiaMes != nil {
		m := p.meses[cual]
		p.copiaMes.Move(fyne.NewPos(origen.X+m.cabecera.Position().X, y))
		p.copiaMes.Resize(m.cabecera.Size())
		// El mes va DEBAJO de la barra: primero en la capa, y la barra encima.
		objs = append([]fyne.CanvasObject{p.copiaMes}, objs...)
	}
	p.capa.Objects = objs
	p.capa.Refresh()
}

// despegar vacía la capa.
func (a *App) despegar() {
	if a.peg.capa != nil && len(a.peg.capa.Objects) > 0 {
		a.peg.capa.Objects = nil
		a.peg.capa.Refresh()
	}
}
