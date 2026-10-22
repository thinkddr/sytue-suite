// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Dropdown.tsx

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El aviso de «queda más abajo» de un panel que se desplaza por dentro (Dropdown.tsx):
//
//	sticky bottom-0 -mt-8 flex h-8 items-end justify-center
//	bg-linear-to-t from-[var(--sytue-menu-surface)] from-35% to-transparent
//	<Icon name="chevronDown" size={14} />, en `text-text-muted`
//
// El borde inferior se funde con la superficie y lleva una flecha. Va pegado al pie de lo
// que se ve y no ocupa sitio; al llegar al final desaparece, así que no tapa el último
// elemento. No se pulsa (`pointer-events-none`): es un ráster, y el recorrido de Fyne no
// para en él.
const (
	// AltoMasAbajo es su `h-8` y GlifoMasAbajo el `size={14}` de la flecha.
	AltoMasAbajo  float32 = 8 * 0.25 * tema.Rem
	GlifoMasAbajo float32 = 14
	// macizoMasAbajo es el `from-35%`: el tramo de abajo que es superficie pura.
	macizoMasAbajo = 0.35
)

// masAbajo es el aviso. Lo llevan el menú y la lista de un selector, que son los dos
// paneles de la suite que se desplazan por dentro.
type masAbajo struct {
	velo   *canvas.Raster
	flecha *iconos.Dibujo
	fondo  color.NRGBA
	radio  float32
}

// nuevoMasAbajo lo crea escondido. `radio` es el de las esquinas de abajo del panel, por
// dentro de su borde: el fundido las sigue en vez de taparlas con un pico.
func nuevoMasAbajo(radio float32) *masAbajo {
	a := &masAbajo{radio: radio}
	a.velo = canvas.NewRaster(a.pintar)
	a.flecha = iconos.Trazo(iconos.ChevronAbajo, GlifoMasAbajo, "text-muted", tema.Claro)
	a.velo.Hide()
	a.flecha.Ver(false)
	return a
}

func (a *masAbajo) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{a.velo, a.flecha.Objeto()}
}

// QuedaMasAbajo es `quedaMasAbajo` de Dropdown.tsx: si al panel le queda contenido por
// debajo de lo que enseña (`scrollHeight - scrollTop - clientHeight > 1`).
func QuedaMasAbajo(natural, desplazado, visible float32) bool {
	return natural-desplazado-visible > 1
}

// colocar lo pone al pie de un panel de ese tamaño, por dentro de su borde, y lo enseña
// solo si queda algo por ver.
func (a *masAbajo) colocar(t fyne.Size, queda bool, v tema.Variante) {
	esconder(a.velo, !queda)
	a.flecha.Ver(queda)
	if !queda {
		return
	}
	a.fondo = tema.Color("menu-surface", v)
	alto := min(AltoMasAbajo, t.Height-2*tema.Borde)
	a.velo.Move(fyne.NewPos(tema.Borde, t.Height-tema.Borde-alto))
	a.velo.Resize(fyne.NewSize(t.Width-2*tema.Borde, alto))
	a.velo.Refresh()
	a.flecha.Tenir("text-muted", v)
	a.flecha.Colocar((t.Width-GlifoMasAbajo)/2, t.Height-tema.Borde-GlifoMasAbajo, GlifoMasAbajo)
}

// pintar es el degradado hacia arriba: superficie maciza en el 35 % de abajo y de ahí a
// transparente. Las dos esquinas de abajo se recortan al radio del panel.
func (a *masAbajo) pintar(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == 0 || h == 0 || a.velo.Size().Height <= 0 {
		return img
	}
	escala := float32(h) / a.velo.Size().Height
	r := a.radio * escala
	for y := range h {
		// De abajo (0) a arriba (1).
		p := float64(h-1-y) / float64(max(h-1, 1))
		alfa := 1.0
		if p > macizoMasAbajo {
			alfa = 1 - (p-macizoMasAbajo)/(1-macizoMasAbajo)
		}
		c := a.fondo
		c.A = uint8(float64(c.A) * alfa)
		for x := range w {
			if fueraDeLaEsquina(float32(x)+0.5, float32(y)+0.5, float32(w), float32(h), r) {
				continue
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// fueraDeLaEsquina dice si un punto cae fuera del arco de una de las dos esquinas de
// abajo de una caja de w × h con ese radio.
func fueraDeLaEsquina(x, y, w, h, r float32) bool {
	if y < h-r {
		return false
	}
	dy := y - (h - r)
	switch {
	case x < r:
		dx := r - x
		return dx*dx+dy*dy > r*r
	case x > w-r:
		dx := x - (w - r)
		return dx*dx+dy*dy > r*r
	}
	return false
}
