/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lógica PURA de los subtítulos de un vídeo o un audio de diapositiva: pistas WebVTT
 * (el fichero `.vtt` entero, su idioma y si es la de por defecto) guardadas en la caja
 * `kind: 'media'`. Sin Preact ni DOM: se prueba con bun (test/slides-subtitulos.test.ts).
 *
 * Frontera de confianza: el `.vtt` lo sube el usuario y el modelo lo leen otras personas
 * y otras versiones, así que `sanitizeTracks` rehace la lista cada vez que se va a pintar.
 * El texto de una pista solo viaja a un `<track src="data:text/vtt,…">`: el navegador lo
 * interpreta como subtítulos y nunca como HTML.
 */
import type { SlideTrack } from '../types';

/** Pistas por medio. Los subtítulos van dentro del modelo de la presentación, que viaja
 *  entero en cada autosave (techo ~1 MiB del op-body); seis idiomas bastan de sobra. */
export const MAX_TRACKS = 6;
/** Techo de una pista, en caracteres: una hora de diálogo continuo son unos 60 000. */
export const MAX_VTT_CHARS = 120_000;

/** `.vtt` limpio (sin BOM, con saltos de línea `\n`) o `null` si no es WebVTT: la
 *  cabecera `WEBVTT` es obligatoria y va sola o seguida de espacio/tabulador/salto. */
export function cleanVtt(raw: string): string | null {
  const text = raw.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n');
  if (!/^WEBVTT(?:[ \t\n]|$)/.test(text) || text.length > MAX_VTT_CHARS) return null;
  return text;
}

/** ¿Es una etiqueta de idioma con forma de BCP 47 (`es`, `en-GB`, `zh-Hant-TW`)? Devuelve
 *  la forma canónica (idioma en minúsculas, región en mayúsculas) o `null`. */
export function normalizeLang(raw: string): string | null {
  const parts = raw.trim().replace(/_/g, '-').split('-');
  if (parts.length === 0 || parts.length > 4) return null;
  const [lang, ...rest] = parts;
  if (!lang || !/^[A-Za-z]{2,3}$/.test(lang)) return null;
  const out = [lang.toLowerCase()];
  for (const p of rest) {
    if (/^[A-Za-z]{4}$/.test(p)) out.push(p[0]!.toUpperCase() + p.slice(1).toLowerCase());
    else if (/^(?:[A-Za-z]{2}|\d{3})$/.test(p)) out.push(p.toUpperCase());
    else return null;
  }
  return out.join('-');
}

/** Idioma que sugiere el nombre de un fichero: `charla.es.vtt` → `es`, `en-GB.vtt` →
 *  `en-GB`. `null` si no lo dice. */
export function langFromFileName(name: string): string | null {
  const base = name.replace(/^.*[\\/]/, '').replace(/\.vtt$/i, '');
  const last = base.split('.').pop() ?? '';
  return normalizeLang(last);
}

/** Rehace la lista de pistas de una caja: solo las bien formadas, un máximo, sin idiomas
 *  repetidos y con una única pista por defecto (la primera que lo pida). */
export function sanitizeTracks(raw: unknown): SlideTrack[] {
  if (!Array.isArray(raw)) return [];
  const out: SlideTrack[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    if (out.length >= MAX_TRACKS) break;
    if (!item || typeof item !== 'object') continue;
    const o = item as Record<string, unknown>;
    const lang = typeof o['lang'] === 'string' ? normalizeLang(o['lang']) : null;
    const vtt = typeof o['vtt'] === 'string' ? cleanVtt(o['vtt']) : null;
    if (!lang || vtt === null || seen.has(lang)) continue;
    seen.add(lang);
    const label = typeof o['label'] === 'string' ? o['label'].trim().slice(0, 60) : '';
    out.push({
      lang,
      ...(label ? { label } : {}),
      vtt,
      ...(o['default'] === true && !out.some((t) => t.default) ? { default: true } : {}),
    });
  }
  return out;
}

/** Añade una pista (o sustituye la de su idioma). La primera, o la que lo pide, queda por defecto. */
export function withTrack(tracks: readonly SlideTrack[], track: SlideTrack): SlideTrack[] {
  const rest = tracks.filter((t) => t.lang !== track.lang);
  const isDefault = track.default === true || !rest.some((t) => t.default);
  return sanitizeTracks([
    ...rest.map((t) => (isDefault ? { ...t, default: false } : t)),
    { ...track, default: isDefault },
  ]);
}

/** Quita la pista de un idioma; si era la de por defecto, la primera que quede lo pasa a ser. */
export function withoutTrack(tracks: readonly SlideTrack[], lang: string): SlideTrack[] {
  const rest = tracks.filter((t) => t.lang !== lang);
  const keep = rest.some((t) => t.default)
    ? rest
    : rest.map((t, i) => ({ ...t, default: i === 0 }));
  return sanitizeTracks(keep);
}

/** Marca como por defecto la pista de un idioma (y desmarca las demás). */
export function withDefaultTrack(tracks: readonly SlideTrack[], lang: string): SlideTrack[] {
  return sanitizeTracks(tracks.map((t) => ({ ...t, default: t.lang === lang })));
}

/** `src` de un `<track>`: el `.vtt` como data URL. */
export function trackSrc(vtt: string): string {
  return `data:text/vtt;charset=utf-8,${encodeURIComponent(vtt)}`;
}

/** Nombre de la pista para el reproductor y el panel: la etiqueta propia o el idioma en su
 *  propia lengua («Español»); sin soporte de `Intl.DisplayNames`, el código. */
export function trackLabel(track: SlideTrack): string {
  if (track.label) return track.label;
  try {
    const name = new Intl.DisplayNames([track.lang], { type: 'language' }).of(track.lang);
    if (name && name !== track.lang)
      return name.charAt(0).toLocaleUpperCase(track.lang) + name.slice(1);
  } catch {
    // idioma que Intl no conoce: el código
  }
  return track.lang;
}

/** Idiomas de las pistas, para el distintivo «CC» de una caja. */
export function trackLangs(tracks: readonly SlideTrack[] | undefined): string {
  return sanitizeTracks(tracks)
    .map((t) => t.lang)
    .join(', ');
}
