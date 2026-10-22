// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConflictoBorrador = errors.New("pages: conflicto de borrador")
	ErrBorradorInvalido  = errors.New("pages: borrador inválido")
)

// Sitio es el resumen privado de una web. El IR queda crudo a propósito: volver
// a serializarlo perdería los tres estados de los deltas del contrato.
type Sitio struct {
	ID       uuid.UUID       `json:"id"`
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	Template string          `json:"template"`
	Draft    json.RawMessage `json:"draft"`
	DraftRev int64           `json:"draft_rev"`
}

// SitioCompleto contiene todos los campos de la tabla sites.
type SitioCompleto struct {
	ID               uuid.UUID       `json:"id"`
	Tenant           string          `json:"tenant"`
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Template         string          `json:"template"`
	Status           string          `json:"status"`
	SuspendedReason  string          `json:"suspended_reason"`
	Draft            json.RawMessage `json:"draft"`
	DraftRev         int64           `json:"draft_rev"`
	PublishedNumber  *int            `json:"published_number"`
	CanonicalHost    *string         `json:"canonical_host"`
	Legal            DatosLegales    `json:"legal"`
	AnalyticsEnabled bool            `json:"analytics_enabled"`
	AnalyticsSiteID  string          `json:"analytics_site_id"`
	ExportOrigins    []string        `json:"export_origins"`
	ExportAppSchemes []string        `json:"export_app_schemes"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	DeletedAt        *time.Time      `json:"deleted_at"`
}

// ResultadoBusqueda es lo que enseña un resultado del buscador del Hub: lo justo para
// nombrar el sitio y abrirlo, sin el borrador.
type ResultadoBusqueda struct {
	ID              uuid.UUID `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	PublishedNumber *int      `json:"published_number"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type VersionSitio struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	IR        json.RawMessage `json:"ir,omitempty"`
	DraftRev  int64           `json:"draft_rev"`
	CreatedBy string          `json:"created_by,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

// NormalizarNombreVersion rechaza nombres invisibles y deja el límite también
// aquí, antes de la restricción de PostgreSQL.
func NormalizarNombreVersion(nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if len(nombre) > 120 {
		return "", ErrBorradorInvalido
	}
	return nombre, nil
}

// Snapshot es el cable de renderizado congelado (§6.3, §1561).
type Snapshot struct {
	VersionID uuid.UUID       `json:"version_id"`
	IRVersion int             `json:"ir_version"`
	IR        json.RawMessage `json:"ir"`
	Rev       int64           `json:"rev"`
	Data      DatosRender     `json:"data"`
}

// DatosRender agrupa los datos estáticos que acompañan al IR (§7.2).
type DatosRender struct {
	Site    SitioRender                 `json:"site"`
	Images  map[string]ImagenRender     `json:"images"`
	Files   map[string]FicheroRender    `json:"files"`
	Fonts   map[string]FuenteRender     `json:"fonts"`
	Legal   DatosLegales                `json:"legal"`
	Store   *DatosTienda                `json:"store"`
	Forms   map[string]FormularioRender `json:"forms"`
	Origins OrigenesRender              `json:"origins"`
	Now     string                      `json:"now"`
}

type SitioRender struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	SitesHost     string   `json:"sites_host"`
	CanonicalHost string   `json:"canonical_host"`
	Hosts         []string `json:"hosts"`
	Analytics     bool     `json:"analytics"`
}

