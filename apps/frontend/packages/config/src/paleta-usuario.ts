/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA PALETA DE COLORES DE USUARIO: la única que se ofrece cuando alguien elige el color
 * de algo suyo —una etiqueta de Mail, un cuaderno o una etiqueta de Notes, un
 * calendario, una marca de sensibilidad—. Antes había seis, con hex distintos y medidas
 * distintas (o ninguna): el mismo «rojo» no era el mismo en Mail que en Notes, y en el
 * oscuro la mitad se perdía contra la superficie.
 *
 * Los doce tonos tienen la MISMA luminancia relativa (≈0,167) y cambian solo de matiz.
 * Es la banda en la que cabe todo lo que se les pide en los cuatro modos de la suite,
 * y lo mide `contraste-paleta.test.ts` contra las hojas de tokens reales:
 *   - 3:1 como indicador (WCAG 1.4.11) contra el lienzo, la superficie y la superficie
 *     elevada del claro, el traslúcido y los dos oscuros: el punto de una etiqueta o la
 *     casilla de un calendario tienen forma sin depender del contorno;
 *   - 4,5:1 con texto blanco encima (WCAG 1.4.3): la barra de un evento, la píldora de
 *     un estado de Notes o la marca de sensibilidad llevan rótulo.
 *
 * Los colores YA GUARDADOS no se migran: se PINTAN con el tono de la paleta más cercano
 * (`colorDeUsuario`), y el valor guardado sigue intacto hasta que su dueño elige otro.
 * Un calendario que trae su color de otro cliente CalDAV, o una etiqueta creada con la
 * paleta de antes, se ven con el tono de la suite sin que nadie reescriba nada.
 *
 * Los IDS son los tokens que Mail guarda desde siempre (`domain.LabelColors`): las ocho
 * etiquetas que ya existen siguen siendo válidas y los cuatro tonos nuevos se suman. El
 * nombre que se lee y se oye es `userColor.<id>` del catálogo común.
 *
 * El espejo nativo es `apps/nucleo/tokens/usuario.go` (lo compara su test).
 */

import type { MessageKey } from '@sytue/i18n';

/** Un tono de la paleta: su id estable y el hex que se guarda y se pinta. */
export interface ColorDeUsuario {
  readonly id: IdColorDeUsuario;
  readonly valor: string;
}

const TONOS = [
  { id: 'red', valor: '#c44744' },
  { id: 'clay', valor: '#b65623' },
  { id: 'brass', valor: '#946a10' },
  { id: 'sage', valor: '#64793c' },
  { id: 'green', valor: '#2f8147' },
  { id: 'teal', valor: '#098070' },
  { id: 'cyan', valor: '#097d91' },
  { id: 'blue', valor: '#2a72c7' },
  { id: 'indigo', valor: '#6564d6' },
  { id: 'plum', valor: '#9259b8' },
  { id: 'rose', valor: '#b84b7c' },
  { id: 'slate', valor: '#677380' },
] as const;

export type IdColorDeUsuario = (typeof TONOS)[number]['id'];

/** Los doce tonos, en el orden del círculo cromático y el gris al final. */
export const PALETA_DE_USUARIO: readonly ColorDeUsuario[] = TONOS;

/**
 * La clave del catálogo común con el nombre del tono («Rojo», «Gris»…). Devuelve
 * `MessageKey`: si a un tono le falta su nombre en el catálogo, esto no compila.
 */
export const claveDeColorDeUsuario = (id: IdColorDeUsuario): MessageKey => `userColor.${id}`;

type Lab = [number, number, number];

/** sRGB → OKLab: la distancia en OKLab se parece a la que ve el ojo; en RGB, no. */
function oklab(hex: string): Lab {
  const n = parseInt(hex.slice(1, 7), 16);
  const lin = [n >> 16, (n >> 8) & 255, n & 255].map((v) => {
    const s = v / 255;
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  const [r, g, b] = lin;
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

/**
 * Un color guardado, normalizado a `#rrggbb` en minúsculas: acepta `#rgb`, `#rrggbb` y
 * el `#rrggbbaa` que escriben algunos clientes CalDAV (la transparencia no se pinta).
 */
function hexDe(guardado: string): string | undefined {
  const m = /^#?([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i.exec(guardado.trim());
  if (!m) return undefined;
  const h = m[1]!.toLowerCase();
  return h.length === 3 ? `#${h[0]}${h[0]}${h[1]}${h[1]}${h[2]}${h[2]}` : `#${h.slice(0, 6)}`;
}

/*
 * La luz pesa la MITAD que el matiz al buscar el más cercano. Los tonos de antes eran casi
 * todos más oscuros que estos, y a distancia plana un azul acero (#2e5a8c) caía en el
 * petróleo y el ciruela de Mail (#7a5480) en el gris: se parecían más en luz que en color.
 */
const W_LUZ = 0.5;
const LABS = PALETA_DE_USUARIO.map((c) => oklab(c.valor));

/**
 * El tono de la paleta con que se pinta un color guardado: el suyo si es un id o un hex
 * de la paleta, y si no, el más cercano. `undefined` si no hay color (vacío) o no se
 * entiende: quien pinta pone entonces su color por defecto.
 */
export function tonoDeUsuario(guardado: string | null | undefined): ColorDeUsuario | undefined {
  if (!guardado) return undefined;
  const porId = PALETA_DE_USUARIO.find((c) => c.id === guardado);
  if (porId) return porId;
  const hex = hexDe(guardado);
  if (!hex) return undefined;
  const [l, a, b] = oklab(hex);
  let mejor = 0;
  let distancia = Infinity;
  LABS.forEach(([pl, pa, pb], i) => {
    const d = (W_LUZ * (l - pl)) ** 2 + (a - pa) ** 2 + (b - pb) ** 2;
    if (d < distancia) [mejor, distancia] = [i, d];
  });
  return PALETA_DE_USUARIO[mejor];
}

/** El hex con que se pinta un color guardado (ver `tonoDeUsuario`). */
export function colorDeUsuario(guardado: string | null | undefined): string | undefined {
  return tonoDeUsuario(guardado)?.valor;
}
