/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Buscar, filtrar y ordenar las listas de Sytue Web (webs, respuestas de
 * formularios y pedidos). Puro, sin DOM ni red: lo usan las islas y se prueba en Bun.
 * Toda búsqueda va sin acentos ni mayúsculas (`contiene` de @sytue/i18n), la misma
 * regla que el resto de la suite.
 */
import { contiene, type Locale } from '@sytue/i18n';

// ---- Webs ----

export interface SitioListado {
  name: string;
  slug: string;
  canonical_host?: string | null;
  updated_at: string;
}

export type OrdenSitios = 'recent' | 'name';

/** Filtra por nombre, slug o dirección y ordena (recientes primero, o por nombre). */
export function filtrarYOrdenarSitios<S extends SitioListado>(
  sitios: readonly S[],
  q: string,
  orden: OrdenSitios,
  locale: Locale,
): S[] {
  const texto = q.trim();
  const base = texto
    ? sitios.filter(
        (s) =>
          contiene(s.name, texto) ||
          contiene(s.slug, texto) ||
          contiene(s.canonical_host ?? '', texto),
      )
    : [...sitios];
  if (orden === 'name') {
    const col = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
    return base.sort((a, b) => col.compare(a.name, b.name));
  }
  return base.sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at));
}

// ---- Respuestas de formularios ----

/** Texto de un valor de respuesta para buscar y para la tabla («sí/no» lo pone la pantalla). */
function textoDe(v: unknown): string {
  if (v === undefined || v === null) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}

/** Busca en todos los valores de la respuesta (nombre, correo, mensaje…). */
export function filtrarEnvios<E extends { data: Record<string, unknown> }>(
  envios: readonly E[],
  q: string,
): E[] {
  const texto = q.trim();
  if (!texto) return [...envios];
  return envios.filter((e) => Object.values(e.data).some((v) => contiene(textoDe(v), texto)));
}

// ---- Pedidos ----

export interface PedidoListado {
  number: string;
  test: boolean;
  status: string;
  buyer_email: string;
  buyer_name: string;
}

export interface FiltroPedidos {
  q: string;
  /** Estado exacto, o '' para todos. */
  estado: string;
  /** Esconder los pedidos de prueba: no son ventas. */
  sinPruebas: boolean;
}

/** Filtra por número, comprador o correo, por estado y, si se pide, sin pruebas. */
export function filtrarPedidos<P extends PedidoListado>(
  pedidos: readonly P[],
  f: FiltroPedidos,
): P[] {
  const texto = f.q.trim();
  return pedidos.filter((p) => {
    if (f.sinPruebas && p.test) return false;
    if (f.estado && p.status !== f.estado) return false;
    if (!texto) return true;
    return (
      contiene(p.number, texto) || contiene(p.buyer_name, texto) || contiene(p.buyer_email, texto)
    );
  });
}

/** Correo plausible para la lista de avisos (la validación que manda es la del servicio). */
export function correoPlausible(correo: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(correo.trim());
}

// ---- Catálogo ----

/** Hasta aquí (y por encima de cero) un producto tiene «pocas existencias». */
export const UMBRAL_STOCK_BAJO = 5;

export interface ProductoListado {
  name: Record<string, string>;
  category: string;
  active: boolean;
  variants?: Array<{ sku: string; stock: number | null }>;
}

/**
 * Existencias de un producto sumando TODAS sus variantes. La tabla enseñaba solo las
 * de la primera, así que una camiseta con la talla M agotada y la L sobrada salía
 * como agotada o como sobrada según el orden de las tallas. Con una variante sin
 * límite, el producto no tiene límite (null).
 */
export function existencias(p: ProductoListado): {
  total: number | null;
  agotado: boolean;
  bajo: boolean;
} {
  const vs = p.variants ?? [];
  if (vs.length === 0 || vs.some((v) => v.stock === null || v.stock === undefined)) {
    return { total: null, agotado: false, bajo: false };
  }
  const total = vs.reduce((s, v) => s + Math.max(0, v.stock ?? 0), 0);
  return { total, agotado: total === 0, bajo: total > 0 && total <= UMBRAL_STOCK_BAJO };
}

export type FiltroCatalogo = '' | 'agotados' | 'bajos' | 'inactivos';

/** Busca por nombre (en cualquier idioma), categoría o SKU y filtra por existencias. */
export function filtrarProductos<P extends ProductoListado>(
  productos: readonly P[],
  q: string,
  filtro: FiltroCatalogo,
): P[] {
  const texto = q.trim();
  return productos.filter((p) => {
    const e = existencias(p);
    if (filtro === 'agotados' && !e.agotado) return false;
    if (filtro === 'bajos' && !e.bajo) return false;
    if (filtro === 'inactivos' && p.active) return false;
    if (!texto) return true;
    return (
      Object.values(p.name).some((n) => contiene(n, texto)) ||
      contiene(p.category, texto) ||
      (p.variants ?? []).some((v) => contiene(v.sku, texto))
    );
  });
}
