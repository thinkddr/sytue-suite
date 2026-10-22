/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const it: AppCatalog<typeof en> = {
  'acceso.boton': 'Chi può interrogare',
  'acceso.titulo': 'Chi può interrogare {nombre}',
  'acceso.ayuda':
    'Le interrogazioni partono con le credenziali della tua organizzazione. Chi amministra e chi ha creato il collegamento può sempre interrogarlo; aggiungi le persone e i gruppi che ne hanno bisogno.',
  'acceso.vacia':
    'Nessun altro: solo chi amministra e chi ha creato il collegamento può interrogarlo.',
  'acceso.persona': 'Persona',
  'acceso.grupo': 'Gruppo',
  'acceso.quitar': 'Rimuovi {nombre}',
  'acceso.anadirPersona': 'Aggiungi una persona',
  'acceso.buscar': 'Nome o email',
  'acceso.sinCandidatos': 'Nessun risultato.',
  'acceso.anadirGrupo': 'Aggiungi un gruppo',
  'acceso.elegirGrupo': 'Scegli un gruppo',
  'acceso.sinGrupos': 'La tua organizzazione non ha gruppi.',
  'acceso.anadir': 'Aggiungi',
  'acceso.guardar': 'Salva',
  'acceso.cancelar': 'Annulla',
  'acceso.guardada': 'Elenco salvato.',
  'acceso.error': 'Impossibile salvare l’elenco.',
  'acceso.errorCarga': 'Impossibile caricare l’elenco.',
  'acceso.cargando': 'Caricamento…',
};
