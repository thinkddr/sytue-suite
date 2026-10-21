// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Alta de una passkey desde «Mi cuenta», contra PostgreSQL real.
//
// El login por passkey tenía tests; el ALTA no tenía ninguno, y es la mitad que
// el usuario toca primero: si registrar falla, la passkey no existe y todo lo
// demás da igual. Se ejercita el flujo entero —begin, ceremonia falsa del
// autenticador, finish— porque los fallos de WebAuthn son de acoplamiento entre
// las dos mitades (el reto, el origen, el rpIdHash), no de una función suelta.
package http

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// authDataConCredencial fabrica el authenticatorData de un REGISTRO: cabecera
// más la credencial adjunta (AAGUID, id y clave COSE), que es lo que distingue
// un alta de una verificación.
func authDataConCredencial(t *testing.T, rpID string, credID, cose []byte) []byte {
	t.Helper()
	h := sha256.Sum256([]byte(rpID))
	b := make([]byte, 37)
	copy(b[0:32], h[:])
	// UP (presencia) + AT (credencial adjunta): sin los dos, el handler rechaza.
	b[32] = 0x01 | 0x40
	binary.BigEndian.PutUint32(b[33:37], 0)

	b = append(b, make([]byte, 16)...) // AAGUID, todo ceros
	var lon [2]byte
	binary.BigEndian.PutUint16(lon[:], uint16(len(credID)))
	b = append(b, lon[:]...)
	b = append(b, credID...)
	return append(b, cose...)
}

// coseES256 serializa una clave pública P-256 en el mapa COSE mínimo.
func coseES256Test(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	x := pub.X.FillBytes(make([]byte, 32))
	y := pub.Y.FillBytes(make([]byte, 32))
	var b []byte
	b = append(b, 0xa5)             // mapa de 5
	b = append(b, 0x01, 0x02)       // kty: EC2
	b = append(b, 0x03, 0x26)       // alg: ES256 (-7)
	b = append(b, 0x20, 0x01)       // crv: P-256
	b = append(b, 0x21, 0x58, 0x20) // x: bytes(32)
	b = append(b, x...)             //
	b = append(b, 0x22, 0x58, 0x20) // y: bytes(32)
	return append(b, y...)          //
}

// attestationNone envuelve el authData en un attestationObject CBOR con
// fmt="none", que es lo que manda un autenticador cuando no se pide attestation
// (y es lo que pide este servidor).
func attestationNone(authData []byte) []byte {
	var b []byte
	b = append(b, 0xa3) // mapa de 3
	b = append(b, 0x63, 'f', 'm', 't')
	b = append(b, 0x64, 'n', 'o', 'n', 'e')
	b = append(b, 0x67, 'a', 't', 't', 'S', 't', 'm', 't')
	b = append(b, 0xa0) // mapa vacío
	b = append(b, 0x68, 'a', 'u', 't', 'h', 'D', 'a', 't', 'a')
	// bytes(n) con longitud de dos octetos: authData siempre pasa de 255.
	b = append(b, 0x59)
	var lon [2]byte
	binary.BigEndian.PutUint16(lon[:], uint16(len(authData)))
	b = append(b, lon[:]...)
	return append(b, authData...)
}

// cookieDeSesion crea una sesión de navegador para el usuario, que es como
// llega /account: con cookie, no con Bearer.
func (l *liveServer) cookieDeSesion(t *testing.T, userID string) *http.Cookie {
	t.Helper()
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	ahora := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: userID,
		AuthTime: ahora, AMR: []string{"pwd"}, UserAgent: "test",
		ExpiresAt: ahora.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: raw}
}

func TestAltaDePasskeyDesdeMiCuenta(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "passkey@id.test", "")
	origen := l.srv.cfg.RPOrigins[0]

	// 1. Begin. Va con la sesión del navegador (no un Bearer), como /account.
	req := httptest.NewRequest(http.MethodPost, "/account/passkeys/begin", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origen)
	req.AddCookie(l.cookieDeSesion(t, u.ID))
	rec := httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("begin: código=%d body=%s", rec.Code, rec.Body)
	}
	var opts struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("opciones ilegibles: %v (%s)", err, rec.Body)
	}
	if opts.Challenge == "" || opts.RP.ID == "" || opts.User.ID == "" {
		t.Fatalf("opciones incompletas: %+v", opts)
	}

	// 2. La ceremonia que haría el autenticador.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("clave: %v", err)
	}
	credID := []byte("credencial-de-prueba-0001")
	authData := authDataConCredencial(t, opts.RP.ID, credID, coseES256Test(t, &priv.PublicKey))
	clientData := []byte(`{"type":"webauthn.create","challenge":"` + opts.Challenge +
		`","origin":"` + origen + `"}`)
	b64 := base64.RawURLEncoding.EncodeToString

	cuerpo, _ := json.Marshal(map[string]string{
		"id":                b64(credID),
		"rawId":             b64(credID),
		"type":              "public-key",
		"name":              "Llave de prueba",
		"attestationObject": b64(attestationNone(authData)),
		"clientDataJSON":    b64(clientData),
	})

	// 3. Finish.
	req = httptest.NewRequest(http.MethodPost, "/account/passkeys/finish", strings.NewReader(string(cuerpo)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origen)
	req.AddCookie(l.cookieDeSesion(t, u.ID))
	rec = httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("finish: código=%d body=%s", rec.Code, rec.Body)
	}

	// 4. La passkey EXISTE y el usuario cuenta como con segundo factor: si esto
	//    no ocurre, el alta «funciona» y el usuario sigue sin poder usarla.
	creds, err := l.st.ListCredentialsForUser(l.ctx, u.ID)
	if err != nil || len(creds) != 1 {
		t.Fatalf("credenciales = %d (%v), quiero 1", len(creds), err)
	}
	usuario, err := l.st.GetUserByID(l.ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !usuario.MFAEnabled {
		t.Error("la passkey se registró pero el usuario sigue sin segundo factor")
	}

	// 5. El reto es de UN SOLO USO: repetir el mismo finish no puede colar otra.
	req = httptest.NewRequest(http.MethodPost, "/account/passkeys/finish", strings.NewReader(string(cuerpo)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origen)
	req.AddCookie(l.cookieDeSesion(t, u.ID))
	rec = httptest.NewRecorder()
	l.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Error("el mismo reto se pudo consumir dos veces")
	}
}
