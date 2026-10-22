// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"bytes"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestLosRadiosNoDivergenDelCanon(t *testing.T) {
	canon := webref.Read(t, "branding/tokens/spacing.json")
	if !bytes.Equal(espaciadoJSON, []byte(canon)) {
		t.Fatal("spacing.json no coincide con branding/tokens/spacing.json; ejecutar scripts/sync-branding.sh")
	}
}
