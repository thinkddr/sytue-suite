// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

// Identificadores de cliente OAuth de la compilación (`-ldflags -X`). Vacíos, Ajustes
// pide a la persona el suyo: registrar la app en Google y en Microsoft es del dueño.
var (
	ClienteGoogle        string
	ClienteGoogleSecreto string
	ClienteMicrosoft     string
)

// ClientesCompilados devuelve los de la compilación.
func ClientesCompilados() ClientesOAuth {
	return ClientesOAuth{Google: ClienteGoogle, GoogleSecreto: ClienteGoogleSecreto, Microsoft: ClienteMicrosoft}
}

// OAuth es un cliente público con PKCE (RFC 7636) y redirección a 127.0.0.1 (RFC 8252
// §7.3) o, en el móvil, al esquema de la app (§7.1, vuelta.go).
type OAuth struct {
	Autorizar, Token, Cliente, Secreto string
	Alcances                           []string
	Extra                              url.Values
	// Vuelta, si viene, es la vuelta por esquema de esta alta; nil, la de la app
	// (UsarVuelta) o, sin ella, 127.0.0.1.
	Vuelta *Vuelta
}

// OAuthGoogle: Drive completo, para poder abrir cualquier fichero de la persona. Es un
// permiso «restringido»: distribuirlo pide la verificación de Google (§11 del plan); con
// un cliente propio en modo pruebas funciona para su dueño.
func OAuthGoogle(cliente, secreto string) OAuth {
	// G101: son las URL públicas de autorización y de tokens de Google, no una credencial.
	return OAuth{ //nolint:gosec // G101: ver arriba
		Autorizar: "https://accounts.google.com/o/oauth2/v2/auth", Token: "https://oauth2.googleapis.com/token", Cliente: cliente, Secreto: secreto,
		Alcances: []string{"https://www.googleapis.com/auth/drive"}, Extra: url.Values{"access_type": {"offline"}, "prompt": {"consent"}},
	}
}

// OAuthMicrosoft: OneDrive personal y de Microsoft 365, con cualquier inquilino.
func OAuthMicrosoft(cliente string) OAuth {
	// G101: son las URL públicas de autorización y de tokens de Microsoft, no una credencial.
	return OAuth{ //nolint:gosec // G101: ver arriba
		Autorizar: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", Token: "https://login.microsoftonline.com/common/oauth2/v2.0/token", Cliente: cliente,
		Alcances: []string{"Files.ReadWrite", "offline_access"},
	}
}

// Tokens es la respuesta del extremo de tokens.
type Tokens struct {
	Acceso   string `json:"access_token"`
	Refresco string `json:"refresh_token"`
	Expira   int    `json:"expires_in"`
}

func aleatorio() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// urlAutorizar es la dirección de la página de permiso del proveedor, con PKCE (S256).
func (o OAuth) urlAutorizar(redireccion, estado string, reto [sha256.Size]byte) string {
	v := url.Values{
		"client_id": {o.Cliente}, "redirect_uri": {redireccion}, "response_type": {"code"},
		"scope": {strings.Join(o.Alcances, " ")}, "state": {estado},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(reto[:])}, "code_challenge_method": {"S256"},
	}
	for k, x := range o.Extra {
		v[k] = x
	}
	return o.Autorizar + "?" + v.Encode()
}

// validarVuelta comprueba lo que trae la vuelta del navegador: el mismo state y un
// código, o el error que da el proveedor (la persona dijo que no).
func validarVuelta(v url.Values, estado string) error {
	if e := v.Get("error"); e != "" && v.Get("state") == estado {
		return fmt.Errorf("nube: el proveedor no ha dado permiso (%s)", e)
	}
	if v.Get("state") != estado || v.Get("code") == "" {
		return errors.New("nube: respuesta de autorización no válida")
	}
	return nil
}

