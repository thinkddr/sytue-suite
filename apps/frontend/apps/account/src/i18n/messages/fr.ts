/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fr: AppCatalog<typeof en> = {
  'app.title': 'Mon compte',
  'app.description': 'Mon compte — profil, sécurité et accès de votre compte Sytue.',
  'app.lead':
    'Votre profil, la façon dont vous vous connectez et protégez le compte, et où il est ouvert.',

  // --- Revisión de seguridad ---
  'checkup.title': 'Contrôle de sécurité',
  'checkup.all_good': 'Tout va bien : votre compte est bien protégé.',
  'checkup.dos_pasos':
    'La validation en deux étapes est désactivée : votre mot de passe suffit pour entrer.',
  'checkup.recuperacion':
    'Aucun e-mail ni téléphone de récupération : si vous perdez l’accès, impossible de revenir.',
  'checkup.sesiones': 'Sessions ouvertes sur d’autres appareils : {count}',
  'checkup.contrasenas_app': 'Mots de passe d’application inutilisés depuis 90 jours : {count}',
  'checkup.dispositivos': 'Appareils de confiance non vus depuis 90 jours : {count}',
  'checkup.turn_on': 'Activer',
  'checkup.add': 'Ajouter',
  'checkup.review': 'Vérifier',

  'profile.title': 'Profil',
  'profile.name': 'Nom affiché',
  'profile.name.hint': 'Votre apparence dans toute la suite.',
  'profile.username': 'Nom d’utilisateur',
  'profile.username.hint':
    'Les personnes d’autres organisations sur Sytue One l’utilisent pour vous trouver, vous écrire ou vous appeler.',
  'profile.email': 'Adresse e-mail',
  'profile.email.hint': 'Vérifiée. La modifier est une autre démarche.',
  'profile.save': 'Enregistrer',
  'profile.saving': 'Enregistrement…',
  'profile.saved': 'Modifications enregistrées.',
  'profile.failed': "Impossible d'enregistrer. Réessayez.",
  'profile.avatarChange': "Changer l'avatar",
  'profile.avatarAdd': 'Ajouter un avatar',
  'profile.avatarPresets': 'Couleurs prédéfinies',
  'profile.avatarUpload': 'Importer une photo…',
  'profile.avatarRemove': "Supprimer l'avatar",
  'photo.errorFormat': 'Format non pris en charge. Utilisez JPEG, PNG, WebP ou GIF.',
  'photo.errorProcess': 'Impossible de traiter l’image.',
  'photo.errorTooLarge': 'L’image est trop lourde, même après optimisation.',
  'photo.errorRead': 'Impossible de lire l’image.',

  // --- Sesiones ---
  'sessions.title': 'Sessions ouvertes',
  'sessions.current': 'Cet appareil',
  'sessions.signed_in': 'Connecté {time}',
  'sessions.close_one': 'Fermer cette session',
  'sessions.close_named': 'Fermer la session sur {device}',
  'sessions.close_one_confirm':
    '{device} sera déconnecté et devra se reconnecter pour accéder à votre compte.',
  'sessions.close_others': 'Fermer toutes les sessions',
  'sessions.close_others_note': 'Celle-ci comprise : vous devrez vous reconnecter.',
  'sessions.close_all_confirm':
    'Toutes les sessions seront fermées, y compris celle-ci. Vous devrez vous reconnecter sur chaque appareil.',
  'sessions.empty': 'Aucune autre session ouverte.',
  'sessions.failed': 'Impossible de charger les sessions.',
  'common.loading': 'Chargement…',
  'common.retry': 'Réessayer',

  // --- Dispositivos ---
  'devices.title': 'Appareils',
  'devices.current': 'Cet appareil',
  'devices.trusted': 'De confiance',
  'devices.trust': 'Marquer comme fiable',
  'devices.forget': 'Oublier cet appareil',
  'devices.forget_named': 'Oublier {device}',
  'devices.forget_confirm':
    '{device} ne sera plus reconnu et perdra son statut de confiance. Une nouvelle connexion depuis cet appareil le traitera comme un nouvel appareil.',
  'devices.empty': 'Aucun appareil reconnu pour le moment.',
  'devices.failed': 'Impossible de charger les appareils.',
  'signature.title': 'Signature enregistrée',
  'signature.help':
    'La signature que vous avez demandé à CertiPDF de mémoriser en signant à la main. Elle est stockée chiffrée dans votre compte.',
  'signature.empty':
    'Vous n’avez aucune signature enregistrée. En signant à la main dans CertiPDF, cochez «Mémoriser ma signature».',
  'signature.failed': 'Impossible de charger votre signature enregistrée.',
  'signature.preview': 'Votre signature enregistrée',
  'signature.saved_on': 'Enregistrée le {date}',
  'signature.delete': 'Supprimer la signature enregistrée',
  'signature.delete_confirm':
    'Votre signature enregistrée sera supprimée. Les documents déjà signés la conservent ; la prochaine fois que vous signerez à la main, il faudra la redessiner.',

  // --- Seguridad ---
  'security.title': 'Sécurité',
  'security.failed': 'Une erreur est survenue. Réessayez.',
  'security.password.title': 'Mot de passe',
  'security.password.warning': 'Le changer ferme la session partout, ici compris.',
  'security.password.current': 'Mot de passe actuel',
  'security.password.new': 'Nouveau mot de passe',
  'security.password.requirements': 'Exigences pour le nouveau mot de passe :',
  'security.password.req_length': {
    one: 'Au moins {count} caractère',
    other: 'Au moins {count} caractères',
  },
  'security.password.req_complexity': 'Des lettres et au moins un chiffre',
  'security.password.req_different': 'Différent du mot de passe actuel',
  'security.password.req_met': 'Respecté',
  'security.password.req_pending': 'Pas encore',
  'security.password.submit': 'Changer le mot de passe',
  'security.password.done':
    'Mot de passe changé. Toutes les sessions ont été fermées, celle-ci comprise.',
  'security.password.signin_again': 'Se reconnecter',
  'security.mfa.title': 'Validation en deux étapes',
  'security.mfa.off': 'Non configurée. Votre mot de passe seul ouvre le compte.',
  'security.mfa.required':
    'Cette opération nécessite la validation en deux étapes. Configurez-la ci-dessous, puis reconnectez-vous.',
  'security.mfa.on': 'Activée. Un code vous est demandé à la connexion.',
  'security.mfa.enable': 'Configurer',
  'security.mfa.scan': "Scannez ceci avec votre application d'authentification.",
  'security.mfa.qr_alt': 'Code QR de configuration de la validation en deux étapes',
  'security.mfa.manual': 'Ou saisissez cette clé à la main :',
  'security.mfa.code': "Code de l'application",
  'security.mfa.confirm': 'Activer',
  'security.mfa.disable': 'Désactiver',
  'security.mfa.disable_hint':
    'Pour la désactiver, saisissez un code valide ou votre mot de passe.',
  'security.mfa.code_or_password': "Code de l'application",
  'security.mfa.new_codes': 'Nouveaux codes de secours',
  'security.codes.title': 'Codes de secours',
  'security.codes.once':
    'Affichés une seule fois. Gardez-les en lieu sûr : chacun ne sert qu’une fois.',
  'security.codes.saved': 'Je les ai enregistrés',
  'security.codes.regenerate_confirm':
    'Vos codes de secours actuels cesseront de fonctionner immédiatement, y compris ceux que vous avez imprimés ou enregistrés.',
  'security.mfa.step.need': 'Avant de commencer',
  'security.mfa.step.need.heading': 'Ce dont vous avez besoin',
  'security.mfa.step.need.desc':
    'Une application d’authentification sur votre téléphone. Elle affiche un nouveau code à six chiffres toutes les 30 secondes.',
  'security.mfa.step.need.apps_title': 'Toute application d’authentification convient',
  'security.mfa.step.need.apps':
    'Par exemple : Aegis, FreeOTP, Microsoft Authenticator, Google Authenticator, 1Password, Bitwarden. Si vous n’en avez pas, installez-en une avant de continuer.',
  'security.mfa.step.need.next': 'Commencer',
  'security.mfa.step.scan': 'Scanner',
  'security.mfa.step.scan.heading': 'Ajoutez votre compte à l’application',
  'security.mfa.key_label': 'Clé de configuration',
  'security.mfa.step.code': 'Code',
  'security.mfa.step.code.heading': 'Saisissez le code de l’application',
  'security.mfa.step.code.desc':
    'Cela vérifie que l’application et votre compte concordent avant l’activation.',
  'security.mfa.code.hint': 'Six chiffres. S’il change pendant la saisie, utilisez le nouveau.',
  'security.mfa.step.codes.heading': 'Enregistrez vos codes de secours',
  'security.codes.copy': 'Tout copier',
  'security.codes.copied': 'Codes copiés.',
  'security.codes.copy_failed': 'Copie impossible. Sélectionnez les codes et copiez-les à la main.',
  'security.codes.download': 'Télécharger le .txt',
  'security.codes.file_title': 'Codes de secours Sytue',
  'security.codes.file_account': 'Compte : {account}',
  'security.codes.file_date': 'Créés le : {date}',
  'security.codes.file_note':
    'Chaque code ne sert qu’une fois. Conservez ce fichier en lieu sûr, loin de votre téléphone.',
  'security.mfa.step.done': 'Terminé',
  'security.mfa.step.done.heading': 'La validation en deux étapes est activée',
  'security.mfa.step.done.desc':
    'À votre prochaine connexion, un code de l’application vous sera demandé.',
  'security.mfa.passkey_tip.title': 'Ajoutez aussi une clé d’accès',
  'security.mfa.passkey_tip':
    'Une clé d’accès permet de vous connecter avec votre empreinte, votre visage ou le verrouillage de l’écran, sans saisir de code.',
  'security.mfa.passkey_tip.action': 'Ajouter une clé d’accès',
  'security.mfa.close': 'Fermer',
  'security.mfa.disable_title': 'Désactiver la validation en deux étapes ?',
  'security.mfa.disable_warning': 'Votre mot de passe suffira de nouveau pour ouvrir le compte.',
  'common.cancel': 'Annuler',

  // --- Perfil: el resto de la ficha ---
  'profile.given_name': 'Prénom',
  'profile.family_name': 'Nom',
  'profile.job_title': 'Fonction',
  'profile.department': 'Service',
  'profile.location': 'Lieu',
  'profile.locale': 'Langue',
  'profile.locale.hint':
    'Utilisée dans toute la suite et dans les messages que nous vous envoyons.',

  // --- Passkeys ---
  'security.passkeys.title': "Clés d'accès",
  'security.passkeys.remove': 'Retirer',
  'security.passkeys.remove_named': 'Retirer {name}',
  'security.passkeys.remove_confirm': 'Vous ne pourrez plus vous connecter avec {name}.',
  'security.passkeys.unnamed': 'Clé sans nom',

  // --- Contraseñas de aplicación ---
  'app_passwords.title': "Mots de passe d'application",
  'app_passwords.hint':
    'Pour les applications de messagerie qui demandent un mot de passe. Chacun vaut seul et se révoque sans toucher aux autres.',
  'app_passwords.label': 'À quoi sert-il',
  'app_passwords.label.hint': 'Par exemple : messagerie du portable, téléphone.',
  'app_passwords.create': 'Créer un mot de passe',
  'app_passwords.revoke': 'Révoquer',
  'app_passwords.revoke_named': 'Révoquer {label}',
  'app_passwords.revoke_confirm':
    'Les applications qui utilisent {label} ne pourront plus se connecter. Cette action est irréversible.',
  'app_passwords.empty': "Aucun mot de passe d'application pour le moment.",
  'app_passwords.failed': "Impossible de charger les mots de passe d'application.",
  'app_passwords.unnamed': 'Sans nom',
  'app_passwords.once': 'Affiché une seule fois. Copiez-le maintenant : il ne sera plus visible.',
  'app_passwords.saved': "Je l'ai copié",
  'app_passwords.never_used': 'Jamais utilisé',
  'app_passwords.last_used': 'Dernière utilisation le {date}',

  // --- Perfil de configuración de Apple ---
  'profile.apple': 'Configurer Mail et Calendrier sur les appareils Apple',
  'profile.apple.hint': 'Télécharge un profil de configuration pour iPhone, iPad et Mac.',

  // --- Alta de passkeys ---
  'security.passkeys.hint':
    'Connectez-vous avec votre empreinte, votre visage ou une clé de sécurité, sans mot de passe.',
  'security.passkeys.add': "Ajouter une clé d'accès",
  'security.passkeys.adding': 'En attente de votre appareil…',
  'security.passkeys.name': 'Donnez-lui un nom',
  'security.passkeys.name.hint': 'Par exemple : ce portable, clé USB.',
  'security.passkeys.duplicada': 'Cette clé est déjà enregistrée sur ce compte.',
  'security.passkeys.no_soportado': "Ce navigateur ne prend pas en charge les clés d'accès.",
  'security.passkeys.error': "La clé n'a pas pu être ajoutée. Réessayez.",

  // --- Plan y almacenamiento (cuentas personales) ---
  'plan.title': 'Forfait et stockage',
  'plan.loading': 'Chargement de votre forfait…',
  'plan.free': 'Forfait gratuit',
  'plan.pro': 'Forfait Pro · 200 Go',
  'plan.enterprise': 'Forfait Enterprise · 1 To · Support Premium',
  'plan.storage': 'Espace utilisé',
  'plan.externalNote': 'Le stockage connecté augmente votre capacité Sytue.',
  'plan.externalCapacity': 'Le stockage connecté ajoute {capacity}.',
  'plan.externalUnknown': 'Le fournisseur ne communique pas sa capacité.',
  'plan.baseUsage': 'Sytue: {used} sur {total}',
  'plan.paidUntil': 'Payé jusqu’au',
  'plan.upgrade': 'Plus d’espace',
  'plan.upgrade.hint':
    '200 Go pour 5 €/Utilisateur/Mois (Pro) ou 1 To avec Support Premium pour 25 €/Utilisateur/Mois (Enterprise), pour vos e-mails, fichiers, photos et documents. Résiliable à tout moment.',
  'plan.upgrade.soon':
    'Les forfaits payants arrivent bientôt ; ils ne sont pas encore disponibles à l’achat.',
  'plan.unavailable': 'Les paiements ne sont pas encore disponibles sur cette installation.',
  'plan.error': 'Impossible de charger votre forfait. Réessayez dans un instant.',

  // Acceso y recuperación (ADR 0041).
  'recovery.title': 'Connexion et récupération',
  'recovery.hint':
    'Vous pouvez vous connecter avec votre @nom d’utilisateur, votre adresse Sytue, votre numéro de mobile ou votre e-mail de récupération, toujours avec le même mot de passe. L’e-mail de récupération reçoit aussi le lien pour le réinitialiser.',
  'recovery.phone': 'Numéro de mobile',
  'recovery.phone.hint': 'Vérifié par SMS à la création du compte.',
  'recovery.email': 'E-mail de récupération',
  'recovery.new': 'Nouvel e-mail de récupération',
  'recovery.send': 'Envoyer le lien de confirmation',
  'recovery.sent': 'Lien envoyé. Ouvrez-le depuis cette adresse pour la confirmer.',
  'recovery.remove': 'Retirer',
  'recovery.removed': 'E-mail de récupération retiré.',
  'recovery.invalid':
    'Cette adresse ne convient pas : elle doit être valide et extérieure à Sytue.',
  'recovery.wrong_password': 'Le mot de passe n’est pas correct.',
  'recovery.failed': 'Impossible d’enregistrer. Réessayez dans un instant.',
  'delete.title': 'Supprimer mon compte',
  'delete.hint':
    'Vos identifiants et tout ce qui n’appartient qu’à vous sont supprimés ; ce qui appartient à votre organisation y reste. Le compte peut être récupéré pendant 30 jours.',
  'delete.link': 'Voir ce qui est supprimé et continuer',
};
