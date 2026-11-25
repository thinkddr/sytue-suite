// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// TestSinLaClaveElDespertarNoEsUnFallo: con la app al fondo o cerrada, Drive, CertiPDF y
// las apps de registros en Matrix no pueden leer su repositorio privado
// (fuentes.ErrSinDatosPrivados). Eso no es un fallo: se le contesta al sistema «sin
// cambios», lo publicado se queda y, ya rancio, el widget dice que se actualiza con la
// app abierta. Contestar «falló» gastaba el presupuesto de refresco de iOS y hacía que
// Android reintentara con espera creciente. Un fallo de verdad lo sigue siendo.
func TestSinLaClaveElDespertarNoEsUnFallo(t *testing.T) {
	for _, err := range []error{fuentes.ErrSinDatosPrivados, fmt.Errorf("ronda: %w", fuentes.ErrSinDatosPrivados)} {
		if r := resultadoDeRonda("eu.sytue.drive", widgets.Snapshot{}, err); r != SinCambios {
			t.Errorf("%v: %v, quiero SinCambios", err, r)
		}
	}
	if r := resultadoDeRonda("eu.sytue.drive", widgets.Snapshot{}, errors.New("red caída")); r != Fallo {
		t.Errorf("un fallo de red contesta %v", r)
	}
}
