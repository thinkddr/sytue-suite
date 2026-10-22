/**
 * Nederlandse teksten voor kunstmatige intelligentie (ia).
 * Prefix: ai.*
 */
export const ia = {
  // Instellingen
  'ai.settings.provider': 'AI-provider',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Eigen API-sleutel (BYOK)',
  'ai.settings.key_placeholder': 'Vul je geheime API-sleutel in…',
  'ai.settings.key_help':
    'Je sleutel wordt versleuteld opgeslagen met AES-256-GCM. Hij wordt nooit naar de browser teruggestuurd.',
  'ai.settings.check_key': 'Sleutel testen',
  'ai.settings.delete_key': 'Sleutel verwijderen',
  'ai.settings.status_configured': 'Sleutel ingesteld',
  'ai.settings.tested_at': 'Geverifieerd op',
  'ai.settings.quota_title': 'Maandelijks AI-tegoed',
  'ai.settings.quota_reset': 'Wordt vernieuwd op {date}',
  'ai.settings.notice_eu_title': 'Melding over doorgifte van gegevens buiten de EER',
  'ai.settings.notice_eu_desc':
    'De gekozen provider ({provider}) verwerkt gegevens buiten de Europese Economische Ruimte. Op grond van artikel 49, lid 1, onder a, AVG moet je uitdrukkelijk toestemming geven voor deze doorgifte om verder te gaan.',
  'ai.settings.notice_eu_accept':
    'Ik geef toestemming voor de internationale doorgifte van gegevens buiten de EER voor AI-verzoeken aan deze provider.',
  'ai.settings.success_save': 'Sleutel opgeslagen en geverifieerd.',
  'ai.settings.error_check': 'Fout bij het verifiëren van de sleutel bij de provider.',
  'ai.settings.form_title': 'Je eigen sleutel instellen (BYOK)',
  'ai.settings.form_help':
    'Gebruik je eigen gegevens van Mistral AI, OpenAI, Anthropic of Zhipu AI, zodat je niet afhankelijk bent van het maandelijkse tegoed.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, AVG)',
  'ai.settings.provider_openai': 'OpenAI (VS)',
  'ai.settings.provider_anthropic': 'Anthropic (VS)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Frankrijk',
  'ai.region.us': 'VS',
  'ai.region.intl': 'Internationaal',
  'ai.settings.show_key': 'Sleutel tonen',
  'ai.settings.hide_key': 'Sleutel verbergen',
  'ai.settings.key_required': 'Vul een sleutel in.',
  'ai.settings.notice_required': 'Je moet de melding over doorgifte buiten de EER accepteren.',
  'ai.settings.check_ok': 'Sleutel gecontroleerd bij de provider.',
  'ai.settings.error_save': 'De sleutel kon niet worden opgeslagen.',
  'ai.settings.error_delete': 'De sleutel kon niet worden verwijderd.',
  'ai.settings.checking': 'Controleren…',
  'ai.settings.save_activate': 'Opslaan en activeren',
  'ai.settings.key_deleted': 'Sleutel verwijderd. Het standaard maandelijkse tegoed geldt weer.',
  'ai.settings.active_provider': 'Actieve provider: {provider} ({model})',
  'ai.settings.no_key_default':
    'Er is geen eigen sleutel ingesteld. De maandelijkse limiet van het abonnement geldt.',
  'ai.settings.status_plan': 'Tegoed van het abonnement',
  'ai.settings.quota_unlimited': 'Onbeperkt',
  'ai.settings.quota_unlimited_help': 'Verzoeken verwerkt met je eigen API-sleutel.',
  'ai.settings.quota_jobs': 'Taken voor ontwerp, vertaling en export.',
  'ai.settings.loading': 'AI-instellingen laden',
  'ai.settings.load_error': 'De AI-instellingen konden niet worden geladen.',
  'ai.settings.load_error_help':
    'De dienst was niet bereikbaar. Je instellingen zijn niet gewijzigd.',
  'ai.settings.key_invalid':
    'De provider heeft de sleutel geweigerd. Controleer of hij juist en actief is.',
  'ai.settings.encryption_unavailable':
    'De dienst kan nog geen sleutels veilig opslaan. Neem contact op met de ondersteuning.',
  'ai.settings.invalid_model': 'Kies een geldig model voor deze provider.',
  'ai.settings.key_state': 'Status van de sleutel',

  'ai.common.cancel': 'Annuleren',

  // Gedeelde context: tegoed, AVG-melding, fouten en status van taken
  'ai.context.quota_label': 'Maandelijks AI-tegoed',
  'ai.context.quota_used': '{used} van {limit} taken deze maand',
  'ai.context.quota_reset': 'Wordt vernieuwd op {date}.',
  'ai.context.quota_exhausted_title': 'Het maandelijkse AI-tegoed is op',
  'ai.context.quota_exhausted_desc':
    'Op {date} kun je weer taken starten. Wie de organisatie beheert, kan de limiet opheffen door in de AI-instellingen een eigen API-sleutel toe te voegen.',
  'ai.context.byok':
    'Je organisatie gebruikt een eigen API-sleutel: deze taak gaat niet van het tegoed af.',
  'ai.context.loading': 'AI-tegoed controleren',
  'ai.context.load_error':
    'Het AI-tegoed kon niet worden gecontroleerd. Je kunt het toch proberen.',
  'ai.context.retry': 'Opnieuw proberen',
  'ai.notice.eu': 'Teksten worden binnen de EU verwerkt, in overeenstemming met de AVG.',
  'ai.notice.non_eu_title': 'Deze taak verlaat de Europese Economische Ruimte',
  'ai.notice.non_eu_desc':
    'De eigen sleutel van je organisatie hoort bij {provider}, dat gegevens buiten de EER verwerkt. Je organisatie heeft bij het opslaan van de sleutel ingestemd met die doorgifte (artikel 49, lid 1, onder a, AVG).',
  'ai.error.quota': 'Het maandelijkse AI-tegoed is op. Het wordt vernieuwd op {date}.',
  'ai.error.quota_nodate': 'Het maandelijkse AI-tegoed is op.',
  'ai.error.busy': 'Er lopen al AI-taken. Wacht tot ze klaar zijn en probeer het opnieuw.',
  'ai.error.admin': 'Alleen wie de organisatie beheert, kan dit doen.',
  'ai.error.prompt': 'Beschrijf je site voordat je het voorstel laat maken.',
  'ai.error.languages': 'Kies minstens één doeltaal.',
  'ai.error.session': 'Je sessie is verlopen. Laad de pagina opnieuw en log opnieuw in.',
  'ai.error.generic': 'De taak kon niet worden gestart. Probeer het over een paar minuten opnieuw.',
  'ai.error.draft_not_saved':
    'Het concept kon niet worden opgeslagen. Los het probleem met opslaan op en start de AI opnieuw.',
  'ai.job.state_queued': 'In de wachtrij',
  'ai.job.state_running': 'Bezig',
  'ai.job.state_done': 'Klaar',
  'ai.job.state_failed': 'Mislukt',
  'ai.job.queued_text': 'Wachten tot de server de taak accepteert…',
  'ai.job.running_text': 'De AI is aan het werk. Dit kan een paar minuten duren.',
  'ai.job.done_text': 'De taak is voltooid.',
  'ai.job.failed_text': 'De taak kon niet worden voltooid.',
  'ai.job.lost_text': 'De verbinding met de server is verbroken.',
  'ai.job.elapsed': 'Verstreken tijd: {time}',
  'ai.job.detail': 'Technische details: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'Binnen de EU',
  'ai.job.badge_non_eu': 'Buiten de EER',
  'ai.job.badge_byok': 'Eigen sleutel',
  'ai.job.back': 'Terug',
  'ai.job.retry': 'Opnieuw proberen',
  'ai.job.stop': 'Niet langer wachten',
  'ai.job.stop_help': 'De taak loopt door op de server en telt mee voor je tegoed.',

  // Ontwerp
  'ai.design.title': 'Ontwerpen met AI',
  'ai.design.desc':
    'Beschrijf je bedrijf en de AI stelt de onderdelen van de pagina voor, met teksten die je meteen kunt bewerken.',
  'ai.design.prompt_label': 'Beschrijf je site of bedrijf',
  'ai.design.prompt_placeholder': 'Bijv. duurzame damesmodewinkel met catalogus en afspraken…',
  'ai.design.prompt_help':
    'Hoe concreter de beschrijving (wat je aanbiedt, voor wie en wat bezoekers van de site moeten doen), hoe beter het voorstel.',
  'ai.design.ideas_label': 'Ideeën om te beginnen',
  'ai.design.idea_1': 'Delicatessenwinkel met ambachtelijke kazen en wijnen en online bestellen',
  'ai.design.idea_2': 'Bureau voor architectuur en renovatie met een projectgalerij',
  'ai.design.idea_3': 'Tandartspraktijk met afspraken, medisch team en veelgestelde vragen',
  'ai.design.idea_4': 'Italiaans restaurant met proeverijmenu en tafelreservering',
  'ai.design.style_label': 'Visuele stijl',
  'ai.design.style_modern': 'Modern en strak',
  'ai.design.style_elegant': 'Elegant en verfijnd',
  'ai.design.style_bold': 'Gedurfd en levendig',
  'ai.design.style_minimal': 'Minimalistisch',
  'ai.design.lang_label': 'Taal van de inhoud',
  'ai.design.button_generate': 'Voorstel maken',
  'ai.design.job_title': 'Voorstel ontwerpen',
  'ai.design.result_title': 'Voorstel klaar',
  'ai.design.result_sections': {
    one: '{count} onderdeel klaar om aan het canvas toe te voegen.',
    other: '{count} onderdelen klaar om aan het canvas toe te voegen.',
  },
  'ai.design.result_help':
    'Ze worden toegevoegd aan de pagina die je bewerkt en je kunt ze daarna aanpassen.',
  'ai.design.result_empty': 'De AI heeft geen bruikbaar onderdeel teruggegeven.',
  'ai.design.button_apply': 'Aan canvas toevoegen',
  'ai.design.button_again': 'Ander idee proberen',

  // Vertalen
  'ai.translate.title': 'Vertalen met AI',
  'ai.translate.desc':
    'Vertaal de teksten van de site naar andere talen zonder het ontwerp te wijzigen.',
  'ai.translate.from_label': 'Brontaal',
  'ai.translate.to_label': 'Doeltalen',
  'ai.translate.to_help':
    'Kies er minstens één. De brontaal kan geen doeltaal zijn. Als je het resultaat toepast, worden er nieuwe talen aan de site toegevoegd.',
  'ai.translate.scope_label': 'Omvang van de vertaling',
  'ai.translate.scope_site': 'Hele site',
  'ai.translate.scope_page': 'Huidige pagina',
  'ai.translate.scope_store': 'Alleen de catalogus van de webwinkel',
  'ai.translate.store_unavailable': 'Het vertalen van de catalogus is nog niet beschikbaar.',
  'ai.translate.scope_invalid': 'Kies een bestaande pagina of de hele site om te vertalen.',
  'ai.translate.overwrite_label': 'Bestaande vertalingen overschrijven',
  'ai.translate.button_translate': 'Vertalen',
  'ai.translate.job_title': 'Teksten vertalen',
  'ai.translate.result_title': 'Vertalingen klaar',
  'ai.translate.result_texts': {
    one: '{count} tekst vertaald.',
    other: '{count} teksten vertaald.',
  },
  'ai.translate.result_help': 'Ze worden aan het concept toegevoegd; het ontwerp verandert niet.',
  'ai.translate.result_empty': 'De AI heeft geen vertaling teruggegeven.',
  'ai.translate.button_apply': 'Vertalingen toepassen',
  'ai.translate.button_again': 'Naar andere talen vertalen',

  // Exporteren
  'ai.export.title': 'Exporteren met AI',
  'ai.export.desc':
    'Maak een pakket met je site, klaar om naar een ander platform mee te nemen, zonder afhankelijk te zijn van Sytue.',
  'ai.export.destination_label': 'Doelplatform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Volledig thema met JSON-templates en Liquid-secties, plus de catalogus als CSV om te importeren vanuit het Shopify-beheer.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Single-pageproject gekoppeld aan de openbare Sytue-API, met afrekenen en formulieren.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Servergerenderde of statische applicatie met ingebouwde betaalprovider.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Native WordPress-blokthema, met export van producten en kenmerken.',
  'ai.export.badge_available': 'Beschikbaar',
  'ai.export.badge_soon': 'Binnenkort',
  'ai.export.button_export': 'Pakket maken',
  'ai.export.job_title': 'Pakket maken voor {destination}',
  'ai.export.result_title': 'Pakket klaar',
  'ai.export.result_help': 'Het thema en de CSV van de catalogus zitten samen in één zip.',
  'ai.export.steps_title': 'Installeren in Shopify',
  'ai.export.step_1': 'Download het zipbestand.',
  'ai.export.step_2':
    'Ga in het Shopify-beheer naar Online Store > Thema’s en kies ‘Zipbestand uploaden’.',
  'ai.export.step_3':
    'Ga voor de producten naar Producten > Importeren en upload het bestand products_export_shopify.csv uit de zip.',
  'ai.export.button_download': 'Zip downloaden',
  'ai.export.button_again': 'Nog een export maken',
};
