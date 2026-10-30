// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// Las respuestas a invitaciones de Sytue Calendar que llegan por correo (iMIP, RFC
// 6047). Calendar manda la invitación a quien no es de la suite con el ORGANIZER de
// quien la creó, así que el REPLY del cliente de calendario de esa persona llega a un
// buzón de aquí. El correo se entrega como cualquier otro —quien organiza lo ve en su
// bandeja— y, además, su parte `text/calendar; method=REPLY` se pasa a Calendar, que
// apunta la respuesta en el evento.
//
// Solo si el `From:` está AUTENTICADO: SPF o DKIM en pass Y alineados con su
// dominio (auth.DMARCResult.Alineado). El REPLY cambia la respuesta de alguien, y un
// From sin autenticar lo puede escribir cualquiera; «DKIM pass» a secas no vale, porque
// lo consigue quien firme con su propio dominio un correo que dice ser de otro.
// Calendar, a su vez, solo acepta la respuesta de la dirección que la manda.

// PuenteCalendario entrega los REPLY a Sytue Calendar. Nil (sin URL o sin secreto):
// los REPLY se quedan solo en el buzón.
type PuenteCalendario struct {
	url, secreto string
	http         *http.Client
	logger       *slog.Logger
}

// NewPuenteCalendario crea el puente con la raíz interna de Calendar y el secreto de
// servicio a servicio de la suite. Nil si falta cualquiera de las dos.
func NewPuenteCalendario(url, secreto string, logger *slog.Logger) *PuenteCalendario {
	if strings.TrimSpace(url) == "" || secreto == "" {
		return nil
	}
	return &PuenteCalendario{url: strings.TrimRight(url, "/"), secreto: secreto, http: &http.Client{Timeout: 15 * time.Second}, logger: logger}
}

// rutaRespuestaIMIP es la de `services/calendar/internal/http/imip.go`.
const rutaRespuestaIMIP = "/internal/imip/reply"

// maxCalendario acota lo que se lee de una parte `text/calendar`.
const maxCalendario = 256 << 10

// Pasar manda el REPLY del mensaje, si lo trae y su remitente está autenticado, en
// segundo plano: la entrega del correo no espera a Calendar.
func (p *PuenteCalendario) Pasar(ctx context.Context, tenant string, env domain.Message, raw []byte, fromAutenticado bool) {
	p.PasarReader(ctx, tenant, env, bytes.NewReader(raw), fromAutenticado)
}

// PasarReader manda el REPLY del mensaje leyendo en flujo desde r.
func (p *PuenteCalendario) PasarReader(ctx context.Context, tenant string, env domain.Message, r io.Reader, fromAutenticado bool) {
	if p == nil || !fromAutenticado {
		return
	}
	ics, ok := ReplyDeCalendarioReader(r)
	if !ok {
		return
	}
	remitente := env.FromAddr
	go func() {
		c, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancelar()
		req, err := http.NewRequestWithContext(c, http.MethodPost, p.url+rutaRespuestaIMIP, bytes.NewReader(ics))
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+p.secreto)
		req.Header.Set("Content-Type", "text/calendar; method=REPLY; charset=utf-8")
		req.Header.Set("X-Sytue-Tenant", tenant)
		req.Header.Set("X-Sytue-Remitente", remitente)
		resp, err := p.http.Do(req)
		if err != nil {
			p.logger.Warn("imip: pasar el REPLY a Calendar", "error", err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			p.logger.Warn("imip: Calendar no aceptó el REPLY", "status", resp.StatusCode)
		}
	}()
}

// ReplyDeCalendario busca en un mensaje RFC 5322 su parte `text/calendar` (o
// `application/ics`) con METHOD:REPLY y la devuelve decodificada.
func ReplyDeCalendario(raw []byte) ([]byte, bool) {
	return ReplyDeCalendarioReader(bytes.NewReader(raw))
}

// ReplyDeCalendarioReader busca la parte `text/calendar` con METHOD:REPLY desde r.
func ReplyDeCalendarioReader(r io.Reader) ([]byte, bool) {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return nil, false
	}
	return buscarReply(m.Header.Get("Content-Type"), m.Header.Get("Content-Transfer-Encoding"), m.Body, 0)
}

func buscarReply(tipo, codificacion string, cuerpo io.Reader, nivel int) ([]byte, bool) {
	if nivel > 6 {
		return nil, false
	}
	mt, params, err := mime.ParseMediaType(tipo)
	if err != nil {
		mt = "text/plain"
	}
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		r := multipart.NewReader(cuerpo, params["boundary"])
		for {
			parte, err := r.NextRawPart()
			if err != nil {
				return nil, false
			}
			if ics, ok := buscarReply(parte.Header.Get("Content-Type"), parte.Header.Get("Content-Transfer-Encoding"), parte, nivel+1); ok {
				return ics, true
			}
		}
	case mt == "text/calendar" || mt == "application/ics":
		datos, err := io.ReadAll(io.LimitReader(decodificar(codificacion, cuerpo), maxCalendario+1))
		if err != nil || len(datos) > maxCalendario {
			return nil, false
		}
		metodo := strings.EqualFold(params["method"], "REPLY")
		if !metodo {
			for _, l := range strings.Split(string(datos), "\n") {
				if strings.EqualFold(strings.TrimSpace(l), "METHOD:REPLY") {
					metodo = true
					break
				}
			}
		}
		return datos, metodo
	}
	return nil, false
}

func decodificar(cte string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, &sinSaltos{r: r})
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	}
	return r
}

// sinSaltos quita los CR/LF de un base64 partido en líneas: `base64.NewDecoder` los
// tolera, pero no los espacios de final de línea que meten algunos clientes.
type sinSaltos struct{ r io.Reader }

func (s *sinSaltos) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	j := 0
	for _, b := range p[:n] {
		if b != '\r' && b != '\n' && b != ' ' && b != '\t' {
			p[j] = b
			j++
		}
	}
	return j, err
}
