// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package tienda es la tienda de las webs: catálogo, checkout, pagos, pedidos y
// formularios (PAGES-ENCARGO §10).
package tienda

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/adyen"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/mollie"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/redsys"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/stripe"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/textos"
)

//go:embed textos/*.json
var textosFS embed.FS

var reFucRedsys = regexp.MustCompile(`^\d{9}$`)

// Servicio gestiona el catálogo, los ajustes, las pasarelas, el checkout, pedidos y barrido de la tienda.
type Servicio struct {
	store       *store.Store
	keyCipher   *keycrypto.Cipher
	publicURL   string
	sitesDomain string
	hub         *hubnotify.Client
	altcha      *altcha.Verifier
	textos      *textos.Catalogo
	client      *http.Client
	// pasarelaFija sustituye a la de verdad en las pruebas: ninguna pasarela real deja
	// reembolsar contra un servidor de pruebas sin credenciales.
	pasarelaFija pagos.Pasarela
	// enviarCorreo manda un aviso por el relé; lo fija Despachar (y las pruebas).
	enviarCorreo func(ctx context.Context, op correo.Opciones, para, asunto, cuerpo string) error
	syerpURL     string
	syerpSecret  string
}

// ConSyERP configura la URL y el secreto de SyERP para entregar pedidos a contabilidad y facturación.
func (s *Servicio) ConSyERP(rawURL, secret string) *Servicio {
	s.syerpURL = strings.TrimRight(rawURL, "/")
	s.syerpSecret = secret
	return s
}

// Nuevo construye el servicio de tienda.
func Nuevo(
	st *store.Store,
	keyCipher *keycrypto.Cipher,
	publicURL, sitesDomain string,
	hub *hubnotify.Client,
	verifier *altcha.Verifier,
) *Servicio {
	cat, _ := textos.Cargar(textosFS, "textos")
	return &Servicio{
		store:       st,
		keyCipher:   keyCipher,
		publicURL:   strings.TrimRight(publicURL, "/"),
		sitesDomain: strings.TrimPrefix(sitesDomain, "."),
		hub:         hub,
		altcha:      verifier,
		textos:      cat,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// WebhookURL devuelve la URL del webhook que debe configurarse en la pasarela.
func (s *Servicio) WebhookURL(provider string) string {
	return fmt.Sprintf("%s/webhooks/%s", s.publicURL, provider)
}

// ProbarYGuardarPasarela valida y prueba las credenciales de la pasarela, las sella
// con keycrypto (encv1:...) y las guarda en la base de datos (§10.2).
func (s *Servicio) ProbarYGuardarPasarela(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	usuario string,
	provider string,
	mode string,
	creds map[string]string,
	publicConfig map[string]any,
	noticeAccepted bool,
) (*domain.Gateway, error) {
	if s.keyCipher == nil {
		return nil, errors.New("tienda: cifrado en reposo no configurado")
	}

	if mode != "test" && mode != "live" {
		mode = "test"
	}

	if publicConfig == nil {
		publicConfig = make(map[string]any)
	}

	var noticeAt *time.Time
	if noticeAccepted {
		t := time.Now().UTC()
		noticeAt = &t
	}

	// 1. Validar y probar credenciales por pasarela (§10.2)
	var err error
	switch provider {
	case "mollie":
		err = validarMollie(mode, creds, publicConfig)
	case "adyen":
		err = validarAdyen(mode, creds, publicConfig)
	case "redsys":
		err = validarRedsys(mode, creds, publicConfig)
	case "stripe":
		err = validarStripe(mode, creds, noticeAt)
	default:
		return nil, fmt.Errorf("%w: pasarela desconocida %q", domain.ErrCredencialesPasarela, provider)
	}
	if err != nil {
		return nil, err
	}

	// 2. Sellar las credenciales en JSON con keycrypto
	rawJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("tienda: serializar credenciales: %w", err)
	}
	enc, err := s.keyCipher.Encrypt(string(rawJSON))
	if err != nil {
		return nil, fmt.Errorf("tienda: sellar credenciales: %w", err)
	}

	ahora := time.Now().UTC()
	g := &domain.Gateway{
		ID:               uuid.New(),
		Tenant:           tenant,
		SiteID:           siteID,
		Provider:         provider,
		Mode:             mode,
		CredentialsEnc:   enc,
		PublicConfig:     publicConfig,
		Active:           true,
		NoticeAcceptedAt: noticeAt,
		NoticeAcceptedBy: usuario,
		CheckedAt:        ahora,
		CreatedBy:        usuario,
		CreatedAt:        ahora,
		UpdatedAt:        ahora,
	}

	if s.store != nil {
		if err := s.store.GuardarPasarela(ctx, tenant, g); err != nil {
			return nil, err
		}
	}

	return g, nil
}

func validarMollie(mode string, creds map[string]string, publicConfig map[string]any) error {
	apiKey := strings.TrimSpace(creds["api_key"])
	profileID := strings.TrimSpace(creds["profile_id"])
	if mode == "live" && !strings.HasPrefix(apiKey, "live_") {
		return fmt.Errorf("%w: la api_key de mollie en modo live debe empezar por live_", domain.ErrCredencialesPasarela)
	}
	if mode == "test" && !strings.HasPrefix(apiKey, "test_") {
		return fmt.Errorf("%w: la api_key de mollie en modo test debe empezar por test_", domain.ErrCredencialesPasarela)
	}
	_ = mollie.New(apiKey, profileID)
	if profileID != "" {
		publicConfig["profile_id"] = profileID
	}
	return nil
}

func validarAdyen(mode string, creds map[string]string, publicConfig map[string]any) error {
	apiKey := strings.TrimSpace(creds["api_key"])
	merchant := strings.TrimSpace(creds["merchant_account"])
	hmacKey := strings.TrimSpace(creds["hmac_key"])
	livePrefix := strings.TrimSpace(creds["live_prefix"])

	if apiKey == "" || merchant == "" || hmacKey == "" {
		return fmt.Errorf("%w: adyen requiere api_key, merchant_account y hmac_key", domain.ErrCredencialesPasarela)
	}
	if _, err := hex.DecodeString(hmacKey); err != nil {
		return fmt.Errorf("%w: la hmac_key de adyen debe ser hexadecimal", domain.ErrCredencialesPasarela)
	}
	if mode == "live" && livePrefix == "" {
		return fmt.Errorf("%w: adyen exige live_prefix en modo live", domain.ErrCredencialesPasarela)
	}
	if _, err := adyen.New(apiKey, merchant, mode, livePrefix); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrCredencialesPasarela, err)
	}
	publicConfig["merchant_account"] = merchant
	if livePrefix != "" {
		publicConfig["live_prefix"] = livePrefix
	}
	return nil
}

