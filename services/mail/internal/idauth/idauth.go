// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package idauth adapta la autenticación SMTP/IMAP (AUTH PLAIN / LOGIN) a Sytue
// ID: el secreto presentado es un token de acceso OAuth2 emitido por Sytue ID
// (o una "contraseña de aplicación" que es un JWT de larga duración con scope de
// correo). Se valida vía el verificador JWKS de platform/pkg/authn; nunca se
// almacenan contraseñas en Sytue Mail.
//
// Esto evita un segundo almacén de credenciales (deny-by-default, mínimo
// privilegio) y mantiene a Sytue ID como pieza nuclear de identidad.
package idauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// Verifier valida credenciales de correo (dirección + token) contra Sytue ID.
type Verifier struct {
	v *authn.Verifier
	// appPass consulta las contraseñas de aplicación en Sytue ID. Nil ⇒ solo se
	// aceptan JWT, que es como estaba: ningún cliente de correo podía entrar.
	appPass *appPassClient
}

// New construye el verificador a partir del verificador JWKS de platform.
func New(v *authn.Verifier) *Verifier { return &Verifier{v: v} }

// WithAppPasswords activa el segundo camino de autenticación: las contraseñas de
// aplicación que la persona crea en /account de Sytue ID. Sin esto el AUTH solo
// admite un JWT con scope `mail`, que dura minutos y que nadie puede teclear en
// Thunderbird — o sea, el correo no se puede usar desde ningún cliente.
//
// URL o secreto vacíos ⇒ no se activa (fail-closed, como estaba).
func (a *Verifier) WithAppPasswords(idURL, secret string) *Verifier {
	if strings.TrimSpace(idURL) == "" || secret == "" {
		return a
	}
	a.appPass = &appPassClient{
		baseURL: strings.TrimRight(idURL, "/"),
		secret:  secret,
		// Camino caliente: un cliente IMAP autentica en cada conexión y abre varias.
		// ponytail: sin caché; añadir un TTL corto por hash si la latencia molesta.
		http: &http.Client{Timeout: 5 * time.Second},
	}
	return a
}

// appPassClient pregunta a Sytue ID de quién es una contraseña de aplicación. Las
// credenciales viven allí —son de la identidad, no del correo— y este servicio no
// guarda copia: un segundo almacén sería un segundo sitio del que filtrarlas.
type appPassClient struct {
	baseURL string
	secret  string
	http    *http.Client
}

type appPassResp struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
	Email   string `json:"email"`
	Mailbox string `json:"mailbox"`
}

func (c *appPassClient) verify(ctx context.Context, identity, password string) (appPassResp, error) {
	var out appPassResp
	cuerpo, err := json.Marshal(map[string]string{"identity": identity, "password": password})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/app-passwords/verify", bytes.NewReader(cuerpo))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return out, fmt.Errorf("idauth: consulta a Sytue ID: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("idauth: credenciales rechazadas por Sytue ID (%d)", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("idauth: respuesta ilegible de Sytue ID: %w", err)
	}
	if out.Subject == "" || out.Tenant == "" {
		return out, fmt.Errorf("idauth: respuesta incompleta de Sytue ID")
	}
	return out, nil
}

// Authenticate valida que `secret` es un JWT válido de Sytue ID cuyo subject/
// email corresponde a `identity` (la dirección de correo presentada). Devuelve
// subject, tenant y la dirección normalizada para fijar el contexto de la sesión.
//
// Implementa la interfaz esperada por internal/smtp.TokenVerifier e
// internal/imap.Authenticator.
func (a *Verifier) Authenticate(ctx context.Context, identity, secret string) (subject, tenant, address string, err error) {
	claims, err := a.v.Verify(ctx, secret)
	if err != nil {
		// No era un JWT (o caducó). Antes de rendirse: una contraseña de aplicación,
		// que es lo ÚNICO que una persona puede escribir en un cliente de correo.
		// El orden es este y no el contrario porque el camino del JWT es local (no
		// sale de la máquina) y es el que usan los servicios de la suite.
		if a.appPass == nil {
			return "", "", "", fmt.Errorf("idauth: token inválido: %w", err)
		}
		r, aperr := a.appPass.verify(ctx, identity, secret)
		if aperr != nil {
			return "", "", "", fmt.Errorf("idauth: credenciales inválidas: %w", aperr)
		}
		addr := r.Mailbox
		if addr == "" {
			addr = r.Email
		}
		if addr == "" {
			addr = identity
		}
		return r.Subject, r.Tenant, strings.ToLower(addr), nil
	}
	// El scope de correo es obligatorio para AUTH de correo (mínimo privilegio).
	if !claims.HasScope("mail") && !claims.HasScope("urn:ietf:params:jmap:mail") {
		return "", "", "", fmt.Errorf("idauth: token sin scope de correo")
	}
	if err := identidadDelToken(identity, claims); err != nil {
		return "", "", "", err
	}
	t, ok := claims.TenantOrEmpty()
	if !ok {
		return "", "", "", fmt.Errorf("idauth: token sin tenant válido")
	}
	// La cuenta se resuelve SIEMPRE por la dirección del buzón, se haya presentado
	// esa o la de registro: si no, entrar con una u otra abriría dos cuentas
	// distintas para la misma persona y el correo aparecería partido en dos.
	addr := claims.MailAddress()
	if addr == "" {
		addr = identity
	}
	return claims.Subject, t.Slug, strings.ToLower(addr), nil
}

// coincide indica si la identidad presentada es alguna de las direcciones que el
// token acredita. Las vacías no cuentan: un token sin direcciones no puede
// autorizar cualquier identidad que se presente.
func coincide(identity string, direcciones ...string) bool {
	for _, d := range direcciones {
		if d != "" && strings.EqualFold(identity, d) {
			return true
		}
	}
	return false
}

// identidadDelToken decide si un JWT acredita la identidad presentada (anti-spoof).
// Se aceptan las DOS direcciones del token —el buzón de la organización y el correo
// de registro— porque quien configura Thunderbird o el móvil escribe la que conoce.
//
// Un token SIN ninguna de las dos se rechaza (COM-14). Antes se aceptaba «como las
// identidades de servicio», y entonces la sesión tomaba la dirección que dijera el
// cliente. Pero un token de servicio (client_credentials) no lleva organización y
// ya lo rechaza Authenticate; uno de persona sin dirección es uno pedido sin el
// scope `email`, y no puede acreditar cualquier dirección.
func identidadDelToken(identity string, claims authn.Claims) error {
	if claims.Mailbox == "" && claims.Email == "" {
		return fmt.Errorf("idauth: el token no acredita ninguna dirección")
	}
	if identity != "" && !coincide(identity, claims.Mailbox, claims.Email) {
		return fmt.Errorf("idauth: la identidad no coincide con el token")
	}
	return nil
}
