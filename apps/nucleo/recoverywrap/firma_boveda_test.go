// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func firmanteDePrueba(t *testing.T, b byte) (*PQVaultSigner, string) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString(seq(pqSigningSeedLen, b))
	s, err := ParsePQVaultSigningKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return s, encoded
}

func TestFirmaBovedaPQ_FirmaYVerificaConSuContexto(t *testing.T) {
	s, _ := firmanteDePrueba(t, 0x10)
	pub := s.Public()
	if err := pub.Validate(); err != nil {
		t.Fatal(err)
	}
	msg := []byte(`{"kind":"key.enroll"}`)
	sig, err := s.Sign("ingress.key.enroll", msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != PQVaultSignatureLen {
		t.Fatalf("firma de %d bytes; quiero %d", len(sig), PQVaultSignatureLen)
	}
	if err := pub.Verify("ingress.key.enroll", msg, sig); err != nil {
		t.Fatalf("la firma propia no verifica: %v", err)
	}
	// Una firma hecha para una cosa no sirve para otra, ni para otro mensaje, ni
	// con la clave de otra bóveda.
	if err := pub.Verify("ingress.task.claim", msg, sig); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otro contexto: %v", err)
	}
	if err := pub.Verify("ingress.key.enroll", []byte(`{"kind":"task.ack"}`), sig); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otro mensaje: %v", err)
	}
	otra, _ := firmanteDePrueba(t, 0x11)
	if err := otra.Public().Verify("ingress.key.enroll", msg, sig); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra clave: %v", err)
	}
}

// Las dos mitades son obligatorias: con una sola buena, la firma no vale. Es lo
// que hace que romper una familia (la clásica o la post-cuántica) no baste.
func TestFirmaBovedaPQ_NoBastaConUnaMitad(t *testing.T) {
	s, _ := firmanteDePrueba(t, 0x20)
	pub := s.Public()
	msg := []byte("mensaje")
	sig, err := s.Sign("prueba", msg)
	if err != nil {
		t.Fatal(err)
	}
	for nombre, pos := range map[string]int{
		"mitad Ed25519 rota": 3,
		"mitad ML-DSA rota":  ed25519.SignatureSize + 7,
	} {
		rota := bytes.Clone(sig)
		rota[pos] ^= 0x01
		if err := pub.Verify("prueba", msg, rota); !errors.Is(err, ErrWrongSecret) {
			t.Errorf("%s: %v; quiero ErrWrongSecret", nombre, err)
		}
	}
	// La mitad Ed25519 de OTRA clave con la ML-DSA buena, y al revés.
	otra, _ := firmanteDePrueba(t, 0x21)
	ajena, err := otra.Sign("prueba", msg)
	if err != nil {
		t.Fatal(err)
	}
	mezclaEd := append(bytes.Clone(ajena[:ed25519.SignatureSize]), sig[ed25519.SignatureSize:]...)
	mezclaML := append(bytes.Clone(sig[:ed25519.SignatureSize]), ajena[ed25519.SignatureSize:]...)
	for nombre, mezcla := range map[string][]byte{"Ed25519 ajena": mezclaEd, "ML-DSA ajena": mezclaML} {
		if err := pub.Verify("prueba", msg, mezcla); !errors.Is(err, ErrWrongSecret) {
			t.Errorf("%s: %v; quiero ErrWrongSecret", nombre, err)
		}
	}
}

func TestFirmaBovedaPQ_RechazaFormasInvalidas(t *testing.T) {
	for nombre, encoded := range map[string]string{
		"vacía":       "",
		"corta":       base64.StdEncoding.EncodeToString(seq(pqSigningSeedLen-1, 1)),
		"larga":       base64.StdEncoding.EncodeToString(seq(pqSigningSeedLen+1, 1)),
		"no es b64":   "¡no!",
		"b64 con url": base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, pqSigningSeedLen)),
	} {
		if _, err := ParsePQVaultSigningKey(encoded); !errors.Is(err, ErrInvalid) {
			t.Errorf("clave %s: %v; quiero ErrInvalid", nombre, err)
		}
	}
	s, _ := firmanteDePrueba(t, 0x30)
	for _, contexto := range []string{"", "con\x00nul", "con\nsalto", string(bytes.Repeat([]byte{'a'}, maxPQSignContext+1))} {
		if _, err := s.Sign(contexto, []byte("m")); !errors.Is(err, ErrInvalid) {
			t.Errorf("contexto %q: %v; quiero ErrInvalid", contexto, err)
		}
	}
	pub := s.Public()
	if err := pub.Verify("prueba", []byte("m"), make([]byte, PQVaultSignatureLen-1)); !errors.Is(err, ErrInvalid) {
		t.Errorf("firma corta: %v", err)
	}
	mala := pub
	mala.Profile = "ed25519-v1"
	if !errors.Is(mala.Validate(), ErrInvalid) {
		t.Error("perfil solo clásico aceptado")
	}
	mala = pub
	mala.MLDSA65 = mala.MLDSA65[:len(mala.MLDSA65)-1]
	if _, err := mala.ID(); !errors.Is(err, ErrInvalid) {
		t.Error("clave pública ML-DSA truncada aceptada")
	}
	// Dos claves generadas no coinciden, y su identificador tampoco.
	a, err := NewPQVaultSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewPQVaultSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("dos claves nuevas iguales")
	}
}

