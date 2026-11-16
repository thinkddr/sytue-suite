/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Portapapeles de cajas de Diapos: la parte PURA (sin DOM) de copiar, cortar, pegar y
 * duplicar. Lo que se copia es la selección efectiva (caja, multiselección o grupo) con
 * sus conectores internos, como JSON propio en `text/plain` precedido de un prefijo con
 * versión: así viaja por el portapapeles del sistema entre diapositivas, entre dos
 * presentaciones abiertas y, más adelante, a la app nativa (que lee el mismo formato).
 *
 * Pegar nunca reutiliza un id: cajas, grupos y conectores reciben ids nuevos y las anclas
 * de los conectores se reasignan a las cajas pegadas. Si las originales siguen en la
 * diapositiva de destino, la copia se desplaza (+2 % por pegado) para que se vea.
 */
import type { SlideBox, SlideConnector } from '../types';
import { boxAnims, normalizeAnimOrders, withAnims } from './anim';
import { remapConnectorIds, sanitizeConnectors } from './connector';

/** Prefijo del texto: identifica el formato y su versión (DIAP-04 lee el mismo). */
export const PREFIJO_RECORTE = 'sytue-diapos-cajas/1:';

/** Desplazamiento (en % del lienzo) de cada pegado sobre las originales. */
export const PASO_PEGADO = 2;

/** Lo que se lleva el portapapeles: cajas en orden de apilado y sus conectores. */
export interface RecorteDiapos {
  boxes: SlideBox[];
  connectors: SlideConnector[];
}

/**
 * Recorta de una diapositiva las cajas `ids` (en su orden de apilado) y los conectores
 * internos: los que tienen al menos un extremo anclado a una caja copiada y ninguno
 * anclado a una caja que se queda fuera (un conector a medias no tendría a qué unirse).
 */
export function recortar(
  boxes: readonly SlideBox[],
  connectors: readonly SlideConnector[],
  ids: readonly string[],
): RecorteDiapos {
  const sel = new Set(ids);
  const elegidas = boxes.filter((b) => sel.has(b.id));
  const dentro = (id: string | undefined): boolean => id === undefined || sel.has(id);
  const anclado = (c: SlideConnector): boolean =>
    c.from.boxId !== undefined || c.to.boxId !== undefined;
  return {
    boxes: elegidas,
    connectors: connectors.filter((c) => anclado(c) && dentro(c.from.boxId) && dentro(c.to.boxId)),
  };
}

/** Texto para `text/plain`: prefijo + JSON. */
export const serializarRecorte = (r: RecorteDiapos): string => PREFIJO_RECORTE + JSON.stringify(r);

const num = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

/**
 * Lee un texto del portapapeles. Devuelve null si no es un recorte de Diapos (cualquier
 * otro texto, o uno corrupto): el portapapeles es una frontera de confianza, así que solo
 * pasan cajas con forma de caja; el HTML se sanea igualmente al pintar.
 */
export function leerRecorte(texto: string): RecorteDiapos | null {
  if (!texto.startsWith(PREFIJO_RECORTE)) return null;
  let raw: unknown;
  try {
    raw = JSON.parse(texto.slice(PREFIJO_RECORTE.length));
  } catch {
    return null;
  }
  if (!raw || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  if (!Array.isArray(o['boxes'])) return null;
  const boxes = o['boxes'].filter((b): b is SlideBox => {
    if (!b || typeof b !== 'object') return false;
    const r = b as Record<string, unknown>;
    return (
      typeof r['id'] === 'string' &&
      r['id'] !== '' &&
      num(r['x']) &&
      num(r['y']) &&
      num(r['w']) &&
      num(r['h']) &&
      typeof r['html'] === 'string'
    );
  });
  if (boxes.length === 0) return null;
  const ids = new Set(boxes.map((b) => b.id));
  return { boxes, connectors: sanitizeConnectors(o['connectors'], ids) };
}

/** Opciones de `pegarRecorte`. */
export interface OpcionesPegado {
  /** Generador de ids (crypto.randomUUID en la app; determinista en las pruebas). */
  nuevoId: () => string;
  /** ¿Se admiten conectores en el destino? (El patrón no los tiene.) */
  conConectores: boolean;
}

/** Resultado de pegar: las cajas y conectores del destino ya fusionados. */
export interface Pegado {
  boxes: SlideBox[];
  connectors: SlideConnector[];
  /** Ids nuevos de las cajas pegadas (para seleccionarlas). */
  ids: string[];
}

const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));

