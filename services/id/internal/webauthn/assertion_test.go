// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"testing"
)

const (
	testRPID   = "sytue.eu"
	testOrigin = "https://id.sytue.eu"
)

// --- Constructores CBOR mínimos para los tests (clean-room, RFC 8949/9052) ---

func cborUint(k uint64) []byte {
	switch {
	case k < 24:
		return []byte{byte(k)}
	case k < 256:
		return []byte{0x18, byte(k)}
	default:
		b := []byte{0x19, 0, 0}
		binary.BigEndian.PutUint16(b[1:], uint16(k))
		return b
	}
}

// cborNegInt codifica un entero negativo (major 1, arg = -1 - v).
func cborNegInt(v int64) []byte {
	arg := uint64(-1 - v)
	h := cborUint(arg)
	h[0] |= 0x20 // major type 1
	return h
}

func cborBytes(b []byte) []byte {
	h := cborUint(uint64(len(b)))
	h[0] |= 0x40 // major type 2 (byte string)
	return append(h, b...)
}

// coseES256 construye una COSE_Key EC2/P-256 a partir de una clave pública ECDSA.
func coseES256(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	// Punto sin comprimir 0x04||X||Y (Bytes sustituye a los campos X/Y, deprecados
	// desde Go 1.26).
	raw, err := pub.Bytes()
	if err != nil {
		t.Fatalf("PublicKey.Bytes: %v", err)
	}
	x, y := raw[1:33], raw[33:65]
	m := make([]byte, 0, 96)
	m = append(m, 0xA5) // map con 5 pares
	m = append(m, cborUint(1)...)
	m = append(m, cborUint(ktyEC2)...) // kty: EC2
	m = append(m, cborUint(3)...)
	m = append(m, cborNegInt(algES256)...) // alg: ES256
	m = append(m, cborNegInt(-1)...)
	m = append(m, cborUint(crvP256)...) // crv: P-256
	m = append(m, cborNegInt(-2)...)
	m = append(m, cborBytes(x)...) // x
	m = append(m, cborNegInt(-3)...)
	m = append(m, cborBytes(y)...) // y
	return m
}

// coseRS256 construye una COSE_Key RSA a partir de una clave pública RSA.
func coseRS256(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	n := pub.N.Bytes()
	e := exponentBytes(pub.E)
	m := make([]byte, 0, len(n)+len(e)+16)
	m = append(m, 0xA4) // map con 4 pares
	m = append(m, cborUint(1)...)
	m = append(m, cborUint(ktyRSA)...) // kty: RSA
	m = append(m, cborUint(3)...)
	m = append(m, cborNegInt(algRS256)...) // alg: RS256
	m = append(m, cborNegInt(-1)...)
	m = append(m, cborBytes(n)...) // n
	m = append(m, cborNegInt(-2)...)
	m = append(m, cborBytes(e)...) // e
	return m
}

func exponentBytes(e int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(e))
	i := 0
	for i < len(b)-1 && b[i] == 0 {
		i++
	}
	return b[i:]
}

// authData construye un authenticatorData de assertion (37 bytes: rpIdHash+flags+count).
func authData(rpID string, flags byte, signCount uint32) []byte {
	h := sha256.Sum256([]byte(rpID))
	b := make([]byte, 37)
	copy(b[0:32], h[:])
	b[32] = flags
	binary.BigEndian.PutUint32(b[33:37], signCount)
	return b
}

func clientDataGet(challenge []byte, origin string) []byte {
	c := base64.RawURLEncoding.EncodeToString(challenge)
	return []byte(`{"type":"webauthn.get","challenge":"` + c + `","origin":"` + origin + `"}`)
}

// signES256 firma authData||SHA-256(clientDataJSON) como haría un autenticador.
func signES256(t *testing.T, priv *ecdsa.PrivateKey, ad, cdj []byte) []byte {
	t.Helper()
	cdHash := sha256.Sum256(cdj)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("SignASN1: %v", err)
	}
	return sig
}

const (
	flagUP   = 0x01
	flagUPUV = 0x05 // UP + UV
)

func TestVerifyAssertion_ES256_Valid(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUPUV, 5)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	res, err := VerifyAssertion(AssertionInput{
		COSEPublicKey:     coseES256(t, &priv.PublicKey),
		AuthenticatorData: ad,
		ClientDataJSON:    cdj,
		Signature:         sig,
		StoredSignCount:   4,
		ExpectedChallenge: challenge,
		RPID:              testRPID,
		AllowedOrigins:    []string{testOrigin},
	})
	if err != nil {
		t.Fatalf("assertion válida rechazada: %v", err)
	}
	if res.SignCount != 5 {
		t.Errorf("SignCount = %d, quiero 5", res.SignCount)
	}
	if !res.UserVerified {
		t.Errorf("UserVerified = false, quiero true (flag UV activo)")
	}
}

