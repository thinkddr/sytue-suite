/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El árbol de buzones del lateral contra los vectores COMPARTIDOS con Fyne
 * (apps/fyne/apps/mail/testdata/arbol-de-buzones.json, que recorre
 * `TestElArbolDeBuzonesEsElDeLaWeb`). Aquí solo se recorren; los casos y su porqué
 * viven en el JSON. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin el `connected` de `estado`, la cuenta desconectada
 * que traía carpetas entraba en la Entrada unificada; sin `servesMail`, salía la de
 * Calendario; sumando solo la carpeta de Sytue, la Entrada de Gmail e iCloud daba 2
 * en vez de 6. Deshecho.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import type { Locale } from '@sytue/i18n';
import type { ExternalMailAccountSummary } from '@sytue/api-clients';
import { arbolDeBuzones } from './buzones.js';

interface Caso {
  nombre: string;
  locale: Locale;
  sytue: { id: string; name: string; role?: string; unread?: number }[];
  externas: ExternalMailAccountSummary[];
  esperado: {
    unificados: unknown[];
    cuentas: {
      id: string;
      titulo: string;
      icloud: boolean;
      estado: string;
      noLeidos: number;
      carpetas: string[];
    }[];
  };
}

const { casos } = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/arbol-de-buzones.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores que recorrer', () => {
  expect(casos.length).toBeGreaterThanOrEqual(5);
});

for (const c of casos) {
  test(c.nombre, () => {
    const arbol = arbolDeBuzones(
      c.locale,
      c.sytue.map((s) => ({ id: s.id, name: s.name, role: s.role, unreadEmails: s.unread ?? 0 })),
      c.externas,
    );
    expect(arbol.unificados).toEqual(c.esperado.unificados as typeof arbol.unificados);
    expect(
      arbol.cuentas.map((cuenta) => ({ ...cuenta, carpetas: cuenta.carpetas.map((f) => f.id) })),
    ).toEqual(c.esperado.cuentas as never);
  });
}
