// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"errors"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Los clientes distinguen el conflicto de edición por su código: es el que abre
// «Quedarme con la mía / Ver la otra / Guardar las dos» y no un «no se pudo guardar».
func TestElConflictoDeEdicionEsUn409ConSuCodigo(t *testing.T) {
	var he *httpx.Error
	if !errors.As(traducir(domain.ErrNoteEditConflict), &he) {
		t.Fatal("el conflicto no se traduce a un error HTTP")
	}
	if he.Status != http.StatusConflict || he.Code != "note_edit_conflict" {
		t.Fatalf("conflicto = %d %q; quiero 409 note_edit_conflict", he.Status, he.Code)
	}
}
