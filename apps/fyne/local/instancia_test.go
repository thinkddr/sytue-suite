// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"net/url"
	"testing"
	"time"
)

// Abrir una app otra vez le pasa sus enlaces a la copia viva y la trae, en vez de poner
// otra sobre el mismo almacén.
func TestLaSegundaCopiaPasaSusEnlacesALaPrimera(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	id := "eu.sytue.office.prueba"
	u, _ := url.Parse("file:///tmp/Informe%20final.docx")
	if PasarAOtraCopia(id, []*url.URL{u}) || CopiaViva(id) {
		t.Fatal("sin copia viva no hay a quién pasarle nada")
	}
	enlaces := make(chan string, 2)
	traida := make(chan struct{}, 1)
	EscucharCopias(t.Context(), id, func(u *url.URL) { enlaces <- u.String() }, func() { traida <- struct{}{} })
	if !CopiaViva(id) || !PasarAOtraCopia(id, []*url.URL{u}) {
		t.Fatal("con copia viva, se le pasa")
	}
	select {
	case got := <-enlaces:
		if got != u.String() {
			t.Fatalf("enlace = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la copia viva no recibió el enlace")
	}
	select {
	case <-traida:
	case <-time.After(5 * time.Second):
		t.Fatal("la copia viva no se ha enseñado")
	}
}
