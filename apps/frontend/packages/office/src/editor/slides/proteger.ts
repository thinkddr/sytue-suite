/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Protección de Diapos: la parte PURA del cliente (sin DOM). La protección la cumple el
 * servicio Go (`domain/proteccion_diapos.go`); lo de aquí solo evita enviar un cambio que
 * el servidor va a rechazar y deja las diapositivas y cajas bloqueadas tal cual las trajo
 * el servidor. Es la MISMA regla, escrita aparte: si cambia una, cambia la otra.
 *
 *   - `slides`: contenido entero fijo, y la diapositiva no se borra (sí se mueve).
 *   - `boxes`: la caja queda como está y en su diapositiva.
 *   - `master`: patrón, diseños, tema, temas propios y tamaño del lienzo fijos.
 *   - Cambiar `protect` (o quitarlo) es también cambiar la protección.
 */
import type { Slide, SlideBox, SlidesModel, SlidesProtection } from '../types';

/** Lo que cuenta de `protect` al comparar: sin `hasPassword`, que fija el servidor. */
export function sanitizeSlidesProtection(raw: unknown): SlidesProtection | undefined {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return undefined;
  const r = raw as Record<string, unknown>;
  const out: SlidesProtection = {};
  if (r.hasPassword === true) out.hasPassword = true;
  const ids = (v: unknown): string[] =>
    Array.isArray(v)
      ? [...new Set(v.filter((x): x is string => typeof x === 'string' && x !== ''))]
      : [];
  if (ids(r.slides).length > 0) out.slides = ids(r.slides);
  if (ids(r.boxes).length > 0) out.boxes = ids(r.boxes);
  if (r.master === true) out.master = true;
  return out;
}

/** JSON con las claves ordenadas: reordenar claves no es un cambio (como en el servidor). */
export function canonico(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(canonico).join(',')}]`;
  if (typeof v === 'object' && v !== null) {
    const o = v as Record<string, unknown>;
    return `{${Object.keys(o)
      .filter((k) => o[k] !== undefined)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canonico(o[k])}`)
      .join(',')}}`;
  }
  return JSON.stringify(v) ?? 'null';
}

export const diapoBloqueada = (m: SlidesModel, id: string): boolean =>
  m.protect?.slides?.includes(id) === true;
export const cajaBloqueada = (m: SlidesModel, id: string): boolean =>
  m.protect?.boxes?.includes(id) === true;

/** Añade el id a la lista si no está, o lo quita si está (lista vacía = campo ausente). */
export function alternarId(lista: string[] | undefined, id: string): string[] | undefined {
  const cur = lista ?? [];
  const next = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id];
  return next.length > 0 ? next : undefined;
}

/** Lo que `master` fija, comparable. */
const marco = (m: SlidesModel): string =>
  canonico({ master: m.master, theme: m.theme, themes: m.themes, size: m.size });

const sinClave = (p: SlidesProtection | undefined): string => {
  if (!p) return '';
  const { hasPassword: _h, ...resto } = p;
  return canonico(resto);
};

function cajasPorId(m: SlidesModel): Map<string, { slide: string; box: SlideBox }[]> {
  const out = new Map<string, { slide: string; box: SlideBox }[]>();
  for (const s of m.slides)
    for (const b of s.boxes) out.set(b.id, [...(out.get(b.id) ?? []), { slide: s.id, box: b }]);
  return out;
}

/**
 * ¿El paso de `prev` a `next` toca algo que la protección fija? Falso si `prev` no está
 * protegido (proteger es de cualquiera). Replica `contenidoDiapos` y la comparación de
 * `protect` del servidor.
 */
export function cambioProtegido(prev: SlidesModel, next: SlidesModel): boolean {
  const p = prev.protect;
  if (!p) return false;
  if (!next.protect || sinClave(p) !== sinClave(next.protect)) return true;
  const nuevas = new Map<string, Slide[]>();
  for (const s of next.slides) nuevas.set(s.id, [...(nuevas.get(s.id) ?? []), s]);
  for (const id of p.slides ?? []) {
    const antes = prev.slides.find((s) => s.id === id);
    if (!antes) continue; // no hay nada que fijar
    const ahora = nuevas.get(id);
    if (ahora?.length !== 1 || canonico(ahora[0]) !== canonico(antes)) return true;
  }
  const antesCajas = cajasPorId(prev);
  const ahoraCajas = cajasPorId(next);
  for (const id of p.boxes ?? []) {
    const antes = antesCajas.get(id);
    if (antes?.length !== 1) continue;
    const ahora = ahoraCajas.get(id);
    if (ahora?.length !== 1 || ahora[0]!.slide !== antes[0]!.slide) return true;
    if (canonico(ahora[0]!.box) !== canonico(antes[0]!.box)) return true;
  }
  return p.master === true && marco(prev) !== marco(next);
}

/**
 * Para quien NO está autorizado: lo bloqueado se guarda tal cual lo trajo el servidor
 * (`original`, el JSON sin sanear). El editor migra y sanea el modelo al cargarlo; sin esto
 * la primera edición de una persona sin permiso reenviaría, p. ej., una diapositiva
 * bloqueada con otra forma y el servidor la rechazaría aunque nadie la haya tocado.
 */
export function conBloqueadoOriginal(next: SlidesModel, original: SlidesModel | null): SlidesModel {
  const p = original?.protect;
  if (!original || !p) return next;
  let slides = next.slides;
  const cambia = (f: (s: Slide) => Slide): void => {
    slides = slides.map(f);
  };
  for (const id of p.slides ?? []) {
    const o = original.slides.find((s) => s.id === id);
    if (o && next.slides.filter((s) => s.id === id).length === 1)
      cambia((s) => (s.id === id ? o : s));
  }
  for (const id of p.boxes ?? []) {
    const o = original.slides.flatMap((s) => s.boxes).find((b) => b.id === id);
    if (o && slides.flatMap((s) => s.boxes).filter((b) => b.id === id).length === 1)
      cambia((s) => ({ ...s, boxes: s.boxes.map((b) => (b.id === id ? o : b)) }));
  }
  // La propia protección también tal cual: el editor la sanea al cargar (duplicados, tipos).
  if (p.master !== true) return { ...next, slides, protect: p };
  // Lo que `master` fija, tal cual lo trajo el servidor (sin lo que el editor haya saneado).
  const { master: _m, theme: _t, themes: _ts, size: _s, ...resto } = next;
  return {
    ...resto,
    slides,
    protect: p,
    ...(original.master !== undefined ? { master: original.master } : {}),
    ...(original.theme !== undefined ? { theme: original.theme } : {}),
    ...(original.themes !== undefined ? { themes: original.themes } : {}),
    ...(original.size !== undefined ? { size: original.size } : {}),
  };
}
