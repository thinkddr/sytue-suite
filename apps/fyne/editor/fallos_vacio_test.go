// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fmt"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// vaciosEn son los estados vacíos colgados de un objeto, en orden de recorrido.
func vaciosEn(o fyne.CanvasObject) []*ui.Vacio {
	var hay []*ui.Vacio
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Vacio:
			hay = append(hay, x)
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				andar(h)
			}
		}
	}
	andar(o)
	return hay
}

// TestLosFallosDeLaPortadaYLaPapeleraSonDeError (DS-06): sin conexión, sin sesión, sin
// permiso y el error de carga son el `tone="error"` de la web (`DocList.tsx`,
// `DocTrash.tsx`): el glifo de peligro a 32 en `danger-text` que pone `VacioError`, no
// el de aviso a 40 pintado a mano. VISTO EN ROJO con el `aviso()` de antes. Con
// `SYTUE_CAPTURAS` deja las imágenes para mirarlas.
func TestLosFallosDeLaPortadaYLaPapeleraSonDeError(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	fallos := []Estado{SinConexion, SinSesion, SinPermiso, Error}
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		for _, e := range fallos {
			p := portadaDePrueba(t, &servicio{estado: http.StatusInternalServerError})
			p.Cargar()
			p.Estado = e
			p.Repintar()
			papelera := NuevaPapeleraDocs(Hoja, nil, "es", nil)
			papelera.Estado = e
			papelera.Repintar()
			for nombre, o := range map[string]fyne.CanvasObject{"portada": p.Objeto(), "papelera": papelera.Objeto()} {
				o.Resize(fyne.NewSize(900, 640))
				img := pinta(t, 900, 640, container.NewStack(o))
				vs := vaciosEn(o)
				if len(vs) != 1 {
					t.Fatalf("%s en %v: %d estados vacíos", nombre, e, len(vs))
				}
				if vs[0].Tono != ui.VacioError || vs[0].Ilustracion != nil {
					t.Errorf("%s en %v: el fallo no es VacioError (tono %v, ilustración %v)", nombre, e, vs[0].Tono, vs[0].Ilustracion != nil)
				}
				if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-fallo-%d-%s.png", nombre, e, variante)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, img)
					_ = f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}
