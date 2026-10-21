/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los textos del panel «Quién puede consultar» de cada conexión (NEG-06), en los 26
 * idiomas.
 *
 * Catálogo APARTE del de la consola a propósito: la consola sigue en cinco idiomas (su
 * paso a 26 lo pospuso el dueño) y su tipo exige TODAS las claves en cada idioma, así
 * que no admite un polaco con solo estas. El dueño pidió este panel en los 26
 * (2026-10-16); el resto de la consola sigue como está. Cuando la consola pase a 26,
 * estas claves se mudan a su catálogo y este fichero desaparece.
 *
 * Misma mecánica que `../index.ts`: el servidor carga los 26, el navegador solo el
 * suyo (`import.meta.glob`).
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

export const useTAcceso =
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

/** Tipo de la `t` del panel ya atada a un idioma. */
export type TAcceso = ReturnType<typeof useTAcceso>;
