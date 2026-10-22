/**
 * Sporočila v slovenščini za umetno inteligenco (ia).
 * Predpona: ai.*
 */
export const ia = {
  // Nastavitve
  'ai.settings.provider': 'Ponudnik UI',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Lastni ključ API (BYOK)',
  'ai.settings.key_placeholder': 'Vnesite svoj skrivni ključ API…',
  'ai.settings.key_help':
    'Vaš ključ je shranjen šifrirano z AES-256-GCM. Nikoli se ne vrne brskalniku.',
  'ai.settings.check_key': 'Preizkusi ključ',
  'ai.settings.delete_key': 'Izbriši ključ',
  'ai.settings.status_configured': 'Ključ je nastavljen',
  'ai.settings.tested_at': 'Preverjeno dne',
  'ai.settings.quota_title': 'Mesečna kvota za UI',
  'ai.settings.quota_reset': 'Ponastavi se {date}',
  'ai.settings.notice_eu_title': 'Obvestilo o prenosu podatkov zunaj EGP',
  'ai.settings.notice_eu_desc':
    'Izbrani ponudnik ({provider}) obdeluje podatke zunaj Evropskega gospodarskega prostora. V skladu s 1. točko (a) 49. člena GDPR morate s tem prenosom izrecno soglašati, če želite nadaljevati.',
  'ai.settings.notice_eu_accept':
    'Soglašam z mednarodnim prenosom podatkov zunaj EGP za zahteve UI pri tem ponudniku.',
  'ai.settings.success_save': 'Ključ je bil uspešno shranjen in preverjen.',
  'ai.settings.error_check': 'Napaka pri preverjanju ključa pri ponudniku.',
  'ai.settings.form_title': 'Nastavite lastni ključ (BYOK)',
  'ai.settings.form_help':
    'Uporabite lastne poverilnice za Mistral AI, OpenAI, Anthropic ali Zhipu AI, da ne boste odvisni od mesečne kvote.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (ZDA)',
  'ai.settings.provider_anthropic': 'Anthropic (ZDA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Francija',
  'ai.region.us': 'ZDA',
  'ai.region.intl': 'Mednarodno',
  'ai.settings.show_key': 'Prikaži ključ',
  'ai.settings.hide_key': 'Skrij ključ',
  'ai.settings.key_required': 'Vnesite ključ.',
  'ai.settings.notice_required': 'Sprejeti morate obvestilo o prenosih zunaj EGP.',
  'ai.settings.check_ok': 'Ključ je bil uspešno preverjen pri ponudniku.',
  'ai.settings.error_save': 'Ključa ni bilo mogoče shraniti.',
  'ai.settings.error_delete': 'Ključa ni bilo mogoče izbrisati.',
  'ai.settings.checking': 'Preverjanje…',
  'ai.settings.save_activate': 'Shrani in aktiviraj',
  'ai.settings.key_deleted': 'Ključ je izbrisan. Veljala bo privzeta mesečna kvota.',
  'ai.settings.active_provider': 'Aktivni ponudnik: {provider} ({model})',
  'ai.settings.no_key_default': 'Lastni ključ ni nastavljen. Velja mesečna omejitev paketa.',
  'ai.settings.status_plan': 'Kvota paketa',
  'ai.settings.quota_unlimited': 'Neomejeno',
  'ai.settings.quota_unlimited_help': 'Zahteve, obravnavane z vašim lastnim ključem API.',
  'ai.settings.quota_jobs': 'Opravila oblikovanja, prevajanja in izvoza.',
  'ai.settings.loading': 'Nalaganje nastavitev UI',
  'ai.settings.load_error': 'Nastavitev UI ni bilo mogoče naložiti.',
  'ai.settings.load_error_help':
    'Storitve ni bilo mogoče doseči. Vaše nastavitve se niso spremenile.',
  'ai.settings.key_invalid': 'Ponudnik je zavrnil ključ. Preverite, ali je pravilen in aktiven.',
  'ai.settings.encryption_unavailable':
    'Storitev še ni pripravljena za varno shranjevanje ključev. Obrnite se na podporo.',
  'ai.settings.invalid_model': 'Izberite veljaven model za tega ponudnika.',
  'ai.settings.key_state': 'Stanje ključa',

  'ai.common.cancel': 'Prekliči',

  // Skupni kontekst: kvota, obvestilo GDPR, napake in stanje opravila
  'ai.context.quota_label': 'Mesečna kvota za UI',
  'ai.context.quota_used': '{used} od {limit} opravil ta mesec',
  'ai.context.quota_reset': 'Obnovi se {date}.',
  'ai.context.quota_exhausted_title': 'Mesečna kvota za UI je porabljena',
  'ai.context.quota_exhausted_desc':
    'Več opravil boste lahko zagnali {date}. Uprava organizacije lahko omejitev odpravi z dodajanjem lastnega ključa API v nastavitvah UI.',
  'ai.context.byok': 'Vaša organizacija uporablja lastni ključ API: to opravilo ne porablja kvote.',
  'ai.context.loading': 'Preverjanje kvote za UI',
  'ai.context.load_error': 'Kvote za UI ni bilo mogoče preveriti. Kljub temu lahko poskusite.',
  'ai.context.retry': 'Poskusi znova',
  'ai.notice.eu': 'Besedila se obdelujejo znotraj EU v skladu z GDPR.',
  'ai.notice.non_eu_title': 'To opravilo zapušča Evropski gospodarski prostor',
  'ai.notice.non_eu_desc':
    'Lastni ključ vaše organizacije pripada ponudniku {provider}, ki podatke obdeluje zunaj EGP. Vaša organizacija je ta prenos sprejela (1. točka (a) 49. člena GDPR) ob shranjevanju ključa.',
  'ai.error.quota': 'Mesečna kvota za UI je porabljena. Obnovi se {date}.',
  'ai.error.quota_nodate': 'Mesečna kvota za UI je porabljena.',
  'ai.error.busy': 'Opravila UI se že izvajajo. Počakajte, da se dokončajo, in poskusite znova.',
  'ai.error.admin': 'To lahko stori le uprava organizacije.',
  'ai.error.prompt': 'Pred generiranjem predloga opišite svoje spletno mesto.',
  'ai.error.languages': 'Izberite vsaj en ciljni jezik.',
  'ai.error.session': 'Vaša seja je potekla. Znova naložite stran in se prijavite.',
  'ai.error.generic': 'Opravila ni bilo mogoče zagnati. Poskusite znova čez nekaj minut.',
  'ai.error.draft_not_saved':
    'Osnutka ni bilo mogoče shraniti. Odpravite težavo s shranjevanjem in znova zaženite UI.',
  'ai.job.state_queued': 'V čakalni vrsti',
  'ai.job.state_running': 'V teku',
  'ai.job.state_done': 'Končano',
  'ai.job.state_failed': 'Spodletelo',
  'ai.job.queued_text': 'Čakanje, da strežnik sprejme opravilo…',
  'ai.job.running_text': 'UI dela. To lahko traja nekaj minut.',
  'ai.job.done_text': 'Opravilo je končano.',
  'ai.job.failed_text': 'Opravila ni bilo mogoče dokončati.',
  'ai.job.lost_text': 'Povezava s strežnikom je bila prekinjena.',
  'ai.job.elapsed': 'Pretečeni čas: {time}',
  'ai.job.detail': 'Tehnična podrobnost: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'Znotraj EU',
  'ai.job.badge_non_eu': 'Zunaj EGP',
  'ai.job.badge_byok': 'Lastni ključ',
  'ai.job.back': 'Nazaj',
  'ai.job.retry': 'Poskusi znova',
  'ai.job.stop': 'Prenehaj čakati',
  'ai.job.stop_help': 'Opravilo se bo nadaljevalo na strežniku in se bo štelo v vašo kvoto.',

  // Oblikovanje
  'ai.design.title': 'Oblikuj z UI',
  'ai.design.desc':
    'Opišite svoje podjetje in UI bo predlagala razdelke strani z besedili, pripravljenimi za urejanje.',
  'ai.design.prompt_label': 'Opišite svoje spletno mesto ali podjetje',
  'ai.design.prompt_placeholder':
    'Npr. Trgovina z naravno vegansko kozmetiko s katalogom in rezervacijami…',
  'ai.design.prompt_help':
    'Bolj ko ste konkretni (kaj ponujate, komu in kaj naj obiskovalci storijo), boljši bo predlog.',
  'ai.design.ideas_label': 'Ideje za začetek',
  'ai.design.idea_1': 'Gurmanska trgovina z domačimi siri in vini ter spletnimi naročili',
  'ai.design.idea_2': 'Arhitekturni biro in celovite prenove z galerijo projektov',
  'ai.design.idea_3': 'Zobozdravstvena ambulanta z naročanjem, ekipo in pogostimi vprašanji',
  'ai.design.idea_4': 'Italijanska restavracija z degustacijskim menijem in rezervacijo miz',
  'ai.design.style_label': 'Vizualni slog',
  'ai.design.style_modern': 'Sodoben in čist',
  'ai.design.style_elegant': 'Eleganten in prefinjen',
  'ai.design.style_bold': 'Drzen in živahen',
  'ai.design.style_minimal': 'Minimalističen',
  'ai.design.lang_label': 'Jezik vsebine',
  'ai.design.button_generate': 'Ustvari predlog',
  'ai.design.job_title': 'Oblikovanje predloga',
  'ai.design.result_title': 'Predlog je pripravljen',
  'ai.design.result_sections': {
    one: '{count} razdelek pripravljen za dodajanje na platno.',
    two: '{count} razdelka pripravljena za dodajanje na platno.',
    few: '{count} razdelki pripravljeni za dodajanje na platno.',
    other: '{count} razdelkov pripravljenih za dodajanje na platno.',
  },
  'ai.design.result_help':
    'Dodani bodo na stran, ki jo urejate, in jih boste lahko nato prilagodili.',
  'ai.design.result_empty': 'UI ni vrnila nobenega uporabnega razdelka.',
  'ai.design.button_apply': 'Dodaj na platno',
  'ai.design.button_again': 'Preizkusi drugo idejo',

  // Prevajanje
  'ai.translate.title': 'Prevedi z UI',
  'ai.translate.desc':
    'Prevedite besedila spletnega mesta v druge jezike brez spreminjanja oblike.',
  'ai.translate.from_label': 'Izvorni jezik',
  'ai.translate.to_label': 'Ciljni jeziki',
  'ai.translate.to_help':
    'Izberite vsaj enega. Izvorni jezik ne more biti ciljni. Z uveljavitvijo rezultata se spletnemu mestu dodajo novi jeziki.',
  'ai.translate.scope_label': 'Obseg prevoda',
  'ai.translate.scope_site': 'Celotno spletno mesto',
  'ai.translate.scope_page': 'Trenutna stran',
  'ai.translate.scope_store': 'Samo katalog trgovine',
  'ai.translate.store_unavailable': 'Prevajanje kataloga trgovine še ni na voljo.',
  'ai.translate.scope_invalid': 'Za prevod izberite obstoječo stran ali celotno spletno mesto.',
  'ai.translate.overwrite_label': 'Prepiši prevode, ki že obstajajo',
  'ai.translate.button_translate': 'Prevedi',
  'ai.translate.job_title': 'Prevajanje besedil',
  'ai.translate.result_title': 'Prevodi so pripravljeni',
  'ai.translate.result_texts': {
    one: '{count} prevedeno besedilo.',
    two: '{count} prevedeni besedili.',
    few: '{count} prevedena besedila.',
    other: '{count} prevedenih besedil.',
  },
  'ai.translate.result_help': 'Dodani bodo v osnutek; oblika se ne spremeni.',
  'ai.translate.result_empty': 'UI ni vrnila nobenega prevoda.',
  'ai.translate.button_apply': 'Uveljavi prevode',
  'ai.translate.button_again': 'Prevedi v druge jezike',

  // Izvoz
  'ai.export.title': 'Izvozi z UI',
  'ai.export.desc':
    'Ustvarite paket s svojim spletnim mestom, pripravljenim za prenos na drugo platformo, brez odvisnosti od Sytue.',
  'ai.export.destination_label': 'Ciljna platforma',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Celotna tema s predlogami JSON in razdelki Liquid ter katalogom v CSV za uvoz v skrbniški plošči Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Enostranski projekt, povezan z javnim API-jem Sytue, z blagajno in obrazci.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Strežniško upodobljena ali statična aplikacija z vgrajenim plačilnim prehodom.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Domorodna blokovna tema za WordPress z izvozom izdelkov in atributov.',
  'ai.export.badge_available': 'Na voljo',
  'ai.export.badge_soon': 'Kmalu',
  'ai.export.button_export': 'Ustvari paket',
  'ai.export.job_title': 'Ustvarjanje paketa za {destination}',
  'ai.export.result_title': 'Paket je pripravljen',
  'ai.export.result_help': 'Tema in katalog v CSV sta združena v eni sami datoteki ZIP.',
  'ai.export.steps_title': 'Kako namestiti v Shopify',
  'ai.export.step_1': 'Prenesite datoteko ZIP.',
  'ai.export.step_2':
    'V skrbniški plošči Shopify pojdite v Online Store > Themes in izberite »Upload zip file«.',
  'ai.export.step_3':
    'Za izdelke pojdite v Products > Import in naložite datoteko products_export_shopify.csv iz datoteke ZIP.',
  'ai.export.button_download': 'Prenesi ZIP',
  'ai.export.button_again': 'Ustvari drug izvoz',
};
