/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ALEMÁN del portal Sytue One. Traducción del original inglés
 * (`en.ts`), que manda: el tipo exige aquí exactamente sus mismas claves.
 *
 * Tratamiento de cortesía (Sie/Ihr) en TODO el catálogo, sin mezclar: es lo que
 * ya hacen el cromo compartido y los catálogos de Docs y Contacts, y cambiar de
 * tratamiento entre productos de la misma suite se nota más que la elección.
 * Terminología: «Postfach», «Domäne», «Datensatz» (registro DNS). Comillas
 * alemanas („…“) y nombres de producto (Mail, Drive, Docs, Rooms, Cloud…) sin
 * traducir, igual que las etiquetas literales del panel de Cloudflare.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const de: AppCatalog<typeof en> = {
  // --- Títulos de página -------------------------------------------------
  'page.home': 'Sytue One — Start',
  'page.search': 'In Sytue One suchen',
  'page.searchQuery': '„{query}“ — In Sytue One suchen',
  'page.devices': 'Geräte verbinden — Sytue One',
  'page.data': 'Ihre Daten — Sytue One',
  'page.downloads': 'Downloads — Sytue One',
  'downloads.title': 'Lade die Apps herunter',
  'downloads.lead':
    'Native Anwendungen für deinen Rechner. Sie installieren sich, leben im Dock oder im Startmenü und melden sich über die Systembenachrichtigungen.',
  'downloads.version': 'Version {version}',
  'downloads.get': 'Herunterladen',
  'downloads.forYou': 'Für dein {platform}',
  'downloads.others': 'Andere Plattformen',
  'downloads.all': 'Alle Plattformen',
  'downloads.empty': 'Noch keine Installationsdateien veröffentlicht',
  'downloads.emptyNoRelease':
    'Noch keine Version hat ihre Installationsdateien veröffentlicht. Sie erscheinen hier, sobald es so weit ist; du musst nichts tun.',
  'downloads.emptyUnconfigured':
    'Diese Installation weiß nicht, wo sie die Installationsdateien suchen soll. Das richtet ein Betreiber ein.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Downloads konnten nicht geladen werden',
  'downloads.failedBody':
    'Der Hub antwortet gerade nicht. Versuchen Sie es in ein paar Sekunden erneut.',
  'downloads.savedTitle': 'Download abgeschlossen',
  'downloads.saveFailedTitle': 'Die Installationsdatei konnte nicht heruntergeladen werden',
  'page.activity': 'Letzte Aktivitäten — Sytue One',
  'page.welcome': 'Willkommen bei Sytue',
  // --- Letzte Aktivitäten (Sytue Hub) ------------------------------------
  'activity.overline': 'Aktivität',
  'activity.heading': 'Letzte Aktivitäten',
  'activity.intro':
    'Was zuletzt in Ihren E-Mails, Dateien, Dokumenten, Terminen und Räumen passiert ist.',
  'activity.today': 'Heute',
  'activity.yesterday': 'Gestern',
  'activity.empty':
    'Noch nichts Aktuelles. Sobald Sie Sytue nutzen, erscheint hier, woran Sie arbeiten.',
  'activity.failedTitle': 'Aktivitäten konnten nicht geladen werden',
  'activity.failedBody':
    'Der Hub antwortet gerade nicht. Versuchen Sie es in ein paar Sekunden erneut.',
  'activity.retry': 'Erneut versuchen',
  'activity.all': 'Alle',
  'activity.filterBy': 'Nach Produkt filtern',
  'activity.count': { one: '{count} Eintrag', other: '{count} Einträge' },
  'page.inbox': 'Benachrichtigungen — Sytue One',
  // --- Benachrichtigungen (Sytue Hub) ------------------------------------
  'inbox.overline': 'Benachrichtigungen',
  'inbox.heading': 'Benachrichtigungen',
  'inbox.intro': 'Alles, was Ihnen die Suite zu sagen hat, an einer Stelle.',
  'inbox.all': 'Alle',
  'inbox.unreadFilter': { one: '{count} ungelesen', other: '{count} ungelesen' },
  'inbox.filterBy': 'Benachrichtigungen filtern',
  'inbox.markAll': 'Alle als gelesen markieren',
  'inbox.loading': 'Benachrichtigungen werden geladen…',
  'inbox.unreadDot': 'Ungelesen',
  'inbox.errorTitle': 'Benachrichtigungen konnten nicht geladen werden',
  'inbox.errorBody':
    'Der Hub antwortet gerade nicht. Versuchen Sie es in ein paar Sekunden erneut.',
  'inbox.retry': 'Erneut versuchen',
  'inbox.emptyTitle': 'Nichts zu lesen',
  'inbox.emptyBody': 'Sobald etwas Ihre Aufmerksamkeit braucht, erscheint es hier.',
  'inbox.emptyUnreadTitle': 'Alles gelesen',
  'inbox.emptyUnreadBody':
    'Sie haben alles gelesen. Wechseln Sie zu „Alle“, um die älteren zu sehen.',
  'inbox.prefsTitle': 'Was Sie erhalten möchten',
  'inbox.prefsBody':
    'Wenn Sie ein Produkt ausschalten, benachrichtigt es Sie weder hier noch per E-Mail. Sie können es jederzeit wieder einschalten.',

  // --- Lanzador ----------------------------------------------------------
  'launcher.heading': 'Ihre Anwendungen',
  'launcher.greeting.morning': 'Guten Morgen',
  'launcher.greeting.afternoon': 'Guten Tag',
  'launcher.greeting.evening': 'Guten Abend',
  'launcher.more': 'Mehr',
  'launcher.apps': 'Anwendungen',
  'launcher.connected': 'Verbunden',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Deaktiviert',

  'product.mail': 'E-Mail',
  'product.drive': 'Dateien und Zusammenarbeit',
  'product.calendar': 'Kalender',
  'product.contacts': 'Kontakte',
  'product.docs': 'Dokumente, Tabellen und Präsentationen',
  'product.rooms': 'Kanäle, Nachrichten und Anrufe',
  'product.cloud': 'Anwendungsplattform (PaaS)',
  'product.console': 'Verwaltung und Organisationen',
  'product.id': 'Identität, Profil und Sicherheit',

  'quick.heading': 'Schnellaktionen',
  'quick.search': 'In der gesamten Suite suchen',
  'quick.compose': 'E-Mail schreiben',
  'quick.newDoc': 'Neues Dokument',
  'quick.upload': 'In Drive hochladen',
  'quick.newEvent': 'Termin erstellen',
  'quick.download': 'App herunterladen',
  'quick.devices': 'Geräte verbinden',
  'quick.data': 'Ihre Daten',

  'storage.connectTitle': 'Externen Speicher verbinden',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV und weitere.',

  'recent.heading': 'Zuletzt in der gesamten Suite',
  'recent.viewAll': 'Alle anzeigen',
  'recent.loading': 'Ihre letzten Aktivitäten werden geladen…',
  'recent.errorTitle': 'Ihre Aktivitäten konnten nicht geladen werden',
  'recent.errorBody':
    'Der Hub antwortet gerade nicht. Ihre Arbeit ist sicher; versuchen Sie es erneut.',
  'recent.retry': 'Erneut versuchen',
  'recent.emptyTitle': 'Noch keine Aktivitäten',
  'recent.emptyBody': 'Sobald Sie in Mail, Drive, Calendar oder Rooms arbeiten, erscheint es hier.',
  'recent.emptyCta': 'In Drive beginnen',

  // --- Búsqueda cross-app ------------------------------------------------
  'search.overline': 'Suche',
  'search.heading': 'Durchsuchen Sie die gesamte Suite',
  'search.placeholder': 'In Mail, Drive, Docs, Calendar, Contacts, Rooms suchen…',
  'search.aria': 'In Sytue One suchen',
  'search.submit': 'Suchen',
  'search.hint':
    'Tippen Sie oben, um Ihre E-Mails, Dateien, Dokumente, Termine, Kontakte und Räume auf einmal zu durchsuchen. Die Ergebnisse öffnen sich direkt im jeweiligen Produkt.',
  'search.failedTitle': 'Die Suche konnte nicht abgeschlossen werden',
  'search.failedBody':
    'Der Hub antwortet gerade nicht. Versuchen Sie es in ein paar Sekunden erneut.',
  'search.retry': 'Erneut versuchen',
  'search.empty': 'Keine Treffer für „{query}“. Versuchen Sie es mit anderen Wörtern.',
  'search.count': { one: '{count} Ergebnis', other: '{count} Ergebnisse' },

  // --- Bienvenida --------------------------------------------------------
  'welcome.heading': 'Willkommen bei Sytue',
  'welcome.lead':
    'Ein paar Minuten und Sie sind startklar. Wir gehen Schritt für Schritt vor; wenn Sie unterwegs aufhören, machen wir bei Ihrer Rückkehr genau dort weiter.',

  'data.title': 'Ihre Daten',
  'data.lead':
    'Laden Sie eine Kopie von allem herunter, was Sie in Sytue One haben – wann immer Sie möchten und ohne jemanden fragen zu müssen.',
  'data.export.section': 'Kopie herunterladen',
  'data.export.body':
    'Eine einzige ZIP-Datei mit Ihren E-Mails, Dateien, Kalendern, Kontakten, Notizen, Dokumenten, Fotos und Nachrichten. Was schon ein Standardformat hat, bleibt, wie es ist: E-Mails als .eml, Kalender als .ics, Kontakte als .vcf und Dateien so, wie Sie sie hochgeladen haben.',
  'data.export.formatLegend': 'Format der Listen',
  'data.export.formatHelp':
    'Die Listen beschreiben, was in der Kopie steckt: Ihre Nachrichten, Termine, Kontakte oder Dateien, einer pro Zeile.',
  'data.export.orgHelp':
    'Jede Kopie gehört zu einer Organisation. Wenn Sie eine andere wählen, wechseln Sie in diese, und der Download startet von selbst.',
  'data.export.json': 'Für andere Apps und Programme.',
  'data.export.csv': 'Zum Öffnen in Excel, Numbers oder Google Sheets.',
  'data.export.xml': 'Für Systeme, die XML verlangen.',
  'data.export.action': 'Kopie herunterladen',
  'data.export.startedTitle': 'Der Download hat begonnen',
  'data.export.startedBody':
    'Ihr Browser speichert weiter, auch wenn Sie diese Seite schließen. Bei vielen Dateien dauert es eine Weile.',
  'data.export.scope':
    'Die Kopie enthält die Daten der Organisation, mit der Sie angemeldet sind. Die Übersichtsdatei darin zeigt, was aus jeder App übernommen wurde.',
  'data.move.section': 'Zu einem anderen Dienst umziehen',
  'data.move.body':
    'Um alles direkt zu Google Workspace oder Microsoft 365 zu übertragen, ohne etwas herunterzuladen: Wenn Sie ein persönliches Konto haben oder Ihre Organisation verwalten, starten Sie den Umzug selbst in Sytue Admin unter „Migrationen“. Andernfalls bitten Sie die Person, die sie verwaltet.',
  'data.move.action': 'Zu den Migrationen',

  // --- Conectar tus dispositivos ----------------------------------------
  'devices.native.body':
    'Verwende den Dateimanager deines Geräts oder verbinde ein Konto mit dessen Zustimmung.',
  'devices.native.driveBody':
    'Sytue Drive wird nach der Anmeldung automatisch hinzugefügt. Erlaube den Speicherort, wenn das System danach fragt.',
  'devices.native.openDrive': 'Im Dateimanager öffnen',
  'devices.native.failed': 'Die Geräteintegration konnte nicht aktualisiert werden.',
  'devices.title': 'Geräte verbinden',
  'devices.lead':
    'Nehmen Sie Ihre E-Mails, Ihren Kalender, Ihre Kontakte und Ihre Dateien von Sytue mit aufs Handy oder auf den Computer. Wir führen Sie Schritt für Schritt.',
  'devices.leadShort':
    'Nehmen Sie Ihre E-Mails, Ihren Kalender und Ihre Kontakte von Sytue mit aufs Handy oder auf den Computer.',
  'devices.unavailable.section': 'Wir können Ihnen die Angaben gerade nicht zeigen',
  'devices.unavailable.title': 'Die Konfiguration konnte nicht abgerufen werden',
  'devices.unavailable.body':
    'Versuchen Sie es gleich noch einmal. Bleibt es dabei, schreiben Sie an die Person, die Sytue in Ihrer Organisation verwaltet.',
  'devices.retry': 'Erneut versuchen',
  'devices.copy': 'Kopieren',
  'devices.copyFailTitle': 'Kopieren nicht möglich',
  'devices.copyFailBody': 'Markieren Sie es und kopieren Sie es von Hand.',
  'devices.copied.address': 'Ihre Adresse kopiert',
  'devices.copied.imap': 'Posteingangsserver kopiert',
  'devices.copied.port': 'Port kopiert',
  'devices.copied.smtp': 'Postausgangsserver kopiert',
  'devices.copied.url': 'Adresse kopiert',
  'devices.copied.drive': 'Drive-Adresse kopiert',
  'devices.oneClick.thunderbird': 'Thunderbird einrichten',
  'devices.oneClick.outlook': 'Outlook einrichten',
  'devices.oneClick.android': 'Auf Android einrichten',
  'devices.oneClick.body':
    'Erstellt ein App-Passwort nur für dieses Programm, kopiert es und zeigt es Ihnen ein einziges Mal.',
  'devices.oneClick.failTitle': 'Das App-Passwort konnte nicht erstellt werden',
  'devices.oneClick.failBody':
    'Versuchen Sie es gleich noch einmal. Sie können auch eines unter Mein Konto erstellen.',
  'devices.oneClick.onceTitle': 'Kopiert. Es wird nur dieses eine Mal angezeigt',
  'devices.oneClick.onceBody':
    'Fügen Sie es dort ein, wo das Programm nach dem Passwort fragt. Wenn Sie es verlieren, widerrufen Sie es unter Mein Konto und erstellen Sie ein neues.',
  'devices.oneClick.passwordLabel': 'App-Passwort',
  'devices.copied.password': 'App-Passwort kopiert',
  'devices.oneClick.notOpened':
    '{program} ließ sich auf diesem Computer nicht öffnen: Öffnen Sie es selbst und folgen Sie diesen Schritten.',
  'devices.oneClick.next.thunderbird':
    'Geben Sie im Assistenten für ein neues Konto in Thunderbird Ihre Adresse ein und fügen Sie das Passwort ein: Die Server findet er selbst. Kalender und Kontakte: Fügen Sie einen Netzwerkkalender und ein CardDAV-Adressbuch mit derselben Adresse und demselben Passwort hinzu (fragt er nach dem Ort, kopieren Sie ihn von unten).',
  'devices.oneClick.next.outlook':
    'Geben Sie in Outlook unter „Konto hinzufügen“ Ihre Adresse ein und fügen Sie das Passwort ein: Outlook findet die Server selbst.',
  'devices.oneClick.next.android':
    'Fügen Sie in Ihrer Mail-App ein Konto „Andere (IMAP)“ mit Ihrer Adresse hinzu und fügen Sie das Passwort ein. Kalender und Kontakte: DAVx5 mit den Adressen unten und demselben Passwort.',
  'devices.android.davx5Calendars': 'Kalender in DAVx5',
  'devices.android.davx5Contacts': 'Kontakte in DAVx5',
  'devices.android.davx5Install': 'DAVx5 bei F-Droid holen',

  'devices.platform.iphone.title': 'iPhone oder iPad',
  'devices.platform.iphone.body':
    'Sie installieren ein Profil und E-Mail, Kalender und Kontakte sind fertig.',
  'devices.platform.iphone.note': 'Am schnellsten: eine Datei und Ihr Passwort.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'Dasselbe Profil wie auf dem iPhone, installiert aus den Systemeinstellungen.',
  'devices.platform.mac.note': 'Apple Mail, Kalender und Kontakte.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Sie tippen Ihre Adresse und Ihr Passwort ein; den Rest findet Outlook von selbst.',
  'devices.platform.outlook.note': 'Nur E-Mail. Kalender und Kontakte laufen getrennt.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Es konfiguriert sich allein aus Ihrer Adresse; Kalender und Kontakte kommen dazu.',
  'devices.platform.thunderbird.note': 'Windows, Mac oder Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'Ihre E-Mail-App für das Postfach und DAVx5 für Kalender und Kontakte.',
  'devices.platform.android.note': 'Keine Sytue-App: es werden die des Systems verwendet.',
  // Compatibilidad drop-in: Drive, Calendar y Contacts hablan las APIs de Google y
  // de Microsoft Graph. Estaba construido y no se decia en ninguna parte.
  'devices.platform.apis.title': 'Ein Werkzeug, das Google oder Microsoft spricht',
  'devices.platform.apis.body':
    'Richte eine Integration, die für Google Drive, Google Calendar, Google Contacts oder Microsoft Graph geschrieben wurde, auf Sytue aus — ohne ihren Code zu ändern.',
  'devices.platform.apis.note': 'Nur dort, wo du die Basisadresse ändern kannst.',
  'devices.apis.step1.title': 'Ändere die Basisadresse',
  'devices.apis.step1.body':
    'Ersetze im Werkzeug die aufgerufene Adresse durch die des Produkts und Dialekts, den du brauchst. Der Rest der Konfiguration bleibt.',
  'devices.apis.step2.title': 'Nutze deine Sytue-Zugangsdaten',
  'devices.apis.step2.body':
    'Authentifiziert wird mit deinem Sytue-ID-Token, nicht mit einem Google- oder Microsoft-Konto: die Rechte sind dieselben, die du hier schon hast.',
  'devices.apis.identityGoogle': 'Identität · Google-OAuth und Verzeichnis',
  'devices.apis.step3.title': 'Eine Drittanbieter-App, als wäre es Google',
  'devices.apis.step3.body':
    'Meldet sich das Werkzeug mit Google-OAuth an, registriert die Administration Ihrer Organisation es in Sytue Admin mit den nötigen Google-Berechtigungen. Die App fordert diese Berechtigungen an und erhält die Daten von Sytue; Google ist nicht beteiligt und sieht nichts.',
  'devices.apis.driveGoogle': 'Drive · Google-Drive-API',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · Google-Calendar-API',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · Google-People-API',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Was abgedeckt ist',
  'devices.apis.expectBody':
    'Auflisten, Lesen, Anlegen, Bearbeiten und Löschen. Was nicht abgedeckt ist, antwortet mit einem klaren Fehler — nie mit einem leeren Erfolg.',
  'devices.copied.apiBase': 'Basisadresse kopiert',
  'devices.check.apis.label': 'Das Werkzeug',
  'devices.check.apis.value':
    'Es listet deine Sytue-Inhalte, als wäre es der ursprüngliche Dienst.',
  'devices.platform.drive.title': 'Datei-Explorer (Drive)',
  'devices.platform.drive.body':
    'Sie sehen Ihre Sytue-Drive-Dateien als weiteres Laufwerk am Computer.',
  'devices.platform.drive.note': 'Finder auf dem Mac, Explorer unter Windows.',

  'devices.mail.address': 'Ihre Adresse (Benutzername)',
  'devices.mail.imapHost': 'Posteingangsserver (IMAP)',
  'devices.mail.imapPort': 'Eingangsport',
  'devices.mail.smtpHost': 'Postausgangsserver (SMTP)',
  'devices.mail.smtpPort': 'Ausgangsport',
  'devices.mail.hint':
    'SSL/TLS-Verschlüsselung bei beiden, und dasselbe Sytue-Passwort, mit dem Sie sich im Web anmelden.',
  'devices.dav.caldav': 'Kalender (CalDAV)',
  'devices.dav.carddav': 'Kontakte (CardDAV)',
  'devices.dav.user': 'Benutzername',

  'devices.apple.download': 'Sytue-Profil herunterladen',
  'devices.apple.step1.title': 'Profil herunterladen',
  'devices.apple.step1.body':
    'Drücken Sie oben auf die Schaltfläche. Es wird eine kleine Datei mit den Einstellungen Ihres Kontos heruntergeladen.',
  'devices.apple.step2.iphone.title': 'Aus den Einstellungen installieren',
  'devices.apple.step2.iphone.body':
    'Öffnen Sie Einstellungen. Ganz oben erscheint „Profil geladen“: tippen Sie darauf und drücken Sie auf Installieren.',
  'devices.apple.step2.mac.title': 'Aus den Systemeinstellungen installieren',
  'devices.apple.step2.mac.body':
    'Öffnen Sie Systemeinstellungen › Datenschutz & Sicherheit › Profile, doppelklicken Sie auf das geladene Profil und drücken Sie auf Installieren.',
  'devices.apple.step3.title': 'Du musst kein Passwort eingeben',
  'devices.apple.step3.body':
    'Das Profil enthält ein eigenes App-Passwort, das nur für dieses Gerät erstellt wurde. Du kannst es jederzeit unter Mein Konto widerrufen.',
  'devices.apple.step4.title': 'Fertig',
  'devices.apple.step4.body':
    'E-Mail, Kalender und Kontakte kommen in einem Rutsch dazu. Es ist kein Server einzutippen.',
  'devices.apple.manualTitle': 'Wenn Sie es lieber von Hand machen',
  'devices.apple.manualBody': 'Sie können das Konto auch manuell mit diesen Angaben anlegen.',
  'devices.manualValues': 'Manuelle Werte ansehen',

  'devices.desktop.step1.title': 'E-Mail-Konto hinzufügen',
  'devices.desktop.step1.outlook': 'In Outlook: Datei › Konto hinzufügen.',
  'devices.desktop.step1.thunderbird': 'In Thunderbird: Konten › Konto einrichten › E-Mail.',
  'devices.desktop.step2.title': 'Nur Ihre Adresse und Ihr Passwort eingeben',
  'devices.desktop.step2.outlook':
    'Sonst nichts. Outlook fragt die Einstellungen bei Sytue ab und trägt sie für Sie ein.',
  'devices.desktop.step2.thunderbird':
    'Sonst nichts. Thunderbird fragt die Einstellungen bei Sytue ab und trägt sie für Sie ein.',
  'devices.desktop.step3.title': 'Bestätigen',
  'devices.desktop.step3.body':
    'Wenn die Server angezeigt werden, bestätigen Sie. Die Verbindung läuft als IMAP-Konto.',
  'devices.desktop.emailLabel': 'Ihre E-Mail-Adresse',
  'devices.desktop.notFoundTitle': 'Nicht von allein gefunden?',
  'devices.desktop.notFoundBody':
    'Wählen Sie die manuelle Konfiguration und verwenden Sie diese Werte.',
  'devices.thunderbird.davTitle': 'Kalender und Kontakte',
  'devices.thunderbird.davBody': 'In Thunderbird kommen sie getrennt dazu, mit diesen Adressen.',
  'devices.outlook.davTitle': 'Kalender und Kontakte in Outlook',
  'devices.outlook.davBody':
    'Outlook für Windows unterstützt CalDAV/CardDAV nicht, deshalb werden Kalender und Kontakte von Sytue dort nicht synchronisiert. Nutzen Sie sie im Sytue-Web oder in einem Programm, das CalDAV/CardDAV beherrscht (Thunderbird, Apple Kalender).',

  'devices.android.mailTitle': 'E-Mail',
  'devices.android.mailBody':
    'Mit der E-Mail-App, die Sie ohnehin nutzen (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Konto hinzufügen',
  'devices.android.mail1.body': 'Wählen Sie „Andere“ oder „Persönlich (IMAP)“, nicht „Google“.',
  'devices.android.mail2.title': 'Ihre Adresse und Ihr Passwort eingeben',
  'devices.android.mail2.body': 'Wenn nach den Servern gefragt wird, kopieren Sie sie hier unten.',
  'devices.android.davTitle': 'Kalender und Kontakte',
  'devices.android.davBody':
    'Android bringt CalDAV/CardDAV nicht mit: Sie brauchen DAVx5, eine kostenlose App, die beides in den Kalender und das Adressbuch des Telefons einbindet.',
  'devices.android.dav1.title': 'DAVx5 installieren',
  'devices.android.dav1.body':
    'Sie ist bei Google Play und bei F-Droid zu finden. Sytue veröffentlicht und pflegt sie nicht.',
  'devices.android.dav2.title': 'Konto über „Anmeldung mit URL und Benutzername“ hinzufügen',
  'devices.android.dav2.body':
    'Fügen Sie die Adresse unten ein und verwenden Sie Ihre Sytue-E-Mail und Ihr Passwort.',
  'devices.android.dav3.title': 'Auswählen, was synchronisiert wird',
  'devices.android.dav3.body': 'Haken Sie Ihre Kalender und Ihr Adressbuch an.',

  'devices.drive.step1.title': 'Verbindung zu einem Server öffnen',
  'devices.drive.step1.body':
    'Auf dem Mac: Finder › Gehe zu › Mit Server verbinden (⌘K). Unter Windows: Explorer › Dieser PC › Netzlaufwerk verbinden, und „Verbindung mit anderen Anmeldeinformationen herstellen“ anhaken.',
  'devices.drive.step2.title': 'Diese Adresse einfügen',
  'devices.drive.step2.body': 'Das ist der Ordner mit Ihren Sytue-Drive-Dateien.',
  'devices.drive.step3.title': 'Mit Ihrem Sytue-Konto anmelden',
  'devices.drive.step3.body': 'Benutzername: Ihre E-Mail-Adresse. Passwort: Ihr Sytue-Passwort.',
  'devices.drive.urlLabel': 'Drive-Adresse (WebDAV)',
  'devices.drive.expectTitle': 'Was Sie erwartet',
  'devices.drive.windowsExpectBody':
    'Unter Windows erscheint Drive im Datei-Explorer als Ordner „Sytue Drive“. Dateien werden beim Öffnen geladen; Rechtsklick › „Immer auf diesem Gerät beibehalten“ hält sie offline bereit. Was du dort speicherst oder anlegst, wird in Drive hochgeladen.',
  'devices.drive.expectBody':
    'Die Dateien werden vom Server geöffnet, nicht auf die Festplatte kopiert. Zum Arbeiten ohne Verbindung laden Sie vorher herunter, was Sie brauchen.',

  'devices.check.mail.label': 'Die E-Mail',
  'devices.check.mail.value':
    'Neue Nachrichten kommen an und Sie können sich selbst eine schicken.',
  'devices.check.dav.label': 'Kalender und Kontakte',
  'devices.check.dav.value':
    'Ein im Sytue-Web erstellter Termin taucht innerhalb weniger Minuten auf dem Gerät auf.',
  'devices.check.profile.label': 'Das Profil',
  'devices.check.profile.value': '„Sytue One“ steht in der Liste der installierten Profile.',
  'devices.check.drive.label': 'Das Laufwerk',
  'devices.check.drive.value': 'Sie sehen Ihre Drive-Ordner und können eine Datei öffnen.',
  'devices.check.write.label': 'Das Schreiben',
  'devices.check.write.value':
    'Sie legen einen Ordner an und er erscheint auch in Sytue Drive im Browser.',

  'devices.step1.title': 'Was Sie nutzen',
  'devices.step1.heading': 'Wo möchten Sie Ihr Sytue-Konto?',
  'devices.step1.description':
    'Wählen Sie eines aus. Bei mehreren wiederholen Sie den Assistenten für jedes.',
  'devices.step2.title': 'So geht es',
  'devices.step2.heading': '{platform} einrichten',
  'devices.step2.headingFallback': 'So geht es',
  'devices.step2.description':
    'Folgen Sie diesen Schritten. Alles, was Sie eintippen müssen, lässt sich mit einer Schaltfläche kopieren.',
  'devices.step2.next': 'Erledigt',
  'devices.step3.title': 'Prüfen',
  'devices.step3.heading': 'Woran Sie merken, dass es geklappt hat',
  'devices.step3.description': 'Wenn das alles zutrifft, ist es richtig verbunden.',
  'devices.step3.helpTitle': 'Etwas passt nicht?',
  'devices.step3.helpBody':
    'Meist liegt es am Passwort: nehmen Sie dasselbe, mit dem Sie sich im Browser bei Sytue anmelden. Wenn Sie die Bestätigung in zwei Schritten nutzen, verlangt Ihre Organisation womöglich ein App-Passwort; fragen Sie die Person, die Sytue verwaltet.',
  'devices.step3.again': 'Weiteres Gerät verbinden',

  // --- Alta guiada (onboarding) -----------------------------------------
  'ob.error.generic': 'Der Vorgang konnte nicht abgeschlossen werden. Versuchen Sie es erneut.',
  'ob.failedTitle': 'Ihre Registrierung konnte nicht geladen werden',
  'ob.failedBody':
    'Der Identitätsdienst antwortet gerade nicht. Es ist nichts verloren gegangen: versuchen Sie es in einer Minute erneut.',
  'ob.retry': 'Erneut versuchen',
  'ob.preparing': 'Ihre Registrierung wird vorbereitet…',
  'ob.lead':
    'In wenigen Minuten ist alles eingerichtet. Wir gehen Schritt für Schritt vor; wenn Sie mittendrin aufhören, machen wir bei Ihrer Rückkehr dort weiter.',
  'ob.saveFailedTitle': 'Speichern nicht möglich',
  'ob.saveFailedBody': 'Versuchen Sie es gleich noch einmal.',
  'ob.copy': 'Kopieren',
  'ob.copyFailTitle': 'Kopieren nicht möglich',
  'ob.copyFailBody': 'Markieren Sie es mit der Maus und kopieren Sie es von Hand.',
  'ob.copied.recordType': 'Datensatztyp kopiert',
  'ob.copied.recordName': 'Datensatzname kopiert',
  'ob.copied.recordValue': 'Datensatzwert kopiert',

  'ob.roleOption.member': 'Mitglied — nutzt die Suite, verwaltet nichts',
  'ob.roleOption.admin': 'Administrator — kann die gesamte Organisation verwalten',
  'ob.roleOption.billing': 'Abrechnung — kümmert sich um die Rechnungen',
  'ob.roleName.owner': 'Inhaber',
  'ob.roleName.admin': 'Administrator',
  'ob.roleName.member': 'Mitglied',
  'ob.roleName.billing': 'Abrechnung',

  'ob.mode.title': 'Start',
  'ob.mode.heading': 'Für wen ist Sytue?',
  'ob.mode.description': 'Danach richtet sich, was wir Sie fragen. Sie können es später ändern.',
  'ob.mode.individual.title': 'Nur für mich',
  'ob.mode.individual.body':
    'Ihre E-Mails, Ihre Dateien und Ihr Kalender, mit Ihrer eigenen Adresse. Wenn Sie eines Tages jemanden einladen möchten, können Sie das tun.',
  'ob.mode.org.title': 'Für meine Organisation',
  'ob.mode.org.body':
    'Sie melden Ihr Team an, verteilen, wer was darf, und verwalten alles an einer Stelle.',
  'ob.mode.org.badge': 'Am häufigsten',

  'ob.identity.titleOrg': 'Ihre Organisation',
  'ob.identity.titleMe': 'Ihr Konto',
  'ob.identity.headingOrg': 'Wie heißt Ihre Organisation?',
  'ob.identity.headingMe': 'Wie soll Ihr Konto heißen?',
  'ob.identity.descriptionCreated':
    'Sie ist bereits angelegt. Den Namen können Sie ändern; die Adresse bleibt, wie sie ist.',
  'ob.identity.description': 'Damit legen wir Ihren Bereich und Ihre Adresse in Sytue an.',
  'ob.identity.nextCreated': 'Weiter',
  'ob.identity.nextOwnDomain': 'Anlegen und sehen, was zu meiner Domäne dazu muss',
  'ob.identity.next': 'Anlegen und weiter',
  'ob.identity.nameLabel': 'Name',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Anna Schmidt',
  'ob.identity.nameHintOrg': 'Das sieht Ihr Team beim Anmelden. Sie können es jederzeit ändern.',
  'ob.identity.nameHintMe': 'Das sehen nur Sie. Sie können es jederzeit ändern.',
  'ob.identity.slugLabel': 'Ihre Adresse in Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Ihre Adresse ist {subdomain}. Sie lässt sich nicht ändern: Ihre E-Mails und Ihre Dateien hängen daran.',
  'ob.identity.slugEmpty': 'Wählen Sie einen kurzen Namen. Er wird Ihre Adresse: acme.{suite}',
  'ob.identity.slugChecking': 'Wird geprüft, ob er frei ist…',
  'ob.identity.slugFree': 'Frei. Ihre Adresse wird {subdomain}',
  'ob.identity.slugTaken': 'Lässt sich nicht verwenden.',
  'ob.identity.firstTitle': 'Zuerst legen wir Ihren Bereich an',
  'ob.identity.firstBody':
    'Wenn Sie unten auf die Schaltfläche drücken, legen wir Ihre Sytue-Adresse an und zeigen Ihnen gleich hier den genauen Datensatz, den Sie zu Ihrer Domäne hinzufügen müssen. Sie brauchen ihn jetzt noch nicht parat zu haben.',

  'ob.domain.legend': 'Und Ihre eigene Domäne?',
  'ob.domain.keep.title': 'Ich bleibe bei {host}',
  'ob.domain.keep.placeholder': 'mein-name',
  'ob.domain.keep.body':
    'Sie müssen nichts anfassen: die Veröffentlichung übernehmen wir. Für den Anfang empfehlen wir genau das.',
  'ob.domain.own.title': 'Ich habe schon eine Domäne und will sie nutzen (acme.com)',
  'ob.domain.own.body':
    'Liegt sie bei Cloudflare, richten wir sie für Sie ein. Wenn nicht, nennen wir Ihnen den genauen Datensatz und prüfen ihn gleich hier.',
  'ob.domain.search.title': 'Ich habe noch keine Domäne',
  'ob.domain.search.body':
    'Wir prüfen, ob der gewünschte Name frei ist und was er im Jahr kostet. Es ist nur eine Abfrage: hier wird nichts gekauft.',
  'ob.domain.hostLabel': 'Ihre Domäne',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint':
    'Schreiben Sie sie so, wie sie ist: acme.com, ohne „https://“ und ohne „www“.',
  'ob.domain.verified': '{host} bestätigt',

  'ob.dns.legend': 'Wo liegt {domain}?',
  'ob.dns.fallback': 'Ihre Domäne',
  'ob.dns.cf.title': 'Bei Cloudflare — bitte für mich einrichten',
  'ob.dns.cf.body':
    'Sie geben uns vorübergehend die Berechtigung für Ihr Konto und wir legen die Datensätze selbst an. Bevor wir etwas anfassen, zeigen wir Ihnen, was wir ändern werden.',
  'ob.dns.cf.badge': 'Ohne DNS anzufassen',
  'ob.dns.manual.title': 'Bei einem anderen Anbieter — ich mache es selbst',
  'ob.dns.manual.body':
    'Wir geben Ihnen den genauen Datensatz zum Kopieren in Ihr DNS-Panel und prüfen ihn hier, sobald er steht.',
  'ob.dns.manualSummary': 'Ich füge die Datensätze lieber selbst hinzu',

  'ob.people.title': 'Ihr Team',
  'ob.people.heading': 'Wen laden Sie zuerst ein?',
  'ob.people.description':
    'Jede Person bekommt eine E-Mail mit einem persönlichen Link, um ihr Passwort anzulegen und sich anzumelden. Sie können diesen Schritt überspringen und es später machen.',
  'ob.people.next': 'Weiter',
  'ob.people.emailsLabel': 'E-Mail-Adressen',
  'ob.people.emailsPlaceholder': 'anna@acme.com, bruno@acme.com',
  'ob.people.emailsHint':
    'Sie können mehrere auf einmal einfügen, getrennt durch Kommas oder Zeilenumbrüche.',
  'ob.people.roleLabel': 'Mit welcher Rolle sie einsteigen',
  'ob.people.roleHint': 'Lässt sich jederzeit in Sytue Admin ändern.',
  'ob.people.send': 'Einladungen senden',
  'ob.people.sendNote': 'Es wird nichts verschickt, bevor Sie hier drücken.',
  'ob.people.pendingTitle': 'Einladungen, die noch angenommen werden müssen',
  'ob.people.emptyTitle': 'Sie haben noch niemanden eingeladen',
  'ob.people.emptyBody':
    'Sie können es jetzt oder später in Sytue Admin tun. Ihre Organisation funktioniert genauso.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Einladung gesendet',
  'ob.invite.already': 'Schon in Ihrer Organisation',
  'ob.invite.invalid': 'Das sieht nicht nach einer E-Mail-Adresse aus',
  'ob.invite.notSent': 'Die E-Mail konnte nicht gesendet werden',
  'ob.invite.failed': 'Sie konnten nicht eingeladen werden',
  'ob.invite.default':
    'Sie bekommen eine E-Mail mit einem persönlichen Link, um ihr Passwort anzulegen und sich anzumelden.',

  'ob.billing.title': 'Abrechnung',
  'ob.billing.heading': 'Auf wen laufen die Rechnungen?',
  'ob.billing.description':
    'Geben Sie die Rechnungsdaten ein und wählen Sie die bezahlten Plätze. Der Zugang startet erst nach der Bestätigung durch Mollie.',
  'ob.billing.next': 'Weiter',
  'ob.billing.pay': 'Sicher mit Mollie bezahlen',
  'ob.billing.verify': 'Zahlung prüfen',
  'ob.billing.legalLabel': 'Firmenname',
  'ob.billing.legalPlaceholder': 'ACME Europe GmbH',
  'ob.billing.legalHint':
    'Der rechtliche Name des Unternehmens, genau so, wie er auf Ihren Rechnungen steht.',
  'ob.billing.taxLabel': 'Steuernummer',
  'ob.billing.taxPlaceholder': 'DE123456789',
  'ob.billing.taxHint': 'Die Steuer- oder Umsatzsteuer-Identifikationsnummer Ihres Landes.',
  'ob.billing.addressLabel': 'Adresse',
  'ob.billing.addressPlaceholder': 'Hauptstraße 1, 10115 Berlin, Deutschland',
  'ob.billing.addressHint': 'Die Firmenanschrift, die auf die Rechnung kommt.',
  'ob.billing.emailLabel': 'E-Mail für Rechnungen',
  'ob.billing.emailPlaceholder': 'rechnungen@acme.com',
  'ob.billing.emailHint':
    'Dorthin schicken wir die Rechnungen. Standardmäßig die E-Mail Ihres Kontos.',
  'ob.billing.note':
    'Wir fragen nur ab, was zum Rechnungstellen nötig ist. Nichts davon wird mit irgendjemandem geteilt.',
  'ob.billing.seatsLabel': 'Bezahlte Plätze',
  'ob.billing.seatsHint': '{seats} Platz/Plätze × {price}/Nutzer/Monat · gesamt {total}/Monat.',
  'ob.billing.active': 'Abonnement aktiv',
  'ob.billing.activeBody': 'Mollie hat die Zahlung bestätigt. Ihr Sytue-One-Zugang ist aktiv.',
  'ob.billing.pending': 'Wir prüfen die Zahlung',
  'ob.billing.pendingBody':
    'Mollie hat die Zahlung noch nicht bestätigt. Warten Sie einige Sekunden und prüfen Sie erneut; Sie werden nicht doppelt belastet.',

  'ob.roles.title': 'Berechtigungen',
  'ob.roles.heading': 'Möchten Sie die Verwaltungsaufgaben verteilen?',
  'ob.roles.description':
    'Das sind fertige Rollen: Sie wählen aus, welche Sie wollen, und wir legen sie an. Danach weisen Sie sie in Sytue Admin zu, wem Sie möchten. Diesen Schritt können Sie überspringen.',
  'ob.roles.next': 'Weiter',
  'ob.roles.already': 'Bereits angelegt',
  'ob.roles.seePermissions': 'Genaue Berechtigungen ansehen',
  'ob.roles.helpTitle': 'Die Kontrolle bleibt bei Ihnen',
  'ob.roles.helpBody':
    'Ihr Konto ist der Inhaber: diese Rollen nehmen ihm nichts weg. Sie können sie jederzeit in Sytue Admin → Rollen bearbeiten oder löschen.',

  'ob.done.title': 'Fertig',
  'ob.done.headingCompleted': 'Alles bereit',
  'ob.done.heading': 'Das haben Sie eingerichtet',
  'ob.done.description':
    'Es stammt aus Ihrem echten Konto, nicht aus dem, was Sie hier eingetippt haben.',
  'ob.done.next': 'Zu Sytue',
  'ob.done.warnTitle': '{host} ist noch nicht bestätigt',
  'ob.done.warnBody':
    'Sytue funktioniert währenddessen genauso, mit {subdomain}. Wenn Sie den Datensatz zu Ihrer Domäne hinzugefügt haben, kommen Sie zum Schritt „{step}“ zurück und drücken Sie auf „Jetzt prüfen“.',
  'ob.done.nextTitle': 'Wie es weitergeht',
  'ob.next.devices.title': 'Handy und Computer verbinden',
  'ob.next.devices.body':
    'Lassen Sie E-Mail, Kalender und Kontakte in den Programmen laufen, die Sie schon immer nutzen.',
  'ob.next.migrations.title': 'Inhalte aus Microsoft 365 oder Google mitnehmen',
  'ob.next.migrations.body':
    'Wir kopieren E-Mails, Dateien und Kalender, ohne dass Ihnen unterwegs etwas verloren geht.',
  'ob.next.apps.title': 'Anwendungen verbinden',
  'ob.next.apps.body': 'Die Werkzeuge, die Ihr Team schon nutzt, verknüpft mit Ihrem Sytue-Konto.',
  'ob.next.users.title': 'Weiter Ihr Team anmelden',
  'ob.next.users.body': 'Laden Sie ein, wer noch fehlt, und prüfen Sie, wer welche Rolle hat.',

  'ob.sum.org': 'Organisation',
  'ob.sum.account': 'Ihr Konto',
  'ob.sum.address': 'Adresse',
  'ob.sum.addressLive': '{host} · veröffentlicht',
  'ob.sum.addressPending': '{host} · wartet auf Veröffentlichung',
  'ob.sum.ownDomain': 'Eigene Domäne',
  'ob.sum.domainVerified': '{host} · bestätigt',
  'ob.sum.domainPending': '{host} · wartet auf den Datensatz',
  'ob.sum.noDomain': 'Keine eigene Domäne',
  'ob.sum.invitations': 'Einladungen',
  'ob.sum.invitationsPending': {
    one: '1 wartet auf Annahme',
    other: '{count} warten auf Annahme',
  },
  'ob.sum.noInvitations': 'Noch keine',
  'ob.sum.billing': 'Abrechnung',
  'ob.sum.billingEmpty': 'Nicht ausgefüllt (noch nicht nötig)',
  'ob.sum.roles': 'Angelegte Rollen',
  'ob.sum.edit': 'Bearbeiten',
  'ob.sum.noRoles': 'Keine',

  'ob.sub.liveTitle': 'Ihre Adresse ist bereits veröffentlicht',
  'ob.sub.liveBody': '{host} löst im Internet auf. Sie müssen nichts weiter tun.',
  'ob.sub.failedTitle': 'Ihre Adresse ist noch nicht veröffentlicht',
  'ob.sub.failedDefault': 'Wir konnten sie in diesem Moment nicht veröffentlichen.',
  'ob.sub.failedTail':
    'Ihr Konto ist angelegt: machen Sie mit der Registrierung weiter und schauen Sie später noch einmal vorbei.',
  'ob.sub.reservedTitle': 'Ihre Adresse ist reserviert',
  'ob.sub.reservedBody':
    '{host} gehört Ihnen. Die Veröffentlichung im Internet übernehmen wir: Sie müssen nichts tun.',

  'ob.search.label': 'Die Domäne, die Sie gern hätten',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Schreiben Sie sie vollständig, mit Endung: acme.com, acme.de, acme.eu…',
  'ob.search.check': 'Verfügbarkeit prüfen',
  'ob.search.useOwn': 'Ich habe sie schon registriert: verbinden',
  'ob.search.note':
    'Bei Sytue werden keine Domänen verkauft: hier sagen wir Ihnen nur, ob sie frei ist und was sie kostet. Registrieren Sie sie, wo Sie möchten, und kommen Sie zu diesem Schritt zurück, zu „Ich habe schon eine Domäne“, um sie mit Ihrem Konto zu verbinden.',
  'ob.avail.freeTitle': '{domain} ist frei',
  'ob.avail.freePrice':
    'Sie kostet {registration} im ersten Jahr und {renewal} bei jeder Verlängerung. Das ist der heutige Preis bei Cloudflare; wo Sie sie registrieren, kann er anders sein.',
  'ob.avail.freeNoPrice':
    'Sie lässt sich registrieren. Den Preis nennt Ihnen der Registrar, bei dem Sie sie kaufen.',
  'ob.avail.takenTitle': '{domain} ist bereits registriert',
  'ob.avail.takenBody':
    'Jemand hat sie. Versuchen Sie einen anderen Namen oder eine andere Endung (.de, .eu, .net…). Gehört die Domäne Ihnen, wählen Sie oben „Ich habe schon eine Domäne“ und wir verbinden sie.',
  'ob.avail.unsupportedTitle': 'Diese Endung können wir nicht abfragen',
  'ob.avail.unsupportedBody':
    'Wir können Ihnen nicht sagen, ob sie frei ist oder was sie kostet: prüfen Sie es bei Ihrem Registrar. Wenn Sie sie haben, kommen Sie hierher zurück und wählen Sie „Ich habe schon eine Domäne“.',
  'ob.avail.invalidTitle': 'Das sieht nicht nach einer Domäne aus',
  'ob.avail.invalidBody':
    'Schreiben Sie sie wie acme.com: ein Name, ein Punkt und die Endung. Ohne „https://“ und ohne „www“.',
  'ob.avail.unconfiguredTitle': 'Domänenpreise fragen wir hier nicht ab',
  'ob.avail.unconfiguredBody':
    'Suchen Sie sie bei dem Registrar, den Sie bevorzugen. Wenn Sie sie haben, kommen Sie zu diesem Schritt zurück und wählen Sie „Ich habe schon eine Domäne“, um sie zu verbinden.',
  'ob.avail.unknownTitle': 'Wir konnten es gerade nicht abfragen',
  'ob.avail.unknownBody':
    'Es liegt nicht an dem, was Sie eingetippt haben: wir haben keine Antwort bekommen. Versuchen Sie es gleich noch einmal.',

  'ob.dnsRec.title': 'Fügen Sie diesen Datensatz dort hinzu, wo Ihre Domäne registriert ist',
  'ob.dnsRec.body':
    'Suchen Sie bei Ihrem Anbieter den Bereich „DNS“ oder „Datensätze“, legen Sie einen TXT-Datensatz an und kopieren Sie diese beiden Werte. Das Vorgehen ist überall dasselbe.',
  'ob.dnsRec.type': 'Typ',
  'ob.dnsRec.name': 'Name',
  'ob.dnsRec.value': 'Wert',
  'ob.dnsRec.verifiedTitle': 'Domäne bestätigt',
  'ob.dnsRec.verifiedBody':
    'Wir haben den Datensatz gefunden: {host} gehört Ihnen und ist auch so eingetragen.',
  'ob.dnsRec.mismatchTitle': 'Es gibt einen Datensatz, aber nicht diesen',
  'ob.dnsRec.mismatchBody':
    'Unter {name} haben wir „{found}“ gefunden. Prüfen Sie, ob Sie den ganzen Wert kopiert haben und kein alter übrig geblieben ist.',
  'ob.dnsRec.unreachableTitle': 'Wir konnten Ihr DNS nicht abfragen',
  'ob.dnsRec.unreachableBody':
    'Es liegt nicht an Ihrem Datensatz: der Nameserver hat nicht geantwortet. Versuchen Sie es gleich noch einmal.',
  'ob.dnsRec.pendingTitle': 'Wir sehen den Datensatz noch nicht',
  'ob.dnsRec.pendingBody':
    'Wenn Sie ihn gerade erst angelegt haben, ist das normal: DNS-Änderungen brauchen ein paar Minuten bis ein paar Stunden, bis sie sichtbar sind. Sie können mit der Registrierung weitermachen und jederzeit erneut prüfen.',
  'ob.dnsRec.uncheckedTitle': 'Noch nicht geprüft',
  'ob.dnsRec.uncheckedBody':
    'Wenn Sie den Datensatz bei Ihrem Anbieter hinzugefügt haben, drücken Sie auf „Jetzt prüfen“ und wir schauen sofort in Ihr DNS.',
  'ob.dnsRec.empty':
    'Wenn Sie auf „Speichern und prüfen“ drücken, geben wir Ihnen den genauen Datensatz zum Hinzufügen.',
  'ob.dnsRec.check': 'Jetzt prüfen',
  'ob.dnsRec.save': 'Speichern und prüfen',

  'ob.cf.state.ok': 'War schon da',
  'ob.cf.state.created': 'Angelegt',
  'ob.cf.state.replaced': 'Ersetzt',
  'ob.cf.state.missing': 'Wir fügen ihn hinzu',
  'ob.cf.state.conflict': 'Da steht schon etwas',
  'ob.cf.state.failed': 'Nicht möglich',
  'ob.cf.state.pending': 'Noch nicht',
  'ob.cf.permTitle': 'Geben Sie uns die Berechtigung nur für Ihr DNS',
  'ob.cf.step1': 'Öffnen Sie Ihre Cloudflare-API-Tokens und drücken Sie auf „Create token“.',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2':
    'Wählen Sie „Create custom token“ und haken Sie eine einzige Berechtigung an: Zone · DNS · Edit.',
  'ob.cf.step3':
    'Wählen Sie unter „Zone resources“ {domain} aus, erstellen Sie das Token und fügen Sie es hier ein.',
  'ob.cf.tokenLabel': 'Cloudflare-Token',
  'ob.cf.tokenPlaceholder': 'Das Token, das Sie gerade erstellt haben',
  'ob.cf.tokenHint':
    'Es funktioniert nur für das DNS dieser Domäne. Wir verwenden es und verwerfen es: es wird nicht gespeichert, außer Sie bitten uns hier unten darum, und es verlässt unsere Server nie.',
  'ob.cf.mailLabel': 'Die E-Mails von @{domain} auch in Sytue Mail empfangen',
  'ob.cf.mailHintOn':
    'Das ändert, wo die E-Mails Ihrer Domäne ankommen. Wenn Sie sie heute woanders empfangen, warnen wir Sie, bevor wir etwas ersetzen.',
  'ob.cf.mailHintOff':
    'Schalten Sie es nur ein, wenn Ihre Postfächer unter @ihre-domain zu Sytue umziehen sollen.',
  'ob.cf.rememberLabel': 'Diese Berechtigung für künftige Änderungen merken',
  'ob.cf.rememberHint':
    'Ohne Haken müssen Sie es beim nächsten Mal erneut einfügen. Mit Haken wird es verschlüsselt gespeichert; wenn Sie den Haken entfernen, wird es gelöscht.',
  'ob.cf.preview': 'Ansehen, was wir ändern werden',
  'ob.cf.previewAgain': 'Erneut nachsehen',
  'ob.cf.nothing': 'Es ist nichts mehr zu tun',
  'ob.cf.apply': { one: '1 Änderung anwenden', other: '{count} Änderungen anwenden' },
  'ob.cf.zone': 'Zone gefunden',
  'ob.cf.token': 'Ihr Token',
  'ob.cf.tokenStored': 'Verschlüsselt gespeichert für künftige Änderungen',
  'ob.cf.tokenDiscarded': 'Verwendet und verworfen (wir haben es nicht gespeichert)',
  'ob.cf.domainState': 'Status der Domäne',
  'ob.cf.verified': 'Bestätigt',
  'ob.cf.pending': 'Wartet auf Bestätigung',
  'ob.cf.nowThere': 'Im Moment steht dort: {current}',
  'ob.cf.replaceLabel': 'Durch den von Sytue ersetzen',
  'ob.cf.failTitle': 'Etwas konnte nicht eingerichtet werden',
  'ob.cf.failBody':
    'Der Rest steht. Oben finden Sie zu jedem fehlgeschlagenen Datensatz den Grund und seinen Wert, um ihn von Hand in Cloudflare hinzuzufügen.',
  'ob.cf.conflictTitle': 'Es gibt Datensätze von Ihnen, die wir nicht anfassen',
  'ob.cf.conflictBody':
    'Wir ersetzen nie etwas, das Sie bereits haben, außer Sie haken es an. Wenn Sie sie lieber von Hand zusammenführen, finden Sie dort oben den aktuellen Wert und unseren.',
  'ob.cf.okTitle': 'Domäne bestätigt',
  'ob.cf.okBody':
    'Wir haben den Datensatz in Ihrer eigenen Zone gesehen: {host} gehört Ihnen und ist auch so eingetragen.',
  'ob.cf.todoTitle': 'Die Änderungen müssen noch angewendet werden',
  'ob.cf.todoBody':
    'Wir haben Ihre Zone noch nicht angefasst. Drücken Sie auf „Anwenden“, wenn das oben für Sie passt.',
  'ob.cf.someFailed': {
    one: '1 Datensatz konnte nicht eingerichtet werden',
    other: '{count} Datensätze konnten nicht eingerichtet werden',
  },
  'ob.cf.someFailedBody':
    'Unten finden Sie zu jedem den Grund und die Werte, um es von Hand zu machen.',
  'ob.cf.configured': '{host} eingerichtet und bestätigt',
  'ob.cf.created': 'Datensätze in Ihrer Zone angelegt',
  'today.title': 'Heute',
  'today.openCalendar': 'Kalender öffnen',
  'today.noEvents': 'Heute steht nichts im Kalender.',
  'today.unavailable': 'Der Kalender ist gerade nicht erreichbar.',
  'today.allDay': 'Ganztägig',
  'today.untitled': '(ohne Titel)',
  'today.unread': { one: '1 ungelesene E-Mail', other: '{count} ungelesene E-Mails' },
  'today.inboxZero': 'Posteingang auf dem Laufenden',
  'launcher.quickActions': 'Schnellzugriff',
  'launcher.recentAll': 'Zuletzt in der gesamten Suite',
  'launcher.activityError': 'Ihre Aktivität konnte nicht geladen werden',
  'launcher.noActivity': 'Keine kürzliche Aktivität',
  'ob.dns.verified': 'Domäne verifiziert',
  'ob.dns.mismatch': 'Es gibt einen Datensatz, aber nicht diesen',
  'ob.dns.unreachable': 'Ihr DNS konnte nicht abgefragt werden',
  'ob.dns.notYet': 'Wir sehen den Datensatz noch nicht',
  'ob.dns.unchecked': 'Noch nicht geprüft',

  // --- Onboarding: rótulos que estaban en duro ---
  'ob.dns.type': 'Typ',
  'ob.dns.name': 'Name',
  'ob.dns.value': 'Wert',
  'ob.cf.tokenField': 'Cloudflare-Token',
  'ob.cf.remember': 'Diese Berechtigung für künftige Änderungen merken',
  'ob.cf.replaceWithSytue': 'Durch den von Sytue ersetzen',
  'recent.hubDown':
    'Der Hub antwortet gerade nicht. Ihre Arbeit ist sicher; versuchen Sie es erneut.',
  'search.partial':
    '{sources} hat nicht geantwortet, daher können Ergebnisse fehlen. Versuchen Sie es gleich noch einmal.',
  'search.partialJoin': ' und ',
  'search.showing': '{count} von {total} Ergebnissen werden angezeigt.',
  'privacy.notice':
    'Wir verkaufen Ihre Daten nicht und geben sie nicht an Dritte weiter. Wir nutzen sie weder für Profilbildung noch für Werbung, noch um unsere Dienste oder die anderer zu trainieren oder zu verbessern. Sie sind bei der Übertragung und im Ruhezustand verschlüsselt und verlassen unsere europäische Infrastruktur nur, soweit es für die Abrechnung unbedingt nötig ist, und in den Integrationen, die Sie aktivieren – dort bittet Sie der Drittanbieter um Zustimmung zu den verwendeten Daten.',
};
