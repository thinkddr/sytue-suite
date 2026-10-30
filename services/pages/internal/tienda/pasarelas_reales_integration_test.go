// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/adyen"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/redsys"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// transporteEnrutado redirige las peticiones HTTPS dirigidas a los dominios reales
// de las pasarelas (checkout-test.adyen.com, sis-t.redsys.es:25443, api.mollie.com,
// api.stripe.com) a un http.Handler de prueba sin rebajar ninguna validación de URL,
// esquema ni dominio en los clientes de pasarela.
type transporteEnrutado struct {
	manejador http.Handler
}

func (tr transporteEnrutado) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	tr.manejador.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

func sembrarPedidoTienda(
	t *testing.T,
	ctx context.Context,
	super *pg.Pool,
	kc *keycrypto.Cipher,
	proveedor string,
	creds map[string]string,
	noticeAccepted bool,
) (*Servicio, *domain.SitioCompleto, *domain.Gateway, uuid.UUID, string, uuid.UUID) {
	t.Helper()
	st := store.New(super)
	svc := Nuevo(st, kc, "https://web.example.test", "sites.example.test", nil, nil)

	var sitioID uuid.UUID
	if err := super.QueryRow(ctx,
		`INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ('org', $1, 'Tienda', '{}', 'sub') RETURNING id`,
		"tienda-"+proveedor,
	).Scan(&sitioID); err != nil {
		t.Fatalf("insertar sitio: %v", err)
	}

	gw, err := svc.ProbarYGuardarPasarela(ctx, "org", sitioID, "admin-1", proveedor, "test", creds, nil, noticeAccepted)
	if err != nil {
		t.Fatalf("ProbarYGuardarPasarela(%s): %v", proveedor, err)
	}

	const tokenPlano = "token-secreto-del-pedido-0123456789"
	hash := sha256.Sum256([]byte(tokenPlano))
	var pedidoID, pagoID uuid.UUID
	if err := super.QueryRow(ctx, `
		INSERT INTO orders (
			tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown,
			buyer_email, buyer_name, shipping_method, language, terms_version,
			terms_accepted_at, reserved_until
		) VALUES (
			'org', $1, 'P-000101', true, gen_random_uuid(), $2, 'pending_payment',
			2500, 0, 2500, 434, '{"21.00":434}', 'comprador@example.test', 'Comprador',
			'{"id":"std","name":{"es":"Estándar"}}', 'es', 1, now(), now() + interval '35 minutes'
		) RETURNING id`, sitioID, hash[:],
	).Scan(&pedidoID); err != nil {
		t.Fatalf("insertar pedido: %v", err)
	}

	if err := super.QueryRow(ctx, `
		INSERT INTO payments (
			tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key
		) VALUES (
			'org', $1, $2, 1, $3, '', 'created', 2500, $4
		) RETURNING id`, pedidoID, gw.ID, proveedor, fmt.Sprintf("pages-1-2500-%s", proveedor),
	).Scan(&pagoID); err != nil {
		t.Fatalf("insertar pago: %v", err)
	}

	sitio := &domain.SitioCompleto{
		ID: sitioID, Tenant: "org", Slug: "tienda-" + proveedor,
	}
	return svc, sitio, gw, pedidoID, tokenPlano, pagoID
}

