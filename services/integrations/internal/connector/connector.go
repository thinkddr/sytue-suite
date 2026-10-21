// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package connector es EL CONTRATO del Sytue Integration Gateway: la superficie
// mínima que implementa cada driver de conector (Salesforce, Slack, un ERP…) y
// que el núcleo de la pasarela conoce.
//
// Principio central (seguridad por construcción, no por convención): el código de
// un driver JAMÁS construye su propio http.Client ni ve tokens/secretos. Todo el
// egress a terceros sale por Runtime.HTTP —el ÚNICO cliente que recibe—, que la
// capa de políticas (internal/policy) fabrica con la cadena SSRF + rate-limit +
// auditoría + inyección de auth. La Instance que un driver recibe NUNCA lleva
// Secrets poblados: el token ya va inyectado en Runtime.HTTP por la pasarela, de
// modo que el driver no tiene forma —ni necesidad— de leer el secreto.
//
// Capacidades OPCIONALES por type-assert (idiom de services/hub): un Driver es
// obligatoriamente Test-able; si además implementa Provisioner/TabularSource/
// EmbedBroker/EventSink, el núcleo lo detecta con una aserción de tipo y expone
// esa capacidad. Nada monolítico: un driver implementa solo lo que soporta.
package connector

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Instance es una instancia configurada de un conector para una organización.
// Config es la configuración PÚBLICA (sin secretos). Secrets es el material
// sensible descifrado, presente SOLO en memoria y SOLO cuando el núcleo fabrica
// el Runtime; la Instance que se pasa a un Driver lo lleva vacío (ver principio
// del paquete).
type Instance struct {
	ID          string
	OrgID       string
	TemplateKey string
	Config      map[string]string
	Secrets     map[string]string
	// HasBearer dice si la pasarela va a INYECTAR un Bearer en este egress (la clave
	// convencional "bearer" del vault, que es donde el canje del CONSENTIMIENTO deja el
	// access token). El driver sigue sin ver el token —esa garantía no se toca—, pero
	// necesita saber que existe: los conectores que se fabrican su propia cabecera con
	// otras claves del vault (Jira con correo+API token, ServiceNow con usuario+
	// contraseña) deben APARTARSE cuando hay consentimiento. Si no, el admin autoriza,
	// el asistente dice «conectado» y el driver sigue usando la credencial vieja: el
	// consentimiento no serviría de nada y nadie lo notaría.
	HasBearer bool
}

// Runtime es lo ÚNICO que un Driver recibe para salir a la red. HTTP es un
// *http.Client fabricado por internal/policy (no hay otra fábrica): valida el
// destino contra SSRF, aplica rate-limit por org:instance, audita el egress e
// inyecta la auth desde el vault. Log es un logger ya etiquetado con la instancia.
type Runtime struct {
	HTTP *http.Client
	Log  *slog.Logger
}

// Driver es la capacidad OBLIGATORIA de todo conector: identificarse por Kind y
// verificar su configuración (Test). Las demás capacidades son opcionales.
type Driver interface {
	// Kind es el identificador estable del tipo de conector (clave del Registry).
	Kind() string
	// Test verifica de extremo a extremo que la instancia puede autenticarse y
	// alcanzar al tercero, usando EXCLUSIVAMENTE rt.HTTP. No devuelve datos.
	Test(ctx context.Context, inst Instance, rt Runtime) error
}

// --- Capacidades opcionales (type-assert) -----------------------------------

// Provisioner sincroniza recursos con el tercero (aprovisionamiento entrante/
// saliente de usuarios, grupos…). Lo implementan los conectores tipo SCIM/IdP.
type Provisioner interface {
	Sync(ctx context.Context, inst Instance, rt Runtime) error
}

// TabularSource expone datos del tercero como una tabla (mismo shape que el
// conector de datos de services/docs): columnas + filas. spec es la consulta
// específica del conector (JSON opaco para el núcleo).
type TabularSource interface {
	Query(ctx context.Context, inst Instance, rt Runtime, spec json.RawMessage) (Table, error)
}

