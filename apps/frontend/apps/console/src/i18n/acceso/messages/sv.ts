/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const sv: AppCatalog<typeof en> = {
  'acceso.boton': 'Vem kan fråga',
  'acceso.titulo': 'Vem kan fråga {nombre}',
  'acceso.ayuda':
    'Frågorna skickas med organisationens inloggningsuppgifter. Administratörer och den som skapade anslutningen kan alltid fråga den; lägg till de personer och grupper som också behöver det.',
  'acceso.vacia':
    'Ingen annan: bara administratörer och den som skapade anslutningen kan fråga den.',
  'acceso.persona': 'Person',
  'acceso.grupo': 'Grupp',
  'acceso.quitar': 'Ta bort {nombre}',
  'acceso.anadirPersona': 'Lägg till en person',
  'acceso.buscar': 'Namn eller e-post',
  'acceso.sinCandidatos': 'Inga träffar.',
  'acceso.anadirGrupo': 'Lägg till en grupp',
  'acceso.elegirGrupo': 'Välj en grupp',
  'acceso.sinGrupos': 'Organisationen har inga grupper.',
  'acceso.anadir': 'Lägg till',
  'acceso.guardar': 'Spara',
  'acceso.cancelar': 'Avbryt',
  'acceso.guardada': 'Listan har sparats.',
  'acceso.error': 'Det gick inte att spara listan.',
  'acceso.errorCarga': 'Det gick inte att läsa in listan.',
  'acceso.cargando': 'Läser in…',
};
