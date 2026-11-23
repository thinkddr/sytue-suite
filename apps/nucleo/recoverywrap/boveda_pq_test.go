// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type vectorBovedaPQ struct {
	UnlockKey    []byte         `json:"unlock_key"`
	Hint         []byte         `json:"hint"`
	VaultWrapKey string         `json:"vault_wrap_key"`
	Tenant       string         `json:"tenant"`
	Product      string         `json:"product"`
	Subject      string         `json:"subject"`
	Secret       PQVaultSecret  `json:"secret"`
	AADHex       string         `json:"aad_hex"`
	Nonce        []byte         `json:"nonce"`
	Wrapper      PQVaultWrapper `json:"wrapper"`
}

func TestBovedaPQ_RoundTripYAislamiento(t *testing.T) {
	auk := seq(32, 0x20)
	id, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	secret := PQVaultSecret{
		V:        PQVaultVersion,
		Profile:  id.Profile(),
		Identity: id.String(),
		OwnerKey: base64.StdEncoding.EncodeToString(seq(32, 0x91)),
	}
	w, err := WrapPQVault(auk, "acme", "drive", "sub-123", secret)
	if err != nil {
		t.Fatalf("WrapPQVault: %v", err)
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	got, err := OpenPQVault(auk, "acme", "drive", "sub-123", w)
	if err != nil {
		t.Fatalf("OpenPQVault: %v", err)
	}
	if got != secret {
		t.Fatalf("secreto distinto: got=%+v want=%+v", got, secret)
	}

	// Aislamiento de dominio: otro tenant, producto o sujeto no puede abrirlo.
	for _, tc := range []struct{ tenant, product, subject string }{
		{"otro", "drive", "sub-123"},
		{"acme", "notes", "sub-123"},
		{"acme", "drive", "sub-999"},
	} {
		if _, err := OpenPQVault(auk, tc.tenant, tc.product, tc.subject, w); !errors.Is(err, ErrWrongSecret) {
			t.Fatalf("OpenPQVault(%+v): got %v, want ErrWrongSecret", tc, err)
		}
	}

	// Otra clave de desbloqueo rechaza por huella.
	otraAUK := seq(32, 0x77)
	if _, err := OpenPQVault(otraAUK, "acme", "drive", "sub-123", w); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra AUK: got %v, want ErrWrongSecret", err)
	}

	// Separación criptográfica respecto a la clave de envoltorio de Matrix.
	vaultKey, err := PQVaultKeyFromAUK(auk)
	if err != nil {
		t.Fatal(err)
	}
	matrixKey, err := SytueIDKey(auk)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(vaultKey) == hex.EncodeToString(matrixKey) {
		t.Fatal("PQVaultKeyFromAUK debe ser distinta de SytueIDKey")
	}
}

func TestBovedaPQ_ValidacionRechazaPerfilClasicoYKeyIDManipulado(t *testing.T) {
	auk := seq(32, 0x20)
	id, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	secret := PQVaultSecret{
		V:        PQVaultVersion,
		Profile:  id.Profile(),
		Identity: id.String(),
		OwnerKey: base64.StdEncoding.EncodeToString(seq(32, 0x42)),
	}
	w, err := WrapPQVault(auk, "acme", "notes", "sub-1", secret)
	if err != nil {
		t.Fatal(err)
	}

	badKeyID := w
	badKeyID.KeyID = "0000000000000000000000000000000000000000000000000000000000000000"
	if !errors.Is(badKeyID.Validate(), ErrInvalid) {
		t.Fatal("esperaba ErrInvalid con key_id manipulado")
	}

	badProfile := w
	badProfile.Profile = pqc.LegacyProfile
	if !errors.Is(badProfile.Validate(), ErrInvalid) {
		t.Fatal("esperaba ErrInvalid con perfil clásico")
	}
}

func TestBovedaPQ_VectoresCompartidos(t *testing.T) {
	path := filepath.Join("testdata", "boveda_pq.json")
	if os.Getenv("SYTUE_UPDATE_VECTORS") == "1" {
		id, err := pqc.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		auk := seq(32, 0x20)
		hint, err := Hint(auk)
		if err != nil {
			t.Fatal(err)
		}
		wrapKey, err := PQVaultKeyFromAUK(auk)
		if err != nil {
			t.Fatal(err)
		}
		secret := PQVaultSecret{
			V:        PQVaultVersion,
			Profile:  id.Profile(),
			Identity: id.String(),
			OwnerKey: base64.StdEncoding.EncodeToString(seq(32, 0x91)),
		}
		nonce := seq(12, 0x61)
		w, err := SealPQVault(auk, "acme", "drive", "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00", secret, nonce)
		if err != nil {
			t.Fatal(err)
		}
		aad, err := PQVaultAAD("acme", "drive", "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00", w.KeyID)
		if err != nil {
			t.Fatal(err)
		}
		v := vectorBovedaPQ{
			UnlockKey:    auk,
			Hint:         hint,
			VaultWrapKey: hex.EncodeToString(wrapKey),
			Tenant:       "acme",
			Product:      "drive",
			Subject:      "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00",
			Secret:       secret,
			AADHex:       hex.EncodeToString(aad),
			Nonce:        nonce,
			Wrapper:      w,
		}
		// Regenera el vector sin perder el de la firma (clave «firma», firma_boveda_test.go).
		var firma json.RawMessage
		if raw, err := os.ReadFile(path); err == nil {
			var previo map[string]json.RawMessage
			if json.Unmarshal(raw, &previo) == nil {
				firma = previo["firma"]
			}
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		todo := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &todo); err != nil {
			t.Fatal(err)
		}
		if firma != nil {
			todo["firma"] = firma
		}
		if raw, err = json.MarshalIndent(todo, "", "  "); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lee %s: %v", path, err)
	}
	var v vectorBovedaPQ
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	wrapKey, err := PQVaultKeyFromAUK(v.UnlockKey)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(wrapKey) != v.VaultWrapKey {
		t.Fatalf("vault_wrap_key distinta: got %s want %s", hex.EncodeToString(wrapKey), v.VaultWrapKey)
	}
	sealed, err := SealPQVault(v.UnlockKey, v.Tenant, v.Product, v.Subject, v.Secret, v.Nonce)
	if err != nil {
		t.Fatalf("SealPQVault: %v", err)
	}
	gotJSON, _ := json.Marshal(sealed)
	wantJSON, _ := json.Marshal(v.Wrapper)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("wrapper distinto:\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}
	opened, err := OpenPQVault(v.UnlockKey, v.Tenant, v.Product, v.Subject, v.Wrapper)
	if err != nil {
		t.Fatalf("OpenPQVault: %v", err)
	}
	if opened != v.Secret {
		t.Fatalf("secret distinto: got %+v want %+v", opened, v.Secret)
	}
}
