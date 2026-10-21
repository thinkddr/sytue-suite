/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ITALIANO de Sytue Admin. Terminología de Microsoft Entra en italiano:
 * «Accesso condizionale», «Autorizzazione», «Etichette di riservatezza».
 * docs/CONVENTIONS.md §13.1.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const it: AppCatalog<typeof en> = {
  // --- Cromo: navegación y armazón ---------------------------------------
  'nav.dashboard': 'Dashboard',
  'nav.tenants': 'Organizzazioni',
  'nav.users': 'Persone',
  'nav.groups': 'Gruppi',
  'nav.roles': 'Ruoli',
  'nav.pim': 'Accesso con privilegi',
  'nav.sso': 'Single sign-on (SSO)',
  'nav.access': 'Accesso condizionale',
  'nav.devices': 'Dispositivi',
  'nav.apps': 'Integrazioni',
  // Editor de documentos de la organizacion: era una variable de entorno de Drive.
  'nav.documentEditor': 'Editor di documenti',
  'nav.externalSignatures': 'Firme dall’esterno',
  'page.documentEditor': 'Editor di documenti',
  'page.externalSignatures': 'Firme dall’esterno',
  'externalSignatures.title': 'Richieste di firma da altre organizzazioni',
  'externalSignatures.description':
    'Decidi cosa succede quando qualcuno di un’altra organizzazione chiede a una persona di qui di firmare un PDF in CertiPDF.',
  'externalSignatures.policy': 'Chi può chiedere',
  'externalSignatures.mode.abierta': 'Chiunque',
  'externalSignatures.mode.abierta.help':
    'Arrivano le richieste di qualsiasi organizzazione. È l’impostazione predefinita.',
  'externalSignatures.mode.cerrada': 'Nessuno dall’esterno',
  'externalSignatures.mode.cerrada.help':
    'Le richieste di altre organizzazioni non arrivano qui e non si possono aprire da qui. Chi ha anche un account personale può riceverle lì.',
  'externalSignatures.mode.confianza': 'Solo organizzazioni fidate',
  'externalSignatures.mode.confianza.help':
    'Arrivano solo le richieste delle organizzazioni nella lista.',
  'externalSignatures.trusted': 'Organizzazioni fidate',
  'externalSignatures.trustedHelp':
    'Una per riga: il nome nel loro indirizzo, come «acme» in …/docs/acme/.',
  'externalSignatures.silent': 'L’altra parte non lo sa',
  'externalSignatures.silentHelp':
    'Per chi l’ha fatta, una richiesta che non arriva sembra comunque inviata. Così nessuno scopre a quali organizzazioni appartiene una persona.',
  'externalSignatures.save': 'Salva',
  'externalSignatures.saved': 'Salvato',
  'externalSignatures.saveFailed': 'Impossibile salvare. Riprova.',
  'externalSignatures.badOrg': '«{org}» non è un nome di organizzazione.',
  'externalSignatures.loadFailed': 'Impossibile caricare',
  'externalSignatures.loadFailedHelp': 'Sytue Docs non risponde. Riprova tra un momento.',
  'editor.title': 'Editor di documenti',
  'editor.description':
    'Scegli con quale editor si aprono i documenti Office della tua organizzazione.',
  'editor.current': 'Editor attuale',
  'editor.none': 'Nessun editor esterno collegato',
  'editor.noneHelp':
    'I documenti Sytue si modificano già in Sytue. Collegane uno solo se la tua organizzazione ha già un editor — Collabora, ONLYOFFICE — e vuole continuare a usarlo per i file di Word, Excel e PowerPoint.',
  'editor.connect': 'Collega un editor',
  'editor.change': 'Cambia',
  'editor.disconnect': 'Scollega',
  'editor.disconnectConfirm': 'Scollegare l’editor? Quei file smetteranno di aprirsi lì.',
  'editor.disconnected': 'Editor scollegato',
  'editor.disconnectFailed': 'Non è stato possibile scollegarlo.',
  'editor.unnamed': 'Editor esterno',
  'editor.responding': 'Risponde',
  'editor.notResponding': 'Non risponde',
  'editor.paused': 'In pausa',
  'editor.notRespondingTitle': 'L’editor non risponde in questo momento',
  'editor.notRespondingUnknown': 'Non ha dato altri dettagli.',
  'editor.opens': 'Apre: {list}',
  'editor.opensNothing': 'Non ha dichiarato alcun tipo di file.',
  'editor.wizard.description': 'Tre passaggi. Non cambia nulla fino all’ultimo.',
  'editor.step1': 'Quale',
  'editor.step1.heading': 'Che editor hai?',
  'editor.step1.description': 'Scegli il tuo: il resto dell’indirizzo lo componiamo noi.',
  'editor.kind.collabora': 'Collabora Online',
  'editor.kind.collabora.desc':
    'Il più diffuso nelle installazioni proprie. Basato su LibreOffice.',
  'editor.kind.onlyoffice': 'ONLYOFFICE',
  'editor.kind.onlyoffice.desc': 'Il più simile a Microsoft Office nell’aspetto.',
  'editor.kind.manual': 'Un altro',
  'editor.kind.manual.desc': 'Qualsiasi editor che parli WOPI. L’indirizzo lo dai tu per intero.',
  'editor.step2': 'Dove',
  'editor.step2.heading': 'Dove si trova?',
  'editor.step2.description': 'L’indirizzo con cui ci accedi, nient’altro.',
  'editor.hostLabel': 'Indirizzo del tuo editor',
  'editor.urlLabel': 'Indirizzo del suo catalogo (discovery)',
  'editor.hostHint':
    'Solo l’indirizzo, come https://office.tuaazienda.com — il resto lo aggiungiamo noi.',
  'editor.urlHint': 'L’indirizzo completo del suo documento di discovery.',
  'editor.willUse': 'Chiederemo a: {url}',
  'editor.check': 'Verifica',
  'editor.checkFailed': 'Non è stato possibile raggiungerlo.',
  'editor.checkFailedTitle': 'Quell’editor non ha risposto',
  'editor.step3': 'Conferma',
  'editor.step3.heading': 'Ha risposto. Questo è ciò che verrà impostato.',
  'editor.step3.description': 'Non è stato ancora salvato nulla.',
  'editor.step3.headingFailed': 'Non ha risposto',
  'editor.step3.descriptionFailed':
    'Torna indietro e controlla l’indirizzo. Non è stato salvato nulla.',
  'editor.summary.kind': 'Editor',
  'editor.summary.url': 'Indirizzo',
  'editor.summary.opens': 'Apre',
  'editor.save': 'Collegalo',
  'editor.connected': 'Editor collegato',
  'editor.hostDisabled': 'Questa installazione non può aprire i file in un editor esterno',
  'editor.hostDisabledHelp':
    'Sytue Drive non ha una chiave WOPI, quindi non può generare i token con cui un editor apre un file. Il tuo editor è salvato, ma nessuno vedrà «apri nell’editor» finché chi amministra questa installazione non imposta quella chiave.',
  'editor.saveFailed': 'Non è stato possibile salvare.',
  'nav.migrations': 'Importa contenuti',
  // Administración de los canales de Rooms de la organización.
  'nav.rooms': 'Canali di Rooms',
  'rooms.channels.title': 'Canali',
  'rooms.channels.subtitle':
    'Tutti i canali dell’organizzazione, compresi i privati. I messaggi diretti non compaiono mai.',
  'rooms.filter.placeholder': 'Filtra per canale o team',
  'rooms.showArchived': 'Mostra archiviati',
  'rooms.col.channel': 'Canale',
  'rooms.col.kind': 'Tipo',
  'rooms.col.members': 'Persone',
  'rooms.col.lastActivity': 'Ultima attività',
  'rooms.col.state': 'Stato',
  'rooms.kind.public': 'Pubblico',
  'rooms.kind.private': 'Privato',
  'rooms.state.active': 'Attivo',
  'rooms.state.archived': 'Archiviato',
  'rooms.action.archive': 'Archivia',
  'rooms.action.restore': 'Ripristina',
  'rooms.stat.channels': 'Canali',
  'rooms.stat.private': 'Privati',
  'rooms.stat.archived': 'Archiviati',
  'rooms.stat.teams': 'Team',
  'rooms.empty': 'Nessun canale corrisponde',
  'rooms.emptyBody':
    'Cambia il filtro, oppure attiva «Mostra archiviati» se cerchi un canale messo da parte.',
  'rooms.toast.archived': 'Canale archiviato',
  'rooms.toast.restored': 'Canale ripristinato',
  'rooms.toast.failed': 'Non è stato possibile applicare la modifica',
  'rooms.error.forbidden': 'Non puoi amministrare Rooms',
  'rooms.error.forbiddenBody': 'Questa schermata è per chi possiede o amministra l’organizzazione.',
  'rooms.error.unavailable': 'Rooms non risponde',
  'rooms.error.unavailableBody':
    'Non è stato possibile leggere i canali. Forse il servizio si sta riavviando; riprova tra poco.',
  'nav.erp': 'Permessi ERP',
  'erp.people.title': 'Persone',
  'erp.people.subtitle': 'Tutta l’organizzazione, che abbia già aperto l’ERP o no.',
  'erp.filter': 'Filtra per nome o email',
  'erp.admin': 'Amministra tutto',
  'erp.adminBody':
    'Gli account con la proprietà o l’amministrazione dell’organizzazione amministrano tutti i moduli dell’ERP. Per limitarli, cambia il loro ruolo in Persone.',
  'erp.pick': 'Scegli una persona',
  'erp.levels.help':
    'Nessun accesso: non vede il modulo · Lettura: lo vede ma non modifica nulla · Utente: crea e modifica · Amministratore: tutto, compreso annullare le registrazioni.',
  'erp.modules.title': 'Accesso per modulo',
  'erp.modules.subtitle':
    'Cosa può vedere e fare questa persona in ogni modulo. Un livello uguale a quello predefinito non viene salvato: se il predefinito cambia, vale per chi non è stato impostato a mano.',
  'erp.col.module': 'Modulo',
  'erp.col.level': 'Livello',
  'erp.default': 'predefinito',
  'erp.level.ninguno': 'Nessun accesso',
  'erp.level.lectura': 'Lettura',
  'erp.level.usuario': 'Uso',
  'erp.level.administrador': 'Amministrazione',
  'erp.obligationsNote':
    'Chi ha accesso agli Adempimenti riceve un’email 10 giorni prima di ogni scadenza.',
  'erp.toast.saved': 'Permessi salvati',
  'erp.toast.failed': 'Non è stato possibile salvare i permessi',
  'erp.error.forbidden': 'Non puoi gestire i permessi dell’ERP',
  'erp.error.forbiddenBody':
    'Questa schermata è per gli account con la proprietà o l’amministrazione dell’organizzazione.',
  'erp.error.unavailable': 'Sytue ERP non risponde',
  'erp.error.unavailableBody':
    'Non è stato possibile leggere i permessi. Forse il servizio si sta riavviando; riprova tra poco.',
  'nav.governance': 'Governance',
  'nav.compliance': 'Conformità',
  'nav.sensitivityLabels': 'Etichette di riservatezza',
  'nav.audit': 'Controllo',
  'nav.account': 'Il mio account',
  'shell.brand': 'Sytue Admin',
  'org.active': 'Organizzazione attiva',
  'org.switch': 'Cambia organizzazione',
  'org.none': 'Nessuna organizzazione',
  'org.switchFailed': 'Cambio non riuscito. Riprova.',
  'scim.title': 'Provisioning automatico (SCIM)',
  'scim.on': 'Attivo',
  'scim.off': 'Non configurato',
  'scim.unknown': 'Sconosciuto',
  'scim.onHint':
    'Il tuo provider di identità crea, aggiorna e DISATTIVA gli account in Sytue automaticamente.',
  'scim.offHint':
    'Gli account si gestiscono a mano: chi se ne va resta attivo finché qualcuno non lo rimuove qui.',
  'scim.unknownHint':
    'Sytue ID non ha risposto, quindi lo stato del provisioning è sconosciuto. Ricarica tra poco.',
  'scim.provisioned': 'Persone provisionate',
  'scim.lastEvent': 'Ultima modifica dal provider',
  'scim.lastEventNever': 'Mai',
  'scim.endpoint': 'Endpoint SCIM',
  'platform.org.protected': 'Organizzazione della piattaforma — protetta',
  'scim.copy': 'Copia',
  'scim.copied': 'Copiato',
  'scim.step1': 'Incolla questo indirizzo nel tuo provider di identità',
  'scim.step2': 'Genera il token e copialo',
  'scim.step3': 'Attiva il provisioning nel tuo provider',
  'scim.step3Hint':
    'In Microsoft Entra ID: Applicazioni aziendali → la tua applicazione → Provisioning → Automatico, incolla l’URL del tenant e il token segreto, poi Testa connessione. In Okta: Applications → la tua applicazione → Provisioning → Configure API Integration. Dal lato Sytue non serve altro.',
  'scim.token': 'Token segreto',
  'scim.tokenOnce':
    'Copialo ora: viene mostrato una sola volta e non è recuperabile. Se lo perdi, generane un altro e revoca questo.',
  'scim.tokenLabel': 'Nome (facoltativo)',
  'scim.tokenLabelHint': 'Per distinguerlo in seguito, es. «Entra ID».',
  'scim.generate': 'Genera token',
  'scim.tokenFailed': 'Impossibile generare il token',
  'scim.tokensLive': 'Token attivi',
  'scim.tokenNoLabel': 'Token senza nome',
  'scim.tokenUsed': 'ultimo uso {when}',
  'scim.tokenNeverUsed': 'mai usato',
  'scim.revoke': 'Revoca',
  'scim.tokenRevoked': 'Token revocato',
  'scim.tokenRevokeFailed': 'Impossibile revocare il token',
  'platform.admins.add': 'Concedi ambito globale',
  'platform.admins.email': 'E-mail della persona',
  'platform.admins.reason': 'Motivo',
  'platform.admins.reasonHint': 'Perché questa persona deve amministrare l’intera piattaforma.',
  'platform.admins.grant': 'Concedi',
  'platform.admins.granted': 'Ambito globale concesso',
  'platform.approval.pending': 'In attesa dell’approvazione di un altro account di amministrazione',
  'platform.action.cancel': 'Annulla',
  'conn.error.catalog':
    'Non è stato possibile caricare il catalogo dei connettori, quindi non si può aggiungere alcun provider.',
  'shell.sectionAdmin': 'Amministrazione',
  'shell.sectionAccount': 'Account',
  'shell.metaDescription': 'Sytue Admin — amministrazione di organizzazioni, persone e gruppi.',

  // --- Títulos de página --------------------------------------------------
  'page.dashboard': 'Dashboard',
  'page.tenants': 'Organizzazioni',
  'page.users': 'Persone',
  'page.groups': 'Gruppi',
  'page.roles': 'Ruoli',
  'page.pim': 'Accesso con privilegi',
  'page.sso': 'Single sign-on',
  'page.access': 'Accesso condizionale',
  'page.devices': 'Dispositivi',
  'page.apps': 'Integrazioni',
  'page.migrations': 'Importa contenuti',
  'page.governance': 'Governance',
  'page.compliance': 'Conformità',
  'page.sensitivityLabels': 'Etichette di riservatezza',
  'page.audit': 'Controllo',
  'page.account': 'Il mio account',
  'page.tenantNotFound': 'Organizzazione non trovata.',

  // --- Acciones y textos compartidos --------------------------------------
  'common.save': 'Salva',
  'common.cancel': 'Annulla',
  'common.create': 'Crea',
  'common.delete': 'Elimina',
  'common.edit': 'Modifica',
  'common.close': 'Chiudi',
  'common.retry': 'Riprova',
  'common.next': 'Avanti',
  'common.search': 'Cerca',
  'common.loading': 'Caricamento…',
  'common.loadMore': 'Carica altro',
  'common.copy': 'Copia',
  'common.copied': 'Copiato',
  'common.enabled': 'Attivato',
  'common.disabled': 'Disattivato',
  'common.yes': 'Sì',
  'common.no': 'No',
  'common.all': 'Tutti',
  'common.optional': 'facoltativo',
  'common.saveFailed': 'Impossibile salvare',
  'common.tryAgain': 'Riprova tra qualche secondo.',
  // --- Etiquetas de dominio (roles, aprovisionamiento) --------------------
  'role.owner': 'Proprietario',
  'role.admin': 'Amministratore',
  'role.member': 'Membro',
  'role.billing': 'Fatturazione',
  'provision.synced': 'sincronizzato',
  'provision.pending': 'in attesa',
  'provision.error': 'errore',

  // --- Panel --------------------------------------------------------------
  'dashboard.subtitle': 'Stato del realm {realm} e provisioning della suite.',
  'dashboard.metric.tenants': 'Organizzazioni',
  'dashboard.metric.tenantsHint': 'tenant attivi',
  'dashboard.metric.users': 'Persone in totale',
  'dashboard.metric.usersHint': 'in Sytue ID',
  'dashboard.metric.mfa': 'MFA abilitata',
  'dashboard.metric.mfaHint': 'obbligatoria per amministrazione/proprietà',
  // Qué hay que mirar hoy (portada accionable).
  'dashboard.todo.title': 'Da guardare oggi',
  'dashboard.todo.review': 'Rivedere',
  'dashboard.todo.reinvite': 'Invitare di nuovo',
  'dashboard.todo.withoutMfa': {
    one: '{count} persona non ha il secondo fattore',
    other: '{count} persone non hanno il secondo fattore',
  },
  'dashboard.todo.invitationsExpired': {
    one: '{count} invito è scaduto senza essere accettato',
    other: '{count} inviti sono scaduti senza essere accettati',
  },
  'dashboard.todo.unverified': {
    one: '{count} indirizzo email è ancora da verificare',
    other: '{count} indirizzi email sono ancora da verificare',
  },
  'dashboard.todo.suspended': {
    one: '{count} account sospeso è ancora aperto',
    other: '{count} account sospesi sono ancora aperti',
  },

  // Destino SIEM de la auditoría (mig 0035, sin pantalla hasta ahora).
  'sink.title': 'Inviare questo registro al tuo SIEM',
  'sink.intro':
    'Gli eventi partono a lotti, in ordine e firmati, da dove si è fermata l’ultima consegna. Se il tuo SIEM cade non si perde nulla: il lotto viene ritentato.',
  'sink.url': 'Destinazione (HTTPS)',
  'sink.authHeader': 'Intestazione di autorizzazione',
  'sink.signingSecret': 'Segreto di firma',
  'sink.signingHint':
    'Si usa nell’intestazione X-Sytue-Signature (HMAC-SHA256), per verificare che il lotto venga da Sytue.',
  'sink.keepSecret': 'Salvato — lascialo vuoto per conservarlo',
  'sink.enabled': 'Invio attivato',
  'sink.remove': 'Togliere la destinazione',
  'sink.state.on': 'In invio',
  'sink.state.off': 'In pausa',
  'sink.state.failing': 'In errore',
  'sink.lastSent': 'Ultima consegna: {when}',
  'sink.toast.saved': 'Destinazione salvata.',
  'sink.toast.removed': 'Destinazione rimossa.',
  'sink.toast.failed': 'Non è stato possibile salvare.',

  'dashboard.tenants.title': 'Organizzazioni',
  'dashboard.tenants.viewAll': 'Visualizza tutte',
  'dashboard.tenants.emptyTitle': 'Nessuna organizzazione',
  'dashboard.tenants.emptyDescription':
    'Crea la prima organizzazione per effettuare il provisioning della suite.',
  'dashboard.tenants.newTenant': 'Nuova organizzazione',
  'dashboard.tenants.userCount': { one: '{count} persona', other: '{count} persone' },
  'dashboard.activity.title': 'Attività recente',
  'dashboard.activity.viewAudit': 'Visualizza controllo',
  'dashboard.activity.emptyTitle': 'Ancora nessuna attività',
  'dashboard.activity.emptyDescription':
    'Gli eventi di identità e provisioning verranno visualizzati qui.',
  'dashboard.activity.ok': ' — riuscito',
  'dashboard.activity.failed': ' — non riuscito',

  // --- Verificación de correo --------------------------------------------
  'verifyEmail.region': 'Verifica dell’indirizzo e-mail',
  'verifyEmail.sent':
    'Abbiamo inviato un collegamento di verifica a {email}. Controlla la posta in arrivo.',
  'verifyEmail.pending':
    'Il tuo indirizzo {email} non è verificato. Verificalo per proteggere il tuo account.',
  'verifyEmail.sentAnnouncement': 'Collegamento di verifica inviato.',
  'verifyEmail.errorAnnouncement': 'Impossibile inviare il collegamento di verifica.',
  'verifyEmail.resend': 'Invia di nuovo la verifica',
  'verifyEmail.errorHint': 'Invio non riuscito. Riprova tra qualche secondo.',

  // --- Mi perfil ----------------------------------------------------------
  'profile.title': 'Profilo',
  'profile.displayName': 'Nome visualizzato',
  'profile.nameEmpty': 'Il nome non può essere vuoto.',
  'profile.email': 'Indirizzo e-mail',
  'profile.emailHint': 'L’indirizzo e-mail si modifica tramite un flusso di verifica separato.',
  'profile.saved': 'Profilo aggiornato',
  'profile.invalidName': 'Il nome non è valido.',

  // --- Política de contraseñas -------------------------------------------
  'passwordPolicy.title': 'Criteri per le password',
  'passwordPolicy.description':
    'Requisiti minimi che Sytue ID applica ogni volta che una password dell’organizzazione viene impostata o modificata.',
  'passwordPolicy.offline':
    'Nessuna connessione a Sytue ID: vengono mostrati i valori predefiniti e le modifiche non possono essere salvate.',
  'passwordPolicy.minLength': 'Lunghezza minima',
  'passwordPolicy.range': {
    one: 'Deve essere compresa tra {min} e {count} carattere.',
    other: 'Deve essere compresa tra {min} e {count} caratteri.',
  },
  'passwordPolicy.complexity': 'Richiedi complessità',
  'passwordPolicy.complexityHint': 'Richiede almeno una lettera e una cifra.',
  'passwordPolicy.save': 'Salva criteri',
  'passwordPolicy.saved': 'Criteri salvati',
  'passwordPolicy.savedHint':
    'Verranno applicati alla prossima impostazione o modifica di una password.',
  'passwordPolicy.saveFailed': 'Impossibile salvare i criteri',
  'passwordPolicy.forbidden':
    'Non disponi delle autorizzazioni per modificare i criteri dell’organizzazione.',

  // --- Dominio di posta dell’organizzazione --------------------------------
  'orgColor.title': "Colore dell'organizzazione",
  'orgColor.description':
    "Il colore della suite: barre, pulsanti e link in tutte le app. Ogni persona può scegliere il proprio nel menu dell'account; questo è quello che vede chi non ha scelto.",
  'orgColor.error': 'Impossibile salvare il colore. Riprova.',
  'orgForma.title': "Forma dell'organizzazione",
  'orgForma.description':
    "La forma della suite: «Cornice», in cui la barra e il menu formano una cornice colorata attorno al contenuto, o «Corsia», in cui solo il menu laterale porta il colore. Ognuno può scegliere la propria nel menu dell'account; questa è quella che vede chi non ha scelto.",
  'orgForma.error': 'Impossibile salvare la forma. Riprova.',
  'logo.title': "Logo dell'organizzazione",
  'logo.description':
    'Sostituisce il marchio di Sytue nella barra superiore di tutte le applicazioni. Senza un logo proprio viene usato il nostro, con un colore proprio della tua organizzazione.',
  'logo.previewDark': 'Tema scuro',
  'logo.previewLight': 'Tema chiaro',
  'logo.previewApp': 'Sytue Drive',
  'logo.choose': 'Scegli immagine…',
  'logo.save': 'Salva il logo',
  'logo.reset': 'Torna al marchio di Sytue',
  'logo.formats':
    'PNG, JPEG o GIF, fino a 1 MB. Se il tuo logo è in SVG, esportalo in PNG con sfondo trasparente.',
  'logo.trimmed': 'Margine vuoto rimosso',
  'logo.trimmedHint':
    "L'immagine aveva spazio vuoto attorno ed è stata ritagliata perché il logo appaia il più grande possibile. Verifica sopra che la composizione sia quella che ti aspetti.",
  'logo.opaqueBox': 'Il logo porta con sé il proprio sfondo',
  'logo.opaqueBoxHint':
    "L'immagine non è trasparente, quindi apparirà come un riquadro sulla barra. Se non è ciò che vuoi, esporta il logo in PNG con sfondo trasparente.",
  'logo.trimCapped': 'È stata ritagliata solo una parte del margine',
  'logo.trimCappedHint':
    "L'immagine aveva molto spazio vuoto attorno. È stata ritagliata fino al limite che il sistema si concede, per non rischiare di tagliare parte del logo. Se lo vuoi più grande, ritaglia tu il margine e caricalo di nuovo.",
  'logo.error.svg':
    'Non è stato possibile leggere questo SVG. Aprilo nel tuo programma di grafica ed esportalo in PNG con sfondo trasparente.',
  'logo.error.title': 'Non è stato possibile usare questa immagine',
  'logo.error.generic': "Non è stato possibile elaborare l'immagine. Riprova.",
  'logo.error.delete': 'Non è stato possibile rimuovere il logo. Riprova.',
  'orgDomain.title': 'Dominio di posta',
  'orgDomain.description':
    'Dove vive la posta della tua organizzazione. Tutti hanno sempre un indirizzo sul sottodominio Sytue; collegare il proprio dominio è facoltativo.',
  'orgDomain.offline':
    'Nessuna connessione a Sytue ID: al momento il dominio non si può leggere né modificare.',
  'orgDomain.base': 'Sottodominio Sytue',
  'orgDomain.baseHint':
    'C’è sempre e non si può togliere. Tutti nell’organizzazione hanno un indirizzo qui, ed è quello che il resto della suite usa per consegnare la posta interna.',
  'orgDomain.own': 'Il tuo dominio',
  'orgDomain.connect': 'Collega',
  'orgDomain.connectHint':
    'Scrivilo come acme.com, senza «https://» né «@». Ti diamo il record DNS esatto per dimostrare che è tuo.',
  'orgDomain.badgeOwn': 'Dominio proprio',
  'orgDomain.recordHint':
    'Aggiungi questo record TXT dove hai registrato il dominio. Cerca la sezione «DNS» o «Record»: la procedura è la stessa ovunque.',
  'orgDomain.recordType': 'Tipo',
  'orgDomain.recordName': 'Nome',
  'orgDomain.recordValue': 'Valore',
  'orgDomain.copy': 'Copia',
  'orgDomain.copied': 'Copiato',
  'orgDomain.copyFailed': 'Non si è potuto copiare: seleziona il valore e copialo a mano.',
  'orgDomain.check': 'Salva e verifica',
  'orgDomain.recheck': 'Verifica di nuovo',
  'orgDomain.verified': 'Dominio verificato',
  'orgDomain.verifiedHint': 'Abbiamo trovato il record: {host} è tuo.',
  'orgDomain.movedHint':
    'I nuovi indirizzi usano già il tuo dominio, e chi era già nell’organizzazione conserva l’indirizzo precedente come alias.',
  'orgDomain.pending': 'Non si vede ancora',
  'orgDomain.pendingHint':
    'Se l’hai appena creato è normale: le modifiche DNS impiegano da qualche minuto a qualche ora. Puoi riprovare quando vuoi.',
  'orgDomain.mismatch': 'È pubblicato un altro record',
  'orgDomain.mismatchHint':
    'Su {name} abbiamo trovato «{found}». Controlla di aver copiato il valore intero e che non ne sia rimasto uno vecchio.',
  'orgDomain.unreachable': 'Non siamo riusciti a interrogare il DNS',
  'orgDomain.unreachableHint':
    'Non dipende dal tuo record: il server dei nomi non ha risposto. Riprova tra un momento.',
  'orgDomain.disconnect': 'Scollega',
  'orgDomain.disconnected': 'Dominio scollegato',
  'orgDomain.disconnectedHint':
    'I nuovi indirizzi tornano al sottodominio Sytue. Le caselle esistenti continuano a funzionare.',
  'orgDomain.primary': 'Principale',
  'orgDomain.badgePending': 'In attesa',
  'orgDomain.addAnother': 'Aggiungi un altro dominio',
  'orgDomain.addedHint':
    'Riceve già la posta e permette di inviare. I nuovi indirizzi continuano a nascere sul dominio principale.',
  'orgDomain.makePrimary': 'Rendi principale',
  'orgDomain.makePrimaryConfirm':
    'I nuovi indirizzi verranno creati su {host} e tutta l’organizzazione ci si trasferisce. Ognuno conserva il proprio indirizzo attuale come alias, quindi non smette di arrivare nulla. Continuare?',
  'orgDomain.primaryChanged': 'Dominio principale cambiato',
  'orgDomain.disconnectConfirm':
    'Sytue smetterà di servire la posta di {host}. Gli indirizzi che esistono già continuano a funzionare. Continuare?',
  'orgDomain.saveFailed': 'Non si è potuto salvare il dominio',

  // --- Usuarios -----------------------------------------------------------
  'users.subtitle': 'Directory di Sytue ID. Ogni account dà accesso all’intera suite.',
  'users.offline':
    'Nessuna connessione a Sytue ID: vengono mostrati dati di esempio. Le modifiche non verranno salvate.',
  'users.searchPlaceholder': 'Cerca persona o e-mail',
  'users.attrs.title': 'Attributi della directory',
  'users.attrs.hint':
    'I gruppi dinamici decidono chi ne fa parte in base a questi tre attributi. Salvando, i suoi gruppi vengono rivalutati subito.',
  'users.attrs.saved': 'Attributi salvati',
  'users.attrs.savedHint': 'I gruppi dinamici di {email} sono stati rivalutati.',
  'users.searchLabel': 'Cerca persona',
  'users.filterTenant': 'Filtra per organizzazione',
  'users.allTenants': 'Tutte le organizzazioni',
  'users.invite': 'Invita una persona',
  'users.listLabel': 'Account di Sytue ID',
  'users.openRecord': 'Apri la scheda di {name}',
  'users.mfaOn': 'attiva',
  'users.mfaOff': 'senza MFA',
  'users.statusActive': 'attivo',
  'users.statusDisabled': 'disabilitato',
  'users.modalFallbackTitle': 'Persona',
  'users.col.user': 'Persona',
  'users.col.roles': 'Ruoli',
  'users.col.tenant': 'Organizzazione',
  'users.col.provisioning': 'Provisioning',
  'users.col.status': 'Stato',
  'users.empty.title': 'Nessun utente corrisponde',
  'users.empty.description': 'Controlla il testo cercato o il filtro per organizzazione.',
  'users.empty.clearFilters': 'Cancella filtri',

  // Acciones sobre varias personas a la vez.
  'users.bulk.select': 'Selezionare {name}',
  'users.bulk.selected': { one: '{count} selezionata', other: '{count} selezionate' },
  'users.bulk.suspend': 'Sospendere',
  'users.bulk.reactivate': 'Riattivare',
  'users.bulk.revokeSessions': 'Chiudere le sessioni',
  'users.bulk.setRole': 'Cambiare ruolo…',
  'users.bulk.clear': 'Togliere la selezione',
  'users.bulk.done': {
    one: 'Applicato a {count} persona',
    other: 'Applicato a {count} persone',
  },
  'users.bulk.partial': {
    one: '{count} è rimasta fuori (non puoi farlo, o non è più qui).',
    other: '{count} sono rimaste fuori (non puoi farlo, o non sono più qui).',
  },
  'users.bulk.failed': 'Non è stato possibile applicare l’azione in blocco',

  // Impacto real antes de confirmar (sesiones, grupos, último propietario).
  'users.suspend.question': 'Togliere l’accesso a {email}?',
  'users.suspend.action': 'Sospendere l’accesso',
  'users.impact.sessions': {
    one: 'Verrà chiusa {count} sessione aperta',
    other: 'Verranno chiuse {count} sessioni aperte',
  },
  'users.impact.groups': { one: 'È in {count} gruppo', other: 'È in {count} gruppi' },
  'users.impact.lastOwner':
    'È l’unica persona proprietaria: senza di lei nessuno può amministrare questa organizzazione.',
  'users.empty.noMatch': 'Nessuno in questa organizzazione corrisponde a quanto hai scritto.',
  'users.toast.searchFailed': 'Non è stato possibile cercare',
  'users.detail.lastLogin': 'Ultimo accesso',
  'users.detail.created': 'Creato',
  'users.detail.emailVerified': 'E-mail verificata',
  'users.detail.provisioning': 'Provisioning (SCIM)',
  'users.roleInOrg': 'Ruolo nell’organizzazione',
  'users.accessEnabled': 'Accesso abilitato',
  'users.action.forceLogout': 'Termina sessioni',
  'users.action.resetPassword': 'Reimposta password',
  'users.action.sendResetLink': 'Invia link di reimpostazione',
  'users.action.resetMfa': 'Reimposta MFA',
  'users.resetMfa.warning':
    'Il segreto TOTP e le chiavi di sicurezza di {email} verranno eliminati. L’account resterà senza secondo fattore finché la MFA non verrà riconfigurata. È un’azione distruttiva.',
  'users.resetPw.intro':
    'Imposta una password temporanea e termina le sessioni di {email}. Condividila tramite un canale sicuro.',
  'users.resetPw.new': 'Nuova password',
  'users.resetPw.repeat': 'Ripeti la password',
  'users.resetPw.tooShort': {
    one: 'La password deve contenere almeno {count} carattere.',
    other: 'La password deve contenere almeno {count} caratteri.',
  },
  'users.resetPw.mismatch': 'Le password non corrispondono.',
  'users.remove.confirmQuestion':
    'Rimuovere {email} dall’organizzazione? Le sessioni verranno chiuse e perderà l’accesso; l’account non viene eliminato.',
  'users.remove.action': 'Rimuovi dall’organizzazione',
  'users.inviteIntro':
    'Invieremo a questo indirizzo un collegamento per creare l’account ed entrare nell’organizzazione.',
  'users.inviteEmail': 'Indirizzo e-mail',
  'users.inviteEmailPlaceholder': 'persona@azienda.com',
  // Alta directa con contraseña: la salida cuando el correo no llega.
  'users.invitePassword': 'Password iniziale (facoltativa)',
  'users.invitePasswordPlaceholder': 'Lascialo vuoto per inviare un invito',
  'users.invitePasswordHint':
    'Con la password l’account si crea subito e non parte nessuna email — utile quando la posta non funziona. Lasciandolo vuoto la sceglie quella persona dal link d’invito, che è più sicuro: la password non viaggia.',
  'users.toast.created': 'Account creato',
  'users.toast.createdHint':
    '{email} può entrare adesso con la password che hai impostato. Non è stata inviata nessuna email, quindi dovrai dargliela tu.',

  'users.inviteRoleLabel': 'Ruolo',
  'users.inviteMailbox': 'Indirizzo della casella',
  'users.inviteMailboxPlaceholder': 'nome.cognome',
  'users.inviteMailboxHint': 'Lascia vuoto per usare nome.cognome automaticamente.',
  'users.pending.title': 'Inviti in sospeso',
  'users.pending.hint':
    'Inviati e non ancora accettati. Scadono da soli; revocarli invalida il link subito.',
  'users.pending.expires': 'Scade il {date}',
  'users.pending.revoke': 'Revoca',
  'users.pending.revoked': 'Invito revocato',
  'users.pending.revokedHint': 'Il link inviato a {email} non funziona più.',
  'users.pending.gone': "Quell'invito non c'era già più",
  'users.pending.goneHint': 'O è stato accettato, o era scaduto. La lista è stata aggiornata.',
  'users.pending.revokeFailed': 'Impossibile revocare',
  'users.mailbox.title': 'Indirizzi e-mail',
  'users.mailbox.primary': 'Principale',
  'users.mailbox.none': 'Nessuna casella assegnata per ora.',
  'users.mailbox.addAlias': 'Aggiungi alias',
  'users.mailbox.addPlaceholder': 'alias',
  'users.mailbox.makePrimary': 'Rendi principale',
  'users.mailbox.remove': 'Rimuovi alias',
  'users.mailbox.hint':
    "Gli alias recapitano nella stessa casella. Cambiando l'indirizzo principale, il precedente resta come alias così la posta diretta a quello continua ad arrivare.",
  'users.mailbox.changed': 'Indirizzo principale cambiato',
  'users.mailbox.aliasAdded': 'Alias aggiunto',
  'users.mailbox.aliasRemoved': 'Alias rimosso',
  'users.mailbox.failed': "Impossibile aggiornare l'indirizzo",
  'users.mailbox.taken': 'Quell indirizzo è già in uso. Scegline un altro.',
  'users.mailbox.invalid': 'Quell indirizzo non è valido per questa organizzazione.',
  'users.inviteSubmit': 'Invia invito',
  'users.sessionsRevoked': {
    one: '{count} sessione è stata chiusa.',
    other: '{count} sessioni sono state chiuse.',
  },
  'users.toast.loadMoreFailed': 'Impossibile caricare altro',
  'users.toast.loadMoreFailedHint': 'Riprova tra qualche secondo; la directory resta dov’era.',
  'users.toast.forbiddenUser':
    'Non disponi delle autorizzazioni per questa azione su questo account.',
  'users.toast.suspended': 'Account sospeso',
  'users.toast.suspendedHint': '{email} non può più accedere.',
  'users.toast.reactivated': 'Account riattivato',
  'users.toast.reactivatedHint': '{email} ha di nuovo accesso alla suite.',
  'users.toast.statusFailed': 'Impossibile cambiare lo stato',
  'users.toast.sessionsClosed': 'Sessioni chiuse',
  'users.toast.sessionsClosedHint': {
    one: '{email}: {count} sessione chiusa. Dovrà accedere di nuovo.',
    other: '{email}: {count} sessioni chiuse. Dovrà accedere di nuovo.',
  },
  'users.toast.sessionsFailed': 'Impossibile chiudere le sessioni',
  'users.toast.pwReset': 'Password reimpostata',
  'users.toast.pwResetHint': '{email} dovrà usare la nuova password.',
  'users.toast.pwResetFailed': 'Impossibile reimpostare la password',
  'users.toast.linkSent': 'Collegamento inviato',
  'users.toast.linkSentHint': '{email} riceverà un collegamento per scegliere una nuova password.',
  'users.toast.linkFailed': 'Impossibile inviare il collegamento',
  'users.toast.ssoOnly': 'L’account non usa una password (accesso tramite SSO).',
  'users.toast.mfaReset': 'MFA reimpostata',
  'users.toast.mfaResetHint': '{email} dovrà riconfigurare la 2FA al prossimo accesso.',
  'users.toast.mfaResetFailed': 'Impossibile reimpostare la MFA',
  'users.toast.notInOrg': 'Questo account non appartiene a questa organizzazione.',
  'users.toast.roleUpdated': 'Ruolo aggiornato',
  'users.toast.roleUpdatedHint': '{email} ha ora il ruolo {role}.',
  'users.toast.roleFailed': 'Impossibile cambiare il ruolo',
  'users.toast.lastAdmin': 'Non puoi lasciare l’organizzazione senza nessuno che la amministri.',
  'users.toast.removed': 'Account rimosso',
  'users.toast.removedHint': '{email} non appartiene più all’organizzazione.',
  'users.toast.removeFailed': 'Impossibile rimuovere l’account',
  'users.toast.removeLastAdmin':
    'Non puoi rimuovere l’ultima persona che amministra l’organizzazione.',
  'users.toast.invited': 'Invito inviato',
  'users.toast.invitedHint': '{email} riceverà un collegamento per unirsi con il ruolo {role}.',
  'users.toast.inviteFailed': 'Impossibile invitare',
  'users.toast.alreadyMember': 'Questo indirizzo fa già parte dell’organizzazione.',
  'users.toast.roleForbidden': 'Non disponi delle autorizzazioni per concedere quel ruolo.',
  'users.toast.checkEmail': 'Controlla l’indirizzo e riprova.',
  'users.toast.inviteNotSent': 'Invito creato, ma l’email non è partita',
  'users.toast.inviteNotSentHint':
    'L’indirizzo va bene: a fallire è stato il nostro sistema di posta. L’invito è già nell’elenco — invita di nuovo quando la posta funzioni e sostituirà questo.',

  // --- Organizaciones (tenants) -------------------------------------------
  'tenants.subtitle':
    'Tenant di Sytue ID. Una creazione effettua il provisioning dell’intera suite (SCIM).',
  'tenants.searchPlaceholder': 'Cerca organizzazione',
  'tenants.statusActive': 'attiva',
  'tenants.statusSuspended': 'sospesa',
  'tenants.mailDomain': 'Dominio di posta',
  'tenants.region': 'Area geografica',
  'tenants.col.plan': 'Piano',
  'tenants.col.namespace': 'Spazio dei nomi',
  'tenants.settings': 'Impostazioni',
  'tenants.settingsFor': 'Impostazioni di {name}',
  'tenants.settingsOf': 'Impostazioni · {name}',
  'tenants.empty.title': 'Ancora nessuna organizzazione',
  'tenants.empty.description':
    'Crea la prima: effettua il provisioning del gruppo, dello spazio dei nomi e dell’intera suite tramite SCIM.',
  'tenants.noResults': 'Nessun risultato per «{query}»',
  'tenants.noResultsHint': 'Prova con un altro nome o slug, oppure cancella la ricerca.',
  'tenants.clearSearch': 'Cancella ricerca',
  'tenants.form.name': 'Nome',
  'tenants.form.namePlaceholder': 'ACME Europa',
  'tenants.form.slug': 'Slug (org/<slug>)',
  'tenants.form.slugHelp':
    'Identificatore in minuscolo; definisce gruppo e spazio dei nomi. Non è modificabile in seguito.',
  'tenants.form.slugImmutable':
    'Lo slug {slug} è l’identità dell’organizzazione e non può essere modificato.',
  'tenants.detail.usersTitle': 'Persone dell’organizzazione',
  'tenants.detail.usersCaption': 'Persone di {name}',
  'tenants.detail.usersEmptyTitle': 'Ancora nessun utente',
  'tenants.detail.usersEmptyDescription':
    'Gli account di questa organizzazione compariranno qui una volta effettuato il provisioning tramite SCIM.',
  'tenants.detail.groupsEmptyTitle': 'Ancora nessun gruppo',
  'tenants.detail.groupsEmptyDescription':
    'Il gruppo org/<slug> e i suoi sottogruppi compariranno qui una volta creati.',
  'tenants.detail.workspace': 'workspace: {name}',
  'tenants.toast.loadMoreFailedHint': 'Riprova tra qualche secondo; l’elenco resta dov’era.',
  'tenants.toast.created': 'Organizzazione creata',
  'tenants.toast.createdHint': 'org/{slug} · {namespace}. La proprietà è tua.',
  'tenants.toast.createFailed': 'Impossibile creare l’organizzazione',
  'tenants.toast.slugTaken': 'Questo slug è già in uso; scegline un altro.',
  'tenants.toast.invalidSlug': 'Controlla lo slug (minuscole, cifre e trattini) e il nome.',
  'tenants.toast.settingsSaved': 'Impostazioni salvate',
  'tenants.toast.settingsFailed': 'Impossibile salvare le impostazioni',
  'tenants.toast.noAccess': 'Non disponi di autorizzazioni su questa organizzazione.',
  // Multi-pertenencia: añadir a alguien que YA tiene cuenta a otra organización.
  'tenants.member.action': 'Aggiungi persona',
  'tenants.member.actionFor': 'Aggiungi una persona a {name}',
  'tenants.member.title': 'Aggiungi una persona a {name}',
  'tenants.member.person': 'Persona',
  'tenants.member.personPlaceholder': 'Nome o indirizzo e-mail',
  'tenants.member.personHelp':
    'Deve avere già un account Sytue: questo la aggiunge, non lo crea. Se non compare, scrivi il suo indirizzo e-mail completo.',
  'tenants.member.noCandidates':
    'Nessuno in questa rubrica corrisponde. Scrivi l’indirizzo e-mail completo.',
  'tenants.member.role': 'Ruolo in questa organizzazione',
  'tenants.member.roleHelp':
    'Vale solo qui: ciò che è nelle sue altre organizzazioni resta invariato.',
  'tenants.member.roleHelpAdmin':
    'Vale solo qui. Concedere il ruolo Proprietario è riservato a un account con la proprietà di {name}.',
  'tenants.member.submit': 'Aggiungi',
  'tenants.member.added': 'Persona aggiunta',
  'tenants.member.addedHint': '{email} appartiene ora anche a {org}.',
  'tenants.member.failed': 'Impossibile aggiungere la persona',
  'tenants.member.noAccount': 'Nessun account Sytue usa {email}. Inviale invece un invito.',

  // --- Auditoría unificada -------------------------------------------------
  'audit.title': 'Controllo unificato',
  'audit.subtitle':
    'Traccia di tutti i servizi (Identità, Posta, File, Integrazioni) in un’unica vista. Esportazione CSV/JSON normalizzata per SIEM.',
  'audit.source.all': 'Tutti i servizi',
  'audit.source.id': 'Identità',
  'audit.source.mail': 'Posta',
  'audit.source.drive': 'File',
  'audit.source.notes': 'Note',
  'audit.source.integrations': 'Integrazioni',
  'audit.filterService': 'Filtra per servizio',
  'audit.filterAction': 'Filtra per azione',
  'audit.filterActor': 'Filtra per attore o destinazione',
  'audit.actionPlaceholder': 'Azione (ad es. policy., ediscovery.)',
  'audit.actorPlaceholder': 'Attore o destinazione',
  'audit.since': 'Dal',
  'audit.until': 'Al',
  'audit.apply': 'Applica',
  'audit.applying': 'Applicazione…',
  'audit.allSourcesDown':
    'Nessuna fonte di controllo ha risposto: non c’è connessione con i servizi.',
  'audit.someSourcesDown': 'Nessuna risposta da: {sources}. La vista può essere incompleta.',
  'audit.loadFailed': 'Impossibile caricare il controllo. Modifica i filtri e riprova.',
  'audit.col.date': 'Data',
  'audit.col.service': 'Servizio',
  'audit.col.action': 'Azione',
  'audit.col.actor': 'Attore',
  'audit.col.target': 'Destinazione',
  'audit.empty.title': 'Nessun evento da mostrare',
  'audit.empty.description':
    'Non ci sono record di controllo per questo filtro. Amplia l’intervallo di date o cancella i filtri.',
  'audit.listLabel': 'Record di controllo unificato',
  'groups.memberCount': { one: '{count} persona', other: '{count} persone' },

  // --- Dispositivi ----------------------------------------------------------
  'devices.subtitle':
    'Dispositivi da cui si accede. Contrassegna come attendibile quello che stai usando per soddisfare le regole di accesso condizionale che richiedono un dispositivo gestito.',
  'devices.offline':
    'Nessuna connessione a Sytue ID: non è possibile caricare né gestire i dispositivi.',
  'devices.trusted': 'Attendibile',
  'devices.untrusted': 'Non attendibile',
  'devices.thisDevice': 'Questo dispositivo',
  'devices.trust': 'Rendi attendibile',
  'devices.revoke': 'Revoca',
  'devices.revokeAria': 'Revoca {name}',
  'devices.revokeOfAria': 'Revoca il dispositivo di {user}',
  'devices.revokeTitle': 'Revoca dispositivo',
  'devices.revokeConfirm':
    '{name} verrà revocato. Non sarà più riconosciuto e perderà l’attendibilità; al prossimo accesso verrà registrato di nuovo come sconosciuto.',
  'devices.lastActivity': 'ultima attività {when}',
  'devices.fallbackUser': 'Persona',
  'devices.mine.title': 'I miei dispositivi',
  'devices.mine.emptyTitle': 'Nessun dispositivo registrato',
  'devices.mine.emptyDescription':
    'Quando accedi da un browser comparirà qui, così potrai contrassegnarlo come attendibile.',
  'devices.org.title': 'Dispositivi dell’organizzazione',
  'devices.org.emptyTitle': 'Nessun dispositivo nell’organizzazione',
  'devices.org.emptyDescription':
    'I dispositivi compaiono qui man mano che le persone effettuano l’accesso.',
  'devices.col.userDevice': 'Persona · Dispositivo',
  'devices.col.trust': 'Attendibilità',
  'devices.col.manage': 'Gestisci',
  'devices.toast.trusted': 'Dispositivo attendibile',
  'devices.toast.trustFailed': 'Impossibile contrassegnare come attendibile',
  'devices.toast.trustOnlyCurrent':
    'Puoi contrassegnare come attendibile solo il dispositivo da cui stai accedendo ora.',
  'devices.toast.revoked': 'Dispositivo revocato',
  'devices.toast.revokeFailed': 'Impossibile revocare',

  // --- Etichette di riservatezza -------------------------------------------
  'labels.description':
    'Contrassegna i file in base a quanto sono delicati — pubblico, interno, riservato — e decidi quali non possono uscire dall’organizzazione. Chi lavora sul file vede l’etichetta; Sytue si assicura che non venga condiviso all’esterno se l’hai stabilito.',
  'labels.new': 'Nuova etichetta',
  'common.offlineIdTitle': 'Non riusciamo a contattare Sytue ID',
  'labels.offlineBody':
    'Al momento le etichette non si possono caricare né modificare. Riprova tra qualche minuto.',
  'labels.scale.title': 'La tua scala',
  'labels.scale.description':
    'Dalla meno alla più riservata. L’ordine lo decidi tu: serve perché tutti capiscano a colpo d’occhio cosa è più delicato di cosa.',
  'labels.empty.title': 'Ancora nessuna etichetta',
  'labels.empty.description':
    'Senza etichette tutti i file vengono trattati allo stesso modo. La scala con cui quasi tutti iniziano: Pubblico, Interno e Riservato — e solo l’ultima impedisce la condivisione esterna.',
  'labels.empty.action': 'Crea la prima',
  'labels.noDescription': 'Senza spiegazione',
  'labels.blocksExternal': 'Non esce dall’organizzazione',
  'labels.informational': 'Solo informativa',
  'common.editName': 'Modifica {name}',
  'common.deleteName': 'Elimina {name}',
  'labels.manualNote':
    'L’etichettatura è manuale: ogni persona sceglie l’etichetta mentre lavora sul file. Sytue non classifica nulla da solo.',
  'labels.wizard.editTitle': 'Modifica «{name}»',
  'labels.wizard.description':
    'Tre domande e un riepilogo: come si chiama, quanto è riservata e cosa deve impedire.',
  'labels.delete.title': 'Elimina l’etichetta',
  'labels.delete.intro': '{name} non potrà più essere scelta durante la classificazione dei file.',
  'labels.delete.keep':
    'I file che la portano già non cambiano: conservano l’etichetta che è stata loro assegnata. È voluto: eliminare un’etichetta non deve declassificare ciò che era già classificato.',
  'labels.delete.keepBlocking':
    'I file che la portano già non cambiano: conservano l’etichetta che è stata loro assegnata e continueranno a non poter essere condivisi fuori dall’organizzazione. È voluto: eliminare un’etichetta non deve declassificare ciò che era già classificato.',
  'labels.toast.deleted': 'Etichetta eliminata',
  'labels.toast.deleteFailed': 'Impossibile eliminare',

  // --- Accesso condizionale -------------------------------------------------
  'access.title': 'Regole di accesso',
  'access.subtitle':
    'Decidi cosa serve per accedere a Sytue a seconda della situazione: da dove ci si collega, con quale dispositivo e a che ora. Senza regole, tutti accedono come sempre.',
  'access.new': 'Nuova regola',
  'access.offlineBody':
    'Al momento né le regole né il loro stato si possono caricare o modificare. Riprova tra qualche minuto.',
  'access.rules.title': 'Le tue regole',
  'access.rules.description':
    'Vengono valutate dall’alto verso il basso a ogni accesso. Se ne corrispondono più di una, vince la più severa: bloccare batte chiedere una verifica, e chiedere una verifica batte lasciar passare.',
  'access.empty.title': 'Non c’è ancora nessuna regola',
  'access.empty.description':
    'Senza regole, chiunque con la propria password accede da qualsiasi posto. La prima che quasi tutti creano: chiedere una verifica in più a chi si collega da fuori ufficio.',
  'access.empty.action': 'Crea la prima',
  'access.action.block': 'Blocca l’accesso',
  'access.action.mfa': 'Chiede una verifica',
  'access.action.allow': 'Lascia passare',
  'access.applies.everyone': 'A tutti',
  'access.applies.groups': { one: '{count} gruppo', other: '{count} gruppi' },
  'access.applies.users': { one: '{count} persona', other: '{count} persone' },
  'access.applies.to': 'A {what}',
  'access.cond.fromNetworks': 'dalle tue reti',
  'access.cond.outsideNetworks': 'fuori dalle tue reti',
  'access.cond.untrustedDevices': 'su dispositivi non attendibili',
  'access.cond.apps': { one: 'in {count} app', other: 'in {count} app' },
  'access.cond.schedule': 'in certi orari',
  'access.cond.always': 'sempre',
  'access.cond.exempt': { one: '{count} esente', other: '{count} esenti' },
  'access.moveUp': 'Sposta {name} in su',
  'access.moveDown': 'Sposta {name} in giù',
  'access.on': 'Attiva',
  'access.off': 'Disattivata',
  'common.switchAria': '{state}: {name}',
  'access.tip':
    'Consiglio: lascia sempre esente il tuo gruppo di amministrazione in almeno una regola. È la tua uscita di emergenza se una di esse ti chiude fuori.',
  'access.delete.title': 'Elimina la regola',
  'access.delete.body':
    '{name} non verrà più valutata all’accesso. Chi ne era interessato accederà come tutti gli altri. Non si può annullare, ma puoi ricrearla.',
  'access.wizard.editTitle': 'Modifica «{name}»',
  'access.wizard.newTitle': 'Nuova regola di accesso',
  'access.wizard.description':
    'Tre domande e un riepilogo. Prima di salvare ti diciamo, in una frase, chi riguarda e chi resterebbe fuori.',
  'common.toggleFailed': 'Non è stato possibile modificarla',
  'common.toggleFailedHint': '{name} resta com’era.',
  'access.toast.reorderFailed': 'Non è stato possibile cambiare l’ordine',
  'access.toast.deleted': 'Regola eliminata',
  'access.toast.deleteFailed': 'Impossibile eliminare',

  // --- Importa contenuti (migrazioni) ---------------------------------------
  'migrations.subtitle':
    'Passa da Microsoft 365 o Google Workspace senza che nessuno perda i propri file o i propri contatti: ogni persona ritrova i suoi nel proprio account Sytue.',
  'migrations.outStart': 'Portare via i miei contenuti',
  'migrations.out.title': 'Portare via i tuoi contenuti',
  'migrations.out.description':
    'Copia i file e i contatti delle persone che scegli in un account Google Workspace o Microsoft 365. Da Sytue non si cancella nulla: è una copia, e puoi fermarla quando vuoi.',
  'wizard.migrate.step.destination': 'Destinazione',
  'wizard.migrate.out.heading': 'In quale account li porti?',
  'wizard.migrate.out.description':
    'Scegli la connessione di destinazione. Si scrive con il permesso che quell’account ha autorizzato, e solo in una cartella creata da Sytue.',
  'wizard.migrate.out.noticeTitle': 'I tuoi dati sono tuoi',
  'wizard.migrate.out.noticeBody':
    'Questo COPIA sull’account di destinazione: da Sytue non si cancella nulla. I file arrivano in una cartella «Sytue» del Drive o del OneDrive di destinazione — non si mescolano con quello che c’è già — e i contatti si aggiungono alla sua rubrica. Se il lavoro si interrompe, riprende da dove era e non duplica nulla.',
  'wizard.migrate.out.authTitle': 'Prima, lascia che Sytue scriva su {provider}',
  'wizard.migrate.out.authBody':
    'La connessione che serve per importare contenuti può solo leggere. Per portarli fuori, {provider} ti chiederà il permesso di creare file, contatti, posta e calendari nell’account. Se l’hai già concesso, vai avanti.',
  'wizard.migrate.out.authAction': 'Autorizza la scrittura',
  'wizard.migrate.out.authFailed':
    'Non è stato possibile aprire l’autorizzazione. Riprova tra qualche secondo.',
  'wizard.migrate.out.whatHeading': 'Che cosa porti via?',
  'wizard.migrate.out.whoHeading': 'Di chi sono i contenuti che escono?',
  'wizard.migrate.out.whoDescription':
    'Di ogni persona che scegli si copia il suo contenuto nell’account di destinazione. Puoi sceglierne una o più.',
  'migrations.self.description':
    'Copia i tuoi file, i tuoi contatti, la tua posta e il tuo calendario in un account Google Workspace o Microsoft 365. Da Sytue non si cancella nulla: è una copia, e puoi fermarla quando vuoi.',
  'wizard.migrate.self.description':
    'Scegli la connessione di destinazione. Si scriverà con il permesso che quell’account ha concesso.',
  'wizard.migrate.self.others': 'Portare fuori i contenuti di altre persone',
  'wizard.migrate.self.noticeBody':
    'Questo COPIA sull’account di destinazione: da Sytue non si cancella nulla. I file arrivano in una cartella «Sytue», la posta nella cartella che le corrisponde (in arrivo, inviata…) e ogni calendario arriva come uno nuovo. Se la copia si interrompe, rilanciala: riprende da dove era e non duplica nulla.',
  'wizard.migrate.self.gets.mail': 'La posta con le sue cartelle, le date e gli allegati.',
  'wizard.migrate.self.gets.calendar': 'I calendari e i loro eventi, con ripetizioni ed eccezioni.',
  'wizard.migrate.self.nots.outlookDrafts':
    'In Outlook, un messaggio con più di 3 MB di allegati arriva segnato come bozza. Arriva intero: è l’unico modo in cui Microsoft accetta allegati grandi.',
  'wizard.migrate.self.nots.invites':
    'Gli inviti: nessuno riceve un avviso. In Outlook, i partecipanti restano annotati nella descrizione dell’evento.',
  'wizard.migrate.self.nots.outlookRepeats':
    'In Outlook, gli eventi annullati e le ripetizioni che non sa rappresentare. Si omettono uno per uno e ti diciamo quali.',
  'wizard.migrate.self.safeBody':
    'Da Sytue leggiamo soltanto: qui non cambia nulla. Se rilanci la copia, porta le novità e non duplica quello che è già arrivato.',
  'wizard.migrate.self.launch': 'Avvia la copia',
  'wizard.migrate.self.reviewTitle': 'Si copia con la tua sessione',
  'wizard.migrate.self.reviewBody':
    'Sytue legge i tuoi contenuti come quando scarichi «I tuoi dati» e li scrive in {provider} con il permesso che hai concesso. Puoi chiudere questa pagina: la copia continua in background.',
  'wizard.migrate.self.conflict':
    'La copia è già in corso. Se sembra bloccata, fermala e rilanciala: riprende da dove si era fermata.',
  'common.offlineGatewayTitle': 'Non riusciamo a contattare il servizio di integrazioni',
  'migrations.offlineBody':
    'Al momento le importazioni non si possono consultare né avviare. Riprova tra qualche minuto.',
  'migrations.domain.drive': 'File',
  'migrations.domain.contacts': 'Contatti',
  // --- Assistente automatico (AutoMigrateWizard) ----------------------------
  'migrations.domain.repos': 'Repository',
  'migrations.domain.gists': 'Gist',
  'migrations.domain.teams': 'Team e canali',
  'migrations.domain.team_files': 'File e cronologia dei canali',
  'migrations.domain.mail': 'Posta',
  'migrations.domain.calendar': 'Calendario',
  'migrations.domain.cloud': 'Applicazioni (Cloud e Firebase)',
  'wizard.auto.source.microsoftDesc':
    'Il tuo OneDrive e i tuoi contatti, e il tuo Microsoft Teams: team, canali e le persone che ne fanno parte arrivano in Sytue Rooms, mentre i file e la cronologia delle conversazioni di ogni canale arrivano in Sytue Drive.',
  'wizard.auto.item.teams': 'Team e canali',
  'wizard.auto.item.teamFiles': 'File e cronologia dei canali',
  'wizard.auto.nots.teamsHistory':
    'I messaggi di Teams non vengono ripubblicati in Rooms. Rooms pubblica un messaggio come chi lo scrive adesso, quindi versarli lì falsificherebbe la firma o la data. La cronologia di ogni canale viaggia come trascrizione fedele in Drive, con il nome e la data reali all’interno.',
  'wizard.auto.nots.teamsGuests':
    'Gli ospiti di altre organizzazioni e i canali condivisi con loro: qui non hanno un account, quindi restano fuori invece di inventargliene uno.',
  'wizard.auto.scope.orgHintMicrosoft':
    'Un accesso apre solo i tuoi contenuti. Per tutta l’azienda basta che l’amministrazione approvi una sola schermata: niente account di servizio e niente da incollare. Ti portiamo direttamente lì.',
  'wizard.auto.delegation.microsoftBody':
    'Apri con un account di amministrazione la schermata Microsoft che colleghiamo qui sotto e approvala. È tutto: la stessa applicazione passa a leggere l’organizzazione, con gli stessi permessi di sola lettura.',
  'wizard.auto.delegation.whyMicrosoft':
    'Approvarlo non dà a Sytue nulla di nuovo: sono gli stessi permessi di lettura, ora per l’organizzazione invece che solo per il tuo account. L’amministrazione può revocarli in Entra quando vuole.',
  'migrations.autoStart': 'Portare tutto',
  'migrations.manualStart': 'Scegliere a mano',
  'migrations.auto.title': 'Porta tutto con un solo accesso',
  'migrations.auto.description':
    'Accedi al servizio che stai lasciando e portiamo tutto ciò che quell’account raggiunge. Vedi il piano prima che venga copiato qualsiasi cosa.',
  'wizard.auto.connected': 'Collegato',
  'wizard.auto.source.googleDesc':
    'I tuoi file e contatti, la tua Gmail, il tuo calendario e le tue applicazioni Google Cloud e Firebase. Accedi una volta e li leggiamo; su Google non cambia nulla.',
  'wizard.auto.source.githubDesc':
    'I tuoi repository arrivano in Sytue Git con la loro cronologia, i rami, i tag, le issue, le pull request, il wiki e le release. I tuoi gist arrivano in Sytue Drive.',
  'wizard.auto.item.drive': 'File',
  'wizard.auto.item.contacts': 'Contatti',
  'wizard.auto.item.repos': 'Repository',
  'wizard.auto.item.gists': 'Gist',
  'wizard.auto.item.mail': 'Posta',
  'wizard.auto.item.calendar': 'Calendario',
  'wizard.auto.item.cloud': 'Applicazioni Google Cloud e Firebase',
  'wizard.auto.nots.cloudDatabases':
    'I database (Cloud SQL, Firestore, BigQuery). Non si copiano con un’API di controllo: vanno esportati e ripristinati, e sei tu a decidere quando fermare le scritture. Te li elenchiamo uno per uno con quello che c’è da fare.',
  'wizard.auto.nots.cloudImages':
    'Le immagini dei container che girano già su Google. Sytue DevX costruisce dal tuo repository: collegalo al progetto e il primo rilascio le sostituisce.',
  'wizard.auto.nots.actions':
    'Le esecuzioni di GitHub Actions e i loro registri. I file dei workflow viaggiano con il codice; lo storico delle esecuzioni resta lì.',
  'wizard.auto.nots.forgeAccount':
    'Chi non è mai entrato in Sytue Git. I suoi repository aspettano che entri una volta; poi riprendi il lavoro.',
  'wizard.auto.nots.settings':
    'Le impostazioni che esistono solo su GitHub: protezione dei rami, segreti, applicazioni e webhook.',
  'wizard.auto.step.origin': 'Da dove',
  'wizard.auto.step.connect': 'Accedere',
  'wizard.auto.step.review': 'Cosa portiamo',
  'wizard.auto.origin.heading': 'Quale servizio stai lasciando?',
  'wizard.auto.origin.description': 'Scegli quale. L’unica cosa che ti chiederemo è di accedervi.',
  'wizard.auto.connect.heading': 'Accedi a {source}',
  'wizard.auto.connect.description':
    'Approvi i permessi sulla loro schermata. Noi leggiamo soltanto: nel servizio che stai lasciando non scriviamo mai.',
  'wizard.auto.connect.action': 'Accedi a {source}',
  'wizard.auto.connect.readOnly':
    'Accesso in sola lettura. Puoi revocarlo quando vuoi, da qui o dal fornitore stesso.',
  'wizard.auto.connect.readOnlyGithub':
    'GitHub non ha un permesso di sola lettura per i repository privati: quello che offre (repo) consente anche di scrivere. Sytue si limita a leggere, e il permesso puoi revocarlo quando vuoi da GitHub.',
  'wizard.auto.connect.checking': 'Stiamo verificando cosa c’è…',
  'wizard.auto.connect.doneTitle': 'Accesso effettuato',
  'wizard.auto.connect.doneBody': 'Siamo collegati a {source} come {account}.',
  'wizard.auto.scope.mine': 'Solo il mio account',
  'wizard.auto.scope.mineHint':
    'Tutto ciò che possiede l’account con cui accedi. Un solo accesso e nulla più: è il caso abituale, e ogni persona può fare lo stesso con le proprie cose.',
  'wizard.auto.scope.oneClick': 'Un solo accesso',
  'wizard.auto.scope.org': 'Tutta l’organizzazione',
  'wizard.auto.scope.orgHint':
    'Google non permette che un accesso legga i contenuti di altre persone, quindi per tutta l’azienda chiede un permesso in più. Ti portiamo direttamente alla schermata dove si concede.',
  'wizard.auto.delegation.title': 'Un permesso in più per tutta l’organizzazione',
  'wizard.auto.delegation.step1':
    'Nella console di Google Cloud, crea un account di servizio e scarica la sua chiave JSON.',
  'wizard.auto.delegation.step2': 'Incolla qui quel JSON e salvalo.',
  'wizard.auto.delegation.step3':
    'Apri la schermata di Google che colleghiamo qui sotto: il client e i permessi sono già compilati, devi solo autorizzare.',
  'wizard.auto.delegation.keyLabel': 'Chiave JSON dell’account di servizio',
  'wizard.auto.delegation.save': 'Salva la chiave',
  'wizard.auto.delegation.open': 'Apri la schermata di Google',
  'wizard.auto.delegation.recheck': 'Controlla di nuovo',
  'wizard.auto.delegation.why':
    'La chiave viene conservata cifrata e serve solo a leggere, per conto di ogni persona, ciò che le appartiene.',
  'wizard.auto.delegation.readyTitle': 'Permesso concesso',
  'wizard.auto.delegation.readyBody': {
    one: 'Ora possiamo portare i contenuti di {count} persona.',
    other: 'Ora possiamo portare i contenuti di {count} persone.',
  },
  'wizard.auto.review.heading': 'Questo è ciò che portiamo',
  'wizard.auto.review.description':
    'Non è stato ancora copiato nulla. Una volta avviata, la copia va in secondo piano e puoi chiudere questa pagina.',
  'wizard.auto.review.account': 'Accesso effettuato come',
  'wizard.auto.review.who': 'Per chi',
  'wizard.auto.review.people': {
    one: '{count} persona, nel proprio account',
    other: '{count} persone, ognuna nel proprio account',
  },
  'wizard.auto.review.what': 'Cosa si porta',
  'wizard.auto.review.jobs': 'Lavori che partono',
  'wizard.auto.review.jobsValue': {
    one: '{count} (uno per persona e tipo di contenuto)',
    other: '{count} (uno per persona e tipo di contenuto)',
  },
  'wizard.auto.review.launch': 'Portare tutto',
  'wizard.auto.unmatched.title': 'Account che non sappiamo abbinare',
  'wizard.auto.unmatched.body': {
    one: '{count} account del vecchio servizio non ha un equivalente in Sytue, quindi lo lasciamo fuori invece di copiarlo nella casella sbagliata. Crea quella persona nella rubrica aziendale e rilancia.',
    other:
      '{count} account del vecchio servizio non hanno un equivalente in Sytue, quindi li lasciamo fuori invece di copiarli nella casella sbagliata. Crea quelle persone nella rubrica aziendale e rilancia.',
  },
  'wizard.auto.limits.title': 'Cosa non arriva',
  'wizard.auto.safe.title': 'All’origine non si perde nulla',
  'wizard.auto.safe.body':
    'Leggiamo soltanto. Il servizio che stai lasciando resta esattamente com’è, e puoi rilanciare: ciò che è già stato copiato non viene duplicato.',
  'wizard.auto.error.title': 'Non ha funzionato',
  'wizard.auto.error.plan':
    'Non siamo riusciti a verificare cosa c’è. Forse l’accesso è scaduto: accedi di nuovo.',
  'wizard.auto.error.connect':
    'Non siamo riusciti ad avviare l’accesso. Riprova tra qualche secondo.',
  'wizard.auto.error.noProviderApp':
    'Sytue non è ancora registrata come applicazione presso {source}, quindi non c’è nessuna schermata di accesso a cui mandarti. La registra un operatore in Amministrazione della piattaforma → Connessioni con terze parti; fino ad allora riprovare non cambia nulla.',
  'wizard.auto.error.noKek':
    'Questo deployment non ha una chiave di cifratura configurata, quindi non può conservare ciò che {source} restituirebbe. Deve configurarla un operatore.',
  'wizard.auto.error.serviceKey':
    'Non siamo riusciti a salvare quella chiave. Controlla che sia il JSON completo dell’account di servizio.',
  'wizard.auto.error.launch':
    'Non siamo riusciti ad avviare la migrazione. Riprova tra qualche secondo.',
  'migrations.status.done': 'Completata',
  'migrations.status.running': 'Copia in corso',
  'migrations.status.queued': 'In coda',
  'migrations.status.canceled': 'Interrotta',
  'migrations.status.error': 'Da verificare',
  'migrations.counts.migrated': { one: '{count} copiato', other: '{count} copiati' },
  'migrations.counts.skipped': { one: '{count} ignorato', other: '{count} ignorati' },
  'migrations.counts.errors': { one: '{count} con errore', other: '{count} con errore' },
  'migrations.from': 'Da {user}',
  'migrations.viewReport': 'Vedi il rapporto',
  'migrations.stop': 'Interrompi',
  'migrations.resume': 'Riprendi',
  'migrations.wizard.title': 'Importa contenuti da Microsoft 365 o Google',
  'migrations.wizard.description':
    'Ti accompagniamo passo dopo passo. Niente viene copiato prima dell’ultimo passaggio.',
  'migrations.toast.launched': 'Importazione avviata',
  'migrations.toast.launchedHint': {
    one: '{count} importazione è in copia in background. Puoi chiudere questa pagina.',
    other: '{count} importazioni sono in copia in background. Puoi chiudere questa pagina.',
  },
  'migrations.toast.stopped': 'Importazione interrotta',
  'migrations.toast.stoppedHint':
    'Quanto già copiato resta al suo posto. Puoi riprendere quando vuoi.',
  'migrations.toast.resumed': 'Importazione ripresa',
  'migrations.toast.resumedHint':
    'Riparte da dove si era fermata; ciò che era già stato copiato non viene duplicato.',
  'migrations.toast.actionFailed': 'Operazione non riuscita',
  'migrations.toast.reportFailed': 'Non è stato possibile aprire il rapporto',
  'migrations.active.title': 'In corso',
  'migrations.active.description':
    'I contatori salgono man mano che si copia. Puoi chiudere questa pagina: continua in background.',
  'migrations.finished.title': 'Completate',
  'migrations.list.title': 'Importazioni',
  'migrations.list.description':
    'Ogni persona ha la propria importazione, con il rapporto di ciò che è arrivato e di ciò che non è arrivato.',
  'migrations.empty.titleActive': 'Non ne è ancora terminata nessuna',
  'migrations.empty.title': 'Non hai ancora importato nulla',
  'migrations.empty.description':
    'Quando avvii la prima importazione, qui vedrai cosa è arrivato in ogni account e, con la stessa chiarezza, cosa non è stato possibile copiare e perché.',
  'migrations.empty.action': 'Importa la prima',
  'migrations.report.title': 'Rapporto dell’importazione',
  'migrations.report.migrated': 'copiati nel suo account',
  'migrations.report.skipped': 'ignorati (con motivo)',
  'migrations.report.content': 'di contenuti',
  'migrations.report.whatFailed': 'Cosa non ha funzionato',
  'migrations.report.nothingLostTitle': 'Non è rimasto indietro nulla',
  'migrations.report.nothingLostBody': 'Tutto ciò che era nell’origine è arrivato nel suo account.',
  'migrations.report.skippedTitle': 'Cosa NON è stato copiato, e perché',

  // --- Integrazioni ---------------------------------------------------------
  'apps.subtitle':
    'Collega le applicazioni che il tuo team già usa, così vi si accede con l’account Sytue e gli ingressi e le uscite si gestiscono da soli.',
  'apps.connect': 'Collega un’applicazione',

  // Registro dinámico de apps (RFC 7591), que no tenía pantalla.
  'apps.dcr.action': 'Token di registrazione',
  'apps.dcr.title': 'Lasciare che un’app si registri da sola',
  'apps.dcr.onceTitle': 'Copialo adesso',
  'apps.dcr.onceBody':
    'Questo token si mostra una volta sola. Sytue ID conserva solo il suo hash: chiudere questa finestra senza copiarlo significa chiederne un altro.',
  'apps.dcr.token': 'Token di accesso iniziale',
  'apps.dcr.endpoint': 'Endpoint di registrazione',
  'apps.dcr.expires': 'Vale fino al {when}',
  'apps.dcr.copy': 'Copiare il token',
  'apps.dcr.copied': 'Copiato.',
  'apps.dcr.failed': 'Non è stato possibile emettere il token',
  'apps.connectOne': 'Collega',
  'apps.wizard.description':
    'Ti accompagniamo passo dopo passo. Puoi tornare indietro in qualsiasi momento; niente viene applicato prima dell’ultimo passaggio.',
  'apps.offlineBody':
    'Al momento né il catalogo né le connessioni si possono caricare o modificare. Riprova tra qualche minuto.',
  'apps.status.active': 'Collegata',
  'apps.status.error': 'In errore',
  'apps.status.disabled': 'Disattivata',
  'apps.status.draft': 'Incompleta',
  'apps.cap.sso': 'l’accesso unico',
  'apps.cap.provisioning': 'ingressi e uscite automatici',
  'apps.cap.data': 'i dati per i report',
  'apps.cap.line': 'Offre {what}.',
  'apps.cap.none': 'Accesso dal portale applicazioni del tuo team.',
  'apps.builtin.title': 'Integrazioni incluse',
  'apps.builtin.description':
    'Ogni persona collega i propri cloud (Google Drive, OneDrive, Dropbox, Box), le chat (Slack, Teams, Google Chat) e le videochiamate dall’app stessa, dando solo il consenso. Qui puoi solo disattivarle per tutta l’organizzazione.',
  'apps.builtin.failed': 'Impossibile salvare la modifica. Riprova.',
  'apps.map.title': 'Chi serve ogni prodotto',
  'apps.map.description':
    'Trascina un’applicazione collegata su un prodotto, oppure clicca il prodotto per scegliere. La modifica vale per tutta l’organizzazione.',
  'apps.connected.title': 'Applicazioni collegate',
  'apps.connected.description': 'Quelle che già funzionano con l’account Sytue del tuo team.',
  'apps.empty.title': 'Non hai ancora collegato nessuna applicazione',
  'apps.empty.description':
    'Quando colleghi la prima, il tuo team vi accederà con l’account Sytue e non dovrai creare nessuno a mano.',
  'apps.empty.action': 'Collega la prima',
  'apps.configured': 'Connessione configurata.',
  'apps.runs.none': 'Non c’è ancora nessuna sincronizzazione registrata.',
  'apps.runs.last':
    'Ultima sincronizzazione: {created} creati, {updated} modificati, {deactivated} rimossi',
  'apps.runs.errors': { one: '{count} con errore', other: '{count} con errore' },
  'apps.open': 'Apri',
  'apps.test': 'Prova',
  'apps.sync': 'Sincronizza',
  'apps.syncStatus': 'Stato',
  'apps.deactivate': 'Disattiva',
  'apps.delete': 'Elimina',
  'apps.catalog.title': 'App disponibili',
  'apps.catalog.description':
    'Scegline una e ti guidiamo noi: in cinque passaggi è pronta, senza leggere documentazione.',
  'apps.catalog.emptyTitle': 'Il catalogo non è disponibile',
  'apps.catalog.emptyDescription':
    'Non appena il servizio di integrazioni risponde, vedrai qui tutte le applicazioni che puoi connettere.',
  'apps.catalog.allConnected': 'Hai già connesso tutte le applicazioni del catalogo.',
  'apps.deactivate.title': 'Disattiva l’integrazione',
  'apps.deactivate.body':
    '{name} non apparirà più nel portale e l’accesso con Sytue verrà revocato. Non viene eliminato nulla nell’applicazione e puoi riconnetterla quando vuoi.',
  'apps.delete.title': 'Elimina l’integrazione',
  'apps.delete.body':
    'L’integrazione {name} verrà eliminata definitivamente e le sue credenziali revocate presso il provider. Per usarla di nuovo dovrai riconfigurarla dall’inizio.',
  'apps.toast.deactivated': 'Disattivata',
  'apps.toast.deactivatedHint': '{name} non appare più nel portale.',
  'apps.toast.deleted': 'Eliminata',
  'apps.toast.deletedHint': '{name} è stata eliminata con successo.',
  'apps.toast.deleteFailed': 'Impossibile eliminare l’integrazione',
  'apps.toast.oauthConnected': 'Integrazione connessa',
  'apps.toast.oauthConnectedHint': '{name} è stata connessa con successo via OAuth2.',
  'apps.toast.testOk': 'Connessione corretta',
  'apps.toast.testOkHint': '{name} risponde: {what}.',
  'apps.toast.testSsoOnly':
    '{name} usa solo l’accesso con Sytue: non ci sono credenziali da verificare.',
  'apps.toast.syncStarted': 'Sincronizzazione avviata',
  'apps.toast.syncStartedHint':
    'Viene eseguita in background; potrai vedere lo stato tra qualche minuto.',
  'apps.toast.testFailed': 'La connessione non è riuscita',
  'apps.toast.actionFailed': 'Operazione non riuscita',
  'apps.toast.gatewayDown':
    'Il servizio dell’applicazione o Sytue ID non rispondono in questo momento.',

  // --- Single sign-on (SSO) -------------------------------------------------
  'sso.subtitle':
    'Fai accedere il tuo team a Sytue con l’account aziendale (Microsoft Entra ID, Google Workspace, Okta…). Sei sempre tu a decidere chi può passare.',
  'sso.connectProvider': 'Collega un provider',
  'sso.offlineBody':
    'Al momento né i provider né la loro configurazione si possono caricare o modificare. Riprova tra qualche minuto.',
  'sso.wizardDescription':
    'Ti accompagniamo passo dopo passo. Puoi tornare indietro in qualsiasi momento; nessuno potrà accedere per questa via finché non lo attivi.',
  'sso.providers.title': 'Provider collegati',
  'sso.providers.description':
    'Ogni provider aggiunge un pulsante «Accedi con la tua organizzazione» nella schermata di accesso. Chi ha già una password Sytue può continuare a usarla.',
  'sso.providers.emptyTitle': 'Non hai ancora collegato nessun provider',
  'sso.providers.emptyDescription':
    'Quando colleghi il primo, il tuo team accederà con l’account aziendale e non dovrai gestire password separate.',
  'sso.providers.emptyAction': 'Collega il primo',
  'sso.providers.caption': 'Provider di identità',
  'sso.col.provider': 'Provider',
  'sso.col.connection': 'Connessione',
  'sso.col.whoCanSignIn': 'Chi può accedere',
  'sso.col.status': 'Stato',
  'sso.unverified': 'Non verificato',
  'sso.anyEmail': 'qualsiasi indirizzo del provider',
  'sso.test': 'Verifica',
  'sso.testAria': 'Verifica {name}',
  'sso.delete': 'Elimina',
  'sso.deleteAria': 'Elimina {name}',
  'sso.deleteTitle': 'Elimina il provider',
  'sso.deleteBody':
    '{name} non comparirà più nella schermata di accesso e il collegamento fra gli account Sytue e quelli del tuo provider verrà interrotto. Chi ha una password Sytue potrà continuare a usarla; gli altri dovranno recuperare la propria.',
  'sso.apps.title': 'Applicazioni che accedono con Sytue',
  'sso.apps.description':
    'Al contrario di sopra: qui registri gli strumenti del tuo team perché il loro pulsante di accesso porti a Sytue, invece che a Google o Microsoft.',
  'sso.apps.register': 'Registra un’applicazione',
  'sso.apps.wizardDescription':
    'Ti accompagniamo passo dopo passo. Alla fine ti diamo i valori da incollare nell’applicazione.',
  'sso.apps.emptyTitle': 'Non hai ancora registrato nessuna applicazione',
  'sso.apps.emptyDescription':
    'Se uno strumento che già usate consente di «accedere con» un provider proprio, può usare Sytue. Ti guidiamo in tre passaggi e ti diamo i valori da incollare.',
  'sso.apps.emptyAction': 'Registra la prima',
  'sso.apps.caption': 'Applicazioni che accedono con Sytue',
  'sso.apps.col.app': 'Applicazione',
  'sso.apps.col.runsOn': 'Dove viene eseguita',
  'sso.apps.col.redirect': 'Torna a',
  'sso.apps.runsPublic': 'Browser o dispositivo mobile (PKCE)',
  'sso.apps.runsConfidential': 'Server proprio (con segreto)',
  'sso.apps.rotate': 'Nuovo segreto',
  'sso.apps.rotateAria': 'Genera un nuovo segreto per {name}',
  'sso.apps.deleteTitle': 'Elimina l’applicazione',
  'sso.apps.deleteBody':
    '{name} non potrà più accedere con Sytue, con effetto immediato, e chi è già dentro dovrà rientrare per un’altra via. Non viene eliminato nulla dentro l’applicazione; puoi registrarla di nuovo quando vuoi (con valori nuovi).',
  // SP SAML: app di terzi per cui Sytue ID fa da IdP.
  'sso.saml.title': 'Applicazioni che accedono con Sytue tramite SAML',
  'sso.saml.description':
    'Sytue ID è il provider di identità di queste applicazioni. Compaiono qui quando attivi un’app dal catalogo e puoi spegnerne ciascuna senza perderne la configurazione.',
  'sso.saml.caption': 'Applicazioni che accedono con Sytue tramite SAML',
  'sso.saml.col.app': 'Applicazione',
  'sso.saml.col.acs': 'Invia la risposta a',
  'sso.saml.emptyTitle': 'Nessuna applicazione usa ancora Sytue come provider SAML',
  'sso.saml.emptyDescription':
    'Attiva un’applicazione dal catalogo e comparirà qui, pronta da spegnere o rimuovere.',
  'sso.saml.incomplete': 'Incompleta',
  'sso.saml.details': 'Dati di connessione',
  'sso.saml.detailsAria': 'Dati di connessione di {name}',
  'sso.saml.detailsBody':
    'Incolla questi valori in {name} affinché si fidi di Sytue ID. Sono gli stessi per tutte le applicazioni della tua organizzazione.',
  'sso.saml.entityId': 'Provider di identità (entity ID)',
  'sso.saml.metadataUrl': 'URL dei metadati',
  'sso.saml.ssoUrl': 'URL di accesso (SSO)',
  'sso.saml.sloUrl': 'URL di uscita (SLO)',
  'sso.saml.initiateUrl': 'Avvia da Sytue',
  'sso.saml.deleteTitle': 'Rimuovi l’applicazione',
  'sso.saml.deleteBody':
    '{name} non potrà più accedere con Sytue, con effetto immediato. Non viene eliminato nulla dentro l’applicazione, ma dovrai registrarla di nuovo per restituirle l’accesso.',
  'sso.saml.toast.toggleFailed': 'Impossibile cambiare lo stato',
  'sso.saml.toast.deleted': 'Applicazione rimossa',
  'sso.secret.title': 'Copia subito il segreto',
  'sso.secret.body':
    'Questo è il nuovo segreto di {name}. Il precedente non vale più: incolla questo nell’applicazione al più presto. Non potremo mostrartelo di nuovo.',
  'sso.secret.label': 'Segreto client',
  'sso.secret.copied': 'Segreto copiato',
  'sso.secret.copyFailed': 'Impossibile copiare',
  'sso.secret.copyFailedHint': 'Selezionalo e copialo a mano.',
  'sso.secret.done': 'L’ho copiato',
  'sso.toast.testOk': 'Tutto corretto',
  'sso.toast.testOkSaml': 'I valori del tuo provider sono validi.',
  'sso.toast.testOkOidc': 'Il tuo provider ha risposto.',
  'sso.toast.testOkOidcPkce': 'Il tuo provider ha risposto (con PKCE, la modalità più sicura).',
  'sso.toast.testFailed': 'Non è stato possibile verificarlo',
  'sso.toast.testBadSaml':
    'Controlla l’Entity ID, l’URL di accesso (https) e il certificato. Modificalo per correggerli.',
  'sso.toast.testBadOidc':
    'L’indirizzo del provider non risponde con la sua configurazione. Modificalo e controllalo.',
  'sso.toast.idpDown': 'Sytue ID non risponde in questo momento. Riprova tra un minuto.',
  'sso.toast.providerDeleted': 'Provider eliminato',
  'sso.toast.deleteFailed': 'Impossibile eliminare',
  'sso.toast.rotateFailed': 'Non è stato possibile generare un nuovo segreto',
  'sso.toast.rotateFailedHint': '{name} conserva quello che aveva.',
  'sso.toast.appDeleted': 'Applicazione eliminata',
  'sso.toast.appDeletedHint': '{name} non può più accedere con Sytue.',

  // --- Ruoli personalizzati e autorizzazioni delegabili ---------------------
  'roles.subtitle':
    'Ruoli personalizzati con autorizzazioni granulari (amministrazione delegata). Assegnali alle persone per delegare compiti specifici senza concedere accesso totale.',
  'roles.offline':
    'Nessuna connessione a Sytue ID: non è possibile caricare né modificare i ruoli.',
  'roles.searchPlaceholder': 'Cerca ruolo',
  'roles.create': 'Crea ruolo',
  'roles.delete': 'Elimina ruolo',
  'roles.permissions': 'Autorizzazioni',
  'roles.notDelegable':
    'La gestione dei ruoli è riservata a proprietà e amministrazione (non è delegabile).',
  'roles.empty.title': 'Non ci sono ancora ruoli',
  'roles.empty.description':
    'Crea un ruolo (per esempio «Help Desk») per delegare compiti specifici senza concedere accesso totale.',
  'roles.noResults': 'Nessun ruolo corrisponde',
  'roles.noResultsHint': 'Prova con un altro testo di ricerca.',
  'roles.openAria': 'Apri il ruolo {name}',
  'roles.noPermissions': 'Nessuna autorizzazione',
  'roles.namePlaceholder': 'es. Help Desk',
  'roles.fallbackTitle': 'Ruolo',
  'roles.saveChanges': 'Salva le modifiche',
  'roles.discard': 'Scarta',
  'roles.assigned': 'Assegnatari',
  'roles.noAssignees': 'Questo ruolo non è ancora assegnato a nessuno.',
  'roles.unassignAria': 'Togli il ruolo a {name}',
  'roles.assignTo': 'Assegna a una persona',
  'roles.assignPlaceholder': 'Cerca una persona dell’organizzazione',
  'roles.noCandidates': 'Nessun utente disponibile corrisponde.',
  'roles.assignHint': 'Scrivi per cercare fra le persone dell’organizzazione.',
  'roles.deleteConfirm':
    'Eliminare il ruolo «{name}»? Verrà tolto a tutti gli assegnatari, che perderanno quelle autorizzazioni. L’azione non si può annullare.',
  'roles.toast.membersFailed': 'Non è stato possibile caricare gli assegnatari',
  'roles.toast.created': 'Ruolo creato',
  'roles.toast.createdHint': '«{name}» ora esiste.',
  'roles.toast.createFailed': 'Non è stato possibile creare il ruolo',
  'roles.toast.nameTaken': 'Esiste già un ruolo con quel nome.',
  'roles.toast.checkNameAndPerms': 'Controlla il nome e le autorizzazioni selezionate.',
  'roles.toast.checkPerms': 'Controlla le autorizzazioni selezionate.',
  'roles.toast.updated': 'Ruolo aggiornato',
  'roles.toast.assignFailed': 'Non è stato possibile assegnare',
  'roles.toast.alreadyAssigned': 'Quella persona ha già il ruolo.',
  'roles.toast.unassignFailed': 'Non è stato possibile togliere il ruolo',
  'roles.toast.deleted': 'Ruolo eliminato',
  'roles.toast.deletedHint': '«{name}» non esiste più.',
  'roles.toast.deleteFailed': 'Non è stato possibile eliminare il ruolo',
  'perm.usersManage': 'Gestire gli account',
  'perm.usersManageHint': 'Creazione/rimozione, ruolo, reimpostazione di password e MFA, sessioni.',
  'perm.usersRead': 'Vedere gli account',
  'perm.usersReadHint': 'Directory e sessioni (sola lettura).',
  'perm.groupsManage': 'Gestire i gruppi',
  'perm.groupsManageHint': 'Gruppi dell’organizzazione e loro composizione.',
  'perm.ssoManage': 'Gestire il single sign-on',
  'perm.ssoManageHint': 'Provider di accesso (OIDC/SAML).',
  'perm.accessManage': 'Accesso condizionale',
  'perm.accessManageHint': 'Regole valutate al momento dell’accesso.',
  'perm.appsManage': 'Integrazioni',
  'perm.appsManageHint': 'App di terze parti (SAML/OIDC in uscita, DCR).',
  'perm.auditView': 'Vedere il controllo',
  'perm.auditViewHint': 'Registro di controllo dell’organizzazione.',
  'perm.billingManage': 'Fatturazione',
  'perm.billingManageHint': 'Fatturazione dell’organizzazione.',

  // --- Accesso con privilegi (PIM) ------------------------------------------
  'pim.subtitle':
    'Elevazione dei privilegi just-in-time (PIM). Le persone idonee attivano un ruolo per una finestra limitata, con una motivazione e — se richiesto — un’approvazione. Concede autorizzazioni solo finché è attivo.',
  'pim.offline':
    'Nessuna connessione a Sytue ID: non è possibile caricare né gestire l’accesso con privilegi.',
  'pim.status.pending': 'In attesa',
  'pim.status.active': 'Attivo',
  'pim.status.revoked': 'Revocato',
  'pim.status.denied': 'Negato',
  'pim.status.expired': 'Scaduto',
  'pim.tab.me': 'Il mio accesso',
  'pim.tab.admin': 'Amministrazione',
  'pim.me.eligibleTitle': 'Ruoli che posso attivare',
  'pim.me.emptyTitle': 'Non sei idoneo per nessun ruolo',
  'pim.me.emptyDescription':
    'Quando l’amministrazione renderà idoneo il tuo account, potrai elevare i privilegi qui per una finestra limitata.',
  'pim.me.activationsTitle': 'Le mie attivazioni',
  'pim.me.noActivations': 'Non hai ancora attivato nessun ruolo.',
  'pim.requiresApproval': 'Richiede approvazione',
  'pim.directActivation': 'Attivazione diretta',
  'pim.withApproval': 'con approvazione',
  'pim.directActivationShort': 'attivazione diretta',
  'pim.maxDuration': 'max {minutes} min',
  'pim.minutes': '{minutes} min',
  'pim.minutesLeft': 'restano ~{minutes} min',
  'pim.expiresAt': 'Scade {when}',
  'pim.activate': 'Attiva',
  'pim.request': 'Richiedi',
  'pim.deactivate': 'Disattiva',
  'pim.approve': 'Approva',
  'pim.deny': 'Nega',
  'pim.noJustification': 'senza motivazione',
  'pim.admin.pendingTitle': 'Richieste in attesa',
  'pim.admin.noPending': 'Non ci sono richieste in attesa di approvazione.',
  'pim.admin.activeTitle': 'Attivazioni in corso',
  'pim.admin.noActive': 'Nessun ruolo con privilegi è attivo in questo momento.',
  'pim.admin.eligibilitiesTitle': 'Idoneità',
  'pim.admin.newEligibility': 'Nuova idoneità',
  'pim.admin.emptyTitle': 'Non ci sono ancora idoneità',
  'pim.admin.emptyDescription':
    'Rendi idonea una persona per un ruolo (per esempio «Help Desk» o amministrazione) e potrà elevare i privilegi just-in-time, con o senza approvazione.',
  'pim.removeEligibilityAria': 'Ritira l’idoneità di {user} per {role}',
  'pim.activateTitle': 'Attiva «{name}»',
  'pim.activateFallbackTitle': 'Attiva ruolo',
  'pim.approvalNotice':
    'Questo ruolo richiede un’approvazione: resterà in attesa finché l’amministrazione non la concede.',
  'pim.justification': 'Motivazione',
  'pim.justificationPlaceholder': 'Motivo dell’elevazione (viene registrato)',
  'pim.durationLabel': 'Durata (minuti)',
  'pim.durationHelp': 'Al massimo {minutes} min per questo ruolo.',
  'pim.newElig.user': 'Persona',
  'pim.newElig.selectUser': 'Scegli una persona…',
  'pim.newElig.role': 'Ruolo',
  'pim.newElig.selectRole': 'Scegli un ruolo…',
  'pim.newElig.customRoles': 'Ruoli personalizzati',
  'pim.newElig.builtinRoles': 'Ruoli integrati',
  'pim.newElig.builtinAdmin': 'admin (superutente)',
  'pim.newElig.builtinOwner': 'owner (superutente)',
  'pim.newElig.maxLabel': 'Durata massima (minuti)',
  'pim.newElig.maxHelp': 'Finestra massima di ogni attivazione (1–1440).',
  'pim.newElig.requiresApproval': 'Richiede l’approvazione dell’amministrazione',
  'pim.newElig.submit': 'Crea idoneità',
  'pim.toast.requested': 'Richiesta inviata',
  'pim.toast.requestedHint': 'L’amministrazione deve approvarla.',
  'pim.toast.activated': 'Ruolo attivato',
  'pim.toast.activatedHint': 'Valido per {minutes} min.',
  'pim.toast.activateFailed': 'Non è stato possibile attivare',
  'pim.toast.notEligible': 'Non sei più idoneo per quel ruolo.',
  'pim.toast.checkDuration': 'Controlla la durata (supera il massimo) e la motivazione.',
  'pim.toast.deactivated': 'Ruolo disattivato',
  'pim.toast.deactivateFailed': 'Non è stato possibile disattivare',
  'pim.toast.approved': 'Richiesta approvata',
  'pim.toast.denied': 'Richiesta negata',
  'pim.toast.decideFailed': 'Operazione non riuscita',
  'pim.toast.eligCreated': 'Idoneità creata',
  'pim.toast.eligCreateFailed': 'Non è stato possibile creare',
  'pim.toast.eligDuplicate': 'Quella persona è già idonea per quel ruolo.',
  'pim.toast.eligCheckFields': 'Controlla la persona, il ruolo e la durata massima.',
  'pim.toast.eligRemoved': 'Idoneità ritirata',
  'pim.toast.eligRemoveFailed': 'Non è stato possibile ritirare',

  // --- Gruppi e regole di appartenenza dinamica -----------------------------
  'groups.subtitle':
    'Gruppi dell’organizzazione in Sytue ID. Governano la condivisione per gruppo di Sytue Drive.',
  'groups.offline':
    'Nessuna connessione a Sytue ID: vengono mostrati dati di esempio. Le modifiche non verranno salvate.',
  'groups.searchPlaceholder': 'Cerca gruppo',
  'groups.create': 'Crea gruppo',
  'groups.delete': 'Elimina gruppo',
  'groups.fallbackTitle': 'Gruppo',
  'groups.empty.title': 'Non ci sono ancora gruppi',
  'groups.empty.description':
    'Crea un gruppo per organizzare le persone dell’organizzazione e condividere risorse per gruppo.',
  'groups.noResults': 'Nessun gruppo corrisponde',
  'groups.openAria': 'Apri il gruppo {name}',
  'groups.dynamic': 'Dinamico',
  'groups.noDescription': 'Senza descrizione',
  'groups.description': 'Descrizione',
  'groups.namePlaceholder': 'es. Ingegneria',
  'groups.dynamicSwitch': 'Gruppo dinamico (appartenenza per regola)',
  'groups.dynamicHelp':
    'L’appartenenza si calcola dagli attributi di ogni account (reparto, ruolo, sede…). Nessuno si aggiunge a mano.',
  'groups.ruleMembership': 'Appartenenza per regola',
  'groups.ruleMembershipHelp':
    'L’appartenenza la calcola questa regola sugli attributi di ogni account.',
  'groups.convertSwitch': 'Trasforma in gruppo dinamico (appartenenza per regola)',
  'groups.convertWarning':
    'Salvando la regola, questo gruppo diventerà dinamico e le persone che ne fanno parte verranno sostituite da quelle indicate dalla regola.',
  'groups.saveRule': 'Salva la regola',
  'groups.convert': 'Trasforma in dinamico',
  'groups.recompute': 'Ricalcola adesso',
  'groups.recomputeAll': 'Ricalcola tutti',
  'groups.recomputeHelp':
    'L’appartenenza si aggiorna da sola quando cambiano gli attributi dell’account (compresa la sincronizzazione SCIM dalla tua directory). Ricalcola se vuoi verificarla subito.',
  'groups.members': 'Persone',
  'groups.computedMembers': 'Persone calcolate',
  'groups.noMembers': 'Nel gruppo non c’è ancora nessuno.',
  'groups.noRuleMatches': 'Nessun utente soddisfa ancora la regola.',
  'groups.removeMemberAria': 'Togli {name} dal gruppo',
  'groups.addMember': 'Aggiungi una persona',
  'groups.deleteConfirm':
    'Eliminare il gruppo «{name}»? Verranno tolte tutte le persone che ne fanno parte e si perderà la condivisione per gruppo associata. L’azione non si può annullare.',
  'groups.rule.include': 'Includi chi soddisfa',
  'groups.rule.matchAll': 'TUTTE le condizioni',
  'groups.rule.matchAny': 'ALMENO una condizione',
  'groups.rule.attribute': 'Attributo',
  'groups.rule.operator': 'Operatore',
  'groups.rule.value': 'valore',
  'groups.rule.valueListHint': 'valore1, valore2, …',
  'groups.rule.removeCondition': 'Togli la condizione',
  'groups.rule.addCondition': 'Aggiungi una condizione',
  'groups.rule.preview': 'Vedi chi entrerebbe',
  'groups.rule.previewNone': 'Nessun utente corrisponde.',
  'groups.rule.previewCount': {
    one: 'Corrisponderebbe {count} persona.',
    other: 'Corrisponderebbero {count} persone.',
  },
  'groups.toast.membersFailed': 'Non è stato possibile caricare l’elenco delle persone',
  'groups.toast.sampleData': 'Nessuna connessione a Sytue ID: mostriamo dati di esempio.',
  'groups.toast.previewFailed': 'Non è stato possibile fare l’anteprima',
  'groups.toast.previewFailedHint': 'Controlla che ogni condizione abbia un valore.',
  'groups.toast.created': 'Gruppo creato',
  'groups.toast.dynamicCreated': 'Gruppo dinamico creato',
  'groups.toast.createdHint': '«{name}» ora esiste.',
  'groups.toast.createFailed': 'Non è stato possibile creare il gruppo',
  'groups.toast.nameTaken': 'Esiste già un gruppo con quel nome.',
  'groups.toast.checkName': 'Controlla il nome e riprova.',
  'groups.toast.checkNameAndRule': 'Controlla il nome e la regola, poi riprova.',
  'groups.toast.updated': 'Gruppo aggiornato',
  'groups.toast.alreadyUpToDate': 'L’appartenenza era già aggiornata',
  'groups.toast.recomputed': 'Appartenenza ricalcolata',
  'groups.toast.recomputedSame': {
    one: '{count} persona secondo la regola.',
    other: '{count} persone secondo la regola.',
  },
  'groups.toast.recomputedAdded': { one: '{count} aggiunto', other: '{count} aggiunti' },
  'groups.toast.recomputedRemoved': { one: '{count} rimosso', other: '{count} rimossi' },
  'groups.toast.recomputedAllGroups': {
    one: '{count} gruppo dinamico',
    other: '{count} gruppi dinamici',
  },
  'groups.toast.recomputeFailed': 'Non è stato possibile ricalcolare',
  'groups.toast.ruleSaved': 'Regola salvata',
  'groups.toast.ruleSavedHint': 'Appartenenza ricalcolata.',
  'groups.toast.ruleSaveFailed': 'Non è stato possibile salvare la regola',
  'groups.toast.ruleInvalid': 'La regola non è valida: controlla condizioni e valori.',
  'groups.toast.addFailed': 'Non è stato possibile aggiungere',
  'groups.toast.alreadyMember': 'Quella persona fa già parte del gruppo.',
  'groups.toast.removeMemberFailed': 'Non è stato possibile togliere la persona',
  'groups.toast.deleted': 'Gruppo eliminato',
  'groups.toast.deletedHint': '«{name}» non esiste più.',
  'groups.toast.deleteFailed': 'Non è stato possibile eliminare il gruppo',
  'rule.attr.department': 'Reparto',
  'rule.attr.title': 'Ruolo',
  'rule.attr.location': 'Sede',
  'rule.attr.email': 'Email',
  'rule.attr.name': 'Nome',
  'rule.op.eq': 'è uguale a',
  'rule.op.ne': 'non è',
  'rule.op.contains': 'contiene',
  'rule.op.startsWith': 'inizia con',
  'rule.op.in': 'è in (elenco)',

  // --- Governance (politica di egress) --------------------------------------
  'gov.subtitle':
    'Controlla cosa possono inviare all’esterno le tue integrazioni. Ogni chiamata a un servizio di terze parti passa da qui, viene registrata e non si può aggirare.',
  'gov.offlineTitle': 'Non riusciamo a contattare il gateway di integrazioni',
  'gov.offlineBody':
    'Al momento né le regole né i registri si possono caricare o modificare. Riprova tra qualche minuto.',
  'gov.tab.rules': 'Regole',
  'gov.tab.egress': 'Registro di egress',
  'gov.tab.decisions': 'Decisioni',
  'gov.badge.block': 'Blocca',
  'gov.badge.warn': 'Avvisa',
  'gov.badge.allow': 'Lascia passare',
  'gov.act.warn': 'Lasciarla passare, ma avvisarti',
  'gov.act.warnBody':
    'La chiamata esce lo stesso e l’integrazione continua a funzionare, ma resta annotata in «Decisioni». È il modo di provare una regola prima di tagliare qualcosa.',
  'gov.act.warnBadge': 'Il modo più sicuro per iniziare',
  'gov.act.block': 'Bloccare la chiamata',
  'gov.act.blockBody':
    'La chiamata non esce da Sytue. L’integrazione riceverà un errore e quella sua funzione smetterà di funzionare. Usalo quando quel dato non deve uscire mai.',
  'gov.act.allow': 'Lasciarla passare e basta',
  'gov.act.allowBody':
    'Esce normalmente e non viene annotata nessuna decisione. Serve per fare un’eccezione a un’altra regola più severa.',
  'gov.field.host': 'Il sito che viene chiamato',
  'gov.field.hostHelp':
    'Il dominio del servizio, per esempio api.salesforce.com. Se lo inizi con un punto (.salesforce.com) include tutti i suoi sottodomini.',
  'gov.field.family': 'L’integrazione',
  'gov.field.familyHelp':
    'La chiave del modello con cui hai collegato il servizio. Usala quando il dominio specifico non conta.',
  'gov.field.method': 'Il tipo di chiamata',
  'gov.field.methodHelp':
    'GET legge soltanto; POST, PUT, PATCH e DELETE scrivono o cancellano. Lascia vuoto per qualsiasi metodo.',
  'gov.field.path': 'La parte dell’API',
  'gov.field.pathHelp':
    'L’inizio del percorso, se vuoi solo una parte del servizio. Lascia vuoto per tutta l’API.',
  'gov.field.body': 'Un testo dentro ciò che si invia',
  'gov.field.bodyHelp':
    'Per le fughe di dati: agisce se il contenuto inviato include quella parola. Si può ispezionare solo negli invii piccoli, quindi non usarla come unica condizione.',
  'gov.cond.any': 'Qualsiasi egress',
  'gov.cond.all': 'tutte le chiamate a terze parti, senza eccezioni',
  'gov.cond.calls': 'le chiamate {what}',
  'gov.cond.subdomain': 'a qualsiasi sottodominio di {host}',
  'gov.cond.host': 'a {host}',
  'gov.cond.family': 'dell’integrazione {family}',
  'gov.cond.method': 'con metodo {method}',
  'gov.cond.path': 'il cui percorso inizia con {path}',
  'gov.cond.body': 'che inviano «{text}» nel contenuto',
  'gov.suggest.block': 'Blocca {what}',
  'gov.suggest.warn': 'Avvisa su {what}',
  'gov.suggest.allow': 'Consenti {what}',
  'gov.impact.allowTitle': 'Questa regola non può tagliare nulla',
  'gov.impact.allowBody':
    'Le regole che lasciano passare non rompono mai un’integrazione: fanno solo eccezione a un’altra più severa.',
  'gov.impact.wideTitle': 'Questa regola taglierebbe TUTTE le tue integrazioni',
  'gov.impact.wideBody':
    'Senza alcuna condizione la regola raggiunge qualsiasi chiamata a terze parti: i dati smetterebbero di sincronizzarsi e l’accesso alle app collegate fallirebbe. Torna al primo passaggio e restringi almeno il sito che viene chiamato.',
  'gov.impact.unknownTitle': 'Non possiamo stimare l’impatto',
  'gov.impact.unknownBody':
    'Il registro di egress è vuoto, quindi non sappiamo quali integrazioni sarebbero colpite. Se hai dubbi, creala come avviso e osservala qualche giorno prima di bloccare.',
  'gov.impact.noneTitle': 'Nulla di ciò che funziona ora verrebbe colpito',
  'gov.impact.noneBody': {
    one: '{count} chiamata registrata, e non corrisponde a questa regola.',
    other: 'Nessuna delle ultime {count} chiamate registrate corrisponde a questa regola.',
  },
  'gov.impact.blockTitle': {
    one: 'Questa regola taglierebbe traffico in uso ({matched} di {count} chiamata)',
    other: 'Questa regola taglierebbe traffico in uso ({matched} delle ultime {count} chiamate)',
  },
  'gov.impact.warnTitle': {
    one: 'Questa regola segnalerebbe {matched} di {count} chiamata',
    other: 'Questa regola segnalerebbe {matched} delle ultime {count} chiamate',
  },
  'gov.impact.blockBody':
    'Sarebbero state bloccate: {hosts}. Se una appartiene a un’integrazione in uso, quella funzione smetterà di funzionare appena salvi. Creala prima come avviso se preferisci verificarlo senza rompere nulla.',
  'gov.impact.warnBody':
    'Sarebbero state segnalate: {hosts}. Se una appartiene a un’integrazione in uso, quella funzione smetterà di funzionare appena salvi.',
  'gov.impact.upperBound':
    'Il conteggio è un limite massimo: il percorso e il contenuto non si possono verificare dal registro.',
  'gov.step.traffic': 'Quale traffico',
  'gov.step.trafficHeading': 'Quali chiamate vuoi che riguardi?',
  'gov.step.trafficDescription':
    'Compila solo ciò che ti serve: ogni campo lasciato vuoto è un jolly. Di solito si restringe in base al sito che viene chiamato.',
  'gov.step.wideTitle': 'Senza condizioni riguarda tutto',
  'gov.step.wideBody':
    'Così com’è, la regola raggiunge tutte le chiamate a terze parti dell’intera organizzazione. È ciò che serve per una regola di avviso generale, ma raramente per una di blocco.',
  'gov.step.willMatch': 'Questa regola guarderà',
  'gov.step.action': 'Cosa fare',
  'gov.step.actionHeading': 'Cosa vuoi che succeda a quelle chiamate?',
  'gov.step.actionDescription': 'Scegli in base alla conseguenza, non al nome tecnico.',
  'gov.step.orderNote':
    'Le regole si valutano in ordine. Una regola «lascia passare» messa prima funziona da eccezione per quelle successive.',
  'gov.step.review': 'Riepilogo',
  'gov.step.reviewNew': 'Controlla e salva la regola',
  'gov.step.reviewEdit': 'Controlla le modifiche',
  'gov.step.reviewDescription':
    'Verifichiamo sul registro recente quale traffico reale toccherebbe questa regola.',
  'gov.step.done': 'Fatto',
  'gov.step.savedChanges': 'Modifiche salvate',
  'gov.review.affects': 'Cosa riguarda',
  'gov.review.happens': 'Cosa succede',
  'gov.review.willApply': 'Si applicherà appena la salvi',
  'gov.review.savedOff': 'Salvata senza applicarla',
  'gov.createRule': 'Crea la regola',
  'gov.newRuleTitle': 'Nuova regola di traffico',
  'gov.wizardDescription':
    'Due domande e un riepilogo. Prima di salvare ti diciamo se la regola taglierebbe chiamate che si stanno facendo proprio ora.',
  'gov.ruleName': 'Nome della regola',
  'gov.ruleNameHint':
    'È come la vedrai nell’elenco. Se lo lasci vuoto la chiameremo «{suggested}».',
  'gov.applyNow': 'Applica subito la regola',
  'gov.applyNowOn': 'Inizierà a essere valutata alla prossima chiamata a terze parti.',
  'gov.applyNowOff':
    'Verrà salvata spenta: puoi prepararla adesso e accenderla dall’elenco quando vuoi.',
  'gov.done.on': 'Viene già valutata a ogni chiamata a terze parti: {what}.',
  'gov.done.off': 'È salvata ma spenta: accendila dall’elenco quando vuoi.',
  'gov.done.checkTitle': 'Verificalo prima di andare',
  'gov.done.checkBodyBlock':
    'Apri la scheda «Decisioni» tra un po’: lì vedrai ogni chiamata che la regola ha bloccato. Se compare un’integrazione che doveva continuare a funzionare, spegni la regola dall’elenco e restringi la condizione.',
  'gov.done.checkBodyWarn':
    'Apri la scheda «Decisioni» tra un po’: lì vedrai ogni chiamata che la regola ha segnalato. Se compare un’integrazione che doveva continuare a funzionare, spegni la regola dall’elenco e restringi la condizione.',
  'gov.done.back': 'Torna alle regole',
  'gov.rules.title': 'Regole del traffico in uscita',
  'gov.rules.description':
    'Vengono valutate dall’alto verso il basso a ogni chiamata che un’integrazione fa a un servizio esterno. Senza regole esce tutto (e viene comunque registrato).',
  'gov.rules.emptyDescription':
    'Senza regole, qualsiasi integrazione collegata può chiamare dove vuole. La prima che quasi tutti creano: avvisare quando qualcosa esce, per vedere il traffico reale prima di bloccare.',
  'gov.rules.emptyAction': 'Crea la prima regola',
  'gov.col.action': 'Azione',
  'gov.col.rule': 'Regola',
  'gov.col.enabled': 'Attiva',
  'gov.col.site': 'Sito',
  'gov.col.call': 'Chiamata',
  'gov.col.response': 'Risposta',
  'gov.col.took': 'Durata',
  'gov.col.decision': 'Decisione',
  'gov.col.destination': 'Destinazione',
  'gov.toggleAria': 'Applica la regola {name}',
  'gov.toast.toggleFailed': 'Non è stato possibile cambiare lo stato',
  'gov.toast.badCondition':
    'Il gateway ha rifiutato un valore. Controlla le condizioni del primo passaggio.',
  'gov.toast.forbidden':
    'Il tuo account non ha l’autorizzazione per modificare le regole di questa organizzazione.',
  'gov.toast.gatewayDown': 'Il gateway non risponde in questo momento. Riprova tra un minuto.',
  'gov.toast.created': 'Regola creata',
  'gov.delete.intro': '{name} verrà eliminata.',
  'gov.delete.afterTitle': 'Da quel momento',
  'gov.delete.afterBlock':
    'Il traffico che questa regola bloccava tornerà a uscire da Sytue normalmente. Se vuoi solo metterla in pausa, spegnila dall’elenco invece di eliminarla: così conservi la condizione.',
  'gov.delete.afterWarn':
    'Quel traffico smetterà di essere annotato in «Decisioni». Quanto già registrato si conserva.',
  'gov.delete.confirm': 'Elimina la regola',
  'gov.networkError': 'errore di rete',
  'gov.egress.title': 'Registro di egress',
  'gov.egress.description':
    'Ogni chiamata che un’integrazione ha fatto a un servizio esterno: dove, quando e come ha risposto. Solo metadati, mai il contenuto né le credenziali.',
  'gov.egress.offline':
    'Nessuna connessione al gateway: non è possibile caricare il registro di egress.',
  'gov.egress.loadFailed': 'Non è stato possibile caricare il registro. Regola i filtri e riprova.',
  'gov.egress.emptyTitle': 'Ancora nulla di registrato',
  'gov.egress.emptyDescription':
    'Per quel filtro non ci sono ancora chiamate a terze parti. Appena un’integrazione uscirà in rete, comparirà qui.',
  'gov.exportCsv': 'Esporta CSV',
  'gov.filterHost': 'Sito di destinazione',
  'gov.filterHostAria': 'Filtra per sito di destinazione',
  'gov.filterStatusAria': 'Filtra per codice di risposta HTTP',
  'gov.applyFilters': 'Applica i filtri',
  'gov.decisions.title': 'Cosa hanno fatto le tue regole',
  'gov.decisions.description':
    'Ogni volta che una regola blocca o segnala una chiamata, resta qui. Un blocco non lascia traccia nel registro di egress (la chiamata non è mai uscita): questa è la sua unica prova.',
  'gov.decisions.filterAria': 'Filtra per tipo di decisione',
  'gov.decisions.onlyBlocks': 'Solo blocchi',
  'gov.decisions.onlyWarnings': 'Solo avvisi',
  'gov.decisions.offline': 'Nessuna connessione al gateway: non è possibile caricare le decisioni.',
  'gov.decisions.loadFailed':
    'Non è stato possibile caricare le decisioni. Riprova tra qualche secondo.',
  'gov.decisions.emptyTitle': 'Nessuna regola è ancora intervenuta',
  'gov.decisions.emptyDescription':
    'Non è stato bloccato né segnalato nulla. Quando una regola agirà su una chiamata, la vedrai qui con la sua data e la sua destinazione.',
  'gov.decision.blocked': 'Bloccata',
  'gov.decision.warned': 'Segnalata',

  // --- Conformità (blocco a fini legali + eDiscovery) -----------------------
  'comp.subtitle':
    'Conserva la posta e i file di una persona per tutta la durata di un caso e cerca o scarica i suoi contenuti quando la legge lo impone. È accesso a dati altrui: tutto ciò che fai qui viene registrato a tuo nome.',
  'comp.tab.holds': 'Blocchi a fini legali',
  'comp.service.mail': 'Posta',
  'comp.service.drive': 'File',
  'comp.service.notes': 'Notepads',
  'comp.noun.mail': 'la posta',
  'comp.noun.drive': 'i file',
  'comp.noun.notes': 'i suoi blocchi note',
  'comp.offline.all': 'Non riusciamo a contattare Posta, File o Notepads',
  'comp.offline.one': 'Non riusciamo a contattare {service}',
  'comp.offline.body':
    'Quanto mostrato può essere incompleto e le azioni su quel servizio falliranno. Riprova tra qualche minuto.',
  'comp.scope.title': 'Cosa viene conservato',
  'comp.scope.help':
    'Ciò che non selezioni qui continua a essere eliminato normalmente, quindi nel dubbio conserva di più: rilasciare dopo è immediato, recuperare ciò che è stato eliminato no.',
  'comp.scope.usual': 'La scelta più comune',
  'comp.scope.bothLabel': 'La sua posta e i suoi file',
  'comp.scope.bothShort': 'Posta e file',
  'comp.scope.bothNoun': 'la sua posta e i suoi file',
  'comp.scope.bothBody':
    'La scelta abituale in un contenzioso: nulla di ciò che quell’account elimina sparisce davvero, né nella casella né in Drive.',
  'comp.scope.mailLabel': 'Solo la sua posta',
  'comp.scope.mailShort': 'Posta',
  'comp.scope.mailNoun': 'la sua posta',
  'comp.scope.mailBody':
    'La casella viene conservata. I file di Drive continueranno a essere eliminati come sempre, anche quelli già nel cestino.',
  'comp.scope.driveLabel': 'Solo i suoi file',
  'comp.scope.driveShort': 'File',
  'comp.scope.driveNoun': 'i suoi file',
  'comp.scope.driveBody':
    'I file di Drive vengono conservati. La posta continuerà a essere eliminata come sempre, anche ciò che è già nel cestino.',
  'comp.scope.notesLabel': 'Solo i suoi blocchi note',
  'comp.scope.notesShort': 'Notepads',
  'comp.scope.notesNoun': 'i suoi blocchi note',
  'comp.scope.notesBody':
    'I blocchi note di Notepads vengono preservati. La posta e i file di Drive continuano a essere eliminati come sempre, incluso ciò che è già nel cestino.',
  'comp.scope.offlineTitle': '{services} non risponde',
  'comp.scope.offlineBody':
    'Quella parte del blocco fallirà. Puoi crearlo lo stesso (ciò che risponde verrà conservato) e ripeterlo dopo per il resto.',
  'comp.subject.wholeOrg': 'Tutta l’organizzazione',
  'comp.subject.wholeOrgInline': 'tutta la tua organizzazione',
  'comp.subject.thatPerson': 'quella persona',
  'comp.subject.notChosen': 'Non scelto',
  'comp.sentence': '{what} di {subject} verrà conservata finché non rilasci il blocco.',
  'comp.step.who': 'Di chi',
  'comp.step.whoHeading': 'Di chi bisogna conservare i dati?',
  'comp.step.whoDescription': 'Inizia decidendo quale account riguarda e cosa se ne conserva.',
  'comp.who.person': 'Una persona',
  'comp.who.personBody':
    'Si conserva solo quell’account. È la scelta normale quando il caso riguarda qualcuno in particolare.',
  'comp.who.orgBody':
    'Si conservano tutti gli account. Usalo solo se il caso riguarda l’intera azienda: finché è attivo non verrà eliminato nulla e lo spazio continuerà a crescere.',
  'comp.who.selectFromDirectory': 'Scegli qualcuno dalla directory…',
  'comp.who.thisAccount': 'questo account',
  'comp.who.willPreserve': 'Verranno conservati i dati di {email}.',
  'comp.who.pickHint':
    'Scegliila dalla directory: il blocco è legato alla sua identità, non al suo indirizzo di posta, quindi resta valido anche se cambia nome.',
  'comp.step.impact': 'Cosa comporta',
  'comp.step.impactHeading': 'Ecco cosa cambia mentre è attivo',
  'comp.step.impactDescription':
    'Nulla di questo si applica ancora. Leggi e prosegui se è ciò che cerchi.',
  'comp.impact.noDeleteTitle': 'Nulla viene eliminato davvero',
  'comp.impact.noDeleteBody':
    'Finché è attivo, né {subject} né tu né altri account di amministrazione potete eliminare definitivamente {what}. La persona può continuare a cancellare quello che vuole: se ne conserva una copia visibile solo da eDiscovery.',
  'comp.impact.silentTitle': 'La persona non se ne accorge',
  'comp.impact.silentBody':
    'Il suo account funziona esattamente allo stesso modo e non mostra alcun avviso. È ciò che le procedure di solito richiedono: nessuno deve sapere che si sta conservando.',
  'comp.impact.auditedTitle': 'Resta registrato a tuo nome',
  'comp.impact.auditedBody':
    'Si annota chi lo crea, quando e perché. Quel registro è la prova che hai conservato in tempo, quindi il motivo conta.',
  'comp.impact.releaseTitle': 'Si rilascia quando il caso finisce',
  'comp.impact.releaseBody':
    'Lo rilasci dall’elenco e torna l’eliminazione normale. Ciò che era nel cestino potrà essere eliminato definitivamente.',
  'comp.impact.orgWideTitle': 'Stai per bloccare tutta l’organizzazione',
  'comp.impact.orgWideBody':
    'Nessun account potrà eliminare nulla definitivamente finché il blocco resta attivo, e lo spazio occupato può solo crescere. Se il caso riguarda persone specifiche, torna indietro e scegli quelle: costa meno ed è più facile da difendere.',
  'comp.impact.whatForTitle': 'A cosa serve un blocco a fini legali',
  'comp.impact.whatForBody':
    'Per contenziosi, indagini interne e richieste dell’autorità: congela la prova prima che qualcuno la cancelli. Non è un backup e non serve ad archiviare.',
  'comp.reason.label': 'Motivo',
  'comp.reason.required': 'Scrivi perché si blocca: è ciò che vedrà il registro di controllo.',
  'comp.reason.placeholder': 'Causa di lavoro di un ex dipendente, richiesta del tribunale n. 4',
  'comp.reason.ok': 'Viene salvato così com’è nel registro di controllo.',
  'comp.reason.hint':
    'Serve per proseguire. Descrivi il caso, non l’azione: «blocco» non dice nulla a nessuno.',
  'comp.step.reasonHeading': 'Perché si blocca?',
  'comp.step.reasonDescription':
    'È la prima cosa che leggerà chi rivedrà il caso fra due anni, te compreso. Basta una frase concreta.',
  'comp.case.label': 'Riferimento del caso',
  'comp.case.none': 'Senza riferimento',
  'comp.case.hint':
    'Facoltativo. L’identificativo che usi nel tuo gestionale dei casi: serve a raggruppare più blocchi della stessa pratica e a etichettare le esportazioni di eDiscovery.',
  'comp.tipTitle': 'Consiglio',
  'comp.tipBody':
    'Se il caso riguarda più persone, crea un blocco per persona con lo stesso riferimento: potrai rilasciarli separatamente man mano che ogni parte si chiude.',
  'comp.review.who': 'Chi riguarda',
  'comp.review.starts': 'Quando inizia',
  'comp.review.startsNow': 'Non appena lo crei',
  'comp.review.sentenceTitle': 'Il blocco, in una frase',
  'comp.review.immediateTitle': 'Si applica subito e resta registrato',
  'comp.review.immediateBody':
    'Da quando lo crei, nulla di {what} può essere eliminato definitivamente. L’azione viene annotata nel registro con il tuo nome, la data e il motivo.',
  'comp.step.reviewHeading': 'Controlla e crea il blocco',
  'comp.step.reviewDescription':
    'Ecco cosa conserveremo. Puoi cambiare qualsiasi cosa prima di crearlo.',
  'comp.create': 'Crea il blocco',
  'comp.newHold': 'Nuovo blocco a fini legali',
  'comp.newHoldShort': 'Nuovo blocco',
  'comp.wizardDescription':
    'Tre domande e un riepilogo. Prima di crearlo ti diciamo esattamente cosa non si potrà più eliminare.',
  'comp.done.heading': 'Blocco attivo',
  'comp.done.rejectedTitle': '{services} non lo ha accettato',
  'comp.done.rejectedBody':
    'Quella parte NON è conservata. Quando il servizio tornerà, crea un altro blocco con lo stesso motivo e riferimento solo per {services}.',
  'comp.done.nextTitle': 'Cosa di solito viene ora',
  'comp.done.nextSearch':
    'Cerca i contenuti del caso dalla scheda eDiscovery: il blocco garantisce che siano ancora lì anche se vengono cancellati.',
  'comp.done.nextRelease':
    'Quando il caso si chiude, rilascialo dall’elenco. Fino ad allora non toccarlo: rilasciare e ricreare lascia una finestra in cui si è potuto eliminare.',
  'comp.done.back': 'Torna ai blocchi',
  'comp.holds.title': 'Blocchi',
  'comp.holds.descriptionActive': 'Quelli che stanno conservando dati in questo momento.',
  'comp.holds.descriptionAll':
    'Attivi e già rilasciati. Quelli rilasciati si conservano come prova di quando è cessata la conservazione.',
  'comp.holds.showReleased': 'Mostra anche quelli rilasciati',
  'comp.holds.whatTitle': 'Cosa fa un blocco a fini legali',
  'comp.holds.whatBody':
    'Congela ciò che c’è in un account per un contenzioso: la persona può continuare a cancellare la posta e i file, ma nulla viene eliminato davvero finché non rilasci il blocco. Crearli e rilasciarli è accesso privilegiato e resta registrato a tuo nome.',
  'comp.holds.emptyActive': 'Non ce n’è nessuno attivo',
  'comp.holds.emptyEver': 'Non ne è mai stato creato nessuno',
  'comp.holds.emptyDescription':
    'Un blocco conserva la posta e i file di una persona per tutta la durata di un caso. Si crea in un minuto e si rilascia quando il caso si chiude.',
  'comp.holds.emptyAction': 'Crea il primo blocco',
  'comp.col.source': 'Origine',
  'comp.col.subject': 'Interessato',
  'comp.col.case': 'Caso',
  'comp.col.created': 'Creato',
  'comp.col.item': 'Elemento',
  'comp.col.size': 'Dimensione',
  'comp.status.preserving': 'In conservazione',
  'comp.status.released': 'Rilasciato',
  'comp.status.deletedPreserved': 'Eliminato, conservato',
  'comp.release': 'Rilascia',
  'comp.releaseAria': 'Rilascia il blocco di {subject} su {service}',
  'comp.release.title': 'Rilascia il blocco',
  'comp.release.intro': 'Smetterai di conservare {what} di {subject}.',
  'comp.release.point1': 'Torna ad applicarsi l’eliminazione normale dell’account.',
  'comp.release.point2':
    'Ciò che è nel cestino potrà essere eliminato definitivamente e sparirà da eDiscovery.',
  'comp.release.point3':
    'Ciò che è già stato eliminato non si recupera: se il caso è ancora aperto, non rilasciarlo.',
  'comp.release.audited':
    'Il rilascio resta registrato con il tuo nome e la data. Il blocco si conserva nello storico (per vederlo: «Mostra anche quelli rilasciati»).',
  'comp.release.confirm': 'Rilascia e torna all’eliminazione normale',
  'comp.toast.createFailed': 'Non è stato possibile creare',
  'comp.toast.createRejected': 'Né Posta né File hanno accettato il blocco. Riprova.',
  'comp.toast.badSubject':
    'Il servizio ha rifiutato un valore. Controlla chi riguarda nel primo passaggio.',
  'comp.toast.forbidden':
    'Il tuo account non ha l’autorizzazione per creare blocchi in questa organizzazione.',
  'comp.toast.serviceDown': 'Il servizio non risponde in questo momento. Riprova tra un minuto.',
  'comp.toast.reloadFailed': 'Non è stato possibile ricaricare l’elenco',
  'comp.toast.released': 'Blocco rilasciato',
  'comp.toast.releasedHint': '{subject} torna all’eliminazione normale.',
  'comp.toast.releaseFailed': 'Non è stato possibile rilasciare',
  'comp.toast.releaseFailedHint': 'Resta attivo. Riprova tra qualche secondo.',
  'comp.toast.noSubject': 'Manca di chi si tratta',
  'comp.toast.noSubjectHint': 'Scegli nell’elenco la persona di cui esaminerai i contenuti.',
  'comp.toast.searchFailed': 'Non è stato possibile cercare',
  'comp.toast.badFilters': 'Qualche filtro non torna. Controlla le date e riprova.',
  'comp.search.warningTitle': 'Stai per esaminare i contenuti di un’altra persona',
  'comp.search.warningBody':
    'Qui si vedono e si scaricano la posta e i file di qualcuno della tua organizzazione, senza che quella persona lo sappia. Ogni ricerca e ogni download resta registrato con il tuo nome, la data e chi hai esaminato. Usalo solo con un caso alle spalle.',
  'comp.search.title': 'Cerca nei contenuti di una persona',
  'comp.search.description':
    'I risultati combinano posta e file e mostrano solo i dati di intestazione: chi, quando e quanto occupa. Per vedere il contenuto bisogna esportare.',
  'comp.search.where': 'Dove cercare',
  'comp.search.keyword': 'Parola chiave',
  'comp.search.keywordHelp':
    'Nella posta cerca in oggetto, mittente e corpo; nei file, nel nome. Lascia vuoto per vedere tutto.',
  'comp.search.keywordPlaceholder': 'contratto',
  'comp.search.caseHelp': 'Viene salvato nel registro di controllo insieme alla ricerca.',
  'comp.search.bothDown':
    'Né Posta né File rispondono in questo momento: la ricerca non restituirà nulla.',
  'comp.search.oneDown':
    '{service} non risponde in questo momento: mancherà quella metà dei risultati.',
  'comp.search.partialTitle': 'Non è stato possibile cercare in {services}',
  'comp.search.partialBody':
    'Quello che vedi sotto è incompleto. Rilancia la ricerca tra qualche minuto prima di considerare il caso rivisto.',
  'comp.search.searching': 'Ricerca nei contenuti di {subject}…',
  'comp.search.emptyTitle': 'Non hai ancora cercato nulla',
  'comp.search.emptyDescription':
    'Scegli la persona e, se lo sai, restringi il periodo. Più restringi, meno contenuti altrui finisci per vedere.',
  'comp.search.noResultsTitle': 'Nulla corrisponde a quei filtri',
  'comp.search.noResultsDescription':
    'Nessun messaggio né file di quella persona corrisponde. Prova ad ampliare il periodo o a togliere la parola chiave.',
  'comp.search.resultCount': {
    one: '{count} elemento di {subject}.',
    other: '{count} elementi di {subject}.',
  },
  'comp.search.downloadFull': 'Scarica il contenuto completo:',
  'comp.period.range': 'dal {from} al {to}',
  'comp.period.from': 'dal {from}',
  'comp.period.to': 'fino al {to}',
  'comp.period.all': 'di tutto lo storico',
  'comp.export.title': 'Scarica il contenuto',
  'comp.export.intro':
    'Non è un elenco: è il contenuto integrale, leggibile fuori da Sytue. Controlla cosa stai per scaricare.',
  'comp.export.what': 'Cosa si scarica',
  'comp.export.whatMail': 'Tutta la sua posta del periodo, con gli allegati, in un file .mbox',
  'comp.export.whatDrive': 'Tutti i suoi file del periodo, in uno .zip',
  'comp.export.whose': 'Di chi',
  'comp.export.period': 'Periodo',
  'comp.export.textFilter': 'Filtro di testo',
  'comp.export.noFilter': 'Senza filtro (tutto il contenuto)',
  'comp.export.mailBtn': 'Posta (.mbox)',
  'comp.export.driveBtn': 'File (.zip)',
  'comp.export.downloadMbox': 'Scarica il .mbox',
  'comp.export.downloadZip': 'Scarica lo .zip',
  'comp.export.auditedTitle': 'Resta registrato ed esce da Sytue',
  'comp.export.auditedBody':
    'Il download viene annotato nel registro con il tuo nome, la data e di chi è il contenuto. Da lì in poi il file vive sul tuo computer: conservalo dove impone la tua procedura ed eliminalo quando il caso si chiude.',

  // --- Mapa de la suite: quién sirve cada producto (AppMap) ---------------
  'appmap.column.apps': 'Applicazioni connesse',
  'appmap.column.products': 'Prodotti della suite',
  'appmap.dragHint': 'Trascina su un prodotto perché sia questa applicazione a servirlo',
  'appmap.menuAria': 'Chi serve {product}',
  'appmap.nativeHint': 'Il prodotto nativo della suite',
  'appmap.handoff.full': 'Apre l’applicazione e anche ogni documento',
  'appmap.handoff.appOnly': 'Apre l’applicazione; i documenti restano in Sytue',
  'appmap.footer':
    'Scegli chi serve ogni prodotto. La modifica vale per tutta l’organizzazione: il launcher e il selettore di applicazioni apriranno l’applicazione scelta. I dati già in Sytue restano dove sono.',
  'appmap.offline.title': 'Non è stato possibile leggere la mappa',
  'appmap.offline.description':
    'Il gateway delle integrazioni non risponde in questo momento. Riprova tra qualche minuto.',
  'appmap.empty.title': 'Tutti i prodotti li serve Sytue',
  'appmap.empty.description':
    'Quando connetterai un’applicazione che può sostituire un prodotto della suite — Google Workspace, Microsoft 365, Slack… — comparirà qui perché tu possa scegliere.',
  'appmap.toast.switched': '{product} ora lo serve l’applicazione connessa',
  'appmap.toast.reverted': '{product} torna a Sytue',
  'appmap.toast.failed': 'Non è stato possibile cambiare',
  'appmap.toast.failedHint': 'Riprova tra qualche secondo.',

  // --- Asistente: traer contenido (MigrateWizard) --------------------------
  'wizard.migrate.goToApps': 'Vai a Integrazioni',
  'wizard.migrate.empty.title': 'Collega prima l’origine',
  'wizard.migrate.empty.description':
    'Per portare qui i contenuti di Microsoft 365 o Google Workspace bisogna collegarli in Integrazioni. Si fa una volta sola e vale per tutta l’organizzazione.',
  'wizard.migrate.bring.drive': 'File',
  'wizard.migrate.bring.contacts': 'Contatti',
  'wizard.migrate.bring.both': 'File e contatti',
  'wizard.migrate.gets.files':
    'Le cartelle e i file, con il loro nome e nello stesso percorso che avevano.',
  'wizard.migrate.gets.contacts':
    'I contatti con il loro nome, le email, i telefoni e l’organizzazione.',
  'wizard.migrate.nots.googleNative':
    'I documenti nativi di Google (Documenti, Fogli, Presentazioni). Non sono file: vanno esportati prima. Vengono saltati uno a uno e ti diciamo quali.',
  'wizard.migrate.nots.tooBig':
    'I file di oltre 2 GiB. Vengono saltati per intero; non si caricano mai a metà.',
  'wizard.migrate.nots.modifiedDate':
    'La data di modifica originale: in Sytue tutto comparirà con la data di oggi. Il contenuto e il percorso invece si conservano.',
  'wizard.migrate.nots.permissions':
    'Le autorizzazioni e gli elementi condivisi dell’origine: ogni persona riceve il proprio.',
  'wizard.migrate.nots.namelessContacts':
    'I contatti senza alcun nome visibile: Sytue Contacts richiede un nome.',
  'wizard.migrate.nots.mailCalendar':
    'La posta e il calendario. Non si migrano ancora; arriveranno più avanti.',
  'wizard.migrate.error.forbidden':
    'Il tuo account non ha l’autorizzazione per migrare contenuti in questa organizzazione.',
  'wizard.migrate.error.sourceGone':
    'La connessione di origine non esiste più. Torna a Integrazioni e controllala.',
  'wizard.migrate.error.generic':
    'Non è stato possibile avviare la migrazione. Riprova tra qualche secondo.',
  'wizard.migrate.step.origin': 'Origine',
  'wizard.migrate.step.what': 'Cosa portare',
  'wizard.migrate.step.who': 'Per chi',
  'wizard.migrate.step.limits': 'Cosa arriva',
  'wizard.migrate.step.review': 'Riepilogo',
  'wizard.migrate.origin.heading': 'Da dove portiamo i contenuti?',
  'wizard.migrate.origin.description':
    'Usiamo la connessione che hai già configurato in Integrazioni. Non c’è nulla da installare.',
  'wizard.migrate.origin.noneTitle': 'Non c’è ancora nessuna connessione di origine',
  'wizard.migrate.origin.noneBody':
    'Per portare qui i contenuti bisogna prima collegare Microsoft 365 o Google Workspace in Integrazioni. È una procedura guidata di cinque passaggi e si fa una volta sola.',
  'wizard.migrate.origin.connection':
    'Leggeremo i contenuti con la connessione «{name}», quella che la tua organizzazione già usa.',
  'wizard.migrate.what.heading': 'Che cosa vuoi portare?',
  'wizard.migrate.what.description':
    'Il resto puoi avviarlo più avanti: ogni cosa procede per conto suo.',
  'wizard.migrate.what.bothHint':
    'Il trasloco tipico: la cartella personale e la rubrica di ogni persona.',
  'wizard.migrate.what.driveHint':
    'Le cartelle e i file dell’unità personale, con la stessa struttura.',
  'wizard.migrate.what.contactsHint': 'La rubrica personale: nomi, email e telefoni.',
  'wizard.migrate.what.mostCommon': 'Il più comune',
  'wizard.migrate.who.heading': 'A chi portiamo i suoi contenuti?',
  'wizard.migrate.who.description':
    'Ogni persona riceve il proprio NEL SUO ACCOUNT Sytue. Puoi sceglierne una o più.',
  'wizard.migrate.who.searchPlaceholder': 'Cerca per nome o email',
  'wizard.migrate.who.searchAria': 'Cerca persone',
  'wizard.migrate.who.loadedHint': {
    one: 'Cerchiamo tra la {count} persona già caricata della tua organizzazione. Se chi cerchi non compare, caricane altre qui sotto.',
    other:
      'Cerchiamo tra le {count} persone già caricate della tua organizzazione. Se chi cerchi non compare, caricane altre qui sotto.',
  },
  'wizard.migrate.who.loading': 'Caricamento della rubrica…',
  'wizard.migrate.who.noMatch': 'Nessuno corrisponde a questa ricerca.',
  'wizard.migrate.who.suspended': 'Sospeso',
  'wizard.migrate.who.loadMore': 'Carica altre persone',
  'wizard.migrate.who.pickHelp':
    'Seleziona a chi vuoi migrare i contenuti. Se siete in pochi, comincia da una persona e verifica il risultato prima di avviare il resto.',
  'wizard.migrate.who.sourceTitle': {
    one: 'Da quale account si copia ({count} persona)',
    other: 'Da quali account si copia ({count} persone)',
  },
  'wizard.migrate.who.sourceHint':
    'Mettiamo la sua email Sytue come predefinita. Cambiala se in {product} aveva un altro indirizzo.',
  'wizard.migrate.who.sourceAria': 'Account di origine di {name}',
  'wizard.migrate.who.missingSource': {
    one: 'Manca l’account di origine di {count} persona.',
    other: 'Manca l’account di origine di {count} persone.',
  },
  'wizard.migrate.limits.heading': 'Questo è ciò che arriva e questo è ciò che non arriva',
  'wizard.migrate.limits.description':
    'Preferiamo dirtelo adesso invece di lasciarti una sorpresa fra due ore.',
  'wizard.migrate.limits.yes': 'Arriva',
  'wizard.migrate.limits.no': 'Non arriva',
  'wizard.migrate.limits.safeTitle': 'Nell’origine non si perde nulla',
  'wizard.migrate.limits.safeBody':
    'Leggiamo soltanto. Il tuo Microsoft 365 o il tuo Google Workspace restano esattamente come sono, e puoi riavviare la migrazione: ciò che è già copiato non viene duplicato.',
  'wizard.migrate.summary.source': '{product} · {name}',
  'wizard.migrate.summary.what': 'Cosa si porta',
  'wizard.migrate.summary.who': 'Per chi',
  'wizard.migrate.summary.whoOne': '{name} ({email})',
  'wizard.migrate.summary.whoMany': {
    one: '{count} persona, nel proprio account',
    other: '{count} persone, ciascuna nel proprio account',
  },
  'wizard.migrate.summary.jobs': 'Lavori che vengono avviati',
  'wizard.migrate.summary.jobsValue': {
    one: '{count} (uno per persona e tipo di contenuto)',
    other: '{count} (uno per persona e tipo di contenuto)',
  },
  'wizard.migrate.summary.skipped': 'Verrà saltato',
  'wizard.migrate.summary.skippedValue': {
    one: '{count} cosa, con il suo motivo nel rapporto',
    other: '{count} cose, con il loro motivo nel rapporto',
  },
  'wizard.migrate.summary.skippedNone': 'Nulla',
  'wizard.migrate.review.heading': 'Controlla e avvia',
  'wizard.migrate.review.description':
    'Una volta avviata, la copia procede in background. Puoi chiudere questa pagina.',
  'wizard.migrate.review.launch': {
    one: 'Migra i contenuti',
    other: 'Migra per {count} persone',
  },
  'wizard.migrate.review.failedTitle': 'Non è stato possibile avviare',
  'wizard.migrate.review.ownAccountTitle': 'Ogni persona riceve il proprio nel SUO account',
  'wizard.migrate.review.ownAccountBody':
    'I contenuti non passano mai dal tuo account. Sytue chiede il permesso di scrivere per conto di ogni persona, quel permesso dura pochi minuti e resta registrato nel registro di controllo.',

  // --- Asistente: etiqueta de confidencialidad (SensitivityLabelWizard) -----
  'wizard.label.color.none': 'Senza colore',
  'wizard.label.color.grey': 'Grigio',
  'wizard.label.color.blue': 'Blu',
  'wizard.label.color.green': 'Verde',
  'wizard.label.color.amber': 'Ambra',
  'wizard.label.color.red': 'Rosso',
  'wizard.label.toast.saveFailed': 'Non è stato possibile salvare',
  'wizard.label.toast.nameTaken': 'Hai già un’etichetta con quel nome. Scegline un altro.',
  'wizard.label.toast.retry': 'Riprova tra qualche secondo.',
  'wizard.label.step.what': 'Che cos’è',
  'wizard.label.step.level': 'Livello',
  'wizard.label.step.effect': 'Che cosa impedisce',
  'wizard.label.step.review': 'Riepilogo',
  'wizard.label.step.done': 'Fatto',
  'wizard.label.what.heading': 'Come si chiama e a che cosa serve?',
  'wizard.label.what.description':
    'Il nome e la spiegazione li vedranno tutti al momento di etichettare un file. Scrivili come li racconteresti a chi comincia oggi.',
  'wizard.label.what.nameLabel': 'Nome dell’etichetta',
  'wizard.label.what.namePlaceholder': 'Riservato',
  'wizard.label.what.nameHint':
    'Corto e riconoscibile. I più comuni: Pubblico, Interno, Riservato, Ad accesso limitato.',
  'wizard.label.what.purposeLabel': 'A che cosa serve',
  'wizard.label.what.purposePlaceholder':
    'Informazioni aziendali che non devono uscire dall’azienda: contratti, buste paga, dati dei clienti.',
  'wizard.label.what.purposeHint':
    'Facoltativa, ma molto consigliata: è l’indizio che decide se qualcuno la usa bene.',
  'wizard.label.what.colourLegend': 'Colore (facoltativo)',
  'wizard.label.what.colourHint': 'Serve solo a riconoscerla a colpo d’occhio negli elenchi.',
  'wizard.label.level.heading': 'Quanto è riservata rispetto alle altre?',
  'wizard.label.level.description':
    'Collocala nella tua scala. Serve solo a ordinarle dalla meno alla più delicata; da sola non cambia ciò che si può fare con i file.',
  'wizard.label.level.firstTitle': 'È la tua prima etichetta',
  'wizard.label.level.firstBody':
    'Quando ne creerai altre, potrai collocarle sopra o sotto questa.',
  'wizard.label.level.first': 'È la prima etichetta della tua organizzazione',
  'wizard.label.level.lowest': 'La meno riservata di tutte (sotto «{name}»)',
  'wizard.label.level.highest': 'La più riservata di tutte (sopra «{name}»)',
  'wizard.label.level.between': 'Tra «{lower}» e «{upper}»',
  'wizard.label.level.hintLowest': 'La useranno per le cose più aperte.',
  'wizard.label.level.hintHighest': 'La useranno per le cose più delicate.',
  'wizard.label.level.hintMiddle': 'Un gradino intermedio.',
  'wizard.label.level.scale': 'La tua scala attuale, dalla meno alla più riservata: {scale}.',
  'wizard.label.level.scaleEmpty': 'ancora vuota',
  'wizard.label.effect.heading': 'Che cosa deve impedire questa etichetta?',
  'wizard.label.effect.description':
    'Scegli in base a ciò che succede davvero quando qualcuno prova a condividere il file.',
  'wizard.label.effect.informTitle': 'Solo informa',
  'wizard.label.effect.informBody':
    'L’etichetta si vede accanto al file e serve perché le persone sappiano che cosa hanno per le mani, ma non impedisce nulla. È l’opzione per «Pubblico» o «Interno».',
  'wizard.label.effect.blockTitle': 'Impedisce di condividere fuori dall’organizzazione',
  'wizard.label.effect.blockBody':
    'Nessuno potrà creare un link pubblico per un file con questa etichetta, e chi ci prova ne vedrà il motivo. I link già esistenti smetteranno di aprire quei file. La condivisione con persone o gruppi della tua organizzazione continua a funzionare come prima.',
  'wizard.label.effect.warnTitle': 'Prima di attivarlo',
  'wizard.label.effect.warnBody':
    'Se il tuo team condivide già questi file con clienti o fornitori tramite link, avvisali: appena etichetti un file, il suo link smette di funzionare.',
  'wizard.label.effect.summaryInform': 'Solo informa: non impedisce nulla',
  'wizard.label.effect.summaryBlock': 'Impedisce di condividere il file fuori dall’organizzazione',
  'wizard.label.summary.name': 'Nome',
  'wizard.label.summary.purpose': 'A che cosa serve',
  'wizard.label.summary.noPurpose': 'Senza spiegazione',
  'wizard.label.summary.level': 'Livello',
  'wizard.label.summary.effect': 'Che cosa impedisce',
  'wizard.label.review.headingNew': 'Controlla e crea l’etichetta',
  'wizard.label.review.headingEdit': 'Controlla le modifiche',
  'wizard.label.review.description':
    'Questo è ciò che salveremo. Puoi cambiare qualsiasi cosa prima.',
  'wizard.label.review.saveNew': 'Crea l’etichetta',
  'wizard.label.review.saveEdit': 'Salva le modifiche',
  'wizard.label.review.editNoteTitle': 'I file già etichettati non cambiano',
  'wizard.label.review.editNoteBody':
    'Un file conserva l’etichetta così com’era quando gliel’hanno messa. Se cambi il nome o ciò che impedisce, quello che è già etichettato resta com’era: così un file classificato un anno fa si continua a leggere allo stesso modo. Per aggiornarlo, etichettalo di nuovo.',
  'wizard.label.review.newNoteTitle': 'Che cosa succede dopo',
  'wizard.label.review.newNoteBody':
    'L’etichetta comparirà tra le scelte al momento di classificare i file in Sytue Drive. Etichettare è manuale: nulla si classifica da solo.',
  'wizard.label.done.headingNew': 'Etichetta creata',
  'wizard.label.done.headingEdit': 'Modifiche salvate',
  'wizard.label.done.bodyBlock':
    'Si può già scegliere al momento di classificare i file. Quelli che la portano non si potranno condividere con un link pubblico.',
  'wizard.label.done.bodyInform':
    'Si può già scegliere al momento di classificare i file. Solo informa: non impedisce di condividere.',
  'wizard.label.done.back': 'Torna alle etichette',

  // --- Asistente: conectar proveedor de identidad (ConnectIdpWizard) --------
  'wizard.idp.kind.oidcBlurb':
    'Quello che usano quasi tutti i provider di oggi. Ti danno un indirizzo e due chiavi, e basta.',
  'wizard.idp.kind.oidcExamples': 'Microsoft Entra ID · Google Workspace · Okta · Auth0 · Keycloak',
  'wizard.idp.kind.samlBlurb':
    'Lo standard classico delle aziende. Il tuo provider ti dà un XML di metadati (o un certificato e due indirizzi).',
  'wizard.idp.kind.samlExamples': 'AD FS · Okta o OneLogin in modalità SAML · provider più datati',
  'wizard.idp.kind.mostCommon': 'Il più comune',
  'wizard.idp.kind.bothTitle': 'Il tuo provider te li offre entrambi?',
  'wizard.idp.kind.bothBody':
    'Scegli OpenID Connect: si configura con meno dati e non ci sono certificati da rinnovare ogni pochi anni.',
  'wizard.idp.effect.signInTitle': 'Accedono con l’account aziendale',
  'wizard.idp.effect.signInBody':
    'Il tuo team userà il pulsante «Accedi con la tua organizzazione» e la solita password aziendale. Una in meno da ricordare.',
  'wizard.idp.effect.noLossTitle': 'Nessuno perde l’accesso',
  'wizard.idp.effect.noLossBody':
    'Chi ha già una password Sytue può continuare a usarla. Questo aggiunge una via d’accesso, non la sostituisce.',
  'wizard.idp.effect.whoTitle': 'Decidi tu chi passa',
  'wizard.idp.effect.whoBody':
    'Puoi limitare l’accesso ai domini di posta della tua organizzazione, anche se il tuo provider ha più persone.',
  'wizard.idp.effect.offTitle': 'Non si attiva finché non lo dici tu',
  'wizard.idp.effect.offBody':
    'Lo configuri, lo provi e lo attivi alla fine. Fino ad allora nessuno lo vede nella schermata di accesso.',
  'wizard.idp.need.samlXml': 'I metadati del provider (un file XML)',
  'wizard.idp.need.samlXmlWhere':
    'Nell’applicazione SAML del tuo provider: «Scarica XML metadati di federazione» in Microsoft Entra ID, «Identity Provider metadata» in Okta. È la via rapida: lo incolli e il resto lo compiliamo noi.',
  'wizard.idp.need.samlManual': 'Oppure: Entity ID, URL di accesso e certificato',
  'wizard.idp.need.samlManualWhere':
    'Sono nella stessa schermata, uno per uno, se il tuo provider non permette di scaricare l’XML. Il certificato è quello di firma del provider (inizia con «-----BEGIN CERTIFICATE-----»).',
  'wizard.idp.need.domains': 'I domini di posta della tua organizzazione',
  'wizard.idp.need.domainsWhere':
    'Quelli degli indirizzi del tuo team, per esempio acme.com. Puoi indicarne più di uno.',
  'wizard.idp.need.issuer': 'L’indirizzo del tuo provider (issuer)',
  'wizard.idp.need.issuerWhere':
    'Microsoft Entra ID: la tua registrazione dell’applicazione → «Endpoint» → copia il documento di metadati OpenID Connect e togli la parte finale «/.well-known/openid-configuration». Google Workspace: è https://accounts.google.com. Okta: https://tua-azienda.okta.com.',
  'wizard.idp.need.client': 'L’ID client e il suo segreto',
  'wizard.idp.need.clientWhere':
    'Si creano registrando un’applicazione presso il tuo provider (in Entra ID, «Registrazioni app» → Nuova registrazione → «Certificati e segreti»). Copia il segreto appena lo crei: quasi tutti lo mostrano una sola volta.',
  'wizard.idp.err.issuerRequired': 'Ci serve l’indirizzo del tuo provider.',
  'wizard.idp.err.hasSpace': 'Si è infilato uno spazio. Incolla solo l’indirizzo.',
  'wizard.idp.err.wellKnown':
    'Togli la parte finale «/.well-known/openid-configuration»: qui va solo la parte che la precede.',
  'wizard.idp.err.insecure': 'Deve iniziare con https:// (con quel http:// non è sicuro).',
  'wizard.idp.err.needHttps': 'Scrivilo per intero, iniziando con https://',
  'wizard.idp.err.noDomain': 'Manca il dominio: controlla l’indirizzo.',
  'wizard.idp.err.badIssuerUrl':
    'Questo indirizzo non è valido. Controllalo: di solito è un errore di battitura.',
  'wizard.idp.err.urlRequired': 'Ci serve questo indirizzo.',
  'wizard.idp.err.mustBeHttps': 'Deve essere un indirizzo https://',
  'wizard.idp.err.badUrl': 'Questo indirizzo non è valido. Controllalo.',
  'wizard.idp.err.badDomain':
    'Questo non sembra un dominio: «{domain}». Scrivilo senza «@» né «https://», solo acme.com.',
  'wizard.idp.err.certRequired': 'Ci serve il certificato di firma del provider.',
  'wizard.idp.err.notCert':
    'Questo non sembra un certificato. Incollalo così come te lo dà il tuo provider, comprese le righe «-----BEGIN CERTIFICATE-----».',
  'wizard.idp.err.nameRequired': 'Dagli un nome per riconoscerlo.',
  'wizard.idp.err.slugRequired': 'Ci serve un identificatore.',
  'wizard.idp.err.slugFormat': 'Solo minuscole, numeri e trattini (senza spazi né accenti).',
  'wizard.idp.err.slugTaken': 'C’è già un altro provider con questo identificatore.',
  'wizard.idp.err.clientIdRequired': 'Ci serve l’ID client.',
  'wizard.idp.err.xmlRequired': 'Incolla qui i metadati che scarichi dal provider.',
  'wizard.idp.err.xmlFormat':
    'Questo non sembra l’XML dei metadati: deve contenere «EntityDescriptor». Incollalo per intero, così come te lo dà il tuo provider.',
  'wizard.idp.err.entityRequired': 'Ci serve l’Entity ID.',
  'wizard.idp.fix.samlData':
    'I dati del provider non tornano. Controlla l’Entity ID, che l’URL di accesso inizi con https:// e che il certificato sia completo (con le righe BEGIN ed END).',
  'wizard.idp.fix.oidcDiscovery':
    'Non siamo riusciti a leggere la configurazione a quell’indirizzo. Quasi sempre è l’indirizzo del provider: copialo di nuovo dal tuo provider e togli la parte finale «/.well-known/openid-configuration».',
  'wizard.idp.fix.slugTaken':
    'C’è già un altro provider con quell’identificatore nella tua organizzazione. Cambia l’identificatore nel passaggio precedente.',
  'wizard.idp.fix.forbidden':
    'Il tuo account non ha l’autorizzazione per configurare l’accesso in questa organizzazione.',
  'wizard.idp.fix.idpDown':
    'Sytue ID non risponde in questo momento. Non dipende dai tuoi dati: riprova tra un minuto.',
  'wizard.idp.fix.generic':
    'Non siamo riusciti a verificarlo. Ricontrolla i dati del passaggio precedente e riprova.',
  'wizard.idp.step.type': 'Tipo',
  'wizard.idp.step.explain': 'Cosa succede',
  'wizard.idp.step.configure': 'Configurare',
  'wizard.idp.step.test': 'Provare',
  'wizard.idp.step.review': 'Riepilogo',
  'wizard.idp.step.done': 'Fatto',
  'wizard.idp.type.heading': 'Con che cosa si collega il tuo provider di identità?',
  'wizard.idp.type.description':
    'È l’unica cosa tecnica che ti chiediamo. Se non lo sai, cerca il nome del tuo provider negli esempi.',
  'wizard.idp.explain.heading': 'Ecco cosa cambierà per il tuo team',
  'wizard.idp.explain.description':
    'Niente di tutto questo si applica ancora: prima lo configuri e lo provi.',
  'wizard.idp.explain.needTitle': 'Tieni questo a portata di mano prima di continuare',
  'wizard.idp.configure.headingOidc': 'Dati del tuo provider OIDC',
  'wizard.idp.configure.headingSaml': 'Dati del tuo provider SAML',
  'wizard.idp.configure.description':
    'Sotto ogni campo ti diciamo dove trovarlo, e controlliamo quello che scrivi al volo.',
  'wizard.idp.field.name': 'Nome',
  'wizard.idp.field.nameHint':
    'È quello che il tuo team vedrà sul pulsante di accesso. Va bene il nome del tuo provider.',
  'wizard.idp.field.slug': 'Identificatore',
  'wizard.idp.field.slugHint':
    'Nome breve interno, si usa negli indirizzi di accesso. Te ne proponiamo uno a partire dal nome.',
  'wizard.idp.field.issuer': 'Indirizzo del provider (issuer)',
  'wizard.idp.field.issuerHint':
    'L’indirizzo di base, senza «/.well-known/openid-configuration» alla fine. Google Workspace: https://accounts.google.com',
  'wizard.idp.field.issuerOk': 'Ha una buona forma. Lo verificheremo nel passaggio successivo.',
  'wizard.idp.field.clientId': 'ID client',
  'wizard.idp.field.clientIdHint':
    'Te lo dà il tuo provider quando registri l’applicazione («Application (client) ID» in Entra ID).',
  'wizard.idp.field.secret': 'Segreto client',
  'wizard.idp.field.secretPlaceholder': '•••••••• (salvato, invariato)',
  'wizard.idp.field.secretKeepHint': 'Lascialo vuoto per conservare quello che hai già salvato.',
  'wizard.idp.field.secretHint':
    'Lascialo vuoto solo se il tuo provider non te ne dà nessuno (applicazione pubblica con PKCE).',
  'wizard.idp.field.secretStored':
    'Si salva cifrato e non viene più mostrato, né a te né a noi. Se un giorno scade, ne scrivi uno nuovo qui e basta.',
  'wizard.idp.field.xml': 'Metadati del provider (XML)',
  'wizard.idp.field.xmlHint':
    'Scaricalo dal tuo provider («XML metadati di federazione») e incollalo per intero. Il resto lo estraiamo noi.',
  'wizard.idp.field.xmlOk': 'Ha una buona forma. Da qui prendiamo l’indirizzo e il certificato.',
  'wizard.idp.field.noXmlLink': 'Il mio provider non mi dà l’XML',
  'wizard.idp.field.entity': 'Entity ID del provider',
  'wizard.idp.field.entityHint':
    'Lo chiamano anche «Identificatore di Azure AD» o «Issuer». Di solito sembra un indirizzo web.',
  'wizard.idp.field.ssoUrl': 'URL di accesso del provider',
  'wizard.idp.field.ssoUrlHint':
    'L’indirizzo a cui manderemo il tuo team per identificarsi («SAML SSO endpoint»).',
  'wizard.idp.field.cert': 'Certificato di firma',
  'wizard.idp.field.certHint':
    'È pubblico: serve a verificare che la risposta venga davvero dal tuo provider. Scaricalo dalla stessa schermata.',
  'wizard.idp.field.useXmlLink': 'Preferisco incollare l’XML dei metadati',
  'wizard.idp.field.domains': 'Domini di posta consentiti',
  'wizard.idp.field.domainsHint':
    'Entrerà solo chi ha la posta in questi domini. Separali con virgole. Vuoto = chiunque il tuo provider lasci passare.',
  'wizard.idp.field.advanced': 'Opzioni avanzate',
  'wizard.idp.field.advancedHint':
    'Non serve toccare niente di tutto questo, a meno che il tuo provider usi nomi diversi da quelli standard.',
  'wizard.idp.field.scopes': 'Autorizzazioni richieste (scope)',
  'wizard.idp.field.scopesHint': 'Separate da spazi. «openid» è obbligatorio.',
  'wizard.idp.mapping.legend': 'Da dove prendiamo i dati di ogni persona',
  'wizard.idp.mapping.hint':
    'Lascia i campi vuoti e useremo i nomi standard (email, name, groups).',
  'wizard.idp.mapping.emailAria': 'Nome dell’attributo dell’indirizzo e-mail',
  'wizard.idp.mapping.nameAria': 'Nome dell’attributo del nome completo',
  'wizard.idp.mapping.groupsAria': 'Nome dell’attributo dei gruppi',
  'wizard.idp.test.headingOidc': 'Verifichiamo che Sytue arrivi al tuo provider',
  'wizard.idp.test.headingSaml': 'Verifichiamo i dati del tuo provider',
  'wizard.idp.test.descriptionOidc':
    'Chiediamo al tuo provider la sua configurazione. Nessuno accede ancora.',
  'wizard.idp.test.descriptionSaml':
    'Controlliamo l’Entity ID, l’indirizzo di accesso e il certificato. Nessuno accede ancora.',
  'wizard.idp.test.okTitle': 'Tutto corretto',
  'wizard.idp.test.okOidc': 'Il tuo provider ci ha risposto e abbiamo letto la sua configurazione.',
  'wizard.idp.test.okOidcPkce':
    'Il tuo provider ci ha risposto e abbiamo letto la sua configurazione (con PKCE, la modalità più sicura).',
  'wizard.idp.test.okSaml':
    'I dati del provider sono validi: il certificato si legge bene e l’indirizzo di accesso è corretto.',
  'wizard.idp.test.failTitle': 'Non siamo riusciti a verificarlo',
  'wizard.idp.test.reviewData': 'Rivedere i dati',
  'wizard.idp.test.idleHelp':
    'Premi «Verifica» e controlliamo i dati che hai inserito. Se qualcosa non torna, ti diciamo esattamente cosa cambiare.',
  'wizard.idp.test.run': 'Verifica',
  'wizard.idp.test.rerun': 'Verifica di nuovo',
  'wizard.idp.test.skipHint':
    'Puoi continuare senza verificare, ma è meglio scoprire ora un errore di battitura.',
  'wizard.idp.test.draftNote':
    'Per poterlo verificare l’abbiamo salvato disattivato. Nessuno può entrare da lì finché non lo attivi nel passaggio successivo.',
  'wizard.idp.summary.type': 'Tipo di connessione',
  'wizard.idp.summary.name': 'Nome',
  'wizard.idp.summary.slug': 'Identificatore',
  'wizard.idp.summary.issuer': 'Indirizzo del provider',
  'wizard.idp.summary.clientId': 'ID client',
  'wizard.idp.summary.secret': 'Segreto client',
  'wizard.idp.summary.secretSaved': 'Salvato (non si mostra)',
  'wizard.idp.summary.secretKept': 'Si conserva quello che avevi già',
  'wizard.idp.summary.secretNone': 'Senza segreto (PKCE)',
  'wizard.idp.summary.samlData': 'Dati del provider',
  'wizard.idp.summary.fromXml': 'Li leggiamo dall’XML dei metadati che hai incollato',
  'wizard.idp.summary.entityValue': 'Entity ID {id}',
  'wizard.idp.summary.ssoUrl': 'URL di accesso',
  'wizard.idp.summary.cert': 'Certificato di firma',
  'wizard.idp.summary.certPasted': 'Incollato',
  'wizard.idp.summary.who': 'Chi può accedere',
  'wizard.idp.summary.whoDomains': 'Solo indirizzi di {domains}',
  'wizard.idp.summary.whoAny': 'Chiunque il tuo provider lasci passare',
  'wizard.idp.summary.test': 'Verifica',
  'wizard.idp.summary.testOk': 'Superata',
  'wizard.idp.summary.testFail': 'Fallita (puoi salvare lo stesso e sistemarlo dopo)',
  'wizard.idp.summary.testNone': 'Non verificata',
  'wizard.idp.review.headingNew': 'Rivedi e salva',
  'wizard.idp.review.headingEdit': 'Rivedi le modifiche',
  'wizard.idp.review.description':
    'Ecco cosa applicheremo. Puoi cambiare qualsiasi dato prima di continuare.',
  'wizard.idp.review.saveEdit': 'Salva le modifiche',
  'wizard.idp.review.saveEnable': 'Salva e attiva',
  'wizard.idp.review.save': 'Salva',
  'wizard.idp.review.enableLabel': 'Consenti subito l’accesso con questo provider',
  'wizard.idp.review.enableOn':
    'Nella schermata di accesso del tuo team comparirà il pulsante «Accedi con la tua organizzazione».',
  'wizard.idp.review.enableOff':
    'Verrà salvato disattivato: nessuno lo vede finché non lo attivi dall’elenco.',
  'wizard.idp.review.pendingTitle': 'Resta un passaggio nel TUO provider',
  'wizard.idp.review.pendingOidc':
    'Al salvataggio ti daremo l’indirizzo di ritorno da registrare presso il tuo provider. Senza di esso l’accesso non funzionerà.',
  'wizard.idp.review.pendingSaml':
    'Al salvataggio ti daremo l’URL di risposta (ACS) e l’identificatore di Sytue, da registrare nella tua applicazione SAML. Senza di essi l’accesso non funzionerà.',
  'wizard.idp.done.headingEdit': 'Modifiche salvate',
  'wizard.idp.done.headingNew': '{name} è pronto',
  'wizard.idp.done.fallbackName': 'Il tuo provider',
  'wizard.idp.done.savedTitle': 'Salvato',
  'wizard.idp.done.savedOn':
    'Il provider è attivo. Appena completi il passaggio qui sotto, il tuo team potrà accedere con l’account aziendale.',
  'wizard.idp.done.savedOff':
    'Il provider è salvato ma disattivato. Attivalo dall’elenco quando avrai finito il passaggio qui sotto.',
  'wizard.idp.done.pasteTitle': 'Incolla questo nella configurazione del tuo provider',
  'wizard.idp.done.pasteOidc':
    'Il tuo provider riporterà il team a Sytue solo se questo indirizzo è registrato nell’applicazione che hai creato.',
  'wizard.idp.done.pasteSaml':
    'Registralo nella tua applicazione SAML: senza questo il tuo provider non sa dove rispondere.',
  'wizard.idp.done.redirectUri': 'URL di reindirizzamento',
  'wizard.idp.done.acsUrl': 'URL di risposta (ACS)',
  'wizard.idp.done.spEntityId': 'Identificatore di Sytue (Entity ID)',
  'wizard.idp.done.metadataUrl': 'Metadati di Sytue',
  'wizard.idp.done.nextTitle': 'Cosa fare adesso',
  'wizard.idp.done.step1Title': 'Registra i dati qui sopra presso il tuo provider',
  'wizard.idp.done.step1Oidc':
    'Nella tua registrazione dell’applicazione, nella sezione URI di reindirizzamento (tipo web). Incollalo così com’è, senza barra finale in più.',
  'wizard.idp.done.step1Saml':
    'Nella tua applicazione SAML, in «URL di risposta» e «Identificatore». Incollali così come sono.',
  'wizard.idp.done.step2Title': 'Provalo con il tuo account',
  'wizard.idp.done.step2Body':
    'Apri una finestra di navigazione privata, vai alla schermata di accesso di Sytue e usa «Accedi con la tua organizzazione». Se entri senza che ti chiediamo la password di Sytue, è configurato bene.',
  'wizard.idp.done.step3Title': 'Avvisa il tuo team',
  'wizard.idp.done.step3Body':
    'Da ora accedono con l’account aziendale. Chi aveva già una password Sytue può continuare a usarla.',
  'wizard.idp.done.back': 'Torna ai provider',

  // --- Asistente: registrar aplicación que entra con Sytue (RegisterOidcAppWizard) ---
  'wizard.oidc.kind.serverTitle': 'Un’applicazione con server proprio',
  'wizard.oidc.kind.serverBlurb':
    'L’applicazione gira su un server e può custodire una password. Gliene daremo una.',
  'wizard.oidc.kind.serverExamples':
    'Grafana · GitLab · Jenkins · Nextcloud · uno strumento interno con backend',
  'wizard.oidc.kind.browserTitle': 'Un’applicazione che gira nel browser o sul cellulare',
  'wizard.oidc.kind.browserBlurb':
    'Tutto il suo codice sta sul dispositivo di chi la usa, quindi non può custodire segreti. Userà PKCE, altrettanto sicuro in questo caso.',
  'wizard.oidc.kind.browserExamples':
    'Un sito fatto in React o Vue · un’app iOS o Android · un’app desktop',
  'wizard.oidc.kind.mostCommon': 'Il più comune',
  'wizard.oidc.effect.signInTitle': 'Il tuo team accede con il proprio account Sytue',
  'wizard.oidc.effect.signInBody':
    'Nell’applicazione comparirà un pulsante di accesso che porta a Sytue. Nessuno ha bisogno di una password a parte.',
  'wizard.oidc.effect.controlTitle': 'L’accesso lo controlli da qui',
  'wizard.oidc.effect.controlBody':
    'Se qualcuno lascia l’organizzazione, smette di entrare nell’applicazione. E puoi togliere l’accesso all’intera applicazione quando vuoi.',
  'wizard.oidc.effect.minimalTitle': 'L’applicazione vede solo l’essenziale',
  'wizard.oidc.effect.minimalBody':
    'Nome, e-mail e identificatore. Nient’altro: né i tuoi documenti, né la tua posta, né i tuoi file.',
  'wizard.oidc.err.hasSpace': '«{url}» contiene uno spazio. Incolla solo l’indirizzo.',
  'wizard.oidc.err.noWildcards':
    'Non si accettano caratteri jolly (*). Scrivi l’indirizzo esatto: è ciò che impedisce a un terzo di infilarsi.',
  'wizard.oidc.err.noFragment':
    'Togli tutto quello che segue «#»: quella parte non arriva al server e qui non vale.',
  'wizard.oidc.err.notUrl':
    '«{url}» non è un indirizzo valido. Deve iniziare con https:// e comprendere il dominio.',
  'wizard.oidc.err.insecure':
    'Deve iniziare con https:// (http:// si accetta solo su localhost, per le prove).',
  'wizard.oidc.err.notWeb': '«{url}» non è un indirizzo web. Deve iniziare con https://',
  'wizard.oidc.err.noDomain': 'A «{url}» manca il dominio. Controllalo.',
  'wizard.oidc.err.redirectRequired': 'Ci serve almeno un indirizzo di ritorno.',
  'wizard.oidc.err.nameRequired': 'Dalle un nome per riconoscerla nell’elenco.',
  'wizard.oidc.fix.badUrls':
    'Sytue ID ha rifiutato uno degli indirizzi. Ricontrollali: devono iniziare con https://, senza «#» né caratteri jolly.',
  'wizard.oidc.fix.forbidden':
    'Il tuo account non ha l’autorizzazione per registrare applicazioni in questa organizzazione.',
  'wizard.oidc.fix.idpDown':
    'Sytue ID non risponde in questo momento. Non dipende dai tuoi dati: riprova tra un minuto.',
  'wizard.oidc.fix.generic': 'Non siamo riusciti a registrarla. Ricontrolla i dati e riprova.',
  'wizard.oidc.toast.saveFailed': 'Non è stato possibile registrarla',
  'wizard.oidc.step.app': 'L’applicazione',
  'wizard.oidc.step.urls': 'Indirizzi',
  'wizard.oidc.step.review': 'Riepilogo',
  'wizard.oidc.step.done': 'Fatto',
  'wizard.oidc.app.heading': 'Quale applicazione vuoi collegare?',
  'wizard.oidc.app.description':
    'Così il tuo team potrà entrarci con il proprio account Sytue, senza un’altra password.',
  'wizard.oidc.app.nameLabel': 'Nome dell’applicazione',
  'wizard.oidc.app.namePlaceholder': 'Grafana del team dati',
  'wizard.oidc.app.nameHint':
    'Solo perché tu la riconosca qui. Puoi usare il nome che usa il tuo team.',
  'wizard.oidc.app.whereLegend': 'Dove viene eseguita?',
  'wizard.oidc.app.whereHint':
    'Se hai dubbi, scegli la prima: è il caso normale degli strumenti installati su un server.',
  'wizard.oidc.urls.heading': 'Dove torna il tuo team dopo essersi identificato?',
  'wizard.oidc.urls.description':
    'Questo indirizzo te lo dice l’applicazione: cercalo come «URL di reindirizzamento», «Redirect URI» o «Callback URL» nella schermata di configurazione dell’accesso.',
  'wizard.oidc.urls.redirectLabel': 'URL di reindirizzamento',
  'wizard.oidc.urls.redirectOk': {
    one: 'L’indirizzo ha una buona forma.',
    other: 'Gli indirizzi hanno una buona forma.',
  },
  'wizard.oidc.urls.redirectHint':
    'Uno per riga, se l’applicazione ne ha più di uno (per esempio uno di prova).',
  'wizard.oidc.urls.slashTitle': 'L’errore più comune: una barra in più o in meno',
  'wizard.oidc.urls.slashBody':
    'Confrontiamo l’indirizzo carattere per carattere. Se l’applicazione invia «…/callback» e qui scrivi «…/callback/», l’accesso fallirà con «redirect_uri non registrato». Copialo così com’è dall’applicazione, senza aggiungere né togliere nulla. È voluto: quel confronto esatto è ciò che impedisce a qualcuno di dirottare l’accesso altrove.',
  'wizard.oidc.urls.advanced': 'Opzioni avanzate',
  'wizard.oidc.google.title': 'Accesso ai dati come un account Google',
  'wizard.oidc.google.body':
    'Per le app scritte per Google Calendar, Drive, Contatti o la directory di Workspace: chiedono le autorizzazioni di Google e ricevono i dati di Sytue. Google non interviene e non vede nulla. Ogni persona che accede all’app le concede questo accesso ai propri dati.',
  'wizard.oidc.google.calendar': 'Calendario',
  'wizard.oidc.google.drive': 'Drive',
  'wizard.oidc.google.contacts': 'Contatti',
  'wizard.oidc.google.none': 'Nessun accesso',
  'wizard.oidc.google.read': 'Sola lettura',
  'wizard.oidc.google.write': 'Lettura e scrittura',
  'wizard.oidc.google.directory': 'Directory dell’organizzazione',
  'wizard.oidc.google.directoryUsers': 'Persone',
  'wizard.oidc.google.directoryGroups': 'Gruppi e chi ne fa parte',
  'wizard.oidc.google.directoryHint':
    'In sola lettura, e solo se chi accede all’app amministra l’organizzazione.',
  'wizard.oidc.summary.google': 'Accesso come account Google',
  'wizard.oidc.done.googleDiscovery': 'Discovery in formato Google',
  'wizard.oidc.urls.logoutLabel': 'URL dopo la disconnessione',
  'wizard.oidc.urls.logoutHint':
    'Facoltativo. Dove portiamo il tuo team se esce dall’applicazione. Se lo lasci vuoto, resta in Sytue.',
  'wizard.oidc.summary.app': 'Applicazione',
  'wizard.oidc.summary.runsOn': 'Dove viene eseguita',
  'wizard.oidc.summary.runsServer': 'Su un server proprio (con segreto)',
  'wizard.oidc.summary.runsBrowser': 'Nel browser o sul cellulare (senza segreto, PKCE)',
  'wizard.oidc.summary.redirect': 'URL di reindirizzamento',
  'wizard.oidc.summary.logout': 'URL dopo la disconnessione',
  'wizard.oidc.summary.sees': 'Dati che vedrà',
  'wizard.oidc.summary.seesValue': 'Nome, e-mail e identificatore',
  'wizard.oidc.review.heading': 'Rivedi e registra',
  'wizard.oidc.review.description':
    'Non è ancora stato creato nulla. Puoi cambiare qualsiasi dato prima di continuare.',
  'wizard.oidc.review.submit': 'Registra l’applicazione',
  'wizard.oidc.review.secretTitle': 'Il segreto si mostra UNA volta sola',
  'wizard.oidc.review.secretBody':
    'Al momento della registrazione ti daremo una password per l’applicazione. Non la conserviamo in chiaro e non potremo mostrartela di nuovo: copiala subito. Se si perde, ne generi un’altra dall’elenco e basta.',
  'wizard.oidc.review.pkceTitle': 'Senza segreto, e va bene così',
  'wizard.oidc.review.pkceBody':
    'Un’applicazione che gira sul dispositivo di chi la usa non può custodire segreti. Userà PKCE, che protegge lo scambio senza bisogno di uno.',
  'wizard.oidc.done.heading': '{name} può già usare Sytue',
  'wizard.oidc.done.fallbackName': 'L’applicazione',
  'wizard.oidc.done.secretTitle': 'Copia il segreto adesso: non verrà più mostrato',
  'wizard.oidc.done.secretBody':
    'Incollalo nell’applicazione accanto all’identificatore client. Se lo perdi, generane uno nuovo dall’elenco (il precedente smetterà di funzionare).',
  'wizard.oidc.done.secretLabel': 'Segreto client',
  'wizard.oidc.done.pasteTitle': 'Incolla questo nell’applicazione',
  'wizard.oidc.done.pasteBody':
    'Quasi tutte chiedono solo i primi due: con l’indirizzo di Sytue compilano il resto da sole. Se la tua chiede ogni indirizzo separatamente, li trovi qui sotto.',
  'wizard.oidc.done.issuer': 'Indirizzo di Sytue (issuer)',
  'wizard.oidc.done.clientId': 'Identificatore client',
  'wizard.oidc.done.discovery': 'Configurazione automatica',
  'wizard.oidc.done.separateTitle':
    'Indirizzi separati (se la tua applicazione li chiede uno a uno)',
  'wizard.oidc.done.authorization': 'Autorizzazione',
  'wizard.oidc.done.token': 'Token',
  'wizard.oidc.done.userinfo': 'Dati della persona',
  'wizard.oidc.done.jwks': 'Chiavi di firma (JWKS)',
  'wizard.oidc.done.endSession': 'Disconnessione',
  'wizard.oidc.done.scopes': 'Autorizzazioni (scope)',
  'wizard.oidc.done.nextTitle': 'Cosa fare adesso',
  'wizard.oidc.done.step1Title': 'Incolla i dati qui sopra nell’applicazione',
  'wizard.oidc.done.step1Body':
    'Cercaci la sezione di accesso con OpenID Connect (o OAuth) e compilala con l’indirizzo di Sytue e l’identificatore client.',
  'wizard.oidc.done.step1BodySecret':
    'Cercaci la sezione di accesso con OpenID Connect (o OAuth) e compilala con l’indirizzo di Sytue, l’identificatore client e il segreto.',
  'wizard.oidc.done.step2Title': 'Provalo con il tuo account',
  'wizard.oidc.done.step2Body':
    'Apri una finestra di navigazione privata ed entra nell’applicazione con il pulsante di accesso con Sytue. Se ti riporta all’applicazione già identificato, è pronto.',
  'wizard.oidc.done.step3Title': 'Avvisa il tuo team',
  'wizard.oidc.done.step3Body':
    'Da ora accedono con il proprio account Sytue. Se qualcuno lascia l’organizzazione, perde automaticamente l’accesso a questa applicazione.',
  'wizard.oidc.done.back': 'Torna alle applicazioni',

  // --- Asistente: conectar una aplicación (ConnectAppWizard) -----------------
  'wizard.app.effect.portalTitle': 'Compare nel portale',
  'wizard.app.effect.portalBody':
    'Il tuo team vedrà {app} tra le sue applicazioni quando accede a Sytue.',
  'wizard.app.effect.ssoTitle': 'Entrano con il proprio account Sytue',
  'wizard.app.effect.ssoBody':
    'Per {app} non serve più una password a parte: si entra con l’account Sytue e si applicano le tue regole di accesso.',
  'wizard.app.effect.scimTitle': 'Ingressi e uscite automatici',
  'wizard.app.effect.scimBody':
    'Quando qualcuno entra o esce dalla tua organizzazione, il suo account {app} viene creato o disattivato da solo.',
  'wizard.app.effect.dataTitle': 'Lettura dei dati',
  'wizard.app.effect.dataBody':
    'Sytue potrà leggere dati da {app} per i report. Non scrive né cancella mai nulla.',
  'wizard.app.need.fieldWhere': 'Lo trovi nelle impostazioni del tuo account {app}.',
  'wizard.app.need.secretWhere':
    'Si genera nel pannello di amministrazione di {app}. Lì ti servono permessi di amministrazione.',
  'wizard.app.err.required': 'Ci serve questo dato per continuare.',
  'wizard.app.err.noSpaces': 'Non può contenere spazi.',
  'wizard.app.err.noScheme': 'Scrivilo senza «https://», solo il nome.',
  'wizard.app.err.noPath': 'Solo il nome o il dominio, senza barre né percorsi.',
  'wizard.app.fix.fieldFallback': 'i dati della connessione',
  'wizard.app.fix.badData': 'I dati non tornano. Controlla «{field}» nel passaggio precedente.',
  'wizard.app.fix.rejectedSecret':
    '{app} ha rifiutato «{secret}»: può essere scaduto o non avere permessi sufficienti. Generane uno nuovo e incollalo di nuovo.',
  'wizard.app.fix.rejected':
    '{app} ha rifiutato la connessione. Controlla che l’applicazione consenta l’accesso da Sytue.',
  'wizard.app.fix.notFound':
    'Non abbiamo trovato quell’account {app}. Controlla «{field}»: di solito è un errore di battitura.',
  'wizard.app.fix.down':
    '{app} non risponde in questo momento. Non dipende dai tuoi dati: riprova tra un minuto.',
  'wizard.app.fix.generic':
    'Non siamo riusciti a connetterci. Ricontrolla i dati del passaggio precedente e riprova.',
  'wizard.app.toast.activateFailed': 'Non è stato possibile attivarla',
  'wizard.app.fix.activateBadData':
    'Controlla i dati del passaggio «Configurare»: non siamo riusciti a registrare l’accesso.',
  'wizard.app.fix.activateIdpDown':
    'Sytue ID non risponde in questo momento. Riprova tra un minuto.',
  'wizard.app.fix.activateGeneric': 'Qualcosa è andato storto per strada. Riprova.',
  'wizard.app.step.choose': 'Scegliere',
  'wizard.app.step.explain': 'Cosa succede',
  'wizard.app.step.configure': 'Configurare',
  'wizard.app.step.test': 'Provare',
  'wizard.app.step.review': 'Riepilogo',
  'wizard.app.step.done': 'Fatto',
  'wizard.app.choose.heading': 'Quale applicazione vuoi collegare?',
  'wizard.app.choose.description':
    'Scegli una delle applicazioni che il tuo team già usa. Al resto ti guidiamo noi: non serve sapere di SSO né di SCIM.',
  'wizard.app.choose.searchAria': 'Cerca applicazione',
  'wizard.app.choose.searchPlaceholder': 'Cerca per nome o categoria…',
  'wizard.app.choose.noResults':
    'Non c’è nessuna applicazione con questo nome. Prova con un altro.',
  'wizard.app.choose.alreadyConnected': 'Già collegata',
  'wizard.app.explain.heading': 'Ecco cosa attiverai in {app}',
  'wizard.app.explain.headingFallback': 'Ecco cosa succederà',
  'wizard.app.explain.description':
    'Niente di tutto questo si applica ancora: prima lo configuri e lo provi.',
  'wizard.app.explain.needTitle': 'Tieni questo a portata di mano prima di continuare',
  'wizard.app.configure.heading': 'Dati del tuo {app}',
  'wizard.app.configure.headingFallback': 'Dati della connessione',
  'wizard.app.configure.description':
    'Solo l’indispensabile. Sotto ogni campo ti diciamo dove trovarlo, e controlliamo quello che scrivi al volo.',
  'wizard.app.configure.nameLabel': 'Nome della connessione',
  'wizard.app.configure.nameError': 'Dalle un nome per riconoscerla.',
  'wizard.app.configure.nameHint':
    'Si vede solo qui, nella console. Puoi lasciare quello che ti proponiamo.',
  'wizard.app.configure.formatOk': 'Formato corretto.',
  'wizard.app.configure.secretStored':
    'Si salva cifrato e non viene più mostrato: non possiamo leggerlo nemmeno noi. Se un giorno cambia, lo sostituisci con uno nuovo.',
  'wizard.app.oauth.title': 'Collegati con il tuo account {app}',
  'wizard.app.oauth.body':
    'Ti porteremo su {app} perché tu approvi l’accesso. Non devi creare né incollare nessuna credenziale.',
  'wizard.app.oauth.willAuthorize': 'Stai per autorizzare:',
  'wizard.app.oauth.revocable':
    'Il permesso scade da solo e puoi ritirarlo quando vuoi, da qui o da {app}.',
  'wizard.app.test.heading': 'Verifichiamo che Sytue arrivi a {app}',
  'wizard.app.test.headingFallback': 'Verifichiamo la connessione',
  'wizard.app.test.description':
    'È una chiamata di prova con i dati che hai inserito. Per il tuo team non si attiva ancora nulla.',
  'wizard.app.test.okTitle': 'Connessione corretta',
  'wizard.app.test.okBody': 'Abbiamo parlato con {app} e ci ha risposto. Ora puoi attivarla.',
  'wizard.app.test.failTitle': 'Non siamo riusciti a connetterci',
  'wizard.app.test.reviewData': 'Rivedere i dati',
  'wizard.app.test.idleHelp':
    'Premi «Prova la connessione» e verifichiamo i dati con {app}. Se qualcosa non torna, ti diciamo esattamente cosa cambiare.',
  'wizard.app.test.run': 'Prova la connessione',
  'wizard.app.test.rerun': 'Prova di nuovo',
  'wizard.app.test.skipHint': 'Puoi continuare senza provare, ma è meglio verificare adesso.',
  'wizard.app.summary.app': 'Applicazione',
  'wizard.app.summary.name': 'Nome della connessione',
  'wizard.app.summary.secretSaved': 'Salvato (non si mostra)',
  'wizard.app.summary.secretNone': 'Senza token',
  'wizard.app.summary.willEnable': 'Si attiverà',
  'wizard.app.summary.test': 'Verifica',
  'wizard.app.summary.testOk': 'Connessione provata con successo',
  'wizard.app.summary.testFail': 'La prova è fallita',
  'wizard.app.summary.testNone': 'Non provata',
  'wizard.app.review.heading': 'Rivedi e attiva',
  'wizard.app.review.description':
    'Ecco cosa applicheremo. Puoi cambiare qualsiasi dato prima di continuare.',
  'wizard.app.review.activate': 'Attiva {app}',
  'wizard.app.review.activateFallback': 'Attiva',
  'wizard.app.review.note':
    'Attivandola, {app} compare nel portale del tuo team e registriamo l’accesso. Puoi disattivarla quando vuoi: in {app} non si cancella nulla.',
  'wizard.app.done.heading': '{app} è già collegata',
  'wizard.app.done.headingFallback': 'Connessione pronta',
  'wizard.app.done.okTitle': 'È andato tutto bene',
  'wizard.app.done.okBody': 'La connessione è attiva e il tuo team la ha già a disposizione.',
  'wizard.app.done.nextTitle': 'Cosa fare adesso',
  'wizard.app.done.step1Title': 'Controlla che compaia nel portale',
  'wizard.app.done.step1Body':
    'Apri il lanciatore di applicazioni di Sytue: {app} compare accanto al resto della suite per chiunque abbia accesso.',
  'wizard.app.done.step2Title': 'Prova l’accesso',
  'wizard.app.done.step2Body':
    'Entra in {app} con il tuo account Sytue. Se ti fa passare senza chiederti la password di {app}, è configurato bene.',
  'wizard.app.done.step2BodyLaunch':
    'Entra in {app} dal pulsante qui sotto con il tuo account Sytue. Se ti fa passare senza chiederti la password di {app}, è configurato bene.',
  'wizard.app.done.step3Title': 'Lascia che si sincronizzino gli account',
  'wizard.app.done.step3Body':
    'La prima sincronizzazione parte da sola. Dalla scheda di {app} puoi vedere lo stato o forzarne un’altra.',
  'wizard.app.done.oidcTitle': 'Copiale adesso: non verranno più mostrate',
  'wizard.app.done.oidcBody':
    'Incolla queste credenziali nella configurazione di accesso OIDC di {app}.',
  'wizard.app.done.samlTitle': 'Metadati SAML di Sytue',
  'wizard.app.done.samlBody': 'Registra questo indirizzo nella configurazione SSO di {app}.',
  'wizard.app.done.back': 'Torna a Integrazioni',
  'wizard.app.done.open': 'Apri {app}',

  // --- Asistente: regla de acceso condicional (AccessPolicyWizard) -----------
  'wizard.accessPolicy.andMore': { one: '{count} altro', other: '{count} altri' },
  'wizard.accessPolicy.aGroup': 'un gruppo',
  'wizard.accessPolicy.aUser': 'una persona',
  'wizard.accessPolicy.who.everyone': 'qualsiasi persona della tua organizzazione',
  'wizard.accessPolicy.who.nobodyYet': 'ancora nessuno (non hai scelto chi)',
  'wizard.accessPolicy.cond.outside': {
    one: 'la connessione arriva da fuori della rete che hai indicato (per esempio da casa, da un hotel o dal cellulare)',
    other:
      'la connessione arriva da fuori delle reti che hai indicato (per esempio da casa, da un hotel o dal cellulare)',
  },
  'wizard.accessPolicy.cond.inside': {
    one: 'la connessione arriva dalla rete che hai indicato',
    other: 'la connessione arriva dalle reti che hai indicato',
  },
  'wizard.accessPolicy.cond.untrusted': 'il dispositivo non è segnato come attendibile',
  'wizard.accessPolicy.cond.apps': 'l’applicazione a cui si accede è {apps}',
  'wizard.accessPolicy.cond.anyDay': 'un giorno qualsiasi',
  'wizard.accessPolicy.cond.scheduleDays': 'il tentativo avviene di {days}',
  'wizard.accessPolicy.cond.scheduleHours':
    'il tentativo avviene di {days} tra le {from} e le {to} (UTC)',
  'wizard.accessPolicy.what.block': 'non potrà accedere',
  'wizard.accessPolicy.what.mfa': 'dovrà confermare la propria identità con un secondo fattore',
  'wizard.accessPolicy.what.allow': 'accederà senza altre verifiche',
  'wizard.accessPolicy.sentence.always':
    'Con questa regola, {who} {what} a ogni tentativo di accesso.',
  'wizard.accessPolicy.sentence.when': 'Con questa regola, {who} {what} se {when}.',
  'wizard.accessPolicy.sentenceTitle': 'La regola, in una frase',
  'wizard.accessPolicy.suggest.block': 'Bloccare',
  'wizard.accessPolicy.suggest.mfa': 'Richiedere verifica',
  'wizard.accessPolicy.suggest.allow': 'Consentire',
  'wizard.accessPolicy.suggest.withCondition': '{verb} se {condition}',
  'wizard.accessPolicy.suggest.plain': '{verb} l’accesso',
  'wizard.accessPolicy.err.badCidr':
    '«{range}» non è un intervallo di rete valido. Scrivi la rete e la sua dimensione: 203.0.113.0/24 per un intero ufficio, o 198.51.100.7/32 per un solo indirizzo.',
  'wizard.accessPolicy.err.bothTimes': 'Indica entrambe le ore: da quando e fino a quando.',
  'wizard.accessPolicy.fix.badData':
    'Il servizio ha rifiutato un dato. Controlla gli intervalli di rete nel passaggio «Quando».',
  'wizard.accessPolicy.fix.forbidden':
    'Il tuo account non ha l’autorizzazione per cambiare le regole di accesso di questa organizzazione.',
  'wizard.accessPolicy.fix.idpDown':
    'Sytue ID non risponde in questo momento. Riprova tra un minuto.',
  'wizard.accessPolicy.risk.allowTitle': 'Questa regola non lascia fuori nessuno',
  'wizard.accessPolicy.risk.allowBody':
    'Le regole che lasciano passare non possono chiuderti fuori: fanno solo un’eccezione ad altre più severe.',
  'wizard.accessPolicy.risk.lockoutTitle':
    'Questa regola lascerebbe fuori tutta la tua organizzazione',
  'wizard.accessPolicy.risk.lockoutBody':
    '{sentence} Nessuno potrebbe più accedere a Sytue, tu compreso, e non potresti nemmeno entrare in questa console per annullarla. Aggiungi una condizione nel passaggio «Quando», oppure escludi i tuoi amministratori nel passaggio «Chi».',
  'wizard.accessPolicy.risk.affectsYouTitle': 'Questa regola riguarda anche te',
  'wizard.accessPolicy.risk.noExitTitle': 'Non hai lasciato nessuna uscita di emergenza',
  'wizard.accessPolicy.risk.noExitBody':
    '{sentence} Se un giorno tocca a te, resterai fuori dalla console e nessuno potrà rimediare. Torna al passaggio «Chi» ed escludi il tuo gruppo di amministratori: continueranno a entrare anche quando la regola si applica.',
  'wizard.accessPolicy.risk.noExitBodyMfa':
    '{sentence} Se un giorno tocca a te, resterai fuori dalla console e nessuno potrà rimediare. Attenzione: chi non ha ancora configurato la verifica in due passaggi dovrà configurarla per poter accedere. Torna al passaggio «Chi» ed escludi il tuo gruppo di amministratori: continueranno a entrare anche quando la regola si applica.',
  'wizard.accessPolicy.risk.hasExitTitle': 'Hai un’uscita di emergenza',
  'wizard.accessPolicy.risk.hasExitBody':
    '{sentence} Sono esenti: {exempt}. Potranno accedere anche quando la regola si applica. Verifica di poter accedere con uno di quegli account prima di andartene.',
  'wizard.accessPolicy.risk.hasExitBodyMfa':
    '{sentence} Sono esenti: {exempt}. Potranno accedere anche quando la regola si applica. Attenzione: chi non ha ancora configurato la verifica in due passaggi dovrà configurarla per poter accedere. Verifica di poter accedere con uno di quegli account prima di andartene.',
  'wizard.accessPolicy.pick.filterPlaceholder': 'Filtra…',
  'wizard.accessPolicy.pick.filterAria': 'Filtra l’elenco',
  'wizard.accessPolicy.pick.noMatches': 'Nessuna corrispondenza.',
  'wizard.accessPolicy.action.mfaTitle': 'Chiedergli una verifica in più',
  'wizard.accessPolicy.action.mfaBody':
    'Entra, ma prima deve confermare di essere chi dice con il suo secondo fattore (l’app sul cellulare o la chiave di sicurezza). È il caso più comune.',
  'wizard.accessPolicy.action.blockTitle': 'Non farlo entrare',
  'wizard.accessPolicy.action.blockBody':
    'Vedrà un messaggio di accesso negato e non potrà proseguire. Usalo per tagliare situazioni che non dovrebbero mai verificarsi.',
  'wizard.accessPolicy.action.allowTitle': 'Farlo passare senza altro',
  'wizard.accessPolicy.action.allowBody':
    'Entra direttamente. Serve a fare un’eccezione a un’altra regola più severa, per esempio dalla rete dell’ufficio.',
  'wizard.accessPolicy.step.who': 'Chi',
  'wizard.accessPolicy.step.when': 'Quando',
  'wizard.accessPolicy.step.what': 'Cosa succede',
  'wizard.accessPolicy.step.review': 'Riepilogo',
  'wizard.accessPolicy.step.done': 'Fatto',
  'wizard.accessPolicy.who.heading': 'Chi riguarda questa regola?',
  'wizard.accessPolicy.who.description':
    'Comincia col decidere a chi si applica. Sotto puoi lasciare fuori le persone che non devono mai esserne toccate.',
  'wizard.accessPolicy.who.allTitle': 'A tutti',
  'wizard.accessPolicy.who.allBody': 'Si applica a chiunque acceda alla tua organizzazione.',
  'wizard.accessPolicy.who.someTitle': 'Solo ad alcune persone o gruppi',
  'wizard.accessPolicy.who.someBody':
    'Si applica solo a chi scegli tu. Gli altri non se ne accorgono.',
  'wizard.accessPolicy.who.groupsLabel': 'Gruppi interessati',
  'wizard.accessPolicy.who.usersLabel': 'Persone interessate',
  'wizard.accessPolicy.who.noGroups': 'Nella tua organizzazione non ci sono ancora gruppi.',
  'wizard.accessPolicy.who.noUsers': 'Non c’è nessuno da mostrare.',
  'wizard.accessPolicy.who.pickAtLeastOne':
    'Scegli almeno un gruppo o una persona, oppure torna a «A tutti».',
  'wizard.accessPolicy.exempt.heading': 'Chi resta sempre esente',
  'wizard.accessPolicy.exempt.description':
    'È la tua uscita di emergenza: questi account entrano anche quando la regola si applica. Escludi qui il tuo gruppo di amministrazione per non chiuderti fuori dalla console.',
  'wizard.accessPolicy.exempt.groupsLabel': 'Gruppi esenti',
  'wizard.accessPolicy.exempt.usersLabel': 'Persone esenti',
  'wizard.accessPolicy.exempt.none':
    'Non hai lasciato esente nessuno. Se questa regola finisce per riguardare anche te, resterai fuori e non potrai annullarla.',
  'wizard.accessPolicy.exempt.some': 'Sono esenti: {names}. Potranno entrare sempre.',
  'wizard.accessPolicy.when.heading': 'In quali situazioni si applica?',
  'wizard.accessPolicy.when.description':
    'Segna solo quello che ti serve. Se non segni nulla, la regola si applica ogni volta che quelle persone provano a entrare.',
  'wizard.accessPolicy.when.networkTitle': 'Da quale rete si collegano',
  'wizard.accessPolicy.when.networkBody':
    'Scrivi le reti che conosci (quella del tuo ufficio, la VPN…) e scegli se la regola deve agire dentro o fuori di esse.',
  'wizard.accessPolicy.when.networksLabel': 'Reti conosciute',
  'wizard.accessPolicy.when.networksOk': {
    one: '{count} rete riconosciuta.',
    other: '{count} reti riconosciute.',
  },
  'wizard.accessPolicy.when.networksHint':
    'Una per riga. Te le dà chi gestisce la rete della tua azienda. Lascia vuoto per non guardare la rete.',
  'wizard.accessPolicy.when.outsideTitle': 'Quando sono FUORI da quelle reti',
  'wizard.accessPolicy.when.outsideBody':
    'È la scelta normale: proteggere quello che succede fuori dall’ufficio (casa, hotel, reti pubbliche).',
  'wizard.accessPolicy.when.insideTitle': 'Quando sono DENTRO quelle reti',
  'wizard.accessPolicy.when.insideBody':
    'Si applica solo a chi si collega da quelle reti. Utile insieme all’azione «farlo passare senza altro».',
  'wizard.accessPolicy.when.deviceTitle': 'Da quale dispositivo',
  'wizard.accessPolicy.when.deviceSwitch': 'Solo quando il dispositivo non è attendibile',
  'wizard.accessPolicy.when.deviceBody':
    'Ogni persona segna i suoi dispositivi abituali come attendibili da «Dispositivi». Con questa opzione attiva, la regola agisce solo quando entrano da uno che non hanno segnato: un computer prestato, un cellulare nuovo, un internet point.',
  'wizard.accessPolicy.when.appsTitle': 'In quali applicazioni',
  'wizard.accessPolicy.when.appsLabel': 'Applicazioni',
  'wizard.accessPolicy.when.appsHint':
    'Identificatori delle applicazioni, separati da spazi. Lascia vuoto per applicarla a tutte (la scelta più comune).',
  'wizard.accessPolicy.when.timeTitle': 'In quale momento',
  'wizard.accessPolicy.when.timeBody':
    'Lascia i giorni non segnati e le ore vuote per non guardare il calendario. Le ore sono in UTC, non nell’ora locale di ogni persona.',
  'wizard.accessPolicy.when.daysLegend': 'Giorni',
  'wizard.accessPolicy.when.from': 'Dalle',
  'wizard.accessPolicy.when.to': 'Alle',
  'wizard.accessPolicy.when.soFarTitle': 'Ecco com’è finora',
  'wizard.accessPolicy.when.soFarNone':
    'Senza condizioni, la regola si applicherà ogni volta che quelle persone provano a entrare.',
  'wizard.accessPolicy.when.soFarSome': 'Si applicherà se {when}.',
  'wizard.accessPolicy.what.heading': 'Cosa vuoi che succeda in quei casi?',
  'wizard.accessPolicy.what.description':
    'Scegli cosa trova la persona quando la regola si applica.',
  'wizard.accessPolicy.what.strictestWins':
    'Se più regole si applicano insieme, comanda la più severa: bloccare batte chiedere verifica, e chiedere verifica batte lasciar passare.',
  'wizard.accessPolicy.summary.who': 'Chi riguarda',
  'wizard.accessPolicy.summary.exempt': 'Chi resta esente',
  'wizard.accessPolicy.summary.nobody': 'Nessuno',
  'wizard.accessPolicy.summary.when': 'Quando si applica',
  'wizard.accessPolicy.summary.whenAlways': 'Ogni volta che provano a entrare',
  'wizard.accessPolicy.summary.whenSome': 'Se {when}',
  'wizard.accessPolicy.summary.what': 'Cosa succede',
  'wizard.accessPolicy.summary.state': 'Stato',
  'wizard.accessPolicy.summary.stateOn': 'Si applicherà da quando la salvi',
  'wizard.accessPolicy.summary.stateOff': 'Salvata senza applicarla',
  'wizard.accessPolicy.review.headingNew': 'Rivedi e salva la regola',
  'wizard.accessPolicy.review.headingEdit': 'Rivedi le modifiche',
  'wizard.accessPolicy.review.description':
    'Ecco cosa applicheremo. Puoi cambiare qualsiasi cosa prima di salvare.',
  'wizard.accessPolicy.review.saveNew': 'Crea la regola',
  'wizard.accessPolicy.review.saveEdit': 'Salva le modifiche',
  'wizard.accessPolicy.review.understood':
    'Capisco che nessuno potrà accedere e voglio salvarla lo stesso',
  'wizard.accessPolicy.review.nameLabel': 'Nome della regola',
  'wizard.accessPolicy.review.nameHint':
    'È come la vedrai nell’elenco. Se lo lasci vuoto la chiameremo «{suggested}».',
  'wizard.accessPolicy.review.priorityLabel': 'Ordine di valutazione',
  'wizard.accessPolicy.review.priorityHint':
    'Cambia solo l’ordine in cui si guardano le regole: il numero più basso va per primo. Puoi spostarla dopo dall’elenco con le frecce.',
  'wizard.accessPolicy.review.enableLabel': 'Applica subito la regola',
  'wizard.accessPolicy.review.enableOn': 'Comincerà a essere valutata al prossimo accesso.',
  'wizard.accessPolicy.review.enableOff':
    'Verrà salvata spenta: puoi prepararla adesso e accenderla quando vuoi dall’elenco.',
  'wizard.accessPolicy.done.headingNew': 'Regola creata',
  'wizard.accessPolicy.done.headingEdit': 'Modifiche salvate',
  'wizard.accessPolicy.done.bodyOn': '{sentence} Viene già valutata a ogni accesso.',
  'wizard.accessPolicy.done.bodyOff':
    '{sentence} È salvata ma spenta: accendila dall’elenco quando vuoi.',
  'wizard.accessPolicy.done.checkTitle': 'Verificalo prima di andartene',
  'wizard.accessPolicy.done.checkBody':
    'Apri una finestra di navigazione privata e accedi con un account che la regola riguarda. Così vedi la stessa cosa che vedrà il tuo team e confermi di avere ancora accesso.',
  'wizard.accessPolicy.done.checkBodyExempt':
    'Apri una finestra di navigazione privata e accedi con un account che la regola riguarda. Così vedi la stessa cosa che vedrà il tuo team e confermi di avere ancora accesso con gli account esenti ({names}).',
  'wizard.accessPolicy.done.back': 'Torna alle regole',

  // --- Amministrazione della piattaforma (ambito globale) ---
  'nav.platform': 'Piattaforma',
  'page.platform': 'Amministrazione della piattaforma',
  // Conexiones con terceros (credenciales OAuth de la plataforma)
  'conn.title': 'Connessioni con terze parti',
  'conn.intro':
    'Credenziali con cui Sytue si presenta a ciascun fornitore. Con queste, per i tuoi clienti collegare o migrare da Google Workspace o Microsoft 365 è questione di un clic. Senza, ogni organizzazione deve registrare la propria applicazione presso il fornitore.',
  'conn.add': 'Aggiungi fornitore',
  'conn.edit': 'Modifica',
  'conn.remove': 'Rimuovi',
  'conn.empty': 'Nessun fornitore configurato per ora',
  'conn.emptyBody':
    'Finché non ne registri almeno uno, ogni connessione dipende dalle credenziali portate dal cliente.',
  'conn.redirectUri': 'URI di reindirizzamento da registrare presso il fornitore',
  'conn.redirectUriHelp':
    'La stessa per tutti. Una discordanza è la causa più frequente di un consenso fallito, e il fornitore la segnala solo come «redirect_uri_mismatch».',
  // Cómo conseguir el par de credenciales de cada proveedor: cuatro pasos dentro
  // del propio diálogo, que es donde hacen falta.
  'conn.guide.step1': 'Apri la console per sviluppatori di {provider} e crea un’app OAuth.',
  'conn.guide.open': 'Apri la console',
  'conn.guide.type': 'Tipo di applicazione',
  'conn.guide.step2': 'Registra lì questo URL di ritorno, esattamente com’è:',
  'conn.guide.copy': 'Copia',
  'conn.guide.copied': 'Copiato',
  'conn.guide.step3': 'Copia l’ID client e il secret che ti dà.',
  'conn.guide.step4':
    'Incollali qui sotto e salva. Il secret si conserva cifrato e non viene più mostrato.',
  'conn.guide.troubleshoot':
    '«invalid_client» significa che l’ID client non coincide con quello di lì; «redirect_uri_mismatch», che non coincide l’URL qui sopra.',
  'nav.connections': 'Connessioni',
  'conn.col.provider': 'Fornitore',
  'conn.col.secret': 'Segreto',
  'conn.col.updated': 'Ultima modifica',
  'conn.secret.set': 'Salvato',
  'conn.secret.public': 'Client pubblico (PKCE)',
  'conn.addTitle': 'Registrare l’applicazione di un fornitore',
  'conn.editTitle': 'Modificare le credenziali',
  'conn.field.provider': 'Fornitore',
  'conn.field.secretHelp':
    'Lascia vuoto se il fornitore ha registrato l’applicazione come client pubblico basato solo su PKCE.',
  'conn.field.secretKeepHelp':
    'Lascia vuoto per conservare il segreto salvato. Molti fornitori lo mostrano una sola volta.',
  'conn.toast.removed': 'Fornitore rimosso',
  'conn.toast.failed': 'Impossibile salvare',
  'conn.error.forbidden': 'Solo per l’amministrazione della piattaforma',
  'conn.error.forbiddenBody':
    'Queste credenziali servono tutti i clienti, quindi richiedono l’elenco consentito della piattaforma e un secondo fattore.',
  'conn.error.unavailable': 'Non configurato in questa installazione',
  'conn.error.unavailableBody':
    'Manca il token operatore di Sytue Integrations, quindi da qui non si possono gestire le credenziali.',
  'conn.templates.title': 'Disponibilità delle integrazioni nella suite',
  'conn.templates.intro':
    'Attiva o disattiva la disponibilità di ciascun connettore per l’intera suite e le organizzazioni. Solo le integrazioni abilitate qui appariranno in Sytue Admin per essere connesse o migrate.',
  'conn.templates.empty': 'Nessuna integrazione nel catalogo',
  'conn.templates.emptyBody': 'Il catalogo globale dei connettori non ha modelli registrati.',
  'conn.templates.tableCaption': 'Disponibilità integrazioni',

  'platform.tab.orgs': 'Organizzazioni',
  'platform.tab.usage': 'Utilizzo',
  'platform.tab.merges': 'Fusioni',

  // Visor de lo que está fallando (mig 0049): problemas agrupados, no líneas.
  'platform.tab.versions': 'Versioni',
  'platform.versions.title': 'Che cosa sta girando',
  'platform.versions.unavailable': 'Non è stato possibile leggere le versioni',
  'platform.versions.empty': 'Nessun componente da controllare',
  'platform.versions.emptyHint':
    'Il chart non ha passato alcun catalogo di componenti, quindi non c’è nessuno a cui chiedere. Controlla global.components nel chart ombrello.',
  'platform.versions.refresh': 'Aggiorna',
  'platform.versions.upToDate': 'Allineata',
  'platform.versions.behind': 'Disallineata',
  'platform.versions.kind.service': 'servizio',
  'platform.versions.kind.surface': 'web',
  'platform.versions.col.component': 'Componente',
  'platform.versions.col.release': 'Versione',
  'platform.versions.col.commit': 'Commit',
  'platform.versions.col.status': 'Stato',
  'platform.versions.kpi.expected': 'Versione maggioritaria',
  'platform.versions.kpi.upToDate': 'Allineate',
  'platform.versions.kpi.behind': 'Disallineate',
  'platform.versions.kpi.silent': 'Senza risposta',
  'platform.tab.logs': 'Cosa sta fallendo',
  'logs.kpi.errors1h': 'Errori nell’ultima ora',
  'logs.kpi.errors1hHint': 'Se non è zero, sta succedendo qualcosa adesso.',
  'logs.kpi.errors24h': 'Errori (24 h)',
  'logs.kpi.warns24h': 'Avvisi (24 h)',
  'logs.kpi.services': 'Servizi coinvolti',
  'logs.kpi.servicesHint': 'Sugli undici della suite.',
  'logs.trend.title': 'Ultime 24 ore',
  'logs.trend.label': 'Errori e avvisi per ora nelle ultime 24 ore',
  'logs.trend.footer': 'Errori in rosso, avvisi sotto. Anche le ore vuote vengono disegnate.',
  'logs.trend.errors': {
    one: '{count} errore',
    other: '{count} errori',
  },
  'logs.trend.warns': {
    one: '{count} avviso',
    other: '{count} avvisi',
  },
  'logs.list.title': 'Problemi, raggruppati',
  'logs.filter.level': 'Livello',
  'logs.filter.allLevels': 'Errori e avvisi',
  'logs.filter.service': 'Servizio',
  'logs.filter.allServices': 'Tutti i servizi',
  'logs.filter.search': 'Cercare nel messaggio',
  'logs.level.error': 'Errore',
  'logs.level.warn': 'Avviso',
  'logs.times': 'volte',
  'logs.since': 'da {when}',
  'logs.last': 'ultima {when}',
  'logs.showDetail': 'Dettaglio',
  'logs.hideDetail': 'Nascondere',
  'logs.empty': 'Non sta fallendo nulla',
  'logs.emptyHint': 'Nessun errore né avviso nella finestra scelta.',
  'logs.unavailable': 'Non si può leggere il registro',
  'logs.unavailableHint':
    'Sytue ID non ha risposto. I servizi continuano a scriverci; a essere giù è questa vista.',
  'platform.tab.admins': 'Admin globali',
  'platform.orgs.title': 'Tutte le organizzazioni',
  'platform.orgs.filter': 'Filtra per nome o identificatore',
  'platform.orgs.empty': 'Nessuna organizzazione corrisponde al filtro.',
  'platform.col.org': 'Organizzazione',
  'platform.col.status': 'Stato',
  'platform.col.capacity': 'Persone / posti',
  'platform.col.owner': 'Proprietà',
  'platform.col.actions': 'Azioni',
  'platform.col.merge': 'Fusione',
  'platform.col.progress': 'Avanzamento',
  'platform.col.email': 'E-mail',
  'platform.col.reason': 'Motivo',
  'platform.col.since': 'Dal',
  'platform.status.active': 'Attiva',
  'platform.status.suspended': 'Sospesa',
  'platform.status.deleted': 'Eliminata',
  'platform.status.merged': 'Fusa',
  'platform.action.grantSeats': 'Concedi posti',
  'platform.action.suspend': 'Sospendi',
  'platform.action.delete': 'Elimina',
  'platform.prompt.deleteTitle': 'Elimina questa organizzazione',
  'platform.prompt.deleteHint':
    'Smette di erogare il servizio e sparisce dall’operatività quotidiana. Le sue tracce restano per poter rispondere che fine ha fatto, e riattivarla la ripristina. Richiede la firma di un altro account di amministrazione globale.',
  'platform.action.reactivate': 'Riattiva',
  'platform.action.merge': 'Unisci',
  'platform.action.resume': 'Riprendi',
  'platform.usage.orgs': 'Organizzazioni',
  'platform.usage.orgsActive': 'Attive',
  'platform.usage.orgsSuspended': 'Sospese',
  'platform.usage.users': 'Persone attive',
  'platform.usage.users30d': 'Con accesso (30 giorni)',
  'platform.usage.seatsSold': 'Posti venduti',
  'platform.usage.seatsGranted': 'Posti regalati',
  'platform.usage.subs': 'Abbonamenti attivi',
  'platform.merges.title': 'Fusioni di organizzazioni',
  'platform.merges.empty': 'Non è ancora stata eseguita alcuna fusione.',
  'platform.merge.state.pending': 'In attesa',
  'platform.merge.state.running': 'In corso',
  'platform.merge.state.completed': 'Completata',
  'platform.merge.state.failed': 'Non riuscita',
  'platform.merge.state.aborted': 'Interrotta',
  'platform.admins.title': 'Amministrazione della piattaforma',
  'platform.seats.count': 'Posti',
  'platform.seats.kind': 'Tipo',
  'platform.seats.kind.goodwill': 'Gesto commerciale',
  'platform.seats.kind.employee': 'Dipendente Sytue',
  'platform.seats.kind.trial': 'Prova',
  'platform.seats.kind.migration': 'Migrazione',
  'platform.seats.reason': 'Motivo (obbligatorio)',
  'platform.seats.expires': 'Scade (facoltativo)',
  'platform.merge.warning':
    "La fusione è irreversibile: le persone e tutti i dati dell'organizzazione di origine passano a quella di destinazione, e l'origine viene chiusa.",
  'platform.merge.target': 'Organizzazione di destinazione',
  'platform.merge.confirm': 'Digita {slug} per confermare',
  'platform.prompt.statusReason': 'Motivo di questa modifica:',
  'platform.toast.statusSaved': 'Stato aggiornato.',
  'platform.toast.seatsGranted': 'Posti concessi.',
  'platform.toast.mergeStarted': 'Fusione avviata.',
  'platform.toast.failed': "Non è stato possibile completare l'operazione.",
  'platform.error.forbidden': "Non puoi accedere all'amministrazione della piattaforma",
  'platform.error.unavailable': "L'amministrazione della piattaforma non è disponibile",
  'platform.error.unavailableHint': 'Sytue ID non ha risposto. Riprova tra un momento.',

  // Los tres 403 de esta API, separados: llevan a sitios distintos.
  'platform.error.mfa': 'Rientra con il tuo secondo fattore',
  'platform.error.mfaHint':
    'Il tuo account ha l’amministrazione globale, ma questa sessione non ha usato la verifica in due passaggi. Esci e rientra con essa.',
  'platform.error.signOut': 'Esci',
  'platform.error.notAllowlisted':
    'Questo account non è nell’allowlist della piattaforma. Deve aggiungerlo un altro account di amministrazione globale; un secondo fattore non cambia nulla.',

  // Elevazione temporanea dell'ambito globale (PIM).
  'platform.pim.gateTitle': 'Elevati per amministrare la piattaforma',
  'platform.pim.gateBody':
    'Avere l’amministrazione globale significa che puoi elevarti, non che sia già fatto. Chiedi una finestra, di’ a cosa ti serve e si chiude da sola.',
  'platform.pim.elevate': 'Elevami',
  'platform.pim.stepDown': 'Abbassare l’ambito',
  'platform.pim.modalTitle': 'Elevarti all’ambito di piattaforma',
  'platform.pim.justification': 'A cosa ti serve (obbligatorio)',
  'platform.pim.justificationHint':
    'Resta nel registro. Fra qualche mese è l’unica risposta al perché qualcuno si era elevato.',
  'platform.pim.duration': 'Per quanto tempo',
  'platform.pim.duration.minutes': { one: '{count} minuto', other: '{count} minuti' },
  'platform.pim.duration.hours': { one: '{count} ora', other: '{count} ore' },
  'platform.pim.active': 'Ambito di piattaforma attivo',
  'platform.pim.remaining': { one: 'Resta {count} minuto', other: 'Restano {count} minuti' },
  'platform.pim.expiringSoon': 'Resta meno di un minuto',
  'platform.pim.toast.elevated': 'Ora sei elevato. Si chiude da solo.',
  'platform.pim.toast.steppedDown': 'Non sei più elevato.',
  'platform.pim.history': 'Storico delle elevazioni',
  'platform.pim.history.empty': 'Nessuno si è ancora elevato.',
  'platform.pim.col.who': 'Chi',
  'platform.pim.col.when': 'Quando',
  'platform.pim.col.window': 'Finestra',
  'platform.pim.state.active': 'Attiva',
  'platform.pim.state.expired': 'Terminata',
  'platform.pim.state.revoked': 'Abbassata',

  // Detalle de una organización y sus plazas regaladas.
  'platform.detail.namespace': 'Spazio dei nomi',
  'platform.detail.grants': 'Postazioni regalate',
  'platform.detail.noGrants': 'A questa organizzazione non è stata regalata nessuna postazione.',
  'platform.detail.grantForever': 'Senza data di fine',
  'platform.detail.grantExpired': 'Scaduta',
  'platform.detail.grantRevoked': 'Ritirata',
  'platform.detail.revokeGrant': 'Ritirare',
  'platform.seats.revoked': 'Postazioni ritirate.',

  // Qué está desplegado (release y commit horneados en la imagen).
  'platform.version.unbuilt': 'Costruita a mano',
  'platform.version.unreachable': 'Non ha risposto',

  // Revisión periódica del alcance global (mig 0047).
  'platform.review.col.reviewed': 'Ultima revisione',
  'platform.review.confirm': 'Serve ancora',
  'platform.review.done': 'Rivisto.',
  'platform.review.notYourself':
    'Il tuo ambito lo rivede un altro account di amministrazione globale',
  'platform.review.overdue': { one: 'Più di {count} giorno', other: 'Più di {count} giorni' },
  'platform.review.pending': {
    one: '{count} accesso globale da rivedere',
    other: '{count} accessi globali da rivedere',
  },
  'platform.review.pendingHint':
    'Non è stato tolto nulla. Qualcuno deve controllare se hanno ancora senso.',

  // Segunda persona para lo irreversible (mig 0046).
  'platform.tab.approvals': 'Approvazioni',
  'platform.approval.title': 'Operazioni in attesa di una seconda persona',
  'platform.approval.intro':
    'Sospendere un’organizzazione lascia fuori un’intera azienda e una fusione non si può annullare. Chi chiede non può essere chi approva.',
  'platform.approval.empty': 'Non c’è nulla in attesa di approvazione.',
  'platform.approval.approve': 'Approva ed esegui',
  'platform.approval.reject': 'Rifiuta',
  'platform.approval.col.requester': 'Richiesta da',
  'platform.approval.action.org.suspend': 'Sospendere l’organizzazione',
  'platform.approval.action.org.delete': 'Eliminare l’organizzazione',
  'platform.approval.action.org.merge': 'Fondere le organizzazioni',
  'platform.approval.action.platform.admin.grant': 'Concedere ambito globale',
  'platform.approval.state.pending': 'In attesa',
  'platform.approval.state.executed': 'Fatta',
  'platform.approval.state.rejected': 'Rifiutata',
  'platform.approval.state.expired': 'Scaduta',
  'platform.approval.queued': 'Resta in attesa di una seconda persona',
  'platform.approval.queuedHint':
    'Non è ancora successo nulla. Deve approvarlo un altro account di amministrazione globale, e scade in un giorno.',
  'platform.approval.samePerson': 'Non puoi approvare ciò che hai chiesto tu',
  'platform.approval.waiting': {
    one: '{count} operazione in attesa',
    other: '{count} operazioni in attesa',
  },
  'platform.approval.waitingHint': 'Qualcuno ha chiesto qualcosa che richiede la tua approvazione.',

  // Registro de la plataforma (mig 0045).
  'platform.tab.audit': 'Registro',
  'platform.audit.title': 'Cosa ha fatto l’amministrazione globale',
  'platform.audit.filter': 'Tipo di azione',
  'platform.audit.empty': 'Dall’amministrazione della piattaforma non è stato ancora fatto nulla.',
  'platform.audit.noOrg': 'Nessuna organizzazione',
  'platform.audit.col.action': 'Azione',
  'platform.audit.col.detail': 'Dettaglio',
  'platform.audit.family.all': 'Tutto',
  'platform.audit.family.platform.org.': 'Organizzazioni',
  'platform.audit.family.platform.seats.': 'Postazioni',
  'platform.audit.family.platform.merge.': 'Fusioni',
  'platform.audit.family.platform.admin.': 'Ambito globale',
  'platform.audit.family.platform.pim.': 'Elevazioni',
  'platform.audit.family.platform.approval.': 'Approvazioni',

  'platform.education.badge': 'Istruzione (50%)',
  'platform.education.toggle': 'Istituzione educativa',
  'platform.education.enable': 'Segna come educativa',
  'platform.education.disable': 'Rimuovi stato educativo',
  'platform.prompt.educationReason': 'Motivo del cambio di stato educativo:',
  'platform.toast.educationSaved': 'Stato educativo aggiornato.',

  'platform.admins.revoke': 'Revoca ambito',
  'platform.prompt.revokeAdminTitle': 'Revoca ambito globale',
  'platform.prompt.revokeAdminHint':
    'Questo account non potrà più amministrare la piattaforma. L’accesso normale e le iscrizioni vengono conservati.',
  'platform.toast.adminRevoked': 'Ambito globale revocato.',
  'platform.create.open': 'Crea organizzazione',
  'platform.create.title': 'Crea un’organizzazione',
  'platform.create.submit': 'Crea',
  'platform.create.slug': 'Identificativo',
  'platform.create.slugHelp': 'Lettere minuscole, cifre e trattini. Compare nell’indirizzo.',
  'platform.create.name': 'Nome',
  'platform.create.owner': 'Email della persona titolare',
  'platform.create.ownerHelp':
    'Deve già esistere come account Sytue. Lascialo vuoto per assegnarla più tardi.',
  'platform.create.trial': 'Posti di prova (facoltativo)',
  'platform.create.trialEnds': 'Fine della prova',
  'platform.create.done': 'Organizzazione «{slug}» creata.',
  'platform.create.doneTrial': {
    one: '{count} posto di prova concesso.',
    other: '{count} posti di prova concessi.',
  },
  'platform.create.error.forbidden':
    'Non consentito. L’elevazione potrebbe essere scaduta: elevati di nuovo e riprova.',
  'platform.create.error.conflict': 'Questo identificativo esiste già o è riservato.',
  'platform.create.error.invalid':
    'Controlla l’identificativo e che l’email appartenga a un account esistente.',
  'platform.create.error.trial':
    'L’organizzazione è stata creata, ma i posti di prova no: concedili dal suo dettaglio.',

  // Propietarios y cuentas de una organización desde plataforma (mig 0113).
  'platform.members.title': 'Persone',
  'platform.members.search': 'Cerca per nome o e-mail',
  'platform.members.empty': 'Nessuno corrisponde.',
  'platform.members.col.person': 'Persona',
  'platform.members.col.role': 'Ruolo',
  'platform.members.status.active': 'Attivo',
  'platform.members.status.suspended': 'Bloccato',
  'platform.members.status.deleted': 'Eliminato',
  'platform.members.ownersHint':
    'Un’organizzazione può avere più proprietari. Assegnarne uno non toglie gli altri: questo si fa dalla console dell’organizzazione stessa.',
  'platform.members.assignOwner': 'Assegna proprietario',
  'platform.members.inviteOwner': 'Crea proprietario',
  'platform.members.makeOwner': 'Rendi proprietario',
  'platform.members.assignTitle': 'Assegna un proprietario',
  'platform.members.assignHelp':
    'L’account deve già esistere. Se non è membro di questa organizzazione, entra come proprietario.',
  'platform.members.inviteTitle': 'Crea un proprietario',
  'platform.members.inviteHelp':
    'La persona riceve un invito via e-mail e sceglie la propria password quando lo accetta.',
  'platform.members.name': 'Nome completo',
  'platform.members.resetPassword': 'Reimposta password',
  'platform.members.passwordTitle': 'Reimposta la password di {email}',
  'platform.members.passwordMode': 'Come',
  'platform.members.passwordLink': 'Invia un link via e-mail',
  'platform.members.passwordTemporary': 'Imposta una password temporanea',
  'platform.members.passwordHint':
    'In entrambi i casi le sue sessioni vengono chiuse e i suoi token revocati. La password temporanea si mostra una sola volta e va cambiata al prossimo accesso.',
  'platform.members.temporaryTitle': 'Password temporanea',
  'platform.members.temporaryOnce':
    'Copiala ora: non verrà più mostrata e non è conservata da nessuna parte. Dalla a {email} tramite un canale di fiducia.',
  'platform.members.block': 'Blocca',
  'platform.members.unblock': 'Sblocca',
  'platform.members.blockTitle': 'Blocca {email}',
  'platform.members.blockHint':
    'Perde l’accesso subito: sessioni chiuse e token revocati. Non si elimina nulla e puoi sbloccarlo.',
  'platform.members.unblockTitle': 'Sblocca {email}',
  'platform.members.deleteTitle': 'Elimina {email}',
  'platform.members.deleteHint':
    'L’account viene chiuso e perde ogni accesso. L’organizzazione può ripristinarlo per 30 giorni; dopo non si può più annullare.',
  'platform.members.deleteConfirm': 'Digita {email} per confermare',
  'platform.members.sessionsClosed': {
    one: '{count} sessione chiusa.',
    other: '{count} sessioni chiuse.',
  },
  'platform.members.toast.ownerAssigned': 'Proprietario assegnato.',
  'platform.members.toast.alreadyOwner': 'Quell’account era già proprietario.',
  'platform.members.toast.ownerInvited': 'Invito inviato a {email}.',
  'platform.members.toast.linkSent': 'Link di reimpostazione inviato.',
  'platform.members.toast.blocked': 'Account bloccato.',
  'platform.members.toast.unblocked': 'Account sbloccato.',
  'platform.members.toast.deleted': 'Account eliminato.',
  'platform.members.error.stepUp':
    'Conferma che sei tu: esci, rientra con il secondo fattore e ripeti l’azione.',
  'platform.members.error.soleOwner':
    'Questa persona è l’unica proprietaria di {orgs}. Assegna prima un altro proprietario lì.',
  'platform.members.error.noSuchUser':
    'Non c’è nessun account con questa e-mail. Crea invece un nuovo proprietario.',
  'platform.members.error.accountExists':
    'Questa e-mail ha già un account. Assegnalo invece come proprietario.',
  'platform.members.error.inactive': 'Quell’account è bloccato o chiuso. Sbloccalo prima.',
  'platform.members.error.platformAdmin':
    'Questo account amministra la piattaforma. Togligli prima l’ambito globale.',
  'platform.audit.family.platform.user.': 'Account',
  'nav.moderation': 'Segnalazioni',
  'mod.intro':
    'Le segnalazioni di Sytue Cast e di Sytue Rooms arrivano in questa coda e in quella di chi amministra l’organizzazione proprietaria del contenuto. Risolverne una qui la chiude anche lì, segnata come gestita da Sytue. Ogni consultazione e ogni azione resta nel registro della piattaforma e in quello dell’organizzazione.',
  'mod.suspend': 'Per sospendere un’organizzazione, vai a Piattaforma.',
  'mod.service.cast': 'Sytue Cast',
  'mod.service.rooms': 'Sytue Rooms',
  'mod.status.open': 'Aperte',
  'mod.status.resolved': 'Risolte',
  'mod.loading': 'Caricamento…',
  'mod.empty': 'Nessuna segnalazione qui.',
  'mod.col.date': 'Data',
  'mod.col.org': 'Organizzazione',
  'mod.col.what': 'Segnalato',
  'mod.col.reason': 'Motivo',
  'mod.col.content': 'Cosa conserva la segnalazione',
  'mod.col.actions': 'Azione',
  'mod.kind.video': 'Video',
  'mod.kind.comment': 'Commento',
  'mod.kind.chat': 'Messaggio della chat in diretta',
  'mod.kind.person': 'Persona',
  'mod.kind.channel': 'Canale',
  'mod.kind.message': 'Messaggio',
  'mod.kind.member': 'Persona in una conversazione',
  'mod.reason.harassment': 'Molestie',
  'mod.reason.sexual': 'Contenuti sessuali o nudità',
  'mod.reason.violence': 'Violenza',
  'mod.reason.spam': 'Spam o truffa',
  'mod.reason.copyright': 'Diritto d’autore',
  'mod.reason.other': 'Altro',
  'mod.action.dismiss': 'Archiviare',
  'mod.action.delete': 'Eliminare',
  'mod.action.unlist': 'Togliere dalla home',
  'mod.action.unpublish': 'Annullare la pubblicazione',
  'mod.action.block': 'Bloccare nell’organizzazione',
  'mod.action.remove': 'Rimuovere dal canale',
  'mod.from.platform': 'da Sytue',
  'mod.from.org': 'dall’organizzazione',
  'mod.confirm.title': '{action}?',
  'mod.confirm.body':
    'Resta a tuo nome, come Sytue, nel registro di {org}, e la segnalazione si chiude anche nella sua coda.',
  'mod.toast.done': 'Segnalazione risolta.',
  'mod.toast.failed': 'Non è stato possibile. Forse la tua elevazione è scaduta.',
  'mod.error.forbidden': 'Prima eleva il tuo ambito di piattaforma',
  'mod.error.forbiddenBody':
    'La coda delle segnalazioni si apre come il resto della console di piattaforma: con la verifica in due passaggi e un’elevazione attiva. Elevati da Piattaforma e torna qui.',
  'mod.error.unavailable': 'Il servizio non risponde',
  'mod.error.unavailableBody':
    'Sytue ID non ha potuto confermare il tuo accesso, o il servizio non ha risposto. Senza questa conferma non si mostra nulla. Riprova tra un momento.',
};
