// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Recolector remoto: cómo llega a la consola de plataforma lo que captura un
// servicio que NO es Sytue ID.
//
// La tabla `service_logs` vive en la base del IdP, y cada servicio tiene la
// suya: sin transporte, «¿qué está fallando?» solo respondía por el IdP —donde
// caen identidad, invitaciones, plataforma y el correo saliente— y los otros
// catorce seguían escribiendo a stdout para nadie.
//
// Se manda al IdP en vez de dar tabla y endpoint a cada servicio porque esto es
// diagnóstico de PLATAFORMA, no dato de negocio de nadie: una sola tabla, una
// sola purga, una sola pantalla, y catorce migraciones que no hay que escribir.
//
// La configuración es del CHART, no del usuario: sin URL o sin token no se
// engancha nada y el servicio se comporta exactamente como antes. Esa es la
// degradación honesta —no capturar es visible en la pantalla; capturar contra
// un destino inventado sería un error nuevo cada dos segundos—.
const (
	// CollectorURLEnv es la URL COMPLETA del receptor (incluye la ruta), para
	// que cambiarla no exija tocar este binario ni el de destino.
	CollectorURLEnv = "SYTUE_LOG_COLLECTOR_URL"
	// CollectorTokenEnv es el bearer estático compartido con el receptor.
	//nolint:gosec // G101: es el NOMBRE de la variable de entorno, no su valor.
	CollectorTokenEnv = "SYTUE_LOG_COLLECTOR_TOKEN"
)

type httpSink struct {
	url   string
	token string
	c     *http.Client
}

func (s *httpSink) Write(ctx context.Context, batch []Entry) error {
	cuerpo, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(cuerpo))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Drenar lo poco que conteste: sin leer el cuerpo, la conexión no se reusa y
	// una tormenta de errores abre un socket nuevo cada dos segundos.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("recolector de logs: %s", resp.Status)
	}
	return nil
}

// collectorDesdeEntorno devuelve el sumidero remoto que configura el chart, o
// nil si este proceso no tiene recolector (dev, tests, herramientas de línea de
// comandos y el propio Sytue ID, que escribe en su base sin dar el rodeo).
func collectorDesdeEntorno() Sink {
	url := strings.TrimSpace(os.Getenv(CollectorURLEnv))
	tok := strings.TrimSpace(os.Getenv(CollectorTokenEnv))
	if url == "" || tok == "" {
		return nil
	}
	// ponytail: sin reintentos. El lote se pierde y el capturador ya cuenta lo
	// descartado; reintentar con la cola llenándose es como se convierte un
	// incidente en dos. Si se ve pérdida real, aquí va un reintento único.
	return &httpSink{url: url, token: tok, c: &http.Client{Timeout: 10 * time.Second}}
}
