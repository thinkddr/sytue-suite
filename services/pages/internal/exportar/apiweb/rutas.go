// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package apiweb

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// MetodoHTTP define el verbo HTTP de la ruta pública.
type MetodoHTTP string

const (
	GET  MetodoHTTP = "GET"
	POST MetodoHTTP = "POST"
)

// Parametro define un parámetro de query o path.
type Parametro struct {
	Nombre      string
	Ubicacion   string // "path" | "query" | "header"
	Requerido   bool
	Tipo        string
	Descripcion string
}

// Ruta define una operación pública bajo /_pages/* según openapi-web.yaml y §9.5.
type Ruta struct {
	Metodo              MetodoHTTP
	Patron              string
	OperacionID         string
	Resumen             string
	Descripcion         string
	Parametros          []Parametro
	CuerpoRequerido     bool
	TipoCuerpo          string
	TipoRespuesta       string
	CodigoExito         int
	RequiereTokenPedido bool
	RequiereAltcha      bool
	EsIdempotente       bool
}

// RutasPublicas contiene el catálogo normativo de las 12 rutas públicas de Pages.
var RutasPublicas = []Ruta{
	{
		Metodo:        GET,
		Patron:        "/_pages/altcha",
		OperacionID:   "getAltcha",
		Resumen:       "Obtener reto ALTCHA",
		Descripcion:   "Emite un reto ALTCHA criptográfico válido durante 10 minutos (§6.4).",
		TipoRespuesta: "AltchaChallenge",
		CodigoExito:   200,
		EsIdempotente: true,
	},
	{
		Metodo:      GET,
		Patron:      "/_pages/catalog",
		OperacionID: "getCatalog",
		Resumen:     "Consultar precios y disponibilidad",
		Descripcion: "Devuelve precio actual, comparativa y existencias para SKUs dados (§6.4).",
		Parametros: []Parametro{
			{Nombre: "skus", Ubicacion: "query", Requerido: true, Tipo: "string", Descripcion: "Lista de SKUs separados por coma"},
		},
		TipoRespuesta: "CatalogResponse",
		CodigoExito:   200,
		EsIdempotente: true,
	},
	{
		Metodo:      GET,
		Patron:      "/_pages/products",
		OperacionID: "getProducts",
		Resumen:     "Listar productos publicados",
		Descripcion: "Devuelve catálogo con imágenes absolutas y variantes (§9.5).",
		Parametros: []Parametro{
			{Nombre: "lang", Ubicacion: "query", Requerido: false, Tipo: "string", Descripcion: "Idioma preferido"},
			{Nombre: "cursor", Ubicacion: "query", Requerido: false, Tipo: "string", Descripcion: "Cursor de paginación"},
		},
		TipoRespuesta: "ProductsResponse",
		CodigoExito:   200,
		EsIdempotente: true,
	},
	{
		Metodo:      GET,
		Patron:      "/_pages/forms/{form_id}",
		OperacionID: "getFormDefinition",
		Resumen:     "Obtener definición pública del formulario",
		Descripcion: "Devuelve campos, requisitos y consentimientos del formulario publicado (§9.5).",
		Parametros: []Parametro{
			{Nombre: "form_id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador del formulario"},
		},
		TipoRespuesta: "FormDefinition",
		CodigoExito:   200,
		EsIdempotente: true,
	},
	{
		Metodo:      POST,
		Patron:      "/_pages/forms/{form_id}",
		OperacionID: "submitForm",
		Resumen:     "Enviar formulario público",
		Descripcion: "Registra respuestas validadas contra el formulario publicado (§10.6).",
		Parametros: []Parametro{
			{Nombre: "form_id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador del formulario"},
		},
		CuerpoRequerido: true,
		TipoCuerpo:      "FormSubmission",
		CodigoExito:     204,
		RequiereAltcha:  true,
	},
	{
		Metodo:          POST,
		Patron:          "/_pages/checkout",
		OperacionID:     "checkout",
		Resumen:         "Crear pedido y reservar existencias",
		Descripcion:     "Calcula total, reserva stock por 35 min y prepara pasarela (§10.4).",
		CuerpoRequerido: true,
		TipoCuerpo:      "CheckoutRequest",
		TipoRespuesta:   "CheckoutResponse",
		CodigoExito:     200,
		RequiereAltcha:  true,
		EsIdempotente:   true,
	},
	{
		Metodo:      POST,
		Patron:      "/_pages/pay/{payment_id}",
		OperacionID: "pay",
		Resumen:     "Proceder al pago",
		Descripcion: "Envía a la pasarela (303) o genera formulario autoenviado de Redsys (§10.4).",
		Parametros: []Parametro{
			{Nombre: "payment_id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador de pago"},
		},
		CuerpoRequerido: true,
		TipoCuerpo:      "PayRequest",
		TipoRespuesta:   "PayResponse",
		CodigoExito:     200,
	},
	{
		Metodo:      GET,
		Patron:      "/_pages/orders/{id}",
		OperacionID: "getOrder",
		Resumen:     "Consultar estado del pedido",
		Descripcion: "Devuelve vista pública reducida requiriendo token en X-Sytue-Pedido (§6.4, S23).",
		Parametros: []Parametro{
			{Nombre: "id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador del pedido"},
			{Nombre: "X-Sytue-Pedido", Ubicacion: "header", Requerido: true, Tipo: "string", Descripcion: "Token secreto del pedido"},
		},
		TipoRespuesta:       "OrderResponse",
		CodigoExito:         200,
		RequiereTokenPedido: true,
		EsIdempotente:       true,
	},
	{
		Metodo:      POST,
		Patron:      "/_pages/orders/{id}/invoice",
		OperacionID: "getInvoice",
		Resumen:     "Solicitar factura del pedido",
		Descripcion: "Devuelve HTML imprimible de la factura si está en el ERP (§10.8).",
		Parametros: []Parametro{
			{Nombre: "id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador del pedido"},
		},
		CuerpoRequerido: true,
		TipoCuerpo:      "PayRequest",
		TipoRespuesta:   "string",
		CodigoExito:     200,
		EsIdempotente:   true,
	},
	{
		Metodo:          POST,
		Patron:          "/_pages/withdrawals",
		OperacionID:     "submitWithdrawal",
		Resumen:         "Solicitar desistimiento legal",
		Descripcion:     "Registra desistimiento legal desde página legal (§10.7). Siempre 204.",
		CuerpoRequerido: true,
		TipoCuerpo:      "WithdrawalRequest",
		CodigoExito:     204,
		RequiereAltcha:  true,
	},
	{
		Metodo:      POST,
		Patron:      "/_pages/orders/{id}/withdrawal",
		OperacionID: "submitOrderWithdrawal",
		Resumen:     "Solicitar desistimiento autenticado",
		Descripcion: "Registra desistimiento prerrellenado con X-Sytue-Pedido (§10.7).",
		Parametros: []Parametro{
			{Nombre: "id", Ubicacion: "path", Requerido: true, Tipo: "string", Descripcion: "Identificador del pedido"},
			{Nombre: "X-Sytue-Pedido", Ubicacion: "header", Requerido: true, Tipo: "string", Descripcion: "Token secreto del pedido"},
		},
		CuerpoRequerido:     true,
		TipoCuerpo:          "OrderWithdrawalRequest",
		CodigoExito:         204,
		RequiereTokenPedido: true,
		RequiereAltcha:      true,
	},
	{
		Metodo:          POST,
		Patron:          "/_pages/reports",
		OperacionID:     "submitReport",
		Resumen:         "Enviar denuncia de contenido",
		Descripcion:     "Notifica informe de abuso o contenido ilegal (§8.3).",
		CuerpoRequerido: true,
		TipoCuerpo:      "ReportRequest",
		CodigoExito:     204,
		RequiereAltcha:  true,
	},
}

