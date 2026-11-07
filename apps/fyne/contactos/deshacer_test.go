// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// avisoConDeshacer es el aviso que ofrece «Deshacer»; nil si no hay.
func avisoConDeshacer(a *App) *ui.Aviso {
	for _, av := range a.avisos.Avisos() {
		if av.RotuloAccion != "" && av.AlPulsarAccion != nil {
			return av
		}
	}
	return nil
}

// restauraciones son las peticiones que sacan algo de la papelera, con su cuerpo.
func restauraciones(s *servicioFalso) []string {
	var v []string
	for _, e := range s.escrituras {
		if strings.HasPrefix(e.ruta, "POST ") && strings.HasSuffix(e.ruta, "/restore") {
			v = append(v, e.ruta+" "+strings.TrimSpace(e.cuerpo))
		}
	}
	return v
}

// avisoPeligro dice si hay un aviso de peligro con ese título.
func avisoPeligro(a *App, clave string) bool {
	for _, x := range a.avisos.Avisos() {
		if x.Tono == ui.AvisoPeligro && x.Titulo == a.Pantalla.t(clave, nil) {
			return true
		}
	}
	return false
}

// TestElLoteAPapeleraSeDeshace: el aviso del lote lleva «Deshacer» y deshacer es UNA
// petición al lote de restaurar con los UID tirados, ni la papelera ni una por contacto
// (una papelera de 200 no puede dejar fuera parte de un lote de 1000).
func TestElLoteAPapeleraSeDeshace(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("luis", "Luis"), persona("marta", "Marta")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.lote(LoteAPapelera, a.Pantalla.Contactos[:2], nil)

	av := avisoConDeshacer(a)
	if av == nil {
		t.Fatal("el aviso de «a la papelera» no ofrece deshacer")
	}
	if av.RotuloAccion != a.Pantalla.t("common.undo", nil) || av.Duracion != ui.DuracionDeshacer {
		t.Errorf("acción %q durante %v", av.RotuloAccion, av.Duracion)
	}
	av.AlPulsarAccion()
	got := restauraciones(s)
	quiero := `POST /api/v1/contacts/batch/restore {"uids":["u-ana","u-luis"]}`
	if len(got) != 1 || got[0] != quiero {
		t.Errorf("deshacer pide %v y debería pedir solo %s", got, quiero)
	}
	if avisoConDeshacer(a) != nil {
		t.Error("tras deshacer sigue el aviso con «Deshacer»")
	}
	if avisoPeligro(a, "trash.undoFailed") {
		t.Error("volvieron todos y el aviso dice que no")
	}
}

// TestElLoteDiceSiNoVuelvenTodos: si el servicio devuelve menos de los que tiró, el
// aviso lo dice, en plural.
func TestElLoteDiceSiNoVuelvenTodos(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana"), persona("luis", "Luis")}, noVuelven: 1}
	a := appDePrueba(t, s)
	a.Cargar()
	a.lote(LoteAPapelera, a.Pantalla.Contactos, nil)
	avisoConDeshacer(a).AlPulsarAccion()
	if !avisoPeligro(a, "trash.undoFailed") {
		t.Error("uno no ha vuelto y el aviso no lo dice")
	}
}

// TestBorrarUnoSeDeshaceYDiceSiNoVuelve: el borrado desde el editor también lleva
// «Deshacer»; si lo tirado ya no vuelve, no se dice que ha vuelto, y se dice en
// singular, sin dar por hecho que sigue en la papelera.
func TestBorrarUnoSeDeshaceYDiceSiNoVuelve(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}, noVuelven: 1}
	a := appDePrueba(t, s)
	a.Cargar()
	a.borrarContacto(*a.Pantalla.Activa, a.Pantalla.Contactos[0], func(error) {})
	av := avisoConDeshacer(a)
	if av == nil {
		t.Fatal("el aviso de «movido a la papelera» no ofrece deshacer")
	}
	av.AlPulsarAccion()
	if got := restauraciones(s); len(got) != 1 || !strings.Contains(got[0], `"u-ana"`) {
		t.Errorf("deshacer el borrado de uno pide %v", got)
	}
	if !avisoPeligro(a, "trash.undoFailedOne") {
		t.Error("no ha vuelto y el aviso no lo dice en singular")
	}
	if avisoPeligro(a, "trash.undoFailed") {
		t.Error("el borrado de uno avisa en plural")
	}
}
