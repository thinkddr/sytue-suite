// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestVallaDeFuentePrivadaEsUn409: la valla de la 0105 rechaza con PST01 la
// escritura sobre filas de una fuente congelada. Llegue cruda (sin pasar por
// traducirError) o ya traducida, los dos traductores la sirven como 409
// private_storage_frozen y no como un 500.
func TestVallaDeFuentePrivadaEsUn409(t *testing.T) {
	crudo := fmt.Errorf("actualizar: %w", &pgconn.PgError{Code: "PST01", Message: "private_storage_legacy_write_blocked"})
	for _, err := range []error{crudo, store.ErrPrivateStorageFrozen} {
		for nombre, traducir := range map[string]func(error) error{
			"escritura": errorDeEscritura,
			"lectura":   errorDeLectura,
		} {
			var api *httpx.Error
			if !errors.As(traducir(err), &api) || api.Status != http.StatusConflict || api.Code != "private_storage_frozen" {
				t.Fatalf("%s(%v) = %v; quería 409 private_storage_frozen", nombre, err, traducir(err))
			}
		}
	}
}
