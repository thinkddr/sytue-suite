// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"net/http"
	"time"
)

// Constructores para las pruebas, con un token fijo y el cliente del servidor falso.

func tokenFijo(context.Context) (string, error) { return "token-de-prueba", nil }

func GoogleCalendarDe(base string, hc *http.Client) Remoto {
	return &GoogleCalendar{api: apiJSON{Token: tokenFijo, HTTP: hc}, Base: base}
}

func GooglePeopleDe(base string, hc *http.Client) Remoto {
	return &GooglePeople{api: apiJSON{Token: tokenFijo, HTTP: hc}, Base: base}
}

func GraphCalendarioDe(base string, hc *http.Client, ahora func() time.Time) Remoto {
	return &GraphCalendario{api: apiJSON{Token: tokenFijo, HTTP: hc}, Base: base, Ahora: ahora}
}

func GraphContactosDe(base string, hc *http.Client) Remoto {
	return &GraphContactos{api: apiJSON{Token: tokenFijo, HTTP: hc}, Base: base}
}

var RRULEDeGraph = func(tipo string, intervalo int, dias []string, indice, rango string, n int) string {
	return rruleDeGraph(recurrenciaGraph{Pattern: patronGraph{Type: tipo, Interval: intervalo, DaysOfWeek: dias, Index: indice}, Range: rangoGraph{Type: rango, NumberOfOccurrences: n}})
}

var GraphDeRRULE = func(r string, inicio time.Time) (string, error) {
	g, err := recurrenciaDeRRULE(r, inicio, "")
	if err != nil {
		return "", err
	}
	return rruleDeGraph(*g), nil
}