func validarRedsys(mode string, creds map[string]string, publicConfig map[string]any) error {
	merchant := strings.TrimSpace(creds["merchant_code"])
	terminal := strings.TrimSpace(creds["terminal"])
	key := strings.TrimSpace(creds["secret_key"])

	if !reFucRedsys.MatchString(merchant) {
		return fmt.Errorf("%w: el FUC de redsys debe tener 9 dígitos numéricos", domain.ErrCredencialesPasarela)
	}
	if terminal == "" {
		return fmt.Errorf("%w: el terminal de redsys es obligatorio", domain.ErrCredencialesPasarela)
	}
	if _, err := base64.StdEncoding.DecodeString(key); err != nil {
		return fmt.Errorf("%w: la clave secreta de redsys debe ser base64", domain.ErrCredencialesPasarela)
	}
	if _, err := redsys.New(merchant, terminal, key, mode); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrCredencialesPasarela, err)
	}
	publicConfig["merchant_code"] = merchant
	publicConfig["terminal"] = terminal
	return nil
}

func validarStripe(mode string, creds map[string]string, noticeAt *time.Time) error {
	secretKey := strings.TrimSpace(creds["secret_key"])
	whSec := strings.TrimSpace(creds["webhook_secret"])

	if !strings.HasPrefix(secretKey, "sk_") && !strings.HasPrefix(secretKey, "rk_") {
		return fmt.Errorf("%w: la clave secreta de stripe debe empezar por sk_ o rk_", domain.ErrCredencialesPasarela)
	}
	if mode == "live" && (strings.Contains(secretKey, "_test_") || strings.HasSuffix(secretKey, "_test")) {
		return fmt.Errorf("%w: clave de test en modo live", domain.ErrCredencialesPasarela)
	}
	if mode == "test" && (strings.Contains(secretKey, "_live_") || strings.HasSuffix(secretKey, "_live")) {
		return fmt.Errorf("%w: clave de live en modo test", domain.ErrCredencialesPasarela)
	}
	if !strings.HasPrefix(whSec, "whsec_") {
		return fmt.Errorf("%w: el webhook secret de stripe debe empezar por whsec_", domain.ErrCredencialesPasarela)
	}
	if noticeAt == nil {
		return fmt.Errorf("%w: stripe exige aceptar el aviso legal de transferencia internacional", domain.ErrCredencialesPasarela)
	}
	if _, err := stripe.New(secretKey, comercioDe("stripe", creds)); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrCredencialesPasarela, err)
	}
	return nil
}

// DesencriptarCredencialesPasarela descifra las credenciales de una pasarela.
func (s *Servicio) DesencriptarCredencialesPasarela(gw *domain.Gateway) (map[string]string, error) {
	if s.keyCipher == nil {
		return nil, errors.New("tienda: keyCipher no configurado")
	}
	raw, err := s.keyCipher.Decrypt(gw.CredentialsEnc)
	if err != nil {
		return nil, fmt.Errorf("tienda: descifrar credenciales: %w", err)
	}
	var creds map[string]string
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return nil, fmt.Errorf("tienda: deserializar credenciales: %w", err)
	}
	return creds, nil
}

// comercioDe es el comercio de unas credenciales tal como lo cuenta su pasarela en cada
// pago (pagos.Estado.Comercio): el perfil de Mollie, la cuenta de Adyen, FUC y terminal
// de Redsys, y en Stripe la cuenta guardada o «stripe».
func comercioDe(proveedor string, creds map[string]string) string {
	switch proveedor {
	case "mollie":
		return strings.TrimSpace(creds["profile_id"])
	case "adyen":
		return strings.TrimSpace(creds["merchant_account"])
	case "redsys":
		return strings.TrimSpace(creds["merchant_code"]) + ":" + strings.TrimSpace(creds["terminal"])
	case "stripe":
		if c := strings.TrimSpace(creds["account_id"]); c != "" {
			return c
		}
		return "stripe"
	}
	return ""
}

// verificarPago es pagos.Verificar con lo que la tienda sabe del pago ANTES de preguntar a
// la pasarela: la fila del pago (su id y su importe) y la pasarela guardada (su modo y su
// comercio). Va antes de tocar nada, con cada notificación y en el barrido (WEB-03): un
// estado de otro importe, de otro pago, de otro comercio o del otro modo no cobra el pedido.
func (s *Servicio) verificarPago(pago *domain.Payment, gw *domain.Gateway, estado pagos.Estado) error {
	creds, err := s.DesencriptarCredencialesPasarela(gw)
	if err != nil {
		return err
	}
	esperado := pagos.Esperado{
		Pago: pago.ID.String(), Importe: int64(pago.AmountCents), Modo: gw.Mode, Comercio: comercioDe(gw.Provider, creds),
	}
	// ponytail: una cuenta de Mollie guardada sin perfil no lo sabe de antemano; el pago se
	// leyó con su propia clave, que solo ve los pagos de su perfil.
	if esperado.Comercio == "" && gw.Provider == "mollie" {
		esperado.Comercio = estado.Comercio
	}
	if err := pagos.Verificar(esperado, estado); err != nil {
		return fmt.Errorf("tienda: el estado no es el del pago %s: %w", pago.ID, err)
	}
	return nil
}

