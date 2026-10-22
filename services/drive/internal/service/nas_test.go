// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"
)

// direccionLocal es la verja del túnel: lo que acepte se publica en
// nas-<tenant>.<dominio> y el agente del cliente reenvía allí. Si dejara pasar
// una dirección pública, cualquiera podría hacer que el túnel de su NAS
// apuntara a un tercero y usar el certificado de Sytue para servirlo.
func TestDireccionLocalSoloAceptaDireccionesPrivadas(t *testing.T) {
	buenas := map[string]struct {
		esquema string
		host    string
		puerto  int
	}{
		"http://192.168.1.50:5005": {"http", "192.168.1.50", 5005},
		"https://10.0.0.4:5006":    {"https", "10.0.0.4", 5006},
		"http://172.16.3.9":        {"http", "172.16.3.9", 80},
		"https://nas.local":        {"https", "nas.local", 443},
	}
	for raw, q := range buenas {
		esquema, host, puerto, err := direccionLocal(raw)
		if err != nil || esquema != q.esquema || host != q.host || puerto != q.puerto {
			t.Fatalf("%s: %q %q %d (%v)", raw, esquema, host, puerto, err)
		}
	}
	for _, mala := range []string{
		"http://8.8.8.8:5005",       // pública
		"https://evil.example.com",  // nombre público con IP pública detrás
		"ftp://192.168.1.50",        // esquema que el proxy no sabe hablar
		"192.168.1.50:5005",         // sin esquema
		"http://192.168.1.50:99999", // puerto imposible
		"",                          // vacío
	} {
		if _, _, _, err := direccionLocal(mala); err == nil {
			t.Fatalf("%q debería rechazarse", mala)
		}
	}
}
