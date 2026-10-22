/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ITALIANO del portal Sytue One. Traducción del original inglés
 * (`en.ts`), que manda: el tipo exige aquí exactamente sus mismas claves.
 *
 * Trato informal (tu) en todo el catálogo, como el resto de la suite
 * («controlla la connessione»). Terminología: «Casella di posta», «Dominio»,
 * «Record» (registro DNS). Comillas angulares («…») y apóstrofo tipográfico (’),
 * que además evita escapes dentro de las comillas simples de JS.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const it: AppCatalog<typeof en> = {
  // --- Títulos de página -------------------------------------------------
  'page.home': 'Sytue One — Home',
  'page.search': 'Cerca in Sytue One',
  'page.searchQuery': '«{query}» — Cerca in Sytue One',
  'page.devices': 'Collega i tuoi dispositivi — Sytue One',
  'page.data': 'I tuoi dati — Sytue One',
  'page.downloads': 'Download — Sytue One',
  'downloads.title': 'Scarica le applicazioni',
  'downloads.lead':
    'Applicazioni native per il tuo computer. Si installano, vivono nel Dock o nel menu Start e ti avvisano con le notifiche di sistema.',
  'downloads.version': 'Versione {version}',
  'downloads.get': 'Scarica',
  'downloads.forYou': 'Per il tuo {platform}',
  'downloads.others': 'Altre piattaforme',
  'downloads.all': 'Tutte le piattaforme',
  'downloads.empty': 'Non ci sono ancora installer pubblicati',
  'downloads.emptyNoRelease':
    'Nessuna versione ha ancora pubblicato i suoi installer. Compariranno qui appena succede; da parte tua non serve fare nulla.',
  'downloads.emptyUnconfigured':
    'Questa installazione non sa dove cercare gli installer. Lo configura un operatore.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Impossibile caricare i download',
  'downloads.failedBody': 'Il Hub non risponde in questo momento. Riprova tra qualche secondo.',
  'downloads.savedTitle': 'Download completato',
  'downloads.saveFailedTitle': 'Impossibile scaricare il programma di installazione',
  'page.activity': 'Attività recente — Sytue One',
  'page.welcome': 'Ti diamo il benvenuto in Sytue',
  // --- Attività recente (Sytue Hub) --------------------------------------
  'activity.overline': 'Attività',
  'activity.heading': 'Attività recente',
  'activity.intro': 'Che cosa è successo di recente in e-mail, file, documenti, eventi e stanze.',
  'activity.today': 'Oggi',
  'activity.yesterday': 'Ieri',
  'activity.empty':
    'Ancora nulla di recente. Man mano che usi Sytue, qui comparirà ciò su cui lavori.',
  'activity.failedTitle': 'Impossibile caricare l’attività',
  'activity.failedBody': 'Il Hub non risponde in questo momento. Riprova tra qualche secondo.',
  'activity.retry': 'Riprova',
  'activity.all': 'Tutto',
  'activity.filterBy': 'Filtra per prodotto',
  'activity.count': { one: '{count} elemento', other: '{count} elementi' },
  'page.inbox': 'Notifiche — Sytue One',
  // --- Notifiche (Sytue Hub) ---------------------------------------------
  'inbox.overline': 'Notifiche',
  'inbox.heading': 'Notifiche',
  'inbox.intro': 'Tutto ciò che la suite ha da dirti, in un unico posto.',
  'inbox.all': 'Tutte',
  'inbox.unreadFilter': { one: '{count} non letta', other: '{count} non lette' },
  'inbox.filterBy': 'Filtra le notifiche',
  'inbox.markAll': 'Segna tutte come lette',
  'inbox.loading': 'Caricamento delle notifiche…',
  'inbox.unreadDot': 'Non letta',
  'inbox.errorTitle': 'Impossibile caricare le notifiche',
  'inbox.errorBody': 'Il Hub non risponde in questo momento. Riprova tra qualche secondo.',
  'inbox.retry': 'Riprova',
  'inbox.emptyTitle': 'Niente da leggere',
  'inbox.emptyBody': 'Quando qualcosa richiede la tua attenzione, comparirà qui.',
  'inbox.emptyUnreadTitle': 'Sei in pari',
  'inbox.emptyUnreadBody': 'Hai letto tutto. Passa a «Tutte» per rivedere le precedenti.',
  'inbox.prefsTitle': 'Che cosa vuoi ricevere',
  'inbox.prefsBody':
    'Se spegni un prodotto, smette di avvisarti qui e via e-mail. Puoi riaccenderlo quando vuoi.',

  // --- Lanzador ----------------------------------------------------------
  'launcher.heading': 'Le tue applicazioni',
  'launcher.greeting.morning': 'Buongiorno',
  'launcher.greeting.afternoon': 'Buon pomeriggio',
  'launcher.greeting.evening': 'Buonasera',
  'launcher.more': 'Altro',
  'launcher.apps': 'Applicazioni',
  'launcher.connected': 'Connesso',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Disattivato',

  'product.mail': 'Posta elettronica',
  'product.drive': 'File e collaborazione',
  'product.calendar': 'Calendario',
  'product.contacts': 'Contatti',
  'product.docs': 'Documenti, fogli di calcolo e presentazioni',
  'product.rooms': 'Canali, messaggi e chiamate',
  'product.cloud': 'Piattaforma applicativa (PaaS)',
  'product.console': 'Amministrazione e organizzazioni',
  'product.id': 'Identità, profilo e sicurezza',

  'quick.heading': 'Azioni rapide',
  'quick.search': 'Cerca in tutta la suite',
  'quick.compose': 'Scrivi un’e-mail',
  'quick.newDoc': 'Nuovo documento',
  'quick.upload': 'Carica su Drive',
  'quick.newEvent': 'Crea un evento',
  'quick.download': 'Scarica l’applicazione',
  'quick.devices': 'Collega i tuoi dispositivi',
  'quick.data': 'I tuoi dati',

  'storage.connectTitle': 'Collega uno spazio esterno',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV e altro.',

  'recent.heading': 'Recenti in tutta la suite',
  'recent.viewAll': 'Mostra tutto',
  'recent.loading': 'Caricamento della tua attività recente…',
  'recent.errorTitle': 'Non siamo riusciti a caricare la tua attività',
  'recent.errorBody': 'L’Hub non risponde in questo momento. Il tuo lavoro è al sicuro; riprova.',
  'recent.retry': 'Riprova',
  'recent.emptyTitle': 'Nessuna attività recente',
  'recent.emptyBody': 'Quando lavorerai in Mail, Drive, Calendar o Rooms, lo vedrai qui.',
  'recent.emptyCta': 'Inizia da Drive',

  // --- Búsqueda cross-app ------------------------------------------------
  'search.overline': 'Ricerca',
  'search.heading': 'Cerca in tutta la suite',
  'search.placeholder': 'Cerca in Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Cerca in Sytue One',
  'search.submit': 'Cerca',
  'search.hint':
    'Scrivi qui sopra per cercare in una volta sola nella tua posta, nei tuoi file, nei tuoi documenti, nei tuoi eventi, nei tuoi contatti e nelle tue stanze. I risultati si aprono direttamente nel prodotto a cui appartengono.',
  'search.failedTitle': 'Non è stato possibile completare la ricerca',
  'search.failedBody': 'L’Hub non risponde in questo momento. Riprova tra qualche secondo.',
  'search.retry': 'Riprova',
  'search.empty': 'Nessun risultato per «{query}». Prova con altre parole.',
  'search.count': { one: '{count} risultato', other: '{count} risultati' },

  // --- Bienvenida --------------------------------------------------------
  'welcome.heading': 'Ti diamo il benvenuto in Sytue',
  'welcome.lead':
    'Bastano pochi minuti e sei operativo. Procediamo passo dopo passo; se ti fermi a metà, quando torni riprendiamo da dove eravamo rimasti.',

  'data.title': 'I tuoi dati',
  'data.lead':
    'Scarica una copia di tutto quello che hai in Sytue One, quando vuoi e senza chiedere niente a nessuno.',
  'data.export.section': 'Scarica una copia',
  'data.export.body':
    'Un unico file ZIP con la tua posta, i file, i calendari, i contatti, le note, i documenti, le foto e i messaggi. Ciò che ha già un formato standard resta così com’è: le email in .eml, i calendari in .ics, i contatti in .vcf e i file come li hai caricati.',
  'data.export.formatLegend': 'Formato degli elenchi',
  'data.export.formatHelp':
    'Gli elenchi descrivono cosa c’è nella copia: i tuoi messaggi, eventi, contatti o file, uno per riga.',
  'data.export.orgHelp':
    'Ogni copia è di una sola organizzazione. Se ne scegli un’altra, entri in quella e il download parte da solo.',
  'data.export.json': 'Per altre app e programmi.',
  'data.export.csv': 'Da aprire in Excel, Numbers o Google Sheets.',
  'data.export.xml': 'Per i sistemi che richiedono XML.',
  'data.export.action': 'Scarica la copia',
  'data.export.startedTitle': 'Il download è iniziato',
  'data.export.startedBody':
    'Il browser continua a salvarla anche se chiudi questa pagina. Se hai molti file, ci vorrà un po’.',
  'data.export.scope':
    'La copia include i dati dell’organizzazione con cui hai effettuato l’accesso. Il file di riepilogo al suo interno indica cosa è stato incluso di ogni app.',
  'data.move.section': 'Portarlo su un altro servizio',
  'data.move.body':
    'Per trasferirlo direttamente su Google Workspace o Microsoft 365, senza scaricare nulla: se il tuo account è personale o amministri la tua organizzazione, avvialo tu da Sytue Admin, in Migrazioni; altrimenti chiedilo a chi la amministra.',
  'data.move.action': 'Vai a Migrazioni',

  // --- Conectar tus dispositivos ----------------------------------------
  'devices.title': 'Collega i tuoi dispositivi',
  'devices.lead':
    'Porta la posta, il calendario, i contatti e i file di Sytue sul telefono o sul computer. Ti guidiamo passo dopo passo.',
  'devices.leadShort':
    'Porta la posta, il calendario e i contatti di Sytue sul telefono o sul computer.',
  'devices.unavailable.section': 'In questo momento non possiamo mostrarti i dati',
  'devices.unavailable.title': 'Non siamo riusciti a recuperare la configurazione',
  'devices.unavailable.body':
    'Riprova tra un momento. Se il problema resta, scrivi a chi amministra Sytue nella tua organizzazione.',
  'devices.retry': 'Riprova',
  'devices.copy': 'Copia',
  'devices.copyFailTitle': 'Non è stato possibile copiare',
  'devices.copyFailBody': 'Selezionalo e copialo a mano.',
  'devices.copied.address': 'Il tuo indirizzo copiato',
  'devices.copied.imap': 'Server in entrata copiato',
  'devices.copied.port': 'Porta copiata',
  'devices.copied.smtp': 'Server in uscita copiato',
  'devices.copied.url': 'Indirizzo copiato',
  'devices.copied.drive': 'Indirizzo di Drive copiato',
  'devices.oneClick.thunderbird': 'Configura Thunderbird',
  'devices.oneClick.outlook': 'Configura Outlook',
  'devices.oneClick.android': 'Configura su Android',
  'devices.oneClick.body':
    'Crea una password per app solo per questo programma, la copia e te la mostra una sola volta.',
  'devices.oneClick.failTitle': 'Non è stato possibile creare la password per app',
  'devices.oneClick.failBody': 'Riprova tra un momento. Puoi anche crearne una in Il mio account.',
  'devices.oneClick.onceTitle': 'Copiata. Viene mostrata solo questa volta',
  'devices.oneClick.onceBody':
    'Incollala dove il programma chiede la password. Se la perdi, revocala in Il mio account e creane un’altra.',
  'devices.oneClick.passwordLabel': 'Password per app',
  'devices.copied.password': 'Password per app copiata',
  'devices.oneClick.notOpened':
    'Non è stato possibile aprire {program} su questo computer: aprilo tu e segui questi passaggi.',
  'devices.oneClick.next.thunderbird':
    'Nella procedura per un nuovo account di Thunderbird, scrivi il tuo indirizzo e incolla la password: trova i server da solo. Calendari e contatti: aggiungi un calendario di rete e una rubrica CardDAV con lo stesso indirizzo e la stessa password (se chiede la posizione, copiala da qui sotto).',
  'devices.oneClick.next.outlook':
    'In «Aggiungi account» di Outlook, scrivi il tuo indirizzo e incolla la password: Outlook trova i server da solo.',
  'devices.oneClick.next.android':
    'Nella tua app di posta, aggiungi un account «Altro (IMAP)» con il tuo indirizzo e incolla la password. Calendari e contatti: DAVx5 con gli indirizzi qui sotto e la stessa password.',
  'devices.android.davx5Calendars': 'Calendari in DAVx5',
  'devices.android.davx5Contacts': 'Contatti in DAVx5',
  'devices.android.davx5Install': 'Installa DAVx5 da F-Droid',

  'devices.platform.iphone.title': 'iPhone o iPad',
  'devices.platform.iphone.body': 'Installi un profilo e posta, calendario e contatti sono pronti.',
  'devices.platform.iphone.note': 'Il modo più rapido: un file e la tua password.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'Lo stesso profilo dell’iPhone, installato da Impostazioni di Sistema.',
  'devices.platform.mac.note': 'Apple Mail, Calendario e Contatti.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Scrivi il tuo indirizzo e la tua password; il resto Outlook lo trova da solo.',
  'devices.platform.outlook.note': 'Solo posta. Calendario e contatti vanno a parte.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Si configura da solo con il tuo indirizzo; calendario e contatti si aggiungono dopo.',
  'devices.platform.thunderbird.note': 'Windows, Mac o Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'La tua app di posta per la casella e DAVx5 per calendario e contatti.',
  'devices.platform.android.note': 'Non c’è un’app Sytue: si usano quelle di sistema.',
  // Compatibilidad drop-in: Drive, Calendar y Contacts hablan las APIs de Google y
  // de Microsoft Graph. Estaba construido y no se decia en ninguna parte.
  'devices.platform.apis.title': 'Uno strumento che parla Google o Microsoft',
  'devices.platform.apis.body':
    'Punta a Sytue un’integrazione scritta per Google Drive, Google Calendar, Google Contacts o Microsoft Graph, senza toccarne il codice.',
  'devices.platform.apis.note': 'Solo dove puoi cambiare l’indirizzo di base.',
  'devices.apis.step1.title': 'Cambia l’indirizzo di base',
  'devices.apis.step1.body':
    'Nello strumento, sostituisci l’indirizzo che chiama con quello del prodotto e del dialetto che ti serve. Il resto della configurazione resta com’è.',
  'devices.apis.step2.title': 'Usa le tue credenziali Sytue',
  'devices.apis.step2.body':
    'L’autenticazione è il tuo token di Sytue ID, non un account Google o Microsoft: i permessi sono gli stessi che hai già qui.',
  'devices.apis.identityGoogle': 'Identità · OAuth e directory di Google',
  'devices.apis.step3.title': 'Un’app di terze parti, come se fosse Google',
  'devices.apis.step3.body':
    'Se lo strumento accede con l’OAuth di Google, l’amministrazione dell’organizzazione lo registra in Sytue Admin con le autorizzazioni Google necessarie. L’app chiede quelle autorizzazioni e riceve i dati di Sytue; Google non interviene e non vede nulla.',
  'devices.apis.driveGoogle': 'Drive · API di Google Drive',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · API di Google Calendar',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · API People di Google',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Che cosa è coperto',
  'devices.apis.expectBody':
    'Elencare, leggere, creare, modificare ed eliminare. Ciò che non è coperto risponde con un errore chiaro, mai con un successo vuoto.',
  'devices.copied.apiBase': 'Indirizzo di base copiato',
  'devices.check.apis.label': 'Lo strumento',
  'devices.check.apis.value': 'Elenca i tuoi contenuti Sytue come se fosse il servizio originale.',
  'devices.platform.drive.title': 'Esplora file (Drive)',
  'devices.platform.drive.body':
    'Vedi i tuoi file di Sytue Drive come un’altra unità del computer.',
  'devices.platform.drive.note': 'Finder su Mac, Esplora file su Windows.',

  'devices.mail.address': 'Il tuo indirizzo (nome utente)',
  'devices.mail.imapHost': 'Server in entrata (IMAP)',
  'devices.mail.imapPort': 'Porta in entrata',
  'devices.mail.smtpHost': 'Server in uscita (SMTP)',
  'devices.mail.smtpPort': 'Porta in uscita',
  'devices.mail.hint':
    'Crittografia SSL/TLS su entrambi, e la stessa password di Sytue che usi per accedere dal web.',
  'devices.dav.caldav': 'Calendario (CalDAV)',
  'devices.dav.carddav': 'Contatti (CardDAV)',
  'devices.dav.user': 'Nome utente',

  'devices.apple.download': 'Scarica il profilo Sytue',
  'devices.apple.step1.title': 'Scarica il profilo',
  'devices.apple.step1.body':
    'Premi il pulsante qui sopra. Viene scaricato un piccolo file con le impostazioni del tuo account.',
  'devices.apple.step2.iphone.title': 'Installalo da Impostazioni',
  'devices.apple.step2.iphone.body':
    'Apri Impostazioni. In cima compare «Profilo scaricato»: toccalo e premi Installa.',
  'devices.apple.step2.mac.title': 'Installalo da Impostazioni di Sistema',
  'devices.apple.step2.mac.body':
    'Apri Impostazioni di Sistema › Privacy e sicurezza › Profili, fai doppio clic sul profilo scaricato e premi Installa.',
  'devices.apple.step3.title': 'Non devi digitare nessuna password',
  'devices.apple.step3.body':
    'Il profilo contiene una propria password per app, creata solo per questo dispositivo. Puoi revocarla in qualsiasi momento in Il mio account.',
  'devices.apple.step4.title': 'Fatto',
  'devices.apple.step4.body':
    'Posta, calendario e contatti si aggiungono in una volta sola. Non c’è nessun server da scrivere.',
  'devices.apple.manualTitle': 'Se preferisci farlo a mano',
  'devices.apple.manualBody': 'Puoi anche creare l’account manualmente con questi dati.',
  'devices.manualValues': 'Vedi i valori manuali',

  'devices.desktop.step1.title': 'Aggiungi un account di posta',
  'devices.desktop.step1.outlook': 'In Outlook: File › Aggiungi account.',
  'devices.desktop.step1.thunderbird': 'In Thunderbird: Account › Configura un account › E-mail.',
  'devices.desktop.step2.title': 'Scrivi solo il tuo indirizzo e la tua password',
  'devices.desktop.step2.outlook':
    'Nient’altro. Outlook chiede la configurazione a Sytue e la compila per te.',
  'devices.desktop.step2.thunderbird':
    'Nient’altro. Thunderbird chiede la configurazione a Sytue e la compila per te.',
  'devices.desktop.step3.title': 'Conferma',
  'devices.desktop.step3.body': 'Quando ti mostra i server, accetta. Si collega come account IMAP.',
  'devices.desktop.emailLabel': 'Il tuo indirizzo e-mail',
  'devices.desktop.notFoundTitle': 'Non l’ha trovata da solo?',
  'devices.desktop.notFoundBody': 'Scegli la configurazione manuale e usa questi valori.',
  'devices.thunderbird.davTitle': 'Calendario e contatti',
  'devices.thunderbird.davBody': 'In Thunderbird si aggiungono a parte, con questi indirizzi.',
  'devices.outlook.davTitle': 'Calendario e contatti in Outlook',
  'devices.outlook.davBody':
    'Outlook per Windows non supporta CalDAV/CardDAV, quindi calendario e contatti di Sytue lì non si sincronizzano. Usali dal web di Sytue, oppure da un client che parli CalDAV/CardDAV (Thunderbird, Calendario di Apple).',

  'devices.android.mailTitle': 'Posta',
  'devices.android.mailBody': 'Con l’app di posta che già usi (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Aggiungi un account',
  'devices.android.mail1.body': 'Scegli «Altro» o «Personale (IMAP)», non «Google».',
  'devices.android.mail2.title': 'Scrivi il tuo indirizzo e la tua password',
  'devices.android.mail2.body': 'Se ti chiede i server, copiali qui sotto.',
  'devices.android.davTitle': 'Calendario e contatti',
  'devices.android.davBody':
    'Android non include CalDAV/CardDAV: ti serve DAVx5, un’app gratuita che li aggiunge al calendario e alla rubrica del telefono.',
  'devices.android.dav1.title': 'Installa DAVx5',
  'devices.android.dav1.body':
    'La trovi su Google Play e su F-Droid. Sytue non la pubblica né la mantiene.',
  'devices.android.dav2.title': 'Aggiungi un account con «Accesso con URL e nome utente»',
  'devices.android.dav2.body':
    'Incolla l’indirizzo qui sotto e usa la tua e-mail e la tua password di Sytue.',
  'devices.android.dav3.title': 'Scegli cosa sincronizzare',
  'devices.android.dav3.body': 'Spunta i tuoi calendari e la tua rubrica.',

  'devices.drive.step1.title': 'Apri la connessione a un server',
  'devices.drive.step1.body':
    'Su Mac: Finder › Vai › Connessione al server (⌘K). Su Windows: Esplora file › Questo PC › Connetti unità di rete, e spunta «Connetti con credenziali diverse».',
  'devices.drive.step2.title': 'Incolla questo indirizzo',
  'devices.drive.step2.body': 'È la cartella con i tuoi file di Sytue Drive.',
  'devices.drive.step3.title': 'Accedi con il tuo account Sytue',
  'devices.drive.step3.body': 'Nome utente: il tuo indirizzo e-mail. Password: quella di Sytue.',
  'devices.drive.urlLabel': 'Indirizzo di Drive (WebDAV)',
  'devices.drive.expectTitle': 'Cosa aspettarti',
  'devices.drive.windowsExpectBody':
    'Su Windows, Drive compare in Esplora file come la cartella «Sytue Drive». I file si scaricano quando li apri; clic destro › «Mantieni sempre su questo dispositivo» per averli offline. Ciò che salvi o crei lì viene caricato su Drive.',
  'devices.drive.expectBody':
    'I file si aprono dal server, non vengono copiati sul disco. Per lavorare senza connessione, scarica prima quello che ti serve.',

  'devices.check.mail.label': 'La posta',
  'devices.check.mail.value': 'Arrivano i messaggi nuovi e puoi inviarne uno a te stesso.',
  'devices.check.dav.label': 'Calendario e contatti',
  'devices.check.dav.value':
    'Un evento creato dal web di Sytue compare sul dispositivo in pochi minuti.',
  'devices.check.profile.label': 'Il profilo',
  'devices.check.profile.value': '«Sytue One» compare nell’elenco dei profili installati.',
  'devices.check.drive.label': 'L’unità',
  'devices.check.drive.value': 'Vedi le tue cartelle di Drive e puoi aprire un file.',
  'devices.check.write.label': 'La scrittura',
  'devices.check.write.value': 'Crei una cartella e compare anche in Sytue Drive dal browser.',

  'devices.step1.title': 'Cosa usi',
  'devices.step1.heading': 'Dove vuoi il tuo account Sytue?',
  'devices.step1.description':
    'Scegline uno. Se ne hai più di uno, ripeti la procedura per ciascuno.',
  'devices.step2.title': 'Come si fa',
  'devices.step2.heading': 'Configura {platform}',
  'devices.step2.headingFallback': 'Come si fa',
  'devices.step2.description':
    'Segui questi passaggi. Tutto quello che devi scrivere si copia con un pulsante.',
  'devices.step2.next': 'Fatto',
  'devices.step3.title': 'Verifica',
  'devices.step3.heading': 'Come capire che ha funzionato',
  'devices.step3.description': 'Se tutto questo è vero, è collegato come si deve.',
  'devices.step3.helpTitle': 'Qualcosa non torna?',
  'devices.step3.helpBody':
    'Di solito è la password: usa la stessa con cui accedi a Sytue dal browser. Se hai la verifica in due passaggi, la tua organizzazione potrebbe richiedere una password per app; chiedi a chi amministra Sytue.',
  'devices.step3.again': 'Collega un altro dispositivo',

  // --- Alta guiada (onboarding) -----------------------------------------
  'ob.error.generic': 'Non siamo riusciti a completare l’operazione. Riprova.',
  'ob.failedTitle': 'Non siamo riusciti a caricare la tua registrazione',
  'ob.failedBody':
    'Il servizio di identità non risponde in questo momento. Non si è perso nulla: riprova tra un minuto.',
  'ob.retry': 'Riprova',
  'ob.preparing': 'Stiamo preparando la tua registrazione…',
  'ob.lead':
    'Pochi minuti e sei operativo. Procediamo passo dopo passo; se ti fermi a metà, al tuo ritorno riprendiamo da dove eravamo rimasti.',
  'ob.saveFailedTitle': 'Non è stato possibile salvare',
  'ob.saveFailedBody': 'Riprova tra un momento.',
  'ob.copy': 'Copia',
  'ob.copyFailTitle': 'Non è stato possibile copiare',
  'ob.copyFailBody': 'Selezionalo con il mouse e copialo a mano.',
  'ob.copied.recordType': 'Tipo di record copiato',
  'ob.copied.recordName': 'Nome del record copiato',
  'ob.copied.recordValue': 'Valore del record copiato',

  'ob.roleOption.member': 'Membro — usa la suite, non amministra nulla',
  'ob.roleOption.admin': 'Amministratore — può gestire tutta l’organizzazione',
  'ob.roleOption.billing': 'Fatturazione — si occupa delle fatture',
  'ob.roleName.owner': 'Proprietario',
  'ob.roleName.admin': 'Amministratore',
  'ob.roleName.member': 'Membro',
  'ob.roleName.billing': 'Fatturazione',

  'ob.mode.title': 'Inizio',
  'ob.mode.heading': 'Per chi è Sytue?',
  'ob.mode.description': 'Da questo dipende cosa ti chiediamo. Potrai cambiarlo più avanti.',
  'ob.mode.individual.title': 'Solo per me',
  'ob.mode.individual.body':
    'La tua posta, i tuoi file e il tuo calendario, con un indirizzo tuo. Se un giorno ti servirà invitare qualcuno, potrai farlo.',
  'ob.mode.org.title': 'Per la mia organizzazione',
  'ob.mode.org.body':
    'Registri la tua squadra, distribuisci chi può fare cosa e amministri tutto da un solo posto.',
  'ob.mode.org.badge': 'Il più comune',

  'ob.identity.titleOrg': 'La tua organizzazione',
  'ob.identity.titleMe': 'Il tuo account',
  'ob.identity.headingOrg': 'Come si chiama la tua organizzazione?',
  'ob.identity.headingMe': 'Come vuoi chiamare il tuo account?',
  'ob.identity.descriptionCreated': 'È già creata. Puoi cambiare il nome; l’indirizzo resta com’è.',
  'ob.identity.description': 'Con questo creiamo il tuo spazio e il tuo indirizzo in Sytue.',
  'ob.identity.nextCreated': 'Continua',
  'ob.identity.nextOwnDomain': 'Crea e mostrami cosa aggiungere al mio dominio',
  'ob.identity.next': 'Crea e continua',
  'ob.identity.nameLabel': 'Nome',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Marta Rossi',
  'ob.identity.nameHintOrg':
    'È quello che vede la tua squadra quando accede. Puoi cambiarlo quando vuoi.',
  'ob.identity.nameHintMe': 'Lo vedi solo tu. Puoi cambiarlo quando vuoi.',
  'ob.identity.slugLabel': 'Il tuo indirizzo in Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Il tuo indirizzo è {subdomain}. Non si può cambiare: da lì dipendono la tua posta e i tuoi file.',
  'ob.identity.slugEmpty': 'Scegli un nome corto. Sarà il tuo indirizzo: acme.{suite}',
  'ob.identity.slugChecking': 'Stiamo controllando se è libero…',
  'ob.identity.slugFree': 'Libero. Il tuo indirizzo sarà {subdomain}',
  'ob.identity.slugTaken': 'Non si può usare.',
  'ob.identity.firstTitle': 'Prima creiamo il tuo spazio',
  'ob.identity.firstBody':
    'Quando premi il pulsante qui sotto creiamo il tuo indirizzo Sytue e ti mostriamo proprio qui il record esatto da aggiungere al tuo dominio. Non serve che tu lo abbia pronto adesso.',

  'ob.domain.legend': 'E il tuo dominio?',
  'ob.domain.keep.title': 'Tengo {host}',
  'ob.domain.keep.placeholder': 'il-mio-nome',
  'ob.domain.keep.body':
    'Non devi toccare nulla: pubblicarlo tocca a noi. È quello che consigliamo per iniziare.',
  'ob.domain.own.title': 'Ho già un dominio e voglio usarlo (acme.com)',
  'ob.domain.own.body':
    'Se è su Cloudflare, lo configuriamo noi. Se no, ti diciamo il record esatto da aggiungere e lo verifichiamo proprio qui.',
  'ob.domain.search.title': 'Non ho ancora un dominio',
  'ob.domain.search.body':
    'Controlliamo se il nome che vuoi è libero e quanto costa all’anno. È solo una ricerca: qui non si compra nulla.',
  'ob.domain.hostLabel': 'Il tuo dominio',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Scrivilo così com’è: acme.com, senza «https://» e senza «www».',
  'ob.domain.verified': '{host} verificato',

  'ob.dns.legend': 'Dov’è {domain}?',
  'ob.dns.fallback': 'il tuo dominio',
  'ob.dns.cf.title': 'Su Cloudflare — configuratelo voi',
  'ob.dns.cf.body':
    'Ci dai un permesso temporaneo sul tuo account e creiamo noi i record. Prima di toccare qualsiasi cosa ti mostriamo cosa stiamo per cambiare.',
  'ob.dns.cf.badge': 'Senza toccare il DNS',
  'ob.dns.manual.title': 'Con un altro provider — lo faccio io',
  'ob.dns.manual.body':
    'Ti diamo il record esatto da copiare nel tuo pannello DNS e lo verifichiamo qui appena ce l’hai.',
  'ob.dns.manualSummary': 'Preferisco aggiungere i record da solo',

  'ob.people.title': 'La tua squadra',
  'ob.people.heading': 'Chi inviti per primo?',
  'ob.people.description':
    'Ogni persona riceve un’e-mail con un link personale per creare la propria password e accedere. Puoi saltare questo passaggio e farlo più tardi.',
  'ob.people.next': 'Continua',
  'ob.people.emailsLabel': 'Indirizzi e-mail',
  'ob.people.emailsPlaceholder': 'anna@acme.com, bruno@acme.com',
  'ob.people.emailsHint': 'Puoi incollarne più di uno alla volta, separati da virgole o da a capo.',
  'ob.people.roleLabel': 'Con quale ruolo entrano',
  'ob.people.roleHint': 'Si può cambiare in qualsiasi momento da Sytue Admin.',
  'ob.people.send': 'Invia gli inviti',
  'ob.people.sendNote': 'Non si invia nulla finché non premi qui.',
  'ob.people.pendingTitle': 'Inviti in attesa di essere accettati',
  'ob.people.emptyTitle': 'Non hai ancora invitato nessuno',
  'ob.people.emptyBody':
    'Puoi farlo adesso o più tardi, da Sytue Admin. La tua organizzazione funziona lo stesso.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Invito inviato',
  'ob.invite.already': 'È già nella tua organizzazione',
  'ob.invite.invalid': 'Questo non sembra un indirizzo e-mail',
  'ob.invite.notSent': 'Non è stato possibile inviare l’e-mail',
  'ob.invite.failed': 'Non è stato possibile invitarli',
  'ob.invite.default':
    'Riceveranno un’e-mail con un link personale per creare la propria password e accedere.',

  'ob.billing.title': 'Fatturazione',
  'ob.billing.heading': 'A chi sono intestate le fatture?',
  'ob.billing.description':
    'Aggiungi i dati di fatturazione e scegli i posti a pagamento. L’accesso parte solo dopo la conferma di Mollie.',
  'ob.billing.next': 'Continua',
  'ob.billing.pay': 'Paga in sicurezza con Mollie',
  'ob.billing.verify': 'Verifica il pagamento',
  'ob.billing.legalLabel': 'Ragione sociale',
  'ob.billing.legalPlaceholder': 'ACME Europe S.r.l.',
  'ob.billing.legalHint':
    'Il nome legale dell’azienda, esattamente come compare sulle tue fatture.',
  'ob.billing.taxLabel': 'Identificativo fiscale',
  'ob.billing.taxPlaceholder': 'IT12345678901',
  'ob.billing.taxHint': 'Il codice fiscale o la partita IVA del tuo paese.',
  'ob.billing.addressLabel': 'Indirizzo',
  'ob.billing.addressPlaceholder': 'Via Roma 1, 20121 Milano, Italia',
  'ob.billing.addressHint': 'La sede legale che va in fattura.',
  'ob.billing.emailLabel': 'E-mail per le fatture',
  'ob.billing.emailPlaceholder': 'fatture@acme.com',
  'ob.billing.emailHint':
    'È lì che mandiamo le fatture. Per impostazione predefinita, l’e-mail del tuo account.',
  'ob.billing.note':
    'Ti chiediamo solo quello che serve per fatturarti. Niente di tutto questo viene condiviso con nessuno.',
  'ob.billing.seatsLabel': 'Posti a pagamento',
  'ob.billing.seatsHint': '{seats} posto/i × {price}/Utente/Mese · totale {total}/Mese.',
  'ob.billing.active': 'Abbonamento attivo',
  'ob.billing.activeBody': 'Mollie ha confermato il pagamento. L’accesso a Sytue One è attivo.',
  'ob.billing.pending': 'Stiamo verificando il pagamento',
  'ob.billing.pendingBody':
    'Mollie non ha ancora confermato il pagamento. Attendi qualche secondo e verifica di nuovo; non riceverai un doppio addebito.',

  'ob.roles.title': 'Autorizzazioni',
  'ob.roles.heading': 'Vuoi distribuire i compiti di amministrazione?',
  'ob.roles.description':
    'Sono ruoli già pronti: scegli quelli che vuoi e li creiamo noi. Poi li assegni a chi vuoi da Sytue Admin. Puoi saltare questo passaggio.',
  'ob.roles.next': 'Continua',
  'ob.roles.already': 'Già creato',
  'ob.roles.seePermissions': 'Vedi le autorizzazioni esatte',
  'ob.roles.helpTitle': 'Il controllo resta tuo',
  'ob.roles.helpBody':
    'Il tuo account ha la proprietà dell’organizzazione: questi ruoli non gli tolgono nulla. Puoi modificarli o eliminarli quando vuoi in Sytue Admin → Ruoli.',

  'ob.done.title': 'Fatto',
  'ob.done.headingCompleted': 'Tutto pronto',
  'ob.done.heading': 'Ecco cosa hai configurato',
  'ob.done.description': 'Viene dal tuo account vero, non da quello che hai scritto qui.',
  'ob.done.next': 'Entra in Sytue',
  'ob.done.warnTitle': '{host} non è ancora verificato',
  'ob.done.warnBody':
    'Nel frattempo Sytue funziona lo stesso, con {subdomain}. Quando aggiungi il record al tuo dominio, torna al passaggio «{step}» e premi «Verifica ora».',
  'ob.done.nextTitle': 'Come proseguire',
  'ob.next.devices.title': 'Collega il telefono e il computer',
  'ob.next.devices.body':
    'Lascia posta, calendario e contatti al lavoro nelle applicazioni di sempre.',
  'ob.next.migrations.title': 'Porta i tuoi contenuti da Microsoft 365 o da Google',
  'ob.next.migrations.body':
    'Copiamo posta, file e calendari senza farti perdere nulla per strada.',
  'ob.next.apps.title': 'Collega le tue applicazioni',
  'ob.next.apps.body': 'Gli strumenti che la tua squadra già usa, collegati al tuo account Sytue.',
  'ob.next.users.title': 'Continua a registrare la tua squadra',
  'ob.next.users.body': 'Invita chi manca e controlla chi ha quale ruolo.',

  'ob.sum.org': 'Organizzazione',
  'ob.sum.account': 'Il tuo account',
  'ob.sum.address': 'Indirizzo',
  'ob.sum.addressLive': '{host} · pubblicato',
  'ob.sum.addressPending': '{host} · in attesa di pubblicazione',
  'ob.sum.ownDomain': 'Dominio personale',
  'ob.sum.domainVerified': '{host} · verificato',
  'ob.sum.domainPending': '{host} · in attesa che compaia il record',
  'ob.sum.noDomain': 'Nessun dominio personale',
  'ob.sum.invitations': 'Inviti',
  'ob.sum.invitationsPending': {
    one: '1 in attesa di accettazione',
    other: '{count} in attesa di accettazione',
  },
  'ob.sum.noInvitations': 'Ancora nessuno',
  'ob.sum.billing': 'Fatturazione',
  'ob.sum.billingEmpty': 'Non compilata (non serve ancora)',
  'ob.sum.roles': 'Ruoli creati',
  'ob.sum.edit': 'Modifica',
  'ob.sum.noRoles': 'Nessuno',

  'ob.sub.liveTitle': 'Il tuo indirizzo è già pubblicato',
  'ob.sub.liveBody': '{host} risolve su internet. Non devi fare altro.',
  'ob.sub.failedTitle': 'Il tuo indirizzo non è ancora pubblicato',
  'ob.sub.failedDefault': 'Non siamo riusciti a pubblicarlo in questo momento.',
  'ob.sub.failedTail': 'Il tuo account è creato: prosegui con la registrazione e torna tra un po’.',
  'ob.sub.reservedTitle': 'Il tuo indirizzo è riservato',
  'ob.sub.reservedBody': '{host} è tuo. Pubblicarlo su internet tocca a noi: non devi fare nulla.',

  'ob.search.label': 'Il dominio che vorresti',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Scrivilo per intero, con l’estensione: acme.com, acme.it, acme.eu…',
  'ob.search.check': 'Controlla la disponibilità',
  'ob.search.useOwn': 'L’ho già registrato: collegalo',
  'ob.search.note':
    'In Sytue non si vendono domini: qui ti diciamo solo se è libero e quanto costa. Registralo dove preferisci e torna a questo passaggio, su «Ho già un dominio», per collegarlo al tuo account.',
  'ob.avail.freeTitle': '{domain} è libero',
  'ob.avail.freePrice':
    'Costa {registration} il primo anno e {renewal} a ogni rinnovo. È il prezzo di oggi su Cloudflare; dove lo registri può essere un altro.',
  'ob.avail.freeNoPrice': 'Si può registrare. Il prezzo te lo dirà il registrar dove lo compri.',
  'ob.avail.takenTitle': '{domain} è già registrato',
  'ob.avail.takenBody':
    'Ce l’ha qualcuno. Prova con un altro nome, o con un’altra estensione (.it, .eu, .net…). Se il dominio è tuo, scegli qui sopra «Ho già un dominio» e lo colleghiamo.',
  'ob.avail.unsupportedTitle': 'Non possiamo controllare quell’estensione',
  'ob.avail.unsupportedBody':
    'Non possiamo dirti se è libero né quanto costa: verificalo dal tuo registrar. Quando ce l’hai, torna qui e scegli «Ho già un dominio».',
  'ob.avail.invalidTitle': 'Questo non sembra un dominio',
  'ob.avail.invalidBody':
    'Scrivilo come acme.com: un nome, un punto e l’estensione. Senza «https://» e senza «www».',
  'ob.avail.unconfiguredTitle': 'Qui non controlliamo i prezzi dei domini',
  'ob.avail.unconfiguredBody':
    'Cercalo dal registrar che preferisci. Quando ce l’hai, torna a questo passaggio e scegli «Ho già un dominio» per collegarlo.',
  'ob.avail.unknownTitle': 'Non siamo riusciti a controllarlo in questo momento',
  'ob.avail.unknownBody':
    'Non è colpa di quello che hai scritto: non abbiamo ricevuto risposta. Riprova tra un momento.',

  'ob.dnsRec.title': 'Aggiungi questo record dove hai registrato il tuo dominio',
  'ob.dnsRec.body':
    'Cerca la sezione «DNS» o «Record» dal tuo provider, crea un record TXT e copia questi due valori. La procedura è la stessa dappertutto.',
  'ob.dnsRec.type': 'Tipo',
  'ob.dnsRec.name': 'Nome',
  'ob.dnsRec.value': 'Valore',
  'ob.dnsRec.verifiedTitle': 'Dominio verificato',
  'ob.dnsRec.verifiedBody':
    'Abbiamo trovato il record: {host} è tuo e risulta registrato come tale.',
  'ob.dnsRec.mismatchTitle': 'C’è un record, ma non è questo',
  'ob.dnsRec.mismatchBody':
    'Su {name} abbiamo trovato «{found}». Controlla di aver copiato tutto il valore e che non ne sia rimasto uno vecchio.',
  'ob.dnsRec.unreachableTitle': 'Non siamo riusciti a interrogare il tuo DNS',
  'ob.dnsRec.unreachableBody':
    'Non è colpa del tuo record: il name server non ha risposto. Riprova tra un momento.',
  'ob.dnsRec.pendingTitle': 'Il record non lo vediamo ancora',
  'ob.dnsRec.pendingBody':
    'Se l’hai appena creato è normale: le modifiche al DNS ci mettono da qualche minuto a qualche ora a comparire. Puoi proseguire con la registrazione e ricontrollare quando vuoi.',
  'ob.dnsRec.uncheckedTitle': 'Non ancora controllato',
  'ob.dnsRec.uncheckedBody':
    'Quando avrai aggiunto il record dal tuo provider, premi «Verifica ora» e guardiamo subito il tuo DNS.',
  'ob.dnsRec.empty': 'Quando premi «Salva e verifica» ti diamo il record esatto da aggiungere.',
  'ob.dnsRec.check': 'Verifica ora',
  'ob.dnsRec.save': 'Salva e verifica',

  'ob.cf.state.ok': 'C’era già',
  'ob.cf.state.created': 'Creato',
  'ob.cf.state.replaced': 'Sostituito',
  'ob.cf.state.missing': 'Lo aggiungiamo',
  'ob.cf.state.conflict': 'C’è già qualcosa',
  'ob.cf.state.failed': 'Non è stato possibile',
  'ob.cf.state.pending': 'Non ancora',
  'ob.cf.permTitle': 'Dacci il permesso solo per il tuo DNS',
  'ob.cf.step1': 'Apri i tuoi token API di Cloudflare e premi «Create token».',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2': 'Scegli «Create custom token» e spunta un solo permesso: Zone · DNS · Edit.',
  'ob.cf.step3': 'In «Zone resources» scegli {domain}, crea il token e incollalo qui.',
  'ob.cf.tokenLabel': 'Token di Cloudflare',
  'ob.cf.tokenPlaceholder': 'Il token che hai appena creato',
  'ob.cf.tokenHint':
    'Vale solo per il DNS di questo dominio. Lo usiamo e lo scartiamo: non viene conservato, a meno che non ce lo chiedi qui sotto, e non esce mai dai nostri server.',
  'ob.cf.mailLabel': 'Ricevi in Sytue Mail anche la posta di @{domain}',
  'ob.cf.mailHintOn':
    'Questo cambia dove arriva la posta del tuo dominio. Se oggi la ricevi altrove, ti avvisiamo prima di sostituire qualsiasi cosa.',
  'ob.cf.mailHintOff':
    'Attivalo solo se vuoi che le tue caselle di posta @tuo-dominio si spostino su Sytue.',
  'ob.cf.rememberLabel': 'Ricorda questo permesso per le modifiche future',
  'ob.cf.rememberHint':
    'Se lo lasci senza spunta, la prossima volta dovrai incollarlo di nuovo. Spuntarlo lo conserva cifrato; togliere la spunta lo cancella.',
  'ob.cf.preview': 'Vedi cosa stiamo per cambiare',
  'ob.cf.previewAgain': 'Guarda di nuovo',
  'ob.cf.nothing': 'Non resta nulla da fare',
  'ob.cf.apply': { one: 'Applica 1 modifica', other: 'Applica {count} modifiche' },
  'ob.cf.zone': 'Zona trovata',
  'ob.cf.token': 'Il tuo token',
  'ob.cf.tokenStored': 'Conservato cifrato per le modifiche future',
  'ob.cf.tokenDiscarded': 'Usato e scartato (non lo abbiamo conservato)',
  'ob.cf.domainState': 'Stato del dominio',
  'ob.cf.verified': 'Verificato',
  'ob.cf.pending': 'In attesa di verifica',
  'ob.cf.nowThere': 'In questo momento c’è: {current}',
  'ob.cf.replaceLabel': 'Sostituiscilo con quello di Sytue',
  'ob.cf.failTitle': 'Qualcosa è rimasto senza configurare',
  'ob.cf.failBody':
    'Il resto è a posto. Qui sopra trovi il motivo di ogni record fallito e il suo valore, per aggiungerlo a mano da Cloudflare.',
  'ob.cf.conflictTitle': 'Ci sono record tuoi che non tocchiamo',
  'ob.cf.conflictBody':
    'Non sostituiamo mai quello che hai già, a meno che tu non lo spunti. Se preferisci combinarli a mano, lì sopra trovi il valore attuale e il nostro.',
  'ob.cf.okTitle': 'Dominio verificato',
  'ob.cf.okBody':
    'Abbiamo visto il record nella tua zona: {host} è tuo e risulta registrato come tale.',
  'ob.cf.todoTitle': 'Le modifiche sono ancora da applicare',
  'ob.cf.todoBody':
    'La tua zona non l’abbiamo ancora toccata. Premi «Applica» quando quello che vedi sopra ti convince.',
  'ob.cf.someFailed': {
    one: '1 record è rimasto senza configurare',
    other: '{count} record sono rimasti senza configurare',
  },
  'ob.cf.someFailedBody': 'Qui sotto trovi il motivo di ciascuno e i valori per farlo a mano.',
  'ob.cf.configured': '{host} configurato e verificato',
  'ob.cf.created': 'Record creati nella tua zona',
  'today.title': 'Oggi',
  'today.openCalendar': 'Apri il calendario',
  'today.noEvents': 'Niente in agenda per oggi.',
  'today.unavailable': 'L’agenda non è disponibile in questo momento.',
  'today.allDay': 'Tutto il giorno',
  'today.untitled': '(senza titolo)',
  'today.unread': { one: '1 email non letta', other: '{count} email non lette' },
  'today.inboxZero': 'Posta in arrivo aggiornata',
  'launcher.quickActions': 'Accessi rapidi',
  'launcher.recentAll': 'Recenti in tutta la suite',
  'launcher.activityError': 'Non è stato possibile caricare la tua attività',
  'launcher.noActivity': 'Nessuna attività recente',
  'ob.dns.verified': 'Dominio verificato',
  'ob.dns.mismatch': 'Esiste un record, ma non è questo',
  'ob.dns.unreachable': 'Non è stato possibile interrogare il tuo DNS',
  'ob.dns.notYet': 'Non vediamo ancora il record',
  'ob.dns.unchecked': 'Non ancora verificato',

  // --- Onboarding: rótulos que estaban en duro ---
  'ob.dns.type': 'Tipo',
  'ob.dns.name': 'Nome',
  'ob.dns.value': 'Valore',
  'ob.cf.tokenField': 'Token Cloudflare',
  'ob.cf.remember': 'Ricorda questa autorizzazione per le modifiche future',
  'ob.cf.replaceWithSytue': 'Sostituiscilo con quello di Sytue',
  'recent.hubDown': 'L’Hub non risponde in questo momento. Il tuo lavoro è al sicuro; riprova.',
  'search.partial':
    '{sources} non ha risposto, quindi potrebbero mancare dei risultati. Riprova tra un momento.',
  'search.partialJoin': ' e ',
  'search.showing': 'Vengono mostrati {count} di {total} risultati.',
  'privacy.notice':
    'Non vendiamo i tuoi dati né li cediamo a terzi. Non li usiamo per profilarti, per pubblicità, né per addestrare o migliorare i nostri servizi o quelli di altri. Sono cifrati in transito e a riposo, e lasciano la nostra infrastruttura europea solo per lo stretto necessario alla fatturazione e nelle integrazioni che attivi tu, in cui il terzo ti chiederà l’approvazione sui dati che userà.',
};
