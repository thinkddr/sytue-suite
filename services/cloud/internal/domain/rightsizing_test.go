// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const mib = 1 << 20

// muestras genera n muestras espaciadas 15 min con la memoria dada (bytes) y una
// CPU tranquila. base marca el instante de la primera.
func muestras(n int, memBytes int64) []domain.ResourceSample {
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	out := make([]domain.ResourceSample, 0, n)
	for i := range n {
		out = append(out, domain.ResourceSample{
			At:          base.Add(time.Duration(i) * 15 * time.Minute),
			MemoryBytes: memBytes,
			CPUMilli:    50,
		})
	}
	return out
}

// TestSinDatosNoRecomienda es el requisito más importante de los tres: adivinar
// aquí es peor que callarse, porque el cliente actúa sobre lo que le decimos.
func TestSinDatosNoRecomienda(t *testing.T) {
	for _, c := range []struct {
		nombre string
		s      []domain.ResourceSample
	}{
		{"ninguna muestra", nil},
		{"pocas muestras", muestras(10, 40*mib)},
		{"muchas muestras pero ventana corta", func() []domain.ResourceSample {
			// 300 muestras en el MISMO instante: hay volumen, no hay historia.
			s := muestras(300, 40*mib)
			for i := range s {
				s[i].At = s[0].At
			}
			return s
		}()},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			adv := domain.RecommendInstanceSize("standard", 1, c.s)
			if adv.Action != domain.SizingKeep || adv.Reason != domain.ReasonInsufficientData {
				t.Fatalf("con %s se esperaba keep/insufficient_data, salió %s/%s",
					c.nombre, adv.Action, adv.Reason)
			}
			if adv.MonthlyDeltaEur != 0 {
				t.Errorf("sin recomendación no puede haber importe: %.2f", adv.MonthlyDeltaEur)
			}
		})
	}
}

// TestRozarElTechoSube: subir se propone con poca evidencia porque protege el
// servicio. Con 8 h de ventana ya basta.
func TestRozarElTechoSube(t *testing.T) {
	// standard = 1 GiB de límite; 900 MiB es el 88 %.
	adv := domain.RecommendInstanceSize("standard", 2, muestras(80, 900*mib))
	if adv.Action != domain.SizingUp || adv.Reason != domain.ReasonNearLimit {
		t.Fatalf("se esperaba up/near_limit, salió %s/%s", adv.Action, adv.Reason)
	}
	if adv.To != "pro" {
		t.Errorf("destino = %q; se esperaba pro", adv.To)
	}
	// pro (2 GiB) - standard (1 GiB) = 1 GiB × 2 réplicas × 730 h × 0,009 €.
	if want := 1.0 * 2 * 730 * 0.009; !casiIgual(adv.MonthlyDeltaEur, want) {
		t.Errorf("coste extra = %.2f €; se esperaba %.2f €", adv.MonthlyDeltaEur, want)
	}
	if adv.MonthlyDeltaEur <= 0 {
		t.Error("subir tiene que salir MÁS caro, no menos")
	}
}

// TestConsumoBajoSostenidoBaja: el caso que paga el producto.
func TestConsumoBajoSostenidoBaja(t *testing.T) {
	// standard = 1 GiB. 120 MiB es el 12 % de lo que paga; basic (512 MiB) le
	// sobra. 400 muestras × 15 min = más de 4 días.
	adv := domain.RecommendInstanceSize("standard", 3, muestras(400, 120*mib))
	if adv.Action != domain.SizingDown || adv.Reason != domain.ReasonOverProvisioned {
		t.Fatalf("se esperaba down/over_provisioned, salió %s/%s", adv.Action, adv.Reason)
	}
	if adv.To != "basic" {
		t.Errorf("destino = %q; se esperaba basic", adv.To)
	}
	if adv.MonthlyDeltaEur >= 0 {
		t.Errorf("bajar tiene que AHORRAR; salió %.2f €", adv.MonthlyDeltaEur)
	}
	// La cifra que la interfaz enseña como «usas el X % de lo que pagas».
	if adv.P95MemoryPercent != 12 {
		t.Errorf("p95 = %d %% del límite; se esperaba 12 %%", adv.P95MemoryPercent)
	}
}

