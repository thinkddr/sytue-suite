// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// La salida admite los dos destinos que tienen escritor; sin esto el asistente
// ofrecía Microsoft 365 y el alta del trabajo lo rechazaba con un 400.
func TestValidOutbound(t *testing.T) {
	ficheros := []domain.MigrationDomain{domain.DomainDrive, domain.DomainContacts}
	for _, src := range []domain.MigrationSource{domain.SourceGoogle, domain.SourceM365} {
		if err := validOutbound(src, ficheros); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	if validOutbound(domain.SourceGitHub, ficheros) == nil {
		t.Error("GitHub no es un destino de salida")
	}
}