/**
 * Cuánto desplazar la copia. Si su sitio está libre en el destino (otra diapositiva, otra
 * presentación o tras cortar) se pega en su sitio; si ya lo ocupa una caja (las originales
 * o un pegado anterior), +2 % por cada copia que ya esté ahí, para que pegar varias veces
 * escalone en vez de apilar, también al pegar repetidamente en otra diapositiva.
 */
export function desplazamientoPegado(destino: readonly SlideBox[], recorte: RecorteDiapos): number {
  const ref = recorte.boxes[0];
  if (!ref) return 0;
  const ocupado = (d: number): boolean =>
    destino.some((b) => Math.abs(b.x - (ref.x + d)) < 0.01 && Math.abs(b.y - (ref.y + d)) < 0.01);
  let d = 0;
  // Tope: más allá de 20 pasos la copia ya se habría salido del lienzo.
  for (let i = 0; i <= 20 && ocupado(d); i++) d += PASO_PEGADO;
  return d;
}

/**
 * Pega el recorte sobre las cajas y conectores del destino: ids nuevos de caja, grupo y
 * conector; anclas reasignadas; desplazamiento según `desplazamientoPegado` (sin salirse
 * del lienzo), y las animaciones de lo pegado detrás de las del destino, renumeradas.
 */
export function pegarRecorte(
  destBoxes: readonly SlideBox[],
  destConnectors: readonly SlideConnector[],
  recorte: RecorteDiapos,
  op: OpcionesPegado,
): Pegado {
  const d = desplazamientoPegado(destBoxes, recorte);
  const idMap = new Map<string, string>();
  const grupos = new Map<string, string>();
  // Las animaciones pegadas van tras las existentes, conservando su orden relativo.
  const base = destBoxes.reduce(
    (m, b) => boxAnims(b).reduce((mm, a) => Math.max(mm, a.order), m),
    0,
  );
  const nuevas = recorte.boxes.map((b) => {
    const id = op.nuevoId();
    idMap.set(b.id, id);
    const w = clamp(b.w, 0, 100);
    const h = clamp(b.h, 0, 100);
    const copia: SlideBox = {
      ...b,
      id,
      x: clamp(b.x + d, 0, 100 - w),
      y: clamp(b.y + d, 0, 100 - h),
    };
    if (b.groupId !== undefined) {
      if (!grupos.has(b.groupId)) grupos.set(b.groupId, op.nuevoId());
      copia.groupId = grupos.get(b.groupId)!;
    }
    const anims = boxAnims(b);
    return anims.length > 0
      ? withAnims(
          copia,
          anims.map((a) => ({ ...a, order: base + a.order })),
        )
      : copia;
  });
  // Un grupo de una sola caja no es un grupo: se suelta (como al desagrupar).
  const cuenta = new Map<string, number>();
  for (const b of nuevas) if (b.groupId) cuenta.set(b.groupId, (cuenta.get(b.groupId) ?? 0) + 1);
  const sueltas = nuevas.map((b) => {
    if (b.groupId && cuenta.get(b.groupId) === 1) {
      const { groupId: _g, ...resto } = b;
      return resto;
    }
    return b;
  });
  const conectores = op.conConectores
    ? remapConnectorIds(recorte.connectors, idMap).map((c) => ({
        ...c,
        id: op.nuevoId(),
        from: c.from.boxId === undefined ? moverLibre(c.from, d) : c.from,
        to: c.to.boxId === undefined ? moverLibre(c.to, d) : c.to,
      }))
    : [];
  return {
    boxes: normalizeAnimOrders([...destBoxes, ...sueltas]),
    connectors: [...destConnectors, ...conectores],
    ids: sueltas.map((b) => b.id),
  };
}

const moverLibre = (e: SlideConnector['from'], d: number): SlideConnector['from'] => ({
  ...e,
  x: clamp((e.x ?? 0) + d, 0, 100),
  y: clamp((e.y ?? 0) + d, 0, 100),
});
