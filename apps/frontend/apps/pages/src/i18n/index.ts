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
import { bg } from './messages/bg.js';
import { cs } from './messages/cs.js';
import { da } from './messages/da.js';
import { de } from './messages/de.js';
import { el } from './messages/el.js';
import { es } from './messages/es.js';
import { et } from './messages/et.js';
import { fi } from './messages/fi.js';
import { fr } from './messages/fr.js';
import { ga } from './messages/ga.js';
import { hr } from './messages/hr.js';
import { hu } from './messages/hu.js';
import { is } from './messages/is.js';
import { it } from './messages/it.js';
import { lt } from './messages/lt.js';
import { lv } from './messages/lv.js';
import { mt } from './messages/mt.js';
import { nb } from './messages/nb.js';
import { nl } from './messages/nl.js';
import { pl } from './messages/pl.js';
import { pt } from './messages/pt.js';
import { ro } from './messages/ro.js';
import { sk } from './messages/sk.js';
import { sl } from './messages/sl.js';
import { sv } from './messages/sv.js';

export type ClaveMensaje = keyof typeof en;

const traductor =
  import.meta.env.SSR !== false
    ? createAppTranslator({
        en,
        bg,
        cs,
        da,
        de,
        el,
        es,
        et,
        fi,
        fr,
        ga,
        hr,
        hu,
        is,
        it,
        lt,
        lv,
        mt,
        nb,
        nl,
        pl,
        pt,
        ro,
        sk,
        sl,
        sv,
      })
    : await loadAppTranslator<typeof en>(import.meta.glob('./messages/*.ts'));

/** Traduce `key` al idioma `locale`; `params.count` elige la forma plural. */
export function t(locale: string, key: ClaveMensaje, params?: Vars): string {
  return traductor(locale as Locale)(key, params);
}
