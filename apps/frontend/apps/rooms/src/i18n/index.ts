/**
 * Catálogo de Sytue Rooms (las apps `chat` y `meet` de la suite).
 *
 * Los textos de la app viven en la app, no en `@sytue/i18n`: de allí solo viene la
 * MECÁNICA (interpolación, plurales por `Intl.PluralRules`, y la garantía de que el
 * inglés manda). docs/CONVENTIONS.md §13.1.
 *
 * El catálogo INGLÉS es la fuente de verdad: `AppCatalog<typeof en>` obliga a que
 * los otros 25 tengan exactamente sus claves y su forma (texto llano o formas de
 * plural), así que añadir una clave en inglés ROMPE su compilación hasta que se
 * traduzcan.
 */
import { createAppTranslator, loadAppTranslator, type Locale } from '@sytue/i18n';
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

/** `t` de la app atada a un locale: `const t = roomsT(locale)`. */
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
export const roomsT =
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

/**
 * Clave válida del catálogo. Es lo que tipa los mapas de claves que se resuelven en
 * tiempo de EJECUCIÓN (`STATE_KEY`, `SOURCE_KEY`, `SidebarGroup.titleKey`…): sin este
 * tipo serían `string` y una clave mal escrita pasaría la compilación para salir en
 * pantalla como la propia clave.
 */
export type MessageKey = keyof typeof en;

/**
 * Función de traducción ya atada a un idioma. Es lo que viaja por props a los
 * componentes de presentación que no reciben el locale entero.
 */
export type T = ReturnType<typeof roomsT>;

export type { Locale };
