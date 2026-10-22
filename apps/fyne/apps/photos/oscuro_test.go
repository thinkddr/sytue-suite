// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElTemaCambiaEnCaliente: elegir el oscuro con la app abierta rehace la pantalla en
// oscuro, como en las demás apps de la suite. Photos arma sus pantallas con la variante
// de su `Vista`, y esa se fijaba al arrancar: medido el 2026-09-27, el visor salía con la
// cabecera del claro y los rótulos en blanco encima (invisibles).
//
// Visto en ROJO quitando el `AddListener` de `Nueva`: la variante se queda en claro.
func TestElTemaCambiaEnCaliente(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(3), porPagina: 10})
	a.Arrancar()
	fyne.DoAndWait(func() {})
	if a.v.Variante != tema.Claro {
		t.Fatalf("arranca en %v con el claro elegido", a.v.Variante)
	}

	tema.Elegir("oscuro")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{}) // lo que hace `sesion.Suite`
	fyne.DoAndWait(func() {})
	if a.v.Variante != tema.Oscuro {
		t.Errorf("con el oscuro elegido en caliente, la pantalla sigue en %v", a.v.Variante)
	}
}

// TestElCampoDelAlbumEsElDeAlbumsTsx: Albums.tsx usa el `Input` de la casa en su talla
// `md`, del alto de su botón; era un `<input>` escrito a mano (`bg-bg border-border
// rounded-sm px-2 py-1 text-sm`) y este test vigilaba esas clases. Ahora vigila que en
// los dos lados sea el campo corriente.
func TestElCampoDelAlbumEsElDeAlbumsTsx(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/photos/src/components/Albums.tsx")
	if !strings.Contains(web, "<Input") || strings.Contains(web, "<input") {
		t.Fatal("Albums.tsx ya no usa el `Input` de la casa para el título del álbum")
	}
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(1), porPagina: 10})
	if a.campoAlbum.Propia != nil || a.campoAlbum.Talla != ui.CampoMD {
		t.Errorf("el campo del álbum es propio (%+v) o de talla %v y la web usa `Input` md", a.campoAlbum.Propia, a.campoAlbum.Talla)
	}
}