func TestPasarelaRealAdyenFlujoCompletoYReembolso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}

	const hmacHex = "44782DEF547AAA06C910C43932B1EB0C71FC68D9D0C057550C48EC2ACF6BA056"
	svc, sitio, gw, pedidoID, tokenPedido, pagoID := sembrarPedidoTienda(t, ctx, super, kc, "adyen", map[string]string{
		"api_key":          "AQEyhmfxLI3Na1w0m/n3Q5qf3VaY9UCJ",
		"merchant_account": "TiendaECOM",
		"hmac_key":         hmacHex,
	}, false)

	var mu sync.Mutex
	var sesionesCreadas int
	var reembolsosAdyen []string

	svc.client = &http.Client{
		Transport: transporteEnrutado{manejador: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host != "checkout-test.adyen.com" || r.Header.Get("X-API-Key") != "AQEyhmfxLI3Na1w0m/n3Q5qf3VaY9UCJ" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/v72/sessions":
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decodificar /sessions: %v", err)
				}
				if body["merchantAccount"] != "TiendaECOM" || body["reference"] != pagoID.String() {
					t.Errorf("/sessions merchant/reference inesperados: %#v", body)
				}
				mu.Lock()
				sesionesCreadas++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":  "CS1234567890ABCDEF",
					"url": "https://checkoutshopper-test.adyen.com/checkout?sessionId=CS1234567890ABCDEF",
				})
			case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v72/payments/") && strings.HasSuffix(r.URL.Path, "/refunds"):
				pspRef := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v72/payments/"), "/refunds")
				// En Adyen el endpoint /payments/{paymentPspReference}/refunds exige el
				// pspReference de AUTHORISATION (16 dígitos), nunca el id de sesión CS...
				if strings.HasPrefix(pspRef, "CS") {
					http.Error(w, `{"status":422,"errorCode":"167","message":"Original pspReference required for this operation"}`, http.StatusUnprocessableEntity)
					return
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				amt, _ := body["amount"].(map[string]any)
				mu.Lock()
				reembolsosAdyen = append(reembolsosAdyen, fmt.Sprintf("%s|%v|%s", pspRef, amt["value"], r.Header.Get("Idempotency-Key")))
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"pspReference": "9999888877776666",
					"status":       "received",
				})
			default:
				http.Error(w, "not found", http.StatusNotFound)
			}
		})},
	}

	// 1. Iniciar pago contra el servidor simulado de Adyen.
	payResp, err := svc.Pagar(ctx, "org", sitio, pagoID, tokenPedido, "tienda.sites.example.test")
	if err != nil {
		t.Fatalf("Pagar(Adyen): %v", err)
	}
	if !strings.HasPrefix(payResp.URL, "https://checkoutshopper-test.adyen.com/") || sesionesCreadas != 1 {
		t.Fatalf("respuesta Pagar(Adyen) = %+v (sesiones=%d)", payResp, sesionesCreadas)
	}

	firmarAdyen := func(n *adyen.Notificacion) {
		k, _ := hex.DecodeString(hmacHex)
		mac := hmac.New(sha256.New, k)
		payload := strings.Join([]string{
			n.PSPReference, n.OriginalReference, n.MerchantAccountCode, n.MerchantReference,
			strconv.FormatInt(n.Amount.Value, 10), n.Amount.Currency, n.EventCode, n.Success,
		}, ":")
		mac.Write([]byte(payload))
		n.AdditionalData = map[string]string{"hmacSignature": base64.StdEncoding.EncodeToString(mac.Sum(nil))}
	}
	baseNotif := func() adyen.Notificacion {
		var n adyen.Notificacion
		n.PSPReference = "7914073381342284"
		n.MerchantAccountCode = "TiendaECOM"
		n.MerchantReference = pagoID.String()
		n.EventCode = "AUTHORISATION"
		n.Success = "true"
		n.Amount.Value = 2500
		n.Amount.Currency = "EUR"
		firmarAdyen(&n)
		return n
	}
	estadoPedido := func() string {
		var s string
		if err := super.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, pedidoID).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	estadoPago := func(id uuid.UUID) (string, string) {
		var st, ref string
		if err := super.QueryRow(ctx, `SELECT status, provider_ref FROM payments WHERE id = $1`, id).Scan(&st, &ref); err != nil {
			t.Fatal(err)
		}
		return st, ref
	}

	// 2. Rechazos fail-closed ante payloads manipulados o que no casan.
	malFirma := baseNotif()
	malFirma.AdditionalData = map[string]string{"hmacSignature": base64.StdEncoding.EncodeToString(make([]byte, 32))}
	if err := svc.ProcesarWebhookAdyen(ctx, malFirma); err == nil {
		t.Fatal("Adyen: aceptó firma HMAC inválida")
	}

	otroImporte := baseNotif()
	otroImporte.Amount.Value = 100
	firmarAdyen(&otroImporte)
	if err := svc.ProcesarWebhookAdyen(ctx, otroImporte); !errors.Is(err, pagos.ErrNoCasa) {
		t.Fatalf("Adyen otro importe: esperaba ErrNoCasa, obtuve %v", err)
	}

	otraDivisa := baseNotif()
	otraDivisa.Amount.Currency = "USD"
	firmarAdyen(&otraDivisa)
	if err := svc.ProcesarWebhookAdyen(ctx, otraDivisa); err == nil {
		t.Fatal("Adyen otra divisa: debía fallar")
	}

	otroComercio := baseNotif()
	otroComercio.MerchantAccountCode = "ComercioAjeno"
	firmarAdyen(&otroComercio)
	if err := svc.ProcesarWebhookAdyen(ctx, otroComercio); !errors.Is(err, pagos.ErrNoCasa) {
		t.Fatalf("Adyen otro comercio: esperaba ErrNoCasa, obtuve %v", err)
	}

	// Un evento REFUND sobre un pedido pendiente nunca debe darlo por pagado.
	notifRefund := baseNotif()
	notifRefund.EventCode = "REFUND"
	notifRefund.OriginalReference = "7914073381342284"
	notifRefund.PSPReference = "8814073381342299"
	firmarAdyen(&notifRefund)
	_ = svc.ProcesarWebhookAdyen(ctx, notifRefund)
	if got := estadoPedido(); got != "pending_payment" {
		t.Fatalf("Adyen REFUND cobró un pedido pendiente: status=%q", got)
	}

	// Una CANCELLATION con Success="false" no debe cancelar ni fallar el pago abierto.
	notifCancelFallida := baseNotif()
	notifCancelFallida.EventCode = "CANCELLATION"
	notifCancelFallida.Success = "false"
	firmarAdyen(&notifCancelFallida)
	_ = svc.ProcesarWebhookAdyen(ctx, notifCancelFallida)
	if st, _ := estadoPago(pagoID); st != "open" {
		t.Fatalf("Adyen CANCELLATION con Success=false cambió el estado del pago a %q; quiero open", st)
	}

	// 3. Notificación AUTHORISATION auténtica: marca el pedido pagado y guarda el PSPReference real.
	buena := baseNotif()
	if err := svc.ProcesarWebhookAdyen(ctx, buena); err != nil {
		t.Fatalf("ProcesarWebhookAdyen válido: %v", err)
	}
	if got := estadoPedido(); got != "paid" {
		t.Fatalf("pedido tras AUTHORISATION = %q, quiero paid", got)
	}
	if st, ref := estadoPago(pagoID); st != "paid" || ref != "7914073381342284" {
		t.Fatalf("pago tras AUTHORISATION: status=%q provider_ref=%q; quiero paid y 7914073381342284", st, ref)
	}

	// 4. Cobro duplicado de un segundo intento en Adyen: debe reembolsar automáticamente
	// contra /v72/payments/<PSPReference>/refunds (no contra el id de sesión CS...).
	var pagoDupID uuid.UUID
	if err := super.QueryRow(ctx, `
		INSERT INTO payments (
			tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key
		) VALUES (
			'org', $1, $2, 2, 'adyen', 'CS9999999999DUP', 'open', 2500, 'pages-2-2500-adyen'
		) RETURNING id`, pedidoID, gw.ID,
	).Scan(&pagoDupID); err != nil {
		t.Fatalf("insertar pago duplicado adyen: %v", err)
	}
	notifDup := baseNotif()
	notifDup.PSPReference = "8514073381342200"
	notifDup.MerchantReference = pagoDupID.String()
	firmarAdyen(&notifDup)
	if err := svc.ProcesarWebhookAdyen(ctx, notifDup); err != nil {
		t.Fatalf("ProcesarWebhookAdyen duplicado: %v", err)
	}
	if st, ref := estadoPago(pagoDupID); st != "refunded" || ref != "8514073381342200" {
		t.Fatalf("pago duplicado Adyen: status=%q provider_ref=%q; quiero refunded y 8514073381342200", st, ref)
	}

	// 5. Reembolso explícito del pedido pagado: usa el PSPReference 7914073381342284.
	if err := svc.Reembolsar(ctx, "org", sitio.ID, pedidoID, 2500, "reembolso-manual-adyen", "admin-1"); err != nil {
		t.Fatalf("Reembolsar(Adyen): %v", err)
	}
	mu.Lock()
	gotReembolsos := append([]string(nil), reembolsosAdyen...)
	mu.Unlock()
	if len(gotReembolsos) != 2 ||
		gotReembolsos[0] != "8514073381342200|2500|dup-"+pagoDupID.String() ||
		gotReembolsos[1] != "7914073381342284|2500|reembolso-manual-adyen" {
		t.Fatalf("reembolsos ejecutados en Adyen = %v", gotReembolsos)
	}
}

