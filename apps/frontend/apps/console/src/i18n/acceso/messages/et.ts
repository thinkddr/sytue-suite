/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const et: AppCatalog<typeof en> = {
  'acceso.boton': 'Kes saavad päringuid teha',
  'acceso.titulo': 'Kes saavad päringuid teha – {nombre}',
  'acceso.ayuda':
    'Päringud saadetakse teie organisatsiooni volitustega. Haldajad ja ühenduse looja saavad alati päringuid teha; lisage inimesed ja rühmad, kes seda samuti vajavad.',
  'acceso.vacia': 'Mitte keegi teine: päringuid saavad teha ainult haldajad ja ühenduse looja.',
  'acceso.persona': 'Isik',
  'acceso.grupo': 'Rühm',
  'acceso.quitar': 'Eemalda {nombre}',
  'acceso.anadirPersona': 'Lisa isik',
  'acceso.buscar': 'Nimi või e-post',
  'acceso.sinCandidatos': 'Tulemusi pole.',
  'acceso.anadirGrupo': 'Lisa rühm',
  'acceso.elegirGrupo': 'Valige rühm',
  'acceso.sinGrupos': 'Teie organisatsioonil pole rühmi.',
  'acceso.anadir': 'Lisa',
  'acceso.guardar': 'Salvesta',
  'acceso.cancelar': 'Loobu',
  'acceso.guardada': 'Loend salvestatud.',
  'acceso.error': 'Loendit ei õnnestunud salvestada.',
  'acceso.errorCarga': 'Loendit ei õnnestunud laadida.',
  'acceso.cargando': 'Laadimine…',
};