// EmbedBroker emite un token/URL de incrustación firmada para embeber una vista
// del tercero (dashboards, informes) en la suite. req es la petición específica
// del conector (JSON opaco).
type EmbedBroker interface {
	EmbedToken(ctx context.Context, inst Instance, rt Runtime, req json.RawMessage) (Embed, error)
}

// EventSink recibe webhooks entrantes del tercero ya autenticados por el núcleo.
// body es el cuerpo crudo; headers, las cabeceras entrantes (para verificar la
// firma del webhook).
//
// ponytail: ningún driver la implementa todavía y no hay endpoint de webhooks en el
// router; es el contrato reservado para cuando lo haya, no una capacidad activa.
type EventSink interface {
	HandleWebhook(ctx context.Context, inst Instance, rt Runtime, body []byte, headers http.Header) error
}

// SearchCapability busca contenido del tercero (personas del directorio, ficheros…)
// para la búsqueda unificada de la suite: query es el término, limit el tope de
// resultados. Devuelve hits ya normalizados a ExternalHit. Egress por rt.HTTP (SSRF
// + rate-limit + auditoría). La implementa cada driver cuyo SaaS ofrezca búsqueda.
type SearchCapability interface {
	Search(ctx context.Context, inst Instance, rt Runtime, query string, limit int) ([]ExternalHit, error)
}

// ExternalHit es un resultado de búsqueda de un SaaS conectado, normalizado a la
// forma canónica que consume la búsqueda federada de Sytue Hub. Source es la
// etiqueta del SaaS de procedencia ("Microsoft 365", "Google Workspace"); Kind el
// tipo de entidad (person, file…); URL el deep-link de cara al usuario.
type ExternalHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Source  string `json:"source"`
}

// Table es el resultado tabular: columnas + filas (fila = valores alineados a
// Columns). Idéntico al shape de services/docs/internal/connectors.
type Table struct {
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Column es una columna del resultado: nombre + tipo lógico.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Embed es una incrustación acotada emitida por un EmbedBroker: la URL a cargar en
// el iframe + el artefacto de embed (token de vida corta, app-owns-data) con su tipo
// y su caducidad. El servidor mintea el token y lo entrega por el canal seguro
// (server-to-server → BFF → postMessage); NUNCA viaja en query string ni se persiste
// en el navegador. Token/TokenType van omitempty: un broker que embeba por URL firmada
// (sin token aparte) los deja vacíos.
type Embed struct {
	URL       string    `json:"embed_url"`
	Token     string    `json:"token,omitempty"`
	TokenType string    `json:"token_type,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// --- Registry ----------------------------------------------------------------

// Registry indexa los drivers disponibles por Kind. Es de solo lectura tras su
// construcción (se puebla al arrancar), así que no necesita sincronización.
type Registry struct {
	byKind map[string]Driver
}

// NewRegistry construye un Registry con los drivers dados. Un Kind duplicado se
// resuelve por el último (el llamante controla el orden al cablear).
func NewRegistry(drivers ...Driver) *Registry {
	m := make(map[string]Driver, len(drivers))
	for _, d := range drivers {
		if d != nil {
			m[d.Kind()] = d
		}
	}
	return &Registry{byKind: m}
}

// Get devuelve el driver de un Kind, si está registrado. El Kind lo nombra el
// manifiesto de la plantilla por capacidad (Manifest.DriverFor).
func (r *Registry) Get(kind string) (Driver, bool) {
	d, ok := r.byKind[kind]
	return d, ok
}

// Kinds devuelve los Kind registrados (orden no garantizado).
func (r *Registry) Kinds() []string {
	out := make([]string, 0, len(r.byKind))
	for k := range r.byKind {
		out = append(out, k)
	}
	return out
}
