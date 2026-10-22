/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const mt: AppCatalog<typeof en> = {
  'acceso.boton': 'Min jista’ jikkonsulta',
  'acceso.titulo': 'Min jista’ jikkonsulta – {nombre}',
  'acceso.ayuda':
    'Il-queries joħorġu bil-kredenzjali tal-organizzazzjoni tiegħek. L-amministrazzjoni u l-persuna li ħolqot il-konnessjoni dejjem jistgħu jikkonsultawha; żid il-persuni u l-gruppi li jeħtieġu dan ukoll.',
  'acceso.vacia':
    'Ħadd iktar: l-amministrazzjoni u l-persuna li ħolqot il-konnessjoni biss jistgħu jikkonsultawha.',
  'acceso.persona': 'Persuna',
  'acceso.grupo': 'Grupp',
  'acceso.quitar': 'Neħħi {nombre}',
  'acceso.anadirPersona': 'Żid persuna',
  'acceso.buscar': 'Isem jew email',
  'acceso.sinCandidatos': 'L-ebda riżultat.',
  'acceso.anadirGrupo': 'Żid grupp',
  'acceso.elegirGrupo': 'Agħżel grupp',
  'acceso.sinGrupos': 'L-organizzazzjoni tiegħek m’għandha l-ebda grupp.',
  'acceso.anadir': 'Żid',
  'acceso.guardar': 'Issejvja',
  'acceso.cancelar': 'Ikkanċella',
  'acceso.guardada': 'Il-lista ġiet issejvjata.',
  'acceso.error': 'Il-lista ma setgħetx tiġi ssejvjata.',
  'acceso.errorCarga': 'Il-lista ma setgħetx titgħabba.',
  'acceso.cargando': 'Qed jitgħabba…',
};
