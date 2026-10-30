// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/adyen"
	pkgstripe "git.sytue.eu/sytue/one/platform/pkg/pagos/stripe"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// ProcesarWebhookMollie procesa una llamada webhook de Mollie (§10.5).
func (s *Servicio) ProcesarWebhookMollie(ctx context.Context, idOrRef string) error {
	idOrRef = strings.TrimSpace(idOrRef)
	if idOrRef == "" {
		return errors.New("tienda: webhook mollie sin referencia")
	}

	var pago *domain.Payment
	var gw *domain.Gateway
	var tenant string
	var err error

	if uid, parseErr := uuid.Parse(idOrRef); parseErr == nil {
		pago, gw, tenant, err = s.store.ResolverPagoPorID(ctx, uid)
	} else {
		pago, gw, tenant, err = s.store.ResolverPagoPorReferencia(ctx, "mollie", idOrRef)
	}
	if err != nil {
		return fmt.Errorf("tienda: resolver pago mollie: %w", err)
	}

	pasarela, err := s.CrearPasarelaCliente(gw)
	if err != nil {
		return fmt.Errorf("tienda: cliente pasarela mollie: %w", err)
	}

	ref := pago.ProviderRef
	if ref == "" {
		ref = idOrRef
	}
	if pago.ProviderRef == "" && strings.HasPrefix(ref, "tr_") {
		pago.ProviderRef = ref
	}

	estado, err := pasarela.ConsultarPago(ctx, ref)
	if err != nil {
		return fmt.Errorf("tienda: consultar pago mollie: %w", err)
	}
	if err := s.verificarSiActua(pago, gw, estado); err != nil {
		return err
	}

	eventID := fmt.Sprintf("mollie-%s-%s", ref, estado.Estado)
	_, _ = s.store.RegistrarEventoPago(ctx, tenant, gw.ID, "mollie", eventID, estado.Estado, &pago.ID)

	switch estado.Estado {
	case pagos.EstadoPagado:
		return s.tratarPagoCompletado(ctx, tenant, pago, gw, estado.Importe)
	case pagos.EstadoFallido:
		return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, ref, domain.PaymentStatusFailed)
	case pagos.EstadoCancelado:
		return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, ref, domain.PaymentStatusCanceled)
	case pagos.EstadoCaducado:
		return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, ref, domain.PaymentStatusExpired)
	}

	return nil
}

// ProcesarWebhookRedsys procesa una notificación webhook de Redsys (§10.5).
func (s *Servicio) ProcesarWebhookRedsys(ctx context.Context, merchantParams, signature string) error {
	merchantParams = strings.TrimSpace(merchantParams)
	signature = strings.TrimSpace(signature)
	if merchantParams == "" || signature == "" {
		return errors.New("tienda: webhook redsys incompleto")
	}

	// Decodificar parcialmente parámetros para localizar el pago
	raw, err := base64.StdEncoding.DecodeString(merchantParams)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(merchantParams)
		if err != nil {
			return errors.New("tienda: parametros redsys invalidos")
		}
	}

	var datos map[string]any
	if err := json.Unmarshal(raw, &datos); err != nil {
		return errors.New("tienda: parametros redsys json invalido")
	}

	var pago *domain.Payment
	var gw *domain.Gateway
	var tenant string

	merchantData, _ := datos["Ds_MerchantData"].(string)
	if uid, err := uuid.Parse(strings.TrimSpace(merchantData)); err == nil {
		pago, gw, tenant, err = s.store.ResolverPagoPorID(ctx, uid)
		if err != nil {
			return fmt.Errorf("tienda: resolver pago redsys por id: %w", err)
		}
	} else if dsOrder, ok := datos["Ds_Order"].(string); ok && dsOrder != "" {
		pago, gw, tenant, err = s.store.ResolverPagoPorReferencia(ctx, "redsys", dsOrder)
		if err != nil {
			return fmt.Errorf("tienda: resolver pago redsys por orden: %w", err)
		}
	} else {
		return errors.New("tienda: datos redsys sin referencia de pago")
	}

	pasarela, err := s.CrearPasarelaCliente(gw)
	if err != nil {
		return fmt.Errorf("tienda: cliente pasarela redsys: %w", err)
	}

	redsysClient, ok := pasarela.(interface {
		EstadoParametros(parametros, firma string) (pagos.Estado, error)
	})
	if !ok {
		return errors.New("tienda: cliente redsys no implementa EstadoParametros")
	}

	estado, err := redsysClient.EstadoParametros(merchantParams, signature)
	if err != nil {
		return fmt.Errorf("tienda: verificar firma redsys: %w", err)
	}
	if err := s.verificarPago(pago, gw, estado); err != nil {
		return err
	}
	if dsOrder, ok := datos["Ds_Order"].(string); ok && strings.TrimSpace(dsOrder) != "" {
		pago.ProviderRef = strings.TrimSpace(dsOrder)
	}

	eventID := fmt.Sprintf("redsys-%s-%s", signature, estado.Estado)
	_, _ = s.store.RegistrarEventoPago(ctx, tenant, gw.ID, "redsys", eventID, estado.Estado, &pago.ID)

	if estado.Estado == pagos.EstadoPagado {
		return s.tratarPagoCompletado(ctx, tenant, pago, gw, estado.Importe)
	}

	return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, pago.ProviderRef, domain.PaymentStatusFailed)
}

