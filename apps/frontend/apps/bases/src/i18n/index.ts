/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de Sytue Bases en los 26 idiomas de la suite. El INGLÉS es la fuente de
 * verdad: los demás derivan su tipo de él (`AppCatalog<typeof en>`), así que una clave
 * nueva en inglés rompe la compilación de los otros 25 en vez de salir sin traducir.
 *
 * Uso: en `.astro`, `const t = useT(Astro.locals.locale)`; en una isla Preact el locale
 * llega por props y la isla hace `const t = useT(locale)`. Nunca se detecta el idioma
 * dentro de la isla. El servidor conserva los 26 catálogos; el navegador baja solo el
 * suyo (ver `catalogo-por-app.test.ts` de @sytue/i18n).
 */
import { createAppTranslator, loadAppTranslator } from '@sytue/i18n';
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

export type BasesMessageKey = keyof typeof en;

export const useT =
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

/** Tipo de la `t` ya atada, para tipar las props de las islas. */
export type T = ReturnType<typeof useT>;
