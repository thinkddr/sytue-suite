/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const pl: AppCatalog<typeof en> = {
  'acceso.boton': 'Kto może wysyłać zapytania',
  'acceso.titulo': 'Kto może wysyłać zapytania – {nombre}',
  'acceso.ayuda':
    'Zapytania są wysyłane z danymi logowania organizacji. Administracja i osoba, która utworzyła połączenie, zawsze mogą wysyłać do niego zapytania; dodaj osoby i grupy, które również tego potrzebują.',
  'acceso.vacia':
    'Nikt więcej: zapytania mogą wysyłać tylko administracja i osoba, która utworzyła połączenie.',
  'acceso.persona': 'Osoba',
  'acceso.grupo': 'Grupa',
  'acceso.quitar': 'Usuń: {nombre}',
  'acceso.anadirPersona': 'Dodaj osobę',
  'acceso.buscar': 'Imię lub e-mail',
  'acceso.sinCandidatos': 'Brak wyników.',
  'acceso.anadirGrupo': 'Dodaj grupę',
  'acceso.elegirGrupo': 'Wybierz grupę',
  'acceso.sinGrupos': 'Organizacja nie ma grup.',
  'acceso.anadir': 'Dodaj',
  'acceso.guardar': 'Zapisz',
  'acceso.cancelar': 'Anuluj',
  'acceso.guardada': 'Lista zapisana.',
  'acceso.error': 'Nie udało się zapisać listy.',
  'acceso.errorCarga': 'Nie udało się wczytać listy.',
  'acceso.cargando': 'Wczytywanie…',
};
