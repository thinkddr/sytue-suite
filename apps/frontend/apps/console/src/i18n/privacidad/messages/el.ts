/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de solicitudes RGPD y la supresión física de una cuenta: traducción
 * del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const el: AppCatalog<typeof en> = {
  'privacidad.titulo': 'Αιτήματα απορρήτου',
  'privacidad.descripcion':
    'Τι ζητούν τα πρόσωπα από τον οργανισμό σας σχετικά με τα προσωπικά τους δεδομένα (ΓΚΠΔ, άρθρα 15 έως 21). Καταχωρίστε κάθε αίτημα, απαντήστε εντός ενός μήνα και, αν πρόκειται για διαγραφή, εκτελέστε την εδώ.',
  'privacidad.registrar': 'Καταχώριση αιτήματος',
  'privacidad.filtro': 'Εμφάνιση',
  'privacidad.filtro.abiertas': 'Ανοιχτά',
  'privacidad.filtro.vencidas': 'Εκπρόθεσμα',
  'privacidad.filtro.todas': 'Όλα',
  'privacidad.resumen.abiertas': 'Ανοιχτά αιτήματα',
  'privacidad.resumen.vencidas': 'Έληξε η προθεσμία',
  'privacidad.vacia': 'Δεν υπάρχουν αιτήματα για εμφάνιση.',
  'privacidad.errorCarga': 'Δεν ήταν δυνατή η φόρτωση του μητρώου.',
  'privacidad.col.persona': 'Πρόσωπο',
  'privacidad.col.tipo': 'Αίτημα',
  'privacidad.col.recibida': 'Παραλήφθηκε',
  'privacidad.col.plazo': 'Προθεσμία',
  'privacidad.col.estado': 'Κατάσταση',
  'privacidad.tipo.access': 'Πρόσβαση στα δεδομένα του',
  'privacidad.tipo.rectification': 'Διόρθωση',
  'privacidad.tipo.erasure': 'Διαγραφή',
  'privacidad.tipo.restriction': 'Περιορισμός επεξεργασίας',
  'privacidad.tipo.portability': 'Φορητότητα δεδομένων',
  'privacidad.tipo.objection': 'Εναντίωση',
  'privacidad.canal.email': 'Email',
  'privacidad.canal.web': 'Φόρμα ιστού',
  'privacidad.canal.letter': 'Επιστολή',
  'privacidad.canal.phone': 'Τηλέφωνο',
  'privacidad.canal.in_person': 'Αυτοπροσώπως',
  'privacidad.canal.other': 'Άλλο',
  'privacidad.estado.received': 'Παραλήφθηκε',
  'privacidad.estado.in_progress': 'Σε εξέλιξη',
  'privacidad.estado.completed': 'Απαντήθηκε',
  'privacidad.estado.rejected': 'Απορρίφθηκε',
  'privacidad.plazo.vencida': 'Εκπρόθεσμο',
  'privacidad.plazo.pronto': 'Λήγει σύντομα',
  'privacidad.plazo.prorrogada': 'Παρατάθηκε',
  'privacidad.alta.quien': 'Ποιος το ζητά',
  'privacidad.alta.miembro': 'Κάποιος από τον οργανισμό',
  'privacidad.alta.fuera': 'Κάποιος εκτός οργανισμού',
  'privacidad.alta.elegirMiembro': 'Επιλέξτε το πρόσωπο',
  'privacidad.alta.correo': 'Η διεύθυνση email του',
  'privacidad.alta.nombre': 'Το όνομά του (προαιρετικό)',
  'privacidad.alta.tipo': 'Τι ζητά',
  'privacidad.alta.canal': 'Πώς υποβλήθηκε',
  'privacidad.alta.recibida': 'Ημερομηνία παραλαβής',
  'privacidad.alta.recibidaAyuda':
    'Η προθεσμία του ενός μήνα μετρά από αυτή την ημερομηνία, όχι από σήμερα.',
  'privacidad.alta.identidad': 'Ταυτότητα επαληθευμένη',
  'privacidad.alta.metodo': 'Πώς επαληθεύτηκε',
  'privacidad.alta.notas': 'Σημειώσεις',
  'privacidad.alta.error.subject': 'Δηλώστε ποιος υποβάλλει το αίτημα.',
  'privacidad.alta.error.email': 'Εισαγάγετε έγκυρη διεύθυνση email.',
  'privacidad.alta.error.date':
    'Η ημερομηνία πρέπει να είναι εντός του τελευταίου έτους και όχι μελλοντική.',
  'privacidad.alta.error.upstream': 'Δεν ήταν δυνατή η καταχώριση του αιτήματος.',
  'privacidad.alta.hecha': 'Το αίτημα καταχωρίστηκε.',
  'privacidad.detalle.plazo': 'Απάντηση έως {fecha}',
  'privacidad.detalle.resolucion': 'Απάντηση που δόθηκε στο πρόσωπο',
  'privacidad.detalle.resolucionAyuda':
    'Απαιτείται για το κλείσιμο του αιτήματος: είναι αυτό που ανακοινώνεται στο πρόσωπο.',
  'privacidad.detalle.guardar': 'Αποθήκευση',
  'privacidad.detalle.guardado': 'Το αίτημα ενημερώθηκε.',
  'privacidad.detalle.cerrada': 'Αυτό το αίτημα έχει κλείσει και δεν μπορεί πλέον να αλλάξει.',
  'privacidad.detalle.prorroga': 'Παράταση κατά δύο μήνες',
  'privacidad.detalle.prorrogaAyuda':
    'Μία φορά και πριν λήξει η προθεσμία. Πρέπει να ενημερωθεί το πρόσωπο για τον λόγο: γράψτε τον εδώ.',
  'privacidad.detalle.prorrogar': 'Παράταση',
  'privacidad.detalle.historia': 'Ιστορικό',
  'privacidad.evento.created': 'Καταχωρίστηκε',
  'privacidad.evento.updated': 'Ενημερώθηκε',
  'privacidad.evento.extended': 'Η προθεσμία παρατάθηκε',
  'privacidad.evento.erasureStarted': 'Η διαγραφή ξεκίνησε',
  'privacidad.evento.erasureCompleted': 'Η διαγραφή ολοκληρώθηκε',
  'privacidad.supresion.titulo': 'Διαγραφή λογαριασμού',
  'privacidad.supresion.explica':
    'Ο λογαριασμός κλείνει αμέσως και οριστικά και οι τρόποι σύνδεσής του διαγράφονται. Μέσα σε λίγα λεπτά το προφίλ ανωνυμοποιείται και διαγράφεται ό,τι διατηρούν οι άλλες υπηρεσίες για το πρόσωπο. Ό,τι δημιούργησε το πρόσωπο για τον οργανισμό (γραμματοκιβώτιο, αρχεία, συμβάντα) παραμένει στον οργανισμό.',
  'privacidad.supresion.confirmar': 'Πληκτρολογήστε {correo} για επιβεβαίωση',
  'privacidad.supresion.iniciada': 'Η διαγραφή ξεκίνησε. Ολοκληρώνεται σε λίγα λεπτά.',
  'privacidad.supresion.enCurso': 'Η διαγραφή βρίσκεται σε εξέλιξη.',
  'privacidad.supresion.hecha': 'Ο λογαριασμός διαγράφηκε.',
  'privacidad.supresion.sinCuenta':
    'Το πρόσωπο δεν έχει λογαριασμό στον οργανισμό: δεν υπάρχει εδώ λογαριασμός προς διαγραφή.',
  'privacidad.supresion.noMiembro': 'Το πρόσωπο δεν είναι πλέον μέλος του οργανισμού.',
  'privacidad.supresion.error': 'Δεν ήταν δυνατή η διαγραφή του λογαριασμού.',
  'privacidad.error.upstream': 'Δεν ήταν δυνατή η αποθήκευση της αλλαγής.',
  'privacidad.cancelar': 'Ακύρωση',
};
