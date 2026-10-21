/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const ro: AppCatalog<typeof en> = {
  'acceso.boton': 'Cine poate interoga',
  'acceso.titulo': 'Cine poate interoga {nombre}',
  'acceso.ayuda':
    'Interogările pleacă cu datele de autentificare ale organizației dumneavoastră. Administrația și persoana care a creat conexiunea o pot interoga oricând; adăugați persoanele și grupurile care au nevoie de asta.',
  'acceso.vacia':
    'Nimeni altcineva: doar administrația și persoana care a creat conexiunea o pot interoga.',
  'acceso.persona': 'Persoană',
  'acceso.grupo': 'Grup',
  'acceso.quitar': 'Elimină {nombre}',
  'acceso.anadirPersona': 'Adaugă o persoană',
  'acceso.buscar': 'Nume sau e-mail',
  'acceso.sinCandidatos': 'Niciun rezultat.',
  'acceso.anadirGrupo': 'Adaugă un grup',
  'acceso.elegirGrupo': 'Alegeți un grup',
  'acceso.sinGrupos': 'Organizația dumneavoastră nu are grupuri.',
  'acceso.anadir': 'Adaugă',
  'acceso.guardar': 'Salvează',
  'acceso.cancelar': 'Anulează',
  'acceso.guardada': 'Lista a fost salvată.',
  'acceso.error': 'Lista nu a putut fi salvată.',
  'acceso.errorCarga': 'Lista nu a putut fi încărcată.',
  'acceso.cargando': 'Se încarcă…',
};
