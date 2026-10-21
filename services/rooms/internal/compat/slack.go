// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Dialecto SLACK. Entrante: el payload de un «Incoming Webhook» (text +
// attachments + Block Kit), en JSON o form-urlencoded con payload={json}.
// Saliente: el POST form-urlencoded de un «Outgoing Webhook» y su respuesta.
//
// El Markdown de Rooms (ver MessageBody del front) soporta **negrita**, _cursiva_,
// ~~tachado~~, `código`, bloques ```, encabezados # y AUTOLINKS (no [texto](url)):
// por eso un enlace mrkdwn se rinde como «texto (url)» y no como enlace Markdown.

// slackPayload es la vista que nos interesa del cuerpo de Slack. Los campos
// desconocidos se ignoran (degradar, nunca fallar).
//
// SEGURIDAD: `channel` NO se declara a propósito. Slack permite redirigir el
// mensaje a otro canal con ese campo; en Rooms el canal lo fija el webhook (el
// token es la única autorización), así que se ignora explícitamente — declararlo
// invitaría a usarlo. Lo mismo con `username`/`icon_*`: la identidad del mensaje es
// la del webhook, no la que diga el emisor.
type slackPayload struct {
	Text        string            `json:"text"`
	Attachments []slackAttachment `json:"attachments"`
	Blocks      []slackBlock      `json:"blocks"`
}

type slackAttachment struct {
	Fallback  string       `json:"fallback"`
	Pretext   string       `json:"pretext"`
	Title     string       `json:"title"`
	TitleLink string       `json:"title_link"`
	Text      string       `json:"text"`
	Fields    []slackField `json:"fields"`
	Footer    string       `json:"footer"`
	Blocks    []slackBlock `json:"blocks"`
	// ponytail: `color` se ignora (Rooms no pinta barra lateral por adjunto).
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	// ponytail: `short` se ignora — el mensaje es texto, no hay columnas.
}

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text"`
	Fields   []slackText `json:"fields"`
	Elements []slackText `json:"elements"`
}

// slackText es el «text object» de Block Kit: type plain_text|mrkdwn. AltText cubre
// los elementos image de un bloque context (su único texto legible).
type slackText struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	AltText string `json:"alt_text"`
}

// render devuelve el texto del objeto ya convertido a Markdown de Rooms.
func (t slackText) render() string {
	s := t.Text
	if s == "" {
		s = t.AltText
	}
	if t.Type == "plain_text" {
		return unescapeSlack(s)
	}
	return Mrkdwn(s)
}

// translateSlack traduce el cuerpo de un webhook entrante de Slack. Acepta JSON o
// form-urlencoded (payload={json}, o text=… de los comandos de barra).
func translateSlack(contentType string, body []byte) (string, error) {
	raw := body
	if isForm(contentType) {
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return "", ErrUnsupported
		}
		p := strings.TrimSpace(vals.Get("payload"))
		if p == "" {
			// Sin payload: un POST estilo comando de barra ⇒ su `text` tal cual.
			if t := strings.TrimSpace(vals.Get("text")); t != "" {
				return Mrkdwn(t), nil
			}
			return "", ErrUnsupported
		}
		raw = []byte(p)
	}
	return renderSlackJSON(raw)
}

// renderSlackJSON rinde un payload de Slack ya en JSON.
func renderSlackJSON(raw []byte) (string, error) {
	var p slackPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", ErrUnsupported
	}
	var lines []string
	if p.Text != "" {
		lines = append(lines, Mrkdwn(p.Text))
	}
	lines = append(lines, renderSlackBlocks(p.Blocks)...)
	for _, a := range p.Attachments {
		lines = append(lines, "")
		lines = append(lines, renderSlackAttachment(a)...)
	}
	out := joinLines(lines)
	if strings.TrimSpace(out) == "" {
		return "", ErrUnsupported // nada legible: 400, nunca un mensaje vacío
	}
	return out, nil
}

