/**
 * Catálogo de emoji buscable para el selector (picker), derivado del mismo mapa
 * EMOJI de la Ola 1 (MessageBody.tsx) — sin librerías externas (clean-room). Un
 * glyph puede tener varios shortcodes (👍 = :thumbsup: / :+1:); aquí se deduplica
 * por glyph conservando todos sus nombres, para buscar por cualquiera de ellos.
 */
import { EMOJI } from '../components/MessageBody.js';
import { contiene } from '@sytue/i18n';

export interface EmojiEntry {
  /** El emoji Unicode (lo que se inserta / con lo que se reacciona). */
  glyph: string;
  /** Shortcode primario (para etiqueta/aria). */
  name: string;
  /** Todos los shortcodes que mapean a este glyph (para la búsqueda). */
  names: string[];
}

/** Catálogo deduplicado por glyph, en el orden estable de EMOJI. */
export const EMOJI_ENTRIES: EmojiEntry[] = (() => {
  const byGlyph = new Map<string, EmojiEntry>();
  for (const [name, glyph] of Object.entries(EMOJI)) {
    const e = byGlyph.get(glyph);
    if (e) e.names.push(name);
    else byGlyph.set(glyph, { glyph, name, names: [name] });
  }
  return [...byGlyph.values()];
})();

/**
 * Filtra el catálogo por nombre/shortcode (subcadena, sin distinción de
 * mayúsculas). Query vacía → catálogo completo. Puro y testeable.
 */
export function searchEmoji(query: string): EmojiEntry[] {
  const q = query.trim().toLowerCase();
  if (!q) return EMOJI_ENTRIES;
  return EMOJI_ENTRIES.filter((e) => e.names.some((n) => contiene(n, q)));
}
