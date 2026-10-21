/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica pura de la galería de plantillas: categorías, buscador y agrupación. El catálogo pasa de
 * setenta plantillas, así que se encuentra por el tipo de negocio («pelu», «barbero», «fontanero»),
 * sin tildes, en singular o plural y en español o inglés.
 */

import { plegar } from '@sytue/i18n';

/** Grupos de la galería, en el orden en que se enseñan. La misma lista que valida `catalogo_test.go`. */
export const CATEGORIAS = [
  'belleza',
  'comida',
  'comercio',
  'oficios',
  'motor',
  'profesionales',
  'salud',
  'deporte',
  'educacion',
  'creativos',
  'turismo',
  'local',
  'comunidad',
  'web',
] as const;
export type Categoria = (typeof CATEGORIAS)[number];

export interface PlantillaInfo {
  key: string;
  name: Record<string, string>;
  description: Record<string, string>;
  vertical: string;
  category?: string;
  keywords?: Record<string, string>;
  store: boolean;
}

export const nombreDe = (p: PlantillaInfo, locale: string): string =>
  p.name[locale] || p.name.es || p.key;
export const descripcionDe = (p: PlantillaInfo, locale: string): string =>
  p.description[locale] || p.description.es || '';

const palabras = (s: string): string[] =>
  plegar(s)
    .split(/[^a-z0-9ñ]+/)
    .filter(Boolean);

/** Variantes de una palabra escrita: tal cual y sin el plural («peluquerias» → «peluqueria»). */
const variantes = (t: string): string[] => {
  const v = new Set([t]);
  if (t.length > 4 && t.endsWith('es')) v.add(t.slice(0, -2));
  if (t.length > 3 && t.endsWith('s')) v.add(t.slice(0, -1));
  return [...v];
};

/**
 * Puntuación de una plantilla para una consulta (0 = no coincide). Cada palabra escrita tiene que
 * aparecer como inicio de alguna palabra del nombre (vale triple, cuatro si es la palabra entera), de
 * las palabras clave o de la categoría; así «pelu» encuentra «peluquería» y «barbero» encuentra la barbería.
 */
export function puntuar(p: PlantillaInfo, consulta: string, etiquetaCategoria: string): number {
  const tokens = palabras(consulta);
  if (tokens.length === 0) return 1;
  const enNombre = palabras(`${p.name.es ?? ''} ${p.name.en ?? ''} ${p.key.replaceAll('-', ' ')}`);
  const enClaves = palabras(
    `${p.keywords?.es ?? ''} ${p.keywords?.en ?? ''} ${p.vertical} ${etiquetaCategoria}`,
  );
  let total = 0;
  for (const t of tokens) {
    const vs = variantes(t);
    if (enNombre.some((w) => vs.some((v) => w.startsWith(v)))) {
      total += enNombre.some((w) => vs.includes(w)) ? 4 : 3; // la palabra exacta gana a un inicio de palabra
    } else if (enClaves.some((w) => vs.some((v) => w.startsWith(v)))) total += 1;
    else return 0;
  }
  return total;
}

export interface Grupo {
  categoria: string;
  plantillas: PlantillaInfo[];
}

/** Plantillas que cumplen la consulta y la categoría, mejores primero y luego por nombre. */
export function buscar(
  lista: PlantillaInfo[],
  consulta: string,
  categoria: string | null,
  locale: string,
  etiquetaCategoria: (c: string) => string,
): PlantillaInfo[] {
  return lista
    .filter((p) => categoria === null || (p.category || 'otras') === categoria)
    .map((p) => ({ p, s: puntuar(p, consulta, etiquetaCategoria(p.category || 'otras')) }))
    .filter((x) => x.s > 0)
    .sort((a, b) => b.s - a.s || nombreDe(a.p, locale).localeCompare(nombreDe(b.p, locale), locale))
    .map((x) => x.p);
}

/** Agrupa por categoría en el orden del catálogo; las de categoría desconocida van al final. */
export function agrupar(lista: PlantillaInfo[], locale: string): Grupo[] {
  const por = new Map<string, PlantillaInfo[]>();
  for (const p of lista) {
    const c = p.category || 'otras';
    por.set(c, [...(por.get(c) ?? []), p]);
  }
  const orden = [
    ...CATEGORIAS,
    ...[...por.keys()].filter((c) => !(CATEGORIAS as readonly string[]).includes(c)),
  ];
  return orden
    .filter((c) => por.has(c))
    .map((c) => ({
      categoria: c,
      plantillas: (por.get(c) ?? []).sort((a, b) =>
        nombreDe(a, locale).localeCompare(nombreDe(b, locale), locale),
      ),
    }));
}
