// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// ADAPTADOR INVERSO: el MISMO driver de Microsoft Graph apuntado a la PROPIA suite.
//
// Sytue Drive habla el dialecto de Graph en /compat/graph (v1.0/me/drive/...), y lo
// que devuelve es una colección OData idéntica en forma a la de Microsoft: `value` +
// `@odata.nextLink`. Por eso el conector no necesita un driver nuevo: basta apuntar
// `graph_base` a esos endpoints y que el token sea el que Sytue entiende.
//
// LA ÚNICA DIFERENCIA REAL ES LA AUTENTICACIÓN, y es irreducible:
//
//	Microsoft 365 → OAuth2 client-credentials contra Entra: un token de APLICACIÓN,
//	  con permisos de aplicación sobre todo el inquilino.
//	Sytue         → Sytue ID: /compat/graph es /me (el drive de UNA persona), así que
//	  un token de aplicación no vale; hace falta uno acotado a esa persona. Se obtiene
//	  con el canje RFC 8693 del client DELEGABLE de la organización (el mismo mecanismo
//	  con el que la migración escribe en la cuenta de cada usuario): client_credentials
//	  para probar quién pide, y token-exchange con requested_subject para actuar en su
//	  nombre. Cada canje deja traza en el audit_log de Sytue ID.
//
// Qué activa este camino: la presencia de `token_url` en la config de la instancia. Lo
// inyecta el SERVICIO desde su propia configuración (ID_URL/DRIVE_URL) al despachar una
// instancia de la plantilla «sytue» — nunca lo teclea un administrador, que es lo que
// permite tratar ese destino como interno de la suite (ver policy.RuntimeParams.Internal).
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// Claves de Instance.Config del camino Sytue. Las inyecta el gateway al despachar; un
// administrador no las escribe (ver el comentario del paquete).
const (
	// ConfigTokenURL es el token endpoint VERBATIM. Presente ⇒ el destino es la propia
	// suite y el token se mintea contra Sytue ID en vez de componer la URL de Entra.
	//nolint:gosec // G101: es el nombre de una clave de configuración, no una credencial.
	ConfigTokenURL = "token_url"
	// ConfigSubject es la persona a la que representa el token (correo o uuid de Sytue).
	ConfigSubject = "subject"
	// ConfigAudience es el producto de la suite al que va dirigido (p. ej. sytue-drive):
	// su verificador solo acepta tokens con su audiencia.
	ConfigAudience = "audience"
)

// Grants y tipos de token del canje (RFC 8693 §2.1/§3), tal como los acepta Sytue ID.
const (
	//nolint:gosec // G101: URN público del estándar, no una credencial.
	grantTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	//nolint:gosec // G101: URN público del estándar, no una credencial.
	tokenTypeAccess = "urn:ietf:params:oauth:token-type:access_token"
)

// isSytue indica si la instancia apunta a la propia suite (config inyectada por el
// gateway) en vez de a Microsoft 365.
func isSytue(inst connector.Instance) bool {
	return strings.TrimSpace(inst.Config[ConfigTokenURL]) != ""
}

// sytueToken mintea el token con el que se lee /compat/graph: canje RFC 8693 acotado a
// la persona de la instancia. Dos POST por consulta (probar quién pide + canjear), sin
// caché: el token de impersonación dura 5 minutos y una consulta tabular es un instante.
//
// ponytail: sin caché de token, igual que el camino de Entra; si el volumen de consultas
// aprieta, cachear por org:instance:subject con el TTL de expires_in.
func sytueToken(ctx context.Context, rt connector.Runtime, inst connector.Instance) (string, error) {
	tokenURL := strings.TrimSpace(inst.Config[ConfigTokenURL])
	clientID := strings.TrimSpace(inst.Config[ConfigClientID])
	subject := strings.TrimSpace(inst.Config[ConfigSubject])
	secret := inst.Secrets[secretClientSecret]
	if clientID == "" || secret == "" {
		return "", fmt.Errorf("%w: la organización no tiene un client delegable con el que actuar en nombre de las personas", connector.ErrBadQuery)
	}
	if subject == "" {
		return "", fmt.Errorf("%w: falta %q (la persona a la que representa el token)", connector.ErrBadQuery, ConfigSubject)
	}

	// (1) Token propio del client (client_credentials): es él quien presenta su identidad.
	own, err := postTokenForm(ctx, rt, tokenURL, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {secret},
	})
	if err != nil {
		return "", err
	}
	// (2) Canje a un token acotado a la persona, dirigido al producto de la suite.
	form := url.Values{
		"grant_type":         {grantTokenExchange},
		"client_id":          {clientID},
		"client_secret":      {secret},
		"subject_token":      {own},
		"subject_token_type": {tokenTypeAccess},
		"requested_subject":  {subject},
	}
	if aud := strings.TrimSpace(inst.Config[ConfigAudience]); aud != "" {
		form.Set("audience", aud)
	}
	return postTokenForm(ctx, rt, tokenURL, form)
}

// postTokenForm pide un token a un endpoint OAuth2 (Entra o Sytue ID: mismo contrato de
// formulario y misma forma de respuesta) por rt.HTTP. Fuente ÚNICA de esa llamada en el
// driver: el secreto viaja en el CUERPO, nunca en cabecera ni en la URL.
func postTokenForm(ctx context.Context, rt connector.Runtime, tokenURL string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Integration-Gateway/1")

	resp, err := rt.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("graph: token: %w", err)
	}
	body, err := readOK(resp)
	if err != nil {
		return "", fmt.Errorf("graph: token: %w", err)
	}
	var tr tokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("graph: la respuesta del token no es JSON: %w", err)
	}
	if tr.AccessToken == "" {
		if tr.Error != "" {
			return "", fmt.Errorf("graph: el token endpoint devolvió %s: %s", tr.Error, truncate(tr.ErrorDescription, 200))
		}
		return "", fmt.Errorf("graph: el token endpoint no devolvió access_token")
	}
	return tr.AccessToken, nil
}
