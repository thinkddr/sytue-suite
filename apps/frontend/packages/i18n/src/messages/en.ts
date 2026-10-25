/**
 * Catálogo INGLÉS — la fuente de verdad (docs/CONVENTIONS.md §13.1). Las cadenas
 * nacen aquí y los otros cuatro catálogos son traducciones de este original; el
 * tipo `Catalog` los obliga a tener EXACTAMENTE estas claves, así que añadir una
 * clave aquí rompe la compilación de es/fr/de/it hasta que se traduzca.
 *
 * Convenios: claves `dominio.cosa`, variables entre llaves (`{name}`), y valor
 * objeto `{ one, other, ... }` cuando el texto depende de una cantidad (la
 * categoría la elige `Intl.PluralRules`, no un `n === 1`).
 */
export const en = {
  // Genéricos del cromo
  'action.retry': 'Retry',
  'error.connection': 'Check your connection and try again.',

  // Etiquetas de accesibilidad del design system. Las pinta @sytue/ui en las
  // diez apps y solo las oye un lector de pantalla: sin traducir, un usuario
  // alemán escucha «Cerrar» y «Cargando» en medio de una interfaz en alemán.
  'ui.save': 'Save',
  'ui.cancel': 'Cancel',
  'ui.close': 'Close',
  // Se anuncia junto a la etiqueta de todo campo obligatorio de la suite.
  'ui.required': '(required)',
  'ui.loading': 'Loading',
  'ui.selectRow': 'Select row',
  'ui.dismissNotification': 'Dismiss notification',
  'ui.mainNav': 'Main navigation',
  'ui.openNav': 'Open navigation',
  'ui.closeNav': 'Close navigation',
  'ui.backToList': 'Back to the list',
  'ui.noResults': 'No results.',

  // Medidor de uso (Meter). El aviso se dice TAMBIÉN con palabras: un umbral que
  // solo existe como color no llega a quien no lo percibe (WCAG 1.4.1).
  'ui.meter.almostFull': 'almost full',
  'ui.meter.full': 'full',

  // Barra de acciones en lote (BulkActionBar).
  'ui.bulk.region': 'Actions for the selection',
  'ui.bulk.clear': 'Clear selection',

  // Wizard chrome. It lives here and not in each app because the wizard is a
  // system component: hard-coding its buttons meant every wizard in the suite
  // showed them in Spanish, whatever language the user had chosen.
  'ui.wizard.back': 'Back',
  'ui.wizard.next': 'Continue',
  'ui.wizard.cancel': 'Cancel',
  'ui.wizard.progress': 'Progress',
  'ui.wizard.goToStep': 'Back to step {n}: ',
  'ui.wizard.stepOf': 'Step {n} of {total}: ',
  'ui.review.edit': 'Edit',

  // Shell
  'shell.skipToContent': 'Skip to content',
  'shell.appSwitcher': 'Sytue One — suite apps',
  'shell.apps': 'Sytue One apps',
  'shell.shortcutHint': 'Shortcut: {keys}',
  // Solo las apps nativas: la lista de inicio del sistema y montar Drive como disco.
  'shell.launchAtLogin': 'Open at login',
  'shell.mountDrive': 'Connect Drive to this computer',
  'shell.update.check': 'Check for updates',
  'shell.update.install': 'Update to version {version}',
  'shell.update.available': 'A new version of {app} is available',
  'shell.update.availableBody': 'Version {version} is ready. Click to install it.',
  'shell.update.upToDate': '{app} is up to date',
  'shell.update.current': 'Installed version: {version}.',
  'shell.update.failed': '{app} could not be updated',
  'shell.update.failedBody': 'Try again later or download it from the portal.',
  'shell.keyring.missing': 'This computer has no keyring',
  'shell.keyring.missingBody':
    'Your Sytue session is stored encrypted in a file only you can access. Turn on KWallet or GNOME Keyring to protect it better.',
  'shell.lock.toggle': 'Require {metodo} to open',
  'shell.lock.devicePassword': 'the computer password',
  'shell.lock.reason': 'unlock your Sytue session',
  'shell.lock.signInTitle': 'Sign in with your Sytue account',
  'shell.lock.signInBody':
    "This computer can't confirm it's you with a fingerprint, face or its password. To open the app with the lock on, sign in with your Sytue account.",
  'shell.lock.rememberNotebook': 'Remember with {metodo} on this device',
  'shell.documentsProvider.openApp': 'Open Sytue Drive to see your files here.',
  'shell.graphics.unsupported':
    'This app needs OpenGL 2.1 and this computer’s graphics driver does not offer it ({detalle}). Install or update the graphics card driver. Without a graphics card (remote desktop or virtual machine) it cannot open.',
  'shell.lock.reasonNotebook': 'open your encrypted notebook',
  'shell.lock.faceIdUsage': 'Sytue uses Face ID to unlock your session when you open the app.',
  'shell.call.microphoneUsage':
    'Sytue Rooms uses the microphone only during a call, and only when you turn it on.',
  'shell.call.cameraUsage':
    'Sytue Rooms uses the camera only during a call, and only when you turn it on.',
  'offline.status': 'Offline',
  'offline.description':
    'Showing content saved on this device. Online-only actions will be available after reconnecting.',

  // Organización sin derecho de uso (el token sale sin `tenant`)
  'noOrg.title': 'Your organization has no active subscription',
  'noOrg.description':
    'You are signed in, but the suite cannot load your data until the subscription is completed.',
  'noOrg.action': 'Complete the subscription',

  // Puerta cerrada: sesión válida SIN el rol/permiso que la página exige (403).
  // La devuelve el guard compartido, así que sirve a las 10 apps de la suite.
  'forbidden.title': 'You do not have access to this page',
  'forbidden.description':
    'You are signed in, but your account does not have the permissions this section requires. Ask an administrator of your organisation to grant them.',
  'forbidden.action': 'Back to Sytue One',
  // Solo organizaciones: una cuenta personal que llega a Sytue ERP o Sytue Web.
  'forbidden.org_only.title': 'For organisations only',
  'forbidden.org_only.body':
    'This app is for organisations, and you are signed in with a personal account. Create an organisation to use it.',
  'forbidden.org_only.action': 'Create an organisation',

  // Ruta inexistente (404): la página que inyecta `sytueNoEncontrada()` en las
  // superficies SSR. Mismos textos que el 404 de www, sin la invitación a la suite.
  'notFound.overline': 'Error 404',
  'notFound.title': 'We could not find this page',
  'notFound.description': 'The link may have changed.',
  'notFound.action': 'Back to home',

  // Búsqueda global
  'search.placeholder': 'Search Sytue One…',
  'search.label': 'Global search',
  'search.busy': 'Searching',
  'search.results': 'Search results',
  'search.resultCount': { one: '{count} result', other: '{count} results' },
  'search.failedShort': 'Search failed',
  'search.failed': 'The search could not be completed',
  'search.emptyTitle': 'No results for “{query}”',
  'search.emptyDescription': 'Try other words or check the spelling.',
  'search.partial': '{sources} did not respond, so some results may be missing.',
  'search.ai.applied': 'Understood as',
  'search.ai.busy': 'Working out what you mean…',
  'search.ai.literal': 'Search the exact words',
  'search.ai.source': 'in {name}',
  'search.ai.kind': 'only {kind}',
  'search.ai.range': '{from} – {to}',
  'search.ai.since': 'since {from}',
  'search.ai.until': 'until {to}',
  'search.kind.file': 'files',
  'search.kind.message': 'messages',
  'search.kind.event': 'events',
  'search.kind.contact': 'contacts',
  'search.kind.note': 'notes',
  'search.kind.channel': 'channels',

  // Notificaciones
  'notifications.title': 'Notifications',
  'notifications.viewAll': 'View all notifications',
  'notifications.historic': 'Before the notification log',
  'notifications.loadMore': 'Load more',
  'notifications.noneLoaded': 'None among those loaded.',
  'notifications.buttonLabel': {
    one: 'Notifications ({count} unread)',
    other: 'Notifications ({count} unread)',
  },
  'notifications.unread': { one: '{count} unread', other: '{count} unread' },
  'notifications.loading': 'Loading notifications…',
  'notifications.failed': 'Notifications could not be loaded',
  'notifications.emptyTitle': 'All caught up',
  'notifications.emptyDescription': 'You have no new notifications.',
  'notifications.mailUnread': { one: '1 unread email', other: '{count} unread emails' },
  'notifications.markAllRead': 'Mark all as read',
  'notifications.kind.mail.message.received': 'New email from {from}',
  'notifications.kind.calendar.invitation': '{actor} invited you to “{title}”',
  'notifications.kind.calendar.reminder.due': 'Reminder: {title}',
  'notifications.kind.notes.reminder.due': 'Reminder: {title}',
  'notifications.kind.notes.reminder.untitled': 'Note reminder',
  'notifications.kind.rooms.message': '{actor} sent you a message',
  'notifications.kind.rooms.mention': '{actor} mentioned you',
  'notifications.kind.rooms.report': 'New report in Sytue Rooms',
  'notifications.kind.rooms.invitation': '{actor} invited you to the channel “{channel}” in Rooms',
  'notifications.kind.rooms.invitation.direct': '{actor} wants to chat with you in Rooms',
  'notifications.kind.cast.report': 'New report in Sytue Cast',
  'notifications.kind.cast.live': '{actor} is live: {title}',
  'notifications.kind.drive.share.created': '{actor} shared “{name}” with you',
  'notifications.kind.docs.shared': '{actor} shared “{name}” with you',
  'notifications.kind.docs.comment': '{actor} commented on “{name}”',
  'notifications.kind.bases.comment': '{actor} commented on “{name}”',
  'notifications.kind.docs.mention': '{actor} mentioned you in “{name}”',
  'notifications.kind.bases.mention': '{actor} mentioned you in “{name}”',
  'notifications.kind.bases.transferred': '{actor} transferred the base “{name}” to you',
  'notifications.kind.docs.access.requested': '{actor} is asking to edit “{name}”',
  'notifications.kind.docs.signature.requested': '{actor} is asking you to sign “{name}”',
  'notifications.kind.docs.signature.completed': '{actor} signed “{name}”',
  'notifications.kind.docs.signature.declined': '{actor} declined to sign “{name}”',
  'notifications.kind.docs.access.granted': '{actor} gave you edit access to “{name}”',
  'notifications.kind.erp.task.assigned': '{actor} assigned you “{title}”',
  'notifications.kind.security.signin.new': 'New sign-in to your account on {device}',
  'notifications.kind.security.password.changed': 'Your account password was changed',
  'notifications.kind.security.mfa.enabled': 'Two-step verification was turned on',
  'notifications.kind.security.mfa.disabled': 'Two-step verification was turned off',
  'notifications.kind.security.passkey.added': 'A passkey was added to your account',
  'notifications.kind.security.passkey.removed': 'A passkey was removed from your account',
  'notifications.kind.drive.quota.warning': 'Your storage is {percent}% full',
  'notifications.kind.pages.order.paid': 'New order {number} on {site}',
  'notifications.kind.pages.order.oversold': 'Order {number} on {site} was paid without stock',
  'notifications.kind.pages.payment.duplicated':
    'Duplicate payment on order {number} on {site}: the second one is being refunded',
  'notifications.kind.pages.withdrawal.requested':
    'Withdrawal requested for order {number} on {site}',
  'notifications.kind.pages.form.received': 'New form submission on {site}',
  'notifications.kind.pages.domain.active':
    '{domain} is active: publish again to make it the main address',
  'notifications.kind.pages.domain.failed':
    'The domain {domain} on {site} is not working: check its settings',
  'notifications.kind.pages.erp.failed': 'Order {number} on {site} did not reach Sytue ERP',
  'notifications.kind.pages.settings.changed': '{actor} changed a Sytue Web setting',
  'notifications.kind.pages.export.ready': 'Your export of {site} is ready',
  'notifications.kind.pages.store.busy': 'The shop on {site} has too many orders awaiting payment',
  'notifications.kind.one.update.available': '{app} {version} is available',

  // Cuenta y preferencias
  'account.menu': 'Account',
  'account.menuFor': '{name}’s account',
  'account.organization': 'Organisation: {tenant}',
  'account.switchOrg': 'Switch organisation',
  'account.switchOrgFailed': 'Could not switch organisation',
  'account.presence': 'Presence',
  'account.presenceActive': ' (active)',
  'account.presenceFailed': 'Your presence could not be changed',
  'account.appearance': 'Appearance',
  'account.appearance.system': 'System',
  'account.appearance.light': 'Light',
  'account.appearance.translucent': 'Translucent',
  'account.appearance.dark': 'Dark',
  'account.color': 'Colour',
  'account.color.petroleo': 'Petrol blue',
  'account.color.cobalto': 'Cobalt blue',
  'account.color.app': 'Per app',
  'account.forma': 'Layout',
  'account.forma.marco': 'Frame',
  'account.forma.carril': 'Rail',
  'account.followOrg': 'Organisation',
  'account.language': 'Language',
  'account.languageRestart': 'It will apply the next time you open the app',
  'account.myAccount': 'My account (Sytue ID)',
  'account.signIn': 'Sign in',
  'account.signedOutTitle': "You're not signed in",
  'account.signedOutDescription': 'Sign in with your Sytue One account to see your data here.',
  'account.signOut': 'Sign out',
  'account.signOutFailed':
    'Could not sign out on this device. Drive may still be connected. Try again.',
  'account.signOutPending.title': 'Notes not uploaded',
  'account.signOutPending.body':
    'Notepads has notes on this device that have not been uploaded yet ({count}). Signing out deletes them from it.',
  'account.signOutPending.upload': 'Upload and sign out',
  'account.signOutPending.discard': 'Discard and sign out',
  'presence.online': 'Online',
  'presence.away': 'Away',
  'presence.dnd': 'Do not disturb',
  'presence.offline': 'Offline',

  // Apps de la suite (el nombre es marca y NO se traduce; la descripción sí)
  'app.one.description': 'Suite portal and launcher',
  'app.mail.description': 'Email',
  'app.drive.description': 'Files and collaboration',
  'app.calendar.description': 'Calendar',
  'app.contacts.description': 'Contacts',
  'app.docs.description': 'Text documents',
  'app.notes.description': 'Notes, notebooks and ideas',
  'app.photos.description': 'Photos, albums and editing',
  'app.erp.description': 'Customers, invoices and the whole business',
  'app.chat.description': 'Team messaging',
  'app.meet.description': 'Video calls and meetings',
  'app.rooms.description': 'Channels, messages and calls',
  'app.cast.description': 'Video on demand and live streaming',
  'app.pages.description': 'Websites and online shops for your business',
  'app.bases.description': 'Collaborative relational tables',
  'app.cloud.description': 'Application platform (PaaS)',
  'app.console.description': 'Administration and organisations',
  'app.hub.description': 'Integration: search and notifications',
  'app.id.description': 'Identity, profile and security',
  'app.sheets.description': 'Spreadsheets',
  'app.slides.description': 'Presentations',
  'app.pdf.description': 'PDF: read and annotate',
  'app.disabled': 'Disabled',

  // Asistente de importacion: traerse uno sus propios datos (@sytue/shell ImportData).
  'import.title': 'Bring your data across',
  'import.help':
    'Copy what you already have in your old provider into Sytue. You can keep working while it runs.',
  'import.fromGoogle': 'Bring it from Google',
  'import.fromMicrosoft': 'Bring it from Microsoft 365',
  'import.running': 'Copying your data… {count} items so far.',
  'import.done': '{count} items brought across, {skipped} left out.',
  'import.someFailed': 'Some of your data could not be copied. Your administrator can see why.',
  'import.failed': 'The import could not be started. Try again in a moment.',

  // Qué pasa con los datos al conectar algo con un tercero (@sytue/ui DataFlow):
  // entra, sale o en los dos sentidos, con la política que aplica a cada lado.
  'dataflow.label': 'What happens to the data',
  'dataflow.in.title': 'Only comes into Sytue One',
  'dataflow.in.body':
    'Data comes from {partner} into Sytue One. Nothing from Sytue One goes to {partner}.',
  'dataflow.out.title': 'Goes out to {partner}',
  'dataflow.out.body':
    'Data goes from Sytue One to {partner}. Sytue One receives nothing from {partner}.',
  'dataflow.out.short': 'Leaves Sytue One',
  'dataflow.both.title': 'Both ways',
  'dataflow.both.body': 'Data moves between Sytue One and {partner} in both directions.',
  'dataflow.none.title': 'No data exchanged',
  'dataflow.none.body':
    'Only a shortcut to {partner} is added to the launcher. No data is sent or received.',
  'dataflow.incoming': 'Comes into Sytue One',
  'dataflow.outgoing': 'Goes out to {partner}',
  'dataflow.policy.ours': 'What comes in is handled under the Sytue One privacy policy.',
  'dataflow.policy.theirs': 'What goes out is subject to the privacy policy of {partner}.',
  'dataflow.policy.link': 'Privacy policy',
  'dataflow.item.identity': 'Name and email',
  'dataflow.item.people': 'Users and groups',
  'dataflow.item.directory': 'Organisation directory',
  'dataflow.item.mail': 'Email',
  'dataflow.item.files': 'Files',
  'dataflow.item.events': 'Calendar',
  'dataflow.item.contacts': 'Contacts',
  'dataflow.item.messages': 'Messages',
  'dataflow.item.records': 'App data',
  'dataflow.item.documents': 'Documents',
  'dataflow.item.code': 'Code repositories',
  'dataflow.item.cloud': 'Cloud resources',
  // Franja de solo lectura (ReadOnlyBar / ui.FranjaSoloLectura).
  'readOnly.requestEdit': 'Request edit access',
  'readOnly.requestSent': 'Request sent',
  'readOnly.requestFailed': "The request couldn't be sent. Try again.",
  'readOnly.editRequest': '{name} is asking for edit access.',
  'readOnly.grant': 'Grant edit access',
  'readOnly.decline': 'Decline',
  'notifications.kind.notes.access.requested': '{actor} is asking to edit one of your notebooks',
  'notifications.kind.notes.access.granted': '{actor} gave you edit access to a notebook',
  'notifications.kind.mail.access.requested': '{actor} is asking to write from a shared mailbox',
  'notifications.kind.mail.access.granted': '{actor} lets you write from a shared mailbox',
  'readOnly.edit': 'Edit',
  'readOnly.protectedSheet':
    "This sheet is protected: its cells can't be changed until it's unprotected.",
  'readOnly.password': 'Password',
  'notifications.kind.erp.access.requested': '{actor} is asking to edit “{name}”',
  'notifications.kind.erp.access.granted': '{actor} gave you edit access to “{name}”',
  'readOnly.module': 'Read-only in “{name}”: you can see it, but not change it.',
  'notifications.kind.drive.access.requested': '{actor} is asking to edit “{name}”',
  'notifications.kind.drive.access.granted': '{actor} gave you edit access to “{name}”',
  'readOnly.sharedWithYou': '“{name}” is shared with you read-only.',
  'notifications.kind.calendar.access.requested': '{actor} is asking to edit “{name}”',
  'notifications.kind.calendar.access.granted': '{actor} gave you edit access to “{name}”',
  'sharePeople.title': 'Share “{name}”',
  'sharePeople.legend': 'Share with someone in your organisation',
  'sharePeople.email': 'Email address',
  'sharePeople.placeholder': 'name@yourcompany.com',
  'sharePeople.hint': 'They open it signed in — no link, no password.',
  'sharePeople.access': 'Access',
  'sharePeople.add': 'Share',
  'sharePeople.added': 'Shared with {email}.',
  'sharePeople.unknown': 'Nobody in your organisation uses that address.',
  'sharePeople.failed': 'It could not be shared.',
  'sharePeople.someone': 'Someone in your organisation',
  'sharePeople.done': 'Done',
  'sharePeople.read': 'Can view',
  'sharePeople.write': 'Can edit',
  'sharePeople.remove': 'Revoke',
  'sharedWithMe.title': 'Shared with me',
  'sharedWithMe.empty': 'Nobody has shared anything with you yet',
  'sharedWithMe.by': 'Shared by {name}',
  'sharedWithMe.readOnly': 'Read only',
  'sharePeople.open': 'Share with people',
  'sharePeople.heading': 'Who it is shared with',
  'sharePeople.empty': 'You have not shared it with anyone yet.',
  'sharePeople.loadFailed': 'It could not be loaded who it is shared with.',
  'sharePeople.removed': 'No longer shared with {name}.',
  'sharePeople.removeFailed': 'It could not be unshared.',
  'notifications.kind.photos.shared': '{actor} shared “{name}” with you',
  'notifications.kind.notes.shared': '{actor} shared “{name}” with you',
  'notifications.kind.notes.mention': '{actor} mentioned you in “{name}”',
  'notifications.kind.photos.album.added': '{actor} added photos to “{name}”',
  'sharedAlbum.addHint':
    'Add photos from your gallery with “{action}”. They are copied to {name}’s Drive and stay there even if you lose access.',
  'sharedAlbum.rename': 'Rename',
  'sharedAlbum.renamePrompt': 'New album title',
  'sharedAlbum.quotaFull':
    'They do not fit: the organisation’s storage is full. Ask whoever manages it to increase it.',
  'notifications.kind.photos.album.renamed': '{actor} renamed your album to “{name}”',
  // Native OS widget actions and counters.
  'widgets.calendars': 'Calendars',
  'widgets.address_books': 'Address books',
  'widgets.open': 'Open',
  'widgets.notes': 'Notes',
  'widgets.rows': 'Rows',
  'widgets.pending': 'Pending',
  'widgets.channels': 'Channels',
  'widgets.photos': 'Photos',
  'widgets.episodes': 'Episodes',
  'widgets.apps': 'Apps',
  'widgets.compose': 'Compose',
  'widgets.storage': 'Storage',
  'widgets.create': 'Create event',

  'linkedServices.services': 'Services',
  'linkedServices.changed': 'This item changed at your provider. Refresh before editing again.',
  'linkedServices.choose':
    'Choose what to connect. Your provider will ask for the permissions of the selected services.',
  'linkedServices.notSupported': 'This provider does not offer this service.',
  'linkedServices.keepAdmin':
    'Google Keep needs an approved Google Workspace administrator setup. Personal Gmail accounts cannot connect it.',
  'linkedServices.notGranted':
    'Access to this service was not granted. Connect the account again to choose its permissions.',
  'linkedServices.connected': 'Connected services',
  'linkedServices.providerData': 'Changes are saved at your provider.',
  'linkedServices.writeUnknown':
    'The provider did not confirm whether this was saved. Refresh and check before trying again.',
  'linkedServices.unavailable': 'Connection to this provider is not available for your account.',
  'localFile.confirmUpload.title': 'Open file from this device',
  'localFile.confirmUpload.body':
    'To open “{name}”, it will be uploaded to your Sytue One account.',
  'localFile.confirmUpload.remember': 'Remember my choice on this device',
  'localFile.confirmUpload.confirm': 'Upload and open',
};
