/**
 * Tekster på norsk bokmål for kunstig intelligens (ia).
 * Prefiks: ai.*
 */
export const ia = {
  // Innstillinger
  'ai.settings.provider': 'KI-leverandør',
  'ai.settings.model': 'Modell',
  'ai.settings.key': 'Egen API-nøkkel (BYOK)',
  'ai.settings.key_placeholder': 'Skriv inn den hemmelige API-nøkkelen din…',
  'ai.settings.key_help':
    'Nøkkelen din lagres kryptert med AES-256-GCM. Den sendes aldri tilbake til nettleseren.',
  'ai.settings.check_key': 'Test nøkkel',
  'ai.settings.delete_key': 'Slett nøkkel',
  'ai.settings.status_configured': 'Nøkkel konfigurert',
  'ai.settings.tested_at': 'Bekreftet',
  'ai.settings.quota_title': 'Månedlig KI-kvote',
  'ai.settings.quota_reset': 'Nullstilles {date}',
  'ai.settings.notice_eu_title': 'Varsel om dataoverføring utenfor EØS',
  'ai.settings.notice_eu_desc':
    'Den valgte leverandøren ({provider}) behandler data utenfor Det europeiske økonomiske samarbeidsområde. Etter GDPR art. 49 nr. 1 bokstav a må du gi uttrykkelig samtykke til denne overføringen for å fortsette.',
  'ai.settings.notice_eu_accept':
    'Jeg samtykker til internasjonal overføring av data utenfor EØS for KI-forespørsler hos denne leverandøren.',
  'ai.settings.success_save': 'Nøkkelen er lagret og bekreftet.',
  'ai.settings.error_check': 'Feil ved bekreftelse av nøkkelen hos leverandøren.',
  'ai.settings.form_title': 'Sett opp egen nøkkel (BYOK)',
  'ai.settings.form_help':
    'Bruk egne nøkler fra Mistral AI, OpenAI, Anthropic eller Zhipu AI for å slippe å være avhengig av den månedlige kvoten.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Frankrike',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Internasjonal',
  'ai.settings.show_key': 'Vis nøkkel',
  'ai.settings.hide_key': 'Skjul nøkkel',
  'ai.settings.key_required': 'Skriv inn en nøkkel.',
  'ai.settings.notice_required': 'Du må godta varselet om overføring utenfor EØS.',
  'ai.settings.check_ok': 'Nøkkelen ble bekreftet hos leverandøren.',
  'ai.settings.error_save': 'Nøkkelen kunne ikke lagres.',
  'ai.settings.error_delete': 'Nøkkelen kunne ikke slettes.',
  'ai.settings.checking': 'Sjekker…',
  'ai.settings.save_activate': 'Lagre og aktiver',
  'ai.settings.key_deleted': 'Nøkkel slettet. Standard månedlig kvote vil gjelde.',
  'ai.settings.active_provider': 'Aktiv leverandør: {provider} ({model})',
  'ai.settings.no_key_default':
    'Ingen egen nøkkel er satt opp. Abonnementets månedlige grense gjelder.',
  'ai.settings.status_plan': 'Abonnementskvote',
  'ai.settings.quota_unlimited': 'Ubegrenset',
  'ai.settings.quota_unlimited_help': 'Forespørsler håndtert med din egen API-nøkkel.',
  'ai.settings.quota_jobs': 'Jobber for utforming, oversettelse og eksport.',
  'ai.settings.loading': 'Laster inn KI-innstillinger',
  'ai.settings.load_error': 'KI-innstillingene kunne ikke lastes inn.',
  'ai.settings.load_error_help': 'Tjenesten kunne ikke nås. Innstillingene dine er ikke endret.',
  'ai.settings.key_invalid': 'Leverandøren avviste nøkkelen. Sjekk at den er riktig og aktiv.',
  'ai.settings.encryption_unavailable':
    'Tjenesten er ikke klar til å lagre nøkler sikkert ennå. Kontakt brukerstøtte.',
  'ai.settings.invalid_model': 'Velg en gyldig modell for denne leverandøren.',
  'ai.settings.key_state': 'Nøkkelstatus',

  'ai.common.cancel': 'Avbryt',

  // Felles kontekst: kvote, GDPR-varsel, feil og jobbstatus
  'ai.context.quota_label': 'Månedlig KI-kvote',
  'ai.context.quota_used': '{used} av {limit} jobber denne måneden',
  'ai.context.quota_reset': 'Fornyes {date}.',
  'ai.context.quota_exhausted_title': 'Du har brukt opp den månedlige KI-kvoten',
  'ai.context.quota_exhausted_desc':
    'Du kan starte flere jobber {date}. Den som administrerer organisasjonen, kan fjerne grensen ved å legge til en egen API-nøkkel i KI-innstillingene.',
  'ai.context.byok':
    'Organisasjonen din bruker sin egen API-nøkkel: denne jobben bruker ikke av kvoten.',
  'ai.context.loading': 'Sjekker KI-kvoten',
  'ai.context.load_error': 'KI-kvoten kunne ikke sjekkes. Du kan likevel prøve.',
  'ai.context.retry': 'Prøv igjen',
  'ai.notice.eu': 'Tekster behandles innenfor EU, i samsvar med GDPR.',
  'ai.notice.non_eu_title': 'Denne jobben forlater Det europeiske økonomiske samarbeidsområde',
  'ai.notice.non_eu_desc':
    'Organisasjonens egen nøkkel tilhører {provider}, som behandler data utenfor EØS. Organisasjonen godtok denne overføringen (GDPR art. 49 nr. 1 bokstav a) da nøkkelen ble lagret.',
  'ai.error.quota': 'Den månedlige KI-kvoten er brukt opp. Den fornyes {date}.',
  'ai.error.quota_nodate': 'Den månedlige KI-kvoten er brukt opp.',
  'ai.error.busy': 'Det kjører allerede KI-jobber. Vent til de er ferdige og prøv igjen.',
  'ai.error.admin': 'Bare de som administrerer organisasjonen, kan gjøre dette.',
  'ai.error.prompt': 'Beskriv nettstedet ditt før du genererer forslaget.',
  'ai.error.languages': 'Velg minst ett målspråk.',
  'ai.error.session': 'Økten din har utløpt. Last inn siden på nytt og logg inn igjen.',
  'ai.error.generic': 'Jobben kunne ikke startes. Prøv igjen om noen minutter.',
  'ai.error.draft_not_saved':
    'Utkastet kunne ikke lagres. Løs lagringsproblemet og start KI på nytt.',
  'ai.job.state_queued': 'I kø',
  'ai.job.state_running': 'Kjører',
  'ai.job.state_done': 'Ferdig',
  'ai.job.state_failed': 'Mislyktes',
  'ai.job.queued_text': 'Venter på at tjeneren skal ta imot jobben…',
  'ai.job.running_text': 'KI-en jobber. Dette kan ta noen minutter.',
  'ai.job.done_text': 'Jobben er fullført.',
  'ai.job.failed_text': 'Jobben kunne ikke fullføres.',
  'ai.job.lost_text': 'Tilkoblingen til tjeneren ble brutt.',
  'ai.job.elapsed': 'Medgått tid: {time}',
  'ai.job.detail': 'Teknisk detalj: {detail}',
  'ai.job.model': 'Modell: {model}',
  'ai.job.badge_eu': 'Innenfor EU',
  'ai.job.badge_non_eu': 'Utenfor EØS',
  'ai.job.badge_byok': 'Egen nøkkel',
  'ai.job.back': 'Tilbake',
  'ai.job.retry': 'Prøv igjen',
  'ai.job.stop': 'Slutt å vente',
  'ai.job.stop_help': 'Jobben fortsetter på tjeneren og teller med i kvoten din.',

  // Utforming
  'ai.design.title': 'Utform med KI',
  'ai.design.desc':
    'Beskriv virksomheten din, så foreslår KI-en inndelingen av siden med tekster klare til redigering.',
  'ai.design.prompt_label': 'Beskriv nettstedet eller virksomheten din',
  'ai.design.prompt_placeholder':
    'F.eks. Butikk for bærekraftig damemote med katalog og timebestilling…',
  'ai.design.prompt_help':
    'Jo mer konkret du er (hva du tilbyr, til hvem, og hva besøkende skal gjøre), desto bedre blir forslaget.',
  'ai.design.ideas_label': 'Ideer for å komme i gang',
  'ai.design.idea_1': 'Gourmetbutikk for håndverksoster og vin med nettbestilling',
  'ai.design.idea_2': 'Arkitekt- og oppussingskontor med prosjektgalleri',
  'ai.design.idea_3': 'Tannklinikk med timebestilling, behandlere og ofte stilte spørsmål',
  'ai.design.idea_4': 'Italiensk restaurant med smaksmeny og bordbestilling',
  'ai.design.style_label': 'Visuell stil',
  'ai.design.style_modern': 'Moderne og ren',
  'ai.design.style_elegant': 'Elegant og raffinert',
  'ai.design.style_bold': 'Dristig og livlig',
  'ai.design.style_minimal': 'Minimalistisk',
  'ai.design.lang_label': 'Innholdsspråk',
  'ai.design.button_generate': 'Generer forslag',
  'ai.design.job_title': 'Utformer forslaget',
  'ai.design.result_title': 'Forslaget er klart',
  'ai.design.result_sections': {
    one: '{count} seksjon klar til å legges til på lerretet.',
    other: '{count} seksjoner klare til å legges til på lerretet.',
  },
  'ai.design.result_help': 'De legges til på siden du redigerer, og du kan tilpasse dem etterpå.',
  'ai.design.result_empty': 'KI-en returnerte ingen brukbar seksjon.',
  'ai.design.button_apply': 'Legg til på lerretet',
  'ai.design.button_again': 'Prøv en annen idé',

  // Oversett
  'ai.translate.title': 'Oversett med KI',
  'ai.translate.desc': 'Oversett tekstene på nettstedet til andre språk uten å endre utformingen.',
  'ai.translate.from_label': 'Kildespråk',
  'ai.translate.to_label': 'Målspråk',
  'ai.translate.to_help':
    'Velg minst ett. Kildespråket kan ikke være et målspråk. Når du tar i bruk resultatet, legges nye språk til på nettstedet.',
  'ai.translate.scope_label': 'Omfang av oversettelsen',
  'ai.translate.scope_site': 'Hele nettstedet',
  'ai.translate.scope_page': 'Gjeldende side',
  'ai.translate.scope_store': 'Bare butikkatalogen',
  'ai.translate.store_unavailable': 'Oversettelse av butikkatalogen er ikke tilgjengelig ennå.',
  'ai.translate.scope_invalid': 'Velg en eksisterende side eller hele nettstedet for å oversette.',
  'ai.translate.overwrite_label': 'Overskriv oversettelser som allerede finnes',
  'ai.translate.button_translate': 'Oversett',
  'ai.translate.job_title': 'Oversetter tekstene',
  'ai.translate.result_title': 'Oversettelsene er klare',
  'ai.translate.result_texts': {
    one: '{count} tekst oversatt.',
    other: '{count} tekster oversatt.',
  },
  'ai.translate.result_help': 'De legges til i utkastet; utformingen endres ikke.',
  'ai.translate.result_empty': 'KI-en returnerte ingen oversettelse.',
  'ai.translate.button_apply': 'Bruk oversettelser',
  'ai.translate.button_again': 'Oversett til andre språk',

  // Eksporter
  'ai.export.title': 'Eksporter med KI',
  'ai.export.desc':
    'Generer en pakke med nettstedet ditt klar til å flyttes til en annen plattform, uten å være avhengig av Sytue.',
  'ai.export.destination_label': 'Målplattform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Komplett tema med JSON-maler og Liquid-seksjoner, pluss katalogen som CSV for import fra Shopify-administrasjonen.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Prosjekt med én side koblet til det offentlige Sytue-API-et, med kasse og skjemaer.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Tjenerrendret eller statisk applikasjon med betalingsløsningen innebygd.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Innebygd WordPress-blokktema, med eksport av produkter og egenskaper.',
  'ai.export.badge_available': 'Tilgjengelig',
  'ai.export.badge_soon': 'Kommer snart',
  'ai.export.button_export': 'Generer pakke',
  'ai.export.job_title': 'Genererer pakken for {destination}',
  'ai.export.result_title': 'Pakken er klar',
  'ai.export.result_help': 'Temaet og katalog-CSV-en leveres samlet i én ZIP-fil.',
  'ai.export.steps_title': 'Slik installerer du det i Shopify',
  'ai.export.step_1': 'Last ned ZIP-filen.',
  'ai.export.step_2':
    'I Shopify-administrasjonen går du til Nettbutikk > Temaer og velger «Last opp zip-fil».',
  'ai.export.step_3':
    'For produkter går du til Produkter > Importer og laster opp filen products_export_shopify.csv som ligger i ZIP-filen.',
  'ai.export.button_download': 'Last ned ZIP',
  'ai.export.button_again': 'Generer en ny eksport',
};
