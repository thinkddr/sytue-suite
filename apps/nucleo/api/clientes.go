// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
)

// ServidorCliente es un servidor de correo tal y como lo anuncia Sytue ID: host, puerto
// y modo de cifrado (hoy siempre "implicit": IMAPS 993 y submission 465).
type ServidorCliente struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  string `json:"tls"`
}

// Coordenadas son las direcciones PÚBLICAS con las que un programa de fuera —el correo
// del móvil, Thunderbird, DAVx5— se conecta a la suite: `/.well-known/sytue-clients.json`
// de Sytue ID (`services/id/internal/http/clientconfig.go`). No llevan nada de la
// persona: la dirección la pone quien las enseña, desde su sesión.
type Coordenadas struct {
	Domain          string          `json:"domain"`
	IMAP            ServidorCliente `json:"imap"`
	SMTP            ServidorCliente `json:"smtp"`
	CalDAVURL       string          `json:"caldav_url"`
	CardDAVURL      string          `json:"carddav_url"`
	WebDAVURL       string          `json:"webdav_url"`
	AppleProfileURL string          `json:"apple_profile_url"`
}

// CoordenadasDe las pide a Sytue ID (`idURL` es su raíz, ServiceURL(apex, "id")). Van
// sin token: es la misma información que Mail publica en su autoconfiguración, y el
// asistente de «Conectar tus dispositivos» la necesita aunque la sesión esté renovándose.
func CoordenadasDe(ctx context.Context, idURL string, h *http.Client) (Coordenadas, error) {
	return JSON[Coordenadas](ctx, New(idURL, nil, h), Request{Path: "/.well-known/sytue-clients.json", Anonymous: true})
}
