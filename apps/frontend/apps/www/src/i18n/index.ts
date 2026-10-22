/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * i18n del sitio público en los 26 idiomas de producto (docs/CONVENTIONS.md
 * §13.1). Dos cosas distintas viven aquí y conviene no confundirlas:
 *
 *   1. El CATÁLOGO — `useT(locale)`, con la mecánica de @sytue/i18n
 *      (interpolación `{var}`, plurales por Intl) y el inglés como fuente de
 *      verdad de la que los otros 25 derivan su tipo.
 *
 *   2. El ENRUTADO — y aquí www es el caso raro de la plataforma. Las otras diez
 *      apps son superficies autenticadas: negocian el idioma por petición
 *      (cookie > Accept-Language) y van `noindex`, así que una sola URL por
 *      página basta. www es el escaparate PÚBLICO e indexable: si el idioma
 *      dependiera de una cookie, Google vería una única URL, indexaría una sola
 *      versión y las otras 25 no existirían para el mundo. Por eso cada
 *      idioma tiene su propia URL:
 *
 *          inglés (BASE)  /            /downloads   /product/mail
 *          español        /es          /es/downloads
 *          francés        /fr          /fr/downloads
 *          alemán         /de          ...
 *          italiano       /it          ...
 *
 *      El inglés va SIN prefijo porque es el idioma base del producto (§13.1):
 *      la raíz es una página real y no una redirección, que es lo que quieren
 *      tanto el rastreador como el `x-default`.
 *
 *      Los SLUGS son los mismos en todos los idiomas (`/es/downloads`, no
 *      `/es/descargas`). Traducirlos daría un pelín de keyword-en-URL a cambio
 *      de una tabla de correspondencias que hay que mantener en cada página y
 *      en el selector de idioma; con hreflang recíproco bien puesto, Google ya
 *      entiende que son la misma página en otra lengua.
 *
 *      El enrutado vive en `./paths.ts` y se reexporta desde aquí. Está separado
 *      para que las islas Preact puedan construir enlaces sin arrastrar los 26
 *      catálogos al navegador: los TEXTOS de una isla se los pasa el servidor ya
 *      resueltos. Este módulo, el del catálogo, es SOLO para código de servidor.
 */
import { createAppTranslator, loadAppTranslator } from '@sytue/i18n';
import { en } from './messages/en';
import { bg } from './messages/bg';
import { cs } from './messages/cs';
import { da } from './messages/da';
import { de } from './messages/de';
import { el } from './messages/el';
import { es } from './messages/es';
import { et } from './messages/et';
import { fi } from './messages/fi';
import { fr } from './messages/fr';
import { ga } from './messages/ga';
import { hr } from './messages/hr';
import { hu } from './messages/hu';
import { is } from './messages/is';
import { it } from './messages/it';
import { lt } from './messages/lt';
import { lv } from './messages/lv';
import { mt } from './messages/mt';
import { nb } from './messages/nb';
import { nl } from './messages/nl';
import { pl } from './messages/pl';
import { pt } from './messages/pt';
import { ro } from './messages/ro';
import { sk } from './messages/sk';
import { sl } from './messages/sl';
import { sv } from './messages/sv';

/** Clave válida del catálogo del sitio público (la lista la fija el inglés). */
export type WwwKey = keyof typeof en;

/** `t` del sitio: `useT(locale)` devuelve la función atada al idioma. */
/*
 * El servidor necesita los 26 catálogos —atiende a la vez a gente de los 26
 * idiomas— y su bundle no viaja a nadie. El navegador solo necesita UNO: el que el
 * servidor ya escribió en `<html lang>`. Vite sustituye `import.meta.env.SSR` por
 * una constante, así que en el build de cliente la rama de los 26 muere y con
 * ella sus imports; el `import.meta.glob` deja un chunk por idioma y se baja el que
 * toca. El porqué y las cifras, en `loadAppTranslator`.
 *
 * `!== false` y no `? :` a secas: fuera de Vite (los tests, que corren en Bun) no
 * hay `import.meta.env.SSR` ni `import.meta.glob`, y con la comparación simple se
 * tomaba la rama del navegador y reventaba al importar el catálogo.
 */
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

/** Tipo de la `t` ya atada, para pasarla por props. */
export type T = ReturnType<typeof useT>;

export { localePath, localeParams, localeFromParam } from './paths';
