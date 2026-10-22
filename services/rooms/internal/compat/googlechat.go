// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package compat

import (
	"encoding/json"
	"strings"
)

// Dialecto GOOGLE CHAT. Un «webhook entrante» de Google Chat recibe
// {"text": "…"} con el formato de texto de Chat (*negrita*, _cursiva_, ~tachado~,
// `código`, <url|texto>) —el mismo subconjunto que el mrkdwn de Slack, así que se
// traduce con la misma función— y, opcionalmente, tarjetas: `cardsV2` (la forma
// actual) o `cards` (la heredada). De una tarjeta se conserva lo legible: cabecera,
// títulos de sección, párrafos, textos decorados y pares clave-valor. Botones,
// imágenes y cuadrículas degradan a nada: nunca es un error.
type googleChatPayload struct {
	Text    string            `json:"text"`
	CardsV2 []googleChatCard2 `json:"cardsV2"`
	Cards   []googleChatCard  `json:"cards"`
}

type googleChatCard2 struct {
	Card googleChatCard `json:"card"`
}

type googleChatCard struct {
	Header   googleChatHeader    `json:"header"`
	Sections []googleChatSection `json:"sections"`
}

type googleChatHeader struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

type googleChatSection struct {
	Header  string             `json:"header"`
	Widgets []googleChatWidget `json:"widgets"`
}

type googleChatWidget struct {
	TextParagraph *googleChatText     `json:"textParagraph"`
	DecoratedText *googleChatDecorado `json:"decoratedText"`
	KeyValue      *googleChatKeyValue `json:"keyValue"`
}

type googleChatText struct {
	Text string `json:"text"`
}

type googleChatDecorado struct {
	TopLabel    string `json:"topLabel"`
	Text        string `json:"text"`
	BottomLabel string `json:"bottomLabel"`
}

type googleChatKeyValue struct {
	TopLabel string `json:"topLabel"`
	Content  string `json:"content"`
}

// translateGoogleChat convierte el cuerpo al texto del mensaje de Rooms.
func translateGoogleChat(body []byte) (string, error) {
	var p googleChatPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return "", ErrUnsupported
	}
	var partes []string
	if t := strings.TrimSpace(p.Text); t != "" {
		partes = append(partes, Mrkdwn(t))
	}
	tarjetas := p.Cards
	for _, c := range p.CardsV2 {
		tarjetas = append(tarjetas, c.Card)
	}
	for _, c := range tarjetas {
		if s := renderGoogleChatCard(c); s != "" {
			partes = append(partes, s)
		}
	}
	out := strings.TrimSpace(strings.Join(partes, "\n\n"))
	if out == "" {
		return "", ErrUnsupported
	}
	return out, nil
}

func renderGoogleChatCard(c googleChatCard) string {
	var l []string
	if t := strings.TrimSpace(c.Header.Title); t != "" {
		l = append(l, "**"+t+"**")
	}
	if t := strings.TrimSpace(c.Header.Subtitle); t != "" {
		l = append(l, t)
	}
	for _, s := range c.Sections {
		if h := strings.TrimSpace(s.Header); h != "" {
			l = append(l, "**"+h+"**")
		}
		for _, w := range s.Widgets {
			switch {
			case w.TextParagraph != nil:
				if t := strings.TrimSpace(w.TextParagraph.Text); t != "" {
					l = append(l, Mrkdwn(sinHTML(t)))
				}
			case w.DecoratedText != nil:
				l = append(l, parClaveValor(w.DecoratedText.TopLabel, w.DecoratedText.Text, w.DecoratedText.BottomLabel)...)
			case w.KeyValue != nil:
				l = append(l, parClaveValor(w.KeyValue.TopLabel, w.KeyValue.Content, "")...)
			}
		}
	}
	return strings.Join(l, "\n")
}

func parClaveValor(clave, valor, pie string) []string {
	clave, valor, pie = strings.TrimSpace(clave), strings.TrimSpace(sinHTML(valor)), strings.TrimSpace(pie)
	var out []string
	switch {
	case clave != "" && valor != "":
		out = append(out, "**"+clave+":** "+Mrkdwn(valor))
	case valor != "":
		out = append(out, Mrkdwn(valor))
	case clave != "":
		out = append(out, "**"+clave+"**")
	}
	if pie != "" {
		out = append(out, "_"+pie+"_")
	}
	return out
}

// sinHTML quita las etiquetas simples que Google Chat admite en los párrafos
// (<b>, <i>, <br>, <font>): Rooms pinta Markdown, no HTML, y una etiqueta suelta
// saldría literal en el mensaje.
func sinHTML(s string) string {
	r := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"<b>", "**", "</b>", "**", "<i>", "_", "</i>", "_", "<s>", "~~", "</s>", "~~",
		"<u>", "", "</u>", "",
	)
	s = r.Replace(s)
	// <font color="…"> y </font>: solo color, sin equivalente.
	for {
		i := strings.Index(s, "<font")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], ">")
		if j < 0 {
			break
		}
		s = s[:i] + s[i+j+1:]
	}
	return strings.ReplaceAll(s, "</font>", "")
}
