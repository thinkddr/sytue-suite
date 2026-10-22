// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compat traduce el modelo de contactos de Sytue (internal/domain) a los
// dialectos de la Google People API v1 y de Microsoft Graph (contacts).
//
// Todo lo de aquí es PURO: entran estructuras de dominio y salen estructuras
// serializables — sin red, sin base de datos, sin contexto. Los handlers finos que
// lo usan viven en internal/http/handlers_compat.go.
//
// PREFIJOS DE MONTAJE (no colisionan con la API nativa /api/v1 ni con /dav):
//
//	/compat/google → Google People API v1  (GET /v1/people/me/connections, /v1/people/{id})
//	/compat/graph  → Microsoft Graph v1.0  (GET /v1.0/me/contacts, /v1.0/me/contacts/{id})
//
// Es lo que el tercero pega como «base URL» de su integración: la emulación sirve
// justo donde el cliente deja configurarla (mismo criterio que services/drive).
//
// AUTORIZACIÓN: no vive aquí. Los handlers llaman al MISMO store owner-scoped que
// la API nativa con el contexto de la petición (Bearer de Sytue ID → tenant + sub),
// así que la verja es idéntica: un contacto ajeno responde 404, igual que uno
// inexistente. Un traductor nunca decide qué se ve.
//
// # Identificadores
//
// El id de un contacto es el MISMO que sirve la API nativa (/api/v1/contacts/{id}),
// así que `people/{id}` de Google y el `id` de Graph son intercambiables con él y
// pasan por la misma verja.
//
// # Parámetros
//
// Los que FILTRAN, ORDENAN o PAGINAN se aplican de verdad o se rechazan con 400
// nombrando el término (CheckParams): nunca un 200 con la lista filtrada a medias.
// Los que solo PROYECTAN campos (`personFields`, `$select`) se IGNORAN a propósito
// —las librerías siempre los mandan—: la respuesta lleva el contacto completo que
// Sytue modela, que es un superconjunto de lo pedido; lo que Sytue no modela (fotos
// binarias, biografías, pertenencias) no aparece.
package compat

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Errores de traducción. El handler los convierte en 400 con el sobre del dialecto
// correspondiente.
var (
	// ErrUnsupportedParam indica un parámetro de consulta fuera del subconjunto
	// que la emulación aplica de verdad.
	ErrUnsupportedParam = errors.New("compat: parámetro no soportado")
	// ErrInvalidPageToken indica un token de continuación que no generó esta API.
	ErrInvalidPageToken = errors.New("compat: pageToken inválido")
)

// MeAlias es el identificador que People acepta para «yo» (people/me/connections).
const MeAlias = "me"

// ignoredParams son los parámetros que la emulación ACEPTA y no cambian el
// resultado: los de sistema de Google y las PROYECCIONES de campos.
var ignoredParams = map[string]bool{
	"alt": true, "prettyPrint": true, "prettyprint": true, "key": true,
	"oauth_token": true, "access_token": true, "quotaUser": true, "userIp": true,
	"fields": true, "personFields": true, "$select": true,
}

// CheckParams valida la query contra los parámetros que el endpoint aplica DE
// VERDAD. Cualquier otro (salvo los de ignoredParams) es ErrUnsupportedParam con
// el nombre exacto.
func CheckParams(q url.Values, supported ...string) error {
	for k := range q {
		if ignoredParams[k] || slices.Contains(supported, k) {
			continue
		}
		return fmt.Errorf("%w: %q (soportados: %s)", ErrUnsupportedParam, k, strings.Join(supported, ", "))
	}
	return nil
}

// EncodePageToken serializa el desplazamiento de la página siguiente en un token
// opaco (base64url), que es como el dialecto lo trata: una caja negra.
func EncodePageToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("off:" + strconv.Itoa(offset)))
}

// DecodePageToken interpreta un token de continuación; "" es la primera página.
// Un token que no generó esta API es ErrInvalidPageToken (400), no una página 0
// silenciosa que repetiría resultados.
func DecodePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidPageToken
	}
	rest, ok := strings.CutPrefix(string(raw), "off:")
	if !ok {
		return 0, ErrInvalidPageToken
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return 0, ErrInvalidPageToken
	}
	return n, nil
}

// PageSize acota el tamaño de página pedido por el cliente: def si no viene o no
// es un entero positivo, y nunca por encima de maxSize.
func PageSize(raw string, def, maxSize int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return def
	}
	if n > maxSize {
		return maxSize
	}
	return n
}

// rfc3339 formatea un instante en UTC; "" para el cero, para que el campo se omita
// en vez de emitir una fecha inventada.
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
