/**
 * Danske tekster til kunstig intelligens (ia).
 * Præfiks: ai.*
 */
export const ia = {
  // Indstillinger
  'ai.settings.provider': 'AI-udbyder',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Egen API-nøgle (BYOK)',
  'ai.settings.key_placeholder': 'Indtast din hemmelige API-nøgle…',
  'ai.settings.key_help':
    'Din nøgle gemmes krypteret med AES-256-GCM. Den sendes aldrig tilbage til browseren.',
  'ai.settings.check_key': 'Test nøgle',
  'ai.settings.delete_key': 'Slet nøgle',
  'ai.settings.status_configured': 'Nøgle konfigureret',
  'ai.settings.tested_at': 'Bekræftet den',
  'ai.settings.quota_title': 'Månedlig AI-kvote',
  'ai.settings.quota_reset': 'Nulstilles den {date}',
  'ai.settings.notice_eu_title': 'Varsel om dataoverførsel uden for EØS',
  'ai.settings.notice_eu_desc':
    'Den valgte udbyder ({provider}) behandler data uden for Det Europæiske Økonomiske Samarbejdsområde. I henhold til GDPR art. 49, stk. 1, litra a, skal du give udtrykkeligt samtykke til denne overførsel for at fortsætte.',
  'ai.settings.notice_eu_accept':
    'Jeg giver samtykke til international overførsel af data uden for EØS ved AI-forespørgsler hos denne udbyder.',
  'ai.settings.success_save': 'Nøglen er gemt og bekræftet.',
  'ai.settings.error_check': 'Fejl ved bekræftelse af nøglen hos udbyderen.',
  'ai.settings.form_title': 'Konfigurer egen nøgle (BYOK)',
  'ai.settings.form_help':
    'Brug dine egne legitimationsoplysninger fra Mistral AI, OpenAI, Anthropic eller Zhipu AI, så du ikke er afhængig af den månedlige kvote.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Frankrig',
  'ai.region.us': 'USA',
  'ai.region.intl': 'International',
  'ai.settings.show_key': 'Vis nøgle',
  'ai.settings.hide_key': 'Skjul nøgle',
  'ai.settings.key_required': 'Indtast en nøgle.',
  'ai.settings.notice_required': 'Du skal acceptere varslet om overførsel uden for EØS.',
  'ai.settings.check_ok': 'Nøglen er bekræftet hos udbyderen.',
  'ai.settings.error_save': 'Nøglen kunne ikke gemmes.',
  'ai.settings.error_delete': 'Nøglen kunne ikke slettes.',
  'ai.settings.checking': 'Tjekker…',
  'ai.settings.save_activate': 'Gem og aktivér',
  'ai.settings.key_deleted': 'Nøgle slettet. Den månedlige standardkvote bruges.',
  'ai.settings.active_provider': 'Aktiv udbyder: {provider} ({model})',
  'ai.settings.no_key_default':
    'Ingen egen nøgle er sat op. Abonnementets månedlige grænse gælder.',
  'ai.settings.status_plan': 'Abonnementskvote',
  'ai.settings.quota_unlimited': 'Ubegrænset',
  'ai.settings.quota_unlimited_help': 'Forespørgsler håndteret med din egen API-nøgle.',
  'ai.settings.quota_jobs': 'Opgaver inden for design, oversættelse og eksport.',
  'ai.settings.loading': 'Indlæser AI-indstillinger',
  'ai.settings.load_error': 'AI-indstillingerne kunne ikke indlæses.',
  'ai.settings.load_error_help':
    'Tjenesten kunne ikke kontaktes. Dine indstillinger er ikke ændret.',
  'ai.settings.key_invalid': 'Udbyderen afviste nøglen. Tjek, at den er korrekt og aktiv.',
  'ai.settings.encryption_unavailable':
    'Tjenesten er endnu ikke klar til at gemme nøgler sikkert. Kontakt support.',
  'ai.settings.invalid_model': 'Vælg en gyldig model for denne udbyder.',
  'ai.settings.key_state': 'Nøglestatus',

  'ai.common.cancel': 'Annuller',

  // Fælles kontekst: kvote, GDPR-varsel, fejl og opgavestatus
  'ai.context.quota_label': 'Månedlig AI-kvote',
  'ai.context.quota_used': '{used} af {limit} opgaver denne måned',
  'ai.context.quota_reset': 'Fornyes den {date}.',
  'ai.context.quota_exhausted_title': 'Du har brugt den månedlige AI-kvote',
  'ai.context.quota_exhausted_desc':
    'Du kan starte flere opgaver den {date}. Den, der administrerer organisationen, kan fjerne grænsen ved at tilføje en egen API-nøgle i AI-indstillingerne.',
  'ai.context.byok':
    'Din organisation bruger sin egen API-nøgle: denne opgave bruger ikke af kvoten.',
  'ai.context.loading': 'Tjekker AI-kvoten',
  'ai.context.load_error': 'AI-kvoten kunne ikke tjekkes. Du kan stadig prøve.',
  'ai.context.retry': 'Prøv igen',
  'ai.notice.eu': 'Tekster behandles inden for EU i overensstemmelse med GDPR.',
  'ai.notice.non_eu_title': 'Denne opgave forlader Det Europæiske Økonomiske Samarbejdsområde',
  'ai.notice.non_eu_desc':
    'Din organisations egen nøgle tilhører {provider}, som behandler data uden for EØS. Din organisation accepterede denne overførsel (GDPR art. 49, stk. 1, litra a), da nøglen blev gemt.',
  'ai.error.quota': 'Den månedlige AI-kvote er opbrugt. Den fornyes den {date}.',
  'ai.error.quota_nodate': 'Den månedlige AI-kvote er opbrugt.',
  'ai.error.busy': 'Der kører allerede AI-opgaver. Vent på, at de bliver færdige, og prøv igen.',
  'ai.error.admin': 'Kun de, der administrerer organisationen, kan gøre dette.',
  'ai.error.prompt': 'Beskriv dit websted, før du genererer forslaget.',
  'ai.error.languages': 'Vælg mindst ét målsprog.',
  'ai.error.session': 'Din session er udløbet. Genindlæs siden, og log ind igen.',
  'ai.error.generic': 'Opgaven kunne ikke startes. Prøv igen om et par minutter.',
  'ai.error.draft_not_saved':
    'Kladden kunne ikke gemmes. Løs problemet med at gemme, og start AI igen.',
  'ai.job.state_queued': 'I kø',
  'ai.job.state_running': 'Kører',
  'ai.job.state_done': 'Færdig',
  'ai.job.state_failed': 'Mislykkedes',
  'ai.job.queued_text': 'Venter på, at serveren accepterer opgaven…',
  'ai.job.running_text': 'AI arbejder. Det kan tage et par minutter.',
  'ai.job.done_text': 'Opgaven er fuldført.',
  'ai.job.failed_text': 'Opgaven kunne ikke gennemføres.',
  'ai.job.lost_text': 'Forbindelsen til serveren blev afbrudt.',
  'ai.job.elapsed': 'Forløbet tid: {time}',
  'ai.job.detail': 'Teknisk detalje: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'Inden for EU',
  'ai.job.badge_non_eu': 'Uden for EØS',
  'ai.job.badge_byok': 'Egen nøgle',
  'ai.job.back': 'Tilbage',
  'ai.job.retry': 'Prøv igen',
  'ai.job.stop': 'Stop med at vente',
  'ai.job.stop_help': 'Opgaven fortsætter på serveren og tæller med i din kvote.',

  // Design
  'ai.design.title': 'Design med AI',
  'ai.design.desc':
    'Beskriv din virksomhed, så foreslår AI sidens sektioner med tekster klar til redigering.',
  'ai.design.prompt_label': 'Beskriv dit websted eller din virksomhed',
  'ai.design.prompt_placeholder':
    'F.eks. Butik med bæredygtigt dametøj med katalog og tidsbestilling…',
  'ai.design.prompt_help':
    'Jo mere konkret du er (hvad du tilbyder, til hvem, og hvad besøgende skal gøre), desto bedre bliver forslaget.',
  'ai.design.ideas_label': 'Idéer til at komme i gang',
  'ai.design.idea_1': 'Gourmetbutik med håndlavede oste og vine samt onlinebestilling',
  'ai.design.idea_2': 'Arkitekt- og renoveringsfirma med projektgalleri',
  'ai.design.idea_3': 'Tandlægeklinik med tidsbestilling, behandlerteam og FAQ',
  'ai.design.idea_4': 'Italiensk restaurant med smagsmenu og bordreservation',
  'ai.design.style_label': 'Visuel stil',
  'ai.design.style_modern': 'Moderne og ren',
  'ai.design.style_elegant': 'Elegant og raffineret',
  'ai.design.style_bold': 'Modig og levende',
  'ai.design.style_minimal': 'Minimalistisk',
  'ai.design.lang_label': 'Indholdets sprog',
  'ai.design.button_generate': 'Generer forslag',
  'ai.design.job_title': 'Designer forslaget',
  'ai.design.result_title': 'Forslag klart',
  'ai.design.result_sections': {
    one: '{count} sektion klar til at blive føjet til lærredet.',
    other: '{count} sektioner klar til at blive føjet til lærredet.',
  },
  'ai.design.result_help': 'De føjes til den side, du redigerer, og du kan tilpasse dem bagefter.',
  'ai.design.result_empty': 'AI returnerede ingen brugbar sektion.',
  'ai.design.button_apply': 'Føj til lærredet',
  'ai.design.button_again': 'Prøv en anden idé',

  // Oversæt
  'ai.translate.title': 'Oversæt med AI',
  'ai.translate.desc': 'Oversæt webstedets tekster til andre sprog uden at røre ved designet.',
  'ai.translate.from_label': 'Kildesprog',
  'ai.translate.to_label': 'Målsprog',
  'ai.translate.to_help':
    'Vælg mindst ét. Kildesproget kan ikke vælges som målsprog. Når du anvender resultatet, føjes nye sprog til webstedet.',
  'ai.translate.scope_label': 'Oversættelsens omfang',
  'ai.translate.scope_site': 'Hele webstedet',
  'ai.translate.scope_page': 'Aktuel side',
  'ai.translate.scope_store': 'Kun butikkens katalog',
  'ai.translate.store_unavailable': 'Oversættelse af butikkens katalog er ikke tilgængelig endnu.',
  'ai.translate.scope_invalid': 'Vælg en eksisterende side eller hele webstedet for at oversætte.',
  'ai.translate.overwrite_label': 'Overskriv oversættelser, der allerede findes',
  'ai.translate.button_translate': 'Oversæt',
  'ai.translate.job_title': 'Oversætter teksterne',
  'ai.translate.result_title': 'Oversættelser klar',
  'ai.translate.result_texts': {
    one: '{count} tekst oversat.',
    other: '{count} tekster oversat.',
  },
  'ai.translate.result_help': 'De føjes til kladden; designet ændres ikke.',
  'ai.translate.result_empty': 'AI returnerede ingen oversættelse.',
  'ai.translate.button_apply': 'Anvend oversættelser',
  'ai.translate.button_again': 'Oversæt til andre sprog',

  // Eksportér
  'ai.export.title': 'Eksportér med AI',
  'ai.export.desc':
    'Generer en pakke med dit websted klar til at flytte til en anden platform uden at være afhængig af Sytue.',
  'ai.export.destination_label': 'Målplatform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Komplet tema med JSON-skabeloner og Liquid-sektioner samt kataloget som CSV til import fra Shopify-administrationen.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Single-page-projekt forbundet til det offentlige Sytue-API med betaling og formularer.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Server-renderet eller statisk applikation med indbygget betalingsgateway.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Standard WordPress-bloktema med eksport af produkter og egenskaber.',
  'ai.export.badge_available': 'Tilgængelig',
  'ai.export.badge_soon': 'Kommer snart',
  'ai.export.button_export': 'Generer pakke',
  'ai.export.job_title': 'Genererer pakken til {destination}',
  'ai.export.result_title': 'Pakke klar',
  'ai.export.result_help': 'Temaet og katalogets CSV leveres samlet i én ZIP-fil.',
  'ai.export.steps_title': 'Sådan installerer du det i Shopify',
  'ai.export.step_1': 'Download ZIP-filen.',
  'ai.export.step_2':
    'Gå til Webshop > Temaer i Shopify-administrationen, og vælg “Upload zip-fil”.',
  'ai.export.step_3':
    'For produkter skal du gå til Produkter > Importér og uploade filen products_export_shopify.csv fra ZIP-filen.',
  'ai.export.button_download': 'Download ZIP',
  'ai.export.button_again': 'Generer en ny eksport',
};