// renderSlackAttachment rinde un attachment clásico (pretext, título con enlace,
// texto, campos, bloques y pie). Si no hay nada, cae al `fallback`.
func renderSlackAttachment(a slackAttachment) []string {
	var lines []string
	if a.Pretext != "" {
		lines = append(lines, Mrkdwn(a.Pretext))
	}
	switch {
	case a.Title != "" && a.TitleLink != "":
		lines = append(lines, "**"+unescapeSlack(a.Title)+"** ("+a.TitleLink+")")
	case a.Title != "":
		lines = append(lines, "**"+unescapeSlack(a.Title)+"**")
	}
	if a.Text != "" {
		lines = append(lines, Mrkdwn(a.Text))
	}
	lines = append(lines, renderSlackBlocks(a.Blocks)...)
	for _, f := range a.Fields {
		lines = append(lines, renderField(f.Title, f.Value))
	}
	if a.Footer != "" {
		lines = append(lines, "_"+Mrkdwn(a.Footer)+"_")
	}
	if len(lines) == 0 && a.Fallback != "" {
		lines = append(lines, Mrkdwn(a.Fallback))
	}
	return lines
}

// renderSlackBlocks rinde Block Kit. Cubre section, header, divider y context; lo
// demás degrada a su `text` si lo trae, y si no se omite (nunca error).
//
//nolint:gocognit // un case por tipo de bloque de Block Kit: la tabla de traducción se lee mejor junta que troceada.
func renderSlackBlocks(blocks []slackBlock) []string {
	var lines []string
	for _, b := range blocks {
		switch strings.ToLower(b.Type) {
		case "header":
			if b.Text != nil {
				lines = append(lines, "## "+b.Text.render())
			}
		case "divider":
			lines = append(lines, "---")
		case "context":
			var parts []string
			for _, e := range b.Elements {
				if s := e.render(); s != "" {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				lines = append(lines, "_"+strings.Join(parts, " · ")+"_")
			}
		case "section":
			if b.Text != nil && b.Text.Text != "" {
				lines = append(lines, b.Text.render())
			}
			for _, f := range b.Fields {
				if s := f.render(); s != "" {
					lines = append(lines, s)
				}
			}
		default:
			// actions, image, input, rich_text…: sin equivalente ⇒ su texto si lo hay.
			if b.Text != nil {
				if s := b.Text.render(); s != "" {
					lines = append(lines, s)
				}
			}
		}
	}
	return lines
}

// renderField rinde un par nombre/valor (attachment fields, facts de Teams).
func renderField(title, value string) string {
	title, value = strings.TrimSpace(title), strings.TrimSpace(value)
	switch {
	case title == "":
		return Mrkdwn(value)
	case value == "":
		return "**" + unescapeSlack(title) + "**"
	default:
		return "**" + unescapeSlack(title) + ":** " + Mrkdwn(value)
	}
}

// ---------------------------------------------------------------------------
// mrkdwn → Markdown de Rooms
// ---------------------------------------------------------------------------

var (
	// <url|texto>, <@U123|nombre>, <#C123|canal>, <!here>, <mailto:a@b|a@b>.
	angleRe = regexp.MustCompile(`<([^<>\n]*)>`)
	// *negrita* (y **negrita** ya en Markdown) ⇒ **negrita**. El contenido no puede
	// empezar/terminar en espacio: así «2 * 3 * 4» no se convierte en negrita.
	boldRe = regexp.MustCompile(`\*\*?([^\s*][^*\n]*?[^\s*]|[^\s*])\*\*?`)
	// ~tachado~ (y ~~tachado~~) ⇒ ~~tachado~~.
	strikeRe = regexp.MustCompile(`~~?([^\s~][^~\n]*?[^\s~]|[^\s~])~~?`)
)

// Mrkdwn convierte el mrkdwn de Slack al Markdown ligero de Rooms, SIN tocar el
// contenido de `código` ni de los bloques ```.
func Mrkdwn(s string) string {
	var out strings.Builder
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			out.WriteString(convertMrkdwn(s))
			break
		}
		out.WriteString(convertMrkdwn(s[:i]))
		delim := "`"
		if strings.HasPrefix(s[i:], "```") {
			delim = "```"
		}
		rest := s[i+len(delim):]
		j := strings.Index(rest, delim)
		if j < 0 {
			out.WriteString(s[i:]) // delimitador sin cerrar: literal
			break
		}
		out.WriteString(delim + rest[:j] + delim)
		s = rest[j+len(delim):]
	}
	return out.String()
}

