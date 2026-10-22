// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// GetBillingAccount devuelve la cuenta de cobro del tenant (plan, tope de gasto,
// gasto month-to-date estimado).
func (h *Handlers) GetBillingAccount(w http.ResponseWriter, r *http.Request) {
	acct, err := h.svc.BillingAccount(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	spend, err := h.svc.CurrentSpend(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"account":           acct,
		"current_spend_eur": spend,
	})
}

// pricingMetric es una fila de la tabla de precios para el front.
type pricingMetric struct {
	Metric       string  `json:"metric"`
	Unit         string  `json:"unit"`
	PricePerUnit float64 `json:"price_per_unit_eur"`
}

// GetPricing expone la tabla de precios autoritativa (PricingTable) y los planes
// (base + cuotas incluidas). El front la consume para el estimador de coste.
func (h *Handlers) GetPricing(w http.ResponseWriter, r *http.Request) {
	metrics := make([]pricingMetric, 0, len(domain.PricingTable))
	for metric, price := range domain.PricingTable {
		metrics = append(metrics, pricingMetric{
			Metric:       metric,
			Unit:         domain.MetricUnit[metric],
			PricePerUnit: price,
		})
	}
	plans := make([]map[string]any, 0, 3)
	for _, p := range domain.AllPlanSpecs() {
		plans = append(plans, map[string]any{
			"plan":            p.Plan,
			"base_eur":        p.BaseEur,
			"included":        p.Included,
			"min_suite_seats": p.MinSuiteSeats,
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"currency": "EUR",
		"metrics":  metrics,
		"plans":    plans,
		// La suscripción a la suite es el precio de ENTRADA a la plataforma: el
		// front la necesita para no anunciar «gratis» un plan que exige contratarla.
		"suite": map[string]any{
			"seat_price_eur": domain.SuiteSeatPriceEur,
			"products":       domain.SuiteProducts,
		},
	})
}

type spendLimitRequest struct {
	// SpendLimitEur nil/ausente quita el tope; un número lo fija (>= 0).
	SpendLimitEur *float64 `json:"spend_limit_eur"`
}

// SetSpendLimit fija/quita el tope de gasto EUR del tenant (admin/owner).
func (h *Handlers) SetSpendLimit(w http.ResponseWriter, r *http.Request) {
	var req spendLimitRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	acct, err := h.svc.SetSpendLimit(r.Context(), req.SpendLimitEur)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, acct)
}

// ListInvoices lista las facturas del tenant (periodo DESC, keyset por ?cursor).
func (h *Handlers) ListInvoices(w http.ResponseWriter, r *http.Request) {
	afterPeriod, afterID := decodeInvoiceCursor(queryCursor(r))
	limit := queryLimit(r)
	invs, err := h.svc.ListInvoices(r.Context(), afterPeriod, afterID, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if invs == nil {
		invs = []domain.Invoice{}
	}
	var next string
	if n := len(invs); n > 0 {
		last := invs[n-1]
		next = encodeInvoiceCursor(last.PeriodFrom, last.ID)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"invoices": invs, "next_cursor": next})
}

// GetInvoice devuelve una factura del tenant por id.
func (h *Handlers) GetInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inv, err := h.svc.GetInvoice(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, inv)
}

type generateInvoiceRequest struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// GenerateInvoice calcula y persiste la factura del periodo (admin/owner). Lo
// dispara un CronJob mensual (K8s CronJob → POST con rol admin); idempotente por
// periodo. ponytail: aquí se expone el endpoint; el scheduler es el CronJob de
// K8s, no un scheduler propio.
func (h *Handlers) GenerateInvoice(w http.ResponseWriter, r *http.Request) {
	var req generateInvoiceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inv, err := h.svc.GenerateInvoice(r.Context(), req.From, req.To)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, inv)
}

// encodeInvoiceCursor / decodeInvoiceCursor codifican el keyset (period_from, id)
// como un token opaco base64 "<rfc3339nano>|<uuid>".
func encodeInvoiceCursor(period time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(period.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeInvoiceCursor(cursor string) (time.Time, uuid.UUID) {
	if cursor == "" {
		return time.Time{}, uuid.Nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil
	}
	period, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil
	}
	return period, id
}
