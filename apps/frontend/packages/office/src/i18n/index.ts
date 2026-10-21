/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de Sytue Docs en los 26 idiomas de producto (docs/CONVENTIONS.md
 * §13.1). El INGLÉS es la fuente de verdad: los otros 25 derivan su tipo de
 * él (`AppCatalog<typeof en>`), así que una clave nueva en inglés rompe la
 * compilación de los demás en vez de aparecer sin traducir en producción.
 *
 * La mecánica (interpolación `{var}`, plurales por `Intl.PluralRules`) la pone
 * `@sytue/i18n`; aquí solo viven los textos de esta app.
 */
import { createAppTranslator, loadAppTranslator, documentLocale } from '@sytue/i18n';
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

/**
 * Clave válida del catálogo de Docs: la lista la fija el inglés. Sin `& string`,
 * que aquí no aporta nada —las claves del catálogo ya son cadenas— y solo hace
 * que el tipo se lea peor.
 */
export type DocsKey = keyof typeof en;

/**
 * `docsT(locale)` devuelve la `t` atada a ese idioma. NO se llama `useDocsT`
 * aposta: no es un hook y llamarlo dentro de un `useMemo` haría saltar la regla
 * de hooks sin motivo.
 */
/*
 * El servidor necesita los 26 catálogos —atiende a la vez a gente de todos los
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
export const docsT =
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

/** Tipo de la `t` ya atada, para pasarla por props / por el EditorContext. */
export type DocsT = ReturnType<typeof docsT>;

/**
 * `t` para los módulos del editor que NO son componentes: los que construyen
 * HTML como cadena (`canvas/exec.ts`, `canvas/embedDom.ts`, `docblocks.ts`,
 * `smartart.ts`). Ahí no hay hook ni props que seguir, y propagar el traductor
 * por firmas obligaría a tocar `exec(command, value?)`, que es el punto por el
 * que pasa TODO el editor.
 *
 * Lee el idioma del `<html lang>`, que es el que ya fija el SSR de la página, y
 * cae a inglés si no hay documento: estos módulos son de cliente, pero alguno se
 * importa desde tests que corren en Node.
 */
export function editorT(): DocsT {
  return docsT(documentLocale());
}
