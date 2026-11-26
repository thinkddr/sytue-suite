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

export const el: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Τι μπορεί να κάνει και γιατί',
  'gestion.acceso.ver': 'Εμφάνιση',
  'gestion.acceso.error': 'Δεν ήταν δυνατή η φόρτωση της πραγματικής πρόσβασης.',
  'gestion.acceso.ayuda':
    'Υπολογίζεται από το Sytue ID με τον ίδιο κανόνα που χρησιμοποιεί για την εξουσιοδότηση. Κάθε δικαίωμα δείχνει από πού προέρχεται· αν αφαιρέσετε αυτή την πηγή, ελέγξτε τις υπόλοιπες που αναφέρονται εδώ.',
  'gestion.acceso.sinPermisos': 'Χωρίς δικαιώματα διαχείρισης.',
  'gestion.acceso.fuente.integrado': 'Ενσωματωμένος ρόλος: {role}',
  'gestion.acceso.fuente.rol': 'Ρόλος: {name}',
  'gestion.acceso.fuente.elevacion': 'Αναβάθμιση σε {name} έως {when}',
  'gestion.acceso.pendientes': 'Αναβαθμίσεις που περιμένουν έγκριση',
  'gestion.acceso.pendiente': '{role}, ζητήθηκε στις {when}',
  'gestion.acceso.grupos': 'Ομάδες',
  'gestion.acceso.sinGrupos': 'Δεν ανήκει σε καμία ομάδα.',
  'gestion.acceso.calculado': 'Υπολογίστηκε στις {when}',
  'gestion.sesiones.titulo': 'Ανοιχτές συνεδρίες',
  'gestion.sesiones.ver': 'Εμφάνιση',
  'gestion.sesiones.error': 'Δεν ήταν δυνατή η φόρτωση των συνεδριών.',
  'gestion.sesiones.vacia': 'Δεν υπάρχουν ανοιχτές συνεδρίες.',
  'gestion.sesiones.inicio': 'σύνδεση στις {when}',
  'gestion.sesiones.caduca': 'λήγει στις {when}',
  'gestion.sesiones.mfa': 'Δεύτερος παράγοντας',
  'gestion.sesiones.sinMfa': 'Μόνο κωδικός',
  'gestion.sesiones.total': 'Ανοιχτές συνεδρίες: {count}',
  'gestion.bajas.titulo': 'Κλειστοί λογαριασμοί που μπορείτε να επαναφέρετε',
  'gestion.bajas.ayuda':
    'Ένας κλειστός λογαριασμός διατηρεί τα δεδομένα του για {days} ημέρες. Η επαναφορά τον επιστρέφει σε αναστολή: ενεργοποιήστε τον ξανά στον κατάλογο για να επιστρέψει η πρόσβαση.',
  'gestion.bajas.cerrada': 'έκλεισε στις {when}',
  'gestion.bajas.motivo': 'αιτία: {reason}',
  'gestion.bajas.quedan': 'Ημέρες που απομένουν: {count}',
  'gestion.bajas.restaurar': 'Επαναφορά',
  'gestion.bajas.confirmarTitulo': 'Επαναφορά του {name};',
  'gestion.bajas.confirmar':
    'Ο λογαριασμός επιστρέφει σε αναστολή, με τα δεδομένα του. Κανείς δεν μπορεί να συνδεθεί μέχρι να τον ενεργοποιήσετε ξανά.',
  'gestion.bajas.restaurada': 'Ο λογαριασμός {name} επαναφέρθηκε',
  'gestion.bajas.reactivar':
    'Είναι σε αναστολή: ενεργοποιήστε τον ξανά στον κατάλογο για να επιστρέψει η πρόσβαση.',
  'gestion.bajas.fallo': 'Δεν ήταν δυνατή η επαναφορά του λογαριασμού',
  'gestion.directorio.exportar': 'Εξαγωγή CSV',
  'gestion.dispositivos.buscar': 'Αναζήτηση ατόμου, συσκευής ή IP',
  'gestion.dispositivos.confianza': 'Φιλτράρισμα κατά εμπιστοσύνη',
  'gestion.dispositivos.todos': 'Όλες οι συσκευές',
  'gestion.dispositivos.recuento': 'Εμφανίζονται {count} από {total}',
  'gestion.dispositivos.sinResultados': 'Καμία συσκευή δεν ταιριάζει.',
  'gestion.auditoria.rango': 'Γρήγορο εύρος',
  'gestion.auditoria.hoy': 'Σήμερα',
  'gestion.auditoria.7d': '7 ημέρες',
  'gestion.auditoria.30d': '30 ημέρες',
  'gestion.roles.duplicar': 'Αντιγραφή',
  'gestion.roles.copia': '{name} (αντίγραφο)',
  'gestion.lotes.titulo': 'Πρόσφατες μαζικές ενέργειες',
  'gestion.lotes.ayuda':
    'Τι εφαρμόστηκε σε πολλά άτομα μαζί και σε ποιους δεν ήταν δυνατό να εφαρμοστεί.',
  'gestion.lotes.ver': 'Εμφάνιση',
  'gestion.lotes.error': 'Δεν ήταν δυνατή η φόρτωση των μαζικών ενεργειών.',
  'gestion.lotes.vacio': 'Δεν υπάρχουν ακόμη μαζικές ενέργειες.',
  'gestion.lotes.progreso': '{done} από {total}',
  'gestion.lotes.accion.setRole': 'Αλλαγή ρόλου',
  'gestion.lotes.estado.ejecutando': 'Σε εξέλιξη',
  'gestion.lotes.estado.terminado': 'Ολοκληρώθηκε',
  'gestion.lotes.estado.cancelado': 'Ακυρώθηκε',
  'gestion.lotes.estado.interrumpido': 'Διακόπηκε',
  'gestion.lotes.detalle': 'Λεπτομέρειες',
  'gestion.lotes.todosBien': 'Εφαρμόστηκε σε όλους.',
  'gestion.lote.fallos': 'Δεν εφαρμόστηκε σε',
  'gestion.lote.cerrar': 'Κλείσιμο',
  'gestion.lote.codigo.not_found': 'δεν ανήκει σε αυτόν τον οργανισμό',
  'gestion.lote.codigo.self': 'είστε εσείς',
  'gestion.lote.codigo.forbidden': 'η αλλαγή λογαριασμού κατόχου απαιτεί δικαιώματα κατόχου',
  'gestion.lote.codigo.deleted': 'ο λογαριασμός είναι κλειστός',
  'gestion.lote.codigo.last_owner': 'ο τελευταίος κάτοχος του οργανισμού',
  'gestion.lote.codigo.last_admin': 'ο τελευταίος διαχειριστής του οργανισμού',
  'gestion.lote.codigo.error': 'απρόσμενο σφάλμα· δοκιμάστε ξανά',
  'gestion.lote.seleccionarVisibles': 'Επιλογή όλων των ορατών ({count})',
  'gestion.lote.recortado':
    'Μια μαζική ενέργεια καλύπτει έως {max} άτομα: επιλέχθηκαν τα πρώτα {max} ορατά.',
};
