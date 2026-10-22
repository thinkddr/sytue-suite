/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const hu: AppCatalog<typeof en> = {
  'acceso.boton': 'Ki kérdezhet le',
  'acceso.titulo': 'Ki kérdezheti le – {nombre}',
  'acceso.ayuda':
    'A lekérdezések a szervezete hitelesítő adataival mennek ki. Az adminisztráció és a kapcsolat létrehozója mindig lekérdezheti; adja hozzá azokat a személyeket és csoportokat, akiknek szintén szükségük van rá.',
  'acceso.vacia': 'Senki más: csak az adminisztráció és a kapcsolat létrehozója kérdezheti le.',
  'acceso.persona': 'Személy',
  'acceso.grupo': 'Csoport',
  'acceso.quitar': '{nombre} eltávolítása',
  'acceso.anadirPersona': 'Személy hozzáadása',
  'acceso.buscar': 'Név vagy e-mail',
  'acceso.sinCandidatos': 'Nincs találat.',
  'acceso.anadirGrupo': 'Csoport hozzáadása',
  'acceso.elegirGrupo': 'Válasszon csoportot',
  'acceso.sinGrupos': 'A szervezetének nincsenek csoportjai.',
  'acceso.anadir': 'Hozzáadás',
  'acceso.guardar': 'Mentés',
  'acceso.cancelar': 'Mégse',
  'acceso.guardada': 'A lista mentve.',
  'acceso.error': 'A listát nem sikerült menteni.',
  'acceso.errorCarga': 'A listát nem sikerült betölteni.',
  'acceso.cargando': 'Betöltés…',
};
