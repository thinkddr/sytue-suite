/**
 * Teachtaireachtaí Gaeilge do chroílár Sytue Web (web).
 * Réimíreanna: nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Níorbh fhéidir seoladh poiblí an tsuímh a fháil.',
  'nav.title': 'Gréasán',
  'nav.sites': 'Mo shuíomhanna',
  'nav.new': 'Suíomh nua',
  'nav.settings': 'Socruithe',

  'sites.title': 'Mo shuíomhanna',
  'sites.empty': 'Níor chruthaigh tú aon suíomh fós.',
  'sites.create': 'Cruthaigh suíomh',
  'sites.status.published': 'Foilsithe',
  'sites.status.draft': 'Dréacht',
  'sites.restore': 'Athchóirigh',
  'sites.trash': 'Suíomhanna sa bhruscar',
  'sites.trash_help': 'Coinnítear suíomhanna scriosta ar feadh 30 lá sula scriostar go buan iad.',
  'sites.open_editor': 'Oscail an t-eagarthóir',
  'sites.manage': 'Bainistigh',
  'sites.last_updated': 'Nuashonraithe an {fecha}',
  'sites.filter_all': 'Gach ceann',
  'sites.filter_published': 'Foilsithe',
  'sites.filter_draft': 'Dréachtaí',
  'sites.count': {
    one: '{count} suíomh',
    two: '{count} shuíomh',
    few: '{count} shuíomh',
    many: '{count} suíomh',
    other: '{count} suíomh',
  },
  'sites.empty_help':
    'Roghnaigh teimpléad nó tosaigh le leathanach bán. Is féidir leat é a fhoilsiú nuair is mian leat.',
  'sites.filter_label': 'Scag de réir stádais',
  'sites.filter_empty': 'Ní mheaitseálann aon suíomh an scagaire seo.',
  'sites.loading': 'Suíomhanna á lódáil',
  'sites.load_error': 'Níorbh fhéidir na suíomhanna a lódáil',
  'sites.load_error_help': 'Seiceáil do nasc agus bain triail eile as.',
  'sites.retry': 'Bain triail eile as',
  'sites.restore_error': 'Níorbh fhéidir an suíomh a athchóiriú. Bain triail eile as.',
  'sites.days_left': {
    one: '{count} lá fágtha',
    two: '{count} lá fágtha',
    few: '{count} lá fágtha',
    many: '{count} lá fágtha',
    other: '{count} lá fágtha',
  },

  'new.title': 'Cruthaigh suíomh nua',
  'new.template_step': 'Roghnaigh teimpléad',
  'new.details_step': 'Sonraí an tsuímh',
  'new.name_label': 'Ainm an tsuímh',
  'new.slug_label': 'Aitheantóir (slug)',
  'new.languages_label': 'Teangacha an tsuímh',
  'new.submit': 'Cruthaigh agus oscail an t-eagarthóir',
  'new.step_template': 'Teimpléad',
  'new.step_info': 'Buneolas',
  'new.step_languages': 'Teangacha',
  'new.primary_language': 'Príomhtheanga',
  'new.step_review': 'Athbhreithniú',
  'new.details_help':
    'Tabhair ainm do do shuíomh. Ullmhóimid a sheoladh go huathoibríoch; is féidir leat d’fhearann féin a cheangal níos déanaí.',
  'new.review_heading': 'Réidh le tosú',
  'new.review_help': 'Déan athbhreithniú ar na roghanna seo sula gcruthóidh tú do shuíomh.',
  'new.draft_notice':
    'Cruthaíonn sé seo dréacht príobháideach. San eagarthóir is féidir leat na téacsanna, na grianghraif agus an dearadh a athrú. Ansin treoróimid thú chun athbhreithniú a dhéanamh, d’fhearann a cheangal agus foilsiú.',
  'new.template_blank': 'Bán',
  'new.template_blank_desc': 'Tosaigh le leathanach folamh agus dear go saor.',
  'new.languages_notice': 'Tuilleadh teangacha go luath',
  'new.slug_available': 'Tá an t-aitheantóir ar fáil',
  'new.slug_unavailable': 'Níl an t-aitheantóir seo ar fáil',
  'new.slug_checking': 'An t-aitheantóir á sheiceáil...',
  'new.next': 'Ar aghaidh',
  'new.back': 'Siar',
  'new.templates_error':
    'Níorbh fhéidir na teimpléid a lódáil. Is féidir leat tosú le leathanach bán.',
  'new.name_placeholder': 'Mo shiopa ar líne',
  'new.slug_placeholder': 'mo-shiopa',
  'new.slug_help':
    'Litreacha beaga, uimhreacha agus fleiscíní amháin. Cruthaíonn sé seoladh an tsuímh.',
  'new.slug_unverified':
    'Níorbh fhéidir an t-aitheantóir a sheiceáil anois; bailíochtófar é nuair a chruthófar an suíomh.',
  'new.main_language': 'Príomhtheanga',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Cumasaigh leagan dátheangach ({idioma})',
  'new.create_error': 'Níorbh fhéidir an suíomh a chruthú. Bain triail eile as.',
  'new.err_site_limit':
    'Tá 20 suíomh ag d’eagraíocht cheana, nó chruthaigh sí 5 cinn inniu. Scrios ceann amháin nó tar ar ais amárach.',
  'new.cancel': 'Cealaigh',
  'new.progress': 'Céimeanna chun an suíomh a chruthú',
  'new.go_to_step': 'Téigh go céim {n}',
  'new.step_of': 'Céim {n} as {total}',
  'app_settings.title': 'Socruithe Sytue Web',
  'new.search_title': 'Tosaigh le do chineál gnó',
  'new.search_help':
    'Clóscríobh cad a dhéanann tú nó roghnaigh grúpa. Tá foclaíocht do cheirde i ngach teimpléad cheana féin: ní athraíonn tú ach an t-ainm, an uimhir gutháin, na praghsanna agus na grianghraif.',
  'new.search_label': 'Do chineál gnó',
  'new.search_placeholder': 'Gruagaire, beár, pluiméir, siopa éadaí...',
  'new.all_groups': 'Gach ceann',
  'new.results': {
    one: '{count} teimpléad',
    two: '{count} theimpléad',
    few: '{count} theimpléad',
    many: '{count} dteimpléad',
    other: '{count} teimpléad',
  },
  'new.no_results': 'Ní mheaitseálann aon teimpléad “{q}”.',
  'new.no_results_help':
    'Bain triail as focal eile (mar shampla “siopa” nó “clinic”) nó tosaigh le leathanach bán.',
  'new.clear_search': 'Glan an cuardach',
  'new.select': 'Roghnaigh an teimpléad seo',
  'new.selected': 'Roghnaithe',
  'new.preview': 'Réamhamharc',
  'new.preview_close': 'Dún',
  'new.preview_loading': 'An teimpléad á lódáil...',
  'new.preview_error': 'Níorbh fhéidir an réamhamharc a lódáil.',
  'new.preview_desktop': 'Ríomhaire',
  'new.preview_mobile': 'Guthán',
  'new.preview_pages': 'Leathanaigh an teimpléid',
  'new.preview_note':
    'Amharc garbh: is ionadchoinneálaithe iad na grianghraif a gcuirfidh tú do chuid féin ina n-áit, agus athraítear na sonraí samplacha san eagarthóir.',
  'new.badge_store': 'Le siopa',
  'new.badge_custom': 'Saincheaptha',
  'new.cat.belleza': 'Áilleacht agus folláine',
  'new.cat.comida': 'Bia agus deoch',
  'new.cat.comercio': 'Siopaí agus miondíol',
  'new.cat.oficios': 'Ceirdeanna agus an baile',
  'new.cat.motor': 'Gluaisteáin agus mótar',
  'new.cat.profesionales': 'Cuideachtaí agus gairmithe',
  'new.cat.salud': 'Sláinte',
  'new.cat.deporte': 'Spórt',
  'new.cat.educacion': 'Oideachas agus oiliúint',
  'new.cat.creativos': 'Cruthaitheacht agus imeachtaí',
  'new.cat.turismo': 'Turasóireacht agus lóistín',
  'new.cat.local': 'Táirgí áitiúla',
  'new.cat.comunidad': 'Cumainn agus pobal',
  'new.cat.web': 'Leathanaigh shimplí',
  'new.cat.otras': 'Eile',

  'publish.title': 'Foilsigh an suíomh',
  'publish.action': 'Foilsigh',
  'publish.checklist': 'Liosta seiceála roimh fhoilsiú',
  'publish.preview': 'Réamhamharc',

  'versions.title': 'Leaganacha',
  'versions.create': 'Cruthaigh leagan',
  'versions.restore': 'Athchóirigh an leagan seo',
  'versions.empty': 'Níl aon leagan sábháilte fós.',
  'versions.name': 'Ainm an leagain',
  'versions.rev': 'Athbhreithniú',
  'versions.restored': 'Athchóiríodh an leagan go rathúil',

  'domains.title': 'Fearainn',
  'domains.add': 'Cuir fearann leis',

  'analytics.title': 'Anailísíocht',
  'analytics.visits': 'Cuairteanna',

  'export.title': 'Easpórtáil',
  'export.origins_label': 'Bunfhoinsí ceadaithe (mar a n-óstaítear an cód)',
  'export.origins_help':
    'Cuir isteach URL https:// amháin in aghaidh na líne as a ndéanfar glaonna ar an API poiblí.',
  'export.schemes_label': 'Scéimeanna aipe (d’aipeanna dúchasacha)',
  'export.schemes_help':
    'Scéimeanna URL saincheaptha (ceann amháin in aghaidh na líne), mar shampla: moaip, sytue.',
  'export.save': 'Sábháil socruithe easpórtála',
  'export.saved': 'Sábháladh na socruithe easpórtála',

  'legal.title': 'Sonraí dlíthiúla',
  'legal.holder_name': 'Ainm na cuideachta nó an tsealbhóra',
  'legal.tax_id': 'Uimhir chánach / CBL',
  'legal.country': 'Tír',
  'legal.street': 'Seoladh sráide',
  'legal.postal_code': 'Cód poist',
  'legal.city': 'Cathair',
  'legal.region': 'Réigiún / Contae',
  'legal.email': 'Ríomhphost teagmhála dlíthiúil',
  'legal.phone': 'Guthán',
  'legal.registry': 'Sonraí clárlainne (Leabhar, Imleabhar, Leathanach)',
  'legal.privacy_contact': 'Teagmháil phríobháideachais (COS)',
  'legal.save': 'Sábháil na sonraí dlíthiúla',
  'legal.saved': 'Sábháladh na sonraí dlíthiúla',

  'settings.title': 'Socruithe an tsuímh',
  'settings.name': 'Ainm an tsuímh',
  'settings.slug': 'Aitheantóir (slug)',
  'settings.analytics': 'Cumasaigh anailísíocht chomhtháite',
  'settings.analytics_help':
    'Cláraíonn sé amhairc leathanaigh ar bhealach príobháideach gan fhianáin tríú páirtí.',
  'settings.save': 'Sábháil na socruithe',
  'settings.saved': 'Sábháladh na socruithe',
  'settings.danger_zone': 'Crios contúirte',
  'settings.delete_title': 'Scrios an suíomh seo',
  'settings.delete_desc':
    'Bogfar an suíomh go dtí an bruscar ar feadh 30 lá, agus is féidir é a athchóiriú le linn na tréimhse sin.',
  'settings.delete_confirm_prompt': 'Clóscríobh ainm an tsuímh ({name}) le deimhniú:',
  'settings.delete_button': 'Scrios an suíomh',

  'errors.not_found': 'Níor aimsíodh an suíomh',
  'errors.unauthorized': 'Níl cead agat féachaint ar an suíomh seo',

  // Sonraí an tsuímh (islands/SitioDetalleApp.tsx, formularios/Bandeja.tsx, tienda/Pedidos.tsx)
  'ficha.page_title': 'Sonraí an tsuímh',
  'ficha.back': 'Ar ais chuig mo shuíomhanna',
  'ficha.sections': 'Rannáin an tsuímh',
  'ficha.loading': 'An suíomh á lódáil',
  'ficha.load_error': 'Níorbh fhéidir an suíomh a lódáil',
  'ficha.open_site': 'Oscail an suíomh',
  'ficha.not_found_help': 'B’fhéidir gur scriosadh é, nó nach bhfuil rochtain agat air.',

  'publish.tab': 'Foilsigh',
  'publish.intro':
    'Déan athbhreithniú ar an liosta seiceála agus foilsigh nuair a bheidh gach rud réidh. Ní athraíonn réamhamharc an méid a fheiceann an pobal.',
  'publish.status': 'Stádas reatha',
  'publish.check_ok': 'Réidh',
  'publish.check_missing': 'In easnamh',
  'publish.check_legal': 'Sonraí dlíthiúla iomlána',
  'publish.check_legal_missing': 'In easnamh: {campos}',
  'publish.check_legal_fix': 'Comhlánaigh',
  'publish.check_demo': 'Gan téacs samplach ón teimpléad',
  'publish.check_ir': 'Tá na leathanaigh dea-chumtha',
  'publish.check_server':
    'Nuair a fhoilsítear, seiceáiltear freisin nach bhfuil aon téacs samplach ón teimpléad fágtha agus go bhfuil na leathanaigh dea-chumtha.',
  'publish.done_preview': 'Cruthaíodh an réamhamharc.',
  'publish.done_live': 'Foilsíodh an suíomh.',
  'publish.open_preview': 'Oscail an réamhamharc',
  'publish.err_legal':
    'Tá sonraí dlíthiúla in easnamh. Comhlánaigh iad sa chluaisín “Sonraí dlíthiúla”.',
  'publish.err_demo':
    'Tá téacs samplach ón teimpléad ar an suíomh fós. Cuir do théacs féin ina áit san eagarthóir agus foilsigh arís.',
  'publish.err_ir':
    'Níl leathanach amháin nó níos mó dea-chumtha. Oscail san eagarthóir é le feiceáil cad is gá a cheartú.',
  'publish.err_previews':
    'Tá 5 réamhamharc ghníomhacha ann cheana féin. Fan go rachaidh ceann amháin in éag sula gcruthóidh tú ceann eile.',
  'publish.err_too_large':
    'Tá an suíomh níos mó ná an t-uasmhéid is féidir a fhoilsiú (64 MiB nó 5,000 comhad). Laghdaigh na híomhánna nó na comhaid.',
  'publish.err_forbidden': 'Ní féidir ach le riarachán na heagraíochta an suíomh a fhoilsiú.',
  'publish.err_conflict':
    'Níorbh fhéidir an suíomh a fhoilsiú toisc gur athraigh sé nó go bhfuil sé ar fionraí. Athlódáil an leathanach agus bain triail eile as.',
  'publish.err_invalid': 'Níl an foilsiú bailí. Seiceáil ábhar an tsuímh san eagarthóir.',
  'publish.err_generic': 'Níorbh fhéidir an suíomh a fhoilsiú. Bain triail eile as.',

  'versions.intro':
    'Is cóip den dréacht é leagan ar féidir leat filleadh air. Cruthaigh ceann roimh athrú mór.',
  'versions.name_placeholder': 'Roimh an leathanach baile a athrú',
  'versions.manual_name': 'Leagan láimhe',
  'versions.unnamed': 'Gan ainm',
  'versions.loading': 'Leaganacha á lódáil',
  'versions.load_error': 'Níorbh fhéidir na leaganacha a lódáil',
  'versions.empty_help':
    'Nuair a chruthóidh tú leagan nó a fhoilseoidh tú an suíomh, taispeánfar anseo é.',
  'versions.created': 'Cruthaíodh an leagan',
  'versions.create_error': 'Níorbh fhéidir an leagan a chruthú. Bain triail eile as.',
  'versions.restore_error': 'Níorbh fhéidir an leagan a athchóiriú. Bain triail eile as.',
  'versions.restore_confirm':
    'Fillfidh an dréacht ar staid “{nombre}”. Sábhálfar an staid reatha ar dtús mar “Roimh athchóiriú”, ionas gur féidir leat filleadh uirthi.',
  'versions.kind.manual': 'Láimhe',
  'versions.kind.publish': 'Foilsiú',
  'versions.kind.preview': 'Réamhamharc',
  'versions.kind.export': 'Easpórtáil',
  'versions.kind.before_restore': 'Roimh athchóiriú',

  'domains.intro':
    'Ceangail d’fhearann féin. Tar éis é a chur leis, cruthaigh na taifid a liostaítear ag do sholáthraí DNS agus seiceáil an socrú.',
  'domains.host_label': 'Fearann',
  'domains.host_placeholder': 'www.sampla.ie',
  'domains.host_help': 'Ainm an fhearainn amháin, gan https:// ná slaiseanna.',
  'domains.empty': 'Níl aon fhearann saincheaptha cumraithe fós.',
  'domains.loading': 'Fearainn á lódáil',
  'domains.load_error': 'Níorbh fhéidir na fearainn a lódáil',
  'domains.primary': 'Príomhfhearann',
  'domains.status.pending': 'Ag fanacht le fíorú',
  'domains.status.verified': 'Fíoraithe',
  'domains.status.active': 'Gníomhach',
  'domains.status.failed': 'Teipthe',
  'domains.dns_title': 'Taifid DNS',
  'domains.dns_txt': 'TXT (fíorú)',
  'domains.dns_cname': 'CNAME (fofhearainn ar nós www)',
  'domains.dns_a': 'A (fréamhfhearann)',
  'domains.check': 'Seiceáil DNS agus HTTPS',
  'domains.make_primary': 'Déan ina phríomhfhearann',
  'domains.remove': 'Bain',
  'domains.remove_title': 'Bain an fearann',
  'domains.remove_confirm':
    'Stopfaidh “{host}” de bheith ag díriú ar an suíomh seo. Beidh ort é a chur leis arís chun é a úsáid.',
  'domains.added':
    'Cuireadh an fearann leis. Cruthaigh na taifid a thaispeántar thíos ag do sholáthraí DNS.',
  'domains.updated': 'Nuashonraíodh na fearainn.',
  'domains.action_error': 'Níorbh fhéidir an gníomh a chríochnú. Bain triail eile as.',
  'domains.err_invalid': 'Níl ainm an fhearainn bailí.',
  'domains.err_reserved': 'Tá an fearann sin curtha in áirithe agus ní féidir é a úsáid.',
  'domains.err_taken': 'Tá an fearann sin in úsáid ar shuíomh eile cheana féin.',
  'domains.err_limit': 'Tá líon uasta na bhfearann don suíomh seo bainte amach agat.',
  'domains.err_suspended': 'Tá an suíomh ar fionraí agus ní ghlacann sé le hathruithe fearainn.',
  'domains.err_forbidden': 'Ní féidir ach le riarachán na heagraíochta fearainn a bhainistiú.',
  'domains.err_generic': 'Níorbh fhéidir an oibríocht fearainn a chríochnú. Bain triail eile as.',
  'domains.start_title': 'Ceangail d’fhearann, céim ar chéim',
  'domains.start_help':
    'Cuir isteach an fearann ba mhaith leat a úsáid. Teastóidh rochtain uait ar shocruithe DNS an tsoláthraí ar cheannaigh tú uaidh é.',
  'domains.progress': 'Dul chun cinn cheangal an fhearainn',
  'domains.step.registered': 'Curtha le Sytue Web',
  'domains.step.dns': 'DNS fíoraithe',
  'domains.step.https': 'HTTPS réidh',
  'domains.step.primary': 'Príomhsheoladh',
  'domains.step.done': 'Críochnaithe',
  'domains.step.pending': 'Ar feitheamh',
  'domains.status.review': 'Teastaíonn aird',
  'domains.record_name': 'Ainm an taifid',
  'domains.record_value': 'Luach an taifid',
  'domains.routing_title': 'Seoladh an tsuímh',
  'domains.dns_help':
    'Oscail socruithe DNS do sholáthraí agus cuir an dá thaifead seo leo. Cóipeáil gach ainm agus luach go díreach mar a thaispeántar.',
  'domains.dns_names_help':
    'Úsáideann roinnt soláthraithe @ don fhréamhfhearann nó www amháin don fhofhearann sin. Taispeántar ainm iomlán an taifid thuas.',
  'domains.routing_legacy_help':
    'Úsáid A don fhréamhfhearann nó CNAME d’fhofhearann ar nós www, leis an gceann scríbe comhfhreagrach a sholáthraíonn an tseirbhís.',
  'domains.dns_propagation':
    'Tar éis na taifid a shábháil, roghnaigh “Seiceáil DNS agus HTTPS”. Mura bhfeictear fós iad, fan go scaipfidh do sholáthraí an t-athrú agus seiceáil arís. Coinnigh do thaifid ríomhphoist agus seirbhísí eile.',
  'domains.records_unavailable':
    'Níor sholáthair an tseirbhís na sonraí go léir don taifead seo fós. Fan leo sula n-athróidh tú do DNS.',
  'domains.copy': 'Cóipeáil',
  'domains.copied': 'Cóipeáilte',
  'domains.copy_field': 'Cóipeáil {campo}',
  'domains.copy_error':
    'Níorbh fhéidir an luach a chóipeáil chuig an ngearrthaisce. Roghnaigh luach an réimse agus cóipeáil de láimh é.',
  'domains.last_check': 'An tseiceáil is déanaí: {fecha}',
  'domains.publish_help':
    'Nuair a bheidh do phríomhsheoladh réidh, foilsigh an suíomh arís ionas go n-úsáidfidh naisc agus atreoruithe d’fhearann.',
  'domains.publish_action': 'Téigh go foilsiú',
  'domains.detail.pending':
    'Tá d’fhearann curtha leis. Socraigh a thaifid DNS agus seiceáil an ceangal le leanúint ar aghaidh.',
  'domains.detail.txt_missing':
    'Níor aimsíodh an taifead TXT fós. Cuir leis ag do sholáthraí DNS é agus seiceáil arís tar éis don athrú scaipeadh.',
  'domains.detail.txt_mismatch':
    'Ní mheaitseálann luach an TXT. Cuir an luach cruinn a thaispeántar ar an scáileán seo ina áit.',
  'domains.detail.dns_unreachable':
    'Níorbh fhéidir DNS an fhearainn a cheistiú. Déan athbhreithniú ar an bhfearann agus ar a fhreastalaithe ainmneacha, agus seiceáil arís.',
  'domains.detail.routing':
    'Ní dhíríonn an fearann ar an suíomh fós. Déan athbhreithniú ar an taifead A nó CNAME a liostaítear agus seiceáil arís.',
  'domains.detail.certificate':
    'Tá do thaifid DNS fíoraithe. Níl an teastas HTTPS réidh fós; seiceáil arís i gceann cúpla nóiméad.',
  'domains.detail.active': 'Tá DNS agus an ceangal HTTPS fíoraithe. Tá d’fhearann réidh.',
  'domains.detail.unknown':
    'Teastaíonn aird ón tseiceáil is déanaí. Déan athbhreithniú ar na taifid agus fíoraigh an ceangal arís.',

  'analytics.intro':
    'Is meastachán laethúil iad na cuairteoirí le linn na tréimhse píolótaí; ní hionann iad agus úsáideoirí uathúla na tréimhse.',
  'analytics.visitors': 'Cuairteoirí measta',
  'analytics.loading': 'Anailísíocht á lódáil',
  'analytics.load_error': 'Níorbh fhéidir an anailísíocht a lódáil',
  'analytics.unavailable':
    'Níl an tseirbhís anailísíochta ar fáil faoi láthair. Bain triail eile as i gceann cúpla nóiméad.',
  'analytics.disabled': 'Tá an anailísíocht casta as',
  'analytics.disabled_help':
    'Cas air i socruithe an tsuímh í chun tosú ag comhaireamh cuairteanna.',
  'analytics.go_settings': 'Téigh go socruithe',
  'analytics.refresh': 'Athnuaigh',

  'export.intro':
    'Má óstálann tú cód an tsuímh lasmuigh de Sytue, tabhair le fios cá as a nglaofar ar an API poiblí.',
  'export.origins_placeholder': 'https://moshiopa.sampla.ie',
  'export.schemes_placeholder': 'moaip',
  'export.origins_invalid':
    'Caithfidh gach líne a bheith ina seoladh bailí https:// (níl líne {n} bailí).',
  'export.schemes_invalid':
    'Caithfidh gach líne a bheith ina scéim bhailí: tosaíonn sí le litir agus ní bhíonn inti ach litreacha, digití, “+”, “-” nó “.” (níl líne {n} bailí).',
  'export.err_generic': 'Níorbh fhéidir na socruithe a shábháil. Bain triail eile as.',

  'legal.intro': 'Tá siad riachtanach chun an suíomh a fhoilsiú.',
  'legal.required_note': 'Tá na réimsí a bhfuil * orthu riachtanach.',
  'legal.saved_incomplete': 'Sonraí sábháilte. Fós in easnamh le foilsiú: {campos}.',
  'legal.country_help': 'Cód dhá litir, mar shampla IE.',
  'legal.registry_help':
    'Riachtanach do chuideachtaí sa Spáinn (uimhir chánach a thosaíonn le A, B, C, D nó F).',
  'legal.ph_holder': 'Comhlacht Teo.',
  'legal.ph_street': '12 An Phríomhshráid, 1ú urlár',
  'legal.ph_city': 'Baile Átha Cliath',
  'legal.ph_email': 'dli@sampla.ie',
  'legal.ph_privacy': 'dpo@sampla.ie',
  'legal.ph_registry': 'Clárlann Tráchtála, Imleabhar 1234, Fóilió 56, Bileog M-789',
  'legal.err_generic': 'Níorbh fhéidir na sonraí dlíthiúla a shábháil. Bain triail eile as.',

  'settings.tab': 'Socruithe',
  'settings.intro': 'Ainm, seoladh agus roghanna ginearálta an tsuímh.',
  'settings.slug_help':
    'Litreacha beaga, digití agus fleiscíní amháin. Má athraíonn tú é, athraíonn seoladh an tsuímh.',
  'settings.admin_only': 'Ní féidir ach le riarachán na heagraíochta é seo a athrú.',
  'settings.name_required': 'Ní féidir le hainm an tsuímh a bheith folamh.',
  'settings.err_slug_taken': 'Tá an t-aitheantóir sin in úsáid ag suíomh eile cheana féin.',
  'settings.err_invalid':
    'Seiceáil an t-ainm, an t-aitheantóir agus an seoladh: níl ceann acu bailí.',
  'settings.err_generic': 'Níorbh fhéidir na socruithe a shábháil. Bain triail eile as.',
  'settings.delete_error': 'Níorbh fhéidir an suíomh a scriosadh. Bain triail eile as.',

  'forms.empty': 'Níl aon fhreagra ann fós',
  'forms.empty_help': 'Nuair a sheolfaidh duine foirm ar do shuíomh gréasáin, feicfidh tú anseo í.',
};
