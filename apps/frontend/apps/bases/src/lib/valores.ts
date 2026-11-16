/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Valores de celda: de lo que viaja (JSON) a lo que se escribe en la rejilla y de vuelta.
 * Es COMODIDAD de la interfaz: el servidor (`services/bases/internal/domain/valores.go`)
 * vuelve a validar todo y es el que manda.
 */
import type { BasesField, BasesRecord, BasesRole, BasesValue } from '@sytue/api-clients/bases';

/** Si el rol permite escribir registros (editor o propietario). Solo pinta la interfaz. */
export function puedeEscribir(rol: BasesRole | '' | undefined): boolean {
  return rol === 'owner' || rol === 'editor';
}

/** Si el rol permite comentar (comentarista o superior). Solo pinta la interfaz. */
export function puedeComentar(rol: BasesRole | '' | undefined): boolean {
  return rol === 'commenter' || puedeEscribir(rol);
}

/** Si el rol permite cambiar el esquema (tablas y campos): solo el propietario. */
export function puedeEsquema(rol: BasesRole | '' | undefined): boolean {
  return rol === 'owner';
}

/** El valor de una celda como texto para mostrar o para editar. */
export function textoDeValor(_campo: BasesField, valor: BasesValue | undefined): string {
  if (valor === undefined || valor === null) return '';
  if (Array.isArray(valor)) return valor.join(', ');
  if (typeof valor === 'boolean') return valor ? '✓' : '';
  return String(valor);
}

/**
 * Lo escrito en un campo de texto → el valor a enviar. `null` = vaciar. `undefined` = no
 * es válido para ese tipo (la interfaz avisa y no envía).
 */
export function valorDeTexto(campo: BasesField, texto: string): BasesValue | undefined {
  const t = texto.trim();
  if (t === '') return null;
  switch (campo.kind) {
    case 'text':
      return texto;
    case 'number': {
      // Admite la coma decimal, que es como se escribe en gran parte de la UE.
      const n = Number(t.replace(',', '.'));
      return Number.isFinite(n) && Math.abs(n) <= 1e15 ? n : undefined;
    }
    case 'date':
      return /^\d{4}-\d{2}-\d{2}$/.test(t) && !Number.isNaN(Date.parse(t)) ? t : undefined;
    case 'select':
      return campo.choices?.includes(t) ? t : undefined;
    case 'currency': {
      const n = Number(t.replace(',', '.'));
      return Number.isFinite(n) && Math.abs(n) <= 1e15 ? Math.round(n * 100) / 100 : undefined;
    }
    case 'rating': {
      const n = Number(t);
      if (!Number.isInteger(n) || n < 0 || n > MAX_VALORACION) return undefined;
      return n === 0 ? null : n;
    }
    case 'url':
      return normalizarURL(t);
    case 'email':
      return normalizarCorreo(t);
    case 'phone':
      return normalizarTelefono(t);
    default:
      return undefined;
  }
}

/** El tope de un campo de valoración (`MaxValoracion` en el servicio). */
export const MAX_VALORACION = 5;

/**
 * Las monedas que se ofrecen al crear un campo de moneda: el euro primero y las demás
 * de Europa, más las de fuera que más se usan en una factura europea. El servicio admite
 * cualquier código ISO 4217; esto es solo la lista del desplegable.
 */
export const MONEDAS = [
  'EUR',
  'USD',
  'GBP',
  'CHF',
  'SEK',
  'DKK',
  'NOK',
  'ISK',
  'PLN',
  'CZK',
  'HUF',
  'RON',
  'BGN',
  'JPY',
] as const;

// Las tres normalizaciones repiten las de `services/bases/internal/domain/formatos.go`
// para avisar antes de enviar; el servicio vuelve a hacerlas y es el que manda.

function normalizarURL(t: string): string | undefined {
  if (t.length > 2048 || /[\s\\]/.test(t)) return undefined;
  let s = t;
  if (!s.includes('://')) {
    const host = s.split('/')[0] ?? '';
    if (host.includes(':') && !s.includes('.')) return undefined;
    s = `https://${s}`;
  }
  try {
    const u = new URL(s);
    if ((u.protocol !== 'http:' && u.protocol !== 'https:') || !u.hostname) return undefined;
    if (u.username || u.password || u.hostname.includes('..')) return undefined;
    return u.href.endsWith('/') && !s.endsWith('/') ? u.href.slice(0, -1) : u.href;
  } catch {
    return undefined;
  }
}