// CrearPasarelaCliente descifra las credenciales de la pasarela y construye la interfaz pagos.Pasarela.
func (s *Servicio) CrearPasarelaCliente(gw *domain.Gateway) (pagos.Pasarela, error) {
	if s.pasarelaFija != nil {
		return s.pasarelaFija, nil
	}
	creds, err := s.DesencriptarCredencialesPasarela(gw)
	if err != nil {
		return nil, err
	}

	switch gw.Provider {
	case "mollie":
		return mollie.New(creds["api_key"], creds["profile_id"]), nil
	case "adyen":
		return adyen.New(creds["api_key"], creds["merchant_account"], gw.Mode, creds["live_prefix"])
	case "redsys":
		return redsys.New(creds["merchant_code"], creds["terminal"], creds["secret_key"], gw.Mode)
	case "stripe":
		return stripe.New(creds["secret_key"], comercioDe("stripe", creds))
	default:
		return nil, fmt.Errorf("tienda: proveedor %q desconocido", gw.Provider)
	}
}

func validarSolicitudCheckout(req *domain.CheckoutRequest, st *domain.StoreSettings, gw *domain.Gateway) error {
	if req.CheckoutToken == uuid.Nil {
		return fmt.Errorf("%w: checkout_token obligatorio", domain.ErrProductoInvalido)
	}
	if len(req.Items) == 0 || len(req.Items) > 20 {
		return fmt.Errorf("%w: entre 1 y 20 líneas por pedido", domain.ErrProductoInvalido)
	}
	if req.TermsVersion < 1 {
		return domain.ErrTerminosRequeridos
	}
	if !st.Enabled {
		return domain.ErrTiendaNoDisponible
	}
	if gw == nil || !gw.Active {
		return domain.ErrPagoNoDisponible
	}
	if req.Base != "" && gw.Mode != "test" {
		return errors.New("tienda: checkout en vista previa solo disponible en modo test")
	}
	return nil
}

func validarFacturaCheckout(invoice *domain.CheckoutInvoice, totalCents int) error {
	if totalCents > 300000 {
		if invoice == nil || strings.TrimSpace(invoice.TaxID) == "" {
			return domain.ErrFacturaRequerida
		}
		if !validation.NIF(invoice.TaxID) {
			return domain.ErrNIFInvalido
		}
	} else if invoice != nil && strings.TrimSpace(invoice.TaxID) != "" {
		if !validation.NIF(invoice.TaxID) {
			return domain.ErrNIFInvalido
		}
	}
	return nil
}

// Checkout procesa la petición de POST /_pages/checkout (§10.4).
func (s *Servicio) Checkout(
	ctx context.Context,
	tenant string,
	site *domain.SitioCompleto,
	req *domain.CheckoutRequest,
	hostCanonical string,
) (*domain.CheckoutResponse, error) {
	// 1. Obtener ajustes de tienda y pasarela
	st, err := s.store.ObtenerAjustesTienda(ctx, tenant, site.ID)
	if err != nil {
		return nil, err
	}
	gw, err := s.store.ObtenerPasarelaConCredenciales(ctx, tenant, site.ID, req.Payment.Gateway)
	if err != nil {
		return nil, domain.ErrPagoNoDisponible
	}

	if err := validarSolicitudCheckout(req, st, gw); err != nil {
		return nil, err
	}

	// 2. Cargar catálogo de productos y calcular totales
	lines, subtotalCents, vatBreakdown, err := s.calcularLineasYTotales(ctx, tenant, site.ID, req.Items)
	if err != nil {
		return nil, err
	}

	// 3. Método de envío
	shippingCents, shipMethod := calcularEnvio(st.Shipping, req.Shipping.MethodID, subtotalCents)

	totalCents := subtotalCents + shippingCents
	vatCents := 0
	for _, v := range vatBreakdown {
		vatCents += v
	}

	// 4. Factura requerida si total > 3.000 € (300.000 céntimos, §10.4)
	if err := validarFacturaCheckout(req.Invoice, totalCents); err != nil {
		return nil, err
	}

	// 5. Generar token de acceso al pedido (32 bytes aleatorios, hash sha256)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	orderToken := hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(orderToken))

	ahora := time.Now().UTC()
	orderID := uuid.New()
	paymentID := uuid.New()

	var invoiceData map[string]any
	if req.Invoice != nil {
		invoiceData = map[string]any{
			"tax_id":  req.Invoice.TaxID,
			"name":    req.Invoice.Name,
			"address": req.Invoice.Address,
		}
	}

	order := &domain.Order{
		ID:              orderID,
		Tenant:          tenant,
		SiteID:          site.ID,
		Test:            gw.Mode == "test",
		CheckoutToken:   req.CheckoutToken,
		AccessTokenHash: hash[:],
		Status:          domain.OrderStatusPendingPayment,
		Currency:        "EUR",
		SubtotalCents:   subtotalCents,
		ShippingCents:   shippingCents,
		TotalCents:      totalCents,
		VATCents:        vatCents,
		VATBreakdown:    vatBreakdown,
		BuyerEmail:      req.Buyer.Email,
		BuyerName:       req.Buyer.Name,
		BuyerPhone:      req.Buyer.Phone,
		ShippingMethod:  *shipMethod,
		ShippingAddress: req.Shipping.Address,
		Invoice:         invoiceData,
		Language:        req.Language,
		TermsVersion:    req.TermsVersion,
		TermsAcceptedAt: ahora,
		ReservedUntil:   ahora.Add(35 * time.Minute),
		ERPStatus:       "none",
	}

	idemKey := fmt.Sprintf("pages-%s-1-%d", paymentID, totalCents)
	initialPayment := &domain.Payment{
		ID:             paymentID,
		Tenant:         tenant,
		OrderID:        orderID,
		GatewayID:      gw.ID,
		Attempt:        1,
		Provider:       gw.Provider,
		Method:         req.Payment.Method,
		Status:         domain.PaymentStatusCreated,
		AmountCents:    totalCents,
		IdempotencyKey: idemKey,
	}

	// 6. Ejecutar en UNA transacción en la base de datos
	savedOrder, savedPayment, err := s.store.CrearPedidoYReservar(ctx, tenant, site.ID, order, lines, initialPayment)
	if err != nil {
		return nil, err
	}
	// Un pedido que acaba de nacer y cuyo pago la pasarela no abre no se queda 35
	// minutos con las existencias reservadas y ocupando uno de los huecos de la tienda
	// (WEB-06). El de un checkout repetido no se toca: ese lo está pagando alguien. Sin
	// la cancelación de la petición: la pasarela que no contesta agota su plazo.
	soltar := func() {
		if savedOrder.ID == order.ID {
			_ = s.store.LiberarReservaPedidoVencido(context.WithoutCancel(ctx), tenant, savedOrder.ID)
		}
	}

	// 7. Preparar llamada a la pasarela (fuera de la transacción, §10.4)
	pasarela, err := s.CrearPasarelaCliente(gw)
	if err != nil {
		soltar()
		return nil, err
	}

	retornoURL := s.calcularURLRetorno(site, req.ReturnURL, req.Base, hostCanonical, req.Language, savedOrder.ID)
	webhookURL := s.calcularURLWebhook(gw.Provider, savedPayment.ID)

	desc := fmt.Sprintf("Pedido %s", savedOrder.Number)
	sol := pagos.Solicitud{
		Centimos:          int64(totalCents),
		Divisa:            pagos.DivisaEUR,
		Descripcion:       desc,
		Referencia:        savedOrder.Number,
		ClaveIdempotencia: savedPayment.IdempotencyKey,
		URLRetorno:        retornoURL,
		URLWebhook:        webhookURL,
		Idioma:            req.Language,
		Metodo:            savedPayment.Method,
		Caduca:            savedOrder.ReservedUntil,
		Metadatos: map[string]string{
			pagos.MetadatoPago: savedPayment.ID.String(),
			"site_id":          site.ID.String(),
			"order_id":         savedOrder.ID.String(),
			"tenant":           tenant,
		},
	}

	creado, err := pasarela.CrearPago(ctx, sol)
	if err != nil {
		soltar()
		return nil, fmt.Errorf("tienda: error pasarela %s: %w", gw.Provider, err)
	}

	_ = s.store.ActualizarEstadoPago(ctx, tenant, savedPayment.ID, creado.Ref, domain.PaymentStatusOpen)

	var formMap map[string]any
	if creado.Formulario != nil {
		formMap = map[string]any{
			"accion": creado.Formulario.Accion,
			"campos": creado.Formulario.Campos,
		}
	}

	return &domain.CheckoutResponse{
		OrderID:    savedOrder.ID,
		OrderToken: orderToken,
		Pay: domain.CheckoutPay{
			PaymentID:  savedPayment.ID,
			URL:        creado.URL,
			Formulario: formMap,
		},
	}, nil
}