func TestVerifyAssertion_ES256_TamperedSignature(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)
	sig[len(sig)-1] ^= 0xFF // manipular la firma

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("firma manipulada: err = %v, quiero ErrBadSignature", err)
	}
}

func TestVerifyAssertion_ES256_TamperedAuthData(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)
	ad[36] ^= 0x01 // alterar el signCount firmado

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("authData manipulado: err = %v, quiero ErrBadSignature", err)
	}
}

func TestVerifyAssertion_SignCountRegression(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	// El autenticador presenta 5, pero el almacenado es 9: retroceso ⇒ posible clon.
	ad := authData(testRPID, flagUP, 5)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, StoredSignCount: 9,
		ExpectedChallenge: challenge, RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrSignCountRegression) {
		t.Fatalf("signCount que retrocede: err = %v, quiero ErrSignCountRegression", err)
	}
}

func TestVerifyAssertion_SignCountEqualRejected(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 7)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, StoredSignCount: 7,
		ExpectedChallenge: challenge, RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrSignCountRegression) {
		t.Fatalf("signCount estancado: err = %v, quiero ErrSignCountRegression", err)
	}
}

func TestVerifyAssertion_SignCountBothZeroOK(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 0)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	if _, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, StoredSignCount: 0,
		ExpectedChallenge: challenge, RPID: testRPID, AllowedOrigins: []string{testOrigin},
	}); err != nil {
		t.Fatalf("ambos signCount 0 (autenticador sin contador) debería pasar: %v", err)
	}
}

func TestVerifyAssertion_ChallengeMismatch(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1)
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig,
		ExpectedChallenge: []byte("otro-reto-distinto-al-firmado-000000"),
		RPID:              testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrChallengeMismatch) {
		t.Fatalf("reto distinto: err = %v, quiero ErrChallengeMismatch", err)
	}
}

func TestVerifyAssertion_OriginNotAllowed(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1)
	cdj := clientDataGet(challenge, "https://evil.example")
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrOriginNotAllowed) {
		t.Fatalf("origen ajeno: err = %v, quiero ErrOriginNotAllowed", err)
	}
}

func TestVerifyAssertion_WrongType(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1)
	c := base64.RawURLEncoding.EncodeToString(challenge)
	cdj := []byte(`{"type":"webauthn.create","challenge":"` + c + `","origin":"` + testOrigin + `"}`)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrAssertionType) {
		t.Fatalf("tipo webauthn.create en login: err = %v, quiero ErrAssertionType", err)
	}
}

func TestVerifyAssertion_RPIDMismatch(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData("otro-dominio.example", flagUP, 1) // rpIdHash de otro dominio
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin},
	})
	if !errors.Is(err, ErrRPIDMismatch) {
		t.Fatalf("rpIdHash ajeno: err = %v, quiero ErrRPIDMismatch", err)
	}
}

func TestVerifyAssertion_RequireUVButAbsent(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 1) // solo UP, sin UV
	cdj := clientDataGet(challenge, testOrigin)
	sig := signES256(t, priv, ad, cdj)

	_, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseES256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, ExpectedChallenge: challenge,
		RPID: testRPID, AllowedOrigins: []string{testOrigin}, RequireUserVerification: true,
	})
	if !errors.Is(err, ErrUserNotVerified) {
		t.Fatalf("UV requerido y ausente: err = %v, quiero ErrUserNotVerified", err)
	}
}

func TestVerifyAssertion_RS256_Valid(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	challenge := []byte("challenge-0123456789-abcdef-32bytes!")
	ad := authData(testRPID, flagUP, 3)
	cdj := clientDataGet(challenge, testOrigin)
	cdHash := sha256.Sum256(cdj)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAssertion(AssertionInput{
		COSEPublicKey: coseRS256(t, &priv.PublicKey), AuthenticatorData: ad,
		ClientDataJSON: cdj, Signature: sig, StoredSignCount: 2,
		ExpectedChallenge: challenge, RPID: testRPID, AllowedOrigins: []string{testOrigin},
	}); err != nil {
		t.Fatalf("assertion RS256 válida rechazada: %v", err)
	}
}

func TestParseCOSEPublicKey_RoundTrip(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, alg, err := ParseCOSEPublicKey(coseES256(t, &priv.PublicKey))
	if err != nil {
		t.Fatalf("ParseCOSEPublicKey: %v", err)
	}
	if alg != algES256 {
		t.Errorf("alg = %d, quiero %d", alg, algES256)
	}
	ep, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("tipo de clave = %T, quiero *ecdsa.PublicKey", pub)
	}
	if !ep.Equal(&priv.PublicKey) {
		t.Errorf("la clave no coincide tras el round-trip")
	}
}
