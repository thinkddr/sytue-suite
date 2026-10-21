/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const sl: AppCatalog<typeof en> = {
  'acceso.boton': 'Kdo lahko poizveduje',
  'acceso.titulo': 'Kdo lahko poizveduje – {nombre}',
  'acceso.ayuda':
    'Poizvedbe se pošiljajo s poverilnicami vaše organizacije. Skrbniki in avtor povezave lahko poizvedujejo vedno; dodajte osebe in skupine, ki to prav tako potrebujejo.',
  'acceso.vacia': 'Nihče drug: poizvedujejo lahko le skrbniki in avtor povezave.',
  'acceso.persona': 'Oseba',
  'acceso.grupo': 'Skupina',
  'acceso.quitar': 'Odstrani {nombre}',
  'acceso.anadirPersona': 'Dodaj osebo',
  'acceso.buscar': 'Ime ali e-pošta',
  'acceso.sinCandidatos': 'Ni zadetkov.',
  'acceso.anadirGrupo': 'Dodaj skupino',
  'acceso.elegirGrupo': 'Izberite skupino',
  'acceso.sinGrupos': 'Vaša organizacija nima skupin.',
  'acceso.anadir': 'Dodaj',
  'acceso.guardar': 'Shrani',
  'acceso.cancelar': 'Prekliči',
  'acceso.guardada': 'Seznam shranjen.',
  'acceso.error': 'Seznama ni bilo mogoče shraniti.',
  'acceso.errorCarga': 'Seznama ni bilo mogoče naložiti.',
  'acceso.cargando': 'Nalaganje …',
};
