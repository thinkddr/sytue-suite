/**
 * Íslenskir textar fyrir gervigreind (ia).
 * Forskeyti: ai.*
 */
export const ia = {
  // Stillingar
  'ai.settings.provider': 'Gervigreindarveita',
  'ai.settings.model': 'Líkan',
  'ai.settings.key': 'Eigin API-lykill (BYOK)',
  'ai.settings.key_placeholder': 'Sláðu inn leynilega API-lykilinn þinn…',
  'ai.settings.key_help':
    'Lykillinn þinn er geymdur dulkóðaður með AES-256-GCM. Hann er aldrei sendur aftur í vafrann.',
  'ai.settings.check_key': 'Prófa lykil',
  'ai.settings.delete_key': 'Eyða lykli',
  'ai.settings.status_configured': 'Lykill stilltur',
  'ai.settings.tested_at': 'Staðfest þann',
  'ai.settings.quota_title': 'Mánaðarlegur gervigreindarkvóti',
  'ai.settings.quota_reset': 'Endurstillist {date}',
  'ai.settings.notice_eu_title': 'Tilkynning um gagnaflutning út fyrir EES',
  'ai.settings.notice_eu_desc':
    'Valin þjónustuveita ({provider}) vinnur gögn utan Evrópska efnahagssvæðisins. Samkvæmt a-lið 1. mgr. 49. gr. GDPR þarftu að veita skýrt samþykki fyrir þessum flutningi til að halda áfram.',
  'ai.settings.notice_eu_accept':
    'Ég samþykki alþjóðlegan flutning gagna út fyrir EES fyrir gervigreindarbeiðnir hjá þessari þjónustuveitu.',
  'ai.settings.success_save': 'Lykill vistaður og sannreyndur.',
  'ai.settings.error_check': 'Villa við að sannreyna lykil hjá þjónustuveitu.',
  'ai.settings.form_title': 'Setja upp eigin lykil (BYOK)',
  'ai.settings.form_help':
    'Notaðu þín eigin skilríki hjá Mistral AI, OpenAI, Anthropic eða Zhipu AI til að vera óháð mánaðarlega kvótanum.',
  'ai.settings.provider_mistral': 'Mistral AI (ESB, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (BNA)',
  'ai.settings.provider_anthropic': 'Anthropic (BNA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'ESB, Frakkland',
  'ai.region.us': 'BNA',
  'ai.region.intl': 'Alþjóðlegt',
  'ai.settings.show_key': 'Sýna lykil',
  'ai.settings.hide_key': 'Fela lykil',
  'ai.settings.key_required': 'Sláðu inn lykil.',
  'ai.settings.notice_required': 'Þú verður að samþykkja tilkynninguna um flutning út fyrir EES.',
  'ai.settings.check_ok': 'Lykill sannreyndur hjá þjónustuveitu.',
  'ai.settings.error_save': 'Ekki tókst að vista lykilinn.',
  'ai.settings.error_delete': 'Ekki tókst að eyða lyklinum.',
  'ai.settings.checking': 'Athugar…',
  'ai.settings.save_activate': 'Vista og virkja',
  'ai.settings.key_deleted': 'Lykli eytt. Sjálfgefinn mánaðarlegur kvóti verður notaður.',
  'ai.settings.active_provider': 'Virk þjónustuveita: {provider} ({model})',
  'ai.settings.no_key_default':
    'Enginn eigin lykill stilltur. Mánaðarleg mörk áskriftarinnar gilda.',
  'ai.settings.status_plan': 'Kvóti áskriftar',
  'ai.settings.quota_unlimited': 'Ótakmarkað',
  'ai.settings.quota_unlimited_help': 'Beiðnir afgreiddar með þínum eigin API-lykli.',
  'ai.settings.quota_jobs': 'Hönnunar-, þýðingar- og útflutningsverk.',
  'ai.settings.loading': 'Hleður stillingum gervigreindar',
  'ai.settings.load_error': 'Ekki tókst að hlaða stillingum gervigreindar.',
  'ai.settings.load_error_help':
    'Ekki náðist samband við þjónustuna. Stillingar þínar hafa ekki breyst.',
  'ai.settings.key_invalid':
    'Þjónustuveitan hafnaði lyklinum. Gakktu úr skugga um að hann sé réttur og virkur.',
  'ai.settings.encryption_unavailable':
    'Þjónustan er ekki enn tilbúin til að geyma lykla á öruggan hátt. Hafðu samband við þjónustuver.',
  'ai.settings.invalid_model': 'Veldu gilt líkan fyrir þessa þjónustuveitu.',
  'ai.settings.key_state': 'Staða lykils',

  'ai.common.cancel': 'Hætta við',

  // Sameiginlegt samhengi: kvóti, GDPR-tilkynning, villur og staða verks
  'ai.context.quota_label': 'Mánaðarlegur gervigreindarkvóti',
  'ai.context.quota_used': '{used} af {limit} verkum í þessum mánuði',
  'ai.context.quota_reset': 'Endurnýjast {date}.',
  'ai.context.quota_exhausted_title': 'Mánaðarlegur gervigreindarkvóti er fullnýttur',
  'ai.context.quota_exhausted_desc':
    'Þú getur sett fleiri verk í gang þann {date}. Stjórnendur fyrirtækisins geta fellt mörkin niður með því að bæta við eigin API-lykli í stillingum gervigreindar.',
  'ai.context.byok': 'Fyrirtækið þitt notar eigin API-lykil: þetta verk notar ekki kvóta.',
  'ai.context.loading': 'Athugar gervigreindarkvóta',
  'ai.context.load_error': 'Ekki tókst að athuga gervigreindarkvótann. Þú getur samt reynt.',
  'ai.context.retry': 'Reyna aftur',
  'ai.notice.eu': 'Textar eru unnir innan ESB í samræmi við GDPR.',
  'ai.notice.non_eu_title': 'Þetta verk fer út fyrir Evrópska efnahagssvæðið',
  'ai.notice.non_eu_desc':
    'Eigin lykill fyrirtækisins tilheyrir {provider}, sem vinnur gögn utan EES. Fyrirtækið þitt samþykkti þann flutning (a-liður 1. mgr. 49. gr. GDPR) þegar lykillinn var vistaður.',
  'ai.error.quota': 'Mánaðarlegur gervigreindarkvóti er fullnýttur. Hann endurnýjast {date}.',
  'ai.error.quota_nodate': 'Mánaðarlegur gervigreindarkvóti er fullnýttur.',
  'ai.error.busy': 'Gervigreindarverk eru þegar í gangi. Bíddu þar til þeim lýkur og reyndu aftur.',
  'ai.error.admin': 'Aðeins stjórnendur fyrirtækisins geta gert þetta.',
  'ai.error.prompt': 'Lýstu vefsvæðinu þínu áður en tillagan er búin til.',
  'ai.error.languages': 'Veldu að minnsta kosti eitt marktungumál.',
  'ai.error.session': 'Lotan þín er útrunnin. Endurhladdu síðuna og skráðu þig inn aftur.',
  'ai.error.generic': 'Ekki tókst að ræsa verkið. Reyndu aftur eftir nokkrar mínútur.',
  'ai.error.draft_not_saved':
    'Ekki tókst að vista drögin. Leystu vistunarvandamálið og ræstu gervigreindina aftur.',
  'ai.job.state_queued': 'Í biðröð',
  'ai.job.state_running': 'Í gangi',
  'ai.job.state_done': 'Lokið',
  'ai.job.state_failed': 'Misheppnaðist',
  'ai.job.queued_text': 'Bíður eftir að netþjónninn taki við verkinu…',
  'ai.job.running_text': 'Gervigreindin er að störfum. Þetta getur tekið nokkrar mínútur.',
  'ai.job.done_text': 'Verkinu er lokið.',
  'ai.job.failed_text': 'Ekki tókst að ljúka verkinu.',
  'ai.job.lost_text': 'Sambandið við netþjóninn rofnaði.',
  'ai.job.elapsed': 'Liðinn tími: {time}',
  'ai.job.detail': 'Tæknileg lýsing: {detail}',
  'ai.job.model': 'Líkan: {model}',
  'ai.job.badge_eu': 'Innan ESB',
  'ai.job.badge_non_eu': 'Utan EES',
  'ai.job.badge_byok': 'Eigin lykill',
  'ai.job.back': 'Til baka',
  'ai.job.retry': 'Reyna aftur',
  'ai.job.stop': 'Hætta að bíða',
  'ai.job.stop_help': 'Verkið heldur áfram á netþjóninum og telst til kvótans þíns.',

  // Hönnun
  'ai.design.title': 'Hanna með gervigreind',
  'ai.design.desc':
    'Lýstu rekstrinum þínum og gervigreindin leggur til hluta síðunnar með textum sem eru tilbúnir til breytinga.',
  'ai.design.prompt_label': 'Lýstu vefsvæðinu eða rekstrinum',
  'ai.design.prompt_placeholder': 'T.d. Sjálfbær fataverslun með vörulista og tímabókunum…',
  'ai.design.prompt_help':
    'Því nákvæmari sem lýsingin er (hvað þú býður upp á, fyrir hverja og hvað fólk sem heimsækir vefinn á að gera), því betri verður tillagan.',
  'ai.design.ideas_label': 'Hugmyndir til að byrja',
  'ai.design.idea_1': 'Sælkeraverslun með handverksosta og vín ásamt netpöntunum',
  'ai.design.idea_2': 'Arkitekta- og endurbótastofa með verkefnagalleríi',
  'ai.design.idea_3': 'Tannlæknastofa með tímabókunum, starfsfólki og algengum spurningum',
  'ai.design.idea_4': 'Ítalskur veitingastaður með smakkseðli og borðapöntunum',
  'ai.design.style_label': 'Sjónrænn stíll',
  'ai.design.style_modern': 'Nútímalegur og hreinn',
  'ai.design.style_elegant': 'Glæsilegur og fágaður',
  'ai.design.style_bold': 'Djarfur og líflegur',
  'ai.design.style_minimal': 'Naumhyggjulegur',
  'ai.design.lang_label': 'Tungumál efnis',
  'ai.design.button_generate': 'Búa til tillögu',
  'ai.design.job_title': 'Hannar tillöguna',
  'ai.design.result_title': 'Tillaga tilbúin',
  'ai.design.result_sections': {
    one: '{count} hluti tilbúinn til að bæta á strigann.',
    other: '{count} hlutar tilbúnir til að bæta á strigann.',
  },
  'ai.design.result_help':
    'Þeim verður bætt við síðuna sem þú ert að breyta og þú getur lagað þá á eftir.',
  'ai.design.result_empty': 'Gervigreindin skilaði engum nothæfum hluta.',
  'ai.design.button_apply': 'Bæta á strigann',
  'ai.design.button_again': 'Prófa aðra hugmynd',

  // Þýðing
  'ai.translate.title': 'Þýða með gervigreind',
  'ai.translate.desc':
    'Þýddu texta vefsvæðisins yfir á önnur tungumál án þess að hrófla við útlitinu.',
  'ai.translate.from_label': 'Upprunatungumál',
  'ai.translate.to_label': 'Marktungumál',
  'ai.translate.to_help':
    'Veldu að minnsta kosti eitt. Upprunatungumálið getur ekki verið marktungumál. Þegar niðurstaðan er virkjuð bætast ný tungumál við vefsvæðið.',
  'ai.translate.scope_label': 'Umfang þýðingar',
  'ai.translate.scope_site': 'Allt vefsvæðið',
  'ai.translate.scope_page': 'Núverandi síða',
  'ai.translate.scope_store': 'Aðeins vörulisti verslunar',
  'ai.translate.store_unavailable': 'Þýðing á vörulista verslunar er ekki enn í boði.',
  'ai.translate.scope_invalid': 'Veldu síðu sem er til eða allt vefsvæðið til að þýða.',
  'ai.translate.overwrite_label': 'Skrifa yfir þýðingar sem þegar eru til',
  'ai.translate.button_translate': 'Þýða',
  'ai.translate.job_title': 'Þýðir textana',
  'ai.translate.result_title': 'Þýðingar tilbúnar',
  'ai.translate.result_texts': {
    one: '{count} texti þýddur.',
    other: '{count} textar þýddir.',
  },
  'ai.translate.result_help': 'Þeim verður bætt við drögin; útlitið breytist ekki.',
  'ai.translate.result_empty': 'Gervigreindin skilaði engri þýðingu.',
  'ai.translate.button_apply': 'Nota þýðingar',
  'ai.translate.button_again': 'Þýða á önnur tungumál',

  // Útflutningur
  'ai.export.title': 'Flytja út með gervigreind',
  'ai.export.desc':
    'Búðu til pakka með vefsvæðinu þínu tilbúnu til flutnings á annan vettvang, án þess að vera háð Sytue.',
  'ai.export.destination_label': 'Markvettvangur',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Heildstætt þema með JSON-sniðmátum og Liquid-hlutum, ásamt vörulistanum í CSV-skrá til innflutnings í stjórnborði Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Einsíðuverkefni tengt við opinbera API-viðmót Sytue, með greiðslugátt og eyðublöðum.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Forrit með myndgerð á netþjóni eða kyrrstætt, með innbyggðri greiðslugátt.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Innbyggt kubbaþema fyrir WordPress, með útflutningi á vörum og eiginleikum.',
  'ai.export.badge_available': 'Í boði',
  'ai.export.badge_soon': 'Væntanlegt',
  'ai.export.button_export': 'Búa til pakka',
  'ai.export.job_title': 'Býr til pakkann fyrir {destination}',
  'ai.export.result_title': 'Pakki tilbúinn',
  'ai.export.result_help': 'Þemað og CSV-skrá vörulistans fylgja saman í einni ZIP-skrá.',
  'ai.export.steps_title': 'Hvernig á að setja hann upp í Shopify',
  'ai.export.step_1': 'Sæktu ZIP-skrána.',
  'ai.export.step_2':
    'Í stjórnborði Shopify skaltu fara í Online Store > Themes og velja „Upload zip file“.',
  'ai.export.step_3':
    'Fyrir vörur skaltu fara í Products > Import og hlaða upp skránni products_export_shopify.csv sem er inni í ZIP-skránni.',
  'ai.export.button_download': 'Sækja ZIP',
  'ai.export.button_again': 'Búa til annan útflutning',
};
