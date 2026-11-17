// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recoverywrap

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jwk es una clave P-256 en el formato que importa WebCrypto: así la prueba de la
// página de Sytue ID repite la entrega con la MISMA clave efímera.
type jwk struct {
	D string `json:"d"`
	X string `json:"x"`
	Y string `json:"y"`
}

func jwkDe(t *testing.T, priv *ecdh.PrivateKey) jwk {
	t.Helper()
	pub := priv.PublicKey().Bytes()
	b := base64.RawURLEncoding.EncodeToString
	return jwk{D: b(priv.Bytes()), X: b(pub[1:33]), Y: b(pub[33:65])}
}

// vectorDesbloqueo es testdata/desbloqueo.json: el contrato de la ADR 0084 que
// comparten Go, la página de inicio de sesión de Sytue ID y la web. Lleva los
// secretos de cada paso para poder comprobarlos uno a uno; en producción ninguno
// sale del dispositivo.
type vectorDesbloqueo struct {
	UnlockKey        []byte   `json:"unlock_key"`
	Hint             []byte   `json:"hint"`
	UserID           string   `json:"user_id"`
	Password         string   `json:"password"`
	PasswordKEK      string   `json:"password_kek"`
	PasswordAAD      string   `json:"password_aad"`
	PasswordEnvelope Envelope `json:"password_envelope"`
	CredentialID     string   `json:"credential_id"`
	PRFInput         []byte   `json:"prf_input"`
	PRFOutput        []byte   `json:"prf_output"`
	PasskeyKEK       string   `json:"passkey_kek"`
	PasskeyEnvelope  Envelope `json:"passkey_envelope"`
	Account          string   `json:"account"`
	KeyID            string   `json:"key_id"`
	RecoveryKey      string   `json:"recovery_key"`
	SytueIDWrapKey   string   `json:"sytue_id_wrap_key"`
	SytueIDWrapper   Wrapper  `json:"sytue_id_wrapper"`
	Native           struct {
		App        jwk      `json:"app"`
		AppPublic  string   `json:"app_public"`
		Ephemeral  jwk      `json:"ephemeral"`
		Nonce      []byte   `json:"nonce"`
		Delivery   Delivery `json:"delivery"`
		Blob       string   `json:"blob"`
		Plaintext  string   `json:"plaintext"`
		SharedInfo string   `json:"info"`
	} `json:"native"`
	// LegacyPasswordIKM abre el vector `password-rooms` de vectors.json (ADR 0082).
	LegacyPasswordIKM []byte `json:"legacy_password_ikm"`
}

