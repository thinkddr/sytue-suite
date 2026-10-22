/**
 * Μηνύματα στα ελληνικά για την τεχνητή νοημοσύνη (ia).
 * Πρόθεμα: ai.*
 */
export const ia = {
  // Ρυθμίσεις
  'ai.settings.provider': 'Πάροχος ΤΝ',
  'ai.settings.model': 'Μοντέλο',
  'ai.settings.key': 'Ίδιο κλειδί API (BYOK)',
  'ai.settings.key_placeholder': 'Εισαγάγετε το μυστικό κλειδί API σας…',
  'ai.settings.key_help':
    'Το κλειδί σας αποθηκεύεται κρυπτογραφημένο με AES-256-GCM. Δεν επιστρέφεται ποτέ στο πρόγραμμα περιήγησης.',
  'ai.settings.check_key': 'Δοκιμή κλειδιού',
  'ai.settings.delete_key': 'Διαγραφή κλειδιού',
  'ai.settings.status_configured': 'Το κλειδί ρυθμίστηκε',
  'ai.settings.tested_at': 'Επαληθεύτηκε στις',
  'ai.settings.quota_title': 'Μηνιαίο όριο ΤΝ',
  'ai.settings.quota_reset': 'Μηδενίζεται στις {date}',
  'ai.settings.notice_eu_title': 'Ειδοποίηση διαβίβασης δεδομένων εκτός ΕΟΧ',
  'ai.settings.notice_eu_desc':
    'Ο επιλεγμένος πάροχος ({provider}) επεξεργάζεται δεδομένα εκτός του Ευρωπαϊκού Οικονομικού Χώρου. Σύμφωνα με το άρθρο 49 παρ. 1 στοιχ. α του ΓΚΠΔ, πρέπει να συναινέσετε ρητά σε αυτή τη διαβίβαση για να συνεχίσετε.',
  'ai.settings.notice_eu_accept':
    'Συναινώ στη διεθνή διαβίβαση δεδομένων εκτός ΕΟΧ για αιτήματα ΤΝ με αυτόν τον πάροχο.',
  'ai.settings.success_save': 'Το κλειδί αποθηκεύτηκε και επαληθεύτηκε με επιτυχία.',
  'ai.settings.error_check': 'Σφάλμα κατά την επαλήθευση του κλειδιού στον πάροχο.',
  'ai.settings.form_title': 'Ρύθμιση δικού σας κλειδιού (BYOK)',
  'ai.settings.form_help':
    'Χρησιμοποιήστε τα δικά σας διαπιστευτήρια Mistral AI, OpenAI, Anthropic ή Zhipu AI ώστε να μην εξαρτάστε από το μηνιαίο όριο.',
  'ai.settings.provider_mistral': 'Mistral AI (ΕΕ, ΓΚΠΔ)',
  'ai.settings.provider_openai': 'OpenAI (ΗΠΑ)',
  'ai.settings.provider_anthropic': 'Anthropic (ΗΠΑ)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'ΕΕ, Γαλλία',
  'ai.region.us': 'ΗΠΑ',
  'ai.region.intl': 'Διεθνώς',
  'ai.settings.show_key': 'Εμφάνιση κλειδιού',
  'ai.settings.hide_key': 'Απόκρυψη κλειδιού',
  'ai.settings.key_required': 'Εισαγάγετε ένα κλειδί.',
  'ai.settings.notice_required':
    'Πρέπει να αποδεχτείτε την ειδοποίηση για τις διαβιβάσεις εκτός ΕΟΧ.',
  'ai.settings.check_ok': 'Το κλειδί ελέγχθηκε με επιτυχία στον πάροχο.',
  'ai.settings.error_save': 'Δεν ήταν δυνατή η αποθήκευση του κλειδιού.',
  'ai.settings.error_delete': 'Δεν ήταν δυνατή η διαγραφή του κλειδιού.',
  'ai.settings.checking': 'Έλεγχος…',
  'ai.settings.save_activate': 'Αποθήκευση και ενεργοποίηση',
  'ai.settings.key_deleted': 'Το κλειδί διαγράφηκε. Θα ισχύει το προεπιλεγμένο μηνιαίο όριο.',
  'ai.settings.active_provider': 'Ενεργός πάροχος: {provider} ({model})',
  'ai.settings.no_key_default':
    'Δεν έχει οριστεί δικό σας κλειδί. Ισχύει το μηνιαίο όριο του προγράμματος.',
  'ai.settings.status_plan': 'Όριο προγράμματος',
  'ai.settings.quota_unlimited': 'Απεριόριστο',
  'ai.settings.quota_unlimited_help': 'Αιτήματα που εκτελούνται με το δικό σας κλειδί API.',
  'ai.settings.quota_jobs': 'Εργασίες σχεδίασης, μετάφρασης και εξαγωγής.',
  'ai.settings.loading': 'Φόρτωση ρυθμίσεων ΤΝ',
  'ai.settings.load_error': 'Δεν ήταν δυνατή η φόρτωση των ρυθμίσεων ΤΝ.',
  'ai.settings.load_error_help':
    'Δεν ήταν δυνατή η επικοινωνία με την υπηρεσία. Οι ρυθμίσεις σας δεν άλλαξαν.',
  'ai.settings.key_invalid': 'Ο πάροχος απέρριψε το κλειδί. Ελέγξτε ότι είναι σωστό και ενεργό.',
  'ai.settings.encryption_unavailable':
    'Η υπηρεσία δεν είναι ακόμη έτοιμη να αποθηκεύσει κλειδιά με ασφάλεια. Επικοινωνήστε με την υποστήριξη.',
  'ai.settings.invalid_model': 'Επιλέξτε ένα έγκυρο μοντέλο για αυτόν τον πάροχο.',
  'ai.settings.key_state': 'Κατάσταση κλειδιού',

  'ai.common.cancel': 'Ακύρωση',

  // Κοινό πλαίσιο: όριο, ειδοποίηση ΓΚΠΔ, σφάλματα και κατάσταση εργασίας
  'ai.context.quota_label': 'Μηνιαίο όριο ΤΝ',
  'ai.context.quota_used': '{used} από {limit} εργασίες αυτόν τον μήνα',
  'ai.context.quota_reset': 'Ανανεώνεται στις {date}.',
  'ai.context.quota_exhausted_title': 'Εξαντλήσατε το μηνιαίο όριο ΤΝ',
  'ai.context.quota_exhausted_desc':
    'Θα μπορείτε να ξεκινήσετε νέες εργασίες στις {date}. Η διαχείριση του οργανισμού μπορεί να καταργήσει το όριο προσθέτοντας δικό της κλειδί API στις ρυθμίσεις ΤΝ.',
  'ai.context.byok':
    'Ο οργανισμός σας χρησιμοποιεί δικό του κλειδί API: αυτή η εργασία δεν καταναλώνει από το όριο.',
  'ai.context.loading': 'Έλεγχος ορίου ΤΝ',
  'ai.context.load_error':
    'Δεν ήταν δυνατός ο έλεγχος του ορίου ΤΝ. Μπορείτε πάντως να δοκιμάσετε.',
  'ai.context.retry': 'Δοκιμάστε ξανά',
  'ai.notice.eu': 'Τα κείμενα υποβάλλονται σε επεξεργασία εντός της ΕΕ, σύμφωνα με τον ΓΚΠΔ.',
  'ai.notice.non_eu_title': 'Αυτή η εργασία εξέρχεται από τον Ευρωπαϊκό Οικονομικό Χώρο',
  'ai.notice.non_eu_desc':
    'Το κλειδί του οργανισμού σας ανήκει στον πάροχο {provider}, ο οποίος επεξεργάζεται δεδομένα εκτός ΕΟΧ. Ο οργανισμός σας αποδέχτηκε αυτή τη διαβίβαση (άρθρο 49 παρ. 1 στοιχ. α του ΓΚΠΔ) κατά την αποθήκευση του κλειδιού.',
  'ai.error.quota': 'Το μηνιαίο όριο ΤΝ εξαντλήθηκε. Ανανεώνεται στις {date}.',
  'ai.error.quota_nodate': 'Το μηνιαίο όριο ΤΝ εξαντλήθηκε.',
  'ai.error.busy': 'Εκτελούνται ήδη εργασίες ΤΝ. Περιμένετε να ολοκληρωθούν και δοκιμάστε ξανά.',
  'ai.error.admin': 'Μόνο η διαχείριση του οργανισμού μπορεί να το κάνει αυτό.',
  'ai.error.prompt': 'Περιγράψτε τον ιστότοπό σας πριν δημιουργήσετε την πρόταση.',
  'ai.error.languages': 'Επιλέξτε τουλάχιστον μία γλώσσα προορισμού.',
  'ai.error.session': 'Η συνεδρία σας έληξε. Επαναφορτώστε τη σελίδα και συνδεθείτε ξανά.',
  'ai.error.generic': 'Δεν ήταν δυνατή η έναρξη της εργασίας. Δοκιμάστε ξανά σε λίγα λεπτά.',
  'ai.error.draft_not_saved':
    'Δεν ήταν δυνατή η αποθήκευση του προχείρου. Επιλύστε το πρόβλημα αποθήκευσης και ξεκινήστε ξανά την ΤΝ.',
  'ai.job.state_queued': 'Σε ουρά',
  'ai.job.state_running': 'Σε εξέλιξη',
  'ai.job.state_done': 'Ολοκληρώθηκε',
  'ai.job.state_failed': 'Απέτυχε',
  'ai.job.queued_text': 'Αναμονή αποδοχής της εργασίας από τον διακομιστή…',
  'ai.job.running_text': 'Η ΤΝ εργάζεται. Αυτό μπορεί να διαρκέσει λίγα λεπτά.',
  'ai.job.done_text': 'Η εργασία ολοκληρώθηκε.',
  'ai.job.failed_text': 'Δεν ήταν δυνατή η ολοκλήρωση της εργασίας.',
  'ai.job.lost_text': 'Χάθηκε η σύνδεση με τον διακομιστή.',
  'ai.job.elapsed': 'Χρόνος που πέρασε: {time}',
  'ai.job.detail': 'Τεχνική λεπτομέρεια: {detail}',
  'ai.job.model': 'Μοντέλο: {model}',
  'ai.job.badge_eu': 'Εντός ΕΕ',
  'ai.job.badge_non_eu': 'Εκτός ΕΟΧ',
  'ai.job.badge_byok': 'Ίδιο κλειδί',
  'ai.job.back': 'Πίσω',
  'ai.job.retry': 'Δοκιμάστε ξανά',
  'ai.job.stop': 'Διακοπή αναμονής',
  'ai.job.stop_help': 'Η εργασία θα συνεχίσει στον διακομιστή και θα προσμετρηθεί στο όριό σας.',

  // Σχεδίαση
  'ai.design.title': 'Σχεδίαση με ΤΝ',
  'ai.design.desc':
    'Περιγράψτε την επιχείρησή σας και η ΤΝ θα προτείνει τις ενότητες της σελίδας, με κείμενα έτοιμα για επεξεργασία.',
  'ai.design.prompt_label': 'Περιγράψτε τον ιστότοπο ή την επιχείρησή σας',
  'ai.design.prompt_placeholder':
    'Π.χ. Κατάστημα φυσικών vegan καλλυντικών με κατάλογο και ραντεβού…',
  'ai.design.prompt_help':
    'Όσο πιο συγκεκριμένη είναι η περιγραφή (τι προσφέρετε, σε ποιους και τι θέλετε να κάνουν όσοι επισκέπτονται τον ιστότοπο), τόσο καλύτερη θα είναι η πρόταση.',
  'ai.design.ideas_label': 'Ιδέες για να ξεκινήσετε',
  'ai.design.idea_1': 'Κατάστημα εκλεκτών τυριών και κρασιών με ηλεκτρονικές παραγγελίες',
  'ai.design.idea_2': 'Αρχιτεκτονικό γραφείο και ανακαινίσεις με συλλογή έργων',
  'ai.design.idea_3': 'Οδοντιατρική κλινική με ραντεβού, ιατρική ομάδα και συχνές ερωτήσεις',
  'ai.design.idea_4': 'Ιταλικό εστιατόριο με μενού γευσιγνωσίας και κράτηση τραπεζιού',
  'ai.design.style_label': 'Οπτικό στιλ',
  'ai.design.style_modern': 'Μοντέρνο και καθαρό',
  'ai.design.style_elegant': 'Κομψό και εκλεπτυσμένο',
  'ai.design.style_bold': 'Τολμηρό και ζωντανό',
  'ai.design.style_minimal': 'Μινιμαλιστικό',
  'ai.design.lang_label': 'Γλώσσα περιεχομένου',
  'ai.design.button_generate': 'Δημιουργία πρότασης',
  'ai.design.job_title': 'Σχεδίαση της πρότασης',
  'ai.design.result_title': 'Η πρόταση είναι έτοιμη',
  'ai.design.result_sections': {
    one: '{count} ενότητα έτοιμη για προσθήκη στον καμβά.',
    other: '{count} ενότητες έτοιμες για προσθήκη στον καμβά.',
  },
  'ai.design.result_help':
    'Θα προστεθούν στη σελίδα που επεξεργάζεστε και θα μπορείτε να τις προσαρμόσετε στη συνέχεια.',
  'ai.design.result_empty': 'Η ΤΝ δεν επέστρεψε καμία χρησιμοποιήσιμη ενότητα.',
  'ai.design.button_apply': 'Προσθήκη στον καμβά',
  'ai.design.button_again': 'Δοκιμή άλλης ιδέας',

  // Μετάφραση
  'ai.translate.title': 'Μετάφραση με ΤΝ',
  'ai.translate.desc':
    'Μεταφράστε τα κείμενα του ιστοτόπου σε άλλες γλώσσες χωρίς να αλλάξει η σχεδίαση.',
  'ai.translate.from_label': 'Γλώσσα προέλευσης',
  'ai.translate.to_label': 'Γλώσσες προορισμού',
  'ai.translate.to_help':
    'Επιλέξτε τουλάχιστον μία. Η γλώσσα προέλευσης δεν μπορεί να επιλεγεί ως προορισμός. Με την εφαρμογή προστίθενται οι νέες γλώσσες στον ιστότοπο.',
  'ai.translate.scope_label': 'Εύρος μετάφρασης',
  'ai.translate.scope_site': 'Ολόκληρος ο ιστότοπος',
  'ai.translate.scope_page': 'Τρέχουσα σελίδα',
  'ai.translate.scope_store': 'Μόνο ο κατάλογος του καταστήματος',
  'ai.translate.store_unavailable':
    'Η μετάφραση του καταλόγου του καταστήματος δεν είναι ακόμη διαθέσιμη.',
  'ai.translate.scope_invalid':
    'Επιλέξτε μια υπάρχουσα σελίδα ή ολόκληρο τον ιστότοπο για μετάφραση.',
  'ai.translate.overwrite_label': 'Αντικατάσταση μεταφράσεων που υπάρχουν ήδη',
  'ai.translate.button_translate': 'Μετάφραση',
  'ai.translate.job_title': 'Μετάφραση των κειμένων',
  'ai.translate.result_title': 'Οι μεταφράσεις είναι έτοιμες',
  'ai.translate.result_texts': {
    one: '{count} κείμενο μεταφράστηκε.',
    other: '{count} κείμενα μεταφράστηκαν.',
  },
  'ai.translate.result_help': 'Θα προστεθούν στο πρόχειρο· η σχεδίαση δεν αλλάζει.',
  'ai.translate.result_empty': 'Η ΤΝ δεν επέστρεψε καμία μετάφραση.',
  'ai.translate.button_apply': 'Εφαρμογή μεταφράσεων',
  'ai.translate.button_again': 'Μετάφραση σε άλλες γλώσσες',

  // Εξαγωγή
  'ai.export.title': 'Εξαγωγή με ΤΝ',
  'ai.export.desc':
    'Δημιουργήστε ένα πακέτο με τον ιστότοπό σας έτοιμο για μεταφορά σε άλλη πλατφόρμα, χωρίς εξάρτηση από το Sytue.',
  'ai.export.destination_label': 'Πλατφόρμα προορισμού',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Πλήρες θέμα με πρότυπα JSON και ενότητες Liquid, μαζί με τον κατάλογο σε CSV για εισαγωγή από τον πίνακα διαχείρισης του Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Έργο μίας σελίδας συνδεδεμένο στο δημόσιο API του Sytue, με ολοκλήρωση αγοράς και φόρμες.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Εφαρμογή με απόδοση στον διακομιστή ή στατική, με ενσωματωμένη την πύλη πληρωμών.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Εγγενές θέμα μπλοκ του WordPress, με εξαγωγή προϊόντων και χαρακτηριστικών.',
  'ai.export.badge_available': 'Διαθέσιμο',
  'ai.export.badge_soon': 'Σύντομα',
  'ai.export.button_export': 'Δημιουργία πακέτου',
  'ai.export.job_title': 'Δημιουργία πακέτου για {destination}',
  'ai.export.result_title': 'Το πακέτο είναι έτοιμο',
  'ai.export.result_help':
    'Το θέμα και το CSV του καταλόγου περιλαμβάνονται μαζί σε ένα ενιαίο ZIP.',
  'ai.export.steps_title': 'Πώς να το εγκαταστήσετε στο Shopify',
  'ai.export.step_1': 'Κατεβάστε το αρχείο ZIP.',
  'ai.export.step_2':
    'Στον πίνακα διαχείρισης του Shopify, μεταβείτε στο Online Store > Themes και επιλέξτε «Upload zip file».',
  'ai.export.step_3':
    'Για τα προϊόντα, μεταβείτε στο Products > Import και ανεβάστε το αρχείο products_export_shopify.csv που βρίσκεται μέσα στο ZIP.',
  'ai.export.button_download': 'Λήψη ZIP',
  'ai.export.button_again': 'Δημιουργία άλλης εξαγωγής',
};
