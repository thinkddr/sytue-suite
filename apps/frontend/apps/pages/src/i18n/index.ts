/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de Sytue Web (el editor, la tienda, la bandeja de formularios y la IA),
 * con la mecánica común de `@sytue/i18n`, como las demás apps. El INGLÉS es la
 * fuente de verdad: los otros derivan su tipo de él (`AppCatalog<typeof en>`), así
 * que una clave nueva en inglés rompe la compilación del idioma que no la tenga.
 *
 * El servidor necesita todos los catálogos; el navegador, solo el de `<html lang>`:
 * la rama de todos queda detrás de `import.meta.env.SSR` y el cliente baja su chunk
 * por `import.meta.glob` (ver `useT` en apps/mail/src/i18n/index.ts).
 */
import { createAppTranslator, loadAppTranslator, type Locale, type Vars } from '@sytue/i18n';
import { en } from './messages/en.js';
import { de } from './messages/de.js';
import { es } from './messages/es.js';
import { fr } from './messages/fr.js';
import { it } from './messages/it.js';
import { nl } from './messages/nl.js';
import { pt } from './messages/pt.js';

export type ClaveMensaje = keyof typeof en;

const traductor =
  import.meta.env.SSR !== false
    ? createAppTranslator({ en, de, es, fr, it, nl, pt })
    : await loadAppTranslator<typeof en>(import.meta.glob('./messages/*.ts'));

/** Traduce `key` al idioma `locale`; `params.count` elige la forma plural. */
export function t(locale: string, key: ClaveMensaje, params?: Vars): string {
  return traductor(locale as Locale)(key, params);
}
