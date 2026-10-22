// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
)

// ErrSpendLimitExceeded se devuelve cuando el gasto month-to-date del tenant
// supera su tope configurado: el choke point de despliegue (service.Deploy) lo
// usa para rechazar un nuevo build ANTES de encolarlo.
var ErrSpendLimitExceeded = errors.New("cloud: límite de gasto superado")

// PricingTable es el precio unitario AUTORITATIVO (EUR sin IVA) de cada métrica
// facturable, fijado como la media de mercado investigada (chargedEur). Fuentes
// por métrica en docs/business.md §13:
//   - build_minutes    0,005  EUR/min    (media Vercel/DO/Scaleway/Fly = 0,005097)
//   - compute_gb_hours 0,009  EUR/GB·h   (media = 0,008985)
//   - storage_gb       0,02   EUR/GB·mes (media cruda 0,0484 recortando el outlier
//     de Fly-block a la banda object-storage 0,016–0,021)
//
// FX USD→EUR = 0,92. Es la FUENTE DE VERDAD del cobro; /billing/pricing la expone
// al front (que hoy tiene una copia estimada en pricing.ts — sincronizar contra
// este endpoint es follow-up de front, ver §13).
var PricingTable = map[string]float64{
	"build_minutes":    0.005,
	"compute_gb_hours": 0.009,
	"storage_gb":       0.02,
}

// ---------------------------------------------------------------------------
// Suscripción a la suite (requisito de entrada de Sytue Cloud)
// ---------------------------------------------------------------------------

// SuiteSeatPriceEur es el precio por USUARIO y MES de la suscripción a la suite
// Sytue (EUR sin IVA). Es el precio de entrada a la plataforma.
const SuiteSeatPriceEur = float64(commerce.ProSeatPriceCents) / 100

// SuiteProducts son los productos que cubre un puesto de la suite.
var SuiteProducts = []string{"Mail", "Drive", "Docs", "Rooms"}

// MetricUnit es la unidad de display de cada métrica (para las líneas de factura
// y la respuesta de /billing/pricing).
var MetricUnit = map[string]string{
	"build_minutes":    "min",
	"compute_gb_hours": "GB·h",
	"storage_gb":       "GB·mes",
}

// PriceOf devuelve el precio unitario de una métrica de consumo Cloud y si es
// facturable. La suscripción a Sytue One se cobra en ID/Mollie y deliberadamente
// no forma parte de esta tabla para no facturarla dos veces.
func PriceOf(metric string) (float64, bool) {
	p, ok := PricingTable[metric]
	return p, ok
}

// PlanSpec define la economía de un plan: una cuota base mensual (BaseEur) y una
// cuota incluida por métrica (Included). El consumo por encima de la cuota se
// cobra al precio unitario de PricingTable (overage). BuildStrategy-style: los
// tres planes son datos, no ramas de código.
type PlanSpec struct {
	Plan     Plan
	BaseEur  float64
	Included map[string]float64 // métrica → cuota incluida (misma unidad que el uso)
	// MinSuiteSeats son los puestos de suite (Mail/Drive/Docs/Rooms) que exige el
	// plan. Sytue Cloud no se vende suelto: el plan FREE requiere al menos uno, de
	// modo que «gratis» significa «sin coste ADICIONAL sobre tu suscripción», no
	// gratis a secas. Es un dato del plan, no una rama de código.
	//
	// Hoy solo Free lo exige, que es la regla que se fijó. Si más adelante se
	// decide que Pro/Team también requieren suscripción, basta subir su valor aquí.
	MinSuiteSeats int
}

// MeetsSuiteRequirement indica si una organización con `seats` puestos de suite
// contratados cumple el requisito de su plan. Predicado PURO.
func MeetsSuiteRequirement(plan Plan, seats int) bool {
	return seats >= PlanSpecFor(plan).MinSuiteSeats
}

// planSpecs son los planes de Sytue Cloud (EUR sin IVA). Documentados en
// docs/business.md §13. Las cuotas incluidas cubren el uso típico de cada
// tramo; el exceso se cobra al precio de PricingTable.
var planSpecs = map[Plan]PlanSpec{
	PlanFree: {
		Plan:          PlanFree,
		BaseEur:       0,
		MinSuiteSeats: 1,
		Included: map[string]float64{
			"build_minutes":    100,
			"compute_gb_hours": 100,
			"storage_gb":       1,
		},
	},
	PlanPro: {
		Plan:    PlanPro,
		BaseEur: 19,
		Included: map[string]float64{
			"build_minutes":    1000,
			"compute_gb_hours": 1000,
			"storage_gb":       25,
		},
	},
	PlanTeam: {
		Plan:    PlanTeam,
		BaseEur: 49,
		Included: map[string]float64{
			"build_minutes":    3000,
			"compute_gb_hours": 4000,
			"storage_gb":       100,
		},
	},
}

