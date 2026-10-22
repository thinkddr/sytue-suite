// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"path/filepath"
	"testing"
)

// TestSoloOpenGL21OMasAbreVentana: el «1.1.0» de GDI Generic (Windows sin controlador)
// no llega; lo de los controladores de verdad y el de Mesa, sí.
func TestSoloOpenGL21OMasAbreVentana(t *testing.T) {
	for v, quiere := range map[string]bool{
		"1.1.0": false, "2.0": false, "": false, "raro": false,
		"2.1": true, "2.1 Mesa 26.2.3": true, "4.6.0 NVIDIA 555.85": true, "3.3.0 - Build 31.0.101": true,
	} {
		if versionSuficiente(v) != quiere {
			t.Errorf("versionSuficiente(%q) = %v", v, !quiere)
		}
	}
}

// TestMesaSoloEnElProcesoDeReserva: el directorio de Mesa se pone SOLO en el proceso
// que se relanzó porque el OpenGL del sistema no llegaba. Si se pusiera siempre, Mesa
// iría por delante del controlador de la tarjeta en todos los equipos. Y el relanzado no
// vuelve a relanzar.
func TestMesaSoloEnElProcesoDeReserva(t *testing.T) {
	var puesto []string
	antes := ponerDirectorioDLL
	t.Cleanup(func() { ponerDirectorioDLL = antes })
	ponerDirectorioDLL = func(d string) error { puesto = append(puesto, d); return nil }

	t.Setenv(envReserva, "")
	_ = Graficos()
	if len(puesto) != 0 {
		t.Fatalf("sin relanzar se ha puesto %q como directorio de DLL: Mesa iría delante del controlador", puesto)
	}

	t.Setenv(envReserva, "1")
	_ = Graficos()
	if len(puesto) != 1 || filepath.Base(puesto[0]) != "mesa" {
		t.Fatalf("en el relanzado el directorio de DLL es %q; quiero la carpeta mesa", puesto)
	}
	if RelanzarConReserva() {
		t.Fatal("el proceso relanzado ha vuelto a relanzarse")
	}
}
