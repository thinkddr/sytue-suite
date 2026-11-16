// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// ultimoAviso es el toast más reciente de la app; uno vacío si no hay ninguno.
func ultimoAviso(a *App) *ui.Aviso {
	l := a.avisos.Avisos()
	if len(l) == 0 {
		return &ui.Aviso{}
	}
	return l[len(l)-1]
}

func soloIDs(l []api.Photo) []string { return idsDe(l) }

// TestReponerDevuelveCadaFotoASuSitio: lo devuelto vuelve donde estaba, no al final; no
// se repite lo que ya está, y lo cargado entretanto se queda detrás. Es la misma tabla
// que `deshacer.test.ts` de la web.
func TestReponerDevuelveCadaFotoASuSitio(t *testing.T) {
	f := func(ids ...string) []api.Photo {
		out := make([]api.Photo, len(ids))
		for i, id := range ids {
			out[i] = api.Photo{ID: id}
		}
		return out
	}
	orden := []string{"a", "b", "c", "d", "e"}
	for _, c := range []struct {
		actual, devueltas, orden, quiero []string
	}{
		{[]string{"a", "c", "e"}, []string{"d", "b"}, orden, orden},
		{[]string{"b", "c"}, []string{"a"}, []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "b", "z"}, []string{"b"}, []string{"a", "b"}, []string{"a", "b", "z"}},
		{[]string{"a", "n1"}, []string{"b"}, []string{"a", "b"}, []string{"a", "b", "n1"}},
	} {
		if got := soloIDs(reponer(f(c.actual...), f(c.devueltas...), c.orden)); !slices.Equal(got, c.quiero) {
			t.Errorf("reponer(%v, %v) = %v; quiero %v", c.actual, c.devueltas, got, c.quiero)
		}
	}
}

// TestLaPapeleraDelVisorNoPreguntaYSeDeshace: Supr en el visor manda la foto a la
// papelera AL MOMENTO —sin banda de confirmación—, cierra el visor y avisa 10 s con
// «Deshacer», que la restaura y la devuelve a SU sitio de la galería.
//
// VISTO EN ROJO (2026-11-09): con la banda de antes, Supr no llega al servicio; y
// reponiendo con `append`, la foto vuelve al final.
func TestLaPapeleraDelVisorNoPreguntaYSeDeshace(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(4), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	antes := soloIDs(a.g.fotos)
	f := a.g.fotos[1]
	a.abrirEnGaleria(f.ID)
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if contar(s, "DELETE /api/v1/assets/"+f.ID) != 1 {
		t.Fatalf("Suprimir no mandó la foto a la papelera al momento: %v", s.vistas())
	}
	if a.vis != nil || posicion(a.g.fotos, f.ID) >= 0 {
		t.Fatal("tirar no cerró el visor ni sacó la foto de la galería")
	}
	av := ultimoAviso(a)
	if av.Titulo != a.v.T.T("gallery.trashed", i18n.Vars{"count": 1}) ||
		av.Descripcion != a.v.T.T("gallery.trashedStays", i18n.Vars{"count": 1}) ||
		av.Duracion != ui.DuracionDeshacer || av.RotuloAccion != a.v.Traduce("gallery.undo") || av.AlPulsarAccion == nil {
		t.Fatalf("el aviso no trae su «Deshacer» de 10 s: %+v", av)
	}
	av.AlPulsarAccion()
	if contar(s, "POST /api/v1/assets/"+f.ID+"/restore") != 1 {
		t.Errorf("«Deshacer» no pidió restaurar la foto: %v", s.vistas())
	}
	if got := soloIDs(a.g.fotos); !slices.Equal(got, antes) {
		t.Errorf("tras deshacer la galería es %v y era %v", got, antes)
	}
	if quiero := a.v.T.T("gallery.restored", i18n.Vars{"count": 1}); ultimoAviso(a).Titulo != quiero {
		t.Errorf("tras deshacer se avisa «%s» y quiero «%s»", ultimoAviso(a).Titulo, quiero)
	}
}