func (s *Servicio) calcularLineasYTotales(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	items []domain.CheckoutItem,
) ([]domain.OrderLine, int, map[string]int, error) {
	prods, err := s.store.ListarProductos(ctx, tenant, siteID)
	if err != nil {
		return nil, 0, nil, err
	}

	prodMap := make(map[string]domain.Product)
	variantMap := make(map[string]domain.Variant)
	for _, p := range prods {
		for _, v := range p.Variants {
			prodMap[v.SKU] = p
			variantMap[v.SKU] = v
		}
	}

	var lines []domain.OrderLine
	subtotalCents := 0
	vatBreakdown := make(map[string]int)

	for i, it := range items {
		v, ok := variantMap[it.SKU]
		if !ok {
			return nil, 0, nil, fmt.Errorf("%w: SKU %q desconocido", domain.ErrProductoInvalido, it.SKU)
		}
		p := prodMap[it.SKU]

		if it.Qty < 1 || it.Qty > p.MaxPerOrder {
			return nil, 0, nil, fmt.Errorf("%w: cantidad inválida para %q", domain.ErrProductoInvalido, it.SKU)
		}
		if v.Stock != nil && (*v.Stock-v.Reserved) < it.Qty {
			return nil, 0, nil, fmt.Errorf("%w: SKU %q sin existencias", domain.ErrExistenciasAgotadas, it.SKU)
		}

		lineTotal := v.PriceCents * it.Qty
		subtotalCents += lineTotal

		// IVA
		cuota := int(float64(lineTotal) * p.VATRate / 100.0)
		rateKey := fmt.Sprintf("%.2f", p.VATRate)
		vatBreakdown[rateKey] += cuota

		var variantID *uuid.UUID
		if v.ID != uuid.Nil {
			vid := v.ID
			variantID = &vid
		}

		nameStr := p.Name["es"]
		if nameStr == "" {
			for _, val := range p.Name {
				nameStr = val
				break
			}
		}

		lines = append(lines, domain.OrderLine{
			Position:         int16(i + 1),
			VariantID:        variantID,
			SKU:              v.SKU,
			Name:             nameStr,
			Options:          v.Options,
			Quantity:         it.Qty,
			UnitPriceCents:   v.PriceCents,
			VATRate:          p.VATRate,
			TotalCents:       lineTotal,
			WithdrawalExempt: p.WithdrawalExempt,
		})
	}

	return lines, subtotalCents, vatBreakdown, nil
}

func calcularEnvio(shipping []domain.ShippingMethod, methodID string, subtotal int) (int, *domain.ShippingMethod) {
	for _, m := range shipping {
		if m.ID == methodID {
			if m.FreeOverCents != nil && subtotal >= *m.FreeOverCents {
				return 0, &m
			}
			return m.PriceCents, &m
		}
	}
	if len(shipping) > 0 {
		m := shipping[0]
		return m.PriceCents, &m
	}
	return 0, &domain.ShippingMethod{ID: "default", Name: map[string]string{"es": "Estándar"}}
}

