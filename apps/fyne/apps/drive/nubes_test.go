// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestLasNubesSeEligenDesdeElMenuYAtrasSubeCarpeta(t *testing.T) {
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	if p.lateral.seccionNubes(tema.Claro) != nil {
		t.Fatal("sin nubes cargadas no hay sección")
	}
	g := api.NubePersonal{InstanceID: "g", Provider: "google-drive", Connected: true}
	p.lateral.Nubes = &api.NubesPersonales{
		Items:     []api.NubePersonal{g},
		Providers: []api.ProveedorNube{{Provider: "google-drive", Available: true}},
	}
	if p.lateral.seccionNubes(tema.Claro) == nil {
		t.Fatal("la nube conectada no sale en el menú")
	}
	p.lateral.AlElegirNube(g)
	if p.seccion != Nube || p.lateral.NubeActiva != "g" || p.sec.nube.Nube.InstanceID != "g" {
		t.Fatal("elegir la nube no la abre")
	}
	q := p.sec.nube
	q.Fase, q.Cursor = Exito, "siguiente"
	q.Elementos = []api.ElementoNube{{ID: "1", Name: "Actas", Folder: true}, {ID: "2", Name: "a.pdf", Size: 10}}
	if p.vista(tema.Claro) == nil {
		t.Fatal("la vista de la nube no se pinta")
	}
	q.Pila = []api.ElementoNube{{ID: "1"}, {ID: "3"}}
	q.AlIr = func(n int) { q.Pila = q.Pila[:n+1] }
	p.atras()
	if len(q.Pila) != 1 || q.carpeta() != "1" {
		t.Fatalf("atrás no sube a la carpeta superior: %+v", q.Pila)
	}
	p.atras()
	if len(q.Pila) != 0 || q.carpeta() != "" {
		t.Fatal("atrás no vuelve a la raíz de la nube")
	}
	p.atras()
	if p.seccion != MiUnidad {
		t.Fatal("en la raíz de la nube atrás vuelve a Mi unidad")
	}
}