// TestLaPapeleraEnLoteSeDeshace: la papelera de la barra de selección no pregunta; su
// aviso cuenta las fotos y su «Deshacer» restaura cada una por id. Lo que el servicio
// niega se dice con su número y se queda fuera de la galería.
//
// VISTO EN ROJO (2026-11-09): con el `confirmar` de antes, tirar abre un diálogo y no
// llega nada al servicio; y sin separar los fallidos, el aviso final es el de éxito.
func TestLaPapeleraEnLoteSeDeshace(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(5), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Arrancar()
	antes := soloIDs(a.g.fotos)
	sel := []api.Photo{a.g.fotos[0], a.g.fotos[2], a.g.fotos[3]}
	s.noRestaura = map[string]bool{sel[1].ID: true}
	a.entrarEnSeleccion()
	for _, f := range sel {
		a.alternar(f)
	}
	a.tirarSeleccion()
	if w.Canvas().Overlays().Top() != nil {
		t.Fatal("la papelera en lote abrió un diálogo")
	}
	if n := contar(s, "DELETE /api/v1/assets/"); n != 3 {
		t.Fatalf("al servicio llegaron %d envíos a la papelera y tocaban 3", n)
	}
	if len(a.g.fotos) != 2 || a.g.seleccion != nil {
		t.Fatalf("tras tirar quedan %d fotos (quiero 2) o la selección sigue abierta", len(a.g.fotos))
	}
	av := ultimoAviso(a)
	if av.Titulo != a.v.T.T("gallery.trashed", i18n.Vars{"count": 3}) || av.Duracion != ui.DuracionDeshacer || av.AlPulsarAccion == nil {
		t.Fatalf("el aviso del lote no trae su «Deshacer» de 10 s: %+v", av)
	}
	av.AlPulsarAccion()
	if n := contar(s, "POST /api/v1/assets/"); n != 3 {
		t.Errorf("«Deshacer» pidió %d restauraciones y tocaban 3", n)
	}
	quiero := slices.DeleteFunc(slices.Clone(antes), func(id string) bool { return id == sel[1].ID })
	if got := soloIDs(a.g.fotos); !slices.Equal(got, quiero) {
		t.Errorf("tras deshacer la galería es %v y quiero %v", got, quiero)
	}
	if ultimo := ultimoAviso(a); ultimo.Titulo != a.v.T.T("gallery.restoreFailed", i18n.Vars{"count": 1}) || ultimo.Tono != ui.AvisoPeligro {
		t.Errorf("tras deshacer con una negada se avisa «%s»", ultimo.Titulo)
	}
}

// TestDeshacerConOtraGaleriaRestauraSinPintar: si en esos 10 s se ha buscado otra cosa,
// «Deshacer» restaura igual —y lo dice— pero no mete la foto en una lista que no la
// pidió.
func TestDeshacerConOtraGaleriaRestauraSinPintar(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(4), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	f := a.g.fotos[0]
	a.abrirEnGaleria(f.ID)
	a.tirarDelVisor(f)
	deshacer := ultimoAviso(a).AlPulsarAccion
	a.buscar("IMG_1003")
	deshacer()
	if contar(s, "POST /api/v1/assets/"+f.ID+"/restore") != 1 {
		t.Error("con otra galería, «Deshacer» no restauró")
	}
	if posicion(a.g.fotos, f.ID) >= 0 {
		t.Errorf("la foto devuelta se pintó en otra búsqueda: %v", soloIDs(a.g.fotos))
	}
	if quiero := a.v.T.T("gallery.restored", i18n.Vars{"count": 1}); ultimoAviso(a).Titulo != quiero {
		t.Errorf("tras deshacer se avisa «%s»", ultimoAviso(a).Titulo)
	}
}

