/**
 * Traductor de Sytue DevX. Los textos de la app viven aquí (no en @sytue/i18n,
 * que solo trae el cromo compartido y la mecánica), y el inglés es la fuente de
 * verdad: los otros 25 catálogos derivan su tipo de él, así que una clave
 * nueva sin traducir rompe la compilación (docs/CONVENTIONS.md §13.1).
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

/** `t` atada a un locale. Se construye una vez por render, en el layout o la isla. */
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
export const appT =
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

/** Tipo de la función de traducción de esta app (la que viaja por props). */
export type T = ReturnType<typeof appT>;

/** Atajo para el caso normal: `const t = useT(locale)`. */
export function useT(locale: Locale): T {
  return appT(locale);
}

/**
 * Mensaje de un error del control-plane a partir de su CÓDIGO.
 *
 * El sobre de la API viaja como (código estable, parámetros) y el `message` que
 * lo acompaña está en inglés: traducir por código es lo único que le enseña el
 * error en su idioma a quien navega en francés. Los códigos NO se traducen —son
 * contrato de API—, y uno desconocido cae en un texto honesto con el par
 * status/código en vez de inventarse una explicación.
 */
export function errorMessage(t: T, status: number, code: string): string {
  switch (code) {
    case 'unauthorized':
      return t('error.unauthorized');
    case 'forbidden':
      return t('error.forbidden');
    case 'not_found':
      return t('error.not_found');
    case 'conflict':
      return t('error.conflict');
    case 'quota_exceeded':
      return t('error.quota_exceeded');
    case 'spend_limit_exceeded':
      return t('error.spend_limit_exceeded');
    case 'suite_subscription_required':
      return t('error.suite_subscription_required');
    case 'too_many_requests':
      return t('error.too_many_requests');
    case 'unprocessable':
      return t('error.unprocessable');
    case 'bad_request':
    case 'invalid_json':
    case 'empty_body':
    case 'body_too_large':
    case 'multiple_json_objects':
      return t('error.bad_request');
    case 'unavailable':
      return t('error.unavailable');
    case 'internal':
    case 'unexpected':
      return t('error.internal');
    default:
      return t('error.unknown', { status, code });
  }
}

/** Etiqueta llana de una métrica facturable (build_minutes, storage_gb…). */
export function metricLabel(t: T, metric: string): string {
  switch (metric) {
    case 'build_minutes':
      return t('metric.build_minutes.label');
    case 'compute_gb_hours':
      return t('metric.compute_gb_hours.label');
    case 'storage_gb':
      return t('metric.storage_gb.label');
    // Una métrica nueva del backend se muestra con su clave antes que romper la vista.
    default:
      return metric;
  }
}

/** Explicación breve de qué mide una métrica facturable. */
export function metricHint(t: T, metric: string): string {
  switch (metric) {
    case 'build_minutes':
      return t('metric.build_minutes.hint');
    case 'compute_gb_hours':
      return t('metric.compute_gb_hours.hint');
    case 'storage_gb':
      return t('metric.storage_gb.hint');
    default:
      return '';
  }
}
