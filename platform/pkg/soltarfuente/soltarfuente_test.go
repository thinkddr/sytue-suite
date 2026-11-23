// SPDX-License-Identifier: Apache-2.0
package soltarfuente

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestAvisoYErrores(t *testing.T) {
	one := Aviso(storage.Scope{Tenant: "acme", Subject: "ana", Product: "hub"})
	if one.Source != "one" || one.Type != TipoAviso || one.Params["name"] != "Sytue One" || one.Level != "warning" {
		t.Fatal("aviso de One", one)
	}
	if n := Aviso(storage.Scope{Tenant: "acme", Subject: "ana", Product: "sheets"}); n.Source != "sheets" || n.Params["name"] != "Sytue Calcus" {
		t.Fatal("aviso de Calcus", n)
	}
	for err, code := range map[error]string{
		fmt.Errorf("x: %w", pg.ErrPrivateStorageReleaseStale): "private_storage_release_stale",
		pg.ErrPrivateStorageScope:                             "storage_identity_changed",
		&pgconn.PgError{Code: "PST01"}:                        "private_storage_frozen",
		errors.New("cualquier otra cosa"):                     "internal",
	} {
		got := Error(err)
		if internal := code == "internal" && got.Status == http.StatusInternalServerError; got.Code != code && !internal {
			t.Errorf("%v: %s", err, got.Code)
		}
	}
	if _, err := Handler(Config{Product: "notes"}); err == nil {
		t.Fatal("sin Soltar no hay ruta")
	}
}
