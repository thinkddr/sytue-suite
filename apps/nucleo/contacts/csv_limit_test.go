// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func TestImportCSVTooManyContactsBeforeSnapshot(t *testing.T) {
	// No hay fuente configurada: acceder al snapshot seria un error. El rechazo
	// debe ocurrir antes de preparar cualquier mutacion.
	r := &Repository{}
	body := []byte("Given Name\n" + strings.Repeat("Ada\n", 50_001))
	imported, skipped, err := r.ImportCSV(context.Background(), "book", body)
	if !errors.Is(err, card.ErrCSVTooManyContacts) || imported != 0 || skipped != 0 {
		t.Fatalf("got %d importados, %d omitidos, err=%v", imported, skipped, err)
	}
}
