/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las herramientas de gestión de personas de Sytue Admin (acceso efectivo,
 * sesiones, bajas recuperables, lotes, export del directorio, dispositivos,
 * auditoría y roles): traducción del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fr: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Ce qu’elle peut faire, et pourquoi',
  'gestion.acceso.ver': 'Afficher',
  'gestion.acceso.error': 'Impossible de charger l’accès effectif.',
  'gestion.acceso.ayuda':
    'Calculé par Sytue ID avec la même règle que celle appliquée pour autoriser. Chaque permission indique sa source ; si vous retirez cette source, vérifiez les autres listées ici.',
  'gestion.acceso.sinPermisos': 'Aucune permission d’administration.',
  'gestion.acceso.fuente.integrado': 'Rôle intégré : {role}',
  'gestion.acceso.fuente.rol': 'Rôle : {name}',
  'gestion.acceso.fuente.elevacion': 'Élevé à {name} jusqu’au {when}',
  'gestion.acceso.pendientes': 'Élévations en attente d’approbation',
  'gestion.acceso.pendiente': '{role}, demandée le {when}',
  'gestion.acceso.grupos': 'Groupes',
  'gestion.acceso.sinGrupos': 'N’appartient à aucun groupe.',
  'gestion.acceso.calculado': 'Calculé le {when}',
  'gestion.sesiones.titulo': 'Sessions ouvertes',
  'gestion.sesiones.ver': 'Afficher',
  'gestion.sesiones.error': 'Impossible de charger les sessions.',
  'gestion.sesiones.vacia': 'Aucune session ouverte.',
  'gestion.sesiones.inicio': 'connexion le {when}',
  'gestion.sesiones.caduca': 'expire le {when}',
  'gestion.sesiones.mfa': 'Second facteur',
  'gestion.sesiones.sinMfa': 'Mot de passe seul',
  'gestion.sesiones.total': 'Sessions ouvertes : {count}',
  'gestion.bajas.titulo': 'Comptes fermés que vous pouvez restaurer',
  'gestion.bajas.ayuda':
    'Un compte fermé conserve ses données {days} jours. La restauration le rend suspendu : réactivez-le dans l’annuaire pour lui rendre l’accès.',
  'gestion.bajas.cerrada': 'fermé le {when}',
  'gestion.bajas.motivo': 'motif : {reason}',
  'gestion.bajas.quedan': 'Jours restants : {count}',
  'gestion.bajas.restaurar': 'Restaurer',
  'gestion.bajas.confirmarTitulo': 'Restaurer {name} ?',
  'gestion.bajas.confirmar':
    'Le compte revient suspendu, avec ses données. Personne ne peut s’y connecter tant que vous ne le réactivez pas.',
  'gestion.bajas.restaurada': '{name} restauré',
  'gestion.bajas.reactivar':
    'Il est suspendu : réactivez-le dans l’annuaire pour lui rendre l’accès.',
  'gestion.bajas.fallo': 'Impossible de restaurer le compte',
  'gestion.directorio.exportar': 'Exporter en CSV',
  'gestion.dispositivos.buscar': 'Rechercher une personne, un appareil ou une IP',
  'gestion.dispositivos.confianza': 'Filtrer par confiance',
  'gestion.dispositivos.todos': 'Tous les appareils',
  'gestion.dispositivos.recuento': '{count} sur {total} affichés',
  'gestion.dispositivos.sinResultados': 'Aucun appareil ne correspond.',
  'gestion.auditoria.rango': 'Période rapide',
  'gestion.auditoria.hoy': 'Aujourd’hui',
  'gestion.auditoria.7d': '7 jours',
  'gestion.auditoria.30d': '30 jours',
  'gestion.roles.duplicar': 'Dupliquer',
  'gestion.roles.copia': '{name} (copie)',
  'gestion.lotes.titulo': 'Actions groupées récentes',
  'gestion.lotes.ayuda':
    'Ce qui a été appliqué à plusieurs personnes à la fois, et à qui cela n’a pas pu l’être.',
  'gestion.lotes.ver': 'Afficher',
  'gestion.lotes.error': 'Impossible de charger les actions groupées.',
  'gestion.lotes.vacio': 'Aucune action groupée pour l’instant.',
  'gestion.lotes.progreso': '{done} sur {total}',
  'gestion.lotes.accion.setRole': 'Changer de rôle',
  'gestion.lotes.estado.ejecutando': 'En cours',
  'gestion.lotes.estado.terminado': 'Terminé',
  'gestion.lotes.estado.cancelado': 'Annulé',
  'gestion.lotes.estado.interrumpido': 'Interrompu',
  'gestion.lotes.detalle': 'Détails',
  'gestion.lotes.todosBien': 'Appliqué à tout le monde.',
  'gestion.lote.fallos': 'Non appliqué à',
  'gestion.lote.cerrar': 'Fermer',
  'gestion.lote.codigo.not_found': 'n’est pas dans cette organisation',
  'gestion.lote.codigo.self': 'c’est vous',
  'gestion.lote.codigo.forbidden':
    'modifier un compte propriétaire demande des droits de propriété',
  'gestion.lote.codigo.deleted': 'le compte est fermé',
  'gestion.lote.codigo.last_owner': 'dernier propriétaire de l’organisation',
  'gestion.lote.codigo.last_admin': 'dernier administrateur de l’organisation',
  'gestion.lote.codigo.error': 'erreur inattendue ; réessayez',
  'gestion.lote.seleccionarVisibles': 'Tout sélectionner à l’écran ({count})',
  'gestion.lote.recortado':
    'Une action groupée porte sur {max} personnes au maximum : les {max} premières visibles sont sélectionnées.',
};