// El secreto de la bóveda lleva la clave de firma sin dejar de abrir los de antes.
func TestFirmaBovedaPQ_ViajaDentroDelSecreto(t *testing.T) {
	id, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	auk := seq(32, 0x40)
	_, firma := firmanteDePrueba(t, 0x41)
	sin := PQVaultSecret{
		V: PQVaultVersion, Profile: id.Profile(), Identity: id.String(),
		OwnerKey: base64.StdEncoding.EncodeToString(seq(32, 0x42)),
	}
	con := sin
	con.SigningKey = firma

	for nombre, secreto := range map[string]PQVaultSecret{"sin clave de firma": sin, "con clave de firma": con} {
		w, err := WrapPQVault(auk, "acme", "mail", "sub-1", secreto)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		abierto, err := OpenPQVault(auk, "acme", "mail", "sub-1", w)
		if err != nil || abierto != secreto {
			t.Fatalf("%s: abierto %+v, %v", nombre, abierto, err)
		}
	}
	// Una bóveda de antes de la ADR 0087 se serializa igual que siempre.
	raw, err := json.Marshal(sin)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("signing_key")) {
		t.Fatalf("un secreto sin clave de firma la menciona: %s", raw)
	}
	if _, err := sin.Signer(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Signer sin clave: %v", err)
	}
	s, err := con.Signer()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := s.Sign("prueba", []byte("m"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Public().Verify("prueba", []byte("m"), sig); err != nil {
		t.Fatal(err)
	}
	// Una clave de firma rota invalida el secreto entero: no se envuelve.
	rota := con
	rota.SigningKey = "AAAA"
	if _, err := WrapPQVault(auk, "acme", "mail", "sub-1", rota); !errors.Is(err, ErrInvalid) {
		t.Fatalf("secreto con clave de firma rota: %v", err)
	}
}

type vectorFirmaBovedaPQ struct {
	SigningKey string           `json:"signing_key"`
	Public     PQVaultVerifyKey `json:"public"`
	KeyID      string           `json:"key_id"`
	Context    string           `json:"context"`
	Message    string           `json:"message"`
	Signature  []byte           `json:"signature"`
	// Una bóveda de Mail cuyo secreto lleva esa clave, sellada por Go: el puente
	// de Sytue ID tiene que abrirla y devolver la clave de firma intacta.
	UnlockKey []byte         `json:"unlock_key"`
	Tenant    string         `json:"tenant"`
	Product   string         `json:"product"`
	Subject   string         `json:"subject"`
	Nonce     []byte         `json:"nonce"`
	Secret    PQVaultSecret  `json:"secret"`
	Wrapper   PQVaultWrapper `json:"wrapper"`
}

// Los vectores los comparten el núcleo Go y el puente de Sytue ID. La mitad
// ML-DSA de una firma es aleatorizada, así que la firma guardada se VERIFICA, no
// se regenera: lo que tiene que salir igual es la clave pública y su identificador.
func TestFirmaBovedaPQ_VectoresCompartidos(t *testing.T) {
	// Va dentro de boveda_pq.json (clave «firma»): ese vector ya está en la lista
	// blanca de gitleaks de One y de la suite, y uno nuevo rompía la exportación.
	path := filepath.Join("testdata", "boveda_pq.json")
	if os.Getenv("SYTUE_UPDATE_VECTORS") == "1" {
		s, encoded := firmanteDePrueba(t, 0x50)
		pub := s.Public()
		id, err := pub.ID()
		if err != nil {
			t.Fatal(err)
		}
		v := vectorFirmaBovedaPQ{
			SigningKey: encoded, Public: pub, KeyID: id,
			Context: "ingress.key.enroll", Message: `{"kind":"key.enroll","generation":1}`,
		}
		if v.Signature, err = s.Sign(v.Context, []byte(v.Message)); err != nil {
			t.Fatal(err)
		}
		identidad, err := pqc.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		v.UnlockKey, v.Nonce = seq(32, 0x20), seq(12, 0x71)
		v.Tenant, v.Product, v.Subject = "acme", "mail", "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00"
		v.Secret = PQVaultSecret{
			V: PQVaultVersion, Profile: identidad.Profile(), Identity: identidad.String(),
			OwnerKey: base64.StdEncoding.EncodeToString(seq(32, 0x92)), SigningKey: encoded,
		}
		if v.Wrapper, err = SealPQVault(v.UnlockKey, v.Tenant, v.Product, v.Subject, v.Secret, v.Nonce); err != nil {
			t.Fatal(err)
		}
		escribirVectorAnidado(t, path, "firma", v)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lee %s: %v", path, err)
	}
	var envoltorio struct {
		Firma vectorFirmaBovedaPQ `json:"firma"`
	}
	if err := json.Unmarshal(raw, &envoltorio); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	v := envoltorio.Firma
	s, err := ParsePQVaultSigningKey(v.SigningKey)
	if err != nil {
		t.Fatal(err)
	}
	pub := s.Public()
	got, _ := json.Marshal(pub)
	want, _ := json.Marshal(v.Public)
	if !bytes.Equal(got, want) {
		t.Fatalf("clave pública distinta:\ngot:  %s\nwant: %s", got, want)
	}
	if id, err := pub.ID(); err != nil || id != v.KeyID {
		t.Fatalf("key_id %q, %v; quiero %q", id, err, v.KeyID)
	}
	if err := v.Public.Verify(v.Context, []byte(v.Message), v.Signature); err != nil {
		t.Fatalf("la firma del vector no verifica: %v", err)
	}
	sellado, err := SealPQVault(v.UnlockKey, v.Tenant, v.Product, v.Subject, v.Secret, v.Nonce)
	if err != nil {
		t.Fatalf("SealPQVault: %v", err)
	}
	gotW, _ := json.Marshal(sellado)
	wantW, _ := json.Marshal(v.Wrapper)
	if !bytes.Equal(gotW, wantW) {
		t.Fatalf("envoltorio distinto:\ngot:  %s\nwant: %s", gotW, wantW)
	}
	abierto, err := OpenPQVault(v.UnlockKey, v.Tenant, v.Product, v.Subject, v.Wrapper)
	if err != nil || abierto != v.Secret || abierto.SigningKey != v.SigningKey {
		t.Fatalf("OpenPQVault: %+v, %v", abierto, err)
	}
}

func TestFirmaBovedaPQ_DeclaracionDeClave(t *testing.T) {
	a, b := hex64(0xa), hex64(0xb)
	got, err := PQVaultKeyStatement("sub-1", "acme", "mail", 2, b, a)
	if err != nil {
		t.Fatal(err)
	}
	want := "sytue-vault-key-v1\x00sub-1\x00acme\x00mail\x002\x00" + b + "\x00" + a
	if string(got) != want {
		t.Fatalf("declaración = %q", got)
	}
	if _, err := PQVaultKeyStatement("sub-1", "acme", "mail", 1, a, ""); err != nil {
		t.Fatalf("alta: %v", err)
	}
	for nombre, c := range map[string]struct {
		sub, tenant, product string
		gen                  int64
		key, prev            string
	}{
		"alta con anterior":         {"s", "t", "mail", 1, a, b},
		"cambio sin anterior":       {"s", "t", "mail", 2, a, ""},
		"anterior igual a la nueva": {"s", "t", "mail", 2, a, a},
		"generación cero":           {"s", "t", "mail", 0, a, ""},
		"producto desconocido":      {"s", "t", "mailx", 1, a, ""},
		"sujeto con NUL":            {"s\x00x", "t", "mail", 1, a, ""},
		"id en mayúsculas":          {"s", "t", "mail", 1, strings.ToUpper(a), ""},
		"id corto":                  {"s", "t", "mail", 1, a[:63], ""},
	} {
		if _, err := PQVaultKeyStatement(c.sub, c.tenant, c.product, c.gen, c.key, c.prev); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v; quiero ErrInvalid", nombre, err)
		}
	}
}

func hex64(b byte) string { return strings.Repeat(string("0123456789abcdef"[b%16]), 64) }

// escribirVectorAnidado guarda v bajo `clave` en el JSON de path sin tocar el resto.
func escribirVectorAnidado(t *testing.T, path, clave string, v any) {
	t.Helper()
	todo := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &todo); err != nil {
			t.Fatal(err)
		}
	}
	parte, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	todo[clave] = parte
	raw, err := json.MarshalIndent(todo, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil { //nolint:gosec // G306: vector público del repositorio.
		t.Fatal(err)
	}
}
