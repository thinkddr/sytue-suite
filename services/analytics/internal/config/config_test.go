// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests del cargador de configuración de Sytue Analytics. Deterministas, sin BD
// ni red (solo lectura de variables de entorno).
package config

import "testing"

// baseValida devuelve una Config que pasa validate() en dev, para que cada test
// cambie SOLO lo que quiere probar.
func baseValida() Config {
	return Config{
		RawRetentionDays: 2,
		PartitionsAhead:  7,
		BatchSize:        100,
		QueueSize:        1000,
		BreakdownTopN:    10,
	}
}

// TestValidateProdExigeKEK: sin KEK, keycrypto no falla —devuelve el texto plano—
// y la sal diaria queda legible en la base, con lo que un volcado basta para
// recalcular los identificadores del día. El colector lo avisaba en el log y la
// API ni eso. En prod es un error de arranque; en dev se sigue permitiendo.
func TestValidateProdExigeKEK(t *testing.T) {
	c := baseValida()
	c.Env = "prod"
	if err := c.validate(); err == nil {
		t.Fatal("prod sin SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY debe fallar")
	}
	c.KeyEncryptionKey = make([]byte, 32)
	if err := c.validate(); err != nil {
		t.Fatalf("prod con KEK: %v", err)
	}
	// Dev sigue arrancando sin clave: no hay tráfico real que anonimizar.
	c.Env, c.KeyEncryptionKey = "dev", nil
	if err := c.validate(); err != nil {
		t.Fatalf("dev sin KEK debe seguir valiendo: %v", err)
	}
}
