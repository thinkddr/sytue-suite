// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compat traduce el modelo de calendario de Sytue (internal/domain) a los
// dialectos de la API de Google Calendar v3 y de Microsoft Graph (calendar), y de
// vuelta en lo único que llega del cliente: el cuerpo de events.insert y el
// `$filter` de Graph.
//
// Todo lo de aquí es PURO: entran estructuras de dominio o cadenas y salen
// estructuras serializables — sin red, sin base de datos, sin contexto. Los
// handlers finos que lo usan viven en internal/http/handlers_compat.go.
//
// PREFIJOS DE MONTAJE (no colisionan con la API nativa /api/v1 ni con /dav):
//
//	/compat/google → Google Calendar v3     (GET /calendar/v3/users/me/calendarList, …)
//	/compat/graph  → Microsoft Graph v1.0   (GET /v1.0/me/calendars, /v1.0/me/events, …)
//
// Es lo que el tercero pega como «base URL» de su integración: la emulación sirve
// justo donde el cliente deja configurarla (mismo criterio que services/drive).
//
// AUTORIZACIÓN: no vive aquí. Los handlers llaman al MISMO servicio que la API
// nativa con el contexto de la petición (Bearer de Sytue ID → tenant + verja de
// calendario), así que la verja es idéntica. Un traductor nunca decide qué se ve.
//
// # Recurrencias
//
// El servicio SÍ expande series (domain.ExpandEvents, el mismo motor que usan
// CalDAV y free/busy), así que `singleEvents=true` de Google devuelve OCURRENCIAS
// reales, con el id compuesto `{eventId}_{YYYYMMDDThhmmssZ}` que usa Google y con
// `recurringEventId`/`originalStartTime`.
//
// Sin `singleEvents=true` (el valor por defecto de Google) se devuelve la SERIE SIN
// EXPANDIR: el maestro con su `recurrence: ["RRULE:…"]`, que es exactamente lo que
// hace Google. Graph `/me/events` tampoco expande (eso es `calendarView`, que no se
// emula): los maestros salen con `type: "seriesMaster"`, pero SIN el objeto
// `recurrence` de Graph — traducir RRULE a `patternedRecurrence` con fidelidad no
// está implementado, y se prefiere omitir el campo antes que inventar un patrón.
//
// # Parámetros
//
// Los parámetros que FILTRAN, ORDENAN o PAGINAN se aplican de verdad o se rechazan
// con 400 nombrando el término (CheckParams): nunca un 200 con la página filtrada a
// medias. Los que solo PROYECTAN campos (`fields`, `$select`, `personFields`) se
// IGNORAN a propósito: la respuesta lleva siempre el recurso completo que Sytue
// modela, que es un superconjunto de lo pedido; lo que Sytue no modela no aparece.
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
// correspondiente (nunca en un 200 con resultados a medias).
var (
	// ErrUnsupportedParam indica un parámetro de consulta fuera del subconjunto
	// que la emulación aplica de verdad.
	ErrUnsupportedParam = errors.New("compat: parámetro no soportado")
	// ErrInvalidPageToken indica un token de continuación que no generó esta API.
	ErrInvalidPageToken = errors.New("compat: pageToken inválido")
	// ErrInvalidEvent indica un cuerpo de evento que la emulación no puede traducir.
	ErrInvalidEvent = errors.New("compat: evento no traducible")
	// ErrUnsupportedFilter indica un término de `$filter` fuera del subconjunto.
	ErrUnsupportedFilter = errors.New("compat: filtro no soportado en $filter")
)

// PrimaryAlias es el identificador que Google acepta para «mi calendario
// principal» (calendars/primary/events). Graph usa la ruta /me/events.
const PrimaryAlias = "primary"

// ignoredParams son los parámetros que la emulación ACEPTA y no cambian el
// resultado: los de sistema de Google (alt=json, prettyPrint, key…) y las
// PROYECCIONES de campos, que aquí siempre devuelven el recurso completo.
var ignoredParams = map[string]bool{
	"alt": true, "prettyPrint": true, "prettyprint": true, "key": true,
	"oauth_token": true, "access_token": true, "quotaUser": true, "userIp": true,
	"fields": true, "$select": true, "personFields": true,
	// timeZone solo cambia la zona en la que Google formatea las fechas de salida;
	// aquí se emiten con su offset (RFC 3339) o con su `timeZone`, que designan el
	// mismo instante sin ambigüedad.
	"timeZone": true,
}

// CheckParams valida la query contra los parámetros que el endpoint aplica DE
// VERDAD. Cualquier otro (salvo los de ignoredParams) es ErrUnsupportedParam con
// el nombre exacto: es preferible un 400 que el cliente ve a una lista filtrada a
// medias que daría por buena.
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
// opaco (base64url), que es como ambos dialectos lo tratan: una caja negra que el
// cliente devuelve tal cual.
func EncodePageToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("off:" + strconv.Itoa(offset)))
}

// DecodePageToken interpreta un token de continuación; "" es la primera página.
// Un token que no generó esta API es ErrInvalidPageToken (400), no una página 0
// silenciosa que devolvería resultados repetidos.
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

// Slice corta la página [offset, offset+size) de un resultado ya cargado y
// devuelve el desplazamiento siguiente (0 = no hay más páginas). La ventana
// temporal obligatoria de los listados acota el resultado completo, así que la
// paginación en memoria no crece sin límite.
func Slice[T any](items []T, offset, size int) ([]T, int) {
	if offset >= len(items) || size <= 0 {
		return nil, 0
	}
	if end := offset + size; end < len(items) {
		return items[offset:end], end
	}
	return items[offset:], 0
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

// parseTime acepta los formatos de fecha-hora que mandan los clientes de ambos
// dialectos: RFC 3339 con offset o sin zona (Graph), que se interpreta como UTC.
func parseTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
