// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// El porcentaje de CPU es una división contra el límite del TAMAÑO, y de ella
// depende que el aviso llegue o no. Con `basic` (500m), 480m es el 96 %: si el
// cálculo tomara otro techo —el de la máquina, o milicores como si fueran
// porcentaje— no avisaría nunca, y el fallo sería invisible porque «no llegan
// alertas» se lee igual que «todo va bien».
func TestMetricUsageOfCPU(t *testing.T) {
	t.Parallel()
	lim := int64(InstanceSizeOrDefault("basic").CPULimitMilli())
	if lim <= 0 {
		t.Fatalf("el tamaño basic debe declarar límite de CPU, es %d", lim)
	}
	casos := []struct {
		nombre  string
		milli   int64
		quiero  int
		avisado bool
	}{
		{"holgado", lim / 2, 50, false},
		{"justo en el umbral", lim * int64(MetricAlertPercent) / 100, MetricAlertPercent, true},
		{"al tope", lim, 100, true},
		{"parado", 0, 0, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			u := MetricUsageOf(SizingAdvice{From: "basic", P95CPUMilli: c.milli})
			if u[0].Resource != MetricResourceCPU {
				t.Fatalf("el primer recurso debe ser cpu, es %q", u[0].Resource)
			}
			if u[0].Percent != c.quiero {
				t.Errorf("porcentaje = %d, quiero %d", u[0].Percent, c.quiero)
			}
			if u[0].Over != c.avisado {
				t.Errorf("avisa = %v, quiero %v (umbral %d %%)", u[0].Over, c.avisado, MetricAlertPercent)
			}
		})
	}
}

// La memoria llega ya en porcentaje del motor de tamaño: lo que se comprueba es
// que se compara con el mismo umbral y no se queda por el camino.
func TestMetricUsageOfMemoria(t *testing.T) {
	t.Parallel()
	u := MetricUsageOf(SizingAdvice{From: "basic", P95MemoryPercent: MetricAlertPercent - 1})
	if u[1].Resource != MetricResourceMemory || u[1].Over {
		t.Errorf("por debajo del umbral no se avisa: %+v", u[1])
	}
	u = MetricUsageOf(SizingAdvice{From: "basic", P95MemoryPercent: 97})
	if !u[1].Over || u[1].Percent != 97 {
		t.Errorf("97 %% debe avisar: %+v", u[1])
	}
}

// Un tamaño desconocido no puede inventarse un techo: sin límite declarado no hay
// «cerca del límite», y avisar ahí sería ruido que enseña a ignorar el canal.
func TestMetricUsageOfSinLimiteDeCPU(t *testing.T) {
	t.Parallel()
	// El catálogo devuelve el tamaño por defecto para un nombre que no existe, así
	// que esto comprueba lo contrario de lo que parece: que no explota y que sigue
	// midiendo contra un techo real.
	u := MetricUsageOf(SizingAdvice{From: "no-existe", P95CPUMilli: 1})
	if u[0].Percent < 0 || u[0].Over {
		t.Errorf("un milicore no puede disparar el aviso: %+v", u[0])
	}
}
