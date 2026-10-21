// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mollie

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
)

func TestParseCentsExact(t *testing.T) {
	for in, want := range map[string]int{"6.00": 600, "60.12": 6012, "0.01": 1} {
		got, err := parseCents(in)
		if err != nil || got != want {
			t.Fatalf("parseCents(%q)=%d,%v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"6", "6.0", "6.000", "-6.00", "x.00"} {
		if _, err := parseCents(in); err == nil {
			t.Fatalf("parseCents(%q) debía fallar", in)
		}
	}
}

func TestCheckoutURLClosedToMollie(t *testing.T) {
	for _, raw := range []string{
		"https://checkout.mollie.com/checkout/test", "https://www.mollie.com/pay/test",
	} {
		if _, err := validateCheckoutURL(raw); err != nil {
			t.Fatalf("URL Mollie válida %q: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://checkout.mollie.com/x", "https://mollie.com.evil.example/x", "https://mollie.com/x",
		"https://user:pass@checkout.mollie.com/x", "https://checkout.mollie.com:8443/x",
	} {
		if _, err := validateCheckoutURL(raw); err == nil {
			t.Fatalf("URL no confiable aceptada: %q", raw)
		}
	}
}

func TestCreateCheckoutPinsPriceAndIdempotency(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/customers":
			if got := r.Header.Get("Idempotency-Key"); got != "sytue-customer-org-123456" {
				t.Errorf("customer Idempotency-Key=%q", got)
			}
			_, _ = w.Write([]byte(`{"id":"cst_123456"}`))
		case "/payments":
			if got := r.Header.Get("Idempotency-Key"); got != "sytue-first-payment-org-123456-2" {
				t.Errorf("payment Idempotency-Key=%q", got)
			}
			_, _ = w.Write([]byte(`{
				"id":"tr_123456",
				"_links":{"checkout":{"href":"https://checkout.mollie.com/checkout/test"}}
			}`))
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	m := New("live_test", "")
	m.baseURL = srv.URL
	checkout, err := m.CreateCheckout(context.Background(), "", "org-123456", "a@example.eu", "A", 2, 1000,
		"https://one.sytue.eu/return", "https://id.sytue.eu/webhook")
	if err != nil || checkout.CustomerID != "cst_123456" || checkout.PaymentID != "tr_123456" {
		t.Fatalf("CreateCheckout=%+v,%v", checkout, err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
	if _, err := m.CreateCheckout(context.Background(), "cst_123456", "org-123456", "a@example.eu", "A", 2, 1199,
		"https://one.sytue.eu/return", "https://id.sytue.eu/webhook"); err == nil {
		t.Fatal("un importe distinto de 6 EUR por asiento debe rechazarse")
	}
}

func TestAddMonthClamped(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"2026-01-31T12:00:00Z", "2026-02-28T12:00:00Z"},
		{"2024-01-31T12:00:00Z", "2024-02-29T12:00:00Z"},
		{"2026-05-30T12:00:00Z", "2026-06-30T12:00:00Z"},
	} {
		in, _ := time.Parse(time.RFC3339, tc.in)
		if got := addMonthClamped(in).Format(time.RFC3339); got != tc.want {
			t.Errorf("addMonthClamped(%s)=%s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestGetPaymentTreatsRefundAsReversal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer live_test" {
			t.Error("falta autorización")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"id":"tr_123456","status":"paid","customerId":"cst_123456",
			"amount":{"currency":"EUR","value":"6.00"},
			"amountRefunded":{"currency":"EUR","value":"1.00"},
			"paidAt":"2026-08-01T12:00:00Z"
		}`))
	}))
	defer srv.Close()
	m := New("live_test", "")
	m.baseURL = srv.URL
	p, err := m.GetPayment(context.Background(), "tr_123456")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "refunded" {
		t.Fatalf("status=%q, want refunded", p.Status)
	}
}

func TestEnsureSubscriptionUsesPersistentMarkerAndIdempotencyKey(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/subscriptions"):
			_, _ = w.Write([]byte(`{"_embedded":{"subscriptions":[]}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/subscriptions"):
			if got := r.Header.Get("Idempotency-Key"); got != "sytue-subscription-tr_123456" {
				t.Errorf("Idempotency-Key=%q", got)
				http.Error(w, "bad idempotency key", http.StatusBadRequest)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("body inválido: %v", err)
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			metadata, _ := body["metadata"].(map[string]any)
			if metadata["initial_payment_id"] != "tr_123456" {
				t.Errorf("metadata sin marcador persistente: %#v", metadata)
				http.Error(w, "bad metadata", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"id":"sub_123456"}`))
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	m := New("live_test", "")
	m.baseURL = srv.URL
	id, err := m.EnsureSubscription(context.Background(), "cst_123456", "org", "tr_123456",
		1, 500, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), "https://id.sytue.eu/billing/webhooks/mollie")
	if err != nil || id != "sub_123456" {
		t.Fatalf("EnsureSubscription=%q,%v", id, err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want GET+POST", calls)
	}
}

func TestEnsureSubscriptionReusesProviderRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("no debe crear otra suscripción: %s", r.Method)
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"_embedded":{"subscriptions":[{
			"id":"sub_existing","metadata":{"initial_payment_id":"tr_123456"}
		}]}}`))
	}))
	defer srv.Close()
	m := New("live_test", "")
	m.baseURL = srv.URL
	id, err := m.EnsureSubscription(context.Background(), "cst_123456", "org", "tr_123456",
		1, 500, time.Now(), "https://id.sytue.eu/billing/webhooks/mollie")
	if err != nil || id != "sub_existing" {
		t.Fatalf("EnsureSubscription=%q,%v", id, err)
	}
}

// servidor falso de Mollie: cada ruta «MÉTODO /ruta» responde con su JSON y apunta la
// petición (cuerpo incluido) para que el test la mire.
type falso struct {
	pedidas []string
	cuerpos map[string]map[string]any
	claves  map[string]string
}

func nuevoFalso(t *testing.T, rutas map[string]string) (*falso, *Mollie) {
	t.Helper()
	f := &falso{cuerpos: map[string]map[string]any{}, claves: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := r.Method + " " + r.URL.RequestURI()
		f.pedidas = append(f.pedidas, k)
		f.claves[k] = r.Header.Get("Idempotency-Key")
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			var c map[string]any
			if err := json.Unmarshal(b, &c); err != nil {
				t.Errorf("%s: cuerpo que no es JSON: %s", k, b)
			}
			f.cuerpos[k] = c
		}
		if r.Header.Get("Authorization") != "Bearer test_clave" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp, ok := rutas[k]
		if !ok {
			http.Error(w, "unexpected "+k, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	m := New("test_clave", "")
	m.baseURL = srv.URL
	return f, m
}

func solicitud() pagos.Solicitud {
	return pagos.Solicitud{
		Centimos: 2910, Divisa: pagos.DivisaEUR, Descripcion: "Pedido P-000123 · Panadería Aurora",
		Referencia: "P-000123", ClaveIdempotencia: "pages-pay1-1-2910",
		URLRetorno: "https://aurora.sites.sytue.eu/es/order/?id=o1", URLWebhook: "https://web.sytue.eu/webhooks/mollie/pay1",
		Idioma: "es", Metadatos: map[string]string{pagos.MetadatoPago: "pay1"},
	}
}

// CrearPago manda el importe del servidor, la clave de idempotencia (con el importe
// dentro) y nuestro id en los metadatos; y solo devuelve una URL de mollie.com.
func TestCrearPagoMandaLoQueToca(t *testing.T) {
	f, m := nuevoFalso(t, map[string]string{
		"POST /payments": `{"id":"tr_abc123","_links":{"checkout":{"href":"https://www.mollie.com/checkout/select-method/abc"}}}`,
	})
	c, err := m.CrearPago(context.Background(), solicitud())
	if err != nil || c.Ref != "tr_abc123" || c.URL != "https://www.mollie.com/checkout/select-method/abc" || c.Formulario != nil {
		t.Fatalf("CrearPago = %+v, %v", c, err)
	}
	if got := f.claves["POST /payments"]; got != "pages-pay1-1-2910" {
		t.Errorf("Idempotency-Key = %q", got)
	}
	cuerpo := f.cuerpos["POST /payments"]
	importe, _ := cuerpo["amount"].(map[string]any)
	meta, _ := cuerpo["metadata"].(map[string]any)
	if importe["value"] != "29.10" || importe["currency"] != "EUR" || meta[pagos.MetadatoPago] != "pay1" ||
		cuerpo["locale"] != "es_ES" || cuerpo["webhookUrl"] != "https://web.sytue.eu/webhooks/mollie/pay1" {
		t.Errorf("cuerpo del pago: %v", cuerpo)
	}
	if _, hay := cuerpo["profileId"]; hay {
		t.Error("con una clave API no se manda profileId")
	}

	malas := map[string]func(s *pagos.Solicitud){
		"sin clave":         func(s *pagos.Solicitud) { s.ClaveIdempotencia = "" },
		"otra divisa":       func(s *pagos.Solicitud) { s.Divisa = "USD" },
		"importe cero":      func(s *pagos.Solicitud) { s.Centimos = 0 },
		"sin nuestro id":    func(s *pagos.Solicitud) { s.Metadatos = nil },
		"retorno en claro":  func(s *pagos.Solicitud) { s.URLRetorno = "http://aurora.sites.sytue.eu/es/order/" },
		"sin webhook":       func(s *pagos.Solicitud) { s.URLWebhook = "" },
		"sin descripción":   func(s *pagos.Solicitud) { s.Descripcion = " " },
		"importe imposible": func(s *pagos.Solicitud) { s.Centimos = maxCentimos + 1 },
	}
	antes := len(f.pedidas)
	for nombre, cambiar := range malas {
		s := solicitud()
		cambiar(&s)
		if _, err := m.CrearPago(context.Background(), s); err == nil {
			t.Errorf("%s: se crea el pago", nombre)
		}
	}
	if len(f.pedidas) != antes {
		t.Errorf("una solicitud mala llega a Mollie: %v", f.pedidas[antes:])
	}

	_, m2 := nuevoFalso(t, map[string]string{
		"POST /payments": `{"id":"tr_abc123","_links":{"checkout":{"href":"https://mollie.com.evil.example/x"}}}`,
	})
	if _, err := m2.CrearPago(context.Background(), solicitud()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("una URL de pago fuera de mollie.com se acepta: %v", err)
	}
}

// Con perfil (token de organización) el perfil va en el pago y en la lista de métodos.
func TestConPerfilVaEnElPagoYEnLosMetodos(t *testing.T) {
	f, m := nuevoFalso(t, map[string]string{
		"POST /payments":                    `{"id":"tr_abc123","_links":{"checkout":{"href":"https://www.mollie.com/c/abc"}}}`,
		"GET /methods?profileId=pfl_abc123": `{"_embedded":{"methods":[{"id":"ideal","description":"iDEAL"},{"id":"bizum","description":"Bizum"}]}}`,
	})
	m.profileID = "pfl_abc123"
	if _, err := m.CrearPago(context.Background(), solicitud()); err != nil {
		t.Fatal(err)
	}
	if f.cuerpos["POST /payments"]["profileId"] != "pfl_abc123" {
		t.Errorf("sin profileId: %v", f.cuerpos["POST /payments"])
	}
	metodos, err := m.ListMethods(context.Background())
	if err != nil || len(metodos) != 2 || metodos[1] != (Metodo{ID: "bizum", Descripcion: "Bizum"}) {
		t.Fatalf("ListMethods = %+v, %v", metodos, err)
	}
	if p, err := m.Perfil(context.Background()); err != nil || p != "pfl_abc123" {
		t.Fatalf("Perfil con perfil dado = %q, %v", p, err)
	}
}

// Perfil pregunta a Mollie de quién es la clave, y no se fía de un id raro.
func TestPerfilDeLaClave(t *testing.T) {
	_, m := nuevoFalso(t, map[string]string{"GET /profiles/me": `{"id":"pfl_QkEhN94Ba"}`})
	if p, err := m.Perfil(context.Background()); err != nil || p != "pfl_QkEhN94Ba" {
		t.Fatalf("Perfil = %q, %v", p, err)
	}
	_, m2 := nuevoFalso(t, map[string]string{"GET /profiles/me": `{"id":"pfl_../x"}`})
	if _, err := m2.Perfil(context.Background()); err == nil {
		t.Fatal("un perfil con otra forma se acepta")
	}
	m3 := New("test_mala", "")
	m3.baseURL = m.baseURL
	if _, err := m3.Perfil(context.Background()); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("una clave mala: %v (sin nada de lo que diga Mollie)", err)
	}
}

// ConsultarPago cuenta el pago en los términos comunes y Verificar lo ata a lo
// esperado: el mismo pago en otro perfil o en otro modo no casa.
func TestConsultarPagoYVerificar(t *testing.T) {
	_, m := nuevoFalso(t, map[string]string{
		"GET /payments/tr_abc123": `{"id":"tr_abc123","status":"paid","mode":"test","profileId":"pfl_abc123",
			"amount":{"currency":"EUR","value":"29.10"},"amountRefunded":{"currency":"EUR","value":"5.00"},
			"amountChargedBack":{"currency":"EUR","value":"1.00"},
			"metadata":{"sytue_payment_id":"pay1","n":3}}`,
		"GET /payments/tr_otro123": `{"id":"tr_abc123","status":"paid","mode":"test","amount":{"currency":"EUR","value":"1.00"}}`,
		"GET /payments/tr_modo123": `{"id":"tr_modo123","status":"paid","mode":"sandbox","amount":{"currency":"EUR","value":"1.00"}}`,
	})
	e, err := m.ConsultarPago(context.Background(), "tr_abc123")
	quiero := pagos.Estado{
		Estado: pagos.EstadoPagado, Importe: 2910, Reembolsado: 500, Contracargado: 100, Divisa: "EUR",
		Modo: pagos.ModoPrueba, Comercio: "pfl_abc123", Metadatos: map[string]string{pagos.MetadatoPago: "pay1"},
	}
	if err != nil || e.Estado != quiero.Estado || e.Importe != quiero.Importe || e.Reembolsado != quiero.Reembolsado ||
		e.Contracargado != quiero.Contracargado || e.Modo != quiero.Modo || e.Comercio != quiero.Comercio ||
		len(e.Metadatos) != 1 || e.Metadatos[pagos.MetadatoPago] != "pay1" {
		t.Fatalf("ConsultarPago = %+v, %v", e, err)
	}
	esperado := pagos.Esperado{Pago: "pay1", Importe: 2910, Modo: pagos.ModoPrueba, Comercio: "pfl_abc123"}
	if err := pagos.Verificar(esperado, e); err != nil {
		t.Fatalf("el pago esperado no verifica: %v", err)
	}
	esperado.Comercio = "pfl_otro"
	if err := pagos.Verificar(esperado, e); !errors.Is(err, pagos.ErrNoCasa) {
		t.Fatalf("el pago de otro perfil verifica: %v", err)
	}
	if _, err := m.ConsultarPago(context.Background(), "tr_otro123"); err == nil {
		t.Fatal("una respuesta de otro pago se acepta")
	}
	if _, err := m.ConsultarPago(context.Background(), "tr_modo123"); err == nil {
		t.Fatal("un modo desconocido se acepta")
	}
	if _, err := m.ConsultarPago(context.Background(), "tr_../../x"); err == nil {
		t.Fatal("una referencia con otra forma se consulta")
	}
}

// Caducar solo cancela lo que Mollie dice que se puede cancelar.
func TestCaducarSoloLoCancelable(t *testing.T) {
	f, m := nuevoFalso(t, map[string]string{
		"GET /payments/tr_abierto1":      `{"id":"tr_abierto1","status":"open","isCancelable":false}`,
		"GET /payments/tr_cancelable":    `{"id":"tr_cancelable","status":"open","isCancelable":true}`,
		"DELETE /payments/tr_cancelable": `{"id":"tr_cancelable","status":"canceled"}`,
	})
	if err := m.Caducar(context.Background(), "tr_abierto1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Caducar(context.Background(), "tr_cancelable"); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /payments/tr_abierto1", "GET /payments/tr_cancelable", "DELETE /payments/tr_cancelable"}
	if strings.Join(f.pedidas, ",") != strings.Join(want, ",") {
		t.Fatalf("peticiones = %v, quiero %v", f.pedidas, want)
	}
}

// Reembolsar lleva la clave de idempotencia y el importe exacto; sin clave no sale.
func TestReembolsarConClave(t *testing.T) {
	f, m := nuevoFalso(t, map[string]string{"POST /payments/tr_abc123/refunds": `{"id":"re_4qqhO89gsT"}`})
	if err := m.Reembolsar(context.Background(), "tr_abc123", 1250, "pages-refund-r1"); err != nil {
		t.Fatal(err)
	}
	k := "POST /payments/tr_abc123/refunds"
	importe, _ := f.cuerpos[k]["amount"].(map[string]any)
	if f.claves[k] != "pages-refund-r1" || importe["value"] != "12.50" || importe["currency"] != "EUR" {
		t.Fatalf("reembolso: clave %q, cuerpo %v", f.claves[k], f.cuerpos[k])
	}
	for _, c := range []struct {
		ref   string
		cents int64
		clave string
	}{{"tr_abc123", 1250, ""}, {"tr_abc123", 0, "k"}, {"re_abc123", 1, "k"}} {
		if err := m.Reembolsar(context.Background(), c.ref, c.cents, c.clave); err == nil {
			t.Errorf("reembolso %+v aceptado", c)
		}
	}
	if len(f.pedidas) != 1 {
		t.Fatalf("un reembolso malo llega a Mollie: %v", f.pedidas)
	}
}
