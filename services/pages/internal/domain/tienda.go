// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

var (
	ErrTiendaNoLista            = errors.New("la tienda no cumple los requisitos para activarse")
	ErrPasarelaModoIncompatible = errors.New("todas las pasarelas activas deben estar en el mismo modo")
	ErrProductoInvalido         = errors.New("datos de producto inválidos")
	ErrStockInsuficiente        = errors.New("existencias insuficientes")
	ErrCredencialesPasarela     = errors.New("credenciales de pasarela inválidas")
)

// Tipos de IVA permitidos en España / UE para la tienda (§10.3).
var TiposIVAPermitidos = map[float64]bool{
	0.0:  true,
	4.0:  true,
	5.0:  true,
	10.0: true,
	21.0: true,
}

// Lista cerrada de los 14 alérgenos obligatorios de la UE (Reglamento 1169/2011, §20.10).
var AlergenosUE = map[string]bool{
	"cereales_gluten":         true,
	"crustaceos":              true,
	"huevos":                  true,
	"pescado":                 true,
	"cacahuetes":              true,
	"soja":                    true,
	"lacteos":                 true,
	"frutos_cascara":          true,
	"apio":                    true,
	"mostaza":                 true,
	"sesamo":                  true,
	"dioxido_azufre_sulfitos": true,
	"altramuces":              true,
	"moluscos":                true,
}