// PlanSpecFor devuelve la especificación económica de un plan (free si inválido).
func PlanSpecFor(plan Plan) PlanSpec {
	if s, ok := planSpecs[plan]; ok {
		return s
	}
	return planSpecs[PlanFree]
}

// AllPlanSpecs devuelve los planes en orden estable (free, pro, team) para
// exponerlos por /billing/pricing.
func AllPlanSpecs() []PlanSpec {
	return []PlanSpec{planSpecs[PlanFree], planSpecs[PlanPro], planSpecs[PlanTeam]}
}

// BillingAccount es la cuenta de cobro de un tenant.
type BillingAccount struct {
	Tenant        string    `json:"tenant"`
	Plan          Plan      `json:"plan"`
	SpendLimitEur *float64  `json:"spend_limit_eur"` // nil = sin tope
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// InvoiceStatus son los estados de una factura.
const (
	InvoiceDraft  = "draft"
	InvoiceIssued = "issued"
	InvoicePaid   = "paid"
	InvoiceVoid   = "void"
)

// ValidInvoiceStatus valida un estado de factura.
func ValidInvoiceStatus(s string) bool {
	switch s {
	case InvoiceDraft, InvoiceIssued, InvoicePaid, InvoiceVoid:
		return true
	default:
		return false
	}
}

// InvoiceLine es el cargo de una métrica en una factura: la cantidad facturable
// (el exceso sobre la cuota incluida), su precio unitario y el subtotal.
type InvoiceLine struct {
	Metric       string  `json:"metric"`
	Quantity     float64 `json:"quantity"`
	UnitPriceEur float64 `json:"unit_price_eur"`
	SubtotalEur  float64 `json:"subtotal_eur"`
}

// Invoice es la factura de un periodo cerrado del tenant.
type Invoice struct {
	ID          uuid.UUID     `json:"id"`
	Tenant      string        `json:"tenant"`
	PeriodFrom  time.Time     `json:"period_from"`
	PeriodTo    time.Time     `json:"period_to"`
	Status      string        `json:"status"`
	SubtotalEur float64       `json:"subtotal_eur"`
	TotalEur    float64       `json:"total_eur"`
	Lines       []InvoiceLine `json:"lines"`
	CreatedAt   time.Time     `json:"created_at"`
	IssuedAt    *time.Time    `json:"issued_at,omitempty"`
}

// RoundCents redondea un importe en EUR a céntimos.
func RoundCents(eur float64) float64 { return math.Round(eur*100) / 100 }

// BuildInvoiceLines aplica un plan a un uso agregado (métrica → cantidad) y
// devuelve las líneas de cobro, el subtotal (suma de overage) y el total
// (subtotal + base del plan), todo redondeado a céntimos. Para cada métrica con
// uso se resta la cuota incluida y el exceso se cobra al precio de PricingTable.
// Métricas sin precio conocido se ignoran (una métrica nueva del metering nunca
// rompe la factura). Determinista: itera las métricas en orden alfabético.
func BuildInvoiceLines(plan Plan, usage map[string]float64) (lines []InvoiceLine, subtotalEur, totalEur float64) {
	spec := PlanSpecFor(plan)
	metrics := make([]string, 0, len(usage))
	for m := range usage {
		metrics = append(metrics, m)
	}
	sort.Strings(metrics)

	for _, m := range metrics {
		price, ok := PriceOf(m)
		if !ok {
			continue // métrica sin tarifa (p. ej. egress diferido): no se factura
		}
		billable := usage[m] - spec.Included[m]
		if billable < 0 {
			billable = 0
		}
		sub := RoundCents(billable * price)
		lines = append(lines, InvoiceLine{
			Metric:       m,
			Quantity:     billable,
			UnitPriceEur: price,
			SubtotalEur:  sub,
		})
		subtotalEur += sub
	}
	subtotalEur = RoundCents(subtotalEur)
	totalEur = RoundCents(subtotalEur + spec.BaseEur)
	return lines, subtotalEur, totalEur
}
