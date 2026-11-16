// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenera testdata/vectors.json")

// vector es una entrada de testdata/vectors.json, el contrato que comparten Go y
// la web. Lo secreto (contraseña, salida PRF, clave de envoltorio) va en el
// vector para poder comprobar cada paso; en producción nunca sale del dispositivo.
type vector struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	Account      string  `json:"account"`
	KeyID        string  `json:"key_id"`
	CredentialID string  `json:"credential_id"`
	Password     string  `json:"password,omitempty"`
	PRFOutput    []byte  `json:"prf_output,omitempty"`
	RecoveryKey  string  `json:"recovery_key"`
	WrapKey      string  `json:"wrap_key"`
	AAD          string  `json:"aad"`
	Wrapper      Wrapper `json:"wrapper"`
}

func seq(n int, start byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = start + byte(i)
	}
	return out
}

func buildVectors(t *testing.T) []vector {
	t.Helper()
	const recovery = "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP"
	password := KDF{Alg: AlgArgon2id, Memory: DefaultMemoryKiB, Time: DefaultTime, Threads: DefaultThreads, Salt: seq(16, 0x10)}
	credential := base64.RawURLEncoding.EncodeToString(seq(20, 0xa0))
	passkey := KDF{Alg: AlgPasskeyPRF, PRFSalt: seq(32, 0x40)}
	out := []vector{
		{Name: "password-rooms", Kind: KindPassword, Account: "@sytue_0f3c:sytue.eu", KeyID: "gTCx1L6rB9mQ0r2A", Password: "correct horse battery staple ñ€", RecoveryKey: recovery, Wrapper: Wrapper{KDF: password, Nonce: seq(12, 0x70)}},
		{Name: "passkey-isolated-drive", Kind: KindPasskey, Account: "@sytue_storage_drive_acme_0f3c:sytue.eu", KeyID: "QW1x8sY0b0dRk1Zp", CredentialID: credential, PRFOutput: seq(32, 0xc0), RecoveryKey: recovery, Wrapper: Wrapper{KDF: passkey, Nonce: seq(12, 0x80)}},
	}
	for i := range out {
		v := &out[i]
		var key []byte
		var err error
		if v.Kind == KindPassword {
			key, err = PasswordKey(v.Password, v.Wrapper.KDF)
		} else {
			key, err = PasskeyKey(v.PRFOutput)
		}
		if err != nil {
			t.Fatal(err)
		}
		v.Wrapper, err = Seal(key, v.Kind, v.Account, v.KeyID, v.CredentialID, v.Wrapper.KDF, v.RecoveryKey, v.Wrapper.Nonce)
		if err != nil {
			t.Fatal(err)
		}
		v.WrapKey, v.AAD = hex.EncodeToString(key), hex.EncodeToString(AAD(v.Kind, v.Account, v.KeyID, v.CredentialID))
	}
	return out
}

// Los vectores fijan el formato byte a byte: si este test cambia sin querer,
// los envoltorios ya guardados dejan de abrirse en la web o aquí.
func TestLosVectoresCompartidosConLaWebNoCambian(t *testing.T) {
	path := filepath.Join("testdata", "vectors.json")
	built := buildVectors(t)
	if *update {
		raw, err := json.MarshalIndent(built, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored []vector
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(built) {
		t.Fatalf("%d vectores; quiero %d", len(stored), len(built))
	}
	for i, v := range stored {
		if !bytes.Equal(v.Wrapper.Ciphertext, built[i].Wrapper.Ciphertext) || v.WrapKey != built[i].WrapKey || v.AAD != built[i].AAD {
			t.Fatalf("%s: el formato ha cambiado", v.Name)
		}
		key, _ := hex.DecodeString(v.WrapKey)
		got, err := Open(key, v.Account, v.Wrapper)
		if err != nil || got != v.RecoveryKey {
			t.Fatalf("%s: Open = %q, %v", v.Name, got, err)
		}
		if v.Kind == KindPassword {
			if got, err := UnwrapPassword(v.Password, v.Account, v.Wrapper); err != nil || got != v.RecoveryKey {
				t.Fatalf("%s: UnwrapPassword = %q, %v", v.Name, got, err)
			}
		}
	}
}

func TestUnEnvoltorioSoloSeAbreConSuContrasenaYEnSuCuenta(t *testing.T) {
	const account, keyID, recovery = "@sytue_a:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP"
	w, err := WrapPassword("buena", account, keyID, recovery)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(w)
	if bytes.Contains(raw, []byte("EsT3")) || bytes.Contains(raw, []byte("buena")) {
		t.Fatalf("el envoltorio lleva el secreto en claro: %s", raw)
	}
	if _, err := UnwrapPassword("mala", account, w); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("contraseña equivocada: %v", err)
	}
	if _, err := UnwrapPassword("buena", "@sytue_storage_drive_acme_a:x", w); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra cuenta: %v", err)
	}
	moved := w
	moved.KeyID = "otra"
	if _, err := UnwrapPassword("buena", account, moved); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra clave de almacenamiento: %v", err)
	}
	if got, err := UnwrapPassword("buena", account, w); err != nil || got != recovery {
		t.Fatalf("UnwrapPassword = %q, %v", got, err)
	}
}

// Un envoltorio con una derivación barata se rechaza antes de derivar nada.
func TestValidateRechazaParametrosDebiles(t *testing.T) {
	w, err := WrapPassword("x", "@a:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Wrapper){
		"memoria":    func(w *Wrapper) { w.KDF.Memory = 1024 },
		"pasadas":    func(w *Wrapper) { w.KDF.Time = 1 },
		"hilos":      func(w *Wrapper) { w.KDF.Threads = 0 },
		"sal":        func(w *Wrapper) { w.KDF.Salt = w.KDF.Salt[:8] },
		"nonce":      func(w *Wrapper) { w.Nonce = w.Nonce[:8] },
		"tipo":       func(w *Wrapper) { w.Kind = "kek" },
		"version":    func(w *Wrapper) { w.V = 2 },
		"credencial": func(w *Wrapper) { w.CredentialID = "abc" },
		"sin key_id": func(w *Wrapper) { w.KeyID = "" },
		"memoria>":   func(w *Wrapper) { w.KDF.Memory = MaxMemoryKiB + 1 },
	} {
		bad := w
		bad.KDF.Salt = bytes.Clone(w.KDF.Salt)
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s: Validate aceptó %+v", name, bad.KDF)
		}
	}
	if w.Validate() != nil {
		t.Fatal("el envoltorio bueno no valida")
	}
}
