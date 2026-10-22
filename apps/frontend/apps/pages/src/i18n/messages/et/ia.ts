/**
 * Eestikeelsed teated tehisintellekti jaoks (ia).
 * Eesliide: ai.*
 */
export const ia = {
  // Seaded
  'ai.settings.provider': 'Tehisintellekti pakkuja',
  'ai.settings.model': 'Mudel',
  'ai.settings.key': 'Oma API-võti (BYOK)',
  'ai.settings.key_placeholder': 'Sisestage oma salajane API-võti…',
  'ai.settings.key_help':
    'Teie võti salvestatakse AES-256-GCM krüpteeringuga. Seda ei tagastata kunagi brauserisse.',
  'ai.settings.check_key': 'Testi võtit',
  'ai.settings.delete_key': 'Kustuta võti',
  'ai.settings.status_configured': 'Võti seadistatud',
  'ai.settings.tested_at': 'Kinnitatud',
  'ai.settings.quota_title': 'Igakuine tehisintellekti kvoot',
  'ai.settings.quota_reset': 'Lähtestub {date}',
  'ai.settings.notice_eu_title': 'Teade andmete edastamise kohta väljapoole EMP-d',
  'ai.settings.notice_eu_desc':
    'Valitud teenusepakkuja ({provider}) töötleb andmeid väljaspool Euroopa Majanduspiirkonda. GDPR-i art 49 lg 1 punkti a kohaselt peate jätkamiseks andma selleks edastamiseks selgesõnalise nõusoleku.',
  'ai.settings.notice_eu_accept':
    'Nõustun andmete rahvusvahelise edastamisega väljapoole EMP-d selle teenusepakkuja tehisintellekti päringute jaoks.',
  'ai.settings.success_save': 'Võti on edukalt salvestatud ja kinnitatud.',
  'ai.settings.error_check': 'Viga võtme kontrollimisel teenusepakkuja juures.',
  'ai.settings.form_title': 'Seadistage oma võti (BYOK)',
  'ai.settings.form_help':
    'Kasutage oma Mistral AI, OpenAI, Anthropicu või Zhipu AI mandaate, et mitte sõltuda igakuisest kvoodist.',
  'ai.settings.provider_mistral': 'Mistral AI (EL, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EL, Prantsusmaa',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Rahvusvaheline',
  'ai.settings.show_key': 'Näita võtit',
  'ai.settings.hide_key': 'Peida võti',
  'ai.settings.key_required': 'Sisestage võti.',
  'ai.settings.notice_required':
    'Peate nõustuma teatega andmete edastamise kohta väljapoole EMP-d.',
  'ai.settings.check_ok': 'Võti on teenusepakkuja juures edukalt kontrollitud.',
  'ai.settings.error_save': 'Võtit ei õnnestunud salvestada.',
  'ai.settings.error_delete': 'Võtit ei õnnestunud kustutada.',
  'ai.settings.checking': 'Kontrollimine…',
  'ai.settings.save_activate': 'Salvesta ja aktiveeri',
  'ai.settings.key_deleted': 'Võti on kustutatud. Kasutatakse vaikimisi igakuist kvooti.',
  'ai.settings.active_provider': 'Aktiivne pakkuja: {provider} ({model})',
  'ai.settings.no_key_default': 'Oma võtit pole määratud. Kehtib paketi igakuine limiit.',
  'ai.settings.status_plan': 'Paketi kvoot',
  'ai.settings.quota_unlimited': 'Piiramatu',
  'ai.settings.quota_unlimited_help': 'Päringud töödeldakse teie oma API-võtmega.',
  'ai.settings.quota_jobs': 'Kujundamise, tõlkimise ja ekspordi tööd.',
  'ai.settings.loading': 'Tehisintellekti seadete laadimine',
  'ai.settings.load_error': 'Tehisintellekti seadeid ei õnnestunud laadida.',
  'ai.settings.load_error_help': 'Teenusega ei saanud ühendust. Teie seaded pole muutunud.',
  'ai.settings.key_invalid':
    'Teenusepakkuja lükkas võtme tagasi. Kontrollige, et see oleks õige ja aktiivne.',
  'ai.settings.encryption_unavailable':
    'Teenus pole veel valmis võtmeid turvaliselt talletama. Võtke ühendust klienditoega.',
  'ai.settings.invalid_model': 'Valige selle teenusepakkuja jaoks kehtiv mudel.',
  'ai.settings.key_state': 'Võtme olek',

  'ai.common.cancel': 'Loobu',

  // Ühine kontekst: kvoot, GDPR-i teade, vead ja töö olek
  'ai.context.quota_label': 'Igakuine tehisintellekti kvoot',
  'ai.context.quota_used': '{used} / {limit} tööd sel kuul',
  'ai.context.quota_reset': 'Uueneb {date}.',
  'ai.context.quota_exhausted_title': 'Igakuine tehisintellekti kvoot on ammendatud',
  'ai.context.quota_exhausted_desc':
    'Rohkem töid saate käivitada {date}. Haldus saab limiidi eemaldada, lisades tehisintellekti seadetes oma API-võtme.',
  'ai.context.byok': 'Teie organisatsioon kasutab oma API-võtit: see töö ei kuluta kvooti.',
  'ai.context.loading': 'Tehisintellekti kvoodi kontrollimine',
  'ai.context.load_error':
    'Tehisintellekti kvooti ei õnnestunud kontrollida. Võite siiski proovida.',
  'ai.context.retry': 'Proovi uuesti',
  'ai.notice.eu': 'Tekste töödeldakse EL-i piires kooskõlas GDPR-iga.',
  'ai.notice.non_eu_title': 'See töö liigub Euroopa Majanduspiirkonnast välja',
  'ai.notice.non_eu_desc':
    'Teie organisatsiooni oma võti kuulub pakkujale {provider}, kes töötleb andmeid väljaspool EMP-d. Teie organisatsioon nõustus selle edastamisega (GDPR art 49 lg 1 p a) võtme salvestamisel.',
  'ai.error.quota': 'Igakuine tehisintellekti kvoot on ammendatud. See uueneb {date}.',
  'ai.error.quota_nodate': 'Igakuine tehisintellekti kvoot on ammendatud.',
  'ai.error.busy': 'Tehisintellekti tööd juba käivad. Oodake nende lõppemist ja proovige uuesti.',
  'ai.error.admin': 'Seda saab teha ainult organisatsiooni haldus.',
  'ai.error.prompt': 'Enne kavandi loomist kirjeldage oma saiti.',
  'ai.error.languages': 'Valige vähemalt üks sihtkeel.',
  'ai.error.session': 'Teie seanss on aegunud. Laadige leht uuesti ja logige sisse.',
  'ai.error.generic': 'Tööd ei õnnestunud käivitada. Proovige mõne minuti pärast uuesti.',
  'ai.error.draft_not_saved':
    'Mustandit ei õnnestunud salvestada. Lahendage salvestamisprobleem ja käivitage tehisintellekt uuesti.',
  'ai.job.state_queued': 'Järjekorras',
  'ai.job.state_running': 'Töötlemisel',
  'ai.job.state_done': 'Valmis',
  'ai.job.state_failed': 'Ebaõnnestus',
  'ai.job.queued_text': 'Oodatakse, kuni server töö vastu võtab…',
  'ai.job.running_text': 'Tehisintellekt töötab. See võib võtta paar minutit.',
  'ai.job.done_text': 'Töö on lõppenud.',
  'ai.job.failed_text': 'Tööd ei õnnestunud lõpule viia.',
  'ai.job.lost_text': 'Ühendus serveriga katkes.',
  'ai.job.elapsed': 'Kulunud aeg: {time}',
  'ai.job.detail': 'Tehniline üksikasi: {detail}',
  'ai.job.model': 'Mudel: {model}',
  'ai.job.badge_eu': 'EL-i piires',
  'ai.job.badge_non_eu': 'Väljaspool EMP-d',
  'ai.job.badge_byok': 'Oma võti',
  'ai.job.back': 'Tagasi',
  'ai.job.retry': 'Proovi uuesti',
  'ai.job.stop': 'Lõpeta ootamine',
  'ai.job.stop_help': 'Töö jätkub serveris ja läheb arvesse teie kvoodis.',

  // Kujundamine
  'ai.design.title': 'Kujunda tehisintellektiga',
  'ai.design.desc':
    'Kirjeldage oma ettevõtet ja tehisintellekt pakub välja lehe jaotised koos muutmiseks valmis tekstidega.',
  'ai.design.prompt_label': 'Kirjeldage oma saiti või ettevõtet',
  'ai.design.prompt_placeholder':
    'Nt Jätkusuutlike naisterõivaste pood kataloogi ja broneerimisega…',
  'ai.design.prompt_help':
    'Mida täpsemalt kirjeldate (mida pakute, kellele ja mida külastajad peaksid tegema), seda parem on kavand.',
  'ai.design.ideas_label': 'Ideed alustamiseks',
  'ai.design.idea_1': 'Käsitööjuustude ja -veinide gurmeepood veebitellimustega',
  'ai.design.idea_2': 'Arhitektuuri- ja renoveerimisbüroo projektigaleriiga',
  'ai.design.idea_3': 'Hambaravikliinik broneerimise, meeskonnatutvustuse ja KKK-ga',
  'ai.design.idea_4': 'Itaalia restoran degustatsioonimenüü ja lauabroneerimisega',
  'ai.design.style_label': 'Visuaalne stiil',
  'ai.design.style_modern': 'Moodne ja puhas',
  'ai.design.style_elegant': 'Elegantne ja viimistletud',
  'ai.design.style_bold': 'Julge ja pilkupüüdev',
  'ai.design.style_minimal': 'Minimalistlik',
  'ai.design.lang_label': 'Sisu keel',
  'ai.design.button_generate': 'Loo kavand',
  'ai.design.job_title': 'Kavandi koostamine',
  'ai.design.result_title': 'Kavand on valmis',
  'ai.design.result_sections': {
    one: '{count} jaotis on lõuendile lisamiseks valmis.',
    other: '{count} jaotist on lõuendile lisamiseks valmis.',
  },
  'ai.design.result_help':
    'Need lisatakse lehele, mida parajasti muudate, ja saate neid hiljem kohandada.',
  'ai.design.result_empty': 'Tehisintellekt ei tagastanud ühtegi kasutatavat jaotist.',
  'ai.design.button_apply': 'Lisa lõuendile',
  'ai.design.button_again': 'Proovi teist ideed',

  // Tõlkimine
  'ai.translate.title': 'Tõlgi tehisintellektiga',
  'ai.translate.desc': 'Tõlkige saidi tekstid teistesse keeltesse kujundust muutmata.',
  'ai.translate.from_label': 'Lähtekeel',
  'ai.translate.to_label': 'Sihtkeeled',
  'ai.translate.to_help':
    'Valige vähemalt üks. Lähtekeel ei saa olla sihtkeel. Tulemuse rakendamisel lisatakse saidile uued keeled.',
  'ai.translate.scope_label': 'Tõlke ulatus',
  'ai.translate.scope_site': 'Kogu sait',
  'ai.translate.scope_page': 'Praegune leht',
  'ai.translate.scope_store': 'Ainult poe kataloog',
  'ai.translate.store_unavailable': 'Poe kataloogi tõlkimine pole veel saadaval.',
  'ai.translate.scope_invalid': 'Valige tõlkimiseks olemasolev leht või kogu sait.',
  'ai.translate.overwrite_label': 'Kirjuta olemasolevad tõlked üle',
  'ai.translate.button_translate': 'Tõlgi',
  'ai.translate.job_title': 'Tekstide tõlkimine',
  'ai.translate.result_title': 'Tõlked on valmis',
  'ai.translate.result_texts': {
    one: '{count} tekst tõlgitud.',
    other: '{count} teksti tõlgitud.',
  },
  'ai.translate.result_help': 'Need lisatakse mustandisse; kujundus ei muutu.',
  'ai.translate.result_empty': 'Tehisintellekt ei tagastanud ühtegi tõlget.',
  'ai.translate.button_apply': 'Rakenda tõlked',
  'ai.translate.button_again': 'Tõlgi teistesse keeltesse',

  // Eksport
  'ai.export.title': 'Ekspordi tehisintellektiga',
  'ai.export.desc':
    'Looge pakett oma saidiga, mis on valmis teisele platvormile viimiseks ilma Sytue’st sõltumata.',
  'ai.export.destination_label': 'Sihtplatvorm',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Täielik teema JSON-mallide ja Liquidi jaotistega ning kataloog CSV-vormingus Shopify halduspaneelist importimiseks.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Üheleheline projekt, mis on ühendatud Sytue avaliku API-ga ning sisaldab kassat ja vorme.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Serveripoolse renderdamisega või staatiline rakendus integreeritud makseväravaga.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'WordPressi algupärane plokiteema koos toodete ja atribuutide ekspordiga.',
  'ai.export.badge_available': 'Saadaval',
  'ai.export.badge_soon': 'Peagi saadaval',
  'ai.export.button_export': 'Loo pakett',
  'ai.export.job_title': 'Paketi loomine platvormile {destination}',
  'ai.export.result_title': 'Pakett on valmis',
  'ai.export.result_help': 'Teema ja kataloogi CSV on koos ühes ZIP-failis.',
  'ai.export.steps_title': 'Kuidas paigaldada see Shopifysse',
  'ai.export.step_1': 'Laadige ZIP-fail alla.',
  'ai.export.step_2':
    'Avage Shopify halduspaneelis Online Store > Themes ja valige „Upload zip file“.',
  'ai.export.step_3':
    'Toodete jaoks avage Products > Import ja laadige üles ZIP-failis olev fail products_export_shopify.csv.',
  'ai.export.button_download': 'Laadi ZIP alla',
  'ai.export.button_again': 'Loo uus eksport',
};
