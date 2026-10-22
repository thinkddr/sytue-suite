// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// La revalidación de la miniatura vive y muere aquí: si `coincideEtiqueta` no
// entiende lo que manda un navegador de verdad, nunca acierta, cada visita se
// trae las cien miniaturas otra vez y nadie lo nota porque «funciona».
func TestCoincideEtiqueta(t *testing.T) {
	const huella = "ec5adbfaaa1f"
	etiqueta := etiquetaDe(huella)
	if etiqueta != `"ec5adbfaaa1f"` {
		t.Fatalf("etiquetaDe(%q) = %s", huella, etiqueta)
	}
	casos := []struct {
		cabecera string
		quiero   bool
	}{
		{`"ec5adbfaaa1f"`, true},
		{`W/"ec5adbfaaa1f"`, true},            // débil: el proxy la pudo marcar
		{`"otra", "ec5adbfaaa1f"`, true},      // lista: el navegador manda varias
		{` W/"otra" , "ec5adbfaaa1f" `, true}, // con espacios de sobra
		{`*`, true},                           // comodín
		{`"1119c7c11660"`, false},             // otra edición de la misma foto
		{"", false},                           // primera visita
		{`"ec5adbfaaa1f-mas"`, false},         // prefijo, no la misma
		{`ec5adbfaaa1f`, false},               // sin comillas no es un ETag
	}
	for _, c := range casos {
		if got := coincideEtiqueta(c.cabecera, etiqueta); got != c.quiero {
			t.Errorf("coincideEtiqueta(%q, %s) = %v; quería %v", c.cabecera, etiqueta, got, c.quiero)
		}
	}
}

// La huella de la receta vacía es el nombre del original, y el navegador la
// calcula por su cuenta para abrir el editor (src/editor/receta.ts). Si las dos
// implementaciones dejaran de coincidir, el editor pediría una derivada que el
// servicio no fabrica y no abriría ninguna foto editada.
func TestHuellaVacia_esElVectorCompartido(t *testing.T) {
	if huellaVacia != "ec5adbfaaa1f" {
		t.Fatalf("huella de la receta vacía = %q; el vector compartido es ec5adbfaaa1f", huellaVacia)
	}
}

func TestRecetaDeLaHuella(t *testing.T) {
	vigente := domain.Receta{Version: domain.VersionReceta, Pasos: []domain.Paso{
		{Tipo: domain.PasoGiro, Params: map[string]float64{"grados": 90}},
	}}
	suHuella, err := vigente.Huella()
	if err != nil {
		t.Fatal(err)
	}

	// La vigente: lo que se ve en la galería.
	if r, ok := recetaDeLaHuella(vigente, suHuella, suHuella); !ok || len(r.Pasos) != 1 {
		t.Fatalf("la vigente se deriva: %v %+v", ok, r)
	}
	// La vacía: el original que abre el editor, aunque la foto esté editada.
	if r, ok := recetaDeLaHuella(vigente, suHuella, huellaVacia); !ok || len(r.Pasos) != 0 {
		t.Fatalf("la vacía se deriva y va sin pasos: %v %+v", ok, r)
	}
	// Cualquier otra, no: fabricarla sería archivar «algo» con nombre ajeno.
	if _, ok := recetaDeLaHuella(vigente, suHuella, "1119c7c11660"); ok {
		t.Fatal("una huella cualquiera no se fabrica")
	}
}