func (s *Servicio) calcularURLRetorno(
	site *domain.SitioCompleto,
	reqReturnURL, base, hostCanonical, language string,
	orderID uuid.UUID,
) string {
	if reqReturnURL != "" {
		u, err := url.Parse(reqReturnURL)
		if err == nil {
			if u.Scheme != "" && u.Host == "" {
				for _, scheme := range site.ExportAppSchemes {
					if u.Scheme == scheme {
						return reqReturnURL
					}
				}
			} else if u.Scheme == "https" {
				origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
				for _, expOrigin := range site.ExportOrigins {
					if origin == expOrigin {
						return reqReturnURL
					}
				}
			}
		}
	}

	if base != "" && strings.HasPrefix(base, "/_vista/") {
		return fmt.Sprintf("https://%s%s/%s/order/?id=%s", hostCanonical, base, language, orderID)
	}
	return fmt.Sprintf("https://%s/%s/order/?id=%s", hostCanonical, language, orderID)
}

func (s *Servicio) calcularURLWebhook(provider string, paymentID uuid.UUID) string {
	if provider == "mollie" || provider == "redsys" {
		return fmt.Sprintf("%s/webhooks/%s/%s", s.publicURL, provider, paymentID)
	}
	return fmt.Sprintf("%s/webhooks/%s", s.publicURL, provider)
}

// Pagar inicia el flujo de pago cuando la isla envía el formulario a POST /_pages/pay/{payment_id} (§10.4).
func (s *Servicio) Pagar(
	ctx context.Context,
	tenant string,
	site *domain.SitioCompleto,
	paymentID uuid.UUID,
	orderToken string,
	hostCanonical string,
) (*domain.CheckoutPay, error) {
	pago, err := s.store.ObtenerPago(ctx, tenant, paymentID)
	if err != nil {
		return nil, domain.ErrPedidoNoExiste
	}

	order, err := s.store.ObtenerPedidoPorID(ctx, tenant, site.ID, pago.OrderID)
	if err != nil {
		return nil, domain.ErrPedidoNoExiste
	}

	tokenHash := sha256.Sum256([]byte(orderToken))
	if !strings.EqualFold(hex.EncodeToString(tokenHash[:]), hex.EncodeToString(order.AccessTokenHash)) {
		return nil, domain.ErrTokenPedidoInvalido
	}

	// Solo se paga un pedido que espera pago, con un intento que no esté cobrado: antes
	// un comprador repetía esta llamada sobre su pedido ya pagado, el pago volvía a
	// `open` y el barrido acababa reembolsándoselo como «duplicado» (WEB-03).
	if order.Status != domain.OrderStatusPendingPayment || domain.PagoCobrado(pago.Status) {
		return nil, domain.ErrPedidoYaPagado
	}

	gw, err := s.store.ObtenerPasarelaConCredenciales(ctx, tenant, site.ID, pago.Provider)
	if err != nil || !gw.Active {
		return nil, domain.ErrPagoNoDisponible
	}

	// Si el intento actual falló y aún tenemos intentos disponibles (< 3), crear intento nuevo
	pagoActual := pago
	if pago.Status == domain.PaymentStatusFailed || pago.Status == domain.PaymentStatusCanceled {
		if pago.Attempt >= 3 {
			return nil, domain.ErrDemasiadosIntentos
		}
		nuevoIntento := &domain.Payment{
			ID:             uuid.New(),
			Tenant:         tenant,
			OrderID:        order.ID,
			GatewayID:      gw.ID,
			Attempt:        pago.Attempt + 1,
			Provider:       gw.Provider,
			Method:         pago.Method,
			Status:         domain.PaymentStatusCreated,
			AmountCents:    order.TotalCents,
			IdempotencyKey: fmt.Sprintf("pages-%s-%d-%d", pago.ID, pago.Attempt+1, order.TotalCents),
		}
		if err := s.store.CrearSiguienteIntentoPago(ctx, tenant, nuevoIntento); err != nil {
			return nil, err
		}
		pagoActual = nuevoIntento
	}

	pasarela, err := s.CrearPasarelaCliente(gw)
	if err != nil {
		return nil, err
	}

	retornoURL := s.calcularURLRetorno(site, "", "", hostCanonical, order.Language, order.ID)
	webhookURL := s.calcularURLWebhook(gw.Provider, pagoActual.ID)

	sol := pagos.Solicitud{
		Centimos:          int64(order.TotalCents),
		Divisa:            pagos.DivisaEUR,
		Descripcion:       fmt.Sprintf("Pedido %s", order.Number),
		Referencia:        order.Number,
		ClaveIdempotencia: pagoActual.IdempotencyKey,
		URLRetorno:        retornoURL,
		URLWebhook:        webhookURL,
		Idioma:            order.Language,
		Metodo:            pagoActual.Method,
		Caduca:            order.ReservedUntil,
		Metadatos: map[string]string{
			pagos.MetadatoPago: pagoActual.ID.String(),
			"site_id":          site.ID.String(),
			"order_id":         order.ID.String(),
			"tenant":           tenant,
		},
	}

	creado, err := pasarela.CrearPago(ctx, sol)
	if err != nil {
		return nil, fmt.Errorf("tienda: error crear pago: %w", err)
	}

	_ = s.store.ActualizarEstadoPago(ctx, tenant, pagoActual.ID, creado.Ref, domain.PaymentStatusOpen)

	var formMap map[string]any
	if creado.Formulario != nil {
		formMap = map[string]any{
			"accion": creado.Formulario.Accion,
			"campos": creado.Formulario.Campos,
		}
	}

	return &domain.CheckoutPay{
		PaymentID:  pagoActual.ID,
		URL:        creado.URL,
		Formulario: formMap,
	}, nil
}

