/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ITALIANO del sitio público. Traducción del inglés (`en.ts`), que es la
 * fuente de verdad: el tipo se deriva de él y una clave de más o de menos rompe
 * la compilación (docs/CONVENTIONS.md §13.1).
 *
 * Vocabulario GDPR oficial italiano, con el falso amigo de rigor: «titolare del
 * trattamento» = responsable (controller) y «responsabile del trattamento» =
 * encargado (processor). No intercambiarlos.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const it: AppCatalog<typeof en> = {
  // --- Cromo del sitio ---------------------------------------------------
  'skip.content': 'Vai al contenuto',
  'nav.products': 'Prodotti',
  'nav.sovereignty': 'Sovranità',
  'nav.standards': 'Standard aperti',
  'nav.pricing': 'Prezzi',
  'footer.pricing': 'Prezzi',
  'home.pricing.title': 'Tre piani, tutta la suite',
  'home.pricing.body':
    'Gli stessi prodotti in tutti e tre: cambia solo lo spazio per utente. Nessuna edizione ridotta.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'IVA esclusa',
  'home.pricing.per': '/Utente/Mese',
  'home.pricing.cta': 'Parla con le vendite',
  'home.pricing.commitment.title': 'Un impegno per sempre',
  'home.pricing.commitment.body':
    'Il prezzo per utente sale di 0,50 € ogni quattro anni, e nient’altro: per sempre. Lo sai il giorno in cui entri e non cambia.',
  'home.pricing.included.title': 'Un’unica edizione',
  'home.pricing.included.body':
    'Nessun piano per funzione, nessun livello, nessuna edizione ridotta: l’intera suite è compresa nel prezzo per utente, con le stesse funzioni per tutti.',
  'home.pricing.metered.title': 'Cosa si paga a parte',
  'home.pricing.metered.body': 'Lo spazio aggiuntivo: a consumo o a blocchi.',
  'home.pricing.storage.title': 'Lo spazio è condiviso',
  'home.pricing.storage.body':
    'La quota di ogni persona si somma a quella dell’organizzazione: ciò che una non usa, lo usano le altre. E la quota per utente si può regolare dall’organizzazione.',
  'nav.downloads': 'Download',
  'nav.aria': 'Navigazione del sito',
  'nav.ariaMobile': 'Navigazione mobile',
  'nav.home': 'Sytue One — Home',
  'nav.openMenu': 'Apri il menu',
  'nav.closeMenu': 'Chiudi il menu',
  'theme.toLight': 'Passa alla modalità chiara',
  'theme.toDark': 'Passa alla modalità scura',
  'lang.label': 'Lingua',
  'cta.start': 'Inizia gratis',
  'cta.signIn': 'Accedi',
  'cta.seeSuite': 'Scopri la suite',

  // --- Portada -----------------------------------------------------------
  'meta.home.title': 'Sytue One — La suite di produttività sovrana europea',
  'meta.home.description':
    'Sytue One: la suite di produttività sovrana europea. Identità, posta, file, documenti, calendario e chat, al 100 % nell’UE e su standard aperti.',
  'home.hero.lead': 'La tua produttività,',
  'home.hero.accent': 'sovrana ed europea',
  'home.hero.subtitle':
    'Posta, file, documenti, calendario, chat e videochiamate: integrati come una sola applicazione e ospitati interamente nell’UE.',
  'home.hero.note': 'Senza carta di credito. Dati su Scaleway (UE). Migrazione assistita.',
  'home.privacy.title': 'Il tuo lavoro è tuo. Anche la tua privacy.',
  'home.privacy.body': 'Non vendiamo i tuoi dati né li usiamo per creare profili pubblicitari.',
  'home.privacy.anonymity':
    'Sytue Analytics protegge la tua privacy con statistiche aggregate, senza cookie di tracciamento e con identificatori pseudonimi dei visitatori che cambiano ogni giorno e sono separati per ciascun sito.',
  'home.privacy.account':
    'Per usare la suite servono dati dell’account e registri di accesso che ci consentano di fornire il servizio e mantenerlo sicuro.',
  'home.privacy.link': 'Come proteggiamo la tua privacy',

  'home.stats.aria': 'Presenza europea',
  'home.stats.regions': 'Regioni UE (fr-par · nl-ams · on-premise Madrid)',
  'home.stats.apps': 'App con una sola identità',
  'home.stats.standards': 'Standard aperti: OIDC, JMAP, CalDAV, CardDAV e WebDAV',

  'home.products.title': 'Una suite, un’esperienza',
  'home.products.subtitle':
    'Ogni prodotto condivide design, identità e interfaccia. Si usano come una sola applicazione.',

  'home.integration.title': 'L’integrazione che una suite deve avere',
  'home.integration.subtitle':
    'Una sola registrazione apre tutto. Provisioning automatico (SCIM) da Sytue ID.',
  'home.integration.sso.title': 'Accesso unico',
  'home.integration.sso.body': 'Un’identità (Sytue ID) per tutta la suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Interfaccia comune',
  'home.integration.shell.body':
    'Topbar, selettore di app, ricerca e notifiche globali in ogni prodotto.',
  'home.integration.cross.title': 'Azioni trasversali',
  'home.integration.cross.body':
    'Allega da Drive dentro Mail, trasforma un’email in un evento di Calendar e condividilo in Rooms senza uscire dal flusso.',

  'home.sovereignty.title': 'Sovranità, davvero',
  'home.sovereignty.body':
    'Non è una promessa di marketing: è l’architettura. L’infrastruttura vive solo in regioni europee e tutto è costruito su protocolli aperti e verificabili, senza backdoor.',
  'home.sovereignty.link': 'Perché standard aperti',
  'home.sovereignty.eu.title': '100 % nell’UE',
  'home.sovereignty.eu.body':
    'Tutta l’infrastruttura nell’UE: Scaleway (fr-par / nl-ams) e una zona on-premise a Madrid. Nessun fornitore statunitense per i dati personali.',
  'home.sovereignty.gdpr.title': 'Conformità GDPR',
  'home.sovereignty.gdpr.body':
    'DPA, registro dei trattamenti, runbook per i diritti degli interessati e conservazione. Cifratura in transito (TLS 1.3) e a riposo.',
  'home.sovereignty.lockin.title': 'Nessun lock-in',
  'home.sovereignty.lockin.body':
    'Costruita su standard aperti: OIDC, JMAP, CalDAV, CardDAV e WebDAV. I tuoi dati restano portabili.',

  'home.standards.overline': 'Opera originale di Sytue',
  'home.standards.title': 'Costruita su standard aperti',
  'home.standards.body':
    'Ogni prodotto Sytue è opera originale, scritta in Go e in Astro + Preact su protocolli aperti (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Nessun motore di terze parti: i tuoi dati sono portabili e la tua suite è sovrana.',

  'home.cta.title': 'Riprendi il controllo del tuo lavoro',
  'home.cta.body':
    'Parti oggi con la tua organizzazione. Migrazione assistita da Google Workspace o Microsoft 365.',
  'home.cta.primary': 'Crea la mia organizzazione',
  'home.cta.secondary': 'Parla con il commerciale',

  // --- Rejilla de productos ----------------------------------------------
  'grid.discover': 'Scopri',
  'product.mail.blurb': 'Posta privata ed europea su JMAP, senza tracker né pubblicità.',
  'product.drive.blurb': 'File e collaborazione, con il marchio della tua organizzazione.',
  'product.docs.blurb':
    'Documenti, fogli di calcolo e presentazioni con co-editing in tempo reale.',
  'product.calendar.blurb': 'Un calendario CalDAV integrato con il resto della suite.',
  'product.contacts.blurb': 'Contatti CardDAV, condivisi nella tua organizzazione.',
  'product.notes.blurb': 'Taccuini che tengono di tutto: testo, file, eventi, documenti interi.',
  'product.photos.blurb':
    'Galleria ed editor di foto: i tuoi originali in Drive, le tue modifiche come ricetta.',
  'product.rooms.blurb': 'Canali, messaggi e videochiamate di squadra in un unico posto.',
  'product.cast.blurb': 'Video on demand e diretta: carica, trasmetti e condividi, nell’UE.',
  'product.pages.blurb':
    'Siti web e negozi online per la tua azienda, progettati su misura e pubblicati nell’UE.',
  'product.bases.blurb': 'Tabelle relazionali collaborative per clienti, ordini o scorte, nell’UE.',
  'product.cloud.blurb': 'Piattaforma di deploy e dati gestiti, nell’UE.',
  'product.id.blurb': 'Un’unica identità (SSO + MFA) con Sytue ID.',
  'product.one.blurb':
    'La tua pagina iniziale della suite: applicazioni, accessi rapidi, attività recente e avvisi.',
  'product.sheets.blurb':
    'Fogli di calcolo con formule, dati esterni e grafici, modificati a più mani.',
  'product.slides.blurb': 'Presentazioni con il tuo marchio, temi e vista relatore.',
  'product.pdf.blurb': 'Leggi, annota, firma e ordina i tuoi PDF senza che lascino l’Europa.',
  'product.console.blurb':
    'Amministrazione dell’organizzazione: persone, accessi, conformità e audit.',
  'product.erp.blurb':
    'Ventiquattro moduli e un libro in partita doppia che il database non lascia sbilanciare.',

  // --- Fichas de producto -------------------------------------------------
  'productPage.back': 'Tutta la suite',
  'productPage.features': 'Tutto quello che fa {name}',
  'productPage.featuresPlanned': 'Che cosa farà {name}',
  'productPage.builtOn': 'Costruito su {standard}',
  'productPage.fromScratch':
    "L'editor di Docus, Calcus, Diapos e CertiPDF è scritto da zero: un motore proprio di Sytue, non un'integrazione di terze parti.",
  'productPage.shot': 'La schermata principale di {name}, esattamente come appare all’accesso.',
  'productPage.shotFeatures': '{name} al lavoro, con le sue funzioni in vista.',
  'productPage.open': 'Apri {name}',
  'productPage.enDesarrollo': 'Questo prodotto è in sviluppo: non è ancora disponibile.',
  'productPage.included':
    'Incluso in Sytue One: una sola identità (Sytue ID) e una sola registrazione danno accesso a tutti i prodotti. Dati al 100 % nell’UE (Scaleway) e portabili grazie agli standard aperti.',

  'product.mail.tagline': 'Posta privata ed europea, senza tracker né pubblicità.',
  'product.mail.standard': 'JMAP (standard aperto)',
  'product.mail.p1': 'Casella veloce con conversazioni, etichette, filtri e ricerca.',
  'product.mail.p2': 'Allega file direttamente da Sytue Drive.',
  'product.mail.p3': 'Trasforma un’email in un evento di Calendar.',
  'product.mail.p4':
    'Cifratura in transito (TLS 1.3). L’infrastruttura di Sytue si trova nell’UE. La posizione dei servizi esterni dipende dal loro fornitore.',

  'product.drive.tagline':
    'I tuoi file e la tua collaborazione, con il marchio della tua organizzazione.',
  'product.drive.standard': 'WebDAV (standard aperto)',
  'product.drive.p1': 'Carica, organizza e condividi con link controllati (permessi e scadenza).',
  'product.drive.p2': 'Anteprima in linea di documenti e immagini.',
  'product.drive.p3': 'Allega a Mail senza duplicare il file.',
  'product.drive.p4': 'Storage basato su S3 in regioni europee.',

  'product.docs.tagline':
    'Documenti, fogli di calcolo e presentazioni con co-editing in tempo reale.',
  'product.docs.standard': 'Editing collaborativo in tempo reale',
  'product.docs.p1': 'Scrivete in più persone, con presenza e modifiche istantanee.',
  'product.docs.p2': 'Fogli di calcolo con formule e formattazione condizionale.',
  'product.docs.p3': 'Presentazioni con l’identità del tuo marchio.',
  'product.docs.p4': 'Tutto salvato nel tuo Drive europeo.',

  'product.calendar.tagline': 'Il tuo calendario, integrato con il resto della suite.',
  'product.calendar.standard': 'CalDAV (standard aperto)',
  'product.calendar.p1': 'Viste giorno, settimana e mese.',
  'product.calendar.p2': 'Eventi ricorrenti e promemoria.',
  'product.calendar.p3': 'Crea eventi direttamente da un’email in Mail.',
  'product.calendar.p4': 'Compatibile con qualsiasi client CalDAV.',

  'product.contacts.tagline': 'I contatti della tua organizzazione, condivisi e portabili.',
  'product.contacts.standard': 'CardDAV (standard aperto)',
  'product.contacts.p1': 'Rubrica condivisa in tutta l’organizzazione.',
  'product.contacts.p2': 'Importa ed esporta in vCard.',
  'product.contacts.p3': 'Unisci i duplicati senza perdere dati.',
  'product.contacts.p4': 'Compatibile con qualsiasi client CardDAV.',
  'product.notes.tagline': 'Tutto ciò che vuoi conservare, in un posto e nelle tue mani.',
  'product.notes.standard': 'Modello proprio su standard aperti (API REST propria)',
  'product.notes.p1': 'Taccuini annidati, su tela libera, a colonne o a elenco.',
  'product.notes.p2':
    'Note di ogni tipo: testo, attività, link, file, eventi del calendario, documenti di Docs.',
  'product.notes.p3': 'Cifratura end-to-end: la passphrase non esce dal tuo browser.',
  'product.notes.p4': 'Decidi tu che cosa entra nella ricerca, e che cosa non entra mai.',

  'product.rooms.tagline': 'Canali, messaggi e videochiamate di squadra, uniti.',
  'product.rooms.standard': 'Messaggistica e chiamate in tempo reale',
  'product.rooms.p1': 'Canali per squadra o progetto.',
  'product.rooms.p2': 'Videochiamata integrata in ogni canale.',
  'product.rooms.p3': 'Menzioni e notifiche unificate in tutta la suite.',
  'product.rooms.p4': 'Cronologia consultabile da tutta Sytue One.',

  'product.cloud.p4': 'Domini personalizzati con TLS automatico.',

  'product.cloud.p3': 'Storage S3 e funzioni gestite.',
  'product.cloud.p2': 'Database Postgres con API REST automatica.',
  'product.cloud.p1': 'Un’anteprima per ogni PR e rollback istantaneo.',
  'product.cloud.standard': 'PaaS Kubernetes-native',
  'product.cloud.tagline': 'Il tuo codice in deploy e i tuoi dati gestiti, nell’Unione europea.',
  'product.id.tagline': 'Un’identità per tutta la suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (standard aperto)',
  'product.id.p1': 'Accesso unico su tutti i prodotti.',
  'product.id.p2': 'MFA e passkey (WebAuthn).',
  'product.id.p3': 'Provisioning automatico (SCIM) dalla tua directory.',
  'product.id.p4': 'Profilo e sicurezza in un solo posto.',
  'product.one.tagline': 'Tutta Sytue One a un clic, dalla stessa porta.',
  'product.one.standard': 'Un solo accesso (Sytue ID) per tutta la suite',
  'product.one.p1':
    'Un launcher con tutte le tue applicazioni e quelle che la tua organizzazione collega.',
  'product.one.p2': 'Ricerca in tutta la suite (⌘K) da qualsiasi schermata.',
  'product.one.p3':
    'L’agenda di oggi e la posta non letta in prima pagina, accanto alle novità di tutta la suite e alle notifiche.',
  'product.one.p4': 'Collega i tuoi dispositivi e scarica le app native.',
  'product.sheets.tagline': 'Numeri che si capiscono, in un foglio che lavora con te.',
  'product.sheets.standard': 'Formati ODF/OOXML e modifica collaborativa in tempo reale',
  'product.sheets.p1': 'Formule, formattazione condizionale e convalida dei dati.',
  'product.sheets.p2': 'Porta dati dall’esterno e interrogali senza uscire dal foglio.',
  'product.sheets.p3': 'Grafici, sparkline, filtri dati e tabelle di dati.',
  'product.sheets.p4': 'Esporta in ODS e XLSX; apre i file Excel.',
  'product.slides.tagline': 'Racconta la storia; la diapositiva tiene il passo.',
  'product.slides.standard': 'Formati ODF/OOXML e modifica collaborativa in tempo reale',
  'product.slides.p1': 'Temi di presentazione con l’identità della tua organizzazione.',
  'product.slides.p2': 'Vista relatore con note e diapositiva successiva.',
  'product.slides.p3': 'Immagini, forme, grafici e SmartArt.',
  'product.slides.p4': 'Esporta in ODP, PPTX e PDF; apre i file PowerPoint.',
  'product.pdf.tagline': 'Il PDF, dall’inizio alla fine, nel tuo browser.',
  'product.pdf.standard': 'PDF (ISO 32000) con annotazioni e moduli standard',
  'product.pdf.p1': 'Evidenziare, sottolineare, commentare e disegnare sul documento.',
  'product.pdf.p2': 'Moduli: compila i campi o crea i tuoi.',
  'product.pdf.p3': 'Unire, estrarre, ruotare, numerare e applicare filigrane.',
  'product.pdf.p4':
    'Pannello delle firme, oscuramento definitivo e documenti protetti da password.',
  'product.console.tagline': 'Governa la suite senza uscire da una schermata.',
  'product.console.standard': 'SCIM, SAML e OpenID Connect (standard aperti)',
  'product.console.p1': 'Persone, gruppi (anche dinamici) e ruoli.',
  'product.console.p2': 'SSO con il tuo provider e provisioning SCIM.',
  'product.console.p3':
    'Regole di accesso condizionale, dispositivi e accesso privilegiato con approvazione.',
  'product.console.p4': 'Audit esportabile nel tuo SIEM e conservazioni legali.',
  'product.erp.tagline': 'Tutto l’ERP, e il catalogo dei modelli è un file di dati.',
  'product.erp.standard': 'Partita doppia, catalogo dei modelli in JSON e una sola API REST',
  'product.erp.p1':
    'Ventiquattro moduli e trentasei modelli: contatti, vendite, acquisti, magazzino, produzione, fatturazione, contabilità, progetti e risorse umane.',
  'product.erp.p2':
    'Aggiungere un modulo significa scrivere un JSON: senza migrazione, senza nuova rotta e senza toccare l’interfaccia.',
  'product.erp.p3':
    'Gli importi sono in centesimi interi, mai in virgola mobile, e il pareggio lo impone il database.',
  'product.erp.p4':
    'Ciò che è registrato non si tocca: si corregge con un’altra scrittura, ed è questo che distingue un libro da un foglio di calcolo.',
  'product.one.f1':
    'Un launcher con le applicazioni della suite e quelle di terzi che la tua organizzazione collega.',
  'product.one.f2':
    'Accessi rapidi: scrivere un’e-mail, nuovo documento, caricare su Drive, creare un evento.',
  'product.one.f3':
    'Recenti di tutta la suite: le ultime cose toccate in Drive, Docus, Mail o Rooms.',
  'product.one.f4': 'Ricerca globale con ⌘K o Ctrl K, con risultati da tutte le applicazioni.',
  'product.one.f5': 'Centro notifiche e presenza condivisi da tutta la suite.',
  'product.one.f6':
    'Collega i tuoi dispositivi: profilo per iPhone e Mac, Android e desktop, in due passi.',
  'product.one.f7': 'Download delle app native per desktop e mobile.',
  'product.one.f8': 'Un assistente di benvenuto che prepara l’organizzazione senza manuali.',
  'product.mail.f1': 'Conversazioni in thread con lettura, risposta e inoltro in un clic.',
  'product.mail.f2':
    'Cartelle, etichette colorate e ricerca con operatori (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Regole automatiche che archiviano, etichettano o spostano la posta in arrivo.',
  'product.mail.f4': 'Invio programmato e «Annulla invio».',
  'product.mail.f5': 'Firme, alias dell’indirizzo e risposta automatica di assenza.',
  'product.mail.f6': 'Caselle condivise e delegate con permessi di lettura e invio.',
  'product.mail.f7': 'Editor con formattazione e allegati da Sytue Drive senza duplicare i file.',
  'product.mail.f8':
    'Contenuti remoti bloccati per impostazione predefinita: nessuno sa quando apri un’e-mail.',
  'product.mail.f9': 'Posta firmata (PGP e S/MIME) riconosciuta e segnalata; TLS in transito.',
  'product.mail.f10':
    'Accesso anche da Thunderbird, Outlook, Apple Mail o dal telefono, con password per app.',
  'product.mail.f11':
    'Indirizzi a pastiglie con la rubrica di Contacts a un clic, e avviso se dici «allegato» senza allegare.',
  'product.mail.f12':
    'Regole dall’email aperta, svuota il cestino in un colpo e scorciatoie da tastiera con «?».',
  'product.drive.f1':
    'La mia unità, unità di team, condivisi con me, recenti, preferiti e cestino.',
  'product.drive.f2': 'Carica trascinando; cartelle, sposta, copia, rinomina e selezione multipla.',
  'product.drive.f3':
    'Link di condivisione a due livelli (guardare o modificare), password e scadenza.',
  'product.drive.f4': 'Cronologia delle versioni con ripristino.',
  'product.drive.f5': 'Anteprima di documenti, immagini e PDF senza scaricare.',
  'product.drive.f6':
    'Apri e modifica i documenti da ufficio in Docus, Calcus e Diapos; i PDF in CertiPDF.',
  'product.drive.f7':
    'Etichette di riservatezza che impediscono i link pubblici dove non devono esserci.',
  'product.drive.f8': 'Condividi in un canale di Rooms o allega a Mail senza duplicare il file.',
  'product.drive.f9': 'Accesso WebDAV e client desktop per vedere i file sul computer.',
  'product.drive.f10': 'Archiviazione S3 in regioni europee, con la quota sempre visibile.',
  'product.docs.f1':
    'Modifica collaborativa in tempo reale, con presenza e senza pestarsi i piedi.',
  'product.docs.f2': 'Una barra a schede: Home, Inserisci, Riferimenti, Lettere e Revisione.',
  'product.docs.f3': 'Tabelle, immagini, forme, SmartArt, grafici, simboli e filigrane.',
  'product.docs.f4': 'Riferimenti: indice, note a piè di pagina, citazioni e bibliografia.',
  'product.docs.f5': 'Lettere: stampa unione per lettere ed etichette.',
  'product.docs.f6': 'Commenti in thread, revisione delle modifiche e versioni.',
  'product.docs.f7':
    'Assistente IA opzionale per riscrivere, tradurre o riassumere, solo se la tua organizzazione lo attiva.',
  'product.docs.f8': 'Esporta in ODT, DOCX e PDF; apre i file Word da Drive.',
  'product.docs.f9': 'Intestazioni, piè di pagina, numerazione e blocchi riutilizzabili.',
  'product.docs.f10': 'Salvataggio automatico nel tuo Drive europeo, con cronologia.',
  'product.docs.f11':
    'Scarica dalla pagina iniziale in PDF, HTML, Markdown, OpenDocument o Microsoft Office.',
  'product.sheets.f1': 'Formule con aiuto contestuale e ricalcolo istantaneo.',
  'product.sheets.f2':
    'Formattazione condizionale, convalida dei dati e formati numerici regionali.',
  'product.sheets.f3': 'Grafici di vari tipi, sparkline e filtri dati.',
  'product.sheets.f4': 'Tabelle di dati con totali e ordinamento; tabella pivot.',
  'product.sheets.f5': 'Dati esterni: collega una fonte e interrogala dal foglio.',
  'product.sheets.f6': 'Risolutore per trovare il valore che raggiunge un obiettivo.',
  'product.sheets.f7': 'Macro per automatizzare le attività ripetitive.',
  'product.sheets.f8': 'Collaborazione in tempo reale con presenza e commenti.',
  'product.sheets.f9': 'Filtri, filtri dati e ricerca nel foglio.',
  'product.sheets.f10': 'Esporta in ODS, XLSX e PDF; apre i file Excel da Drive.',
  'product.slides.f1': 'Temi e tavolozze coerenti con il marchio della tua organizzazione.',
  'product.slides.f2': 'Una tela con forme, immagini, grafici, SmartArt e connettori.',
  'product.slides.f3': 'Presenta a schermo intero con vista relatore e note.',
  'product.slides.f4': 'Collaborazione in tempo reale con presenza e commenti.',
  'product.slides.f5': 'Diapositive con sfondo, poster e layout.',
  'product.slides.f6': 'Esporta in ODP, PPTX e PDF.',
  'product.slides.f7': 'Apre i file PowerPoint da Drive.',
  'product.slides.f8': 'Salvataggio automatico con versioni.',
  'product.pdf.f1':
    'Visualizzatore veloce con indice, miniature, ricerca e zoom a pagina o a larghezza.',
  'product.pdf.f2':
    'Annotazioni: evidenziare, sottolineare, barrare, commentare, disegnare, forme, frecce e timbri.',
  'product.pdf.f3': 'Caselle di testo, misure e ritagli di zona.',
  'product.pdf.f4': 'Moduli: compila i campi esistenti o aggiungi i tuoi.',
  'product.pdf.f5': 'Pannello delle firme del documento.',
  'product.pdf.f6':
    'Oscuramento definitivo: ciò che segni sparisce dal file, non solo dalla vista.',
  'product.pdf.f7':
    'Pagine: unire, estrarre, eliminare, ruotare, numerare (anche Bates), intestazioni e piè di pagina.',
  'product.pdf.f8': 'Filigrana, sfondo e metadati modificabili.',
  'product.pdf.f9': 'Documenti protetti da password e permessi.',
  'product.pdf.f10': 'Lettura ad alta voce e commenti in thread.',
  'product.calendar.f1': 'Viste mese, settimana, giorno e agenda, con mini calendario e «Oggi».',
  'product.calendar.f2':
    'Ripetizione giornaliera, settimanale o mensile con fine; modifica solo questo, questo e i seguenti, o tutti.',
  'product.calendar.f3': 'Promemoria per evento e notifiche nella suite.',
  'product.calendar.f4': 'Inviti con risposta (partecipo, forse, no) e follow-up.',
  'product.calendar.f5': '«Trova uno spazio» libero per tutte le persone invitate.',
  'product.calendar.f6': 'Sale e risorse prenotabili con disponibilità.',
  'product.calendar.f7': 'Calendari condivisi con permessi, e quelli che altri condividono con te.',
  'product.calendar.f8': 'Iscrizione a calendari esterni (ICS) e più calendari con colore.',
  'product.calendar.f9': 'Crea un evento da un’e-mail di Mail o da una nota.',
  'product.calendar.f10':
    'Compatibile con qualsiasi client CalDAV: Apple Calendario, Thunderbird e altri.',
  'product.calendar.f11':
    'Duplica un evento o copialo nel tuo calendario, e scaricalo in .ics per chi non è su Sytue.',
  'product.contacts.f1': 'Rubriche, compresa quella condivisa della tua organizzazione.',
  'product.contacts.f2':
    'Scheda completa: telefoni, e-mail, indirizzi, organizzazione, compleanno, note e foto.',
  'product.contacts.f3': 'Etichette e preferiti per raggruppare e trovare.',
  'product.contacts.f4': 'Rilevamento e unione dei duplicati senza perdere dati.',
  'product.contacts.f5': 'Importa ed esporta vCard e CSV; esporta un singolo contatto.',
  'product.contacts.f6': 'Azioni in blocco e cestino con recupero.',
  'product.contacts.f7': 'Ricerca istantanea e completamento automatico scrivendo in Mail.',
  'product.contacts.f8': 'Compatibile con qualsiasi client CardDAV.',
  'product.contacts.f9':
    'Prossimi compleanni della tua rubrica, con quelli di questo mese in evidenza.',
  'product.contacts.f10':
    'Crea una riunione con un contatto o copia la sua email senza uscire dalla scheda.',
  'product.notes.f1':
    'Quaderni annidati con icona e colore; tre disposizioni: libera, colonne ed elenco.',
  'product.notes.f2':
    'Dodici tipi di nota: testo, attività, link, colore, immagine e riferimenti a Drive, Docus, Calendar, Rooms e altre note.',
  'product.notes.f3':
    'Un editor con formattazione (grassetto, elenchi, link) che salva all’uscita.',
  'product.notes.f4':
    'Etichette, fissare, duplicare, spostare tra taccuini, raggruppare e trascinare sulla tela.',
  'product.notes.f5':
    'Visibilità per quaderno: privato, personale, organizzazione o link pubblico.',
  'product.notes.f6': 'Cifratura nel tuo browser per i quaderni sensibili: Sytue non può leggerli.',
  'product.notes.f7': 'Decidi tu quali quaderni entrano nella ricerca della suite.',
  'product.notes.f8': 'Backlink: vedi quali note puntano a questa.',
  'product.notes.f9': 'Esporta un intero quaderno in HTML, PDF, Markdown e altro; importa copie.',
  'product.notes.f10':
    'Condividi una nota in un canale di Rooms; trascina o incolla per creare note.',
  'product.notes.f11':
    'Tabelle nell’editor e scorciatoie Markdown mentre scrivi: «- », «# », «> » e «```».',
  'product.notes.f12':
    'Sposta una nota in un altro taccuino, esporta una singola nota e ordina il taccuino come vuoi.',
  'product.photos.tagline': 'Le tue foto, in ordine e a tuo gusto, senza lasciare l’Europa.',
  'product.photos.standard': 'Originali in Sytue Drive; motore di modifica nel browser',
  'product.photos.p1': 'Una linea del tempo per mesi con ricerca per nome.',
  'product.photos.p2': 'Album con copertina; visualizzatore a schermo intero.',
  'product.photos.p3':
    'Un editor con luce, colore, rotazione, specchio e ritaglio, in tempo reale.',
  'product.photos.p4': 'Ogni modifica è una ricetta: l’originale non si tocca mai.',
  'product.photos.f1':
    'Una linea del tempo raggruppata per mesi, con ricerca per nome e miniature nitide.',
  'product.photos.f2':
    'Album: creare, rinominare, scegliere la copertina, aggiungere dal visualizzatore e rimuovere.',
  'product.photos.f3': 'Visualizzatore a schermo intero con navigazione da tastiera.',
  'product.photos.f4':
    'Un editor con quattro cursori (esposizione, contrasto, saturazione, temperatura), rotazione e specchio.',
  'product.photos.f5':
    'Ritaglio trascinando e scorciatoie da tastiera; anteprima su GPU con ripiego su CPU.',
  'product.photos.f6':
    'Ogni modifica è una ricetta salvata a parte: l’originale in Drive non viene mai modificato.',
  'product.photos.f7':
    'Esportazione a piena risoluzione, calcolata sul server con le stesse formule che hai visto.',
  'product.photos.f8': 'Cestino con ripristino.',
  'product.photos.f9':
    'Gli originali vivono nel tuo Sytue Drive; i derivati sono una cache rigenerabile.',
  'product.photos.f10': 'Dati EXIF letti dal file stesso; tutto nell’Unione europea.',
  'product.photos.f11':
    'Selezione multipla per mese: in un nuovo album, nel cestino o scaricate in una volta.',
  'product.photos.f12':
    'Presentazione, dimensione reale, rotazione dal visore e ritaglio con proporzione fissa.',
  'product.rooms.f1': 'Canali pubblici e privati in categorie; messaggi diretti e gruppi.',
  'product.rooms.f2': 'Thread, reazioni, menzioni, fissati, salvati e bozze.',
  'product.rooms.f3':
    'Formattazione con barra «Aa» e scorciatoie: grassetto, corsivo, codice, citazioni, elenchi e link.',
  'product.rooms.f4': 'Allegati, immagini, GIF ed emoji personalizzate; file da Drive.',
  'product.rooms.f5': 'Invio programmato, priorità, conferma di lettura e «visto da».',
  'product.rooms.f6': 'Comandi con «/» e schede: sondaggi e attività assegnabili.',
  'product.rooms.f7':
    'Videochiamate nel canale stesso: fotocamera, schermo, mano alzata, chat e registrazione.',
  'product.rooms.f8': 'Riunioni programmate e link di invito.',
  'product.rooms.f9': 'Bot, webhook e integrazioni con altri strumenti.',
  'product.rooms.f10': 'Presenza e stato personalizzato; avvisi per canale su misura.',
  'product.rooms.f11': 'Ricerca nella cronologia e da tutta la suite.',
  'product.rooms.f12': 'Tempo reale via WebSocket, con riconnessione senza perdere messaggi.',
  'product.rooms.f13':
    'Selettore rapido di canali con Ctrl+G, segna tutto come letto e scorciatoie con «?».',
  'product.cast.tagline':
    'I tuoi video e le tue dirette, on demand e in tempo reale, senza uscire dall’Europa.',
  'product.cast.standard':
    'Riproduzione HLS nel browser; ingestione RTMP da qualsiasi programma di trasmissione',
  'product.cast.p1':
    'Carica un video e viene transcodificato in più qualità per qualsiasi schermo.',
  'product.cast.p2': 'Vai in diretta da un programma di trasmissione, dal browser o dall’app.',
  'product.cast.p3':
    'Quattro livelli di visibilità: pubblico, non in elenco, organizzazione o persone specifiche.',
  'product.cast.p4':
    'Chat in diretta e commenti, con moderazione di base da parte di chi pubblica.',
  'product.cast.f1': 'Carica video fino a 8 GiB; l’originale viene conservato.',
  'product.cast.f2': 'Riproduzione adattiva in tre qualità: il browser sceglie la migliore.',
  'product.cast.f3':
    'Diretta da un programma di trasmissione via RTMP, con una chiave di streaming per persona.',
  'product.cast.f4':
    'Diretta dal browser o dall’app: fotocamera e microfono. Condivisione dello schermo nei browser, sistemi desktop e Android compatibili; non disponibile nell’app iOS/iPadOS.',
  'product.cast.f5': 'Le registrazioni delle dirette diventano video on demand da sole.',
  'product.cast.f6': 'Una pagina iniziale pubblica e un canale per organizzazione, indicizzabili.',
  'product.cast.f7':
    'Link non in elenco, video solo per l’organizzazione o accesso a persone specifiche per @utente.',
  'product.cast.f8':
    'Chat in diretta e commenti in thread, moderati da chi pubblica e dall’amministrazione dell’organizzazione.',
  'product.cast.f9': 'Lettore incorporabile e oEmbed per i video pubblici e non in elenco.',
  'product.cast.f10': 'Lo spazio conta nella tua quota di Sytue Drive; tutto nell’Unione europea.',
  'product.cast.f11':
    '«Carica su Cast» da Sytue Drive: una copia del video va su Cast e il file resta in Drive.',
  'product.pages.tagline':
    'Il sito web della tua azienda e il suo negozio, progettati in un editor visuale e pubblicati senza uscire dall’Europa.',
  'product.pages.standard':
    'HTML e CSS statici e accessibili, senza cookie di terze parti; esportazione in Astro',
  'product.pages.p1':
    'Un editor visuale con riquadri che si adattano da soli a desktop, tablet e smartphone.',
  'product.pages.p2': 'Dieci modelli per settore, con testi veri e foto di pubblico dominio.',
  'product.pages.p3': 'Un negozio con catalogo, carrello e incassi sul conto della tua azienda.',
  'product.pages.p4':
    'Il tuo dominio con certificato automatico, oppure un indirizzo su sites.sytue.eu.',
  'product.pages.f1':
    'Editor visuale a riquadri con layout automatico, componenti riutilizzabili e sezioni già pronte.',
  'product.pages.f2':
    'Tre breakpoint (desktop, tablet e smartphone), ciascuno con le proprie regolazioni.',
  'product.pages.f3': 'Analisi delle visite senza cookie, nella dashboard del sito.',
  'product.pages.f4':
    'Siti multilingue con indirizzi per lingua, hreflang e traduzione assistita da Sytue AI.',
  'product.pages.f5':
    'Immagini dal tuo computer, da Sytue Drive o da Sytue Photos, con testo alternativo richiesto al momento dell’inserimento.',
  'product.pages.f6':
    'Negozio con varianti, scorte, IVA per prodotto e pagamento con Mollie, Adyen, Redsys o Stripe.',
  'product.pages.f7':
    'Ordini con fattura semplificata emessa da Sytue ERP e recesso dal sito stesso.',
  'product.pages.f8':
    'Moduli con consenso, una casella dei messaggi e notifiche alle persone della tua organizzazione.',
  'product.pages.f9':
    'Versioni con nome, anteprima dietro un link segreto e ripristino di qualsiasi pubblicazione precedente.',
  'product.pages.f10':
    'Esportazione del codice in Astro, React, Next.js e app native, e del catalogo in Shopify, WooCommerce e altre piattaforme.',
  'product.bases.tagline':
    'Tabelle collegate per gestire clienti, ordini o scorte, con permessi e registro di ogni modifica.',
  'product.bases.standard':
    'CSV e JSON aperti; schema e relazioni con integrità garantita dal database stesso',
  'product.bases.p1':
    'Clienti, ordini o scorte in tabelle collegate, senza bisogno di conoscere i database.',
  'product.bases.p2': 'Condividi con la tua organizzazione con il ruolo che decidi.',
  'product.bases.p3':
    'Ogni modifica viene registrata: chi l’ha fatta e quale era il valore precedente.',
  'product.bases.p4': 'I tuoi dati restano nell’UE e li esporti quando vuoi.',
  'product.bases.f1':
    'Tabelle con campi di testo, numero, data, casella, scelta singola e scelta multipla.',
  'product.bases.f2':
    'Relazioni tra tabelle: collega un ordine al suo cliente e il database non permette di spezzare il legame.',
  'product.bases.f3':
    'Griglia modificabile da tastiera, leggibile con lo screen reader e utilizzabile da smartphone.',
  'product.bases.f4':
    'Permessi per base e per tabella: proprietario, editor, commentatore e lettore.',
  'product.bases.f5': 'Importa un CSV come nuova tabella ed esporta in CSV o JSON.',
  'product.bases.f6': 'Registro di ogni modifica: chi, quale record e campo, prima e dopo.',
  'product.cloud.f12': 'Utilizzo, costi e fatturazione europea nella stessa console.',
  'product.cloud.f11': 'Persone, token API e audit esportabile.',
  'product.cloud.f10': 'Analisi web rispettosa della privacy, senza cookie.',
  'product.cloud.f9': 'Attività pianificate e modelli di progetto.',
  'product.cloud.f8': 'Terminale nel container; riavvia o metti in pausa il servizio.',
  'product.cloud.f7': 'Log in diretta, metriche e monitoraggio con avvisi.',
  'product.cloud.f6': 'Variabili d’ambiente e segreti per ambiente.',
  'product.cloud.f5': 'Domini personalizzati con TLS automatico; redirect e intestazioni.',
  'product.cloud.f4': 'Archiviazione S3 e funzioni gestite.',
  'product.cloud.f3': 'Database Postgres con API REST automatica ed editor SQL.',
  'product.cloud.f2': 'Anteprima per ogni PR e ritorno a qualsiasi deploy in un clic.',
  'product.cloud.f1':
    'Progetti dal tuo repository (GitHub, GitLab o Git proprio) con deploy a ogni push.',
  'product.console.f1':
    'Un pannello con lo stato dell’organizzazione e ciò che merita uno sguardo oggi.',
  'product.console.f2':
    'Account: creazione, sospensione, reimpostazione di password e MFA, sessioni, alias e casella.',
  'product.console.f3': 'Gruppi statici e dinamici per regole; ruoli con permessi.',
  'product.console.f4': 'Accesso privilegiato (PIM): attivazione temporanea con approvazione.',
  'product.console.f5': 'SSO con provider SAML o OIDC, con test di connessione; provisioning SCIM.',
  'product.console.f6': 'Regole di accesso condizionale e inventario dei dispositivi fidati.',
  'product.console.f7':
    'Integrazioni: client OIDC, SAML e connessioni con terzi; sostituzione dei prodotti.',
  'product.console.f8': 'Porta i contenuti: migrazione da altri provider.',
  'product.console.f9': 'Editor di documenti e canali di Rooms amministrabili.',
  'product.console.f10':
    'Governance: conservazioni legali, etichette di riservatezza e policy delle password.',
  'product.console.f11': 'Audit con esportazione CSV e invio al tuo SIEM; log dei servizi.',
  'product.console.f12': 'Dominio e marchio propri: logo dell’organizzazione e dominio e-mail.',
  'product.id.f1':
    'Accesso unico (OpenID Connect e OAuth 2.0 con PKCE) per la suite e le tue applicazioni.',
  'product.id.f2': 'Passkey (WebAuthn) e MFA TOTP con codici di recupero.',
  'product.id.f3': 'Password per app per IMAP e client che non parlano OIDC.',
  'product.id.f4': 'Sessioni e dispositivi: revoca una o tutte le altre.',
  'product.id.f5': 'Federazione SAML con il tuo IdP, e Sytue come IdP SAML per altre applicazioni.',
  'product.id.f6': 'Provisioning SCIM dalla tua directory.',
  'product.id.f7': 'Inviti, verifica dell’e-mail e recupero della password.',
  'product.id.f8': 'Profilo per iPhone e Mac (.mobileconfig) in un tocco.',
  'product.id.f9': 'Policy delle password e regole di accesso definite dall’organizzazione.',
  'product.id.f10': 'Registrazione dinamica dei client (DCR) e discovery standard.',
  'product.erp.f1':
    'Ventiquattro moduli e trentasei modelli in un solo servizio e un solo database.',
  'product.erp.f2':
    'Il catalogo dei modelli è dato: ogni modulo dichiara campi, stati e colonne dell’elenco.',
  'product.erp.f3':
    'Un modulo nuovo è un file JSON — senza DDL, senza migrazione e senza bloccare una tabella.',
  'product.erp.f4':
    'Una sola API per i trentasei modelli e una sola interfaccia generata dal catalogo.',
  'product.erp.f5':
    'Contabilità in partita doppia con importi in centesimi interi; nemmeno un numero in virgola mobile.',
  'product.erp.f6':
    'Il pareggio lo verifica il database al commit della transazione, non il codice applicativo.',
  'product.erp.f7':
    'Le scritture chiuse sono immutabili: non si modificano, non si cancellano e non accettano nuove righe.',
  'product.erp.f8':
    'Un documento, una scrittura: un nuovo tentativo o due schede non possono duplicare un ricavo.',
  'product.erp.f9':
    'Le etichette comuni ci sono in tutte le 26 lingue, e un campo non tradotto impedisce l’avvio.',
  'product.erp.f10':
    'Elenchi serviti da indice: la prima pagina di ventimila righe arriva in meno di un millisecondo.',

  // --- Descargas ----------------------------------------------------------
  'meta.downloads.title': 'Download — Sytue One per desktop e mobile',
  'meta.downloads.description':
    'App native di Sytue One per macOS, Windows, Linux, iOS e Android. Tutta la suite sul tuo dispositivo, con le integrazioni di ciascun sistema.',
  'downloads.overline': 'App',
  'downloads.h1': 'Sytue One sul tuo dispositivo',
  'downloads.intro':
    'Vere applicazioni desktop e mobile: si installano, vivono nel Dock o nel menu Start, avvisano con le notifiche di sistema e aprono i file dal tuo dispositivo.',
  'downloads.desktop.title': 'Desktop',
  'downloads.desktop.body':
    'Un’applicazione nativa per ogni prodotto, che disegna da sé la propria interfaccia con la scheda grafica: nessun browser al suo interno, quindi si avvia in fretta e consuma come un’app di sistema.',
  'downloads.apps.title': 'Solo quello che serve',
  'downloads.apps.body':
    'Sytue One porta l’intera giornata di lavoro. Se vuoi solo il disco, Sytue Drive si installa per conto suo.',
  'downloads.includes': 'Include:',
  'downloads.mobile.title': 'Mobile',
  'downloads.mobile.body':
    'Applicazione nativa iOS e Android, con notifiche di sistema, condivisione e accesso a file e fotocamera.',
  'downloads.selfhost.title': 'La tua organizzazione è su un dominio proprio?',
  'downloads.selfhost.body':
    'Le applicazioni native si collegano a sytue.eu. Se la tua organizzazione ha un’installazione propria, accedi dal browser sul suo dominio: la suite funziona allo stesso modo sul web.',
  'downloads.yourSystem': 'Il tuo sistema',
  'downloads.signInToGet': 'Accedi per scaricare',
  'downloads.store': 'Vedi su {format}',
  'downloads.soon': 'Presto su {format}',
  'downloads.browser.text':
    'Preferisci il browser? Tutta la suite funziona allo stesso modo sul web, senza installare nulla:',
  'downloads.browser.link': 'apri Sytue One',
  'platform.macos.req': 'macOS 11 o successivo · Apple Silicon e Intel',
  'platform.windows.req': 'Windows 10 o successivo · 64 bit',
  'platform.linux.req': 'OpenGL 2.1 · 64 bit',
  'platform.ios.req': 'iOS 14 o successivo',
  'platform.android.req': 'Android 6.0 o successivo',

  // --- Pie ----------------------------------------------------------------
  'footer.tagline':
    'Ogni app della suite è un servizio nostro, costruito su protocolli aperti e pubblicati.',
  'footer.products': 'Prodotti',
  'footer.resources': 'Risorse',
  'footer.legal': 'Note legali',
  'footer.suite': 'La suite',
  'footer.downloads': 'Scarica le app',
  'footer.sales': 'Parla con il commerciale',
  'footer.privacy': 'Privacy e GDPR',
  'footer.accessibility': 'Accessibilità',
  'footer.openSource': 'Open source',
  'footer.legalNotice': 'Note legali',
  'footer.terms': 'Termini',
  'footer.copyright':
    '© {year} Sytue · Dati ospitati nell’UE (Scaleway fr-par / nl-ams · on-premise Madrid).',
  'footer.privacyNotice':
    'Non vendiamo i tuoi dati né li cediamo a terzi. Non li usiamo per profilarti, per pubblicità, né per addestrare o migliorare i nostri servizi o quelli di altri. Sono cifrati in transito e a riposo, e lasciano la nostra infrastruttura europea solo per lo stretto necessario alla fatturazione, per avvisarti con l’app chiusa (il servizio di push vede solo il tuo dispositivo e l’ora, mai il contenuto) e nelle integrazioni che attivi tu, in cui il terzo ti chiederà l’approvazione sui dati che userà.',

  // --- 404 ----------------------------------------------------------------
  'meta.notFound.title': 'Pagina non trovata — Sytue One',
  'notFound.overline': 'Errore 404',
  'notFound.title': 'Non abbiamo trovato questa pagina',
  'notFound.body': 'Forse il link è cambiato. Torna alla home o esplora la suite.',
  'notFound.home': 'Torna alla home',

  // --- Legal: privacidad ---------------------------------------------------
  'meta.legal.title': 'Note legali — Sytue One',
  'meta.legal.description':
    'Dati identificativi del titolare di www.sytue.eu e del servizio Sytue One: Sytue Projects, S.L.',
  'aviso.title': 'Note legali',
  'aviso.intro':
    'In conformità all’articolo 10 della legge spagnola 34/2002, dell’11 luglio, sui servizi della società dell’informazione e il commercio elettronico (LSSI), questi sono i dati identificativi del titolare di questo sito e del servizio Sytue One.',
  'aviso.review.body':
    'I dati di registro di questa nota sono verificati nel Registro delle imprese. Il resto del testo è in revisione legale e la sua formulazione può cambiare senza che cambi ciò che descrive.',
  'aviso.owner.title': 'Titolare',
  'aviso.owner.name': 'Denominazione sociale',
  'aviso.owner.form': 'Forma giuridica',
  'aviso.owner.formValue': 'Società a responsabilità limitata unipersonale di diritto spagnolo',
  'aviso.owner.nif': 'Codice fiscale (NIF)',
  'aviso.owner.address': 'Sede legale',
  'aviso.owner.registry': 'Iscrizione al registro',
  'aviso.owner.registryValue':
    'Registro delle imprese di Madrid (Registro Mercantil de Madrid), sezione 8, foglio M-887153, iscrizione 1',
  'aviso.owner.email': 'E-mail',
  'aviso.owner.web': 'Sito web',
  'aviso.scope.title': 'Oggetto',
  'aviso.scope.body':
    'Questo sito presenta la suite Sytue One e dà accesso alle sue applicazioni. L’uso del servizio è regolato dai Termini del servizio e il trattamento dei dati personali dall’Informativa sulla privacy, entrambi linkati a piè di pagina.',
  'aviso.ip.title': 'Proprietà intellettuale e industriale',
  'aviso.ip.body':
    'I contenuti di questo sito (testi, design, codice, marchi e loghi Sytue e Sytue One) appartengono a Sytue Projects, S.L. o sono usati su licenza. I nomi di prodotti di terzi citati come riferimento di compatibilità sono marchi dei rispettivi titolari.',
  'aviso.liability.title': 'Responsabilità',
  'aviso.liability.body':
    'Sytue Projects, S.L. si impegna a mantenere questo sito disponibile e aggiornato, senza garantire l’assenza di interruzioni o errori. Non risponde dei contenuti dei siti di terzi a cui rimanda.',
  'aviso.law.title': 'Legge applicabile',
  'aviso.law.body':
    'Questa nota è regolata dalla legge spagnola. Se sei un consumatore, conservi i diritti e il foro che ti riconosce la normativa del tuo paese di residenza nell’Unione europea.',
  'aviso.updated': 'Ultimo aggiornamento: settembre 2026.',
  'meta.accessibility.title': 'Dichiarazione di accessibilità — Sytue One',
  'meta.accessibility.description':
    'Quanto sono accessibili la suite Sytue One e questo sito, cosa misuriamo, cosa manca ancora e come segnalarcelo.',
  'a11y.title': 'Dichiarazione di accessibilità',
  'a11y.intro':
    'Sytue vuole che ogni persona possa usare Sytue One e questo sito, con qualunque tecnologia assistiva le serva. Questa dichiarazione riguarda www.sytue.eu e le applicazioni web della suite.',
  'a11y.standard.title': 'Standard di riferimento',
  'a11y.standard.body':
    'Progettiamo e sviluppiamo secondo le WCAG 2.1 livello AA, il livello a cui rimandano la norma europea EN 301 549 e l’Atto europeo sull’accessibilità (direttiva (UE) 2019/882).',
  'a11y.status.title': 'Stato di conformità',
  'a11y.status.body':
    'Parzialmente conforme: la maggior parte della suite rispetta lo standard, e le parti elencate più sotto non lo rispettano ancora del tutto. Non abbiamo ancora commissionato un audit indipendente.',
  'a11y.measures.title': 'Cosa facciamo per arrivarci',
  'a11y.measures.design':
    'Un solo sistema di design per tutta la suite: colori del testo con contrasto verificato, anello di focus visibile su ogni controllo e uso da tastiera di menu, finestre di dialogo ed elenchi.',
  'a11y.measures.ci':
    'Verifiche automatiche di accessibilità (Lighthouse) nella catena di integrazione a ogni modifica del sito pubblico.',
  'a11y.measures.system':
    'L’interfaccia segue le impostazioni del tuo sistema: tema scuro o chiaro, animazioni ridotte e dimensione del testo, senza dimensioni fisse in pixel nei campi dei moduli.',
  'a11y.measures.text':
    'Ogni immagine, icona e stato vuoto ha un’alternativa testuale, e ogni pagina dichiara la propria lingua perché gli screen reader la pronuncino correttamente.',
  'a11y.limits.title': 'Limiti noti',
  'a11y.limits.body':
    'La griglia del foglio di calcolo, la tela delle presentazioni e l’annotazione dei PDF sono editor complessi su cui c’è ancora lavoro da fare perché siano pienamente usabili con tastiera e screen reader. I documenti e i file caricati dalle persone vengono mostrati così come sono, senza correzioni da parte nostra.',
  'a11y.feedback.title': 'Dicci cosa non funziona',
  'a11y.feedback.body':
    'Se qualcosa nella suite o in questo sito ti risulta difficile o impossibile da usare, scrivici: lo esaminiamo e ti rispondiamo.',
  'a11y.enforcement.title': 'Se non sei soddisfatto',
  'a11y.enforcement.body':
    'Se hai segnalato una barriera e la nostra risposta non ti soddisfa, puoi rivolgerti all’autorità competente per l’accessibilità del tuo paese ai sensi dell’Atto europeo sull’accessibilità.',
  'a11y.updated':
    'Questa dichiarazione è stata redatta nel settembre 2026 e viene rivista quando la suite cambia in modo sostanziale.',
  'meta.openSource.title': 'Open source — Sytue One',
  'meta.openSource.description':
    'Sytue costruisce quasi tutto da zero, ma non reinventa ciò che già funziona bene. I progetti open source su cui si basa la suite.',
  'oss.title': 'Open source',
  'oss.intro':
    'Sytue costruisce quasi tutto da zero: la posta, gli editor, il calendario. Ma non reinventa ciò che già funziona bene. Questi sono i progetti open source su cui si basano alcune parti della suite.',
  'oss.livekit.body':
    "L'audio e il video in tempo reale delle chiamate di Sytue Rooms funzionano su LiveKit.",
  'oss.postgres.body':
    'Ogni servizio della suite salva i propri dati in PostgreSQL, un database per organizzazione.',
  'meta.privacy.title': 'Privacy e GDPR — Sytue One',
  'meta.privacy.description':
    'Come Sytue One tratta i dati personali: titolare, dati, basi giuridiche, conservazione, responsabili, sicurezza, trasferimenti internazionali e i tuoi diritti GDPR.',
  'legal.overline': 'Note legali',
  'legal.updated': 'Ultimo aggiornamento: luglio 2026.',
  'legal.review.title': 'In revisione legale.',
  'privacy.title': 'Privacy e GDPR',
  'privacy.review.body':
    'Questa informativa descrive con esattezza come trattiamo i dati oggi. I termini di conservazione specifici saranno completati nella versione finale, rivista dal nostro team legale.',
  'privacy.controller.title': 'Titolare del trattamento',
  'privacy.controller.body':
    'Sytue Projects, S.L. (codice fiscale spagnolo B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spagna; iscritta al Registro delle imprese di Madrid, sezione 8, foglio M-887153), in qualità di fornitore della suite Sytue One, è il titolare del trattamento dei dati degli account e delle organizzazioni del servizio. Contatto in materia di protezione dei dati:',
  'privacy.data.title': 'Quali dati trattiamo',
  'privacy.data.account.label': 'Dati dell’account:',
  'privacy.data.account.body':
    'nome, indirizzo email e organizzazione, gestiti con Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Contenuti della tua organizzazione:',
  'privacy.data.content.body':
    'posta, file, documenti, calendario e messaggi che la tua organizzazione ospita nella suite. Quei contenuti appartengono alla tua organizzazione.',
  'privacy.data.technical.label': 'Dati tecnici di esercizio:',
  'privacy.data.technical.body':
    'registri di accesso e dati di connessione necessari alla sicurezza, alla prevenzione degli abusi e al funzionamento del servizio.',
  'privacy.data.note':
    'Non vendiamo i dati né li usiamo per la pubblicità. L’infrastruttura e l’archiviazione gestite da Sytue si trovano nell’Unione europea (Scaleway, regioni fr-par e nl-ams, e una zona on-premise a Madrid), con crittografia in transito (TLS 1.3) e a riposo. Le connessioni ai servizi esterni inviano i dati che autorizzi al loro fornitore, secondo la sua ubicazione e le sue condizioni.',
  'privacy.basis.title': 'Base giuridica',
  'privacy.basis.body':
    'I trattamenti si fondano sulle basi dell’articolo 6 del GDPR: l’esecuzione del contratto di servizio con la tua organizzazione, il legittimo interesse alla sicurezza e alla prevenzione degli abusi, l’adempimento di obblighi di legge e, quando espressamente richiesto, il tuo consenso. Il dettaglio base per base, trattamento per trattamento, farà parte del testo definitivo.',
  'privacy.retention.title': 'Conservazione',
  'privacy.retention.body':
    'Conserviamo i dati dell’account e i contenuti finché la tua organizzazione mantiene attivo il servizio. Alla chiusura di un account o di un’organizzazione, i dati vengono cancellati o anonimizzati entro un termine ragionevole, salvo quanto dobbiamo conservare per obbligo di legge (ad esempio la fatturazione). I registri tecnici di esercizio vengono ripuliti periodicamente. I termini specifici per categoria sono indicati nella versione finale. Le segnalazioni di contenuti sono conservate finché restano aperte e cancellate dodici mesi dopo essere state risolte.',
  'privacy.processors.title': 'Responsabili e sub-responsabili',
  'privacy.processors.body':
    'Il nostro fornitore di infrastruttura è Scaleway (regioni fr-par e nl-ams), che agisce come responsabile del trattamento e opera esclusivamente nell’Unione europea; gestiamo inoltre una zona on-premise a Madrid (Spagna), in locali propri. Per l’incasso degli abbonamenti ci avvaliamo di Mollie B.V. (Paesi Bassi), che tratta solo i dati necessari al pagamento. Per inviare l’SMS con il codice che verifica il numero di cellulare alla creazione di un account ci avvaliamo di Sweego (Mindbaz SAS, Francia), che riceve solo quel numero e il codice. Non ricorriamo a sub-responsabili fuori dallo SEE. L’elenco dei responsabili sarà pubblicato e tenuto aggiornato, con preavviso di ogni nuovo inserimento.',
  'privacy.security.title': 'Sicurezza',
  'privacy.security.body':
    'Applichiamo misure tecniche e organizzative conformi al GDPR: cifratura in transito (TLS 1.3) e a riposo, isolamento tra organizzazioni (multi-tenant con sicurezza a livello di riga), autenticazione forte (MFA/WebAuthn), registrazione degli accessi e principio del minimo privilegio. Nessun sistema è infallibile: se si verificasse una violazione che ti riguarda, te lo notificheremo nei termini previsti dal Regolamento.',
  'privacy.cookies.title': 'Cookie',
  'privacy.cookies.body':
    'Usiamo solo cookie tecnici necessari all’accesso unico e alla sicurezza della sessione. Non usiamo cookie pubblicitari né di tracciamento di terze parti: per questo non ti mostriamo alcun banner di consenso al tracciamento.',
  'privacy.transfers.title': 'Trasferimenti internazionali',
  'privacy.transfers.body':
    'Il trattamento e l’hosting gestiti da Sytue avvengono in regioni dell’Unione europea. Le connessioni esterne attivate dall’utente inviano i dati autorizzati al loro fornitore; la loro ubicazione dipende dal fornitore e dalla sua configurazione. Anche le notifiche inviate con l’app o il browser chiusi possono utilizzare servizi esterni. Il servizio di push del tuo dispositivo (Google, Mozilla o Apple, a seconda del browser o del telefono) vede un identificatore anonimo del dispositivo e l’ora di ogni notifica; mai il contenuto, che viaggia cifrato end-to-end e su iOS parte addirittura vuoto. Puoi disattivare le notifiche, e allora non esce nemmeno quello.',
  'privacy.rights.title': 'I tuoi diritti',
  'privacy.rights.body1':
    'Il GDPR ti riconosce i diritti di accesso, rettifica, cancellazione, opposizione, limitazione del trattamento e portabilità. Puoi esercitarli scrivendo a',
  'privacy.rights.body2':
    '; rispondiamo nei termini previsti dal Regolamento. Hai inoltre il diritto di proporre reclamo alla',
  'privacy.rights.authority': 'tua autorità di controllo',
  'privacy.dpo.title': 'Responsabile della protezione dei dati (DPO)',
  'privacy.dpo.body':
    'Per qualsiasi domanda sulla protezione dei dati o per contattare il nostro responsabile della protezione dei dati:',
  'privacy.minors.title': 'Minori',
  'privacy.minors.body':
    'Sytue One è un servizio per organizzazioni e per le persone che ci lavorano; non è rivolto a minori e non raccogliamo consapevolmente dati di minori.',
  'privacy.changes.title': 'Modifiche a questa informativa',
  'privacy.changes.body':
    'Se aggiorniamo questa informativa cambieremo la data di «ultimo aggiornamento» e, quando le modifiche saranno rilevanti, te lo comunicheremo attraverso i canali del servizio prima che abbiano effetto.',

  // --- Legal: términos -----------------------------------------------------
  'meta.terms.title': 'Termini del servizio — Sytue One',
  'meta.terms.description':
    'Termini del servizio di Sytue One: fornitore, oggetto, account e organizzazioni, uso accettabile, disponibilità, i tuoi dati, proprietà intellettuale e legge applicabile.',
  'terms.title': 'Termini del servizio',
  'terms.review.body':
    'Questi termini descrivono con esattezza il servizio e il suo uso. Le clausole contrattuali specifiche (livello di servizio, responsabilità e foro competente) saranno completate nella versione finale, rivista dal nostro team legale.',
  'terms.provider.title': 'Chi eroga il servizio',
  'terms.provider.body':
    'Sytue One è erogato da Sytue Projects, S.L. (codice fiscale spagnolo B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spagna; iscritta al Registro delle imprese di Madrid, sezione 8, foglio M-887153) da un’infrastruttura situata esclusivamente nell’Unione europea (Scaleway, regioni fr-par e nl-ams, e una zona on-premise a Madrid), ed è regolato dal diritto dell’UE.',
  'terms.scope.title': 'Oggetto del servizio',
  'terms.scope.body':
    'Sytue One è una suite di produttività per organizzazioni: identità (Sytue ID), posta, file, documenti, calendario, contatti e messaggistica con videochiamate. Il servizio si sottoscrive per organizzazione.',
  'terms.accounts.title': 'Account e organizzazioni',
  'terms.accounts.body':
    'Ogni organizzazione amministra i propri account, gruppi e permessi tramite Sytue ID (SSO + MFA). L’amministrazione dell’organizzazione è responsabile delle attivazioni, delle disattivazioni e dell’uso che si fa del servizio con quegli account.',
  'terms.use.title': 'Uso accettabile',
  'terms.use.body':
    'Ti impegni a usare il servizio nel rispetto della legge e a non impiegarlo per attività illecite, invio di spam, distribuzione di malware, violazione di diritti altrui o qualsiasi uso che comprometta la sicurezza o la disponibilità del servizio per altre organizzazioni.',
  'terms.availability.title': 'Disponibilità e assistenza',
  'terms.availability.body1':
    'Lavoriamo per offrire un’alta disponibilità ed eseguiamo manutenzioni e migliorie in modo continuo. Gli impegni specifici sul livello di servizio (SLA), se si applicano al tuo piano, sono fissati nel contratto di servizio. L’assistenza è erogata tramite',
  'terms.data.title': 'I tuoi dati',
  'terms.data.body1':
    'I contenuti che la tua organizzazione ospita nella suite appartengono alla tua organizzazione. Sono costruiti su standard aperti (OIDC, JMAP, CalDAV, CardDAV, WebDAV), così i tuoi dati restano sempre portabili e puoi portarteli via. Il trattamento dei dati personali è descritto nella pagina',
  'terms.ip.title': 'Proprietà intellettuale',
  'terms.ip.body':
    'Il software di Sytue One è opera originale di Sytue ed è protetto dai relativi diritti di proprietà intellettuale. Conservi la titolarità dei contenuti che la tua organizzazione ospita nella suite; noi li trattiamo solo per erogarti il servizio.',
  'terms.liability.title': 'Responsabilità',
  'terms.liability.body':
    'Mettiamo il massimo impegno in un servizio affidabile e sicuro. Le garanzie e le limitazioni di responsabilità applicabili sono fissate nel contratto di servizio; nulla in questi termini esclude responsabilità che la legge non consente di escludere.',
  'terms.contact.title': 'Sottoscrizione e contatti',
  'terms.contact.body1':
    'Per condizioni di sottoscrizione, prezzi per organizzazione o qualsiasi domanda sul servizio, scrivici a',
  'terms.changes.title': 'Modifiche',
  'terms.changes.body':
    'Possiamo aggiornare questi termini per riflettere cambiamenti nel servizio o nella normativa. Quando le modifiche saranno rilevanti te lo comunicheremo attraverso i canali del servizio prima che abbiano effetto, aggiornando la data di «ultimo aggiornamento».',
  'terms.law.title': 'Legge applicabile',
  'terms.law.body':
    'Il servizio è regolato dal diritto dell’Unione europea e dalla normativa del Paese del fornitore. Il foro competente per la risoluzione delle controversie sarà indicato nel testo definitivo. Indipendentemente da ciò, conservi il diritto di proporre reclamo alla tua autorità di controllo in materia di protezione dei dati.',

  // --- Datos estructurados (JSON-LD) --------------------------------------
  'seo.org.description':
    'Sytue One: la suite di produttività sovrana europea. Identità, posta, file, documenti, calendario, chat, videochiamate e una PaaS, al 100 % nell’UE.',
  'seo.org.slogan': 'La tua produttività, sovrana ed europea.',
  'seo.software.description':
    'Suite di produttività sovrana europea: posta (JMAP), file (WebDAV), documenti collaborativi, calendario (CalDAV), contatti (CardDAV), chat, videochiamate e PaaS. SSO con Sytue ID.',
  'seo.software.f1': 'Accesso unico (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Posta, file, documenti, calendario, contatti, chat e videochiamate',
  'seo.software.f3': 'Piattaforma di deploy e dati gestiti',
  'seo.software.f4': 'Dati al 100 % nell’UE (Scaleway)',
  'seo.offer.description': 'Prova gratuita, senza carta di credito.',
  'seo.downloads.description':
    'App native della suite Sytue One per desktop (macOS, Windows, Linux) e mobile (iOS, Android).',

  // --- Precios: cuenta personal ---
  'home.pricing.personal.title': 'Gratis',
  'home.pricing.personal.freeStorage': '5 GB, per una persona',
  'home.pricing.personal.freeBody':
    'Posta, file, foto, documenti, calendario, contatti, note e stanze. Senza carta e senza limiti di tempo.',
  'home.pricing.personal.note':
    'La tua organizzazione è la tua email: niente amministrazione dell’organizzazione né ERP, e sì a tutto il resto, incluso Sytue Admin per integrazioni e migrazioni.',
  'home.pricing.personal.cta': 'Crea un account gratis',
  'grid.orgOnly': 'Solo per organizzazioni',
  'grid.enDesarrollo': 'In sviluppo',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB per utente',
  'home.pricing.paidBody':
    'Per le organizzazioni e anche per una persona sola. Tutta la suite, con Sytue ERP e Sytue Admin per gestire la tua organizzazione.',
  'home.pricing.thirdParty':
    'Tutti i piani permettono di collegare il tuo storage: Google Drive, Microsoft OneDrive, un bucket S3 (Scaleway, OVH, Hetzner, Wasabi, MinIO), un server WebDAV come Nextcloud o Internxt, o il tuo NAS nella tua sede —senza aprire porte, tramite un tunnel Sytue—. I backup e il ripristino dei dati salvati in uno storage collegato sono responsabilità del cliente (lei), e Sytue® è sollevata da ogni responsabilità per la perdita di dati negli storage collegati a Sytue®.',
  'home.pricing.soon': 'Presto',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB per utente',
  'home.pricing.enterprise.support': 'Con Supporto Premium.',
  'home.pricing.education': 'Istituzioni educative: 50 % di sconto su Pro.',
};
