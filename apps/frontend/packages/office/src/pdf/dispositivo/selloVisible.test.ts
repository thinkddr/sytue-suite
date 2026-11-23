/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Firmar en el dispositivo» (ADR 0086), segunda vuelta: el SELLO DE TIEMPO (PAdES
 * B-T) y la firma VISIBLE.
 *
 * El sello: a la autoridad solo le llega el SHA-256 del VALOR DE LA FIRMA (32 bytes) y
 * el token va como atributo no firmado; un token que no sella esa firma no entra. La
 * autoridad de estas pruebas fabrica el TSTInfo aquí mismo; la de verdad es
 * `openssl ts -reply` en `firmado-en-el-dispositivo-visible-con-sello.pdf` del corpus
 * de Go, que verifican `pdfverificar_corpus_test.go` (firma Y sello de confianza) y
 * Poppler (`pdfsig`), y que también se comprueba abajo.
 *
 * La visible: el mismo dibujo que el servidor (`objetoAparienciaFirma`), en la página
 * y el recuadro elegidos.
 */
import { expect, test } from 'bun:test';
import {
  cadenaOctetos,
  conjunto,
  contexto,
  enteroDER,
  hijo,
  hijos,
  leerNodo,
  nulo,
  oidDER,
  secuencia,
  tlv,
} from './der';
import { leerPkcs12 } from './pkcs12';
import { DocumentoPdf, ErrorPdf, Lector, buscar, esCadena } from './pdfObjetos';
import { firmarPdfConDetalle, verificarFirmas } from './pades';
import {
  ErrorSello,
  OID_SELLO,
  comprobarSello,
  leerSello,
  pedirSelloAlServidor,
  selloDisponible,
} from './sello';
import { rectFirma } from '../firmaPosicion';
import { motivoFirmaLocal } from './firmarEnBoveda';

const CORPUS = new URL('../../../../../../nucleo/pdf/testdata/pdf/', import.meta.url);
const P12 = new URL('../../../../../../../platform/pkg/pkcs12/testdata/', import.meta.url);
const leer = async (base: URL, nombre: string) =>
  new Uint8Array(await Bun.file(new URL(nombre, base)).arrayBuffer());
const latin1 = (b: Uint8Array) => Array.from(b, (c) => String.fromCharCode(c)).join('');
const ascii = (s: string) => Uint8Array.from(s, (c) => c.charCodeAt(0));
const sha256 = async (b: Uint8Array) =>
  new Uint8Array(await crypto.subtle.digest('SHA-256', b.slice()));
const fecha = new Date('2026-11-23T10:00:00Z');

/** Un TimeStampToken con la estructura de RFC 3161 que sella `huella` a las 10:00:30. */
function tokenDePrueba(huella: Uint8Array, alg: string = OID_SELLO.sha256) {
  const info = secuencia(
    enteroDER(1),
    oidDER('1.2.3.4.1'),
    secuencia(secuencia(oidDER(alg), nulo()), cadenaOctetos(huella)),
    enteroDER(7),
    tlv(0x18, ascii('20261123100030Z')),
  );
  const firmado = secuencia(
    enteroDER(3),
    conjunto(),
    secuencia(oidDER(OID_SELLO.tstInfo), tlv(contexto(0), cadenaOctetos(info))),
    conjunto(),
  );
  return secuencia(oidDER(OID_SELLO.firmado), tlv(contexto(0), firmado));
}

/** El valor de la firma del ÚLTIMO SignerInfo del PDF (lo que se sella). */
function valorDeLaUltimaFirma(pdf: Uint8Array<ArrayBuffer>): Uint8Array<ArrayBuffer> {
  let p = -1;
  for (let i = buscar(pdf, '/ByteRange'); i >= 0; i = buscar(pdf, '/ByteRange', i + 1)) p = i;
  const rango = new Lector(pdf, p + '/ByteRange'.length).objeto() as { v: string }[];
  const hex = new Lector(pdf, Number(rango[1]!.v)).objeto();
  if (!esCadena(hex)) throw new Error('sin /Contents');
  const ci = leerNodo(hex.v.slice());
  const sd = hijo(hijo(ci, 1, contexto(0)), 0);
  const firmantes = hijos(sd);
  const campos = hijos(hijo(firmantes[firmantes.length - 1]!, 0));
  const ultimo = campos[campos.length - 1]!;
  return campos[campos.length - (ultimo.etiqueta === contexto(1) ? 2 : 1)]!.valor.slice();
}

