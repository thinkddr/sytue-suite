// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
)

// Coordenadas PÚBLICAS de los clientes nativos de la suite (IMAP/SMTP, CalDAV,
// CardDAV, WebDAV) en un solo JSON, para que el asistente «Conectar tus
// dispositivos» del portal no las lleve escritas a mano.
//
// Es la MISMA información que Sytue Mail ya publica sin autenticar en su
// autoconfiguración (Thunderbird/Autodiscover) y que va dentro del perfil de
// Apple: nombres de servidor y puertos. NO lleva nada del usuario, así que no
// exige sesión — la dirección de correo la pone cada superficie desde SU sesión.
//
// URI well-known de uso privado (RFC 8615 §2): el nombre lleva el prefijo del
// fabricante, no pretende ser un estándar.

type clientHostPort struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// TLS es el modo de cifrado anunciado: hoy siempre "implicit" (IMAPS 993 /
	// submission 465), que es lo único que anuncia la autoconfiguración de Mail.
	TLS string `json:"tls"`
}

type clientEndpoints struct {
	Domain          string         `json:"domain"`
	IMAP            clientHostPort `json:"imap"`
	SMTP            clientHostPort `json:"smtp"`
	CalDAVURL       string         `json:"caldav_url"`
	CardDAVURL      string         `json:"carddav_url"`
	WebDAVURL       string         `json:"webdav_url"`
	AppleProfileURL string         `json:"apple_profile_url"`
}

// handleClientEndpoints sirve las coordenadas públicas de los clientes nativos.
func (s *Server) handleClientEndpoints(w http.ResponseWriter, _ *http.Request) {
	body, err := clientEndpointsOf(s.cfg)
	if err != nil {
		s.log.Error("coordenadas de clientes nativos", "error", err)
		http.Error(w, "configuración de servidor incompleta", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(body)
}

// clientEndpointsOf resuelve las coordenadas desde la configuración del servicio
// (las mismas que alimentan el perfil .mobileconfig).
func clientEndpointsOf(cfg Config) (clientEndpoints, error) {
	imapHost, imapPort, err := splitAddr(cfg.MailIMAPAddr, 993)
	if err != nil {
		return clientEndpoints{}, err
	}
	smtpHost, smtpPort, err := splitAddr(cfg.MailSMTPAddr, 465)
	if err != nil {
		return clientEndpoints{}, err
	}
	return clientEndpoints{
		Domain:          suiteDomain(cfg.Issuer),
		IMAP:            clientHostPort{Host: imapHost, Port: imapPort, TLS: "implicit"},
		SMTP:            clientHostPort{Host: smtpHost, Port: smtpPort, TLS: "implicit"},
		CalDAVURL:       cfg.CalDAVURL,
		CardDAVURL:      cfg.CardDAVURL,
		WebDAVURL:       cfg.DriveWebDAVURL,
		AppleProfileURL: cfg.Issuer + "/account/apple.mobileconfig",
	}, nil
}
