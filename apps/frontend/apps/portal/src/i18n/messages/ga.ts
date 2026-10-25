/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const ga: AppCatalog<typeof en> = {
  'page.home': 'Sytue One — Baile',
  'page.search': 'Cuardaigh in Sytue One',
  'page.searchQuery': '“{query}” — Cuardaigh in Sytue One',
  'page.devices': 'Ceangail do ghléasanna — Sytue One',
  'page.data': 'Do shonraí — Sytue One',
  'page.downloads': 'Íoslódálacha — Sytue One',
  'downloads.title': 'Íoslódáil na haipeanna',
  'downloads.lead':
    'Feidhmchláir dhúchasacha do do ríomhaire. Suiteáiltear iad, fanann siad sa Dock nó sa roghchlár Start agus cuireann siad fógraí córais chugat.',
  'downloads.version': 'Leagan {version}',
  'downloads.get': 'Íoslódáil',
  'downloads.forYou': 'Le haghaidh {platform}',
  'downloads.others': 'Ardáin eile',
  'downloads.all': 'Gach ardán',
  'downloads.empty': 'Níl aon suiteálaí foilsithe fós',
  'downloads.emptyNoRelease':
    'Níl a suiteálaithe foilsithe ag aon leagan fós. Beidh siad le feiceáil anseo a luaithe a fhoilseofar iad; níl aon rud le déanamh agat féin.',
  'downloads.emptyUnconfigured':
    'Níl a fhios ag an imscaradh seo cá háit le cuardach a dhéanamh ar na suiteálaithe. Is oibreoir a chumraíonn é.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Níorbh fhéidir na híoslódálacha a lódáil',
  'downloads.failedBody':
    'Níl an Hub ag freagairt faoi láthair. Bain triail eile as i gceann cúpla soicind.',
  'downloads.savedTitle': 'Íoslódáil críochnaithe',
  'downloads.saveFailedTitle': 'Níorbh fhéidir an suiteálaí a íoslódáil',
  'page.activity': 'Gníomhaíocht le déanaí — Sytue One',
  'page.welcome': 'Fáilte go Sytue',
  'activity.overline': 'Gníomhaíocht',
  'activity.heading': 'Gníomhaíocht le déanaí',
  'activity.intro':
    "Cad a tharla le déanaí i do ríomhphost, i do chomhaid, i do cháipéisí, i d'imeachtaí agus i do sheomraí.",
  'activity.today': 'Inniu',
  'activity.yesterday': 'Inné',
  'activity.empty':
    'Níl aon rud le déanaí fós. De réir mar a úsáideann tú Sytue, beidh an méid a bhíonn idir lámha agat le feiceáil anseo.',
  'activity.failedTitle': 'Níorbh fhéidir an ghníomhaíocht a lódáil',
  'activity.failedBody':
    'Níl an Hub ag freagairt faoi láthair. Bain triail eile as i gceann cúpla soicind.',
  'activity.retry': 'Bain triail eile as',
  'activity.all': 'Gach rud',
  'activity.filterBy': 'Scag de réir táirge',
  'activity.count': {
    one: '{count} mhír',
    two: '{count} mhír',
    few: '{count} mhír',
    many: '{count} mír',
    other: '{count} mír',
  },
  'page.inbox': 'Fógraí — Sytue One',
  'inbox.overline': 'Fógraí',
  'inbox.heading': 'Fógraí',
  'inbox.intro': 'Gach a bhfuil le rá ag an tsraith leat, in aon áit amháin.',
  'inbox.all': 'Gach ceann',
  'inbox.unreadFilter': {
    one: '{count} gan léamh',
    two: '{count} gan léamh',
    few: '{count} gan léamh',
    many: '{count} gan léamh',
    other: '{count} gan léamh',
  },
  'inbox.filterBy': 'Scag fógraí',
  'inbox.markAll': 'Marcáil gach ceann léite',
  'inbox.loading': 'Fógraí á lódáil…',
  'inbox.unreadDot': 'Gan léamh',
  'inbox.errorTitle': 'Níorbh fhéidir na fógraí a lódáil',
  'inbox.errorBody':
    'Níl an Hub ag freagairt faoi láthair. Bain triail eile as i gceann cúpla soicind.',
  'inbox.retry': 'Bain triail eile as',
  'inbox.emptyTitle': 'Níl aon rud le léamh',
  'inbox.emptyBody': "Nuair a bheidh d'aird de dhíth ar rud éigin, beidh sé le feiceáil anseo.",
  'inbox.emptyUnreadTitle': 'Tá gach rud léite',
  'inbox.emptyUnreadBody':
    'Tá gach rud léite agat. Athraigh go “Gach ceann” chun na cinn roimhe sin a fheiceáil.',
  'inbox.prefsTitle': 'Cad ba mhaith leat a fháil',
  'inbox.prefsBody':
    'Má mhúchann tú táirge, stopfaidh sé ag cur fógraí chugat anseo agus trí ríomhphost. Is féidir leat é a chasadh air arís am ar bith.',
  'launcher.heading': 'Do chuid aipeanna',
  'launcher.greeting.morning': 'Maidin mhaith',
  'launcher.greeting.afternoon': 'Tráthnóna maith',
  'launcher.greeting.evening': 'Oíche mhaith',
  'launcher.more': 'Tuilleadh',
  'launcher.apps': 'Aipeanna',
  'launcher.connected': 'Ceangailte',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Díchumasaithe',
  'product.mail': 'Ríomhphost',
  'product.drive': 'Comhaid agus comhoibriú',
  'product.calendar': 'Féilire',
  'product.contacts': 'Teagmhálaithe',
  'product.docs': 'Cáipéisí, scarbhileoga agus cur i láthair',
  'product.rooms': 'Cainéil, teachtaireachtaí agus glaonna',
  'product.cloud': 'Ardán feidhmchlár (PaaS)',
  'product.console': 'Riarachán agus eagraíochtaí',
  'product.id': 'Aitheantas, próifíl agus slándáil',
  'quick.heading': 'Gníomhartha tapa',
  'quick.search': 'Cuardaigh sa tsraith ar fad',
  'quick.compose': 'Scríobh ríomhphost',
  'quick.newDoc': 'Cáipéis nua',
  'quick.upload': 'Uaslódáil chuig Drive',
  'quick.newEvent': 'Cruthaigh imeacht',
  'quick.download': 'Íoslódáil an aip',
  'quick.devices': 'Ceangail do ghléasanna',
  'quick.data': 'Do shonraí',

  'storage.connectTitle': 'Ceangail stóras seachtrach',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV agus eile.',
  'recent.heading': 'Le déanaí ar fud na sraithe',
  'recent.viewAll': 'Féach ar gach rud',
  'recent.loading': 'Gníomhaíocht le déanaí á lódáil…',
  'recent.errorTitle': 'Níorbh fhéidir linn do ghníomhaíocht a lódáil',
  'recent.errorBody':
    'Níl an Hub ag freagairt faoi láthair. Tá do chuid oibre slán; bain triail eile as.',
  'recent.retry': 'Bain triail eile as',
  'recent.emptyTitle': 'Gan ghníomhaíocht le déanaí',
  'recent.emptyBody':
    'Nuair a oibreoidh tú in Mail, Drive, Calendar nó Rooms, feicfidh tú anseo é.',
  'recent.emptyCta': 'Tosaigh in Drive',
  'search.overline': 'Cuardach',
  'search.heading': 'Cuardaigh ar fud na sraithe ar fad',
  'search.placeholder': 'Cuardaigh in Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Cuardaigh in Sytue One',
  'search.submit': 'Cuardaigh',
  'search.hint':
    "Clóscríobh thuas chun cuardach a dhéanamh i do ríomhphost, i do chomhaid, i do cháipéisí, i d'imeachtaí, i do theagmhálaithe agus i do sheomraí in éineacht. Osclaítear na torthaí go díreach ina dtáirge féin.",
  'search.failedTitle': 'Níorbh fhéidir an cuardach a chríochnú',
  'search.failedBody':
    'Níl an Hub ag freagairt faoi láthair. Bain triail eile as i gceann cúpla soicind.',
  'search.retry': 'Bain triail eile as',
  'search.empty': 'Gan torthaí ar “{query}”. Bain triail as focail eile.',
  'search.count': {
    one: '{count} toradh',
    two: '{count} thoradh',
    few: '{count} thoradh',
    many: '{count} dtoradh',
    other: '{count} toradh',
  },
  'welcome.heading': 'Fáilte go Sytue',
  'welcome.lead':
    'Cúpla nóiméad agus beidh gach rud ag obair. Téimid céim ar chéim; má stopann tú leath bealaigh, leanfaimid ar aghaidh ón áit ar fhág tú é nuair a thiocfaidh tú ar ais.',
  'data.title': 'Do shonraí',
  'data.lead':
    'Íoslódáil cóip de gach a bhfuil agat in Sytue One, aon uair is mian leat agus gan cead a iarraidh ar aon duine.',
  'data.export.section': 'Íoslódáil cóip',
  'data.export.body':
    'Comhad ZIP amháin le do ríomhphost, comhaid, féilirí, teagmhálaithe, nótaí, doiciméid, grianghraif agus teachtaireachtaí. An méid a bhfuil formáid chaighdeánach aige cheana, téann sé mar atá: ríomhphoist mar .eml, féilirí mar .ics, teagmhálaithe mar .vcf agus comhaid díreach mar a d’uaslódáil tú iad.',
  'data.export.formatLegend': 'Formáid na liostaí',
  'data.export.formatHelp':
    'Déanann na liostaí cur síos ar a bhfuil sa chóip: do theachtaireachtaí, imeachtaí, teagmhálaithe nó comhaid, ceann in aghaidh an ró.',
  'data.export.orgHelp':
    'Baineann gach cóip le heagraíocht amháin. Má roghnaíonn tú ceann eile, rachaidh tú isteach ann agus tosóidh an íoslódáil léi féin.',
  'data.export.json': 'Le haghaidh aipeanna agus ríomhchláir eile.',
  'data.export.csv': 'Le hoscailt in Excel, Numbers nó Google Sheets.',
  'data.export.xml': 'Le haghaidh córas a iarrann XML.',
  'data.export.action': 'Íoslódáil an chóip',
  'data.export.startedTitle': 'Tá an íoslódáil tosaithe',
  'data.export.startedBody':
    'Leanann do bhrabhsálaí á sábháil fiú má dhúnann tú an leathanach seo. Má tá a lán comhad agat, tógfaidh sé tamall.',
  'data.export.scope':
    'Tá sonraí na heagraíochta lena bhfuil tú logáilte isteach sa chóip. Taispeánann an comhad achoimre atá istigh cad a cuireadh isteach ó gach aip.',
  'data.move.section': 'Bog chuig seirbhís eile é',
  'data.move.body':
    'Chun gach rud a sheoladh go díreach chuig Google Workspace nó Microsoft 365 gan aon rud a íoslódáil: más cuntas pearsanta atá agat nó má riarann tú d’eagraíocht, tosaigh é tú féin ó Sytue Admin, faoi Imirce; mura ndéanann, iarr ar an duine a riarann í.',
  'data.move.action': 'Téigh go hImirce',

  'devices.native.body':
    'Úsáid bainisteoir comhad do ghléis nó ceangail cuntas le toiliú a úinéara.',
  'devices.native.driveBody':
    'Cuirtear Sytue Drive leis go huathoibríoch tar éis síniú isteach. Ceadaigh a shuíomh má iarrann an córas ort é.',
  'devices.native.openDrive': 'Oscail sa bhainisteoir comhad',
  'devices.native.failed': 'Níorbh fhéidir comhtháthú an ghléis a nuashonrú.',
  'devices.title': 'Ceangail do ghléasanna',
  'devices.lead':
    'Tabhair ríomhphost, féilire, teagmhálaithe agus comhaid Sytue leat ar do ghuthán nó ar do ríomhaire. Treoróidh muid thú céim ar chéim.',
  'devices.leadShort':
    'Tabhair ríomhphost, féilire agus teagmhálaithe Sytue leat ar do ghuthán nó ar do ríomhaire.',
  'devices.unavailable.section': 'Ní féidir linn na sonraí a thaispeáint duit faoi láthair',
  'devices.unavailable.title': 'Níorbh fhéidir linn an chumraíocht a fháil',
  'devices.unavailable.body':
    "Bain triail eile as i gceann tamaillín. Mura n-athraíonn sé, scríobh chuig an duine a riarann Sytue i d'eagraíocht.",
  'devices.retry': 'Bain triail eile as',
  'devices.copy': 'Cóipeáil',
  'devices.copyFailTitle': 'Níorbh fhéidir é a chóipeáil',
  'devices.copyFailBody': 'Roghnaigh é agus cóipeáil de láimh é.',
  'devices.copied.address': 'Cóipeáladh do sheoladh',
  'devices.copied.imap': 'Cóipeáladh an freastalaí isteach',
  'devices.copied.port': 'Cóipeáladh an port',
  'devices.copied.smtp': 'Cóipeáladh an freastalaí amach',
  'devices.copied.url': 'Cóipeáladh an seoladh',
  'devices.copied.drive': 'Cóipeáladh seoladh Drive',
  'devices.oneClick.thunderbird': 'Socraigh Thunderbird',
  'devices.oneClick.outlook': 'Socraigh Outlook',
  'devices.oneClick.android': 'Socraigh ar Android',
  'devices.oneClick.body':
    'Cruthaíonn sé pasfhocal aipe don ríomhchlár seo amháin, cóipeálann sé é agus taispeánann sé duit é uair amháin.',
  'devices.oneClick.failTitle': 'Níorbh fhéidir an pasfhocal aipe a chruthú',
  'devices.oneClick.failBody':
    'Bain triail eile as i gceann nóiméid. Is féidir leat ceann a chruthú i Mo chuntas freisin.',
  'devices.oneClick.onceTitle': 'Cóipeáladh é. Ní thaispeántar é ach an uair seo',
  'devices.oneClick.onceBody':
    'Greamaigh é áit a n-iarrann an ríomhchlár an pasfhocal. Má chailleann tú é, cúlghair é i Mo chuntas agus cruthaigh ceann eile.',
  'devices.oneClick.passwordLabel': 'Pasfhocal aipe',
  'devices.copied.password': 'Cóipeáladh an pasfhocal aipe',
  'devices.oneClick.notOpened':
    'Níorbh fhéidir {program} a oscailt ar an ríomhaire seo: oscail féin é agus lean na céimeanna seo.',
  'devices.oneClick.next.thunderbird':
    'I gcúntóir cuntais nua Thunderbird, clóscríobh do sheoladh agus greamaigh an pasfhocal: aimsíonn sé na freastalaithe leis féin. Féilirí agus teagmhálaithe: cuir féilire líonra agus leabhar seoltaí CardDAV leis leis an seoladh agus an pasfhocal céanna (má iarrann sé an suíomh, cóipeáil thíos é).',
  'devices.oneClick.next.outlook':
    'In «Cuir cuntas leis» in Outlook, clóscríobh do sheoladh agus greamaigh an pasfhocal: aimsíonn Outlook na freastalaithe leis féin.',
  'devices.oneClick.next.android':
    'I d’aip ríomhphoist, cuir cuntas «Eile (IMAP)» leis le do sheoladh agus greamaigh an pasfhocal. Féilirí agus teagmhálaithe: DAVx5 leis na seoltaí thíos agus an pasfhocal céanna.',
  'devices.android.davx5Calendars': 'Féilirí in DAVx5',
  'devices.android.davx5Contacts': 'Teagmhálaithe in DAVx5',
  'devices.android.davx5Install': 'Faigh DAVx5 ó F-Droid',
  'devices.platform.iphone.title': 'iPhone nó iPad',
  'devices.platform.iphone.body':
    'Suiteálann tú próifíl agus bíonn ríomhphost, féilire agus teagmhálaithe réidh.',
  'devices.platform.iphone.note': 'An bealach is tapúla: comhad amháin agus do phasfhocal.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'An phróifíl chéanna agus atá ar an iPhone, suiteáilte ó System Settings.',
  'devices.platform.mac.note': 'Apple Mail, Calendar agus Contacts.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Clóscríobhann tú do sheoladh agus do phasfhocal; aimsíonn Outlook an chuid eile leis féin.',
  'devices.platform.outlook.note':
    'Ríomhphost amháin. Socraítear an féilire agus na teagmhálaithe ar leith.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Cumraíonn sé é féin ó do sheoladh amháin; cuirtear an féilire agus na teagmhálaithe leis ina dhiaidh sin.',
  'devices.platform.thunderbird.note': 'Windows, Mac nó Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    "D'aip ríomhphoist don bhosca poist agus DAVx5 don fhéilire agus do na teagmhálaithe.",
  'devices.platform.android.note': 'Níl aip Sytue ann: úsáidtear aipeanna an chórais.',
  'devices.platform.apis.title': 'Uirlis a labhraíonn Google nó Microsoft',
  'devices.platform.apis.body':
    'Dírigh comhtháthú a scríobhadh do Google Drive, Google Calendar, Google Contacts nó Microsoft Graph ar Sytue, gan a chód a athrú.',
  'devices.platform.apis.note': 'Ach amháin nuair is féidir an bun-URL a shocrú.',
  'devices.apis.step1.title': 'Athraigh an bunseoladh',
  'devices.apis.step1.body':
    'San uirlis, cuir seoladh an táirge agus na canúna atá uait in áit an tseolta a nglaonn sí air. Fanann an chuid eile dá cumraíocht mar atá.',
  'devices.apis.step2.title': 'Úsáid do dhintiúir Sytue',
  'devices.apis.step2.body':
    'Is é do chomhartha Sytue ID a úsáidtear le fíordheimhniú, ní cuntas Google ná Microsoft: is iad na ceadanna céanna atá agat anseo cheana.',
  'devices.apis.identityGoogle': 'Céannacht · Google OAuth agus eolaire',
  'devices.apis.step3.title': 'Aip tríú páirtí, amhail is gurbh é Google í',
  'devices.apis.step3.body':
    'Má shíníonn an uirlis isteach le Google OAuth, cláraítear í in Sytue Admin leis na ceadanna Google a theastaíonn uaithi. Iarrann an aip na ceadanna sin agus faigheann sí sonraí Sytue; níl baint ar bith ag Google leis agus ní fheiceann sé faic.',
  'devices.apis.driveGoogle': 'Drive · Google Drive API',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · Google Calendar API',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · Google People API',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Cad atá clúdaithe',
  'devices.apis.expectBody':
    'Liostú, léamh, cruthú, eagarthóireacht agus scriosadh. Freagraíonn aon rud nach bhfuil clúdaithe le hearráid shoiléir, riamh le rath folamh.',
  'devices.copied.apiBase': 'Cóipeáladh an bunseoladh',
  'devices.check.apis.label': 'An uirlis',
  'devices.check.apis.value':
    "Liostaíonn sí d'ábhar Sytue amhail is gurbh í an tseirbhís bhunaidh í.",
  'devices.platform.drive.title': 'Taiscéalaí comhad (Drive)',
  'devices.platform.drive.body':
    'Feiceann tú do chomhaid Sytue Drive mar thiomántán eile ar an ríomhaire.',
  'devices.platform.drive.note': 'Finder ar Mac, File Explorer ar Windows.',
  'devices.mail.address': 'Do sheoladh (ainm úsáideora)',
  'devices.mail.imapHost': 'Freastalaí isteach (IMAP)',
  'devices.mail.imapPort': 'Port isteach',
  'devices.mail.smtpHost': 'Freastalaí amach (SMTP)',
  'devices.mail.smtpPort': 'Port amach',
  'devices.mail.hint':
    'Criptiúchán SSL/TLS ar an dá cheann, agus an pasfhocal Sytue céanna a úsáideann tú chun logáil isteach ar an ngréasán.',
  'devices.dav.caldav': 'Féilire (CalDAV)',
  'devices.dav.carddav': 'Teagmhálaithe (CardDAV)',
  'devices.dav.user': 'Ainm úsáideora',
  'devices.apple.download': 'Íoslódáil próifíl Sytue',
  'devices.apple.step1.title': 'Íoslódáil an phróifíl',
  'devices.apple.step1.body':
    'Brúigh an cnaipe thuas. Íoslódáiltear comhad beag le socruithe do chuntais.',
  'devices.apple.step2.iphone.title': 'Suiteáil ó Settings í',
  'devices.apple.step2.iphone.body':
    'Oscail Settings. Taispeántar “Profile Downloaded” ag an mbarr ar fad: tapáil air agus brúigh Install.',
  'devices.apple.step2.mac.title': 'Suiteáil ó System Settings í',
  'devices.apple.step2.mac.body':
    'Oscail System Settings › Privacy & Security › Profiles, cliceáil faoi dhó ar an bpróifíl a íoslódáladh agus brúigh Install.',
  'devices.apple.step3.title': 'Ní gá aon phasfhocal a chlóscríobh',
  'devices.apple.step3.body':
    'Tá a phasfhocal aipe féin sa phróifíl, cruthaithe don ghléas seo amháin. Is féidir leat é a chúlghairm am ar bith i Mo chuntas.',
  'devices.apple.step4.title': 'Déanta',
  'devices.apple.step4.body':
    'Cuirtear ríomhphost, féilire agus teagmhálaithe leis in aon iarracht amháin. Níl aon fhreastalaí le clóscríobh.',
  'devices.apple.manualTitle': 'Más fearr leat é a dhéanamh de láimh',
  'devices.apple.manualBody':
    'Is féidir leat an cuntas a chruthú de láimh freisin leis na sonraí seo.',
  'devices.manualValues': 'Féach ar na luachanna láimhe',
  'devices.desktop.step1.title': 'Cuir cuntas ríomhphoist leis',
  'devices.desktop.step1.outlook': 'In Outlook: File › Add account.',
  'devices.desktop.step1.thunderbird': 'In Thunderbird: Accounts › Set up an account › Email.',
  'devices.desktop.step2.title': 'Clóscríobh do sheoladh agus do phasfhocal amháin',
  'devices.desktop.step2.outlook':
    'Sin a bhfuil. Iarrann Outlook na socruithe ar Sytue agus líonann sé isteach duit iad.',
  'devices.desktop.step2.thunderbird':
    'Sin a bhfuil. Iarrann Thunderbird na socruithe ar Sytue agus líonann sé isteach duit iad.',
  'devices.desktop.step3.title': 'Deimhnigh',
  'devices.desktop.step3.body':
    'Nuair a thaispeánfaidh sé na freastalaithe duit, glac leo. Ceanglaíonn sé mar chuntas IMAP.',
  'devices.desktop.emailLabel': 'Do sheoladh ríomhphoist',
  'devices.desktop.notFoundTitle': 'Nár aimsigh sé na socruithe leis féin?',
  'devices.desktop.notFoundBody': 'Roghnaigh cumraíocht láimhe agus úsáid na luachanna seo.',
  'devices.thunderbird.davTitle': 'Féilire agus teagmhálaithe',
  'devices.thunderbird.davBody': 'In Thunderbird cuirtear leis iad ar leith, leis na seoltaí seo.',
  'devices.outlook.davTitle': 'Féilire agus teagmhálaithe in Outlook',
  'devices.outlook.davBody':
    'Ní thacaíonn Outlook do Windows le CalDAV/CardDAV, mar sin ní shioncronaítear féilire ná teagmhálaithe Sytue ansin. Úsáid iad ó Sytue ar an ngréasán, nó ó chliant a thacaíonn le CalDAV/CardDAV (Thunderbird, Apple Calendar).',
  'devices.android.mailTitle': 'Ríomhphost',
  'devices.android.mailBody':
    'Leis an aip ríomhphoist a úsáideann tú cheana (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Cuir cuntas leis',
  'devices.android.mail1.body': 'Roghnaigh “Other” nó “Personal (IMAP)”, ní “Google”.',
  'devices.android.mail2.title': 'Clóscríobh do sheoladh agus do phasfhocal',
  'devices.android.mail2.body': 'Má iarrann sé na freastalaithe, cóipeáil ón liosta thíos iad.',
  'devices.android.davTitle': 'Féilire agus teagmhálaithe',
  'devices.android.davBody':
    'Níl CalDAV/CardDAV ag Android as an mbosca: teastaíonn DAVx5 uait, aip saor in aisce a chuireann iad le féilire agus le leabhar seoltaí an ghutháin féin.',
  'devices.android.dav1.title': 'Suiteáil DAVx5',
  'devices.android.dav1.body':
    'Tá sé ar Google Play agus ar F-Droid. Ní fhoilsíonn ná ní chothaíonn Sytue é.',
  'devices.android.dav2.title': 'Cuir cuntas leis le “Login with URL and user name”',
  'devices.android.dav2.body':
    'Greamaigh an seoladh thíos agus úsáid do ríomhphost agus do phasfhocal Sytue.',
  'devices.android.dav3.title': 'Roghnaigh cad atá le sioncronú',
  'devices.android.dav3.body': "Cuir tic le d'fhéilirí agus le do leabhar seoltaí.",
  'devices.drive.step1.title': 'Oscail an ceangal le freastalaí',
  'devices.drive.step1.body':
    'Ar Mac: Finder › Go › Connect to Server (⌘K). Ar Windows: File Explorer › This PC › Map network drive, agus cuir tic le “Connect using different credentials”.',
  'devices.drive.step2.title': 'Greamaigh an seoladh seo',
  'devices.drive.step2.body': 'Is é an fillteán le do chomhaid Sytue Drive é.',
  'devices.drive.step3.title': 'Logáil isteach le do chuntas Sytue',
  'devices.drive.step3.body':
    'Ainm úsáideora: do sheoladh ríomhphoist. Pasfhocal: do phasfhocal Sytue.',
  'devices.drive.urlLabel': 'Seoladh Drive (WebDAV)',
  'devices.drive.expectTitle': 'Cad is féidir a bheith ag súil leis',
  'devices.drive.windowsExpectBody':
    'Ar Windows, feictear Drive i File Explorer mar an fillteán “Sytue Drive”. Íoslódáiltear na comhaid nuair a osclaíonn tú iad; cliceáil ar dheis › “Always keep on this device” chun iad a bheith agat as líne. Uaslódáiltear chuig Drive a shábhálann nó a chruthaíonn tú ann.',
  'devices.drive.expectBody':
    'Osclaítear comhaid ón bhfreastalaí; ní chóipeáiltear chuig an diosca iad. Chun oibriú as líne, íoslódáil roimh ré an méid a theastaíonn uait.',
  'devices.check.mail.label': 'An ríomhphost',
  'devices.check.mail.value':
    'Tagann teachtaireachtaí nua isteach agus is féidir leat ceann a sheoladh chugat féin.',
  'devices.check.dav.label': 'Féilire agus teagmhálaithe',
  'devices.check.dav.value':
    'Taispeántar imeacht a cruthaíodh ar Sytue ar an ngréasán ar an ngléas laistigh de chúpla nóiméad.',
  'devices.check.profile.label': 'An phróifíl',
  'devices.check.profile.value': 'Tá “Sytue One” le feiceáil i liosta na bpróifílí suiteáilte.',
  'devices.check.drive.label': 'An tiomántán',
  'devices.check.drive.value':
    "Feiceann tú d'fhillteáin Drive agus is féidir leat comhad a oscailt.",
  'devices.check.write.label': 'Scríobh',
  'devices.check.write.value':
    'Cruthaíonn tú fillteán agus bíonn sé le feiceáil in Sytue Drive sa bhrabhsálaí freisin.',
  'devices.step1.title': 'Cad a úsáideann tú',
  'devices.step1.heading': 'Cá háit ar mhaith leat do chuntas Sytue?',
  'devices.step1.description':
    'Roghnaigh ceann amháin. Má tá níos mó ná ceann amháin agat, déan an draoi arís do gach ceann.',
  'devices.step2.title': 'Conas é a dhéanamh',
  'devices.step2.heading': 'Socraigh {platform}',
  'devices.step2.headingFallback': 'Conas é a dhéanamh',
  'devices.step2.description':
    'Lean na céimeanna seo. Cóipeáiltear le cnaipe gach rud a chaithfidh tú a chlóscríobh.',
  'devices.step2.next': 'Tá sé déanta',
  'devices.step3.title': 'Seiceáil',
  'devices.step3.heading': 'Conas a bheith cinnte gur oibrigh sé',
  'devices.step3.description': 'Má tá na rudaí seo fíor, tá sé ceangailte i gceart.',
  'devices.step3.helpTitle': 'An bhfuil rud éigin mícheart?',
  'devices.step3.helpBody':
    "Is minic gurb é an pasfhocal is cúis leis: úsáid an ceann céanna a úsáideann tú chun logáil isteach in Sytue sa bhrabhsálaí. Má tá fíorú dhá chéim agat, d'fhéadfadh go n-éileoidh d'eagraíocht pasfhocal aipe; fiafraigh den duine a riarann Sytue.",
  'devices.step3.again': 'Ceangail gléas eile',
  'ob.error.generic': 'Níorbh fhéidir linn an oibríocht a chríochnú. Bain triail eile as.',
  'ob.failedTitle': 'Níorbh fhéidir linn do chlárú a lódáil',
  'ob.failedBody':
    'Níl an tseirbhís aitheantais ag freagairt faoi láthair. Níor cailleadh aon rud: bain triail eile as i gceann nóiméid.',
  'ob.retry': 'Bain triail eile as',
  'ob.preparing': 'Do chlárú á ullmhú…',
  'ob.lead':
    'Cúpla nóiméad agus beidh tú ag obair. Téimid céim ar chéim; má stopann tú leath bealaigh, leanaimid ar aghaidh ón áit ar fhágamar é nuair a fhilleann tú.',
  'ob.saveFailedTitle': 'Níorbh fhéidir é a shábháil',
  'ob.saveFailedBody': 'Bain triail eile as i gceann tamaillín.',
  'ob.copy': 'Cóipeáil',
  'ob.copyFailTitle': 'Níorbh fhéidir é a chóipeáil',
  'ob.copyFailBody': 'Roghnaigh é leis an luch agus cóipeáil de láimh é.',
  'ob.copied.recordType': 'Cóipeáladh cineál an taifid',
  'ob.copied.recordName': 'Cóipeáladh ainm an taifid',
  'ob.copied.recordValue': 'Cóipeáladh luach an taifid',
  'ob.roleOption.member': 'Ball — úsáid na sraithe, gan aon riarachán',
  'ob.roleOption.admin': 'Riarthóir — bainistíocht na heagraíochta ar fad',
  'ob.roleOption.billing': 'Billeáil — cúram na sonrasc',
  'ob.roleName.owner': 'Úinéir',
  'ob.roleName.admin': 'Riarthóir',
  'ob.roleName.member': 'Ball',
  'ob.roleName.billing': 'Billeáil',
  'ob.mode.title': 'Tosú',
  'ob.mode.heading': 'Cén úsáid a bhainfidh tú as Sytue?',
  'ob.mode.description':
    'Insíonn sé seo dúinn cad ba cheart a fhiafraí díot. Is féidir leat é a athrú níos déanaí.',
  'ob.mode.individual.title': 'Dom féin amháin',
  'ob.mode.individual.body':
    "Do ríomhphost, do chomhaid agus d'fhéilire, le do sheoladh féin. Má bhíonn ort cuireadh a thabhairt do dhuine éigin lá éigin, beidh tú in ann é a dhéanamh.",
  'ob.mode.org.title': "Do m'eagraíocht",
  'ob.mode.org.body':
    "Cláraíonn tú d'fhoireann, roinneann tú cé a fhéadann cad a dhéanamh, agus riarann tú gach rud ó áit amháin.",
  'ob.mode.org.badge': 'Is coitianta',
  'ob.identity.titleOrg': "D'eagraíocht",
  'ob.identity.titleMe': 'Do chuntas',
  'ob.identity.headingOrg': "Cén t-ainm atá ar d'eagraíocht?",
  'ob.identity.headingMe': 'Cén t-ainm ba mhaith leat ar do chuntas?',
  'ob.identity.descriptionCreated':
    'Tá sé cruthaithe cheana. Is féidir leat an t-ainm a athrú; fanfaidh an seoladh mar atá.',
  'ob.identity.description': 'Leis seo cruthaímid do spás agus do sheoladh in Sytue.',
  'ob.identity.nextCreated': 'Lean ar aghaidh',
  'ob.identity.nextOwnDomain': "Cruthaigh agus féach cad atá le cur le m'fhearann",
  'ob.identity.next': 'Cruthaigh agus lean ar aghaidh',
  'ob.identity.nameLabel': 'Ainm',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Aoife Ní Bhriain',
  'ob.identity.nameHintOrg':
    "Is é seo a fheiceann d'fhoireann nuair a logálann siad isteach. Is féidir leat é a athrú am ar bith.",
  'ob.identity.nameHintMe': 'Is tusa amháin a fheiceann é. Is féidir leat é a athrú am ar bith.',
  'ob.identity.slugLabel': 'Do sheoladh in Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Is é {subdomain} do sheoladh. Ní féidir é a athrú: tá do ríomhphost agus do chomhaid ag brath air.',
  'ob.identity.slugEmpty': 'Roghnaigh ainm gearr. Beidh sé mar sheoladh agat: acme.{suite}',
  'ob.identity.slugChecking': 'Á sheiceáil an bhfuil sé saor…',
  'ob.identity.slugFree': 'Saor. Is é {subdomain} a bheidh mar sheoladh agat',
  'ob.identity.slugTaken': 'Ní féidir é a úsáid.',
  'ob.identity.firstTitle': 'Ar dtús cruthaímid do spás',
  'ob.identity.firstBody':
    "Nuair a bhrúnn tú an cnaipe thíos, cruthaímid do sheoladh Sytue agus taispeánaimid duit anseo an taifead cruinn a chaithfidh tú a chur le d'fhearann. Ní gá é a bheith réidh agat anois.",
  'ob.domain.legend': "Agus d'fhearann féin?",
  'ob.domain.keep.title': 'Coinneoidh mé {host}',
  'ob.domain.keep.placeholder': 'm-ainm',
  'ob.domain.keep.body':
    'Níl ort aon rud a dhéanamh: is sinne a fhoilseoidh é. Sin a mholaimid chun tosú.',
  'ob.domain.own.title': 'Tá fearann agam cheana agus ba mhaith liom é a úsáid (acme.com)',
  'ob.domain.own.body':
    'Má tá sé ar Cloudflare, socróimid duit é. Mura bhfuil, inseoimid duit an taifead cruinn atá le cur leis agus seiceálfaimid anseo é.',
  'ob.domain.search.title': 'Níl fearann agam fós',
  'ob.domain.search.body':
    'Seiceálaimid an bhfuil an t-ainm atá uait saor agus cé mhéad a chosnaíonn sé in aghaidh na bliana. Níl ann ach cuardach: ní cheannaítear aon rud anseo.',
  'ob.domain.hostLabel': "D'fhearann",
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Scríobh é díreach mar atá: acme.com, gan “https://” agus gan “www”.',
  'ob.domain.verified': '{host} fíoraithe',
  'ob.dns.legend': 'Cá bhfuil {domain}?',
  'ob.dns.fallback': "d'fhearann",
  'ob.dns.cf.title': 'Ar Cloudflare — socróidh Sytue é',
  'ob.dns.cf.body':
    'Tugann tú cead sealadach dúinn ar do chuntas agus cruthaímid féin na taifid. Sula mbainfimid le haon rud, taispeánfaimid duit cad a athróimid.',
  'ob.dns.cf.badge': 'Gan baint le DNS',
  'ob.dns.manual.title': 'Le soláthraí eile — déanfaidh mé féin é',
  'ob.dns.manual.body':
    'Tugaimid duit an taifead cruinn le cóipeáil isteach i do phainéal DNS agus seiceálaimid anseo é a luaithe a bheidh sé ann.',
  'ob.dns.manualSummary': "B'fhearr liom na taifid a chur leis mé féin",
  'ob.people.title': "D'fhoireann",
  'ob.people.heading': 'Cé na daoine a dtabharfaidh tú cuireadh dóibh ar dtús?',
  'ob.people.description':
    'Seoltar ríomhphost chuig gach seoladh le nasc pearsanta chun pasfhocal a chruthú agus logáil isteach. Is féidir leat an chéim seo a scipeáil agus é a dhéanamh níos déanaí.',
  'ob.people.next': 'Lean ar aghaidh',
  'ob.people.emailsLabel': 'Seoltaí ríomhphoist',
  'ob.people.emailsPlaceholder': 'ana@acme.com, bruno@acme.com',
  'ob.people.emailsHint':
    'Is féidir leat roinnt acu a ghreamú in éineacht, scartha le camóga nó le línte nua.',
  'ob.people.roleLabel': 'An ról a bheidh acu ar theacht isteach',
  'ob.people.roleHint': 'Is féidir é a athrú am ar bith ó Sytue Admin.',
  'ob.people.send': 'Seol cuirí',
  'ob.people.sendNote': 'Ní sheoltar aon rud go dtí go mbrúnn tú anseo.',
  'ob.people.pendingTitle': 'Cuirí ag fanacht le glacadh',
  'ob.people.emptyTitle': "Níor thug tú cuireadh d'aon duine fós",
  'ob.people.emptyBody':
    "Is féidir leat é a dhéanamh anois nó níos déanaí, ó Sytue Admin. Oibríonn d'eagraíocht mar an gcéanna.",
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Cuireadh seolta',
  'ob.invite.already': "Cheana féin i d'eagraíocht",
  'ob.invite.invalid': 'Ní cosúil gur seoladh ríomhphoist é sin',
  'ob.invite.notSent': 'Níorbh fhéidir an ríomhphost a sheoladh',
  'ob.invite.failed': 'Níorbh fhéidir an cuireadh a thabhairt',
  'ob.invite.default':
    'Seolfar ríomhphost le nasc pearsanta chun pasfhocal a chruthú agus logáil isteach.',
  'ob.billing.title': 'Billeáil',
  'ob.billing.heading': 'Cén t-ainm a bheidh ar na sonraisc?',
  'ob.billing.description':
    'Cuir isteach do shonraí sonraisc agus roghnaigh na suíocháin íoctha. Ní thosaíonn an rochtain go dtí go ndeimhníonn Mollie an íocaíocht.',
  'ob.billing.next': 'Lean ar aghaidh',
  'ob.billing.pay': 'Íoc go slán le Mollie',
  'ob.billing.verify': 'Fíoraigh an íocaíocht',
  'ob.billing.legalLabel': 'Ainm dlíthiúil',
  'ob.billing.legalPlaceholder': 'ACME Europe Teo.',
  'ob.billing.legalHint': 'Ainm dlíthiúil na cuideachta, díreach mar atá sé ar do shonraisc.',
  'ob.billing.taxLabel': 'Aitheantas cánach',
  'ob.billing.taxPlaceholder': 'IE1234567T',
  'ob.billing.taxHint': 'Uimhir chánach nó uimhir CBL do thíre.',
  'ob.billing.addressLabel': 'Seoladh',
  'ob.billing.addressPlaceholder': '1 Sráid Uí Chonaill, Baile Átha Cliath 1, Éire',
  'ob.billing.addressHint': 'An seoladh cláraithe a théann ar an sonrasc.',
  'ob.billing.emailLabel': 'Ríomhphost do shonraisc',
  'ob.billing.emailPlaceholder': 'sonraisc@acme.com',
  'ob.billing.emailHint':
    'Is ansin a sheolaimid na sonraisc. De réir réamhshocraithe, ríomhphost do chuntais.',
  'ob.billing.note':
    'Ní iarraimid ach an méid is gá chun sonrasc a eisiúint chugat. Ní roinntear aon chuid de seo le haon duine.',
  'ob.billing.seatsLabel': 'Suíocháin íoctha',
  'ob.billing.seatsHint': '{seats} suíochán × {price}/Úsáideoir/Mí · iomlán {total}/Mí.',
  'ob.billing.active': 'Síntiús gníomhach',
  'ob.billing.activeBody':
    'Dheimhnigh Mollie an íocaíocht. Tá do rochtain ar Sytue One cumasaithe.',
  'ob.billing.pending': 'Táimid ag fíorú na híocaíochta',
  'ob.billing.pendingBody':
    'Níor dheimhnigh Mollie an íocaíocht fós. Fan cúpla soicind agus fíoraigh arís; ní ghearrfar táille ort faoi dhó.',
  'ob.roles.title': 'Ceadanna',
  'ob.roles.heading': 'Ar mhaith leat cúraimí riaracháin a roinnt?',
  'ob.roles.description':
    'Róil réamhdhéanta iad seo: roghnaíonn tú na cinn atá uait agus cruthaímid iad. Ina dhiaidh sin sannann tú iad do cibé duine is mian leat ó Sytue Admin. Is féidir leat an chéim seo a scipeáil.',
  'ob.roles.next': 'Lean ar aghaidh',
  'ob.roles.already': 'Cruthaithe cheana',
  'ob.roles.seePermissions': 'Féach ar na ceadanna cruinne',
  'ob.roles.helpTitle': 'Is agatsa atá an smacht fós',
  'ob.roles.helpBody':
    'Is é do chuntas an t-úinéir: ní bhaineann na róil seo aon rud de. Is féidir leat iad a chur in eagar nó a scriosadh am ar bith in Sytue Admin → Róil.',
  'ob.done.title': 'Déanta',
  'ob.done.headingCompleted': 'Gach rud réidh',
  'ob.done.heading': 'Seo a bhfuil socraithe agat',
  'ob.done.description': 'Tagann sé ó do chuntas fíor, ní ón méid a chlóscríobh tú anseo.',
  'ob.done.next': 'Isteach in Sytue',
  'ob.done.warnTitle': 'Níl {host} fíoraithe fós',
  'ob.done.warnBody':
    "Idir an dá linn oibríonn Sytue mar an gcéanna, le {subdomain}. Nuair a chuirfidh tú an taifead le d'fhearann, fill ar an gcéim “{step}” agus brúigh “Seiceáil anois”.",
  'ob.done.nextTitle': 'Cá háit le dul ar aghaidh',
  'ob.next.devices.title': 'Ceangail do ghuthán agus do ríomhaire',
  'ob.next.devices.body':
    "Bíodh ríomhphost, féilire agus teagmhálaithe ag obair sna haipeanna a d'úsáid tú riamh.",
  'ob.next.migrations.title': "Tabhair d'ábhar ó Microsoft 365 nó ó Google",
  'ob.next.migrations.body':
    'Cóipeálaimid ríomhphost, comhaid agus féilirí gan aon rud a chailleadh ar an mbealach.',
  'ob.next.apps.title': "Ceangail d'aipeanna",
  'ob.next.apps.body': "Na huirlisí a úsáideann d'fhoireann cheana, nasctha le do chuntas Sytue.",
  'ob.next.users.title': "Lean ort ag clárú d'fhoirne",
  'ob.next.users.body':
    'Tabhair cuireadh don dream atá in easnamh agus athbhreithnigh cén ról atá ag gach duine.',
  'ob.sum.org': 'Eagraíocht',
  'ob.sum.account': 'Do chuntas',
  'ob.sum.address': 'Seoladh',
  'ob.sum.addressLive': '{host} · foilsithe',
  'ob.sum.addressPending': '{host} · ag fanacht le foilsiú',
  'ob.sum.ownDomain': 'Fearann féin',
  'ob.sum.domainVerified': '{host} · fíoraithe',
  'ob.sum.domainPending': '{host} · ag fanacht go dtaispeánfar an taifead',
  'ob.sum.noDomain': 'Gan fearann féin',
  'ob.sum.invitations': 'Cuirí',
  'ob.sum.invitationsPending': {
    one: '1 ag fanacht le glacadh',
    two: '{count} ag fanacht le glacadh',
    few: '{count} ag fanacht le glacadh',
    many: '{count} ag fanacht le glacadh',
    other: '{count} ag fanacht le glacadh',
  },
  'ob.sum.noInvitations': 'Ceann ar bith fós',
  'ob.sum.billing': 'Billeáil',
  'ob.sum.billingEmpty': 'Gan líonadh (ní gá fós)',
  'ob.sum.roles': 'Róil cruthaithe',
  'ob.sum.edit': 'Cuir in eagar',
  'ob.sum.noRoles': 'Dada',
  'ob.sub.liveTitle': 'Tá do sheoladh foilsithe cheana',
  'ob.sub.liveBody': 'Réitítear {host} ar an idirlíon. Níl aon rud eile le déanamh agat.',
  'ob.sub.failedTitle': 'Níl do sheoladh foilsithe fós',
  'ob.sub.failedDefault': 'Níorbh fhéidir linn é a fhoilsiú faoi láthair.',
  'ob.sub.failedTail':
    'Tá do chuntas cruthaithe: lean ar aghaidh leis an gclárú agus fill ar ais i gceann tamaill.',
  'ob.sub.reservedTitle': 'Tá do sheoladh curtha in áirithe',
  'ob.sub.reservedBody':
    'Is leatsa {host}. Is sinne a fhoilseoidh ar an idirlíon é: níl aon rud le déanamh agat.',
  'ob.search.label': 'An fearann ba mhaith leat',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Scríobh é ina iomláine, lena iarmhír: acme.com, acme.ie, acme.eu…',
  'ob.search.check': 'Seiceáil infhaighteacht',
  'ob.search.useOwn': 'Chláraigh mé cheana é: ceangail é',
  'ob.search.note':
    'Ní dhíoltar fearainn ag Sytue: anseo ní insímid duit ach an bhfuil sé saor agus cé mhéad a chosnaíonn sé. Cláraigh é cibé áit is fearr leat agus fill ar an gcéim seo, faoi “Tá fearann agam cheana”, chun é a cheangal le do chuntas.',
  'ob.avail.freeTitle': 'Tá {domain} saor',
  'ob.avail.freePrice':
    "Cosnaíonn sé {registration} don chéad bhliain agus {renewal} ar gach athnuachan. Sin praghas an lae inniu ag Cloudflare; cibé áit a gcláraíonn tú é, d'fhéadfadh praghas eile a bheith air.",
  'ob.avail.freeNoPrice':
    'Is féidir é a chlárú. Inseoidh an cláraitheoir óna gceannaíonn tú é an praghas duit.',
  'ob.avail.takenTitle': 'Tá {domain} cláraithe cheana',
  'ob.avail.takenBody':
    'Tá sé ag duine éigin eile. Bain triail as ainm eile, nó iarmhír eile (.ie, .eu, .net…). Más leatsa an fearann, roghnaigh “Tá fearann agam cheana” thuas agus ceanglóimid é.',
  'ob.avail.unsupportedTitle': 'Ní féidir linn an iarmhír sin a sheiceáil',
  'ob.avail.unsupportedBody':
    'Ní féidir linn a rá leat an bhfuil sé saor ná cé mhéad a chosnaíonn sé: seiceáil é le do chláraitheoir. Nuair a bheidh sé agat, fill anseo agus roghnaigh “Tá fearann agam cheana”.',
  'ob.avail.invalidTitle': 'Ní cosúil gur fearann é sin',
  'ob.avail.invalidBody':
    'Scríobh é mar acme.com: ainm, ponc agus an iarmhír. Gan “https://” agus gan “www”.',
  'ob.avail.unconfiguredTitle': 'Ní sheiceálaimid praghsanna fearainn anseo',
  'ob.avail.unconfiguredBody':
    'Cuardaigh é ag an gcláraitheoir is fearr leat. Nuair a bheidh sé agat, fill ar an gcéim seo agus roghnaigh “Tá fearann agam cheana” chun é a cheangal.',
  'ob.avail.unknownTitle': 'Níorbh fhéidir linn é a sheiceáil faoi láthair',
  'ob.avail.unknownBody':
    'Ní hé an méid a chlóscríobh tú is cúis leis: níor fhaigheamar freagra. Bain triail eile as i gceann tamaillín.',
  'ob.dnsRec.title': "Cuir an taifead seo leis san áit a bhfuil d'fhearann cláraithe",
  'ob.dnsRec.body':
    'Aimsigh an chuid “DNS” nó “Records” ag do sholáthraí, cruthaigh taifead TXT agus cóipeáil an dá luach seo. Is é an nós imeachta céanna é i ngach áit.',
  'ob.dnsRec.type': 'Cineál',
  'ob.dnsRec.name': 'Ainm',
  'ob.dnsRec.value': 'Luach',
  'ob.dnsRec.verifiedTitle': 'Fearann fíoraithe',
  'ob.dnsRec.verifiedBody':
    "D'aimsíomar an taifead: is leatsa {host} agus tá sé cláraithe mar sin.",
  'ob.dnsRec.mismatchTitle': 'Tá taifead ann, ach ní hé seo é',
  'ob.dnsRec.mismatchBody':
    "Ag {name} d'aimsíomar “{found}”. Seiceáil gur chóipeáil tú an luach ar fad agus nach bhfuil seanluach fágtha.",
  'ob.dnsRec.unreachableTitle': 'Níorbh fhéidir linn do DNS a cheistiú',
  'ob.dnsRec.unreachableBody':
    'Ní ar do thaifead atá an locht: níor fhreagair an freastalaí ainmneacha. Bain triail eile as i gceann tamaillín.',
  'ob.dnsRec.pendingTitle': 'Ní fheicimid an taifead fós',
  'ob.dnsRec.pendingBody':
    'Má chruthaigh tú díreach anois é, is gnáth sin: tógann athruithe DNS idir cúpla nóiméad agus cúpla uair an chloig le teacht i bhfeidhm. Is féidir leat leanúint ar aghaidh leis an gclárú agus seiceáil arís am ar bith.',
  'ob.dnsRec.uncheckedTitle': 'Gan seiceáil fós',
  'ob.dnsRec.uncheckedBody':
    'Nuair a bheidh an taifead curtha leis agat ag do sholáthraí, brúigh “Seiceáil anois” agus féachfaimid ar do DNS láithreach.',
  'ob.dnsRec.empty':
    'Nuair a bhrúnn tú “Sábháil agus seiceáil” tabharfaimid duit an taifead cruinn atá le cur leis.',
  'ob.dnsRec.check': 'Seiceáil anois',
  'ob.dnsRec.save': 'Sábháil agus seiceáil',
  'ob.cf.state.ok': 'Ann cheana',
  'ob.cf.state.created': 'Cruthaithe',
  'ob.cf.state.replaced': 'Curtha ina ionad',
  'ob.cf.state.missing': 'Cuirfimid leis é',
  'ob.cf.state.conflict': 'Tá rud éigin ann cheana',
  'ob.cf.state.failed': 'Níorbh fhéidir',
  'ob.cf.state.pending': 'Ní fós',
  'ob.cf.permTitle': 'Tabhair cead dúinn do do DNS amháin',
  'ob.cf.step1': 'Oscail do chomharthaí API in Cloudflare agus brúigh “Create token”.',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2': 'Roghnaigh “Create custom token” agus cuir tic le cead amháin: Zone · DNS · Edit.',
  'ob.cf.step3':
    'Faoi “Zone resources” roghnaigh {domain}, cruthaigh an comhartha agus greamaigh anseo é.',
  'ob.cf.tokenLabel': 'Comhartha Cloudflare',
  'ob.cf.tokenPlaceholder': 'An comhartha a chruthaigh tú díreach anois',
  'ob.cf.tokenHint':
    'Ní oibríonn sé ach do DNS an fhearainn seo. Úsáidimid é agus caithimid uainn é: ní stóráiltear é mura n-iarrann tú orainn é thíos, agus ní fhágann sé ár bhfreastalaithe riamh.',
  'ob.cf.mailLabel': 'Faigh an ríomhphost do @{domain} in Sytue Mail freisin',
  'ob.cf.mailHintOn':
    "Athraíonn sé seo cá dtagann ríomhphost d'fhearainn. Má fhaigheann tú in áit eile inniu é, tabharfaimid rabhadh duit sula gcuirfimid aon rud ina ionad.",
  'ob.cf.mailHintOff':
    'Cas air é ach amháin más mian leat do bhoscaí poist @d-fhearann a aistriú go Sytue.',
  'ob.cf.rememberLabel': 'Cuimhnigh ar an gcead seo le haghaidh athruithe amach anseo',
  'ob.cf.rememberHint':
    'Má fhágann tú gan tic é, beidh ort an comhartha a ghreamú arís an chéad uair eile. Má chuireann tú tic leis, stóráiltear criptithe é; má bhaineann tú an tic, scriostar é.',
  'ob.cf.preview': 'Féach cad a athróimid',
  'ob.cf.previewAgain': 'Féach arís',
  'ob.cf.nothing': 'Níl aon rud fágtha le déanamh',
  'ob.cf.apply': {
    one: 'Cuir 1 athrú i bhfeidhm',
    two: 'Cuir {count} athrú i bhfeidhm',
    few: 'Cuir {count} athrú i bhfeidhm',
    many: 'Cuir {count} n-athrú i bhfeidhm',
    other: 'Cuir {count} athrú i bhfeidhm',
  },
  'ob.cf.zone': 'Crios aimsithe',
  'ob.cf.token': 'Do chomhartha',
  'ob.cf.tokenStored': 'Stóráilte criptithe le haghaidh athruithe amach anseo',
  'ob.cf.tokenDiscarded': 'Úsáidte agus caite uainn (níor stórálamar é)',
  'ob.cf.domainState': 'Stádas an fhearainn',
  'ob.cf.verified': 'Fíoraithe',
  'ob.cf.pending': 'Ag fanacht le fíorú',
  'ob.cf.nowThere': 'Faoi láthair tá: {current}',
  'ob.cf.replaceLabel': 'Cuir taifead Sytue ina ionad',
  'ob.cf.failTitle': 'Níor socraíodh rud éigin',
  'ob.cf.failBody':
    'Tá an chuid eile i bhfeidhm. Thuas tá an chúis le gach taifead a theip agus a luach, chun é a chur leis de láimh ó Cloudflare.',
  'ob.cf.conflictTitle': 'Tá taifid agat nach mbainfimid leo',
  'ob.cf.conflictBody':
    'Ní athraímid riamh aon rud atá agat cheana mura gcuireann tú tic leis. Más fearr leat iad a chumasc de láimh, tá an luach reatha agus ár luach féin thuas.',
  'ob.cf.okTitle': 'Fearann fíoraithe',
  'ob.cf.okBody':
    'Chonaiceamar an taifead i do chrios féin: is leatsa {host} agus tá sé cláraithe mar sin.',
  'ob.cf.todoTitle': 'Tá na hathruithe fós le cur i bhfeidhm',
  'ob.cf.todoBody':
    'Níor bhaineamar le do chrios fós. Brúigh “Cuir i bhfeidhm” nuair a bheidh tú sásta leis an méid thuas.',
  'ob.cf.someFailed': {
    one: 'Níor socraíodh 1 taifead',
    two: 'Níor socraíodh {count} thaifead',
    few: 'Níor socraíodh {count} thaifead',
    many: 'Níor socraíodh {count} dtaifead',
    other: 'Níor socraíodh {count} taifead',
  },
  'ob.cf.someFailedBody':
    'Thíos tá an chúis le gach ceann acu agus na luachanna chun é a dhéanamh de láimh.',
  'ob.cf.configured': '{host} socraithe agus fíoraithe',
  'ob.cf.created': 'Taifid cruthaithe i do chrios',
  'today.title': 'Inniu',
  'today.openCalendar': 'Oscail an féilire',
  'today.noEvents': 'Níl aon rud ar an gclár inniu.',
  'today.unavailable': 'Níl an clár ar fáil faoi láthair.',
  'today.allDay': 'An lá ar fad',
  'today.untitled': '(gan teideal)',
  'today.unread': {
    one: '1 ríomhphost gan léamh',
    two: '{count} ríomhphost gan léamh',
    few: '{count} ríomhphost gan léamh',
    many: '{count} ríomhphost gan léamh',
    other: '{count} ríomhphost gan léamh',
  },
  'today.inboxZero': 'Bosca isteach glan',
  'launcher.quickActions': 'Gníomhartha tapa',
  'launcher.recentAll': 'Le déanaí ar fud na sraithe',
  'launcher.activityError': 'Níorbh fhéidir linn do ghníomhaíocht a lódáil',
  'launcher.noActivity': 'Gan ghníomhaíocht le déanaí',
  'ob.dns.verified': 'Fearann fíoraithe',
  'ob.dns.mismatch': 'Tá taifead ann, ach ní hé seo é',
  'ob.dns.unreachable': 'Níorbh fhéidir linn do DNS a cheistiú',
  'ob.dns.notYet': 'Ní fheicimid an taifead fós',
  'ob.dns.unchecked': 'Gan seiceáil fós',
  'ob.dns.type': 'Cineál',
  'ob.dns.name': 'Ainm',
  'ob.dns.value': 'Luach',
  'ob.cf.tokenField': 'Comhartha Cloudflare',
  'ob.cf.remember': 'Cuimhnigh ar an gcead seo le haghaidh athruithe amach anseo',
  'ob.cf.replaceWithSytue': 'Cuir taifead Sytue ina ionad',
  'recent.hubDown':
    'Níl an Hub ag freagairt faoi láthair. Tá do chuid oibre slán; bain triail eile as.',
  'search.partial':
    "Níor fhreagair {sources}, mar sin d'fhéadfadh torthaí a bheith in easnamh. Bain triail eile as i gceann tamaillín.",
  'search.partialJoin': ' agus ',
  'search.showing': 'Ag taispeáint {count} as {total} toradh.',
  'privacy.notice':
    'Ní dhíolaimid do chuid sonraí agus ní thugaimid do thríú páirtithe iad. Ní úsáidimid iad chun próifíl a dhéanamh díot, le haghaidh fógraíochta, ná chun ár seirbhísí féin ná seirbhísí aon duine eile a thraenáil ná a fheabhsú. Tá siad criptithe agus iad á seoladh agus i mbun stórála, agus ní fhágann siad ár mbonneagar Eorpach ach a mhéid is fíor-riachtanach chun billeáil a dhéanamh ort agus sna comhtháthuithe a chuireann tú féin ar siúl, áit a n-iarrfaidh an tríú páirtí do chead maidir leis na sonraí a úsáidfidh sé.',
};
