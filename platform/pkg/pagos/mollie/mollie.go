// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package mollie habla con la API de Mollie sin incorporar un SDK de terceros.
// El cliente solo puede hablar con el origen fijo de Mollie y limita todas las
// respuestas: menos dependencias y sin convertir URLs de entrada en SSRF.
//
// Lo usan dos: Sytue ID, para el cobro recurrente de la suite (CreateCheckout,
// GetPayment, EnsureSubscription; vivía en `services/id/internal/billing`), y la
// tienda de Sytue Web, que cobra en la cuenta de CADA organización con la clave
// que esta pega (Mollie implementa pagos.Pasarela). Nunca Mollie Connect ni
// comisiones de plataforma: el dinero no pasa por Sytue.
package mollie

import (
	"bytes"
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

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

const mollieAPI = "https://api.mollie.com/v2"

// ErrUnavailable es cualquier fallo de Mollie o de su respuesta. No lleva nada de lo
// que dijo el proveedor: ni cuerpo ni cabeceras.
var ErrUnavailable = errors.New("mollie: proveedor no disponible")

// Mollie es un cliente con UNA credencial.
type Mollie struct {
	apiKey    string
	profileID string
	baseURL   string
	http      *http.Client
}

var _ pagos.Pasarela = (*Mollie)(nil)

// New crea el cliente con una clave API (`live_…`/`test_…`). profileID es opcional:
// una clave API ya pertenece a un perfil (Perfil lo dice), y solo un token de
// organización lo necesita. Si se da, va en cada pago y en la lista de métodos.
func New(apiKey, profileID string) *Mollie {
	return &Mollie{
		apiKey: strings.TrimSpace(apiKey), profileID: strings.TrimSpace(profileID), baseURL: mollieAPI,
		http: &http.Client{
			Timeout: 12 * time.Second,
			// La API tiene un origen fijo. No seguir redirecciones impide que una
			// respuesta comprometida convierta el Bearer en una credencial saliente.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ConTransporte sustituye el transporte HTTP manteniendo el timeout y la prohibición de redirecciones.
func (m *Mollie) ConTransporte(rt http.RoundTripper) *Mollie {
	if m != nil && rt != nil {
		m.http.Transport = rt
	}
	return m
}

// Checkout es el primer pago de una suscripción de Sytue ID.
type Checkout struct {
	CustomerID  string
	PaymentID   string
	CheckoutURL string
}

// Payment es un cobro de Sytue ID tal como lo ve su webhook.
type Payment struct {
	ID             string
	Status         string
	CustomerID     string
	SubscriptionID string
	Currency       string
	AmountCents    int
	PaidAt         time.Time
}

type customerResponse struct {
	ID string `json:"id"`
}

type amount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

type paymentResponse struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	Mode           string  `json:"mode"`
	ProfileID      string  `json:"profileId"`    //nolint:tagliatelle // Contrato de la API Mollie.
	IsCancelable   bool    `json:"isCancelable"` //nolint:tagliatelle // Contrato de la API Mollie.
	Metadata       any     `json:"metadata"`
	CustomerID     string  `json:"customerId"`     //nolint:tagliatelle // Contrato de la API Mollie.
	SubscriptionID string  `json:"subscriptionId"` //nolint:tagliatelle // Contrato de la API Mollie.
	Amount         amount  `json:"amount"`
	AmountRefunded *amount `json:"amountRefunded"`    //nolint:tagliatelle // Contrato de la API Mollie.
	AmountCharged  *amount `json:"amountChargedBack"` //nolint:tagliatelle // Contrato de la API Mollie.
	PaidAt         string  `json:"paidAt"`            //nolint:tagliatelle // Contrato de la API Mollie.
	Links          struct {
		Checkout struct {
			Href string `json:"href"`
		} `json:"checkout"`
	} `json:"_links"` //nolint:tagliatelle // HAL de Mollie usa _links.
}

type subscriptionResponse struct {
	ID       string `json:"id"`
	Metadata struct {
		InitialPaymentID string `json:"initial_payment_id"`
	} `json:"metadata"`
}

type subscriptionListResponse struct {
	Embedded struct {
		Subscriptions []subscriptionResponse `json:"subscriptions"`
	} `json:"_embedded"` //nolint:tagliatelle // HAL de Mollie usa _embedded.
}

// Configured dice si hay credencial.
func (m *Mollie) Configured() bool { return m != nil && m.apiKey != "" }

// CreateCheckout crea el primer pago de una suscripción de Sytue ID (y el cliente si
// no lo había), con el importe atado al contrato de precios.
func (m *Mollie) CreateCheckout(ctx context.Context, customerID, orgID, email, name string,
	seats, amountCents int, redirectURL, webhookURL string,
) (Checkout, error) {
	if !m.Configured() {
		return Checkout{}, ErrUnavailable
	}
	if seats < 1 || !importeDeContrato(seats, amountCents) || orgID == "" {
		return Checkout{}, errors.New("mollie: contrato de precio inválido")
	}
	if customerID == "" {
		var customer customerResponse
		if err := m.doWithKey(ctx, http.MethodPost, "/customers", map[string]any{
			"name": name, "email": email,
			"metadata": map[string]string{"org_id": orgID},
		}, &customer, "sytue-customer-"+orgID); err != nil {
			return Checkout{}, err
		}
		customerID = customer.ID
	}
	if !validMollieID(customerID, "cst_") {
		return Checkout{}, fmt.Errorf("%w: id de cliente inválido", ErrUnavailable)
	}
	var payment paymentResponse
	err := m.doWithKey(ctx, http.MethodPost, "/payments", map[string]any{
		"amount":       amount{Currency: commerce.SytueOneCurrency, Value: centsValue(int64(amountCents))},
		"description":  fmt.Sprintf("Sytue One — %d usuario(s)", seats),
		"customerId":   customerID,
		"sequenceType": "first",
		"redirectUrl":  redirectURL,
		"webhookUrl":   webhookURL,
		"metadata": map[string]any{
			"org_id": orgID, "seats": seats, "unit_price_cents": commerce.ProSeatPriceCents,
		},
	}, &payment, fmt.Sprintf("sytue-first-payment-%s-%d", orgID, seats))
	if err != nil {
		return Checkout{}, err
	}
	checkoutURL, err := validateCheckoutURL(payment.Links.Checkout.Href)
	if err != nil || payment.ID == "" || customerID == "" {
		return Checkout{}, fmt.Errorf("%w: respuesta de checkout inválida", ErrUnavailable)
	}
	return Checkout{CustomerID: customerID, PaymentID: payment.ID, CheckoutURL: checkoutURL}, nil
}

// GetPayment lee un cobro de Sytue ID; un reembolso o un contracargo lo vuelven
// «refunded» o «charged_back» aunque Mollie siga diciendo «paid».
func (m *Mollie) GetPayment(ctx context.Context, id string) (Payment, error) {
	p, err := m.leerPago(ctx, id)
	if err != nil {
		return Payment{}, err
	}
	cents, err := parseCents(p.Amount.Value)
	if err != nil {
		return Payment{}, fmt.Errorf("%w: importe inválido", ErrUnavailable)
	}
	var paidAt time.Time
	if p.PaidAt != "" {
		paidAt, err = time.Parse(time.RFC3339, p.PaidAt)
		if err != nil {
			return Payment{}, fmt.Errorf("%w: fecha de pago inválida", ErrUnavailable)
		}
	}
	status := p.Status
	if !validPaymentStatus(status) {
		return Payment{}, fmt.Errorf("%w: estado de pago desconocido", ErrUnavailable)
	}
	// Mollie conserva status=paid tras ciertos reembolsos/chargebacks y expone
	// los importes revertidos aparte. Cualquier reversión suspende el derecho
	// hasta revisión; no se mantiene acceso sobre un cobro disputado.
	if reversedAmount(p.AmountCharged, p.Amount.Currency) {
		status = "charged_back"
	} else if reversedAmount(p.AmountRefunded, p.Amount.Currency) {
		status = "refunded"
	}
	return Payment{
		ID: p.ID, Status: status, CustomerID: p.CustomerID,
		SubscriptionID: p.SubscriptionID, Currency: p.Amount.Currency,
		AmountCents: cents, PaidAt: paidAt,
	}, nil
}

// EnsureSubscription recupera primero una suscripción creada previamente con el
// mismo pago inicial. Junto con Idempotency-Key evita cargos dobles incluso si el
// proceso cae tras el POST pero antes de persistir la respuesta local.
func (m *Mollie) EnsureSubscription(ctx context.Context, customerID, orgID, initialPaymentID string,
	seats, amountCents int, firstPaidAt time.Time, webhookURL string,
) (string, error) {
	if !validMollieID(customerID, "cst_") || !validMollieID(initialPaymentID, "tr_") || firstPaidAt.IsZero() {
		return "", errors.New("mollie: cliente o fecha inicial inválidos")
	}
	if seats < 1 || !importeDeContrato(seats, amountCents) {
		return "", errors.New("mollie: contrato de precio inválido")
	}
	var list subscriptionListResponse
	if err := m.do(ctx, http.MethodGet,
		"/customers/"+url.PathEscape(customerID)+"/subscriptions?limit=250", &list); err != nil {
		return "", err
	}
	for _, existing := range list.Embedded.Subscriptions {
		if existing.Metadata.InitialPaymentID == initialPaymentID && validMollieID(existing.ID, "sub_") {
			return existing.ID, nil
		}
	}
	var sub subscriptionResponse
	err := m.doWithKey(ctx, http.MethodPost, "/customers/"+url.PathEscape(customerID)+"/subscriptions", map[string]any{
		"amount":      amount{Currency: commerce.SytueOneCurrency, Value: centsValue(int64(amountCents))},
		"interval":    "1 month",
		"startDate":   addMonthClamped(firstPaidAt).Format(time.DateOnly),
		"description": fmt.Sprintf("Sytue One — %d usuario(s)", seats),
		"webhookUrl":  webhookURL,
		"metadata": map[string]any{
			"org_id": orgID, "seats": seats, "unit_price_cents": commerce.ProSeatPriceCents,
			"initial_payment_id": initialPaymentID,
		},
	}, &sub, "sytue-subscription-"+initialPaymentID)
	if err != nil {
		return "", err
	}
	if !validMollieID(sub.ID, "sub_") {
		return "", fmt.Errorf("%w: id de suscripción inválido", ErrUnavailable)
	}
	return sub.ID, nil
}

// CrearPago abre un pago de la tienda en la cuenta de la organización. La caducidad
// de la Solicitud no se manda: Mollie caduca solo los pagos abiertos (minutos, según
// el método) y el barrido de la tienda llama a Caducar al vencer la reserva.
// Referencia tampoco: Mollie no la pide y el pedido ya va en la descripción.
func (m *Mollie) CrearPago(ctx context.Context, s pagos.Solicitud) (pagos.Creado, error) {
	if s.Centimos <= 0 || s.Centimos > maxCentimos || s.Divisa != pagos.DivisaEUR || s.ClaveIdempotencia == "" ||
		strings.TrimSpace(s.Descripcion) == "" ||
		s.Metadatos[pagos.MetadatoPago] == "" || !urlHTTPS(s.URLRetorno) || !urlHTTPS(s.URLWebhook) {
		return pagos.Creado{}, errors.New("mollie: solicitud de pago incompleta")
	}
	cuerpo := map[string]any{
		"amount":      amount{Currency: s.Divisa, Value: centsValue(s.Centimos)},
		"description": validation.TruncateRunes(s.Descripcion, 255),
		"redirectUrl": s.URLRetorno,
		"webhookUrl":  s.URLWebhook,
		"metadata":    s.Metadatos,
	}
	if l := locales[s.Idioma]; l != "" {
		cuerpo["locale"] = l
	}
	if s.Metodo != "" {
		cuerpo["method"] = s.Metodo
	}
	if m.profileID != "" {
		cuerpo["profileId"] = m.profileID
	}
	var p paymentResponse
	if err := m.doWithKey(ctx, http.MethodPost, "/payments", cuerpo, &p, s.ClaveIdempotencia); err != nil {
		return pagos.Creado{}, err
	}
	checkoutURL, err := validateCheckoutURL(p.Links.Checkout.Href)
	if err != nil || !validMollieID(p.ID, "tr_") {
		return pagos.Creado{}, fmt.Errorf("%w: respuesta de pago inválida", ErrUnavailable)
	}
	return pagos.Creado{Ref: p.ID, URL: checkoutURL}, nil
}

// ConsultarPago cuenta un pago como pagos.Estado: el estado tal cual, y lo
// reembolsado y lo contracargado en sus importes. Comercio es el perfil del pago.
func (m *Mollie) ConsultarPago(ctx context.Context, ref string) (pagos.Estado, error) {
	p, err := m.leerPago(ctx, ref)
	if err != nil {
		return pagos.Estado{}, err
	}
	importe, err := parseCents(p.Amount.Value)
	if err != nil || !validPaymentStatus(p.Status) || (p.Mode != pagos.ModoPrueba && p.Mode != pagos.ModoReal) {
		return pagos.Estado{}, fmt.Errorf("%w: respuesta de pago inválida", ErrUnavailable)
	}
	reembolsado, err1 := importeOpcional(p.AmountRefunded, p.Amount.Currency)
	contracargado, err2 := importeOpcional(p.AmountCharged, p.Amount.Currency)
	if err1 != nil || err2 != nil {
		return pagos.Estado{}, fmt.Errorf("%w: importe inválido", ErrUnavailable)
	}
	return pagos.Estado{
		Estado: p.Status, Importe: int64(importe), Reembolsado: reembolsado, Contracargado: contracargado,
		Divisa: p.Amount.Currency, Modo: p.Mode, Comercio: p.ProfileID, Metadatos: soloTextos(p.Metadata),
	}, nil
}

// Caducar cancela el pago si Mollie dice que aún se puede (isCancelable). Muchos
// métodos no lo permiten con el pago abierto: esos caducan solos y no es un error.
func (m *Mollie) Caducar(ctx context.Context, ref string) error {
	p, err := m.leerPago(ctx, ref)
	if err != nil || !p.IsCancelable {
		return err
	}
	var cancelado paymentResponse
	return m.do(ctx, http.MethodDelete, "/payments/"+url.PathEscape(ref), &cancelado)
}

// Reembolsar devuelve `centimos` del pago. La clave de idempotencia es obligatoria:
// dos clics o dos administradores son un solo reembolso.
func (m *Mollie) Reembolsar(ctx context.Context, ref string, centimos int64, clave string) error {
	if !validMollieID(ref, "tr_") || centimos <= 0 || centimos > maxCentimos || clave == "" {
		return errors.New("mollie: reembolso inválido")
	}
	var r struct {
		ID string `json:"id"`
	}
	// ponytail: EUR fijo, la única divisa de la tienda (store_settings.currency).
	if err := m.doWithKey(ctx, http.MethodPost, "/payments/"+url.PathEscape(ref)+"/refunds", map[string]any{
		"amount": amount{Currency: pagos.DivisaEUR, Value: centsValue(centimos)},
	}, &r, clave); err != nil {
		return err
	}
	if !validMollieID(r.ID, "re_") {
		return fmt.Errorf("%w: respuesta de reembolso inválida", ErrUnavailable)
	}
	return nil
}

// Metodo es un método de pago activo en el perfil.
type Metodo struct {
	ID          string // ideal, creditcard, bizum…
	Descripcion string // el nombre que da Mollie, en inglés
}

// ListMethods lista los métodos activos: prueba la clave al guardarla y da las
// opciones que la tienda enseña al comprador.
func (m *Mollie) ListMethods(ctx context.Context) ([]Metodo, error) {
	ruta := "/methods"
	if m.profileID != "" {
		ruta += "?profileId=" + url.QueryEscape(m.profileID)
	}
	var r struct {
		Embedded struct {
			Methods []struct {
				ID          string `json:"id"`
				Description string `json:"description"`
			} `json:"methods"`
		} `json:"_embedded"` //nolint:tagliatelle // HAL de Mollie usa _embedded.
	}
	if err := m.do(ctx, http.MethodGet, ruta, &r); err != nil {
		return nil, err
	}
	out := make([]Metodo, 0, len(r.Embedded.Methods))
	for _, x := range r.Embedded.Methods {
		if x.ID == "" || len(x.ID) > 40 {
			return nil, fmt.Errorf("%w: método inválido", ErrUnavailable)
		}
		out = append(out, Metodo{ID: x.ID, Descripcion: validation.TruncateRunes(x.Description, 100)})
	}
	return out, nil
}

// Perfil dice a qué perfil pertenece la clave (`pfl_…`): es el comercio que la tienda
// guarda con la pasarela y que Verificar exige en cada pago.
func (m *Mollie) Perfil(ctx context.Context) (string, error) {
	if m.profileID != "" {
		return m.profileID, nil
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := m.do(ctx, http.MethodGet, "/profiles/me", &r); err != nil {
		return "", err
	}
	if !validMollieID(r.ID, "pfl_") {
		return "", fmt.Errorf("%w: perfil inválido", ErrUnavailable)
	}
	return r.ID, nil
}

func (m *Mollie) leerPago(ctx context.Context, ref string) (paymentResponse, error) {
	if !validMollieID(ref, "tr_") {
		return paymentResponse{}, errors.New("mollie: identificador de pago inválido")
	}
	var p paymentResponse
	if err := m.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(ref), &p); err != nil {
		return paymentResponse{}, err
	}
	if p.ID != ref {
		return paymentResponse{}, fmt.Errorf("%w: respuesta de otro pago", ErrUnavailable)
	}
	return p, nil
}

// maxCentimos es el tope de parseCents (10 millones de euros): lo que no se puede
// leer de vuelta tampoco se manda.
const maxCentimos = 10_000_000 * 100

// locales son los de la página de pago de Mollie para los idiomas de las webs.
var locales = map[string]string{
	"es": "es_ES", "en": "en_US", "ca": "ca_ES", "de": "de_DE", "fr": "fr_FR",
	"it": "it_IT", "nl": "nl_NL", "pt": "pt_PT", "pl": "pl_PL",
}

func importeOpcional(a *amount, divisa string) (int64, error) {
	if a == nil {
		return 0, nil
	}
	if a.Currency != divisa {
		return 0, errors.New("divisa distinta")
	}
	c, err := parseCents(a.Value)
	return int64(c), err
}

// soloTextos se queda con los metadatos de texto: los nuestros son todos texto.
func soloTextos(v any) map[string]string {
	m, _ := v.(map[string]any)
	out := make(map[string]string, len(m))
	for k, x := range m {
		if t, ok := x.(string); ok {
			out[k] = t
		}
	}
	return out
}

func urlHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func (m *Mollie) do(ctx context.Context, method, path string, out any) error {
	return m.doWithKey(ctx, method, path, nil, out, "")
}

func (m *Mollie) doWithKey(ctx context.Context, method, path string, body, out any, idempotencyKey string) error {
	if !m.Configured() {
		return ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Accept", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: respuesta inválida", ErrUnavailable)
	}
	return nil
}

func validPaymentStatus(status string) bool {
	switch status {
	case "open", "pending", "authorized", "paid", "failed", "canceled", "expired":
		return true
	default:
		return false
	}
}

func reversedAmount(a *amount, currency string) bool {
	if a == nil || a.Currency != currency {
		return false
	}
	cents, err := parseCents(a.Value)
	return err == nil && cents > 0
}

func centsValue(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// addMonthClamped coincide con PostgreSQL `timestamp + interval '1 month'`:
// 31 de enero pasa al último día de febrero, no a marzo como time.AddDate.
// Mantener ambas fechas alineadas evita un hueco de acceso entre el primer pago
// y el primer cobro recurrente.
func addMonthClamped(t time.Time) time.Time {
	t = t.UTC()
	year, month, day := t.Date()
	firstNext := time.Date(year, month+1, 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	lastNext := firstNext.AddDate(0, 1, -1).Day()
	if day > lastNext {
		day = lastNext
	}
	return time.Date(firstNext.Year(), firstNext.Month(), day,
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

func parseCents(value string) (int, error) {
	whole, fraction, ok := strings.Cut(value, ".")
	if !ok || len(fraction) != 2 || whole == "" || strings.HasPrefix(whole, "-") {
		return 0, errors.New("importe inválido")
	}
	euros, err := strconv.Atoi(whole)
	if err != nil {
		return 0, err
	}
	cents, err := strconv.Atoi(fraction)
	if err != nil || cents < 0 || cents > 99 || euros > 10_000_000 {
		return 0, errors.New("importe inválido")
	}
	return euros*100 + cents, nil
}

func validateCheckoutURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") ||
		!strings.HasSuffix(strings.ToLower(u.Hostname()), ".mollie.com") {
		return "", errors.New("mollie: URL de checkout no confiable")
	}
	return u.String(), nil
}

func validMollieID(id, prefix string) bool {
	if !strings.HasPrefix(id, prefix) || len(id) < len(prefix)+6 || len(id) > 80 {
		return false
	}
	for _, r := range id[len(prefix):] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// importeDeContrato comprueba que el importe sea EXACTAMENTE asientos × el
// precio de alguno de los planes de pago (commerce): la pasarela conoce
// importes, no planes, y es la última defensa contra un checkout manipulado.
func importeDeContrato(seats, amountCents int) bool {
	return seats > 0 && amountCents%seats == 0 && commerce.ValidSeatPriceCents(amountCents/seats)
}
