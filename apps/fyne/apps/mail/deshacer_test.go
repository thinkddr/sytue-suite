// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestArchivarOfreceDeshacerYDevuelveASuCarpeta: el aviso del triaje confirmado trae
// «Deshacer», y pulsarlo devuelve los dos mensajes del hilo a Recibidos en una llamada
// y vuelve a pedir la lista.
//
// VISTO EN ROJO (2026-11-05): sin `avisarDeshacer` el archivado no deja aviso, y con
// el destino en lugar del origen en `deshacerMovimiento` la llamada es MoveMany(ar,…).
func TestArchivarOfreceDeshacerYDevuelveASuCarpeta(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	listas := s.pidio("Messages")

	b.P.Acciones.Archivar(b.P.Bandeja.Conversaciones[0])
	av := b.P.Avisos.Avisos()
	if len(av) != 1 || av[0].Tono != ui.AvisoExito || av[0].AlPulsarAccion == nil ||
		!strings.Contains(av[0].Titulo, "Archivo") {
		t.Fatalf("el aviso de archivado no ofrece deshacer: %+v", av)
	}
	av[0].AlPulsarAccion()
	if !slices.Contains(s.llamadas, "MoveMany(in,m1,m3)") {
		t.Errorf("deshacer no devuelve el hilo a Recibidos: %v", s.llamadas)
	}
	if s.pidio("Messages") != listas+1 {
		t.Error("tras deshacer no se ha vuelto a pedir la lista")
	}
}

// TestDeshacerDevuelveCadaMensajeASuCarpeta: en una búsqueda la vista cruza carpetas, y
// cada mensaje vuelve a la SUYA; lo que ya estaba en el destino no se mueve.
func TestDeshacerDevuelveCadaMensajeASuCarpeta(t *testing.T) {
	cs := []Conversacion{{HiloID: "h1", Mensajes: []Mensaje{
		{ID: "m1", CarpetaID: "in"}, {ID: "m2", CarpetaID: "se"}, {ID: "m3", CarpetaID: "ar"},
	}}, {HiloID: "h2", Mensajes: []Mensaje{{ID: "m4"}}}}
	got := origenesDeMovimiento(cs, "", "ar")
	want := map[string][]string{"in": {"m1"}, "se": {"m2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("búsqueda: %v, quería %v", got, want)
	}
	// En una carpeta, sin carpeta en el sobre, el origen es la que se mira.
	if got := origenesDeMovimiento(cs[1:], "in", "tr"); !reflect.DeepEqual(got, map[string][]string{"in": {"m4"}}) {
		t.Errorf("carpeta: %v", got)
	}
}

// TestDeshacerEnUnaBusquedaLaRelanza: lo que se ve en una búsqueda son sus resultados,
// así que deshacer vuelve a buscar —no recarga la carpeta de debajo, que no se ve— y
// la carpeta, que también perdió el hilo, se pide al salir de la búsqueda.
//
// VISTO EN ROJO (2026-11-05): con `cargarLista` a secas, el hilo devuelto no vuelve a
// los resultados hasta repetir la búsqueda.
func TestDeshacerEnUnaBusquedaLaRelanza(t *testing.T) {
	s := servicioConTodas()
	s.buscados = s.paginas[""][:1]
	b, pendientes := buzonDePrueba(t, s)
	t.Cleanup(func() {
		for _, a := range b.P.Avisos.Avisos() {
			b.P.Avisos.Descartar(a)
		}
	})
	b.Arrancar(context.Background())
	b.Buscar("calendario")
	(*pendientes)[len(*pendientes)-1]()
	busquedas, listas := s.pidio("Search"), s.pidio("Messages")

	b.P.Acciones.Archivar(b.P.Bandeja.Conversaciones[0])
	if len(b.P.Bandeja.Conversaciones) != 0 {
		t.Fatalf("archivar no lo saca de los resultados: %v", hilos(b.P.Bandeja.Conversaciones))
	}
	b.P.Avisos.Avisos()[0].AlPulsarAccion()
	if s.pidio("Search") != busquedas+1 || !b.P.Bandeja.Buscando {
		t.Errorf("deshacer en una búsqueda no la relanza: %v", s.llamadas)
	}
	if s.pidio("Messages") != listas {
		t.Error("deshacer en una búsqueda ha recargado la carpeta, que no se ve")
	}
	b.Buscar("")
	if s.pidio("Messages") != listas+1 {
		t.Error("al salir de la búsqueda no se vuelve a pedir la carpeta que cambió")
	}
}
