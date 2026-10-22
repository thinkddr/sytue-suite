/**
 * Traductor de Sytue Photos: los cinco catálogos de la app sobre el mecanismo
 * común de @sytue/i18n. `useT(locale)` devuelve la función `t` ya resuelta.
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

export type PhotosMessageKey = keyof typeof en;
/*
 * El servidor necesita los cinco catálogos —atiende a la vez a gente de los cinco
 * idiomas— y su bundle no viaja a nadie. El navegador solo necesita UNO: el que el
 * servidor ya escribió en `<html lang>`. Vite sustituye `import.meta.env.SSR` por
 * una constante, así que en el build de cliente la rama de los cinco muere y con
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
export type T = ReturnType<typeof useT>;