test('con sello: solo viaja la huella del valor de la firma y el token la cubre (B-T)', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), 'secreta');
  const original = await leer(CORPUS, 'sin-firmas.pdf');
  const pedidas: Uint8Array[] = [];
  const r = await firmarPdfConDetalle(original, id, {
    fecha,
    sello: async (huella) => {
      pedidas.push(huella.slice());
      return tokenDePrueba(huella);
    },
  });
  // Una sola petición, de 32 bytes, que es EL SHA-256 del valor de la firma.
  expect(pedidas).toHaveLength(1);
  expect(pedidas[0]!).toHaveLength(32);
  expect(pedidas[0]!).toEqual(await sha256(valorDeLaUltimaFirma(r.datos)));
  // Ni el documento ni su resumen: la huella no es el SHA-256 del PDF.
  expect(pedidas[0]!).not.toEqual(await sha256(original));
  expect(r.sello?.toISOString()).toBe('2026-11-23T10:00:30.000Z');
  expect(r.selloFallido).toBe(false);
  const [firma] = await verificarFirmas(r.datos);
  expect(firma).toMatchObject({
    firmante: 'EMPRESA DE PRUEBAS SL',
    integra: true,
    cubreTodo: true,
  });
  expect(firma!.sello).toEqual({ fecha: new Date('2026-11-23T10:00:30Z'), cubreFirma: true });
  // Lo que se firmó antes del sello sigue siendo lo mismo: el anexo no lo toca.
  expect(r.datos.subarray(0, original.length)).toEqual(original);
});

test('un sello de otra firma, o una autoridad caída, dejan la firma válida SIN sello', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), 'secreta');
  const original = await leer(CORPUS, 'sin-firmas.pdf');
  for (const sello of [
    async () => tokenDePrueba(new Uint8Array(32).fill(7)),
    async (h: Uint8Array) => tokenDePrueba(h, '2.16.840.1.101.3.4.2.2'),
    async () => {
      throw new ErrorSello('la autoridad no dio el sello (502)');
    },
    async () => ascii('no es un token'),
  ]) {
    const r = await firmarPdfConDetalle(original, id, { fecha, sello });
    expect(r.sello).toBeNull();
    expect(r.selloFallido).toBe(true);
    const [firma] = await verificarFirmas(r.datos);
    expect(firma).toEqual({ firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true });
    expect(latin1(r.datos)).not.toContain('no es un token');
  }
  // Cortar es cortar: no se firma sin sello a escondidas.
  const corte = new AbortController();
  const cortado = await firmarPdfConDetalle(original, id, {
    fecha,
    signal: corte.signal,
    sello: async () => {
      corte.abort();
      throw new DOMException('cortado', 'AbortError');
    },
  }).catch((e: unknown) => e);
  expect(cortado).toBeInstanceOf(DOMException);
});

test('el token de una autoridad de verdad (openssl ts) del corpus cubre la firma', async () => {
  const pdf = await leer(CORPUS, 'firmado-en-el-dispositivo-visible-con-sello.pdf');
  const [firma] = await verificarFirmas(pdf);
  expect(firma).toMatchObject({
    firmante: 'EMPRESA DE PRUEBAS SL',
    integra: true,
    cubreTodo: true,
  });
  expect(firma!.sello?.cubreFirma).toBe(true);
  // Y es visible: tiene recuadro y su apariencia dice quién y cuándo.
  const texto = latin1(pdf);
  expect(texto).toContain('(Firmado por EMPRESA DE PRUEBAS SL) Tj');
  expect(texto).toContain('(2026-11-23 10:00 UTC) Tj');
});

test('leerSello y comprobarSello: la hora y qué huella; lo demás se rechaza', async () => {
  const h = await sha256(ascii('firma'));
  const t = tokenDePrueba(h);
  expect(leerSello(t).huella).toEqual(h);
  expect(comprobarSello(t, h).toISOString()).toBe('2026-11-23T10:00:30.000Z');
  expect(() => comprobarSello(t, new Uint8Array(32))).toThrow(ErrorSello);
  expect(() => comprobarSello(tokenDePrueba(h, '1.3.14.3.2.26'), h)).toThrow(ErrorSello);
  expect(() => leerSello(ascii('basura'))).toThrow(ErrorSello);
  expect(motivoFirmaLocal(new ErrorSello('x'))).toBe('fallo');
});

