/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const is: AppCatalog<typeof en> = {
  'acceso.boton': 'Hver má senda fyrirspurnir',
  'acceso.titulo': 'Hver má senda fyrirspurnir – {nombre}',
  'acceso.ayuda':
    'Fyrirspurnir fara út með innskráningarupplýsingum stofnunarinnar. Stjórnendur og sá aðili sem stofnaði tenginguna geta alltaf sent fyrirspurnir á hana; bættu við fólki og hópum sem þurfa það líka.',
  'acceso.vacia':
    'Engir aðrir: aðeins stjórnendur og sá aðili sem stofnaði tenginguna geta sent fyrirspurnir á hana.',
  'acceso.persona': 'Einstaklingur',
  'acceso.grupo': 'Hópur',
  'acceso.quitar': 'Fjarlægja {nombre}',
  'acceso.anadirPersona': 'Bæta við einstaklingi',
  'acceso.buscar': 'Nafn eða netfang',
  'acceso.sinCandidatos': 'Engar niðurstöður.',
  'acceso.anadirGrupo': 'Bæta við hópi',
  'acceso.elegirGrupo': 'Veldu hóp',
  'acceso.sinGrupos': 'Stofnunin hefur enga hópa.',
  'acceso.anadir': 'Bæta við',
  'acceso.guardar': 'Vista',
  'acceso.cancelar': 'Hætta við',
  'acceso.guardada': 'Listinn var vistaður.',
  'acceso.error': 'Ekki tókst að vista listann.',
  'acceso.errorCarga': 'Ekki tókst að hlaða listanum.',
  'acceso.cargando': 'Hleður…',
};
