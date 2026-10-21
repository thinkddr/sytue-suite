// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"html"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
)

// CONSENTIMIENTO DE LAS APPS NATIVAS, UNA VEZ POR DISPOSITIVO (NAT-02; decidido por el
// dueño el 2026-10-16).
//
// `sytue-desktop` y `sytue-mobile` son públicos: nada en la petición dice que quien la
// hace es nuestra app. Con una sesión abierta en el navegador, otra app del mismo móvil
// —o otra persona del mismo equipo por el loopback— abría /authorize con su propio PKCE y
// se llevaba los tokens sin que nadie viera nada. Ahora la primera vez en cada
// dispositivo Sytue ID pregunta; después, no.
//
// EL DISPOSITIVO es un secreto que la app guarda en el almacén seguro del sistema. A
// /authorize solo viaja su hash (`sytue_device`); el secreto va en el canje, por el canal
// de atrás (`sytue_device_secret`), igual que el verificador de PKCE (token.go). Así, lo
// que se recuerda no se puede falsificar copiando un dato: un código emitido sin
// preguntar para el hash de un dispositivo solo lo canjea quien tiene su secreto. Ni la
// cookie del navegador ni el User-Agent valen: los comparten todas las apps del equipo,
// que es justo de quién hay que distinguir a la nuestra.
//
// Una app que no manda dispositivo (una versión anterior) recibe la pregunta cada vez.

// paramDispositivo es el hash del dispositivo en /authorize; paramSecretoDispositivo, el
// secreto en /token.
const (
	paramDispositivo        = "sytue_device"
	paramSecretoDispositivo = "sytue_device_secret" //nolint:gosec // G101: es el NOMBRE del parámetro, no un secreto
	paramConsentimiento     = "sytue_consent"
)

// appsNativas da el nombre con el que se reconoce cada app móvil por su vuelta.
//
// ponytail: la lista de apps vive en apps/fyne/cmd/*/FyneApp.toml (otro módulo); una
// vuelta que no esté aquí pregunta por «las apps de Sytue», que también es verdad.
var appsNativas = map[string]string{
	"sytue-mail": "Sytue Mail", "sytue-drive": "Sytue Drive", "sytue-docs": "Sytue Docus",
	"sytue-sheets": "Sytue Calcus", "sytue-slides": "Sytue Diapos", "sytue-pdf": "Sytue CertiPDF",
	"sytue-calendar": "Sytue Calendar", "sytue-contacts": "Sytue Contacts", "sytue-notes": "Sytue Notepads",
	"sytue-photos": "Sytue Photos", "sytue-erp": "Sytue ERP", "sytue-rooms": "Sytue Rooms",
	"sytue-one": "Sytue Hub", "sytue-bases": "Sytue Bases", "sytue-cast": "Sytue Cast",
}

// consentimientoNativo decide si una app nativa puede llevarse el código ya. Devuelve
// el dispositivo al que atarlo y si sigue; si no sigue, ya ha respondido (la pantalla
// de consentimiento o el error al redirect_uri).
func (s *Server) consentimientoNativo(w http.ResponseWriter, r *http.Request, req authzRequest,
	client domain.Client, sess domain.Session, redirectErr func(code, desc string),
) (string, bool) {
	dispositivo := r.FormValue(paramDispositivo)
	if dispositivo != "" && !domain.ValidDeviceHash(dispositivo) {
		redirectErr("invalid_request", paramDispositivo+" no es válido")
		return "", false
	}
	if dispositivo != "" {
		ya, err := s.store.ConsentidaEnDispositivo(r.Context(), sess.UserID, client.ID, req.RedirectURI, dispositivo)
		if err != nil {
			redirectErr("server_error", "")
			return "", false
		}
		if ya {
			return dispositivo, true
		}
	}
	firma := s.firmaConsentimiento(sess, req, dispositivo)
	if got := r.FormValue(paramConsentimiento); got != "" && hmac.Equal([]byte(got), []byte(firma)) {
		if dispositivo != "" {
			if err := s.store.RecordarConsentimiento(r.Context(), sess.UserID, client.ID, req.RedirectURI, dispositivo); err != nil {
				redirectErr("server_error", "")
				return "", false
			}
		}
		_ = s.store.Audit(r.Context(), nil, auditEntry(sess.UserID, sess.OrgID, "oauth.authorize.native_consent", client.ID, r))
		return dispositivo, true
	}
	s.pantallaConsentimiento(w, r, req, sess, firma)
	return "", false
}

// firmaConsentimiento ata el «Permitir» a la sesión y a ESTA petición: un enlace ajeno
// no lo puede fabricar sin la sesión, y uno de otra petición no vale para esta.
func (s *Server) firmaConsentimiento(sess domain.Session, req authzRequest, dispositivo string) string {
	m := hmac.New(sha256.New, s.consentKey)
	for _, v := range []string{sess.ID, req.ClientID, req.RedirectURI, req.CodeChallenge, dispositivo} {
		m.Write([]byte(v))
		m.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// pantallaConsentimiento: «¿Dar acceso a Sytue <app> en este dispositivo?». Las dos
// salidas son ENLACES y no un formulario: la CSP del IdP (`form-action`) no admite el
// loopback ni el esquema de la app, y una redirección tras un POST cae bajo ella.
func (s *Server) pantallaConsentimiento(w http.ResponseWriter, r *http.Request, req authzRequest, sess domain.Session, firma string) {
	loc := i18n.From(r)
	esc := html.EscapeString

	titulo := i18n.T(loc, "consent.title_suite")
	if u, err := url.Parse(req.RedirectURI); err == nil {
		if app, ok := appsNativas[u.Scheme]; ok {
			titulo = strings.ReplaceAll(i18n.T(loc, "consent.title_app"), "{app}", app)
		}
	}
	cuenta := ""
	if u, err := s.store.GetUserByID(r.Context(), sess.UserID); err == nil {
		cuenta = u.Email
	}

	permitir := *r.URL
	q := r.URL.Query()
	if r.Method == http.MethodPost {
		q = r.Form
	}
	q.Set(paramConsentimiento, firma)
	permitir.RawQuery = q.Encode()

	denegar, _ := url.Parse(req.RedirectURI)
	dq := denegar.Query()
	dq.Set("error", "access_denied")
	if req.State != "" {
		dq.Set("state", req.State)
	}
	denegar.RawQuery = dq.Encode()

	cuerpo := `<p class="sub">` + esc(strings.ReplaceAll(i18n.T(loc, "consent.body"), "{account}", cuenta)) + `</p>` +
		`<a class="cta" href="` + esc(permitir.RequestURI()) + `">` + esc(i18n.T(loc, "consent.allow")) + `</a>` +
		`<p class="alt"><a class="btn-link" href="` + esc(denegar.String()) + `">` + esc(i18n.T(loc, "consent.deny")) + `</a></p>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPage(loc, titulo, cuerpo)))
}