function normalizarCorreo(t: string): string | undefined {
  if (t.length > 254 || /[\s<>,;"]/.test(t)) return undefined;
  const i = t.lastIndexOf('@');
  if (i <= 0) return undefined;
  const dominio = t.slice(i + 1).toLowerCase();
  if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(dominio)) return undefined;
  return `${t.slice(0, i)}@${dominio}`;
}

function normalizarTelefono(t: string): string | undefined {
  if (t.length > 32 || !/^\+?[0-9 ()./-]+$/.test(t)) return undefined;
  const digitos = t.replace(/\D/g, '').length;
  return digitos >= 3 && digitos <= 20 ? t.split(/\s+/).join(' ') : undefined;
}

/**
 * El valor como se LEE en la rejilla, el tablero o la ficha: un importe con su moneda y
 * el formato del idioma, y una valoración en estrellas. Para editar, `textoDeValor`.
 */
export function textoVisible(
  campo: BasesField,
  valor: BasesValue | undefined,
  idioma: string | undefined = idiomaDePagina(),
): string {
  if (campo.kind === 'currency' && typeof valor === 'number') {
    try {
      return new Intl.NumberFormat(idioma, {
        style: 'currency',
        currency: campo.currency || 'EUR',
      }).format(valor);
    } catch {
      return `${valor} ${campo.currency ?? ''}`.trim(); // un código que Intl no conoce
    }
  }
  if (campo.kind === 'rating' && typeof valor === 'number') return estrellas(valor);
  return textoDeValor(campo, valor);
}

/** «★★★☆☆»: las llenas y las vacías, para que se lea el tope. */
export function estrellas(n: number): string {
  const llenas = Math.max(0, Math.min(MAX_VALORACION, Math.round(n)));
  return '★'.repeat(llenas) + '☆'.repeat(MAX_VALORACION - llenas);
}

/**
 * Adónde lleva pulsar el valor de una URL, un correo o un teléfono, o null. Solo
 * http(s), mailto: y tel:, construidos aquí: aunque el servicio ya rechaza otros
 * esquemas, un valor guardado antes nunca llega crudo a un `href`.
 */
export function enlaceDeValor(campo: BasesField, valor: BasesValue | undefined): string | null {
  if (typeof valor !== 'string' || valor === '') return null;
  switch (campo.kind) {
    case 'url':
      return /^https?:\/\//i.test(valor) ? valor : null;
    case 'email':
      return `mailto:${encodeURIComponent(valor).replace('%40', '@')}`;
    case 'phone':
      return `tel:${valor.replace(/[^0-9+]/g, '')}`;
    default:
      return null;
  }
}

function idiomaDePagina(): string | undefined {
  return (typeof document !== 'undefined' && document.documentElement.lang) || undefined;
}

/** Cómo se llama un registro en una lista de enlaces: su primer campo con valor. */
export function etiquetaDeRegistro(campos: readonly BasesField[], r: BasesRecord): string {
  for (const c of campos) {
    if (c.kind === 'link' || c.kind === 'checkbox' || c.kind === 'attachment') continue;
    const t = textoDeValor(c, r.data[c.id]);
    if (t) return t;
  }
  return r.id.slice(0, 8);
}

/** Los ids de los enlaces de un registro en un campo, o []. */
export function enlacesDe(r: BasesRecord, campo: BasesField): string[] {
  return r.links[campo.id] ?? [];
}

/** El `type` del editor de un campo: el teclado del móvil y el autocompletado ya saben de ellos. */
export const TIPO_DE_INPUT: Partial<Record<BasesField['kind'], string>> = {
  date: 'date',
  url: 'url',
  email: 'email',
  phone: 'tel',
};

/** El `inputMode` del editor de un campo numérico. */
export const MODO_DE_ENTRADA: Partial<Record<BasesField['kind'], 'decimal' | 'numeric'>> = {
  number: 'decimal',
  currency: 'decimal',
  rating: 'numeric',
};
