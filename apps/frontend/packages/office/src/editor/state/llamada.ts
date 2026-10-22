/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Llamar desde el documento»: lógica PURA de la respuesta de /api/llamada (sin DOM). El
 * transporte y el estado viven en layout/LlamadaDocumento.tsx.
 */

/** Lo que dice el BFF de la llamada de un documento. */
export interface EstadoLlamada {
  /** Rooms está desplegado y contesta; si no, la cabecera no pinta la función. */
  available: boolean;
  /** Hay una sala viva ligada al documento. */
  active: boolean;
  /** Quién está dentro ahora: su sub y, si Rooms lo sabe, su nombre. */
  participants: { sub: string; name?: string }[];
}

export const SIN_LLAMADA: EstadoLlamada = { available: false, active: false, participants: [] };

/** Valida la respuesta (JSON opaco); cualquier forma rara es «no disponible». */
export function parseLlamada(raw: unknown): EstadoLlamada {
  if (typeof raw !== 'object' || raw === null) return SIN_LLAMADA;
  const o = raw as Record<string, unknown>;
  if (o.available !== true) return SIN_LLAMADA;
  const participants: EstadoLlamada['participants'] = [];
  if (Array.isArray(o.participants)) {
    for (const p of o.participants.slice(0, 100)) {
      if (typeof p !== 'object' || p === null) continue;
      const { sub, name } = p as Record<string, unknown>;
      if (typeof sub !== 'string' || sub === '') continue;
      participants.push({
        sub,
        ...(typeof name === 'string' && name.trim() ? { name: name.trim() } : {}),
      });
    }
  }
  return { available: true, active: o.active === true, participants };
}

/** ¿Está esta persona dentro de la llamada? */
export const enLlamada = (e: EstadoLlamada, sub: string): boolean =>
  e.participants.some((p) => p.sub === sub);

/** De quién se conoce el nombre: el de la llamada o el que trae su cursor. */
export const nombreDe = (
  e: EstadoLlamada,
  sub: string,
  conocidos: ReadonlyMap<string, string>,
): string | undefined => e.participants.find((p) => p.sub === sub)?.name ?? conocidos.get(sub);
