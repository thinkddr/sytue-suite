// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

const apiURL = "https://api.stripe.com/v1"

var errUnavailable = errors.New("stripe: provider unavailable")

// Stripe opera una cuenta de comercio. comercio se guarda junto a la pasarela
// y Verificar lo compara para impedir mezclar pagos de cuentas distintas.
type Stripe struct {
	key, comercio string
	http          *http.Client
}

var _ pagos.Pasarela = (*Stripe)(nil)

func New(key, comercio string) (*Stripe, error) {
	key, comercio = strings.TrimSpace(key), strings.TrimSpace(comercio)
	if comercio == "" || (!strings.HasPrefix(key, "sk_test_") && !strings.HasPrefix(key, "sk_live_") && !strings.HasPrefix(key, "rk_test_") && !strings.HasPrefix(key, "rk_live_")) {
		return nil, errors.New("stripe: invalid credentials")
	}
	return &Stripe{key: key, comercio: comercio, http: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (s *Stripe) CrearPago(ctx context.Context, p pagos.Solicitud) (pagos.Creado, error) {
	if s == nil || p.Centimos <= 0 || p.Divisa != pagos.DivisaEUR || p.ClaveIdempotencia == "" || p.Metadatos[pagos.MetadatoPago] == "" || !httpsURL(p.URLRetorno) || p.Caduca.Before(time.Now().Add(30*time.Minute)) || p.Caduca.After(time.Now().Add(24*time.Hour)) {
		return pagos.Creado{}, errors.New("stripe: invalid payment request")
	}
	v := url.Values{"mode": {"payment"}, "success_url": {p.URLRetorno}, "cancel_url": {p.URLRetorno}, "line_items[0][price_data][currency]": {"eur"}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(p.Centimos, 10)}, "line_items[0][price_data][product_data][name]": {truncate(p.Descripcion, 200)}, "line_items[0][quantity]": {"1"}}
	v.Set("expires_at", strconv.FormatInt(p.Caduca.Unix(), 10))
	for k, x := range p.Metadatos {
		v.Set("metadata["+k+"]", x)
	}
	var out struct{ ID, URL string }
	if err := s.call(ctx, http.MethodPost, "/checkout/sessions", v, p.ClaveIdempotencia, &out); err != nil || !checkoutID(out.ID) || !checkoutURL(out.URL) {
		return pagos.Creado{}, errUnavailable
	}
	return pagos.Creado{Ref: out.ID, URL: out.URL}, nil
}

func (s *Stripe) ConsultarPago(ctx context.Context, ref string) (pagos.Estado, error) {
	if !checkoutID(ref) {
		return pagos.Estado{}, errors.New("stripe: invalid checkout reference")
	}
	var out struct {
		PaymentStatus, Status, Currency, PaymentIntent string
		AmountTotal                                    int64             `json:"amount_total"`
		Livemode                                       bool              `json:"livemode"`
		Metadata                                       map[string]string `json:"metadata"`
	}
	if err := s.call(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(ref), nil, "", &out); err != nil {
		return pagos.Estado{}, err
	}
	estado := pagos.EstadoAbierto
	if out.PaymentStatus == "paid" {
		estado = pagos.EstadoPagado
	} else if out.Status == "expired" {
		estado = pagos.EstadoCaducado
	}
	modo := pagos.ModoPrueba
	if out.Livemode {
		modo = pagos.ModoReal
	}
	return pagos.Estado{Estado: estado, Importe: out.AmountTotal, Divisa: strings.ToUpper(out.Currency), Modo: modo, Comercio: s.comercio, Metadatos: out.Metadata}, nil
}

func (s *Stripe) Caducar(ctx context.Context, ref string) error {
	if !checkoutID(ref) {
		return errors.New("stripe: invalid checkout reference")
	}
	var out any
	return s.call(ctx, http.MethodPost, "/checkout/sessions/"+url.PathEscape(ref)+"/expire", nil, "", &out)
}

func (s *Stripe) Reembolsar(ctx context.Context, ref string, centimos int64, clave string) error {
	if s == nil || !checkoutID(ref) || centimos <= 0 || clave == "" {
		return errors.New("stripe: invalid refund request")
	}
	var session struct {
		PaymentIntent string `json:"payment_intent"`
	}
	if err := s.call(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(ref), nil, "", &session); err != nil || session.PaymentIntent == "" {
		return errUnavailable
	}
	var refund struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodPost, "/refunds", url.Values{
		"payment_intent": {session.PaymentIntent},
		"amount":         {strconv.FormatInt(centimos, 10)},
	}, clave, &refund); err != nil || refund.ID == "" {
		return errUnavailable
	}
	return nil
}

func (s *Stripe) call(ctx context.Context, method, path string, form url.Values, idem string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.key, "")
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return errUnavailable
	}
	if out != nil && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out) != nil {
		return errUnavailable
	}
	return nil
}

func httpsURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func checkoutURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.EqualFold(u.Hostname(), "checkout.stripe.com")
}

func checkoutID(v string) bool {
	if !strings.HasPrefix(v, "cs_") || len(v) < 10 || len(v) > 255 {
		return false
	}
	for _, r := range v[3:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func truncate(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) > n {
		return v[:n]
	}
	return v
}
