// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
)

// El código del fallo es corto, fijo y sin nada de la persona: ni el texto del
// error, ni la URL, ni el cuerpo de la respuesta salvo el código del servicio.
func TestCodigoFalloSoloLlevaLoFijo(t *testing.T) {
	secreto := "https://drive.example/api/v1/private-storage/migration/7f1e2d3c?nombre=informe-secreto.pdf"
	casos := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{privateDrive.ErrNotReady, "drive.not_ready"},
		{fmt.Errorf("envuelto: %w", privateDrive.ErrConflict), "drive.conflict"},
		{&privateDrive.ImportError{Check: "row_heads", Err: privateDrive.ErrConflict}, "drive.import.row_heads"},
		{pqvault.ErrWrong, "vault.wrong"},
		// La clave de firma, con lo que contestó Sytue ID: el mismo código que la web.
		{&pqvault.SigningKeyError{Check: "signing_put_400_vault_key_proof"}, "vault.unavailable.signing_put_400_vault_key_proof"},
		{fmt.Errorf("envuelto: %w", &pqvault.SigningKeyError{Check: "signing_missing"}), "vault.unavailable.signing_missing"},
		{&pqvault.SigningKeyError{Check: secreto}, "vault.unavailable.signing"},
		{pqvault.ErrSigningKey, "vault.unavailable.signing"},
		{context.Canceled, "canceled"},
		{&api.Error{Status: 409, URL: secreto, Body: `{"error":{"code":"private_storage_snapshot_changed","message":"` + secreto + `"}}`}, "http.409.private_storage_snapshot_changed"},
		{&api.Error{Status: 400, URL: secreto, Body: `{"error":{"code":"` + secreto + `"}}`}, "http.400"},
		{errors.New(secreto), "other"},
	}
	for _, c := range casos {
		got := CodigoFallo(c.err)
		if got != c.want {
			t.Errorf("CodigoFallo(%v) = %q, quiero %q", c.err, got, c.want)
		}
		if strings.Contains(got, "informe") || strings.Contains(got, "drive.example") {
			t.Errorf("el código deja ver datos: %q", got)
		}
	}
}
