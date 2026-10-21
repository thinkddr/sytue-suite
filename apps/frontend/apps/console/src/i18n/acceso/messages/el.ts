/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const el: AppCatalog<typeof en> = {
  'acceso.boton': 'Ποιος μπορεί να υποβάλλει ερωτήματα',
  'acceso.titulo': 'Ποιος μπορεί να υποβάλλει ερωτήματα – {nombre}',
  'acceso.ayuda':
    'Τα ερωτήματα στέλνονται με τα διαπιστευτήρια του οργανισμού σας. Η διαχείριση και το άτομο που δημιούργησε τη σύνδεση μπορούν πάντα να υποβάλλουν ερωτήματα· προσθέστε τα άτομα και τις ομάδες που το χρειάζονται επίσης.',
  'acceso.vacia':
    'Κανείς άλλος: ερωτήματα μπορούν να υποβάλλουν μόνο η διαχείριση και το άτομο που δημιούργησε τη σύνδεση.',
  'acceso.persona': 'Άτομο',
  'acceso.grupo': 'Ομάδα',
  'acceso.quitar': 'Αφαίρεση {nombre}',
  'acceso.anadirPersona': 'Προσθήκη ατόμου',
  'acceso.buscar': 'Όνομα ή email',
  'acceso.sinCandidatos': 'Κανένα αποτέλεσμα.',
  'acceso.anadirGrupo': 'Προσθήκη ομάδας',
  'acceso.elegirGrupo': 'Επιλέξτε ομάδα',
  'acceso.sinGrupos': 'Ο οργανισμός σας δεν έχει ομάδες.',
  'acceso.anadir': 'Προσθήκη',
  'acceso.guardar': 'Αποθήκευση',
  'acceso.cancelar': 'Ακύρωση',
  'acceso.guardada': 'Η λίστα αποθηκεύτηκε.',
  'acceso.error': 'Δεν ήταν δυνατή η αποθήκευση της λίστας.',
  'acceso.errorCarga': 'Δεν ήταν δυνατή η φόρτωση της λίστας.',
  'acceso.cargando': 'Φόρτωση…',
};
