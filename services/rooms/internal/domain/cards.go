// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Objetos ricos del chat: un mensaje normal puede llevar una TARJETA con estructura
// —una encuesta o una tarea— además de su texto. El texto sigue siendo el cuerpo del
// mensaje (la pregunta o el título), así que la búsqueda, el push y cualquier cliente
// que no entienda tarjetas siguen mostrando algo legible.
const (
	// CardPoll es una encuesta: pregunta (el cuerpo) + opciones votables.
	CardPoll = "poll"
	// CardTask es una tarea: título (el cuerpo) + responsable y fecha opcionales,
	// que se marca como hecha desde el propio mensaje.
	CardTask = "task"
)

// CardDone es el valor de estado de una tarea completada (ver CardVote.Value).
const CardDone = "done"

// Topes de una tarjeta (acotan el abuso y encajan en una tarjeta legible).
const (
	minPollOptions   = 2
	maxPollOptions   = 10
	maxCardOptionLen = 120
	maxAssigneeLen   = 200
)

// Card es la tarjeta estructurada de un mensaje. Kind vacío = mensaje sin tarjeta.
//
// Los campos de DEFINICIÓN (Kind/Options/Multi/Assignee/Due) se persisten tal cual en
// messages.card; Votes NO: es el estado agregado, vive en message_card_votes y se
// adjunta al leer (como las reacciones). Al persistir va vacío y `omitempty` lo deja
// fuera del JSON, así que no hay dos copias del mismo dato.
type Card struct {
	// Kind es "poll" o "task".
	Kind string `json:"kind,omitempty"`
	// Options son las respuestas de una encuesta, en orden. El voto referencia su
	// ÍNDICE ("0".."9"), no su texto: renumerar no rompe los votos ya emitidos.
	Options []string `json:"options,omitempty"`
	// Multi permite votar varias opciones a la vez (encuesta de casillas en vez de
	// radios). Con Multi=false, votar sustituye el voto anterior.
	Multi bool `json:"multi,omitempty"`
	// AssigneeSub/AssigneeName son el responsable de una tarea (opcional).
	AssigneeSub  string `json:"assignee_sub,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
	// Due es la fecha límite de una tarea (opcional).
	Due *time.Time `json:"due,omitempty"`
	// Votes es el estado agregado por valor: votos de cada opción de la encuesta o
	// el "done" de la tarea. Solo se rellena en LECTURA.
	Votes []CardVote `json:"votes,omitempty"`
}

// CardVote agrega quién marcó un valor de la tarjeta: la opción "0".."9" de una
// encuesta o "done" en una tarea. Los subjects viajan (como en las reacciones) para
// que el cliente sepa si el VISOR ya votó y pueda enseñar quién votó qué.
type CardVote struct {
	Value string   `json:"value"`
	Count int      `json:"count"`
	Subs  []string `json:"subs,omitempty"`
}

// HasCard indica si el mensaje lleva tarjeta.
func (c Card) HasCard() bool { return c.Kind != "" }

// Done indica si una tarea está completada (alguien marcó "done").
func (c Card) Done() bool {
	for _, v := range c.Votes {
		if v.Value == CardDone && v.Count > 0 {
			return true
		}
	}
	return false
}

// NormalizeCard sanea y valida la tarjeta que llega del cliente: recorta textos,
// comprueba los topes y DESCARTA el estado (Votes), que nunca lo fija quien publica.
// Una tarjeta vacía (Kind vacío) es válida y devuelve la tarjeta vacía: el mensaje
// simplemente no lleva tarjeta.
func NormalizeCard(c Card) (Card, error) {
	if !c.HasCard() {
		return Card{}, nil
	}
	out := Card{Kind: c.Kind, Multi: c.Multi, Due: c.Due}
	switch c.Kind {
	case CardPoll:
		for _, o := range c.Options {
			o = strings.TrimSpace(o)
			if o == "" || utf8.RuneCountInString(o) > maxCardOptionLen {
				return Card{}, ErrInvalidInput
			}
			out.Options = append(out.Options, o)
		}
		if len(out.Options) < minPollOptions || len(out.Options) > maxPollOptions {
			return Card{}, ErrInvalidInput
		}
		// Una encuesta no tiene responsable ni fecha: se ignoran en silencio en vez
		// de rechazar (un cliente que los mande de más no rompe el envío).
		out.Multi = c.Multi
		out.Due = nil
	case CardTask:
		out.AssigneeSub = strings.TrimSpace(c.AssigneeSub)
		out.AssigneeName = strings.TrimSpace(c.AssigneeName)
		if len(out.AssigneeSub) > maxAssigneeLen || utf8.RuneCountInString(out.AssigneeName) > maxAssigneeLen {
			return Card{}, ErrInvalidInput
		}
		out.Multi = false
	default:
		return Card{}, ErrInvalidInput
	}
	return out, nil
}

// ValidCardValue indica si `value` es un valor marcable de esta tarjeta: el índice de
// una opción existente en una encuesta, o "done" en una tarea. Es la verja que impide
// votar la opción 7 de una encuesta de tres o inventarse un estado en una tarea.
func ValidCardValue(c Card, value string) bool {
	switch c.Kind {
	case CardPoll:
		i, err := strconv.Atoi(value)
		return err == nil && i >= 0 && i < len(c.Options)
	case CardTask:
		return value == CardDone
	default:
		return false
	}
}
