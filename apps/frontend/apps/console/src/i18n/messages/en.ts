/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo INGLÉS de Sytue Admin — FUENTE DE VERDAD (docs/CONVENTIONS.md §13.1).
 * Las cadenas nacen aquí y los otros cuatro idiomas las traducen. Añadir una
 * clave aquí rompe la compilación de es/fr/de/it hasta que la traduzcan: esa es
 * la garantía que queremos.
 *
 * Inglés europeo (en-GB): «organisation», «authorisation». El formateador ya
 * usa en-GB (`intlTag`), y mezclar ortografías en la misma interfaz canta.
 *
 * Terminología: la que usan Microsoft Entra y Google Admin en cada lengua. El
 * público son administradores de TI, no usuarios finales.
 */
import type { MessageEntry } from '@sytue/i18n';

export const en = {
  // --- Cromo: navegación y armazón ---------------------------------------
  'nav.dashboard': 'Dashboard',
  'nav.tenants': 'Organisations',
  'nav.users': 'Users',
  'nav.groups': 'Groups',
  'nav.roles': 'Roles',
  'nav.pim': 'Privileged access',
  'nav.sso': 'Single sign-on (SSO)',
  'nav.access': 'Conditional access',
  'nav.devices': 'Devices',
  'nav.apps': 'Integrations',
  // Editor de documentos de la organizacion: era una variable de entorno de Drive.
  'nav.documentEditor': 'Document editor',
  'nav.externalSignatures': 'Signatures from outside',
  'page.documentEditor': 'Document editor',
  'page.externalSignatures': 'Signatures from outside',
  'externalSignatures.title': 'Signature requests from other organisations',
  'externalSignatures.description':
    'Decide what happens when someone from another organisation asks a person here to sign a PDF in CertiPDF.',
  'externalSignatures.policy': 'Who can ask',
  'externalSignatures.mode.abierta': 'Anyone',
  'externalSignatures.mode.abierta.help':
    'Requests from any organisation arrive. This is the default.',
  'externalSignatures.mode.cerrada': 'Nobody from outside',
  'externalSignatures.mode.cerrada.help':
    'Requests from other organisations do not arrive here and cannot be opened from here. Someone who also has a personal account can still receive them there.',
  'externalSignatures.mode.confianza': 'Only trusted organisations',
  'externalSignatures.mode.confianza.help': 'Only requests from the organisations you list arrive.',
  'externalSignatures.trusted': 'Trusted organisations',
  'externalSignatures.trustedHelp':
    'One per line: the name in their address, such as “acme” in …/docs/acme/.',
  'externalSignatures.silent': 'The other side is not told',
  'externalSignatures.silentHelp':
    'A request that does not arrive still looks sent to the person who made it. That way nobody learns which organisations someone belongs to.',
  'externalSignatures.save': 'Save',
  'externalSignatures.saved': 'Saved',
  'externalSignatures.saveFailed': 'It could not be saved. Try again.',
  'externalSignatures.badOrg': '“{org}” is not an organisation name.',
  'externalSignatures.loadFailed': 'It could not be loaded',
  'externalSignatures.loadFailedHelp': 'Sytue Docs is not answering. Try again in a moment.',
  'editor.title': 'Document editor',
  'editor.description':
    'Choose which editor opens the Office documents stored in your organisation.',
  'editor.current': 'Current editor',
  'editor.none': 'No external editor connected',
  'editor.noneHelp':
    'Sytue documents already open in Sytue. Connect one only if your organisation already runs an editor —Collabora, ONLYOFFICE— and wants to keep using it for Word, Excel and PowerPoint files.',
  'editor.connect': 'Connect an editor',
  'editor.change': 'Change',
  'editor.disconnect': 'Disconnect',
  'editor.disconnectConfirm': 'Disconnect the editor? Those files will stop opening in it.',
  'editor.disconnected': 'Editor disconnected',
  'editor.disconnectFailed': 'It could not be disconnected.',
  'editor.unnamed': 'External editor',
  'editor.responding': 'Responding',
  'editor.notResponding': 'Not responding',
  'editor.paused': 'Paused',
  'editor.notRespondingTitle': 'The editor is not answering right now',
  'editor.notRespondingUnknown': 'No further detail was given.',
  'editor.opens': 'Opens: {list}',
  'editor.opensNothing': 'It did not declare any file type.',
  'editor.wizard.description': 'Three steps. Nothing changes until the last one.',
  'editor.step1': 'Which one',
  'editor.step1.heading': 'Which editor do you run?',
  'editor.step1.description': 'Pick yours and we will work out the rest of the address.',
  'editor.kind.collabora': 'Collabora Online',
  'editor.kind.collabora.desc': 'The most common self-hosted one. Based on LibreOffice.',
  'editor.kind.onlyoffice': 'ONLYOFFICE',
  'editor.kind.onlyoffice.desc': 'Closest to the look of Microsoft Office.',
  'editor.kind.manual': 'Another one',
  'editor.kind.manual.desc': 'Any editor that speaks WOPI. You give the full address.',
  'editor.step2': 'Where',
  'editor.step2.heading': 'Where does it live?',
  'editor.step2.description': 'The address you use to reach it, nothing else.',
  'editor.hostLabel': 'Address of your editor',
  'editor.urlLabel': 'Address of its catalogue (discovery)',
  'editor.hostHint': 'Just the address, like https://office.yourcompany.com — we add the rest.',
  'editor.urlHint': 'The full address of its discovery document.',
  'editor.willUse': 'We will ask: {url}',
  'editor.check': 'Check it',
  'editor.checkFailed': 'It could not be reached.',
  'editor.checkFailedTitle': 'That editor did not answer',
  'editor.step3': 'Confirm',
  'editor.step3.heading': 'It answered. This is what will be set.',
  'editor.step3.description': 'Nothing has been saved yet.',
  'editor.step3.headingFailed': 'It did not answer',
  'editor.step3.descriptionFailed': 'Go back and check the address. Nothing has been saved.',
  'editor.summary.kind': 'Editor',
  'editor.summary.url': 'Address',
  'editor.summary.opens': 'Opens',
  'editor.save': 'Connect it',
  'editor.connected': 'Editor connected',
  'editor.hostDisabled': 'This deployment cannot open files in an external editor',
  'editor.hostDisabledHelp':
    'Sytue Drive has no WOPI key, so it cannot mint the tokens an editor needs to open a file. Your editor is saved, but nobody will see “open in editor” until whoever runs this installation sets that key.',
  'editor.saveFailed': 'It could not be saved.',
  'nav.migrations': 'Import content',
  // Administración de los canales de Rooms de la organización.
  'nav.rooms': 'Rooms channels',
  'rooms.channels.title': 'Channels',
  'rooms.channels.subtitle':
    'Every channel in the organisation, including private ones. Direct messages are never listed.',
  'rooms.filter.placeholder': 'Filter by channel or team',
  'rooms.showArchived': 'Show archived',
  'rooms.col.channel': 'Channel',
  'rooms.col.kind': 'Type',
  'rooms.col.members': 'Members',
  'rooms.col.lastActivity': 'Last activity',
  'rooms.col.state': 'State',
  'rooms.kind.public': 'Public',
  'rooms.kind.private': 'Private',
  'rooms.state.active': 'Active',
  'rooms.state.archived': 'Archived',
  'rooms.action.archive': 'Archive',
  'rooms.action.restore': 'Restore',
  'rooms.stat.channels': 'Channels',
  'rooms.stat.private': 'Private',
  'rooms.stat.archived': 'Archived',
  'rooms.stat.teams': 'Teams',
  'rooms.empty': 'No channels match',
  'rooms.emptyBody':
    'Change the filter, or turn on «Show archived» if you are looking for one that was put away.',
  'rooms.toast.archived': 'Channel archived',
  'rooms.toast.restored': 'Channel restored',
  'rooms.toast.failed': 'The change could not be applied',
  'rooms.error.forbidden': 'You cannot administer Rooms',
  'rooms.error.forbiddenBody': 'This screen is for owners and administrators of the organisation.',
  'rooms.error.unavailable': 'Rooms is not answering',
  'rooms.error.unavailableBody':
    'The channels could not be read. The service may be restarting; try again in a moment.',
  'nav.erp': 'ERP permissions',
  'erp.people.title': 'People',
  'erp.people.subtitle': 'Everyone in the organisation, whether or not they have opened the ERP.',
  'erp.filter': 'Filter by name or email',
  'erp.admin': 'Manages everything',
  'erp.adminBody':
    'Owners and administrators of the organisation manage every ERP module. To limit them, change their role in Users.',
  'erp.pick': 'Choose a person',
  'erp.levels.help':
    'No access: does not see the module · Read: sees it but changes nothing · User: creates and edits · Administrator: everything, including voiding entries.',
  'erp.modules.title': 'Access by module',
  'erp.modules.subtitle':
    'What this person can see and do in each module. A level equal to the default is not stored, so changing a default reaches everyone who was not set by hand.',
  'erp.col.module': 'Module',
  'erp.col.level': 'Level',
  'erp.default': 'default',
  'erp.level.ninguno': 'No access',
  'erp.level.lectura': 'Read',
  'erp.level.usuario': 'User',
  'erp.level.administrador': 'Administrator',
  'erp.obligationsNote':
    'Everyone with access to Obligations gets an email 10 days before each date.',
  'erp.toast.saved': 'Permissions saved',
  'erp.toast.failed': 'The permissions could not be saved',
  'erp.error.forbidden': 'You cannot manage ERP permissions',
  'erp.error.forbiddenBody': 'This screen is for owners and administrators of the organisation.',
  'erp.error.unavailable': 'Sytue ERP is not answering',
  'erp.error.unavailableBody':
    'The permissions could not be read. The service may be restarting; try again in a moment.',
  'nav.governance': 'Governance',
  'nav.compliance': 'Compliance',
  'nav.sensitivityLabels': 'Sensitivity labels',
  'nav.audit': 'Audit',
  'nav.account': 'My account',
  'shell.brand': 'Sytue Admin',
  'org.active': 'Active organisation',
  'org.switch': 'Switch organisation',
  'org.none': 'No organisation',
  'org.switchFailed': 'Could not switch. Try again.',
  'scim.title': 'Automatic provisioning (SCIM)',
  'scim.on': 'Active',
  'scim.off': 'Not configured',
  'scim.unknown': 'Unknown',
  'scim.onHint':
    'Your identity provider creates, updates and DEACTIVATES accounts in Sytue automatically.',
  'scim.offHint':
    'Accounts are managed by hand: leavers stay active until someone removes them here.',
  'scim.unknownHint':
    'Sytue ID did not answer, so the provisioning state is unknown. Reload in a moment.',
  'scim.provisioned': 'People provisioned',
  'scim.lastEvent': 'Last change from the provider',
  'scim.lastEventNever': 'Never',
  'scim.endpoint': 'SCIM endpoint',
  'platform.org.protected': 'Platform organisation — protected',
  'scim.copy': 'Copy',
  'scim.copied': 'Copied',
  'scim.step1': 'Paste this address in your identity provider',
  'scim.step2': 'Generate the token and copy it',
  'scim.step3': 'Turn provisioning on in your provider',
  'scim.step3Hint':
    'In Microsoft Entra ID: Enterprise applications → your app → Provisioning → Automatic, paste the tenant URL and the secret token, then Test connection. In Okta: Applications → your app → Provisioning → Configure API Integration. Nothing else is needed on the Sytue side.',
  'scim.token': 'Secret token',
  'scim.tokenOnce':
    'Copy it now: it is shown once and cannot be recovered. If you lose it, generate another one and revoke this.',
  'scim.tokenLabel': 'Name (optional)',
  'scim.tokenLabelHint': 'To tell it apart later, e.g. «Entra ID».',
  'scim.generate': 'Generate token',
  'scim.tokenFailed': 'The token could not be generated',
  'scim.tokensLive': 'Active tokens',
  'scim.tokenNoLabel': 'Unnamed token',
  'scim.tokenUsed': 'last used {when}',
  'scim.tokenNeverUsed': 'never used',
  'scim.revoke': 'Revoke',
  'scim.tokenRevoked': 'Token revoked',
  'scim.tokenRevokeFailed': 'The token could not be revoked',
  'platform.admins.add': 'Grant global scope',
  'platform.admins.email': 'Email of the person',
  'platform.admins.reason': 'Reason',
  'platform.admins.reasonHint': 'Why this person needs to administer the whole platform.',
  'platform.admins.grant': 'Grant',
  'platform.admins.granted': 'Global scope granted',
  'platform.approval.pending': 'Waiting for a second administrator to approve it',
  'platform.action.cancel': 'Cancel',
  'conn.error.catalog':
    'The connector catalogue could not be loaded, so no new provider can be added.',
  'shell.sectionAdmin': 'Administration',
  'shell.sectionAccount': 'Account',
  'shell.metaDescription': 'Sytue Admin — administration of organisations, users and groups.',

  // --- Títulos de página --------------------------------------------------
  'page.dashboard': 'Dashboard',
  'page.tenants': 'Organisations',
  'page.users': 'Users',
  'page.groups': 'Groups',
  'page.roles': 'Roles',
  'page.pim': 'Privileged access',
  'page.sso': 'Single sign-on',
  'page.access': 'Conditional access',
  'page.devices': 'Devices',
  'page.apps': 'Integrations',
  'page.migrations': 'Import content',
  'page.governance': 'Governance',
  'page.compliance': 'Compliance',
  'page.sensitivityLabels': 'Sensitivity labels',
  'page.audit': 'Audit',
  'page.account': 'My account',
  'page.tenantNotFound': 'Organisation not found.',

  // --- Acciones y textos compartidos --------------------------------------
  'common.save': 'Save',
  'common.cancel': 'Cancel',
  'common.create': 'Create',
  'common.delete': 'Delete',
  'common.edit': 'Edit',
  'common.close': 'Close',
  'common.retry': 'Retry',
  'common.next': 'Next',
  'common.search': 'Search',
  'common.loading': 'Loading…',
  'common.loadMore': 'Load more',
  'common.copy': 'Copy',
  'common.copied': 'Copied',
  'common.enabled': 'Enabled',
  'common.disabled': 'Disabled',
  'common.yes': 'Yes',
  'common.no': 'No',
  'common.all': 'All',
  'common.optional': 'optional',
  'common.saveFailed': 'Could not save',
  'common.tryAgain': 'Try again in a few seconds.',
  // --- Etiquetas de dominio (roles, aprovisionamiento) --------------------
  'role.owner': 'Owner',
  'role.admin': 'Administrator',
  'role.member': 'Member',
  'role.billing': 'Billing',
  'provision.synced': 'synced',
  'provision.pending': 'pending',
  'provision.error': 'error',

  // --- Panel --------------------------------------------------------------
  'dashboard.subtitle': 'Status of realm {realm} and suite provisioning.',
  'dashboard.metric.tenants': 'Organisations',
  'dashboard.metric.tenantsHint': 'active tenants',
  'dashboard.metric.users': 'Total users',
  'dashboard.metric.usersHint': 'in Sytue ID',
  'dashboard.metric.mfa': 'MFA enabled',
  'dashboard.metric.mfaHint': 'mandatory for admin/owner',
  // Qué hay que mirar hoy (portada accionable).
  'dashboard.todo.title': 'Worth a look today',
  'dashboard.todo.review': 'Review',
  'dashboard.todo.reinvite': 'Invite again',
  'dashboard.todo.withoutMfa': {
    one: '{count} person has no second factor',
    other: '{count} people have no second factor',
  },
  'dashboard.todo.invitationsExpired': {
    one: '{count} invitation expired without being accepted',
    other: '{count} invitations expired without being accepted',
  },
  'dashboard.todo.unverified': {
    one: '{count} email address is still unverified',
    other: '{count} email addresses are still unverified',
  },
  'dashboard.todo.suspended': {
    one: '{count} suspended account is still open',
    other: '{count} suspended accounts are still open',
  },

  // Destino SIEM de la auditoría (mig 0035, sin pantalla hasta ahora).
  'sink.title': 'Send this audit trail to your SIEM',
  'sink.intro':
    'Events are pushed in batches, in order and signed, from where the last delivery left off. If your SIEM is down nothing is lost: the batch is retried.',
  'sink.url': 'Endpoint (HTTPS)',
  'sink.authHeader': 'Authorization header',
  'sink.signingSecret': 'Signing secret',
  'sink.signingHint':
    'Used for the X-Sytue-Signature header (HMAC-SHA256), so you can verify the batch came from Sytue.',
  'sink.keepSecret': 'Saved — leave blank to keep it',
  'sink.enabled': 'Sending enabled',
  'sink.remove': 'Remove destination',
  'sink.state.on': 'Sending',
  'sink.state.off': 'Paused',
  'sink.state.failing': 'Failing',
  'sink.lastSent': 'Last delivery: {when}',
  'sink.toast.saved': 'Destination saved.',
  'sink.toast.removed': 'Destination removed.',
  'sink.toast.failed': 'It could not be saved.',

  'dashboard.tenants.title': 'Organisations',
  'dashboard.tenants.viewAll': 'View all',
  'dashboard.tenants.emptyTitle': 'No organisations',
  'dashboard.tenants.emptyDescription': 'Create the first organisation to provision the suite.',
  'dashboard.tenants.newTenant': 'New organisation',
  'dashboard.tenants.userCount': { one: '{count} user', other: '{count} users' },
  'dashboard.activity.title': 'Recent activity',
  'dashboard.activity.viewAudit': 'View audit',
  'dashboard.activity.emptyTitle': 'No activity yet',
  'dashboard.activity.emptyDescription': 'Identity and provisioning events will appear here.',
  'dashboard.activity.ok': ' — succeeded',
  'dashboard.activity.failed': ' — failed',

  // --- Verificación de correo --------------------------------------------
  'verifyEmail.region': 'Email verification',
  'verifyEmail.sent': 'We have sent a verification link to {email}. Check your inbox.',
  'verifyEmail.pending': 'Your email {email} is not verified. Verify it to secure your account.',
  'verifyEmail.sentAnnouncement': 'Verification link sent.',
  'verifyEmail.errorAnnouncement': 'The verification link could not be sent.',
  'verifyEmail.resend': 'Resend verification',
  'verifyEmail.errorHint': 'Could not send. Try again in a few seconds.',

  // --- Mi perfil ----------------------------------------------------------
  'profile.title': 'Profile',
  'profile.displayName': 'Display name',
  'profile.nameEmpty': 'The name cannot be empty.',
  'profile.email': 'Email',
  'profile.emailHint': 'The email address is changed through a separate verification flow.',
  'profile.saved': 'Profile updated',
  'profile.invalidName': 'The name is not valid.',

  // --- Política de contraseñas -------------------------------------------
  'passwordPolicy.title': 'Password policy',
  'passwordPolicy.description':
    'Minimum requirements Sytue ID enforces whenever any password in the organisation is set or changed.',
  'passwordPolicy.offline':
    'No connection to Sytue ID: default values are shown and changes cannot be saved.',
  'passwordPolicy.minLength': 'Minimum length',
  'passwordPolicy.range': {
    one: 'Must be between {min} and {count} character.',
    other: 'Must be between {min} and {count} characters.',
  },
  'passwordPolicy.complexity': 'Require complexity',
  'passwordPolicy.complexityHint': 'Requires at least one letter and one digit.',
  'passwordPolicy.save': 'Save policy',
  'passwordPolicy.saved': 'Policy saved',
  'passwordPolicy.savedHint': 'It will apply the next time someone sets or changes their password.',
  'passwordPolicy.saveFailed': 'Could not save the policy',
  'passwordPolicy.forbidden': 'You do not have permission to change the organisation policy.',

  // --- Dominio de correo de la organización -------------------------------
  'orgColor.title': 'Organisation colour',
  'orgColor.description':
    'The colour of the suite: bars, buttons and links across every app. Each person can choose their own in the account menu; this is the one seen by everyone who has not chosen.',
  'orgColor.error': 'The colour could not be saved. Please try again.',
  'orgForma.title': 'Organisation layout',
  'orgForma.description':
    'The layout of the suite: “Frame”, where the top bar and the menu form a coloured frame around the content, or “Rail”, where only the side menu carries the colour. Each person can choose their own in the account menu; this is the one seen by everyone who has not chosen.',
  'orgForma.error': 'The layout could not be saved. Please try again.',
  'logo.title': 'Organisation logo',
  'logo.description':
    'Replaces the Sytue mark in the top bar across every app. Without your own logo we use ours, in a colour of your organisation.',
  'logo.previewDark': 'Dark theme',
  'logo.previewLight': 'Light theme',
  'logo.previewApp': 'Sytue Drive',
  'logo.choose': 'Choose image…',
  'logo.save': 'Save logo',
  'logo.reset': 'Back to the Sytue mark',
  'logo.formats':
    'PNG, JPEG or GIF, up to 1 MB. If your logo is an SVG, export it to PNG with a transparent background.',
  'logo.trimmed': 'Empty margin removed',
  'logo.trimmedHint':
    'The image had empty space around it and was cropped so the logo shows as large as possible. Check above that the composition is what you expect.',
  'logo.opaqueBox': 'The logo brings its own background',
  'logo.opaqueBoxHint':
    'The image is not transparent, so it will show as a block on the bar. If that is not what you want, export the logo as a PNG with a transparent background.',
  'logo.trimCapped': 'Only part of the margin was removed',
  'logo.trimCappedHint':
    'The image had a lot of empty space around it. It was trimmed up to the limit the system allows, so as not to risk cutting into the logo. If you want it to show larger, crop the margin yourself and upload it again.',
  'logo.error.svg':
    'That SVG could not be read. Open it in your design tool and export it as a PNG with a transparent background.',
  'logo.error.title': 'That image could not be used',
  'logo.error.generic': 'The image could not be processed. Please try again.',
  'logo.error.delete': 'The logo could not be removed. Please try again.',
  'orgDomain.title': 'Email domain',
  'orgDomain.description':
    'Where your organisation\u2019s email lives. Everyone always has an address on the Sytue subdomain; connecting your own domain is optional.',
  'orgDomain.offline': 'No connection to Sytue ID: the domain cannot be read or changed right now.',
  'orgDomain.base': 'Sytue subdomain',
  'orgDomain.baseHint':
    'Always available and cannot be removed. Everyone in the organisation has an address here, and it is what the rest of the suite uses to deliver internal mail.',
  'orgDomain.own': 'Your own domain',
  'orgDomain.connect': 'Connect',
  'orgDomain.connectHint':
    'Write it as acme.com, without \u00abhttps://\u00bb or \u00ab@\u00bb. We will give you the exact DNS record to prove it is yours.',
  'orgDomain.badgeOwn': 'Own domain',
  'orgDomain.recordHint':
    'Add this TXT record wherever your domain is registered. Look for the \u00abDNS\u00bb or \u00abRecords\u00bb section \u2014 it is the same everywhere.',
  'orgDomain.recordType': 'Type',
  'orgDomain.recordName': 'Name',
  'orgDomain.recordValue': 'Value',
  'orgDomain.copy': 'Copy',
  'orgDomain.copied': 'Copied',
  'orgDomain.copyFailed': 'Could not copy \u2014 select the value and copy it by hand.',
  'orgDomain.check': 'Save and check',
  'orgDomain.recheck': 'Check again',
  'orgDomain.verified': 'Domain verified',
  'orgDomain.verifiedHint': 'We found the record: {host} is yours.',
  'orgDomain.movedHint':
    'New addresses now use your domain, and everyone already in the organisation keeps their previous address as an alias.',
  'orgDomain.pending': 'Not visible yet',
  'orgDomain.pendingHint':
    'If you have just created it, that is normal: DNS changes take from a few minutes to a few hours. Check again whenever you like.',
  'orgDomain.mismatch': 'A different record is published',
  'orgDomain.mismatchHint':
    'At {name} we found \u00ab{found}\u00bb. Check that you copied the whole value and that no old one is left behind.',
  'orgDomain.unreachable': 'We could not check the DNS',
  'orgDomain.unreachableHint':
    'It is not your record: the name server did not answer. Try again in a moment.',
  'orgDomain.disconnect': 'Disconnect',
  'orgDomain.disconnected': 'Domain disconnected',
  'orgDomain.disconnectedHint':
    'New addresses go back to the Sytue subdomain. Existing mailboxes keep working.',
  'orgDomain.primary': 'Primary',
  'orgDomain.badgePending': 'Pending',
  'orgDomain.addAnother': 'Add another domain',
  'orgDomain.addedHint':
    'It now receives mail and can be used to send from. New addresses keep being created on your primary domain.',
  'orgDomain.makePrimary': 'Make primary',
  'orgDomain.makePrimaryConfirm':
    'New addresses will be created on {host}, and everyone in the organisation moves to it. Their current address is kept as an alias, so nothing stops arriving. Continue?',
  'orgDomain.primaryChanged': 'Primary domain changed',
  'orgDomain.disconnectConfirm':
    'Sytue will stop serving mail for {host}. Addresses that already exist there keep working. Continue?',
  'orgDomain.saveFailed': 'Could not save the domain',

  // --- Usuarios -----------------------------------------------------------
  'users.subtitle': 'Sytue ID directory. Every account grants access to the whole suite.',
  'users.offline': 'No connection to Sytue ID: sample data is shown. Changes will not be saved.',
  'users.searchPlaceholder': 'Search user or email',
  'users.attrs.title': 'Directory attributes',
  'users.attrs.hint':
    'Dynamic groups decide who belongs from these three. Saving re-evaluates this person’s groups right away.',
  'users.attrs.saved': 'Attributes saved',
  'users.attrs.savedHint': 'Dynamic groups for {email} have been re-evaluated.',
  'users.searchLabel': 'Search user',
  'users.filterTenant': 'Filter by organisation',
  'users.allTenants': 'All organisations',
  'users.invite': 'Invite user',
  'users.listLabel': 'Sytue ID users',
  'users.openRecord': 'Open the record for {name}',
  'users.mfaOn': 'active',
  'users.mfaOff': 'no MFA',
  'users.statusActive': 'active',
  'users.statusDisabled': 'disabled',
  'users.modalFallbackTitle': 'User',
  'users.col.user': 'User',
  'users.col.roles': 'Roles',
  'users.col.tenant': 'Organisation',
  'users.col.provisioning': 'Provisioning',
  'users.col.status': 'Status',
  'users.empty.title': 'No user matches',
  'users.empty.description': 'Review the search text or the organisation filter.',
  'users.empty.clearFilters': 'Clear filters',

  // Acciones sobre varias personas a la vez.
  'users.bulk.select': 'Select {name}',
  'users.bulk.selected': { one: '{count} selected', other: '{count} selected' },
  'users.bulk.suspend': 'Suspend',
  'users.bulk.reactivate': 'Reactivate',
  'users.bulk.revokeSessions': 'Close sessions',
  'users.bulk.setRole': 'Change role…',
  'users.bulk.clear': 'Clear selection',
  'users.bulk.done': { one: 'Applied to {count} person', other: 'Applied to {count} people' },
  'users.bulk.partial': {
    one: '{count} was left out (you cannot do it, or they are no longer here).',
    other: '{count} were left out (you cannot do it, or they are no longer here).',
  },
  'users.bulk.failed': 'The bulk action could not be run',

  // Impacto real antes de confirmar (sesiones, grupos, último propietario).
  'users.suspend.question': 'Cut off access for {email}?',
  'users.suspend.action': 'Suspend access',
  'users.impact.sessions': {
    one: '{count} open session will be closed',
    other: '{count} open sessions will be closed',
  },
  'users.impact.groups': { one: 'Belongs to {count} group', other: 'Belongs to {count} groups' },
  'users.impact.lastOwner':
    'They are the only owner: without them nobody can administer this organisation.',
  'users.empty.noMatch': 'Nobody in this organisation matches what you typed.',
  'users.toast.searchFailed': 'The search could not be run',
  'users.detail.lastLogin': 'Last sign-in',
  'users.detail.created': 'Created',
  'users.detail.emailVerified': 'Email verified',
  'users.detail.provisioning': 'Provisioning (SCIM)',
  'users.roleInOrg': 'Role in the organisation',
  'users.accessEnabled': 'Access enabled',
  'users.action.forceLogout': 'Sign out everywhere',
  'users.action.resetPassword': 'Reset password',
  'users.action.sendResetLink': 'Send reset link',
  'users.action.resetMfa': 'Reset MFA',
  'users.resetMfa.warning':
    'The TOTP secret and security keys of {email} will be deleted. Their account will have no second factor until they set MFA up again. This action is destructive.',
  'users.resetPw.intro':
    'Set a temporary password and sign {email} out of every session. Share it over a secure channel.',
  'users.resetPw.new': 'New password',
  'users.resetPw.repeat': 'Repeat the password',
  'users.resetPw.tooShort': {
    one: 'The password must be at least {count} character long.',
    other: 'The password must be at least {count} characters long.',
  },
  'users.resetPw.mismatch': 'The passwords do not match.',
  'users.remove.confirmQuestion':
    'Remove {email} from the organisation? Their sessions will be closed and they will lose access; the account is not deleted.',
  'users.remove.action': 'Remove from organisation',
  'users.inviteIntro':
    'We will send this address a link to create their account and join the organisation.',
  'users.inviteEmail': 'Email address',
  'users.inviteEmailPlaceholder': 'person@company.com',
  // Alta directa con contraseña: la salida cuando el correo no llega.
  'users.invitePassword': 'Initial password (optional)',
  'users.invitePasswordPlaceholder': 'Leave blank to send an invitation',
  'users.invitePasswordHint':
    'With a password the account is created straight away and no email is sent — useful when mail is down. Leave it blank and they choose their own from the invitation link, which is safer: the password never travels.',
  'users.toast.created': 'Account created',
  'users.toast.createdHint':
    '{email} can sign in now with the password you set. No email was sent, so you will have to pass it on yourself.',

  'users.inviteRoleLabel': 'Role',
  'users.inviteMailbox': 'Mailbox address',
  'users.inviteMailboxPlaceholder': 'first.last',
  'users.inviteMailboxHint': 'Leave blank to use first.last automatically.',
  'users.pending.title': 'Pending invitations',
  'users.pending.hint':
    'Sent and not accepted yet. They expire on their own; revoking invalidates the link right away.',
  'users.pending.expires': 'Expires on {date}',
  'users.pending.revoke': 'Revoke',
  'users.pending.revoked': 'Invitation revoked',
  'users.pending.revokedHint': 'The link sent to {email} no longer works.',
  'users.pending.gone': 'That invitation was already gone',
  'users.pending.goneHint':
    'Either it was accepted or it had expired. The list has been refreshed.',
  'users.pending.revokeFailed': "Couldn't revoke it",
  'users.mailbox.title': 'Email addresses',
  'users.mailbox.primary': 'Primary',
  'users.mailbox.none': 'No mailbox assigned yet.',
  'users.mailbox.addAlias': 'Add alias',
  'users.mailbox.addPlaceholder': 'alias',
  'users.mailbox.makePrimary': 'Make primary',
  'users.mailbox.remove': 'Remove alias',
  'users.mailbox.hint':
    'Aliases deliver to the same mailbox. Changing the primary keeps the previous address as an alias, so mail sent to it still arrives.',
  'users.mailbox.changed': 'Primary address changed',
  'users.mailbox.aliasAdded': 'Alias added',
  'users.mailbox.aliasRemoved': 'Alias removed',
  'users.mailbox.failed': 'Could not update the address',
  'users.mailbox.taken': 'That address is already in use. Choose another.',
  'users.mailbox.invalid': 'That address is not valid for this organisation.',
  'users.inviteSubmit': 'Send invitation',
  'users.sessionsRevoked': {
    one: '{count} session was closed.',
    other: '{count} sessions were closed.',
  },
  'users.toast.loadMoreFailed': 'Could not load more',
  'users.toast.loadMoreFailedHint': 'Try again in a few seconds; the directory stays where it was.',
  'users.toast.forbiddenUser': 'You do not have permission for this action on this user.',
  'users.toast.suspended': 'User suspended',
  'users.toast.suspendedHint': '{email} can no longer sign in.',
  'users.toast.reactivated': 'User reactivated',
  'users.toast.reactivatedHint': '{email} has access to the suite again.',
  'users.toast.statusFailed': 'Could not change the status',
  'users.toast.sessionsClosed': 'Sessions closed',
  'users.toast.sessionsClosedHint': {
    one: '{email}: {count} session closed. They will have to sign in again.',
    other: '{email}: {count} sessions closed. They will have to sign in again.',
  },
  'users.toast.sessionsFailed': 'Could not close the sessions',
  'users.toast.pwReset': 'Password reset',
  'users.toast.pwResetHint': '{email} will have to use the new password.',
  'users.toast.pwResetFailed': 'Could not reset the password',
  'users.toast.linkSent': 'Link sent',
  'users.toast.linkSentHint': '{email} will receive a link to choose a new password.',
  'users.toast.linkFailed': 'Could not send the link',
  'users.toast.ssoOnly': 'The account does not use a password (SSO sign-in).',
  'users.toast.mfaReset': 'MFA reset',
  'users.toast.mfaResetHint': '{email} will have to set up 2FA again at the next sign-in.',
  'users.toast.mfaResetFailed': 'Could not reset MFA',
  'users.toast.notInOrg': 'The user does not belong to this organisation.',
  'users.toast.roleUpdated': 'Role updated',
  'users.toast.roleUpdatedHint': '{email} now has the {role} role.',
  'users.toast.roleFailed': 'Could not change the role',
  'users.toast.lastAdmin': 'You cannot leave the organisation without an administrator.',
  'users.toast.removed': 'User removed',
  'users.toast.removedHint': '{email} no longer belongs to the organisation.',
  'users.toast.removeFailed': 'Could not remove the user',
  'users.toast.removeLastAdmin': 'You cannot remove the last administrator of the organisation.',
  'users.toast.invited': 'Invitation sent',
  'users.toast.invitedHint': '{email} will receive a link to join with the {role} role.',
  'users.toast.inviteFailed': 'Could not invite',
  'users.toast.alreadyMember': 'That address is already a member of the organisation.',
  'users.toast.roleForbidden': 'You do not have permission to grant that role.',
  'users.toast.checkEmail': 'Check the address and try again.',
  'users.toast.inviteNotSent': 'Invitation created, but the email did not go out',
  'users.toast.inviteNotSentHint':
    'The address is fine: what failed was our mail system. The invitation is already on the list — invite again when mail is back, and it will replace this one.',

  // --- Organizaciones (tenants) -------------------------------------------
  'tenants.subtitle': 'Sytue ID tenants. Creating one provisions the whole suite (SCIM).',
  'tenants.searchPlaceholder': 'Search organisation',
  'tenants.statusActive': 'active',
  'tenants.statusSuspended': 'suspended',
  'tenants.mailDomain': 'Mail domain',
  'tenants.region': 'Region',
  'tenants.col.plan': 'Plan',
  'tenants.col.namespace': 'Namespace',
  'tenants.settings': 'Settings',
  'tenants.settingsFor': 'Settings for {name}',
  'tenants.settingsOf': 'Settings · {name}',
  'tenants.empty.title': 'No organisations yet',
  'tenants.empty.description':
    'Create the first one: it provisions the group, the namespace and the whole suite over SCIM.',
  'tenants.noResults': 'No results for “{query}”',
  'tenants.noResultsHint': 'Try another name or slug, or clear the search.',
  'tenants.clearSearch': 'Clear search',
  'tenants.form.name': 'Name',
  'tenants.form.namePlaceholder': 'ACME Europe',
  'tenants.form.slug': 'Slug (org/<slug>)',
  'tenants.form.slugHelp':
    'Lowercase identifier; it defines the group and the namespace. It cannot be changed later.',
  'tenants.form.slugImmutable':
    'The slug {slug} is the identity of the organisation and cannot be changed.',
  'tenants.detail.usersTitle': 'Users of the organisation',
  'tenants.detail.usersCaption': 'Users of {name}',
  'tenants.detail.usersEmptyTitle': 'No users yet',
  'tenants.detail.usersEmptyDescription':
    'Accounts of this organisation appear here once they are provisioned over SCIM.',
  'tenants.detail.groupsEmptyTitle': 'No groups yet',
  'tenants.detail.groupsEmptyDescription':
    'The org/<slug> group and its subgroups appear here once created.',
  'tenants.detail.workspace': 'workspace: {name}',
  'tenants.toast.loadMoreFailedHint': 'Try again in a few seconds; the list stays where it was.',
  'tenants.toast.created': 'Organisation created',
  'tenants.toast.createdHint': 'org/{slug} · {namespace}. You are its owner.',
  'tenants.toast.createFailed': 'Could not create the organisation',
  'tenants.toast.slugTaken': 'That slug is already in use; choose another one.',
  'tenants.toast.invalidSlug': 'Check the slug (lowercase, digits and hyphens) and the name.',
  'tenants.toast.settingsSaved': 'Settings saved',
  'tenants.toast.settingsFailed': 'Could not save the settings',
  'tenants.toast.noAccess': 'You do not have permission over this organisation.',
  // Multi-pertenencia: añadir a alguien que YA tiene cuenta a otra organización.
  'tenants.member.action': 'Add person',
  'tenants.member.actionFor': 'Add a person to {name}',
  'tenants.member.title': 'Add a person to {name}',
  'tenants.member.person': 'Person',
  'tenants.member.personPlaceholder': 'Name or email address',
  'tenants.member.personHelp':
    'They must already have a Sytue account: this adds them, it does not create one. If they do not show up, type their full email address.',
  'tenants.member.noCandidates': 'Nobody in this directory matches. Type the full email address.',
  'tenants.member.role': 'Role in this organisation',
  'tenants.member.roleHelp':
    'It only applies here: whatever they are in their other organisations stays untouched.',
  'tenants.member.roleHelpAdmin':
    'It only applies here. Granting the owner role is reserved to an owner of {name}.',
  'tenants.member.submit': 'Add',
  'tenants.member.added': 'Person added',
  'tenants.member.addedHint': '{email} now belongs to {org} as well.',
  'tenants.member.failed': 'Could not add the person',
  'tenants.member.noAccount': 'No Sytue account uses {email}. Send them an invitation instead.',

  // --- Auditoría unificada -------------------------------------------------
  'audit.title': 'Unified audit',
  'audit.subtitle':
    'Trail of every service (Identity, Mail, Files, Integrations) in a single view. Normalised CSV/JSON export for SIEM.',
  'audit.source.all': 'All services',
  'audit.source.id': 'Identity',
  'audit.source.mail': 'Mail',
  'audit.source.drive': 'Files',
  'audit.source.notes': 'Notes',
  'audit.source.integrations': 'Integrations',
  'audit.filterService': 'Filter by service',
  'audit.filterAction': 'Filter by action',
  'audit.filterActor': 'Filter by actor or target',
  'audit.actionPlaceholder': 'Action (e.g. policy., ediscovery.)',
  'audit.actorPlaceholder': 'Actor or target',
  'audit.since': 'From',
  'audit.until': 'To',
  'audit.apply': 'Apply',
  'audit.applying': 'Applying…',
  'audit.allSourcesDown': 'No audit source responded: there is no connection to the services.',
  'audit.someSourcesDown': 'No response from: {sources}. This view may be incomplete.',
  'audit.loadFailed': 'Could not load the audit trail. Adjust the filters and try again.',
  'audit.col.date': 'Date',
  'audit.col.service': 'Service',
  'audit.col.action': 'Action',
  'audit.col.actor': 'Actor',
  'audit.col.target': 'Target',
  'audit.empty.title': 'No events to show',
  'audit.empty.description':
    'There are no audit records for that filter. Widen the date range or clear the filters.',
  'audit.listLabel': 'Unified audit records',
  'groups.memberCount': { one: '{count} member', other: '{count} members' },

  // --- Dispositivos --------------------------------------------------------
  'devices.subtitle':
    'Devices used to sign in. Mark the one you are on as trusted so it satisfies conditional access rules that require a managed device.',
  'devices.offline': 'No connection to Sytue ID: devices cannot be loaded or managed.',
  'devices.trusted': 'Trusted',
  'devices.untrusted': 'Not trusted',
  'devices.thisDevice': 'This device',
  'devices.trust': 'Trust',
  'devices.revoke': 'Revoke',
  'devices.revokeAria': 'Revoke {name}',
  'devices.revokeOfAria': 'Revoke the device of {user}',
  'devices.revokeTitle': 'Revoke device',
  'devices.revokeConfirm':
    '{name} will be revoked. It will no longer be recognised and will lose its trust; at the next sign-in it is registered again as unknown.',
  'devices.lastActivity': 'last activity {when}',
  'devices.fallbackUser': 'User',
  'devices.mine.title': 'My devices',
  'devices.mine.emptyTitle': 'No devices registered',
  'devices.mine.emptyDescription':
    'When you sign in from a browser it appears here so you can mark it as trusted.',
  'devices.org.title': 'Organisation devices',
  'devices.org.emptyTitle': 'No devices in the organisation',
  'devices.org.emptyDescription': 'Devices appear here as users sign in.',
  'devices.col.userDevice': 'User · Device',
  'devices.col.trust': 'Trust',
  'devices.col.manage': 'Manage',
  'devices.toast.trusted': 'Trusted device',
  'devices.toast.trustFailed': 'Could not mark as trusted',
  'devices.toast.trustOnlyCurrent':
    'You can only mark the device you are signing in from as trusted.',
  'devices.toast.revoked': 'Device revoked',
  'devices.toast.revokeFailed': 'Could not revoke',

  // --- Etiquetas de sensibilidad -------------------------------------------
  'labels.description':
    'Mark files by how sensitive they are — public, internal, confidential — and decide which ones may not leave the organisation. Whoever works with the file sees the label; Sytue makes sure it is not shared outside if you said so.',
  'labels.new': 'New label',
  'common.offlineIdTitle': 'We cannot reach Sytue ID',
  'labels.offlineBody': 'Labels cannot be loaded or edited right now. Try again in a few minutes.',
  'labels.scale.title': 'Your scale',
  'labels.scale.description':
    'From least to most sensitive. The order is yours: it lets everyone see at a glance what is more delicate than what.',
  'labels.empty.title': 'No labels yet',
  'labels.empty.description':
    'Without labels every file is treated the same. The scale almost everyone starts with: Public, Internal and Confidential — and only the last one blocks sharing outside.',
  'labels.empty.action': 'Create the first one',
  'labels.noDescription': 'No explanation',
  'labels.blocksExternal': 'Stays in the organisation',
  'labels.informational': 'Informational only',
  'common.editName': 'Edit {name}',
  'common.deleteName': 'Delete {name}',
  'labels.manualNote':
    'Labelling is manual: each person picks the label while working with the file. Sytue does not classify anything on its own.',
  'labels.wizard.editTitle': 'Edit “{name}”',
  'labels.wizard.description':
    'Three questions and a summary: what it is called, how sensitive it is and what it must prevent.',
  'labels.delete.title': 'Delete the label',
  'labels.delete.intro': '{name} can no longer be chosen when classifying files.',
  'labels.delete.keep':
    'Files that already carry it do not change: they keep the label they were given. That is deliberate — deleting a label must not declassify what was already classified.',
  'labels.delete.keepBlocking':
    'Files that already carry it do not change: they keep the label they were given and still cannot be shared outside the organisation. That is deliberate — deleting a label must not declassify what was already classified.',
  'labels.toast.deleted': 'Label deleted',
  'labels.toast.deleteFailed': 'Could not delete',

  // --- Acceso condicional ---------------------------------------------------
  'access.title': 'Access rules',
  'access.subtitle':
    'Decide what it takes to sign in to Sytue depending on the situation: where people connect from, on which device and at what time. With no rules, everyone signs in as usual.',
  'access.new': 'New rule',
  'access.offlineBody':
    'Neither the rules nor their state can be loaded or edited right now. Try again in a few minutes.',
  'access.rules.title': 'Your rules',
  'access.rules.description':
    'They are checked top to bottom at every sign-in. If several match, the strictest one wins: blocking beats asking for verification, and asking for verification beats letting through.',
  'access.empty.title': 'No rules yet',
  'access.empty.description':
    'Without rules, anyone with their password signs in from anywhere. The one most people create first: ask for one extra verification from whoever connects from outside the office.',
  'access.empty.action': 'Create the first one',
  'access.action.block': 'Blocks sign-in',
  'access.action.mfa': 'Asks for verification',
  'access.action.allow': 'Lets through',
  'access.applies.everyone': 'Everyone',
  'access.applies.groups': { one: '{count} group', other: '{count} groups' },
  'access.applies.users': { one: '{count} person', other: '{count} people' },
  'access.applies.to': 'For {what}',
  'access.cond.fromNetworks': 'from your networks',
  'access.cond.outsideNetworks': 'outside your networks',
  'access.cond.untrustedDevices': 'on untrusted devices',
  'access.cond.apps': { one: 'in {count} app', other: 'in {count} apps' },
  'access.cond.schedule': 'at certain times',
  'access.cond.always': 'always',
  'access.cond.exempt': { one: '{count} exempt', other: '{count} exempt' },
  'access.moveUp': 'Move {name} up',
  'access.moveDown': 'Move {name} down',
  'access.on': 'On',
  'access.off': 'Off',
  'common.switchAria': '{state}: {name}',
  'access.tip':
    'Tip: always leave your administrators exempt from at least one rule. It is your emergency exit if one of them locks you out.',
  'access.delete.title': 'Delete the rule',
  'access.delete.body':
    '{name} will no longer be checked at sign-in. Anyone affected by it will sign in like everybody else. It cannot be undone, but you can create it again.',
  'access.wizard.editTitle': 'Edit “{name}”',
  'access.wizard.newTitle': 'New access rule',
  'access.wizard.description':
    'Three questions and a summary. Before saving we tell you, in one sentence, who it affects and who it would lock out.',
  'common.toggleFailed': 'Could not change it',
  'common.toggleFailedHint': '{name} stays as it was.',
  'access.toast.reorderFailed': 'Could not change the order',
  'access.toast.deleted': 'Rule deleted',
  'access.toast.deleteFailed': 'Could not delete',

  // --- Traer contenido (migraciones) ---------------------------------------
  'migrations.subtitle':
    'Move over from Microsoft 365 or Google Workspace without anyone losing their files or their contacts: each person gets their own in their own Sytue account.',
  'migrations.outStart': 'Take my content with me',
  'migrations.out.title': 'Take your content with you',
  'migrations.out.description':
    'Copy the files and contacts of the people you choose to a Google Workspace or Microsoft 365 account. Nothing is deleted from Sytue: this is a copy, and you can stop it whenever you want.',
  'wizard.migrate.step.destination': 'Destination',
  'wizard.migrate.out.heading': 'Which account are you taking it to?',
  'wizard.migrate.out.description':
    'Pick the destination connection. We write with the permission that account granted, and only into a folder created by Sytue.',
  'wizard.migrate.out.noticeTitle': 'Your data is yours',
  'wizard.migrate.out.noticeBody':
    'This COPIES to the destination account: nothing is deleted from Sytue. Files land in a “Sytue” folder on the destination Drive or OneDrive — they are not mixed with what is already there — and contacts are added to its address book. If the job is interrupted, it resumes where it left off and does not duplicate anything.',
  'wizard.migrate.out.authTitle': 'Let Sytue write to {provider} first',
  'wizard.migrate.out.authBody':
    'The connection used to bring content in can only read. To take it out, {provider} will ask you for permission to create files, contacts, mail and calendars in the account. If you already allowed it, just continue.',
  'wizard.migrate.out.authAction': 'Allow writing',
  'wizard.migrate.out.authFailed':
    'We could not open the authorisation. Try again in a few seconds.',
  'wizard.migrate.out.whatHeading': 'What are you taking with you?',
  'wizard.migrate.out.whoHeading': 'Whose content is leaving?',
  'wizard.migrate.out.whoDescription':
    'For every person you pick, their own content is copied to the destination account. You can pick one or several.',
  'migrations.self.description':
    'Copy your files, contacts, mail and calendar to a Google Workspace or Microsoft 365 account. Nothing is deleted from Sytue: this is a copy, and you can stop it whenever you want.',
  'wizard.migrate.self.description':
    'Choose the destination connection. It will write with the permission that account allowed.',
  'wizard.migrate.self.others': 'Take out other people’s content',
  'wizard.migrate.self.noticeBody':
    'This COPIES to the destination account: nothing is deleted from Sytue. Files land in a “Sytue” folder, mail in the folder it belongs to (inbox, sent…) and each calendar arrives as a new one. If the copy stops, launch it again: it picks up where it was and does not duplicate anything.',
  'wizard.migrate.self.gets.mail': 'Mail with its folders, dates and attachments.',
  'wizard.migrate.self.gets.calendar':
    'Calendars and their events, with their repeats and exceptions.',
  'wizard.migrate.self.nots.outlookDrafts':
    'In Outlook, a message with more than 3 MB of attachments arrives marked as a draft. It arrives whole: that is the only way Microsoft accepts large attachments.',
  'wizard.migrate.self.nots.invites':
    'Invitations: nobody gets notified. In Outlook, attendees are listed in the event description.',
  'wizard.migrate.self.nots.outlookRepeats':
    'In Outlook, cancelled events and repeats it cannot represent. They are skipped one by one and we tell you which.',
  'wizard.migrate.self.safeBody':
    'We only read from Sytue: nothing changes here. If you launch the copy again, it brings what is new and does not duplicate what already arrived.',
  'wizard.migrate.self.launch': 'Start the copy',
  'wizard.migrate.self.reviewTitle': 'Copied with your own session',
  'wizard.migrate.self.reviewBody':
    'Sytue reads your content just like when you download “Your data” and writes it to {provider} with the permission you allowed. You can close this page: the copy keeps going in the background.',
  'wizard.migrate.self.conflict':
    'It is already being copied. If it looks stuck, stop it and launch it again: it continues where it left off.',
  'common.offlineGatewayTitle': 'We cannot reach the integrations service',
  'migrations.offlineBody':
    'Imports cannot be listed or launched right now. Try again in a few minutes.',
  'migrations.domain.drive': 'Files',
  'migrations.domain.contacts': 'Contacts',
  // --- Automatic import assistant (AutoMigrateWizard) -----------------------
  'migrations.domain.repos': 'Repositories',
  'migrations.domain.gists': 'Gists',
  'migrations.domain.teams': 'Teams and channels',
  'migrations.domain.team_files': 'Channel files and history',
  'migrations.domain.mail': 'Mail',
  'migrations.domain.calendar': 'Calendar',
  'migrations.domain.cloud': 'Apps (Cloud and Firebase)',
  'wizard.auto.source.microsoftDesc':
    'Your OneDrive and your contacts, and your Microsoft Teams: teams, channels and members go to Sytue Rooms, and each channel’s files and conversation history go to Sytue Drive.',
  'wizard.auto.item.teams': 'Teams and channels',
  'wizard.auto.item.teamFiles': 'Channel files and history',
  'wizard.auto.nots.teamsHistory':
    'Teams messages are not republished in Rooms. Rooms posts a message as whoever writes it now, so putting them there would fake either the author or the date. Each channel’s history travels as a faithful transcript in Drive, with the real author and date inside.',
  'wizard.auto.nots.teamsGuests':
    'Guests from other organisations, and channels shared with them: they have no account here, so we leave them out instead of inventing one.',
  'wizard.auto.scope.orgHintMicrosoft':
    'One sign-in only opens your own content. For the whole company, an administrator approves a single screen — no service accounts and nothing to paste. We take you straight there.',
  'wizard.auto.delegation.microsoftBody':
    'Open the Microsoft screen we link below with an administrator account and approve it. That is the whole step: the same application then reads the organisation, with the same read-only permissions.',
  'wizard.auto.delegation.whyMicrosoft':
    'Approving it does not give Sytue anything new: they are the same read permissions, now for the organisation instead of just your account. An administrator can revoke it in Entra whenever they want.',
  'migrations.autoStart': 'Bring everything over',
  'migrations.manualStart': 'Choose manually',
  'migrations.auto.title': 'Bring everything over in one sign-in',
  'migrations.auto.description':
    'Sign in to the service you are leaving and we will bring across everything that account can reach. You see the plan before anything is copied.',
  'wizard.auto.connected': 'Connected',
  'wizard.auto.source.googleDesc':
    'Your files and contacts, your Gmail, your calendar and your Google Cloud and Firebase apps. You sign in once and we read them; nothing changes on Google.',
  'wizard.auto.source.githubDesc':
    'Your repositories go to Sytue Git with their history, branches, tags, issues, pull requests, wiki and releases. Your gists go to Sytue Drive.',
  'wizard.auto.item.drive': 'Files',
  'wizard.auto.item.contacts': 'Contacts',
  'wizard.auto.item.repos': 'Repositories',
  'wizard.auto.item.gists': 'Gists',
  'wizard.auto.item.mail': 'Mail',
  'wizard.auto.item.calendar': 'Calendar',
  'wizard.auto.item.cloud': 'Google Cloud and Firebase apps',
  'wizard.auto.nots.cloudDatabases':
    'Databases (Cloud SQL, Firestore, BigQuery). They cannot be copied through a control API: they have to be exported and restored, and you decide when writes stop. We list every one of them with what to do about it.',
  'wizard.auto.nots.cloudImages':
    'Container images already running on Google. Sytue DevX builds from your repository: connect it to the project and the first deployment replaces them.',
  'wizard.auto.nots.actions':
    'GitHub Actions runs and their logs. The workflow files travel with the code; the run history stays behind.',
  'wizard.auto.nots.forgeAccount':
    'People who have never signed in to Sytue Git yet. Their repositories wait until they sign in once; then you resume the job.',
  'wizard.auto.nots.settings':
    'Repository settings that only exist on GitHub: branch protection, secrets, apps and webhooks.',
  'wizard.auto.step.origin': 'Where from',
  'wizard.auto.step.connect': 'Sign in',
  'wizard.auto.step.review': 'What we bring',
  'wizard.auto.origin.heading': 'What are you moving away from?',
  'wizard.auto.origin.description':
    'Pick the service. The only thing we will ask you for is to sign in to it.',
  'wizard.auto.connect.heading': 'Sign in to {source}',
  'wizard.auto.connect.description':
    'You approve the permissions on their screen. We only ever read; we never write to the service you are leaving.',
  'wizard.auto.connect.action': 'Sign in to {source}',
  'wizard.auto.connect.readOnly':
    'Read-only access. You can revoke it whenever you want, from here or from the provider.',
  'wizard.auto.connect.readOnlyGithub':
    'GitHub has no read-only permission for private repositories: the one it offers (repo) also allows writing. Sytue only ever reads, and the permission is yours to revoke at any moment from GitHub.',
  'wizard.auto.connect.checking': 'Checking what there is…',
  'wizard.auto.connect.doneTitle': 'Signed in',
  'wizard.auto.connect.doneBody': 'We are connected to {source} as {account}.',
  'wizard.auto.scope.mine': 'Just my account',
  'wizard.auto.scope.mineHint':
    'Everything the account you sign in with owns. One sign-in and nothing else — this is the usual choice, and every other person can do the same for themselves.',
  'wizard.auto.scope.oneClick': 'One sign-in',
  'wizard.auto.scope.org': 'Everyone in the organisation',
  'wizard.auto.scope.orgHint':
    'Google does not let one sign-in read other people’s content, so for the whole company it asks for one extra permission. We take you straight to the screen where you grant it.',
  'wizard.auto.delegation.title': 'One extra permission for the whole organisation',
  'wizard.auto.delegation.step1':
    'In the Google Cloud console, create a service account and download its JSON key.',
  'wizard.auto.delegation.step2': 'Paste that JSON here and save it.',
  'wizard.auto.delegation.step3':
    'Open the Google screen we link below: the client and the permissions come pre-filled, so you only have to authorise.',
  'wizard.auto.delegation.keyLabel': 'Service account JSON key',
  'wizard.auto.delegation.save': 'Save the key',
  'wizard.auto.delegation.open': 'Open the Google screen',
  'wizard.auto.delegation.recheck': 'Check again',
  'wizard.auto.delegation.why':
    'The key is stored encrypted and is only used to read, on behalf of each person, what belongs to them.',
  'wizard.auto.delegation.readyTitle': 'Permission granted',
  'wizard.auto.delegation.readyBody': {
    one: 'We can now bring across the content of {count} person.',
    other: 'We can now bring across the content of {count} people.',
  },
  'wizard.auto.review.heading': 'This is what we bring',
  'wizard.auto.review.description':
    'Nothing has been copied yet. Once you launch it, the copy runs in the background and you can close this page.',
  'wizard.auto.review.account': 'Signed in as',
  'wizard.auto.review.who': 'Who gets it',
  'wizard.auto.review.people': {
    one: '{count} person, each in their own account',
    other: '{count} people, each in their own account',
  },
  'wizard.auto.review.what': 'What we bring',
  'wizard.auto.review.jobs': 'Jobs we launch',
  'wizard.auto.review.jobsValue': {
    one: '{count} (one per person and type of content)',
    other: '{count} (one per person and type of content)',
  },
  'wizard.auto.review.launch': 'Bring it all over',
  'wizard.auto.unmatched.title': 'Accounts we cannot match',
  'wizard.auto.unmatched.body': {
    one: '{count} account from the old service has no equivalent in Sytue, so we leave it out rather than copy it into the wrong mailbox. Create that person in the directory and run this again.',
    other:
      '{count} accounts from the old service have no equivalent in Sytue, so we leave them out rather than copy them into the wrong mailbox. Create those people in the directory and run this again.',
  },
  'wizard.auto.limits.title': 'What does not come across',
  'wizard.auto.safe.title': 'Nothing is lost at the source',
  'wizard.auto.safe.body':
    'We only read. The service you are leaving stays exactly as it is, and you can run this again: whatever has already been copied is not duplicated.',
  'wizard.auto.error.title': 'That did not work',
  'wizard.auto.error.plan':
    'We could not check what there is. The sign-in may have expired: sign in again.',
  'wizard.auto.error.connect': 'We could not start the sign-in. Try again in a few seconds.',
  'wizard.auto.error.noProviderApp':
    'Sytue is not registered as an app with {source} yet, so there is no sign-in to send you to. An operator adds it under Platform administration → Third-party connections; until then, retrying changes nothing.',
  'wizard.auto.error.noKek':
    'This deployment has no encryption key configured, so it cannot store what {source} would hand back. An operator has to set it up.',
  'wizard.auto.error.serviceKey':
    'We could not save that key. Check that it is the full JSON of the service account.',
  'wizard.auto.error.launch': 'We could not launch the import. Try again in a few seconds.',
  'migrations.status.done': 'Finished',
  'migrations.status.running': 'Copying',
  'migrations.status.queued': 'Queued',
  'migrations.status.canceled': 'Stopped',
  'migrations.status.error': 'Needs attention',
  'migrations.counts.migrated': { one: '{count} copied', other: '{count} copied' },
  'migrations.counts.skipped': { one: '{count} skipped', other: '{count} skipped' },
  'migrations.counts.errors': { one: '{count} with an error', other: '{count} with errors' },
  'migrations.from': 'From {user}',
  'migrations.viewReport': 'View report',
  'migrations.stop': 'Stop',
  'migrations.resume': 'Resume',
  'migrations.wizard.title': 'Import content from Microsoft 365 or Google',
  'migrations.wizard.description':
    'We walk you through it step by step. Nothing is copied until the last step.',
  'migrations.toast.launched': 'Import under way',
  'migrations.toast.launchedHint': {
    one: '{count} import is being copied in the background. You can close this page.',
    other: '{count} imports are being copied in the background. You can close this page.',
  },
  'migrations.toast.stopped': 'Import stopped',
  'migrations.toast.stoppedHint':
    'What has already been copied stays where it is. You can resume whenever you want.',
  'migrations.toast.resumed': 'Import resumed',
  'migrations.toast.resumedHint':
    'It carries on where it left off; what was already copied is not duplicated.',
  'migrations.toast.actionFailed': 'Could not complete',
  'migrations.toast.reportFailed': 'We could not open the report',
  'migrations.active.title': 'In progress',
  'migrations.active.description':
    'The counters go up as things are copied. You can close this page: it keeps running in the background.',
  'migrations.finished.title': 'Finished',
  'migrations.list.title': 'Imports',
  'migrations.list.description':
    'Each person has their own import, with its own report of what arrived and what did not.',
  'migrations.empty.titleActive': 'None have finished yet',
  'migrations.empty.title': 'You have not imported anything yet',
  'migrations.empty.description':
    'Once you launch the first import, here you will see what reached each account and, just as plainly, what could not be copied and why.',
  'migrations.empty.action': 'Import the first one',
  'migrations.report.title': 'Import report',
  'migrations.report.migrated': 'copied to their account',
  'migrations.report.skipped': 'skipped (with a reason)',
  'migrations.report.content': 'of content',
  'migrations.report.whatFailed': 'What failed',
  'migrations.report.nothingLostTitle': 'Nothing was left behind',
  'migrations.report.nothingLostBody': 'Everything in the source reached their account.',
  'migrations.report.skippedTitle': 'What was NOT copied, and why',

  // --- Integraciones --------------------------------------------------------
  'apps.subtitle':
    'Connect the applications your team already uses so that they sign in with their Sytue account and joiners and leavers are handled automatically.',
  'apps.connect': 'Connect an app',

  // Registro dinámico de apps (RFC 7591), que no tenía pantalla.
  'apps.dcr.action': 'Registration token',
  'apps.dcr.title': 'Let an app register itself',
  'apps.dcr.onceTitle': 'Copy it now',
  'apps.dcr.onceBody':
    'This token is shown once. Sytue ID only keeps its hash, so closing this window without copying it means asking for another one.',
  'apps.dcr.token': 'Initial access token',
  'apps.dcr.endpoint': 'Registration endpoint',
  'apps.dcr.expires': 'Valid until {when}',
  'apps.dcr.copy': 'Copy token',
  'apps.dcr.copied': 'Copied.',
  'apps.dcr.failed': 'The token could not be issued',
  'apps.connectOne': 'Connect',
  'apps.wizard.description':
    'We walk you through it step by step. You can go back at any point; nothing is applied until the last step.',
  'apps.offlineBody':
    'Neither the catalogue nor the connections can be loaded or edited right now. Try again in a few minutes.',
  'apps.status.active': 'Connected',
  'apps.status.error': 'Error',
  'apps.status.disabled': 'Disabled',
  'apps.status.draft': 'Unfinished',
  'apps.cap.sso': 'single sign-on',
  'apps.cap.provisioning': 'automatic joiners and leavers',
  'apps.cap.data': 'data for reports',
  'apps.cap.line': 'Provides {what}.',
  'apps.cap.none': 'Access from your team’s app portal.',
  'apps.builtin.title': 'Built-in integrations',
  'apps.builtin.description':
    'Each person connects their clouds (Google Drive, OneDrive, Dropbox, Box), chats (Slack, Teams, Google Chat) and video calls from the app itself, just by giving consent. Here you can only turn them off for the whole organization.',
  'apps.builtin.failed': 'The change could not be saved. Try again.',
  'apps.map.title': 'Which app serves each product',
  'apps.map.description':
    'Drag a connected application onto a product, or click the product to choose. The change applies to the whole organisation.',
  'apps.connected.title': 'Connected applications',
  'apps.connected.description': 'The ones that already work with your team’s Sytue account.',
  'apps.empty.title': 'You have not connected any application yet',
  'apps.empty.description':
    'Once you connect the first one, your team signs in to it with their Sytue account and you will not have to create anybody by hand.',
  'apps.empty.action': 'Connect the first one',
  'apps.configured': 'Connection configured.',
  'apps.runs.none': 'No synchronisation recorded yet.',
  'apps.runs.last': 'Last sync: {created} added, {updated} changed, {deactivated} removed',
  'apps.runs.errors': { one: '{count} with an error', other: '{count} with errors' },
  'apps.open': 'Open',
  'apps.test': 'Test',
  'apps.sync': 'Synchronise',
  'apps.syncStatus': 'Status',
  'apps.deactivate': 'Deactivate',
  'apps.delete': 'Delete',
  'apps.catalog.title': 'You can connect',
  'apps.catalog.description':
    'Pick one and we guide you: five steps and it is ready, without reading any documentation.',
  'apps.catalog.emptyTitle': 'The catalogue is not available',
  'apps.catalog.emptyDescription':
    'As soon as the integrations service responds, you will see here every application you can connect.',
  'apps.catalog.allConnected': 'You have already connected every application in the catalogue.',
  'apps.deactivate.title': 'Deactivate the integration',
  'apps.deactivate.body':
    '{name} will stop appearing in the portal and its Sytue sign-in will be withdrawn. Nothing is deleted inside the application and you can connect it again whenever you want.',
  'apps.delete.title': 'Delete the integration',
  'apps.delete.body':
    'The {name} integration will be permanently deleted and its credentials revoked at the provider. To use it again, you will have to configure it from scratch.',
  'apps.toast.deactivated': 'Deactivated',
  'apps.toast.deactivatedHint': '{name} no longer appears in the portal.',
  'apps.toast.deleted': 'Deleted',
  'apps.toast.deletedHint': '{name} has been successfully deleted.',
  'apps.toast.deleteFailed': 'Failed to delete integration',
  'apps.toast.oauthConnected': 'Integration connected',
  'apps.toast.oauthConnectedHint': '{name} was successfully connected via OAuth2.',
  'apps.toast.testOk': 'Connection works',
  'apps.toast.testOkHint': '{name} responds: {what}.',
  'apps.toast.testSsoOnly':
    '{name} only uses signing in with Sytue: there is no credential to check.',
  'apps.toast.syncStarted': 'Synchronisation under way',
  'apps.toast.syncStartedHint':
    'It runs in the background; you can check the status in a few minutes.',
  'apps.toast.testFailed': 'The connection failed',
  'apps.toast.actionFailed': 'Could not complete',
  'apps.toast.gatewayDown': 'The application service or Sytue ID are not responding right now.',

  // --- Inicio de sesión único (SSO) -----------------------------------------
  'sso.subtitle':
    'Let your team sign in to Sytue with their company account (Microsoft Entra ID, Google Workspace, Okta…). You still decide who gets through.',
  'sso.connectProvider': 'Connect a provider',
  'sso.offlineBody':
    'Neither the providers nor their settings can be loaded or edited right now. Try again in a few minutes.',
  'sso.wizardDescription':
    'We walk you through it step by step. You can go back at any point; nobody can sign in this way until you enable it.',
  'sso.providers.title': 'Connected providers',
  'sso.providers.description':
    'Each provider adds a “Sign in with your organisation” button to the sign-in screen. Anyone who already has a Sytue password can keep using it.',
  'sso.providers.emptyTitle': 'You have not connected any provider yet',
  'sso.providers.emptyDescription':
    'Once you connect the first one, your team signs in with the company account and you will not have to manage separate passwords.',
  'sso.providers.emptyAction': 'Connect the first one',
  'sso.providers.caption': 'Identity providers',
  'sso.col.provider': 'Provider',
  'sso.col.connection': 'Connection',
  'sso.col.whoCanSignIn': 'Who can sign in',
  'sso.col.status': 'Status',
  'sso.unverified': 'Not tested',
  'sso.anyEmail': 'any email from the provider',
  'sso.test': 'Test',
  'sso.testAria': 'Test {name}',
  'sso.delete': 'Delete',
  'sso.deleteAria': 'Delete {name}',
  'sso.deleteTitle': 'Delete the provider',
  'sso.deleteBody':
    '{name} will stop appearing on the sign-in screen and the link between Sytue accounts and your provider’s accounts will be broken. Anyone with a Sytue password can carry on signing in with it; everybody else will have to recover theirs.',
  'sso.apps.title': 'Applications that sign in with Sytue',
  'sso.apps.description':
    'The other way round from above: here you register your team’s tools so that their sign-in button leads to Sytue instead of to Google or Microsoft.',
  'sso.apps.register': 'Register an application',
  'sso.apps.wizardDescription':
    'We walk you through it step by step. At the end we give you the values to paste into the application.',
  'sso.apps.emptyTitle': 'You have not registered any application yet',
  'sso.apps.emptyDescription':
    'If a tool you already use supports “sign in with” your own provider, it can use Sytue. We guide you in three steps and give you the values to paste into it.',
  'sso.apps.emptyAction': 'Register the first one',
  'sso.apps.caption': 'Applications that sign in with Sytue',
  'sso.apps.col.app': 'Application',
  'sso.apps.col.runsOn': 'Where it runs',
  'sso.apps.col.redirect': 'Returns to',
  'sso.apps.runsPublic': 'Browser or mobile (PKCE)',
  'sso.apps.runsConfidential': 'Your own server (with a secret)',
  'sso.apps.rotate': 'New secret',
  'sso.apps.rotateAria': 'Generate a new secret for {name}',
  'sso.apps.deleteTitle': 'Delete the application',
  'sso.apps.deleteBody':
    '{name} will no longer be able to sign in with Sytue, effective immediately, and anyone inside will have to sign in another way. Nothing is deleted inside the application; you can register it again whenever you want (with new values).',
  // SP SAML: apps de terceros para las que Sytue ID hace de IdP (las crea el catálogo
  // de plantillas al activar una app; hasta ahora no había forma de verlas ni apagarlas).
  'sso.saml.title': 'Applications that sign in with Sytue over SAML',
  'sso.saml.description':
    'Sytue ID is the identity provider for these applications. They appear here when you enable an app from the catalogue, and you can switch each one off without losing its configuration.',
  'sso.saml.caption': 'Applications that sign in with Sytue over SAML',
  'sso.saml.col.app': 'Application',
  'sso.saml.col.acs': 'Sends the response to',
  'sso.saml.emptyTitle': 'No application uses Sytue as its SAML provider yet',
  'sso.saml.emptyDescription':
    'Enable an application from the catalogue and it will show up here, ready to switch off or remove.',
  'sso.saml.incomplete': 'Incomplete',
  'sso.saml.details': 'Connection data',
  'sso.saml.detailsAria': 'Connection data for {name}',
  'sso.saml.detailsBody':
    'Paste these values into {name} so it trusts Sytue ID. They are the same for every application in your organisation.',
  'sso.saml.entityId': 'Identity provider (entity ID)',
  'sso.saml.metadataUrl': 'Metadata URL',
  'sso.saml.ssoUrl': 'Sign-in URL (SSO)',
  'sso.saml.sloUrl': 'Sign-out URL (SLO)',
  'sso.saml.initiateUrl': 'Start from Sytue',
  'sso.saml.deleteTitle': 'Remove the application',
  'sso.saml.deleteBody':
    '{name} will no longer be able to sign in with Sytue, effective immediately. Nothing is deleted inside the application, but you will have to register it again to restore access.',
  'sso.saml.toast.toggleFailed': 'Could not change the status',
  'sso.saml.toast.deleted': 'Application removed',
  'sso.secret.title': 'Copy the secret now',
  'sso.secret.body':
    'This is the new secret for {name}. The previous one is no longer valid: paste this one into the application as soon as you can. We will not be able to show it to you again.',
  'sso.secret.label': 'Client secret',
  'sso.secret.copied': 'Secret copied',
  'sso.secret.copyFailed': 'Could not copy',
  'sso.secret.copyFailedHint': 'Select it and copy it by hand.',
  'sso.secret.done': 'I have copied it',
  'sso.toast.testOk': 'All correct',
  'sso.toast.testOkSaml': 'Your provider’s values are valid.',
  'sso.toast.testOkOidc': 'Your provider responded.',
  'sso.toast.testOkOidcPkce': 'Your provider responded (with PKCE, the safest mode).',
  'sso.toast.testFailed': 'We could not check it',
  'sso.toast.testBadSaml':
    'Check the Entity ID, the sign-in URL (https) and the certificate. Edit it to fix them.',
  'sso.toast.testBadOidc':
    'The provider address does not respond with its configuration. Edit it and check the address.',
  'sso.toast.idpDown': 'Sytue ID is not responding right now. Try again in a minute.',
  'sso.toast.providerDeleted': 'Provider deleted',
  'sso.toast.deleteFailed': 'Could not delete',
  'sso.toast.rotateFailed': 'Could not generate a new secret',
  'sso.toast.rotateFailedHint': '{name} keeps the one it had.',
  'sso.toast.appDeleted': 'Application deleted',
  'sso.toast.appDeletedHint': '{name} can no longer sign in with Sytue.',

  // --- Roles personalizados y permisos delegables ---------------------------
  'roles.subtitle':
    'Custom roles with granular permissions (delegated administration). Assign them to users to delegate specific tasks without granting full access.',
  'roles.offline': 'No connection to Sytue ID: roles cannot be loaded or edited.',
  'roles.searchPlaceholder': 'Search role',
  'roles.create': 'Create role',
  'roles.delete': 'Delete role',
  'roles.permissions': 'Permissions',
  'roles.notDelegable':
    'Managing roles is reserved to owner/administrator (it cannot be delegated).',
  'roles.empty.title': 'No roles yet',
  'roles.empty.description':
    'Create a role (for example “Help Desk”) to delegate specific tasks without granting full access.',
  'roles.noResults': 'No role matches',
  'roles.noResultsHint': 'Try a different search text.',
  'roles.openAria': 'Open the {name} role',
  'roles.noPermissions': 'No permissions',
  'roles.namePlaceholder': 'e.g. Help Desk',
  'roles.fallbackTitle': 'Role',
  'roles.saveChanges': 'Save changes',
  'roles.discard': 'Discard',
  'roles.assigned': 'Assignees',
  'roles.noAssignees': 'This role is not assigned to anybody yet.',
  'roles.unassignAria': 'Remove the role from {name}',
  'roles.assignTo': 'Assign to a user',
  'roles.assignPlaceholder': 'Search a user of the organisation',
  'roles.noCandidates': 'No unassigned user matches.',
  'roles.assignHint': 'Type to search among the users of the organisation.',
  'roles.deleteConfirm':
    'Delete the “{name}” role? It will be removed from every assignee and they will lose those permissions. This action cannot be undone.',
  'roles.toast.membersFailed': 'Could not load the assignees',
  'roles.toast.created': 'Role created',
  'roles.toast.createdHint': '“{name}” now exists.',
  'roles.toast.createFailed': 'Could not create the role',
  'roles.toast.nameTaken': 'A role with that name already exists.',
  'roles.toast.checkNameAndPerms': 'Check the name and the selected permissions.',
  'roles.toast.checkPerms': 'Check the selected permissions.',
  'roles.toast.updated': 'Role updated',
  'roles.toast.assignFailed': 'Could not assign',
  'roles.toast.alreadyAssigned': 'That user already has the role.',
  'roles.toast.unassignFailed': 'Could not remove the role',
  'roles.toast.deleted': 'Role deleted',
  'roles.toast.deletedHint': '“{name}” no longer exists.',
  'roles.toast.deleteFailed': 'Could not delete the role',
  'perm.usersManage': 'Manage users',
  'perm.usersManageHint': 'Create/remove, role, password and MFA reset, sessions.',
  'perm.usersRead': 'View users',
  'perm.usersReadHint': 'Directory and sessions (read only).',
  'perm.groupsManage': 'Manage groups',
  'perm.groupsManageHint': 'Organisation groups and their membership.',
  'perm.ssoManage': 'Manage SSO',
  'perm.ssoManageHint': 'Sign-in providers (OIDC/SAML).',
  'perm.accessManage': 'Conditional access',
  'perm.accessManageHint': 'Rules evaluated at sign-in.',
  'perm.appsManage': 'Integrations',
  'perm.appsManageHint': 'Third-party apps (outbound SAML/OIDC, DCR).',
  'perm.auditView': 'View audit',
  'perm.auditViewHint': 'Audit trail of the organisation.',
  'perm.billingManage': 'Billing',
  'perm.billingManageHint': 'Billing of the organisation.',

  // --- Acceso privilegiado (PIM) --------------------------------------------
  'pim.subtitle':
    'Just-in-time privilege elevation (PIM). Eligible users activate a role for a bounded window, with a justification and — where required — approval. It only grants permissions while it is active.',
  'pim.offline': 'No connection to Sytue ID: privileged access cannot be loaded or operated.',
  'pim.status.pending': 'Pending',
  'pim.status.active': 'Active',
  'pim.status.revoked': 'Revoked',
  'pim.status.denied': 'Denied',
  'pim.status.expired': 'Expired',
  'pim.tab.me': 'My access',
  'pim.tab.admin': 'Administration',
  'pim.me.eligibleTitle': 'Roles I can activate',
  'pim.me.emptyTitle': 'You are not eligible for any role',
  'pim.me.emptyDescription':
    'Once an administrator makes you eligible, you will be able to elevate privileges here for a bounded window.',
  'pim.me.activationsTitle': 'My activations',
  'pim.me.noActivations': 'You have not activated any role yet.',
  'pim.requiresApproval': 'Requires approval',
  'pim.directActivation': 'Direct activation',
  'pim.withApproval': 'with approval',
  'pim.directActivationShort': 'direct activation',
  'pim.maxDuration': 'max. {minutes} min',
  'pim.minutes': '{minutes} min',
  'pim.minutesLeft': '~{minutes} min left',
  'pim.expiresAt': 'Expires {when}',
  'pim.activate': 'Activate',
  'pim.request': 'Request',
  'pim.deactivate': 'Deactivate',
  'pim.approve': 'Approve',
  'pim.deny': 'Deny',
  'pim.noJustification': 'no justification',
  'pim.admin.pendingTitle': 'Pending requests',
  'pim.admin.noPending': 'There are no requests waiting for approval.',
  'pim.admin.activeTitle': 'Active right now',
  'pim.admin.noActive': 'No privileged role is active right now.',
  'pim.admin.eligibilitiesTitle': 'Eligibilities',
  'pim.admin.newEligibility': 'New eligibility',
  'pim.admin.emptyTitle': 'No eligibilities yet',
  'pim.admin.emptyDescription':
    'Make a user eligible for a role (for example “Help Desk” or administrator) and they will be able to elevate privileges just in time, with or without approval.',
  'pim.removeEligibilityAria': 'Withdraw the eligibility of {user} for {role}',
  'pim.activateTitle': 'Activate “{name}”',
  'pim.activateFallbackTitle': 'Activate role',
  'pim.approvalNotice':
    'This role requires approval: it stays pending until an administrator grants it.',
  'pim.justification': 'Justification',
  'pim.justificationPlaceholder': 'Reason for the elevation (it is audited)',
  'pim.durationLabel': 'Duration (minutes)',
  'pim.durationHelp': 'At most {minutes} min for this role.',
  'pim.newElig.user': 'User',
  'pim.newElig.selectUser': 'Choose a user…',
  'pim.newElig.role': 'Role',
  'pim.newElig.selectRole': 'Choose a role…',
  'pim.newElig.customRoles': 'Custom roles',
  'pim.newElig.builtinRoles': 'Built-in roles',
  'pim.newElig.builtinAdmin': 'admin (superuser)',
  'pim.newElig.builtinOwner': 'owner (superuser)',
  'pim.newElig.maxLabel': 'Maximum duration (minutes)',
  'pim.newElig.maxHelp': 'Maximum window of each activation (1–1440).',
  'pim.newElig.requiresApproval': 'Requires an administrator’s approval',
  'pim.newElig.submit': 'Create eligibility',
  'pim.toast.requested': 'Request sent',
  'pim.toast.requestedHint': 'An administrator has to approve it.',
  'pim.toast.activated': 'Role activated',
  'pim.toast.activatedHint': 'Valid for {minutes} min.',
  'pim.toast.activateFailed': 'Could not activate',
  'pim.toast.notEligible': 'You are no longer eligible for that role.',
  'pim.toast.checkDuration': 'Check the duration (it exceeds the maximum) and the justification.',
  'pim.toast.deactivated': 'Role deactivated',
  'pim.toast.deactivateFailed': 'Could not deactivate',
  'pim.toast.approved': 'Request approved',
  'pim.toast.denied': 'Request denied',
  'pim.toast.decideFailed': 'Could not complete',
  'pim.toast.eligCreated': 'Eligibility created',
  'pim.toast.eligCreateFailed': 'Could not create it',
  'pim.toast.eligDuplicate': 'That user is already eligible for that role.',
  'pim.toast.eligCheckFields': 'Check the user, the role and the maximum duration.',
  'pim.toast.eligRemoved': 'Eligibility withdrawn',
  'pim.toast.eligRemoveFailed': 'Could not withdraw it',

  // --- Grupos y reglas de membresía dinámica --------------------------------
  'groups.subtitle':
    'Groups of the organisation in Sytue ID. They drive per-group sharing in Sytue Drive.',
  'groups.offline': 'No connection to Sytue ID: sample data is shown. Changes will not be saved.',
  'groups.searchPlaceholder': 'Search group',
  'groups.create': 'Create group',
  'groups.delete': 'Delete group',
  'groups.fallbackTitle': 'Group',
  'groups.empty.title': 'No groups yet',
  'groups.empty.description':
    'Create a group to organise the people of the organisation and share resources per group.',
  'groups.noResults': 'No group matches',
  'groups.openAria': 'Open the {name} group',
  'groups.dynamic': 'Dynamic',
  'groups.noDescription': 'No description',
  'groups.description': 'Description',
  'groups.namePlaceholder': 'e.g. Engineering',
  'groups.dynamicSwitch': 'Dynamic group (membership by rule)',
  'groups.dynamicHelp':
    'Membership is calculated from each user’s attributes (department, job title, location…). Members are not added by hand.',
  'groups.ruleMembership': 'Membership by rule',
  'groups.ruleMembershipHelp': 'Membership is calculated by this rule from each user’s attributes.',
  'groups.convertSwitch': 'Turn into a dynamic group (membership by rule)',
  'groups.convertWarning':
    'When you save the rule, this group becomes dynamic and its current members are replaced by the ones the rule dictates.',
  'groups.saveRule': 'Save rule',
  'groups.convert': 'Turn into dynamic',
  'groups.recompute': 'Recalculate now',
  'groups.recomputeAll': 'Recalculate all',
  'groups.recomputeHelp':
    'Membership updates itself whenever a user’s attributes change (including the SCIM sync from your directory). Recalculate if you want to check it right now.',
  'groups.members': 'Members',
  'groups.computedMembers': 'Calculated members',
  'groups.noMembers': 'The group has no members yet.',
  'groups.noRuleMatches': 'No user matches the rule yet.',
  'groups.removeMemberAria': 'Remove {name} from the group',
  'groups.addMember': 'Add member',
  'groups.deleteConfirm':
    'Delete the “{name}” group? Every member is removed and the associated per-group sharing is lost. This action cannot be undone.',
  'groups.rule.include': 'Include whoever matches',
  'groups.rule.matchAll': 'ALL the conditions',
  'groups.rule.matchAny': 'ANY condition',
  'groups.rule.attribute': 'Attribute',
  'groups.rule.operator': 'Operator',
  'groups.rule.value': 'value',
  'groups.rule.valueListHint': 'value1, value2, …',
  'groups.rule.removeCondition': 'Remove condition',
  'groups.rule.addCondition': 'Add condition',
  'groups.rule.preview': 'Preview members',
  'groups.rule.previewNone': 'No user matches.',
  'groups.rule.previewCount': {
    one: '{count} user would match.',
    other: '{count} users would match.',
  },
  'groups.toast.membersFailed': 'Could not load the members',
  'groups.toast.sampleData': 'No connection to Sytue ID: showing sample data.',
  'groups.toast.previewFailed': 'Could not preview',
  'groups.toast.previewFailedHint': 'Check that every condition has a value.',
  'groups.toast.created': 'Group created',
  'groups.toast.dynamicCreated': 'Dynamic group created',
  'groups.toast.createdHint': '“{name}” now exists.',
  'groups.toast.createFailed': 'Could not create the group',
  'groups.toast.nameTaken': 'A group with that name already exists.',
  'groups.toast.checkName': 'Check the name and try again.',
  'groups.toast.checkNameAndRule': 'Check the name and the rule and try again.',
  'groups.toast.updated': 'Group updated',
  'groups.toast.alreadyUpToDate': 'Membership was already up to date',
  'groups.toast.recomputed': 'Membership recalculated',
  'groups.toast.recomputedSame': {
    one: '{count} member according to the rule.',
    other: '{count} members according to the rule.',
  },
  'groups.toast.recomputedAdded': { one: '{count} added', other: '{count} added' },
  'groups.toast.recomputedRemoved': { one: '{count} removed', other: '{count} removed' },
  'groups.toast.recomputedAllGroups': {
    one: '{count} dynamic group',
    other: '{count} dynamic groups',
  },
  'groups.toast.recomputeFailed': 'Could not recalculate',
  'groups.toast.ruleSaved': 'Rule saved',
  'groups.toast.ruleSavedHint': 'Membership recalculated.',
  'groups.toast.ruleSaveFailed': 'Could not save the rule',
  'groups.toast.ruleInvalid': 'The rule is not valid: check the conditions and values.',
  'groups.toast.addFailed': 'Could not add',
  'groups.toast.alreadyMember': 'That user is already a member.',
  'groups.toast.removeMemberFailed': 'Could not remove the member',
  'groups.toast.deleted': 'Group deleted',
  'groups.toast.deletedHint': '“{name}” no longer exists.',
  'groups.toast.deleteFailed': 'Could not delete the group',
  'rule.attr.department': 'Department',
  'rule.attr.title': 'Job title',
  'rule.attr.location': 'Location',
  'rule.attr.email': 'Email',
  'rule.attr.name': 'Name',
  'rule.op.eq': 'is equal to',
  'rule.op.ne': 'is not',
  'rule.op.contains': 'contains',
  'rule.op.startsWith': 'starts with',
  'rule.op.in': 'is in (list)',

  // --- Gobernanza (política de egress) --------------------------------------
  'gov.subtitle':
    'Control what your integrations may send out. Every call to a third-party service goes through here, is recorded and cannot be bypassed.',
  'gov.offlineTitle': 'We cannot reach the integrations gateway',
  'gov.offlineBody':
    'Neither the rules nor the logs can be loaded or edited right now. Try again in a few minutes.',
  'gov.tab.rules': 'Rules',
  'gov.tab.egress': 'Egress log',
  'gov.tab.decisions': 'Decisions',
  'gov.badge.block': 'Blocks',
  'gov.badge.warn': 'Warns',
  'gov.badge.allow': 'Lets through',
  'gov.act.warn': 'Let it through, but tell you',
  'gov.act.warnBody':
    'The call goes out all the same and the integration keeps working, but it is recorded under “Decisions”. This is how you try a rule out before cutting anything off.',
  'gov.act.warnBadge': 'The safest way to start',
  'gov.act.block': 'Block the call',
  'gov.act.blockBody':
    'The call does not leave Sytue. The integration gets an error and that feature of it stops working. Use it when that data must never leave.',
  'gov.act.allow': 'Let it through, full stop',
  'gov.act.allowBody':
    'It goes out as usual and no decision is recorded. It is there to make an exception to another, stricter rule.',
  'gov.field.host': 'The site being called',
  'gov.field.hostHelp':
    'The domain of the service, for example api.salesforce.com. If you start it with a dot (.salesforce.com) it covers every subdomain.',
  'gov.field.family': 'The integration',
  'gov.field.familyHelp':
    'The key of the template you connected the service with. Use it when the specific domain does not matter.',
  'gov.field.method': 'The kind of call',
  'gov.field.methodHelp':
    'GET only reads; POST, PUT, PATCH and DELETE write or delete. Leave it empty for any of them.',
  'gov.field.path': 'The part of the API',
  'gov.field.pathHelp':
    'The start of the path, in case you only want part of the service. Leave it empty for the whole API.',
  'gov.field.body': 'A text inside what is sent',
  'gov.field.bodyHelp':
    'For data leaks: it acts if the sent content includes that word. It can only be inspected in small payloads, so do not use it as the only condition.',
  'gov.cond.any': 'Any egress',
  'gov.cond.all': 'all calls to third parties, without exception',
  'gov.cond.calls': 'calls {what}',
  'gov.cond.subdomain': 'to any subdomain of {host}',
  'gov.cond.host': 'to {host}',
  'gov.cond.family': 'of the {family} integration',
  'gov.cond.method': 'with method {method}',
  'gov.cond.path': 'whose path starts with {path}',
  'gov.cond.body': 'that send “{text}” in the content',
  'gov.suggest.block': 'Block {what}',
  'gov.suggest.warn': 'Warn about {what}',
  'gov.suggest.allow': 'Allow {what}',
  'gov.impact.allowTitle': 'This rule cannot cut anything off',
  'gov.impact.allowBody':
    'Rules that let traffic through never break an integration: they only make an exception to a stricter one.',
  'gov.impact.wideTitle': 'This rule would cut off ALL your integrations',
  'gov.impact.wideBody':
    'With no condition, the rule reaches every call to third parties: data would stop syncing and signing in to the connected apps would fail. Go back to the first step and narrow at least the site being called.',
  'gov.impact.unknownTitle': 'We cannot estimate the impact',
  'gov.impact.unknownBody':
    'The egress log is empty, so we do not know which integrations would be affected. If in doubt, create it as a warning and watch it for a few days before blocking.',
  'gov.impact.noneTitle': 'Nothing that works today would be affected',
  'gov.impact.noneBody': {
    one: '{count} call recorded, and it does not match this rule.',
    other: 'None of the last {count} recorded calls match this rule.',
  },
  'gov.impact.blockTitle': {
    one: 'This rule would cut off traffic that is in use ({matched} of {count} call)',
    other: 'This rule would cut off traffic that is in use ({matched} of the last {count} calls)',
  },
  'gov.impact.warnTitle': {
    one: 'This rule would flag {matched} of {count} call',
    other: 'This rule would flag {matched} of the last {count} calls',
  },
  'gov.impact.blockBody':
    'These would have been blocked: {hosts}. If any belongs to an integration in use, that feature stops working as soon as you save. Create it as a warning first if you would rather check without breaking anything.',
  'gov.impact.warnBody':
    'These would have been flagged: {hosts}. If any belongs to an integration in use, that feature stops working as soon as you save.',
  'gov.impact.upperBound':
    'The count is an upper bound: the path and the content cannot be checked from the log.',
  'gov.step.traffic': 'Which traffic',
  'gov.step.trafficHeading': 'Which calls should it affect?',
  'gov.step.trafficDescription':
    'Fill in only what you need: every field you leave empty is a wildcard. The usual choice is to narrow by the site being called.',
  'gov.step.wideTitle': 'With no conditions, it affects everything',
  'gov.step.wideBody':
    'As it stands, the rule reaches every call to third parties across the whole organisation. That is what you want for a general warning rule, but rarely for a blocking one.',
  'gov.step.willMatch': 'This rule will look at',
  'gov.step.action': 'What to do',
  'gov.step.actionHeading': 'What should happen to those calls?',
  'gov.step.actionDescription': 'Choose by the consequence, not by the technical name.',
  'gov.step.orderNote':
    'Rules are checked in order. A “let through” rule placed earlier acts as an exception to the ones that come after it.',
  'gov.step.review': 'Summary',
  'gov.step.reviewNew': 'Review and save the rule',
  'gov.step.reviewEdit': 'Review the changes',
  'gov.step.reviewDescription':
    'We check against the recent log which real traffic this rule would touch.',
  'gov.step.done': 'Done',
  'gov.step.savedChanges': 'Changes saved',
  'gov.review.affects': 'What it affects',
  'gov.review.happens': 'What happens',
  'gov.review.willApply': 'It applies as soon as you save it',
  'gov.review.savedOff': 'Saved without applying',
  'gov.createRule': 'Create the rule',
  'gov.newRuleTitle': 'New traffic rule',
  'gov.wizardDescription':
    'Two questions and a summary. Before saving we tell you whether the rule would be cutting off calls that are being made right now.',
  'gov.ruleName': 'Name of the rule',
  'gov.ruleNameHint':
    'This is how you will see it in the list. If you leave it empty we will call it “{suggested}”.',
  'gov.applyNow': 'Apply the rule right away',
  'gov.applyNowOn': 'It starts being evaluated on the next call to third parties.',
  'gov.applyNowOff':
    'It will be saved switched off: you can prepare it now and switch it on from the list whenever you want.',
  'gov.done.on': 'It is already evaluated on every call to third parties: {what}.',
  'gov.done.off': 'It is saved but switched off: turn it on from the list whenever you want.',
  'gov.done.checkTitle': 'Check it before you go',
  'gov.done.checkBodyBlock':
    'Open the “Decisions” tab in a while: there you will see every call the rule has blocked. If an integration that had to keep working shows up, switch the rule off from the list and narrow the condition.',
  'gov.done.checkBodyWarn':
    'Open the “Decisions” tab in a while: there you will see every call the rule has flagged. If an integration that had to keep working shows up, switch the rule off from the list and narrow the condition.',
  'gov.done.back': 'Back to the rules',
  'gov.rules.title': 'Outbound traffic rules',
  'gov.rules.description':
    'They are checked top to bottom on every call an integration makes to an outside service. With no rules, everything goes out (and is logged anyway).',
  'gov.rules.emptyDescription':
    'Without rules, any connected integration can call wherever it likes. The one most people create first: warn when something is sent out, to see the real traffic before blocking anything.',
  'gov.rules.emptyAction': 'Create the first rule',
  'gov.col.action': 'Action',
  'gov.col.rule': 'Rule',
  'gov.col.enabled': 'On',
  'gov.col.site': 'Site',
  'gov.col.call': 'Call',
  'gov.col.response': 'Response',
  'gov.col.took': 'Took',
  'gov.col.decision': 'Decision',
  'gov.col.destination': 'Destination',
  'gov.toggleAria': 'Apply the rule {name}',
  'gov.toast.toggleFailed': 'Could not change the state',
  'gov.toast.badCondition':
    'The gateway rejected some value. Check the conditions in the first step.',
  'gov.toast.forbidden':
    'Your account does not have permission to change the rules of this organisation.',
  'gov.toast.gatewayDown': 'The gateway is not responding right now. Try again in a minute.',
  'gov.toast.created': 'Rule created',
  'gov.delete.intro': '{name} will be deleted.',
  'gov.delete.afterTitle': 'From that moment on',
  'gov.delete.afterBlock':
    'The traffic this rule was blocking goes out of Sytue as usual again. If you only want to pause it, switch it off from the list instead of deleting it: that way you keep the condition.',
  'gov.delete.afterWarn':
    'That traffic stops being recorded under “Decisions”. What is already logged is kept.',
  'gov.delete.confirm': 'Delete the rule',
  'gov.networkError': 'network error',
  'gov.egress.title': 'Egress log',
  'gov.egress.description':
    'Every call an integration has made to an outside service: where, when and how it answered. Metadata only, never the content or the credentials.',
  'gov.egress.offline': 'No connection to the gateway: the egress log cannot be loaded.',
  'gov.egress.loadFailed': 'Could not load the log. Adjust the filters and try again.',
  'gov.egress.emptyTitle': 'Nothing logged yet',
  'gov.egress.emptyDescription':
    'There are no calls to third parties for that filter yet. As soon as an integration reaches the network, it appears here.',
  'gov.exportCsv': 'Export CSV',
  'gov.filterHost': 'Destination site',
  'gov.filterHostAria': 'Filter by destination site',
  'gov.filterStatusAria': 'Filter by HTTP response code',
  'gov.applyFilters': 'Apply filters',
  'gov.decisions.title': 'What your rules have done',
  'gov.decisions.description':
    'Every time a rule blocks or flags a call, it lands here. A block leaves no trace in the egress log (the call never went out): this is its only evidence.',
  'gov.decisions.filterAria': 'Filter by kind of decision',
  'gov.decisions.onlyBlocks': 'Blocks only',
  'gov.decisions.onlyWarnings': 'Warnings only',
  'gov.decisions.offline': 'No connection to the gateway: the decisions cannot be loaded.',
  'gov.decisions.loadFailed': 'Could not load the decisions. Try again in a few seconds.',
  'gov.decisions.emptyTitle': 'No rule has acted yet',
  'gov.decisions.emptyDescription':
    'Nothing has been blocked or flagged. When a rule acts on a call, you will see it here with its date and its destination.',
  'gov.decision.blocked': 'Blocked',
  'gov.decision.warned': 'Flagged',

  // --- Cumplimiento (retención legal + eDiscovery) --------------------------
  'comp.subtitle':
    'Preserve someone’s email and files for as long as a case lasts, and search or download their content when the law requires it. This is access to other people’s data: everything you do here is recorded under your name.',
  'comp.tab.holds': 'Legal holds',
  'comp.service.mail': 'Email',
  'comp.service.drive': 'Files',
  'comp.service.notes': 'Notepads',
  'comp.noun.mail': 'the email',
  'comp.noun.drive': 'the files',
  'comp.noun.notes': 'their notebooks',
  'comp.offline.all': 'We cannot reach Email, Files or Notepads',
  'comp.offline.one': 'We cannot reach {service}',
  'comp.offline.body':
    'What you see may be incomplete and actions on that service will fail. Try again in a few minutes.',
  'comp.scope.title': 'What is preserved',
  'comp.scope.help':
    'Whatever you do not tick here keeps being deleted as usual, so when in doubt preserve more: releasing later is instant, recovering what was purged is not.',
  'comp.scope.usual': 'The usual choice',
  'comp.scope.bothLabel': 'Their email and their files',
  'comp.scope.bothShort': 'Email and files',
  'comp.scope.bothNoun': 'their email and their files',
  'comp.scope.bothBody':
    'The usual choice in litigation: nothing that account deletes really disappears, neither in the mailbox nor in Drive.',
  'comp.scope.mailLabel': 'Only their email',
  'comp.scope.mailShort': 'Email',
  'comp.scope.mailNoun': 'their email',
  'comp.scope.mailBody':
    'The mailbox is preserved. Drive files keep being deleted as always, including the ones already in the bin.',
  'comp.scope.driveLabel': 'Only their files',
  'comp.scope.driveShort': 'Files',
  'comp.scope.driveNoun': 'their files',
  'comp.scope.driveBody':
    'Drive files are preserved. Email keeps being deleted as always, including what is already in the bin.',
  'comp.scope.notesLabel': 'Only their notebooks',
  'comp.scope.notesShort': 'Notepads',
  'comp.scope.notesNoun': 'their notebooks',
  'comp.scope.notesBody':
    'Notepads notebooks are preserved. Email and Drive files keep being deleted as always, including what is already in the bin.',
  'comp.scope.offlineTitle': '{services} is not responding',
  'comp.scope.offlineBody':
    'That part of the hold will fail. You can create it anyway (whatever does respond will be preserved) and repeat it later for what is missing.',
  'comp.subject.wholeOrg': 'The whole organisation',
  'comp.subject.wholeOrgInline': 'your whole organisation',
  'comp.subject.thatPerson': 'that person',
  'comp.subject.notChosen': 'Not chosen',
  'comp.sentence': '{what} of {subject} will be preserved until you release the hold.',
  'comp.step.who': 'Who',
  'comp.step.whoHeading': 'Whose data has to be preserved?',
  'comp.step.whoDescription':
    'Start by deciding which account it affects and what is preserved from it.',
  'comp.who.person': 'One person',
  'comp.who.personBody':
    'Only that account is preserved. This is the normal case when the matter points at someone specific.',
  'comp.who.orgBody':
    'Every account is preserved. Use it only if the case reaches the whole company: nothing will be purged while it is active and storage will keep growing.',
  'comp.who.selectFromDirectory': 'Choose someone from the directory…',
  'comp.who.thisAccount': 'this account',
  'comp.who.willPreserve': 'The data of {email} will be preserved.',
  'comp.who.pickHint':
    'Pick them from the directory: the hold is tied to their identity, not to their email address, so it still holds if their name changes.',
  'comp.step.impact': 'What it means',
  'comp.step.impactHeading': 'This is what changes while it is active',
  'comp.step.impactDescription':
    'None of this applies yet. Read it and carry on if this is what you want.',
  'comp.impact.noDeleteTitle': 'Nothing is really deleted',
  'comp.impact.noDeleteBody':
    'While it is active, neither {subject} nor you nor any other administrator can permanently delete {what}. They can keep deleting whatever they like: a copy is kept that is only visible from eDiscovery.',
  'comp.impact.silentTitle': 'The person does not find out',
  'comp.impact.silentBody':
    'Their account works exactly the same and shows no notice. That is what procedures usually require: nobody must know that data is being preserved.',
  'comp.impact.auditedTitle': 'It is recorded under your name',
  'comp.impact.auditedBody':
    'Who creates it, when and why is recorded. That record is the proof that you preserved in time, so the reason matters.',
  'comp.impact.releaseTitle': 'It is released when the case ends',
  'comp.impact.releaseBody':
    'You release it from the list and normal deletion resumes. Whatever was in the bin becomes purgeable.',
  'comp.impact.orgWideTitle': 'You are about to hold the whole organisation',
  'comp.impact.orgWideBody':
    'No account will be able to purge anything while the hold is active, and the space used can only grow. If the case points at specific people, go back and choose them: it is cheaper and easier to defend.',
  'comp.impact.whatForTitle': 'What a legal hold is for',
  'comp.impact.whatForBody':
    'For litigation, internal investigations and legal requirements: it freezes the evidence before anyone deletes it. It is not a backup and it is not for archiving.',
  'comp.reason.label': 'Reason',
  'comp.reason.required': 'Write why it is being held: this is what the audit trail will show.',
  'comp.reason.placeholder': 'Employment claim from a former employee, court order no. 4',
  'comp.reason.ok': 'It is stored verbatim in the audit trail.',
  'comp.reason.hint':
    'It is needed to continue. Describe the case, not the action: “hold” tells nobody anything.',
  'comp.step.reasonHeading': 'Why is it being held?',
  'comp.step.reasonDescription':
    'It is the first thing whoever reviews the case in two years will read, yourself included. One concrete sentence is enough.',
  'comp.case.label': 'Case reference',
  'comp.case.none': 'No reference',
  'comp.case.hint':
    'Optional. The identifier you use in your case management tool: it groups several holds of the same matter and labels the eDiscovery exports.',
  'comp.tipTitle': 'Tip',
  'comp.tipBody':
    'If the case reaches several people, create one hold per person with the same reference: you will be able to release them separately as each part closes.',
  'comp.review.who': 'Who it affects',
  'comp.review.starts': 'When it starts',
  'comp.review.startsNow': 'As soon as you create it',
  'comp.review.sentenceTitle': 'The hold, in one sentence',
  'comp.review.immediateTitle': 'It applies immediately and is recorded',
  'comp.review.immediateBody':
    'From the moment you create it, nothing of {what} can be purged. The action is recorded in the audit trail with your name, the date and the reason.',
  'comp.step.reviewHeading': 'Review and create the hold',
  'comp.step.reviewDescription':
    'This is what we are going to preserve. You can change anything before creating it.',
  'comp.create': 'Create the hold',
  'comp.newHold': 'New legal hold',
  'comp.newHoldShort': 'New hold',
  'comp.wizardDescription':
    'Three questions and a summary. Before creating it we tell you exactly what stops being deletable.',
  'comp.done.heading': 'Hold active',
  'comp.done.rejectedTitle': '{services} has not accepted it',
  'comp.done.rejectedBody':
    'That part is NOT preserved. When the service is back, create another hold with the same reason and reference just for {services}.',
  'comp.done.nextTitle': 'What usually comes next',
  'comp.done.nextSearch':
    'Search the content of the case from the eDiscovery tab: the hold guarantees it is still there even if it gets deleted.',
  'comp.done.nextRelease':
    'When the case closes, release it from the list. Until then, leave it alone: releasing and re-creating leaves a gap in which purging was possible.',
  'comp.done.back': 'Back to the holds',
  'comp.holds.title': 'Holds',
  'comp.holds.descriptionActive': 'The ones preserving data right now.',
  'comp.holds.descriptionAll':
    'Active and already released. The released ones are kept as evidence of when preservation stopped.',
  'comp.holds.showReleased': 'Show released ones too',
  'comp.holds.whatTitle': 'What a legal hold does',
  'comp.holds.whatBody':
    'It freezes what is in an account for litigation: the person can keep deleting their email and files, but nothing is really removed until you release the hold. Creating and releasing them is privileged access and is recorded under your name.',
  'comp.holds.emptyActive': 'There is no active hold',
  'comp.holds.emptyEver': 'None has ever been created',
  'comp.holds.emptyDescription':
    'A hold preserves someone’s email and files for as long as a case lasts. It takes a minute to create and is released when the case closes.',
  'comp.holds.emptyAction': 'Create the first hold',
  'comp.col.source': 'Source',
  'comp.col.subject': 'Subject',
  'comp.col.case': 'Case',
  'comp.col.created': 'Created',
  'comp.col.item': 'Item',
  'comp.col.size': 'Size',
  'comp.status.preserving': 'Preserving',
  'comp.status.released': 'Released',
  'comp.status.deletedPreserved': 'Deleted, preserved',
  'comp.release': 'Release',
  'comp.releaseAria': 'Release the hold of {subject} on {service}',
  'comp.release.title': 'Release the hold',
  'comp.release.intro': 'You will stop preserving {what} of {subject}.',
  'comp.release.point1': 'The account’s normal deletion applies again.',
  'comp.release.point2': 'Whatever is in the bin can be purged and stops appearing in eDiscovery.',
  'comp.release.point3':
    'What has already been purged cannot be recovered: if the case is still open, do not release it yet.',
  'comp.release.audited':
    'The release is recorded with your name and the date. The hold is kept in the history (to see it: “Show released ones too”).',
  'comp.release.confirm': 'Release and go back to normal deletion',
  'comp.toast.createFailed': 'Could not create it',
  'comp.toast.createRejected': 'Neither Email nor Files accepted the hold. Try again.',
  'comp.toast.badSubject':
    'The service rejected some value. Check who it affects in the first step.',
  'comp.toast.forbidden':
    'Your account does not have permission to create holds in this organisation.',
  'comp.toast.serviceDown': 'The service is not responding right now. Try again in a minute.',
  'comp.toast.reloadFailed': 'Could not reload the list',
  'comp.toast.released': 'Hold released',
  'comp.toast.releasedHint': '{subject} goes back to normal deletion.',
  'comp.toast.releaseFailed': 'Could not release it',
  'comp.toast.releaseFailedHint': 'It is still active. Try again in a few seconds.',
  'comp.toast.noSubject': 'You have not said whose',
  'comp.toast.noSubjectHint':
    'Pick from the list the person whose content you are going to look at.',
  'comp.toast.searchFailed': 'Could not search',
  'comp.toast.badFilters': 'Some filter does not add up. Check the dates and try again.',
  'comp.search.warningTitle': 'You are about to look at someone else’s content',
  'comp.search.warningBody':
    'Here you see and download the email and files of someone in your organisation, without that person knowing. Every search and every download is recorded with your name, the date and whose content you looked at. Only use it with a case behind it.',
  'comp.search.title': 'Search someone’s content',
  'comp.search.description':
    'Results combine email and files and only show header data: who, when and how big. To see the content you have to export.',
  'comp.search.where': 'Where to search',
  'comp.search.keyword': 'Keyword',
  'comp.search.keywordHelp':
    'In email it searches subject, sender and body; in files, the name. Leave it empty to see everything.',
  'comp.search.keywordPlaceholder': 'contract',
  'comp.search.caseHelp': 'It is stored in the audit trail together with the search.',
  'comp.search.bothDown':
    'Neither Email nor Files is responding right now: the search will return nothing.',
  'comp.search.oneDown':
    '{service} is not responding right now: that half of the results is missing.',
  'comp.search.partialTitle': 'Could not search in {services}',
  'comp.search.partialBody':
    'What you see below is incomplete. Run the search again in a few minutes before considering the case reviewed.',
  'comp.search.searching': 'Searching the content of {subject}…',
  'comp.search.emptyTitle': 'You have not searched anything yet',
  'comp.search.emptyDescription':
    'Choose the person and, if you know it, narrow the period. The more you narrow it, the less of someone else’s content you end up seeing.',
  'comp.search.noResultsTitle': 'Nothing matches those filters',
  'comp.search.noResultsDescription':
    'Not one email or file of that person matches. Try widening the period or removing the keyword.',
  'comp.search.resultCount': {
    one: '{count} item from {subject}.',
    other: '{count} items from {subject}.',
  },
  'comp.search.downloadFull': 'Download the full content:',
  'comp.period.range': 'from {from} to {to}',
  'comp.period.from': 'from {from} onwards',
  'comp.period.to': 'up to {to}',
  'comp.period.all': 'across the whole history',
  'comp.export.title': 'Download the content',
  'comp.export.intro':
    'This is not a listing: it is the full content, readable outside Sytue. Review what you are about to download.',
  'comp.export.what': 'What is downloaded',
  'comp.export.whatMail': 'All their email of the period, with attachments, in an .mbox file',
  'comp.export.whatDrive': 'All their files of the period, in a .zip',
  'comp.export.whose': 'Whose',
  'comp.export.period': 'Period',
  'comp.export.textFilter': 'Text filter',
  'comp.export.noFilter': 'No filter (all the content)',
  'comp.export.mailBtn': 'Email (.mbox)',
  'comp.export.driveBtn': 'Files (.zip)',
  'comp.export.downloadMbox': 'Download the .mbox',
  'comp.export.downloadZip': 'Download the .zip',
  'comp.export.auditedTitle': 'It is recorded and it leaves Sytue',
  'comp.export.auditedBody':
    'The download is recorded in the audit trail with your name, the date and whose content it is. From then on the file lives on your machine: keep it wherever your procedure requires and delete it when the case closes.',
  // --- Mapa de la suite: quién sirve cada producto (AppMap) ---------------
  'appmap.column.apps': 'Connected applications',
  'appmap.column.products': 'Suite products',
  'appmap.dragHint': 'Drag onto a product to have this application serve it',
  'appmap.menuAria': 'Which app serves {product}',
  'appmap.nativeHint': 'The suite’s own product',
  'appmap.handoff.full': 'Opens the application and each document too',
  'appmap.handoff.appOnly': 'Opens the application; documents stay in Sytue',
  'appmap.footer':
    'Choose which app serves each product. The change applies to the whole organisation: the launcher and the app switcher will open the chosen application. Data already in Sytue stays where it is.',
  'appmap.offline.title': 'The map could not be read',
  'appmap.offline.description':
    'The integrations gateway is not responding right now. Try again in a few minutes.',
  'appmap.empty.title': 'Every product is served by Sytue',
  'appmap.empty.description':
    'When you connect an application that can replace a suite product — Google Workspace, Microsoft 365, Slack… — it will appear here for you to choose.',
  'appmap.toast.switched': '{product} is now served by the connected application',
  'appmap.toast.reverted': '{product} goes back to Sytue',
  'appmap.toast.failed': 'Could not change it',
  'appmap.toast.failedHint': 'Try again in a few seconds.',

  // --- Asistente: traer contenido (MigrateWizard) --------------------------
  'wizard.migrate.goToApps': 'Go to Integrations',
  'wizard.migrate.empty.title': 'Connect the source first',
  'wizard.migrate.empty.description':
    'To bring content over from Microsoft 365 or Google Workspace you have to connect it in Integrations. It is done once and serves the whole organisation.',
  'wizard.migrate.bring.drive': 'Files',
  'wizard.migrate.bring.contacts': 'Contacts',
  'wizard.migrate.bring.both': 'Files and contacts',
  'wizard.migrate.gets.files':
    'The folders and the files, with their name and in the same path they had.',
  'wizard.migrate.gets.contacts':
    'Contacts with their name, their email addresses, their phone numbers and their organisation.',
  'wizard.migrate.nots.googleNative':
    'Google’s own documents (Docs, Sheets, Slides). They are not files: they have to be exported first. They are skipped one by one and we tell you which ones.',
  'wizard.migrate.nots.tooBig':
    'Files over 2 GiB. They are skipped whole; they are never uploaded half-way.',
  'wizard.migrate.nots.modifiedDate':
    'The original modification date: in Sytue everything will show today’s date. The content and the path are kept.',
  'wizard.migrate.nots.permissions':
    'The permissions and shared items of the source: each person gets their own.',
  'wizard.migrate.nots.namelessContacts':
    'Contacts without any visible name: Sytue Contacts requires a name.',
  'wizard.migrate.nots.mailCalendar':
    'Email and calendar. They are not migrated yet; they will come later.',
  'wizard.migrate.error.forbidden':
    'Your account is not allowed to migrate content in this organisation.',
  'wizard.migrate.error.sourceGone':
    'The source connection no longer exists. Go back to Integrations and check it.',
  'wizard.migrate.error.generic': 'We could not launch the migration. Try again in a few seconds.',
  'wizard.migrate.step.origin': 'Source',
  'wizard.migrate.step.what': 'What to bring',
  'wizard.migrate.step.who': 'For whom',
  'wizard.migrate.step.limits': 'What comes over',
  'wizard.migrate.step.review': 'Summary',
  'wizard.migrate.origin.heading': 'Where do we bring the content from?',
  'wizard.migrate.origin.description':
    'We use the connection you already configured in Integrations. There is nothing to install.',
  'wizard.migrate.origin.noneTitle': 'There is no source connection yet',
  'wizard.migrate.origin.noneBody':
    'To bring your content over, Microsoft 365 or Google Workspace has to be connected in Integrations first. It is a five-step wizard and it is only done once.',
  'wizard.migrate.origin.connection':
    'We will read the content with the “{name}” connection, the one your organisation already uses.',
  'wizard.migrate.what.heading': 'What do you want to bring over?',
  'wizard.migrate.what.description': 'You can launch the rest later: each thing goes on its own.',
  'wizard.migrate.what.bothHint': 'The usual move: each person’s personal folder and address book.',
  'wizard.migrate.what.driveHint':
    'The folders and files of the personal drive, with the same structure.',
  'wizard.migrate.what.contactsHint':
    'The personal address book: names, email addresses and phone numbers.',
  'wizard.migrate.what.mostCommon': 'Most common',
  'wizard.migrate.who.heading': 'Whose content do we bring over?',
  'wizard.migrate.who.description':
    'Each person gets their own IN THEIR OWN Sytue account. You can choose one or several.',
  'wizard.migrate.who.searchPlaceholder': 'Search by name or email',
  'wizard.migrate.who.searchAria': 'Search people',
  'wizard.migrate.who.loadedHint': {
    one: 'We search among the {count} person already loaded from your organisation. If the person you are after does not appear, load more below.',
    other:
      'We search among the {count} people already loaded from your organisation. If the person you are after does not appear, load more below.',
  },
  'wizard.migrate.who.loading': 'Loading the directory…',
  'wizard.migrate.who.noMatch': 'Nobody matches that search.',
  'wizard.migrate.who.suspended': 'Suspended',
  'wizard.migrate.who.loadMore': 'Load more people',
  'wizard.migrate.who.pickHelp':
    'Tick whose content you want to migrate. If there are few of you, start with one person and check the result before launching the rest.',
  'wizard.migrate.who.sourceTitle': {
    one: 'Which account it is copied from ({count} person)',
    other: 'Which account it is copied from ({count} people)',
  },
  'wizard.migrate.who.sourceHint':
    'We put their Sytue email by default. Change it if they had a different address in {product}.',
  'wizard.migrate.who.sourceAria': 'Source account of {name}',
  'wizard.migrate.who.missingSource': {
    one: 'The source account of {count} person is missing.',
    other: 'The source account of {count} people is missing.',
  },
  'wizard.migrate.limits.heading': 'This is what arrives and this is what does not',
  'wizard.migrate.limits.description':
    'We would rather tell you now than leave you a surprise two hours in.',
  'wizard.migrate.limits.yes': 'Comes over',
  'wizard.migrate.limits.no': 'Does not come over',
  'wizard.migrate.limits.safeTitle': 'Nothing is lost at the source',
  'wizard.migrate.limits.safeBody':
    'We only read. Your Microsoft 365 or your Google Workspace stay exactly as they are, and you can launch the migration again: what is already copied is not duplicated.',
  'wizard.migrate.summary.source': '{product} · {name}',
  'wizard.migrate.summary.what': 'What is brought',
  'wizard.migrate.summary.who': 'For whom',
  'wizard.migrate.summary.whoOne': '{name} ({email})',
  'wizard.migrate.summary.whoMany': {
    one: '{count} person, in their own account',
    other: '{count} people, each in their own account',
  },
  'wizard.migrate.summary.jobs': 'Jobs that are launched',
  'wizard.migrate.summary.jobsValue': {
    one: '{count} (one per person and content type)',
    other: '{count} (one per person and content type)',
  },
  'wizard.migrate.summary.skipped': 'Will be skipped',
  'wizard.migrate.summary.skippedValue': {
    one: '{count} thing, with its reason in the report',
    other: '{count} things, with their reason in the report',
  },
  'wizard.migrate.summary.skippedNone': 'Nothing',
  'wizard.migrate.review.heading': 'Review and launch',
  'wizard.migrate.review.description':
    'Once launched, the copy runs in the background. You can close this page.',
  'wizard.migrate.review.launch': {
    one: 'Migrate the content',
    other: 'Migrate for {count} people',
  },
  'wizard.migrate.review.failedTitle': 'Could not be launched',
  'wizard.migrate.review.ownAccountTitle': 'Each person gets theirs in THEIR account',
  'wizard.migrate.review.ownAccountBody':
    'The content never passes through your account. Sytue asks for permission to write on behalf of each person, that permission lasts minutes and is recorded in the audit trail.',

  // --- Asistente: etiqueta de confidencialidad (SensitivityLabelWizard) -----
  'wizard.label.color.none': 'No colour',
  'wizard.label.color.grey': 'Grey',
  'wizard.label.color.blue': 'Blue',
  'wizard.label.color.green': 'Green',
  'wizard.label.color.amber': 'Amber',
  'wizard.label.color.red': 'Red',
  'wizard.label.toast.saveFailed': 'Could not save',
  'wizard.label.toast.nameTaken': 'You already have a label with that name. Choose another one.',
  'wizard.label.toast.retry': 'Try again in a few seconds.',
  'wizard.label.step.what': 'What it is',
  'wizard.label.step.level': 'Level',
  'wizard.label.step.effect': 'What it prevents',
  'wizard.label.step.review': 'Summary',
  'wizard.label.step.done': 'Done',
  'wizard.label.what.heading': 'What is it called and what is it for?',
  'wizard.label.what.description':
    'Everybody will see the name and the explanation when labelling a file. Write them as you would tell someone who starts today.',
  'wizard.label.what.nameLabel': 'Name of the label',
  'wizard.label.what.namePlaceholder': 'Confidential',
  'wizard.label.what.nameHint':
    'Short and recognisable. The most common ones: Public, Internal, Confidential, Restricted.',
  'wizard.label.what.purposeLabel': 'What it is for',
  'wizard.label.what.purposePlaceholder':
    'Business information that must not leave the company: contracts, payroll, customer data.',
  'wizard.label.what.purposeHint':
    'Optional, but strongly recommended: it is the clue that decides whether somebody uses it well.',
  'wizard.label.what.colourLegend': 'Colour (optional)',
  'wizard.label.what.colourHint': 'It only helps to recognise it at a glance in the lists.',
  'wizard.label.level.heading': 'How sensitive is it, compared with the others?',
  'wizard.label.level.description':
    'Place it on your scale. It only serves to order them from least to most delicate; on its own it does not change what can be done with the files.',
  'wizard.label.level.firstTitle': 'This is your first label',
  'wizard.label.level.firstBody':
    'When you create more, you will be able to place them above or below this one.',
  'wizard.label.level.first': 'It is the first label of your organisation',
  'wizard.label.level.lowest': 'The least sensitive of all (below “{name}”)',
  'wizard.label.level.highest': 'The most sensitive of all (above “{name}”)',
  'wizard.label.level.between': 'Between “{lower}” and “{upper}”',
  'wizard.label.level.hintLowest': 'They will use it for the most open things.',
  'wizard.label.level.hintHighest': 'They will use it for the most delicate things.',
  'wizard.label.level.hintMiddle': 'A middle rung.',
  'wizard.label.level.scale': 'Your current scale, from least to most sensitive: {scale}.',
  'wizard.label.level.scaleEmpty': 'still empty',
  'wizard.label.effect.heading': 'What should this label prevent?',
  'wizard.label.effect.description':
    'Choose by what really happens when somebody tries to share the file.',
  'wizard.label.effect.informTitle': 'It only informs',
  'wizard.label.effect.informBody':
    'The label is shown next to the file so people know what they have in their hands, but it prevents nothing. It is the option for “Public” or “Internal”.',
  'wizard.label.effect.blockTitle': 'Prevents sharing outside the organisation',
  'wizard.label.effect.blockBody':
    'Nobody will be able to create a public link for a file with this label, and whoever tries will see the reason. Links that already existed will stop opening those files. Sharing with people or groups of your organisation keeps working the same.',
  'wizard.label.effect.warnTitle': 'Before switching it on',
  'wizard.label.effect.warnBody':
    'If your team already shares these files with customers or suppliers by link, warn them: as soon as you label a file, its link stops working.',
  'wizard.label.effect.summaryInform': 'Only informs: it prevents nothing',
  'wizard.label.effect.summaryBlock': 'Prevents sharing the file outside the organisation',
  'wizard.label.summary.name': 'Name',
  'wizard.label.summary.purpose': 'What it is for',
  'wizard.label.summary.noPurpose': 'No explanation',
  'wizard.label.summary.level': 'Level',
  'wizard.label.summary.effect': 'What it prevents',
  'wizard.label.review.headingNew': 'Review and create the label',
  'wizard.label.review.headingEdit': 'Review the changes',
  'wizard.label.review.description':
    'This is what we are going to save. You can change anything beforehand.',
  'wizard.label.review.saveNew': 'Create the label',
  'wizard.label.review.saveEdit': 'Save changes',
  'wizard.label.review.editNoteTitle': 'Files already labelled do not change',
  'wizard.label.review.editNoteBody':
    'A file keeps the label exactly as it was when it was applied. If you change the name or what it prevents, what is already labelled stays as it was: that way a file classified a year ago still reads the same. To update it, label it again.',
  'wizard.label.review.newNoteTitle': 'What happens next',
  'wizard.label.review.newNoteBody':
    'The label will appear to choose from when classifying files in Sytue Drive. Labelling is manual: nothing is classified on its own.',
  'wizard.label.done.headingNew': 'Label created',
  'wizard.label.done.headingEdit': 'Changes saved',
  'wizard.label.done.bodyBlock':
    'It can already be chosen when classifying files. Those that carry it will not be shareable by public link.',
  'wizard.label.done.bodyInform':
    'It can already be chosen when classifying files. It only informs: it does not prevent sharing.',
  'wizard.label.done.back': 'Back to the labels',

  // --- Asistente: conectar proveedor de identidad (ConnectIdpWizard) --------
  'wizard.idp.kind.oidcBlurb':
    'What almost every provider uses today. They give you an address and two keys, and that is it.',
  'wizard.idp.kind.oidcExamples': 'Microsoft Entra ID · Google Workspace · Okta · Auth0 · Keycloak',
  'wizard.idp.kind.samlBlurb':
    'The classic enterprise standard. Your provider gives you a metadata XML (or a certificate and two addresses).',
  'wizard.idp.kind.samlExamples': 'AD FS · Okta or OneLogin in SAML mode · legacy providers',
  'wizard.idp.kind.mostCommon': 'Most common',
  'wizard.idp.kind.bothTitle': 'Does your provider offer both?',
  'wizard.idp.kind.bothBody':
    'Choose OpenID Connect: it takes fewer values to set up and there are no certificates to renew every few years.',
  'wizard.idp.effect.signInTitle': 'They sign in with the company account',
  'wizard.idp.effect.signInBody':
    'Your team will use the “Sign in with your organisation” button and their usual company password. One less to remember.',
  'wizard.idp.effect.noLossTitle': 'Nobody loses access',
  'wizard.idp.effect.noLossBody':
    'Anyone who already has a Sytue password can carry on using it. This adds a way in, it does not replace one.',
  'wizard.idp.effect.whoTitle': 'You decide who gets through',
  'wizard.idp.effect.whoBody':
    'You can restrict access to your organisation’s email domains, even if your provider holds more people.',
  'wizard.idp.effect.offTitle': 'Nothing is switched on until you say so',
  'wizard.idp.effect.offBody':
    'You configure it, you test it and you enable it at the end. Until then, nobody sees it on the sign-in screen.',
  'wizard.idp.need.samlXml': 'The provider metadata (an XML file)',
  'wizard.idp.need.samlXmlWhere':
    'In your provider’s SAML application: “Download federation metadata XML” in Microsoft Entra ID, “Identity Provider metadata” in Okta. It is the quick way: you paste it and we fill in the rest for you.',
  'wizard.idp.need.samlManual': 'Or else: Entity ID, sign-on URL and certificate',
  'wizard.idp.need.samlManualWhere':
    'They are on that same screen, one by one, in case your provider does not let you download the XML. The certificate is the provider’s signing certificate (it starts with “-----BEGIN CERTIFICATE-----”).',
  'wizard.idp.need.domains': 'Your organisation’s email domains',
  'wizard.idp.need.domainsWhere':
    'The ones your team’s addresses use, for example acme.com. You can give several.',
  'wizard.idp.need.issuer': 'Your provider’s address (issuer)',
  'wizard.idp.need.issuerWhere':
    'Microsoft Entra ID: your app registration → “Endpoints” → copy the OpenID Connect metadata document and remove the trailing “/.well-known/openid-configuration”. Google Workspace: it is https://accounts.google.com. Okta: https://your-company.okta.com.',
  'wizard.idp.need.client': 'The client ID and its secret',
  'wizard.idp.need.clientWhere':
    'They are created when you register an application in your provider (in Entra ID, “App registrations” → New registration → “Certificates & secrets”). Copy the secret as soon as you create it: almost every provider shows it only once.',
  'wizard.idp.err.issuerRequired': 'We need your provider’s address.',
  'wizard.idp.err.hasSpace': 'A space slipped in. Paste the address only.',
  'wizard.idp.err.wellKnown':
    'Remove the trailing “/.well-known/openid-configuration”: only the part before it goes here.',
  'wizard.idp.err.insecure': 'It has to start with https:// (that http:// is not secure).',
  'wizard.idp.err.needHttps': 'Write it in full, starting with https://',
  'wizard.idp.err.noDomain': 'The domain is missing: check the address.',
  'wizard.idp.err.badIssuerUrl': 'That address is not valid. Check it: it is usually a typo.',
  'wizard.idp.err.urlRequired': 'We need this address.',
  'wizard.idp.err.mustBeHttps': 'It has to be an https:// address.',
  'wizard.idp.err.badUrl': 'That address is not valid. Check it.',
  'wizard.idp.err.badDomain':
    'This does not look like a domain: “{domain}”. Write it without “@” or “https://”, just acme.com.',
  'wizard.idp.err.certRequired': 'We need the provider’s signing certificate.',
  'wizard.idp.err.notCert':
    'This does not look like a certificate. Paste it exactly as your provider gives it to you, including the “-----BEGIN CERTIFICATE-----” lines.',
  'wizard.idp.err.nameRequired': 'Give it a name so you can recognise it.',
  'wizard.idp.err.slugRequired': 'We need an identifier.',
  'wizard.idp.err.slugFormat': 'Lowercase letters, digits and hyphens only (no spaces or accents).',
  'wizard.idp.err.slugTaken': 'Another provider already uses this identifier.',
  'wizard.idp.err.clientIdRequired': 'We need the client ID.',
  'wizard.idp.err.xmlRequired': 'Paste here the metadata you download from your provider.',
  'wizard.idp.err.xmlFormat':
    'This does not look like the metadata XML: it must contain “EntityDescriptor”. Paste it whole, exactly as your provider gives it to you.',
  'wizard.idp.err.entityRequired': 'We need the Entity ID.',
  'wizard.idp.fix.samlData':
    'The provider values do not add up. Check the Entity ID, that the sign-on URL starts with https:// and that the certificate is complete (with its BEGIN and END lines).',
  'wizard.idp.fix.oidcDiscovery':
    'We could not read the configuration at that address. It is almost always the provider address: copy it again from your provider and remove the trailing “/.well-known/openid-configuration”.',
  'wizard.idp.fix.slugTaken':
    'Another provider in your organisation already uses that identifier. Change the identifier in the previous step.',
  'wizard.idp.fix.forbidden':
    'Your account does not have permission to configure sign-in in this organisation.',
  'wizard.idp.fix.idpDown':
    'Sytue ID is not responding right now. It is not your values: try again in a minute.',
  'wizard.idp.fix.generic':
    'We could not check it. Review the values in the previous step and try again.',
  'wizard.idp.step.type': 'Type',
  'wizard.idp.step.explain': 'What happens',
  'wizard.idp.step.configure': 'Configure',
  'wizard.idp.step.test': 'Test',
  'wizard.idp.step.review': 'Summary',
  'wizard.idp.step.done': 'Done',
  'wizard.idp.type.heading': 'What does your identity provider connect with?',
  'wizard.idp.type.description':
    'This is the only technical thing we will ask you. If you do not know, look for your provider’s name in the examples.',
  'wizard.idp.explain.heading': 'This is what will change for your team',
  'wizard.idp.explain.description': 'None of this applies yet: first you configure it and test it.',
  'wizard.idp.explain.needTitle': 'Have this to hand before you carry on',
  'wizard.idp.configure.headingOidc': 'Your OIDC provider’s values',
  'wizard.idp.configure.headingSaml': 'Your SAML provider’s values',
  'wizard.idp.configure.description':
    'Under every field we tell you where to find it, and we check what you type as you go.',
  'wizard.idp.field.name': 'Name',
  'wizard.idp.field.nameHint':
    'This is what your team sees on the sign-in button. Your provider’s name will do.',
  'wizard.idp.field.slug': 'Identifier',
  'wizard.idp.field.slugHint':
    'Short internal name, used in the sign-in addresses. We suggest one from the name.',
  'wizard.idp.field.issuer': 'Provider address (issuer)',
  'wizard.idp.field.issuerHint':
    'The base address, without “/.well-known/openid-configuration” at the end. Google Workspace: https://accounts.google.com',
  'wizard.idp.field.issuerOk': 'Looks right. We will check it in the next step.',
  'wizard.idp.field.clientId': 'Client ID',
  'wizard.idp.field.clientIdHint':
    'Your provider gives it to you when you register the application (“Application (client) ID” in Entra ID).',
  'wizard.idp.field.secret': 'Client secret',
  'wizard.idp.field.secretPlaceholder': '•••••••• (saved, unchanged)',
  'wizard.idp.field.secretKeepHint': 'Leave it empty to keep the one you already have saved.',
  'wizard.idp.field.secretHint':
    'Leave it empty only if your provider does not give you one (public application with PKCE).',
  'wizard.idp.field.secretStored':
    'It is stored encrypted and never shown again, neither to you nor to us. If it ever expires, you type a new one here and that is that.',
  'wizard.idp.field.xml': 'Provider metadata (XML)',
  'wizard.idp.field.xmlHint':
    'Download it from your provider (“federation metadata XML”) and paste it whole. We extract the rest.',
  'wizard.idp.field.xmlOk': 'Looks right. We will take the address and the certificate from here.',
  'wizard.idp.field.noXmlLink': 'My provider does not give me the XML',
  'wizard.idp.field.entity': 'Provider Entity ID',
  'wizard.idp.field.entityHint':
    'Also called “Azure AD Identifier” or “Issuer”. It usually looks like a web address.',
  'wizard.idp.field.ssoUrl': 'Provider sign-on URL',
  'wizard.idp.field.ssoUrlHint':
    'The address we send your team to so that they can identify themselves (“SAML SSO endpoint”).',
  'wizard.idp.field.cert': 'Signing certificate',
  'wizard.idp.field.certHint':
    'It is public: it is what proves the response really comes from your provider. Download it from the same screen.',
  'wizard.idp.field.useXmlLink': 'I would rather paste the metadata XML',
  'wizard.idp.field.domains': 'Allowed email domains',
  'wizard.idp.field.domainsHint':
    'Only people whose email is in these domains will get in. Separate them with commas. Empty = anyone your provider lets through.',
  'wizard.idp.field.advanced': 'Advanced options',
  'wizard.idp.field.advancedHint':
    'You do not need to touch any of this unless your provider uses names other than the standard ones.',
  'wizard.idp.field.scopes': 'Requested permissions (scopes)',
  'wizard.idp.field.scopesHint': 'Separated by spaces. “openid” is mandatory.',
  'wizard.idp.mapping.legend': 'Where we take each person’s details from',
  'wizard.idp.mapping.hint':
    'Leave the fields empty and we will use the standard names (email, name, groups).',
  'wizard.idp.mapping.emailAria': 'Name of the email claim',
  'wizard.idp.mapping.nameAria': 'Name of the full-name claim',
  'wizard.idp.mapping.groupsAria': 'Name of the groups claim',
  'wizard.idp.test.headingOidc': 'Let us check that Sytue reaches your provider',
  'wizard.idp.test.headingSaml': 'Let us check your provider’s values',
  'wizard.idp.test.descriptionOidc':
    'We ask your provider for its configuration. Nobody signs in yet.',
  'wizard.idp.test.descriptionSaml':
    'We check the Entity ID, the sign-on address and the certificate. Nobody signs in yet.',
  'wizard.idp.test.okTitle': 'All correct',
  'wizard.idp.test.okOidc': 'Your provider answered us and we read its configuration.',
  'wizard.idp.test.okOidcPkce':
    'Your provider answered us and we read its configuration (with PKCE, the safest mode).',
  'wizard.idp.test.okSaml':
    'The provider values are valid: the certificate reads correctly and the sign-on address is right.',
  'wizard.idp.test.failTitle': 'We could not check it',
  'wizard.idp.test.reviewData': 'Review the values',
  'wizard.idp.test.idleHelp':
    'Press “Check” and we verify the values you entered. If something does not add up, we tell you exactly what to change.',
  'wizard.idp.test.run': 'Check',
  'wizard.idp.test.rerun': 'Check again',
  'wizard.idp.test.skipHint':
    'You can carry on without checking, but it is better to catch a typo now.',
  'wizard.idp.test.draftNote':
    'To be able to check it we saved it disabled. Nobody can sign in through it until you enable it in the next step.',
  'wizard.idp.summary.type': 'Connection type',
  'wizard.idp.summary.name': 'Name',
  'wizard.idp.summary.slug': 'Identifier',
  'wizard.idp.summary.issuer': 'Provider address',
  'wizard.idp.summary.clientId': 'Client ID',
  'wizard.idp.summary.secret': 'Client secret',
  'wizard.idp.summary.secretSaved': 'Saved (not shown)',
  'wizard.idp.summary.secretKept': 'The one you already had is kept',
  'wizard.idp.summary.secretNone': 'No secret (PKCE)',
  'wizard.idp.summary.samlData': 'Provider values',
  'wizard.idp.summary.fromXml': 'We read them from the metadata XML you pasted',
  'wizard.idp.summary.entityValue': 'Entity ID {id}',
  'wizard.idp.summary.ssoUrl': 'Sign-on URL',
  'wizard.idp.summary.cert': 'Signing certificate',
  'wizard.idp.summary.certPasted': 'Pasted',
  'wizard.idp.summary.who': 'Who can sign in',
  'wizard.idp.summary.whoDomains': 'Only email addresses from {domains}',
  'wizard.idp.summary.whoAny': 'Anyone your provider lets through',
  'wizard.idp.summary.test': 'Check',
  'wizard.idp.summary.testOk': 'Passed',
  'wizard.idp.summary.testFail': 'Failed (you can save anyway and fix it later)',
  'wizard.idp.summary.testNone': 'Not checked',
  'wizard.idp.review.headingNew': 'Review and save',
  'wizard.idp.review.headingEdit': 'Review the changes',
  'wizard.idp.review.description':
    'This is what we are going to apply. You can change anything before you carry on.',
  'wizard.idp.review.saveEdit': 'Save changes',
  'wizard.idp.review.saveEnable': 'Save and enable',
  'wizard.idp.review.save': 'Save',
  'wizard.idp.review.enableLabel': 'Allow access with this provider straight away',
  'wizard.idp.review.enableOn':
    'The “Sign in with your organisation” button will appear on your team’s sign-in screen.',
  'wizard.idp.review.enableOff':
    'It will be saved disabled: nobody sees it until you enable it from the list.',
  'wizard.idp.review.pendingTitle': 'One step is left in YOUR provider',
  'wizard.idp.review.pendingOidc':
    'When you save we give you the return address you have to register in your provider. Without it, signing in will not work.',
  'wizard.idp.review.pendingSaml':
    'When you save we give you the reply URL (ACS) and the Sytue identifier, which you have to register in your SAML application. Without them, signing in will not work.',
  'wizard.idp.done.headingEdit': 'Changes saved',
  'wizard.idp.done.headingNew': '{name} is ready',
  'wizard.idp.done.fallbackName': 'Your provider',
  'wizard.idp.done.savedTitle': 'Saved',
  'wizard.idp.done.savedOn':
    'The provider is enabled. As soon as you complete the step below, your team will be able to sign in with the company account.',
  'wizard.idp.done.savedOff':
    'The provider is saved but disabled. Enable it from the list once you have finished the step below.',
  'wizard.idp.done.pasteTitle': 'Paste this into your provider’s settings',
  'wizard.idp.done.pasteOidc':
    'Your provider will only return your team to Sytue if this address is registered in the application you created.',
  'wizard.idp.done.pasteSaml':
    'Register it in your SAML application: without this, your provider does not know where to reply.',
  'wizard.idp.done.redirectUri': 'Redirect URL',
  'wizard.idp.done.acsUrl': 'Reply URL (ACS)',
  'wizard.idp.done.spEntityId': 'Sytue identifier (Entity ID)',
  'wizard.idp.done.metadataUrl': 'Sytue metadata',
  'wizard.idp.done.nextTitle': 'What to do now',
  'wizard.idp.done.step1Title': 'Register the values above in your provider',
  'wizard.idp.done.step1Oidc':
    'In your app registration, under redirect URI (web type). Paste it exactly, with no extra trailing slash.',
  'wizard.idp.done.step1Saml':
    'In your SAML application, under “Reply URL” and “Identifier”. Paste them exactly.',
  'wizard.idp.done.step2Title': 'Try it with your own account',
  'wizard.idp.done.step2Body':
    'Open a private browser window, go to the Sytue sign-in screen and use “Sign in with your organisation”. If you get in without us asking for a Sytue password, it is set up correctly.',
  'wizard.idp.done.step3Title': 'Tell your team',
  'wizard.idp.done.step3Body':
    'From now on they sign in with the company account. Anyone who already had a Sytue password can carry on using it.',
  'wizard.idp.done.back': 'Back to the providers',

  // --- Asistente: registrar aplicación que entra con Sytue (RegisterOidcAppWizard) ---
  'wizard.oidc.kind.serverTitle': 'An application with its own server',
  'wizard.oidc.kind.serverBlurb':
    'The application runs on a server and can keep a password secret. We will give it one.',
  'wizard.oidc.kind.serverExamples':
    'Grafana · GitLab · Jenkins · Nextcloud · an in-house tool with a backend',
  'wizard.oidc.kind.browserTitle': 'An application that runs in the browser or on a phone',
  'wizard.oidc.kind.browserBlurb':
    'All its code sits on the user’s device, so it cannot keep secrets. It will use PKCE, which is just as safe for this case.',
  'wizard.oidc.kind.browserExamples':
    'A website built with React or Vue · an iOS or Android app · a desktop app',
  'wizard.oidc.kind.mostCommon': 'Most common',
  'wizard.oidc.effect.signInTitle': 'Your team signs in with their Sytue account',
  'wizard.oidc.effect.signInBody':
    'A sign-in button that leads to Sytue will appear in the application. Nobody needs a separate password.',
  'wizard.oidc.effect.controlTitle': 'You control access from here',
  'wizard.oidc.effect.controlBody':
    'If somebody leaves the organisation, they stop getting into the application. And you can withdraw access to the whole application whenever you want.',
  'wizard.oidc.effect.minimalTitle': 'The application only sees the basics',
  'wizard.oidc.effect.minimalBody':
    'Name, email and identifier. Nothing else: not your documents, not your email, not your files.',
  'wizard.oidc.err.hasSpace': '“{url}” has a space in it. Paste the address only.',
  'wizard.oidc.err.noWildcards':
    'Wildcards (*) are not allowed. Write the exact address: that is what stops a third party from slipping in.',
  'wizard.oidc.err.noFragment':
    'Remove everything after “#”: that part never reaches the server and is not valid here.',
  'wizard.oidc.err.notUrl':
    '“{url}” is not a valid address. It has to start with https:// and include the domain.',
  'wizard.oidc.err.insecure':
    'It has to start with https:// (http:// is only accepted on localhost, for testing).',
  'wizard.oidc.err.notWeb': '“{url}” is not a web address. It must start with https://',
  'wizard.oidc.err.noDomain': '“{url}” is missing the domain. Check it.',
  'wizard.oidc.err.redirectRequired': 'We need at least one return address.',
  'wizard.oidc.err.nameRequired': 'Give it a name so you can recognise it in the list.',
  'wizard.oidc.fix.badUrls':
    'Sytue ID rejected one of the addresses. Go over them: they have to start with https://, with no “#” and no wildcards.',
  'wizard.oidc.fix.forbidden':
    'Your account does not have permission to register applications in this organisation.',
  'wizard.oidc.fix.idpDown':
    'Sytue ID is not responding right now. It is not your values: try again in a minute.',
  'wizard.oidc.fix.generic': 'We could not register it. Review the values and try again.',
  'wizard.oidc.toast.saveFailed': 'Could not register it',
  'wizard.oidc.step.app': 'The application',
  'wizard.oidc.step.urls': 'Addresses',
  'wizard.oidc.step.review': 'Summary',
  'wizard.oidc.step.done': 'Done',
  'wizard.oidc.app.heading': 'Which application do you want to connect?',
  'wizard.oidc.app.description':
    'With this, your team will be able to sign in to it with their Sytue account, with no extra password.',
  'wizard.oidc.app.nameLabel': 'Name of the application',
  'wizard.oidc.app.namePlaceholder': 'Grafana for the data team',
  'wizard.oidc.app.nameHint':
    'Only so that you recognise it here. You can use the name your team uses.',
  'wizard.oidc.app.whereLegend': 'Where does it run?',
  'wizard.oidc.app.whereHint':
    'If in doubt, choose the first one: that is the normal case for tools installed on a server.',
  'wizard.oidc.urls.heading': 'Where does your team come back to after identifying themselves?',
  'wizard.oidc.urls.description':
    'The application tells you this address: look for “redirect URL”, “Redirect URI” or “Callback URL” on its sign-in settings screen.',
  'wizard.oidc.urls.redirectLabel': 'Redirect URL',
  'wizard.oidc.urls.redirectOk': {
    one: 'The address looks right.',
    other: 'The addresses look right.',
  },
  'wizard.oidc.urls.redirectHint':
    'One per line, if the application has several (a test one, for example).',
  'wizard.oidc.urls.slashTitle': 'The most common mistake: a slash too many or too few',
  'wizard.oidc.urls.slashBody':
    'We compare the address character by character. If the application sends “…/callback” and you write “…/callback/” here, signing in will fail with “redirect_uri not registered”. Copy it exactly from the application, adding and removing nothing. It is deliberate: that exact comparison is what stops somebody diverting the sign-in somewhere else.',
  'wizard.oidc.urls.advanced': 'Advanced options',
  'wizard.oidc.google.title': 'Data access as a Google account',
  'wizard.oidc.google.body':
    'For apps written for Google Calendar, Drive, Contacts or the Workspace directory: they ask for Google permissions and get Sytue data. Google is not involved and sees nothing. Everyone who signs in to the app gives it this access to their data.',
  'wizard.oidc.google.calendar': 'Calendar',
  'wizard.oidc.google.drive': 'Drive',
  'wizard.oidc.google.contacts': 'Contacts',
  'wizard.oidc.google.none': 'No access',
  'wizard.oidc.google.read': 'Read only',
  'wizard.oidc.google.write': 'Read and write',
  'wizard.oidc.google.directory': 'Organisation directory',
  'wizard.oidc.google.directoryUsers': 'Users',
  'wizard.oidc.google.directoryGroups': 'Groups and their members',
  'wizard.oidc.google.directoryHint':
    'Read only, and only when the person signing in to the app administers the organisation.',
  'wizard.oidc.summary.google': 'Access as a Google account',
  'wizard.oidc.done.googleDiscovery': 'Google-style discovery',
  'wizard.oidc.urls.logoutLabel': 'URL after signing out',
  'wizard.oidc.urls.logoutHint':
    'Optional. Where we take your team if they sign out from the application. If you leave it empty, they stay in Sytue.',
  'wizard.oidc.summary.app': 'Application',
  'wizard.oidc.summary.runsOn': 'Where it runs',
  'wizard.oidc.summary.runsServer': 'On its own server (with a secret)',
  'wizard.oidc.summary.runsBrowser': 'In the browser or on a phone (no secret, PKCE)',
  'wizard.oidc.summary.redirect': 'Redirect URL',
  'wizard.oidc.summary.logout': 'URL after signing out',
  'wizard.oidc.summary.sees': 'What it will see',
  'wizard.oidc.summary.seesValue': 'Name, email and identifier',
  'wizard.oidc.review.heading': 'Review and register',
  'wizard.oidc.review.description':
    'Nothing has been created yet. You can change anything before you carry on.',
  'wizard.oidc.review.submit': 'Register the application',
  'wizard.oidc.review.secretTitle': 'The secret is shown ONCE only',
  'wizard.oidc.review.secretBody':
    'When you register it we give you a password for the application. We do not store it in the clear and we will not be able to show it to you again: copy it there and then. If it gets lost, you generate another one from the list and that is that.',
  'wizard.oidc.review.pkceTitle': 'No secret, and that is fine',
  'wizard.oidc.review.pkceBody':
    'An application that runs on the user’s device cannot keep secrets. It will use PKCE, which protects the exchange without needing one.',
  'wizard.oidc.done.heading': '{name} can now use Sytue',
  'wizard.oidc.done.fallbackName': 'The application',
  'wizard.oidc.done.secretTitle': 'Copy the secret now: it will not be shown again',
  'wizard.oidc.done.secretBody':
    'Paste it into the application next to the client identifier. If you lose it, generate a new one from the list (the previous one stops working).',
  'wizard.oidc.done.secretLabel': 'Client secret',
  'wizard.oidc.done.pasteTitle': 'Paste this into the application',
  'wizard.oidc.done.pasteBody':
    'Almost all of them ask only for the first two: with the Sytue address they fill in the rest themselves. If yours asks for each address separately, they are below.',
  'wizard.oidc.done.issuer': 'Sytue address (issuer)',
  'wizard.oidc.done.clientId': 'Client identifier',
  'wizard.oidc.done.discovery': 'Automatic configuration',
  'wizard.oidc.done.separateTitle':
    'Individual addresses (if your application asks for them one by one)',
  'wizard.oidc.done.authorization': 'Authorisation',
  'wizard.oidc.done.token': 'Token',
  'wizard.oidc.done.userinfo': 'User details',
  'wizard.oidc.done.jwks': 'Signing keys (JWKS)',
  'wizard.oidc.done.endSession': 'Sign-out',
  'wizard.oidc.done.scopes': 'Permissions (scopes)',
  'wizard.oidc.done.nextTitle': 'What to do now',
  'wizard.oidc.done.step1Title': 'Paste the values above into the application',
  'wizard.oidc.done.step1Body':
    'Look in it for the OpenID Connect (or OAuth) sign-in section and fill it in with the Sytue address and the client identifier.',
  'wizard.oidc.done.step1BodySecret':
    'Look in it for the OpenID Connect (or OAuth) sign-in section and fill it in with the Sytue address, the client identifier and the secret.',
  'wizard.oidc.done.step2Title': 'Try it with your own account',
  'wizard.oidc.done.step2Body':
    'Open a private browser window and go into the application with the sign-in-with-Sytue button. If it brings you back to the application already identified, it is ready.',
  'wizard.oidc.done.step3Title': 'Tell your team',
  'wizard.oidc.done.step3Body':
    'From now on they sign in with their Sytue account. If somebody leaves the organisation, they lose access to this application automatically.',
  'wizard.oidc.done.back': 'Back to the applications',

  // --- Asistente: conectar una aplicación (ConnectAppWizard) -----------------
  'wizard.app.effect.portalTitle': 'It appears in the portal',
  'wizard.app.effect.portalBody':
    'Your team will see {app} among their applications when they sign in to Sytue.',
  'wizard.app.effect.ssoTitle': 'They sign in with their Sytue account',
  'wizard.app.effect.ssoBody':
    'No separate password is needed for {app}: people sign in with their Sytue account and your access rules apply.',
  'wizard.app.effect.scimTitle': 'Automatic joiners and leavers',
  'wizard.app.effect.scimBody':
    'When somebody joins or leaves your organisation, their {app} account is created or disabled on its own.',
  'wizard.app.effect.dataTitle': 'Reading data',
  'wizard.app.effect.dataBody':
    'Sytue will be able to read data from {app} for reports. It never writes or deletes anything.',
  'wizard.app.need.fieldWhere': 'You will find it in the settings of your {app} account.',
  'wizard.app.need.secretWhere':
    'It is generated in the {app} admin panel. You need administrator permissions there.',
  'wizard.app.err.required': 'We need this value to carry on.',
  'wizard.app.err.noSpaces': 'It cannot contain spaces.',
  'wizard.app.err.noScheme': 'Write it without “https://”, just the name.',
  'wizard.app.err.noPath': 'Only the name or the domain, with no slashes and no paths.',
  'wizard.app.fix.fieldFallback': 'the connection values',
  'wizard.app.fix.badData': 'The values do not add up. Check “{field}” in the previous step.',
  'wizard.app.fix.rejectedSecret':
    '{app} rejected the “{secret}”: it may have expired or may not have enough permissions. Generate a new one and paste it again.',
  'wizard.app.fix.rejected':
    '{app} rejected the connection. Check that the application allows access from Sytue.',
  'wizard.app.fix.notFound':
    'We could not find that {app} account. Check “{field}”: it is usually a typo.',
  'wizard.app.fix.down':
    '{app} is not responding right now. It is not your values: try again in a minute.',
  'wizard.app.fix.generic':
    'We could not connect. Review the values in the previous step and try again.',
  'wizard.app.toast.activateFailed': 'Could not activate it',
  'wizard.app.fix.activateBadData':
    'Check the values in the “Configure” step: we could not register the sign-in.',
  'wizard.app.fix.activateIdpDown': 'Sytue ID is not responding right now. Try again in a minute.',
  'wizard.app.fix.activateGeneric': 'Something went wrong along the way. Try again.',
  'wizard.app.step.choose': 'Choose',
  'wizard.app.step.explain': 'What happens',
  'wizard.app.step.configure': 'Configure',
  'wizard.app.step.test': 'Test',
  'wizard.app.step.review': 'Summary',
  'wizard.app.step.done': 'Done',
  'wizard.app.choose.heading': 'Which application do you want to connect?',
  'wizard.app.choose.description':
    'Pick one of the applications your team already uses. We guide you through the rest: you do not have to know about SSO or SCIM.',
  'wizard.app.choose.searchAria': 'Search application',
  'wizard.app.choose.searchPlaceholder': 'Search by name or category…',
  'wizard.app.choose.noResults': 'No application is called that. Try another name.',
  'wizard.app.choose.alreadyConnected': 'Already connected',
  'wizard.app.explain.heading': 'This is what you will switch on in {app}',
  'wizard.app.explain.headingFallback': 'This is what will happen',
  'wizard.app.explain.description': 'None of this applies yet: first you configure it and test it.',
  'wizard.app.explain.needTitle': 'Have this to hand before you carry on',
  'wizard.app.configure.heading': 'Your {app} values',
  'wizard.app.configure.headingFallback': 'Connection values',
  'wizard.app.configure.description':
    'Only the essentials. Under every field we tell you where to find it, and we check what you type as you go.',
  'wizard.app.configure.nameLabel': 'Name of the connection',
  'wizard.app.configure.nameError': 'Give it a name so you can recognise it.',
  'wizard.app.configure.nameHint':
    'It is only shown here, in the console. You can keep the one we suggest.',
  'wizard.app.configure.formatOk': 'Correct format.',
  'wizard.app.configure.secretStored':
    'It is stored encrypted and never shown again: not even we can read it. If it ever changes, you replace it with a new one.',
  'wizard.app.oauth.title': 'Connect with your {app} account',
  'wizard.app.oauth.body':
    'We will take you to {app} so that you can approve the access. You do not have to create or paste any credential.',
  'wizard.app.oauth.willAuthorize': 'You are about to authorise:',
  'wizard.app.oauth.revocable':
    'The permission expires on its own and you can withdraw it whenever you want, from here or from {app}.',
  'wizard.app.test.heading': 'Let us check that Sytue reaches {app}',
  'wizard.app.test.headingFallback': 'Let us check the connection',
  'wizard.app.test.description':
    'It is a test call with the values you entered. Nothing is switched on for your team yet.',
  'wizard.app.test.okTitle': 'Connection works',
  'wizard.app.test.okBody': 'We talked to {app} and it answered. You can activate it now.',
  'wizard.app.test.failTitle': 'We could not connect',
  'wizard.app.test.reviewData': 'Review the values',
  'wizard.app.test.idleHelp':
    'Press “Test connection” and we verify the values against {app}. If something does not add up, we tell you exactly what to change.',
  'wizard.app.test.run': 'Test connection',
  'wizard.app.test.rerun': 'Test again',
  'wizard.app.test.skipHint': 'You can carry on without testing, but it is better to check now.',
  'wizard.app.summary.app': 'Application',
  'wizard.app.summary.name': 'Name of the connection',
  'wizard.app.summary.secretSaved': 'Saved (not shown)',
  'wizard.app.summary.secretNone': 'No token',
  'wizard.app.summary.willEnable': 'Will be switched on',
  'wizard.app.summary.test': 'Check',
  'wizard.app.summary.testOk': 'Connection tested successfully',
  'wizard.app.summary.testFail': 'The test failed',
  'wizard.app.summary.testNone': 'Not tested',
  'wizard.app.review.heading': 'Review and activate',
  'wizard.app.review.description':
    'This is what we are going to apply. You can change anything before you carry on.',
  'wizard.app.review.activate': 'Activate {app}',
  'wizard.app.review.activateFallback': 'Activate',
  'wizard.app.review.note':
    'Once activated, {app} appears in your team’s portal and we register the sign-in. You can deactivate it whenever you want: nothing is deleted inside {app}.',
  'wizard.app.done.heading': '{app} is now connected',
  'wizard.app.done.headingFallback': 'Connection ready',
  'wizard.app.done.okTitle': 'It all went well',
  'wizard.app.done.okBody': 'The connection is active and your team already has it available.',
  'wizard.app.done.nextTitle': 'What to do now',
  'wizard.app.done.step1Title': 'Check that it appears in the portal',
  'wizard.app.done.step1Body':
    'Open the Sytue app launcher: {app} shows up next to the rest of the suite for everybody who has access.',
  'wizard.app.done.step2Title': 'Try signing in',
  'wizard.app.done.step2Body':
    'Go into {app} with your Sytue account. If it lets you through without asking for an {app} password, it is set up correctly.',
  'wizard.app.done.step2BodyLaunch':
    'Go into {app} from the button below with your Sytue account. If it lets you through without asking for an {app} password, it is set up correctly.',
  'wizard.app.done.step3Title': 'Let the users synchronise',
  'wizard.app.done.step3Body':
    'The first synchronisation starts on its own. From the {app} card you can check the status or force another one.',
  'wizard.app.done.oidcTitle': 'Copy them now: they are not shown again',
  'wizard.app.done.oidcBody': 'Paste these credentials into the OIDC sign-in settings of {app}.',
  'wizard.app.done.samlTitle': 'Sytue SAML metadata',
  'wizard.app.done.samlBody': 'Register this address in the SSO settings of {app}.',
  'wizard.app.done.back': 'Back to Integrations',
  'wizard.app.done.open': 'Open {app}',

  // --- Asistente: regla de acceso condicional (AccessPolicyWizard) -----------
  'wizard.accessPolicy.andMore': { one: '{count} more', other: '{count} more' },
  'wizard.accessPolicy.aGroup': 'a group',
  'wizard.accessPolicy.aUser': 'a user',
  'wizard.accessPolicy.who.everyone': 'anybody in your organisation',
  'wizard.accessPolicy.who.nobodyYet': 'nobody yet (you have not chosen who)',
  'wizard.accessPolicy.cond.outside': {
    one: 'the connection comes from outside the network you listed (from home, a hotel or a phone, for example)',
    other:
      'the connection comes from outside the networks you listed (from home, a hotel or a phone, for example)',
  },
  'wizard.accessPolicy.cond.inside': {
    one: 'the connection comes from the network you listed',
    other: 'the connection comes from the networks you listed',
  },
  'wizard.accessPolicy.cond.untrusted': 'the device has not been marked as trusted',
  'wizard.accessPolicy.cond.apps': 'the application being opened is {apps}',
  'wizard.accessPolicy.cond.anyDay': 'any day',
  'wizard.accessPolicy.cond.scheduleDays': 'the attempt happens on {days}',
  'wizard.accessPolicy.cond.scheduleHours':
    'the attempt happens on {days} between {from} and {to} (UTC)',
  'wizard.accessPolicy.what.block': 'will not be able to sign in',
  'wizard.accessPolicy.what.mfa': 'will have to confirm their identity with a second factor',
  'wizard.accessPolicy.what.allow': 'will sign in with no further checks',
  'wizard.accessPolicy.sentence.always': 'With this rule, {who} {what} at every sign-in attempt.',
  'wizard.accessPolicy.sentence.when': 'With this rule, {who} {what} if {when}.',
  'wizard.accessPolicy.sentenceTitle': 'The rule, in one sentence',
  'wizard.accessPolicy.suggest.block': 'Block',
  'wizard.accessPolicy.suggest.mfa': 'Require verification',
  'wizard.accessPolicy.suggest.allow': 'Allow',
  'wizard.accessPolicy.suggest.withCondition': '{verb} if {condition}',
  'wizard.accessPolicy.suggest.plain': '{verb} access',
  'wizard.accessPolicy.err.badCidr':
    '“{range}” is not a valid network range. Write the network and its size: 203.0.113.0/24 for a whole office, or 198.51.100.7/32 for a single address.',
  'wizard.accessPolicy.err.bothTimes': 'Give both times: from when and until when.',
  'wizard.accessPolicy.fix.badData':
    'The service rejected one of the values. Check the network ranges in the “When” step.',
  'wizard.accessPolicy.fix.forbidden':
    'Your account does not have permission to change the access rules of this organisation.',
  'wizard.accessPolicy.fix.idpDown': 'Sytue ID is not responding right now. Try again in a minute.',
  'wizard.accessPolicy.risk.allowTitle': 'This rule cannot lock anybody out',
  'wizard.accessPolicy.risk.allowBody':
    'Rules that let people through cannot shut you out: they only make exceptions to stricter ones.',
  'wizard.accessPolicy.risk.lockoutTitle': 'This rule would lock out your whole organisation',
  'wizard.accessPolicy.risk.lockoutBody':
    '{sentence} Nobody would be able to sign in to Sytue again, you included, and you would not be able to get into this console to undo it either. Add a condition in the “When” step, or exclude your administrators in the “Who” step.',
  'wizard.accessPolicy.risk.affectsYouTitle': 'This rule affects you too',
  'wizard.accessPolicy.risk.noExitTitle': 'You have not left an emergency exit',
  'wizard.accessPolicy.risk.noExitBody':
    '{sentence} If it ever catches you, you will be locked out of the console and nobody will be able to put it right. Go back to the “Who” step and exclude your administrators group: they will still get in even when the rule matches.',
  'wizard.accessPolicy.risk.noExitBodyMfa':
    '{sentence} If it ever catches you, you will be locked out of the console and nobody will be able to put it right. Careful: anybody who has not set up two-step verification yet will have to set it up in order to sign in. Go back to the “Who” step and exclude your administrators group: they will still get in even when the rule matches.',
  'wizard.accessPolicy.risk.hasExitTitle': 'You have an emergency exit',
  'wizard.accessPolicy.risk.hasExitBody':
    '{sentence} Exempt: {exempt}. They will be able to sign in even when the rule matches. Check that you can sign in with one of those accounts before you leave.',
  'wizard.accessPolicy.risk.hasExitBodyMfa':
    '{sentence} Exempt: {exempt}. They will be able to sign in even when the rule matches. Careful: anybody who has not set up two-step verification yet will have to set it up in order to sign in. Check that you can sign in with one of those accounts before you leave.',
  'wizard.accessPolicy.pick.filterPlaceholder': 'Filter…',
  'wizard.accessPolicy.pick.filterAria': 'Filter the list',
  'wizard.accessPolicy.pick.noMatches': 'No matches.',
  'wizard.accessPolicy.action.mfaTitle': 'Ask them for one more check',
  'wizard.accessPolicy.action.mfaBody':
    'They get in, but first they have to confirm they are who they say with their second factor (the phone app or the security key). This is the most common choice.',
  'wizard.accessPolicy.action.blockTitle': 'Do not let them in',
  'wizard.accessPolicy.action.blockBody':
    'They will see an access denied message and will not be able to carry on. Use it to cut off situations that should never happen.',
  'wizard.accessPolicy.action.allowTitle': 'Let them through, full stop',
  'wizard.accessPolicy.action.allowBody':
    'They get straight in. It is there to make an exception to another, stricter rule — from the office network, for example.',
  'wizard.accessPolicy.step.who': 'Who',
  'wizard.accessPolicy.step.when': 'When',
  'wizard.accessPolicy.step.what': 'What happens',
  'wizard.accessPolicy.step.review': 'Summary',
  'wizard.accessPolicy.step.done': 'Done',
  'wizard.accessPolicy.who.heading': 'Who does this rule affect?',
  'wizard.accessPolicy.who.description':
    'Start by deciding who it applies to. Below you can leave out the people it must never affect.',
  'wizard.accessPolicy.who.allTitle': 'Everybody',
  'wizard.accessPolicy.who.allBody': 'It applies to anybody who signs in to your organisation.',
  'wizard.accessPolicy.who.someTitle': 'Only some people or groups',
  'wizard.accessPolicy.who.someBody':
    'It applies only to whoever you choose. Nobody else notices it.',
  'wizard.accessPolicy.who.groupsLabel': 'Groups affected',
  'wizard.accessPolicy.who.usersLabel': 'People affected',
  'wizard.accessPolicy.who.noGroups': 'There are no groups in your organisation yet.',
  'wizard.accessPolicy.who.noUsers': 'There are no users to show.',
  'wizard.accessPolicy.who.pickAtLeastOne':
    'Choose at least one group or one person, or go back to “Everybody”.',
  'wizard.accessPolicy.exempt.heading': 'Who is always exempt',
  'wizard.accessPolicy.exempt.description':
    'This is your emergency exit: these accounts get in even when the rule matches. Exclude your administrators here so that you do not lock yourself out of the console.',
  'wizard.accessPolicy.exempt.groupsLabel': 'Exempt groups',
  'wizard.accessPolicy.exempt.usersLabel': 'Exempt people',
  'wizard.accessPolicy.exempt.none':
    'You have not left anybody exempt. If this rule ends up affecting you, you will be locked out and unable to undo it.',
  'wizard.accessPolicy.exempt.some': 'Exempt: {names}. They will always be able to sign in.',
  'wizard.accessPolicy.when.heading': 'In which situations does it apply?',
  'wizard.accessPolicy.when.description':
    'Tick only what you need. If you tick nothing, the rule applies whenever those people try to sign in.',
  'wizard.accessPolicy.when.networkTitle': 'Which network they connect from',
  'wizard.accessPolicy.when.networkBody':
    'Write the networks you know (your office, the VPN…) and choose whether the rule should act inside or outside them.',
  'wizard.accessPolicy.when.networksLabel': 'Known networks',
  'wizard.accessPolicy.when.networksOk': {
    one: '{count} network recognised.',
    other: '{count} networks recognised.',
  },
  'wizard.accessPolicy.when.networksHint':
    'One per line. Whoever runs your company network can give them to you. Leave it empty not to look at the network.',
  'wizard.accessPolicy.when.outsideTitle': 'When they are OUTSIDE those networks',
  'wizard.accessPolicy.when.outsideBody':
    'This is the usual one: protecting what happens away from the office (home, hotels, public networks).',
  'wizard.accessPolicy.when.insideTitle': 'When they are INSIDE those networks',
  'wizard.accessPolicy.when.insideBody':
    'It only applies to whoever connects from those networks. Useful together with the “let them through, full stop” action.',
  'wizard.accessPolicy.when.deviceTitle': 'From which device',
  'wizard.accessPolicy.when.deviceSwitch': 'Only when the device is not trusted',
  'wizard.accessPolicy.when.deviceBody':
    'Each person marks their usual devices as trusted under “Devices”. With this switched on, the rule only acts when they sign in from one they have not marked: a borrowed computer, a new phone, an internet café.',
  'wizard.accessPolicy.when.appsTitle': 'In which applications',
  'wizard.accessPolicy.when.appsLabel': 'Applications',
  'wizard.accessPolicy.when.appsHint':
    'Identifiers of the applications, separated by spaces. Leave it empty to apply it to all of them (the most common choice).',
  'wizard.accessPolicy.when.timeTitle': 'At which times',
  'wizard.accessPolicy.when.timeBody':
    'Leave the days unticked and the times empty not to look at the calendar. The times are in UTC, not in each person’s local time.',
  'wizard.accessPolicy.when.daysLegend': 'Days',
  'wizard.accessPolicy.when.from': 'From',
  'wizard.accessPolicy.when.to': 'To',
  'wizard.accessPolicy.when.soFarTitle': 'How it stands so far',
  'wizard.accessPolicy.when.soFarNone':
    'With no conditions, the rule will apply whenever those people try to sign in.',
  'wizard.accessPolicy.when.soFarSome': 'It will apply if {when}.',
  'wizard.accessPolicy.what.heading': 'What do you want to happen in those cases?',
  'wizard.accessPolicy.what.description': 'Choose what the person finds when the rule matches.',
  'wizard.accessPolicy.what.strictestWins':
    'If several rules match at once, the strictest one wins: blocking beats asking for verification, and asking for verification beats letting through.',
  'wizard.accessPolicy.summary.who': 'Who it affects',
  'wizard.accessPolicy.summary.exempt': 'Who is exempt',
  'wizard.accessPolicy.summary.nobody': 'Nobody',
  'wizard.accessPolicy.summary.when': 'When it applies',
  'wizard.accessPolicy.summary.whenAlways': 'Whenever they try to sign in',
  'wizard.accessPolicy.summary.whenSome': 'If {when}',
  'wizard.accessPolicy.summary.what': 'What happens',
  'wizard.accessPolicy.summary.state': 'State',
  'wizard.accessPolicy.summary.stateOn': 'It will apply as soon as you save it',
  'wizard.accessPolicy.summary.stateOff': 'Saved without applying',
  'wizard.accessPolicy.review.headingNew': 'Review and save the rule',
  'wizard.accessPolicy.review.headingEdit': 'Review the changes',
  'wizard.accessPolicy.review.description':
    'This is what we are going to apply. You can change anything before saving.',
  'wizard.accessPolicy.review.saveNew': 'Create the rule',
  'wizard.accessPolicy.review.saveEdit': 'Save changes',
  'wizard.accessPolicy.review.understood':
    'I understand that nobody will be able to sign in and I want to save it anyway',
  'wizard.accessPolicy.review.nameLabel': 'Name of the rule',
  'wizard.accessPolicy.review.nameHint':
    'This is how you will see it in the list. If you leave it empty we will call it “{suggested}”.',
  'wizard.accessPolicy.review.priorityLabel': 'Evaluation order',
  'wizard.accessPolicy.review.priorityHint':
    'It only changes the order in which the rules are checked: the lowest number goes first. You can move it later from the list with the arrows.',
  'wizard.accessPolicy.review.enableLabel': 'Apply the rule straight away',
  'wizard.accessPolicy.review.enableOn': 'It will start being evaluated at the next sign-in.',
  'wizard.accessPolicy.review.enableOff':
    'It will be saved switched off: you can prepare it now and switch it on from the list whenever you want.',
  'wizard.accessPolicy.done.headingNew': 'Rule created',
  'wizard.accessPolicy.done.headingEdit': 'Changes saved',
  'wizard.accessPolicy.done.bodyOn': '{sentence} It is already being evaluated at every sign-in.',
  'wizard.accessPolicy.done.bodyOff':
    '{sentence} It is saved but switched off: switch it on from the list whenever you want.',
  'wizard.accessPolicy.done.checkTitle': 'Check it before you go',
  'wizard.accessPolicy.done.checkBody':
    'Open a private browser window and sign in with an account the rule affects. That way you see the same as your team and confirm that you still have access.',
  'wizard.accessPolicy.done.checkBodyExempt':
    'Open a private browser window and sign in with an account the rule affects. That way you see the same as your team and confirm that you still have access with the exempt accounts ({names}).',
  'wizard.accessPolicy.done.back': 'Back to the rules',

  // --- Platform administration (global scope) ---
  'nav.platform': 'Platform',
  'page.platform': 'Platform administration',
  // Conexiones con terceros (credenciales OAuth de la plataforma)
  'conn.title': 'Third-party connections',
  'conn.intro':
    'Credentials Sytue uses to identify itself with each provider. With them, connecting or migrating from Google Workspace or Microsoft 365 takes one click for your customers. Without them, each organisation must register its own app with the provider.',
  'conn.add': 'Add provider',
  'conn.edit': 'Edit',
  'conn.remove': 'Remove',
  'conn.empty': 'No provider configured yet',
  'conn.emptyBody':
    'Until you register at least one, every connection falls back to each customer bringing their own credentials.',
  'conn.redirectUri': 'Redirect URI to register with the provider',
  'conn.redirectUriHelp':
    'The same for every provider. A mismatch here is the most common cause of a failed consent, and the provider only reports it as «redirect_uri_mismatch».',
  // Cómo conseguir el par de credenciales de cada proveedor: cuatro pasos dentro
  // del propio diálogo, que es donde hacen falta.
  'conn.guide.step1': 'Open the {provider} developer console and create an OAuth app.',
  'conn.guide.open': 'Open console',
  'conn.guide.type': 'Application type',
  'conn.guide.step2': 'Register this redirect URL there, exactly as it reads:',
  'conn.guide.copy': 'Copy',
  'conn.guide.copied': 'Copied',
  'conn.guide.step3': 'Copy the client ID and the secret it gives you.',
  'conn.guide.step4':
    'Paste them below and save. The secret is stored encrypted and never shown again.',
  'conn.guide.troubleshoot':
    '«invalid_client» means the client ID does not match the one over there; «redirect_uri_mismatch», the URL above.',
  'nav.connections': 'Connections',
  'conn.col.provider': 'Provider',
  'conn.col.secret': 'Secret',
  'conn.col.updated': 'Last change',
  'conn.secret.set': 'Stored',
  'conn.secret.public': 'Public client (PKCE)',
  'conn.addTitle': 'Register a provider app',
  'conn.editTitle': 'Edit credentials',
  'conn.field.provider': 'Provider',
  'conn.field.secretHelp':
    'Leave empty if the provider registered the app as a public client and relies on PKCE alone.',
  'conn.field.secretKeepHelp':
    'Leave empty to keep the stored secret. Many providers only show it once.',
  'conn.toast.removed': 'Provider removed',
  'conn.toast.failed': 'Could not be saved',
  'conn.error.forbidden': 'Platform administrators only',
  'conn.error.forbiddenBody':
    'These credentials serve every customer, so they require the platform allowlist and a second factor.',
  'conn.error.unavailable': 'Not configured in this deployment',
  'conn.error.unavailableBody':
    'The operator token for Sytue Integrations is missing, so credentials cannot be managed from here.',
  'conn.templates.title': 'Integration availability across the suite',
  'conn.templates.intro':
    'Enable or disable availability for each connector across the suite and organisations. Only integrations enabled here will appear in Sytue Admin for connection or migration.',
  'conn.templates.empty': 'No integrations in the catalog',
  'conn.templates.emptyBody': 'The global connector catalog has no registered templates.',
  'conn.templates.tableCaption': 'Integration availability',

  'platform.tab.orgs': 'Organisations',
  'platform.tab.usage': 'Usage',
  'platform.tab.merges': 'Merges',

  // Visor de lo que está fallando (mig 0049): problemas agrupados, no líneas.
  'platform.tab.versions': 'Versions',
  'platform.versions.title': 'What is running',
  'platform.versions.unavailable': 'Versions could not be read',
  'platform.versions.empty': 'No components to check',
  'platform.versions.emptyHint':
    'The chart did not pass a component catalogue, so there is nothing to ask. Check global.components in the umbrella chart.',
  'platform.versions.refresh': 'Refresh',
  'platform.versions.upToDate': 'In step',
  'platform.versions.behind': 'Out of step',
  'platform.versions.kind.service': 'service',
  'platform.versions.kind.surface': 'web',
  'platform.versions.col.component': 'Component',
  'platform.versions.col.release': 'Release',
  'platform.versions.col.commit': 'Commit',
  'platform.versions.col.status': 'Status',
  'platform.versions.kpi.expected': 'Release in the majority',
  'platform.versions.kpi.upToDate': 'In step',
  'platform.versions.kpi.behind': 'Out of step',
  'platform.versions.kpi.silent': 'Not answering',
  'platform.tab.logs': 'What is failing',
  'logs.kpi.errors1h': 'Errors in the last hour',
  'logs.kpi.errors1hHint': 'If this is not zero, something is happening right now.',
  'logs.kpi.errors24h': 'Errors (24 h)',
  'logs.kpi.warns24h': 'Warnings (24 h)',
  'logs.kpi.services': 'Services affected',
  'logs.kpi.servicesHint': 'Out of the eleven in the suite.',
  'logs.trend.title': 'Last 24 hours',
  'logs.trend.label': 'Errors and warnings per hour over the last 24 hours',
  'logs.trend.footer': 'Errors in red, warnings underneath. Empty hours are shown too.',
  'logs.trend.errors': {
    one: '{count} error',
    other: '{count} errors',
  },
  'logs.trend.warns': {
    one: '{count} warning',
    other: '{count} warnings',
  },
  'logs.list.title': 'Problems, grouped',
  'logs.filter.level': 'Level',
  'logs.filter.allLevels': 'Errors and warnings',
  'logs.filter.service': 'Service',
  'logs.filter.allServices': 'All services',
  'logs.filter.search': 'Search in the message',
  'logs.level.error': 'Error',
  'logs.level.warn': 'Warning',
  'logs.times': 'times',
  'logs.since': 'since {when}',
  'logs.last': 'last {when}',
  'logs.showDetail': 'Detail',
  'logs.hideDetail': 'Hide',
  'logs.empty': 'Nothing is failing',
  'logs.emptyHint': 'No errors or warnings in the selected window.',
  'logs.unavailable': 'The log cannot be read',
  'logs.unavailableHint':
    'Sytue ID did not answer. The services keep writing to it; only this view is down.',
  'platform.tab.admins': 'Global admins',
  'platform.orgs.title': 'All organisations',
  'platform.orgs.filter': 'Filter by name or identifier',
  'platform.orgs.empty': 'No organisation matches the filter.',
  'platform.col.org': 'Organisation',
  'platform.col.status': 'Status',
  'platform.col.capacity': 'Users / seats',
  'platform.col.owner': 'Owners',
  'platform.col.actions': 'Actions',
  'platform.col.merge': 'Merge',
  'platform.col.progress': 'Progress',
  'platform.col.email': 'Email',
  'platform.col.reason': 'Reason',
  'platform.col.since': 'Since',
  'platform.status.active': 'Active',
  'platform.status.suspended': 'Suspended',
  'platform.status.deleted': 'Deleted',
  'platform.status.merged': 'Merged',
  'platform.action.grantSeats': 'Grant seats',
  'platform.action.suspend': 'Suspend',
  'platform.action.delete': 'Delete',
  'platform.prompt.deleteTitle': 'Delete this organisation',
  'platform.prompt.deleteHint':
    'It stops serving and disappears from day-to-day operations. Its records are kept so you can answer what happened to it, and reactivating brings it back. Requires a second global admin to sign off.',
  'platform.action.reactivate': 'Reactivate',
  'platform.action.merge': 'Merge',
  'platform.action.resume': 'Resume',
  'platform.usage.orgs': 'Organisations',
  'platform.usage.orgsActive': 'Active',
  'platform.usage.orgsSuspended': 'Suspended',
  'platform.usage.users': 'Active users',
  'platform.usage.users30d': 'Signed in (30 days)',
  'platform.usage.seatsSold': 'Seats sold',
  'platform.usage.seatsGranted': 'Seats granted',
  'platform.usage.subs': 'Active subscriptions',
  'platform.merges.title': 'Organisation merges',
  'platform.merges.empty': 'No merge has been run yet.',
  'platform.merge.state.pending': 'Pending',
  'platform.merge.state.running': 'Running',
  'platform.merge.state.completed': 'Completed',
  'platform.merge.state.failed': 'Failed',
  'platform.merge.state.aborted': 'Stopped',
  'platform.admins.title': 'Platform administrators',
  'platform.seats.count': 'Seats',
  'platform.seats.kind': 'Type',
  'platform.seats.kind.goodwill': 'Commercial gesture',
  'platform.seats.kind.employee': 'Sytue employee',
  'platform.seats.kind.trial': 'Trial',
  'platform.seats.kind.migration': 'Migration',
  'platform.seats.reason': 'Reason (required)',
  'platform.seats.expires': 'Expires (optional)',
  'platform.merge.warning':
    'Merging is irreversible: the members and all the data of the source organisation move to the target one, and the source is closed.',
  'platform.merge.target': 'Target organisation',
  'platform.merge.confirm': 'Type {slug} to confirm',
  'platform.prompt.statusReason': 'Reason for this change:',
  'platform.toast.statusSaved': 'Status updated.',
  'platform.toast.seatsGranted': 'Seats granted.',
  'platform.toast.mergeStarted': 'Merge started.',
  'platform.toast.failed': 'The operation could not be completed.',
  'platform.error.forbidden': 'You cannot access platform administration',
  'platform.error.unavailable': 'Platform administration is unavailable',
  'platform.error.unavailableHint': 'Sytue ID did not answer. Try again in a moment.',

  // Los tres 403 de esta API, separados: llevan a sitios distintos.
  'platform.error.mfa': 'Sign in again with your second factor',
  'platform.error.mfaHint':
    'You are a global administrator, but this session did not use two-step verification. Sign out and back in with it.',
  'platform.error.signOut': 'Sign out',
  'platform.error.notAllowlisted':
    'This account is not on the platform allowlist. Another global administrator has to add it; a second factor will not change this.',

  // Elevacion temporal del alcance global (PIM).
  'platform.pim.gateTitle': 'Elevate to administer the platform',
  'platform.pim.gateBody':
    'Being a global administrator means you can elevate, not that you are elevated. Ask for a window, say what you need it for, and it ends on its own.',
  'platform.pim.elevate': 'Elevate',
  'platform.pim.stepDown': 'Step down',
  'platform.pim.modalTitle': 'Elevate to platform scope',
  'platform.pim.justification': 'What do you need it for (required)',
  'platform.pim.justificationHint':
    'It goes into the record. Months from now it is the only answer to why anyone was elevated.',
  'platform.pim.duration': 'For how long',
  'platform.pim.duration.minutes': { one: '{count} minute', other: '{count} minutes' },
  'platform.pim.duration.hours': { one: '{count} hour', other: '{count} hours' },
  'platform.pim.active': 'Platform scope active',
  'platform.pim.remaining': { one: '{count} minute left', other: '{count} minutes left' },
  'platform.pim.expiringSoon': 'Less than a minute left',
  'platform.pim.toast.elevated': 'You are elevated. It ends on its own.',
  'platform.pim.toast.steppedDown': 'You are no longer elevated.',
  'platform.pim.history': 'Elevation history',
  'platform.pim.history.empty': 'Nobody has elevated yet.',
  'platform.pim.col.who': 'Who',
  'platform.pim.col.when': 'When',
  'platform.pim.col.window': 'Window',
  'platform.pim.state.active': 'Active',
  'platform.pim.state.expired': 'Ended',
  'platform.pim.state.revoked': 'Stepped down',

  // Detalle de una organización y sus plazas regaladas.
  'platform.detail.namespace': 'Namespace',
  'platform.detail.grants': 'Granted seats',
  'platform.detail.noGrants': 'No seats have been granted to this organisation.',
  'platform.detail.grantForever': 'No end date',
  'platform.detail.grantExpired': 'Expired',
  'platform.detail.grantRevoked': 'Withdrawn',
  'platform.detail.revokeGrant': 'Withdraw',
  'platform.seats.revoked': 'Seats withdrawn.',

  // Qué está desplegado (release y commit horneados en la imagen).
  'platform.version.unbuilt': 'Built by hand',
  'platform.version.unreachable': 'Did not answer',

  // Revisión periódica del alcance global (mig 0047).
  'platform.review.col.reviewed': 'Last reviewed',
  'platform.review.confirm': 'Still needed',
  'platform.review.done': 'Reviewed.',
  'platform.review.notYourself': 'Your own scope is reviewed by another global administrator',
  'platform.review.overdue': { one: 'Over {count} day', other: 'Over {count} days' },
  'platform.review.pending': {
    one: '{count} global access to review',
    other: '{count} global accesses to review',
  },
  'platform.review.pendingHint':
    'Nothing has been removed. Somebody has to look at whether these still make sense.',

  // Segunda persona para lo irreversible (mig 0046).
  'platform.tab.approvals': 'Approvals',
  'platform.approval.title': 'Operations waiting for a second person',
  'platform.approval.intro':
    'Suspending an organisation locks out a whole company and merging cannot be undone. Whoever asks for one of these cannot be the one who approves it.',
  'platform.approval.empty': 'Nothing is waiting to be approved.',
  'platform.approval.approve': 'Approve and run',
  'platform.approval.reject': 'Reject',
  'platform.approval.col.requester': 'Asked by',
  'platform.approval.action.org.suspend': 'Suspend organisation',
  'platform.approval.action.org.delete': 'Delete organisation',
  'platform.approval.action.org.merge': 'Merge organisations',
  'platform.approval.action.platform.admin.grant': 'Grant global scope',
  'platform.approval.state.pending': 'Waiting',
  'platform.approval.state.executed': 'Done',
  'platform.approval.state.rejected': 'Rejected',
  'platform.approval.state.expired': 'Expired',
  'platform.approval.queued': 'It is waiting for a second person',
  'platform.approval.queuedHint':
    'Nothing has happened yet. Another global administrator has to approve it, and it expires in a day.',
  'platform.approval.samePerson': 'You cannot approve what you asked for yourself',
  'platform.approval.waiting': {
    one: '{count} operation waiting',
    other: '{count} operations waiting',
  },
  'platform.approval.waitingHint': 'Somebody asked for something that needs your approval.',

  // Registro de la plataforma (mig 0045).
  'platform.tab.audit': 'Record',
  'platform.audit.title': 'What global administrators have done',
  'platform.audit.filter': 'Kind of action',
  'platform.audit.empty': 'Nothing has been done from platform administration yet.',
  'platform.audit.noOrg': 'No organisation',
  'platform.audit.col.action': 'Action',
  'platform.audit.col.detail': 'Detail',
  'platform.audit.family.all': 'Everything',
  'platform.audit.family.platform.org.': 'Organisations',
  'platform.audit.family.platform.seats.': 'Seats',
  'platform.audit.family.platform.merge.': 'Merges',
  'platform.audit.family.platform.admin.': 'Global scope',
  'platform.audit.family.platform.pim.': 'Elevations',
  'platform.audit.family.platform.approval.': 'Approvals',

  'platform.education.badge': 'Education (50%)',
  'platform.education.toggle': 'Educational institution',
  'platform.education.enable': 'Mark as educational',
  'platform.education.disable': 'Remove educational status',
  'platform.prompt.educationReason': 'Reason for educational status change:',
  'platform.toast.educationSaved': 'Educational status updated.',

  'platform.admins.revoke': 'Revoke scope',
  'platform.prompt.revokeAdminTitle': 'Revoke global scope',
  'platform.prompt.revokeAdminHint':
    'This user will no longer be able to manage the platform. Their normal account and memberships are kept.',
  'platform.toast.adminRevoked': 'Global scope revoked.',
  'platform.create.open': 'Create organisation',
  'platform.create.title': 'Create an organisation',
  'platform.create.submit': 'Create',
  'platform.create.slug': 'Identifier',
  'platform.create.slugHelp': 'Lowercase letters, digits and hyphens. It appears in the address.',
  'platform.create.name': 'Name',
  'platform.create.owner': "Owner's email",
  'platform.create.ownerHelp':
    'It must already exist as a Sytue account. Leave it empty to assign it later.',
  'platform.create.trial': 'Trial seats (optional)',
  'platform.create.trialEnds': 'End of the trial',
  'platform.create.done': 'Organisation “{slug}” created.',
  'platform.create.doneTrial': {
    one: '{count} trial seat granted.',
    other: '{count} trial seats granted.',
  },
  'platform.create.error.forbidden':
    'Not allowed. The elevation may have expired: elevate again and retry.',
  'platform.create.error.conflict': 'That identifier already exists or is reserved.',
  'platform.create.error.invalid':
    'Check the identifier, and that the email belongs to an existing account.',
  'platform.create.error.trial':
    'The organisation was created, but the trial seats were not: grant them from its details.',

  // Propietarios y cuentas de una organización desde plataforma (mig 0113).
  'platform.members.title': 'People',
  'platform.members.search': 'Search by name or email',
  'platform.members.empty': 'Nobody here matches.',
  'platform.members.col.person': 'Person',
  'platform.members.col.role': 'Role',
  'platform.members.status.active': 'Active',
  'platform.members.status.suspended': 'Blocked',
  'platform.members.status.deleted': 'Deleted',
  'platform.members.ownersHint':
    "An organisation can have several owners. Assigning one does not remove the others: that is done from the organisation's own console.",
  'platform.members.assignOwner': 'Assign owner',
  'platform.members.inviteOwner': 'Create owner',
  'platform.members.makeOwner': 'Make owner',
  'platform.members.assignTitle': 'Assign an owner',
  'platform.members.assignHelp':
    'The account must already exist. If it is not a member of this organisation, it joins as owner.',
  'platform.members.inviteTitle': 'Create an owner',
  'platform.members.inviteHelp':
    'They get an invitation by email and choose their own password when they accept it.',
  'platform.members.name': 'Full name',
  'platform.members.resetPassword': 'Reset password',
  'platform.members.passwordTitle': 'Reset the password of {email}',
  'platform.members.passwordMode': 'How',
  'platform.members.passwordLink': 'Send a reset link by email',
  'platform.members.passwordTemporary': 'Set a temporary password',
  'platform.members.passwordHint':
    'Either way their sessions are closed and their tokens revoked. A temporary password is shown once and has to be changed at the next sign-in.',
  'platform.members.temporaryTitle': 'Temporary password',
  'platform.members.temporaryOnce':
    'Copy it now: it will not be shown again and is not kept anywhere. Give it to {email} through a channel you trust.',
  'platform.members.block': 'Block',
  'platform.members.unblock': 'Unblock',
  'platform.members.blockTitle': 'Block {email}',
  'platform.members.blockHint':
    'They lose access at once: sessions closed and tokens revoked. Nothing is deleted, and you can unblock them.',
  'platform.members.unblockTitle': 'Unblock {email}',
  'platform.members.deleteTitle': 'Delete {email}',
  'platform.members.deleteHint':
    'The account is closed and loses all access. The organisation can restore it for 30 days; after that it cannot be undone.',
  'platform.members.deleteConfirm': 'Type {email} to confirm',
  'platform.members.sessionsClosed': {
    one: '{count} session closed.',
    other: '{count} sessions closed.',
  },
  'platform.members.toast.ownerAssigned': 'Owner assigned.',
  'platform.members.toast.alreadyOwner': 'That account was already an owner.',
  'platform.members.toast.ownerInvited': 'Invitation sent to {email}.',
  'platform.members.toast.linkSent': 'Reset link sent.',
  'platform.members.toast.blocked': 'Account blocked.',
  'platform.members.toast.unblocked': 'Account unblocked.',
  'platform.members.toast.deleted': 'Account deleted.',
  'platform.members.error.stepUp':
    'Confirm it is you: sign out, sign back in with your second factor and repeat the action.',
  'platform.members.error.soleOwner':
    'This person is the only owner of {orgs}. Assign another owner there first.',
  'platform.members.error.noSuchUser':
    'There is no account with that email. Create the owner instead.',
  'platform.members.error.accountExists':
    'That email already has an account. Assign it as owner instead.',
  'platform.members.error.inactive': 'That account is blocked or closed. Unblock it first.',
  'platform.members.error.platformAdmin':
    'This account administers the platform. Revoke its global scope first.',
  'platform.audit.family.platform.user.': 'Accounts',
  'nav.moderation': 'Reports',
  'mod.intro':
    "Reports from Sytue Cast and Sytue Rooms reach this queue and the one of whoever administers the organisation that owns the content. Resolving one here closes it there, marked as done by Sytue. Every view and every action is recorded in the platform audit log and in the organisation's.",
  'mod.suspend': 'To suspend an organisation, go to Platform.',
  'mod.service.cast': 'Sytue Cast',
  'mod.service.rooms': 'Sytue Rooms',
  'mod.status.open': 'Open',
  'mod.status.resolved': 'Resolved',
  'mod.loading': 'Loading…',
  'mod.empty': 'No reports here.',
  'mod.col.date': 'Date',
  'mod.col.org': 'Organisation',
  'mod.col.what': 'Reported',
  'mod.col.reason': 'Reason',
  'mod.col.content': 'What the report keeps',
  'mod.col.actions': 'Action',
  'mod.kind.video': 'Video',
  'mod.kind.comment': 'Comment',
  'mod.kind.chat': 'Live chat message',
  'mod.kind.person': 'Person',
  'mod.kind.channel': 'Channel',
  'mod.kind.message': 'Message',
  'mod.kind.member': 'Person in a conversation',
  'mod.reason.harassment': 'Harassment',
  'mod.reason.sexual': 'Sexual content or nudity',
  'mod.reason.violence': 'Violence',
  'mod.reason.spam': 'Spam or scam',
  'mod.reason.copyright': 'Copyright',
  'mod.reason.other': 'Other',
  'mod.action.dismiss': 'Dismiss',
  'mod.action.delete': 'Delete',
  'mod.action.unlist': 'Take off the front page',
  'mod.action.unpublish': 'Unpublish',
  'mod.action.block': 'Block in the organisation',
  'mod.action.remove': 'Remove from the channel',
  'mod.from.platform': 'by Sytue',
  'mod.from.org': 'by the organisation',
  'mod.confirm.title': '{action}?',
  'mod.confirm.body':
    'It is recorded in your name, as Sytue, in the audit log of {org}, and the report closes in its queue too.',
  'mod.toast.done': 'Report resolved.',
  'mod.toast.failed': 'It could not be done. Your elevation may have expired.',
  'mod.error.forbidden': 'Elevate your platform scope first',
  'mod.error.forbiddenBody':
    'The report queue opens like the rest of the platform console: with two-step verification and an active elevation. Elevate from Platform and come back.',
  'mod.error.unavailable': 'The service is not responding',
  'mod.error.unavailableBody':
    'Sytue ID could not confirm your access, or the service did not answer. Nothing is shown without that confirmation. Try again in a moment.',
} satisfies Record<string, MessageEntry>;
