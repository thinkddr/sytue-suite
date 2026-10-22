/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const nb: AppCatalog<typeof en> = {
  'acceso.boton': 'Hvem kan spørre',
  'acceso.titulo': 'Hvem kan spørre mot {nombre}',
  'acceso.ayuda':
    'Spørringene sendes med organisasjonens påloggingsinformasjon. Administratorer og den som opprettet tilkoblingen, kan alltid spørre mot den; legg til personene og gruppene som også trenger det.',
  'acceso.vacia':
    'Ingen andre: bare administratorer og den som opprettet tilkoblingen, kan spørre mot den.',
  'acceso.persona': 'Person',
  'acceso.grupo': 'Gruppe',
  'acceso.quitar': 'Fjern {nombre}',
  'acceso.anadirPersona': 'Legg til en person',
  'acceso.buscar': 'Navn eller e-post',
  'acceso.sinCandidatos': 'Ingen treff.',
  'acceso.anadirGrupo': 'Legg til en gruppe',
  'acceso.elegirGrupo': 'Velg en gruppe',
  'acceso.sinGrupos': 'Organisasjonen har ingen grupper.',
  'acceso.anadir': 'Legg til',
  'acceso.guardar': 'Lagre',
  'acceso.cancelar': 'Avbryt',
  'acceso.guardada': 'Listen er lagret.',
  'acceso.error': 'Kunne ikke lagre listen.',
  'acceso.errorCarga': 'Kunne ikke laste inn listen.',
  'acceso.cargando': 'Laster inn …',
};
