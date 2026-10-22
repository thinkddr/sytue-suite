/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de los subtítulos de un medio de diapositiva (slides/subtitulos.ts): validación
 * del `.vtt`, idioma, saneado de la lista en la frontera de confianza y alta/baja/por defecto
 * de las pistas. Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  MAX_TRACKS,
  MAX_VTT_CHARS,
  cleanVtt,
  langFromFileName,
  normalizeLang,
  sanitizeTracks,
  trackLabel,
  trackLangs,
  trackSrc,
  withDefaultTrack,
  withTrack,
  withoutTrack,
} from '../src/editor/slides/subtitulos';
import { sanitizeMediaSpec } from '../src/editor/slides/SlidesCanvas';

const VTT = 'WEBVTT\n\n00:00.000 --> 00:02.000\nHola\n';
const pista = (lang: string, extra: object = {}) => ({ lang, vtt: VTT, ...extra });

test('cleanVtt: exige la cabecera WEBVTT, quita el BOM y normaliza los saltos de línea', () => {
  expect(cleanVtt(VTT)).toBe(VTT);
  expect(cleanVtt('\uFEFFWEBVTT\r\n\r\n00:00.000 --> 00:01.000\r\nHi')).toBe(
    'WEBVTT\n\n00:00.000 --> 00:01.000\nHi',
  );
  expect(cleanVtt('WEBVTT - charla')).toBe('WEBVTT - charla');
  expect(cleanVtt('WEBVTTX\n')).toBeNull(); // no es la cabecera
  expect(cleanVtt('1\n00:00:01,000 --> 00:00:02,000\nSubRip, no WebVTT')).toBeNull();
  expect(cleanVtt('')).toBeNull();
  expect(cleanVtt('WEBVTT\n' + 'x'.repeat(MAX_VTT_CHARS))).toBeNull(); // pasa del techo
});

test('normalizeLang: forma canónica BCP 47 y rechazo de lo que no es un idioma', () => {
  expect(normalizeLang('es')).toBe('es');
  expect(normalizeLang(' EN-gb ')).toBe('en-GB');
  expect(normalizeLang('pt_br')).toBe('pt-BR');
  expect(normalizeLang('zh-hant-tw')).toBe('zh-Hant-TW');
  expect(normalizeLang('es-419')).toBe('es-419');
  for (const malo of ['', 'e', 'espanol', 'es-', 'es--ES', '<b>', 'es-XXXXXX', 'a-b-c-d-e'])
    expect(normalizeLang(malo)).toBeNull();
});

test('langFromFileName: el idioma que sugiere el nombre del fichero', () => {
  expect(langFromFileName('charla.es.vtt')).toBe('es');
  expect(langFromFileName('C:\\subs\\en-GB.VTT')).toBe('en-GB');
  expect(langFromFileName('subtitulos.vtt')).toBeNull(); // «subtitulos» no es un idioma
  expect(langFromFileName('video.vtt')).toBeNull();
});

test('sanitizeTracks: rehace la lista —solo lo bien formado, sin repetidos, una por defecto—', () => {
  const raw = [
    pista('es', { default: true }),
    pista('ES'), // mismo idioma
    pista('en', { default: true }), // segunda «por defecto»
    { lang: 'fr', vtt: 'no es webvtt' },
    { lang: '??', vtt: VTT },
    null,
    'texto',
    pista('de', { label: '  Deutsch  ' }),
  ];
  const out = sanitizeTracks(raw);
  expect(out.map((t) => t.lang)).toEqual(['es', 'en', 'de']);
  expect(out.filter((t) => t.default).map((t) => t.lang)).toEqual(['es']);
  expect(out[2]!.label).toBe('Deutsch');
  expect(sanitizeTracks('x')).toEqual([]);
  expect(sanitizeTracks(undefined)).toEqual([]);
});

test('sanitizeTracks: tope de pistas', () => {
  const langs = ['es', 'en', 'fr', 'de', 'it', 'pt', 'nl', 'pl'];
  expect(sanitizeTracks(langs.map((l) => pista(l)))).toHaveLength(MAX_TRACKS);
});

test('withTrack / withoutTrack / withDefaultTrack: la por defecto siempre es una', () => {
  let t = withTrack([], pista('es'));
  expect(t.map((x) => [x.lang, x.default])).toEqual([['es', true]]);
  t = withTrack(t, pista('en'));
  expect(t.map((x) => [x.lang, x.default === true])).toEqual([
    ['es', true],
    ['en', false],
  ]);
  t = withDefaultTrack(t, 'en');
  expect(t.find((x) => x.default)?.lang).toBe('en');
  t = withTrack(t, pista('es', { default: true })); // sustituye la de es y pasa a ser la de por defecto
  expect(t).toHaveLength(2);
  expect(t.filter((x) => x.default).map((x) => x.lang)).toEqual(['es']);
  t = withoutTrack(t, 'es'); // se quita la de por defecto: la que queda la hereda
  expect(t.map((x) => [x.lang, x.default])).toEqual([['en', true]]);
  expect(withoutTrack(t, 'en')).toEqual([]);
});

test('trackSrc: un data URL text/vtt que se puede decodificar', () => {
  const src = trackSrc(VTT);
  expect(src.startsWith('data:text/vtt;charset=utf-8,')).toBe(true);
  expect(decodeURIComponent(src.slice(src.indexOf(',') + 1))).toBe(VTT);
  expect(src).not.toContain('\n'); // nada que rompa el atributo
});

test('trackLabel y trackLangs', () => {
  expect(trackLabel({ lang: 'es', vtt: VTT })).toBe('Español');
  expect(trackLabel({ lang: 'es', vtt: VTT, label: 'Castellano' })).toBe('Castellano');
  expect(trackLabel({ lang: 'qq', vtt: VTT })).toBe('qq');
  expect(trackLangs([pista('es'), pista('en')] as never)).toBe('es, en');
  expect(trackLangs(undefined)).toBe('');
});

test('sanitizeMediaSpec: las pistas del descriptor se sanean; sin válidas no hay campo', () => {
  const base = { mediaType: 'video', src: 'https://x.com/a.mp4' };
  const conPistas = sanitizeMediaSpec({
    ...base,
    tracks: [pista('es'), { lang: 'xx', vtt: 'mal' }],
  });
  expect(conPistas?.tracks?.map((t) => t.lang)).toEqual(['es']);
  expect(sanitizeMediaSpec({ ...base, tracks: [{ lang: 'es', vtt: 'mal' }] })).not.toHaveProperty(
    'tracks',
  );
  expect(sanitizeMediaSpec(base)).not.toHaveProperty('tracks');
});
