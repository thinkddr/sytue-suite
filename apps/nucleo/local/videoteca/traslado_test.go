// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Los vídeos copiados en la carpeta de datos pasan a la nueva; los apuntados de fuera,
// no. Sin índice, nada.
func TestTrasladar(t *testing.T) {
	base := t.TempDir()
	vieja, nueva := filepath.Join(base, "vieja"), filepath.Join(base, "nueva")
	datos := filepath.Join(nueva, "Datos", "cast")
	fuera := filepath.Join(base, "Películas", "boda.mp4")
	lista := []*Entrada{
		{ID: "a", Ruta: filepath.Join(vieja, "Vídeos", "viaje.webm"), Titulo: "Viaje", Copiado: true},
		{ID: "b", Ruta: fuera, Titulo: "Boda"},
		{ID: "c", Ruta: filepath.Join(base, "viejas", "x.mp4"), Titulo: "Otra carpeta que empieza igual"},
	}
	if err := espacio.EscribirJSON(filepath.Join(datos, "videoteca.json"), lista); err != nil {
		t.Fatal(err)
	}
	if err := Trasladar(datos, vieja, nueva); err != nil {
		t.Fatal(err)
	}
	var got []*Entrada
	if err := espacio.LeerJSON(filepath.Join(datos, "videoteca.json"), &got); err != nil {
		t.Fatal(err)
	}
	if got[0].Ruta != filepath.Join(nueva, "Vídeos", "viaje.webm") || got[1].Ruta != fuera || got[2].Ruta != lista[2].Ruta || got[0].Titulo != "Viaje" || !got[0].Copiado {
		t.Errorf("%+v %+v %+v", got[0], got[1], got[2])
	}
	if err := Trasladar(t.TempDir(), vieja, nueva); err != nil {
		t.Errorf("sin índice: %v", err)
	}
}
