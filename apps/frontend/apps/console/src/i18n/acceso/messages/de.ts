/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  'acceso.boton': 'Wer abfragen darf',
  'acceso.titulo': 'Wer {nombre} abfragen darf',
  'acceso.ayuda':
    'Abfragen laufen mit den Zugangsdaten Ihrer Organisation. Die Administration und die Person, die die Verbindung angelegt hat, dürfen sie immer abfragen; fügen Sie die Personen und Gruppen hinzu, die das ebenfalls brauchen.',
  'acceso.vacia':
    'Sonst niemand: Nur die Administration und die Person, die die Verbindung angelegt hat, dürfen sie abfragen.',
  'acceso.persona': 'Person',
  'acceso.grupo': 'Gruppe',
  'acceso.quitar': '{nombre} entfernen',
  'acceso.anadirPersona': 'Person hinzufügen',
  'acceso.buscar': 'Name oder E-Mail-Adresse',
  'acceso.sinCandidatos': 'Keine Treffer.',
  'acceso.anadirGrupo': 'Gruppe hinzufügen',
  'acceso.elegirGrupo': 'Gruppe auswählen',
  'acceso.sinGrupos': 'Ihre Organisation hat keine Gruppen.',
  'acceso.anadir': 'Hinzufügen',
  'acceso.guardar': 'Speichern',
  'acceso.cancelar': 'Abbrechen',
  'acceso.guardada': 'Liste gespeichert.',
  'acceso.error': 'Die Liste konnte nicht gespeichert werden.',
  'acceso.errorCarga': 'Die Liste konnte nicht geladen werden.',
  'acceso.cargando': 'Wird geladen …',
};
