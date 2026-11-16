// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net/url"
	"testing"
)

func TestEnlacesNoCambianDeIdentidadAlEsperarEnLaCola(t *testing.T) {
	var cola []func()
	var abiertos []string
	actual := "A:revision-1"
	e := &enlacesIdentidad{
		despachar: func(fn func()) { cola = append(cola, fn) },
		abrir:     func(u *url.URL) { abiertos = append(abiertos, u.RawQuery) },
	}
	e.recibir(&url.URL{RawQuery: "action=create&title=sin-sesion"})
	antigua, pararAntigua := context.WithCancel(context.Background())
	t.Cleanup(pararAntigua)
	e.activar(antigua, func() bool { return actual == "A:revision-1" })
	if len(cola) != 1 {
		t.Fatal("el enlace recibido sin sesión no se conservó")
	}
	cola[0]()
	cola = nil
	e.recibir(&url.URL{RawQuery: "action=create&title=anterior"})
	if len(cola) != 1 {
		t.Fatal("el enlace de la sesión anterior no se encoló")
	}
	anteriorEncolada := cola[0]
	cola = nil
	actual = "B:revision-2"
	actual = "A:revision-3"
	// Rechaza la revisión vieja incluso antes de recibir el aviso de cambio.
	anteriorEncolada()
	pararAntigua()
	e.retirar()
	nueva, pararNueva := context.WithCancel(context.Background())
	t.Cleanup(pararNueva)
	e.activar(nueva, func() bool { return actual == "A:revision-3" })
	anteriorEncolada()
	e.recibir(&url.URL{RawQuery: "action=create&title=nueva"})
	if len(cola) != 1 {
		t.Fatal("el enlace de la sesión nueva no se encoló")
	}
	cola[0]()
	if len(abiertos) != 2 || abiertos[0] != "action=create&title=sin-sesion" || abiertos[1] != "action=create&title=nueva" {
		t.Fatalf("enlaces abiertos: %q", abiertos)
	}
	cola = nil
	e.recibir(&url.URL{RawQuery: "action=create&title=al-cerrar"})
	if len(cola) != 1 {
		t.Fatal("el enlace anterior al cierre no se encoló")
	}
	e.cerrar()
	cola[0]()
	e.recibir(&url.URL{RawQuery: "action=create&title=despues"})
	if len(abiertos) != 2 || len(cola) != 1 {
		t.Fatal("la ventana cerrada aceptó un enlace")
	}
}
