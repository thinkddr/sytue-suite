/**
 * Mensajes en letón (lv) para el área de inteligencia artificial (ia).
 * Prefijo: ai.*
 */
export const ia = {
  // Iestatījumi
  'ai.settings.provider': 'MI nodrošinātājs',
  'ai.settings.model': 'Modelis',
  'ai.settings.key': 'Sava API atslēga (BYOK)',
  'ai.settings.key_placeholder': 'Ievadiet savu slepeno API atslēgu…',
  'ai.settings.key_help':
    'Jūsu atslēga tiek glabāta šifrēta ar AES-256-GCM. Tā nekad netiek atgriezta pārlūkprogrammai.',
  'ai.settings.check_key': 'Pārbaudīt atslēgu',
  'ai.settings.delete_key': 'Dzēst atslēgu',
  'ai.settings.status_configured': 'Atslēga konfigurēta',
  'ai.settings.tested_at': 'Verificēta',
  'ai.settings.quota_title': 'Mēneša MI kvota',
  'ai.settings.quota_reset': 'Atjaunojas {date}',
  'ai.settings.notice_eu_title': 'Paziņojums par datu nodošanu ārpus EEZ',
  'ai.settings.notice_eu_desc':
    'Izvēlētais nodrošinātājs ({provider}) apstrādā datus ārpus Eiropas Ekonomikas zonas. Saskaņā ar VDAR 49. panta 1. punkta a) apakšpunktu, lai turpinātu, ir skaidri jāpiekrīt šai datu nodošanai.',
  'ai.settings.notice_eu_accept':
    'Piekrītu starptautiskai datu nodošanai ārpus EEZ MI pieprasījumiem pie šī nodrošinātāja.',
  'ai.settings.success_save': 'Atslēga veiksmīgi saglabāta un verificēta.',
  'ai.settings.error_check': 'Kļūda, verificējot atslēgu pie nodrošinātāja.',
  'ai.settings.form_title': 'Iestatīt savu atslēgu (BYOK)',
  'ai.settings.form_help':
    'Izmantojiet savus Mistral AI, OpenAI, Anthropic vai Zhipu AI akreditācijas datus, lai nebūtu atkarības no mēneša kvotas.',
  'ai.settings.provider_mistral': 'Mistral AI (ES, VDAR)',
  'ai.settings.provider_openai': 'OpenAI (ASV)',
  'ai.settings.provider_anthropic': 'Anthropic (ASV)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'ES, Francija',
  'ai.region.us': 'ASV',
  'ai.region.intl': 'Starptautisks',
  'ai.settings.show_key': 'Rādīt atslēgu',
  'ai.settings.hide_key': 'Slēpt atslēgu',
  'ai.settings.key_required': 'Ievadiet atslēgu.',
  'ai.settings.notice_required': 'Jums jāpiekrīt paziņojumam par nodošanu ārpus EEZ.',
  'ai.settings.check_ok': 'Atslēga veiksmīgi pārbaudīta pie nodrošinātāja.',
  'ai.settings.error_save': 'Atslēgu neizdevās saglabāt.',
  'ai.settings.error_delete': 'Atslēgu neizdevās izdzēst.',
  'ai.settings.checking': 'Pārbauda…',
  'ai.settings.save_activate': 'Saglabāt un aktivizēt',
  'ai.settings.key_deleted': 'Atslēga izdzēsta. Tiks piemērota noklusējuma mēneša kvota.',
  'ai.settings.active_provider': 'Aktīvais nodrošinātājs: {provider} ({model})',
  'ai.settings.no_key_default': 'Sava atslēga nav iestatīta. Tiek piemērots plāna mēneša limits.',
  'ai.settings.status_plan': 'Plāna kvota',
  'ai.settings.quota_unlimited': 'Neierobežota',
  'ai.settings.quota_unlimited_help': 'Pieprasījumi tiek apstrādāti ar jūsu API atslēgu.',
  'ai.settings.quota_jobs': 'Dizaina, tulkošanas un eksportēšanas darbi.',
  'ai.settings.loading': 'Notiek MI iestatījumu ielāde',
  'ai.settings.load_error': 'MI iestatījumus neizdevās ielādēt.',
  'ai.settings.load_error_help':
    'Ar pakalpojumu neizdevās sazināties. Jūsu iestatījumi nav mainījušies.',
  'ai.settings.key_invalid':
    'Nodrošinātājs noraidīja atslēgu. Pārbaudiet, vai tā ir pareiza un aktīva.',
  'ai.settings.encryption_unavailable':
    'Pakalpojums vēl nav gatavs drošai atslēgu glabāšanai. Sazinieties ar atbalsta dienestu.',
  'ai.settings.invalid_model': 'Izvēlieties šim nodrošinātājam derīgu modeli.',
  'ai.settings.key_state': 'Atslēgas statuss',

  'ai.common.cancel': 'Atcelt',

  // Kopīgais konteksts: kvota, VDAR paziņojums, kļūdas un darba statuss
  'ai.context.quota_label': 'Mēneša MI kvota',
  'ai.context.quota_used': 'Šomēnes izmantoti {used} no {limit} darbiem',
  'ai.context.quota_reset': 'Atjaunojas {date}.',
  'ai.context.quota_exhausted_title': 'Mēneša MI kvota ir izsmelta',
  'ai.context.quota_exhausted_desc':
    'Vairāk darbu varēsiet palaist {date}. Organizācijas administrācija var atcelt limitu, pievienojot savu API atslēgu MI iestatījumos.',
  'ai.context.byok': 'Jūsu organizācija izmanto savu API atslēgu: šis darbs nepatērē kvotu.',
  'ai.context.loading': 'Pārbauda MI kvotu',
  'ai.context.load_error': 'MI kvotu neizdevās pārbaudīt. Joprojām varat mēģināt.',
  'ai.context.retry': 'Mēģināt vēlreiz',
  'ai.notice.eu': 'Teksti tiek apstrādāti ES robežās saskaņā ar VDAR.',
  'ai.notice.non_eu_title': 'Šis darbs iziet ārpus Eiropas Ekonomikas zonas',
  'ai.notice.non_eu_desc':
    'Jūsu organizācijas atslēga pieder pakalpojumam {provider}, kas apstrādā datus ārpus EEZ. Jūsu organizācija piekrita šai datu nodošanai (VDAR 49. panta 1. punkta a) apakšpunkts), saglabājot atslēgu.',
  'ai.error.quota': 'Mēneša MI kvota ir izsmelta. Tā atjaunojas {date}.',
  'ai.error.quota_nodate': 'Mēneša MI kvota ir izsmelta.',
  'ai.error.busy': 'MI darbi jau notiek. Uzgaidiet, līdz tie beidzas, un mēģiniet vēlreiz.',
  'ai.error.admin': 'To var darīt tikai organizācijas administrācija.',
  'ai.error.prompt': 'Pirms piedāvājuma ģenerēšanas aprakstiet savu vietni.',
  'ai.error.languages': 'Izvēlieties vismaz vienu mērķa valodu.',
  'ai.error.session':
    'Jūsu sesijas darbība ir beigusies. Pārlādējiet lapu un pierakstieties vēlreiz.',
  'ai.error.generic': 'Darbu neizdevās sākt. Mēģiniet vēlreiz pēc dažām minūtēm.',
  'ai.error.draft_not_saved':
    'Melnrakstu neizdevās saglabāt. Atrisiniet saglabāšanas problēmu un palaidiet MI vēlreiz.',
  'ai.job.state_queued': 'Rindā',
  'ai.job.state_running': 'Notiek',
  'ai.job.state_done': 'Gatavs',
  'ai.job.state_failed': 'Neizdevās',
  'ai.job.queued_text': 'Gaida, kamēr serveris pieņems darbu…',
  'ai.job.running_text': 'MI strādā. Tas var ilgt dažas minūtes.',
  'ai.job.done_text': 'Darbs ir pabeigts.',
  'ai.job.failed_text': 'Darbu neizdevās pabeigt.',
  'ai.job.lost_text': 'Savienojums ar serveri tika zaudēts.',
  'ai.job.elapsed': 'Pagājušais laiks: {time}',
  'ai.job.detail': 'Tehniskā informācija: {detail}',
  'ai.job.model': 'Modelis: {model}',
  'ai.job.badge_eu': 'ES robežās',
  'ai.job.badge_non_eu': 'Ārpus EEZ',
  'ai.job.badge_byok': 'Sava atslēga',
  'ai.job.back': 'Atpakaļ',
  'ai.job.retry': 'Mēģināt vēlreiz',
  'ai.job.stop': 'Pārtraukt gaidīšanu',
  'ai.job.stop_help': 'Darbs turpināsies serverī un tiks ieskaitīts jūsu kvotā.',

  // Dizains
  'ai.design.title': 'Veidot dizainu ar MI',
  'ai.design.desc':
    'Aprakstiet savu uzņēmumu, un MI piedāvās lapas sadaļas ar rediģēšanai gataviem tekstiem.',
  'ai.design.prompt_label': 'Aprakstiet savu vietni vai uzņēmumu',
  'ai.design.prompt_placeholder':
    'Piem., ilgtspējīgas sieviešu modes veikals ar katalogu un pierakstu…',
  'ai.design.prompt_help':
    'Jo konkrētāk aprakstīsiet (ko piedāvājat, kam un kas jāizdara vietnes apmeklējumā), jo labāks būs piedāvājums.',
  'ai.design.ideas_label': 'Idejas sākumam',
  'ai.design.idea_1': 'Gardēžu veikals amatnieku sieriem un vīniem ar pasūtījumiem tiešsaistē',
  'ai.design.idea_2': 'Arhitektūras un renovācijas studija ar projektu galeriju',
  'ai.design.idea_3': 'Zobārstniecības klīnika ar pierakstu, ārstu komandu un BUJ',
  'ai.design.idea_4': 'Itāļu restorāns ar degustācijas ēdienkarti un galdiņu rezervēšanu',
  'ai.design.style_label': 'Vizuālais stils',
  'ai.design.style_modern': 'Moderns un tīrs',
  'ai.design.style_elegant': 'Elegants un izsmalcināts',
  'ai.design.style_bold': 'Drosmīgs un košs',
  'ai.design.style_minimal': 'Minimālistisks',
  'ai.design.lang_label': 'Satura valoda',
  'ai.design.button_generate': 'Ģenerēt piedāvājumu',
  'ai.design.job_title': 'Veido piedāvājuma dizainu',
  'ai.design.result_title': 'Piedāvājums gatavs',
  'ai.design.result_sections': {
    zero: '{count} sadaļas ir gatavas pievienošanai audeklam.',
    one: '{count} sadaļa ir gatava pievienošanai audeklam.',
    other: '{count} sadaļas ir gatavas pievienošanai audeklam.',
  },
  'ai.design.result_help':
    'Tās tiks pievienotas lapai, kuru pašlaik rediģējat, un pēc tam varēsiet tās pielāgot.',
  'ai.design.result_empty': 'MI neatgrieza nevienu izmantojamu sadaļu.',
  'ai.design.button_apply': 'Pievienot audeklam',
  'ai.design.button_again': 'Izmēģināt citu ideju',

  // Tulkošana
  'ai.translate.title': 'Tulkot ar MI',
  'ai.translate.desc': 'Iztulkojiet vietnes tekstus citās valodās, nemainot dizainu.',
  'ai.translate.from_label': 'Avota valoda',
  'ai.translate.to_label': 'Mērķa valodas',
  'ai.translate.to_help':
    'Izvēlieties vismaz vienu. Avota valoda nevar būt mērķa valoda. Piemērojot rezultātu, vietnei tiek pievienotas jaunas valodas.',
  'ai.translate.scope_label': 'Tulkošanas apjoms',
  'ai.translate.scope_site': 'Visa vietne',
  'ai.translate.scope_page': 'Pašreizējā lapa',
  'ai.translate.scope_store': 'Tikai veikala katalogs',
  'ai.translate.store_unavailable': 'Veikala kataloga tulkošana vēl nav pieejama.',
  'ai.translate.scope_invalid': 'Tulkošanai izvēlieties esošu lapu vai visu vietni.',
  'ai.translate.overwrite_label': 'Pārrakstīt jau esošos tulkojumus',
  'ai.translate.button_translate': 'Tulkot',
  'ai.translate.job_title': 'Tulko tekstus',
  'ai.translate.result_title': 'Tulkojumi gatavi',
  'ai.translate.result_texts': {
    zero: 'Iztulkoti {count} teksti.',
    one: 'Iztulkots {count} teksts.',
    other: 'Iztulkoti {count} teksti.',
  },
  'ai.translate.result_help': 'Tie tiks pievienoti melnrakstam; dizains nemainās.',
  'ai.translate.result_empty': 'MI neatgrieza nevienu tulkojumu.',
  'ai.translate.button_apply': 'Piemērot tulkojumus',
  'ai.translate.button_again': 'Tulkot citās valodās',

  // Eksportēšana
  'ai.export.title': 'Eksportēt ar MI',
  'ai.export.desc':
    'Ģenerējiet pakotni ar savu vietni, kas ir gatava pārcelšanai uz citu platformu neatkarīgi no Sytue.',
  'ai.export.destination_label': 'Mērķa platforma',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Pilna tēma ar JSON veidnēm un Liquid sadaļām, kā arī katalogs CSV formātā importēšanai no Shopify administrācijas paneļa.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Vienas lapas projekts, kas savienots ar Sytue publisko API, ar norēķiniem un veidlapām.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Serverī renderēta vai statiska lietotne ar iebūvētu maksājumu vārteju.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Vietējā WordPress bloku tēma ar produktu un atribūtu eksportu.',
  'ai.export.badge_available': 'Pieejams',
  'ai.export.badge_soon': 'Drīzumā',
  'ai.export.button_export': 'Ģenerēt pakotni',
  'ai.export.job_title': 'Ģenerē pakotni platformai {destination}',
  'ai.export.result_title': 'Pakotne gatava',
  'ai.export.result_help': 'Tēma un kataloga CSV ir apvienoti vienā ZIP failā.',
  'ai.export.steps_title': 'Kā to instalēt Shopify',
  'ai.export.step_1': 'Lejupielādējiet ZIP failu.',
  'ai.export.step_2':
    'Shopify administrācijas panelī atveriet Online Store > Themes un izvēlieties “Upload zip file”.',
  'ai.export.step_3':
    'Produktiem atveriet Products > Import un augšupielādējiet ZIP failā esošo failu products_export_shopify.csv.',
  'ai.export.button_download': 'Lejupielādēt ZIP',
  'ai.export.button_again': 'Ģenerēt citu eksportu',
};