// convertMrkdwn aplica las conversiones a un fragmento SIN código.
func convertMrkdwn(s string) string {
	if s == "" {
		return ""
	}
	s = angleRe.ReplaceAllStringFunc(s, convertAngle)
	s = unescapeSlack(s)
	s = boldRe.ReplaceAllString(s, "**$1**")
	s = strikeRe.ReplaceAllString(s, "~~$1~~")
	return s // _cursiva_ ya es la sintaxis de Rooms: sin cambio
}

// convertAngle traduce un token <…> de Slack. Los enlaces se rinden «texto (url)»
// porque Rooms autolinka las URLs pero no soporta [texto](url).
func convertAngle(m string) string {
	inner := m[1 : len(m)-1]
	label := ""
	if i := strings.IndexByte(inner, '|'); i >= 0 {
		label, inner = inner[i+1:], inner[:i]
	}
	switch {
	case inner == "":
		return ""
	case strings.HasPrefix(inner, "@"): // usuario
		if label != "" {
			return "@" + label
		}
		return inner
	case strings.HasPrefix(inner, "#"): // canal (<#C123|general>)
		if label != "" {
			return "#" + label
		}
		return inner
	case strings.HasPrefix(inner, "!"): // <!here>, <!channel>, <!everyone>
		return "@" + strings.TrimPrefix(inner, "!")
	case strings.HasPrefix(inner, "mailto:"):
		addr := strings.TrimPrefix(inner, "mailto:")
		if label != "" && label != addr {
			return label + " (" + addr + ")"
		}
		return addr
	default:
		if label != "" && label != inner {
			return label + " (" + inner + ")"
		}
		return inner
	}
}

// unescapeSlack deshace el escapado HTML de Slack (&amp;/&lt;/&gt;). Va DESPUÉS de
// tratar los <…> para que un enlace escapado siga siendo texto literal.
func unescapeSlack(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(s)
}

// ---------------------------------------------------------------------------
// Saliente en dialecto Slack
// ---------------------------------------------------------------------------

// SlackOutgoing son los campos del POST form-urlencoded de un «Outgoing Webhook»
// de Slack, tal cual los espera un bot escrito para Slack.
type SlackOutgoing struct {
	Token       string
	TeamDomain  string
	ChannelID   string
	ChannelName string
	UserID      string
	UserName    string
	Text        string
	TriggerWord string
	Timestamp   int64
}

// Form serializa los campos como el cuerpo form-urlencoded que envía Slack.
func (o SlackOutgoing) Form() string {
	v := url.Values{}
	v.Set("token", o.Token)
	v.Set("team_id", o.TeamDomain)
	v.Set("team_domain", o.TeamDomain)
	v.Set("channel_id", o.ChannelID)
	v.Set("channel_name", o.ChannelName)
	v.Set("user_id", o.UserID)
	v.Set("user_name", o.UserName)
	v.Set("text", o.Text)
	v.Set("trigger_word", o.TriggerWord)
	v.Set("timestamp", strconv.FormatInt(o.Timestamp, 10))
	return v.Encode()
}

// ParseSlackReply interpreta la respuesta de un bot de Slack ({"text":…,
// "response_type":"in_channel"|"ephemeral"}, con attachments/blocks opcionales) y
// devuelve el texto a publicar; vacío si no hay nada que publicar.
//
// En un webhook SALIENTE se publica siempre, sea cual sea el `response_type`: la
// respuesta llega por el outbox, cuando ya no hay nadie esperándola, así que no hay a
// quién enseñársela en privado y tragársela sería perderla. Un slash command sí tiene
// destinatario —quien lo ejecutó— y por eso consulta SlackResponseType.
func ParseSlackReply(raw []byte) string {
	text, err := renderSlackJSON(raw)
	if err != nil {
		return ""
	}
	return text
}

// SlackResponseType devuelve el `response_type` declarado por el bot, o "" si no lo
// declara. Quien llama decide qué hacer con él.
func SlackResponseType(raw []byte) string {
	var obj struct {
		ResponseType string `json:"response_type"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return obj.ResponseType
}
