/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las carpetas de una cuenta externa contra los vectores COMPARTIDOS con Fyne
 * (apps/fyne/apps/mail/testdata/carpetas-externas.json, que recorre
 * `TestLasCarpetasExternasSonLasDeLaWeb`): Gmail las daba como «INBOX», «SENT» y
 * «DRAFT», y con la Papelera antes que Recibidos. Aquí solo se recorren; los casos y
 * su porqué viven en el JSON. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin ordenar en `sortMailboxes`, falla el caso de Gmail
 * por los puestos; con un collator fijo en inglés, el español pone «Ñandú» delante
 * de «Nube» y el sueco la «Ä» con la A. Deshecho.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import type { Locale } from '@sytue/i18n';
import type { ExternalMailFolder } from '@sytue/api-clients';
import { useT } from '../i18n/index.js';
import { mailboxGlyph, mailboxLabel, sortMailboxes } from './utils.js';
import type { GlyphName } from './glyphs.js';

interface Caso {
  nombre: string;
  locale: Locale;
  carpetas: ExternalMailFolder[];
  esperado: { id: string; rotulo: string; glifo: GlyphName }[];
}

const { casos } = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/carpetas-externas.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores que recorrer', () => {
  expect(casos.length).toBeGreaterThanOrEqual(6);
});

for (const c of casos) {
  test(c.nombre, () => {
    const t = useT(c.locale);
    const pintadas = sortMailboxes(c.locale, c.carpetas).map((f) => ({
      id: f.id,
      rotulo: mailboxLabel(t, f.name, f.role),
      glifo: mailboxGlyph(f.role),
    }));
    expect(pintadas).toEqual(c.esperado);
  });
}
