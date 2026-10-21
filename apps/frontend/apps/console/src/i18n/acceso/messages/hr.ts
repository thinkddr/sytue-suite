/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const hr: AppCatalog<typeof en> = {
  'acceso.boton': 'Tko smije slati upite',
  'acceso.titulo': 'Tko smije slati upite – {nombre}',
  'acceso.ayuda':
    'Upiti se šalju s vjerodajnicama vaše organizacije. Administratori i autor veze uvijek mogu slati upite; dodajte osobe i grupe kojima je to također potrebno.',
  'acceso.vacia': 'Nitko drugi: upite mogu slati samo administratori i autor veze.',
  'acceso.persona': 'Osoba',
  'acceso.grupo': 'Grupa',
  'acceso.quitar': 'Ukloni {nombre}',
  'acceso.anadirPersona': 'Dodaj osobu',
  'acceso.buscar': 'Ime ili e-pošta',
  'acceso.sinCandidatos': 'Nema rezultata.',
  'acceso.anadirGrupo': 'Dodaj grupu',
  'acceso.elegirGrupo': 'Odaberite grupu',
  'acceso.sinGrupos': 'Vaša organizacija nema grupa.',
  'acceso.anadir': 'Dodaj',
  'acceso.guardar': 'Spremi',
  'acceso.cancelar': 'Odustani',
  'acceso.guardada': 'Popis spremljen.',
  'acceso.error': 'Popis nije moguće spremiti.',
  'acceso.errorCarga': 'Popis nije moguće učitati.',
  'acceso.cargando': 'Učitavanje…',
};
