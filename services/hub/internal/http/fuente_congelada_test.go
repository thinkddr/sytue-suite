// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// Marcar o cambiar preferencias con la fuente de One congelada (guarda de la 0015) es
// un 409 con el mismo código que la fuente privada, no un 500.
func TestFuenteCongeladaEs409(t *testing.T) {
	var e *httpx.Error
	if !errors.As(toAPIError(fmt.Errorf("marcar: %w", store.ErrPrivateStorageFrozen)), &e) || e.Status != http.StatusConflict || e.Code != "private_storage_frozen" {
		t.Fatalf("toAPIError(ErrPrivateStorageFrozen) = %v", e)
	}
}

// Sin autoridad de Rooms, la entrada privada dice 503 private_ingress_required: los
// avisos esperan en la copia, no se rechazan como si no existieran.
func TestEntradaSinAutoridadEs503(t *testing.T) {
	for _, err := range []error{store.ErrEntradaPrivada, callaccess.ErrUnavailable} {
		if e := hubIngressError(err); e.Status != http.StatusServiceUnavailable || e.Code != "private_ingress_required" {
			t.Fatalf("hubIngressError(%v) = %v", err, e)
		}
	}
	if e := hubIngressError(store.ErrPrivateStorageEpoch); e.Status != http.StatusConflict {
		t.Fatalf("época cambiada: %v", e)
	}
}
