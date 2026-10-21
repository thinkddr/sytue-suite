// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"strings"
	"testing"
)

// TestEncryptPushRoundTrip verifica el cifrado Web Push (RFC 8291 / aes128gcm) de extremo a
// extremo: cifra con la clave pública del cliente y REDESCIFRA como lo haría el navegador
// (re-derivando CEK/NONCE desde el lado del cliente con la clave efímera del header). Es el
// self-check autoritativo del cripto: un descifrador conforme recupera el plano ⇔ el cifrado
// cumple la especificación.
func TestEncryptPushRoundTrip(t *testing.T) {
	curve := ecdh.P256()
	uaPriv, err := curve.GenerateKey(rand.Reader) // par del "navegador"
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("rand auth: %v", err)
	}
	plaintext := []byte(`{"title":"Ana","body":"te ha mencionado en #general","url":"/?channel=c&message=m"}`)

	body, err := encryptPush(rand.Reader, uaPriv.PublicKey().Bytes(), auth, plaintext)
	if err != nil {
		t.Fatalf("encryptPush: %v", err)
	}

	// Header aes128gcm (RFC 8188): salt(16) || rs(4) || idlen(1)=65 || keyid(65).
	if len(body) < 16+4+1+65 {
		t.Fatalf("cuerpo demasiado corto: %d", len(body))
	}
	if rs := binary.BigEndian.Uint32(body[16:20]); rs != 4096 {
		t.Fatalf("record size = %d, quiero 4096", rs)
	}
	if idlen := int(body[20]); idlen != 65 {
		t.Fatalf("idlen = %d, quiero 65", idlen)
	}

	got, err := decryptPushForTest(uaPriv, auth, body)
	if err != nil {
		t.Fatalf("descifrado: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip: got %q, quiero %q", got, plaintext)
	}
}

// TestEncryptPushRejectsBadInput cubre los guardas de forma.
func TestEncryptPushRejectsBadInput(t *testing.T) {
	uaPriv, _ := ecdh.P256().GenerateKey(rand.Reader)
	// auth de tamaño incorrecto.
	if _, err := encryptPush(rand.Reader, uaPriv.PublicKey().Bytes(), make([]byte, 8), []byte("x")); err == nil {
		t.Fatal("auth de 8 bytes debería fallar")
	}
	// p256dh inválida (no es un punto).
	if _, err := encryptPush(rand.Reader, []byte("no-soy-un-punto"), make([]byte, 16), []byte("x")); err == nil {
		t.Fatal("p256dh inválida debería fallar")
	}
}

// decryptPushForTest redescifra el cuerpo aes128gcm como el navegador: re-deriva las claves
// desde el lado del cliente (RFC 8291) usando la clave efímera del servidor tomada del header.
func decryptPushForTest(uaPriv *ecdh.PrivateKey, auth, body []byte) ([]byte, error) {
	salt := body[0:16]
	idlen := int(body[20])
	asPub := body[21 : 21+idlen]
	ciphertext := body[21+idlen:]

	asPubKey, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		return nil, err
	}
	shared, err := uaPriv.ECDH(asPubKey)
	if err != nil {
		return nil, err
	}
	uaPub := uaPriv.PublicKey().Bytes()

	prkKey, err := hkdf.Extract(sha256.New, shared, auth)
	if err != nil {
		return nil, err
	}
	keyInfo := append([]byte("WebPush: info\x00"), uaPub...)
	keyInfo = append(keyInfo, asPub...)
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	rec, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	// Quita el relleno 0x00 final y el delimitador 0x02 (registro único = último).
	i := len(rec) - 1
	for i >= 0 && rec[i] == 0x00 {
		i--
	}
	if i < 0 || rec[i] != 0x02 {
		return nil, errTestNoDelimiter
	}
	return rec[:i], nil
}

var errTestNoDelimiter = errStr("delimitador de registro ausente")

type errStr string

func (e errStr) Error() string { return string(e) }

