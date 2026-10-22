// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// --- Darse de baja de listas (List-Unsubscribe / RFC 8058 one-click) ---
//
// Un correo de lista anuncia cómo darse de baja en sus cabeceras (RFC 2369
// List-Unsubscribe + RFC 8058 List-Unsubscribe-Post). El lector ofrece «Darse de
// baja»; el servidor ejecuta la baja SIN exponer al usuario a la URL/dirección:
//   - one-click https  → POST "List-Unsubscribe=One-Click" al endpoint del
//     remitente, SSRF-safe en doble barrera (platform/pkg/ssrfhttp): solo https,
//     validando host/IP en la RESOLUCIÓN y en el DIAL, bloqueando privadas/
//     loopback/link-local/ULA/CGNAT/metadata, con timeout, redirects acotados
//     re-validados y cuerpo capado. Nunca alcanza un destino interno.
//   - solo mailto      → envía el correo de baja desde la dirección del usuario
//     (reusa el saliente con DKIM + cola), sin que el usuario toque la dirección.
//
// El método y su URL/mailto SIEMPRE se re-parsean del mensaje almacenado en el
// servidor (el cliente solo aporta el id): un cliente no puede inducir un POST a
// una URL arbitraria (cierra el SSRF por parámetro). Aislado por dueño: un mensaje
// ajeno responde 404 (account_id, además de RLS por tenant).

const (
	unsubTimeout      = 10 * time.Second
	unsubMaxRedirects = 3
	unsubMaxRespBytes = 8 << 10 // 8 KiB: no descargamos la página de confirmación.
	unsubReadCap      = 1 << 20 // 1 MiB del .eml basta para las cabeceras.
)

var (
	errUnsubInsecure  = errors.New("unsubscribe: solo se permite https")
	errUnsubRedirects = errors.New("unsubscribe: demasiados redirects")
)

// getUnsubscribe anuncia al lector qué métodos de baja ofrece un mensaje, SIN
// exponer la URL/dirección al navegador: solo booleanos. El POST hermano ejecuta
// la baja por id.
func (a *API) getUnsubscribe(w http.ResponseWriter, r *http.Request) {
	_, u, err := a.messageUnsubscribe(r)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"one_click": u.OneClick,
		"mailto":    u.Mailto != "",
	})
}

// postUnsubscribe ejecuta la baja. Prioriza one-click https (RFC 8058); si no,
// envía el correo de baja al mailto. Responde 200 con {status: ok|sent|failed}
// para que el lector muestre el resultado sin romper la vista.
func (a *API) postUnsubscribe(w http.ResponseWriter, r *http.Request) {
	acc, u, err := a.messageUnsubscribe(r)
	if err != nil {
		httpx.Fail(w, r, mapErr(err))
		return
	}
	switch {
	case u.OneClick && u.HTTPS != "":
		if err := oneClickPOST(r.Context(), u.HTTPS); err != nil {
			a.logger.Warn("unsubscribe one-click", "error", err)
			httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "failed"})
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "ok"})
	case u.Mailto != "":
		if err := a.sendUnsubscribeMail(r.Context(), acc, u); err != nil {
			a.logger.Warn("unsubscribe mailto", "error", err)
			httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "failed"})
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"status": "sent"})
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This message offers no unsubscribe method."))
	}
}

// messageUnsubscribe resuelve la cuenta, carga el mensaje del DUEÑO y parsea sus
// métodos de baja del .eml crudo. Aísla por dueño (account_id, además de RLS por
// tenant): un mensaje ajeno responde 404.
func (a *API) messageUnsubscribe(r *http.Request) (domain.Account, mime.Unsubscribe, error) {
	acc, err := a.account(r)
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, err
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, httpx.ErrBadRequest.WithMessage("Invalid messageID.")
	}
	m, err := a.store.GetMessage(r.Context(), id)
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, err
	}
	if m.AccountID != acc.ID {
		return domain.Account{}, mime.Unsubscribe{}, domain.ErrNotFound
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, httpx.ErrUnauthorized
	}
	blob, err := a.blobsGet(r, t, m.BlobKey)
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, err
	}
	defer blob.Close()
	raw, err := io.ReadAll(io.LimitReader(blob, unsubReadCap))
	if err != nil {
		return domain.Account{}, mime.Unsubscribe{}, err
	}
	return acc, mime.ParseUnsubscribe(raw), nil
}

