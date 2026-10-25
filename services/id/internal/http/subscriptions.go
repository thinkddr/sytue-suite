// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// entitledTTL es corto A PROPÓSITO. Cachear el derecho de una organización es
// cachear el «sí» a cobrar: un minuto es el retraso máximo con el que se aplica una
// baja, y eso se acepta porque el corte real lo da la invalidación explícita de
// abajo. Subirlo a diez minutos serían diez minutos de servicio regalado cada vez
// que alguien cancela.
const entitledTTL = time.Minute

// entitledKey lleva el ID de la organización DENTRO de la clave. Es lo que impide
// que el derecho de una organización se sirva a otra: sin ese prefijo, la cache
// sería un agujero de aislamiento, no una optimización.
func entitledKey(orgID string) string { return "id:entitled:" + orgID }

// orgEntitled responde si la organización tiene derecho a la suite.
//
// SE CACHEA porque se pregunta en CADA emisión y en CADA refresco de token, y la
// consulta no es trivial: cruza la suscripción con los miembros activos y con los
// asientos concedidos. Con sesiones que refrescan cada cuarto de hora, doscientos
// mil usuarios son unas doscientas consultas por segundo dedicadas a responder casi
// siempre lo mismo.
//
// DEGRADACIÓN: sin Dragonfly —o si falla— se consulta PostgreSQL igual que antes.
// La cache acelera, nunca decide: si no está, el comportamiento es el de siempre.
func (s *Server) orgEntitled(ctx context.Context, orgID string) bool {
	if orgID == "" || !s.cfg.RequirePaidSubscription {
		return true
	}
	// GetSigned: el "1" cacheado ES la respuesta, así que solo se acepta si lo
	// sellamos nosotros. Sin sello válido se pregunta a PostgreSQL, que es el
	// comportamiento de siempre.
	if s.cache.Enabled() {
		if v, found, err := s.cache.GetSigned(ctx, entitledKey(orgID)); err == nil && found {
			return v == "1"
		}
	}
	ok, err := s.store.OrgSubscriptionEntitled(ctx, orgID, time.Now())
	if err != nil {
		s.log.Warn("no se pudo verificar el derecho de suscripción", "org_id", orgID, "error", err)
		return false
	}
	if s.cache.Enabled() {
		v := "0"
		if ok {
			v = "1"
		}
		// Un fallo al guardar no cambia la respuesta: la siguiente petición vuelve
		// a preguntar a la base, que es exactamente el comportamiento sin cache.
		if err := s.cache.SetSignedTTL(ctx, entitledKey(orgID), v, entitledTTL); err != nil {
			s.log.Debug("no se pudo cachear el derecho de suscripción", "org_id", orgID, "error", err)
		}
	}
	return ok
}

// invalidarEntitled borra el derecho cacheado de una organización. Hay que llamarlo
// en TODO camino que cambie la suscripción o los asientos —alta, baja, cambio de
// plan, conceder o quitar asientos—, porque si no el TTL manda y durante un minuto
// se sirve una respuesta que ya no es cierta.
func (s *Server) invalidarEntitled(ctx context.Context, orgID string) {
	if orgID == "" || !s.cache.Enabled() {
		return
	}
	if err := s.cache.Del(ctx, entitledKey(orgID)); err != nil {
		s.log.Warn("no se pudo invalidar el derecho cacheado", "org_id", orgID, "error", err)
	}
}

type subscriptionCheckoutRequest struct {
	Seats int `json:"seats"`
	// Plan es el contrato que se compra: "pro" (por defecto) o "enterprise".
	Plan string `json:"plan"`
}

