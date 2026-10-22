/**
 * Testi italiani dell’intelligenza artificiale (ia).
 * Prefisso: ai.*
 */
export const ia = {
  // Impostazioni
  'ai.settings.provider': 'Provider di IA',
  'ai.settings.model': 'Modello',
  'ai.settings.key': 'Chiave API propria (BYOK)',
  'ai.settings.key_placeholder': 'Inserisci la tua chiave API segreta…',
  'ai.settings.key_help':
    'La chiave viene conservata cifrata con AES-256-GCM. Non viene mai restituita al browser.',
  'ai.settings.check_key': 'Verifica chiave',
  'ai.settings.delete_key': 'Elimina chiave',
  'ai.settings.status_configured': 'Chiave configurata',
  'ai.settings.tested_at': 'Verificata il',
  'ai.settings.quota_title': 'Quota mensile di IA',
  'ai.settings.quota_reset': 'Si azzera il {date}',
  'ai.settings.notice_eu_title': 'Avviso di trasferimento di dati fuori dal SEE',
  'ai.settings.notice_eu_desc':
    'Il provider selezionato ({provider}) tratta i dati fuori dallo Spazio economico europeo. Ai sensi dell’art. 49, par. 1, lett. a) del GDPR, per continuare devi acconsentire espressamente a questo trasferimento.',
  'ai.settings.notice_eu_accept':
    'Acconsento al trasferimento internazionale dei dati fuori dal SEE per le richieste di IA a questo provider.',
  'ai.settings.success_save': 'Chiave salvata e verificata.',
  'ai.settings.error_check': 'Errore durante la verifica della chiave con il provider.',
  'ai.settings.form_title': 'Configura la tua chiave (BYOK)',
  'ai.settings.form_help':
    'Usa le tue credenziali di Mistral AI, OpenAI, Anthropic o Zhipu AI per non dipendere dalla quota mensile.',
  'ai.settings.provider_mistral': 'Mistral AI (UE, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'UE, Francia',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Internazionale',
  'ai.settings.show_key': 'Mostra chiave',
  'ai.settings.hide_key': 'Nascondi chiave',
  'ai.settings.key_required': 'Inserisci una chiave.',
  'ai.settings.notice_required': 'Devi accettare l’avviso sui trasferimenti fuori dal SEE.',
  'ai.settings.check_ok': 'Chiave verificata correttamente con il provider.',
  'ai.settings.error_save': 'Impossibile salvare la chiave.',
  'ai.settings.error_delete': 'Impossibile eliminare la chiave.',
  'ai.settings.checking': 'Verifica in corso…',
  'ai.settings.save_activate': 'Salva e attiva',
  'ai.settings.key_deleted': 'Chiave eliminata. Si applicherà la quota mensile predefinita.',
  'ai.settings.active_provider': 'Provider attivo: {provider} ({model})',
  'ai.settings.no_key_default':
    'Nessuna chiave propria configurata. Si applica il limite mensile del piano.',
  'ai.settings.status_plan': 'Quota del piano',
  'ai.settings.quota_unlimited': 'Illimitata',
  'ai.settings.quota_unlimited_help': 'Richieste gestite con la tua chiave API.',
  'ai.settings.quota_jobs': 'Lavori di design, traduzione ed esportazione.',
  'ai.settings.loading': 'Caricamento delle impostazioni di IA',
  'ai.settings.load_error': 'Impossibile caricare le impostazioni di IA.',
  'ai.settings.load_error_help':
    'Non è stato possibile raggiungere il servizio. Le tue impostazioni non sono cambiate.',
  'ai.settings.key_invalid':
    'Il provider ha rifiutato la chiave. Controlla che sia corretta e attiva.',
  'ai.settings.encryption_unavailable':
    'Il servizio non è ancora pronto per conservare le chiavi in modo sicuro. Contatta l’assistenza.',
  'ai.settings.invalid_model': 'Scegli un modello valido per questo provider.',
  'ai.settings.key_state': 'Stato della chiave',

  'ai.common.cancel': 'Annulla',

  // Contesto comune: quota, avviso GDPR, errori e stato dei lavori
  'ai.context.quota_label': 'Quota mensile di IA',
  'ai.context.quota_used': '{used} di {limit} lavori questo mese',
  'ai.context.quota_reset': 'Si rinnova il {date}.',
  'ai.context.quota_exhausted_title': 'La quota mensile di IA è esaurita',
  'ai.context.quota_exhausted_desc':
    'Potrai avviare altri lavori il {date}. Chi amministra l’organizzazione può togliere il limite aggiungendo una propria chiave API nelle impostazioni di IA.',
  'ai.context.byok':
    'La tua organizzazione usa una propria chiave API: questo lavoro non consuma quota.',
  'ai.context.loading': 'Verifica della quota di IA',
  'ai.context.load_error': 'Impossibile verificare la quota di IA. Puoi comunque provare.',
  'ai.context.retry': 'Riprova',
  'ai.notice.eu': 'I testi vengono trattati all’interno dell’UE, in conformità al GDPR.',
  'ai.notice.non_eu_title': 'Questo lavoro esce dallo Spazio economico europeo',
  'ai.notice.non_eu_desc':
    'La chiave propria della tua organizzazione appartiene a {provider}, che tratta i dati fuori dal SEE. La tua organizzazione ha accettato questo trasferimento (art. 49, par. 1, lett. a) del GDPR) al momento di salvare la chiave.',
  'ai.error.quota': 'La quota mensile di IA è esaurita. Si rinnova il {date}.',
  'ai.error.quota_nodate': 'La quota mensile di IA è esaurita.',
  'ai.error.busy': 'Ci sono già lavori di IA in corso. Attendi che finiscano e riprova.',
  'ai.error.admin': 'Solo chi amministra l’organizzazione può farlo.',
  'ai.error.prompt': 'Descrivi il tuo sito prima di generare la proposta.',
  'ai.error.languages': 'Scegli almeno una lingua di destinazione.',
  'ai.error.session': 'La sessione è scaduta. Ricarica la pagina e accedi di nuovo.',
  'ai.error.generic': 'Impossibile avviare il lavoro. Riprova tra qualche minuto.',
  'ai.error.draft_not_saved':
    'Impossibile salvare la bozza. Risolvi il problema di salvataggio e avvia di nuovo l’IA.',
  'ai.job.state_queued': 'In coda',
  'ai.job.state_running': 'In corso',
  'ai.job.state_done': 'Completato',
  'ai.job.state_failed': 'Non riuscito',
  'ai.job.queued_text': 'In attesa che il server accetti il lavoro…',
  'ai.job.running_text': 'L’IA è al lavoro. Può richiedere alcuni minuti.',
  'ai.job.done_text': 'Il lavoro è terminato.',
  'ai.job.failed_text': 'Impossibile completare il lavoro.',
  'ai.job.lost_text': 'La connessione al server si è interrotta.',
  'ai.job.elapsed': 'Tempo trascorso: {time}',
  'ai.job.detail': 'Dettaglio tecnico: {detail}',
  'ai.job.model': 'Modello: {model}',
  'ai.job.badge_eu': 'Nell’UE',
  'ai.job.badge_non_eu': 'Fuori dal SEE',
  'ai.job.badge_byok': 'Chiave propria',
  'ai.job.back': 'Indietro',
  'ai.job.retry': 'Riprova',
  'ai.job.stop': 'Smetti di attendere',
  'ai.job.stop_help': 'Il lavoro continuerà sul server e sarà conteggiato nella tua quota.',

  // Design
  'ai.design.title': 'Progetta con l’IA',
  'ai.design.desc':
    'Descrivi la tua attività e l’IA proporrà le sezioni della pagina, con testi pronti da modificare.',
  'ai.design.prompt_label': 'Descrivi il tuo sito o la tua attività',
  'ai.design.prompt_placeholder':
    'Es.: negozio di moda donna sostenibile con catalogo e prenotazione appuntamenti…',
  'ai.design.prompt_help':
    'Più la descrizione è precisa (cosa offri, a chi e cosa devono fare le persone che visitano il sito), migliore sarà la proposta.',
  'ai.design.ideas_label': 'Idee per iniziare',
  'ai.design.idea_1': 'Gastronomia di formaggi e vini artigianali con ordini online',
  'ai.design.idea_2': 'Studio di architettura e ristrutturazioni con galleria di progetti',
  'ai.design.idea_3': 'Studio dentistico con prenotazioni, équipe medica e domande frequenti',
  'ai.design.idea_4': 'Ristorante italiano con menu degustazione e prenotazione del tavolo',
  'ai.design.style_label': 'Stile visivo',
  'ai.design.style_modern': 'Moderno e pulito',
  'ai.design.style_elegant': 'Elegante e raffinato',
  'ai.design.style_bold': 'Audace e vivace',
  'ai.design.style_minimal': 'Minimalista',
  'ai.design.lang_label': 'Lingua dei contenuti',
  'ai.design.button_generate': 'Genera proposta',
  'ai.design.job_title': 'Progettazione della proposta',
  'ai.design.result_title': 'Proposta pronta',
  'ai.design.result_sections': {
    one: '{count} sezione pronta da aggiungere all’area di lavoro.',
    many: '{count} sezioni pronte da aggiungere all’area di lavoro.',
    other: '{count} sezioni pronte da aggiungere all’area di lavoro.',
  },
  'ai.design.result_help':
    'Verranno aggiunte alla pagina che stai modificando e potrai regolarle in seguito.',
  'ai.design.result_empty': 'L’IA non ha restituito nessuna sezione utilizzabile.',
  'ai.design.button_apply': 'Aggiungi all’area di lavoro',
  'ai.design.button_again': 'Prova un’altra idea',

  // Traduzione
  'ai.translate.title': 'Traduci con l’IA',
  'ai.translate.desc': 'Traduci i testi del sito in altre lingue senza toccare il design.',
  'ai.translate.from_label': 'Lingua di origine',
  'ai.translate.to_label': 'Lingue di destinazione',
  'ai.translate.to_help':
    'Scegline almeno una. La lingua di origine non può essere una destinazione. Applicando il risultato si aggiungono nuove lingue al sito.',
  'ai.translate.scope_label': 'Ambito della traduzione',
  'ai.translate.scope_site': 'Intero sito',
  'ai.translate.scope_page': 'Pagina attuale',
  'ai.translate.scope_store': 'Solo catalogo del negozio',
  'ai.translate.store_unavailable':
    'La traduzione del catalogo del negozio non è ancora disponibile.',
  'ai.translate.scope_invalid': 'Scegli una pagina esistente o l’intero sito da tradurre.',
  'ai.translate.overwrite_label': 'Sovrascrivi le traduzioni già esistenti',
  'ai.translate.button_translate': 'Traduci',
  'ai.translate.job_title': 'Traduzione dei testi',
  'ai.translate.result_title': 'Traduzioni pronte',
  'ai.translate.result_texts': {
    one: '{count} testo tradotto.',
    many: '{count} testi tradotti.',
    other: '{count} testi tradotti.',
  },
  'ai.translate.result_help': 'Verranno aggiunte alla bozza; il design non cambia.',
  'ai.translate.result_empty': 'L’IA non ha restituito nessuna traduzione.',
  'ai.translate.button_apply': 'Applica le traduzioni',
  'ai.translate.button_again': 'Traduci in altre lingue',

  // Esportazione
  'ai.export.title': 'Esporta con l’IA',
  'ai.export.desc':
    'Genera un pacchetto con il tuo sito, pronto da portare su un’altra piattaforma, senza dipendere da Sytue.',
  'ai.export.destination_label': 'Piattaforma di destinazione',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Tema completo con modelli JSON e sezioni Liquid, più il catalogo in CSV da importare dal pannello di Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Progetto a pagina singola collegato all’API pubblica di Sytue, con checkout e moduli.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Applicazione con rendering lato server o statica, con il gateway di pagamento integrato.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Tema a blocchi nativo di WordPress, con esportazione di prodotti e attributi.',
  'ai.export.badge_available': 'Disponibile',
  'ai.export.badge_soon': 'Prossimamente',
  'ai.export.button_export': 'Genera pacchetto',
  'ai.export.job_title': 'Generazione del pacchetto per {destination}',
  'ai.export.result_title': 'Pacchetto pronto',
  'ai.export.result_help': 'Il tema e il CSV del catalogo sono insieme in un unico ZIP.',
  'ai.export.steps_title': 'Come installarlo in Shopify',
  'ai.export.step_1': 'Scarica il file ZIP.',
  'ai.export.step_2':
    'Nel pannello di Shopify vai su Negozio online > Temi e scegli «Carica file zip».',
  'ai.export.step_3':
    'Per i prodotti vai su Prodotti > Importa e carica il file products_export_shopify.csv contenuto nello ZIP.',
  'ai.export.button_download': 'Scarica ZIP',
  'ai.export.button_again': 'Genera un’altra esportazione',
};
