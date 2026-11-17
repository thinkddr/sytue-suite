/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const it: AppCatalog<typeof en> = {
  'app.title': 'Il mio account',
  'app.description': 'Il mio account — profilo, sicurezza e accessi del tuo account Sytue.',
  'app.lead': "Il tuo profilo, come accedi e proteggi l'account e dove è aperto.",

  // --- Revisión de seguridad ---
  'checkup.title': 'Controllo di sicurezza',
  'checkup.all_good': 'Tutto a posto: il tuo account è ben protetto.',
  'checkup.dos_pasos': 'La verifica in due passaggi è disattivata: basta la password per entrare.',
  'checkup.recuperacion':
    'Nessuna email né telefono di recupero: se perdi l’accesso non potrai rientrare.',
  'checkup.sesiones': 'Sessioni aperte su altri dispositivi: {count}',
  'checkup.contrasenas_app': 'Password per app inutilizzate da 90 giorni: {count}',
  'checkup.dispositivos': 'Dispositivi attendibili non visti da 90 giorni: {count}',
  'checkup.turn_on': 'Attiva',
  'checkup.add': 'Aggiungi',
  'checkup.review': 'Controlla',

  'profile.title': 'Profilo',
  'profile.name': 'Nome visualizzato',
  'profile.name.hint': 'Come appari in tutta la suite.',
  'profile.username': 'Nome utente',
  'profile.username.hint':
    'Le persone di altre organizzazioni su Sytue One lo usano per trovarti, scriverti o chiamarti.',
  'profile.email': 'Indirizzo e-mail',
  'profile.email.hint': 'Verificato. Modificarlo è una procedura a parte.',
  'profile.save': 'Salva modifiche',
  'profile.saving': 'Salvataggio…',
  'profile.saved': 'Modifiche salvate.',
  'profile.failed': 'Impossibile salvare le modifiche. Riprova.',
  'profile.avatarChange': 'Cambia avatar',
  'profile.avatarAdd': 'Aggiungi avatar',
  'profile.avatarPresets': 'Colori predefiniti',
  'profile.avatarUpload': 'Carica una foto…',
  'profile.avatarRemove': 'Rimuovi avatar',
  'photo.errorFormat': 'Formato non supportato. Usa JPEG, PNG, WebP o GIF.',
  'photo.errorProcess': 'Impossibile elaborare l’immagine.',
  'photo.errorTooLarge': 'L’immagine è troppo grande anche dopo l’ottimizzazione.',
  'photo.errorRead': 'Impossibile leggere l’immagine.',

  // --- Sesiones ---
  'sessions.title': 'Sessioni aperte',
  'sessions.current': 'Questo dispositivo',
  'sessions.signed_in': 'Accesso {time}',
  'sessions.close_one': 'Chiudi questa sessione',
  'sessions.close_named': 'Chiudi la sessione su {device}',
  'sessions.close_one_confirm':
    '{device} verrà disconnesso e dovrà accedere di nuovo per usare il tuo account.',
  'sessions.close_others': 'Chiudi tutte le sessioni',
  'sessions.close_others_note': 'Questa compresa: dovrai accedere di nuovo.',
  'sessions.close_all_confirm':
    'Verranno chiuse tutte le sessioni, compresa questa. Dovrai accedere di nuovo su ogni dispositivo.',
  'sessions.empty': 'Nessuna altra sessione aperta.',
  'sessions.failed': 'Impossibile caricare le sessioni.',
  'common.loading': 'Caricamento…',
  'common.retry': 'Riprova',

  // --- Dispositivos ---
  'devices.title': 'Dispositivi',
  'devices.current': 'Questo dispositivo',
  'devices.trusted': 'Attendibile',
  'devices.trust': 'Segna come attendibile',
  'devices.forget': 'Dimentica questo dispositivo',
  'devices.forget_named': 'Dimentica {device}',
  'devices.forget_confirm':
    '{device} non sarà più riconosciuto e perderà lo stato di dispositivo attendibile. Un nuovo accesso da lì varrà come nuovo dispositivo.',
  'devices.empty': 'Nessun dispositivo riconosciuto per ora.',
  'devices.failed': 'Impossibile caricare i dispositivi.',
  'signature.title': 'Firma salvata',
  'signature.help':
    'La firma che hai chiesto a CertiPDF di ricordare firmando a mano. È conservata cifrata nel tuo account.',
  'signature.empty':
    'Non hai nessuna firma salvata. Quando firmi a mano in CertiPDF, spunta «Ricorda la mia firma».',
  'signature.failed': 'Impossibile caricare la firma salvata.',
  'signature.preview': 'La tua firma salvata',
  'signature.saved_on': 'Salvata il {date}',
  'signature.delete': 'Elimina la firma salvata',
  'signature.delete_confirm':
    'La tua firma salvata verrà eliminata. I documenti già firmati la mantengono; la prossima volta che firmi a mano dovrai disegnarla di nuovo.',

  // --- Seguridad ---
  'security.title': 'Sicurezza',
  'security.failed': 'Qualcosa è andato storto. Riprova.',
  'security.password.title': 'Password',
  'security.password.warning': 'Cambiarla chiude la sessione ovunque, anche qui.',
  'security.password.current': 'Password attuale',
  'security.password.new': 'Nuova password',
  'security.password.requirements': 'Requisiti della nuova password:',
  'security.password.req_length': {
    one: 'Almeno {count} carattere',
    other: 'Almeno {count} caratteri',
  },
  'security.password.req_complexity': 'Lettere e almeno un numero',
  'security.password.req_different': 'Diversa dalla password attuale',
  'security.password.req_met': 'Soddisfatto',
  'security.password.req_pending': 'Non ancora',
  'security.password.submit': 'Cambia la password',
  'security.password.done':
    'Password cambiata. Tutte le sessioni sono state chiuse, questa compresa.',
  'security.password.signin_again': 'Accedi di nuovo',
  'security.mfa.title': 'Verifica in due passaggi',
  'security.mfa.off': 'Non configurata. La sola password apre l’account.',
  'security.mfa.required':
    'Questa operazione richiede la verifica in due passaggi. Configurala qui sotto, poi accedi di nuovo.',
  'security.mfa.on': 'Attiva. All’accesso ti viene chiesto un codice.',
  'security.mfa.enable': 'Configura',
  'security.mfa.scan': 'Inquadra questo con la tua app di autenticazione.',
  'security.mfa.qr_alt': 'Codice QR per configurare la verifica in due passaggi',
  'security.mfa.manual': 'Oppure inserisci questa chiave a mano:',
  'security.mfa.code': 'Codice dell’app',
  'security.mfa.confirm': 'Attiva',
  'security.mfa.disable': 'Disattiva',
  'security.mfa.disable_hint': 'Per disattivarla, inserisci un codice valido o la tua password.',
  'security.mfa.code_or_password': 'Codice dell’app',
  'security.mfa.new_codes': 'Nuovi codici di recupero',
  'security.codes.title': 'Codici di recupero',
  'security.codes.once': 'Mostrati una sola volta. Conservali al sicuro: ognuno vale una volta.',
  'security.codes.saved': 'Li ho salvati',
  'security.codes.regenerate_confirm':
    'I codici di recupero attuali smetteranno subito di funzionare, compresi quelli stampati o salvati.',
  'security.mfa.step.need': 'Prima di iniziare',
  'security.mfa.step.need.heading': 'Cosa ti serve',
  'security.mfa.step.need.desc':
    'Un’app di autenticazione sul telefono. Mostra un nuovo codice di sei cifre ogni 30 secondi.',
  'security.mfa.step.need.apps_title': 'Va bene qualsiasi app di autenticazione',
  'security.mfa.step.need.apps':
    'Per esempio: Aegis, FreeOTP, Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden. Se non ne hai una, installala prima di continuare.',
  'security.mfa.step.need.next': 'Inizia',
  'security.mfa.step.scan': 'Scansiona',
  'security.mfa.step.scan.heading': 'Aggiungi il tuo account all’app',
  'security.mfa.key_label': 'Chiave di configurazione',
  'security.mfa.step.code': 'Codice',
  'security.mfa.step.code.heading': 'Scrivi il codice dell’app',
  'security.mfa.step.code.desc':
    'Serve a verificare che app e account corrispondano prima di attivarla.',
  'security.mfa.code.hint': 'Sei cifre. Se cambia mentre scrivi, usa quello nuovo.',
  'security.mfa.step.codes.heading': 'Salva i codici di recupero',
  'security.codes.copy': 'Copia tutti',
  'security.codes.copied': 'Codici copiati.',
  'security.codes.copy_failed': 'Impossibile copiare. Seleziona i codici e copiali a mano.',
  'security.codes.download': 'Scarica .txt',
  'security.codes.file_title': 'Codici di recupero Sytue',
  'security.codes.file_account': 'Account: {account}',
  'security.codes.file_date': 'Generati: {date}',
  'security.codes.file_note':
    'Ogni codice vale una sola volta. Conserva questo file in un luogo sicuro, lontano dal telefono.',
  'security.mfa.step.done': 'Fatto',
  'security.mfa.step.done.heading': 'La verifica in due passaggi è attiva',
  'security.mfa.step.done.desc': 'Al prossimo accesso ti verrà chiesto un codice dell’app.',
  'security.mfa.passkey_tip.title': 'Aggiungi anche una passkey',
  'security.mfa.passkey_tip':
    'Una passkey ti fa entrare con impronta, volto o blocco schermo, senza digitare codici.',
  'security.mfa.passkey_tip.action': 'Aggiungi passkey',
  'security.mfa.close': 'Chiudi',
  'security.mfa.disable_title': 'Disattivare la verifica in due passaggi?',
  'security.mfa.disable_warning': 'Basterà di nuovo la sola password per aprire l’account.',
  'common.cancel': 'Annulla',

  // --- Perfil: el resto de la ficha ---
  'profile.given_name': 'Nome',
  'profile.family_name': 'Cognome',
  'profile.job_title': 'Ruolo',
  'profile.department': 'Reparto',
  'profile.location': 'Sede',
  'profile.locale': 'Lingua',
  'profile.locale.hint': 'Vale in tutta la suite e nei messaggi che ti inviamo.',

  // --- Passkeys ---
  'security.passkeys.title': 'Passkey',
  'security.passkeys.remove': 'Rimuovi',
  'security.passkeys.remove_named': 'Rimuovi {name}',
  'security.passkeys.remove_confirm': 'Non potrai più accedere con {name}.',
  'security.passkeys.unnamed': 'Chiave senza nome',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': 'Password per le app',
  'app_passwords.hint':
    'Per le app di posta che chiedono una password. Ognuna vale da sola e si revoca senza toccare le altre.',
  'app_passwords.label': 'A cosa serve',
  'app_passwords.label.hint': 'Per esempio: posta del portatile, telefono.',
  'app_passwords.create': 'Crea password',
  'app_passwords.revoke': 'Revoca',
  'app_passwords.revoke_named': 'Revoca {label}',
  'app_passwords.revoke_confirm':
    'Le app che usano {label} non potranno più accedere. Non si può annullare.',
  'app_passwords.empty': 'Ancora nessuna password per le app.',
  'app_passwords.failed': 'Impossibile caricare le password per le app.',
  'app_passwords.unnamed': 'Senza nome',
  'app_passwords.once': 'Mostrata una sola volta. Copiala subito nell’app: non sarà più visibile.',
  'app_passwords.saved': 'L’ho copiata',
  'app_passwords.never_used': 'Mai usata',
  'app_passwords.last_used': 'Usata l’ultima volta il {date}',

  // --- Perfil de configuración de Apple ---
  'profile.apple': 'Configura Mail e Calendario sui dispositivi Apple',
  'profile.apple.hint': 'Scarica un profilo di configurazione per iPhone, iPad e Mac.',

  // --- Alta de passkeys ---
  'security.passkeys.hint':
    'Accedi con impronta, volto o una chiave di sicurezza, senza scrivere la password.',
  'security.passkeys.add': 'Aggiungi passkey',
  'security.passkeys.adding': 'In attesa del tuo dispositivo…',
  'security.passkeys.name': 'Dalle un nome',
  'security.passkeys.name.hint': 'Per esempio: questo portatile, chiave USB.',
  'security.passkeys.duplicada': 'Quella chiave è già registrata su questo account.',
  'security.passkeys.no_soportado': 'Questo browser non supporta le passkey.',
  'security.passkeys.error': 'Non è stato possibile aggiungere la passkey. Riprova.',

  // --- Plan y almacenamiento (cuentas personales) ---
  'plan.title': 'Piano e spazio',
  'plan.loading': 'Caricamento del piano…',
  'plan.free': 'Piano gratuito',
  'plan.pro': 'Piano Pro · 200 GB',
  'plan.enterprise': 'Piano Enterprise · 1 TB · Supporto Premium',
  'plan.storage': 'Spazio usato',
  'plan.externalNote': 'Lo spazio di archiviazione collegato aumenta la tua capacità Sytue.',
  'plan.externalCapacity': 'Lo spazio di archiviazione collegato aggiunge {capacity}.',
  'plan.externalUnknown': 'Il fornitore non comunica la sua capacità.',
  'plan.baseUsage': 'Sytue: {used} di {total}',
  'plan.paidUntil': 'Pagato fino al',
  'plan.upgrade': 'Più spazio',
  'plan.upgrade.hint':
    '200 GB a 5 €/Utente/Mese (Pro) o 1 TB con Supporto Premium a 25 €/Utente/Mese (Enterprise), per posta, file, foto e documenti. Disdici quando vuoi.',
  'plan.upgrade.soon': 'I piani a pagamento arriveranno presto; per ora non si possono acquistare.',
  'plan.unavailable': 'I pagamenti non sono ancora disponibili su questa installazione.',
  'plan.error': 'Non è stato possibile caricare il piano. Riprova tra un momento.',

  // Acceso y recuperación (ADR 0041).
  'recovery.title': 'Accesso e recupero',
  'recovery.hint':
    'Puoi accedere con il tuo @nome utente, il tuo indirizzo Sytue, il tuo cellulare o la tua e-mail di recupero, sempre con la stessa password. L’e-mail di recupero riceve anche il link per reimpostarla.',
  'recovery.phone': 'Numero di cellulare',
  'recovery.phone.hint': 'Verificato via SMS alla creazione dell’account.',
  'recovery.email': 'E-mail di recupero',
  'recovery.new': 'Nuova e-mail di recupero',
  'recovery.send': 'Invia link di conferma',
  'recovery.sent': 'Link inviato. Aprilo da quell’indirizzo per confermarlo.',
  'recovery.remove': 'Rimuovi',
  'recovery.removed': 'E-mail di recupero rimossa.',
  'recovery.invalid': 'Questo indirizzo non va bene: deve essere valido ed esterno a Sytue.',
  'recovery.wrong_password': 'La password non è corretta.',
  'recovery.failed': 'Non è stato possibile salvare. Riprova tra poco.',
  'delete.title': 'Elimina il mio account',
  'delete.hint':
    'Vengono eliminate le tue credenziali e tutto ciò che è solo tuo; ciò che appartiene alla tua organizzazione resta lì. Per 30 giorni si può recuperare.',
  'delete.link': 'Vedi cosa viene eliminato e continua',
};
