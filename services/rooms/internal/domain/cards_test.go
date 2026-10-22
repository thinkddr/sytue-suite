// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestNormalizeCard(t *testing.T) {
	t.Parallel()
	ahora := time.Now()

	casos := []struct {
		nombre string
		in     domain.Card
		quiere func(domain.Card) bool
		err    bool
	}{
		{
			nombre: "sin tarjeta es válido y no la inventa",
			in:     domain.Card{},
			quiere: func(c domain.Card) bool { return !c.HasCard() },
		},
		{
			nombre: "encuesta recorta las opciones",
			in:     domain.Card{Kind: domain.CardPoll, Options: []string{"  sí  ", "no"}},
			quiere: func(c domain.Card) bool { return c.Options[0] == "sí" && c.Options[1] == "no" },
		},
		{
			nombre: "encuesta de una sola opción no es una encuesta",
			in:     domain.Card{Kind: domain.CardPoll, Options: []string{"sí"}},
			err:    true,
		},
		{
			nombre: "encuesta con opción en blanco",
			in:     domain.Card{Kind: domain.CardPoll, Options: []string{"sí", "   "}},
			err:    true,
		},
		{
			nombre: "encuesta con más de diez opciones",
			in:     domain.Card{Kind: domain.CardPoll, Options: make([]string, 11)},
			err:    true,
		},
		{
			nombre: "encuesta con opción larguísima",
			in:     domain.Card{Kind: domain.CardPoll, Options: []string{"a", strings.Repeat("é", 121)}},
			err:    true,
		},
		{
			nombre: "la encuesta ignora la fecha límite de una tarea",
			in:     domain.Card{Kind: domain.CardPoll, Options: []string{"a", "b"}, Due: &ahora},
			quiere: func(c domain.Card) bool { return c.Due == nil },
		},
		{
			nombre: "tarea conserva responsable y fecha",
			in:     domain.Card{Kind: domain.CardTask, AssigneeSub: " sub-1 ", AssigneeName: " Ana ", Due: &ahora},
			quiere: func(c domain.Card) bool {
				return c.AssigneeSub == "sub-1" && c.AssigneeName == "Ana" && c.Due != nil
			},
		},
		{
			nombre: "tipo desconocido",
			in:     domain.Card{Kind: "pizarra"},
			err:    true,
		},
		{
			nombre: "el estado que manda el cliente se descarta",
			in: domain.Card{
				Kind:    domain.CardPoll,
				Options: []string{"a", "b"},
				Votes:   []domain.CardVote{{Value: "0", Count: 99, Subs: []string{"colado"}}},
			},
			quiere: func(c domain.Card) bool { return len(c.Votes) == 0 },
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			got, err := domain.NormalizeCard(c.in)
			if c.err {
				if err == nil {
					t.Fatalf("se esperaba error, salió %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if !c.quiere(got) {
				t.Fatalf("tarjeta normalizada inesperada: %+v", got)
			}
		})
	}
}

func TestValidCardValue(t *testing.T) {
	t.Parallel()
	encuesta := domain.Card{Kind: domain.CardPoll, Options: []string{"a", "b", "c"}}
	tarea := domain.Card{Kind: domain.CardTask}

	casos := []struct {
		card  domain.Card
		value string
		ok    bool
	}{
		{encuesta, "0", true},
		{encuesta, "2", true},
		{encuesta, "3", false},  // fuera de rango: la opción no existe
		{encuesta, "-1", false}, // negativo
		{encuesta, "done", false},
		{encuesta, "", false},
		{tarea, domain.CardDone, true},
		{tarea, "0", false},
		{domain.Card{}, "0", false}, // mensaje sin tarjeta: nada es votable
	}
	for _, c := range casos {
		if got := domain.ValidCardValue(c.card, c.value); got != c.ok {
			t.Errorf("ValidCardValue(%s, %q) = %v; quiero %v", c.card.Kind, c.value, got, c.ok)
		}
	}
}

func TestCardDone(t *testing.T) {
	t.Parallel()
	pendiente := domain.Card{Kind: domain.CardTask}
	if pendiente.Done() {
		t.Error("una tarea sin votos no está hecha")
	}
	hecha := domain.Card{Kind: domain.CardTask, Votes: []domain.CardVote{{Value: domain.CardDone, Count: 1, Subs: []string{"sub-1"}}}}
	if !hecha.Done() {
		t.Error("una tarea con 'done' sí está hecha")
	}
	votada := domain.Card{Kind: domain.CardPoll, Votes: []domain.CardVote{{Value: "0", Count: 3}}}
	if votada.Done() {
		t.Error("una encuesta votada no es una tarea hecha")
	}
}