// ConsultarPedido devuelve la vista pública reducida de un pedido validando el token (§6.4, S23).
func (s *Servicio) ConsultarPedido(
	ctx context.Context,
	tenant string,
	siteID, orderID uuid.UUID,
	orderToken string,
) (*domain.OrderPublicDTO, error) {
	order, err := s.store.ObtenerPedidoPorID(ctx, tenant, siteID, orderID)
	if err != nil {
		return nil, domain.ErrPedidoNoExiste
	}

	tokenHash := sha256.Sum256([]byte(orderToken))
	if !strings.EqualFold(hex.EncodeToString(tokenHash[:]), hex.EncodeToString(order.AccessTokenHash)) {
		return nil, domain.ErrTokenPedidoInvalido
	}

	var itemsDTO []domain.OrderLineDTO
	for _, l := range order.Lines {
		itemsDTO = append(itemsDTO, domain.OrderLineDTO{
			SKU:              l.SKU,
			Name:             l.Name,
			Quantity:         l.Quantity,
			UnitPriceCents:   l.UnitPriceCents,
			TotalCents:       l.TotalCents,
			WithdrawalExempt: l.WithdrawalExempt,
		})
	}

	return &domain.OrderPublicDTO{
		ID:                   order.ID,
		Number:               order.Number,
		Status:               order.Status,
		CreatedAt:            order.CreatedAt,
		Currency:             order.Currency,
		SubtotalCents:        order.SubtotalCents,
		ShippingCents:        order.ShippingCents,
		TotalCents:           order.TotalCents,
		VATCents:             order.VATCents,
		VATBreakdown:         order.VATBreakdown,
		Items:                itemsDTO,
		WithdrawalPeriodDays: 14,
		HasInvoice:           order.ERPInvoiceRef != "",
		ERPInvoiceRef:        order.ERPInvoiceRef,
	}, nil
}

// Reembolsar ejecuta un reembolso sobre un pago (§10.5).
func (s *Servicio) Reembolsar(
	ctx context.Context,
	tenant string,
	siteID, orderID uuid.UUID,
	amountCents int,
	idempotencyKey string,
	createdBy string,
) error {
	pago, err := s.store.ObtenerUltimoPagoPorOrden(ctx, tenant, orderID)
	if err != nil {
		return domain.ErrPedidoNoExiste
	}

	gw, err := s.store.ObtenerPasarelaConCredenciales(ctx, tenant, siteID, pago.Provider)
	if err != nil {
		return err
	}

	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("pages-refund-%s-%d", uuid.New(), amountCents)
	}

	refundID := uuid.New()
	ref := &domain.Refund{
		ID:             refundID,
		Tenant:         tenant,
		PaymentID:      pago.ID,
		AmountCents:    amountCents,
		IdempotencyKey: idempotencyKey,
		Status:         "pending",
		CreatedBy:      createdBy,
	}

	if err := s.store.CrearReembolso(ctx, tenant, ref); err != nil {
		return err
	}

	pasarela, err := s.CrearPasarelaCliente(gw)
	if err != nil {
		_ = s.store.ActualizarReembolso(ctx, tenant, refundID, "failed")
		return err
	}

	if err := pasarela.Reembolsar(ctx, pago.ProviderRef, int64(amountCents), idempotencyKey); err != nil {
		_ = s.store.ActualizarReembolso(ctx, tenant, refundID, "failed")
		return fmt.Errorf("tienda: pasarela reembolsar: %w", err)
	}

	return s.store.ActualizarReembolso(ctx, tenant, refundID, "done")
}

// ObtenerCatalogoPublico consulta disponibilidad y precios para una lista de SKUs (§6.4).
func (s *Servicio) ObtenerCatalogoPublico(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	skus []string,
) ([]domain.CatalogItemDTO, error) {
	prods, err := s.store.ListarProductos(ctx, tenant, siteID)
	if err != nil {
		return nil, err
	}

	skuMap := make(map[string]domain.Variant)
	for _, p := range prods {
		for _, v := range p.Variants {
			skuMap[v.SKU] = v
		}
	}

	var items []domain.CatalogItemDTO
	for _, sku := range skus {
		sku = strings.TrimSpace(sku)
		if sku == "" {
			continue
		}
		v, ok := skuMap[sku]
		if !ok {
			items = append(items, domain.CatalogItemDTO{
				SKU:        sku,
				Available:  false,
				StockState: "out",
			})
			continue
		}

		available := true
		stockState := "in"
		if v.Stock != nil {
			disponible := *v.Stock - v.Reserved
			if disponible <= 0 {
				available = false
				stockState = "out"
			} else if disponible <= 5 {
				stockState = "low"
			}
		}

		items = append(items, domain.CatalogItemDTO{
			SKU:            v.SKU,
			PriceCents:     v.PriceCents,
			CompareAtCents: v.CompareAtCents,
			Lowest30dCents: nil,
			Available:      available,
			StockState:     stockState,
		})
	}

	return items, nil
}

// ListarProductosPublicos devuelve todos los productos con variantes para el código exportado (§9.5).
func (s *Servicio) ListarProductosPublicos(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	canonicalHost string,
	lang, cursor string,
) ([]domain.ProductPublicDTO, string, error) {
	prods, err := s.store.ListarProductos(ctx, tenant, siteID)
	if err != nil {
		return nil, "", err
	}

	if lang == "" {
		lang = "es"
	}

	baseURL := fmt.Sprintf("https://%s", canonicalHost)

	var items []domain.ProductPublicDTO
	for _, p := range prods {
		name := p.Name[lang]
		if name == "" {
			name = p.Name["es"]
		}
		desc := p.Description[lang]
		if desc == "" {
			desc = p.Description["es"]
		}

		slug := p.Slug[lang]
		if slug == "" {
			slug = p.Slug["es"]
		}
		if slug == "" {
			for _, val := range p.Slug {
				slug = val
				break
			}
		}

		var absImages []string
		for _, img := range p.Images {
			absImages = append(absImages, fmt.Sprintf("%s/_assets/%s", baseURL, img.String()))
		}

		var varsDTO []domain.VariantPublicDTO
		for _, v := range p.Variants {
			available := true
			stockState := "in"
			if v.Stock != nil {
				disponible := *v.Stock - v.Reserved
				if disponible <= 0 {
					available = false
					stockState = "out"
				} else if disponible <= 5 {
					stockState = "low"
				}
			}

			varsDTO = append(varsDTO, domain.VariantPublicDTO{
				ID:             v.ID,
				SKU:            v.SKU,
				Options:        v.Options,
				PriceCents:     v.PriceCents,
				CompareAtCents: v.CompareAtCents,
				Available:      available,
				StockState:     stockState,
			})
		}

		items = append(items, domain.ProductPublicDTO{
			ID:               p.ID,
			Slug:             slug,
			Name:             name,
			Description:      desc,
			Category:         p.Category,
			Images:           absImages,
			VATRate:          p.VATRate,
			Options:          p.Options,
			Kind:             p.Kind,
			GPSR:             p.GPSR,
			Food:             p.Food,
			WithdrawalExempt: p.WithdrawalExempt,
			MaxPerOrder:      p.MaxPerOrder,
			Variants:         varsDTO,
		})
	}

	return items, "", nil
}

