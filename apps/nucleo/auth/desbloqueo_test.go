// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
)

// navegadorConRelevo hace de navegador Y de relevo de Sytue ID (ADR 0084): lee la
// clave pública de /authorize, cifra para ella la clave de desbloqueo de `sub` y la
// añade a la vuelta, como hace el JavaScript del relevo.
func (idp *idpFalso) navegadorConRelevo(t *testing.T, sub string, clave []byte, vistas *url.Values) func(string) error {
	return func(autorizar string) error {
		u, err := url.Parse(autorizar)
		if err != nil {
			return err
		}
		q := u.Query()
		*vistas = q
		idp.reto = q.Get("code_challenge")
		idp.dispositivo = q.Get("sytue_device")
		vuelta := url.Values{"code": {"c0d1g0"}, "state": {q.Get("state")}}
		if pub := q.Get("sytue_unlock_key"); pub != "" {
			blob, err := recoverywrap.SealDelivery(pub, recoverywrap.Delivery{V: 1, Sub: sub, Key: clave}, nil, nil)
			if err != nil {
				t.Errorf("SealDelivery: %v", err)
			}
			vuelta.Set("sytue_unlock", blob)
		}
		go func() {
			resp, err := http.Get(redirectURI + "?" + vuelta.Encode())
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

// El inicio de sesión nativo pide el relevo con una clave efímera y se queda con la
// clave de desbloqueo SOLO si es de la persona de los tokens. Nunca la pide a nadie:
// llega cifrada en la vuelta del navegador.
func TestElLoginNativoTraeLaClaveDeDesbloqueoDeSuPersona(t *testing.T) {
	clave := bytes.Repeat([]byte{7}, 32)
	idp := nuevoIDP(t)
	var vistas url.Values
	o := &OIDC{Issuer: idp.URL, Open: idp.navegadorConRelevo(t, "ada", clave, &vistas)}
	tok, err := o.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if vistas.Get("sytue_unlock_key") == "" || vistas.Has("prompt") || vistas.Has("sytue_legacy") {
		t.Fatalf("/authorize: %v", vistas)
	}
	if tok.Desbloqueo == nil || !bytes.Equal(tok.Desbloqueo.Clave, clave) {
		t.Fatalf("sin clave de desbloqueo: %+v", tok.Desbloqueo)
	}
	// La de otra persona no se acepta (el IdP falso dice que el sub es «ada»).
	idp = nuevoIDP(t)
	o = &OIDC{Issuer: idp.URL, Open: idp.navegadorConRelevo(t, "beto", clave, &vistas)}
	if tok, err = o.Login(context.Background()); err != nil || tok.Desbloqueo != nil {
		t.Fatalf("aceptó la clave de otra persona: %+v, %v", tok.Desbloqueo, err)
	}
	// Volver a entrar para desbloquear: prompt=login y los parámetros de la 0082.
	idp = nuevoIDP(t)
	o = &OIDC{Issuer: idp.URL, Open: idp.navegadorConRelevo(t, "ada", clave, &vistas)}
	if _, err = o.LoginCon(context.Background(), PeticionLogin{Prompt: "login", Legado: "p.EBESExQVFhcYGRobHB0eHw"}); err != nil {
		t.Fatal(err)
	}
	if vistas.Get("prompt") != "login" || vistas.Get("sytue_legacy") != "p.EBESExQVFhcYGRobHB0eHw" {
		t.Fatalf("/authorize para desbloquear: %v", vistas)
	}
	// Un legado con algo que no es base64url no viaja.
	if legadoValido("p.<script>") || !legadoValido("p.a-b_c~k.x.y") {
		t.Fatal("legadoValido")
	}
}

// La clave viaja con la sesión al almacén seguro (y así a las demás apps nativas),
// sobrevive a las renovaciones de la misma persona y se va con la sesión. Fuera del
// almacén no se serializa.
func TestLaClaveDeDesbloqueoVaConLaSesion(t *testing.T) {
	d := &Desbloqueo{Clave: bytes.Repeat([]byte{9}, 32), Legado: []recoverywrap.LegacyIKM{{Kind: "password", Salt: []byte{1}, IKM: bytes.Repeat([]byte{2}, 32)}}}
	t0 := Tokens{Refresh: "r", Subject: "ada", Desbloqueo: d}
	crudo, err := serializar(t0)
	if err != nil {
		t.Fatal(err)
	}
	vuelta, err := recuperar(crudo)
	if err != nil || vuelta.Desbloqueo == nil || !bytes.Equal(vuelta.Desbloqueo.Clave, d.Clave) || len(vuelta.Desbloqueo.Legado) != 1 {
		t.Fatalf("recuperar: %+v, %v", vuelta.Desbloqueo, err)
	}
	if crudoTokens, _ := json.Marshal(t0); strings.Contains(string(crudoTokens), "CQkJ") {
		t.Fatalf("Tokens serializa la clave: %s", crudoTokens)
	}
	// Sin sesión no se guarda.
	if crudo, _ := serializar(Tokens{Subject: "ada", Desbloqueo: d}); strings.Contains(string(crudo), "desbloqueo") {
		t.Fatal("se guarda la clave sin sesión")
	}
	// Una renovación de la misma persona la conserva; la de otra, no.
	if got := conservar(Tokens{Access: "a2", Subject: "ada"}, t0); got.Desbloqueo != d {
		t.Fatal("la renovación pierde la clave")
	}
	if got := conservar(Tokens{Access: "a2", Subject: "beto"}, t0); got.Desbloqueo != nil {
		t.Fatal("la clave pasa a otra persona")
	}
	var guardados []Tokens
	s := NewSession(t0, nil, func(t Tokens) error { guardados = append(guardados, t); return nil })
	if clave, legado := s.Desbloqueo(); !bytes.Equal(clave, d.Clave) || len(legado) != 1 {
		t.Fatal("la sesión no da su clave")
	}
	if err := s.OlvidarLegado(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, legado := s.Desbloqueo(); len(legado) != 0 || len(guardados) != 1 || len(guardados[0].Desbloqueo.Legado) != 0 {
		t.Fatal("el legado sigue")
	}
	if err := s.Logout(); err != nil {
		t.Fatal(err)
	}
	if clave, _ := s.Desbloqueo(); clave != nil {
		t.Fatal("la clave sobrevive al cierre de sesión")
	}
}
