/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo INGLÉS de Mi cuenta — es la FUENTE DE VERDAD: los otros cuatro
 * idiomas derivan su tipo de él, así que una clave nueva aquí rompe la
 * compilación de es/fr/de/it en vez de salir sin traducir.
 */
export const en = {
  'app.title': 'My account',

  // --- Perfil ---
  'profile.title': 'Profile',
  'profile.name': 'Display name',
  'profile.name.hint': 'How you appear across the suite.',
  'profile.username': 'Username',
  'profile.username.hint':
    'People from other organisations on Sytue One use it to find you, message you or call you.',
  'profile.email': 'Email address',
  'profile.email.hint': 'Verified. Changing it is a separate process.',
  'profile.save': 'Save changes',
  'profile.saving': 'Saving…',
  'profile.saved': 'Changes saved.',
  'profile.failed': 'Could not save your changes. Try again.',
  'profile.avatarChange': 'Change avatar',
  'profile.avatarAdd': 'Add an avatar',
  'profile.avatarPresets': 'Preset colours',
  'profile.avatarUpload': 'Upload a photo…',
  'profile.avatarRemove': 'Remove avatar',
  'photo.errorFormat': 'Unsupported format. Use JPEG, PNG, WebP or GIF.',
  'photo.errorProcess': 'The image could not be processed.',
  'photo.errorTooLarge': 'The image is too large even after optimising it.',
  'photo.errorRead': 'The image could not be read.',

  // --- Sesiones ---
  'sessions.title': 'Open sessions',
  'sessions.current': 'This device',
  'sessions.close_one': 'Close this session',
  'sessions.close_others': 'Close all sessions',
  'sessions.close_others_note': 'This one included: you will have to sign in again.',
  'sessions.empty': 'No other sessions are open.',
  'sessions.failed': 'Could not load your sessions.',
  'common.loading': 'Loading…',

  // --- Dispositivos ---
  'devices.title': 'Devices',
  'devices.current': 'This device',
  'devices.trusted': 'Trusted',
  'devices.trust': 'Mark as trusted',
  'devices.forget': 'Forget this device',
  'devices.empty': 'No devices recognised yet.',
  'devices.failed': 'Could not load your devices.',

  // --- Seguridad ---
  'security.title': 'Security',
  'security.failed': 'Something went wrong. Try again.',
  'security.password.title': 'Password',
  'security.password.warning': 'Changing it signs you out everywhere, including here.',
  'security.password.current': 'Current password',
  'security.password.new': 'New password',
  'security.password.submit': 'Change password',
  'security.password.done': 'Password changed. All sessions were closed, including this one.',
  'security.password.signin_again': 'Sign in again',
  'security.mfa.title': 'Two-step verification',
  'security.mfa.off': 'Not set up. Your password alone opens the account.',
  'security.mfa.required':
    'This action requires two-step verification. Set it up below, then sign in again.',
  'security.mfa.on': 'On. You are asked for a code when you sign in.',
  'security.mfa.enable': 'Set up',
  'security.mfa.scan': 'Scan this with your authenticator app.',
  'security.mfa.qr_alt': 'QR code for two-step verification setup',
  'security.mfa.manual': 'Or enter this key by hand:',
  'security.mfa.code': 'Code from the app',
  'security.mfa.confirm': 'Turn on',
  'security.mfa.disable': 'Turn off',
  'security.mfa.disable_hint': 'To turn it off, give a current code or your password.',
  'security.mfa.code_or_password': 'Code from the app',
  'security.mfa.new_codes': 'New recovery codes',
  'security.codes.title': 'Recovery codes',
  'security.codes.once': 'Shown once. Save them somewhere safe: each one works a single time.',
  'security.codes.saved': 'I have saved them',
  'common.cancel': 'Cancel',

  // --- Perfil: el resto de la ficha ---
  'profile.given_name': 'First name',
  'profile.family_name': 'Last name',
  'profile.job_title': 'Job title',
  'profile.department': 'Department',
  'profile.location': 'Location',
  'profile.locale': 'Language',
  'profile.locale.hint': 'Used across Sytue and in the messages we send you.',

  // --- Passkeys ---
  'security.passkeys.title': 'Passkeys',
  'security.passkeys.remove': 'Remove',
  'security.passkeys.unnamed': 'Unnamed key',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': 'App passwords',
  'app_passwords.hint':
    'For mail apps that ask for a password. Each one works on its own and can be revoked without touching the others.',
  'app_passwords.label': 'What is it for',
  'app_passwords.label.hint': 'For example: laptop mail, phone.',
  'app_passwords.create': 'Create password',
  'app_passwords.revoke': 'Revoke',
  'app_passwords.empty': 'No app passwords yet.',
  'app_passwords.failed': 'Could not load your app passwords.',
  'app_passwords.unnamed': 'Unnamed',
  'app_passwords.once': 'Shown once. Copy it into the app now: it cannot be seen again.',
  'app_passwords.saved': 'I have copied it',
  'app_passwords.never_used': 'Never used',
  'app_passwords.last_used': 'Last used {date}',

  // --- Perfil de configuración de Apple ---
  'profile.apple': 'Set up Mail and Calendar on Apple devices',
  'profile.apple.hint': 'Downloads a configuration profile for iPhone, iPad and Mac.',

  // --- Alta de passkeys ---
  'security.passkeys.hint':
    'Sign in with your fingerprint, face or a security key, without typing a password.',
  'security.passkeys.add': 'Add a passkey',
  'security.passkeys.adding': 'Waiting for your device…',
  'security.passkeys.name': 'Name it',
  'security.passkeys.name.hint': 'For example: this laptop, USB key.',
  'security.passkeys.duplicada': 'That key is already registered on this account.',
  'security.passkeys.no_soportado': 'This browser does not support passkeys.',
  'security.passkeys.error': 'The passkey could not be added. Try again.',

  // --- Plan y almacenamiento (cuentas personales) ---
  'plan.title': 'Plan and storage',
  'plan.loading': 'Loading your plan…',
  'plan.free': 'Free plan',
  'plan.pro': 'Pro plan · 200 GB',
  'plan.enterprise': 'Enterprise plan · 1 TB · Premium Support',
  'plan.storage': 'Storage used',
  'plan.externalNote': 'Connected storage extends your Sytue capacity.',
  'plan.externalCapacity': 'Connected storage adds {capacity}.',
  'plan.externalUnknown': 'The provider does not report its capacity.',
  'plan.baseUsage': 'Sytue: {used} of {total}',
  'plan.paidUntil': 'Paid until',
  'plan.upgrade': 'Get more space',
  'plan.upgrade.hint':
    '200 GB for €5/User/Month (Pro) or 1 TB with Premium Support for €25/User/Month (Enterprise), for your mail, files, photos and documents. Cancel whenever you like.',
  'plan.upgrade.soon': 'Paid plans are coming soon; they cannot be purchased yet.',
  'plan.unavailable': 'Payments are not available on this installation yet.',
  'plan.error': 'We couldn’t load your plan. Try again in a moment.',

  // Acceso y recuperación (ADR 0041).
  'recovery.title': 'Sign-in and recovery',
  'recovery.hint':
    'You can sign in with your @username, your Sytue address, your mobile number or your recovery email, always with the same password. The recovery email also receives the link to reset it.',
  'recovery.phone': 'Mobile number',
  'recovery.phone.hint': 'Verified by text message when the account was created.',
  'recovery.email': 'Recovery email',
  'recovery.new': 'New recovery email',
  'recovery.send': 'Send confirmation link',
  'recovery.sent': 'Link sent. Open it from that address to confirm it.',
  'recovery.remove': 'Remove',
  'recovery.removed': 'Recovery email removed.',
  'recovery.invalid': 'That address can’t be used: it must be a valid address outside Sytue.',
  'recovery.wrong_password': 'The password isn’t correct.',
  'recovery.failed': 'It couldn’t be saved. Try again in a moment.',
  'delete.title': 'Delete my account',
  'delete.hint':
    'Your sign-in details and everything that is yours alone are deleted; what belongs to your organisation stays with it. For 30 days it can be recovered.',
  'delete.link': 'See what is deleted and continue',
} as const;
