/**
 * Teachtaireachtaí Gaeilge don intleacht shaorga (ia).
 * Réimír: ai.*
 */
export const ia = {
  // Socruithe
  'ai.settings.provider': 'Soláthraí IS',
  'ai.settings.model': 'Samhail',
  'ai.settings.key': 'Eochair API féin (BYOK)',
  'ai.settings.key_placeholder': 'Cuir isteach d’eochair rúnda API…',
  'ai.settings.key_help':
    'Stóráiltear d’eochair criptithe le AES-256-GCM. Ní sheoltar ar ais chuig an mbrabhsálaí choíche í.',
  'ai.settings.check_key': 'Tástáil an eochair',
  'ai.settings.delete_key': 'Scrios an eochair',
  'ai.settings.status_configured': 'Eochair cumraithe',
  'ai.settings.tested_at': 'Fíoraithe an',
  'ai.settings.quota_title': 'Cuóta míosúil IS',
  'ai.settings.quota_reset': 'Athshocraítear an {date}',
  'ai.settings.notice_eu_title': 'Fógra maidir le haistriú sonraí lasmuigh den LEE',
  'ai.settings.notice_eu_desc':
    'Próiseálann an soláthraí roghnaithe ({provider}) sonraí lasmuigh den Limistéar Eorpach Eacnamaíoch. Faoi Airteagal 49.1.a den RGCS, caithfidh tú toiliú go follasach leis an aistriú seo le leanúint ar aghaidh.',
  'ai.settings.notice_eu_accept':
    'Toilím le haistriú idirnáisiúnta sonraí lasmuigh den LEE le haghaidh iarratas IS leis an soláthraí seo.',
  'ai.settings.success_save': 'Sábháladh agus fíoraíodh an eochair go rathúil.',
  'ai.settings.error_check': 'Earráid agus an eochair á fíorú leis an soláthraí.',
  'ai.settings.form_title': 'Cumraigh d’eochair féin (BYOK)',
  'ai.settings.form_help':
    'Úsáid do dhintiúir féin ó Mistral AI, OpenAI, Anthropic nó Zhipu AI ionas nach mbeidh tú ag brath ar an gcuóta míosúil.',
  'ai.settings.provider_mistral': 'Mistral AI (AE, RGCS)',
  'ai.settings.provider_openai': 'OpenAI (SAM)',
  'ai.settings.provider_anthropic': 'Anthropic (SAM)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'AE, an Fhrainc',
  'ai.region.us': 'SAM',
  'ai.region.intl': 'Idirnáisiúnta',
  'ai.settings.show_key': 'Taispeáin an eochair',
  'ai.settings.hide_key': 'Folaigh an eochair',
  'ai.settings.key_required': 'Cuir isteach eochair.',
  'ai.settings.notice_required':
    'Caithfidh tú glacadh leis an bhfógra maidir le haistrithe lasmuigh den LEE.',
  'ai.settings.check_ok': 'Seiceáladh an eochair go rathúil leis an soláthraí.',
  'ai.settings.error_save': 'Níorbh fhéidir an eochair a shábháil.',
  'ai.settings.error_delete': 'Níorbh fhéidir an eochair a scriosadh.',
  'ai.settings.checking': 'Á seiceáil…',
  'ai.settings.save_activate': 'Sábháil agus gníomhachtaigh',
  'ai.settings.key_deleted':
    'Scriosadh an eochair. Beidh feidhm ag an gcuóta míosúil réamhshocraithe.',
  'ai.settings.active_provider': 'Soláthraí gníomhach: {provider} ({model})',
  'ai.settings.no_key_default':
    'Níl eochair dá chuid féin cumraithe. Tá feidhm ag teorainn mhíosúil an phlean.',
  'ai.settings.status_plan': 'Cuóta an phlean',
  'ai.settings.quota_unlimited': 'Gan teorainn',
  'ai.settings.quota_unlimited_help': 'Iarratais á láimhseáil le d’eochair API féin.',
  'ai.settings.quota_jobs': 'Jabanna dearaidh, aistriúcháin agus easpórtála.',
  'ai.settings.loading': 'Socruithe IS á lódáil',
  'ai.settings.load_error': 'Níorbh fhéidir socruithe IS a lódáil.',
  'ai.settings.load_error_help':
    'Níorbh fhéidir teagmháil a dhéanamh leis an tseirbhís. Níor athraigh do chuid socruithe.',
  'ai.settings.key_invalid':
    'Dhiúltaigh an soláthraí don eochair. Seiceáil go bhfuil sí ceart agus gníomhach.',
  'ai.settings.encryption_unavailable':
    'Níl an tseirbhís réidh chun eochracha a stóráil go slán fós. Déan teagmháil leis an tacaíocht.',
  'ai.settings.invalid_model': 'Roghnaigh samhail bhailí don soláthraí seo.',
  'ai.settings.key_state': 'Stádas na heochrach',

  'ai.common.cancel': 'Cealaigh',

  // Comhthéacs coiteann: cuóta, fógra RGCS, earráidí agus stádas an jab
  'ai.context.quota_label': 'Cuóta míosúil IS',
  'ai.context.quota_used': '{used} as {limit} jab an mhí seo',
  'ai.context.quota_reset': 'Athnuaitear an {date}.',
  'ai.context.quota_exhausted_title': 'Tá an cuóta míosúil IS ídithe agat',
  'ai.context.quota_exhausted_desc':
    'Beidh tú in ann tuilleadh jabanna a thosú an {date}. Is féidir le riarachán na heagraíochta an teorainn a bhaint trí eochair API dá chuid féin a chur leis sna socruithe IS.',
  'ai.context.byok': 'Úsáideann d’eagraíocht a heochair API féin: ní ídíonn an jab seo aon chuóta.',
  'ai.context.loading': 'An cuóta IS á sheiceáil',
  'ai.context.load_error':
    'Níorbh fhéidir an cuóta IS a sheiceáil. Is féidir leat triail a bhaint as mar sin féin.',
  'ai.context.retry': 'Bain triail eile as',
  'ai.notice.eu': 'Próiseáiltear na téacsanna laistigh den AE, i gcomhréir leis an RGCS.',
  'ai.notice.non_eu_title': 'Fágann an jab seo an Limistéar Eorpach Eacnamaíoch',
  'ai.notice.non_eu_desc':
    'Is le {provider} eochair féin d’eagraíochta, a phróiseálann sonraí lasmuigh den LEE. Ghlac d’eagraíocht leis an aistriú sin (Airteagal 49(1)(a) den RGCS) nuair a shábháil sí an eochair.',
  'ai.error.quota': 'Tá an cuóta míosúil IS ídithe. Athnuaitear é an {date}.',
  'ai.error.quota_nodate': 'Tá an cuóta míosúil IS ídithe.',
  'ai.error.busy':
    'Tá jabanna IS ar siúl cheana féin. Fan go gcríochnóidh siad agus bain triail eile as.',
  'ai.error.admin': 'Ní féidir ach le riarachán na heagraíochta é seo a dhéanamh.',
  'ai.error.prompt': 'Déan cur síos ar do shuíomh sula nginfidh tú an togra.',
  'ai.error.languages': 'Roghnaigh sprioctheanga amháin ar a laghad.',
  'ai.error.session':
    'Tá do sheisiún imithe in éag. Athlódáil an leathanach agus logáil isteach arís.',
  'ai.error.generic': 'Níorbh fhéidir an jab a thosú. Bain triail eile as i gceann cúpla nóiméad.',
  'ai.error.draft_not_saved':
    'Níorbh fhéidir an dréacht a shábháil. Réitigh an fhadhb shábhála agus tosaigh an IS arís.',
  'ai.job.state_queued': 'Sa scuaine',
  'ai.job.state_running': 'Ar siúl',
  'ai.job.state_done': 'Déanta',
  'ai.job.state_failed': 'Teipthe',
  'ai.job.queued_text': 'Ag fanacht leis an bhfreastalaí glacadh leis an jab…',
  'ai.job.running_text': 'Tá an IS ag obair. D’fhéadfadh sé cúpla nóiméad a thógáil.',
  'ai.job.done_text': 'Tá an jab críochnaithe.',
  'ai.job.failed_text': 'Níorbh fhéidir an jab a chríochnú.',
  'ai.job.lost_text': 'Cailleadh an ceangal leis an bhfreastalaí.',
  'ai.job.elapsed': 'Am caite: {time}',
  'ai.job.detail': 'Sonraí teicniúla: {detail}',
  'ai.job.model': 'Samhail: {model}',
  'ai.job.badge_eu': 'Laistigh den AE',
  'ai.job.badge_non_eu': 'Lasmuigh den LEE',
  'ai.job.badge_byok': 'Eochair féin',
  'ai.job.back': 'Siar',
  'ai.job.retry': 'Bain triail eile as',
  'ai.job.stop': 'Stop ag fanacht',
  'ai.job.stop_help':
    'Leanfaidh an jab ar aghaidh ar an bhfreastalaí agus comhairfear é i do chuóta.',

  // Dearadh
  'ai.design.title': 'Dear le hIS',
  'ai.design.desc':
    'Déan cur síos ar do ghnó agus molfaidh an IS rannáin an leathanaigh, le téacsanna réidh le cur in eagar.',
  'ai.design.prompt_label': 'Déan cur síos ar do shuíomh nó ar do ghnó',
  'ai.design.prompt_placeholder': 'M.sh. Siopa faisin inbhuanaithe le catalóg agus coinní…',
  'ai.design.prompt_help':
    'Dá shonraí a bheidh tú (cad a thairgeann tú, do cé, agus cad ba mhaith leat go ndéanfadh cuairteoirí), is ea is fearr a bheidh an togra.',
  'ai.design.ideas_label': 'Smaointe le tosú',
  'ai.design.idea_1':
    'Siopa bia gairméise le cáiseanna agus fíonta ceardaíochta agus orduithe ar líne',
  'ai.design.idea_2': 'Stiúideo ailtireachta agus athchóirithe le gailearaí tionscadal',
  'ai.design.idea_3': 'Clinic fiaclóireachta le coinní, foireann leighis agus ceisteanna coitianta',
  'ai.design.idea_4': 'Bialann Iodálach le biachlár blaiseadh agus áirithint boird',
  'ai.design.style_label': 'Stíl amhairc',
  'ai.design.style_modern': 'Nua-aimseartha agus glan',
  'ai.design.style_elegant': 'Galánta agus mín',
  'ai.design.style_bold': 'Dána agus beoga',
  'ai.design.style_minimal': 'Íostach',
  'ai.design.lang_label': 'Teanga an ábhair',
  'ai.design.button_generate': 'Gin togra',
  'ai.design.job_title': 'An togra á dhearadh',
  'ai.design.result_title': 'Togra réidh',
  'ai.design.result_sections': {
    one: '{count} rannán réidh le cur leis an chanbhás.',
    two: '{count} rannán réidh le cur leis an chanbhás.',
    few: '{count} rannán réidh le cur leis an chanbhás.',
    many: '{count} rannán réidh le cur leis an chanbhás.',
    other: '{count} rannán réidh le cur leis an chanbhás.',
  },
  'ai.design.result_help':
    'Cuirfear leis an leathanach atá á chur in eagar agat iad agus is féidir leat iad a choigeartú ina dhiaidh sin.',
  'ai.design.result_empty': 'Níor thug an IS aon rannán inúsáidte ar ais.',
  'ai.design.button_apply': 'Cuir leis an chanbhás',
  'ai.design.button_again': 'Bain triail as smaoineamh eile',

  // Aistriúchán
  'ai.translate.title': 'Aistrigh le hIS',
  'ai.translate.desc': 'Aistrigh téacsanna an tsuímh go teangacha eile gan baint leis an dearadh.',
  'ai.translate.from_label': 'Teanga fhoinseach',
  'ai.translate.to_label': 'Sprioctheangacha',
  'ai.translate.to_help':
    'Roghnaigh ceann amháin ar a laghad. Ní féidir an teanga fhoinseach a roghnú mar sprioc. Nuair a chuirfear an toradh i bhfeidhm, cuirfear teangacha nua leis an suíomh.',
  'ai.translate.scope_label': 'Raon an aistriúcháin',
  'ai.translate.scope_site': 'An suíomh ar fad',
  'ai.translate.scope_page': 'An leathanach reatha',
  'ai.translate.scope_store': 'Catalóg an tsiopa amháin',
  'ai.translate.store_unavailable': 'Níl aistriúchán chatalóg an tsiopa ar fáil fós.',
  'ai.translate.scope_invalid':
    'Roghnaigh leathanach atá ann cheana nó an suíomh ar fad le haistriú.',
  'ai.translate.overwrite_label': 'Forscríobh aistriúcháin atá ann cheana',
  'ai.translate.button_translate': 'Aistrigh',
  'ai.translate.job_title': 'Na téacsanna á n-aistriú',
  'ai.translate.result_title': 'Aistriúcháin réidh',
  'ai.translate.result_texts': {
    one: '{count} téacs aistrithe.',
    two: '{count} théacs aistrithe.',
    few: '{count} théacs aistrithe.',
    many: '{count} dtéacs aistrithe.',
    other: '{count} téacs aistrithe.',
  },
  'ai.translate.result_help': 'Cuirfear leis an dréacht iad; ní athraíonn an dearadh.',
  'ai.translate.result_empty': 'Níor thug an IS aon aistriúchán ar ais.',
  'ai.translate.button_apply': 'Cuir na haistriúcháin i bhfeidhm',
  'ai.translate.button_again': 'Aistrigh go teangacha eile',

  // Easpórtáil
  'ai.export.title': 'Easpórtáil le hIS',
  'ai.export.desc':
    'Gin pacáiste le do shuíomh réidh le tabhairt chuig ardán eile, gan bheith ag brath ar Sytue.',
  'ai.export.destination_label': 'Ardán sprice',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Téama iomlán le teimpléid JSON agus rannáin Liquid, mar aon leis an gcatalóg mar CSV le hiompórtáil ó phainéal riaracháin Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Tionscadal leathanaigh aonair ceangailte le hAPI poiblí Sytue, le seiceáil amach agus foirmeacha.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Feidhmchlár rindreáilte ar an bhfreastalaí nó statach leis an ngeata íocaíochta ionsuite.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Téama bloc dúchasach WordPress, le heaspórtáil táirgí agus tréithe.',
  'ai.export.badge_available': 'Ar fáil',
  'ai.export.badge_soon': 'Go luath',
  'ai.export.button_export': 'Gin pacáiste',
  'ai.export.job_title': 'An pacáiste do {destination} á ghiniúint',
  'ai.export.result_title': 'Pacáiste réidh',
  'ai.export.result_help': 'Tagann an téama agus CSV na catalóige le chéile in ZIP amháin.',
  'ai.export.steps_title': 'Conas é a shuiteáil in Shopify',
  'ai.export.step_1': 'Íoslódáil an comhad ZIP.',
  'ai.export.step_2':
    'I bpainéal riaracháin Shopify, téigh go Siopa ar líne > Téamaí agus roghnaigh “Uaslódáil comhad zip”.',
  'ai.export.step_3':
    'I gcás táirgí, téigh go Táirgí > Iompórtáil agus uaslódáil an comhad products_export_shopify.csv atá laistigh den ZIP.',
  'ai.export.button_download': 'Íoslódáil ZIP',
  'ai.export.button_again': 'Gin easpórtáil eile',
};