func buildDesbloqueo(t *testing.T) vectorDesbloqueo {
	t.Helper()
	var v vectorDesbloqueo
	v.UnlockKey = seq(32, 0x20)
	v.UserID = "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00"
	v.Password = "correct horse battery staple ñ€"
	v.CredentialID = base64.RawURLEncoding.EncodeToString(seq(20, 0xa0))
	v.PRFInput = PasskeyPRFInput
	v.PRFOutput = seq(32, 0xc0)
	v.Account = "@sytue_0f3c:sytue.eu"
	v.KeyID = "gTCx1L6rB9mQ0r2A"
	v.RecoveryKey = "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP"
	var err error
	if v.Hint, err = Hint(v.UnlockKey); err != nil {
		t.Fatal(err)
	}
	pwKDF := KDF{Alg: AlgArgon2id, Memory: DefaultMemoryKiB, Time: DefaultTime, Threads: DefaultThreads, Salt: seq(16, 0x30)}
	kek, err := EnvelopePasswordKEK(v.Password, pwKDF)
	if err != nil {
		t.Fatal(err)
	}
	v.PasswordKEK = hex.EncodeToString(kek)
	v.PasswordAAD = hex.EncodeToString(EnvelopeAAD(KindPassword, v.UserID, ""))
	if v.PasswordEnvelope, err = SealEnvelope(kek, KindPassword, v.UserID, "", pwKDF, v.UnlockKey, seq(12, 0x50)); err != nil {
		t.Fatal(err)
	}
	pkKEK, err := EnvelopePasskeyKEK(v.PRFOutput)
	if err != nil {
		t.Fatal(err)
	}
	v.PasskeyKEK = hex.EncodeToString(pkKEK)
	if v.PasskeyEnvelope, err = SealEnvelope(pkKEK, KindPasskey, v.UserID, v.CredentialID, KDF{Alg: AlgPasskeyPRF, PRFSalt: PasskeyPRFInput}, v.UnlockKey, seq(12, 0x60)); err != nil {
		t.Fatal(err)
	}
	wrapKey, err := SytueIDKey(v.UnlockKey)
	if err != nil {
		t.Fatal(err)
	}
	v.SytueIDWrapKey = hex.EncodeToString(wrapKey)
	if v.SytueIDWrapper, err = SealSytueID(v.UnlockKey, v.Account, v.KeyID, v.RecoveryKey, seq(12, 0x90)); err != nil {
		t.Fatal(err)
	}
	app, err := ecdh.P256().NewPrivateKey(seq(32, 0x01))
	if err != nil {
		t.Fatal(err)
	}
	eph, err := ecdh.P256().NewPrivateKey(seq(32, 0x41))
	if err != nil {
		t.Fatal(err)
	}
	v.Native.App, v.Native.Ephemeral = jwkDe(t, app), jwkDe(t, eph)
	v.Native.AppPublic = DeliveryPublic(app)
	v.Native.Nonce = seq(12, 0xb0)
	v.Native.SharedInfo = deliveryInfo
	v.Native.Delivery = Delivery{V: 1, Sub: v.UserID, Key: v.UnlockKey, Legacy: []LegacyIKM{{Kind: KindPassword, Salt: seq(16, 0x10), IKM: seq(32, 0xd0)}}}
	plain, err := json.Marshal(v.Native.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	v.Native.Plaintext = string(plain)
	if v.Native.Blob, err = SealDelivery(v.Native.AppPublic, v.Native.Delivery, eph, v.Native.Nonce); err != nil {
		t.Fatal(err)
	}
	if v.LegacyPasswordIKM, err = PasswordIKM(v.Password, KDF{Alg: AlgArgon2id, Memory: DefaultMemoryKiB, Time: DefaultTime, Threads: DefaultThreads, Salt: seq(16, 0x10)}); err != nil {
		t.Fatal(err)
	}
	return v
}

// Los vectores fijan el formato byte a byte: si cambian sin querer, los sobres de
// Sytue ID y los envoltorios de Rooms ya guardados dejan de abrirse.
func TestLosVectoresDelDesbloqueoNoCambian(t *testing.T) {
	path := filepath.Join("testdata", "desbloqueo.json")
	built := buildDesbloqueo(t)
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
	var v vectorDesbloqueo
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.PasswordEnvelope.Ciphertext, built.PasswordEnvelope.Ciphertext) ||
		!bytes.Equal(v.PasskeyEnvelope.Ciphertext, built.PasskeyEnvelope.Ciphertext) ||
		!bytes.Equal(v.SytueIDWrapper.Ciphertext, built.SytueIDWrapper.Ciphertext) ||
		v.Native.Blob != built.Native.Blob || !bytes.Equal(v.Hint, built.Hint) || v.PasswordKEK != built.PasswordKEK {
		t.Fatal("el formato del desbloqueo ha cambiado")
	}
	kek, _ := hex.DecodeString(v.PasswordKEK)
	if got, err := OpenEnvelope(kek, v.UserID, v.PasswordEnvelope); err != nil || !bytes.Equal(got, v.UnlockKey) {
		t.Fatalf("sobre de contraseña: %x, %v", got, err)
	}
	pk, _ := hex.DecodeString(v.PasskeyKEK)
	if got, err := OpenEnvelope(pk, v.UserID, v.PasskeyEnvelope); err != nil || !bytes.Equal(got, v.UnlockKey) {
		t.Fatalf("sobre de passkey: %x, %v", got, err)
	}
	if got, err := UnwrapSytueID(v.UnlockKey, v.Account, v.SytueIDWrapper); err != nil || got != v.RecoveryKey {
		t.Fatalf("envoltorio sytue-id: %q, %v", got, err)
	}
	app, _ := ecdh.P256().NewPrivateKey(seq(32, 0x01))
	d, err := OpenDelivery(app, v.Native.Blob)
	if err != nil || d.Sub != v.UserID || !bytes.Equal(d.Key, v.UnlockKey) || len(d.Legacy) != 1 {
		t.Fatalf("entrega nativa: %+v, %v", d, err)
	}
	// La transición: lo que calcula la página de inicio de sesión abre el
	// envoltorio de contraseña de la 0082.
	old := buildVectors(t)[0]
	got, err := OpenLegacy(LegacyIKM{Kind: KindPassword, Salt: old.Wrapper.KDF.Salt, IKM: v.LegacyPasswordIKM}, old.Account, []Wrapper{old.Wrapper})
	if err != nil || got != old.RecoveryKey {
		t.Fatalf("legado: %q, %v", got, err)
	}
}

func TestUnSobreSoloSeAbreConSuFactorYParaSuPersona(t *testing.T) {
	key, err := NewUnlockKey()
	if err != nil {
		t.Fatal(err)
	}
	kdf, err := NewPasswordKDF()
	if err != nil {
		t.Fatal(err)
	}
	kek, err := EnvelopePasswordKEK("buena", kdf)
	if err != nil {
		t.Fatal(err)
	}
	e, err := SealEnvelope(kek, KindPassword, "ana", "", kdf, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if bytes.Contains(raw, key) || strings.Contains(string(raw), base64.StdEncoding.EncodeToString(key)) {
		t.Fatalf("el sobre lleva la clave en claro: %s", raw)
	}
	mala, _ := EnvelopePasswordKEK("mala", kdf)
	if _, err := OpenEnvelope(mala, "ana", e); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("contraseña equivocada: %v", err)
	}
	if _, err := OpenEnvelope(kek, "beto", e); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra persona: %v", err)
	}
	// La envoltura de Rooms con la clave de desbloqueo se ata a su cuenta de Matrix.
	w, err := WrapSytueID(key, "@sytue_ana:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnwrapSytueID(key, "@sytue_storage_drive_acme_ana:x", w); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra cuenta: %v", err)
	}
	otra, _ := NewUnlockKey()
	if _, err := UnwrapSytueID(otra, "@sytue_ana:x", w); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("otra clave de desbloqueo: %v", err)
	}
}

