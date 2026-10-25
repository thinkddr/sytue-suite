/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo INGLÉS del portal Sytue One — la FUENTE DE VERDAD
 * (docs/CONVENTIONS.md §13.1). Las cadenas nacen aquí y es/fr/de/it son
 * traducciones de este original: su tipo se deriva de él (`AppCatalog<typeof
 * en>`), así que una clave nueva rompe la compilación de los otros cuatro.
 *
 * Los NOMBRES de los productos (Mail, Drive, Docs, Rooms, Cloud…) son marca y no
 * se traducen NUNCA; sus descripciones sí, y viven aquí (`product.*`) y no en
 * `@sytue/ui`, cuyo catálogo está en español.
 */
export const en = {
  // --- Títulos de página -------------------------------------------------
  'page.home': 'Sytue One — Home',
  'page.search': 'Search in Sytue One',
  'page.searchQuery': '“{query}” — Search in Sytue One',
  'page.devices': 'Connect your devices — Sytue One',
  'page.data': 'Your data — Sytue One',
  'page.downloads': 'Downloads — Sytue One',
  'downloads.title': 'Download the apps',
  'downloads.lead':
    'Native applications for your machine. They install, live in the Dock or the Start menu and alert you through system notifications.',
  'downloads.version': 'Version {version}',
  'downloads.get': 'Download',
  'downloads.forYou': 'For your {platform}',
  'downloads.others': 'Other platforms',
  'downloads.all': 'All platforms',
  'downloads.empty': 'No installers published yet',
  'downloads.emptyNoRelease':
    'No version has published its installers yet. They appear here as soon as one does; nothing needs doing on your side.',
  'downloads.emptyUnconfigured':
    'This deployment does not know where to look for the installers. An operator configures it.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Downloads could not be loaded',
  'downloads.failedBody': 'The Hub is not responding right now. Try again in a few seconds.',
  'downloads.savedTitle': 'Download complete',
  'downloads.saveFailedTitle': 'The installer could not be downloaded',
  'page.activity': 'Recent activity — Sytue One',
  'page.welcome': 'Welcome to Sytue',
  // --- Actividad reciente (Sytue Hub) ------------------------------------
  'activity.overline': 'Activity',
  'activity.heading': 'Recent activity',
  'activity.intro':
    'What has happened lately across your mail, files, documents, events and rooms.',
  'activity.today': 'Today',
  'activity.yesterday': 'Yesterday',
  'activity.empty': 'Nothing recent yet. As you use Sytue, what you touch shows up here.',
  'activity.failedTitle': 'Activity could not be loaded',
  'activity.failedBody': 'The Hub is not responding right now. Try again in a few seconds.',
  'activity.retry': 'Try again',
  'activity.all': 'All',
  'activity.filterBy': 'Filter by product',
  'activity.count': { one: '{count} item', other: '{count} items' },
  'page.inbox': 'Notifications — Sytue One',
  // --- Bandeja de notificaciones (Sytue Hub) -----------------------------
  'inbox.overline': 'Notifications',
  'inbox.heading': 'Notifications',
  'inbox.intro': 'Everything the suite has to tell you, in one place.',
  'inbox.all': 'All',
  'inbox.unreadFilter': { one: '{count} unread', other: '{count} unread' },
  'inbox.filterBy': 'Filter notifications',
  'inbox.markAll': 'Mark all as read',
  'inbox.loading': 'Loading notifications…',
  'inbox.unreadDot': 'Unread',
  'inbox.errorTitle': 'Notifications could not be loaded',
  'inbox.errorBody': 'The Hub is not responding right now. Try again in a few seconds.',
  'inbox.retry': 'Try again',
  'inbox.emptyTitle': 'Nothing to read',
  'inbox.emptyBody': 'When something needs your attention, it shows up here.',
  'inbox.emptyUnreadTitle': 'All caught up',
  'inbox.emptyUnreadBody': 'You have read everything. Switch to “All” to see the earlier ones.',
  'inbox.prefsTitle': 'What you want to receive',
  'inbox.prefsBody':
    'Turn off a product and it stops notifying you here and by email. You can turn it back on whenever you like.',

  // --- Lanzador ----------------------------------------------------------
  // El texto más grande de la portada ORIENTA (dice dónde estás); no saluda.
  'launcher.heading': 'Your applications',
  'launcher.greeting.morning': 'Good morning',
  'launcher.greeting.afternoon': 'Good afternoon',
  'launcher.greeting.evening': 'Good evening',
  'launcher.more': 'More',
  'launcher.apps': 'Applications',
  'launcher.connected': 'Connected',
  'launcher.appAria': '{name} — {description}',
  // Producto servido por otra aplicación: la tesela lo dice ANTES del clic.
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Disabled',

  'product.mail': 'Email',
  'product.drive': 'Files and collaboration',
  'product.calendar': 'Calendar',
  'product.contacts': 'Contacts',
  'product.docs': 'Documents, spreadsheets and presentations',
  'product.rooms': 'Channels, messages and calls',
  'product.cloud': 'Application platform (PaaS)',
  'product.console': 'Administration and organisations',
  'product.id': 'Identity, profile and security',

  'quick.heading': 'Quick actions',
  'quick.search': 'Search across the suite',
  'quick.compose': 'Write an email',
  'quick.newDoc': 'New document',
  'quick.upload': 'Upload to Drive',
  'quick.newEvent': 'Create an event',
  'quick.download': 'Download the app',
  'quick.devices': 'Connect your devices',
  'quick.data': 'Your data',

  'storage.connectTitle': 'Connect external storage',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV and more.',

  'recent.heading': 'Recent across the suite',
  'recent.viewAll': 'View all',
  'recent.loading': 'Loading recent activity…',
  'recent.errorTitle': 'We could not load your activity',
  'recent.errorBody': 'The Hub is not responding right now. Your work is safe; try again.',
  'recent.retry': 'Try again',
  'recent.emptyTitle': 'No recent activity',
  'recent.emptyBody': 'When you work in Mail, Drive, Calendar or Rooms, you will see it here.',
  'recent.emptyCta': 'Start in Drive',

  // --- Búsqueda cross-app ------------------------------------------------
  'search.overline': 'Search',
  'search.heading': 'Search across the whole suite',
  'search.placeholder': 'Search in Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Search in Sytue One',
  'search.submit': 'Search',
  'search.hint':
    'Type above to search your mail, files, documents, events, contacts and rooms all at once. Results open straight in their product.',
  'search.failedTitle': 'The search could not be completed',
  'search.failedBody': 'The Hub is not responding right now. Try again in a few seconds.',
  'search.retry': 'Try again',
  'search.empty': 'No results for “{query}”. Try other words.',
  'search.count': { one: '{count} result', other: '{count} results' },

  // --- Bienvenida --------------------------------------------------------
  'welcome.heading': 'Welcome to Sytue',
  'welcome.lead':
    'A few minutes and you will be up and running. We go step by step; if you stop halfway, we pick up where we left off when you come back.',

  // --- Tus datos: exportar (RGPD art. 20) -------------------------------
  'data.title': 'Your data',
  'data.lead':
    'Download a copy of everything you have in Sytue One, whenever you want and without asking anyone.',
  'data.export.section': 'Download a copy',
  'data.export.body':
    'A single ZIP file with your mail, files, calendars, contacts, notes, documents, photos and messages. Anything that already has a standard format goes as it is: emails as .eml, calendars as .ics, contacts as .vcf and files just as you uploaded them.',
  'data.export.formatLegend': 'Format for the lists',
  'data.export.formatHelp':
    'The lists describe what is in the copy: your messages, events, contacts or files, one per row.',
  'data.export.orgHelp':
    'Each copy belongs to one organisation. If you choose another, you switch to it and the download starts on its own.',
  'data.export.json': 'For other apps and programs.',
  'data.export.csv': 'To open in Excel, Numbers or Google Sheets.',
  'data.export.xml': 'For systems that ask for XML.',
  'data.export.action': 'Download copy',
  'data.export.startedTitle': 'Your download has started',
  'data.export.startedBody':
    'Your browser keeps saving it even if you close this page. If you have many files, it will take a while.',
  'data.export.scope':
    'The copy includes the data of the organisation you are signed in to. The summary file inside says what was included from each app.',
  'data.move.section': 'Move it to another service',
  'data.move.body':
    'To send it straight to Google Workspace or Microsoft 365, without downloading anything: if your account is personal or you administer your organisation, start it yourself from Sytue Admin, in Migrations; otherwise, ask whoever administers it.',
  'data.move.action': 'Go to Migrations',

  // --- Conectar tus dispositivos ----------------------------------------
  'devices.native.body': 'Use your device’s file manager or connect an account with its consent.',
  'devices.native.driveBody':
    'Sytue Drive is added automatically after sign-in. Allow its location if your system asks.',
  'devices.native.openDrive': 'Open in file manager',
  'devices.native.failed': 'Could not update the device integration.',
  'devices.title': 'Connect your devices',
  'devices.lead':
    'Take your Sytue mail, calendar, contacts and files to your phone or your computer. We guide you step by step.',
  'devices.leadShort':
    'Take your Sytue mail, calendar and contacts to your phone or your computer.',
  'devices.unavailable.section': 'We cannot show you the details right now',
  'devices.unavailable.title': 'We could not fetch the configuration',
  'devices.unavailable.body':
    'Try again in a moment. If it stays the same, write to whoever administers Sytue in your organisation.',
  'devices.retry': 'Try again',
  'devices.copy': 'Copy',
  'devices.copyFailTitle': 'It could not be copied',
  'devices.copyFailBody': 'Select it and copy it by hand.',
  'devices.copied.address': 'Your address copied',
  'devices.copied.imap': 'Incoming server copied',
  'devices.copied.port': 'Port copied',
  'devices.copied.smtp': 'Outgoing server copied',
  'devices.copied.url': 'Address copied',
  'devices.copied.drive': 'Drive address copied',
  'devices.oneClick.thunderbird': 'Set up Thunderbird',
  'devices.oneClick.outlook': 'Set up Outlook',
  'devices.oneClick.android': 'Set up on Android',
  'devices.oneClick.body':
    'Creates an app password just for this program, copies it and shows it to you once.',
  'devices.oneClick.failTitle': 'The app password could not be created',
  'devices.oneClick.failBody': 'Try again in a moment. You can also create one in My account.',
  'devices.oneClick.onceTitle': 'Copied. It is shown only this once',
  'devices.oneClick.onceBody':
    'Paste it where the program asks for the password. If you lose it, revoke it in My account and create another.',
  'devices.oneClick.passwordLabel': 'App password',
  'devices.copied.password': 'App password copied',
  'devices.oneClick.notOpened':
    '{program} could not be opened on this computer: open it yourself and follow these steps.',
  'devices.oneClick.next.thunderbird':
    'In Thunderbird’s new account assistant, type your address and paste the password: it finds the servers on its own. Calendars and contacts: add a network calendar and a CardDAV address book with the same address and password (if it asks for the location, copy it from below).',
  'devices.oneClick.next.outlook':
    'In Outlook’s Add account, type your address and paste the password: Outlook finds the servers on its own.',
  'devices.oneClick.next.android':
    'In your mail app, add an “Other (IMAP)” account with your address and paste the password. Calendars and contacts: DAVx5 with the addresses below and the same password.',
  'devices.android.davx5Calendars': 'Calendars in DAVx5',
  'devices.android.davx5Contacts': 'Contacts in DAVx5',
  'devices.android.davx5Install': 'Get DAVx5 from F-Droid',

  'devices.platform.iphone.title': 'iPhone or iPad',
  'devices.platform.iphone.body':
    'You install a profile and mail, calendar and contacts are ready.',
  'devices.platform.iphone.note': 'The fastest way: one file and your password.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body': 'The same profile as on the iPhone, installed from System Settings.',
  'devices.platform.mac.note': 'Apple Mail, Calendar and Contacts.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'You type your address and your password; Outlook finds the rest on its own.',
  'devices.platform.outlook.note': 'Mail only. Calendar and contacts go separately.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'It configures itself from your address alone; calendar and contacts are added.',
  'devices.platform.thunderbird.note': 'Windows, Mac or Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'Your mail app for the mailbox and DAVx5 for calendar and contacts.',
  'devices.platform.android.note': 'No Sytue app: the system ones are used.',
  // Compatibilidad drop-in: Drive, Calendar y Contacts hablan las APIs de Google y
  // de Microsoft Graph. Estaba construido y no se decia en ninguna parte.
  'devices.platform.apis.title': 'A tool that speaks Google or Microsoft',
  'devices.platform.apis.body':
    'Point an integration written for Google Drive, Google Calendar, Google Contacts or Microsoft Graph at Sytue, without changing its code.',
  'devices.platform.apis.note': 'Only where you can set the base URL.',
  'devices.apis.step1.title': 'Change the base address',
  'devices.apis.step1.body':
    'In the tool, replace the address it calls with the one for the product and dialect you need. Everything else in its configuration stays as it is.',
  'devices.apis.step2.title': 'Use your Sytue credentials',
  'devices.apis.step2.body':
    'Authentication is your Sytue ID token, not a Google or Microsoft account: the permissions are the same ones you already have here.',
  'devices.apis.identityGoogle': 'Identity · Google OAuth and directory',
  'devices.apis.step3.title': 'A third-party app, as if it were Google',
  'devices.apis.step3.body':
    'If the tool signs in with Google OAuth, an administrator registers it in Sytue Admin with the Google permissions it needs. The app asks for those permissions and gets Sytue data; Google is not involved and sees nothing.',
  'devices.apis.driveGoogle': 'Drive · Google Drive API',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · Google Calendar API',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · Google People API',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'What is covered',
  'devices.apis.expectBody':
    'Listing, reading, creating, editing and deleting. Anything not covered answers with a clear error, never with an empty success.',
  'devices.copied.apiBase': 'Base address copied',
  'devices.check.apis.label': 'The tool',
  'devices.check.apis.value': 'It lists your Sytue content as if it were the original service.',
  'devices.platform.drive.title': 'File explorer (Drive)',
  'devices.platform.drive.body':
    'You see your Sytue Drive files as one more drive on the computer.',
  'devices.platform.drive.note': 'Finder on Mac, File Explorer on Windows.',

  'devices.mail.address': 'Your address (username)',
  'devices.mail.imapHost': 'Incoming server (IMAP)',
  'devices.mail.imapPort': 'Incoming port',
  'devices.mail.smtpHost': 'Outgoing server (SMTP)',
  'devices.mail.smtpPort': 'Outgoing port',
  'devices.mail.hint':
    'SSL/TLS encryption on both, and the same Sytue password you use to sign in on the web.',
  'devices.dav.caldav': 'Calendar (CalDAV)',
  'devices.dav.carddav': 'Contacts (CardDAV)',
  'devices.dav.user': 'Username',

  'devices.apple.download': 'Download the Sytue profile',
  'devices.apple.step1.title': 'Download the profile',
  'devices.apple.step1.body':
    'Press the button above. A small file with your account settings is downloaded.',
  'devices.apple.step2.iphone.title': 'Install it from Settings',
  'devices.apple.step2.iphone.body':
    'Open Settings. “Profile downloaded” appears right at the top: tap it and press Install.',
  'devices.apple.step2.mac.title': 'Install it from System Settings',
  'devices.apple.step2.mac.body':
    'Open System Settings › Privacy & Security › Profiles, double-click the downloaded profile and press Install.',
  'devices.apple.step3.title': 'No password to type',
  'devices.apple.step3.body':
    'The profile carries its own app password, created just for this device. You can revoke it at any time in My account.',
  'devices.apple.step4.title': 'Done',
  'devices.apple.step4.body':
    'Mail, calendar and contacts are added in one go. There is no server to type in.',
  'devices.apple.manualTitle': 'If you would rather do it by hand',
  'devices.apple.manualBody': 'You can also create the account manually with these details.',
  'devices.manualValues': 'See the manual values',

  'devices.desktop.step1.title': 'Add a mail account',
  'devices.desktop.step1.outlook': 'In Outlook: File › Add account.',
  'devices.desktop.step1.thunderbird': 'In Thunderbird: Accounts › Set up an account › Email.',
  'devices.desktop.step2.title': 'Type only your address and your password',
  'devices.desktop.step2.outlook':
    'Nothing else. Outlook asks Sytue for the settings and fills them in for you.',
  'devices.desktop.step2.thunderbird':
    'Nothing else. Thunderbird asks Sytue for the settings and fills them in for you.',
  'devices.desktop.step3.title': 'Confirm',
  'devices.desktop.step3.body':
    'When it shows you the servers, accept. It connects as an IMAP account.',
  'devices.desktop.emailLabel': 'Your email address',
  'devices.desktop.notFoundTitle': 'Did it not find it on its own?',
  'devices.desktop.notFoundBody': 'Choose manual configuration and use these values.',
  'devices.thunderbird.davTitle': 'Calendar and contacts',
  'devices.thunderbird.davBody': 'In Thunderbird they are added separately, with these addresses.',
  'devices.outlook.davTitle': 'Calendar and contacts in Outlook',
  'devices.outlook.davBody':
    'Outlook for Windows does not support CalDAV/CardDAV, so Sytue calendar and contacts do not sync there. Use them from the Sytue web, or from a client that does speak CalDAV/CardDAV (Thunderbird, Apple Calendar).',

  'devices.android.mailTitle': 'Mail',
  'devices.android.mailBody': 'With the mail app you already use (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Add an account',
  'devices.android.mail1.body': 'Choose “Other” or “Personal (IMAP)”, not “Google”.',
  'devices.android.mail2.title': 'Type your address and your password',
  'devices.android.mail2.body': 'If it asks for the servers, copy them from down here.',
  'devices.android.davTitle': 'Calendar and contacts',
  'devices.android.davBody':
    'Android does not ship CalDAV/CardDAV: you need DAVx5, a free app that adds them to the phone’s own calendar and address book.',
  'devices.android.dav1.title': 'Install DAVx5',
  'devices.android.dav1.body':
    'It is on Google Play and on F-Droid. Sytue neither publishes nor maintains it.',
  'devices.android.dav2.title': 'Add an account with “Login with URL and user name”',
  'devices.android.dav2.body': 'Paste the address below and use your Sytue email and password.',
  'devices.android.dav3.title': 'Choose what to sync',
  'devices.android.dav3.body': 'Tick your calendars and your address book.',

  'devices.drive.step1.title': 'Open the connection to a server',
  'devices.drive.step1.body':
    'On Mac: Finder › Go › Connect to Server (⌘K). On Windows: File Explorer › This PC › Map network drive, and tick “Connect using different credentials”.',
  'devices.drive.step2.title': 'Paste this address',
  'devices.drive.step2.body': 'It is the folder with your Sytue Drive files.',
  'devices.drive.step3.title': 'Sign in with your Sytue account',
  'devices.drive.step3.body': 'Username: your email address. Password: your Sytue one.',
  'devices.drive.urlLabel': 'Drive address (WebDAV)',
  'devices.drive.expectTitle': 'What to expect',
  'devices.drive.windowsExpectBody':
    'On Windows, Drive shows up in File Explorer as the “Sytue Drive” folder. Files download when you open them; right-click › “Always keep on this device” to have them offline. Whatever you save or create there is uploaded to Drive.',
  'devices.drive.expectBody':
    'Files open from the server, they are not copied to the disk. To work offline, download what you need beforehand.',

  'devices.check.mail.label': 'The mail',
  'devices.check.mail.value': 'New messages arrive and you can send one to yourself.',
  'devices.check.dav.label': 'Calendar and contacts',
  'devices.check.dav.value':
    'An event created on the Sytue web shows up on the device within a few minutes.',
  'devices.check.profile.label': 'The profile',
  'devices.check.profile.value': '“Sytue One” appears in the list of installed profiles.',
  'devices.check.drive.label': 'The drive',
  'devices.check.drive.value': 'You see your Drive folders and you can open a file.',
  'devices.check.write.label': 'Writing',
  'devices.check.write.value':
    'You create a folder and it also shows up in Sytue Drive in the browser.',

  'devices.step1.title': 'What you use',
  'devices.step1.heading': 'Where do you want your Sytue account?',
  'devices.step1.description': 'Choose one. If you have several, repeat the wizard for each one.',
  'devices.step2.title': 'How to do it',
  'devices.step2.heading': 'Set up {platform}',
  'devices.step2.headingFallback': 'How to do it',
  'devices.step2.description':
    'Follow these steps. Everything you have to type is copied with a button.',
  'devices.step2.next': 'It is done',
  'devices.step3.title': 'Check',
  'devices.step3.heading': 'How to know that it worked',
  'devices.step3.description': 'If these things are true, it is properly connected.',
  'devices.step3.helpTitle': 'Something does not add up?',
  'devices.step3.helpBody':
    'The password is the usual culprit: use the same one you use to sign in to Sytue in the browser. If you have two-step verification, your organisation may require an app password; ask whoever administers Sytue.',
  'devices.step3.again': 'Connect another device',

  // --- Alta guiada (onboarding) -----------------------------------------
  'ob.error.generic': 'We could not complete the operation. Try again.',
  'ob.failedTitle': 'We could not load your sign-up',
  'ob.failedBody':
    'The identity service is not responding right now. Nothing has been lost: try again in a minute.',
  'ob.retry': 'Try again',
  'ob.preparing': 'Getting your sign-up ready…',
  'ob.lead':
    'A few minutes and you are up and running. We go step by step; if you stop halfway, we pick up where we left off when you come back.',
  'ob.saveFailedTitle': 'It could not be saved',
  'ob.saveFailedBody': 'Try again in a moment.',
  'ob.copy': 'Copy',
  'ob.copyFailTitle': 'It could not be copied',
  'ob.copyFailBody': 'Select it with the mouse and copy it by hand.',
  'ob.copied.recordType': 'Record type copied',
  'ob.copied.recordName': 'Record name copied',
  'ob.copied.recordValue': 'Record value copied',

  'ob.roleOption.member': 'Member — uses the suite, administers nothing',
  'ob.roleOption.admin': 'Administrator — can manage the whole organisation',
  'ob.roleOption.billing': 'Billing — takes care of the invoices',
  'ob.roleName.owner': 'Owner',
  'ob.roleName.admin': 'Administrator',
  'ob.roleName.member': 'Member',
  'ob.roleName.billing': 'Billing',

  'ob.mode.title': 'Start',
  'ob.mode.heading': 'Who is Sytue for?',
  'ob.mode.description': 'This tells us what to ask you. You will be able to change it later.',
  'ob.mode.individual.title': 'Just for me',
  'ob.mode.individual.body':
    'Your mail, your files and your calendar, with your own address. If one day you need to invite someone, you will be able to.',
  'ob.mode.org.title': 'For my organisation',
  'ob.mode.org.body':
    'You sign your team up, share out who can do what, and administer it all from one place.',
  'ob.mode.org.badge': 'Most common',

  'ob.identity.titleOrg': 'Your organisation',
  'ob.identity.titleMe': 'Your account',
  'ob.identity.headingOrg': 'What is your organisation called?',
  'ob.identity.headingMe': 'What do you want your account to be called?',
  'ob.identity.descriptionCreated':
    'It is already created. You can change the name; the address stays as it is.',
  'ob.identity.description': 'With this we create your space and your address in Sytue.',
  'ob.identity.nextCreated': 'Continue',
  'ob.identity.nextOwnDomain': 'Create and see what to add to my domain',
  'ob.identity.next': 'Create and continue',
  'ob.identity.nameLabel': 'Name',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Marta Ruiz',
  'ob.identity.nameHintOrg':
    'It is what your team sees when they sign in. You can change it whenever you want.',
  'ob.identity.nameHintMe': 'Only you see it. You can change it whenever you want.',
  'ob.identity.slugLabel': 'Your address in Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Your address is {subdomain}. It cannot be changed: your mail and your files hang from it.',
  'ob.identity.slugEmpty': 'Choose a short name. It will be your address: acme.{suite}',
  'ob.identity.slugChecking': 'Checking whether it is free…',
  'ob.identity.slugFree': 'Free. Your address will be {subdomain}',
  'ob.identity.slugTaken': 'It cannot be used.',
  'ob.identity.firstTitle': 'First we create your space',
  'ob.identity.firstBody':
    'When you press the button below we create your Sytue address and show you right here the exact record you have to add to your domain. You do not need to have it ready now.',

  'ob.domain.legend': 'And your own domain?',
  'ob.domain.keep.title': 'I will keep {host}',
  'ob.domain.keep.placeholder': 'my-name',
  'ob.domain.keep.body':
    'You do not have to touch anything: publishing it is on us. It is what we recommend to start with.',
  'ob.domain.own.title': 'I already have a domain and I want to use it (acme.com)',
  'ob.domain.own.body':
    'If it is on Cloudflare we set it up for you. If not, we tell you the exact record to add and we check it right here.',
  'ob.domain.search.title': 'I do not have a domain yet',
  'ob.domain.search.body':
    'We check whether the name you want is free and how much it costs per year. It is only a lookup: nothing is bought here.',
  'ob.domain.hostLabel': 'Your domain',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Write it as it is: acme.com, without “https://” and without “www”.',
  'ob.domain.verified': '{host} verified',

  'ob.dns.legend': 'Where is {domain}?',
  'ob.dns.fallback': 'your domain',
  'ob.dns.cf.title': 'On Cloudflare — set it up for me',
  'ob.dns.cf.body':
    'You give us temporary permission on your account and we create the records ourselves. Before touching anything we show you what we are going to change.',
  'ob.dns.cf.badge': 'Without touching DNS',
  'ob.dns.manual.title': 'With another provider — I will do it myself',
  'ob.dns.manual.body':
    'We give you the exact record to copy into your DNS panel and we check it here as soon as you have it.',
  'ob.dns.manualSummary': 'I would rather add the records myself',

  'ob.people.title': 'Your team',
  'ob.people.heading': 'Who do you invite first?',
  'ob.people.description':
    'Each person gets an email with a personal link to create their password and sign in. You can skip this step and do it later.',
  'ob.people.next': 'Continue',
  'ob.people.emailsLabel': 'Email addresses',
  'ob.people.emailsPlaceholder': 'ana@acme.com, bruno@acme.com',
  'ob.people.emailsHint': 'You can paste several at once, separated by commas or line breaks.',
  'ob.people.roleLabel': 'Which role they join with',
  'ob.people.roleHint': 'It can be changed at any time from Sytue Admin.',
  'ob.people.send': 'Send invitations',
  'ob.people.sendNote': 'Nothing is sent until you press here.',
  'ob.people.pendingTitle': 'Invitations waiting to be accepted',
  'ob.people.emptyTitle': 'You have not invited anyone yet',
  'ob.people.emptyBody':
    'You can do it now or later, from Sytue Admin. Your organisation works just the same.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Invitation sent',
  'ob.invite.already': 'Already in your organisation',
  'ob.invite.invalid': 'That does not look like an email address',
  'ob.invite.notSent': 'The email could not be sent',
  'ob.invite.failed': 'They could not be invited',
  'ob.invite.default':
    'They will get an email with a personal link to create their password and sign in.',

  'ob.billing.title': 'Billing',
  'ob.billing.heading': 'Who are the invoices made out to?',
  'ob.billing.description':
    'Add your invoice details and choose the paid seats. Access starts only after Mollie confirms the payment.',
  'ob.billing.next': 'Continue',
  'ob.billing.pay': 'Pay securely with Mollie',
  'ob.billing.verify': 'Verify payment',
  'ob.billing.legalLabel': 'Legal name',
  'ob.billing.legalPlaceholder': 'ACME Europe Ltd.',
  'ob.billing.legalHint': 'The legal name of the company, exactly as it appears on your invoices.',
  'ob.billing.taxLabel': 'Tax identification',
  'ob.billing.taxPlaceholder': 'B12345678',
  'ob.billing.taxHint': 'The tax or VAT number of your country.',
  'ob.billing.addressLabel': 'Address',
  'ob.billing.addressPlaceholder': 'Calle Mayor 1, 28013 Madrid, Spain',
  'ob.billing.addressHint': 'The registered address that goes on the invoice.',
  'ob.billing.emailLabel': 'Email for invoices',
  'ob.billing.emailPlaceholder': 'invoices@acme.com',
  'ob.billing.emailHint': 'That is where we send the invoices. By default, your account email.',
  'ob.billing.note':
    'We only ask for what is needed to invoice you. None of this is shared with anyone.',
  'ob.billing.seatsLabel': 'Paid seats',
  'ob.billing.seatsHint': '{seats} seat(s) × {price}/User/Month · total {total}/Month.',
  'ob.billing.active': 'Subscription active',
  'ob.billing.activeBody': 'Mollie has confirmed the payment. Your Sytue One access is enabled.',
  'ob.billing.pending': 'We are verifying the payment',
  'ob.billing.pendingBody':
    'Mollie has not confirmed the payment yet. Wait a few seconds and verify again; you will not be charged twice.',

  'ob.roles.title': 'Permissions',
  'ob.roles.heading': 'Do you want to share out administration tasks?',
  'ob.roles.description':
    'These are ready-made roles: you choose the ones you want and we create them. Afterwards you assign them to whoever you like from Sytue Admin. You can skip this step.',
  'ob.roles.next': 'Continue',
  'ob.roles.already': 'Already created',
  'ob.roles.seePermissions': 'See the exact permissions',
  'ob.roles.helpTitle': 'You stay in control',
  'ob.roles.helpBody':
    'Your account is the owner: these roles take nothing away from it. You can edit or delete them whenever you want in Sytue Admin → Roles.',

  'ob.done.title': 'Done',
  'ob.done.headingCompleted': 'All set',
  'ob.done.heading': 'This is what you have set up',
  'ob.done.description': 'It comes from your real account, not from what you typed here.',
  'ob.done.next': 'Enter Sytue',
  'ob.done.warnTitle': '{host} is not verified yet',
  'ob.done.warnBody':
    'Sytue works just the same meanwhile, with {subdomain}. When you add the record to your domain, come back to the “{step}” step and press “Check now”.',
  'ob.done.nextTitle': 'Where to go next',
  'ob.next.devices.title': 'Connect your phone and your computer',
  'ob.next.devices.body':
    'Leave mail, calendar and contacts working in the applications you have always used.',
  'ob.next.migrations.title': 'Bring your content from Microsoft 365 or Google',
  'ob.next.migrations.body':
    'We copy mail, files and calendars without you losing anything along the way.',
  'ob.next.apps.title': 'Connect your applications',
  'ob.next.apps.body': 'The tools your team already uses, linked to your Sytue account.',
  'ob.next.users.title': 'Carry on signing your team up',
  'ob.next.users.body': 'Invite whoever is missing and review who has which role.',

  'ob.sum.org': 'Organisation',
  'ob.sum.account': 'Your account',
  'ob.sum.address': 'Address',
  'ob.sum.addressLive': '{host} · published',
  'ob.sum.addressPending': '{host} · waiting to be published',
  'ob.sum.ownDomain': 'Own domain',
  'ob.sum.domainVerified': '{host} · verified',
  'ob.sum.domainPending': '{host} · waiting for the record to appear',
  'ob.sum.noDomain': 'No own domain',
  'ob.sum.invitations': 'Invitations',
  'ob.sum.invitationsPending': {
    one: '1 waiting to be accepted',
    other: '{count} waiting to be accepted',
  },
  'ob.sum.noInvitations': 'None yet',
  'ob.sum.billing': 'Billing',
  'ob.sum.billingEmpty': 'Not filled in (not needed yet)',
  'ob.sum.roles': 'Roles created',
  'ob.sum.edit': 'Edit',
  'ob.sum.noRoles': 'None',

  'ob.sub.liveTitle': 'Your address is already published',
  'ob.sub.liveBody': '{host} resolves on the internet. You do not have to do anything else.',
  'ob.sub.failedTitle': 'Your address is not published yet',
  'ob.sub.failedDefault': 'We could not publish it at this moment.',
  'ob.sub.failedTail':
    'Your account is created: carry on with the sign-up and come back in a while.',
  'ob.sub.reservedTitle': 'Your address is reserved',
  'ob.sub.reservedBody':
    '{host} is yours. Publishing it on the internet is on us: you do not have to do anything.',

  'ob.search.label': 'The domain you would like',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Write it in full, with its extension: acme.com, acme.es, acme.eu…',
  'ob.search.check': 'Check availability',
  'ob.search.useOwn': 'I have already registered it: connect it',
  'ob.search.note':
    'Domains are not sold at Sytue: here we only tell you whether it is free and how much it costs. Register it wherever you prefer and come back to this step, under “I already have a domain”, to connect it to your account.',
  'ob.avail.freeTitle': '{domain} is free',
  'ob.avail.freePrice':
    'It costs {registration} for the first year and {renewal} on each renewal. That is today’s price at Cloudflare; wherever you register it, it may be another.',
  'ob.avail.freeNoPrice':
    'It can be registered. The registrar where you buy it will tell you the price.',
  'ob.avail.takenTitle': '{domain} is already registered',
  'ob.avail.takenBody':
    'Somebody has it. Try another name, or another extension (.es, .eu, .net…). If the domain is yours, choose “I already have a domain” above and we will connect it.',
  'ob.avail.unsupportedTitle': 'We cannot look up that extension',
  'ob.avail.unsupportedBody':
    'We cannot tell you whether it is free or how much it costs: check it at your registrar. When you have it, come back here and choose “I already have a domain”.',
  'ob.avail.invalidTitle': 'That does not look like a domain',
  'ob.avail.invalidBody':
    'Write it like acme.com: a name, a dot and the extension. Without “https://” and without “www”.',
  'ob.avail.unconfiguredTitle': 'We do not look up domain prices here',
  'ob.avail.unconfiguredBody':
    'Search for it at the registrar you prefer. When you have it, come back to this step and choose “I already have a domain” to connect it.',
  'ob.avail.unknownTitle': 'We could not look it up right now',
  'ob.avail.unknownBody':
    'It is not the fault of what you typed: we got no answer. Try again in a moment.',

  'ob.dnsRec.title': 'Add this record where your domain is registered',
  'ob.dnsRec.body':
    'Look for the “DNS” or “Records” section at your provider, create a TXT record and copy these two values. The procedure is the same everywhere.',
  'ob.dnsRec.type': 'Type',
  'ob.dnsRec.name': 'Name',
  'ob.dnsRec.value': 'Value',
  'ob.dnsRec.verifiedTitle': 'Domain verified',
  'ob.dnsRec.verifiedBody': 'We found the record: {host} is yours and it is registered as such.',
  'ob.dnsRec.mismatchTitle': 'There is a record, but not this one',
  'ob.dnsRec.mismatchBody':
    'At {name} we found “{found}”. Check that you copied the whole value and that no old one has been left behind.',
  'ob.dnsRec.unreachableTitle': 'We could not query your DNS',
  'ob.dnsRec.unreachableBody':
    'It is not your record’s fault: the name server did not answer. Try again in a moment.',
  'ob.dnsRec.pendingTitle': 'We do not see the record yet',
  'ob.dnsRec.pendingBody':
    'If you have just created it, that is normal: DNS changes take between a few minutes and a few hours to show up. You can carry on with the sign-up and check again whenever you want.',
  'ob.dnsRec.uncheckedTitle': 'Not checked yet',
  'ob.dnsRec.uncheckedBody':
    'When you have added the record at your provider, press “Check now” and we look at your DNS straight away.',
  'ob.dnsRec.empty': 'When you press “Save and check” we give you the exact record to add.',
  'ob.dnsRec.check': 'Check now',
  'ob.dnsRec.save': 'Save and check',

  'ob.cf.state.ok': 'Already in place',
  'ob.cf.state.created': 'Created',
  'ob.cf.state.replaced': 'Replaced',
  'ob.cf.state.missing': 'We add it',
  'ob.cf.state.conflict': 'Something is already there',
  'ob.cf.state.failed': 'Could not be done',
  'ob.cf.state.pending': 'Not yet',
  'ob.cf.permTitle': 'Give us permission for your DNS only',
  'ob.cf.step1': 'Open your Cloudflare API tokens and press “Create token”.',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2': 'Choose “Create custom token” and tick a single permission: Zone · DNS · Edit.',
  'ob.cf.step3': 'Under “Zone resources” choose {domain}, create the token and paste it here.',
  'ob.cf.tokenLabel': 'Cloudflare token',
  'ob.cf.tokenPlaceholder': 'The token you have just created',
  'ob.cf.tokenHint':
    'It only works for the DNS of this domain. We use it and discard it: it is not stored unless you ask us to below, and it never leaves our servers.',
  'ob.cf.mailLabel': 'Also receive the mail for @{domain} in Sytue Mail',
  'ob.cf.mailHintOn':
    'This changes where your domain’s mail arrives. If you receive it somewhere else today, we will warn you before replacing anything.',
  'ob.cf.mailHintOff': 'Turn it on only if you want your @your-domain mailboxes to move to Sytue.',
  'ob.cf.rememberLabel': 'Remember this permission for future changes',
  'ob.cf.rememberHint':
    'If you leave it unticked, you will have to paste it again next time. Ticking it stores it encrypted; unticking it deletes it.',
  'ob.cf.preview': 'See what we are going to change',
  'ob.cf.previewAgain': 'Look again',
  'ob.cf.nothing': 'Nothing left to do',
  'ob.cf.apply': { one: 'Apply 1 change', other: 'Apply {count} changes' },
  'ob.cf.zone': 'Zone found',
  'ob.cf.token': 'Your token',
  'ob.cf.tokenStored': 'Stored encrypted for future changes',
  'ob.cf.tokenDiscarded': 'Used and discarded (we did not store it)',
  'ob.cf.domainState': 'Domain status',
  'ob.cf.verified': 'Verified',
  'ob.cf.pending': 'Waiting to be verified',
  'ob.cf.nowThere': 'Right now there is: {current}',
  'ob.cf.replaceLabel': 'Replace it with Sytue’s',
  'ob.cf.failTitle': 'Something was left unset',
  'ob.cf.failBody':
    'The rest is in place. Above you have the reason for each record that failed and its value, to add it by hand from Cloudflare.',
  'ob.cf.conflictTitle': 'There are records of yours that we will not touch',
  'ob.cf.conflictBody':
    'We never replace anything you already have unless you tick it. If you would rather combine them by hand, up there you have the current value and ours.',
  'ob.cf.okTitle': 'Domain verified',
  'ob.cf.okBody':
    'We saw the record in your own zone: {host} is yours and it is registered as such.',
  'ob.cf.todoTitle': 'The changes still need to be applied',
  'ob.cf.todoBody':
    'We have not touched your zone yet. Press “Apply” when what is above looks right to you.',
  'ob.cf.someFailed': {
    one: '1 record was left unset',
    other: '{count} records were left unset',
  },
  'ob.cf.someFailedBody': 'Below you have the reason for each one and the values to do it by hand.',
  'ob.cf.configured': '{host} configured and verified',
  'ob.cf.created': 'Records created in your zone',
  'today.title': 'Today',
  'today.openCalendar': 'Open the calendar',
  'today.noEvents': 'Nothing on the agenda for today.',
  'today.unavailable': 'The agenda is not available right now.',
  'today.allDay': 'All day',
  'today.untitled': '(no title)',
  'today.unread': { one: '1 unread email', other: '{count} unread emails' },
  'today.inboxZero': 'Inbox up to date',
  'launcher.quickActions': 'Quick actions',
  'launcher.recentAll': 'Recent across the suite',
  'launcher.activityError': 'We could not load your activity',
  'launcher.noActivity': 'No recent activity',
  'ob.dns.verified': 'Domain verified',
  'ob.dns.mismatch': 'There is a record, but not this one',
  'ob.dns.unreachable': 'We could not query your DNS',
  'ob.dns.notYet': 'We do not see the record yet',
  'ob.dns.unchecked': 'Not checked yet',

  // --- Onboarding: rótulos que estaban en duro ---
  'ob.dns.type': 'Type',
  'ob.dns.name': 'Name',
  'ob.dns.value': 'Value',
  'ob.cf.tokenField': 'Cloudflare token',
  'ob.cf.remember': 'Remember this permission for future changes',
  'ob.cf.replaceWithSytue': 'Replace it with Sytue’s',
  'recent.hubDown': 'The Hub is not responding right now. Your work is safe; try again.',
  'search.partial':
    '{sources} did not respond, so some results may be missing. Try again in a moment.',
  'search.partialJoin': ' and ',
  'search.showing': 'Showing {count} of {total} results.',
  'privacy.notice':
    'We do not sell your data or share it with third parties. We do not use it to profile you, for advertising, or to train or improve our services or anyone else’s. It is encrypted in transit and at rest, and only leaves our European infrastructure where strictly needed to bill you and in the integrations you enable, where the third party will ask your approval for the data it will use.',
};