const plantillaClienteTS = `// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Generado automáticamente por apiweb/rutas.go. NO EDITAR DIRECTAMENTE.

/**
 * Host canónico base para la API pública de Sytue Web (§9.5).
 */
export const PAGES_BASE_URL: string = {{.BaseURLJSON}};

/* -------------------------------------------------------------------------
 * Tipos de datos de la API pública /_pages/*
 * ------------------------------------------------------------------------- */

export interface AltchaChallenge {
  algorithm: string;
  challenge: string;
  maxnumber?: number;
  salt: string;
  signature: string;
}

export interface AltchaPayload {
  algorithm: string;
  challenge: string;
  number: number;
  salt: string;
  signature: string;
}

export interface CatalogItem {
  sku: string;
  price_cents: number;
  compare_at_cents?: number | null;
  lowest_30d_cents?: number | null;
  available: boolean;
  stock_state: 'in' | 'low' | 'out';
}

export interface CatalogResponse {
  items: CatalogItem[];
}

export interface ProductVariant {
  id: string;
  sku: string;
  options: Record<string, string>;
  price_cents: number;
  compare_at_cents?: number | null;
  available: boolean;
  stock_state: 'in' | 'low' | 'out';
}

export interface Product {
  id: string;
  slug: string;
  name: string;
  description: string;
  category: string;
  images: string[];
  vat_rate: number;
  options: Array<Record<string, unknown>>;
  kind: 'general' | 'food';
  gpsr?: Record<string, unknown>;
  food?: Record<string, unknown>;
  withdrawal_exempt: string;
  max_per_order: number;
  variants: ProductVariant[];
}

export interface ProductsResponse {
  items: Product[];
  next_cursor?: string | null;
}

export interface FormField {
  id: string;
  kind: string;
  label: Record<string, string>;
  required: boolean;
  options?: Record<string, string>[];
  max_length?: number;
  min?: number;
  max?: number;
  autocomplete?: string;
}

export interface FormDefinition {
  form_id: string;
  fields: FormField[];
  consent_text: Record<string, string>;
}

export interface FormSubmission {
  fields: Record<string, unknown>;
  consent: boolean;
  language: string;
  altcha: string;
}

export interface CheckoutItem {
  sku: string;
  qty: number;
}

export interface BuyerInfo {
  email: string;
  name: string;
  phone?: string;
}

export interface ShippingInfo {
  method_id: string;
  address?: Record<string, unknown>;
}

export interface InvoiceInfo {
  tax_id: string;
  name: string;
  address?: Record<string, unknown>;
}

export interface PaymentInfo {
  gateway: 'mollie' | 'adyen' | 'redsys' | 'stripe';
  method?: string;
}

export interface CheckoutRequest {
  checkout_token: string;
  items: CheckoutItem[];
  buyer: BuyerInfo;
  shipping: ShippingInfo;
  invoice?: InvoiceInfo;
  payment: PaymentInfo;
  terms_version: number;
  language: string;
  altcha: string;
  base?: string;
  return_url?: string;
}

export interface CheckoutResponse {
  order_id: string;
  order_token: string;
  pay: {
    payment_id: string;
    url?: string;
    formulario?: Record<string, unknown>;
  };
}

export interface PayRequest {
  token: string;
}

export interface PayResponse {
  url?: string;
  formulario?: Record<string, unknown>;
}

export interface OrderItem {
  sku: string;
  name: string;
  price_cents: number;
  qty: number;
  total_cents: number;
}

export interface OrderResponse {
  id: string;
  number: string;
  status: string;
  created_at: string;
  currency: string;
  subtotal_cents: number;
  shipping_cents: number;
  total_cents: number;
  vat_cents: number;
  vat_breakdown: Record<string, unknown>;
  items: OrderItem[];
  withdrawal_period_days: number;
  has_invoice: boolean;
  erp_invoice_ref?: string;
}

export interface WithdrawalRequest {
  order_number: string;
  email: string;
  name?: string;
  reason?: string;
  altcha: string;
}

export interface OrderWithdrawalRequest {
  name?: string;
  reason?: string;
  altcha: string;
}

export interface ReportRequest {
  reason: 'phishing' | 'malware' | 'illegal' | 'copyright' | 'other';
  details: string;
  email?: string;
  path?: string;
  altcha: string;
}

/* -------------------------------------------------------------------------
 * Resolvedor determinista de ALTCHA (SHA-256 nativo)
 * ------------------------------------------------------------------------- */

function toBase64(str: string): string {
  if (typeof btoa === 'function') {
    return btoa(str);
  }
  if (typeof Buffer !== 'undefined') {
    return Buffer.from(str, 'utf-8').toString('base64');
  }
  throw new Error('altcha: entorno sin soporte para codificación base64');
}

async function sha256Hex(text: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(text);

  if (typeof globalThis !== 'undefined' && globalThis.crypto?.subtle) {
    const hashBuffer = await globalThis.crypto.subtle.digest('SHA-256', data);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
  }

  // Fallback para entornos Node sin global crypto.subtle inicializado
  try {
    const nodeCrypto = await import('node:crypto');
    return nodeCrypto.createHash('sha256').update(data).digest('hex');
  } catch {
    throw new Error('altcha: entorno sin implementación de SHA-256');
  }
}

/**
 * Resuelve un reto ALTCHA determinista buscando el número que satisface
 * sha256(salt + number) === challenge (§9.5, platform/pkg/altcha).
 */
export async function resolverAltcha(challenge: AltchaChallenge): Promise<string> {
  if (challenge.algorithm !== 'SHA-256') {
    throw new Error('altcha: algoritmo no soportado: ' + challenge.algorithm);
  }

  const maxNumber = challenge.maxnumber && challenge.maxnumber > 0 ? challenge.maxnumber : 50000;
  const target = challenge.challenge.toLowerCase();
  const salt = challenge.salt;

  for (let n = 0; n <= maxNumber; n++) {
    const hash = await sha256Hex(salt + n);
    if (hash === target) {
      const payload: AltchaPayload = {
        algorithm: challenge.algorithm,
        challenge: challenge.challenge,
        number: n,
        salt: challenge.salt,
        signature: challenge.signature,
      };
      return toBase64(JSON.stringify(payload));
    }
  }

  throw new Error('altcha: no se encontró solución dentro del rango [0, ' + maxNumber + ']');
}

/* -------------------------------------------------------------------------
 * Cliente HTTP con reintentos solo en GET (§9.5)
 * ------------------------------------------------------------------------- */

export interface ClientConfig {
  baseUrl?: string;
  fetchFn?: typeof fetch;
}

export class PagesApiClient {
  private readonly baseUrl: string;
  private readonly customFetch: typeof fetch;

  constructor(config: ClientConfig = {}) {
    let base = config.baseUrl || PAGES_BASE_URL;
    if (base.endsWith('/')) {
      base = base.slice(0, -1);
    }
    this.baseUrl = base;
    this.customFetch = config.fetchFn || globalThis.fetch;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    isGet = false
  ): Promise<T> {
    const url = this.baseUrl + endpoint;
    const maxRetries = isGet ? 3 : 1;
    let attempt = 0;
    let lastError: unknown;

    while (attempt < maxRetries) {
      attempt++;
      try {
        const response = await this.customFetch(url, options);

        if (response.status === 204) {
          return undefined as unknown as T;
        }

        if (!response.ok) {
          if (isGet && response.status >= 500 && attempt < maxRetries) {
            await new Promise((r) => setTimeout(r, 100 * Math.pow(2, attempt - 1)));
            continue;
          }
          const errText = await response.text();
          throw new Error('pages-api: error HTTP ' + response.status + ': ' + errText);
        }

        const contentType = response.headers.get('content-type') || '';
        if (contentType.includes('application/json')) {
          return (await response.json()) as T;
        }
        return (await response.text()) as unknown as T;
      } catch (err) {
        lastError = err;
        if (isGet && attempt < maxRetries) {
          await new Promise((r) => setTimeout(r, 100 * Math.pow(2, attempt - 1)));
          continue;
        }
        throw err;
      }
    }

    throw lastError;
  }

  /** GET /_pages/altcha - Reto ALTCHA (§6.4) */
  async getAltcha(): Promise<AltchaChallenge> {
    return this.request<AltchaChallenge>('/_pages/altcha', { method: 'GET' }, true);
  }

  /** GET /_pages/catalog?skus=... - Precios y existencias (§6.4) */
  async getCatalog(skus: string[]): Promise<CatalogResponse> {
    const query = encodeURIComponent(skus.join(','));
    return this.request<CatalogResponse>('/_pages/catalog?skus=' + query, { method: 'GET' }, true);
  }

  /** GET /_pages/products - Listar catálogo publicado (§9.5) */
  async getProducts(params?: { lang?: string; cursor?: string }): Promise<ProductsResponse> {
    const searchParams = new URLSearchParams();
    if (params?.lang) searchParams.set('lang', params.lang);
    if (params?.cursor) searchParams.set('cursor', params.cursor);
    const qs = searchParams.toString();
    const endpoint = '/_pages/products' + (qs ? '?' + qs : '');
    return this.request<ProductsResponse>(endpoint, { method: 'GET' }, true);
  }

  /** GET /_pages/forms/{form_id} - Definición pública del formulario (§9.5) */
  async getFormDefinition(formId: string): Promise<FormDefinition> {
    return this.request<FormDefinition>('/_pages/forms/' + encodeURIComponent(formId), { method: 'GET' }, true);
  }

  /** POST /_pages/forms/{form_id} - Enviar formulario público (§10.6) */
  async submitForm(formId: string, data: FormSubmission): Promise<void> {
    return this.request<void>('/_pages/forms/' + encodeURIComponent(formId), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
  }

  /** POST /_pages/checkout - Crear pedido y reservar stock (§10.4) */
  async checkout(req: CheckoutRequest): Promise<CheckoutResponse> {
    return this.request<CheckoutResponse>('/_pages/checkout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
  }

  /** POST /_pages/pay/{payment_id} - Proceder al pago (§10.4) */
  async pay(paymentId: string, orderToken: string): Promise<PayResponse> {
    return this.request<PayResponse>('/_pages/pay/' + encodeURIComponent(paymentId), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: orderToken }),
    });
  }

  /** GET /_pages/orders/{id} - Consultar estado del pedido con token (§6.4) */
  async getOrder(orderId: string, orderToken: string): Promise<OrderResponse> {
    return this.request<OrderResponse>(
      '/_pages/orders/' + encodeURIComponent(orderId),
      {
        method: 'GET',
        headers: { 'X-Sytue-Pedido': orderToken },
      },
      true
    );
  }

  /** POST /_pages/orders/{id}/invoice - Factura imprimible en HTML (§10.8) */
  async getInvoice(orderId: string, orderToken: string): Promise<string> {
    return this.request<string>('/_pages/orders/' + encodeURIComponent(orderId) + '/invoice', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: orderToken }),
    });
  }

  /** POST /_pages/withdrawals - Desistimiento legal no autenticado (§10.7) */
  async submitWithdrawal(req: WithdrawalRequest): Promise<void> {
    return this.request<void>('/_pages/withdrawals', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
  }

  /** POST /_pages/orders/{id}/withdrawal - Desistimiento autenticado con token (§10.7) */
  async submitOrderWithdrawal(
    orderId: string,
    orderToken: string,
    req: OrderWithdrawalRequest
  ): Promise<void> {
    return this.request<void>('/_pages/orders/' + encodeURIComponent(orderId) + '/withdrawal', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Sytue-Pedido': orderToken,
      },
      body: JSON.stringify(req),
    });
  }

  /** POST /_pages/reports - Enviar denuncia de contenido (§8.3) */
  async submitReport(req: ReportRequest): Promise<void> {
    return this.request<void>('/_pages/reports', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
  }
}

/** Instancia preconfigurada por defecto */
export const pagesApi = new PagesApiClient();
`

// GenerarClienteTS compila la plantilla Go determinista produciendo el archivo pages-api.ts.
func GenerarClienteTS(baseURL string) ([]byte, error) {
	if baseURL == "" {
		baseURL = "https://ejemplo.sytue.eu"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	tmpl, err := template.New("pages-api.ts").Parse(plantillaClienteTS)
	if err != nil {
		return nil, fmt.Errorf("apiweb: error al parsear plantilla TS: %w", err)
	}

	datos := struct {
		BaseURLJSON string
	}{
		BaseURLJSON: fmt.Sprintf("%q", baseURL),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, datos); err != nil {
		return nil, fmt.Errorf("apiweb: error al renderizar plantilla TS: %w", err)
	}

	return buf.Bytes(), nil
}
