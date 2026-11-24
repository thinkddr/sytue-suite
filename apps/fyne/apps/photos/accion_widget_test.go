// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"
	"time"
)

// TestAnyadirFotosDelWidgetAbreElSelector: la acción «Añadir fotos» del widget
// (`sytue-photos:/?action=upload`) lleva a «Fotos» y abre el selector del sistema, el
// del botón de la galería; sin elegir nada no se sube nada. Con el editor abierto no
// hace nada: no se cierra a medias lo que se está editando.
func TestAnyadirFotosDelWidgetAbreElSelector(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(3), porPagina: 10}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	abierto := make(chan struct{}, 4)
	a.Elegir = func() ([]string, error) { abierto <- struct{}{}; return nil, nil }
	a.Ir(RutaAlbumes)
	a.AnyadirFotos()
	select {
	case <-abierto:
	case <-time.After(5 * time.Second):
		t.Fatal("no se abrió el selector")
	}
	if a.ruta != RutaFotos {
		t.Fatalf("la acción deja la galería en %v, no en «Fotos»", a.ruta)
	}
	a.ed = &estadoEditor{}
	a.AnyadirFotos()
	select {
	case <-abierto:
		t.Fatal("con el editor abierto se abrió el selector")
	case <-time.After(100 * time.Millisecond):
	}
	a.ed = nil
}