type ImagenRender struct {
	File     string `json:"file"`
	Ext      string `json:"ext"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Variants []int  `json:"variants"`
}

type FicheroRender struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type FuenteRender struct {
	Name     string             `json:"name"`
	Fallback string             `json:"fallback"`
	Scripts  []string           `json:"scripts"`
	Faces    []FuenteCaraRender `json:"faces"`
}

type FuenteCaraRender struct {
	File         string `json:"file"`
	Style        string `json:"style"`
	Weight       string `json:"weight"`
	UnicodeRange string `json:"unicode_range"`
}

type FormularioRender struct {
	Enabled bool `json:"enabled"`
}

type OrigenesRender struct {
	Cast  string `json:"cast"`
	Pages string `json:"pages"`
}

type DatosTienda struct {
	Mode           string             `json:"mode"`
	Checkout       bool               `json:"checkout"`
	Currency       string             `json:"currency"`
	Countries      []string           `json:"countries"`
	PaymentOptions []OpcionPagoRender `json:"payment_options"`
	Shipping       []EnvioRender      `json:"shipping"`
	Terms          TerminosTienda     `json:"terms"`
	WithdrawalDays int                `json:"withdrawal_days"`
	Products       []ProductoRender   `json:"products"`
}

type OpcionPagoRender struct {
	Gateway string            `json:"gateway"`
	Method  string            `json:"method"`
	Label   map[string]string `json:"label"`
}

type EnvioRender struct {
	ID            string            `json:"id"`
	Name          map[string]string `json:"name"`
	PriceCents    int64             `json:"price_cents"`
	VATRate       float64           `json:"vat_rate"`
	FreeOverCents *int64            `json:"free_over_cents,omitempty"`
	Countries     []string          `json:"countries"`
	Pickup        bool              `json:"pickup"`
}

type TerminosTienda struct {
	DeliveryTime   map[string]string `json:"delivery_time"`
	ReturnsAddress string            `json:"returns_address"`
}

type ProductoRender struct {
	ID               string             `json:"id"`
	Slug             map[string]string  `json:"slug"`
	Name             map[string]string  `json:"name"`
	Description      map[string]string  `json:"description"`
	Category         string             `json:"category"`
	Images           []string           `json:"images"`
	VATRate          float64            `json:"vat_rate"`
	Kind             string             `json:"kind"`
	MaxPerOrder      int                `json:"max_per_order"`
	WithdrawalExempt string             `json:"withdrawal_exempt"`
	Options          []OpcionProducto   `json:"options"`
	GPSR             *GPSRProducto      `json:"gpsr"`
	Food             *AlimentoProducto  `json:"food"`
	Variants         []VarianteProducto `json:"variants"`
}

type OpcionProducto struct {
	Key    string                `json:"key"`
	Name   map[string]string     `json:"name"`
	Values []ValorOpcionProducto `json:"values"`
}

type ValorOpcionProducto struct {
	Key   string            `json:"key"`
	Label map[string]string `json:"label"`
}

type GPSRProducto struct {
	ManufacturerName    string            `json:"manufacturer_name"`
	ManufacturerContact string            `json:"manufacturer_contact"`
	EUResponsible       string            `json:"eu_responsible"`
	Identifier          string            `json:"identifier"`
	Safety              map[string]string `json:"safety"`
}

type AlimentoProducto struct {
	Ingredients map[string]string `json:"ingredients"`
	Allergens   []string          `json:"allergens"`
	NetQuantity string            `json:"net_quantity"`
}

type VarianteProducto struct {
	SKU            string            `json:"sku"`
	Options        map[string]string `json:"options"`
	PriceCents     int64             `json:"price_cents"`
	CompareAtCents *int64            `json:"compare_at_cents"`
	Lowest30dCents int64             `json:"lowest_30d_cents"`
	Available      bool              `json:"available"`
}

type Publicacion struct {
	ID              uuid.UUID       `json:"id"`
	SiteID          uuid.UUID       `json:"site_id"`
	Kind            string          `json:"kind"`
	Number          *int            `json:"number,omitempty"`
	PreviewKey      *string         `json:"preview_key,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	VersionID       uuid.UUID       `json:"version_id"`
	Files           int             `json:"files"`
	Bytes           int64           `json:"bytes"`
	RendererVersion string          `json:"renderer_version"`
	DataHash        json.RawMessage `json:"data_hash"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       time.Time       `json:"created_at"`
	Current         bool            `json:"current"`
}

type Redirect struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
}

type CrearSitioInput struct {
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	Template  string   `json:"template"`
	Languages []string `json:"languages,omitempty"`
}

// ActualizarSitioInput es lo que quien edita puede cambiar de un sitio: se decodifica tal
// cual del cuerpo de PUT /sites/{id}. Por eso NO lleva `canonical_host` ni
// `analytics_site_id`: son del servidor (store.FijarCanonico, store.FijarAnaliticaSitio).
// Mientras estuvieron aquí, cualquiera ponía como canónico el host de otro sitio (WEB-01).
type ActualizarSitioInput struct {
	Name             *string       `json:"name,omitempty"`
	Slug             *string       `json:"slug,omitempty"`
	Legal            *DatosLegales `json:"legal,omitempty"`
	AnalyticsEnabled *bool         `json:"analytics_enabled,omitempty"`
	ExportOrigins    *[]string     `json:"export_origins,omitempty"`
	ExportAppSchemes *[]string     `json:"export_app_schemes,omitempty"`
}
