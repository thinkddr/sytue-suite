/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los mismos casos que recorre el servicio Go (services/mail/internal/logo,
 * `TestDominiosCompartidos`): la web pide el logo del dominio que el servicio acepta.
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { dominioDeLogo } from './dominioRemitente.js';

const { casos } = JSON.parse(
  readFileSync(
    new URL(
      '../../../../../../services/mail/internal/logo/testdata/dominios.json',
      import.meta.url,
    ),
    'utf8',
  ),
) as { casos: { correo: string; dominio: string | null }[] };

describe('dominioDeLogo', () => {
  test('decide lo mismo que el servicio Go', () => {
    expect(casos.length).toBeGreaterThan(0);
    for (const c of casos) {
      expect(dominioDeLogo(c.correo), c.correo).toBe(c.dominio);
    }
  });
});
