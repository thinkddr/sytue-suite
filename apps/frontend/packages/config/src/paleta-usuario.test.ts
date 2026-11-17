/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La paleta de colores de usuario: lo que cada tono promete en contraste lo mide
 * `contraste-paleta.test.ts` contra las hojas de tokens; aquí, que los doce se distinguen
 * entre sí y que un color guardado con cualquiera de las paletas de antes se pinta con el
 * tono que su dueño reconocería.
 */
import { expect, test } from 'bun:test';
import { catalogs } from '@sytue/i18n';
import {
  PALETA_DE_USUARIO,
  claveDeColorDeUsuario,
  colorDeUsuario,
  tonoDeUsuario,
} from './paleta-usuario.js';

test('doce tonos con id único y hex de seis cifras en minúsculas', () => {
  expect(PALETA_DE_USUARIO).toHaveLength(12);
  expect(new Set(PALETA_DE_USUARIO.map((c) => c.id)).size).toBe(12);
  for (const { valor } of PALETA_DE_USUARIO) expect(valor).toMatch(/^#[0-9a-f]{6}$/);
});

test('cada tono tiene su nombre en el catálogo común', () => {
  for (const { id } of PALETA_DE_USUARIO) {
    expect(Object.keys(catalogs.en)).toContain(claveDeColorDeUsuario(id));
  }
});

test('los ocho tokens que Mail ya guarda siguen siendo tonos de la paleta', () => {
  // `domain.LabelColors` del servicio de Mail antes de esta paleta.
  for (const token of ['slate', 'brass', 'sage', 'clay', 'blue', 'plum', 'teal', 'rose']) {
    expect(tonoDeUsuario(token)?.id).toBe(token as never);
  }
});

test('un tono de la paleta se pinta tal cual, escrito como se escriba', () => {
  for (const { valor } of PALETA_DE_USUARIO) {
    expect(colorDeUsuario(valor)).toBe(valor);
    expect(colorDeUsuario(valor.toUpperCase())).toBe(valor);
    // `#rrggbbaa`: lo escriben clientes CalDAV como el de Apple.
    expect(colorDeUsuario(`${valor}ff`)).toBe(valor);
  }
});

test('sin color, o con uno que no se entiende, no hay tono y pinta el de reserva', () => {
  for (const nada of [undefined, null, '', 'var(--sytue-primary)', 'rojo', '#12345'])
    expect(colorDeUsuario(nada)).toBeUndefined();
});

test('los colores guardados con las paletas de antes caen en su tono', () => {
  const casos: [string, string][] = [
    // Calendar (los tonos joya)
    ['#a31f38', 'red'],
    ['#b0470f', 'clay'],
    ['#94590f', 'brass'],
    ['#146a47', 'green'],
    ['#0f7052', 'teal'],
    ['#0e6c84', 'cyan'],
    ['#1e50a8', 'blue'],
    ['#2e5a8c', 'blue'],
    ['#4338ca', 'indigo'],
    ['#6b3e9e', 'plum'],
    ['#a23e8c', 'rose'],
    ['#334155', 'slate'],
    // Cuadernos y etiquetas de Notes
    ['#86198f', 'plum'],
    ['#be123c', 'red'],
    ['#b0413a', 'red'],
    ['#8a6a1e', 'brass'],
    ['#565b62', 'slate'],
    // Los hex que pintaba Mail para sus tokens
    ['#5f7a5a', 'sage'],
    ['#7a5480', 'plum'],
    ['#9c5470', 'rose'],
    // Marcas de sensibilidad
    ['#047857', 'teal'],
    ['#b91c1c', 'red'],
    ['#4b5563', 'slate'],
    // Y lo que traiga otro cliente: los primarios puros y los grises
    ['#f00', 'red'],
    ['#00ff00', 'green'],
    ['#000000', 'slate'],
    ['#ffffff', 'slate'],
  ];
  const mal = casos
    .map(([hex, quiero]) => [hex, quiero, tonoDeUsuario(hex)?.id] as const)
    .filter(([, quiero, sale]) => quiero !== sale);
  expect(mal).toEqual([]);
});

test('cada tono se reconoce aunque se acerque un 30 % a su vecino', () => {
  // Con la misma luz, solo los separa el matiz: si dos vecinos estuvieran demasiado
  // juntos, un color guardado a medio camino caería en el que no es.
  const mezcla = (x: string, y: string, p: number): string =>
    '#' +
    [1, 3, 5]
      .map((k) => {
        const v = parseInt(x.slice(k, k + 2), 16) * (1 - p) + parseInt(y.slice(k, k + 2), 16) * p;
        return Math.round(v).toString(16).padStart(2, '0');
      })
      .join('');
  const mal: string[] = [];
  PALETA_DE_USUARIO.forEach((a, i) => {
    for (const b of [PALETA_DE_USUARIO.at(i - 1)!, PALETA_DE_USUARIO.at(i + 1 - 12)!]) {
      const sale = tonoDeUsuario(mezcla(a.valor, b.valor, 0.3))?.id;
      if (sale !== a.id) mal.push(`${a.id} hacia ${b.id} → ${sale}`);
    }
  });
  expect(mal).toEqual([]);
});
