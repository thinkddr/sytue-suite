/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Serialización de la escena: JSON ESTABLE (mismas claves, mismo orden, números
 * a 3 decimales: `serializar(leer(t)) === t` para todo `t` ya canónico) y
 * VERSIONADO (`v`), con una tabla de migraciones de la versión n a la n+1.
 *
 * `leer` es la frontera de confianza: el texto llega de un documento que puede
 * haber editado cualquiera, así que todo se sanea —tipos desconocidos fuera,
 * números finitos y acotados, colores `#RRGGBB`, ids únicos y bien formados,
 * imágenes solo `data:image/…`, http(s) o rutas absolutas, tope de nodos,
 * profundidad y puntos— y lo que no se puede arreglar se descarta, no se pinta.
 * Un `v` MAYOR que el conocido no se interpreta (se perdería lo que no
 * entendemos al guardar): devuelve `version-futura` para que quien llama muestre
 * el SVG de respaldo en solo lectura.
 */
import { ESCENA_VERSION } from './tipos.js';
import type { Capa, Escena, Estilo, ExtremoConector, Nodo, TipoForma } from './tipos.js';
import type { ConnectorAnchor, ConnectorKind } from './conector.js';

export const LIMITES = {
  nodos: 5000,
  profundidad: 8,
  capas: 64,
  /** Puntos (pares x, y) de un trazo a mano alzada. */
  puntos: 20000,
  texto: 10000,
  dimension: 100000,
  coordenada: 1000000,
  /** Largo máximo de un `data:` de imagen. */
  imagen: 4000000,
} as const;

export type Migracion = (raw: Record<string, unknown>) => Record<string, unknown>;

/** Migraciones: la clave `n` convierte un documento de la versión `n` en uno de la `n + 1`.
 *  Hoy no hay ninguna (la versión 1 es la primera). NUNCA se reescribe una ya publicada. */
export const MIGRACIONES: Readonly<Record<number, Migracion>> = {};

export type Lectura =
  { ok: true; escena: Escena } | { ok: false; motivo: 'json' | 'forma' | 'version-futura' };

