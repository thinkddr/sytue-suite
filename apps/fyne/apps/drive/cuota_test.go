// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestElLateralDistingueAmpliacionDesconocidaYCuotaBase(t *testing.T) {
	tr, err := i18n.New("en", i18n.Common(), catalogos.Drive())
	if err != nil {
		t.Fatal(err)
	}
	baseTope, baseUsado, total, usado, libre := int64(100), int64(20), int64(1024), int64(200), int64(824)
	lateral := NuevoLateral(tr, MiUnidad, nil)
	lateral.Cuota = &Cuota{
		Usado: 220, Tope: 1124, BaseUsado: &baseUsado, BaseTope: &baseTope, Externo: true,
		CapacidadExterna: &api.DriveExternalCapacity{TotalBytes: &total, UsedBytes: &usado, AvailableBytes: &libre},
	}
	notas := lateral.notasCuota()
	if len(notas) != 3 || notas[1] != "Connected storage adds 1.0 KB." || notas[2] != "Sytue: 20 B of 100 B" {
		t.Fatalf("ampliación y base no separadas: %v", notas)
	}
	lateral.Cuota.CapacidadExterna.Partial = true
	notas = lateral.notasCuota()
	if len(notas) != 4 || notas[1] != "Connected storage adds 1.0 KB." || notas[2] != "The provider does not report its capacity." || notas[3] != "Sytue: 20 B of 100 B" {
		t.Fatalf("capacidad conocida o aviso parcial perdidos: %v", notas)
	}
	// Un proveedor que sólo declara su total todavía no permite combinar cuotas.
	lateral.Cuota.CapacidadExterna.UsedBytes = nil
	notas = lateral.notasCuota()
	if len(notas) != 3 || notas[1] != "The provider does not report its capacity." || strings.Contains(notas[1], "0 B") {
		t.Fatalf("capacidad desconocida inventada: %v", notas)
	}
	// La respuesta anterior no tenía campos base: sus totales eran la cuota Sytue.
	lateral.Cuota = &Cuota{Usado: 20, Tope: 100, Externo: true}
	if notas = lateral.notasCuota(); notas[2] != "Sytue: 20 B of 100 B" {
		t.Fatalf("base del backend anterior perdida: %v", notas)
	}
	lateral.Cuota.Externo = false
	if notas = lateral.notasCuota(); len(notas) != 0 {
		t.Fatalf("ampliación tras desconectar: %v", notas)
	}
}
