/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ALEMÁN del sitio público. Traducción del inglés (`en.ts`), que es la
 * fuente de verdad: el tipo se deriva de él y una clave de más o de menos rompe
 * la compilación (docs/CONVENTIONS.md §13.1).
 *
 * El alemán es ~30 % más largo que el inglés: los TITULARES de este catálogo se
 * han acortado a propósito (no son calcos) para que no desborden el hero ni las
 * tarjetas. Tratamiento formal («Sie»), como corresponde a B2B. Vocabulario
 * DSGVO oficial: Verantwortlicher, Auftragsverarbeiter, Aufsichtsbehörde.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  // --- Cromo del sitio ---------------------------------------------------
  'skip.content': 'Zum Inhalt springen',
  'nav.products': 'Produkte',
  'nav.sovereignty': 'Souveränität',
  'nav.standards': 'Offene Standards',
  'nav.pricing': 'Preise',
  'footer.pricing': 'Preise',
  'home.pricing.title': 'Drei Tarife, die ganze Suite',
  'home.pricing.body':
    'Dieselben Produkte in allen dreien: Was sich ändert, ist der Speicher pro Nutzer. Keine abgespeckten Editionen.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'zzgl. MwSt.',
  'home.pricing.per': '/Nutzer/Monat',
  'home.pricing.cta': 'Mit dem Vertrieb sprechen',
  'home.pricing.commitment.title': 'Eine Zusage fürs Leben',
  'home.pricing.commitment.body':
    'Der Preis pro Nutzer steigt alle vier Jahre um 0,50 €, und sonst nichts: lebenslang. Du weißt es am Tag deines Beitritts, und es ändert sich nicht.',
  'home.pricing.included.title': 'Eine einzige Edition',
  'home.pricing.included.body':
    'Keine Tarife pro Funktion, keine Stufen, keine abgespeckten Editionen: Die gesamte Suite ist im Preis pro Nutzer enthalten, mit denselben Funktionen für alle.',
  'home.pricing.metered.title': 'Was separat berechnet wird',
  'home.pricing.metered.body': 'Zusätzlicher Speicher: nach Verbrauch oder in Blöcken.',
  'home.pricing.storage.title': 'Der Speicher wird geteilt',
  'home.pricing.storage.body':
    'Das Kontingent jeder Person zählt zum Kontingent der Organisation: Was die eine nicht verbraucht, nutzen die anderen. Und das Kontingent pro Nutzer lässt sich in der Organisation anpassen.',
  'nav.downloads': 'Downloads',
  'nav.aria': 'Website-Navigation',
  'nav.ariaMobile': 'Mobile Navigation',
  'nav.home': 'Sytue One — Startseite',
  'nav.openMenu': 'Menü öffnen',
  'nav.closeMenu': 'Menü schließen',
  'theme.toLight': 'Zum hellen Modus wechseln',
  'theme.toDark': 'Zum dunklen Modus wechseln',
  'lang.label': 'Sprache',
  'cta.start': 'Kostenlos starten',
  'cta.signIn': 'Anmelden',
  'cta.seeSuite': 'Suite ansehen',

  // --- Portada -----------------------------------------------------------
  'meta.home.title': 'Sytue One — Die souveräne europäische Produktivitäts-Suite',
  'meta.home.description':
    'Sytue One: die souveräne europäische Produktivitäts-Suite. Identität, E-Mail, Dateien, Dokumente, Kalender und Chat — zu 100 % in der EU und auf offenen Standards.',
  'home.hero.lead': 'Ihre Produktivität,',
  'home.hero.accent': 'souverän und europäisch',
  'home.hero.subtitle':
    'E-Mail, Dateien, Dokumente, Kalender, Chat und Videoanrufe — integriert wie eine einzige Anwendung und vollständig in der EU gehostet.',
  'home.hero.note': 'Ohne Kreditkarte. Daten bei Scaleway (EU). Begleitete Migration.',
  'home.privacy.title': 'Ihre Arbeit gehört Ihnen. Ihre Privatsphäre auch.',
  'home.privacy.body':
    'Wir verkaufen Ihre Daten nicht und nutzen sie nicht, um Werbeprofile zu erstellen.',
  'home.privacy.anonymity':
    'Sytue Analytics schützt Ihre Privatsphäre mit aggregierten Statistiken, ohne Tracking-Cookies und mit pseudonymen Besucherkennungen, die täglich wechseln und für jede Website getrennt sind.',
  'home.privacy.account':
    'Für die Nutzung der Suite benötigen wir Kontodaten und Zugriffsprotokolle, damit wir den Dienst bereitstellen und sicher betreiben können.',
  'home.privacy.link': 'So schützen wir Ihre Privatsphäre',

  'home.stats.aria': 'Europäische Präsenz',
  'home.stats.regions': 'EU-Regionen (fr-par · nl-ams · On-Premise Madrid)',
  'home.stats.apps': 'Apps mit einer einzigen Identität',
  'home.stats.standards': 'Offene Standards: OIDC, JMAP, CalDAV, CardDAV und WebDAV',

  'home.products.title': 'Eine Suite, ein Erlebnis',
  'home.products.subtitle':
    'Jedes Produkt teilt Design, Identität und Oberfläche. Sie wirken und verhalten sich wie eine einzige Anwendung.',

  'home.integration.title': 'Integration, wie sie sein soll',
  'home.integration.subtitle':
    'Eine Anmeldung öffnet alles. Automatische Bereitstellung (SCIM) aus Sytue ID.',
  'home.integration.sso.title': 'Single Sign-on',
  'home.integration.sso.body':
    'Eine Identität (Sytue ID) für die gesamte Suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Gemeinsame Oberfläche',
  'home.integration.shell.body':
    'Topbar, App-Umschalter, Suche und globale Benachrichtigungen in jedem Produkt.',
  'home.integration.cross.title': 'Übergreifende Aktionen',
  'home.integration.cross.body':
    'Dateien aus Drive in Mail anhängen, aus einer E-Mail einen Calendar-Termin machen und ihn in Rooms teilen — ohne den Arbeitsfluss zu verlassen.',

  'home.sovereignty.title': 'Echte Souveränität',
  'home.sovereignty.body':
    'Das ist kein Marketingversprechen, sondern die Architektur. Die Infrastruktur läuft ausschließlich in europäischen Regionen, und alles baut auf offenen, prüfbaren Protokollen auf — ohne Hintertüren.',
  'home.sovereignty.link': 'Warum offene Standards',
  'home.sovereignty.eu.title': '100 % in der EU',
  'home.sovereignty.eu.body':
    'Die gesamte Infrastruktur in der EU: Scaleway (fr-par / nl-ams) und eine On-Premise-Zone in Madrid. Keine US-Anbieter für personenbezogene Daten.',
  'home.sovereignty.gdpr.title': 'DSGVO-Konformität',
  'home.sovereignty.gdpr.body':
    'AV-Vertrag, Verarbeitungsverzeichnis, Runbook für Betroffenenrechte und Löschfristen. Verschlüsselt bei der Übertragung (TLS 1.3) und im Ruhezustand.',
  'home.sovereignty.lockin.title': 'Kein Lock-in',
  'home.sovereignty.lockin.body':
    'Auf offenen Standards gebaut: OIDC, JMAP, CalDAV, CardDAV und WebDAV. Ihre Daten bleiben portabel.',

  'home.standards.overline': 'Eigenentwicklung von Sytue',
  'home.standards.title': 'Auf offenen Standards gebaut',
  'home.standards.body':
    'Jedes Sytue-Produkt ist eine Eigenentwicklung in Go und Astro + Preact auf offenen Protokollen (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Keine fremden Engines: Ihre Daten sind portabel und Ihre Suite ist souverän.',

  'home.cta.title': 'Holen Sie sich die Kontrolle zurück',
  'home.cta.body':
    'Starten Sie heute mit Ihrer Organisation. Begleitete Migration von Google Workspace oder Microsoft 365.',
  'home.cta.primary': 'Organisation anlegen',
  'home.cta.secondary': 'Mit dem Vertrieb sprechen',

  // --- Rejilla de productos ----------------------------------------------
  'grid.discover': 'Entdecken',
  'product.mail.blurb': 'Private, europäische E-Mail über JMAP — ohne Tracker und Werbung.',
  'product.drive.blurb': 'Dateien und Zusammenarbeit im Design Ihrer Organisation.',
  'product.docs.blurb': 'Dokumente, Tabellen und Präsentationen mit Echtzeit-Zusammenarbeit.',
  'product.calendar.blurb': 'Ein CalDAV-Kalender, verzahnt mit der restlichen Suite.',
  'product.contacts.blurb': 'CardDAV-Kontakte, geteilt in Ihrer Organisation.',
  'product.notes.blurb': 'Notizbücher für alles: Text, Dateien, Termine, ganze Dokumente.',
  'product.photos.blurb':
    'Fotogalerie und -editor: Ihre Originale in Drive, Ihre Bearbeitung als Rezept.',
  'product.rooms.blurb': 'Kanäle, Nachrichten und Team-Videoanrufe an einem Ort.',
  'product.cast.blurb':
    'Video auf Abruf und Livestreaming: hochladen, senden und teilen, in der EU.',
  'product.pages.blurb':
    'Websites und Onlineshops für Ihr Unternehmen, nach Ihren Vorstellungen gestaltet und in der EU veröffentlicht.',
  'product.bases.blurb':
    'Gemeinsame relationale Tabellen für Kunden, Bestellungen oder Bestände, in der EU.',
  'product.cloud.blurb': 'Plattform für Deployments und verwaltete Daten, in der EU.',
  'product.id.blurb': 'Eine einzige Identität (SSO + MFA) mit Sytue ID.',
  'product.one.blurb':
    'Ihre Startseite der Suite: Anwendungen, Schnellzugriffe, letzte Aktivität und Hinweise.',
  'product.sheets.blurb':
    'Tabellenkalkulation mit Formeln, externen Daten und Diagrammen, von vielen Händen bearbeitet.',
  'product.slides.blurb': 'Präsentationen mit Ihrer Marke, Designs und Referentenansicht.',
  'product.pdf.blurb':
    'PDFs lesen, kommentieren, unterschreiben und ordnen, ohne dass sie Europa verlassen.',
  'product.console.blurb': 'Verwaltung der Organisation: Personen, Zugriff, Compliance und Audit.',
  'product.erp.blurb':
    'Vierundzwanzig Module und ein doppisches Buch, das die Datenbank nicht aus dem Gleichgewicht lässt.',

  // --- Fichas de producto -------------------------------------------------
  'productPage.back': 'Die gesamte Suite',
  'productPage.features': 'Alles, was {name} kann',
  'productPage.featuresPlanned': 'Was {name} können wird',
  'productPage.builtOn': 'Gebaut auf {standard}',
  'productPage.fromScratch':
    'Der Editor von Docus, Calcus, Diapos und CertiPDF ist von Grund auf neu geschrieben: eine eigene Engine von Sytue, keine Integration eines Drittanbieters.',
  'productPage.shot': 'Der Hauptbildschirm von {name}, genau so wie nach der Anmeldung.',
  'productPage.shotFeatures': '{name} im Einsatz, mit seinen Funktionen im Bild.',
  'productPage.open': '{name} öffnen',
  'productPage.enDesarrollo':
    'Dieses Produkt befindet sich in Entwicklung: Es ist noch nicht verfügbar.',
  'productPage.included':
    'In Sytue One enthalten: Eine Identität (Sytue ID) und eine Anmeldung geben Zugriff auf alle Produkte. Daten zu 100 % in der EU (Scaleway) und portabel über offene Standards.',

  'product.mail.tagline': 'Private, europäische E-Mail — ohne Tracker und ohne Werbung.',
  'product.mail.standard': 'JMAP (offener Standard)',
  'product.mail.p1': 'Schneller Posteingang mit Threads, Labels, Filtern und Suche.',
  'product.mail.p2': 'Dateien direkt aus Sytue Drive anhängen.',
  'product.mail.p3': 'Aus einer E-Mail einen Calendar-Termin machen.',
  'product.mail.p4':
    'Verschlüsselt bei der Übertragung (TLS 1.3). Die Infrastruktur von Sytue befindet sich in der EU. Der Standort externer Dienste hängt von ihrem Anbieter ab.',

  'product.drive.tagline': 'Ihre Dateien und Ihre Zusammenarbeit, im Design Ihrer Organisation.',
  'product.drive.standard': 'WebDAV (offener Standard)',
  'product.drive.p1':
    'Hochladen, ordnen und über kontrollierte Links teilen (Rechte und Ablaufdatum).',
  'product.drive.p2': 'Inline-Vorschau für Dokumente und Bilder.',
  'product.drive.p3': 'An Mail anhängen, ohne die Datei zu duplizieren.',
  'product.drive.p4': 'Speicher auf S3-Basis in europäischen Regionen.',

  'product.docs.tagline': 'Dokumente, Tabellen und Präsentationen mit Echtzeit-Zusammenarbeit.',
  'product.docs.standard': 'Kollaboratives Bearbeiten in Echtzeit',
  'product.docs.p1': 'Zu mehreren schreiben, mit Präsenz und sofortigen Änderungen.',
  'product.docs.p2': 'Tabellen mit Formeln und bedingter Formatierung.',
  'product.docs.p3': 'Präsentationen im Design Ihrer Marke.',
  'product.docs.p4': 'Alles gespeichert in Ihrem europäischen Drive.',

  'product.calendar.tagline': 'Ihr Kalender, verzahnt mit der restlichen Suite.',
  'product.calendar.standard': 'CalDAV (offener Standard)',
  'product.calendar.p1': 'Tages-, Wochen- und Monatsansicht.',
  'product.calendar.p2': 'Serientermine und Erinnerungen.',
  'product.calendar.p3': 'Termine direkt aus einer E-Mail in Mail anlegen.',
  'product.calendar.p4': 'Kompatibel mit jedem CalDAV-Client.',

  'product.contacts.tagline': 'Die Kontakte Ihrer Organisation — geteilt und portabel.',
  'product.contacts.standard': 'CardDAV (offener Standard)',
  'product.contacts.p1': 'Gemeinsames Adressbuch für die ganze Organisation.',
  'product.contacts.p2': 'Import und Export im vCard-Format.',
  'product.contacts.p3': 'Duplikate zusammenführen, ohne Daten zu verlieren.',
  'product.contacts.p4': 'Kompatibel mit jedem CardDAV-Client.',
  'product.notes.tagline': 'Alles, was Sie behalten wollen — an einem Ort und in Ihrer Hand.',
  'product.notes.standard': 'Eigenes Modell auf offenen Standards (eigene REST-API)',
  'product.notes.p1': 'Verschachtelte Notizbücher: freie Fläche, Spalten oder Liste.',
  'product.notes.p2':
    'Notizen jeder Art: Text, Aufgaben, Links, Dateien, Kalendertermine, Docs-Dokumente.',
  'product.notes.p3': 'Ende-zu-Ende-Verschlüsselung: die Passphrase verlässt Ihren Browser nicht.',
  'product.notes.p4': 'Sie entscheiden, was in die Suche kommt — und was nie.',

  'product.rooms.tagline': 'Kanäle, Nachrichten und Team-Videoanrufe, vereint.',
  'product.rooms.standard': 'Messaging und Anrufe in Echtzeit',
  'product.rooms.p1': 'Kanäle pro Team oder Projekt.',
  'product.rooms.p2': 'Videoanruf direkt in jedem Kanal.',
  'product.rooms.p3': 'Erwähnungen und Benachrichtigungen suiteweit vereinheitlicht.',
  'product.rooms.p4': 'Durchsuchbarer Verlauf aus ganz Sytue One.',

  'product.cloud.p4': 'Eigene Domains mit automatischem TLS.',

  'product.cloud.p3': 'Verwalteter S3-Speicher und Functions.',
  'product.cloud.p2': 'Postgres-Datenbank mit automatischer REST-API.',
  'product.cloud.p1': 'Eine Preview für jeden PR und sofortiges Rollback.',
  'product.cloud.standard': 'Kubernetes-native PaaS',
  'product.cloud.tagline':
    'Dein Code deployt und deine Daten verwaltet, in der Europäischen Union.',
  'product.id.tagline': 'Eine Identität für die gesamte Suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (offener Standard)',
  'product.id.p1': 'Single Sign-on über alle Produkte hinweg.',
  'product.id.p2': 'MFA und Passkeys (WebAuthn).',
  'product.id.p3': 'Automatische Bereitstellung (SCIM) aus Ihrem Verzeichnis.',
  'product.id.p4': 'Profil und Sicherheit an einem Ort.',
  'product.one.tagline': 'Ganz Sytue One einen Klick entfernt, durch dieselbe Tür.',
  'product.one.standard': 'Eine Anmeldung (Sytue ID) für die ganze Suite',
  'product.one.p1':
    'Ein Launcher mit allen Ihren Anwendungen und denen, die Ihre Organisation anbindet.',
  'product.one.p2': 'Suche in der ganzen Suite (⌘K) von jedem Bildschirm aus.',
  'product.one.p3':
    'Deine heutige Agenda und ungelesene Mails auf der Startseite, neben dem Neuesten aus der ganzen Suite und den Benachrichtigungen.',
  'product.one.p4': 'Verbinden Sie Ihre Geräte und laden Sie die nativen Apps.',
  'product.sheets.tagline': 'Zahlen, die man versteht, in einem Blatt, das mitarbeitet.',
  'product.sheets.standard': 'ODF/OOXML-Formate und gemeinsames Bearbeiten in Echtzeit',
  'product.sheets.p1': 'Formeln, bedingte Formatierung und Datenprüfung.',
  'product.sheets.p2': 'Externe Daten einbinden und abfragen, ohne das Blatt zu verlassen.',
  'product.sheets.p3': 'Diagramme, Sparklines, Datenschnitte und Datentabellen.',
  'product.sheets.p4': 'Export als ODS und XLSX; Excel-Dateien öffnen.',
  'product.slides.tagline': 'Erzählen Sie die Geschichte; die Folie hält Schritt.',
  'product.slides.standard': 'ODF/OOXML-Formate und gemeinsames Bearbeiten in Echtzeit',
  'product.slides.p1': 'Präsentationsdesigns mit der Identität Ihrer Organisation.',
  'product.slides.p2': 'Referentenansicht mit Notizen und nächster Folie.',
  'product.slides.p3': 'Bilder, Formen, Diagramme und SmartArt.',
  'product.slides.p4': 'Export als ODP, PPTX und PDF; PowerPoint-Dateien öffnen.',
  'product.pdf.tagline': 'Das PDF, von Anfang bis Ende, in Ihrem Browser.',
  'product.pdf.standard': 'PDF (ISO 32000) mit Standard-Anmerkungen und -Formularen',
  'product.pdf.p1': 'Markieren, unterstreichen, kommentieren und auf dem Dokument zeichnen.',
  'product.pdf.p2': 'Formulare: Felder ausfüllen oder eigene anlegen.',
  'product.pdf.p3':
    'Zusammenführen, extrahieren, drehen, nummerieren und mit Wasserzeichen versehen.',
  'product.pdf.p4': 'Signaturbereich, endgültiges Schwärzen und passwortgeschützte Dokumente.',
  'product.console.tagline': 'Steuern Sie die Suite, ohne einen Bildschirm zu verlassen.',
  'product.console.standard': 'SCIM, SAML und OpenID Connect (offene Standards)',
  'product.console.p1': 'Personen, Gruppen (auch dynamische) und Rollen.',
  'product.console.p2': 'SSO mit Ihrem Anbieter und SCIM-Provisionierung.',
  'product.console.p3':
    'Regeln für bedingten Zugriff, Geräte und privilegierter Zugriff mit Genehmigung.',
  'product.console.p4': 'Audit-Export an Ihr SIEM und Legal Holds.',
  'product.erp.tagline': 'Das ganze ERP – und der Modellkatalog ist eine Datendatei.',
  'product.erp.standard': 'Doppelte Buchführung, Modellkatalog als JSON und eine einzige REST-API',
  'product.erp.p1':
    'Vierundzwanzig Module und sechsunddreißig Modelle: Kontakte, Verkauf, Einkauf, Lager, Fertigung, Fakturierung, Buchhaltung, Projekte und Personal.',
  'product.erp.p2':
    'Ein neues Modul ist eine JSON-Datei: ohne Migration, ohne neue Route und ohne Eingriff in die Oberfläche.',
  'product.erp.p3':
    'Beträge liegen in ganzen Cent vor, nie als Gleitkommazahl, und den Ausgleich erzwingt die Datenbank.',
  'product.erp.p4':
    'Gebuchtes bleibt unangetastet: korrigiert wird mit einer weiteren Buchung – das unterscheidet ein Buch von einer Tabelle.',
  'product.one.f1':
    'Ein Launcher mit den Anwendungen der Suite und den Drittanbieter-Apps, die Ihre Organisation anbindet.',
  'product.one.f2':
    'Schnellzugriffe: E-Mail schreiben, neues Dokument, in Drive hochladen, Termin anlegen.',
  'product.one.f3':
    'Zuletzt verwendet in der ganzen Suite: das Letzte aus Drive, Docus, Mail oder Rooms.',
  'product.one.f4': 'Globale Suche mit ⌘K oder Strg K, mit Ergebnissen aus allen Anwendungen.',
  'product.one.f5': 'Benachrichtigungszentrum und Präsenz, geteilt in der ganzen Suite.',
  'product.one.f6':
    'Geräte verbinden: Profil für iPhone und Mac, Android und Desktop, in zwei Schritten.',
  'product.one.f7': 'Downloads der nativen Desktop- und Mobil-Apps.',
  'product.one.f8': 'Ein Willkommensassistent, der die Organisation ohne Handbuch einrichtet.',
  'product.mail.f1': 'Unterhaltungsstränge mit Lesen, Antworten und Weiterleiten in einem Klick.',
  'product.mail.f2':
    'Ordner, farbige Labels und Suche mit Operatoren (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Automatische Regeln, die Post beim Eingang archivieren, kennzeichnen oder verschieben.',
  'product.mail.f4': 'Zeitversetztes Senden und „Senden rückgängig“.',
  'product.mail.f5': 'Signaturen, Adress-Aliasse und automatische Abwesenheitsantwort.',
  'product.mail.f6': 'Geteilte und delegierte Postfächer mit Lese- und Senderechten.',
  'product.mail.f7':
    'Editor mit Rich-Text und Anhängen aus Sytue Drive, ohne Dateien zu duplizieren.',
  'product.mail.f8':
    'Externe Inhalte standardmäßig blockiert: niemand erfährt, wann Sie eine E-Mail öffnen.',
  'product.mail.f9':
    'Signierte Post (PGP und S/MIME) wird erkannt und markiert; TLS beim Transport.',
  'product.mail.f10':
    'Zugriff auch aus Thunderbird, Outlook, Apple Mail oder vom Handy, mit App-Passwörtern.',
  'product.mail.f11':
    'Adressen als Chips mit dem Adressbuch aus Contacts einen Klick entfernt, und Hinweis, wenn du «Anhang» schreibst, ohne etwas anzuhängen.',
  'product.mail.f12':
    'Regeln direkt aus der geöffneten E-Mail, Papierkorb in einem Zug leeren und Tastenkürzel mit «?».',
  'product.drive.f1':
    'Meine Ablage, Team-Ablagen, Mit mir geteilt, Zuletzt, Favoriten und Papierkorb.',
  'product.drive.f2':
    'Hochladen per Ziehen; Ordner, verschieben, kopieren, umbenennen und Mehrfachauswahl.',
  'product.drive.f3':
    'Freigabelinks mit zwei Stufen (ansehen oder bearbeiten), Passwort und Ablauf.',
  'product.drive.f4': 'Versionsverlauf mit Wiederherstellung.',
  'product.drive.f5': 'Vorschau von Dokumenten, Bildern und PDFs ohne Download.',
  'product.drive.f6':
    'Office-Dokumente in Docus, Calcus und Diapos öffnen und bearbeiten; PDFs in CertiPDF.',
  'product.drive.f7':
    'Vertraulichkeitslabels, die öffentliche Links unterbinden, wo sie nicht hingehören.',
  'product.drive.f8':
    'In einen Rooms-Kanal teilen oder an Mail anhängen, ohne die Datei zu duplizieren.',
  'product.drive.f9': 'WebDAV-Zugriff und Desktop-Client, um die Dateien am Rechner zu sehen.',
  'product.drive.f10': 'S3-Speicher in europäischen Regionen, mit stets sichtbarem Kontingent.',
  'product.docs.f1':
    'Gemeinsames Bearbeiten in Echtzeit, mit Präsenz und ohne sich in die Quere zu kommen.',
  'product.docs.f2':
    'Ein Menüband mit Registern: Start, Einfügen, Referenzen, Seriendruck und Überprüfen.',
  'product.docs.f3': 'Tabellen, Bilder, Formen, SmartArt, Diagramme, Symbole und Wasserzeichen.',
  'product.docs.f4': 'Referenzen: Inhaltsverzeichnis, Fußnoten, Zitate und Literaturverzeichnis.',
  'product.docs.f5': 'Seriendruck: Datenzusammenführung für Briefe und Etiketten.',
  'product.docs.f6': 'Kommentare im Strang, Änderungsprüfung und Versionen.',
  'product.docs.f7':
    'Optionaler KI-Assistent zum Umschreiben, Übersetzen oder Zusammenfassen, nur wenn Ihre Organisation ihn aktiviert.',
  'product.docs.f8': 'Export als ODT, DOCX und PDF; Word-Dateien aus Drive öffnen.',
  'product.docs.f9': 'Kopf- und Fußzeilen, Seitennummern und wiederverwendbare Bausteine.',
  'product.docs.f10': 'Automatisches Speichern in Ihrem europäischen Drive, mit Verlauf.',
  'product.docs.f11':
    'Lade von der Startseite als PDF, HTML, Markdown, OpenDocument oder Microsoft Office herunter.',
  'product.sheets.f1': 'Formeln mit Kontexthilfe und sofortiger Neuberechnung.',
  'product.sheets.f2': 'Bedingte Formatierung, Datenprüfung und regionale Zahlenformate.',
  'product.sheets.f3': 'Diagramme mehrerer Typen, Sparklines und Datenschnitte.',
  'product.sheets.f4': 'Datentabellen mit Summen und Sortierung; Pivot-Tabelle.',
  'product.sheets.f5': 'Externe Daten: eine Quelle verbinden und aus dem Blatt abfragen.',
  'product.sheets.f6': 'Solver, um den Wert zu finden, der ein Ziel erfüllt.',
  'product.sheets.f7': 'Makros zur Automatisierung wiederkehrender Aufgaben.',
  'product.sheets.f8': 'Zusammenarbeit in Echtzeit mit Präsenz und Kommentaren.',
  'product.sheets.f9': 'Filter, Datenschnitte und Suche im Blatt.',
  'product.sheets.f10': 'Export als ODS, XLSX und PDF; Excel-Dateien aus Drive öffnen.',
  'product.slides.f1': 'Designs und Paletten passend zur Marke Ihrer Organisation.',
  'product.slides.f2': 'Eine Fläche mit Formen, Bildern, Diagrammen, SmartArt und Verbindern.',
  'product.slides.f3': 'Im Vollbild präsentieren, mit Referentenansicht und Notizen.',
  'product.slides.f4': 'Zusammenarbeit in Echtzeit mit Präsenz und Kommentaren.',
  'product.slides.f5': 'Folien mit Hintergrund, Postern und Layouts.',
  'product.slides.f6': 'Export als ODP, PPTX und PDF.',
  'product.slides.f7': 'PowerPoint-Dateien aus Drive öffnen.',
  'product.slides.f8': 'Automatisches Speichern mit Versionen.',
  'product.pdf.f1':
    'Schneller Viewer mit Gliederung, Miniaturen, Suche und Zoom auf Seite oder Breite.',
  'product.pdf.f2':
    'Anmerkungen: markieren, unterstreichen, durchstreichen, kommentieren, zeichnen, Formen, Pfeile und Stempel.',
  'product.pdf.f3': 'Textfelder, Messungen und Bereichsausschnitte.',
  'product.pdf.f4': 'Formulare: vorhandene Felder ausfüllen oder eigene hinzufügen.',
  'product.pdf.f5': 'Signaturbereich des Dokuments.',
  'product.pdf.f6':
    'Endgültiges Schwärzen: das Markierte verschwindet aus der Datei, nicht nur aus der Ansicht.',
  'product.pdf.f7':
    'Seiten: zusammenführen, extrahieren, löschen, drehen, nummerieren (auch Bates), Kopf- und Fußzeilen.',
  'product.pdf.f8': 'Wasserzeichen, Hintergrund und bearbeitbare Metadaten.',
  'product.pdf.f9': 'Passwortgeschützte Dokumente und Berechtigungen.',
  'product.pdf.f10': 'Vorlesen und Kommentare im Strang.',
  'product.calendar.f1':
    'Monats-, Wochen-, Tages- und Agendaansicht, mit Minikalender und „Heute“.',
  'product.calendar.f2':
    'Tägliche, wöchentliche oder monatliche Wiederholung mit Ende; nur diesen, diesen und folgende oder alle bearbeiten.',
  'product.calendar.f3': 'Erinnerungen pro Termin und Benachrichtigungen in der Suite.',
  'product.calendar.f4':
    'Einladungen mit Antwort (zusagen, vielleicht, absagen) und Nachverfolgung.',
  'product.calendar.f5': '„Freien Termin finden“ unter den Teilnehmenden.',
  'product.calendar.f6': 'Buchbare Räume und Ressourcen mit Verfügbarkeit.',
  'product.calendar.f7':
    'Geteilte Kalender mit Berechtigungen, und die, die andere mit Ihnen teilen.',
  'product.calendar.f8': 'Externe Kalender abonnieren (ICS) und mehrere farbige Kalender führen.',
  'product.calendar.f9': 'Einen Termin aus einer Mail oder aus einer Notiz anlegen.',
  'product.calendar.f10':
    'Kompatibel mit jedem CalDAV-Client: Apple Kalender, Thunderbird und mehr.',
  'product.calendar.f11':
    'Dupliziere einen Termin oder kopiere ihn in deinen Kalender, und lade ihn als .ics für alle außerhalb von Sytue.',
  'product.contacts.f1': 'Adressbücher, einschließlich des gemeinsamen Ihrer Organisation.',
  'product.contacts.f2':
    'Vollständige Karte: Telefone, E-Mails, Adressen, Organisation, Geburtstag, Notizen und Foto.',
  'product.contacts.f3': 'Labels und Favoriten zum Gruppieren und Finden.',
  'product.contacts.f4': 'Duplikate erkennen und zusammenführen, ohne Daten zu verlieren.',
  'product.contacts.f5':
    'vCard und CSV importieren und exportieren; einzelnen Kontakt exportieren.',
  'product.contacts.f6': 'Stapelaktionen und Papierkorb mit Wiederherstellung.',
  'product.contacts.f7': 'Sofortsuche und Autovervollständigung beim Schreiben in Mail.',
  'product.contacts.f8': 'Kompatibel mit jedem CardDAV-Client.',
  'product.contacts.f9':
    'Bevorstehende Geburtstage aus deinem Adressbuch, mit denen dieses Monats hervorgehoben.',
  'product.contacts.f10':
    'Erstelle einen Termin mit einem Kontakt oder kopiere seine Adresse, ohne die Karte zu verlassen.',
  'product.notes.f1':
    'Verschachtelte Notizbücher mit Symbol und Farbe; drei Layouts: frei, Spalten und Liste.',
  'product.notes.f2':
    'Zwölf Notiztypen: Text, Aufgaben, Links, Farbe, Bild und Verweise auf Drive, Docus, Calendar, Rooms und andere Notizen.',
  'product.notes.f3':
    'Ein Editor mit Formatierung (fett, Listen, Links), der beim Verlassen speichert.',
  'product.notes.f4':
    'Schlagwörter, anheften, duplizieren, zwischen Notizbüchern verschieben, gruppieren und auf der Leinwand ziehen.',
  'product.notes.f5':
    'Sichtbarkeit pro Notizbuch: privat, persönlich, Organisation oder öffentlicher Link.',
  'product.notes.f6':
    'Verschlüsselung im Browser für sensible Notizbücher: Sytue kann sie nicht lesen.',
  'product.notes.f7': 'Sie entscheiden, welche Notizbücher in die Suite-Suche eingehen.',
  'product.notes.f8': 'Rückverweise: sehen, welche Notizen auf diese zeigen.',
  'product.notes.f9':
    'Ein ganzes Notizbuch als HTML, PDF, Markdown und mehr exportieren; Kopien importieren.',
  'product.notes.f10':
    'Eine Notiz in einen Rooms-Kanal teilen; ablegen oder einfügen, um Notizen anzulegen.',
  'product.notes.f11':
    'Tabellen im Editor und Markdown-Kürzel beim Schreiben: «- », «# », «> » und «```».',
  'product.notes.f12':
    'Verschiebe eine Notiz in ein anderes Notizbuch, exportiere eine einzelne Notiz und sortiere das Notizbuch nach Wunsch.',
  'product.photos.tagline':
    'Ihre Fotos, geordnet und nach Ihrem Geschmack, ohne Europa zu verlassen.',
  'product.photos.standard': 'Originale in Sytue Drive; Bearbeitungs-Engine im Browser',
  'product.photos.p1': 'Eine Zeitleiste nach Monaten mit Suche nach Namen.',
  'product.photos.p2': 'Alben mit Titelbild; Vollbildansicht.',
  'product.photos.p3':
    'Ein Editor mit Licht, Farbe, Drehen, Spiegeln und Zuschneiden, in Echtzeit.',
  'product.photos.p4': 'Jede Bearbeitung ist ein Rezept: das Original wird nie angetastet.',
  'product.photos.f1':
    'Eine nach Monaten gruppierte Zeitleiste mit Namenssuche und scharfen Miniaturen.',
  'product.photos.f2':
    'Alben: anlegen, umbenennen, Titelbild wählen, aus der Ansicht hinzufügen und entfernen.',
  'product.photos.f3': 'Vollbildansicht mit Tastaturnavigation.',
  'product.photos.f4':
    'Ein Editor mit vier Reglern (Belichtung, Kontrast, Sättigung, Temperatur), Drehen und Spiegeln.',
  'product.photos.f5': 'Zuschneiden per Ziehen und Tastenkürzel; GPU-Vorschau mit CPU-Rückfall.',
  'product.photos.f6':
    'Jede Bearbeitung ist ein separat gespeichertes Rezept: das Original in Drive wird nie verändert.',
  'product.photos.f7':
    'Export in voller Auflösung, auf dem Server mit denselben Formeln gerechnet, die Sie gesehen haben.',
  'product.photos.f8': 'Papierkorb mit Wiederherstellung.',
  'product.photos.f9':
    'Die Originale liegen in Ihrem Sytue Drive; Ableitungen sind ein regenerierbarer Cache.',
  'product.photos.f10': 'EXIF-Daten direkt aus der Datei gelesen; alles in der Europäischen Union.',
  'product.photos.f11':
    'Mehrfachauswahl nach Monat: in ein neues Album, in den Papierkorb oder auf einmal heruntergeladen.',
  'product.photos.f12':
    'Diashow, Originalgröße, Drehen im Betrachter und Zuschnitt mit festem Seitenverhältnis.',
  'product.rooms.f1':
    'Öffentliche und private Kanäle in Kategorien; Direktnachrichten und Gruppen.',
  'product.rooms.f2': 'Stränge, Reaktionen, Erwähnungen, Angepinntes, Gespeichertes und Entwürfe.',
  'product.rooms.f3':
    'Formatierung mit „Aa“-Leiste und Kurzbefehlen: fett, kursiv, Code, Zitate, Listen und Links.',
  'product.rooms.f4': 'Anhänge, Bilder, GIFs und eigene Emoji; Dateien aus Drive.',
  'product.rooms.f5': 'Zeitversetztes Senden, Priorität, Lesebestätigung und „gesehen von“.',
  'product.rooms.f6': 'Befehle mit „/“ und Karten: Umfragen und zuweisbare Aufgaben.',
  'product.rooms.f7':
    'Videoanrufe direkt im Kanal: Kamera, Bildschirm, Handzeichen, Chat und Aufnahme.',
  'product.rooms.f8': 'Geplante Besprechungen und Einladungslinks.',
  'product.rooms.f9': 'Bots, Webhooks und Integrationen mit anderen Werkzeugen.',
  'product.rooms.f10': 'Präsenz und eigener Status; Benachrichtigungen je Kanal nach Maß.',
  'product.rooms.f11': 'Suche im Verlauf und aus der ganzen Suite.',
  'product.rooms.f12': 'Echtzeit über WebSocket, mit Wiederverbindung ohne Nachrichtenverlust.',
  'product.rooms.f13':
    'Schneller Kanalwechsel mit Strg+G, alles als gelesen markieren und Kürzel mit «?».',
  'product.cast.tagline':
    'Ihre Videos und Ihre Livestreams, auf Abruf und in Echtzeit, ohne Europa zu verlassen.',
  'product.cast.standard': 'HLS-Wiedergabe im Browser; RTMP-Eingang aus jeder Sendesoftware',
  'product.cast.p1':
    'Ein Video hochladen, und es wird in mehreren Qualitäten für jeden Bildschirm umgewandelt.',
  'product.cast.p2': 'Live gehen aus einer Sendesoftware, aus dem Browser oder aus der App.',
  'product.cast.p3':
    'Vier Sichtbarkeitsstufen: öffentlich, ungelistet, Organisation oder bestimmte Personen.',
  'product.cast.p4':
    'Live-Chat und Kommentare, mit einfacher Moderation durch die veröffentlichende Person.',
  'product.cast.f1': 'Videos bis 8 GiB hochladen; das Original bleibt erhalten.',
  'product.cast.f2': 'Adaptive Wiedergabe in drei Qualitäten; der Browser wählt die beste.',
  'product.cast.f3':
    'Livestreaming aus einer Sendesoftware über RTMP, mit einem Streamschlüssel pro Person.',
  'product.cast.f4':
    'Live aus dem Browser oder der App: Kamera und Mikrofon. Bildschirmfreigabe in unterstützten Browsern, Desktopsystemen und Android; nicht in der iOS/iPadOS-App verfügbar.',
  'product.cast.f5': 'Aufzeichnungen von Livestreams werden automatisch zu Videos auf Abruf.',
  'product.cast.f6': 'Eine öffentliche Startseite und ein Kanal pro Organisation, indexierbar.',
  'product.cast.f7':
    'Ungelistete Links, Videos nur für die Organisation oder Zugriff für bestimmte Personen per @Benutzername.',
  'product.cast.f8':
    'Live-Chat und Kommentare in Threads, moderiert von der veröffentlichenden Person und der Organisationsverwaltung.',
  'product.cast.f9': 'Einbettbarer Player und oEmbed für öffentliche und ungelistete Videos.',
  'product.cast.f10':
    'Der Speicher zählt zu Ihrem Kontingent in Sytue Drive; alles in der Europäischen Union.',
  'product.cast.f11':
    '„In Cast hochladen“ aus Sytue Drive: Eine Kopie des Videos geht an Cast, die Datei bleibt in Drive.',
  'product.pages.tagline':
    'Die Website Ihres Unternehmens und sein Shop, gestaltet in einem visuellen Editor und veröffentlicht, ohne Europa zu verlassen.',
  'product.pages.standard':
    'Statisches, barrierefreies HTML und CSS ohne Cookies von Dritten; Export nach Astro',
  'product.pages.p1':
    'Ein visueller Editor mit Rahmen, die sich von selbst an Desktop, Tablet und Smartphone anpassen.',
  'product.pages.p2': 'Zehn Vorlagen nach Branche, mit echten Texten und gemeinfreien Fotos.',
  'product.pages.p3':
    'Ein Shop mit Katalog, Warenkorb und Zahlung auf das Konto Ihres Unternehmens.',
  'product.pages.p4':
    'Ihre eigene Domain mit automatischem Zertifikat oder eine Adresse unter sites.sytue.eu.',
  'product.pages.f1':
    'Visueller Editor mit Rahmen, automatischem Layout, wiederverwendbaren Komponenten und fertigen Abschnitten.',
  'product.pages.f2':
    'Drei Breakpoints (Desktop, Tablet und Smartphone), jeder mit eigenen Anpassungen.',
  'product.pages.f3': 'Besucheranalyse ohne Cookies, direkt im Dashboard der Website.',
  'product.pages.f4':
    'Mehrsprachige Websites mit Adressen pro Sprache, hreflang und Übersetzung mit Unterstützung von Sytue AI.',
  'product.pages.f5':
    'Bilder von Ihrem Rechner, aus Sytue Drive oder Sytue Photos, wobei beim Einfügen ein Alternativtext verlangt wird.',
  'product.pages.f6':
    'Shop mit Varianten, Lagerbestand, Umsatzsteuer je Produkt und Zahlung über Mollie, Adyen, Redsys oder Stripe.',
  'product.pages.f7':
    'Bestellungen mit einer von Sytue ERP ausgestellten vereinfachten Rechnung und Widerruf direkt auf der Website.',
  'product.pages.f8':
    'Formulare mit Einwilligung, ein Nachrichteneingang und Benachrichtigungen an die Personen Ihrer Organisation.',
  'product.pages.f9':
    'Benannte Versionen, eine Vorschau hinter einem geheimen Link und Rückkehr zu jeder früheren Veröffentlichung.',
  'product.pages.f10':
    'Code-Export nach Astro, React, Next.js und in native Apps sowie Katalogexport zu Shopify, WooCommerce und anderen Plattformen.',
  'product.bases.tagline':
    'Verknüpfte Tabellen für Kunden, Bestellungen oder Bestände, mit Berechtigungen und Protokoll jeder Änderung.',
  'product.bases.standard':
    'Offene Formate CSV und JSON; Schema und Beziehungen von der Datenbank selbst gesichert',
  'product.bases.p1':
    'Kunden, Bestellungen oder Bestände in verknüpften Tabellen, ganz ohne Datenbankkenntnisse.',
  'product.bases.p2': 'Teilen Sie mit Ihrer Organisation in der Rolle, die Sie festlegen.',
  'product.bases.p3':
    'Jede Änderung wird protokolliert: wer sie vorgenommen hat und was vorher galt.',
  'product.bases.p4': 'Ihre Daten bleiben in der EU, und Sie exportieren sie, wann Sie wollen.',
  'product.bases.f1':
    'Tabellen mit Feldern für Text, Zahl, Datum, Kontrollkästchen sowie Einzel- und Mehrfachauswahl.',
  'product.bases.f2':
    'Beziehungen zwischen Tabellen: Verknüpfen Sie eine Bestellung mit ihrem Kunden, und die Datenbank verhindert, dass die Verknüpfung bricht.',
  'product.bases.f3':
    'Bearbeitbares Raster mit Tastatur, für Screenreader lesbar und auf dem Smartphone nutzbar.',
  'product.bases.f4':
    'Berechtigungen pro Datenbank und pro Tabelle: Eigentümer, Bearbeiter, Kommentator und Leser.',
  'product.bases.f5': 'CSV-Datei als neue Tabelle importieren und als CSV oder JSON exportieren.',
  'product.bases.f6':
    'Änderungsprotokoll: wer, welcher Datensatz und welches Feld, vorher und nachher.',
  'product.cloud.f12': 'Nutzung, Kosten und europäische Abrechnung in derselben Konsole.',
  'product.cloud.f11': 'Mitglieder, API-Token und exportierbares Audit.',
  'product.cloud.f10': 'Datenschutzfreundliche Web-Analyse, ohne Cookies.',
  'product.cloud.f9': 'Geplante Aufgaben und Projektvorlagen.',
  'product.cloud.f8': 'Terminal im Container; Dienst neu starten oder pausieren.',
  'product.cloud.f7': 'Live-Logs, Metriken und Überwachung mit Alarmen.',
  'product.cloud.f6': 'Umgebungsvariablen und Geheimnisse je Umgebung.',
  'product.cloud.f5': 'Eigene Domains mit automatischem TLS; Weiterleitungen und Header.',
  'product.cloud.f4': 'S3-Speicher und verwaltete Funktionen.',
  'product.cloud.f3': 'Postgres-Datenbanken mit automatischer REST-API und SQL-Editor.',
  'product.cloud.f2': 'Eine Vorschau pro PR und Rückkehr zu jedem Deployment mit einem Klick.',
  'product.cloud.f1':
    'Projekte aus Ihrem Repository (GitHub, GitLab oder eigenes Git) mit Deployment bei jedem Push.',
  'product.console.f1':
    'Ein Dashboard mit dem Zustand der Organisation und dem, was heute einen Blick verdient.',
  'product.console.f2':
    'Konten: anlegen, sperren, Passwort und MFA zurücksetzen, Sitzungen, Aliasse und Postfach.',
  'product.console.f3':
    'Statische und regelbasierte dynamische Gruppen; Rollen mit Berechtigungen.',
  'product.console.f4':
    'Privilegierter Zugriff (PIM): zeitlich begrenzte Aktivierung mit Genehmigung.',
  'product.console.f5':
    'SSO mit SAML- oder OIDC-Anbietern, mit Verbindungstest; SCIM-Provisionierung.',
  'product.console.f6': 'Regeln für bedingten Zugriff und Inventar vertrauenswürdiger Geräte.',
  'product.console.f7':
    'Integrationen: OIDC-Clients, SAML und Verbindungen zu Dritten; Produktersetzung.',
  'product.console.f8': 'Inhalte mitbringen: Migration von anderen Anbietern.',
  'product.console.f9': 'Dokumenteditor und Rooms-Kanäle, die sich verwalten lassen.',
  'product.console.f10': 'Governance: Legal Holds, Vertraulichkeitslabels und Passwortrichtlinie.',
  'product.console.f11': 'Audit mit CSV-Export und Weiterleitung an Ihr SIEM; Dienstprotokolle.',
  'product.console.f12': 'Eigene Domain und Marke: Logo der Organisation und Mail-Domain.',
  'product.id.f1':
    'Single Sign-on (OpenID Connect und OAuth 2.0 mit PKCE) für die Suite und Ihre Anwendungen.',
  'product.id.f2': 'Passkeys (WebAuthn) und TOTP-MFA mit Wiederherstellungscodes.',
  'product.id.f3': 'App-Passwörter für IMAP und Clients, die kein OIDC sprechen.',
  'product.id.f4': 'Sitzungen und Geräte: eine oder alle anderen widerrufen.',
  'product.id.f5': 'SAML-Föderation mit Ihrem IdP, und Sytue als SAML-IdP für andere Anwendungen.',
  'product.id.f6': 'SCIM-Provisionierung aus Ihrem Verzeichnis.',
  'product.id.f7': 'Einladungen, E-Mail-Bestätigung und Passwortwiederherstellung.',
  'product.id.f8': 'Profil für iPhone und Mac (.mobileconfig) mit einem Tipp.',
  'product.id.f9': 'Passwortrichtlinien und Zugriffsregeln, die die Organisation festlegt.',
  'product.id.f10': 'Dynamische Client-Registrierung (DCR) und Standard-Discovery.',
  'product.erp.f1':
    'Vierundzwanzig Module und sechsunddreißig Modelle in einem Dienst und einer Datenbank.',
  'product.erp.f2':
    'Der Modellkatalog ist Datum: Jedes Modul deklariert seine Felder, Zustände und Listenspalten.',
  'product.erp.f3':
    'Ein neues Modul ist eine JSON-Datei – kein DDL, keine Migration, keine Tabellensperre.',
  'product.erp.f4':
    'Eine API für alle sechsunddreißig Modelle und eine aus dem Katalog erzeugte Oberfläche.',
  'product.erp.f5':
    'Doppelte Buchführung mit Beträgen in ganzen Cent; keine einzige Gleitkommazahl.',
  'product.erp.f6': 'Den Ausgleich prüft die Datenbank beim Commit, nicht der Anwendungscode.',
  'product.erp.f7':
    'Abgeschlossene Buchungen sind unveränderlich: kein Ändern, kein Löschen, keine neuen Zeilen.',
  'product.erp.f8':
    'Ein Beleg, eine Buchung: ein Wiederholversuch oder zwei Tabs können Erlöse nicht verdoppeln.',
  'product.erp.f9':
    'Gemeinsame Bezeichnungen liegen in 26 Sprachen vor; ein unübersetztes Feld verhindert den Start.',
  'product.erp.f10':
    'Listen über Index: Die erste Seite von zwanzigtausend Zeilen kommt in unter einer Millisekunde.',

  // --- Descargas ----------------------------------------------------------
  'meta.downloads.title': 'Downloads — Sytue One für Desktop und Mobil',
  'meta.downloads.description':
    'Native Sytue-One-Apps für macOS, Windows, Linux, iOS und Android. Die ganze Suite auf Ihrem Gerät, mit den Integrationen des jeweiligen Systems.',
  'downloads.overline': 'Apps',
  'downloads.h1': 'Sytue One auf Ihrem Gerät',
  'downloads.intro':
    'Echte Desktop- und Mobil-Anwendungen: Sie werden installiert, liegen im Dock oder im Startmenü, melden sich über die Systembenachrichtigungen und öffnen Dateien von Ihrem Gerät.',
  'downloads.desktop.title': 'Desktop',
  'downloads.desktop.body':
    'Eine native Anwendung pro Produkt, die ihre Oberfläche selbst über die Grafikkarte zeichnet: kein Browser im Inneren, deshalb startet sie schnell und verbraucht so wenig wie eine Systemanwendung.',
  'downloads.apps.title': 'Nur was du brauchst',
  'downloads.apps.body':
    'Sytue One bringt den ganzen Arbeitstag mit. Wer nur den Speicher will, installiert Sytue Drive einzeln.',
  'downloads.includes': 'Enthält:',
  'downloads.mobile.title': 'Mobil',
  'downloads.mobile.body':
    'Native iOS- und Android-Anwendung, mit Systembenachrichtigungen, Teilen sowie Zugriff auf Dateien und Kamera.',
  'downloads.selfhost.title': 'Läuft Ihre Organisation auf einer eigenen Domain?',
  'downloads.selfhost.body':
    'Die nativen Anwendungen verbinden sich mit sytue.eu. Betreibt Ihre Organisation eine eigene Installation, melden Sie sich im Browser auf deren Domain an: Die Suite funktioniert im Web genauso.',
  'downloads.yourSystem': 'Ihr System',
  'downloads.signInToGet': 'Zum Herunterladen anmelden',
  // Sin Präposition: `{format}` es «App Store» o
  // «Google Play», y en alemán cada tienda pide una distinta —«im App Store» pero
  // «bei Google Play»—, así que ninguna plantilla con preposición vale para las dos.
  'downloads.store': 'Ansehen · {format}',
  'downloads.soon': 'Demnächst · {format}',
  'downloads.browser.text':
    'Lieber im Browser? Die ganze Suite funktioniert im Web genauso, ganz ohne Installation:',
  'downloads.browser.link': 'Sytue One öffnen',
  'platform.macos.req': 'macOS 11 oder neuer · Apple Silicon und Intel',
  'platform.windows.req': 'Windows 10 oder neuer · 64 Bit',
  'platform.linux.req': 'OpenGL 2.1 · 64 Bit',
  'platform.ios.req': 'iOS 14 oder neuer',
  'platform.android.req': 'Android 6.0 oder neuer',

  // --- Pie ----------------------------------------------------------------
  'footer.tagline':
    'Jede App der Suite ist ein eigener Dienst, gebaut auf offenen, veröffentlichten Protokollen.',
  'footer.products': 'Produkte',
  'footer.resources': 'Ressourcen',
  'footer.legal': 'Rechtliches',
  'footer.suite': 'Die Suite',
  'footer.downloads': 'Apps herunterladen',
  'footer.sales': 'Mit dem Vertrieb sprechen',
  'footer.privacy': 'Datenschutz & DSGVO',
  'footer.accessibility': 'Barrierefreiheit',
  'footer.openSource': 'Open Source',
  'footer.legalNotice': 'Impressum',
  'footer.terms': 'Nutzungsbedingungen',
  'footer.copyright':
    '© {year} Sytue · Daten in der EU gehostet (Scaleway fr-par / nl-ams · On-Premise Madrid).',
  'footer.privacyNotice':
    'Wir verkaufen Ihre Daten nicht und geben sie nicht an Dritte weiter. Wir nutzen sie weder für Profilbildung noch für Werbung, noch um unsere Dienste oder die anderer zu trainieren oder zu verbessern. Sie sind bei der Übertragung und im Ruhezustand verschlüsselt und verlassen unsere europäische Infrastruktur nur, soweit es für die Abrechnung unbedingt nötig ist, um Sie bei geschlossener App zu benachrichtigen (der Push-Dienst sieht nur Ihr Gerät und die Uhrzeit, nie den Inhalt), und in den Integrationen, die Sie aktivieren – dort bittet Sie der Drittanbieter um Zustimmung zu den verwendeten Daten.',

  // --- 404 ----------------------------------------------------------------
  'meta.notFound.title': 'Seite nicht gefunden — Sytue One',
  'notFound.overline': 'Fehler 404',
  'notFound.title': 'Diese Seite haben wir nicht gefunden',
  'notFound.body':
    'Vielleicht hat sich der Link geändert. Zurück zur Startseite oder die Suite erkunden.',
  'notFound.home': 'Zur Startseite',

  // --- Legal: privacidad ---------------------------------------------------
  'meta.legal.title': 'Impressum — Sytue One',
  'meta.legal.description':
    'Angaben zum Betreiber von www.sytue.eu und des Dienstes Sytue One: Sytue Projects, S.L.',
  'aviso.title': 'Impressum',
  'aviso.intro':
    'Gemäß Artikel 10 des spanischen Gesetzes 34/2002 vom 11. Juli über Dienste der Informationsgesellschaft und den elektronischen Geschäftsverkehr (LSSI) sind dies die Angaben zum Betreiber dieser Website und des Dienstes Sytue One.',
  'aviso.review.body':
    'Die Registerangaben in diesem Impressum sind im Handelsregister geprüft. Der übrige Text befindet sich in rechtlicher Prüfung; seine Formulierung kann sich ändern, ohne dass sich das Beschriebene ändert.',
  'aviso.owner.title': 'Betreiber',
  'aviso.owner.name': 'Firma',
  'aviso.owner.form': 'Rechtsform',
  'aviso.owner.formValue':
    'Spanische Einpersonen-Gesellschaft mit beschränkter Haftung (sociedad limitada unipersonal)',
  'aviso.owner.nif': 'Steuernummer (NIF)',
  'aviso.owner.address': 'Sitz',
  'aviso.owner.registry': 'Registereintrag',
  'aviso.owner.registryValue':
    'Handelsregister Madrid (Registro Mercantil de Madrid), Abschnitt 8, Blatt M-887153, Eintragung 1',
  'aviso.owner.email': 'E-Mail',
  'aviso.owner.web': 'Website',
  'aviso.scope.title': 'Gegenstand',
  'aviso.scope.body':
    'Diese Website informiert über die Suite Sytue One und gibt Zugang zu ihren Anwendungen. Die Nutzung des Dienstes richtet sich nach den Nutzungsbedingungen, die Verarbeitung personenbezogener Daten nach der Datenschutzerklärung; beide sind in der Fußzeile jeder Seite verlinkt.',
  'aviso.ip.title': 'Geistiges und gewerbliches Eigentum',
  'aviso.ip.body':
    'Die Inhalte dieser Website (Texte, Gestaltung, Code sowie die Marken und Logos Sytue und Sytue One) gehören Sytue Projects, S.L. oder werden unter Lizenz genutzt. Produktnamen Dritter, die als Kompatibilitätshinweis erscheinen, sind Marken ihrer Inhaber.',
  'aviso.liability.title': 'Haftung',
  'aviso.liability.body':
    'Sytue Projects, S.L. bemüht sich, diese Website verfügbar und aktuell zu halten, ohne Unterbrechungs- oder Fehlerfreiheit zu garantieren. Für Inhalte verlinkter Websites Dritter wird keine Verantwortung übernommen.',
  'aviso.law.title': 'Anwendbares Recht',
  'aviso.law.body':
    'Für dieses Impressum gilt spanisches Recht. Verbraucherinnen und Verbraucher behalten die Rechte und den Gerichtsstand, die ihnen das Recht ihres Wohnsitzlandes in der Europäischen Union einräumt.',
  'aviso.updated': 'Zuletzt aktualisiert: September 2026.',
  'meta.accessibility.title': 'Erklärung zur Barrierefreiheit — Sytue One',
  'meta.accessibility.description':
    'Wie barrierefrei die Suite Sytue One und diese Website sind, was wir messen, was noch fehlt und wie Sie es uns melden.',
  'a11y.title': 'Erklärung zur Barrierefreiheit',
  'a11y.intro':
    'Sytue möchte, dass jede Person Sytue One und diese Website nutzen kann, mit welcher Hilfstechnik auch immer. Diese Erklärung gilt für www.sytue.eu und die Webanwendungen der Suite.',
  'a11y.standard.title': 'Maßstab',
  'a11y.standard.body':
    'Wir gestalten und bauen nach WCAG 2.1 Stufe AA, der Stufe, auf die die europäische Norm EN 301 549 und der European Accessibility Act (Richtlinie (EU) 2019/882) verweisen.',
  'a11y.status.title': 'Stand der Konformität',
  'a11y.status.body':
    'Teilweise konform: Der größte Teil der Suite erfüllt die Norm, die unten genannten Teile noch nicht vollständig. Ein unabhängiges Audit haben wir noch nicht beauftragt.',
  'a11y.measures.title': 'Was wir dafür tun',
  'a11y.measures.design':
    'Ein Designsystem für die ganze Suite: Textfarben mit geprüftem Kontrast, ein sichtbarer Fokusring an jedem Bedienelement und Tastaturbedienung von Menüs, Dialogen und Listen.',
  'a11y.measures.ci':
    'Automatisierte Barrierefreiheitsprüfungen (Lighthouse) in der Integrationskette bei jeder Änderung der öffentlichen Website.',
  'a11y.measures.system':
    'Die Oberfläche folgt Ihren Systemeinstellungen: dunkles oder helles Thema, reduzierte Bewegung und Textgröße, ohne feste Pixelgrößen in Formularfeldern.',
  'a11y.measures.text':
    'Jedes Bild, Symbol und jeder leere Zustand hat eine Textalternative, und jede Seite deklariert ihre Sprache, damit Screenreader sie richtig aussprechen.',
  'a11y.limits.title': 'Bekannte Einschränkungen',
  'a11y.limits.body':
    'Das Tabellenraster, die Folienfläche und das Kommentieren von PDFs sind komplexe Editoren, an denen noch gearbeitet wird, damit sie vollständig mit Tastatur und Screenreader bedienbar sind. Von Personen hochgeladene Dokumente und Dateien werden unverändert angezeigt und von uns nicht nachgebessert.',
  'a11y.feedback.title': 'Sagen Sie uns, was nicht funktioniert',
  'a11y.feedback.body':
    'Wenn Ihnen etwas in der Suite oder auf dieser Website schwer- oder unmöglich zu nutzen ist, schreiben Sie uns: Wir prüfen es und antworten Ihnen.',
  'a11y.enforcement.title': 'Wenn Sie nicht zufrieden sind',
  'a11y.enforcement.body':
    'Wenn Sie eine Barriere gemeldet haben und mit unserer Antwort nicht zufrieden sind, können Sie sich nach dem European Accessibility Act an die für Barrierefreiheit zuständige Stelle Ihres Landes wenden.',
  'a11y.updated':
    'Diese Erklärung wurde im September 2026 verfasst und wird überprüft, wenn sich die Suite wesentlich ändert.',
  'meta.openSource.title': 'Open Source — Sytue One',
  'meta.openSource.description':
    'Sytue baut fast alles von Grund auf neu, erfindet aber nicht neu, was bereits gut funktioniert. Die Open-Source-Projekte, auf die sich die Suite stützt.',
  'oss.title': 'Open Source',
  'oss.intro':
    'Sytue baut fast alles von Grund auf neu: die E-Mail, die Editoren, den Kalender. Aber es erfindet nicht neu, was bereits gut funktioniert. Das sind die Open-Source-Projekte, auf die sich einige Teile der Suite stützen.',
  'oss.livekit.body': 'Echtzeit-Audio und -Video der Anrufe in Sytue Rooms laufen über LiveKit.',
  'oss.postgres.body':
    'Jeder Dienst der Suite speichert seine Daten in PostgreSQL, eine Datenbank pro Organisation.',
  'meta.privacy.title': 'Datenschutz & DSGVO — Sytue One',
  'meta.privacy.description':
    'Wie Sytue One personenbezogene Daten verarbeitet: Verantwortlicher, Daten, Rechtsgrundlagen, Speicherdauer, Auftragsverarbeiter, Sicherheit, internationale Übermittlungen und Ihre Rechte nach der DSGVO.',
  'legal.overline': 'Rechtliches',
  'legal.updated': 'Zuletzt aktualisiert: Juli 2026.',
  'legal.review.title': 'In rechtlicher Prüfung.',
  'privacy.title': 'Datenschutz & DSGVO',
  'privacy.review.body':
    'Diese Erklärung beschreibt genau, wie wir Daten heute verarbeiten. Die konkreten Speicherfristen werden mit der endgültigen, von unserem Rechtsteam geprüften Fassung ergänzt.',
  'privacy.controller.title': 'Verantwortlicher',
  'privacy.controller.body':
    'Sytue Projects, S.L. (spanische Steuernummer B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spanien; eingetragen im Handelsregister Madrid, Abschnitt 8, Blatt M-887153) ist als Anbieterin der Suite Sytue One die Verantwortliche für die Verarbeitung der Daten der Konten und Organisationen des Dienstes. Kontakt in Datenschutzfragen:',
  'privacy.data.title': 'Welche Daten wir verarbeiten',
  'privacy.data.account.label': 'Kontodaten:',
  'privacy.data.account.body':
    'Name, E-Mail-Adresse und Organisation, verwaltet mit Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Inhalte Ihrer Organisation:',
  'privacy.data.content.body':
    'E-Mails, Dateien, Dokumente, Kalender und Nachrichten, die Ihre Organisation in der Suite hostet. Diese Inhalte gehören Ihrer Organisation.',
  'privacy.data.technical.label': 'Technische Betriebsdaten:',
  'privacy.data.technical.body':
    'Zugriffsprotokolle und Verbindungsdaten, die für Sicherheit, Missbrauchsprävention und den Betrieb des Dienstes erforderlich sind.',
  'privacy.data.note':
    'Wir verkaufen keine Daten und nutzen sie nicht für Werbung. Die von Sytue verwaltete Infrastruktur und Speicherung befinden sich in der Europäischen Union (Scaleway, Regionen fr-par und nl-ams sowie eine On-Premise-Zone in Madrid), mit Verschlüsselung bei der Übertragung (TLS 1.3) und im Ruhezustand. Verbindungen zu externen Diensten senden die von dir freigegebenen Daten an deren Anbieter, gemäß dessen Standort und Bedingungen.',
  'privacy.basis.title': 'Rechtsgrundlage',
  'privacy.basis.body':
    'Die Verarbeitungen stützen sich auf die Rechtsgrundlagen des Artikels 6 DSGVO: die Erfüllung des Dienstvertrags mit Ihrer Organisation, das berechtigte Interesse an Sicherheit und Missbrauchsprävention, die Erfüllung rechtlicher Verpflichtungen und — soweit ausdrücklich eingeholt — Ihre Einwilligung. Die Aufschlüsselung je Rechtsgrundlage und je Verarbeitung wird Teil der endgültigen Fassung.',
  'privacy.retention.title': 'Speicherdauer',
  'privacy.retention.body':
    'Wir speichern Kontodaten und Inhalte, solange Ihre Organisation den Dienst aktiv hält. Wird ein Konto oder eine Organisation gekündigt, werden die Daten innerhalb einer angemessenen Frist gelöscht oder anonymisiert — mit Ausnahme dessen, was wir aufgrund gesetzlicher Pflichten aufbewahren müssen (etwa zur Rechnungsstellung). Technische Betriebsprotokolle werden regelmäßig bereinigt. Die konkreten Fristen je Kategorie enthält die endgültige Fassung. Meldungen zu Inhalten werden aufbewahrt, solange sie offen sind, und zwölf Monate nach ihrer Erledigung gelöscht.',
  'privacy.processors.title': 'Auftragsverarbeiter und Unterauftragsverarbeiter',
  'privacy.processors.body':
    'Unser Infrastrukturanbieter ist Scaleway (Regionen fr-par und nl-ams), der als Auftragsverarbeiter handelt und ausschließlich in der Europäischen Union tätig ist; zusätzlich betreiben wir eine On-Premise-Zone in Madrid (Spanien) in eigenen Räumen. Für die Abrechnung der Abonnements nutzen wir Mollie B.V. (Niederlande), die ausschließlich die für die Zahlung erforderlichen Daten verarbeitet. Für die SMS mit dem Code, der bei der Kontoerstellung die Handynummer bestätigt, nutzen wir Sweego (Mindbaz SAS, Frankreich), das ausschließlich diese Nummer und den Code erhält. Wir setzen keine Unterauftragsverarbeiter außerhalb des EWR ein. Die Liste der Auftragsverarbeiter wird veröffentlicht und aktuell gehalten, mit vorheriger Information über jede Aufnahme.',
  'privacy.security.title': 'Sicherheit',
  'privacy.security.body':
    'Wir setzen technische und organisatorische Maßnahmen nach der DSGVO um: Verschlüsselung bei der Übertragung (TLS 1.3) und im Ruhezustand, Trennung zwischen Organisationen (mandantenfähig mit Sicherheit auf Zeilenebene), starke Authentifizierung (MFA/WebAuthn), Zugriffsprotokollierung und das Prinzip der geringsten Rechte. Kein System ist unfehlbar: Sollte eine Verletzung Sie betreffen, informieren wir Sie innerhalb der von der Verordnung vorgesehenen Fristen.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'Wir verwenden ausschließlich technisch notwendige Cookies für Single Sign-on und Sitzungssicherheit. Wir setzen keine Werbe- oder Tracking-Cookies Dritter ein — deshalb gibt es bei uns auch kein Einwilligungsbanner für Tracking.',
  'privacy.transfers.title': 'Internationale Übermittlungen',
  'privacy.transfers.body':
    'Die von Sytue verwaltete Verarbeitung und das Hosting finden in Regionen der Europäischen Union statt. Vom Nutzer aktivierte externe Verbindungen senden freigegebene Daten an deren Anbieter; ihr Standort hängt vom Anbieter und seiner Konfiguration ab. Benachrichtigungen bei geschlossener App oder geschlossenem Browser können ebenfalls externe Dienste nutzen. Der Push-Dienst Ihres Geräts (Google, Mozilla oder Apple, je nach Browser oder Telefon) sieht eine anonyme Kennung des Geräts und den Zeitpunkt jeder Benachrichtigung – nie den Inhalt, der Ende-zu-Ende verschlüsselt übertragen wird und unter iOS ganz leer bleibt. Sie können Benachrichtigungen abschalten, dann verlässt auch das nichts mehr.',
  'privacy.rights.title': 'Ihre Rechte',
  'privacy.rights.body1':
    'Die DSGVO gibt Ihnen das Recht auf Auskunft, Berichtigung, Löschung, Widerspruch, Einschränkung der Verarbeitung und Datenübertragbarkeit. Sie können diese Rechte ausüben, indem Sie schreiben an',
  // «Recht auf Beschwerde bei …» en vez de «das Recht, sich bei … zu beschweren»:
  // es la fórmula del propio art. 77 DSGVO y deja la autoridad AL FINAL de la frase,
  // que es donde va el enlace. Con el infinitivo al final, el enlace quedaba partido.
  'privacy.rights.body2':
    '; wir antworten innerhalb der von der Verordnung vorgesehenen Fristen. Sie haben zudem das Recht auf Beschwerde bei',
  'privacy.rights.authority': 'Ihrer Aufsichtsbehörde',
  'privacy.dpo.title': 'Datenschutzbeauftragter',
  'privacy.dpo.body':
    'Für Fragen zum Datenschutz oder für den Kontakt zu unserem Datenschutzbeauftragten:',
  'privacy.minors.title': 'Minderjährige',
  'privacy.minors.body':
    'Sytue One ist ein Dienst für Organisationen und die Menschen, die dort arbeiten; er richtet sich nicht an Minderjährige, und wir erheben wissentlich keine Daten von Minderjährigen.',
  'privacy.changes.title': 'Änderungen dieser Erklärung',
  'privacy.changes.body':
    'Wenn wir diese Erklärung aktualisieren, ändern wir das Datum der „letzten Aktualisierung“ und informieren Sie bei wesentlichen Änderungen über die Kanäle des Dienstes, bevor sie wirksam werden.',

  // --- Legal: términos -----------------------------------------------------
  'meta.terms.title': 'Nutzungsbedingungen — Sytue One',
  'meta.terms.description':
    'Nutzungsbedingungen von Sytue One: Anbieter, Gegenstand, Konten und Organisationen, zulässige Nutzung, Verfügbarkeit, Ihre Daten, geistiges Eigentum und anwendbares Recht.',
  'terms.title': 'Nutzungsbedingungen',
  'terms.review.body':
    'Diese Bedingungen beschreiben den Dienst und seine Nutzung genau so, wie sie heute funktionieren. Die konkreten Vertragsklauseln (Service-Level, Haftung und Gerichtsstand) werden mit der endgültigen, von unserem Rechtsteam geprüften Fassung ergänzt.',
  'terms.provider.title': 'Wer den Dienst anbietet',
  'terms.provider.body':
    'Sytue One wird von Sytue Projects, S.L. (spanische Steuernummer B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Spanien; eingetragen im Handelsregister Madrid, Abschnitt 8, Blatt M-887153) aus einer Infrastruktur erbracht, die sich ausschließlich in der Europäischen Union befindet (Scaleway, Regionen fr-par und nl-ams, sowie eine On-Premise-Zone in Madrid), und unterliegt dem Recht der EU.',
  'terms.scope.title': 'Gegenstand des Dienstes',
  'terms.scope.body':
    'Sytue One ist eine Produktivitäts-Suite für Organisationen: Identität (Sytue ID), E-Mail, Dateien, Dokumente, Kalender, Kontakte und Messaging mit Videoanrufen. Der Dienst wird je Organisation abgeschlossen.',
  'terms.accounts.title': 'Konten und Organisationen',
  'terms.accounts.body':
    'Jede Organisation verwaltet ihre Konten, Gruppen und Berechtigungen über Sytue ID (SSO + MFA). Die Administration der Organisation ist für Anlage und Löschung von Konten sowie für die Nutzung des Dienstes über diese Konten verantwortlich.',
  'terms.use.title': 'Zulässige Nutzung',
  'terms.use.body':
    'Sie verpflichten sich, den Dienst rechtskonform zu nutzen und ihn nicht für rechtswidrige Aktivitäten, Spam-Versand, die Verbreitung von Schadsoftware, die Verletzung von Rechten Dritter oder eine Nutzung einzusetzen, die Sicherheit oder Verfügbarkeit des Dienstes für andere Organisationen beeinträchtigt.',
  'terms.availability.title': 'Verfügbarkeit und Support',
  'terms.availability.body1':
    'Wir arbeiten an einer hohen Verfügbarkeit und führen Wartung und Verbesserungen fortlaufend durch. Konkrete Service-Level-Zusagen (SLA) werden, soweit sie für Ihren Tarif gelten, im Servicevertrag festgelegt. Der Support erfolgt über',
  'terms.data.title': 'Ihre Daten',
  'terms.data.body1':
    'Die Inhalte, die Ihre Organisation in der Suite hostet, gehören Ihrer Organisation. Sie beruhen auf offenen Standards (OIDC, JMAP, CalDAV, CardDAV, WebDAV), sodass Ihre Daten stets portabel bleiben und Sie sie mitnehmen können. Die Verarbeitung personenbezogener Daten ist beschrieben auf der Seite',
  'terms.ip.title': 'Geistiges Eigentum',
  'terms.ip.body':
    'Die Software von Sytue One ist eine Eigenentwicklung von Sytue und durch die entsprechenden Schutzrechte geschützt. Sie behalten die Rechte an den Inhalten, die Ihre Organisation in der Suite hostet; wir verarbeiten sie ausschließlich zur Erbringung des Dienstes.',
  'terms.liability.title': 'Haftung',
  'terms.liability.body':
    'Wir setzen alles daran, einen zuverlässigen und sicheren Dienst zu bieten. Die geltenden Gewährleistungen und Haftungsbeschränkungen werden im Servicevertrag festgelegt; nichts in diesen Bedingungen schließt eine Haftung aus, die nach dem Gesetz nicht ausgeschlossen werden darf.',
  'terms.contact.title': 'Vertragsabschluss und Kontakt',
  'terms.contact.body1':
    'Für Vertragsbedingungen, Preise je Organisation oder jede Frage zum Dienst schreiben Sie uns an',
  'terms.changes.title': 'Änderungen',
  'terms.changes.body':
    'Wir können diese Bedingungen anpassen, um Änderungen am Dienst oder an der Rechtslage abzubilden. Bei wesentlichen Änderungen informieren wir Sie über die Kanäle des Dienstes, bevor sie wirksam werden, und aktualisieren das Datum der „letzten Aktualisierung“.',
  'terms.law.title': 'Anwendbares Recht',
  'terms.law.body':
    'Der Dienst unterliegt dem Recht der Europäischen Union und den Vorschriften des Sitzlandes des Anbieters. Der konkrete Gerichtsstand für Streitigkeiten wird in der endgültigen Fassung festgelegt. Unabhängig davon behalten Sie Ihr Recht, sich bei Ihrer Datenschutzaufsichtsbehörde zu beschweren.',

  // --- Datos estructurados (JSON-LD) --------------------------------------
  'seo.org.description':
    'Sytue One: die souveräne europäische Produktivitäts-Suite. Identität, E-Mail, Dateien, Dokumente, Kalender, Chat, Videoanrufe und eine PaaS — zu 100 % in der EU.',
  'seo.org.slogan': 'Ihre Produktivität, souverän und europäisch.',
  'seo.software.description':
    'Souveräne europäische Produktivitäts-Suite: E-Mail (JMAP), Dateien (WebDAV), kollaborative Dokumente, Kalender (CalDAV), Kontakte (CardDAV), Chat, Videoanrufe und PaaS. SSO mit Sytue ID.',
  'seo.software.f1': 'Single Sign-on (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'E-Mail, Dateien, Dokumente, Kalender, Kontakte, Chat und Videoanrufe',
  'seo.software.f3': 'Plattform für Deployments und verwaltete Daten',
  'seo.software.f4': 'Daten zu 100 % in der EU (Scaleway)',
  'seo.offer.description': 'Kostenlos testen, ohne Kreditkarte.',
  'seo.downloads.description':
    'Native Apps der Suite Sytue One für Desktop (macOS, Windows, Linux) und Mobil (iOS, Android).',

  // --- Precios: cuenta personal ---
  'home.pricing.personal.title': 'Kostenlos',
  'home.pricing.personal.freeStorage': '5 GB, für eine Person',
  'home.pricing.personal.freeBody':
    'E-Mail, Dateien, Fotos, Dokumente, Kalender, Kontakte, Notizen und Räume. Ohne Karte und ohne Zeitlimit.',
  'home.pricing.personal.note':
    'Deine Organisation ist deine E-Mail: keine Organisationsverwaltung und kein ERP, aber alles andere, auch Sytue Admin für deine Integrationen und Migrationen.',
  'home.pricing.personal.cta': 'Kostenloses Konto erstellen',
  'grid.orgOnly': 'Nur für Organisationen',
  'grid.enDesarrollo': 'In Entwicklung',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB pro Nutzer',
  'home.pricing.paidBody':
    'Für Organisationen und auch für eine einzelne Person. Die ganze Suite, mit Sytue ERP und Sytue Admin für deine Organisation.',
  'home.pricing.thirdParty':
    'Alle Tarife erlauben, eigenen Speicher anzubinden: Google Drive, Microsoft OneDrive, einen S3-Bucket (Scaleway, OVH, Hetzner, Wasabi, MinIO), einen WebDAV-Server wie Nextcloud oder Internxt oder dein NAS bei dir vor Ort —ohne offene Ports, über einen Sytue-Tunnel—. Sicherungen und Wiederherstellung der in angebundenem Speicher abgelegten Daten liegen in der Verantwortung des Kunden (Sie), und Sytue® ist von jeder Haftung für Datenverlust in an Sytue® angebundenen Speichern freigestellt.',
  'home.pricing.soon': 'Demnächst',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB pro Nutzer',
  'home.pricing.enterprise.support': 'Mit Premium-Support.',
  'home.pricing.education': 'Bildungseinrichtungen: 50 % Rabatt auf Pro.',
};
