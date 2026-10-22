/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const da: AppCatalog<typeof en> = {
  'acceso.boton': 'Hvem må forespørge',
  'acceso.titulo': 'Hvem må forespørge {nombre}',
  'acceso.ayuda':
    'Forespørgsler sendes med organisationens loginoplysninger. Administratorer og den, der oprettede forbindelsen, kan altid forespørge den; tilføj de personer og grupper, der også har brug for det.',
  'acceso.vacia':
    'Ingen andre: kun administratorer og den, der oprettede forbindelsen, kan forespørge den.',
  'acceso.persona': 'Person',
  'acceso.grupo': 'Gruppe',
  'acceso.quitar': 'Fjern {nombre}',
  'acceso.anadirPersona': 'Tilføj en person',
  'acceso.buscar': 'Navn eller e-mail',
  'acceso.sinCandidatos': 'Ingen resultater.',
  'acceso.anadirGrupo': 'Tilføj en gruppe',
  'acceso.elegirGrupo': 'Vælg en gruppe',
  'acceso.sinGrupos': 'Organisationen har ingen grupper.',
  'acceso.anadir': 'Tilføj',
  'acceso.guardar': 'Gem',
  'acceso.cancelar': 'Annuller',
  'acceso.guardada': 'Listen er gemt.',
  'acceso.error': 'Listen kunne ikke gemmes.',
  'acceso.errorCarga': 'Listen kunne ikke indlæses.',
  'acceso.cargando': 'Indlæser…',
};
