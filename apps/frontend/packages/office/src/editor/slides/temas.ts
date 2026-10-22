/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica PURA de los temas propios de una presentación: el fondo, la tinta y la fuente que
 * fija quien la hace, guardados en el modelo (`themes`, y `theme` con el id de uno de
 * ellos) y no en un catálogo. Sin Preact ni DOM: se prueba con bun (test/slides-temas.test.ts).
 *
 * Frontera de confianza: el modelo lo escriben otras personas y otras versiones, y la
 * fuente acaba en un atributo de estilo, así que `sanitizeCustomThemes` rehace la lista
 * cada vez que se va a pintar y no deja pasar más que colores `#RRGGBB` y nombres de
 * fuente de verdad. Los mismos topes y reglas que `slides_temas.go` del servicio.
 */
import type { SlideCustomTheme } from '../types';

export const MAX_CUSTOM_THEMES = 12;
export const MAX_THEME_NAME = 60;
/** Por debajo de esta razón de contraste (WCAG para texto grande) el panel avisa. */
export const LOW_CONTRAST = 3;

/** Fuentes que ofrece el panel: las de uso común en una presentación que cualquiera tiene.
 *  Vacía = la de la app. */
export const THEME_FONTS: readonly string[] = [
  'Georgia',
  'Calibri',
  'Arial',
  'Verdana',
  'Trebuchet MS',
  'Times New Roman',
  'Garamond',
  'Courier New',
];

/** `#RRGGBB` en mayúsculas desde `#RGB`, `RGB`, `RRGGBB` o `#RRGGBB`; `null` si no es un color. */
export function normalizeHex(raw: string): string | null {
  const m = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(raw.trim());
  if (!m) return null;
  const h =
    m[1]!.length === 3
      ? m[1]!
          .split('')
          .map((c) => c + c)
          .join('')
      : m[1]!;
  return `#${h.toUpperCase()}`;
}

/** La familia tipográfica si es una de verdad (letras, cifras, espacios, puntos y guiones,
 *  hasta 40): va a un `font-family` y a un nombre de fuente de OOXML/ODF. `''` si no. */
export function sanitizeThemeFont(raw: unknown): string {
  if (typeof raw !== 'string') return '';
  const f = raw.trim();
  return /^[\p{L}\p{N} .-]{1,40}$/u.test(f) ? f : '';
}

/** Rehace la lista de temas propios: ids `custom-…` únicos, colores válidos, nombre acotado
 *  y fuente sana; hasta el tope. */
export function sanitizeCustomThemes(raw: unknown): SlideCustomTheme[] {
  if (!Array.isArray(raw)) return [];
  const out: SlideCustomTheme[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    if (out.length >= MAX_CUSTOM_THEMES) break;
    if (!item || typeof item !== 'object') continue;
    const o = item as Record<string, unknown>;
    const id =
      typeof o['id'] === 'string' && /^custom-[a-z0-9-]{1,24}$/.test(o['id']) ? o['id'] : '';
    const bg = typeof o['bg'] === 'string' ? normalizeHex(o['bg']) : null;
    const ink = typeof o['ink'] === 'string' ? normalizeHex(o['ink']) : null;
    if (!id || !bg || !ink || seen.has(id)) continue;
    seen.add(id);
    const name =
      typeof o['name'] === 'string'
        ? Array.from(o['name'].trim()).slice(0, MAX_THEME_NAME).join('')
        : '';
    const font = sanitizeThemeFont(o['font']);
    out.push({ id, name, bg, ink, ...(font ? { font } : {}) });
  }
  return out;
}

/** Id libre para un tema propio nuevo: `custom-1`, `custom-2`… */
export function nextCustomThemeId(themes: readonly SlideCustomTheme[]): string {
  const used = new Set(themes.map((t) => t.id));
  for (let n = 1; ; n++) if (!used.has(`custom-${n}`)) return `custom-${n}`;
}

/** Tema propio nuevo a partir de unos colores y una fuente (los de otro tema, para
 *  personalizarlo); `null` si ya hay `MAX_CUSTOM_THEMES`. */
export function addCustomTheme(
  themes: readonly SlideCustomTheme[],
  base: { bg: string; ink: string; font?: string },
  name: string,
): { themes: SlideCustomTheme[]; id: string } | null {
  const list = sanitizeCustomThemes(themes);
  if (list.length >= MAX_CUSTOM_THEMES) return null;
  const id = nextCustomThemeId(list);
  const [created] = sanitizeCustomThemes([{ id, name, ...base }]);
  return created ? { themes: [...list, created], id } : null;
}

/** Cambia el nombre, los colores o la fuente de un tema propio; un color o una fuente que no
 *  valen se ignoran (el tema conserva el que tenía). */
export function patchCustomTheme(
  themes: readonly SlideCustomTheme[],
  id: string,
  patch: Partial<Omit<SlideCustomTheme, 'id'>>,
): SlideCustomTheme[] {
  const bg = patch.bg === undefined ? null : normalizeHex(patch.bg);
  const ink = patch.ink === undefined ? null : normalizeHex(patch.ink);
  return sanitizeCustomThemes(
    sanitizeCustomThemes(themes).map((t) =>
      t.id !== id
        ? t
        : {
            ...t,
            ...(patch.name === undefined ? {} : { name: patch.name }),
            ...(bg ? { bg } : {}),
            ...(ink ? { ink } : {}),
            ...(patch.font === undefined ? {} : { font: sanitizeThemeFont(patch.font) }),
          },
    ),
  );
}

/** Quita un tema propio. */
export function removeCustomTheme(
  themes: readonly SlideCustomTheme[],
  id: string,
): SlideCustomTheme[] {
  return sanitizeCustomThemes(themes.filter((t) => t.id !== id));
}

function luminance(hex: string): number {
  const v = parseInt(hex.slice(1), 16);
  const lin = (c: number): number => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * lin((v >> 16) & 255) + 0.7152 * lin((v >> 8) & 255) + 0.0722 * lin(v & 255);
}

/** Razón de contraste WCAG entre dos colores `#RRGGBB` (1 a 21); 1 si alguno no lo es. */
export function contrastRatio(a: string, b: string): number {
  const x = normalizeHex(a);
  const y = normalizeHex(b);
  if (!x || !y) return 1;
  const [hi, lo] = [luminance(x), luminance(y)].sort((p, q) => q - p) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
}
