// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

const (
	cbSub     = "0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00"
	cbTenant  = "acme"
	cbProduct = "mail"
)

// cbFirmante es una clave de firma de bóveda determinista por semilla.
func cbFirmante(t *testing.T, b byte) *recoverywrap.PQVaultSigner {
	t.Helper()
	s, err := recoverywrap.ParsePQVaultSigningKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cbID(t *testing.T, s *recoverywrap.PQVaultSigner) string {
	t.Helper()
	id, err := s.Public().ID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// cbSolicitud arma la solicitud de registrar `nueva` como generación `gen`, encima
// de `anterior` (nil en el alta). `aprueba` firma también con la anterior.
func cbSolicitud(t *testing.T, nueva, anterior *recoverywrap.PQVaultSigner, gen int64, aprueba bool) solicitudClaveBoveda {
	t.Helper()
	prev := ""
	if anterior != nil {
		prev = cbID(t, anterior)
	}
	decl, err := recoverywrap.PQVaultKeyStatement(cbSub, cbTenant, cbProduct, gen, cbID(t, nueva), prev)
	if err != nil {
		t.Fatal(err)
	}
	prueba, err := nueva.Sign(recoverywrap.VaultKeyEnrollContext, decl)
	if err != nil {
		t.Fatal(err)
	}
	sol := solicitudClaveBoveda{PublicKey: nueva.Public(), PreviousKeyID: prev, Proof: prueba}
	if aprueba {
		if sol.Continuity, err = anterior.Sign(recoverywrap.VaultKeyRotateContext, decl); err != nil {
			t.Fatal(err)
		}
	}
	return sol
}

func cbFila(t *testing.T, s *recoverywrap.PQVaultSigner, gen int64) *store.VaultKeyRow {
	t.Helper()
	publica, err := json.Marshal(s.Public())
	if err != nil {
		t.Fatal(err)
	}
	return &store.VaultKeyRow{Generation: gen, KeyID: cbID(t, s), PublicKey: publica}
}

func cbCodigo(t *testing.T, err error, quiero *httpx.Error) {
	t.Helper()
	var e *httpx.Error
	if !errors.As(err, &e) || e.Code != quiero.Code || e.Status != quiero.Status {
		t.Fatalf("error = %v; quiero %s (%d)", err, quiero.Code, quiero.Status)
	}
}

func TestClaveBovedaAltaContinuidadYVaciado(t *testing.T) {
	a, b, c := cbFirmante(t, 1), cbFirmante(t, 2), cbFirmante(t, 3)

	// Alta: la primera, con la sesión y sin nada más.
	cambio, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, cbSolicitud(t, a, nil, 1, false), nil, false)
	if err != nil || cambio.Reason != motivoAlta || cambio.KeyID != cbID(t, a) || cambio.PreviousKeyID != "" {
		t.Fatalf("alta: %+v, %v", cambio, err)
	}
	// La misma clave otra vez: nada que guardar.
	if _, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, cbSolicitud(t, a, nil, 1, false), cbFila(t, a, 1), false); !errors.Is(err, store.ErrVaultKeyUnchanged) {
		t.Fatalf("repetida: %v", err)
	}
	// Continuidad: la activa aprueba la nueva; no hace falta identificarse otra vez.
	cambio, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, cbSolicitud(t, b, a, 2, true), cbFila(t, a, 1), false)
	if err != nil || cambio.Reason != motivoContinuidad || cambio.PreviousKeyID != cbID(t, a) {
		t.Fatalf("continuidad: %+v, %v", cambio, err)
	}
	// Vaciado: sin la firma de la activa, solo con identificación reciente.
	sinAprobar := cbSolicitud(t, c, b, 3, false)
	if _, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, sinAprobar, cbFila(t, b, 2), false); err == nil {
		t.Fatal("vaciado sin identificación reciente aceptado")
	} else {
		cbCodigo(t, err, errClaveBovedaReautenticar)
	}
	cambio, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, sinAprobar, cbFila(t, b, 2), true)
	if err != nil || cambio.Reason != motivoVaciado {
		t.Fatalf("vaciado reciente: %+v, %v", cambio, err)
	}
}

// El traspaso de la autoridad de Matrix a la bóveda cuesta siempre una
// identificación reciente, también en un alta, y deja el motivo `traspaso`.
func TestClaveBovedaTraspaso(t *testing.T) {
	a, b := cbFirmante(t, 1), cbFirmante(t, 2)
	traspaso := func(sol solicitudClaveBoveda) solicitudClaveBoveda { sol.Handover = true; return sol }

	// Alta con traspaso: sin identificación reciente, no.
	alta := traspaso(cbSolicitud(t, a, nil, 1, false))
	_, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, alta, nil, false)
	cbCodigo(t, err, errClaveBovedaReautenticar)
	cambio, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, alta, nil, true)
	if err != nil || cambio.Reason != motivoTraspaso || cambio.PreviousKeyID != "" {
		t.Fatalf("alta con traspaso: %+v, %v", cambio, err)
	}
	// Sobre una clave que ya había: generación siguiente, también reciente.
	encima := traspaso(cbSolicitud(t, b, a, 2, false))
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, encima, cbFila(t, a, 1), false)
	cbCodigo(t, err, errClaveBovedaReautenticar)
	if cambio, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, encima, cbFila(t, a, 1), true); err != nil || cambio.Reason != motivoTraspaso {
		t.Fatalf("traspaso sobre otra clave: %+v, %v", cambio, err)
	}
	// La misma clave que ya estaba sin traspaso: no se le cuelga la marca.
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, alta, cbFila(t, a, 1), true)
	cbCodigo(t, err, errClaveBovedaTraspaso)
	// Repetido tras una respuesta perdida: el ya hecho, nada nuevo.
	hecho := cbFila(t, a, 1)
	hecho.Reason = motivoTraspaso
	if _, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, alta, hecho, true); !errors.Is(err, store.ErrVaultKeyUnchanged) {
		t.Fatalf("traspaso repetido: %v", err)
	}
	// Solo el correo, y nunca con la aprobación de la anterior a la vez.
	cal := traspaso(cbSolicitud(t, a, nil, 1, false))
	_, err = decidirClaveBoveda(cbSub, cbTenant, "calendar", cal, nil, true)
	cbCodigo(t, err, errClaveBovedaTraspaso)
	aprobado := traspaso(cbSolicitud(t, b, a, 2, true))
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, aprobado, cbFila(t, a, 1), true)
	cbCodigo(t, err, errClaveBovedaTraspaso)
	// Y sin la marca, nada cambia: un alta sigue entrando con la sesión.
	if cambio, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, cbSolicitud(t, a, nil, 1, false), nil, false); err != nil || cambio.Reason != motivoAlta {
		t.Fatalf("alta sin traspaso: %+v, %v", cambio, err)
	}
}