func TestPasarelaRealRedsysFlujoCompletoYReembolso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}

	const claveRedsys = "sq7HjrUOBfKmC576ILgskD5srU870gJ7"
	svc, sitio, gw, pedidoID, tokenPedido, pagoID := sembrarPedidoTienda(t, ctx, super, kc, "redsys", map[string]string{
		"merchant_code": "999008881",
		"terminal":      "1",
		"secret_key":    claveRedsys,
	}, false)

	var reembolsosRedsys []string
	svc.client = &http.Client{
		Transport: transporteEnrutado{manejador: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host != "sis-t.redsys.es:25443" || r.URL.Path != "/sis/rest/trataPeticionREST" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			var reqBody map[string]string
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			rawParams, err := base64.StdEncoding.DecodeString(reqBody["Ds_MerchantParameters"])
			if err != nil {
				http.Error(w, "bad base64", http.StatusBadRequest)
				return
			}
			var params map[string]string
			if err := json.Unmarshal(rawParams, &params); err != nil {
				http.Error(w, "bad params", http.StatusBadRequest)
				return
			}
			orden := params["DS_MERCHANT_ORDER"]
			if !redsys.Verificar(reqBody["Ds_MerchantParameters"], orden, claveRedsys, reqBody["Ds_Signature"]) {
				http.Error(w, "bad signature", http.StatusForbidden)
				return
			}
			if params["DS_MERCHANT_TRANSACTIONTYPE"] != "3" || params["DS_MERCHANT_MERCHANTCODE"] != "999008881" {
				http.Error(w, "invalid transaction", http.StatusBadRequest)
				return
			}
			reembolsosRedsys = append(reembolsosRedsys, orden+"|"+params["DS_MERCHANT_AMOUNT"])

			respJSON, _ := json.Marshal(map[string]string{
				"Ds_Response":     "0900",
				"Ds_Order":        orden,
				"Ds_Amount":       params["DS_MERCHANT_AMOUNT"],
				"Ds_MerchantCode": "999008881",
				"Ds_Terminal":     "1",
				"Ds_Currency":     "978",
			})
			respB64 := base64.StdEncoding.EncodeToString(respJSON)
			respSig, _ := redsys.Firmar(respB64, orden, claveRedsys)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Ds_SignatureVersion":   redsys.FirmaVersion,
				"Ds_MerchantParameters": respB64,
				"Ds_Signature":          respSig,
			})
		})},
	}

	// 1. Pagar genera el formulario autoenviable firmado con 3DES-CBC + HMAC-SHA256.
	payResp, err := svc.Pagar(ctx, "org", sitio, pagoID, tokenPedido, "tienda.sites.example.test")
	if err != nil {
		t.Fatalf("Pagar(Redsys): %v", err)
	}
	ordenEsperada := redsys.Orden(pagoID.String())
	campos, _ := payResp.Formulario["campos"].(map[string]string)
	if payResp.Formulario["accion"] != "https://sis-t.redsys.es:25443/sis/realizarPago" ||
		!redsys.Verificar(campos["Ds_MerchantParameters"], ordenEsperada, claveRedsys, campos["Ds_Signature"]) {
		t.Fatalf("formulario Redsys inválido: %#v", payResp.Formulario)
	}

	empaquetarRedsys := func(campos map[string]string) (string, string) {
		raw, _ := json.Marshal(campos)
		b64 := base64.StdEncoding.EncodeToString(raw)
		sig, err := redsys.Firmar(b64, campos["Ds_Order"], claveRedsys)
		if err != nil {
			t.Fatalf("Firmar Redsys: %v", err)
		}
		return b64, sig
	}
	notifBase := func() map[string]string {
		return map[string]string{
			"Ds_Order":           ordenEsperada,
			"Ds_Amount":          "2500",
			"Ds_Currency":        "978",
			"Ds_MerchantCode":    "999008881",
			"Ds_Terminal":        "1",
			"Ds_TransactionType": "0",
			"Ds_Response":        "0000",
			"Ds_MerchantData":    pagoID.String(),
		}
	}

	// 2. Rechazos fail-closed (firma inválida, importe distinto, divisa distinta, FUC distinto, pago distinto).
	b64Ok, sigOk := empaquetarRedsys(notifBase())
	if err := svc.ProcesarWebhookRedsys(ctx, b64Ok, "firma_manipulada_invalida"); err == nil {
		t.Fatal("Redsys: aceptó firma inválida")
	}
	for nombre, mutar := range map[string]func(map[string]string){
		"importe distinto":  func(m map[string]string) { m["Ds_Amount"] = "100" },
		"divisa distinta":   func(m map[string]string) { m["Ds_Currency"] = "840" },
		"comercio distinto": func(m map[string]string) { m["Ds_MerchantCode"] = "123456789" },
		"terminal distinto": func(m map[string]string) { m["Ds_Terminal"] = "2" },
		"pago distinto":     func(m map[string]string) { m["Ds_MerchantData"] = uuid.NewString() },
	} {
		m := notifBase()
		mutar(m)
		b64, sig := empaquetarRedsys(m)
		if err := svc.ProcesarWebhookRedsys(ctx, b64, sig); err == nil {
			t.Fatalf("Redsys (%s): aceptó notificación que no casa", nombre)
		}
	}

	// 3. Notificación válida cobra el pedido.
	if err := svc.ProcesarWebhookRedsys(ctx, b64Ok, sigOk); err != nil {
		t.Fatalf("ProcesarWebhookRedsys válido: %v", err)
	}
	var estado string
	if err := super.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, pedidoID).Scan(&estado); err != nil || estado != "paid" {
		t.Fatalf("pedido tras Redsys = %q (%v); quiero paid", estado, err)
	}

	// 4. Cobro duplicado en Redsys (incluso si el intento 2 aún tenía provider_ref vacío
	// en base de datos y se resuelve por Ds_MerchantData): debe guardar Ds_Order y
	// reembolsar automáticamente por REST.
	var pagoDupID uuid.UUID
	if err := super.QueryRow(ctx, `
		INSERT INTO payments (
			tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key
		) VALUES (
			'org', $1, $2, 2, 'redsys', '', 'open', 2500, 'pages-2-2500-redsys'
		) RETURNING id`, pedidoID, gw.ID,
	).Scan(&pagoDupID); err != nil {
		t.Fatalf("insertar pago duplicado redsys: %v", err)
	}
	ordenDup := redsys.Orden(pagoDupID.String())
	mDup := notifBase()
	mDup["Ds_Order"] = ordenDup
	mDup["Ds_MerchantData"] = pagoDupID.String()
	b64Dup, sigDup := empaquetarRedsys(mDup)
	if err := svc.ProcesarWebhookRedsys(ctx, b64Dup, sigDup); err != nil {
		t.Fatalf("ProcesarWebhookRedsys duplicado: %v", err)
	}
	var stDup, refDup string
	if err := super.QueryRow(ctx, `SELECT status, provider_ref FROM payments WHERE id = $1`, pagoDupID).Scan(&stDup, &refDup); err != nil {
		t.Fatal(err)
	}
	if stDup != "refunded" || refDup != ordenDup || len(reembolsosRedsys) != 1 || reembolsosRedsys[0] != ordenDup+"|2500" {
		t.Fatalf("duplicado Redsys: status=%q ref=%q reembolsos=%v; quiero refunded, %s y [%s|2500]",
			stDup, refDup, reembolsosRedsys, ordenDup, ordenDup)
	}
}