// TestVAPIDAuthorizationHeader firma un JWT VAPID y verifica su firma ES256 con la clave
// pública, más la estructura de la cabecera «vapid t=…, k=…».
func TestVAPIDAuthorizationHeader(t *testing.T) {
	// Genera un par P-256 y exprésalo como lo espera parseVAPIDKeys (base64url).
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pubBytes := priv.PublicKey().Bytes() // 65 bytes sin comprimir
	v, err := parseVAPIDKeys(b64urlEncode(pubBytes), b64urlEncode(priv.Bytes()), "mailto:ops@sytue.eu")
	if err != nil {
		t.Fatalf("parseVAPIDKeys: %v", err)
	}

	header, err := v.authorizationHeader("https://fcm.googleapis.com/fcm/send/abc123")
	if err != nil {
		t.Fatalf("authorizationHeader: %v", err)
	}
	if !strings.HasPrefix(header, "vapid t=") || !strings.Contains(header, ", k="+v.pubB64) {
		t.Fatalf("cabecera con formato inesperado: %q", header)
	}

	jwt := strings.TrimPrefix(strings.SplitN(header, ",", 2)[0], "vapid t=")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT debe tener 3 partes, tiene %d", len(parts))
	}
	sig, err := b64urlDecode(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("firma ES256 debe ser 64 bytes (R||S): len=%d err=%v", len(sig), err)
	}

	// Verifica la firma con la clave pública reconstruida del punto sin comprimir.
	// Se parsea el punto entero en vez de armar la clave con X e Y a mano: esos
	// campos quedaron obsoletos en Go 1.26 y el parseo comprueba además que el
	// punto esté en la curva, que aquí es justo lo que se quiere afirmar.
	pk, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pubBytes[:65])
	if err != nil {
		t.Fatalf("la clave pública VAPID no es un punto válido de P-256: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pk, digest[:], r, s) {
		t.Fatal("la firma ES256 del JWT VAPID no verifica con la clave pública")
	}

	// El payload debe llevar el aud del origen del endpoint.
	payload, err := b64urlDecode(parts[1])
	if err != nil || !strings.Contains(string(payload), `"aud":"https://fcm.googleapis.com"`) {
		t.Fatalf("payload sin aud del origen: %s (err=%v)", payload, err)
	}
}

// TestParseVAPIDKeysErrors cubre los rechazos de claves malformadas.
func TestParseVAPIDKeysErrors(t *testing.T) {
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	goodPub := b64urlEncode(priv.PublicKey().Bytes())
	goodPriv := b64urlEncode(priv.Bytes())

	if _, err := parseVAPIDKeys("!!!", goodPriv, ""); err == nil {
		t.Fatal("pública base64 inválida debería fallar")
	}
	if _, err := parseVAPIDKeys(goodPub, "!!!", ""); err == nil {
		t.Fatal("privada base64 inválida debería fallar")
	}
	if _, err := parseVAPIDKeys(b64urlEncode([]byte{0x04, 0x01}), goodPriv, ""); err == nil {
		t.Fatal("pública de tamaño incorrecto debería fallar")
	}
	if _, err := parseVAPIDKeys(goodPub, b64urlEncode(make([]byte, 16)), ""); err == nil {
		t.Fatal("escalar privado de 16 bytes debería fallar")
	}
	// Sin subject explícito cae al contacto por defecto.
	v, err := parseVAPIDKeys(goodPub, goodPriv, "")
	if err != nil || v.subject == "" {
		t.Fatalf("parseVAPIDKeys sin subject: v=%v err=%v", v, err)
	}
}

// El par que GENERA el despliegue es el que el servicio sabe leer.
//
// Es el eslabón que nadie probaba y por el que el Web Push estuvo apagado: el
// código llevaba desde la Ola 18 esperando `SYTUE_VAPID_*`, y las claves las
// fabrica `02b-secretos.sh` con openssl —punto sin comprimir de 65 bytes y
// escalar de 32, base64url SIN relleno—. Si esos dos formatos dejan de coincidir,
// el fallo no es un error: es `NewPushSender` devolviendo (nil, nil) y ninguna
// notificación saliendo, en silencio.
//
// Aquí se reproduce exactamente lo que el script produce, incluidas las
// longitudes que comprueba antes de guardar (87 y 43 caracteres), y se exige que
// `parseVAPIDKeys` lo acepte.
func TestElParQueGeneraElDespliegueSeParsea(t *testing.T) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generar: %v", err)
	}
	// `openssl ec -pubout -outform DER | tail -c 65` son estos 65 bytes, y
	// `openssl ec -outform DER | tail -c 32`, estos 32.
	pubB64 := b64urlEncode(priv.PublicKey().Bytes())
	privB64 := b64urlEncode(priv.Bytes())

	if len(pubB64) != 87 || len(privB64) != 43 {
		t.Fatalf("longitudes %d/%d: el guardián de 02b-secretos.sh espera 87/43 y rechazaría este par",
			len(pubB64), len(privB64))
	}
	if pubB64[0] != 'B' {
		t.Errorf("la pública empieza por %q: debería ser el punto SIN comprimir (0x04 ⇒ 'B')", pubB64[0])
	}

	v, err := parseVAPIDKeys(pubB64, privB64, "mailto:tls@sytue.eu")
	if err != nil {
		t.Fatalf("el servicio rechaza el par que genera el despliegue: %v", err)
	}
	if v.pubB64 != pubB64 {
		t.Errorf("la pública que publica el endpoint no es la generada: %q", v.pubB64)
	}
	if v.subject != "mailto:tls@sytue.eu" {
		t.Errorf("subject = %q", v.subject)
	}
}
