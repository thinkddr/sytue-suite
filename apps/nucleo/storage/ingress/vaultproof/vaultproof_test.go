// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultproof

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

var ahora = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func firmante(t *testing.T, b byte) (*recoverywrap.PQVaultSigner, Registered) {
	t.Helper()
	s, err := recoverywrap.ParsePQVaultSigningKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := s.Public().ID()
	if err != nil {
		t.Fatal(err)
	}
	return s, Registered{Generation: 2, KeyID: keyID, Public: s.Public()}
}

func accion(t *testing.T, reg Registered, kind string) matrixproof.Action {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00", Product: "mail"}
	digest, err := matrixproof.RequestDigest(map[string]string{"recipient": "age1pq1…"})
	if err != nil {
		t.Fatal(err)
	}
	return matrixproof.Action{
		Version: 1, Generation: reg.Generation, Scope: scope, Epoch: "11111111-1111-4111-8111-111111111111",
		Kind: kind, Target: storage.PublicationTarget{RoomID: "!pqv_abc:sytue-vault", DeviceID: "pq-abcdef"},
		RootKey: base64.RawStdEncoding.EncodeToString(reg.Public.Ed25519), RequestSHA256: digest,
		Nonce: "server_challenge", IssuedAt: ahora.UnixMilli(), ExpiresAt: ahora.Add(time.Minute).UnixMilli(),
	}
}

func TestVaultproofFirmaYAutoriza(t *testing.T) {
	s, reg := firmante(t, 1)
	esperada := accion(t, reg, "key.enroll")
	prueba, err := Sign(s, esperada)
	if err != nil {
		t.Fatal(err)
	}
	if !Signed(prueba) {
		t.Fatal("una prueba de bóveda no se reconoce como tal")
	}
	auth, err := Verify(esperada, prueba, reg, ahora)
	if err != nil {
		t.Fatal(err)
	}
	// La autorización es la misma que daría Matrix: el resto del correo la valida igual.
	if err := auth.Validate(esperada); err != nil {
		t.Fatalf("autorización inválida: %v", err)
	}
	if auth.Authority.UserID != UserID(esperada.Scope) || !strings.HasSuffix(auth.Authority.UserID, ":sytue-vault") ||
		auth.SigningKey != esperada.RootKey || auth.Authority.Generation != reg.Generation {
		t.Fatalf("autorización = %+v", auth)
	}
	if Signed(matrixproof.SignedAction{Action: esperada}) {
		t.Fatal("una prueba sin firmas se toma por de bóveda")
	}
}

func TestVaultproofRechaza(t *testing.T) {
	s, reg := firmante(t, 1)
	otra, otraReg := firmante(t, 2)
	esperada := accion(t, reg, "task.claim")
	prueba, err := Sign(s, esperada)
	if err != nil {
		t.Fatal(err)
	}
	casos := map[string]func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time){
		"caducada": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			return esperada, prueba, reg, ahora.Add(2 * time.Minute)
		},
		"clave que ya no es la registrada": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			return esperada, prueba, otraReg, ahora
		},
		"generación vieja": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			r := reg
			r.Generation = 3
			return esperada, prueba, r, ahora
		},
		"acción distinta de la esperada": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			cambiada := esperada
			cambiada.Nonce = "otro"
			return cambiada, prueba, reg, ahora
		},
		"firmada por otra bóveda con el nombre de esta": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			ajena, err := Sign(otra, esperada)
			if err != nil {
				t.Fatal(err)
			}
			for u, keys := range ajena.Signatures {
				for _, v := range keys {
					ajena.Signatures[u] = map[id.KeyID]string{id.KeyID(Algorithm + ":" + reg.KeyID): v}
				}
			}
			return esperada, ajena, reg, ahora
		},
		"firma de otro tipo de acción": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			otraClase := esperada
			otraClase.Kind = "task.ack"
			firmada, err := Sign(s, otraClase)
			if err != nil {
				t.Fatal(err)
			}
			firmada.Action = esperada
			return esperada, firmada, reg, ahora
		},
		"firma de más": func() (matrixproof.Action, matrixproof.SignedAction, Registered, time.Time) {
			doble, err := Sign(s, esperada)
			if err != nil {
				t.Fatal(err)
			}
			doble.Signatures["@otro:matrix.test"] = map[id.KeyID]string{"ed25519:X": "x"}
			return esperada, doble, reg, ahora
		},
	}
	for nombre, caso := range casos {
		e, p, r, cuando := caso()
		if _, err := Verify(e, p, r, cuando); !errors.Is(err, ErrUnverified) {
			t.Errorf("%s: %v; quiero ErrUnverified", nombre, err)
		}
	}
}