// TestUnaSolaRafagaImpideBajar es la línea que separa un consejo de un OOMKill.
// El p95 ignora por definición el 5 % más alto, así que sin mirar el PICO esta
// app «ahorraría» y se caería al primer pico igual.
func TestUnaSolaRafagaImpideBajar(t *testing.T) {
	s := muestras(400, 120*mib)
	// Un único pico de 500 MiB: no mueve el p95, pero no cabe en basic (512 MiB).
	s[137].MemoryBytes = 500 * mib

	adv := domain.RecommendInstanceSize("standard", 1, s)
	if adv.Action != domain.SizingKeep {
		t.Fatalf("con un pico que no cabe NO se puede bajar; salió %s → %s", adv.Action, adv.To)
	}
	if adv.Reason != domain.ReasonPeakBlocksDowngrade {
		t.Errorf("motivo = %s; se esperaba peak_blocks_downgrade (hay que poder EXPLICAR por qué no se ahorra)", adv.Reason)
	}
	// Y el p95 sigue siendo bajo: si el motor mirara solo el p95, habría bajado.
	if adv.P95MemoryPercent > 20 {
		t.Fatalf("el p95 debería seguir bajo (%d %%): el test no prueba lo que dice", adv.P95MemoryPercent)
	}
}

// TestNoSeSaleDelCatalogo: en los extremos no hay a dónde mover, y hay que
// decirlo en vez de proponer un tamaño que no existe.
func TestNoSeSaleDelCatalogo(t *testing.T) {
	bajo := domain.RecommendInstanceSize("basic", 1, muestras(400, 20*mib))
	if bajo.Action != domain.SizingKeep || bajo.Reason != domain.ReasonAtSmallest {
		t.Errorf("en el menor: %s/%s; se esperaba keep/at_smallest", bajo.Action, bajo.Reason)
	}
	// pro-xl = 4 GiB; 3,8 GiB roza el techo y ya no hay tamaño mayor.
	alto := domain.RecommendInstanceSize("pro-xl", 1, muestras(80, 3891*mib))
	if alto.Action != domain.SizingKeep || alto.Reason != domain.ReasonAtLargest {
		t.Errorf("en el mayor: %s/%s; se esperaba keep/at_largest", alto.Action, alto.Reason)
	}
}

// TestBajarExigeMasVentanaQueSubir fija la asimetría deliberada: el MISMO
// consumo holgado no baja con ventana corta y sí con ventana larga.
func TestBajarExigeMasVentanaQueSubir(t *testing.T) {
	corta := domain.RecommendInstanceSize("standard", 1, muestras(100, 100*mib)) // 25 h
	if corta.Action != domain.SizingKeep || corta.Reason != domain.ReasonInsufficientData {
		t.Errorf("con 25 h no se puede proponer bajar: %s/%s", corta.Action, corta.Reason)
	}
	larga := domain.RecommendInstanceSize("standard", 1, muestras(400, 100*mib)) // 100 h
	if larga.Action != domain.SizingDown {
		t.Errorf("con 100 h del mismo consumo sí se baja; salió %s", larga.Action)
	}
}

// TestElAjustadoSeQuedaComoEsta: ni sobra ni falta.
func TestElAjustadoSeQuedaComoEsta(t *testing.T) {
	// 400 MiB en standard (1 GiB): el 39 %, pero en basic (512 MiB) sería el 78 %,
	// por encima del 60 % de holgura que se exige para bajar.
	adv := domain.RecommendInstanceSize("standard", 1, muestras(400, 400*mib))
	if adv.Action != domain.SizingKeep || adv.Reason != domain.ReasonWellSized {
		t.Errorf("se esperaba keep/well_sized, salió %s/%s", adv.Action, adv.Reason)
	}
}

func casiIgual(a, b float64) bool {
	d := a - b
	return d < 0.005 && d > -0.005
}