// Conectar abre el navegador del sistema con abrir, espera la vuelta y cambia el código
// por tokens. La vuelta es a 127.0.0.1 en el escritorio y por el esquema de la app en el
// móvil, si la cáscara la ha puesto ([UsarVuelta]; o.Vuelta manda si viene).
func (o OAuth) Conectar(ctx context.Context, hc *http.Client, abrir func(string) error) (Tokens, error) {
	if o.Cliente == "" {
		return Tokens{}, errors.New("nube: falta el identificador de cliente OAuth")
	}
	verificador, estado := aleatorio(), aleatorio()
	reto := sha256.Sum256([]byte(verificador))
	if v := cmp.Or(o.Vuelta, VueltaDeLaApp()); v != nil {
		q, err := v.esperar(ctx, estado, func() error { return abrir(o.urlAutorizar(v.Redireccion, estado, reto)) })
		if err != nil {
			return Tokens{}, err
		}
		if err := validarVuelta(q, estado); err != nil {
			return Tokens{}, err
		}
		return o.pedir(ctx, hc, url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {v.Redireccion}, "code_verifier": {verificador}})
	}
	var redireccion string
	q, err := autorizacion.WaitLoopback(ctx, autorizacion.LoopbackCallback{
		Address: "127.0.0.1:0",
		Path:    "/oauth",
		Open: func(r string) error {
			redireccion = r
			return abrir(o.urlAutorizar(r, estado, reto))
		},
		Validate: func(v url.Values) error {
			if v.Get("state") != estado || v.Get("code") == "" {
				return errors.New("nube: respuesta de autorización no válida")
			}
			return nil
		},
		Success: "Cuenta conectada. Ya puedes volver a Sytue Office.",
		Failure: "No se pudo conectar la cuenta.",
	})
	if err != nil {
		return Tokens{}, err
	}
	return o.pedir(ctx, hc, url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")}, "redirect_uri": {redireccion}, "code_verifier": {verificador}})
}

// Renovar cambia el token de refresco por uno de acceso.
func (o OAuth) Renovar(ctx context.Context, hc *http.Client, refresco string) (Tokens, error) {
	return o.pedir(ctx, hc, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresco}})
}

func (o OAuth) pedir(ctx context.Context, hc *http.Client, v url.Values) (Tokens, error) {
	v.Set("client_id", o.Cliente)
	if o.Secreto != "" {
		v.Set("client_secret", o.Secreto)
	}
	if v.Get("grant_type") == "refresh_token" && len(o.Alcances) > 0 {
		v.Set("scope", strings.Join(o.Alcances, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Token, strings.NewReader(v.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return Tokens{}, fmt.Errorf("nube: el proveedor rechazó la autorización (%d); vuelve a conectar la cuenta", resp.StatusCode)
	}
	var t Tokens
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t); err != nil {
		return Tokens{}, err
	}
	if t.Acceso == "" {
		return Tokens{}, errors.New("nube: el proveedor no devolvió token")
	}
	return t, nil
}

// ConectarOAuth conecta una cuenta de Google o de OneDrive y la guarda.
func (r *Registro) ConectarOAuth(ctx context.Context, tipo, cliente string, abrir func(string) error) (Cuenta, error) {
	var o OAuth
	switch tipo {
	case Google:
		if cliente == "" {
			cliente = r.Clientes.Google
		}
		o = OAuthGoogle(cliente, r.Clientes.GoogleSecreto)
	case OneDrive:
		if cliente == "" {
			cliente = r.Clientes.Microsoft
		}
		o = OAuthMicrosoft(cliente)
	default:
		return Cuenta{}, fmt.Errorf("nube: %q no usa OAuth", tipo)
	}
	t, err := o.Conectar(ctx, r.http(), abrir)
	if err != nil {
		return Cuenta{}, err
	}
	if t.Refresco == "" {
		return Cuenta{}, errors.New("nube: el proveedor no dio acceso sin conexión (falta el token de refresco)")
	}
	return r.Añadir(Cuenta{Tipo: tipo, ClienteID: cliente}, t.Refresco)
}
