/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica pura de la ficha del sitio: qué datos legales faltan para publicar y a qué mensaje del
 * catálogo se traduce cada rechazo del servicio. Vive aparte de la isla para poder probarse sin
 * montar nada.
 */
import type { ClaveMensaje } from '../i18n/index.js';
import { ruta } from './rutas.js';

export interface DatosLegales {
  holder_name?: string;
  tax_id?: string;
  country?: string;
  address?: {
    street?: string;
    postal_code?: string;
    city?: string;
    region?: string;
  };
  email?: string;
  phone?: string;
  registry?: string;
  privacy_contact?: string;
}

export type CampoLegal =
  'holder_name' | 'tax_id' | 'country' | 'street' | 'postal_code' | 'city' | 'email' | 'registry';

const vacio = (v: string | undefined) => !v || v.trim() === '';

/**
 * Sociedades españolas (NIF que empieza por A, B, C, D o F): el Registro Mercantil es obligatorio.
 * Es la misma regla que `domain.EsLegalCompleto` en el servicio, y tiene que seguir siéndolo: si la
 * ficha exigiera menos, dejaría pulsar «Publicar» para recibir un 422; si exigiera más, bloquearía
 * lo que el servidor admite.
 */
export function exigeRegistro(legal: DatosLegales): boolean {
  const pais = (legal.country ?? '').trim().toUpperCase();
  const nif = (legal.tax_id ?? '').trim().toUpperCase();
  return pais === 'ES' && nif !== '' && 'ABCDF'.includes(nif[0] ?? '');
}

/** Campos que faltan para que los datos legales cuenten como completos, en el orden del formulario. */
export function legalFaltante(legal: DatosLegales | undefined): CampoLegal[] {
  const l = legal ?? {};
  const faltan: CampoLegal[] = [];
  if (vacio(l.holder_name)) faltan.push('holder_name');
  if (vacio(l.tax_id)) faltan.push('tax_id');
  if (vacio(l.country)) faltan.push('country');
  if (vacio(l.address?.street)) faltan.push('street');
  if (vacio(l.address?.postal_code)) faltan.push('postal_code');
  if (vacio(l.address?.city)) faltan.push('city');
  if (vacio(l.email)) faltan.push('email');
  if (exigeRegistro(l) && vacio(l.registry)) faltan.push('registry');
  return faltan;
}

/** Cómo contesta el servicio cuando algo falla: `{ error: { code, message } }`. */
export interface FalloApi {
  status: number;
  code: string;
  details?: Record<string, unknown> | undefined;
}

/** Lee el fallo de una respuesta no satisfactoria. Un cuerpo ilegible no lo convierte en excepción. */
export async function leerFallo(res: Response): Promise<FalloApi> {
  const cuerpo = (await res.json().catch(() => null)) as {
    error?: { code?: string; details?: Record<string, unknown> };
  } | null;
  return { status: res.status, code: cuerpo?.error?.code ?? '', details: cuerpo?.error?.details };
}

export type IntentoPublicacion =
  { ok: true; url?: string | undefined } | { ok: false; fallo: FalloApi };

/** El editor y la ficha publican por el mismo BFF, con sesión y organización. */
export async function ejecutarPublicacion(
  siteId: string,
  kind: 'preview' | 'live',
): Promise<IntentoPublicacion> {
  try {
    const res = await fetch(ruta(`/api/publicar/${encodeURIComponent(siteId)}`), {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ kind }),
    });
    if (!res.ok) return { ok: false, fallo: await leerFallo(res) };
    const datos = (await res.json()) as { url?: string };
    if (kind === 'preview' && !datos.url) {
      return { ok: false, fallo: { status: 502, code: 'bad_gateway' } };
    }
    return { ok: true, url: datos.url };
  } catch {
    return { ok: false, fallo: { status: 0, code: '' } };
  }
}

/** Mensaje del catálogo para un rechazo al publicar o crear una vista previa. */
export function mensajeDePublicacion(fallo: FalloApi): ClaveMensaje {
  switch (fallo.code) {
    case 'legal_data_missing':
      return 'publish.err_legal';
    case 'demo_content':
      return 'publish.err_demo';
    case 'ir_invalid':
      return 'publish.err_ir';
    case 'too_many_previews':
      return 'publish.err_previews';
    case 'publication_too_large':
      return 'publish.err_too_large';
  }
  switch (fallo.status) {
    case 403:
      return 'publish.err_forbidden';
    case 409:
      return 'publish.err_conflict';
    case 413:
      return 'publish.err_too_large';
    case 422:
      return 'publish.err_invalid';
    default:
      return 'publish.err_generic';
  }
}

/** Mensaje del catálogo para un rechazo al añadir un dominio. */
export function mensajeDeDominio(fallo: FalloApi): ClaveMensaje {
  switch (fallo.code) {
    case 'domain_invalid':
      return 'domains.err_invalid';
    case 'domain_reserved':
      return 'domains.err_reserved';
    case 'domain_taken':
      return 'domains.err_taken';
    case 'domain_limit':
      return 'domains.err_limit';
    case 'site_suspended':
      return 'domains.err_suspended';
    default:
      return fallo.status === 403 ? 'domains.err_forbidden' : 'domains.err_generic';
  }
}

/** Mensaje del catálogo para un rechazo al guardar los ajustes generales del sitio. */
export function mensajeDeAjustes(fallo: FalloApi): ClaveMensaje {
  switch (fallo.code) {
    case 'slug_conflict':
      return 'settings.err_slug_taken';
    case 'invalid_input':
    case 'domain_invalid':
    case 'domain_reserved':
      return 'settings.err_invalid';
    case 'domain_taken':
      return 'domains.err_taken';
    default:
      return 'settings.err_generic';
  }
}

/** Un valor por línea, sin espacios y sin líneas vacías (el formato de los orígenes y los esquemas). */
export function lineas(texto: string): string[] {
  return texto
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
}

/** Posición (desde 1) de la primera dirección que no es https:// válida, o 0 si todas lo son. */
export function primerOrigenInvalido(valores: readonly string[]): number {
  const i = valores.findIndex((v) => {
    try {
      return new URL(v).protocol !== 'https:';
    } catch {
      return true;
    }
  });
  return i + 1;
}

/** Posición (desde 1) del primer esquema que no es válido (RFC 3986), o 0 si todos lo son. */
export function primerEsquemaInvalido(valores: readonly string[]): number {
  return valores.findIndex((v) => !/^[a-z][a-z0-9+.-]*$/i.test(v)) + 1;
}