// StoreSettings agrupa la configuración general de la tienda de un sitio (§10.3).
type StoreSettings struct {
	SiteID          uuid.UUID           `json:"site_id"`
	Tenant          string              `json:"tenant"`
	Enabled         bool                `json:"enabled"`
	Currency        string              `json:"currency"`
	Countries       []string            `json:"countries"`
	Shipping        []ShippingMethod    `json:"shipping"`
	Terms           StoreTerms          `json:"terms"`
	TermsVersion    int                 `json:"terms_version"`
	Notify          []StoreNotifyPerson `json:"notify"`
	ReplyTo         string              `json:"reply_to"`
	ERPEnabled      bool                `json:"erp_enabled"`
	NextOrderNumber int                 `json:"next_order_number"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// ShippingMethod define un método de envío o recogida.
type ShippingMethod struct {
	ID            string            `json:"id"`
	Name          map[string]string `json:"name"`
	PriceCents    int               `json:"price_cents"`
	VATRate       float64           `json:"vat_rate"`
	FreeOverCents *int              `json:"free_over_cents,omitempty"`
	Countries     []string          `json:"countries"`
	Pickup        bool              `json:"pickup"`
}

// StoreTerms define plazos y devoluciones legales.
type StoreTerms struct {
	DeliveryTime   map[string]string `json:"delivery_time"`
	ReturnsAddress string            `json:"returns_address"`
}

// StoreNotifyPerson identifica un destinatario de avisos de tienda.
type StoreNotifyPerson struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Product representa un artículo del catálogo.
type Product struct {
	ID               uuid.UUID           `json:"id"`
	Tenant           string              `json:"tenant"`
	SiteID           uuid.UUID           `json:"site_id"`
	Slug             map[string]string   `json:"slug"`
	Name             map[string]string   `json:"name"`
	Description      map[string]string   `json:"description"`
	Category         string              `json:"category"`
	Images           []uuid.UUID         `json:"images"`
	VATRate          float64             `json:"vat_rate"`
	Options          []ProductOptionAxis `json:"options"`
	Kind             string              `json:"kind"` // "general", "food"
	GPSR             *ProductGPSR        `json:"gpsr,omitempty"`
	Food             *ProductFood        `json:"food,omitempty"`
	WithdrawalExempt string              `json:"withdrawal_exempt"` // "", "perishable", "custom", "sealed"
	MaxPerOrder      int                 `json:"max_per_order"`
	Active           bool                `json:"active"`
	Position         int                 `json:"position"`
	Variants         []Variant           `json:"variants,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// ProductOptionAxis define un eje de opciones (talla, color, acabado). Hasta 3.
type ProductOptionAxis struct {
	Key    string               `json:"key"`
	Name   map[string]string    `json:"name"`
	Values []ProductOptionValue `json:"values"`
}

// ProductOptionValue define un valor de un eje de opciones.
type ProductOptionValue struct {
	Key   string            `json:"key"`
	Label map[string]string `json:"label"`
}

// ProductGPSR contiene la ficha de seguridad general de productos (Reglamento GPSR UE 2023/988).
type ProductGPSR struct {
	ManufacturerName    string            `json:"manufacturer_name"`
	ManufacturerContact string            `json:"manufacturer_contact"`
	EUResponsible       string            `json:"eu_responsible"`
	Identifier          string            `json:"identifier"`
	Safety              map[string]string `json:"safety"`
}

// ProductFood contiene la información alimentaria obligatoria (§20.10).
type ProductFood struct {
	Ingredients map[string]string `json:"ingredients"`
	Allergens   []string          `json:"allergens"` // 14 de la lista cerrada
	NetQuantity string            `json:"net_quantity"`
}

// Variant representa una combinación concreta de opciones con precio y stock.
type Variant struct {
	ID             uuid.UUID         `json:"id"`
	Tenant         string            `json:"tenant"`
	SiteID         uuid.UUID         `json:"site_id"`
	ProductID      uuid.UUID         `json:"product_id"`
	SKU            string            `json:"sku"`
	Options        map[string]string `json:"options"`
	PriceCents     int               `json:"price_cents"`
	CompareAtCents *int              `json:"compare_at_cents,omitempty"`
	Stock          *int              `json:"stock"`
	Reserved       int               `json:"reserved"`
}

// Gateway representa la configuración y estado de una pasarela de pago guardada.
type Gateway struct {
	ID               uuid.UUID      `json:"id"`
	Tenant           string         `json:"tenant"`
	SiteID           uuid.UUID      `json:"site_id"`
	Provider         string         `json:"provider"` // mollie, adyen, redsys, stripe
	Mode             string         `json:"mode"`     // test, live
	CredentialsEnc   string         `json:"-"`        // encv1:... nunca se devuelve a la UI
	PublicConfig     map[string]any `json:"public_config"`
	NextRef          int64          `json:"-"`
	Active           bool           `json:"active"`
	NoticeAcceptedAt *time.Time     `json:"notice_accepted_at,omitempty"`
	NoticeAcceptedBy string         `json:"notice_accepted_by,omitempty"`
	CheckedAt        time.Time      `json:"checked_at"`
	CreatedBy        string         `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Estados de un pedido (§10.4-10.5).
const (
	OrderStatusPendingPayment    = "pending_payment"
	OrderStatusPaid              = "paid"
	OrderStatusFulfilled         = "fulfilled"
	OrderStatusCanceled          = "canceled"
	OrderStatusExpired           = "expired"
	OrderStatusRefunded          = "refunded"
	OrderStatusPartiallyRefunded = "partially_refunded"
	OrderStatusDisputed          = "disputed"
)

// Order representa un pedido en la tienda (§10.4).
type Order struct {
	ID               uuid.UUID      `json:"id"`
	Tenant           string         `json:"tenant"`
	SiteID           uuid.UUID      `json:"site_id"`
	Number           string         `json:"number"`
	Test             bool           `json:"test"`
	CheckoutToken    uuid.UUID      `json:"checkout_token"`
	AccessTokenHash  []byte         `json:"-"`
	Status           string         `json:"status"`
	Oversold         bool           `json:"oversold"`
	Currency         string         `json:"currency"`
	SubtotalCents    int            `json:"subtotal_cents"`
	ShippingCents    int            `json:"shipping_cents"`
	TotalCents       int            `json:"total_cents"`
	VATCents         int            `json:"vat_cents"`
	VATBreakdown     map[string]int `json:"vat_breakdown"`
	BuyerEmail       string         `json:"buyer_email"`
	BuyerName        string         `json:"buyer_name"`
	BuyerPhone       string         `json:"buyer_phone,omitempty"`
	ShippingMethod   ShippingMethod `json:"shipping_method"`
	ShippingAddress  map[string]any `json:"shipping_address,omitempty"`
	Invoice          map[string]any `json:"invoice,omitempty"`
	Language         string         `json:"language"`
	TermsVersion     int            `json:"terms_version"`
	TermsAcceptedAt  time.Time      `json:"terms_accepted_at"`
	ReservedUntil    time.Time      `json:"reserved_until"`
	PaidAt           *time.Time     `json:"paid_at,omitempty"`
	FulfilledAt      *time.Time     `json:"fulfilled_at,omitempty"`
	BuyerEmailSentAt *time.Time     `json:"buyer_email_sent_at,omitempty"`
	ERPStatus        string         `json:"erp_status"`
	ERPOrderRef      string         `json:"erp_order_ref,omitempty"`
	ERPInvoiceRef    string         `json:"erp_invoice_ref,omitempty"`
	AnonymizedAt     *time.Time     `json:"anonymized_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	Lines            []OrderLine    `json:"lines,omitempty"`
}

// OrderLine representa una línea de un pedido (§10.4).
type OrderLine struct {
	OrderID          uuid.UUID         `json:"order_id"`
	Tenant           string            `json:"tenant"`
	Position         int16             `json:"position"`
	VariantID        *uuid.UUID        `json:"variant_id,omitempty"`
	SKU              string            `json:"sku"`
	Name             string            `json:"name"`
	Options          map[string]string `json:"options"`
	Quantity         int               `json:"quantity"`
	UnitPriceCents   int               `json:"unit_price_cents"`
	VATRate          float64           `json:"vat_rate"`
	TotalCents       int               `json:"total_cents"`
	WithdrawalExempt string            `json:"withdrawal_exempt"`
}

// Estados de un intento de pago (§10.4-10.5).
const (
	PaymentStatusCreated           = "created"
	PaymentStatusOpen              = "open"
	PaymentStatusPending           = "pending"
	PaymentStatusAuthorized        = "authorized"
	PaymentStatusPaid              = "paid"
	PaymentStatusCanceled          = "canceled"
	PaymentStatusExpired           = "expired"
	PaymentStatusFailed            = "failed"
	PaymentStatusRefunded          = "refunded"
	PaymentStatusPartiallyRefunded = "partially_refunded"
	PaymentStatusChargedBack       = "charged_back"
)

// PagoCobrado dice si un estado de pago es de un cobro ya hecho: de ahí no se vuelve
// a abierto ni a fallido, y una nueva notificación no lo cambia (WEB-03).
func PagoCobrado(status string) bool {
	switch status {
	case PaymentStatusPaid, PaymentStatusRefunded, PaymentStatusPartiallyRefunded, PaymentStatusChargedBack:
		return true
	}
	return false
}

// Payment representa un intento de pago de un pedido (§10.4-10.5).
type Payment struct {
	ID               uuid.UUID `json:"id"`
	Tenant           string    `json:"tenant"`
	OrderID          uuid.UUID `json:"order_id"`
	GatewayID        uuid.UUID `json:"gateway_id"`
	Attempt          int16     `json:"attempt"`
	Provider         string    `json:"provider"`
	Method           string    `json:"method"`
	ProviderRef      string    `json:"provider_ref"`
	Status           string    `json:"status"`
	AmountCents      int       `json:"amount_cents"`
	RefundedCents    int       `json:"refunded_cents"`
	ChargedBackCents int       `json:"charged_back_cents"`
	IdempotencyKey   string    `json:"idempotency_key"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Refund representa un reembolso ejecutado sobre un pago (§10.5).
type Refund struct {
	ID             uuid.UUID  `json:"id"`
	Tenant         string     `json:"tenant"`
	PaymentID      uuid.UUID  `json:"payment_id"`
	AmountCents    int        `json:"amount_cents"`
	IdempotencyKey string     `json:"idempotency_key"`
	Status         string     `json:"status"` // pending, done, failed
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	DoneAt         *time.Time `json:"done_at,omitempty"`
}

// Withdrawal representa una solicitud de desistimiento legal (§10.7).
type Withdrawal struct {
	ID             uuid.UUID  `json:"id"`
	Tenant         string     `json:"tenant"`
	SiteID         uuid.UUID  `json:"site_id"`
	OrderID        uuid.UUID  `json:"order_id"`
	OrderNumber    string     `json:"order_number,omitempty"`
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	Source         string     `json:"source"` // form, token
	Reason         string     `json:"reason"`
	CreatedAt      time.Time  `json:"created_at"`
	AckSentAt      *time.Time `json:"ack_sent_at,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

// FormSettings define la configuración de un formulario público (§10.6, §9.5).
type FormSettings struct {
	SiteID  uuid.UUID           `json:"site_id"`
	Tenant  string              `json:"tenant"`
	FormID  string              `json:"form_id"`
	Enabled bool                `json:"enabled"`
	Notify  []StoreNotifyPerson `json:"notify"`
}

// FormSubmission registra un envío de formulario (§10.6).
type FormSubmission struct {
	ID          uuid.UUID      `json:"id"`
	Tenant      string         `json:"tenant"`
	SiteID      uuid.UUID      `json:"site_id"`
	FormID      string         `json:"form_id"`
	Data        map[string]any `json:"data"`
	Language    string         `json:"language"`
	ConsentText string         `json:"consent_text"`
	ConsentAt   time.Time      `json:"consent_at"`
	ReadAt      *time.Time     `json:"read_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// --- DTOs de Checkout y API pública (§10.4, §9.5, §6.4) ---

// CheckoutItem representa una línea solicitada en el checkout.
type CheckoutItem struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

// CheckoutBuyer representa los datos del comprador en el checkout.
type CheckoutBuyer struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Phone string `json:"phone,omitempty"`
}

// CheckoutShipping representa la elección de envío en el checkout.
type CheckoutShipping struct {
	MethodID string         `json:"method_id"`
	Address  map[string]any `json:"address,omitempty"`
}

// CheckoutInvoice representa los datos de facturación en el checkout.
type CheckoutInvoice struct {
	TaxID   string         `json:"tax_id"`
	Name    string         `json:"name"`
	Address map[string]any `json:"address,omitempty"`
}

// CheckoutPayment representa la pasarela y método elegidos.
type CheckoutPayment struct {
	Gateway string `json:"gateway"`
	Method  string `json:"method,omitempty"`
}

// CheckoutRequest es el cuerpo de POST /_pages/checkout (§10.4).
type CheckoutRequest struct {
	CheckoutToken uuid.UUID        `json:"checkout_token"`
	Items         []CheckoutItem   `json:"items"`
	Buyer         CheckoutBuyer    `json:"buyer"`
	Shipping      CheckoutShipping `json:"shipping"`
	Invoice       *CheckoutInvoice `json:"invoice,omitempty"`
	Payment       CheckoutPayment  `json:"payment"`
	TermsVersion  int              `json:"terms_version"`
	Language      string           `json:"language"`
	Altcha        string           `json:"altcha"`
	Base          string           `json:"base,omitempty"`
	ReturnURL     string           `json:"return_url,omitempty"`
}

// CheckoutResponse es la respuesta de POST /_pages/checkout (§10.4).
type CheckoutResponse struct {
	OrderID    uuid.UUID   `json:"order_id"`
	OrderToken string      `json:"order_token"`
	Pay        CheckoutPay `json:"pay"`
}

// CheckoutPay lleva la referencia para pagar en la pasarela.
type CheckoutPay struct {
	PaymentID  uuid.UUID      `json:"payment_id"`
	URL        string         `json:"url,omitempty"`
	Formulario map[string]any `json:"formulario,omitempty"`
}

// OrderPublicDTO es la vista pública reducida de un pedido (§6.4, S23).
type OrderPublicDTO struct {
	ID                   uuid.UUID      `json:"id"`
	Number               string         `json:"number"`
	Status               string         `json:"status"`
	CreatedAt            time.Time      `json:"created_at"`
	Currency             string         `json:"currency"`
	SubtotalCents        int            `json:"subtotal_cents"`
	ShippingCents        int            `json:"shipping_cents"`
	TotalCents           int            `json:"total_cents"`
	VATCents             int            `json:"vat_cents"`
	VATBreakdown         map[string]int `json:"vat_breakdown"`
	Items                []OrderLineDTO `json:"items"`
	WithdrawalPeriodDays int            `json:"withdrawal_period_days"`
	HasInvoice           bool           `json:"has_invoice"`
	ERPInvoiceRef        string         `json:"erp_invoice_ref,omitempty"`
}

// OrderLineDTO es una línea de pedido expuesta en la consulta pública.
type OrderLineDTO struct {
	SKU              string `json:"sku"`
	Name             string `json:"name"`
	Quantity         int    `json:"quantity"`
	UnitPriceCents   int    `json:"unit_price_cents"`
	TotalCents       int    `json:"total_cents"`
	WithdrawalExempt string `json:"withdrawal_exempt,omitempty"`
}

// CatalogItemDTO es la respuesta para un ítem en GET /_pages/catalog (§6.4).
type CatalogItemDTO struct {
	SKU            string `json:"sku"`
	PriceCents     int    `json:"price_cents"`
	CompareAtCents *int   `json:"compare_at_cents,omitempty"`
	Lowest30dCents *int   `json:"lowest_30d_cents,omitempty"`
	Available      bool   `json:"available"`
	StockState     string `json:"stock_state"` // in, low, out
}

// ProductPublicDTO es un producto expuesto en GET /_pages/products (§9.5).
type ProductPublicDTO struct {
	ID               uuid.UUID           `json:"id"`
	Slug             string              `json:"slug"`
	Name             string              `json:"name"`
	Description      string              `json:"description"`
	Category         string              `json:"category"`
	Images           []string            `json:"images"` // URLs absolutas
	VATRate          float64             `json:"vat_rate"`
	Options          []ProductOptionAxis `json:"options"`
	Kind             string              `json:"kind"`
	GPSR             *ProductGPSR        `json:"gpsr,omitempty"`
	Food             *ProductFood        `json:"food,omitempty"`
	WithdrawalExempt string              `json:"withdrawal_exempt"`
	MaxPerOrder      int                 `json:"max_per_order"`
	Variants         []VariantPublicDTO  `json:"variants"`
}

// VariantPublicDTO es una variante expuesta en GET /_pages/products (§9.5).
type VariantPublicDTO struct {
	ID             uuid.UUID         `json:"id"`
	SKU            string            `json:"sku"`
	Options        map[string]string `json:"options"`
	PriceCents     int               `json:"price_cents"`
	CompareAtCents *int              `json:"compare_at_cents,omitempty"`
	Available      bool              `json:"available"`
	StockState     string            `json:"stock_state"` // in, low, out
}

// FormPublicDTO es la definición pública de un formulario en GET /_pages/forms/{form_id} (§9.5).
type FormPublicDTO struct {
	FormID      string               `json:"form_id"`
	Fields      []ir.CampoFormulario `json:"fields"`
	ConsentText map[string]string    `json:"consent_text"`
}

// FormInbox es un formulario en la bandeja del editor: el de la versión en vivo, o uno
// que ya no está en ella pero guarda respuestas (Live = false).
type FormInbox struct {
	FormID string               `json:"form_id"`
	Name   string               `json:"name"`
	Live   bool                 `json:"live"`
	Fields []ir.CampoFormulario `json:"fields"`
	Notify []string             `json:"notify"`
	Total  int                  `json:"total"`
}