// TestUnSegundoToqueNoTiraDosVeces: mientras la primera llamada sigue en vuelo, otra
// pulsación (o Supr mantenido) no manda otra vez la misma foto.
func TestUnSegundoToqueNoTiraDosVeces(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	var cola []func()
	a.lanzar = func(f func()) { cola = append(cola, f) }
	f := a.g.fotos[0]
	a.abrirEnGaleria(f.ID)
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyDelete})
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if len(cola) != 1 {
		t.Fatalf("dos Supr seguidos lanzaron %d envíos a la papelera", len(cola))
	}
	cola[0]()
	if n := contar(s, "DELETE /api/v1/assets/"+f.ID); n != 1 {
		t.Errorf("la foto se mandó %d veces", n)
	}
	if n := len(a.avisos.Avisos()); n != 1 {
		t.Errorf("salieron %d avisos con «Deshacer»", n)
	}
}

// TestLaGaleriaYaNoPreguntaAntesDeTirar: ni el visor ni la barra vuelven a las
// confirmaciones de antes (design §5.4).
func TestLaGaleriaYaNoPreguntaAntesDeTirar(t *testing.T) {
	for _, nombre := range []string{"app_galeria.go", "app_visor.go", "visor.go"} {
		b, err := os.ReadFile(nombre)
		if err != nil {
			t.Fatal(err)
		}
		fuente := string(b)
		for _, prohibido := range []string{"gallery.trashSelectedConfirm", "viewer.deleteConfirm", "borrando"} {
			if strings.Contains(fuente, prohibido) {
				t.Errorf("%s vuelve a preguntar antes de tirar (%q)", nombre, prohibido)
			}
		}
	}
}

// TestTirarYBuscarEnSeguidaNoPierdeElDeshacer: si la llamada vuelve cuando ya se ha
// buscado otra cosa (o ha saltado el refresco), las fotos YA están en la papelera, así
// que el aviso con «Deshacer» sale igual: sin él no habría con qué devolverlas.
//
// VISTO EN ROJO (2026-11-09): con `listo` atado a la generación, el aviso no salía.
func TestTirarYBuscarEnSeguidaNoPierdeElDeshacer(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	var cola []func()
	a.lanzar = func(f func()) { cola = append(cola, f) }
	f := a.g.fotos[0]
	a.abrirEnGaleria(f.ID)
	a.teclaSuelta(&fyne.KeyEvent{Name: fyne.KeyDelete})
	a.gen++ // lo que hace un refresco o una búsqueda mientras la llamada vuela
	cola[0]()
	if contar(s, "DELETE /api/v1/assets/"+f.ID) != 1 {
		t.Fatalf("la foto no llegó a la papelera: %v", s.vistas())
	}
	av := ultimoAviso(a)
	if av.AlPulsarAccion == nil || av.Titulo != a.v.T.T("gallery.trashed", i18n.Vars{"count": 1}) {
		t.Fatalf("con la galería cambiada no salió el «Deshacer»: %+v", av)
	}
	if a.vis != nil {
		t.Error("el visor sigue en la foto que se ha tirado")
	}
}

// TestOtraCuentaNoHeredaElDeshacer: al cambiar de cuenta se van los avisos de la
// anterior, y un «Deshacer» que quedara a mano no restaura con las credenciales de la
// nueva.
func TestOtraCuentaNoHeredaElDeshacer(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.CambiarSesion("uno")
	a.Arrancar()
	f := a.g.fotos[0]
	a.abrirEnGaleria(f.ID)
	a.tirarDelVisor(f)
	deshacer := ultimoAviso(a).AlPulsarAccion
	if deshacer == nil {
		t.Fatal("no salió el «Deshacer»")
	}
	a.CambiarSesion("dos")
	if n := len(a.avisos.Avisos()); n != 0 {
		t.Errorf("con otra cuenta siguen %d avisos de la anterior", n)
	}
	deshacer()
	if n := contar(s, "POST /api/v1/assets/"+f.ID+"/restore"); n != 0 {
		t.Errorf("el «Deshacer» de la otra cuenta pidió %d restauraciones", n)
	}
}
