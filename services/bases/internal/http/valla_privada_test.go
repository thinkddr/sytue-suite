// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package baseshttp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// La valla de la fuente privada (PST01 desde la base de datos) llega al cliente como
// un 409 con código estable, también envuelta y también desde los scripts.
func TestErrorAPIVallaPrivada(t *testing.T) {
	valla := fmt.Errorf("guardar: %w", &pgconn.PgError{Code: "PST01", Message: "private_storage_legacy_write_blocked"})
	for nombre, got := range map[string]error{"api": errorAPI(valla), "script": errorScript(valla)} {
		var he *httpx.Error
		if !errors.As(got, &he) || he.Status != http.StatusConflict || he.Code != "private_storage_migration_required" {
			t.Fatalf("%s: la valla no se traduce a 409: %#v", nombre, got)
		}
	}
	otro := &pgconn.PgError{Code: "23505"}
	var he *httpx.Error
	if got := errorAPI(otro); errors.As(got, &he) || !errors.Is(got, otro) {
		t.Fatalf("un error ajeno a la valla no debe cambiar: %#v", got)
	}
}