func TestPasarelaRealMollieYStripeFlujoCompletoYFallosCerrados(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}

	// --- MOLLIE ---
	svcMollie, sitioMollie, gwMollie, pedidoMollie, tokenMollie, pagoMollie := sembrarPedidoTienda(t, ctx, super, kc, "mollie", map[string]string{
		"api_key":    "test_x",
		"profile_id": "pfl_QkEhN94Ba",
	}, false)

	respuestaPagoMollie := map[string]any{
		"id":        "tr_WDqYK6vllg",
		"status":    "paid",
		"mode":      "test",
		"profileId": "pfl_QkEhN94Ba",
		"amount":    map[string]string{"currency": "EUR", "value": "25.00"},
		"metadata":  map[string]any{pagos.MetadatoPago: pagoMollie.String()},
	}
	var reembolsosMollie []string

	svcMollie.client = &http.Client{
		Transport: transporteEnrutado{manejador: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Host != "api.mollie.com" || r.Header.Get("Authorization") != "Bearer test_x" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/v2/payments":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "tr_WDqYK6vllg",
					"_links": map[string]any{
						"checkout": map[string]string{"href": "https://checkout.mollie.com/checkout/select-method/WDqYK6vllg"},
					},
				})
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/payments/"):
				_ = json.NewEncoder(w).Encode(respuestaPagoMollie)
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refunds"):
				ref := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/payments/"), "/refunds")
				reembolsosMollie = append(reembolsosMollie, ref+"|"+r.Header.Get("Idempotency-Key"))
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "re_4qqhO89gsT"})
			default:
				http.Error(w, "not found", http.StatusNotFound)
			}
		})},
	}

	payMollie, err := svcMollie.Pagar(ctx, "org", sitioMollie, pagoMollie, tokenMollie, "tienda.sites.example.test")
	if err != nil || !strings.HasPrefix(payMollie.URL, "https://checkout.mollie.com/") {
		t.Fatalf("Pagar(Mollie) = %+v, %v", payMollie, err)
	}

	// Rechazos fail-closed en Mollie (importe, divisa, modo, perfil, metadatos).
	for nombre, mutar := range map[string]func(){
		"importe": func() { respuestaPagoMollie["amount"] = map[string]string{"currency": "EUR", "value": "1.00"} },
		"divisa":  func() { respuestaPagoMollie["amount"] = map[string]string{"currency": "USD", "value": "25.00"} },
		"modo":    func() { respuestaPagoMollie["mode"] = "live" },
		"perfil":  func() { respuestaPagoMollie["profileId"] = "pfl_OtroComercio" },
		"pago_id": func() { respuestaPagoMollie["metadata"] = map[string]any{pagos.MetadatoPago: uuid.NewString()} },
	} {
		respuestaPagoMollie["status"] = "paid"
		respuestaPagoMollie["mode"] = "test"
		respuestaPagoMollie["profileId"] = "pfl_QkEhN94Ba"
		respuestaPagoMollie["amount"] = map[string]string{"currency": "EUR", "value": "25.00"}
		respuestaPagoMollie["metadata"] = map[string]any{pagos.MetadatoPago: pagoMollie.String()}
		mutar()
		if err := svcMollie.ProcesarWebhookMollie(ctx, "tr_WDqYK6vllg"); !errors.Is(err, pagos.ErrNoCasa) {
			t.Fatalf("Mollie (%s): esperaba ErrNoCasa, obtuve %v", nombre, err)
		}
	}

	// Restaurar respuesta legítima y cobrar pedido.
	respuestaPagoMollie["mode"] = "test"
	respuestaPagoMollie["profileId"] = "pfl_QkEhN94Ba"
	respuestaPagoMollie["amount"] = map[string]string{"currency": "EUR", "value": "25.00"}
	respuestaPagoMollie["metadata"] = map[string]any{pagos.MetadatoPago: pagoMollie.String()}
	if err := svcMollie.ProcesarWebhookMollie(ctx, "tr_WDqYK6vllg"); err != nil {
		t.Fatalf("ProcesarWebhookMollie válido: %v", err)
	}

	// Duplicado en Mollie: reembolso automático contra /v2/payments/tr_DupMollie99/refunds.
	var pagoDupMollie uuid.UUID
	if err := super.QueryRow(ctx, `
		INSERT INTO payments (
			tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key
		) VALUES (
			'org', $1, $2, 2, 'mollie', 'tr_DupMollie99', 'open', 2500, 'pages-2-2500-mollie'
		) RETURNING id`, pedidoMollie, gwMollie.ID,
	).Scan(&pagoDupMollie); err != nil {
		t.Fatal(err)
	}
	respuestaPagoMollie["id"] = "tr_DupMollie99"
	respuestaPagoMollie["metadata"] = map[string]any{pagos.MetadatoPago: pagoDupMollie.String()}
	if err := svcMollie.ProcesarWebhookMollie(ctx, pagoDupMollie.String()); err != nil {
		t.Fatalf("ProcesarWebhookMollie duplicado: %v", err)
	}
	if len(reembolsosMollie) != 1 || reembolsosMollie[0] != "tr_DupMollie99|dup-"+pagoDupMollie.String() {
		t.Fatalf("reembolsos Mollie = %v", reembolsosMollie)
	}

	// --- STRIPE ---
	const stripeSecret = "whsec_test_secreto_stripe"
	svcStripe, sitioStripe, gwStripe, pedidoStripe, tokenStripe, pagoStripe := sembrarPedidoTienda(t, ctx, super, kc, "stripe", map[string]string{
		"secret_key":     "sk_test_x",
		"webhook_secret": stripeSecret,
		"account_id":     "acct_1TiendaStripe",
	}, true)

	var reembolsosStripe []string
	svcStripe.client = &http.Client{
		Transport: transporteEnrutado{manejador: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _, ok := r.BasicAuth()
			if r.URL.Host != "api.stripe.com" || !ok || user != "sk_test_x" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
				_ = json.NewEncoder(w).Encode(map[string]string{
					"id":  "cs_test_sesionStripe01",
					"url": "https://checkout.stripe.com/c/pay/cs_test_sesionStripe01",
				})
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":             strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/"),
					"payment_intent": "pi_test_intent99",
				})
			case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
				_ = r.ParseForm()
				reembolsosStripe = append(reembolsosStripe, r.FormValue("payment_intent")+"|"+r.FormValue("amount")+"|"+r.Header.Get("Idempotency-Key"))
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "re_test_123"})
			default:
				http.Error(w, "not found", http.StatusNotFound)
			}
		})},
	}

	payStripe, err := svcStripe.Pagar(ctx, "org", sitioStripe, pagoStripe, tokenStripe, "tienda.sites.example.test")
	if err != nil || !strings.HasPrefix(payStripe.URL, "https://checkout.stripe.com/") {
		t.Fatalf("Pagar(Stripe) = %+v, %v", payStripe, err)
	}

	firmarStripe := func(cuerpo string, ts time.Time, sec string) string {
		marca := strconv.FormatInt(ts.Unix(), 10)
		mac := hmac.New(sha256.New, []byte(sec))
		mac.Write([]byte(marca + "." + cuerpo))
		return "t=" + marca + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	}
	eventoJSON := func(sessionID, pagoIDStr string, importe int, divisa string, livemode bool) string {
		return fmt.Sprintf(
			`{"id":"evt_%s","type":"checkout.session.completed","livemode":%v,"data":{"object":{"id":%q,"payment_status":"paid","amount_total":%d,"currency":%q,"metadata":{"sytue_payment_id":%q}}}}`,
			uuid.NewString(), livemode, sessionID, importe, divisa, pagoIDStr,
		)
	}

	// Rechazos fail-closed en Stripe: firma inválida, marca temporal caducada, modo live, otra divisa, otro pago.
	cuerpoValido := eventoJSON("cs_test_sesionStripe01", pagoStripe.String(), 2500, "eur", false)
	if err := svcStripe.ProcesarWebhookStripe(ctx, []byte(cuerpoValido), firmarStripe(cuerpoValido, time.Now(), "whsec_incorrecto")); err == nil {
		t.Fatal("Stripe: aceptó firma con secreto incorrecto")
	}
	if err := svcStripe.ProcesarWebhookStripe(ctx, []byte(cuerpoValido), firmarStripe(cuerpoValido, time.Now().Add(-10*time.Minute), stripeSecret)); err == nil {
		t.Fatal("Stripe: aceptó firma fuera de ventana temporal")
	}
	for nombre, cuerpoMal := range map[string]string{
		"modo live":   eventoJSON("cs_test_sesionStripe01", pagoStripe.String(), 2500, "eur", true),
		"otra divisa": eventoJSON("cs_test_sesionStripe01", pagoStripe.String(), 2500, "usd", false),
		"otro pago":   eventoJSON("cs_test_sesionStripe01", uuid.NewString(), 2500, "eur", false),
	} {
		if err := svcStripe.ProcesarWebhookStripe(ctx, []byte(cuerpoMal), firmarStripe(cuerpoMal, time.Now(), stripeSecret)); !errors.Is(err, pagos.ErrNoCasa) {
			t.Fatalf("Stripe (%s): esperaba ErrNoCasa, obtuve %v", nombre, err)
		}
	}

	// Evento válido cobra el pedido.
	if err := svcStripe.ProcesarWebhookStripe(ctx, []byte(cuerpoValido), firmarStripe(cuerpoValido, time.Now(), stripeSecret)); err != nil {
		t.Fatalf("ProcesarWebhookStripe válido: %v", err)
	}

	// Cobro duplicado en Stripe (resuelto por sytue_payment_id cuando provider_ref aún estaba vacío):
	// guarda cs_test_dupStripe02 y reembolsa automáticamente el PaymentIntent.
	var pagoDupStripe uuid.UUID
	if err := super.QueryRow(ctx, `
		INSERT INTO payments (
			tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key
		) VALUES (
			'org', $1, $2, 2, 'stripe', '', 'open', 2500, 'pages-2-2500-stripe'
		) RETURNING id`, pedidoStripe, gwStripe.ID,
	).Scan(&pagoDupStripe); err != nil {
		t.Fatal(err)
	}
	cuerpoDup := eventoJSON("cs_test_dupStripe02", pagoDupStripe.String(), 2500, "eur", false)
	if err := svcStripe.ProcesarWebhookStripe(ctx, []byte(cuerpoDup), firmarStripe(cuerpoDup, time.Now(), stripeSecret)); err != nil {
		t.Fatalf("ProcesarWebhookStripe duplicado: %v", err)
	}
	if len(reembolsosStripe) != 1 || reembolsosStripe[0] != "pi_test_intent99|2500|dup-"+pagoDupStripe.String() {
		t.Fatalf("reembolsos Stripe = %v", reembolsosStripe)
	}
}
