/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fi: AppCatalog<typeof en> = {
  'acceso.boton': 'Kuka voi kysellä',
  'acceso.titulo': 'Kuka voi kysellä – {nombre}',
  'acceso.ayuda':
    'Kyselyt lähtevät organisaatiosi tunnuksilla. Ylläpito ja yhteyden luoja voivat aina kysellä sitä; lisää henkilöt ja ryhmät, jotka myös tarvitsevat sitä.',
  'acceso.vacia': 'Ei muita: vain ylläpito ja yhteyden luoja voivat kysellä sitä.',
  'acceso.persona': 'Henkilö',
  'acceso.grupo': 'Ryhmä',
  'acceso.quitar': 'Poista {nombre}',
  'acceso.anadirPersona': 'Lisää henkilö',
  'acceso.buscar': 'Nimi tai sähköposti',
  'acceso.sinCandidatos': 'Ei tuloksia.',
  'acceso.anadirGrupo': 'Lisää ryhmä',
  'acceso.elegirGrupo': 'Valitse ryhmä',
  'acceso.sinGrupos': 'Organisaatiollasi ei ole ryhmiä.',
  'acceso.anadir': 'Lisää',
  'acceso.guardar': 'Tallenna',
  'acceso.cancelar': 'Peruuta',
  'acceso.guardada': 'Luettelo tallennettu.',
  'acceso.error': 'Luettelon tallennus epäonnistui.',
  'acceso.errorCarga': 'Luettelon lataus epäonnistui.',
  'acceso.cargando': 'Ladataan…',
};
