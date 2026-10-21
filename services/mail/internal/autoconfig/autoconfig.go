// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package autoconfig sirve el autodescubrimiento de Sytue Mail para los dos
// clientes de correo dominantes, de modo que el usuario solo escriba su
// dirección y el cliente se configure solo:
//
//   - Thunderbird / Mozilla autoconfig: GET /.well-known/autoconfig/mail/config-v1.1.xml
//     (y el alias histórico /mail/config-v1.1.xml?emailaddress=…).
//   - Microsoft Outlook Autodiscover (POX): POST /autodiscover/autodiscover.xml
//     (y un GET tolerante con ?emailaddress=…).
//
// Ambos protocolos son PÚBLICOS por diseño: el cliente todavía no tiene
// credenciales cuando pregunta. Por eso estos endpoints no autentican y no
// exponen NADA del usuario: solo la configuración de servidor (host, puerto,
// cifrado y método de autenticación), más la dirección que el propio cliente
// acaba de enviar. Los hosts y puertos salen de la configuración del servicio.
//
// Solo se anuncia lo que el despliegue sirve de verdad: IMAP con TLS implícito
// (IMAPS_ADDR) y submission con TLS implícito (SMTPS_ADDR). No hay POP y no se
// anuncia STARTTLS (el 587 solo lo ofrece si hay certificado; el 465 cubre a
// todos los clientes).
package autoconfig

import (
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/mail/internal/config"
)

// maxRequestBytes acota el cuerpo de una petición Autodiscover (el XML real
// ronda 1 KB). Endpoint público sin auth: el límite es la primera defensa.
const maxRequestBytes = 32 << 10

// placeholderAddress es el marcador de Mozilla autoconfig cuando el cliente no
// nos ha dicho aún qué dirección configura: lo sustituye él mismo.
const placeholderAddress = "%EMAILADDRESS%"

// handler agrupa la configuración ya resuelta a host/puerto públicos.
type handler struct {
	domain     string
	imapHost   string
	imapPort   int
	smtpHost   string
	smtpPort   int
	brandName  string
	brandShort string
	// mxHost y stsMode arman la política MTA-STS (RFC 8461).
	mxHost  string
	stsMode string
}

// mtaSTSMaxAge es la vigencia que el remitente cachea la política, en segundos
// (una semana, el valor que recomienda el RFC). Cachearla es justo lo que da la
// protección: un atacante que degrade la conexión más adelante se topa con la
// política que el remitente ya guardó.
const mtaSTSMaxAge = 604800

// Mount registra las rutas de autoconfiguración en el router de Sytue Mail.
func Mount(r chi.Router, cfg config.Config) {
	h := &handler{
		domain:     cfg.PrimaryDomain,
		imapHost:   cfg.IMAPPublicHost,
		imapPort:   portOf(cfg.IMAPSAddr, 993),
		smtpHost:   cfg.SMTPPublicHost,
		smtpPort:   portOf(cfg.SMTPSImplicitAddr, 465),
		brandName:  "Sytue Mail",
		brandShort: "Sytue",
		mxHost:     cfg.Hostname,
		stsMode:    cfg.MTASTSMode,
	}

	// Thunderbird y clientes Mozilla-compatibles (RFC-less, ISPDB de Mozilla).
	r.Get("/.well-known/autoconfig/mail/config-v1.1.xml", h.mozillaAutoconfig)
	r.Get("/mail/config-v1.1.xml", h.mozillaAutoconfig) // alias histórico

	// Outlook Autodiscover (POX). Outlook prueba ambas capitalizaciones.
	r.Post("/autodiscover/autodiscover.xml", h.autodiscover)
	r.Get("/autodiscover/autodiscover.xml", h.autodiscover)
	r.Post("/Autodiscover/Autodiscover.xml", h.autodiscover)
	r.Get("/Autodiscover/Autodiscover.xml", h.autodiscover)

	// MTA-STS (RFC 8461): el remitente resuelve el TXT _mta-sts.<dominio> y, si
	// cambió el id, baja ESTA política por HTTPS desde mta-sts.<dominio>. El TXT
	// ya estaba publicado sin que nadie sirviera el fichero, que es la peor de
	// las combinaciones: se anuncia una protección contra downgrade que ningún
	// remitente puede llegar a aplicar.
	r.Get("/.well-known/mta-sts.txt", h.mtaSTSPolicy)
}

// mtaSTSPolicy sirve la política MTA-STS en el formato de texto del RFC 8461.
//
// El `mode` es configurable a propósito. En `enforce`, un remitente que no
// pueda validar el certificado del MX contra el nombre publicado NO entrega el
// correo: pasar a enforce antes de que el certificado cubra mx.<dominio> es
// cortarse el correo entrante uno mismo. Por eso el defecto es `testing`, que
// pide informes (los recoge el TLS-RPT que ya publicamos) sin bloquear nada.
func (h *handler) mtaSTSPolicy(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// CRLF: el RFC 8461 define el fichero con finales de línea de red.
	_, _ = io.WriteString(w, "version: STSv1\r\nmode: "+h.stsMode+
		"\r\nmx: "+h.mxHost+
		"\r\nmax_age: "+strconv.Itoa(mtaSTSMaxAge)+"\r\n")
}

// --- Thunderbird / Mozilla autoconfig ---

type clientConfig struct {
	XMLName  xml.Name      `xml:"clientConfig"`
	Version  string        `xml:"version,attr"`
	Provider emailProvider `xml:"emailProvider"`
}