// Nadie registra una clave que no tiene, ni la hace pasar por aprobada, ni la
// cuela en otra bóveda, ni sobre una generación que ya no es la activa.
func TestClaveBovedaRechazaLoQueNoVerifica(t *testing.T) {
	a, b, intrusa := cbFirmante(t, 1), cbFirmante(t, 2), cbFirmante(t, 9)
	activa := cbFila(t, a, 1)

	ajena := cbSolicitud(t, b, a, 2, true)
	ajena.PublicKey = intrusa.Public() // la prueba es de b, la clave que se registra no
	_, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, ajena, activa, true)
	cbCodigo(t, err, errClaveBovedaPrueba)

	// Continuidad firmada por una clave que no es la activa: no vale como
	// aprobación… ni se convierte en vaciado aunque la sesión sea reciente.
	falsa := cbSolicitud(t, b, a, 2, false)
	if falsa.Continuity, err = intrusa.Sign(recoverywrap.VaultKeyRotateContext, []byte("x")); err != nil {
		t.Fatal(err)
	}
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, falsa, activa, true)
	cbCodigo(t, err, errClaveBovedaPrueba)

	// Una prueba de alta presentada como aprobación (mismos bytes, otro contexto).
	cruzada := cbSolicitud(t, b, a, 2, false)
	cruzada.Continuity = cruzada.Proof
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, cruzada, activa, true)
	cbCodigo(t, err, errClaveBovedaPrueba)

	// La misma solicitud, en otra bóveda (otro producto u otra persona).
	buena := cbSolicitud(t, b, a, 2, true)
	_, err = decidirClaveBoveda(cbSub, cbTenant, "drive", buena, activa, true)
	cbCodigo(t, err, errClaveBovedaPrueba)
	_, err = decidirClaveBoveda("otra-persona", cbTenant, cbProduct, buena, activa, true)
	cbCodigo(t, err, errClaveBovedaPrueba)

	// Firmada para la generación 2 cuando la activa ya es la 2: desfasada.
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, buena, cbFila(t, cbFirmante(t, 4), 2), true)
	cbCodigo(t, err, errClaveBovedaDesfasada)
	// Un alta que dice venir de una anterior que no existe.
	_, err = decidirClaveBoveda(cbSub, cbTenant, cbProduct, buena, nil, true)
	cbCodigo(t, err, errClaveBovedaDesfasada)

	rota := buena
	rota.PublicKey.MLDSA65 = rota.PublicKey.MLDSA65[:10]
	if _, err := decidirClaveBoveda(cbSub, cbTenant, cbProduct, rota, activa, true); err == nil {
		t.Fatal("clave pública truncada aceptada")
	}
}

func TestClaveBovedaIdentificacionReciente(t *testing.T) {
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	hace := func(d time.Duration) tokenClaims { return tokenClaims{AuthTime: ahora.Add(-d)} }
	if !identificacionRecienteDelToken(hace(time.Minute), false, ahora) {
		t.Error("hace un minuto, sin segundo factor: vale")
	}
	if identificacionRecienteDelToken(hace(frescuraStepUp+time.Second), false, ahora) {
		t.Error("pasado el plazo: no vale")
	}
	if identificacionRecienteDelToken(tokenClaims{}, false, ahora) {
		t.Error("sin auth_time: no vale")
	}
	conPwd := hace(time.Minute)
	conPwd.AMR = []string{"pwd"}
	if identificacionRecienteDelToken(conPwd, true, ahora) {
		t.Error("cuenta con segundo factor, entrada solo con contraseña: no vale")
	}
	for _, amr := range []string{"mfa", "otp"} {
		c := hace(time.Minute)
		c.AMR = []string{"pwd", amr}
		if !identificacionRecienteDelToken(c, true, ahora) {
			t.Errorf("cuenta con segundo factor, entrada con %s: vale", amr)
		}
	}
	sinPIN := hace(time.Minute)
	sinPIN.AMR = []string{"webauthn"}
	if identificacionRecienteDelToken(sinPIN, true, ahora) {
		t.Error("llave sin verificación de usuario: no vale")
	}
}

func TestClaveBovedaAvisoDeVaciado(t *testing.T) {
	msg := string(buildVaultKeyResetEmail("Sytue <no-reply@sytue.eu>", "ana@acme.test", "mail", 3))
	for _, quiero := range []string{"To: ana@acme.test", "generacion 3", "cambia ya tu contrasena"} {
		if !strings.Contains(msg, quiero) {
			t.Errorf("al aviso le falta %q:\n%s", quiero, msg)
		}
	}
	if strings.Contains(msg, "http") {
		t.Error("el aviso no lleva enlaces")
	}
}
