// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// maxFeedICS es lo más que se lee de un feed: un calendario de festivos o de un equipo
// cabe de sobra; más es un fichero que no es lo que se pidió.
const maxFeedICS = 10 << 20

// clienteICS es el de las suscripciones: la única red de Calendar, hacia la URL que la
// persona escribió (propósito `suscripcion-ics`, PLAN-EDICION-LOCAL.md Anexo B), y por el
// portero (§5.8), que abre ese host solo mientras dura la descarga. Sin cookies ni
// credenciales, y las redirecciones solo a http(s).
func clienteICS(l *local.App) *http.Client {
	return &http.Client{
		Transport: l.Red.Transporte(red.SuscripcionICS),
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !event.ValidSubscriptionURL(req.URL.String()) {
				return errors.New("calendar: redirección no admitida")
			}
			return nil
		},
	}
}

var errFeedGrande = errors.New("calendar: el calendario de la URL es demasiado grande")

// descargadorICS trae el `.ics` de una suscripción.
func descargadorICS(l *local.App) func(ctx context.Context, url string) ([]byte, error) {
	return func(ctx context.Context, url string) ([]byte, error) { return descargarICS(ctx, l, url) }
}

func descargarICS(ctx context.Context, l *local.App, url string) ([]byte, error) {
	if !event.ValidSubscriptionURL(url) {
		return nil, errors.New("calendar: URL de suscripción no válida")
	}
	defer l.Red.PermitirURL(red.SuscripcionICS, url, url)()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/calendar, text/plain;q=0.9, */*;q=0.5")
	req.Header.Set("User-Agent", "SytueOffice-Calendar")
	resp, err := clienteICS(l).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar: la URL respondió %s", resp.Status)
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedICS+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > maxFeedICS {
		return nil, errFeedGrande
	}
	return datos, nil
}
