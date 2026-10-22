/**
 * Los catálogos, indexados por locale. Punto único de importación de los
 * mensajes: quien traduce añade la clave en `en.ts` y el compilador le exige el
 * resto.
 */
import type { Locale } from '../locale.js';
import type { Catalog } from './catalog.js';
import { en } from './en.js';
import { bg } from './bg.js';
import { cs } from './cs.js';
import { da } from './da.js';
import { de } from './de.js';
import { el } from './el.js';
import { es } from './es.js';
import { et } from './et.js';
import { fi } from './fi.js';
import { fr } from './fr.js';
import { ga } from './ga.js';
import { hr } from './hr.js';
import { hu } from './hu.js';
import { is } from './is.js';
import { it } from './it.js';
import { lt } from './lt.js';
import { lv } from './lv.js';
import { mt } from './mt.js';
import { nb } from './nb.js';
import { nl } from './nl.js';
import { pl } from './pl.js';
import { pt } from './pt.js';
import { ro } from './ro.js';
import { sk } from './sk.js';
import { sl } from './sl.js';
import { sv } from './sv.js';

export type { Catalog, MessageKey, PluralForms } from './catalog.js';

export const catalogs: Record<Locale, Catalog> = {
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
};
