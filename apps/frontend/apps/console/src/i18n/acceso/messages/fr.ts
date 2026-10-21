/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fr: AppCatalog<typeof en> = {
  'acceso.boton': 'Qui peut interroger',
  'acceso.titulo': 'Qui peut interroger {nombre}',
  'acceso.ayuda':
    'Les requêtes partent avec les identifiants de votre organisation. L’administration et la personne qui a créé la connexion peuvent toujours l’interroger ; ajoutez les personnes et les groupes qui en ont aussi besoin.',
  'acceso.vacia':
    'Personne d’autre : seules l’administration et la personne qui a créé la connexion peuvent l’interroger.',
  'acceso.persona': 'Personne',
  'acceso.grupo': 'Groupe',
  'acceso.quitar': 'Retirer {nombre}',
  'acceso.anadirPersona': 'Ajouter une personne',
  'acceso.buscar': 'Nom ou adresse e-mail',
  'acceso.sinCandidatos': 'Aucun résultat.',
  'acceso.anadirGrupo': 'Ajouter un groupe',
  'acceso.elegirGrupo': 'Choisissez un groupe',
  'acceso.sinGrupos': 'Votre organisation n’a aucun groupe.',
  'acceso.anadir': 'Ajouter',
  'acceso.guardar': 'Enregistrer',
  'acceso.cancelar': 'Annuler',
  'acceso.guardada': 'Liste enregistrée.',
  'acceso.error': 'Impossible d’enregistrer la liste.',
  'acceso.errorCarga': 'Impossible de charger la liste.',
  'acceso.cargando': 'Chargement…',
};
