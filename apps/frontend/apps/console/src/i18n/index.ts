/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de Sytue Admin en los cinco idiomas de producto (docs/CONVENTIONS.md
 * §13.1). El INGLÉS es la fuente de verdad: los otros cuatro derivan su tipo de
 * él (`AppCatalog<typeof en>`), así que una clave nueva —o convertida en plural—
 * en inglés rompe la compilación de es/fr/de/it en vez de salir sin traducir.
 *
 * La mecánica (interpolación, plurales por `Intl.PluralRules`) la pone
 * `@sytue/i18n`; aquí solo viven los textos de ESTA app.
 *
 * Uso:
 *   - `.astro`: `const t = useT(Astro.locals.locale)`
 *   - isla Preact: el locale llega por props desde el `.astro` y la isla hace
 *     `const t = useT(locale)`. Nunca se detecta el idioma dentro de la isla.
 */
import { createAppTranslator, loadAppTranslator } from '@sytue/i18n';
import { en } from './messages/en.js';
import { es } from './messages/es.js';
import { fr } from './messages/fr.js';
import { de } from './messages/de.js';
import { it } from './messages/it.js';

export type ConsoleMessageKey = keyof typeof en;

/**
 * `t` de la consola atada a un locale.
 *
 * El servidor necesita los cinco catálogos —atiende a la vez a gente de los cinco
 * idiomas— y su bundle no viaja a nadie. El navegador solo necesita UNO: el que
 * el servidor ya escribió en `<html lang>`. Vite sustituye `import.meta.env.SSR`
 * por una constante, así que en el build de cliente la rama de arriba muere y con
 * ella los cinco imports; el `import.meta.glob` deja un chunk por idioma y se baja
 * el que toca.
 *
 * `!== false` y no `? :` a secas: fuera de Vite (los tests, que corren en Bun) no
 * hay `import.meta.env.SSR` ni `import.meta.glob`, y con la comparación simple se
 * tomaba la rama del navegador y reventaba al importar el catálogo.
 *
 * Medido en esta app: el chunk i18n pasa de 895.861 B (246.097 B gzip) a ~180 KB.
 */
export const useT =
  import.meta.env.SSR !== false
    ? createAppTranslator({ en, es, fr, de, it })
    : await loadAppTranslator<typeof en>(import.meta.glob('./messages/*.ts'));

/** Tipo de la `t` ya atada, para tipar las props de las islas. */
export type T = ReturnType<typeof useT>;
