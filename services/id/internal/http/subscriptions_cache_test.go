// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// La cache del derecho de suscripción ACELERA, no decide. Lo que se fija aquí es
// que un Server SIN cache —el caso de dev, de test y de cualquier producción donde
// Dragonfly no esté— se comporta exactamente como antes de que la cache existiera.
//
// Importa porque Sytue ID es la puerta de entrada de TODA la suite: si dependiera
// de un servicio que por contrato puede desaparecer entero (una réplica, sin
// persistencia, con evicción), una cache caída dejaría a todo el mundo fuera.
func TestSinCacheElDerechoSeConsultaComoSiempre(t *testing.T) {
	s := &Server{} // cache nil a propósito: es el estado por defecto
	if s.cache.Enabled() {
		t.Fatal("un Server sin cache la declara disponible")
	}
	// Y la invalidación tiene que ser un no-op silencioso, no un panic: se llama
	// desde los caminos de pago y de asientos, que deben funcionar sin Dragonfly.
	s.invalidarEntitled(t.Context(), "org-cualquiera")
}

// La clave lleva el ID de la organización dentro. Es lo único que impide que el
// derecho de una organización se sirva a otra, así que se comprueba explícitamente:
// una cache multi-tenant con la clave mal construida no es una optimización, es un
// fallo de aislamiento.
func TestLaClaveSeparaOrganizaciones(t *testing.T) {
	a, b := entitledKey("org-a"), entitledKey("org-b")
	if a == b {
		t.Fatal("dos organizaciones comparten clave de cache")
	}
	if a != "id:entitled:org-a" {
		t.Errorf("clave inesperada: %q", a)
	}
}

// El TTL es corto a propósito: cachear el derecho es cachear el «sí» a cobrar, y
// el retraso máximo con el que se aplica una baja no debe medirse en minutos. Si
// alguien lo sube, este test le obliga a pasar por aquí y justificarlo.
func TestElDerechoNoSeCacheaMasDeUnMinuto(t *testing.T) {
	if entitledTTL.Minutes() > 1 {
		t.Errorf("el derecho se cachea %v: es tiempo de servicio regalado tras una baja", entitledTTL)
	}
}