// ObtenerFormularioPublico lee la definición pública del formulario (§9.5): la de la
// versión EN VIVO del sitio, la misma con la que se valida el envío. form_settings
// solo guarda si está encendido y a quién avisar; antes la definición se leía de ahí,
// donde no hay campos, y salía vacía.
func (s *Servicio) ObtenerFormularioPublico(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	formID string,
) (*domain.FormPublicDTO, error) {
	fs, err := s.store.ObtenerDefinicionFormulario(ctx, tenant, siteID, formID)
	if err != nil || !fs.Enabled {
		return nil, domain.ErrFormularioNoExiste
	}
	nodo, err := s.formularioPublicado(ctx, tenant, siteID, formID)
	if err != nil {
		return nil, err
	}
	campos := nodo.Fields
	if campos == nil {
		campos = []ir.CampoFormulario{}
	}
	consent := map[string]string(nodo.Consent)
	if consent == nil {
		consent = map[string]string{}
	}
	return &domain.FormPublicDTO{FormID: formID, Fields: campos, ConsentText: consent}, nil
}

// EnviarFormularioPublico valida y guarda una respuesta de formulario (§10.6).
//
// El formulario ha de estar encendido y existir en la versión EN VIVO del sitio, y solo
// se aceptan sus campos, con su tipo y su longitud (WEB-07): antes no se miraba
// `enabled` y los campos no se validaban (la definición nunca se cargaba), así que
// cualquier mapa de hasta 256 KiB se guardaba entero. El texto de consentimiento que se
// guarda es el publicado. Y el aviso a la organización tiene tope diario por sitio.
func (s *Servicio) EnviarFormularioPublico(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	formID string,
	fields map[string]any,
	consent bool,
	language string,
) error {
	if !consent {
		return domain.ErrConsentimientoRequerido
	}

	fs, err := s.store.ObtenerDefinicionFormulario(ctx, tenant, siteID, formID)
	if err != nil || !fs.Enabled {
		return domain.ErrFormularioNoExiste
	}
	nodo, err := s.formularioPublicado(ctx, tenant, siteID, formID)
	if err != nil {
		return err
	}
	datos, err := validarEnvio(nodo.Fields, fields, language)
	if err != nil {
		return err
	}

	sub := &domain.FormSubmission{
		ID:          uuid.New(),
		Tenant:      tenant,
		SiteID:      siteID,
		FormID:      formID,
		Data:        datos,
		Language:    language,
		ConsentText: textoEnIdioma(nodo.Consent, language),
		ConsentAt:   time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.store.GuardarFormulario(ctx, tenant, sub); err != nil {
		return err
	}

	// Aviso a la organización por el outbox, con solo identificadores (WEB-10): los campos
	// se leen del envío y a quién avisar, de form_settings, nunca de la petición. Pasado
	// el tope del día, el envío queda en la bandeja y no avisa.
	if len(fs.Notify) > 0 {
		if n, err := s.store.AvisosDeFormularioHoy(ctx, tenant, siteID); err == nil && n < maxAvisosFormularioDia {
			payload := map[string]any{
				"site_id":       siteID,
				"form_id":       formID,
				"submission_id": sub.ID,
			}
			if ev, err := events.NewEvent("pages", "pages.form.submitted", tenant, payload); err == nil {
				_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
			}
		}
	}

	return nil
}

// maxAvisosFormularioDia es el tope de avisos de formulario por sitio y día (§10.6).
const maxAvisosFormularioDia = 200

// formularioPublicado es el nodo del formulario en la versión EN VIVO del sitio, o
// ErrFormularioNoExiste si el sitio no está publicado o esa versión no lo tiene.
// ponytail: decodifica el IR publicado en cada envío; una caché por sitio y versión
// (§10.6 dice 5 min) si los envíos llegan a pesar.
func (s *Servicio) formularioPublicado(ctx context.Context, tenant string, siteID uuid.UUID, formID string) (*ir.Nodo, error) {
	raw, err := s.store.IRPublicado(ctx, tenant, siteID)
	if err != nil {
		return nil, domain.ErrFormularioNoExiste
	}
	sitio, err := ir.Decodificar(raw)
	if err != nil {
		return nil, domain.ErrFormularioNoExiste
	}
	nodo, ok := ir.Formularios(sitio)[formID]
	if !ok {
		return nil, domain.ErrFormularioNoExiste
	}
	return nodo, nil
}

// Largo por defecto de un campo de texto sin `max_length` (el IR admite hasta 5000).
const (
	maxLargoTexto     = 500
	maxLargoAreaTexto = 5000
)

// validarEnvio deja pasar solo los campos del formulario publicado, cada uno con su tipo
// y su longitud (§10.6): un campo que no es suyo, o uno que no cumple, invalida el envío.
// Un `select` llega como el índice de la opción y se guarda su etiqueta en el idioma del
// envío.
func validarEnvio(campos []ir.CampoFormulario, enviados map[string]any, idioma string) (map[string]any, error) {
	porID := make(map[string]ir.CampoFormulario, len(campos))
	for _, c := range campos {
		porID[c.ID] = c
	}
	for id := range enviados {
		if _, ok := porID[id]; !ok {
			return nil, fmt.Errorf("%w: el campo %q no es de este formulario", domain.ErrBorradorInvalido, id)
		}
	}
	datos := make(map[string]any, len(campos))
	for _, c := range campos {
		v, hay := enviados[c.ID]
		if !hay || v == nil || v == "" {
			if c.Required {
				return nil, fmt.Errorf("%w: el campo %q es obligatorio", domain.ErrBorradorInvalido, c.ID)
			}
			continue
		}
		valor, ok := valorDeCampo(c, v, idioma)
		if !ok {
			return nil, fmt.Errorf("%w: el campo %q no es válido", domain.ErrBorradorInvalido, c.ID)
		}
		datos[c.ID] = valor
	}
	return datos, nil
}

func valorDeCampo(c ir.CampoFormulario, v any, idioma string) (any, bool) {
	switch c.Kind {
	case "checkbox":
		b, ok := v.(bool)
		return b, ok
	case "number":
		return valorNumerico(c, v)
	case "select":
		return valorDeOpcion(c, v, idioma)
	case "date":
		return valorConFormato(v, "2006-01-02")
	case "time":
		return valorConFormato(v, "15:04")
	}
	return valorDeTexto(c, v) // text, email, tel, textarea
}

func valorNumerico(c ir.CampoFormulario, v any) (any, bool) {
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) ||
		(c.Min != nil && n < float64(*c.Min)) || (c.Max != nil && n > float64(*c.Max)) {
		return nil, false
	}
	return n, true
}

