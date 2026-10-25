/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const el: AppCatalog<typeof en> = {
  'page.home': 'Sytue One — Αρχική',
  'page.search': 'Αναζήτηση στο Sytue One',
  'page.searchQuery': '«{query}» — Αναζήτηση στο Sytue One',
  'page.devices': 'Συνδέστε τις συσκευές σας — Sytue One',
  'page.data': 'Τα δεδομένα σας — Sytue One',
  'page.downloads': 'Λήψεις — Sytue One',
  'downloads.title': 'Κατεβάστε τις εφαρμογές',
  'downloads.lead':
    'Εγγενείς εφαρμογές για τον υπολογιστή σας. Εγκαθίστανται, βρίσκονται στο Dock ή στο μενού Έναρξη και σας ειδοποιούν μέσω των ειδοποιήσεων του συστήματος.',
  'downloads.version': 'Έκδοση {version}',
  'downloads.get': 'Λήψη',
  'downloads.forYou': 'Για {platform}',
  'downloads.others': 'Άλλες πλατφόρμες',
  'downloads.all': 'Όλες οι πλατφόρμες',
  'downloads.empty': 'Δεν έχουν δημοσιευτεί ακόμη προγράμματα εγκατάστασης',
  'downloads.emptyNoRelease':
    'Καμία έκδοση δεν έχει δημοσιεύσει ακόμη τα προγράμματα εγκατάστασής της. Θα εμφανιστούν εδώ μόλις συμβεί αυτό· δεν χρειάζεται να κάνετε τίποτα.',
  'downloads.emptyUnconfigured':
    'Αυτή η εγκατάσταση δεν γνωρίζει πού να αναζητήσει τα προγράμματα εγκατάστασης. Η ρύθμιση γίνεται από την ομάδα λειτουργίας.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Δεν ήταν δυνατή η φόρτωση των λήψεων',
  'downloads.failedBody':
    'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Δοκιμάστε ξανά σε λίγα δευτερόλεπτα.',
  'downloads.savedTitle': 'Η λήψη ολοκληρώθηκε',
  'downloads.saveFailedTitle': 'Δεν ήταν δυνατή η λήψη του προγράμματος εγκατάστασης',
  'page.activity': 'Πρόσφατη δραστηριότητα — Sytue One',
  'page.welcome': 'Καλώς ήρθατε στο Sytue',
  'activity.overline': 'Δραστηριότητα',
  'activity.heading': 'Πρόσφατη δραστηριότητα',
  'activity.intro':
    'Ό,τι έχει συμβεί πρόσφατα στην αλληλογραφία, τα αρχεία, τα έγγραφα, τα συμβάντα και τις αίθουσές σας.',
  'activity.today': 'Σήμερα',
  'activity.yesterday': 'Χθες',
  'activity.empty':
    'Δεν υπάρχει ακόμη τίποτα πρόσφατο. Καθώς χρησιμοποιείτε το Sytue, ό,τι αγγίζετε θα εμφανίζεται εδώ.',
  'activity.failedTitle': 'Δεν ήταν δυνατή η φόρτωση της δραστηριότητας',
  'activity.failedBody':
    'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Δοκιμάστε ξανά σε λίγα δευτερόλεπτα.',
  'activity.retry': 'Δοκιμή ξανά',
  'activity.all': 'Όλα',
  'activity.filterBy': 'Φιλτράρισμα ανά προϊόν',
  'activity.count': { one: '{count} στοιχείο', other: '{count} στοιχεία' },
  'page.inbox': 'Ειδοποιήσεις — Sytue One',
  'inbox.overline': 'Ειδοποιήσεις',
  'inbox.heading': 'Ειδοποιήσεις',
  'inbox.intro': 'Ό,τι έχει να σας πει η σουίτα, σε ένα σημείο.',
  'inbox.all': 'Όλες',
  'inbox.unreadFilter': { one: '{count} μη αναγνωσμένη', other: '{count} μη αναγνωσμένες' },
  'inbox.filterBy': 'Φιλτράρισμα ειδοποιήσεων',
  'inbox.markAll': 'Σήμανση όλων ως αναγνωσμένων',
  'inbox.loading': 'Φόρτωση ειδοποιήσεων…',
  'inbox.unreadDot': 'Μη αναγνωσμένη',
  'inbox.errorTitle': 'Δεν ήταν δυνατή η φόρτωση των ειδοποιήσεων',
  'inbox.errorBody': 'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Δοκιμάστε ξανά σε λίγα δευτερόλεπτα.',
  'inbox.retry': 'Δοκιμή ξανά',
  'inbox.emptyTitle': 'Τίποτα για διάβασμα',
  'inbox.emptyBody': 'Όταν κάτι χρειάζεται την προσοχή σας, θα εμφανιστεί εδώ.',
  'inbox.emptyUnreadTitle': 'Δεν υπάρχει κάτι νέο',
  'inbox.emptyUnreadBody': 'Τα διαβάσατε όλα. Επιλέξτε «Όλες» για να δείτε τις προηγούμενες.',
  'inbox.prefsTitle': 'Τι θέλετε να λαμβάνετε',
  'inbox.prefsBody':
    'Αν απενεργοποιήσετε ένα προϊόν, σταματά να σας ειδοποιεί εδώ και μέσω email. Μπορείτε να το ενεργοποιήσετε ξανά όποτε θέλετε.',
  'launcher.heading': 'Οι εφαρμογές σας',
  'launcher.greeting.morning': 'Καλημέρα',
  'launcher.greeting.afternoon': 'Καλό απόγευμα',
  'launcher.greeting.evening': 'Καλησπέρα',
  'launcher.more': 'Περισσότερα',
  'launcher.apps': 'Εφαρμογές',
  'launcher.connected': 'Συνδεδεμένο',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Απενεργοποιημένο',
  'product.mail': 'Ηλεκτρονικό ταχυδρομείο',
  'product.drive': 'Αρχεία και συνεργασία',
  'product.calendar': 'Ημερολόγιο',
  'product.contacts': 'Επαφές',
  'product.docs': 'Έγγραφα, υπολογιστικά φύλλα και παρουσιάσεις',
  'product.rooms': 'Κανάλια, μηνύματα και κλήσεις',
  'product.cloud': 'Πλατφόρμα εφαρμογών (PaaS)',
  'product.console': 'Διαχείριση και οργανισμοί',
  'product.id': 'Ταυτότητα, προφίλ και ασφάλεια',
  'quick.heading': 'Γρήγορες ενέργειες',
  'quick.search': 'Αναζήτηση σε όλη τη σουίτα',
  'quick.compose': 'Σύνταξη email',
  'quick.newDoc': 'Νέο έγγραφο',
  'quick.upload': 'Μεταφόρτωση στο Drive',
  'quick.newEvent': 'Δημιουργία συμβάντος',
  'quick.download': 'Λήψη της εφαρμογής',
  'quick.devices': 'Σύνδεση συσκευών',
  'quick.data': 'Τα δεδομένα σας',

  'storage.connectTitle': 'Σύνδεση εξωτερικού χώρου αποθήκευσης',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV και άλλα.',
  'recent.heading': 'Πρόσφατα σε όλη τη σουίτα',
  'recent.viewAll': 'Προβολή όλων',
  'recent.loading': 'Φόρτωση πρόσφατης δραστηριότητας…',
  'recent.errorTitle': 'Δεν μπορέσαμε να φορτώσουμε τη δραστηριότητά σας',
  'recent.errorBody':
    'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Η δουλειά σας είναι ασφαλής· δοκιμάστε ξανά.',
  'recent.retry': 'Δοκιμή ξανά',
  'recent.emptyTitle': 'Καμία πρόσφατη δραστηριότητα',
  'recent.emptyBody':
    'Όταν εργάζεστε στο Mail, το Drive, το Calendar ή το Rooms, θα το βλέπετε εδώ.',
  'recent.emptyCta': 'Ξεκινήστε στο Drive',
  'search.overline': 'Αναζήτηση',
  'search.heading': 'Αναζήτηση σε όλη τη σουίτα',
  'search.placeholder': 'Αναζήτηση σε Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Αναζήτηση στο Sytue One',
  'search.submit': 'Αναζήτηση',
  'search.hint':
    'Πληκτρολογήστε παραπάνω για να αναζητήσετε ταυτόχρονα στην αλληλογραφία, τα αρχεία, τα έγγραφα, τα συμβάντα, τις επαφές και τις αίθουσές σας. Τα αποτελέσματα ανοίγουν απευθείας στο προϊόν τους.',
  'search.failedTitle': 'Δεν ήταν δυνατή η ολοκλήρωση της αναζήτησης',
  'search.failedBody':
    'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Δοκιμάστε ξανά σε λίγα δευτερόλεπτα.',
  'search.retry': 'Δοκιμή ξανά',
  'search.empty': 'Κανένα αποτέλεσμα για «{query}». Δοκιμάστε άλλες λέξεις.',
  'search.count': { one: '{count} αποτέλεσμα', other: '{count} αποτελέσματα' },
  'welcome.heading': 'Καλώς ήρθατε στο Sytue',
  'welcome.lead':
    'Λίγα λεπτά και όλα θα λειτουργούν. Προχωράμε βήμα βήμα· αν σταματήσετε στη μέση, όταν επιστρέψετε συνεχίζουμε από εκεί που μείναμε.',
  'data.title': 'Τα δεδομένα σας',
  'data.lead':
    'Κατεβάστε ένα αντίγραφο όλων όσων έχετε στο Sytue One, όποτε θέλετε και χωρίς να ζητήσετε τίποτα από κανέναν.',
  'data.export.section': 'Λήψη αντιγράφου',
  'data.export.body':
    'Ένα μόνο αρχείο ZIP με την αλληλογραφία, τα αρχεία, τα ημερολόγια, τις επαφές, τις σημειώσεις, τα έγγραφα, τις φωτογραφίες και τα μηνύματά σας. Ό,τι έχει ήδη τυπική μορφή μένει όπως είναι: τα email ως .eml, τα ημερολόγια ως .ics, οι επαφές ως .vcf και τα αρχεία ακριβώς όπως τα ανεβάσατε.',
  'data.export.formatLegend': 'Μορφή των λιστών',
  'data.export.formatHelp':
    'Οι λίστες περιγράφουν τι περιέχει το αντίγραφο: τα μηνύματα, τα συμβάντα, τις επαφές ή τα αρχεία σας, ένα ανά γραμμή.',
  'data.export.orgHelp':
    'Κάθε αντίγραφο αφορά έναν οργανισμό. Αν επιλέξετε άλλον, μπαίνετε σε αυτόν και η λήψη ξεκινά αυτόματα.',
  'data.export.json': 'Για άλλες εφαρμογές και προγράμματα.',
  'data.export.csv': 'Για άνοιγμα σε Excel, Numbers ή Google Sheets.',
  'data.export.xml': 'Για συστήματα που ζητούν XML.',
  'data.export.action': 'Λήψη αντιγράφου',
  'data.export.startedTitle': 'Η λήψη ξεκίνησε',
  'data.export.startedBody':
    'Το πρόγραμμα περιήγησης συνεχίζει να την αποθηκεύει ακόμη κι αν κλείσετε αυτή τη σελίδα. Αν έχετε πολλά αρχεία, θα πάρει λίγη ώρα.',
  'data.export.scope':
    'Το αντίγραφο περιλαμβάνει τα δεδομένα του οργανισμού με τον οποίο έχετε συνδεθεί. Το αρχείο σύνοψης που περιέχει δείχνει τι συμπεριλήφθηκε από κάθε εφαρμογή.',
  'data.move.section': 'Μεταφορά σε άλλη υπηρεσία',
  'data.move.body':
    'Για να μεταφερθούν όλα απευθείας στο Google Workspace ή στο Microsoft 365 χωρίς λήψη: αν ο λογαριασμός σας είναι προσωπικός ή διαχειρίζεστε τον οργανισμό σας, ξεκινήστε τη μεταφορά εσείς από το Sytue Admin, στην ενότητα «Μετεγκαταστάσεις»· αλλιώς, ζητήστε το από όποιον τον διαχειρίζεται.',
  'data.move.action': 'Μετάβαση στις «Μετεγκαταστάσεις»',

  'devices.native.body':
    'Χρησιμοποίησε τη διαχείριση αρχείων της συσκευής ή σύνδεσε έναν λογαριασμό με τη συγκατάθεση του κατόχου του.',
  'devices.native.driveBody':
    'Το Sytue Drive προστίθεται αυτόματα μετά τη σύνδεση. Επίτρεψε την τοποθεσία του αν το ζητήσει το σύστημα.',
  'devices.native.openDrive': 'Άνοιγμα στη διαχείριση αρχείων',
  'devices.native.failed': 'Δεν ήταν δυνατή η ενημέρωση της ενσωμάτωσης της συσκευής.',
  'devices.title': 'Συνδέστε τις συσκευές σας',
  'devices.lead':
    'Πάρτε την αλληλογραφία, το ημερολόγιο, τις επαφές και τα αρχεία σας από το Sytue στο κινητό ή στον υπολογιστή σας. Σας καθοδηγούμε βήμα βήμα.',
  'devices.leadShort':
    'Πάρτε την αλληλογραφία, το ημερολόγιο και τις επαφές σας από το Sytue στο κινητό ή στον υπολογιστή σας.',
  'devices.unavailable.section': 'Δεν μπορούμε να σας δείξουμε τα στοιχεία αυτή τη στιγμή',
  'devices.unavailable.title': 'Δεν μπορέσαμε να λάβουμε τη διαμόρφωση',
  'devices.unavailable.body':
    'Δοκιμάστε ξανά σε λίγο. Αν δεν αλλάξει κάτι, επικοινωνήστε με τη διαχείριση του Sytue στον οργανισμό σας.',
  'devices.retry': 'Δοκιμή ξανά',
  'devices.copy': 'Αντιγραφή',
  'devices.copyFailTitle': 'Δεν ήταν δυνατή η αντιγραφή',
  'devices.copyFailBody': 'Επιλέξτε το και αντιγράψτε το χειροκίνητα.',
  'devices.copied.address': 'Η διεύθυνσή σας αντιγράφηκε',
  'devices.copied.imap': 'Ο διακομιστής εισερχομένων αντιγράφηκε',
  'devices.copied.port': 'Η θύρα αντιγράφηκε',
  'devices.copied.smtp': 'Ο διακομιστής εξερχομένων αντιγράφηκε',
  'devices.copied.url': 'Η διεύθυνση αντιγράφηκε',
  'devices.copied.drive': 'Η διεύθυνση του Drive αντιγράφηκε',
  'devices.oneClick.thunderbird': 'Ρύθμιση του Thunderbird',
  'devices.oneClick.outlook': 'Ρύθμιση του Outlook',
  'devices.oneClick.android': 'Ρύθμιση στο Android',
  'devices.oneClick.body':
    'Δημιουργεί έναν κωδικό εφαρμογής μόνο για αυτό το πρόγραμμα, τον αντιγράφει και σας τον δείχνει μία φορά.',
  'devices.oneClick.failTitle': 'Δεν ήταν δυνατή η δημιουργία του κωδικού εφαρμογής',
  'devices.oneClick.failBody':
    'Δοκιμάστε ξανά σε λίγο. Μπορείτε επίσης να δημιουργήσετε έναν από τον Λογαριασμό μου.',
  'devices.oneClick.onceTitle': 'Αντιγράφηκε. Εμφανίζεται μόνο αυτή τη φορά',
  'devices.oneClick.onceBody':
    'Επικολλήστε τον εκεί όπου το πρόγραμμα ζητά τον κωδικό. Αν τον χάσετε, ανακαλέστε τον από τον Λογαριασμό μου και δημιουργήστε νέο.',
  'devices.oneClick.passwordLabel': 'Κωδικός εφαρμογής',
  'devices.copied.password': 'Ο κωδικός εφαρμογής αντιγράφηκε',
  'devices.oneClick.notOpened':
    'Δεν ήταν δυνατό να ανοίξει το {program} σε αυτόν τον υπολογιστή: ανοίξτε το εσείς και ακολουθήστε αυτά τα βήματα.',
  'devices.oneClick.next.thunderbird':
    'Στον οδηγό νέου λογαριασμού του Thunderbird, πληκτρολογήστε τη διεύθυνσή σας και επικολλήστε τον κωδικό: βρίσκει μόνο του τους διακομιστές. Ημερολόγια και επαφές: προσθέστε ένα ημερολόγιο δικτύου και ένα βιβλίο διευθύνσεων CardDAV με την ίδια διεύθυνση και τον ίδιο κωδικό (αν ζητήσει τοποθεσία, αντιγράψτε την από κάτω).',
  'devices.oneClick.next.outlook':
    'Στην «Προσθήκη λογαριασμού» του Outlook, πληκτρολογήστε τη διεύθυνσή σας και επικολλήστε τον κωδικό: το Outlook βρίσκει μόνο του τους διακομιστές.',
  'devices.oneClick.next.android':
    'Στην εφαρμογή αλληλογραφίας σας, προσθέστε έναν λογαριασμό «Άλλο (IMAP)» με τη διεύθυνσή σας και επικολλήστε τον κωδικό. Ημερολόγια και επαφές: DAVx5 με τις διευθύνσεις παρακάτω και τον ίδιο κωδικό.',
  'devices.android.davx5Calendars': 'Ημερολόγια στο DAVx5',
  'devices.android.davx5Contacts': 'Επαφές στο DAVx5',
  'devices.android.davx5Install': 'Λήψη του DAVx5 από το F-Droid',
  'devices.platform.iphone.title': 'iPhone ή iPad',
  'devices.platform.iphone.body':
    'Εγκαθιστάτε ένα προφίλ και η αλληλογραφία, το ημερολόγιο και οι επαφές είναι έτοιμα.',
  'devices.platform.iphone.note': 'Ο πιο γρήγορος τρόπος: ένα αρχείο και ο κωδικός σας.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'Το ίδιο προφίλ με το iPhone, εγκατεστημένο από τις Ρυθμίσεις συστήματος.',
  'devices.platform.mac.note': 'Apple Mail, Ημερολόγιο και Επαφές.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Πληκτρολογείτε τη διεύθυνση και τον κωδικό σας· το Outlook βρίσκει μόνο του τα υπόλοιπα.',
  'devices.platform.outlook.note':
    'Μόνο αλληλογραφία. Το ημερολόγιο και οι επαφές ρυθμίζονται ξεχωριστά.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Ρυθμίζεται μόνο του μόνο με τη διεύθυνσή σας· το ημερολόγιο και οι επαφές προστίθενται στη συνέχεια.',
  'devices.platform.thunderbird.note': 'Windows, Mac ή Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'Η εφαρμογή αλληλογραφίας σας για το γραμματοκιβώτιο και το DAVx5 για το ημερολόγιο και τις επαφές.',
  'devices.platform.android.note':
    'Δεν υπάρχει εφαρμογή Sytue: χρησιμοποιούνται οι εφαρμογές του συστήματος.',
  'devices.platform.apis.title': 'Ένα εργαλείο που μιλά τη γλώσσα της Google ή της Microsoft',
  'devices.platform.apis.body':
    'Στρέψτε προς το Sytue μια ενσωμάτωση γραμμένη για Google Drive, Google Calendar, Google Contacts ή Microsoft Graph, χωρίς να αλλάξετε τον κώδικά της.',
  'devices.platform.apis.note': 'Μόνο όπου μπορείτε να ορίσετε τη βασική διεύθυνση URL.',
  'devices.apis.step1.title': 'Αλλάξτε τη βασική διεύθυνση',
  'devices.apis.step1.body':
    'Στο εργαλείο, αντικαταστήστε τη διεύθυνση που καλεί με εκείνη του προϊόντος και της διαλέκτου που χρειάζεστε. Η υπόλοιπη διαμόρφωσή του μένει ως έχει.',
  'devices.apis.step2.title': 'Χρησιμοποιήστε τα διαπιστευτήριά σας του Sytue',
  'devices.apis.step2.body':
    'Η ταυτοποίηση γίνεται με το διακριτικό σας του Sytue ID, όχι με λογαριασμό Google ή Microsoft: τα δικαιώματα είναι τα ίδια που έχετε ήδη εδώ.',
  'devices.apis.identityGoogle': 'Ταυτότητα · Google OAuth και κατάλογος',
  'devices.apis.step3.title': 'Εφαρμογή τρίτου μέρους, σαν να ήταν η Google',
  'devices.apis.step3.body':
    'Αν το εργαλείο συνδέεται με Google OAuth, καταχωρίζεται στο Sytue Admin με τα δικαιώματα Google που χρειάζεται. Η εφαρμογή ζητά αυτά τα δικαιώματα και λαμβάνει δεδομένα του Sytue· η Google δεν εμπλέκεται και δεν βλέπει τίποτα.',
  'devices.apis.driveGoogle': 'Drive · Google Drive API',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · Google Calendar API',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · Google People API',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Τι καλύπτεται',
  'devices.apis.expectBody':
    'Εμφάνιση λίστας, ανάγνωση, δημιουργία, επεξεργασία και διαγραφή. Ό,τι δεν καλύπτεται απαντά με σαφές σφάλμα, ποτέ με κενή επιτυχία.',
  'devices.copied.apiBase': 'Η βασική διεύθυνση αντιγράφηκε',
  'devices.check.apis.label': 'Το εργαλείο',
  'devices.check.apis.value':
    'Εμφανίζει το περιεχόμενό σας στο Sytue σαν να ήταν η αρχική υπηρεσία.',
  'devices.platform.drive.title': 'Εξερεύνηση αρχείων (Drive)',
  'devices.platform.drive.body':
    'Βλέπετε τα αρχεία σας του Sytue Drive σαν μία ακόμη μονάδα δίσκου στον υπολογιστή.',
  'devices.platform.drive.note': 'Finder σε Mac, Εξερεύνηση αρχείων σε Windows.',
  'devices.mail.address': 'Η διεύθυνσή σας (όνομα χρήστη)',
  'devices.mail.imapHost': 'Διακομιστής εισερχομένων (IMAP)',
  'devices.mail.imapPort': 'Θύρα εισερχομένων',
  'devices.mail.smtpHost': 'Διακομιστής εξερχομένων (SMTP)',
  'devices.mail.smtpPort': 'Θύρα εξερχομένων',
  'devices.mail.hint':
    'Κρυπτογράφηση SSL/TLS και στους δύο, και ο ίδιος κωδικός Sytue με τον οποίο συνδέεστε στο web.',
  'devices.dav.caldav': 'Ημερολόγιο (CalDAV)',
  'devices.dav.carddav': 'Επαφές (CardDAV)',
  'devices.dav.user': 'Όνομα χρήστη',
  'devices.apple.download': 'Λήψη του προφίλ Sytue',
  'devices.apple.step1.title': 'Κατεβάστε το προφίλ',
  'devices.apple.step1.body':
    'Πατήστε το κουμπί παραπάνω. Κατεβαίνει ένα μικρό αρχείο με τις ρυθμίσεις του λογαριασμού σας.',
  'devices.apple.step2.iphone.title': 'Εγκαταστήστε το από τις Ρυθμίσεις',
  'devices.apple.step2.iphone.body':
    'Ανοίξτε τις Ρυθμίσεις. Στην κορυφή εμφανίζεται «Λήφθηκε προφίλ»: αγγίξτε το και πατήστε Εγκατάσταση.',
  'devices.apple.step2.mac.title': 'Εγκαταστήστε το από τις Ρυθμίσεις συστήματος',
  'devices.apple.step2.mac.body':
    'Ανοίξτε Ρυθμίσεις συστήματος › Απόρρητο και ασφάλεια › Προφίλ, κάντε διπλό κλικ στο προφίλ που κατεβάσατε και πατήστε Εγκατάσταση.',
  'devices.apple.step3.title': 'Δεν χρειάζεται να πληκτρολογήσετε κωδικό',
  'devices.apple.step3.body':
    'Το προφίλ περιέχει δικό του κωδικό εφαρμογής, που δημιουργήθηκε μόνο για αυτή τη συσκευή. Μπορείτε να τον ανακαλέσετε όποτε θέλετε από τον Λογαριασμό μου.',
  'devices.apple.step4.title': 'Έγινε',
  'devices.apple.step4.body':
    'Η αλληλογραφία, το ημερολόγιο και οι επαφές προστίθενται με μία κίνηση. Δεν χρειάζεται να πληκτρολογήσετε διακομιστή.',
  'devices.apple.manualTitle': 'Αν προτιμάτε να το κάνετε χειροκίνητα',
  'devices.apple.manualBody':
    'Μπορείτε επίσης να δημιουργήσετε τον λογαριασμό χειροκίνητα με αυτά τα στοιχεία.',
  'devices.manualValues': 'Προβολή τιμών για χειροκίνητη ρύθμιση',
  'devices.desktop.step1.title': 'Προσθέστε λογαριασμό email',
  'devices.desktop.step1.outlook': 'Στο Outlook: Αρχείο › Προσθήκη λογαριασμού.',
  'devices.desktop.step1.thunderbird':
    'Στο Thunderbird: Λογαριασμοί › Ρύθμιση λογαριασμού › Email.',
  'devices.desktop.step2.title': 'Πληκτρολογήστε μόνο τη διεύθυνση και τον κωδικό σας',
  'devices.desktop.step2.outlook':
    'Τίποτε άλλο. Το Outlook ζητά τις ρυθμίσεις από το Sytue και τις συμπληρώνει για εσάς.',
  'devices.desktop.step2.thunderbird':
    'Τίποτε άλλο. Το Thunderbird ζητά τις ρυθμίσεις από το Sytue και τις συμπληρώνει για εσάς.',
  'devices.desktop.step3.title': 'Επιβεβαιώστε',
  'devices.desktop.step3.body':
    'Όταν σας δείξει τους διακομιστές, αποδεχτείτε. Συνδέεται ως λογαριασμός IMAP.',
  'devices.desktop.emailLabel': 'Η διεύθυνση email σας',
  'devices.desktop.notFoundTitle': 'Δεν βρέθηκαν οι ρυθμίσεις αυτόματα;',
  'devices.desktop.notFoundBody':
    'Επιλέξτε χειροκίνητη ρύθμιση και χρησιμοποιήστε αυτές τις τιμές.',
  'devices.thunderbird.davTitle': 'Ημερολόγιο και επαφές',
  'devices.thunderbird.davBody':
    'Στο Thunderbird προστίθενται ξεχωριστά, με αυτές τις διευθύνσεις.',
  'devices.outlook.davTitle': 'Ημερολόγιο και επαφές στο Outlook',
  'devices.outlook.davBody':
    'Το Outlook για Windows δεν υποστηρίζει CalDAV/CardDAV, οπότε το ημερολόγιο και οι επαφές του Sytue δεν συγχρονίζονται εκεί. Χρησιμοποιήστε τα από το Sytue στο web ή από ένα πρόγραμμα που υποστηρίζει CalDAV/CardDAV (Thunderbird, Ημερολόγιο της Apple).',
  'devices.android.mailTitle': 'Αλληλογραφία',
  'devices.android.mailBody':
    'Με την εφαρμογή αλληλογραφίας που ήδη χρησιμοποιείτε (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Προσθέστε λογαριασμό',
  'devices.android.mail1.body': 'Επιλέξτε «Άλλο» ή «Προσωπικός (IMAP)», όχι «Google».',
  'devices.android.mail2.title': 'Πληκτρολογήστε τη διεύθυνση και τον κωδικό σας',
  'devices.android.mail2.body': 'Αν ζητήσει τους διακομιστές, αντιγράψτε τους από εδώ κάτω.',
  'devices.android.davTitle': 'Ημερολόγιο και επαφές',
  'devices.android.davBody':
    'Το Android δεν έχει ενσωματωμένο CalDAV/CardDAV: χρειάζεστε το DAVx5, μια δωρεάν εφαρμογή που τα προσθέτει στο ημερολόγιο και στις επαφές του ίδιου του τηλεφώνου.',
  'devices.android.dav1.title': 'Εγκαταστήστε το DAVx5',
  'devices.android.dav1.body':
    'Υπάρχει στο Google Play και στο F-Droid. Το Sytue ούτε το δημοσιεύει ούτε το συντηρεί.',
  'devices.android.dav2.title': 'Προσθέστε λογαριασμό με «Σύνδεση με URL και όνομα χρήστη»',
  'devices.android.dav2.body':
    'Επικολλήστε την παρακάτω διεύθυνση και χρησιμοποιήστε το email και τον κωδικό σας του Sytue.',
  'devices.android.dav3.title': 'Επιλέξτε τι θα συγχρονίζεται',
  'devices.android.dav3.body': 'Επιλέξτε τα ημερολόγια και το βιβλίο διευθύνσεών σας.',
  'devices.drive.step1.title': 'Ανοίξτε τη σύνδεση σε διακομιστή',
  'devices.drive.step1.body':
    'Σε Mac: Finder › Μετάβαση › Σύνδεση σε διακομιστή (⌘K). Σε Windows: Εξερεύνηση αρχείων › Αυτός ο υπολογιστής › Αντιστοίχιση μονάδας δίσκου δικτύου και επιλέξτε «Σύνδεση με χρήση διαφορετικών διαπιστευτηρίων».',
  'devices.drive.step2.title': 'Επικολλήστε αυτή τη διεύθυνση',
  'devices.drive.step2.body': 'Είναι ο φάκελος με τα αρχεία σας του Sytue Drive.',
  'devices.drive.step3.title': 'Συνδεθείτε με τον λογαριασμό σας του Sytue',
  'devices.drive.step3.body': 'Όνομα χρήστη: η διεύθυνση email σας. Κωδικός: αυτός του Sytue.',
  'devices.drive.urlLabel': 'Διεύθυνση Drive (WebDAV)',
  'devices.drive.expectTitle': 'Τι να περιμένετε',
  'devices.drive.windowsExpectBody':
    'Στα Windows, το Drive εμφανίζεται στην Εξερεύνηση αρχείων ως φάκελος «Sytue Drive». Τα αρχεία κατεβαίνουν όταν τα ανοίγετε· δεξί κλικ › «Να διατηρείται πάντα σε αυτή τη συσκευή» για να είναι διαθέσιμα εκτός σύνδεσης. Ό,τι αποθηκεύετε ή δημιουργείτε εκεί ανεβαίνει στο Drive.',
  'devices.drive.expectBody':
    'Τα αρχεία ανοίγουν από τον διακομιστή, δεν αντιγράφονται στον δίσκο. Για να εργαστείτε εκτός σύνδεσης, κατεβάστε από πριν ό,τι χρειάζεστε.',
  'devices.check.mail.label': 'Η αλληλογραφία',
  'devices.check.mail.value':
    'Τα νέα μηνύματα φτάνουν και μπορείτε να στείλετε ένα στον εαυτό σας.',
  'devices.check.dav.label': 'Ημερολόγιο και επαφές',
  'devices.check.dav.value':
    'Ένα συμβάν που δημιουργήθηκε στο Sytue στο web εμφανίζεται στη συσκευή μέσα σε λίγα λεπτά.',
  'devices.check.profile.label': 'Το προφίλ',
  'devices.check.profile.value': 'Το «Sytue One» εμφανίζεται στη λίστα εγκατεστημένων προφίλ.',
  'devices.check.drive.label': 'Η μονάδα δίσκου',
  'devices.check.drive.value':
    'Βλέπετε τους φακέλους σας του Drive και μπορείτε να ανοίξετε ένα αρχείο.',
  'devices.check.write.label': 'Η εγγραφή',
  'devices.check.write.value':
    'Δημιουργείτε έναν φάκελο και εμφανίζεται και στο Sytue Drive στο πρόγραμμα περιήγησης.',
  'devices.step1.title': 'Τι χρησιμοποιείτε',
  'devices.step1.heading': 'Πού θέλετε τον λογαριασμό σας του Sytue;',
  'devices.step1.description':
    'Επιλέξτε ένα. Αν έχετε περισσότερα, επαναλάβετε τον οδηγό για το καθένα.',
  'devices.step2.title': 'Πώς γίνεται',
  'devices.step2.heading': 'Ρύθμιση: {platform}',
  'devices.step2.headingFallback': 'Πώς γίνεται',
  'devices.step2.description':
    'Ακολουθήστε αυτά τα βήματα. Ό,τι πρέπει να πληκτρολογήσετε αντιγράφεται με ένα κουμπί.',
  'devices.step2.next': 'Έγινε',
  'devices.step3.title': 'Έλεγχος',
  'devices.step3.heading': 'Πώς θα ξέρετε ότι λειτούργησε',
  'devices.step3.description': 'Αν ισχύουν αυτά, η σύνδεση είναι σωστή.',
  'devices.step3.helpTitle': 'Κάτι δεν ταιριάζει;',
  'devices.step3.helpBody':
    'Συνήθως φταίει ο κωδικός: χρησιμοποιήστε τον ίδιο με τον οποίο συνδέεστε στο Sytue από το πρόγραμμα περιήγησης. Αν έχετε επαλήθευση σε δύο βήματα, ο οργανισμός σας ίσως απαιτεί κωδικό εφαρμογής· ρωτήστε τη διαχείριση του Sytue.',
  'devices.step3.again': 'Σύνδεση άλλης συσκευής',
  'ob.error.generic': 'Δεν μπορέσαμε να ολοκληρώσουμε την ενέργεια. Δοκιμάστε ξανά.',
  'ob.failedTitle': 'Δεν μπορέσαμε να φορτώσουμε τη ρύθμισή σας',
  'ob.failedBody':
    'Η υπηρεσία ταυτότητας δεν αποκρίνεται αυτή τη στιγμή. Δεν χάθηκε τίποτα: δοκιμάστε ξανά σε ένα λεπτό.',
  'ob.retry': 'Δοκιμή ξανά',
  'ob.preparing': 'Προετοιμασία της ρύθμισής σας…',
  'ob.lead':
    'Λίγα λεπτά και είστε έτοιμοι. Προχωράμε βήμα βήμα· αν σταματήσετε στη μέση, όταν επιστρέψετε συνεχίζουμε από εκεί που μείναμε.',
  'ob.saveFailedTitle': 'Δεν ήταν δυνατή η αποθήκευση',
  'ob.saveFailedBody': 'Δοκιμάστε ξανά σε λίγο.',
  'ob.copy': 'Αντιγραφή',
  'ob.copyFailTitle': 'Δεν ήταν δυνατή η αντιγραφή',
  'ob.copyFailBody': 'Επιλέξτε το με το ποντίκι και αντιγράψτε το χειροκίνητα.',
  'ob.copied.recordType': 'Ο τύπος εγγραφής αντιγράφηκε',
  'ob.copied.recordName': 'Το όνομα εγγραφής αντιγράφηκε',
  'ob.copied.recordValue': 'Η τιμή εγγραφής αντιγράφηκε',
  'ob.roleOption.member': 'Μέλος — χρησιμοποιεί τη σουίτα, δεν διαχειρίζεται τίποτα',
  'ob.roleOption.admin': 'Διαχειριστής — μπορεί να διαχειρίζεται όλο τον οργανισμό',
  'ob.roleOption.billing': 'Χρέωση — αναλαμβάνει τα τιμολόγια',
  'ob.roleName.owner': 'Ιδιοκτήτης',
  'ob.roleName.admin': 'Διαχειριστής',
  'ob.roleName.member': 'Μέλος',
  'ob.roleName.billing': 'Χρέωση',
  'ob.mode.title': 'Έναρξη',
  'ob.mode.heading': 'Πώς θα χρησιμοποιήσετε το Sytue;',
  'ob.mode.description': 'Έτσι ξέρουμε τι να σας ρωτήσουμε. Μπορείτε να το αλλάξετε αργότερα.',
  'ob.mode.individual.title': 'Μόνο για μένα',
  'ob.mode.individual.body':
    'Η αλληλογραφία, τα αρχεία και το ημερολόγιό σας, με δική σας διεύθυνση. Αν κάποια μέρα χρειαστεί να προσκαλέσετε άλλα άτομα, θα μπορείτε.',
  'ob.mode.org.title': 'Για τον οργανισμό μου',
  'ob.mode.org.body':
    'Εγγράφετε την ομάδα σας, κατανέμετε τα δικαιώματα και διαχειρίζεστε τα πάντα από ένα σημείο.',
  'ob.mode.org.badge': 'Το πιο συνηθισμένο',
  'ob.identity.titleOrg': 'Ο οργανισμός σας',
  'ob.identity.titleMe': 'Ο λογαριασμός σας',
  'ob.identity.headingOrg': 'Πώς λέγεται ο οργανισμός σας;',
  'ob.identity.headingMe': 'Πώς θέλετε να λέγεται ο λογαριασμός σας;',
  'ob.identity.descriptionCreated':
    'Έχει ήδη δημιουργηθεί. Μπορείτε να αλλάξετε το όνομα· η διεύθυνση μένει ως έχει.',
  'ob.identity.description': 'Με αυτό δημιουργούμε τον χώρο και τη διεύθυνσή σας στο Sytue.',
  'ob.identity.nextCreated': 'Συνέχεια',
  'ob.identity.nextOwnDomain': 'Δημιουργία και οδηγίες για τον τομέα μου',
  'ob.identity.next': 'Δημιουργία και συνέχεια',
  'ob.identity.nameLabel': 'Όνομα',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Μαρία Παπαδοπούλου',
  'ob.identity.nameHintOrg':
    'Αυτό βλέπει η ομάδα σας όταν συνδέεται. Μπορείτε να το αλλάξετε όποτε θέλετε.',
  'ob.identity.nameHintMe': 'Το βλέπετε μόνο εσείς. Μπορείτε να το αλλάξετε όποτε θέλετε.',
  'ob.identity.slugLabel': 'Η διεύθυνσή σας στο Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Η διεύθυνσή σας είναι {subdomain}. Δεν μπορεί να αλλάξει: από αυτή εξαρτώνται η αλληλογραφία και τα αρχεία σας.',
  'ob.identity.slugEmpty': 'Επιλέξτε ένα σύντομο όνομα. Θα είναι η διεύθυνσή σας: acme.{suite}',
  'ob.identity.slugChecking': 'Έλεγχος διαθεσιμότητας…',
  'ob.identity.slugFree': 'Διαθέσιμο. Η διεύθυνσή σας θα είναι {subdomain}',
  'ob.identity.slugTaken': 'Δεν μπορεί να χρησιμοποιηθεί.',
  'ob.identity.firstTitle': 'Πρώτα δημιουργούμε τον χώρο σας',
  'ob.identity.firstBody':
    'Όταν πατήσετε το κουμπί παρακάτω, δημιουργούμε τη διεύθυνσή σας στο Sytue και σας δείχνουμε εδώ την ακριβή εγγραφή που πρέπει να προσθέσετε στον τομέα σας. Δεν χρειάζεται να την έχετε έτοιμη τώρα.',
  'ob.domain.legend': 'Και ο δικός σας τομέας;',
  'ob.domain.keep.title': 'Κρατάω το {host}',
  'ob.domain.keep.placeholder': 'to-onoma-mou',
  'ob.domain.keep.body':
    'Δεν χρειάζεται να κάνετε τίποτα: τη δημοσίευση την αναλαμβάνουμε εμείς. Αυτό προτείνουμε για αρχή.',
  'ob.domain.own.title': 'Έχω ήδη τομέα και θέλω να τον χρησιμοποιήσω (acme.com)',
  'ob.domain.own.body':
    'Αν είναι στο Cloudflare, τον ρυθμίζουμε εμείς. Αν όχι, σας λέμε την ακριβή εγγραφή που πρέπει να προσθέσετε και την ελέγχουμε εδώ.',
  'ob.domain.search.title': 'Δεν έχω ακόμη τομέα',
  'ob.domain.search.body':
    'Ελέγχουμε αν το όνομα που θέλετε είναι διαθέσιμο και πόσο κοστίζει τον χρόνο. Είναι μόνο αναζήτηση: εδώ δεν αγοράζεται τίποτα.',
  'ob.domain.hostLabel': 'Ο τομέας σας',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Γράψτε τον ακριβώς: acme.com, χωρίς «https://» και χωρίς «www».',
  'ob.domain.verified': 'Το {host} επαληθεύτηκε',
  'ob.dns.legend': 'Πού διαχειρίζεστε το DNS για {domain};',
  'ob.dns.fallback': 'τον τομέα σας',
  'ob.dns.cf.title': 'Στο Cloudflare — να το ρυθμίσει το Sytue',
  'ob.dns.cf.body':
    'Μας δίνετε προσωρινή πρόσβαση στον λογαριασμό σας και δημιουργούμε εμείς τις εγγραφές. Πριν αγγίξουμε οτιδήποτε, σας δείχνουμε τι θα αλλάξουμε.',
  'ob.dns.cf.badge': 'Χωρίς επεμβάσεις στο DNS',
  'ob.dns.manual.title': 'Σε άλλον πάροχο — θα το κάνω χειροκίνητα',
  'ob.dns.manual.body':
    'Σας δίνουμε την ακριβή εγγραφή για να την αντιγράψετε στον πίνακα DNS σας και την ελέγχουμε εδώ μόλις προστεθεί.',
  'ob.dns.manualSummary': 'Προτιμώ να προσθέσω τις εγγραφές χειροκίνητα',
  'ob.people.title': 'Η ομάδα σας',
  'ob.people.heading': 'Ποιες προσκλήσεις θα στείλετε πρώτα;',
  'ob.people.description':
    'Σε κάθε διεύθυνση αποστέλλεται email με προσωπικό σύνδεσμο για δημιουργία κωδικού και σύνδεση. Μπορείτε να παραλείψετε αυτό το βήμα και να το κάνετε αργότερα.',
  'ob.people.next': 'Συνέχεια',
  'ob.people.emailsLabel': 'Διευθύνσεις email',
  'ob.people.emailsPlaceholder': 'ana@acme.com, bruno@acme.com',
  'ob.people.emailsHint':
    'Μπορείτε να επικολλήσετε πολλές μαζί, χωρισμένες με κόμματα ή αλλαγές γραμμής.',
  'ob.people.roleLabel': 'Ρόλος κατά την ένταξη',
  'ob.people.roleHint': 'Μπορεί να αλλάξει οποιαδήποτε στιγμή από το Sytue Admin.',
  'ob.people.send': 'Αποστολή προσκλήσεων',
  'ob.people.sendNote': 'Δεν αποστέλλεται τίποτα μέχρι να πατήσετε εδώ.',
  'ob.people.pendingTitle': 'Προσκλήσεις σε αναμονή αποδοχής',
  'ob.people.emptyTitle': 'Δεν έχετε στείλει ακόμη προσκλήσεις',
  'ob.people.emptyBody':
    'Μπορείτε να το κάνετε τώρα ή αργότερα, από το Sytue Admin. Ο οργανισμός σας λειτουργεί κανονικά.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Η πρόσκληση στάλθηκε',
  'ob.invite.already': 'Ήδη μέλος του οργανισμού σας',
  'ob.invite.invalid': 'Δεν μοιάζει με διεύθυνση email',
  'ob.invite.notSent': 'Δεν ήταν δυνατή η αποστολή του email',
  'ob.invite.failed': 'Η πρόσκληση απέτυχε',
  'ob.invite.default': 'Θα σταλεί email με προσωπικό σύνδεσμο για δημιουργία κωδικού και σύνδεση.',
  'ob.billing.title': 'Χρέωση',
  'ob.billing.heading': 'Σε ποιο όνομα εκδίδονται τα τιμολόγια;',
  'ob.billing.description':
    'Προσθέστε τα στοιχεία τιμολόγησης και επιλέξτε τις επί πληρωμή θέσεις. Η πρόσβαση ενεργοποιείται μόνο αφού το Mollie επιβεβαιώσει την πληρωμή.',
  'ob.billing.next': 'Συνέχεια',
  'ob.billing.pay': 'Ασφαλής πληρωμή με Mollie',
  'ob.billing.verify': 'Επαλήθευση πληρωμής',
  'ob.billing.legalLabel': 'Επωνυμία',
  'ob.billing.legalPlaceholder': 'ACME Europe Μ.Ι.Κ.Ε.',
  'ob.billing.legalHint':
    'Η νόμιμη επωνυμία της εταιρείας, ακριβώς όπως εμφανίζεται στα τιμολόγιά σας.',
  'ob.billing.taxLabel': 'Φορολογικός αριθμός',
  'ob.billing.taxPlaceholder': 'EL123456789',
  'ob.billing.taxHint': 'Ο φορολογικός αριθμός ή ο αριθμός ΦΠΑ της χώρας σας.',
  'ob.billing.addressLabel': 'Διεύθυνση',
  'ob.billing.addressPlaceholder': 'Ερμού 1, 105 63 Αθήνα, Ελλάδα',
  'ob.billing.addressHint': 'Η διεύθυνση έδρας που αναγράφεται στο τιμολόγιο.',
  'ob.billing.emailLabel': 'Email για τιμολόγια',
  'ob.billing.emailPlaceholder': 'timologia@acme.com',
  'ob.billing.emailHint':
    'Εκεί στέλνουμε τα τιμολόγια. Από προεπιλογή, το email του λογαριασμού σας.',
  'ob.billing.note':
    'Ζητάμε μόνο ό,τι χρειάζεται για την τιμολόγηση. Τίποτα από αυτά δεν κοινοποιείται σε κανέναν.',
  'ob.billing.seatsLabel': 'Επί πληρωμή θέσεις',
  'ob.billing.seatsHint': '{seats} θέση(-εις) × {price}/Χρήστη/Μήνα · σύνολο {total}/Μήνα.',
  'ob.billing.active': 'Ενεργή συνδρομή',
  'ob.billing.activeBody':
    'Το Mollie επιβεβαίωσε την πληρωμή. Η πρόσβασή σας στο Sytue One ενεργοποιήθηκε.',
  'ob.billing.pending': 'Επαληθεύουμε την πληρωμή',
  'ob.billing.pendingBody':
    'Το Mollie δεν έχει επιβεβαιώσει ακόμη την πληρωμή. Περιμένετε λίγα δευτερόλεπτα και επαληθεύστε ξανά· δεν θα γίνει διπλή χρέωση.',
  'ob.roles.title': 'Δικαιώματα',
  'ob.roles.heading': 'Θέλετε να μοιράσετε τις εργασίες διαχείρισης;',
  'ob.roles.description':
    'Είναι έτοιμοι ρόλοι: επιλέγετε όσους θέλετε και τους δημιουργούμε. Έπειτα τους αναθέτετε από το Sytue Admin σε όποια μέλη θέλετε. Μπορείτε να παραλείψετε αυτό το βήμα.',
  'ob.roles.next': 'Συνέχεια',
  'ob.roles.already': 'Έχει ήδη δημιουργηθεί',
  'ob.roles.seePermissions': 'Προβολή των ακριβών δικαιωμάτων',
  'ob.roles.helpTitle': 'Ο έλεγχος μένει σε εσάς',
  'ob.roles.helpBody':
    'Ο λογαριασμός σας είναι ο ιδιοκτήτης: αυτοί οι ρόλοι δεν του αφαιρούν τίποτα. Μπορείτε να τους επεξεργαστείτε ή να τους διαγράψετε όποτε θέλετε στο Sytue Admin → Ρόλοι.',
  'ob.done.title': 'Έγινε',
  'ob.done.headingCompleted': 'Όλα έτοιμα',
  'ob.done.heading': 'Αυτά ρυθμίσατε',
  'ob.done.description':
    'Προέρχονται από τον πραγματικό λογαριασμό σας, όχι από όσα πληκτρολογήσατε εδώ.',
  'ob.done.next': 'Είσοδος στο Sytue',
  'ob.done.warnTitle': 'Το {host} δεν έχει επαληθευτεί ακόμη',
  'ob.done.warnBody':
    'Στο μεταξύ το Sytue λειτουργεί κανονικά, με το {subdomain}. Όταν προσθέσετε την εγγραφή στον τομέα σας, επιστρέψτε στο βήμα «{step}» και πατήστε «Έλεγχος τώρα».',
  'ob.done.nextTitle': 'Πού να συνεχίσετε',
  'ob.next.devices.title': 'Συνδέστε το κινητό και τον υπολογιστή σας',
  'ob.next.devices.body':
    'Αφήστε την αλληλογραφία, το ημερολόγιο και τις επαφές να λειτουργούν στις εφαρμογές που χρησιμοποιείτε πάντα.',
  'ob.next.migrations.title': 'Φέρτε το περιεχόμενό σας από το Microsoft 365 ή το Google',
  'ob.next.migrations.body':
    'Αντιγράφουμε αλληλογραφία, αρχεία και ημερολόγια χωρίς να χαθεί τίποτα στην πορεία.',
  'ob.next.apps.title': 'Συνδέστε τις εφαρμογές σας',
  'ob.next.apps.body':
    'Τα εργαλεία που ήδη χρησιμοποιεί η ομάδα σας, συνδεδεμένα με τον λογαριασμό σας στο Sytue.',
  'ob.next.users.title': 'Συνεχίστε την εγγραφή της ομάδας σας',
  'ob.next.users.body': 'Στείλτε τις προσκλήσεις που λείπουν και ελέγξτε την κατανομή των ρόλων.',
  'ob.sum.org': 'Οργανισμός',
  'ob.sum.account': 'Ο λογαριασμός σας',
  'ob.sum.address': 'Διεύθυνση',
  'ob.sum.addressLive': '{host} · δημοσιευμένη',
  'ob.sum.addressPending': '{host} · σε αναμονή δημοσίευσης',
  'ob.sum.ownDomain': 'Δικός σας τομέας',
  'ob.sum.domainVerified': '{host} · επαληθευμένος',
  'ob.sum.domainPending': '{host} · αναμονή για την εμφάνιση της εγγραφής',
  'ob.sum.noDomain': 'Χωρίς δικό σας τομέα',
  'ob.sum.invitations': 'Προσκλήσεις',
  'ob.sum.invitationsPending': {
    one: '1 σε αναμονή αποδοχής',
    other: '{count} σε αναμονή αποδοχής',
  },
  'ob.sum.noInvitations': 'Καμία ακόμη',
  'ob.sum.billing': 'Χρέωση',
  'ob.sum.billingEmpty': 'Δεν έχει συμπληρωθεί (δεν χρειάζεται ακόμη)',
  'ob.sum.roles': 'Ρόλοι που δημιουργήθηκαν',
  'ob.sum.edit': 'Επεξεργασία',
  'ob.sum.noRoles': 'Κανένας',
  'ob.sub.liveTitle': 'Η διεύθυνσή σας έχει ήδη δημοσιευτεί',
  'ob.sub.liveBody':
    'Το {host} είναι πλέον προσβάσιμο στο διαδίκτυο. Δεν χρειάζεται να κάνετε τίποτε άλλο.',
  'ob.sub.failedTitle': 'Η διεύθυνσή σας δεν έχει δημοσιευτεί ακόμη',
  'ob.sub.failedDefault': 'Δεν μπορέσαμε να τη δημοσιεύσουμε αυτή τη στιγμή.',
  'ob.sub.failedTail':
    'Ο λογαριασμός σας έχει δημιουργηθεί: συνεχίστε τη ρύθμιση και επιστρέψτε σε λίγο.',
  'ob.sub.reservedTitle': 'Η διεύθυνσή σας έχει δεσμευτεί',
  'ob.sub.reservedBody':
    'Το {host} είναι δικό σας. Τη δημοσίευσή του στο διαδίκτυο την αναλαμβάνουμε εμείς: δεν χρειάζεται να κάνετε τίποτα.',
  'ob.search.label': 'Ο τομέας που θέλετε',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Γράψτε τον ολόκληρο, με την κατάληξη: acme.com, acme.gr, acme.eu…',
  'ob.search.check': 'Έλεγχος διαθεσιμότητας',
  'ob.search.useOwn': 'Τον έχω ήδη καταχωρίσει: σύνδεση',
  'ob.search.note':
    'Το Sytue δεν πουλά τομείς: εδώ σας λέμε μόνο αν είναι διαθέσιμος και πόσο κοστίζει. Καταχωρίστε τον όπου προτιμάτε και επιστρέψτε σε αυτό το βήμα, στην επιλογή «Έχω ήδη τομέα», για να τον συνδέσετε με τον λογαριασμό σας.',
  'ob.avail.freeTitle': 'Το {domain} είναι διαθέσιμο',
  'ob.avail.freePrice':
    'Κοστίζει {registration} τον πρώτο χρόνο και {renewal} σε κάθε ανανέωση. Αυτή είναι η σημερινή τιμή στο Cloudflare· όπου κι αν τον καταχωρίσετε, μπορεί να διαφέρει.',
  'ob.avail.freeNoPrice':
    'Μπορεί να καταχωριστεί. Την τιμή θα σας την πει ο καταχωρητής από τον οποίο θα τον αγοράσετε.',
  'ob.avail.takenTitle': 'Το {domain} είναι ήδη καταχωρισμένο',
  'ob.avail.takenBody':
    'Είναι ήδη κατειλημμένο. Δοκιμάστε άλλο όνομα ή άλλη κατάληξη (.gr, .eu, .net…). Αν ο τομέας είναι δικός σας, επιλέξτε «Έχω ήδη τομέα» παραπάνω και θα τον συνδέσουμε.',
  'ob.avail.unsupportedTitle': 'Δεν μπορούμε να ελέγξουμε αυτή την κατάληξη',
  'ob.avail.unsupportedBody':
    'Δεν μπορούμε να σας πούμε αν είναι διαθέσιμος ή πόσο κοστίζει: ελέγξτε το στον καταχωρητή σας. Όταν τον αποκτήσετε, επιστρέψτε εδώ και επιλέξτε «Έχω ήδη τομέα».',
  'ob.avail.invalidTitle': 'Δεν μοιάζει με τομέα',
  'ob.avail.invalidBody':
    'Γράψτε τον όπως το acme.com: ένα όνομα, μια τελεία και η κατάληξη. Χωρίς «https://» και χωρίς «www».',
  'ob.avail.unconfiguredTitle': 'Εδώ δεν ελέγχουμε τιμές τομέων',
  'ob.avail.unconfiguredBody':
    'Αναζητήστε τον στον καταχωρητή που προτιμάτε. Όταν τον αποκτήσετε, επιστρέψτε σε αυτό το βήμα και επιλέξτε «Έχω ήδη τομέα» για να τον συνδέσετε.',
  'ob.avail.unknownTitle': 'Δεν μπορέσαμε να το ελέγξουμε αυτή τη στιγμή',
  'ob.avail.unknownBody':
    'Δεν φταίει αυτό που πληκτρολογήσατε: δεν λάβαμε απάντηση. Δοκιμάστε ξανά σε λίγο.',
  'ob.dnsRec.title': 'Προσθέστε αυτή την εγγραφή εκεί όπου είναι καταχωρισμένος ο τομέας σας',
  'ob.dnsRec.body':
    'Βρείτε την ενότητα «DNS» ή «Εγγραφές» στον πάροχό σας, δημιουργήστε μια εγγραφή TXT και αντιγράψτε αυτές τις δύο τιμές. Η διαδικασία είναι ίδια παντού.',
  'ob.dnsRec.type': 'Τύπος',
  'ob.dnsRec.name': 'Όνομα',
  'ob.dnsRec.value': 'Τιμή',
  'ob.dnsRec.verifiedTitle': 'Ο τομέας επαληθεύτηκε',
  'ob.dnsRec.verifiedBody': 'Βρήκαμε την εγγραφή: το {host} είναι δικό σας και έτσι καταγράφεται.',
  'ob.dnsRec.mismatchTitle': 'Υπάρχει εγγραφή, αλλά όχι αυτή',
  'ob.dnsRec.mismatchBody':
    'Στο {name} βρήκαμε «{found}». Ελέγξτε ότι αντιγράψατε ολόκληρη την τιμή και ότι δεν έχει μείνει κάποια παλιά.',
  'ob.dnsRec.unreachableTitle': 'Δεν μπορέσαμε να ελέγξουμε το DNS σας',
  'ob.dnsRec.unreachableBody':
    'Δεν φταίει η εγγραφή σας: ο διακομιστής ονομάτων δεν απάντησε. Δοκιμάστε ξανά σε λίγο.',
  'ob.dnsRec.pendingTitle': 'Δεν βλέπουμε ακόμη την εγγραφή',
  'ob.dnsRec.pendingBody':
    'Αν μόλις τη δημιουργήσατε, είναι φυσιολογικό: οι αλλαγές DNS χρειάζονται από λίγα λεπτά έως μερικές ώρες για να εμφανιστούν. Μπορείτε να συνεχίσετε τη ρύθμιση και να ελέγξετε ξανά όποτε θέλετε.',
  'ob.dnsRec.uncheckedTitle': 'Δεν έχει ελεγχθεί ακόμη',
  'ob.dnsRec.uncheckedBody':
    'Όταν προσθέσετε την εγγραφή στον πάροχό σας, πατήστε «Έλεγχος τώρα» και θα ελέγξουμε αμέσως το DNS σας.',
  'ob.dnsRec.empty':
    'Όταν πατήσετε «Αποθήκευση και έλεγχος», θα σας δώσουμε την ακριβή εγγραφή που πρέπει να προσθέσετε.',
  'ob.dnsRec.check': 'Έλεγχος τώρα',
  'ob.dnsRec.save': 'Αποθήκευση και έλεγχος',
  'ob.cf.state.ok': 'Υπάρχει ήδη',
  'ob.cf.state.created': 'Δημιουργήθηκε',
  'ob.cf.state.replaced': 'Αντικαταστάθηκε',
  'ob.cf.state.missing': 'Θα την προσθέσουμε',
  'ob.cf.state.conflict': 'Υπάρχει ήδη κάτι',
  'ob.cf.state.failed': 'Δεν ήταν δυνατό',
  'ob.cf.state.pending': 'Όχι ακόμη',
  'ob.cf.permTitle': 'Δώστε μας πρόσβαση μόνο στο DNS σας',
  'ob.cf.step1': 'Ανοίξτε τα API tokens σας στο Cloudflare και πατήστε «Create token».',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2':
    'Επιλέξτε «Create custom token» και ενεργοποιήστε ένα μόνο δικαίωμα: Zone · DNS · Edit.',
  'ob.cf.step3':
    'Στο «Zone resources» επιλέξτε {domain}, δημιουργήστε το token και επικολλήστε το εδώ.',
  'ob.cf.tokenLabel': 'Token του Cloudflare',
  'ob.cf.tokenPlaceholder': 'Το token που μόλις δημιουργήσατε',
  'ob.cf.tokenHint':
    'Λειτουργεί μόνο για το DNS αυτού του τομέα. Το χρησιμοποιούμε και το απορρίπτουμε: δεν αποθηκεύεται, εκτός αν μας το ζητήσετε παρακάτω, και δεν φεύγει ποτέ από τους διακομιστές μας.',
  'ob.cf.mailLabel': 'Λήψη και της αλληλογραφίας για @{domain} στο Sytue Mail',
  'ob.cf.mailHintOn':
    'Αυτό αλλάζει πού φτάνει η αλληλογραφία του τομέα σας. Αν σήμερα τη λαμβάνετε αλλού, θα σας ειδοποιήσουμε πριν αντικαταστήσουμε οτιδήποτε.',
  'ob.cf.mailHintOff':
    'Ενεργοποιήστε το μόνο αν θέλετε τα γραμματοκιβώτιά σας @ο-τομέας-σας να μεταφερθούν στο Sytue.',
  'ob.cf.rememberLabel': 'Απομνημόνευση αυτής της πρόσβασης για μελλοντικές αλλαγές',
  'ob.cf.rememberHint':
    'Αν δεν το επιλέξετε, την επόμενη φορά θα πρέπει να επικολλήσετε ξανά το token. Με την επιλογή αποθηκεύεται κρυπτογραφημένο· με την αποεπιλογή διαγράφεται.',
  'ob.cf.preview': 'Δείτε τι θα αλλάξουμε',
  'ob.cf.previewAgain': 'Νέος έλεγχος',
  'ob.cf.nothing': 'Δεν απομένει τίποτα',
  'ob.cf.apply': { one: 'Εφαρμογή 1 αλλαγής', other: 'Εφαρμογή {count} αλλαγών' },
  'ob.cf.zone': 'Βρέθηκε ζώνη',
  'ob.cf.token': 'Το token σας',
  'ob.cf.tokenStored': 'Αποθηκεύτηκε κρυπτογραφημένο για μελλοντικές αλλαγές',
  'ob.cf.tokenDiscarded': 'Χρησιμοποιήθηκε και απορρίφθηκε (δεν το αποθηκεύσαμε)',
  'ob.cf.domainState': 'Κατάσταση τομέα',
  'ob.cf.verified': 'Επαληθευμένος',
  'ob.cf.pending': 'Σε αναμονή επαλήθευσης',
  'ob.cf.nowThere': 'Αυτή τη στιγμή υπάρχει: {current}',
  'ob.cf.replaceLabel': 'Αντικατάσταση με την εγγραφή του Sytue',
  'ob.cf.failTitle': 'Κάτι έμεινε χωρίς ρύθμιση',
  'ob.cf.failBody':
    'Τα υπόλοιπα είναι στη θέση τους. Παραπάνω βλέπετε την αιτία για κάθε εγγραφή που απέτυχε και την τιμή της, για να την προσθέσετε χειροκίνητα από το Cloudflare.',
  'ob.cf.conflictTitle': 'Υπάρχουν εγγραφές σας που δεν θα αγγίξουμε',
  'ob.cf.conflictBody':
    'Δεν αντικαθιστούμε ποτέ κάτι που ήδη έχετε, εκτός αν το επιλέξετε. Αν προτιμάτε να τις συνδυάσετε χειροκίνητα, παραπάνω βλέπετε την τρέχουσα τιμή και τη δική μας.',
  'ob.cf.okTitle': 'Ο τομέας επαληθεύτηκε',
  'ob.cf.okBody':
    'Είδαμε την εγγραφή στη δική σας ζώνη: το {host} είναι δικό σας και έτσι καταγράφεται.',
  'ob.cf.todoTitle': 'Οι αλλαγές δεν έχουν εφαρμοστεί ακόμη',
  'ob.cf.todoBody':
    'Δεν έχουμε αγγίξει ακόμη τη ζώνη σας. Πατήστε «Εφαρμογή» όταν όσα βλέπετε παραπάνω σας φαίνονται σωστά.',
  'ob.cf.someFailed': {
    one: '1 εγγραφή έμεινε χωρίς ρύθμιση',
    other: '{count} εγγραφές έμειναν χωρίς ρύθμιση',
  },
  'ob.cf.someFailedBody':
    'Παρακάτω βλέπετε την αιτία για την καθεμία και τις τιμές για να το κάνετε χειροκίνητα.',
  'ob.cf.configured': 'Το {host} ρυθμίστηκε και επαληθεύτηκε',
  'ob.cf.created': 'Εγγραφές που δημιουργήθηκαν στη ζώνη σας',
  'today.title': 'Σήμερα',
  'today.openCalendar': 'Άνοιγμα ημερολογίου',
  'today.noEvents': 'Τίποτα στο πρόγραμμα για σήμερα.',
  'today.unavailable': 'Το πρόγραμμα δεν είναι διαθέσιμο αυτή τη στιγμή.',
  'today.allDay': 'Ολοήμερο',
  'today.untitled': '(χωρίς τίτλο)',
  'today.unread': { one: '1 μη αναγνωσμένο email', other: '{count} μη αναγνωσμένα email' },
  'today.inboxZero': 'Τα εισερχόμενα είναι ενημερωμένα',
  'launcher.quickActions': 'Γρήγορες ενέργειες',
  'launcher.recentAll': 'Πρόσφατα σε όλη τη σουίτα',
  'launcher.activityError': 'Δεν μπορέσαμε να φορτώσουμε τη δραστηριότητά σας',
  'launcher.noActivity': 'Καμία πρόσφατη δραστηριότητα',
  'ob.dns.verified': 'Ο τομέας επαληθεύτηκε',
  'ob.dns.mismatch': 'Υπάρχει εγγραφή, αλλά όχι αυτή',
  'ob.dns.unreachable': 'Δεν μπορέσαμε να ελέγξουμε το DNS σας',
  'ob.dns.notYet': 'Δεν βλέπουμε ακόμη την εγγραφή',
  'ob.dns.unchecked': 'Δεν έχει ελεγχθεί ακόμη',
  'ob.dns.type': 'Τύπος',
  'ob.dns.name': 'Όνομα',
  'ob.dns.value': 'Τιμή',
  'ob.cf.tokenField': 'Token του Cloudflare',
  'ob.cf.remember': 'Απομνημόνευση αυτής της πρόσβασης για μελλοντικές αλλαγές',
  'ob.cf.replaceWithSytue': 'Αντικατάσταση με την εγγραφή του Sytue',
  'recent.hubDown':
    'Το Hub δεν αποκρίνεται αυτή τη στιγμή. Η δουλειά σας είναι ασφαλής· δοκιμάστε ξανά.',
  'search.partial':
    'Χωρίς απάντηση από: {sources}. Ίσως λείπουν ορισμένα αποτελέσματα. Δοκιμάστε ξανά σε λίγο.',
  'search.partialJoin': ' και ',
  'search.showing': 'Εμφανίζονται {count} από {total} αποτελέσματα.',
  'privacy.notice':
    'Δεν πουλάμε τα δεδομένα σας ούτε τα παραχωρούμε σε τρίτους. Δεν τα χρησιμοποιούμε για να δημιουργήσουμε προφίλ σας, για διαφήμιση, ούτε για να εκπαιδεύσουμε ή να βελτιώσουμε τις υπηρεσίες μας ή οποιουδήποτε άλλου. Είναι κρυπτογραφημένα κατά τη μεταφορά και σε ηρεμία, και φεύγουν από την ευρωπαϊκή μας υποδομή μόνο στο απολύτως απαραίτητο για τη χρέωσή σας και στις ενσωματώσεις που ενεργοποιείτε εσείς, όπου το τρίτο μέρος θα σας ζητήσει έγκριση για τα δεδομένα που θα χρησιμοποιήσει.',
};
