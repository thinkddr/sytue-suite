/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06), en INGLÉS: la fuente de
 * verdad. Los otros 25 derivan su tipo de aquí, así que una clave nueva rompe su
 * compilación hasta que se traduzca.
 */
import type { MessageEntry } from '@sytue/i18n';

export const en = {
  'acceso.boton': 'Who can query',
  'acceso.titulo': 'Who can query {nombre}',
  'acceso.ayuda':
    "Queries go out with your organisation's credentials. Administrators and the person who created the connection can always query it; add the people and groups who also need to.",
  'acceso.vacia':
    'Nobody else: only administrators and the person who created the connection can query it.',
  'acceso.persona': 'Person',
  'acceso.grupo': 'Group',
  'acceso.quitar': 'Remove {nombre}',
  'acceso.anadirPersona': 'Add a person',
  'acceso.buscar': 'Name or email',
  'acceso.sinCandidatos': 'No matches.',
  'acceso.anadirGrupo': 'Add a group',
  'acceso.elegirGrupo': 'Choose a group',
  'acceso.sinGrupos': 'Your organisation has no groups.',
  'acceso.anadir': 'Add',
  'acceso.guardar': 'Save',
  'acceso.cancelar': 'Cancel',
  'acceso.guardada': 'List saved.',
  'acceso.error': 'The list could not be saved.',
  'acceso.errorCarga': 'The list could not be loaded.',
  'acceso.cargando': 'Loading…',
} satisfies Record<string, MessageEntry>;