// oneClickPOST ejecuta la baja RFC 8058: POST application/x-www-form-urlencoded con
// cuerpo "List-Unsubscribe=One-Click" al endpoint https del remitente, SSRF-safe en
// DOBLE BARRERA (platform/pkg/ssrfhttp):
//   - solo https (RFC 8058), en la URL inicial Y en cada redirect;
//   - barrera 1 (ValidateURL): resuelve y rechaza IP privada/loopback/link-local/
//     ULA/CGNAT/metadata ANTES de conectar;
//   - barrera 2 (Transport/SafeDialContext): re-resuelve y valida en el CONNECT,
//     cerrando la ventana TOCTOU / DNS-rebinding;
//   - timeout corto, redirects acotados re-validados, cuerpo de respuesta capado.
func oneClickPOST(ctx context.Context, rawURL string) error {
	if !strings.EqualFold(schemeOfURL(rawURL), "https") {
		return errUnsubInsecure
	}
	if err := ssrfhttp.ValidateURL(ctx, rawURL); err != nil { // barrera 1 (pre-flight)
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, unsubTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL,
		strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Sytue-Mail-Unsubscribe/1")
	client := &http.Client{
		Timeout:   unsubTimeout,
		Transport: ssrfhttp.Transport(), // barrera 2 (dialer que valida en el connect)
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= unsubMaxRedirects {
				return errUnsubRedirects
			}
			if !strings.EqualFold(req.URL.Scheme, "https") {
				return errUnsubInsecure // no degradar a http en un redirect
			}
			return ssrfhttp.ValidateURL(req.Context(), req.URL.String()) // re-valida cada salto
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, unsubMaxRespBytes)) // cap de bytes
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unsubscribe: estado %d", resp.StatusCode)
	}
	return nil
}

// sendUnsubscribeMail envía el correo de baja al mailto anunciado DESDE la
// dirección del usuario (reusa el saliente con firma DKIM + cola), sin exponer al
// usuario a la dirección de la lista.
func (a *API) sendUnsubscribeMail(ctx context.Context, acc domain.Account, u mime.Unsubscribe) error {
	if a.outbound == nil {
		return httpx.ErrInternal.WithMessage("Sending is unavailable.")
	}
	to, err := domain.ParseAddress(u.Mailto)
	if err != nil {
		return httpx.ErrBadRequest.WithMessage("Invalid unsubscribe address.")
	}
	subject := sanitizeHeader(u.MailtoSubject)
	if subject == "" {
		subject = "unsubscribe"
	}
	raw := buildUnsubscribeMail(acc.Address, to.String(), subject)
	return a.outbound.SubmitRaw(ctx, acc.Address, []string{to.String()}, raw)
}

// buildUnsubscribeMail compone un .eml RFC 5322 mínimo para la baja por mailto.
func buildUnsubscribeMail(from, to, subject string) []byte {
	fromDomain := "mail.sytue"
	if at := strings.LastIndexByte(from, '@'); at >= 0 && at < len(from)-1 {
		fromDomain = from[at+1:]
	}
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID: <" + uuid.NewString() + "@" + fromDomain + ">",
		"Auto-Submitted: auto-generated",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" +
		"Please unsubscribe this address from your mailing list.\r\n")
}

// sanitizeHeader deja solo ASCII imprimible (0x20–0x7e): elimina CR/LF y control,
// evitando la inyección de cabeceras a través del ?subject= del mailto (que llega
// URL-decodificado). Acota la longitud.
func sanitizeHeader(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return -1
		}
		return r
	}, s)
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.TrimSpace(s)
}

// schemeOfURL devuelve el esquema de una URL ("" si no parsea).
func schemeOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme
}
