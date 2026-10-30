// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
)

// sweegoSendURL es el envío de Sweego (Mindbaz SAS, Lille; alojado en Francia).
// El mismo /send sirve correo y SMS: lo distingue `channel`.
const sweegoSendURL = "https://api.sweego.io/send"

// SMS manda los códigos de un solo uso del alta (ADR 0041).
type SMS interface {
	Send(ctx context.Context, to, text string) error
}

// newSMS elige cómo salen los SMS. Con clave de Sweego, de verdad. Sin ella, en
// desarrollo se escriben en el log; en producción no hay SMS (nil) y el alta
// autoservicio se apaga sola, porque dar cuentas sin verificar el móvil es
// justo lo que no se quiere, y fingir que se envió sería peor.
func newSMS(cfg Config, logger *slog.Logger) SMS {
	if cfg.SMSSweegoAPIKey != "" {
		return sweegoSMS{
			url: sweegoSendURL, apiKey: cfg.SMSSweegoAPIKey, sender: cfg.SMSSender,
			client: &http.Client{Timeout: 10 * time.Second},
		}
	}
	if cfg.Env.IsProd() {
		logger.Warn("SYTUE_ID_SMS_SWEEGO_API_KEY vacía: sin SMS no hay alta autoservicio")
		return nil
	}
	return logSMS{log: logger}
}

// logSMS es el SMS de desarrollo: escribe el código en el log para copiarlo.
type logSMS struct{ log *slog.Logger }

func (m logSMS) Send(ctx context.Context, to, text string) error {
	m.log.WarnContext(ctx, "SMS no configurado: mensaje SOLO en el log (dev)", "to", logsafe.Text(to), "text", text)
	return nil
}

// sweegoSMS envía por la API de Sweego como SMS transaccional.
type sweegoSMS struct {
	url, apiKey, sender string
	client              *http.Client
}

func (s sweegoSMS) Send(ctx context.Context, to, text string) error {
	destino := map[string]string{"num": to}
	if region := regionDelMovil(to); region != "" {
		destino["region"] = region
	}
	cuerpo := map[string]any{
		"channel":       "sms",
		"provider":      "sweego",
		"campaign-type": "transac",
		"recipients":    []map[string]string{destino},
		"message-txt":   text,
	}
	// Sin remitente, Sweego usa el que haya dado de alta la cuenta (o un número
	// corto): un nombre que no esté registrado allí haría fallar cada envío.
	if s.sender != "" {
		cuerpo["sender-id"] = s.sender
	}
	body, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sweego sms: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		// El cuerpo del error dice si faltan créditos o el número no vale, que es
		// lo que hace falta para arreglarlo.
		detalle, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("sweego sms: HTTP %d: %s", res.StatusCode, bytes.TrimSpace(detalle))
	}
	return nil
}

// prefijosDeRegion: prefijo internacional → país ISO 3166 que Sweego pide junto
// al número. Los de la UE, el EEE, Suiza y el Reino Unido, que es a quien se
// vende la suite.
// ponytail: fuera de esta lista el número va sin región y decide Sweego; si
// rechaza los de algún país, se añade aquí su prefijo.
var prefijosDeRegion = map[string]string{
	"30": "GR", "31": "NL", "32": "BE", "33": "FR", "34": "ES", "36": "HU", "39": "IT",
	"40": "RO", "41": "CH", "43": "AT", "44": "GB", "45": "DK", "46": "SE", "47": "NO",
	"48": "PL", "49": "DE", "351": "PT", "352": "LU", "353": "IE", "354": "IS", "356": "MT",
	"357": "CY", "358": "FI", "359": "BG", "370": "LT", "371": "LV", "372": "EE",
	"385": "HR", "386": "SI", "420": "CZ", "421": "SK", "423": "LI",
}

// regionDelMovil da el país de un móvil en E.164, o "" si su prefijo no está en
// la lista. Los prefijos de tres cifras van antes: «+351» no es «+35…».
func regionDelMovil(e164 string) string {
	cifras := strings.TrimPrefix(e164, "+")
	for _, n := range []int{3, 2} {
		if len(cifras) > n {
			if region, ok := prefijosDeRegion[cifras[:n]]; ok {
				return region
			}
		}
	}
	return ""
}