type emailProvider struct {
	ID               string     `xml:"id,attr"`
	Domains          []string   `xml:"domain"`
	DisplayName      string     `xml:"displayName"`
	DisplayShortName string     `xml:"displayShortName"`
	Incoming         mailServer `xml:"incomingServer"`
	Outgoing         mailServer `xml:"outgoingServer"`
}

type mailServer struct {
	Type           string `xml:"type,attr"`
	Hostname       string `xml:"hostname"`
	Port           int    `xml:"port"`
	SocketType     string `xml:"socketType"`
	Authentication string `xml:"authentication"`
	Username       string `xml:"username"`
}

func (h *handler) mozillaAutoconfig(w http.ResponseWriter, r *http.Request) {
	// La dirección es opcional: sin ella se emite el marcador de Mozilla.
	username := placeholderAddress
	domains := []string{h.domain}
	if addr, ok := normalizeAddress(r.URL.Query().Get("emailaddress")); ok {
		username = addr
		// Dominio de tenant distinto del principal: anunciarlo también, si no
		// Thunderbird descarta la configuración por no casar el dominio.
		if d := addr[strings.LastIndex(addr, "@")+1:]; !strings.EqualFold(d, h.domain) {
			domains = append(domains, d)
		}
	}

	cc := clientConfig{
		Version: "1.1",
		Provider: emailProvider{
			ID:               h.domain,
			Domains:          domains,
			DisplayName:      h.brandName,
			DisplayShortName: h.brandShort,
			Incoming: mailServer{
				Type:           "imap",
				Hostname:       h.imapHost,
				Port:           h.imapPort,
				SocketType:     "SSL",
				Authentication: "password-cleartext", // sobre TLS implícito
				Username:       username,
			},
			Outgoing: mailServer{
				Type:           "smtp",
				Hostname:       h.smtpHost,
				Port:           h.smtpPort,
				SocketType:     "SSL",
				Authentication: "password-cleartext",
				Username:       username,
			},
		},
	}
	writeXML(w, cc)
}

// --- Outlook Autodiscover (POX) ---

// Espacios de nombres del protocolo POX de Autodiscover.
const (
	nsAutodiscover = "http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006"
	nsOutlook      = "http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a"
)

// autodiscoverRequest solo extrae la dirección. Sin espacio de nombres en el
// tag, el decodificador casa por nombre local: tolera las variantes que envían
// las distintas versiones de Outlook.
type autodiscoverRequest struct {
	EMailAddress string `xml:"Request>EMailAddress"`
}

type autodiscoverResponse struct {
	XMLName  xml.Name   `xml:"http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006 Autodiscover"`
	Response adResponse `xml:"http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a Response"`
}

type adResponse struct {
	Account adAccount `xml:"Account"`
}

type adAccount struct {
	AccountType string       `xml:"AccountType"`
	Action      string       `xml:"Action"`
	Protocols   []adProtocol `xml:"Protocol"`
}

type adProtocol struct {
	Type         string `xml:"Type"`
	Server       string `xml:"Server"`
	Port         int    `xml:"Port"`
	LoginName    string `xml:"LoginName"`
	SPA          string `xml:"SPA"`
	SSL          string `xml:"SSL"`
	AuthRequired string `xml:"AuthRequired"`
	Encryption   string `xml:"Encryption,omitempty"`
}

func (h *handler) autodiscover(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("emailaddress")
	if r.Method == http.MethodPost {
		var req autodiscoverRequest
		if err := xml.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
			http.Error(w, "Petición Autodiscover no válida.", http.StatusBadRequest)
			return
		}
		raw = req.EMailAddress
	}
	addr, ok := normalizeAddress(raw)
	if !ok {
		http.Error(w, "Falta una dirección de correo válida.", http.StatusBadRequest)
		return
	}

	resp := autodiscoverResponse{
		Response: adResponse{Account: adAccount{
			AccountType: "email",
			Action:      "settings",
			Protocols: []adProtocol{
				{
					Type: "IMAP", Server: h.imapHost, Port: h.imapPort, LoginName: addr,
					SPA: "off", SSL: "on", AuthRequired: "on",
				},
				{
					Type: "SMTP", Server: h.smtpHost, Port: h.smtpPort, LoginName: addr,
					SPA: "off", SSL: "on", AuthRequired: "on", Encryption: "SSL",
				},
			},
		}},
	}
	writeXML(w, resp)
}

// --- comunes ---

func writeXML(w http.ResponseWriter, v any) {
	body, err := xml.MarshalIndent(v, "", "  ")
	if err != nil { // estructuras estáticas: solo un bug lo provoca
		http.Error(w, "No se pudo generar la configuración.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

// normalizeAddress valida que lo recibido parezca una dirección de correo antes
// de reflejarlo en la respuesta. No comprueba que la cuenta exista: eso filtraría
// usuarios en un endpoint sin autenticar (enumeración).
func normalizeAddress(raw string) (string, bool) {
	addr := strings.TrimSpace(raw)
	if addr == "" || len(addr) > 320 || strings.ContainsAny(addr, " \t\r\n<>\"") {
		return "", false
	}
	at := strings.LastIndex(addr, "@")
	if at <= 0 || at == len(addr)-1 || strings.Count(addr, "@") != 1 {
		return "", false
	}
	if !strings.Contains(addr[at+1:], ".") {
		return "", false
	}
	return addr, true
}

// portOf extrae el puerto de una dirección de escucha (":993", "0.0.0.0:993").
func portOf(addr string, def int) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return def
	}
	return n
}