// eventoStripe es lo que se lee de un evento de Stripe: de los de la sesión de pago, su
// importe, su divisa, su modo y sus metadatos.
type eventoStripe struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Livemode bool   `json:"livemode"`
	Data     struct {
		Object struct {
			ID            string            `json:"id"`
			PaymentStatus string            `json:"payment_status"`
			AmountTotal   int64             `json:"amount_total"`
			Currency      string            `json:"currency"`
			Metadata      map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// deSesion dice si es uno de los eventos de la sesión que se tratan.
func (e eventoStripe) deSesion() bool {
	switch e.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.expired":
		return true
	}
	return false
}

// estado es el evento como pagos.Estado, para verificarlo contra el pago esperado.
func (e eventoStripe) estado(comercio string) pagos.Estado {
	modo := pagos.ModoPrueba
	if e.Livemode {
		modo = pagos.ModoReal
	}
	o := e.Data.Object
	return pagos.Estado{
		Importe:   o.AmountTotal,
		Divisa:    strings.ToUpper(o.Currency),
		Modo:      modo,
		Comercio:  comercio,
		Metadatos: o.Metadata,
	}
}

func (s *Servicio) resolverPagoStripe(ctx context.Context, sessionID, pagoIDStr string) (*domain.Payment, *domain.Gateway, string, error) {
	var pago *domain.Payment
	var gw *domain.Gateway
	var tenant string
	var err error

	if sessionID != "" {
		pago, gw, tenant, err = s.store.ResolverPagoPorReferencia(ctx, "stripe", sessionID)
	}
	if (err != nil || pago == nil) && pagoIDStr != "" {
		if uid, parseErr := uuid.Parse(pagoIDStr); parseErr == nil {
			pago, gw, tenant, err = s.store.ResolverPagoPorID(ctx, uid)
		}
	}
	if err != nil || pago == nil || gw == nil {
		return nil, nil, "", fmt.Errorf("tienda: resolver pago stripe: %w", err)
	}
	return pago, gw, tenant, nil
}

// ProcesarWebhookStripe procesa una notificación webhook firmada de Stripe (§10.5).
func (s *Servicio) ProcesarWebhookStripe(ctx context.Context, rawBody []byte, sigHeader string) error {
	if len(rawBody) == 0 || sigHeader == "" {
		return errors.New("tienda: webhook stripe vacio")
	}

	var payload eventoStripe
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return errors.New("tienda: cuerpo stripe no es json")
	}

	sessionID := payload.Data.Object.ID
	pagoIDStr := payload.Data.Object.Metadata[pagos.MetadatoPago]

	pago, gw, tenant, err := s.resolverPagoStripe(ctx, sessionID, pagoIDStr)
	if err != nil {
		return err
	}

	// Obtener secreto de webhook para verificar firma
	creds, err := s.DesencriptarCredencialesPasarela(gw)
	if err != nil {
		return fmt.Errorf("tienda: credenciales stripe: %w", err)
	}
	webhookSecret := creds["webhook_secret"]
	if webhookSecret == "" {
		webhookSecret = creds["secret_key"]
	}

	if err := pkgstripe.VerificarFirma(rawBody, sigHeader, webhookSecret, time.Now()); err != nil {
		return fmt.Errorf("tienda: firma stripe invalida: %w", err)
	}

	// Solo los eventos de la sesión que se tratan; de los demás no se toca nada.
	if payload.deSesion() {
		if err := s.verificarPago(pago, gw, payload.estado(comercioDe("stripe", creds))); err != nil {
			return err
		}
		if sessionID != "" {
			pago.ProviderRef = sessionID
		}
	}

	eventID := payload.ID
	if eventID == "" {
		eventID = fmt.Sprintf("stripe-%s-%s", sessionID, payload.Type)
	}
	_, _ = s.store.RegistrarEventoPago(ctx, tenant, gw.ID, "stripe", eventID, payload.Type, &pago.ID)

	switch payload.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		if payload.Data.Object.PaymentStatus == "paid" {
			return s.tratarPagoCompletado(ctx, tenant, pago, gw, payload.Data.Object.AmountTotal)
		}
	case "checkout.session.expired":
		return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, sessionID, domain.PaymentStatusExpired)
	}

	return nil
}

