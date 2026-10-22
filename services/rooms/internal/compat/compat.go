// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compat traduce los dialectos de webhook de Slack, Microsoft Teams y Google Chat al
// mensaje NATIVO de Sytue Rooms (y de vuelta, para los salientes). Todo lo de aquí
// es PURO: entra un []byte y sale texto — sin red, sin BD, sin contexto. Así el
// endpoint público /hooks/{token} acepta, pegando la URL y sin cambiar nada en el
// emisor, cualquier herramienta que ya sepa hablar con «un webhook de Slack» o «un
// webhook de Teams».
//
// SEGURIDAD: la traducción NO decide destino ni autorización. El token de la ruta
// sigue siendo la única autorización y el canal lo fija el webhook; campos del
// payload como `channel` de Slack se IGNORAN a propósito (ver slack.go).
package compat

import (
	"encoding/json"
	"errors"
	"mime"
	"strings"
)

// Dialectos soportados. Son los valores que también persisten en la columna
// `dialect` de outgoing_webhooks (sytue = comportamiento histórico).
const (
	Native     = "sytue"
	Slack      = "slack"
	Teams      = "teams"
	GoogleChat = "googlechat"
)

// ErrUnsupported indica que el cuerpo no se pudo interpretar en el dialecto dado.
// El handler lo traduce a 400 (nunca 500: la basura degrada, no rompe).
var ErrUnsupported = errors.New("compat: cuerpo no interpretable")

// Detect deduce el dialecto del cuerpo por su FORMA, sin heurísticas frágiles:
//   - form-urlencoded ⇒ Slack (solo Slack postea `payload={json}` así);
//   - claves propias de Teams (@type/@context de MessageCard, themeColor, sections,
//     o el envoltorio {"type":"message"} de Adaptive Card) ⇒ Teams;
//   - claves propias de Slack (attachments, blocks, channel, username, icon_*, …) ⇒ Slack;
//   - cualquier otra cosa (incluido {"text":"…"} a secas) ⇒ Native.
//
// El sesgo es deliberado: el payload nativo de hoy —{"text":"…"}— sigue cayendo en
// Native y se procesa EXACTAMENTE igual que antes (cero regresión).
func Detect(contentType string, body []byte) string {
	if isForm(contentType) {
		return Slack
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || len(obj) == 0 {
		return Native // que falle (o pase) por el camino nativo de siempre
	}
	if _, ok := obj["@type"]; ok {
		return Teams
	}
	if _, ok := obj["@context"]; ok {
		return Teams
	}
	if _, ok := obj["themeColor"]; ok {
		return Teams
	}
	if _, ok := obj["sections"]; ok {
		return Teams
	}
	switch strings.ToLower(stringOf(obj["type"])) {
	case "message", "adaptivecard":
		return Teams
	}
	// Las tarjetas de Google Chat tienen nombre propio; un {"text"} a secas sigue
	// cayendo en Native, que lo procesa igual (el formato de texto coincide).
	if _, ok := obj["cardsV2"]; ok {
		return GoogleChat
	}
	if _, ok := obj["cards"]; ok {
		return GoogleChat
	}
	for k := range obj {
		switch k {
		case "attachments", "blocks", "channel", "username", "icon_emoji", "icon_url",
			"mrkdwn", "unfurl_links", "unfurl_media", "thread_ts", "response_type",
			"link_names", "as_user", "parse":
			return Slack
		}
	}
	return Native
}

// Translate convierte el cuerpo de un dialecto NO nativo al texto del mensaje de
// Rooms. El dialecto Native no pasa por aquí: lo decodifica el handler con su
// validación de siempre.
func Translate(dialect, contentType string, body []byte) (string, error) {
	switch dialect {
	case Slack:
		return translateSlack(contentType, body)
	case Teams:
		return translateTeams(body)
	case GoogleChat:
		return translateGoogleChat(body)
	default:
		return "", ErrUnsupported
	}
}

// Ack devuelve el cuerpo EXACTO que el cliente de cada dialecto espera ver en un
// 200: Slack comprueba literalmente `ok` y Teams `1`; muchos emisores marcan la
// entrega como fallida si no lo reciben. Native devuelve ("","") ⇒ el handler
// responde su JSON de siempre.
func Ack(dialect string) (contentType, body string) {
	switch dialect {
	case Slack:
		return "text/plain; charset=utf-8", "ok"
	case Teams:
		return "text/plain; charset=utf-8", "1"
	case GoogleChat:
		// Google devuelve el mensaje creado; a un emisor le basta un JSON.
		return "application/json", "{}"
	default:
		return "", ""
	}
}

// NormalizeOutgoing valida el dialecto de un webhook SALIENTE: nativo, el
// form-urlencoded de Slack o la actividad de Teams. Vacío ⇒ Native (compatibilidad
// total con los salientes ya creados). El segundo valor es false si no existe.
func NormalizeOutgoing(d string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case "", Native:
		return Native, true
	case Slack:
		return Slack, true
	case Teams:
		return Teams, true
	}
	return "", false
}

// isForm indica si el Content-Type es application/x-www-form-urlencoded.
func isForm(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/x-www-form-urlencoded"
}

// stringOf extrae un string de un json.RawMessage (vacío si no lo es).
func stringOf(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// joinLines une las líneas de un render descartando las vacías del principio y del
// final, y colapsando los huecos de más de una línea en blanco.
func joinLines(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue // sin blancos iniciales ni dobles
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
