// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func TestValidatePVCSize(t *testing.T) {
	for _, c := range []struct {
		size string
		ok   bool
		por  string
	}{
		{"", true, "vacío = «usa el de tu plan»"},
		{"1Gi", true, "el mínimo"},
		{"10Gi", true, "lo habitual"},
		{"4096Gi", true, "el máximo justo"},
		{"1024Mi", true, "1 GiB escrito en Mi"},
		{"2Ti", true, "en Ti"},

		// El valor que hacía pánico en el reconciliador y dejaba el servicio
		// colgado en 'provisioning' sin que nadie supiera por qué.
		{"grande", false, "no es una cantidad"},
		{"", true, ""},
		{"10", false, "sin unidad: ambiguo (¿bytes?)"},
		{"10GB", false, "GB no es Gi y la diferencia se factura"},
		{"1.5Gi", false, "decimal: se escribe 1536Mi, que no admite dos lecturas"},
		{"10gi", false, "las unidades de Kubernetes distinguen mayúsculas"},
		{"0Gi", false, "un disco de cero"},
		{"512Mi", false, "medio GiB: por debajo del mínimo"},
		{"-5Gi", false, "negativo"},
		{"99999Ti", false, "por encima del tope de cordura"},
		{" 10Gi", false, "espacio delante"},
		{"10Gi ", false, "espacio detrás"},
	} {
		if c.por == "" {
			continue
		}
		err := domain.ValidatePVCSize(c.size)
		if c.ok && err != nil {
			t.Errorf("%q debería valer (%s): %v", c.size, c.por, err)
		}
		if !c.ok {
			if err == nil {
				t.Errorf("%q NO debería valer (%s)", c.size, c.por)
				continue
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("%q: el error tiene que ser ErrInvalidInput para que salga 400 y no 500: %v", c.size, err)
			}
		}
	}
}

// TestPVCSizeAceptadoLoParseaKubernetes fija la asimetría deliberada: la
// validación es MÁS ESTRECHA que Kubernetes, así que todo lo que aquí pasa allí
// también. Al revés no, y da igual.
func TestPVCSizeAceptadoLoParseaKubernetes(t *testing.T) {
	// Muestra de lo que Kubernetes admite y nosotros rechazamos a propósito: no
	// es un fallo, es no dejar entrar formas que se leen de dos maneras.
	for _, s := range []string{"1.5Gi", "10G", "1e3", "10M"} {
		if err := domain.ValidatePVCSize(s); err == nil {
			t.Errorf("%q lo admite Kubernetes pero nosotros no debemos: es ambiguo o SI en vez de binario", s)
		}
	}
}
