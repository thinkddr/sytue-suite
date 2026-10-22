/**
 * Mesaje în română pentru inteligența artificială (ia).
 * Prefix: ai.*
 */
export const ia = {
  // Setări
  'ai.settings.provider': 'Furnizor de IA',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Cheie API proprie (BYOK)',
  'ai.settings.key_placeholder': 'Introduceți cheia secretă API…',
  'ai.settings.key_help':
    'Cheia dumneavoastră este stocată criptat cu AES-256-GCM. Nu este returnată niciodată browserului.',
  'ai.settings.check_key': 'Testați cheia',
  'ai.settings.delete_key': 'Ștergeți cheia',
  'ai.settings.status_configured': 'Cheie configurată',
  'ai.settings.tested_at': 'Verificată pe',
  'ai.settings.quota_title': 'Cotă lunară de IA',
  'ai.settings.quota_reset': 'Se resetează pe {date}',
  'ai.settings.notice_eu_title': 'Notificare privind transferul de date în afara SEE',
  'ai.settings.notice_eu_desc':
    'Furnizorul selectat ({provider}) prelucrează date în afara Spațiului Economic European. Conform art. 49 alin. (1) lit. (a) din RGPD, trebuie să vă exprimați consimțământul explicit pentru acest transfer pentru a continua.',
  'ai.settings.notice_eu_accept':
    'Îmi exprim consimțământul pentru transferul internațional de date în afara SEE pentru cererile de IA către acest furnizor.',
  'ai.settings.success_save': 'Cheia a fost salvată și verificată.',
  'ai.settings.error_check': 'Eroare la verificarea cheii la furnizor.',
  'ai.settings.form_title': 'Configurați propria cheie (BYOK)',
  'ai.settings.form_help':
    'Folosiți propriile credențiale Mistral AI, OpenAI, Anthropic sau Zhipu AI pentru a nu depinde de cota lunară.',
  'ai.settings.provider_mistral': 'Mistral AI (UE, RGPD)',
  'ai.settings.provider_openai': 'OpenAI (SUA)',
  'ai.settings.provider_anthropic': 'Anthropic (SUA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'UE, Franța',
  'ai.region.us': 'SUA',
  'ai.region.intl': 'Internațional',
  'ai.settings.show_key': 'Afișați cheia',
  'ai.settings.hide_key': 'Ascundeți cheia',
  'ai.settings.key_required': 'Introduceți o cheie.',
  'ai.settings.notice_required':
    'Trebuie să acceptați notificarea privind transferul în afara SEE.',
  'ai.settings.check_ok': 'Cheia a fost verificată cu succes la furnizor.',
  'ai.settings.error_save': 'Cheia nu a putut fi salvată.',
  'ai.settings.error_delete': 'Cheia nu a putut fi ștearsă.',
  'ai.settings.checking': 'Se verifică…',
  'ai.settings.save_activate': 'Salvați și activați',
  'ai.settings.key_deleted': 'Cheia a fost ștearsă. Se va aplica cota lunară implicită.',
  'ai.settings.active_provider': 'Furnizor activ: {provider} ({model})',
  'ai.settings.no_key_default':
    'Nu este configurată o cheie proprie. Se aplică limita lunară a planului.',
  'ai.settings.status_plan': 'Cota planului',
  'ai.settings.quota_unlimited': 'Nelimitată',
  'ai.settings.quota_unlimited_help': 'Cereri gestionate cu propria cheie API.',
  'ai.settings.quota_jobs': 'Sarcini de design, traducere și export.',
  'ai.settings.loading': 'Se încarcă setările de IA',
  'ai.settings.load_error': 'Setările de IA nu au putut fi încărcate.',
  'ai.settings.load_error_help':
    'Serviciul nu a putut fi apelat. Configurația dumneavoastră nu s-a modificat.',
  'ai.settings.key_invalid': 'Furnizorul a respins cheia. Verificați dacă este corectă și activă.',
  'ai.settings.encryption_unavailable':
    'Serviciul nu este încă pregătit pentru stocarea securizată a cheilor. Contactați asistența.',
  'ai.settings.invalid_model': 'Alegeți un model valid pentru acest furnizor.',
  'ai.settings.key_state': 'Starea cheii',

  'ai.common.cancel': 'Anulați',

  // Context comun: cotă, notificare RGPD, erori și starea sarcinii
  'ai.context.quota_label': 'Cotă lunară de IA',
  'ai.context.quota_used': '{used} din {limit} sarcini în această lună',
  'ai.context.quota_reset': 'Se reînnoiește pe {date}.',
  'ai.context.quota_exhausted_title': 'Ați epuizat cota lunară de IA',
  'ai.context.quota_exhausted_desc':
    'Veți putea lansa mai multe sarcini pe {date}. Administrarea organizației poate elimina limita adăugând o cheie API proprie în setările de IA.',
  'ai.context.byok':
    'Organizația dumneavoastră folosește propria cheie API: această sarcină nu consumă din cotă.',
  'ai.context.loading': 'Se verifică cota de IA',
  'ai.context.load_error': 'Cota de IA nu a putut fi verificată. Puteți încerca totuși.',
  'ai.context.retry': 'Încercați din nou',
  'ai.notice.eu': 'Textele sunt prelucrate în UE, în conformitate cu RGPD.',
  'ai.notice.non_eu_title': 'Această sarcină iese din Spațiul Economic European',
  'ai.notice.non_eu_desc':
    'Cheia proprie a organizației dumneavoastră aparține furnizorului {provider}, care prelucrează datele în afara SEE. Organizația a acceptat acest transfer (art. 49 alin. (1) lit. (a) din RGPD) la salvarea cheii.',
  'ai.error.quota': 'Cota lunară de IA a fost epuizată. Se reînnoiește pe {date}.',
  'ai.error.quota_nodate': 'Cota lunară de IA a fost epuizată.',
  'ai.error.busy':
    'Există deja sarcini de IA în desfășurare. Așteptați finalizarea lor și încercați din nou.',
  'ai.error.admin': 'Doar persoanele care administrează organizația pot face acest lucru.',
  'ai.error.prompt': 'Descrieți site-ul înainte de a genera propunerea.',
  'ai.error.languages': 'Alegeți cel puțin o limbă țintă.',
  'ai.error.session': 'Sesiunea a expirat. Reîncărcați pagina și autentificați-vă din nou.',
  'ai.error.generic': 'Sarcina nu a putut fi pornită. Încercați din nou în câteva minute.',
  'ai.error.draft_not_saved':
    'Ciorna nu a putut fi salvată. Rezolvați problema de salvare și porniți din nou IA.',
  'ai.job.state_queued': 'În coadă',
  'ai.job.state_running': 'În curs',
  'ai.job.state_done': 'Finalizată',
  'ai.job.state_failed': 'Eșuată',
  'ai.job.queued_text': 'Se așteaptă ca serverul să preia sarcina…',
  'ai.job.running_text': 'IA lucrează. Poate dura câteva minute.',
  'ai.job.done_text': 'Sarcina s-a încheiat.',
  'ai.job.failed_text': 'Sarcina nu a putut fi finalizată.',
  'ai.job.lost_text': 'Conexiunea cu serverul a fost întreruptă.',
  'ai.job.elapsed': 'Timp scurs: {time}',
  'ai.job.detail': 'Detaliu tehnic: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'În UE',
  'ai.job.badge_non_eu': 'În afara SEE',
  'ai.job.badge_byok': 'Cheie proprie',
  'ai.job.back': 'Înapoi',
  'ai.job.retry': 'Încercați din nou',
  'ai.job.stop': 'Nu mai așteptați',
  'ai.job.stop_help': 'Sarcina va continua pe server și va fi contorizată în cotă.',

  // Design
  'ai.design.title': 'Design cu IA',
  'ai.design.desc':
    'Descrieți afacerea dumneavoastră, iar IA va propune secțiunile paginii, cu texte pregătite pentru editare.',
  'ai.design.prompt_label': 'Descrieți site-ul sau afacerea dumneavoastră',
  'ai.design.prompt_placeholder':
    'De ex. Magazin de cosmetice naturale vegane cu catalog și programări…',
  'ai.design.prompt_help':
    'Cu cât descrierea este mai concretă (ce oferiți, cui și ce doriți să facă vizitatorii pe site), cu atât propunerea va fi mai bună.',
  'ai.design.ideas_label': 'Idei de pornire',
  'ai.design.idea_1': 'Magazin gourmet de brânzeturi și vinuri artizanale cu comenzi online',
  'ai.design.idea_2': 'Birou de arhitectură și renovări complete cu galerie de proiecte',
  'ai.design.idea_3': 'Clinică stomatologică cu programări, echipă medicală și întrebări frecvente',
  'ai.design.idea_4': 'Restaurant italian cu meniu de degustare și rezervare de mese',
  'ai.design.style_label': 'Stil vizual',
  'ai.design.style_modern': 'Modern și curat',
  'ai.design.style_elegant': 'Elegant și rafinat',
  'ai.design.style_bold': 'Îndrăzneț și vibrant',
  'ai.design.style_minimal': 'Minimalist',
  'ai.design.lang_label': 'Limba conținutului',
  'ai.design.button_generate': 'Generați propunerea',
  'ai.design.job_title': 'Se creează propunerea',
  'ai.design.result_title': 'Propunerea este gata',
  'ai.design.result_sections': {
    one: '{count} secțiune gata de adăugat pe pânză.',
    few: '{count} secțiuni gata de adăugat pe pânză.',
    other: '{count} de secțiuni gata de adăugat pe pânză.',
  },
  'ai.design.result_help':
    'Vor fi adăugate în pagina pe care o editați și le veți putea ajusta ulterior.',
  'ai.design.result_empty': 'IA nu a returnat nicio secțiune utilizabilă.',
  'ai.design.button_apply': 'Adăugați pe pânză',
  'ai.design.button_again': 'Încercați altă idee',

  // Traducere
  'ai.translate.title': 'Traduceți cu IA',
  'ai.translate.desc': 'Traduceți textele site-ului în alte limbi fără a modifica designul.',
  'ai.translate.from_label': 'Limba sursă',
  'ai.translate.to_label': 'Limbi țintă',
  'ai.translate.to_help':
    'Alegeți cel puțin una. Limba sursă nu poate fi aleasă ca limbă țintă. La aplicare, noile limbi vor fi adăugate în site.',
  'ai.translate.scope_label': 'Domeniul traducerii',
  'ai.translate.scope_site': 'Întregul site',
  'ai.translate.scope_page': 'Pagina curentă',
  'ai.translate.scope_store': 'Doar catalogul magazinului',
  'ai.translate.store_unavailable': 'Traducerea catalogului magazinului nu este încă disponibilă.',
  'ai.translate.scope_invalid': 'Alegeți o pagină existentă sau întregul site pentru traducere.',
  'ai.translate.overwrite_label': 'Suprascrieți traducerile deja existente',
  'ai.translate.button_translate': 'Traduceți',
  'ai.translate.job_title': 'Se traduc textele',
  'ai.translate.result_title': 'Traducerile sunt gata',
  'ai.translate.result_texts': {
    one: '{count} text tradus.',
    few: '{count} texte traduse.',
    other: '{count} de texte traduse.',
  },
  'ai.translate.result_help': 'Vor fi adăugate în ciornă; designul nu se modifică.',
  'ai.translate.result_empty': 'IA nu a returnat nicio traducere.',
  'ai.translate.button_apply': 'Aplicați traducerile',
  'ai.translate.button_again': 'Traduceți în alte limbi',

  // Export
  'ai.export.title': 'Exportați cu IA',
  'ai.export.desc':
    'Generați un pachet cu site-ul dumneavoastră pregătit pentru o altă platformă, fără a depinde de Sytue.',
  'ai.export.destination_label': 'Platforma de destinație',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Temă completă cu șabloane JSON și secțiuni Liquid, plus catalogul în format CSV pentru import din panoul Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Proiect single-page conectat la API-ul public Sytue, cu finalizare de comandă și formulare.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Aplicație randată pe server sau statică, cu procesatorul de plăți integrat.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Temă nativă cu blocuri WordPress, cu export de produse și atribute.',
  'ai.export.badge_available': 'Disponibil',
  'ai.export.badge_soon': 'În curând',
  'ai.export.button_export': 'Generați pachetul',
  'ai.export.job_title': 'Se generează pachetul pentru {destination}',
  'ai.export.result_title': 'Pachetul este gata',
  'ai.export.result_help':
    'Tema și fișierul CSV al catalogului sunt incluse într-o singură arhivă ZIP.',
  'ai.export.steps_title': 'Cum se instalează în Shopify',
  'ai.export.step_1': 'Descărcați fișierul ZIP.',
  'ai.export.step_2':
    'În panoul de administrare Shopify, accesați Online Store > Themes și alegeți „Upload zip file”.',
  'ai.export.step_3':
    'Pentru produse, accesați Products > Import și încărcați fișierul products_export_shopify.csv din interiorul arhivei ZIP.',
  'ai.export.button_download': 'Descărcați ZIP',
  'ai.export.button_again': 'Generați un alt export',
};
