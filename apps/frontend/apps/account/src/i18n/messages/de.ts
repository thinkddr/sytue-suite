/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  'app.title': 'Mein Konto',
  'app.description': 'Mein Konto — Profil, Sicherheit und Zugänge deines Sytue-Kontos.',
  'app.lead': 'Dein Profil, wie du dich anmeldest und das Konto schützt und wo es geöffnet ist.',

  // --- Revisión de seguridad ---
  'checkup.title': 'Sicherheitscheck',
  'checkup.all_good': 'Alles in Ordnung: Ihr Konto ist gut geschützt.',
  'checkup.dos_pasos':
    'Die Bestätigung in zwei Schritten ist aus: Ihr Passwort allein öffnet das Konto.',
  'checkup.recuperacion':
    'Keine Wiederherstellungs-E-Mail und keine Telefonnummer: Ohne Zugang gibt es keinen Weg zurück.',
  'checkup.sesiones': 'Offene Sitzungen auf anderen Geräten: {count}',
  'checkup.contrasenas_app': 'App-Passwörter seit 90 Tagen ungenutzt: {count}',
  'checkup.dispositivos': 'Vertrauenswürdige Geräte seit 90 Tagen nicht gesehen: {count}',
  'checkup.turn_on': 'Aktivieren',
  'checkup.add': 'Hinzufügen',
  'checkup.review': 'Prüfen',

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
  'sessions.signed_in': 'Angemeldet {time}',
  'sessions.close_one': 'Diese Sitzung beenden',
  'sessions.close_named': 'Sitzung auf {device} beenden',
  'sessions.close_one_confirm':
    '{device} wird abgemeldet und muss sich erneut anmelden, um dein Konto zu nutzen.',
  'sessions.close_others': 'Alle Sitzungen beenden',
  'sessions.close_others_note': 'Auch diese: Sie müssen sich erneut anmelden.',
  'sessions.close_all_confirm':
    'Alle Sitzungen werden beendet, auch diese. Du musst dich auf jedem Gerät erneut anmelden.',
  'sessions.empty': 'Keine weiteren Sitzungen offen.',
  'sessions.failed': 'Sitzungen konnten nicht geladen werden.',
  'common.loading': 'Wird geladen…',
  'common.retry': 'Erneut versuchen',

  // --- Dispositivos ---
  'devices.title': 'Geräte',
  'devices.current': 'Dieses Gerät',
  'devices.trusted': 'Vertrauenswürdig',
  'devices.trust': 'Als vertrauenswürdig markieren',
  'devices.forget': 'Dieses Gerät vergessen',
  'devices.forget_named': '{device} vergessen',
  'devices.forget_confirm':
    '{device} wird nicht mehr erkannt und verliert den Status als vertrauenswürdig. Eine erneute Anmeldung darüber gilt als neues Gerät.',
  'devices.empty': 'Noch keine Geräte erkannt.',
  'devices.failed': 'Geräte konnten nicht geladen werden.',
  'signature.title': 'Gespeicherte Unterschrift',
  'signature.help':
    'Die Unterschrift, die sich CertiPDF beim handschriftlichen Unterschreiben merken sollte. Sie wird verschlüsselt in deinem Konto gespeichert.',
  'signature.empty':
    'Du hast keine gespeicherte Unterschrift. Wenn du in CertiPDF von Hand unterschreibst, aktiviere «Meine Unterschrift merken».',
  'signature.failed': 'Deine gespeicherte Unterschrift konnte nicht geladen werden.',
  'signature.preview': 'Deine gespeicherte Unterschrift',
  'signature.saved_on': 'Gespeichert am {date}',
  'signature.delete': 'Gespeicherte Unterschrift löschen',
  'signature.delete_confirm':
    'Deine gespeicherte Unterschrift wird gelöscht. Bereits unterschriebene Dokumente behalten sie; beim nächsten handschriftlichen Unterschreiben musst du sie neu zeichnen.',

  // --- Seguridad ---
  'security.title': 'Sicherheit',
  'security.failed': 'Etwas ist schiefgelaufen. Versuchen Sie es erneut.',
  'security.password.title': 'Passwort',
  'security.password.warning': 'Eine Änderung meldet Sie überall ab, auch hier.',
  'security.password.current': 'Aktuelles Passwort',
  'security.password.new': 'Neues Passwort',
  'security.password.requirements': 'Anforderungen an das neue Passwort:',
  'security.password.req_length': {
    one: 'Mindestens {count} Zeichen',
    other: 'Mindestens {count} Zeichen',
  },
  'security.password.req_complexity': 'Buchstaben und mindestens eine Ziffer',
  'security.password.req_different': 'Anders als das aktuelle Passwort',
  'security.password.req_met': 'Erfüllt',
  'security.password.req_pending': 'Noch nicht',
  'security.password.submit': 'Passwort ändern',
  'security.password.done': 'Passwort geändert. Alle Sitzungen wurden beendet, auch diese.',
  'security.password.signin_again': 'Erneut anmelden',
  'security.mfa.title': 'Bestätigung in zwei Schritten',
  'security.mfa.off': 'Nicht eingerichtet. Ihr Passwort allein öffnet das Konto.',
  'security.mfa.required':
    'Für diese Aktion ist eine Bestätigung in zwei Schritten erforderlich. Richte sie unten ein und melde dich erneut an.',
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
  'security.codes.regenerate_confirm':
    'Deine aktuellen Wiederherstellungscodes werden sofort ungültig, auch ausgedruckte oder gespeicherte.',
  'security.mfa.step.need': 'Bevor du anfängst',
  'security.mfa.step.need.heading': 'Was du brauchst',
  'security.mfa.step.need.desc':
    'Eine Authentifizierungs-App auf deinem Handy. Sie zeigt alle 30 Sekunden einen neuen sechsstelligen Code.',
  'security.mfa.step.need.apps_title': 'Jede Authentifizierungs-App funktioniert',
  'security.mfa.step.need.apps':
    'Zum Beispiel: Aegis, FreeOTP, Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden. Wenn du noch keine hast, installiere eine, bevor du weitermachst.',
  'security.mfa.step.need.next': 'Los geht’s',
  'security.mfa.step.scan': 'Scannen',
  'security.mfa.step.scan.heading': 'Füge dein Konto zur App hinzu',
  'security.mfa.key_label': 'Einrichtungsschlüssel',
  'security.mfa.step.code': 'Code',
  'security.mfa.step.code.heading': 'Gib den Code aus der App ein',
  'security.mfa.step.code.desc':
    'So wird vor dem Aktivieren geprüft, dass App und Konto zusammenpassen.',
  'security.mfa.code.hint': 'Sechs Ziffern. Ändert er sich beim Tippen, nimm den neuen.',
  'security.mfa.step.codes.heading': 'Speichere deine Wiederherstellungscodes',
  'security.codes.copy': 'Alle kopieren',
  'security.codes.copied': 'Codes kopiert.',
  'security.codes.copy_failed':
    'Kopieren nicht möglich. Markiere die Codes und kopiere sie von Hand.',
  'security.codes.download': '.txt herunterladen',
  'security.codes.file_title': 'Sytue-Wiederherstellungscodes',
  'security.codes.file_account': 'Konto: {account}',
  'security.codes.file_date': 'Erstellt: {date}',
  'security.codes.file_note':
    'Jeder Code funktioniert nur einmal. Bewahre diese Datei sicher und getrennt von deinem Handy auf.',
  'security.mfa.step.done': 'Fertig',
  'security.mfa.step.done.heading': 'Die Bestätigung in zwei Schritten ist aktiv',
  'security.mfa.step.done.desc':
    'Bei der nächsten Anmeldung wirst du nach einem Code aus der App gefragt.',
  'security.mfa.passkey_tip.title': 'Füge auch einen Passkey hinzu',
  'security.mfa.passkey_tip':
    'Mit einem Passkey meldest du dich per Fingerabdruck, Gesicht oder Bildschirmsperre an, ohne Codes einzutippen.',
  'security.mfa.passkey_tip.action': 'Passkey hinzufügen',
  'security.mfa.close': 'Schließen',
  'security.mfa.disable_title': 'Bestätigung in zwei Schritten deaktivieren?',
  'security.mfa.disable_warning':
    'Dann reicht wieder dein Passwort allein, um das Konto zu öffnen.',
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
  'security.passkeys.remove_named': '{name} entfernen',
  'security.passkeys.remove_confirm': 'Du kannst dich nicht mehr mit {name} anmelden.',
  'security.passkeys.unnamed': 'Schlüssel ohne Namen',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': 'App-Passwörter',
  'app_passwords.hint':
    'Für Mail-Apps, die ein Passwort verlangen. Jedes gilt für sich und lässt sich einzeln widerrufen.',
  'app_passwords.label': 'Wofür ist es',
  'app_passwords.label.hint': 'Zum Beispiel: Mail am Laptop, Telefon.',
  'app_passwords.create': 'Passwort erstellen',
  'app_passwords.revoke': 'Widerrufen',
  'app_passwords.revoke_named': '{label} widerrufen',
  'app_passwords.revoke_confirm':
    'Apps, die {label} verwenden, können sich nicht mehr anmelden. Das lässt sich nicht rückgängig machen.',
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
  'plan.externalNote': 'Der verbundene Speicher erweitert deine Sytue-Kapazität.',
  'plan.externalCapacity': 'Der verbundene Speicher fügt {capacity} hinzu.',
  'plan.externalUnknown': 'Der Anbieter gibt seine Speicherkapazität nicht an.',
  'plan.baseUsage': 'Sytue: {used} von {total}',
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
  'recovery.password.hint':
    'Für jede Änderung auf dieser Karte brauchst du dein aktuelles Passwort.',
  'recovery.username.new': 'Neuer Benutzername',
  'recovery.username.hint':
    'Den bisherigen halten wir 30 Tage für dich frei, und du kannst ihn einmal alle 30 Tage ändern.',
  'recovery.username.change': 'Benutzernamen ändern',
  'recovery.username.changed': 'Benutzername geändert.',
  'recovery.username.invalid':
    'Dieser Benutzername ist ungültig: Verwende 3 bis 64 Buchstaben, Ziffern, Punkte oder Bindestriche.',
  'recovery.username.taken': 'Dieser Benutzername ist nicht verfügbar.',
  'recovery.username.recent': 'Du hast deinen Benutzernamen vor weniger als 30 Tagen geändert.',
  'recovery.username.is_address':
    'In einem persönlichen Konto ist dein Benutzername deine Sytue-Adresse und lässt sich hier nicht ändern.',
  'recovery.phone.none': 'Noch keine Handynummer: Füge eine hinzu, um dich auch damit anzumelden.',
  'recovery.phone.new': 'Neue Handynummer',
  'recovery.phone.format': 'Mit Ländervorwahl, zum Beispiel +49 151 0000000.',
  'recovery.phone.send': 'Code senden',
  'recovery.phone.sent': 'Wir haben einen Code per SMS an {phone} gesendet.',
  'recovery.phone.code': 'Code',
  'recovery.phone.confirm': 'Bestätigen',
  'recovery.phone.changed': 'Handynummer gespeichert.',
  'recovery.phone.invalid': 'Diese Nummer ist ungültig: Schreib sie mit Ländervorwahl.',
  'recovery.phone.same': 'Das ist schon deine Handynummer.',
  'recovery.phone.wait': 'Warte eine Minute, bevor du einen neuen Code anforderst.',
  'recovery.phone.limit': 'Zu viele Codes angefordert. Versuch es später noch einmal.',
  'recovery.phone.code_wrong': 'Der Code stimmt nicht.',
  'recovery.phone.code_expired': 'Der Code ist abgelaufen. Fordere einen neuen an.',
  'recovery.phone.has_account': 'Diese Handynummer gehört schon zu einem anderen Konto.',
  'recovery.phone.unavailable': 'SMS sind gerade nicht verfügbar. Versuch es später noch einmal.',
  'delete.title': 'Mein Konto löschen',
  'delete.hint':
    'Ihre Anmeldedaten und alles, was nur Ihnen gehört, werden gelöscht; was Ihrer Organisation gehört, bleibt dort. 30 Tage lang lässt sich das Konto wiederherstellen.',
  'delete.link': 'Ansehen, was gelöscht wird, und fortfahren',
};