// ProcesarWebhookAdyen procesa una notificación autenticada de Adyen (§10.5).
func (s *Servicio) ProcesarWebhookAdyen(ctx context.Context, notif adyen.Notificacion) error {
	merchantRef := strings.TrimSpace(notif.MerchantReference)
	if merchantRef == "" {
		return errors.New("tienda: webhook adyen sin merchant reference")
	}

	uid, err := uuid.Parse(merchantRef)
	if err != nil {
		return errors.New("tienda: merchant reference adyen no es uuid")
	}

	pago, gw, tenant, err := s.store.ResolverPagoPorID(ctx, uid)
	if err != nil {
		return fmt.Errorf("tienda: resolver pago adyen: %w", err)
	}

	creds, err := s.DesencriptarCredencialesPasarela(gw)
	if err != nil {
		return fmt.Errorf("tienda: credenciales adyen: %w", err)
	}

	hmacKey := creds["hmac_key"]
	if err := notif.VerificarFirma(hmacKey); err != nil {
		return fmt.Errorf("tienda: verificar firma adyen: %w", err)
	}

	// La notificación de Adyen no trae los metadatos del cobro: el pago es el de su
	// merchantReference, con el que se ha resuelto arriba.
	estado, err := notif.Estado(gw.Mode, map[string]string{pagos.MetadatoPago: merchantRef})
	if err != nil {
		return fmt.Errorf("tienda: estado adyen: %w", err)
	}
	if err := s.verificarPago(pago, gw, estado); err != nil {
		return err
	}

	eventID := fmt.Sprintf("adyen-%s-%s", notif.PSPReference, notif.EventCode)
	_, _ = s.store.RegistrarEventoPago(ctx, tenant, gw.ID, "adyen", eventID, notif.EventCode, &pago.ID)

	// Solo AUTHORISATION confirma o rechaza el cobro y sustituye el id de sesión
	// inicial (CS...) por el PSPReference del pago, que es el que Adyen exige en
	// /payments/{paymentPspReference}/refunds. Un evento REFUND nunca debe dar por
	// pagado un pedido pendiente ni pisar el PSPReference del cobro original.
	switch notif.EventCode {
	case "AUTHORISATION":
		pago.ProviderRef = notif.PSPReference
		if estado.Estado == pagos.EstadoPagado {
			return s.tratarPagoCompletado(ctx, tenant, pago, gw, estado.Importe)
		}
		return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, notif.PSPReference, domain.PaymentStatusFailed)
	case "CANCELLATION", "CANCEL_OR_REFUND":
		if estado.Estado == pagos.EstadoCancelado {
			return s.store.ActualizarEstadoPago(ctx, tenant, pago.ID, notif.PSPReference, domain.PaymentStatusCanceled)
		}
		return nil
	default:
		return nil
	}
}

// verificarSiActua es verificarPago para los estados que cambian algo (pagado, fallido,
// cancelado, caducado). De uno abierto o pendiente no se toca nada, y no hay que exigirle.
func (s *Servicio) verificarSiActua(pago *domain.Payment, gw *domain.Gateway, estado pagos.Estado) error {
	switch estado.Estado {
	case pagos.EstadoPagado, pagos.EstadoFallido, pagos.EstadoCancelado, pagos.EstadoCaducado:
		return s.verificarPago(pago, gw, estado)
	}
	return nil
}

