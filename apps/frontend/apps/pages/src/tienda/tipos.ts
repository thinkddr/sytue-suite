/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tipos y utilidades compartidas por las secciones de la tienda de Sytue Web.
 */
import type { Locale } from '@sytue/i18n';
import { t as traducir, type ClaveMensaje } from '../i18n/index.js';

export type Traductor = (clave: ClaveMensaje, params?: Record<string, string | number>) => string;

export const traductor =
  (locale: Locale): Traductor =>
  (clave, params) =>
    traducir(locale, clave, params);

export interface ShippingMethod {
  id: string;
  name: Record<string, string>;
  price_cents: number;
  vat_rate: number;
  free_over_cents?: number;
  countries: string[];
  pickup: boolean;
}

export interface StoreSettings {
  site_id: string;
  enabled: boolean;
  currency: string;
  countries: string[];
  shipping: ShippingMethod[];
  terms: {
    delivery_time: Record<string, string>;
    returns_address: string;
  };
  terms_version: number;
  notify: Array<{ sub: string; email: string; name: string }>;
  reply_to: string;
  erp_enabled: boolean;
}

export interface ProductVariant {
  id?: string;
  sku: string;
  price_cents: number;
  compare_at_cents?: number;
  stock: number | null;
  options: Record<string, string>;
}

export interface Product {
  id: string;
  slug: Record<string, string>;
  name: Record<string, string>;
  description: Record<string, string>;
  category: string;
  vat_rate: number;
  kind: 'general' | 'food';
  withdrawal_exempt: '' | 'perishable' | 'custom' | 'sealed';
  max_per_order: number;
  active: boolean;
  gpsr?: {
    manufacturer_name: string;
    manufacturer_contact: string;
    eu_responsible: string;
    identifier: string;
    safety: Record<string, string>;
  };
  food?: {
    ingredients: Record<string, string>;
    allergens: string[];
    net_quantity: string;
  };
  variants?: ProductVariant[];
}

export type Proveedor = 'mollie' | 'adyen' | 'redsys' | 'stripe';
export const PROVEEDORES: readonly Proveedor[] = ['mollie', 'adyen', 'redsys', 'stripe'];
export type ModoPasarela = 'test' | 'live';

export interface Gateway {
  id: string;
  provider: Proveedor;
  mode: ModoPasarela;
  active: boolean;
  public_config: Record<string, unknown>;
  notice_accepted_at?: string;
}

/** Los 14 alérgenos de declaración obligatoria en la UE; el rótulo sale del catálogo. */
export const ALERGENOS_UE = [
  'cereales_gluten',
  'crustaceos',
  'huevos',
  'pescado',
  'cacahuetes',
  'soja',
  'lacteos',
  'frutos_cascara',
  'apio',
  'mostaza',
  'sesamo',
  'dioxido_azufre_sulfitos',
  'altramuces',
  'moluscos',
] as const;

/** Mensaje del servidor, si lo hay, a continuación del texto propio. */
export async function detalleDeError(res: Response): Promise<string> {
  const body = (await res.json().catch(() => ({}))) as { message?: string };
  return body.message ?? '';
}
