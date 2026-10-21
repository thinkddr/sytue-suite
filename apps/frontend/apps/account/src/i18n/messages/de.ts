/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  'app.title': 'Mein Konto',

  'profile.title': 'Profil',
  'profile.name': 'Anzeigename',
  'profile.name.hint': 'So erscheinen Sie in der gesamten Suite.',
  'profile.username': 'Benutzername',
  'profile.username.hint':
    'Über diesen Namen können Personen aus anderen Organisationen in Sytue One Sie finden, Ihnen schreiben oder Sie anrufen.',
  'profile.email': 'E-Mail-Adresse',
  'profile.email.hint': 'Bestätigt. Eine Änderung ist ein eigener Vorgang.',
  'profile.save': 'Änderungen speichern',
  'profile.saving': 'Wird gespeichert…',
  'profile.saved': 'Änderungen gespeichert.',
  'profile.failed': 'Änderungen konnten nicht gespeichert werden. Bitte erneut versuchen.',
  'profile.avatarChange': 'Avatar ändern',
  'profile.avatarAdd': 'Avatar hinzufügen',
  'profile.avatarPresets': 'Vordefinierte Farben',
  'profile.avatarUpload': 'Foto hochladen…',
  'profile.avatarRemove': 'Avatar entfernen',
  'photo.errorFormat': 'Format nicht unterstützt. Verwenden Sie JPEG, PNG, WebP oder GIF.',
  'photo.errorProcess': 'Das Bild konnte nicht verarbeitet werden.',
  'photo.errorTooLarge': 'Das Bild ist auch nach der Optimierung zu groß.',
  'photo.errorRead': 'Das Bild konnte nicht gelesen werden.',

  // --- Sesiones ---
  'sessions.title': 'Offene Sitzungen',
  'sessions.current': 'Dieses Gerät',
  'sessions.close_one': 'Diese Sitzung beenden',
  'sessions.close_others': 'Alle Sitzungen beenden',
  'sessions.close_others_note': 'Auch diese: Sie müssen sich erneut anmelden.',
  'sessions.empty': 'Keine weiteren Sitzungen offen.',
  'sessions.failed': 'Sitzungen konnten nicht geladen werden.',
  'common.loading': 'Wird geladen…',

  // --- Dispositivos ---
  'devices.title': 'Geräte',
  'devices.current': 'Dieses Gerät',
  'devices.trusted': 'Vertrauenswürdig',
  'devices.trust': 'Als vertrauenswürdig markieren',
  'devices.forget': 'Dieses Gerät vergessen',
  'devices.empty': 'Noch keine Geräte erkannt.',
  'devices.failed': 'Geräte konnten nicht geladen werden.',

  // --- Seguridad ---
  'security.title': 'Sicherheit',
  'security.failed': 'Etwas ist schiefgelaufen. Versuchen Sie es erneut.',
  'security.password.title': 'Passwort',
  'security.password.warning': 'Eine Änderung meldet Sie überall ab, auch hier.',
  'security.password.current': 'Aktuelles Passwort',
  'security.password.new': 'Neues Passwort',
  'security.password.submit': 'Passwort ändern',
  'security.password.done': 'Passwort geändert. Alle Sitzungen wurden beendet, auch diese.',
  'security.password.signin_again': 'Erneut anmelden',
  'security.mfa.title': 'Bestätigung in zwei Schritten',
  'security.mfa.off': 'Nicht eingerichtet. Ihr Passwort allein öffnet das Konto.',
  'security.mfa.required':
    'Deine Organisation verlangt die Bestätigung in zwei Schritten. Aktiviere sie unten und melde dich erneut an.',
  'security.mfa.on': 'Aktiv. Bei der Anmeldung wird ein Code abgefragt.',
  'security.mfa.enable': 'Einrichten',
  'security.mfa.scan': 'Scannen Sie dies mit Ihrer Authenticator-App.',
  'security.mfa.qr_alt': 'QR-Code zur Einrichtung der Bestätigung in zwei Schritten',
  'security.mfa.manual': 'Oder geben Sie diesen Schlüssel von Hand ein:',
  'security.mfa.code': 'Code aus der App',
  'security.mfa.confirm': 'Aktivieren',
  'security.mfa.disable': 'Deaktivieren',
  'security.mfa.disable_hint': 'Zum Deaktivieren einen gültigen Code oder Ihr Passwort eingeben.',
  'security.mfa.code_or_password': 'Code aus der App',
  'security.mfa.new_codes': 'Neue Wiederherstellungscodes',
  'security.codes.title': 'Wiederherstellungscodes',
  'security.codes.once': 'Werden einmal angezeigt. Sicher aufbewahren: jeder gilt nur einmal.',
  'security.codes.saved': 'Ich habe sie gespeichert',
  'common.cancel': 'Abbrechen',

  // --- Perfil: el resto de la ficha ---
  'profile.given_name': 'Vorname',
  'profile.family_name': 'Nachname',
  'profile.job_title': 'Position',
  'profile.department': 'Abteilung',
  'profile.location': 'Standort',
  'profile.locale': 'Sprache',
  'profile.locale.hint': 'Gilt in der ganzen Suite und für unsere Nachrichten an Sie.',

  // --- Passkeys ---
  'security.passkeys.title': 'Passkeys',
  'security.passkeys.remove': 'Entfernen',
  'security.passkeys.unnamed': 'Schlüssel ohne Namen',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': 'App-Passwörter',
  'app_passwords.hint':
    'Für Mail-Apps, die ein Passwort verlangen. Jedes gilt für sich und lässt sich einzeln widerrufen.',
  'app_passwords.label': 'Wofür ist es',
  'app_passwords.label.hint': 'Zum Beispiel: Mail am Laptop, Telefon.',
  'app_passwords.create': 'Passwort erstellen',
  'app_passwords.revoke': 'Widerrufen',
  'app_passwords.empty': 'Noch keine App-Passwörter.',
  'app_passwords.failed': 'App-Passwörter konnten nicht geladen werden.',
  'app_passwords.unnamed': 'Ohne Namen',
  'app_passwords.once':
    'Wird einmal angezeigt. Jetzt in die App kopieren: danach nicht mehr sichtbar.',
  'app_passwords.saved': 'Ich habe es kopiert',
  'app_passwords.never_used': 'Nie benutzt',
  'app_passwords.last_used': 'Zuletzt benutzt am {date}',

  // --- Perfil de configuración de Apple ---
  'profile.apple': 'Mail und Kalender auf Apple-Geräten einrichten',
  'profile.apple.hint': 'Lädt ein Konfigurationsprofil für iPhone, iPad und Mac.',

  // --- Alta de passkeys ---
  'security.passkeys.hint':
    'Melden Sie sich mit Fingerabdruck, Gesicht oder Sicherheitsschlüssel an, ohne Passwort.',
  'security.passkeys.add': 'Passkey hinzufügen',
  'security.passkeys.adding': 'Warten auf Ihr Gerät…',
  'security.passkeys.name': 'Namen vergeben',
  'security.passkeys.name.hint': 'Zum Beispiel: dieses Notebook, USB-Schlüssel.',
  'security.passkeys.duplicada': 'Dieser Schlüssel ist für dieses Konto schon registriert.',
  'security.passkeys.no_soportado': 'Dieser Browser unterstützt keine Passkeys.',
  'security.passkeys.error':
    'Der Passkey konnte nicht hinzugefügt werden. Versuchen Sie es erneut.',

  // --- Plan y almacenamiento (cuentas personales) ---
  'plan.title': 'Tarif und Speicher',
  'plan.loading': 'Tarif wird geladen…',
  'plan.free': 'Kostenloser Tarif',
  'plan.pro': 'Pro-Tarif · 200 GB',
  'plan.enterprise': 'Enterprise-Tarif · 1 TB · Premium-Support',
  'plan.storage': 'Belegter Speicher',
  'plan.paidUntil': 'Bezahlt bis',
  'plan.upgrade': 'Mehr Speicher',
  'plan.upgrade.hint':
    '200 GB für 5 €/Nutzer/Monat (Pro) oder 1 TB mit Premium-Support für 25 €/Nutzer/Monat (Enterprise), für E-Mails, Dateien, Fotos und Dokumente. Jederzeit kündbar.',
  'plan.upgrade.soon':
    'Die kostenpflichtigen Tarife kommen bald; sie können noch nicht gebucht werden.',
  'plan.unavailable': 'Zahlungen sind auf dieser Installation noch nicht verfügbar.',
  'plan.error': 'Der Tarif konnte nicht geladen werden. Versuche es gleich noch einmal.',

  // Acceso y recuperación (ADR 0041).
  'recovery.title': 'Anmeldung und Wiederherstellung',
  'recovery.hint':
    'Du kannst dich mit deinem @Benutzernamen, deiner Sytue-Adresse, deiner Handynummer oder deiner E-Mail zur Wiederherstellung anmelden, immer mit demselben Passwort. An die E-Mail zur Wiederherstellung geht auch der Link zum Zurücksetzen.',
  'recovery.phone': 'Handynummer',
  'recovery.phone.hint': 'Bei der Kontoerstellung per SMS bestätigt.',
  'recovery.email': 'E-Mail zur Wiederherstellung',
  'recovery.new': 'Neue E-Mail zur Wiederherstellung',
  'recovery.send': 'Bestätigungslink senden',
  'recovery.sent': 'Link gesendet. Öffne ihn in diesem Postfach, um die Adresse zu bestätigen.',
  'recovery.remove': 'Entfernen',
  'recovery.removed': 'E-Mail zur Wiederherstellung entfernt.',
  'recovery.invalid':
    'Diese Adresse geht nicht: Sie muss gültig sein und außerhalb von Sytue liegen.',
  'recovery.wrong_password': 'Das Passwort stimmt nicht.',
  'recovery.failed': 'Das konnte nicht gespeichert werden. Versuch es gleich noch einmal.',
  'delete.title': 'Mein Konto löschen',
  'delete.hint':
    'Ihre Anmeldedaten und alles, was nur Ihnen gehört, werden gelöscht; was Ihrer Organisation gehört, bleibt dort. 30 Tage lang lässt sich das Konto wiederherstellen.',
  'delete.link': 'Ansehen, was gelöscht wird, und fortfahren',
};
