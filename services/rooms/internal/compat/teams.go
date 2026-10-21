// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Dialecto MICROSOFT TEAMS. Dos formatos conviven en la práctica:
//   - MessageCard legacy (Office 365 connector): {"@type":"MessageCard", title,
//     text, sections:[{activityTitle, facts:[…]}]} — lo que emite hoy la mayoría
//     de herramientas con «webhook de Teams»;
//   - el envoltorio Adaptive Card: {"type":"message","attachments":[{contentType:
//     "application/vnd.microsoft.card.adaptive", content:{body:[…]}}]}.
//
// Todo lo que no tenga equivalente (imágenes, acciones, columnas con estilo…)
// degrada a texto legible; nunca es un error.

type teamsPayload struct {
	// MessageCard
	Summary  string         `json:"summary"`
	Title    string         `json:"title"`
	Text     string         `json:"text"`
	Sections []teamsSection `json:"sections"`
	// Envoltorio Adaptive Card
	Attachments []teamsAttachment `json:"attachments"`
	// Adaptive Card «pelada» ({"type":"AdaptiveCard","body":[…]}).
	Body []adaptiveEl `json:"body"`
}

type teamsSection struct {
	ActivityTitle    string       `json:"activityTitle"`
	ActivitySubtitle string       `json:"activitySubtitle"`
	ActivityText     string       `json:"activityText"`
	Title            string       `json:"title"`
	Text             string       `json:"text"`
	Facts            []teamsFact  `json:"facts"`
	Body             []adaptiveEl `json:"body"`
}

type teamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Adaptive Card usa title/value en su FactSet.
	Title string `json:"title"`
}

type teamsAttachment struct {
	ContentType string       `json:"contentType"`
	Content     adaptiveCard `json:"content"`
}

type adaptiveCard struct {
	Body []adaptiveEl `json:"body"`
}

// adaptiveEl es un elemento del cuerpo de una Adaptive Card. La recursión
// (Items/Columns) cubre Container y ColumnSet sin más código.
type adaptiveEl struct {
	Type    string       `json:"type"`
	Text    string       `json:"text"`
	Size    string       `json:"size"`
	Weight  string       `json:"weight"`
	Facts   []teamsFact  `json:"facts"`
	Items   []adaptiveEl `json:"items"`
	Columns []struct {
		Items []adaptiveEl `json:"items"`
	} `json:"columns"`
}

var (
	// brRe: los MessageCard traen <br> como salto de línea (su `text` admite HTML
	// básico). ponytail: solo <br>; no hay sanitizador HTML porque el cuerpo del
	// mensaje se renderiza como texto/Markdown, no como HTML.
	brRe = regexp.MustCompile(`(?i)<br\s*/?>`)
	// Teams sí usa enlaces [texto](url); Rooms solo autolinka ⇒ «texto (url)».
	mdLinkRe = regexp.MustCompile(`\[([^\]\n]*)\]\((https?://[^\s)]+)\)`)
)

// translateTeams traduce un MessageCard o una Adaptive Card al texto de Rooms.
func translateTeams(body []byte) (string, error) {
	var p teamsPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return "", ErrUnsupported
	}
	var lines []string
	if p.Title != "" {
		lines = append(lines, "## "+teamsText(p.Title))
	}
	if p.Text != "" {
		lines = append(lines, teamsText(p.Text))
	}
	for _, s := range p.Sections {
		lines = append(lines, renderTeamsSection(s)...)
	}
	for _, a := range p.Attachments {
		lines = append(lines, renderAdaptive(a.Content.Body)...)
	}
	lines = append(lines, renderAdaptive(p.Body)...)
	if joinLines(lines) == "" && p.Summary != "" {
		lines = append(lines, teamsText(p.Summary)) // último recurso legible
	}
	out := joinLines(lines)
	if strings.TrimSpace(out) == "" {
		return "", ErrUnsupported
	}
	return out, nil
}