test('el cliente del BFF manda SOLO {resumen} en base64 y lee el token', async () => {
  const llamadas: { url: string; init: RequestInit | undefined }[] = [];
  const h = await sha256(ascii('firma'));
  const token = tokenDePrueba(h);
  const falso = (async (url: string, init?: RequestInit) => {
    llamadas.push({ url, init });
    if (init?.method === 'POST')
      return Response.json({ token: btoa(String.fromCharCode(...token)) });
    return Response.json({ disponible: true, autoridad: 'tsa.example' });
  }) as unknown as typeof fetch;
  expect(await selloDisponible(undefined, falso)).toEqual({
    disponible: true,
    autoridad: 'tsa.example',
  });
  expect(await pedirSelloAlServidor(falso)(h)).toEqual(token);
  const post = llamadas.find((l) => l.init?.method === 'POST')!;
  expect(post.url).toBe('/api/pdf-sello');
  expect(JSON.parse(String(post.init!.body))).toEqual({
    resumen: btoa(String.fromCharCode(...h)),
  });
  // Nada que no sea una huella SHA-256 sale de aquí.
  await expect(pedirSelloAlServidor(falso)(new Uint8Array(64))).rejects.toThrow(ErrorSello);
  // Sin autoridad (o sin saberlo), no hay sello: la pantalla lo dice.
  const sin = (async () => new Response('', { status: 404 })) as unknown as typeof fetch;
  expect(await selloDisponible(undefined, sin)).toEqual({ disponible: false, autoridad: '' });
  const caido = (async () => {
    throw new TypeError('red');
  }) as unknown as typeof fetch;
  expect((await selloDisponible(undefined, caido)).disponible).toBe(false);
});

test('firma visible: la página y el recuadro elegidos, con la apariencia del servidor', async () => {
  const id = await leerPkcs12(await leer(P12, 'clasico.p12'), 'secreta');
  const original = await leer(CORPUS, 'sin-firmas.pdf');
  const paginas = await (await DocumentoPdf.abrir(original)).paginas();
  expect(paginas).toHaveLength(2);
  const rect = rectFirma('abajoIzquierda', paginas[1]!.caja);
  const trazos: [number, number][][] = [
    [
      [rect[0] + 10, rect[1] + 40],
      [rect[0] + 60, rect[1] + 50],
      [rect[0] + 120, rect[1] + 30],
    ],
  ];
  const r = await firmarPdfConDetalle(original, id, {
    fecha,
    visible: { pagina: 2, rect, trazos },
  });
  const anexo = latin1(r.datos.subarray(original.length));
  // El widget cuelga de la página 2, con su recuadro (no el 0 0 0 0 de la invisible).
  expect(anexo).toContain(`/P ${paginas[1]!.n} 0 R`);
  expect(anexo).toContain(`/Rect [${rect.join(' ')}]`);
  expect(anexo).not.toContain('/Rect [0 0 0 0]');
  // El dibujo: fondo, borde, la rúbrica en coordenadas de SU caja y el texto.
  expect(anexo).toContain('/Subtype /Form /FormType 1 /BBox [0 0 240 74]');
  expect(anexo).toContain('/BaseFont /Helvetica /Encoding /WinAnsiEncoding');
  expect(anexo).toContain('q 0.98 0.98 0.97 rg 0 0 240 74 re f');
  expect(anexo).toContain('10 40 m 60 50 l 120 30 l\nS');
  expect(anexo).toContain('(Firmado por EMPRESA DE PRUEBAS SL) Tj');
  expect(anexo).toContain('(2026-11-23 10:00 UTC) Tj');
  // La página 2 reescrita lleva la anotación, y el PDF se vuelve a leer y verifica.
  const doc = await DocumentoPdf.abrir(r.datos);
  const pagina2 = await doc.objeto(paginas[1]!.n);
  expect(pagina2 instanceof Map && Array.isArray(pagina2.get('Annots'))).toBe(true);
  expect(await verificarFirmas(r.datos)).toEqual([
    { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true },
  ]);

  // Un nombre con paréntesis y fuera de WinAnsi no rompe el flujo.
  const raro = await firmarPdfConDetalle(original, id, {
    fecha,
    nombre: 'Ana (Δ) \\ Pérez',
    visible: { pagina: 1, rect: rectFirma('arribaDerecha', paginas[0]!.caja) },
  });
  expect(latin1(raro.datos)).toContain('(Firmado por Ana \\(?\\) \\\\ P\xe9rez) Tj');

  // Fuera de la página, o en una que no existe: se dice, no se escribe nada raro.
  for (const visible of [
    { pagina: 3, rect },
    { pagina: 1, rect: [-50, -50, 10, 10] as [number, number, number, number] },
    { pagina: 1, rect: [10, 10, 10.5, 80] as [number, number, number, number] },
  ]) {
    const e = await firmarPdfConDetalle(original, id, { fecha, visible }).catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ErrorPdf);
    expect(motivoFirmaLocal(e)).toBe('pagina');
  }
});

test('la firma invisible sigue siendo la de siempre (por defecto)', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), 'secreta');
  const original = await leer(CORPUS, 'sin-firmas-flujos.pdf');
  const r = await firmarPdfConDetalle(original, id, { fecha });
  const anexo = latin1(r.datos.subarray(original.length));
  expect(anexo).toContain('/Rect [0 0 0 0]');
  expect(anexo).toContain('/BBox [0 0 0 0] /Length 0');
  expect(anexo).not.toContain('Firmado por');
  expect(r.sello).toBeNull();
  expect(r.selloFallido).toBe(false);
});
