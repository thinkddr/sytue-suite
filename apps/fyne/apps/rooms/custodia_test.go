// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// Rooms guarda la clave envuelta de su cuenta en su propio servicio (ADR 0082): el
// panel de Rooms usa la API de Rooms como custodia. Sin API (una Sala de prueba)
// no hay custodia, y no una interfaz con un *api.Rooms nil dentro. La lógica de
// la custodia (desbloquear, crear, envolver, restablecer) se prueba en cifrado.
func TestRoomsUsaSuServicioComoCustodia(t *testing.T) {
	sa, _, _ := salaEn(t, &servicioFalso{}, 640)
	service := api.NewRooms("https://rooms.test", nil, nil)
	if c := NuevoCifrado(sa, service, mx.Config{}, nil); c.Custodio != service {
		t.Fatal("el panel de Rooms no usa su API como custodia")
	}
	if c := NuevoCifrado(sa, nil, mx.Config{}, nil); c.Custodio != nil {
		t.Fatal("sin API hay una custodia con un *api.Rooms nil dentro")
	}
}
