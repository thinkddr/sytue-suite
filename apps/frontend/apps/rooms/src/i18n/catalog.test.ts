// Checks del catálogo de Sytue Rooms. Ejecutar: `bun test`.
//
// El TIPO ya garantiza que todos los idiomas tengan las mismas claves y la misma
// FORMA (`AppCatalog<typeof en>`). Lo que el tipo NO puede garantizar es lo de
// dentro de un plural: `PluralForms` solo exige `other`, así que un catálogo al que
// le falte `one` compila igual y luego dice «1 respuestas» en producción. Eso es lo
// que se comprueba aquí, junto con los contadores de la mensajería —que es donde
// más duele— resueltos de verdad por `Intl.PluralRules`.
import { expect, test } from 'bun:test';
import type { Locale } from '@sytue/i18n';
import { roomsT } from './index.js';
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

// Los 26 de Rooms: los idiomas se publican app a app (CONVENTIONS §13.1).
const CATALOGS: Partial<Record<Locale, Record<string, unknown>>> = {
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

test('ningún plural pierde la forma «one» al traducirse', () => {
  const pluralKeys = Object.entries(en)
    .filter(([, v]) => typeof v === 'object')
    .map(([k]) => k);
  // Si esto es 0, alguien convirtió los contadores en texto llano: la comprobación
  // pasaría sin comprobar nada.
  expect(pluralKeys.length).toBeGreaterThan(15);
  for (const [locale, catalog] of Object.entries(CATALOGS)) {
    for (const key of pluralKeys) {
      // `one` y `other` siempre; el polaco o el checo añaden `few`/`many` y están bien.
      const formas = Object.keys(catalog[key] as Record<string, string>);
      const faltan = ['one', 'other'].filter((f) => !formas.includes(f));
      expect(`${locale}:${key} sin ${faltan.join(',')}`).toBe(`${locale}:${key} sin `);
    }
  }
});

test('los contadores de la mensajería concuerdan en singular y en plural', () => {
  expect(roomsT('en')('thread.replyCount', { count: 1 })).toBe('1 reply');
  expect(roomsT('en')('thread.replyCount', { count: 3 })).toBe('3 replies');
  expect(roomsT('es')('thread.replyCount', { count: 1 })).toBe('1 respuesta');
  expect(roomsT('es')('thread.replyCount', { count: 3 })).toBe('3 respuestas');
  expect(roomsT('de')('channel.mentions', { count: 1 })).toBe('1 Erwähnung');
  expect(roomsT('de')('channel.mentions', { count: 2 })).toBe('2 Erwähnungen');
  expect(roomsT('it')('members.count', { count: 1 })).toBe('1 persona');
  expect(roomsT('it')('members.count', { count: 9 })).toBe('9 persone');
  // El francés manda el 0 al singular: es justo el caso que un `count === 1` falla.
  expect(roomsT('fr')('search.resultCount', { count: 0 })).toBe('0 résultat');
  expect(roomsT('fr')('search.resultCount', { count: 2 })).toBe('2 résultats');
  // Las lenguas con más de dos formas: si una cae en `other`, el número no concuerda.
  expect(roomsT('pl')('thread.replyCount', { count: 3 })).toBe('3 odpowiedzi');
  expect(roomsT('pl')('dm.peopleCount', { count: 5 })).toBe('5 osób');
  expect(roomsT('cs')('channel.mentions', { count: 2 })).toBe('2 zmínky');
  expect(roomsT('sl')('thread.replyCount', { count: 2 })).toBe('2 odgovora');
  expect(roomsT('lv')('search.resultCount', { count: 10 })).toBe('10 rezultātu');
  expect(roomsT('lt')('members.count', { count: 12 })).toBe('12 asmenų');
  expect(roomsT('mt')('search.resultCount', { count: 2 })).toBe('2 riżultati');
  expect(roomsT('ga')('thread.replyCount', { count: 1 })).toBe('Freagra amháin');
});

test('las frases con nombre van enteras, no pegadas por trozos', () => {
  // El hueco {name} viaja dentro de la plantilla de cada lengua, así que el orden
  // de las palabras puede cambiar sin tocar el código.
  expect(roomsT('en')('call.started', { name: 'Ana' })).toBe('Ana started a call');
  expect(roomsT('de')('call.started', { name: 'Ana' })).toBe('Ana hat einen Anruf gestartet');
  expect(roomsT('en')('notification.mentionedIn', { name: 'Ana', channel: 'diseño' })).toBe(
    'Ana mentioned you in #diseño',
  );
});