// renderTeamsSection rinde una sección de MessageCard (actividad + hechos + texto).
func renderTeamsSection(s teamsSection) []string {
	var lines []string
	if s.Title != "" {
		lines = append(lines, "**"+teamsText(s.Title)+"**")
	}
	if s.ActivityTitle != "" {
		lines = append(lines, "**"+teamsText(s.ActivityTitle)+"**")
	}
	if s.ActivitySubtitle != "" {
		lines = append(lines, "_"+teamsText(s.ActivitySubtitle)+"_")
	}
	if s.ActivityText != "" {
		lines = append(lines, teamsText(s.ActivityText))
	}
	if s.Text != "" {
		lines = append(lines, teamsText(s.Text))
	}
	for _, f := range s.Facts {
		lines = append(lines, renderField(factName(f), f.Value))
	}
	lines = append(lines, renderAdaptive(s.Body)...)
	return lines
}

// renderAdaptive rinde el cuerpo de una Adaptive Card: TextBlock y FactSet, con
// recursión por Container/ColumnSet. El resto se omite (degradación silenciosa).
func renderAdaptive(body []adaptiveEl) []string {
	var lines []string
	for _, el := range body {
		switch strings.ToLower(el.Type) {
		case "textblock":
			if el.Text == "" {
				continue
			}
			t := teamsText(el.Text)
			if strings.EqualFold(el.Weight, "bolder") || strings.EqualFold(el.Size, "large") || strings.EqualFold(el.Size, "extralarge") {
				t = "**" + t + "**"
			}
			lines = append(lines, t)
		case "factset":
			for _, f := range el.Facts {
				lines = append(lines, renderField(factName(f), f.Value))
			}
		default:
			// Container, ColumnSet y cualquier otro contenedor: bajar a sus hijos.
			lines = append(lines, renderAdaptive(el.Items)...)
			for _, c := range el.Columns {
				lines = append(lines, renderAdaptive(c.Items)...)
			}
			if len(el.Items) == 0 && len(el.Columns) == 0 && el.Text != "" {
				lines = append(lines, teamsText(el.Text))
			}
		}
	}
	return lines
}

// factName acepta el `name` del MessageCard o el `title` del FactSet.
func factName(f teamsFact) string {
	if f.Name != "" {
		return f.Name
	}
	return f.Title
}

// TeamsOutgoing es el mensaje que Rooms envía a un webhook saliente con dialecto
// Teams. Reproduce la ACTIVIDAD de Bot Framework que Teams entrega a un outgoing
// webhook: es lo que un bot escrito para Teams ya sabe leer.
type TeamsOutgoing struct {
	ID        string
	Text      string
	UserID    string
	UserName  string
	ChannelID string
	Tenant    string
	Timestamp time.Time
}

// JSON serializa la actividad. `channelId: "msteams"` y el `channelData.tenant` son
// los dos campos por los que un bot de Teams se orienta; omitirlos hace que descarte
// el mensaje.
func (o TeamsOutgoing) JSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"type":         "message",
		"id":           o.ID,
		"timestamp":    o.Timestamp.UTC().Format(time.RFC3339),
		"text":         o.Text,
		"channelId":    "msteams",
		"from":         map[string]string{"id": o.UserID, "name": o.UserName},
		"conversation": map[string]string{"id": o.ChannelID},
		"channelData":  map[string]any{"tenant": map[string]string{"id": o.Tenant}},
	})
}

// teamsText normaliza el texto de Teams: <br> ⇒ salto de línea y [texto](url) ⇒
// «texto (url)». El resto de su Markdown coincide con el de Rooms (**negrita**,
// _cursiva_, `código`).
func teamsText(s string) string {
	s = brRe.ReplaceAllString(s, "\n")
	s = mdLinkRe.ReplaceAllString(s, "$1 ($2)")
	return strings.TrimSpace(s)
}
