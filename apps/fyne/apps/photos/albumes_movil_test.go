// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fmt"
	"math"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El campo de Albums.tsx puede encoger: fijarlo a w-64 sin flex-shrink dejaba
// Crear fuera del teléfono. Se mide la pantalla real antes de capturarla, también
// con un botón alto y al volver de escritorio a móvil.
func TestCrearAlbumCabeEnMovil(t *testing.T) {
	for _, talla := range []ui.Talla{ui.TallaMD, ui.TallaLG} {
		t.Run(fmt.Sprintf("talla-%d", talla), func(t *testing.T) {
			v := vistaDePrueba(t, nil)
			v.Ventana = 320
			campo := ui.NuevaEntrada(v.Traduce("albums.title"))
			campo.PonerTexto("Viaje")
			creado := ""
			p := PantallaAlbumes(v, Albumes{Titulo: campo, AlCrear: func(titulo string) { creado = titulo }})
			bloques := p.Objects[1].(*fyne.Container)
			form := bloques.Objects[1].(*fyne.Container)
			crear := form.Objects[1].(*ui.Boton)
			crear.Talla = talla
			for _, ancho := range []float32{320, 360, 375, 390, 1280, 320} {
				p.Resize(fyne.NewSize(ancho, 740))
				m := crear.MinSize()
				want := min(tema.Espacio(64), max(form.Size().Width-tema.Espacio(2)-m.Width, 0))
				if got := campo.Size().Width; math.Abs(float64(got-want)) > .01 {
					t.Errorf("a%g el campo mide%g; debe encoger a%g reservando Crear", ancho, got, want)
				}
				if fin := crear.Position().X + crear.Size().Width; fin > form.Size().Width+.01 {
					t.Errorf("a%g Crear termina en%g fuera del formulario de%g", ancho, fin, form.Size().Width)
				}
				if fin := campo.Position().X + campo.Size().Width + tema.Espacio(2); fin > crear.Position().X+.01 {
					t.Errorf("a%g el campo se superpone a Crear", ancho)
				}
				for _, o := range []fyne.CanvasObject{campo, crear} {
					if o.Position().Y < 0 || o.Position().Y+o.Size().Height > form.Size().Height+.01 {
						t.Errorf("a%g el formulario de%g recorta %T", ancho, form.Size().Height, o)
					}
				}
				if os.Getenv("SYTUE_CAPTURA") != "" {
					guardar(t, fmt.Sprintf("albumes-crear-%g-talla-%d", ancho, talla), pintaPantallaA(t, nil, p, ancho, 740))
				}
			}
			test.Tap(crear)
			if creado != "Viaje" {
				t.Errorf("Crear recibe%q en vez del título escrito", creado)
			}
		})
	}
}
