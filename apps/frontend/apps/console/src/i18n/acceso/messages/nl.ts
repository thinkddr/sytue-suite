/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const nl: AppCatalog<typeof en> = {
  'acceso.boton': 'Wie mag raadplegen',
  'acceso.titulo': 'Wie {nombre} mag raadplegen',
  'acceso.ayuda':
    'Raadplegingen gebeuren met de inloggegevens van je organisatie. Beheerders en wie de koppeling heeft gemaakt, kunnen haar altijd raadplegen; voeg de mensen en groepen toe die dat ook nodig hebben.',
  'acceso.vacia':
    'Verder niemand: alleen beheerders en wie de koppeling heeft gemaakt, kunnen haar raadplegen.',
  'acceso.persona': 'Persoon',
  'acceso.grupo': 'Groep',
  'acceso.quitar': '{nombre} verwijderen',
  'acceso.anadirPersona': 'Persoon toevoegen',
  'acceso.buscar': 'Naam of e-mailadres',
  'acceso.sinCandidatos': 'Geen resultaten.',
  'acceso.anadirGrupo': 'Groep toevoegen',
  'acceso.elegirGrupo': 'Kies een groep',
  'acceso.sinGrupos': 'Je organisatie heeft geen groepen.',
  'acceso.anadir': 'Toevoegen',
  'acceso.guardar': 'Opslaan',
  'acceso.cancelar': 'Annuleren',
  'acceso.guardada': 'Lijst opgeslagen.',
  'acceso.error': 'De lijst kon niet worden opgeslagen.',
  'acceso.errorCarga': 'De lijst kon niet worden geladen.',
  'acceso.cargando': 'Laden…',
};