func (s *Server) handleSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.onboardingOrg(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, err := s.store.GetOrgSubscription(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	members, err := s.store.ActiveMemberCount(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	now := time.Now()
	granted, err := s.store.GrantedSeats(r.Context(), org.ID, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// `granted_seats` va en la respuesta: sin él, una organización con plazas
	// concedidas ve `status: pending` y `entitled: true` a la vez, y el asistente
	// no tiene con qué explicar por qué ya puede entrar sin haber pagado.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"status": sub.Status, "seats": sub.Seats, "active_members": members,
		"granted_seats":    granted,
		"unit_price_cents": sub.EffectivePlan(now).UnitPriceCents(org.Education), "currency": domain.SuiteCurrency,
		"education":  org.Education,
		"paid_until": nullableTime(sub.PaidUntil), "entitled": sub.Entitled(now, members, granted),
		// El contrato vigente y lo que concede: es lo que la app de Cuenta enseña
		// y lo que decide si ofrece pasar a 1 TB.
		"plan": sub.EffectivePlan(now), "contracted_plan": sub.Plan,
		"storage_bytes": sub.StorageBytes(now), "mailbox_bytes": sub.MailboxBytes(now),
		"personal": org.Personal(),
		// Si una cuenta personal puede contratar un plan de pago: hoy no
		// (commerce.PersonalUpgradeAvailable).
		// La app de Cuenta lo lee de aquí y no de una constante propia, para que
		// abrirlo sea un solo cambio.
		"upgrade_available": !org.Personal() || commerce.PersonalUpgradeAvailable,
	})
}

func (s *Server) handleSubscriptionCheckout(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, err := s.onboardingOrg(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req subscriptionCheckoutRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	members, err := s.store.ActiveMemberCount(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	plan, err := contratoDelCheckout(org, &req, members)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sub, err := s.store.GetOrgSubscription(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	now := time.Now()
	granted, err := s.store.GrantedSeats(r.Context(), org.ID, now)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// «Ya activa» significa cosas distintas: en una organización, que hay derecho
	// de uso; en una cuenta personal —que siempre lo tiene—, que ESE plan está
	// cobrado y vigente (cambiar de estándar a pro sí se permite).
	yaActiva := sub.Entitled(now, members, granted)
	if org.Personal() {
		yaActiva = sub.Plan == plan && sub.Paid(now)
	}
	if yaActiva {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("The subscription is already active."))
		return
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil || !user.Active() {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	// Con el descuento educativo si la plataforma se lo concedió a la organización.
	amountCents := req.Seats * plan.UnitPriceCents(org.Education)
	checkout, err := s.billing.CreateCheckout(r.Context(), sub.ProviderCustomerID, org.ID,
		user.Email, user.Name, req.Seats, amountCents,
		s.cfg.MollieRedirectURL, s.cfg.MollieWebhookURL)
	if err != nil {
		s.log.Error("no se pudo crear checkout", "org_id", org.ID, "error", err)
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Payments are temporarily unavailable."))
		return
	}
	if err := s.store.RegisterSubscriptionCheckout(r.Context(), org.ID, checkout.CustomerID,
		checkout.PaymentID, req.Seats, amountCents, plan); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID,
		"billing.checkout.created", checkout.PaymentID, r))
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"checkout_url": checkout.CheckoutURL, "amount_cents": amountCents,
		"currency": domain.SuiteCurrency, "seats": req.Seats, "plan": plan,
	})
}

// handleMollieWebhook no confía en el cuerpo salvo para obtener un ID opaco. El
// estado, importe, cliente y suscripción se recuperan de Mollie con nuestra API
// key; una petición falsificada no puede conceder acceso.
func (s *Server) handleMollieWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid webhook."))
		return
	}
	id := r.Form.Get("id")
	payment, err := s.billing.GetPayment(r.Context(), id)
	if err != nil || payment.ID != id {
		s.log.Warn("webhook de pago no verificable", "payment_id", logsafe.Text(id), "error", err)
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The payment could not be verified."))
		return
	}

	var sub domain.OrgSubscription
	var expectedAmount int
	if payment.SubscriptionID != "" {
		sub, err = s.store.SubscriptionForProviderID(r.Context(), payment.SubscriptionID)
		expectedAmount = sub.Seats * sub.Plan.PriceCents()
	} else {
		var storedStatus string
		sub, expectedAmount, storedStatus, err = s.suscripcionDelPago(r.Context(), payment.ID)
		_ = storedStatus
	}
	// El importe cobrado tiene que ser EXACTAMENTE el del contrato local —asientos
	// por el precio de SU plan—, y un plan gratuito no cobra nunca: un pago que
	// llegue a nombre de uno es un pago que nadie pidió.
	if err != nil || payment.CustomerID != sub.ProviderCustomerID || payment.Currency != domain.SuiteCurrency ||
		sub.Plan.PriceCents() == 0 ||
		payment.AmountCents != expectedAmount || expectedAmount != sub.Seats*sub.Plan.UnitPriceCents(sub.Education) {
		s.log.Warn("pago no coincide con el contrato local", "payment_id", logsafe.Text(id), "error", err)
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unrecognised payment."))
		return
	}

	providerSubscriptionID := payment.SubscriptionID
	if payment.Status == "paid" && payment.PaidAt.IsZero() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The payment has no verifiable date."))
		return
	}
	if payment.Status == "paid" && providerSubscriptionID == "" {
		providerSubscriptionID = sub.ProviderSubscriptionID
		if providerSubscriptionID == "" {
			providerSubscriptionID, err = s.billing.EnsureSubscription(r.Context(), sub.ProviderCustomerID,
				sub.OrgID, payment.ID, sub.Seats, expectedAmount, payment.PaidAt, s.cfg.MollieWebhookURL)
			if err != nil {
				s.log.Error("pago cobrado pero suscripción recurrente no creada",
					"payment_id", logsafe.Text(id), "org_id", sub.OrgID, "error", err)
				http.Error(w, "retry", http.StatusServiceUnavailable)
				return
			}
		}
	}
	if err := s.store.RecordVerifiedPayment(r.Context(), payment.ID, sub.OrgID, payment.Status,
		payment.AmountCents, providerSubscriptionID, payment.PaidAt); err != nil {
		http.Error(w, "retry", http.StatusServiceUnavailable)
		return
	}
	// Un pago cambia el derecho de la organización, así que el valor cacheado ya no
	// vale. Sin esto, quien acaba de pagar seguiría viendo la puerta cerrada hasta
	// que expirara el TTL, y quien deja de pagar seguiría entrando.
	s.invalidarEntitled(r.Context(), sub.OrgID)
	if payment.Status == "paid" {
		_ = s.store.Audit(r.Context(), nil, auditEntry("", sub.OrgID,
			"billing.payment.verified", payment.ID, r))
	}
	w.WriteHeader(http.StatusOK)
}

