// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Recepción de los avisos y errores que capturan los OTROS servicios.
//
// `service_logs` vive en la base de Sytue ID y la consola de plataforma lee de
// ahí, así que hasta ahora la pantalla «qué está fallando» solo enseñaba lo del
// propio IdP: los otros catorce servicios escribían su JSON a stdout y ahí
// moría. Este endpoint es el transporte que faltaba (`platform/pkg/log`,
// collector.go), y a propósito NO va bajo `/api/v1`: ahí manda el JWT de un
// usuario, y quien llama aquí es un proceso sin persona detrás.
//
// Se protege con bearer estático —el mismo patrón que SCIM— porque el ingress
// de Sytue ID publica el host entero (`path: /`) y esto es, por tanto,
// alcanzable desde fuera. Las tres verjas son el token en tiempo constante, el
// tope de cuerpo y el tope de entradas por lote: sin ellas, quien encontrara la
// ruta podría llenar la tabla de diagnóstico de la plataforma.
package http

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	plog "git.sytue.eu/sytue/one/platform/pkg/log"
)

const (
	// Tope del cuerpo. Un lote son cien entradas de unos cientos de bytes; el
	// megabyte deja margen de sobra para atributos largos y acota el daño.
	ingestaMaxCuerpo = 1 << 20
	// Tope de entradas por lote. El capturador manda cien; quinientas dejan
	// margen para un vaciado de cierre sin abrir la puerta a un volcado masivo.
	ingestaMaxEntradas = 500
)

// handleIngestServiceLogs guarda un lote de entradas capturadas por otro
// servicio. Responde 204 y nada más: quien llama es un logger, y darle un
// cuerpo que leer solo sirve para que se lo trague.
func (s *Server) handleIngestServiceLogs(w http.ResponseWriter, r *http.Request) {
	if s.cfg.LogIngestToken == "" {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Log collector not configured."))
		return
	}
	tok, ok := bearer(r)
	if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.LogIngestToken)) != 1 {
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid collector token."))
		return
	}

	var lote []plog.Entry
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, ingestaMaxCuerpo))
	if err := dec.Decode(&lote); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Malformed log batch.").Wrap(err))
		return
	}
	if len(lote) > ingestaMaxEntradas {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Log batch too large."))
		return
	}

	ahora := time.Now()
	limpio := lote[:0]
	for _, e := range lote {
		// Solo lo que la pantalla enseña. Aceptar INFO por esta puerta llenaría
		// la tabla con el 99 % del volumen que el capturador existe para no
		// guardar, y bastaría un servicio mal configurado para conseguirlo.
		nivel := strings.ToUpper(strings.TrimSpace(e.Level))
		if nivel != "WARN" && nivel != "ERROR" {
			continue
		}
		e.Level = nivel
		e.Service = strings.TrimSpace(e.Service)
		if e.Service == "" {
			continue
		}
		// La marca de tiempo la pone el emisor y puede venir sin reloj o con uno
		// desviado; una fecha del futuro se quedaría clavada arriba en la lista y
		// una del 1970 no la vería nadie. Fuera de la ventana razonable, ahora.
		if e.At.IsZero() || e.At.After(ahora.Add(time.Hour)) || e.At.Before(ahora.Add(-24*time.Hour)) {
			e.At = ahora
		}
		limpio = append(limpio, e)
	}
	if len(limpio) > 0 {
		if err := s.store.WriteServiceLogs(r.Context(), limpio); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