func (s *Servicio) tratarPagoCompletado(
	ctx context.Context,
	tenant string,
	pago *domain.Payment,
	gw *domain.Gateway,
	importe int64,
) error {
	order, oversold, duplicado, err := s.store.MarcarPedidoPagado(
		ctx, tenant, pago.OrderID, pago.ID, pago.ProviderRef, int(importe),
	)
	if errors.Is(err, domain.ErrPagoYaTratado) {
		// Notificación repetida: nada que hacer otra vez, salvo reintentar el reembolso
		// de un duplicado que la primera vez falló (solo ese: nunca uno nuevo).
		s.reembolsarDuplicado(ctx, tenant, gw, pago, importe, false)
		return nil
	}
	if err != nil {
		return fmt.Errorf("tienda: marcar pedido pagado: %w", err)
	}

	if duplicado {
		// Pago duplicado sobre un pedido ya pagado: reembolso automático + alerta (§10.5)
		s.reembolsarDuplicado(ctx, tenant, gw, pago, importe, true)

		alertPayload := map[string]any{
			"order_id":     pago.OrderID,
			"payment_id":   pago.ID,
			"site_id":      gw.SiteID,
			"amount_cents": importe,
		}
		if ev, err := events.NewEvent("pages", "pages.payment.duplicated", tenant, alertPayload); err == nil {
			_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
		}
		return nil
	}

	if oversold {
		// Pedido pagado fuera de plazo / existencias agotadas: alerta (§10.5)
		oversoldPayload := map[string]any{
			"order_id": order.ID,
			"site_id":  order.SiteID,
			"number":   order.Number,
		}
		if ev, err := events.NewEvent("pages", "pages.order.oversold", tenant, oversoldPayload); err == nil {
			_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
		}
	}

	// Los avisos llevan SOLO identificadores e importes: el outbox no tiene RLS, y
	// quien lo consuma lee el comprador, la factura y las líneas del pedido con la RLS de
	// su organización (WEB-10).

	// 1. Correo confirmación al comprador
	buyerPayload := map[string]any{
		"order_id":               order.ID,
		"number":                 order.Number,
		"site_id":                order.SiteID,
		"language":               order.Language,
		"currency":               order.Currency,
		"total_cents":            order.TotalCents,
		"withdrawal_period_days": 14,
	}
	if ev, err := events.NewEvent("pages", "order.paid.email_buyer", tenant, buyerPayload); err == nil {
		_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
	}

	// 2. Correo notificación a la tienda
	storePayload := map[string]any{
		"order_id":    order.ID,
		"number":      order.Number,
		"site_id":     order.SiteID,
		"total_cents": order.TotalCents,
		"currency":    order.Currency,
	}
	if ev, err := events.NewEvent("pages", "order.paid.email_store", tenant, storePayload); err == nil {
		_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
	}

	// 3. Notificación a Hub
	hubPayload := map[string]any{
		"order_id":    order.ID,
		"number":      order.Number,
		"site_id":     order.SiteID,
		"total_cents": order.TotalCents,
	}
	if ev, err := events.NewEvent("pages", "order.paid.hub", tenant, hubPayload); err == nil {
		_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
	}

	// 4. Integración ERP si aplica
	if len(order.Invoice) > 0 {
		erpPayload := map[string]any{
			"order_id":       order.ID,
			"site_id":        order.SiteID,
			"subtotal_cents": order.SubtotalCents,
			"vat_cents":      order.VATCents,
			"total_cents":    order.TotalCents,
		}
		if ev, err := events.NewEvent("pages", "order.paid.erp", tenant, erpPayload); err == nil {
			_ = s.store.EscribirEventoOutbox(ctx, tenant, ev)
		}
	}

	return nil
}

// reembolsarDuplicado devuelve por su pasarela el cobro `pago`, que es un duplicado: otro
// pago del mismo pedido ya constaba cobrado (MarcarPedidoPagado, WEB-03). Decisión del
// dueño (2026-10-16): automático, y la tienda recibe el aviso `pages.payment.duplicated`.
//
// Idempotente: el reembolso es UNO por cobro (clave `dup-<pago>`, única en `refunds`).
// Una notificación repetida no crea otro; solo reintenta el que falló, y con la misma
// clave de idempotencia, que la pasarela también reconoce. `nuevo` es false en una
// notificación repetida: entonces solo se reintenta, nunca se abre uno.
//
// Se reembolsa ESTE cobro, con la pasarela con que se cobró, y no el último pago del
// pedido como hacía Reembolsar: un duplicado puede ser un intento anterior que llegó
// tarde, y entonces se devolvía el bueno.
func (s *Servicio) reembolsarDuplicado(ctx context.Context, tenant string, gw *domain.Gateway, pago *domain.Payment, importe int64, nuevo bool) {
	if importe <= 0 {
		importe = int64(pago.AmountCents)
	}
	clave := "dup-" + pago.ID.String()
	id, toca, err := s.store.ReservarReembolsoDuplicado(ctx, tenant, pago.ID, int(importe), clave, nuevo)
	if err != nil || !toca {
		return
	}
	estado := "failed"
	if pasarela, err := s.CrearPasarelaCliente(gw); err == nil {
		if pasarela.Reembolsar(ctx, pago.ProviderRef, importe, clave) == nil {
			estado = "done"
		}
	}
	_ = s.store.ActualizarReembolso(ctx, tenant, id, estado)
}
