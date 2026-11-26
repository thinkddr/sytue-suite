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

export const de: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Was die Person darf und warum',
  'gestion.acceso.ver': 'Anzeigen',
  'gestion.acceso.error': 'Der effektive Zugriff konnte nicht geladen werden.',
  'gestion.acceso.ayuda':
    'Berechnet von Sytue ID nach derselben Regel, mit der es autorisiert. Jede Berechtigung zeigt ihre Quelle; entfernst du diese, prüfe die übrigen hier aufgeführten.',
  'gestion.acceso.sinPermisos': 'Keine Administrationsberechtigungen.',
  'gestion.acceso.fuente.integrado': 'Integrierte Rolle: {role}',
  'gestion.acceso.fuente.rol': 'Rolle: {name}',
  'gestion.acceso.fuente.elevacion': 'Erhöht auf {name} bis {when}',
  'gestion.acceso.pendientes': 'Erhöhungen, die auf Genehmigung warten',
  'gestion.acceso.pendiente': '{role}, beantragt am {when}',
  'gestion.acceso.grupos': 'Gruppen',
  'gestion.acceso.sinGrupos': 'In keiner Gruppe.',
  'gestion.acceso.calculado': 'Berechnet am {when}',
  'gestion.sesiones.titulo': 'Offene Sitzungen',
  'gestion.sesiones.ver': 'Anzeigen',
  'gestion.sesiones.error': 'Die Sitzungen konnten nicht geladen werden.',
  'gestion.sesiones.vacia': 'Keine offenen Sitzungen.',
  'gestion.sesiones.inicio': 'angemeldet am {when}',
  'gestion.sesiones.caduca': 'läuft ab am {when}',
  'gestion.sesiones.mfa': 'Zweiter Faktor',
  'gestion.sesiones.sinMfa': 'Nur Passwort',
  'gestion.sesiones.total': 'Offene Sitzungen: {count}',
  'gestion.bajas.titulo': 'Geschlossene Konten, die du wiederherstellen kannst',
  'gestion.bajas.ayuda':
    'Ein geschlossenes Konto behält seine Daten {days} Tage. Wiederherstellen bringt es gesperrt zurück: Reaktiviere es im Verzeichnis, um den Zugriff zurückzugeben.',
  'gestion.bajas.cerrada': 'geschlossen am {when}',
  'gestion.bajas.motivo': 'Grund: {reason}',
  'gestion.bajas.quedan': 'Verbleibende Tage: {count}',
  'gestion.bajas.restaurar': 'Wiederherstellen',
  'gestion.bajas.confirmarTitulo': '{name} wiederherstellen?',
  'gestion.bajas.confirmar':
    'Das Konto kommt gesperrt und mit seinen Daten zurück. Niemand kann sich damit anmelden, bis du es reaktivierst.',
  'gestion.bajas.restaurada': '{name} wiederhergestellt',
  'gestion.bajas.reactivar':
    'Es ist gesperrt: Reaktiviere es im Verzeichnis, um den Zugriff zurückzugeben.',
  'gestion.bajas.fallo': 'Das Konto konnte nicht wiederhergestellt werden',
  'gestion.directorio.exportar': 'CSV exportieren',
  'gestion.dispositivos.buscar': 'Person, Gerät oder IP suchen',
  'gestion.dispositivos.confianza': 'Nach Vertrauen filtern',
  'gestion.dispositivos.todos': 'Alle Geräte',
  'gestion.dispositivos.recuento': '{count} von {total} angezeigt',
  'gestion.dispositivos.sinResultados': 'Kein Gerät passt.',
  'gestion.auditoria.rango': 'Schnellauswahl Zeitraum',
  'gestion.auditoria.hoy': 'Heute',
  'gestion.auditoria.7d': '7 Tage',
  'gestion.auditoria.30d': '30 Tage',
  'gestion.roles.duplicar': 'Duplizieren',
  'gestion.roles.copia': '{name} (Kopie)',
  'gestion.lotes.titulo': 'Letzte Massenaktionen',
  'gestion.lotes.ayuda':
    'Was auf mehrere Personen gleichzeitig angewendet wurde und bei wem es nicht ging.',
  'gestion.lotes.ver': 'Anzeigen',
  'gestion.lotes.error': 'Die Massenaktionen konnten nicht geladen werden.',
  'gestion.lotes.vacio': 'Noch keine Massenaktionen.',
  'gestion.lotes.progreso': '{done} von {total}',
  'gestion.lotes.accion.setRole': 'Rolle ändern',
  'gestion.lotes.estado.ejecutando': 'Läuft',
  'gestion.lotes.estado.terminado': 'Abgeschlossen',
  'gestion.lotes.estado.cancelado': 'Abgebrochen',
  'gestion.lotes.estado.interrumpido': 'Unterbrochen',
  'gestion.lotes.detalle': 'Details',
  'gestion.lotes.todosBien': 'Auf alle angewendet.',
  'gestion.lote.fallos': 'Nicht angewendet bei',
  'gestion.lote.cerrar': 'Schließen',
  'gestion.lote.codigo.not_found': 'nicht in dieser Organisation',
  'gestion.lote.codigo.self': 'das bist du',
  'gestion.lote.codigo.forbidden': 'ein Eigentümerkonto ändern erfordert Eigentumsrechte',
  'gestion.lote.codigo.deleted': 'das Konto ist geschlossen',
  'gestion.lote.codigo.last_owner': 'letzter Eigentümer der Organisation',
  'gestion.lote.codigo.last_admin': 'letzter Administrator der Organisation',
  'gestion.lote.codigo.error': 'unerwarteter Fehler; erneut versuchen',
  'gestion.lote.seleccionarVisibles': 'Alle sichtbaren auswählen ({count})',
  'gestion.lote.recortado':
    'Eine Massenaktion umfasst höchstens {max} Personen: Die ersten {max} sichtbaren sind ausgewählt.',
};