const FORMAS: ReadonlySet<string> = new Set<TipoForma>([
  'rect',
  'ellipse',
  'triangle',
  'diamond',
  'pentagon',
  'hexagon',
  'star',
  'cylinder',
  'callout',
]);
const TRAZADOS: ReadonlySet<string> = new Set<ConnectorKind>(['straight', 'elbow', 'curved']);
const LADOS: ReadonlySet<string> = new Set<ConnectorAnchor>(['top', 'right', 'bottom', 'left']);
const ALINEAR: ReadonlySet<string> = new Set(['start', 'middle', 'end']);
const HEX = /^#[0-9a-fA-F]{6}$/;
const ID = /^[A-Za-z0-9_-]{1,64}$/;
const IMG_DATA = /^data:image\/(png|jpeg|webp|gif);base64,[A-Za-z0-9+/=]+$/;
const IMG_URL = /^(https?:\/\/[^\s"'<>]+|\/(?!\/)[^\s"'<>]*)$/;

type Obj = Record<string, unknown>;
const esObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);

const r3 = (n: number): number => {
  const r = Math.round(n * 1000) / 1000;
  return Object.is(r, -0) ? 0 : r;
};
const num = (v: unknown, def: number, lo: number, hi: number): number =>
  typeof v === 'number' && Number.isFinite(v) ? r3(Math.min(hi, Math.max(lo, v))) : def;
const coord = (v: unknown, def = 0): number => num(v, def, -LIMITES.coordenada, LIMITES.coordenada);
const texto = (v: unknown, max: number): string | undefined =>
  typeof v === 'string' && v !== '' ? v.slice(0, max) : undefined;
const color = (v: unknown): string | undefined =>
  typeof v === 'string' && HEX.test(v) ? v.toLowerCase() : undefined;

/** Asigna una propiedad solo si hay valor (exactOptionalPropertyTypes + JSON sin `undefined`). */
function poner<T extends object, K extends string, V>(o: T, k: K, v: V | undefined): void {
  if (v !== undefined) (o as Record<string, unknown>)[k] = v;
}

function estilo(raw: unknown): Estilo | undefined {
  if (!esObj(raw)) return undefined;
  const e: Estilo = {};
  poner(e, 'relleno', color(raw['relleno']));
  poner(e, 'trazo', color(raw['trazo']));
  if (typeof raw['grosor'] === 'number') poner(e, 'grosor', num(raw['grosor'], 0, 0, 64));
  if (typeof raw['opacidad'] === 'number' && raw['opacidad'] !== 1) {
    poner(e, 'opacidad', num(raw['opacidad'], 1, 0, 1));
  }
  if (raw['discontinuo'] === true) e.discontinuo = true;
  return Object.keys(e).length > 0 ? e : undefined;
}

interface Ctx {
  ids: Set<string>;
  nodos: number;
  siguiente: number;
}

function idUnico(raw: unknown, ctx: Ctx): string {
  if (typeof raw === 'string' && ID.test(raw) && !ctx.ids.has(raw)) {
    ctx.ids.add(raw);
    return raw;
  }
  let id: string;
  do id = `x${ctx.siguiente++}`;
  while (ctx.ids.has(id));
  ctx.ids.add(id);
  return id;
}

function extremo(raw: unknown): ExtremoConector {
  const o = esObj(raw) ? raw : {};
  const nodo = o['nodo'];
  if (typeof nodo === 'string' && ID.test(nodo)) {
    const lado = o['lado'];
    return typeof lado === 'string' && LADOS.has(lado)
      ? { nodo, lado: lado as ConnectorAnchor }
      : { nodo };
  }
  return { x: coord(o['x']), y: coord(o['y']) };
}

function nodo(raw: unknown, ctx: Ctx, prof: number): Nodo | undefined {
  if (!esObj(raw) || ctx.nodos >= LIMITES.nodos) return undefined;
  /** id único + nombre + estilo (se llama solo cuando el nodo se acepta). */
  const base = (): Obj => {
    ctx.nodos++;
    const b: Obj = { id: idUnico(raw['id'], ctx) };
    poner(b, 'nombre', texto(raw['nombre'], 200));
    poner(b, 'estilo', estilo(raw['estilo']));
    return b;
  };
  const caja = (): Obj => {
    const giro = ((num(raw['rot'], 0, -360000, 360000) % 360) + 360) % 360;
    const c: Obj = {
      x: coord(raw['x']),
      y: coord(raw['y']),
      w: num(raw['w'], 1, 1, LIMITES.dimension),
      h: num(raw['h'], 1, 1, LIMITES.dimension),
    };
    if (giro !== 0) c['rot'] = r3(giro);
    return c;
  };
  const tam = (o: Obj): void => {
    if (typeof raw['tamTexto'] === 'number') o['tamTexto'] = num(raw['tamTexto'], 14, 1, 1000);
  };

  switch (raw['tipo']) {
    case 'forma': {
      const f = raw['forma'];
      const o: Obj = {
        ...base(),
        tipo: 'forma',
        forma: typeof f === 'string' && FORMAS.has(f) ? f : 'rect',
        ...caja(),
      };
      poner(o, 'texto', texto(raw['texto'], LIMITES.texto));
      tam(o);
      return ordenar(o);
    }
    case 'texto': {
      const o: Obj = {
        ...base(),
        tipo: 'texto',
        texto: (typeof raw['texto'] === 'string' ? raw['texto'] : '').slice(0, LIMITES.texto),
        ...caja(),
      };
      tam(o);
      const al = raw['alinear'];
      if (typeof al === 'string' && ALINEAR.has(al) && al !== 'start') o['alinear'] = al;
      if (raw['negrita'] === true) o['negrita'] = true;
      return ordenar(o);
    }
    case 'imagen': {
      const href = raw['href'];
      const valido =
        typeof href === 'string' &&
        ((href.length <= LIMITES.imagen && IMG_DATA.test(href)) ||
          (href.length <= 2048 && IMG_URL.test(href)));
      if (!valido) return undefined;
      const alt = typeof raw['alt'] === 'string' ? raw['alt'].slice(0, 500) : '';
      return ordenar({ ...base(), tipo: 'imagen', href, alt, ...caja() });
    }
    case 'grupo': {
      if (prof >= LIMITES.profundidad) return undefined;
      const o: Obj = { ...base(), tipo: 'grupo', ...caja() };
      const hijos: Nodo[] = [];
      for (const h of Array.isArray(raw['hijos']) ? raw['hijos'] : []) {
        const n = nodo(h, ctx, prof + 1);
        if (n) hijos.push(n);
      }
      o['hijos'] = hijos;
      return ordenar(o);
    }
    case 'linea': {
      const o: Obj = {
        ...base(),
        tipo: 'linea',
        x1: coord(raw['x1']),
        y1: coord(raw['y1']),
        x2: coord(raw['x2']),
        y2: coord(raw['y2']),
      };
      if (raw['flechaIni'] === true) o['flechaIni'] = true;
      if (raw['flechaFin'] === true) o['flechaFin'] = true;
      return ordenar(o);
    }
    case 'conector': {
      // Solo en el primer nivel: sus coordenadas son las del lienzo.
      if (prof > 0) return undefined;
      const t = raw['trazado'];
      const o: Obj = {
        ...base(),
        tipo: 'conector',
        trazado: typeof t === 'string' && TRAZADOS.has(t) ? t : 'straight',
        desde: extremo(raw['desde']),
        hasta: extremo(raw['hasta']),
      };
      if (raw['flechaIni'] === true) o['flechaIni'] = true;
      if (raw['flechaFin'] === true) o['flechaFin'] = true;
      return ordenar(o);
    }
    case 'trazo': {
      const pts = Array.isArray(raw['puntos']) ? raw['puntos'] : [];
      const puntos: number[] = [];
      for (let i = 0; i + 1 < pts.length && puntos.length < LIMITES.puntos * 2; i += 2) {
        const x: unknown = pts[i];
        const y: unknown = pts[i + 1];
        if (
          typeof x === 'number' &&
          typeof y === 'number' &&
          Number.isFinite(x) &&
          Number.isFinite(y)
        ) {
          puntos.push(coord(x), coord(y));
        }
      }
      if (puntos.length === 0) return undefined;
      return ordenar({ ...base(), tipo: 'trazo', puntos });
    }
    default:
      return undefined;
  }
}

/** Orden canónico de las claves (JSON estable): identificación, tipo, geometría, resto. */
const ORDEN = [
  'id',
  'tipo',
  'nombre',
  'forma',
  'x',
  'y',
  'w',
  'h',
  'rot',
  'x1',
  'y1',
  'x2',
  'y2',
  'trazado',
  'desde',
  'hasta',
  'puntos',
  'href',
  'alt',
  'texto',
  'tamTexto',
  'alinear',
  'negrita',
  'flechaIni',
  'flechaFin',
  'estilo',
  'hijos',
];
function ordenar(src: Obj): Nodo {
  const out: Obj = {};
  for (const k of ORDEN) if (src[k] !== undefined) out[k] = src[k];
  return out as unknown as Nodo;
}

/** Sustituye por punto libre los extremos anclados a nodos que no existen. */
function repararConectores(nodos: Nodo[], ids: ReadonlySet<string>): Nodo[] {
  return nodos.map((n) => {
    if (n.tipo !== 'conector') return n;
    const fix = (e: ExtremoConector): ExtremoConector =>
      'nodo' in e && !ids.has(e.nodo) ? { x: 0, y: 0 } : e;
    return ordenar({ ...n, desde: fix(n.desde), hasta: fix(n.hasta) });
  });
}

/** Escena saneada y canónica desde un objeto de la versión actual. */
function canonica(raw: object): Escena {
  const r = raw as Obj;
  const ctx: Ctx = { ids: new Set(), nodos: 0, siguiente: 1 };
  const capasRaw = Array.isArray(r['capas']) ? r['capas'].slice(0, LIMITES.capas) : [];
  const idsCapa = new Set<string>();
  const capas: Capa[] = [];
  capasRaw.forEach((c, i) => {
    if (!esObj(c)) return;
    let id = typeof c['id'] === 'string' && ID.test(c['id']) ? c['id'] : `capa${i + 1}`;
    while (idsCapa.has(id)) id = `${id}_`;
    idsCapa.add(id);
    const nodos: Nodo[] = [];
    for (const n of Array.isArray(c['nodos']) ? c['nodos'] : []) {
      const s = nodo(n, ctx, 0);
      if (s) nodos.push(s);
    }
    capas.push({
      id,
      nombre: texto(c['nombre'], 120) ?? `Capa ${i + 1}`,
      visible: c['visible'] !== false,
      bloqueada: c['bloqueada'] === true,
      nodos,
    });
  });
  if (capas.length === 0) {
    capas.push({ id: 'capa1', nombre: 'Capa 1', visible: true, bloqueada: false, nodos: [] });
  }
  for (const c of capas) c.nodos = repararConectores(c.nodos, ctx.ids);
  const escena: Escena = {
    v: ESCENA_VERSION,
    ancho: num(r['ancho'], 800, 1, LIMITES.dimension),
    alto: num(r['alto'], 600, 1, LIMITES.dimension),
    capas,
  };
  poner(escena, 'titulo', texto(r['titulo'], 200));
  poner(escena, 'descripcion', texto(r['descripcion'], 2000));
  poner(escena, 'fondo', color(r['fondo']));
  return escena;
}

/** Aplica las migraciones n → n+1 hasta la versión actual. */
export function migrar(
  raw: unknown,
  migraciones: Readonly<Record<number, Migracion>> = MIGRACIONES,
  actual: number = ESCENA_VERSION,
): { ok: true; raw: Obj } | { ok: false; motivo: 'forma' | 'version-futura' } {
  if (!esObj(raw)) return { ok: false, motivo: 'forma' };
  let v = raw['v'];
  if (typeof v !== 'number' || !Number.isInteger(v) || v < 1) return { ok: false, motivo: 'forma' };
  if (v > actual) return { ok: false, motivo: 'version-futura' };
  let cur: Obj = raw;
  while (v < actual) {
    const paso = migraciones[v];
    if (!paso) return { ok: false, motivo: 'forma' };
    cur = { ...paso(cur), v: v + 1 };
    v += 1;
  }
  return { ok: true, raw: cur };
}

/** Sanea un valor ya parseado (migrándolo si es de una versión anterior). */
export function sanear(
  raw: unknown,
  migraciones: Readonly<Record<number, Migracion>> = MIGRACIONES,
): Lectura {
  const m = migrar(raw, migraciones);
  return m.ok ? { ok: true, escena: canonica(m.raw) } : m;
}

/** Lee una escena serializada. Nunca lanza. */
export function leer(texto: string): Lectura {
  let raw: unknown;
  try {
    raw = JSON.parse(texto);
  } catch {
    return { ok: false, motivo: 'json' };
  }
  return sanear(raw);
}

/** Texto canónico de la escena (claves en orden fijo, números a 3 decimales). */
export const serializar = (escena: Escena): string => JSON.stringify(canonica(escena));
