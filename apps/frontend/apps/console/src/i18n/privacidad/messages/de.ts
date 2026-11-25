/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de solicitudes RGPD y la supresión física de una cuenta: traducción
 * del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  'privacidad.titulo': 'Datenschutzanfragen',
  'privacidad.descripcion':
    'Was Personen von Ihrer Organisation in Bezug auf ihre personenbezogenen Daten verlangen (DSGVO, Artikel 15 bis 21). Erfassen Sie jede Anfrage, antworten Sie innerhalb eines Monats und führen Sie eine Löschung hier aus.',
  'privacidad.registrar': 'Anfrage erfassen',
  'privacidad.filtro': 'Anzeigen',
  'privacidad.filtro.abiertas': 'Offen',
  'privacidad.filtro.vencidas': 'Überfällig',
  'privacidad.filtro.todas': 'Alle',
  'privacidad.resumen.abiertas': 'Offene Anfragen',
  'privacidad.resumen.vencidas': 'Frist überschritten',
  'privacidad.vacia': 'Keine Anfragen vorhanden.',
  'privacidad.errorCarga': 'Das Verzeichnis konnte nicht geladen werden.',
  'privacidad.col.persona': 'Person',
  'privacidad.col.tipo': 'Anfrage',
  'privacidad.col.recibida': 'Eingegangen',
  'privacidad.col.plazo': 'Frist',
  'privacidad.col.estado': 'Status',
  'privacidad.tipo.access': 'Auskunft über ihre Daten',
  'privacidad.tipo.rectification': 'Berichtigung',
  'privacidad.tipo.erasure': 'Löschung',
  'privacidad.tipo.restriction': 'Einschränkung der Verarbeitung',
  'privacidad.tipo.portability': 'Datenübertragbarkeit',
  'privacidad.tipo.objection': 'Widerspruch',
  'privacidad.canal.email': 'E-Mail',
  'privacidad.canal.web': 'Webformular',
  'privacidad.canal.letter': 'Brief',
  'privacidad.canal.phone': 'Telefon',
  'privacidad.canal.in_person': 'Persönlich',
  'privacidad.canal.other': 'Sonstiges',
  'privacidad.estado.received': 'Eingegangen',
  'privacidad.estado.in_progress': 'In Bearbeitung',
  'privacidad.estado.completed': 'Beantwortet',
  'privacidad.estado.rejected': 'Abgelehnt',
  'privacidad.plazo.vencida': 'Überfällig',
  'privacidad.plazo.pronto': 'Bald fällig',
  'privacidad.plazo.prorrogada': 'Verlängert',
  'privacidad.alta.quien': 'Wer fragt an',
  'privacidad.alta.miembro': 'Jemand aus der Organisation',
  'privacidad.alta.fuera': 'Jemand von außerhalb',
  'privacidad.alta.elegirMiembro': 'Person auswählen',
  'privacidad.alta.correo': 'Ihre E-Mail-Adresse',
  'privacidad.alta.nombre': 'Ihr Name (optional)',
  'privacidad.alta.tipo': 'Was verlangt wird',
  'privacidad.alta.canal': 'Wie sie eingegangen ist',
  'privacidad.alta.recibida': 'Eingangsdatum',
  'privacidad.alta.recibidaAyuda': 'Die Monatsfrist läuft ab diesem Datum, nicht ab heute.',
  'privacidad.alta.identidad': 'Identität geprüft',
  'privacidad.alta.metodo': 'Wie sie geprüft wurde',
  'privacidad.alta.notas': 'Notizen',
  'privacidad.alta.error.subject': 'Geben Sie an, wer die Anfrage stellt.',
  'privacidad.alta.error.email': 'Geben Sie eine gültige E-Mail-Adresse ein.',
  'privacidad.alta.error.date':
    'Das Datum muss im letzten Jahr liegen und darf nicht in der Zukunft liegen.',
  'privacidad.alta.error.upstream': 'Die Anfrage konnte nicht erfasst werden.',
  'privacidad.alta.hecha': 'Anfrage erfasst.',
  'privacidad.detalle.plazo': 'Antworten bis {fecha}',
  'privacidad.detalle.resolucion': 'Antwort an die Person',
  'privacidad.detalle.resolucionAyuda':
    'Zum Abschließen der Anfrage erforderlich: Das wird der Person mitgeteilt.',
  'privacidad.detalle.guardar': 'Speichern',
  'privacidad.detalle.guardado': 'Anfrage aktualisiert.',
  'privacidad.detalle.cerrada':
    'Diese Anfrage ist abgeschlossen und kann nicht mehr geändert werden.',
  'privacidad.detalle.prorroga': 'Um zwei Monate verlängern',
  'privacidad.detalle.prorrogaAyuda':
    'Nur einmal und vor Fristablauf. Der Person muss der Grund mitgeteilt werden: Schreiben Sie ihn hier.',
  'privacidad.detalle.prorrogar': 'Verlängern',
  'privacidad.detalle.historia': 'Verlauf',
  'privacidad.evento.created': 'Erfasst',
  'privacidad.evento.updated': 'Aktualisiert',
  'privacidad.evento.extended': 'Frist verlängert',
  'privacidad.evento.erasureStarted': 'Löschung gestartet',
  'privacidad.evento.erasureCompleted': 'Löschung abgeschlossen',
  'privacidad.supresion.titulo': 'Konto löschen',
  'privacidad.supresion.explica':
    'Das Konto wird sofort und unwiderruflich geschlossen, und seine Anmeldemethoden werden gelöscht. Innerhalb weniger Minuten wird sein Profil anonymisiert und gelöscht, was andere Dienste über die Person speichern. Was die Person für die Organisation erstellt hat (Postfach, Dateien, Termine), bleibt bei der Organisation.',
  'privacidad.supresion.confirmar': 'Geben Sie {correo} zur Bestätigung ein',
  'privacidad.supresion.iniciada': 'Löschung gestartet. Sie ist in wenigen Minuten abgeschlossen.',
  'privacidad.supresion.enCurso': 'Die Löschung läuft.',
  'privacidad.supresion.hecha': 'Das Konto wurde gelöscht.',
  'privacidad.supresion.sinCuenta':
    'Die Person hat kein Konto in der Organisation: Hier gibt es kein Konto zu löschen.',
  'privacidad.supresion.noMiembro': 'Die Person ist nicht mehr Mitglied der Organisation.',
  'privacidad.supresion.error': 'Das Konto konnte nicht gelöscht werden.',
  'privacidad.error.upstream': 'Die Änderung konnte nicht gespeichert werden.',
  'privacidad.cancelar': 'Abbrechen',
};
