/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { ESPERA_MS, puedeEsconderse, type Contexto } from './controles.js';

const quieto: Contexto = {
  menuAbierto: false,
  focoDentro: false,
  punteroEncima: false,
  fijada: false,
};

describe('puedeEsconderse', () => {
  it('con la llamada quieta, la barra se aparta', () => {
    expect(puedeEsconderse(quieto)).toBe(true);
  });

  // Cada uno de estos cuatro evita un fallo concreto, no una molestia: un menú
  // huérfano flotando, el foco del teclado perdido, un clic en «salir» sin
  // querer, y desobedecer a quien la pidió.
  it.each([
    ['menuAbierto', { menuAbierto: true }],
    ['focoDentro', { focoDentro: true }],
    ['punteroEncima', { punteroEncima: true }],
    ['fijada', { fijada: true }],
  ] as const)('no se esconde con %s', (_nombre, parche) => {
    expect(puedeEsconderse({ ...quieto, ...parche })).toBe(false);
  });

  it('basta con uno para impedirlo, aunque el resto esté libre', () => {
    expect(puedeEsconderse({ ...quieto, focoDentro: true, menuAbierto: true })).toBe(false);
  });
});

describe('ESPERA_MS', () => {
  // Rango, no valor exacto: lo que se protege es que nadie lo deje en 500 ms
  // —parpadeo— ni en medio minuto —no se esconde nunca en un teléfono—.
  it('está en un rango usable', () => {
    expect(ESPERA_MS).toBeGreaterThanOrEqual(2000);
    expect(ESPERA_MS).toBeLessThanOrEqual(8000);
  });
});