// valorDeOpcion: un `select` llega como el índice de la opción y se guarda su etiqueta.
func valorDeOpcion(c ir.CampoFormulario, v any, idioma string) (any, bool) {
	i, ok := v.(float64)
	if !ok || i != math.Trunc(i) || i < 0 || int(i) >= len(c.Options) {
		return nil, false
	}
	return textoEnIdioma(c.Options[int(i)], idioma), true
}

func valorConFormato(v any, formato string) (any, bool) {
	t, ok := v.(string)
	if !ok {
		return nil, false
	}
	_, err := time.Parse(formato, t)
	return t, err == nil
}

func valorDeTexto(c ir.CampoFormulario, v any) (any, bool) {
	t, ok := v.(string)
	if !ok {
		return nil, false
	}
	largo := maxLargoTexto
	if c.Kind == "textarea" {
		largo = maxLargoAreaTexto
	}
	if c.MaxLength != nil {
		largo = *c.MaxLength
	}
	if utf8.RuneCountInString(t) > largo {
		return nil, false
	}
	if c.Kind == "email" {
		a, err := mail.ParseAddress(t)
		if err != nil || a.Name != "" || a.Address != t {
			return nil, false
		}
	}
	return t, true
}

// textoEnIdioma es el texto en ese idioma o, si no lo tiene, en el primero que haya.
func textoEnIdioma(t ir.Textos, idioma string) string {
	if v := t[idioma]; v != "" {
		return v
	}
	claves := make([]string, 0, len(t))
	for k := range t {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	for _, k := range claves {
		if t[k] != "" {
			return t[k]
		}
	}
	return ""
}

// RegistrarDesistimientoPublico anota una solicitud de desistimiento legal (§10.7).
//
// Solo cuenta si es de un pedido cobrado (pagado o servido) hace menos de 12 meses y 14
// días y, por el formulario, si el correo es el del comprador; por el enlace del pedido
// lo prueba su token. Antes bastaba con el número del pedido, que es correlativo: cualquiera
// registraba desistimientos de pedidos ajenos y el acuse salía al comprador. Lo que no
// cuenta es ErrPedidoNoExiste, y la ruta contesta 204 igual, sin oráculo.
func (s *Servicio) RegistrarDesistimientoPublico(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	orderNumber string,
	email, name, reason string,
	orderID *uuid.UUID,
) error {
	source := "form"
	var pedido *domain.Order
	var err error
	switch {
	case orderID != nil && *orderID != uuid.Nil:
		source = "token"
		pedido, err = s.store.ObtenerPedidoPorID(ctx, tenant, siteID, *orderID)
	case orderNumber != "":
		pedido, err = s.store.ObtenerPedidoPorNumero(ctx, tenant, siteID, orderNumber)
		if err == nil && !strings.EqualFold(strings.TrimSpace(pedido.BuyerEmail), strings.TrimSpace(email)) {
			err = domain.ErrPedidoNoExiste
		}
	default:
		err = domain.ErrPedidoNoExiste
	}
	if err != nil || !desistible(pedido, time.Now()) {
		return domain.ErrPedidoNoExiste
	}

	w := &domain.Withdrawal{
		ID:          uuid.New(),
		Tenant:      tenant,
		SiteID:      siteID,
		OrderID:     pedido.ID,
		OrderNumber: pedido.Number,
		Email:       pedido.BuyerEmail, // el acuse va al del pedido, nunca al de la petición
		Name:        name,
		Source:      source,
		Reason:      reason,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.store.RegistrarDesistimiento(ctx, tenant, w); err != nil {
		return err
	}

	payload := map[string]any{
		"withdrawal_id": w.ID,
		"site_id":       siteID,
		"order_number":  pedido.Number,
	}
	if ev, err := events.NewEvent("pages", "pages.order.withdrawal", tenant, payload); err == nil {
		_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
	}

	return nil
}

// desistible: un pedido cobrado (pagado o servido) hace menos de 12 meses y 14 días, el
// plazo máximo del art. 105 TRLGDCU (§10.7). Lo demás lo decide la tienda.
func desistible(o *domain.Order, ahora time.Time) bool {
	if o.Status != domain.OrderStatusPaid && o.Status != domain.OrderStatusFulfilled {
		return false
	}
	return o.PaidAt != nil && ahora.Before(o.PaidAt.AddDate(1, 0, 14))
}

// EmitirAltcha genera un reto ALTCHA criptográfico (§6.4).
func (s *Servicio) EmitirAltcha() (altcha.Challenge, error) {
	if s.altcha == nil {
		return altcha.Challenge{}, errors.New("tienda: altcha no configurado")
	}
	return s.altcha.CreateChallenge(100000, 10*time.Minute)
}

// VerificarAltcha valida el reto y previene ataques de repetición (§6.4).
func (s *Servicio) VerificarAltcha(ctx context.Context, payload string) error {
	if s.altcha == nil {
		return nil
	}
	ok, err := s.altcha.Verify(ctx, payload)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("tienda: reto altcha invalido")
	}
	return nil
}
