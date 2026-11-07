// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"errors"
	"testing"
)

// TestDeshacerEstaVaASuAccion: el «Deshacer» de un aviso deshace SU paso aunque después
// se hiciera otro, y la que era la última pasa a rehacer. Si falla, vuelve a su sitio.
// Es el `deshacerEsta` de historial.test.ts.
func TestDeshacerEstaVaASuAccion(t *testing.T) {
	var r []string
	paso := func(n string, err error) Accion {
		return Accion{Etiqueta: n, Deshacer: func() error {
			if err != nil {
				return err
			}
			r = append(r, "undo "+n)
			return nil
		}}
	}
	var h Historial
	a := h.Registrar(paso("a", nil), "")
	h.Registrar(paso("b", nil), "")
	if et, err := h.DeshacerEsta(a); err != nil || et != "a" {
		t.Fatalf("DeshacerEsta(a) = %q, %v", et, err)
	}
	if len(r) != 1 || r[0] != "undo a" || h.SiguienteDeshacer() != "b" || h.SiguienteRehacer() != "" {
		t.Fatalf("después: %v, deshacer %q, rehacer %q", r, h.SiguienteDeshacer(), h.SiguienteRehacer())
	}
	if et, _ := h.DeshacerEsta(a); et != "" {
		t.Fatalf("deshacer dos veces la misma: %q", et)
	}

	c := h.Registrar(paso("c", nil), "")
	if et, _ := h.DeshacerEsta(c); et != "c" || h.SiguienteRehacer() != "c" {
		t.Fatalf("la última deshecha desde el aviso no queda para rehacer: %q", h.SiguienteRehacer())
	}

	mala := h.Registrar(paso("mala", errors.New("no")), "")
	h.Registrar(paso("d", nil), "")
	if _, err := h.DeshacerEsta(mala); err == nil {
		t.Fatal("el fallo de la inversa se ha perdido")
	}
	if _, err := h.Deshacer(); err != nil || h.SiguienteDeshacer() != "mala" {
		t.Fatalf("la que falló no ha vuelto a su sitio: lo siguiente es %q", h.SiguienteDeshacer())
	}
}

// TestDeshacerEstaNoConfundeHistoriales: un historial nuevo (el de después de cerrar
// sesión) no reparte números que ya dio el anterior, así que el «Deshacer» de un aviso
// viejo no deshace una acción nueva. VISTO EN ROJO con el contador dentro del struct.
func TestDeshacerEstaNoConfundeHistoriales(t *testing.T) {
	var viejo Historial
	paso := viejo.Registrar(Accion{Etiqueta: "vieja", Deshacer: func() error { return nil }}, "")
	nuevo := Historial{}
	deshecha := false
	nuevo.Registrar(Accion{Etiqueta: "nueva", Deshacer: func() error { deshecha = true; return nil }}, "")
	if et, err := nuevo.DeshacerEsta(paso); err != nil || et != "" || deshecha {
		t.Fatalf("el aviso viejo ha deshecho %q en el historial nuevo", et)
	}
}
