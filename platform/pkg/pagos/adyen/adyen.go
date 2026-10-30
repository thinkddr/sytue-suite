// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package adyen implementa Hosted Checkout Sessions para las tiendas Pages.
package adyen

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

var (
	merchantRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{3,80}$`)
	livePrefixRe = regexp.MustCompile(`^[a-z0-9]{4,64}$`)
)

// Adyen conserva la configuración de una cuenta. El endpoint live se deriva
// exclusivamente del prefijo emitido por Adyen; nunca llega de una petición.
type Adyen struct {
	apiKey, merchant, mode, livePrefix string
	baseURL                            string
	http                               *http.Client
}

var _ pagos.Pasarela = (*Adyen)(nil)

func New(apiKey, merchant, mode, livePrefix string) (*Adyen, error) {
	apiKey, merchant, livePrefix = strings.TrimSpace(apiKey), strings.TrimSpace(merchant), strings.TrimSpace(livePrefix)
	if apiKey == "" || !merchantRe.MatchString(merchant) || (mode != pagos.ModoPrueba && mode != pagos.ModoReal) || (mode == pagos.ModoReal && !livePrefixRe.MatchString(livePrefix)) {
		return nil, errors.New("adyen: invalid credentials")
	}
	a := &Adyen{apiKey: apiKey, merchant: merchant, mode: mode, livePrefix: livePrefix, http: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	a.baseURL = a.endpoint()
	return a, nil
}

// ConTransporte sustituye el transporte HTTP manteniendo el timeout y la prohibición de redirecciones.
func (a *Adyen) ConTransporte(rt http.RoundTripper) *Adyen {
	if a != nil && rt != nil {
		a.http.Transport = rt
	}
	return a
}

// CrearPago abre Hosted Checkout. Adyen comunica después el resultado definitivo
// por webhook; consultar repetidamente una sesión no lo actualiza.
//
// La `reference` es el id de nuestro pago (pagos.MetadatoPago): la notificación
// solo devuelve esa referencia, sin los metadatos, y por ella se encuentra el pago.
// Con el número de pedido, que es de cada tienda y no es único, ninguna
// notificación casaba. El pedido sigue en los metadatos.
func (a *Adyen) CrearPago(ctx context.Context, p pagos.Solicitud) (pagos.Creado, error) {
	if a == nil || p.Centimos <= 0 || p.Divisa != pagos.DivisaEUR || p.ClaveIdempotencia == "" || p.Referencia == "" || p.Caduca.Before(time.Now().Add(-time.Minute)) || p.Metadatos[pagos.MetadatoPago] == "" {
		return pagos.Creado{}, errors.New("adyen: invalid payment request")
	}
	body := map[string]any{"merchantAccount": a.merchant, "amount": map[string]any{"currency": "EUR", "value": p.Centimos}, "reference": p.Metadatos[pagos.MetadatoPago], "returnUrl": p.URLRetorno, "mode": "hosted", "expiresAt": p.Caduca.UTC().Format(time.RFC3339), "metadata": p.Metadatos}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := a.call(ctx, http.MethodPost, "/sessions", body, p.ClaveIdempotencia, &out); err != nil || out.ID == "" || !checkoutURL(out.URL, a.mode) {
		return pagos.Creado{}, errors.New("adyen: invalid session response")
	}
	return pagos.Creado{Ref: out.ID, URL: out.URL}, nil
}

// ConsultarPago no puede usar una sesión como fuente de verdad: la API de
// sesiones exige el sessionResult que entrega el navegador y su respuesta no se
// actualiza. La tienda persiste el Estado de Notificacion, cuya HMAC se verifica
// antes de cambiar el pedido. Mantener este error evita tratar una vuelta del
// navegador como pago confirmado.
func (*Adyen) ConsultarPago(context.Context, string) (pagos.Estado, error) {
	return pagos.Estado{}, errors.New("adyen: payment status is delivered by webhook")
}
func (*Adyen) Caducar(context.Context, string) error { return nil }
func (a *Adyen) Reembolsar(ctx context.Context, ref string, centimos int64, clave string) error {
	if a == nil || !pspReference(ref) || centimos <= 0 || clave == "" {
		return errors.New("adyen: invalid refund request")
	}
	var out struct {
		PSPReference string `json:"pspReference"` //nolint:tagliatelle // Contrato de Checkout API de Adyen.
	}
	if err := a.call(ctx, http.MethodPost, "/payments/"+url.PathEscape(ref)+"/refunds", map[string]any{
		"merchantAccount": a.merchant,
		"reference":       clave,
		"amount":          map[string]any{"currency": pagos.DivisaEUR, "value": centimos},
	}, clave, &out); err != nil || !pspReference(out.PSPReference) {
		return errors.New("adyen: provider unavailable")
	}
	return nil
}

func (a *Adyen) call(ctx context.Context, method, path string, input any, idem string, out any) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("adyen: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return errors.New("adyen: provider unavailable")
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return errors.New("adyen: invalid provider response")
	}
	return nil
}

func (a *Adyen) endpoint() string {
	if a.mode == pagos.ModoPrueba {
		return "https://checkout-test.adyen.com/v72"
	}
	return "https://" + a.livePrefix + "-" + a.merchant + "-checkout-live.adyenpayments.com/checkout/v72"
}

func checkoutURL(raw, mode string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if mode == pagos.ModoPrueba {
		return host == "checkoutshopper-test.adyen.com"
	}
	return host == "checkoutshopper-live.adyen.com"
}

func pspReference(v string) bool {
	if len(v) < 10 || len(v) > 80 {
		return false
	}
	for _, r := range v {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

// Notificacion es el formato Standard webhook de Adyen que necesita Pages.
// Los campos usados para la firma se conservan como texto exactamente como los
// entrega Adyen; el llamante persiste el evento de forma idempotente.
type Notificacion struct {
	PSPReference        string `json:"pspReference"`        //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
	OriginalReference   string `json:"originalReference"`   //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
	MerchantAccountCode string `json:"merchantAccountCode"` //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
	MerchantReference   string `json:"merchantReference"`   //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
	EventCode           string `json:"eventCode"`           //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
	Success             string `json:"success"`
	Amount              struct {
		Value    int64  `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	AdditionalData map[string]string `json:"additionalData"` //nolint:tagliatelle // Contrato del webhook Standard de Adyen.
}