func TestValidateDelSobreYDelEnvoltorioSytueID(t *testing.T) {
	key, _ := NewUnlockKey()
	prf := seq(32, 1)
	kek, _ := EnvelopePasskeyKEK(prf)
	cred := base64.RawURLEncoding.EncodeToString([]byte("cred"))
	good, err := SealEnvelope(kek, KindPasskey, "ana", cred, KDF{Alg: AlgPasskeyPRF, PRFSalt: PasskeyPRFInput}, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Envelope){
		"entrada PRF distinta": func(e *Envelope) { e.KDF.PRFSalt = seq(32, 9) },
		"sin credencial":       func(e *Envelope) { e.CredentialID = "" },
		"huella corta":         func(e *Envelope) { e.Hint = e.Hint[:8] },
		"cifrado corto":        func(e *Envelope) { e.Ciphertext = e.Ciphertext[:20] },
		"tipo":                 func(e *Envelope) { e.Kind = KindSytueID },
		"versión":              func(e *Envelope) { e.V = 2 },
	} {
		bad := good
		bad.KDF.PRFSalt = bytes.Clone(good.KDF.PRFSalt)
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s: Validate aceptó el sobre", name)
		}
	}
	w, _ := WrapSytueID(key, "@a:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	for name, mutate := range map[string]func(*Wrapper){
		"con credencial": func(w *Wrapper) { w.CredentialID = cred },
		"con sal":        func(w *Wrapper) { w.KDF.Salt = seq(16, 1) },
		"sin huella":     func(w *Wrapper) { w.KDF.Hint = nil },
		"otro alg":       func(w *Wrapper) { w.KDF.Alg = AlgArgon2id },
	} {
		bad := w
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s: Validate aceptó el envoltorio", name)
		}
	}
	// Un envoltorio de contraseña con huella tampoco vale: cada campo, en su tipo.
	pw, _ := WrapPassword("x", "@a:x", "kid", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	pw.KDF.Hint = seq(16, 1)
	if pw.Validate() == nil {
		t.Error("contraseña con huella aceptada")
	}
}

// La entrega a una app nativa solo la abre la clave efímera de ESE inicio de
// sesión: otra app, u otro intento, no ve nada.
func TestLaEntregaNativaSoloLaAbreSuClaveEfimera(t *testing.T) {
	app, err := NewDeliveryKey()
	if err != nil {
		t.Fatal(err)
	}
	otra, _ := NewDeliveryKey()
	key, _ := NewUnlockKey()
	blob, err := SealDelivery(DeliveryPublic(app), Delivery{V: 1, Sub: "ana", Key: key}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(blob, base64.RawURLEncoding.EncodeToString(key)) {
		t.Fatal("la entrega lleva la clave en claro")
	}
	if _, err := OpenDelivery(otra, blob); !errors.Is(err, ErrDelivery) {
		t.Fatalf("otra clave: %v", err)
	}
	cortada := blob[:len(blob)-4]
	if _, err := OpenDelivery(app, cortada); !errors.Is(err, ErrDelivery) {
		t.Fatalf("manipulada: %v", err)
	}
	d, err := OpenDelivery(app, blob)
	if err != nil || d.Sub != "ana" || !bytes.Equal(d.Key, key) {
		t.Fatalf("OpenDelivery = %+v, %v", d, err)
	}
	if _, err := SealDelivery("no-es-una-clave", Delivery{V: 1, Sub: "ana", Key: key}, nil, nil); !errors.Is(err, ErrDelivery) {
		t.Fatalf("clave pública rota: %v", err)
	}
}

// El parámetro de la transición lleva solo lo público de los envoltorios de la 0082,
// con la misma forma que compone la web.
func TestLegacyRequestSoloLlevaParametrosPublicos(t *testing.T) {
	old := buildVectors(t)
	got := LegacyRequest([]Wrapper{old[0].Wrapper, old[1].Wrapper})
	want := "p.EBESExQVFhcYGRobHB0eHw~k." + old[1].Wrapper.CredentialID + ".QEFCQ0RFRkdISUpLTE1OT1BRUlNUVVZXWFlaW1xdXl8"
	if got != want {
		t.Fatalf("LegacyRequest = %q; quiero %q", got, want)
	}
	key, _ := NewUnlockKey()
	nuevo, _ := WrapSytueID(key, "@a:x", "k", "EsT3 4kza y1Xs efFA tKyi HVwT kdEP 4UEk rvNr dFvy 8nGq AkqP")
	if LegacyRequest([]Wrapper{nuevo}) != "" {
		t.Fatal("un envoltorio sytue-id no es de la transición")
	}
}