func nullableTime(t time.Time) any {
	if t.IsZero() || t.Year() <= 1 {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// contratoDelCheckout decide qué se compra: el plan lo elige el cliente entre
// los de pago y el precio lo fija el servidor. Una cuenta personal compra un
// solo asiento —los que mande el navegador se ignoran a propósito, no se
// validan— y hoy no compra nada (commerce.PersonalUpgradeAvailable).
func contratoDelCheckout(org domain.Organization, req *subscriptionCheckoutRequest, members int) (commerce.Plan, error) {
	plan := commerce.Plan(req.Plan)
	if plan == "" {
		plan = commerce.PlanPro
	}
	if !commerce.ValidPlan(plan) || plan.Free() {
		return "", httpx.ErrBadRequest.WithMessage("Unknown plan.")
	}
	if org.Personal() {
		if !commerce.PersonalUpgradeAvailable {
			return "", httpx.ErrUnavailable.WithMessage("Paid plans are not available for personal accounts yet.")
		}
		req.Seats = 1
		return plan, nil
	}
	if req.Seats < members || req.Seats < 1 || req.Seats > 10000 {
		return "", httpx.ErrBadRequest.WithMessage("Seats must cover every active user (10,000 maximum).")
	}
	return plan, nil
}

// suscripcionDelPago es SubscriptionForPayment con lo que la organización
// aporta al importe esperado: el descuento educativo (mig 0064).
func (s *Server) suscripcionDelPago(ctx context.Context, paymentID string) (domain.OrgSubscription, int, string, error) {
	sub, expected, status, err := s.store.SubscriptionForPayment(ctx, paymentID)
	if err != nil {
		return sub, expected, status, err
	}
	if org, oerr := s.store.GetOrgByID(ctx, sub.OrgID); oerr == nil {
		sub.Education = org.Education
	}
	return sub, expected, status, nil
}
