/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los textos de las herramientas de gestión de personas (acceso efectivo, sesiones,
 * bajas recuperables, historial y detalle de los lotes, export del directorio,
 * filtros de dispositivos, rangos de auditoría y duplicar un rol), en los 26
 * idiomas.
 *
 * Catálogo APARTE por lo mismo que `../acceso`: la consola sigue en cinco idiomas
 * (su paso a 26 lo pospuso el dueño) y su tipo exige TODAS las claves en cada
 * idioma; lo nuevo nace ya en los 26. Cuando la consola pase a 26, estas claves se
 * mudan a su catálogo y este fichero desaparece.
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

export const useTGestion =
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

/** Tipo de la `t` de gestión ya atada a un idioma. */
export type TGestion = ReturnType<typeof useTGestion>;

/** Clave del catálogo de gestión (para mapas código → texto). */
export type ClaveGestion = keyof typeof en;