// VerificarFirma comprueba la HMAC del Standard webhook con la clave hexadecimal
// configurada para ese endpoint. No acepta claves ni firmas mal codificadas.
func (n Notificacion) VerificarFirma(claveHex string) error {
	clave, err := hex.DecodeString(strings.TrimSpace(claveHex))
	if err != nil || len(clave) == 0 {
		return errors.New("adyen: invalid hmac key")
	}
	firma, err := base64.StdEncoding.DecodeString(n.AdditionalData["hmacSignature"])
	if err != nil || len(firma) != sha256.Size {
		return errors.New("adyen: invalid webhook signature")
	}
	payload := strings.Join([]string{n.PSPReference, n.OriginalReference, n.MerchantAccountCode, n.MerchantReference, strconv.FormatInt(n.Amount.Value, 10), n.Amount.Currency, n.EventCode, n.Success}, ":")
	h := hmac.New(sha256.New, clave)
	_, _ = h.Write([]byte(payload))
	if !hmac.Equal(firma, h.Sum(nil)) {
		return errors.New("adyen: invalid webhook signature")
	}
	return nil
}

// Estado convierte una notificación ya autenticada al contrato común. El
// comercio y el modo se validan contra la configuración que guarda la tienda.
func (n Notificacion) Estado(modo string, metadatos map[string]string) (pagos.Estado, error) {
	if !pspReference(n.PSPReference) || n.Amount.Value <= 0 || !strings.EqualFold(n.Amount.Currency, pagos.DivisaEUR) || (modo != pagos.ModoPrueba && modo != pagos.ModoReal) || n.MerchantAccountCode == "" {
		return pagos.Estado{}, errors.New("adyen: invalid webhook notification")
	}
	var estado string
	var reembolsado int64
	switch n.EventCode {
	case "AUTHORISATION":
		if n.Success == "true" {
			estado = pagos.EstadoPagado
		} else {
			estado = pagos.EstadoFallido
		}
	case "CANCELLATION", "CANCEL_OR_REFUND":
		if n.Success == "true" {
			estado = pagos.EstadoCancelado
		} else {
			estado = pagos.EstadoAbierto
		}
	case "REFUND":
		estado = pagos.EstadoPagado
		if n.Success == "true" {
			reembolsado = n.Amount.Value
		}
	default:
		return pagos.Estado{}, errors.New("adyen: unsupported webhook event")
	}
	return pagos.Estado{Estado: estado, Importe: n.Amount.Value, Reembolsado: reembolsado, Divisa: pagos.DivisaEUR, Modo: modo, Comercio: n.MerchantAccountCode, Metadatos: metadatos}, nil
}
