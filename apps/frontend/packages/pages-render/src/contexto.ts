/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import type { Id, Textos } from './ir/tipos';

/** Datos inmutables que acompañan al IR durante un render. Es el cable de Pages. */
export interface DatosRender {
  site: {
    id: string;
    slug: string;
    sites_host: string;
    canonical_host: string;
    hosts: string[];
    analytics: boolean;
  };
  images: Record<
    string,
    { file: string; ext: 'jpg' | 'png'; width: number; height: number; variants: number[] }
  >;
  files: Record<string, { file: string; name: string; bytes: number }>;
  fonts: Record<
    string,
    {
      name: string;
      fallback: 'sans-serif' | 'serif';
      scripts: string[];
      faces: {
        file: string;
        style: 'normal' | 'italic';
        weight: string;
        unicode_range: string;
      }[];
    }
  >;
  legal: DatosLegales;
  store: DatosTienda | null;
  forms: Record<Id, { enabled: boolean }>;
  origins: { cast: string; pages: string };
  now: string;
}

export interface DatosLegales {
  holder_name: string;
  tax_id: string;
  country: string;
  address: { street: string; postal_code: string; city: string; region: string };
  email: string;
  phone: string;
  registry: string;
  privacy_contact: string;
  profession?: { title: string; college: string; number: string; country: string };
  authorization?: { authority: string; number: string };
}

export interface DatosTienda {
  mode: 'test' | 'live';
  checkout: boolean;
  currency: 'EUR';
  countries: string[];
  payment_options: {
    gateway: 'mollie' | 'adyen' | 'redsys' | 'stripe';
    method: string;
    label: Textos;
  }[];
  shipping: {
    id: string;
    name: Textos;
    price_cents: number;
    vat_rate: number;
    free_over_cents?: number;
    countries: string[];
    pickup: boolean;
  }[];
  terms: { delivery_time: Textos; returns_address: string };
  withdrawal_days: 14;
  products: ProductoRender[];
}

export interface ProductoRender {
  id: string;
  slug: Textos;
  name: Textos;
  description: Textos;
  category: string;
  images: string[];
  vat_rate: number;
  kind: 'general' | 'food';
  max_per_order: number;
  withdrawal_exempt: '' | 'perishable' | 'custom' | 'sealed';
  options: { key: string; name: Textos; values: { key: string; label: Textos }[] }[];
  gpsr: {
    manufacturer_name: string;
    manufacturer_contact: string;
    eu_responsible: string;
    identifier: string;
    safety: Textos;
  } | null;
  food: { ingredients: Textos; allergens: string[]; net_quantity: string } | null;
  variants: {
    sku: string;
    options: Record<string, string>;
    price_cents: number;
    compare_at_cents: number | null;
    lowest_30d_cents: number;
    available: boolean;
  }[];
}
