/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo FRANCÉS del portal Sytue One. Traducción del original inglés
 * (`en.ts`), que manda: el tipo exige aquí exactamente sus mismas claves.
 *
 * Tratamiento de cortesía (vous) en todo el catálogo, como el resto de la suite.
 * Terminología: «Boîte aux lettres», «Domaine», «Enregistrement» (registro DNS)
 * y «Agenda» para el calendario personal —Calendar con mayúscula es la marca—.
 * Apóstrofo tipográfico (’) siempre: es lo correcto en francés y además evita
 * escapes dentro de las comillas simples de JS.
 *
 * OJO al plural: en francés el 0 va en SINGULAR, y eso lo decide
 * `Intl.PluralRules` («0 résultat»); por eso `one` no dice «1» sino «{count}».
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const fr: AppCatalog<typeof en> = {
  // --- Títulos de página -------------------------------------------------
  'page.home': 'Sytue One — Accueil',
  'page.search': 'Rechercher dans Sytue One',
  'page.searchQuery': '« {query} » — Rechercher dans Sytue One',
  'page.devices': 'Connectez vos appareils — Sytue One',
  'page.data': 'Vos données — Sytue One',
  'page.downloads': 'Téléchargements — Sytue One',
  'downloads.title': 'Téléchargez les applications',
  'downloads.lead':
    'Des applications natives pour votre machine. Elles s’installent, vivent dans le Dock ou le menu Démarrer et vous alertent via les notifications du système.',
  'downloads.version': 'Version {version}',
  'downloads.get': 'Télécharger',
  'downloads.forYou': 'Pour votre {platform}',
  'downloads.others': 'Autres plateformes',
  'downloads.all': 'Toutes les plateformes',
  'downloads.empty': 'Aucun installeur publié pour l’instant',
  'downloads.emptyNoRelease':
    'Aucune version n’a encore publié ses installeurs. Ils apparaîtront ici dès que ce sera le cas ; rien à faire de votre côté.',
  'downloads.emptyUnconfigured':
    'Ce déploiement ne sait pas où chercher les installeurs. Un opérateur le configure.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'Impossible de charger les téléchargements',
  'downloads.failedBody': 'Le Hub ne répond pas pour le moment. Réessayez dans quelques secondes.',
  'downloads.savedTitle': 'Téléchargement terminé',
  'downloads.saveFailedTitle': 'Impossible de télécharger l’installeur',
  'page.activity': 'Activité récente — Sytue One',
  'page.welcome': 'Bienvenue chez Sytue',
  // --- Activité récente (Sytue Hub) --------------------------------------
  'activity.overline': 'Activité',
  'activity.heading': 'Activité récente',
  'activity.intro':
    'Ce qui s’est passé récemment dans vos e-mails, fichiers, documents, événements et salons.',
  'activity.today': 'Aujourd’hui',
  'activity.yesterday': 'Hier',
  'activity.empty':
    'Rien de récent pour l’instant. Au fil de votre utilisation de Sytue, ce que vous touchez apparaîtra ici.',
  'activity.failedTitle': 'Impossible de charger l’activité',
  'activity.failedBody': 'Le Hub ne répond pas pour le moment. Réessayez dans quelques secondes.',
  'activity.retry': 'Réessayer',
  'activity.all': 'Tout',
  'activity.filterBy': 'Filtrer par produit',
  'activity.count': { one: '{count} élément', other: '{count} éléments' },
  'page.inbox': 'Notifications — Sytue One',
  // --- Boîte de notifications (Sytue Hub) --------------------------------
  'inbox.overline': 'Notifications',
  'inbox.heading': 'Notifications',
  'inbox.intro': 'Tout ce que la suite a à vous dire, au même endroit.',
  'inbox.all': 'Toutes',
  'inbox.unreadFilter': { one: '{count} non lue', other: '{count} non lues' },
  'inbox.filterBy': 'Filtrer les notifications',
  'inbox.markAll': 'Tout marquer comme lu',
  'inbox.loading': 'Chargement des notifications…',
  'inbox.unreadDot': 'Non lue',
  'inbox.errorTitle': 'Impossible de charger les notifications',
  'inbox.errorBody': 'Le Hub ne répond pas pour le moment. Réessayez dans quelques secondes.',
  'inbox.retry': 'Réessayer',
  'inbox.emptyTitle': 'Rien à lire',
  'inbox.emptyBody': 'Dès que quelque chose demande votre attention, cela apparaît ici.',
  'inbox.emptyUnreadTitle': 'Vous êtes à jour',
  'inbox.emptyUnreadBody': 'Vous avez tout lu. Passez à « Toutes » pour revoir les précédentes.',
  'inbox.prefsTitle': 'Ce que vous souhaitez recevoir',
  'inbox.prefsBody':
    'Si vous désactivez un produit, il cesse de vous avertir ici et par e-mail. Vous pouvez le réactiver quand vous voulez.',

  // --- Lanzador ----------------------------------------------------------
  'launcher.heading': 'Vos applications',
  'launcher.greeting.morning': 'Bonjour',
  'launcher.greeting.afternoon': 'Bon après-midi',
  'launcher.greeting.evening': 'Bonsoir',
  'launcher.more': 'Plus',
  'launcher.apps': 'Applications',
  'launcher.connected': 'Connecté',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Désactivé',

  'product.mail': 'Messagerie',
  'product.drive': 'Fichiers et collaboration',
  'product.calendar': 'Agenda',
  'product.contacts': 'Contacts',
  'product.docs': 'Documents, feuilles de calcul et présentations',
  'product.rooms': 'Canaux, messages et appels',
  'product.cloud': 'Plateforme applicative (PaaS)',
  'product.console': 'Administration et organisations',
  'product.id': 'Identité, profil et sécurité',

  'quick.heading': 'Actions rapides',
  'quick.search': 'Chercher dans toute la suite',
  'quick.compose': 'Écrire un e-mail',
  'quick.newDoc': 'Nouveau document',
  'quick.upload': 'Importer dans Drive',
  'quick.newEvent': 'Créer un événement',
  'quick.download': 'Télécharger l’application',
  'quick.devices': 'Connectez vos appareils',
  'quick.data': 'Vos données',

  'storage.connectTitle': 'Connecter un stockage externe',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV et plus encore.',

  'recent.heading': 'Activité récente dans toute la suite',
  'recent.viewAll': 'Tout afficher',
  'recent.loading': 'Chargement de votre activité récente…',
  'recent.errorTitle': 'Impossible de charger votre activité',
  'recent.errorBody':
    'Le Hub ne répond pas pour le moment. Votre travail est en sécurité ; réessayez.',
  'recent.retry': 'Réessayer',
  'recent.emptyTitle': 'Aucune activité récente',
  'recent.emptyBody':
    'Dès que vous travaillerez dans Mail, Drive, Calendar ou Rooms, cela apparaîtra ici.',
  'recent.emptyCta': 'Commencer dans Drive',

  // --- Búsqueda cross-app ------------------------------------------------
  'search.overline': 'Recherche',
  'search.heading': 'Cherchez dans toute la suite',
  'search.placeholder': 'Cherchez dans Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Rechercher dans Sytue One',
  'search.submit': 'Rechercher',
  'search.hint':
    'Écrivez ci-dessus pour chercher d’un coup dans vos e-mails, vos fichiers, vos documents, vos événements, vos contacts et vos salons. Les résultats s’ouvrent directement dans leur produit.',
  'search.failedTitle': 'La recherche n’a pas abouti',
  'search.failedBody': 'Le Hub ne répond pas pour le moment. Réessayez dans quelques secondes.',
  'search.retry': 'Réessayer',
  'search.empty': 'Aucun résultat pour « {query} ». Essayez d’autres mots.',
  'search.count': { one: '{count} résultat', other: '{count} résultats' },

  // --- Bienvenida --------------------------------------------------------
  'welcome.heading': 'Bienvenue chez Sytue',
  'welcome.lead':
    'Quelques minutes et tout sera opérationnel. Nous avançons étape par étape ; si vous vous arrêtez en chemin, nous reprenons là où vous en étiez à votre retour.',

  'data.title': 'Vos données',
  'data.lead':
    'Téléchargez une copie de tout ce que vous avez dans Sytue One, quand vous le souhaitez et sans rien demander à personne.',
  'data.export.section': 'Télécharger une copie',
  'data.export.body':
    'Un seul fichier ZIP avec votre messagerie, vos fichiers, agendas, contacts, notes, documents, photos et messages. Ce qui a déjà un format standard est fourni tel quel : les e-mails en .eml, les agendas en .ics, les contacts en .vcf et les fichiers tels que vous les avez envoyés.',
  'data.export.formatLegend': 'Format des listes',
  'data.export.formatHelp':
    'Les listes décrivent le contenu de la copie : vos messages, événements, contacts ou fichiers, un par ligne.',
  'data.export.orgHelp':
    'Chaque copie concerne une seule organisation. Si vous en choisissez une autre, vous y entrez et le téléchargement démarre tout seul.',
  'data.export.json': 'Pour d’autres applications et programmes.',
  'data.export.csv': 'Pour l’ouvrir dans Excel, Numbers ou Google Sheets.',
  'data.export.xml': 'Pour les systèmes qui demandent du XML.',
  'data.export.action': 'Télécharger la copie',
  'data.export.startedTitle': 'Le téléchargement a commencé',
  'data.export.startedBody':
    'Votre navigateur continue de l’enregistrer même si vous fermez cette page. Si vous avez beaucoup de fichiers, cela prendra un moment.',
  'data.export.scope':
    'La copie contient les données de l’organisation de votre session. Le fichier de résumé inclus indique ce qui a été repris de chaque application.',
  'data.move.section': 'Le transférer vers un autre service',
  'data.move.body':
    'Pour l’envoyer directement vers Google Workspace ou Microsoft 365, sans rien télécharger : si votre compte est personnel ou si vous administrez votre organisation, lancez-le vous-même depuis Sytue Admin, dans Migrations ; sinon, demandez-le à la personne qui l’administre.',
  'data.move.action': 'Aller à Migrations',

  // --- Conectar tus dispositivos ----------------------------------------
  'devices.title': 'Connectez vos appareils',
  'devices.lead':
    'Emportez votre messagerie, votre agenda, vos contacts et vos fichiers Sytue sur votre téléphone ou votre ordinateur. Nous vous guidons étape par étape.',
  'devices.leadShort':
    'Emportez votre messagerie, votre agenda et vos contacts Sytue sur votre téléphone ou votre ordinateur.',
  'devices.unavailable.section': 'Impossible de vous montrer les détails pour le moment',
  'devices.unavailable.title': 'Nous n’avons pas pu récupérer la configuration',
  'devices.unavailable.body':
    'Réessayez dans un instant. Si cela persiste, écrivez à la personne qui administre Sytue dans votre organisation.',
  'devices.retry': 'Réessayer',
  'devices.copy': 'Copier',
  'devices.copyFailTitle': 'La copie a échoué',
  'devices.copyFailBody': 'Sélectionnez-le et copiez-le à la main.',
  'devices.copied.address': 'Votre adresse copiée',
  'devices.copied.imap': 'Serveur entrant copié',
  'devices.copied.port': 'Port copié',
  'devices.copied.smtp': 'Serveur sortant copié',
  'devices.copied.url': 'Adresse copiée',
  'devices.copied.drive': 'Adresse Drive copiée',
  'devices.oneClick.thunderbird': 'Configurer Thunderbird',
  'devices.oneClick.outlook': 'Configurer Outlook',
  'devices.oneClick.android': 'Configurer sur Android',
  'devices.oneClick.body':
    'Crée un mot de passe d’application rien que pour ce programme, le copie et vous l’affiche une seule fois.',
  'devices.oneClick.failTitle': 'Le mot de passe d’application n’a pas pu être créé',
  'devices.oneClick.failBody':
    'Réessayez dans un instant. Vous pouvez aussi en créer un dans Mon compte.',
  'devices.oneClick.onceTitle': 'Copié. Il n’est affiché que cette fois',
  'devices.oneClick.onceBody':
    'Collez-le là où le programme demande le mot de passe. Si vous le perdez, révoquez-le dans Mon compte et créez-en un autre.',
  'devices.oneClick.passwordLabel': 'Mot de passe d’application',
  'devices.copied.password': 'Mot de passe d’application copié',
  'devices.oneClick.notOpened':
    'Impossible d’ouvrir {program} sur cet ordinateur : ouvrez-le vous-même et suivez ces étapes.',
  'devices.oneClick.next.thunderbird':
    'Dans l’assistant de nouveau compte de Thunderbird, saisissez votre adresse et collez le mot de passe : il trouve les serveurs tout seul. Agendas et contacts : ajoutez un agenda sur le réseau et un carnet d’adresses CardDAV avec la même adresse et le même mot de passe (s’il demande l’emplacement, copiez-le ci-dessous).',
  'devices.oneClick.next.outlook':
    'Dans « Ajouter un compte » d’Outlook, saisissez votre adresse et collez le mot de passe : Outlook trouve les serveurs tout seul.',
  'devices.oneClick.next.android':
    'Dans votre app de messagerie, ajoutez un compte « Autre (IMAP) » avec votre adresse et collez le mot de passe. Agendas et contacts : DAVx5 avec les adresses ci-dessous et le même mot de passe.',
  'devices.android.davx5Calendars': 'Agendas dans DAVx5',
  'devices.android.davx5Contacts': 'Contacts dans DAVx5',
  'devices.android.davx5Install': 'Installer DAVx5 depuis F-Droid',

  'devices.platform.iphone.title': 'iPhone ou iPad',
  'devices.platform.iphone.body':
    'Vous installez un profil et la messagerie, l’agenda et les contacts sont prêts.',
  'devices.platform.iphone.note': 'Le plus rapide : un fichier et votre mot de passe.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'Le même profil que sur l’iPhone, installé depuis les Réglages Système.',
  'devices.platform.mac.note': 'Apple Mail, Calendrier et Contacts.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Vous saisissez votre adresse et votre mot de passe ; Outlook trouve le reste tout seul.',
  'devices.platform.outlook.note':
    'Messagerie uniquement. L’agenda et les contacts se configurent à part.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Il se configure à partir de votre seule adresse ; l’agenda et les contacts s’ajoutent ensuite.',
  'devices.platform.thunderbird.note': 'Windows, Mac ou Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'Votre application de messagerie pour la boîte aux lettres et DAVx5 pour l’agenda et les contacts.',
  'devices.platform.android.note':
    'Pas d’application Sytue : ce sont celles du système qui servent.',
  // Compatibilidad drop-in: Drive, Calendar y Contacts hablan las APIs de Google y
  // de Microsoft Graph. Estaba construido y no se decia en ninguna parte.
  'devices.platform.apis.title': 'Un outil qui parle Google ou Microsoft',
  'devices.platform.apis.body':
    'Faites pointer vers Sytue une intégration écrite pour Google Drive, Google Calendar, Google Contacts ou Microsoft Graph, sans toucher à son code.',
  'devices.platform.apis.note': 'Uniquement là où vous pouvez changer l’adresse de base.',
  'devices.apis.step1.title': 'Changez l’adresse de base',
  'devices.apis.step1.body':
    'Dans l’outil, remplacez l’adresse appelée par celle du produit et du dialecte dont vous avez besoin. Le reste de sa configuration ne change pas.',
  'devices.apis.step2.title': 'Utilisez vos identifiants Sytue',
  'devices.apis.step2.body':
    'L’authentification est votre jeton Sytue ID, pas un compte Google ou Microsoft : les autorisations sont celles que vous avez déjà ici.',
  'devices.apis.identityGoogle': 'Identité · OAuth et annuaire Google',
  'devices.apis.step3.title': 'Une app tierce, comme si c’était Google',
  'devices.apis.step3.body':
    'Si l’outil se connecte avec l’OAuth de Google, l’administration de l’organisation l’enregistre dans Sytue Admin avec les autorisations Google dont il a besoin. L’app demande ces autorisations et reçoit les données de Sytue ; Google n’intervient pas et ne voit rien.',
  'devices.apis.driveGoogle': 'Drive · API Google Drive',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · API Google Calendar',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · API People de Google',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Ce qui est couvert',
  'devices.apis.expectBody':
    'Lister, lire, créer, modifier et supprimer. Ce qui n’est pas couvert répond par une erreur claire, jamais par une réussite vide.',
  'devices.copied.apiBase': 'Adresse de base copiée',
  'devices.check.apis.label': 'L’outil',
  'devices.check.apis.value':
    'Il liste votre contenu Sytue comme s’il s’agissait du service d’origine.',
  'devices.platform.drive.title': 'Explorateur de fichiers (Drive)',
  'devices.platform.drive.body':
    'Vous voyez vos fichiers Sytue Drive comme un disque de plus sur l’ordinateur.',
  'devices.platform.drive.note': 'Finder sur Mac, Explorateur de fichiers sur Windows.',

  'devices.mail.address': 'Votre adresse (identifiant)',
  'devices.mail.imapHost': 'Serveur entrant (IMAP)',
  'devices.mail.imapPort': 'Port entrant',
  'devices.mail.smtpHost': 'Serveur sortant (SMTP)',
  'devices.mail.smtpPort': 'Port sortant',
  'devices.mail.hint':
    'Chiffrement SSL/TLS sur les deux, et le même mot de passe Sytue que pour vous connecter sur le web.',
  'devices.dav.caldav': 'Agenda (CalDAV)',
  'devices.dav.carddav': 'Contacts (CardDAV)',
  'devices.dav.user': 'Nom d’utilisateur',

  'devices.apple.download': 'Télécharger le profil Sytue',
  'devices.apple.step1.title': 'Téléchargez le profil',
  'devices.apple.step1.body':
    'Appuyez sur le bouton ci-dessus. Un petit fichier contenant les réglages de votre compte est téléchargé.',
  'devices.apple.step2.iphone.title': 'Installez-le depuis Réglages',
  'devices.apple.step2.iphone.body':
    'Ouvrez Réglages. « Profil téléchargé » apparaît tout en haut : touchez-le et appuyez sur Installer.',
  'devices.apple.step2.mac.title': 'Installez-le depuis les Réglages Système',
  'devices.apple.step2.mac.body':
    'Ouvrez Réglages Système › Confidentialité et sécurité › Profils, double-cliquez sur le profil téléchargé et appuyez sur Installer.',
  'devices.apple.step3.title': 'Aucun mot de passe à saisir',
  'devices.apple.step3.body':
    'Le profil contient son propre mot de passe d’application, créé uniquement pour cet appareil. Vous pouvez le révoquer à tout moment dans Mon compte.',
  'devices.apple.step4.title': 'C’est fait',
  'devices.apple.step4.body':
    'La messagerie, l’agenda et les contacts s’ajoutent d’un coup. Aucun serveur à saisir.',
  'devices.apple.manualTitle': 'Si vous préférez le faire à la main',
  'devices.apple.manualBody':
    'Vous pouvez aussi créer le compte manuellement avec ces informations.',
  'devices.manualValues': 'Voir les valeurs manuelles',

  'devices.desktop.step1.title': 'Ajoutez un compte de messagerie',
  'devices.desktop.step1.outlook': 'Dans Outlook : Fichier › Ajouter un compte.',
  'devices.desktop.step1.thunderbird':
    'Dans Thunderbird : Comptes › Configurer un compte › E-mail.',
  'devices.desktop.step2.title': 'Saisissez seulement votre adresse et votre mot de passe',
  'devices.desktop.step2.outlook':
    'Rien d’autre. Outlook demande la configuration à Sytue et la remplit pour vous.',
  'devices.desktop.step2.thunderbird':
    'Rien d’autre. Thunderbird demande la configuration à Sytue et la remplit pour vous.',
  'devices.desktop.step3.title': 'Confirmez',
  'devices.desktop.step3.body':
    'Quand il vous montre les serveurs, acceptez. La connexion se fait en compte IMAP.',
  'devices.desktop.emailLabel': 'Votre adresse e-mail',
  'devices.desktop.notFoundTitle': 'Il ne l’a pas trouvée tout seul ?',
  'devices.desktop.notFoundBody': 'Choisissez la configuration manuelle et utilisez ces valeurs.',
  'devices.thunderbird.davTitle': 'Agenda et contacts',
  'devices.thunderbird.davBody': 'Dans Thunderbird, ils s’ajoutent à part, avec ces adresses.',
  'devices.outlook.davTitle': 'Agenda et contacts dans Outlook',
  'devices.outlook.davBody':
    'Outlook pour Windows ne gère pas CalDAV/CardDAV : l’agenda et les contacts Sytue ne s’y synchronisent donc pas. Utilisez-les depuis le web Sytue, ou depuis un client qui parle bien CalDAV/CardDAV (Thunderbird, Calendrier Apple).',

  'devices.android.mailTitle': 'Messagerie',
  'devices.android.mailBody':
    'Avec l’application de messagerie que vous utilisez déjà (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Ajoutez un compte',
  'devices.android.mail1.body': 'Choisissez « Autre » ou « Personnel (IMAP) », pas « Google ».',
  'devices.android.mail2.title': 'Saisissez votre adresse et votre mot de passe',
  'devices.android.mail2.body': 'S’il demande les serveurs, copiez-les ci-dessous.',
  'devices.android.davTitle': 'Agenda et contacts',
  'devices.android.davBody':
    'Android ne fournit pas CalDAV/CardDAV : il vous faut DAVx5, une application gratuite qui les ajoute à l’agenda et au carnet d’adresses du téléphone.',
  'devices.android.dav1.title': 'Installez DAVx5',
  'devices.android.dav1.body':
    'Elle est sur Google Play et sur F-Droid. Sytue ne la publie ni ne la maintient.',
  'devices.android.dav2.title':
    'Ajoutez un compte avec « Connexion avec URL et nom d’utilisateur »',
  'devices.android.dav2.body':
    'Collez l’adresse ci-dessous et utilisez votre e-mail et votre mot de passe Sytue.',
  'devices.android.dav3.title': 'Choisissez quoi synchroniser',
  'devices.android.dav3.body': 'Cochez vos agendas et votre carnet d’adresses.',

  'devices.drive.step1.title': 'Ouvrez la connexion à un serveur',
  'devices.drive.step1.body':
    'Sur Mac : Finder › Aller › Se connecter au serveur (⌘K). Sur Windows : Explorateur de fichiers › Ce PC › Connecter un lecteur réseau, et cochez « Se connecter à l’aide d’identifiants différents ».',
  'devices.drive.step2.title': 'Collez cette adresse',
  'devices.drive.step2.body': 'C’est le dossier qui contient vos fichiers Sytue Drive.',
  'devices.drive.step3.title': 'Connectez-vous avec votre compte Sytue',
  'devices.drive.step3.body': 'Identifiant : votre adresse e-mail. Mot de passe : celui de Sytue.',
  'devices.drive.urlLabel': 'Adresse Drive (WebDAV)',
  'devices.drive.expectTitle': 'À quoi vous attendre',
  'devices.drive.windowsExpectBody':
    'Sous Windows, Drive apparaît dans l’Explorateur de fichiers comme le dossier « Sytue Drive ». Les fichiers se téléchargent quand vous les ouvrez ; clic droit › « Toujours conserver sur cet appareil » pour les avoir hors ligne. Ce que vous y enregistrez ou créez est envoyé dans Drive.',
  'devices.drive.expectBody':
    'Les fichiers s’ouvrent depuis le serveur, ils ne sont pas copiés sur le disque. Pour travailler hors connexion, téléchargez à l’avance ce dont vous avez besoin.',

  'devices.check.mail.label': 'La messagerie',
  'devices.check.mail.value':
    'Les nouveaux messages arrivent et vous pouvez vous en envoyer un à vous-même.',
  'devices.check.dav.label': 'Agenda et contacts',
  'devices.check.dav.value':
    'Un événement créé sur le web Sytue apparaît sur l’appareil en quelques minutes.',
  'devices.check.profile.label': 'Le profil',
  'devices.check.profile.value': '« Sytue One » figure dans la liste des profils installés.',
  'devices.check.drive.label': 'Le lecteur',
  'devices.check.drive.value': 'Vous voyez vos dossiers Drive et vous pouvez ouvrir un fichier.',
  'devices.check.write.label': 'L’écriture',
  'devices.check.write.value':
    'Vous créez un dossier et il apparaît aussi dans Sytue Drive dans le navigateur.',

  'devices.step1.title': 'Ce que vous utilisez',
  'devices.step1.heading': 'Où voulez-vous votre compte Sytue ?',
  'devices.step1.description':
    'Choisissez-en un. Si vous en avez plusieurs, refaites l’assistant pour chacun.',
  'devices.step2.title': 'Comment faire',
  'devices.step2.heading': 'Configurer {platform}',
  'devices.step2.headingFallback': 'Comment faire',
  'devices.step2.description':
    'Suivez ces étapes. Tout ce que vous avez à saisir se copie d’un bouton.',
  'devices.step2.next': 'C’est fait',
  'devices.step3.title': 'Vérification',
  'devices.step3.heading': 'Comment savoir que ça a marché',
  'devices.step3.description': 'Si tout cela est vrai, la connexion est bonne.',
  'devices.step3.helpTitle': 'Quelque chose ne colle pas ?',
  'devices.step3.helpBody':
    'Le mot de passe est le coupable habituel : utilisez le même que pour vous connecter à Sytue dans le navigateur. Si vous avez la validation en deux étapes, votre organisation peut exiger un mot de passe d’application ; demandez à la personne qui administre Sytue.',
  'devices.step3.again': 'Connecter un autre appareil',

  // --- Alta guiada (onboarding) -----------------------------------------
  'ob.error.generic': 'Nous n’avons pas pu terminer l’opération. Réessayez.',
  'ob.failedTitle': 'Nous n’avons pas pu charger votre inscription',
  'ob.failedBody':
    'Le service d’identité ne répond pas pour le moment. Rien n’est perdu : réessayez dans une minute.',
  'ob.retry': 'Réessayer',
  'ob.preparing': 'Préparation de votre inscription…',
  'ob.lead':
    'Quelques minutes et c’est prêt. On avance pas à pas ; si vous vous arrêtez en cours de route, on reprend là où vous en étiez à votre retour.',
  'ob.saveFailedTitle': 'Impossible d’enregistrer',
  'ob.saveFailedBody': 'Réessayez dans un instant.',
  'ob.copy': 'Copier',
  'ob.copyFailTitle': 'La copie a échoué',
  'ob.copyFailBody': 'Sélectionnez-le à la souris et copiez-le à la main.',
  'ob.copied.recordType': 'Type d’enregistrement copié',
  'ob.copied.recordName': 'Nom de l’enregistrement copié',
  'ob.copied.recordValue': 'Valeur de l’enregistrement copiée',

  'ob.roleOption.member': 'Membre — utilise la suite, n’administre rien',
  'ob.roleOption.admin': 'Administrateur — peut gérer toute l’organisation',
  'ob.roleOption.billing': 'Facturation — s’occupe des factures',
  'ob.roleName.owner': 'Propriétaire',
  'ob.roleName.admin': 'Administrateur',
  'ob.roleName.member': 'Membre',
  'ob.roleName.billing': 'Facturation',

  'ob.mode.title': 'Démarrage',
  'ob.mode.heading': 'Sytue, c’est pour qui ?',
  'ob.mode.description': 'Cela nous dit quoi vous demander. Vous pourrez le changer plus tard.',
  'ob.mode.individual.title': 'Juste pour moi',
  'ob.mode.individual.body':
    'Votre messagerie, vos fichiers et votre agenda, avec votre propre adresse. Si un jour vous devez inviter quelqu’un, vous le pourrez.',
  'ob.mode.org.title': 'Pour mon organisation',
  'ob.mode.org.body':
    'Vous inscrivez votre équipe, vous répartissez qui peut faire quoi et vous administrez le tout depuis un seul endroit.',
  'ob.mode.org.badge': 'Le plus courant',

  'ob.identity.titleOrg': 'Votre organisation',
  'ob.identity.titleMe': 'Votre compte',
  'ob.identity.headingOrg': 'Comment s’appelle votre organisation ?',
  'ob.identity.headingMe': 'Comment voulez-vous appeler votre compte ?',
  'ob.identity.descriptionCreated':
    'Elle est déjà créée. Vous pouvez changer le nom ; l’adresse, elle, ne bouge pas.',
  'ob.identity.description': 'Avec cela, nous créons votre espace et votre adresse dans Sytue.',
  'ob.identity.nextCreated': 'Continuer',
  'ob.identity.nextOwnDomain': 'Créer et voir quoi ajouter à mon domaine',
  'ob.identity.next': 'Créer et continuer',
  'ob.identity.nameLabel': 'Nom',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Marie Dupont',
  'ob.identity.nameHintOrg':
    'C’est ce que votre équipe voit en se connectant. Vous pouvez le changer quand vous voulez.',
  'ob.identity.nameHintMe': 'Vous seul le voyez. Vous pouvez le changer quand vous voulez.',
  'ob.identity.slugLabel': 'Votre adresse dans Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Votre adresse est {subdomain}. Elle ne peut pas changer : votre messagerie et vos fichiers en dépendent.',
  'ob.identity.slugEmpty': 'Choisissez un nom court. Ce sera votre adresse : acme.{suite}',
  'ob.identity.slugChecking': 'Vérification de sa disponibilité…',
  'ob.identity.slugFree': 'Libre. Votre adresse sera {subdomain}',
  'ob.identity.slugTaken': 'Impossible de l’utiliser.',
  'ob.identity.firstTitle': 'Nous créons d’abord votre espace',
  'ob.identity.firstBody':
    'Quand vous appuierez sur le bouton ci-dessous, nous créons votre adresse Sytue et nous vous montrons ici même l’enregistrement exact à ajouter à votre domaine. Pas besoin de l’avoir sous la main tout de suite.',

  'ob.domain.legend': 'Et votre propre domaine ?',
  'ob.domain.keep.title': 'Je garde {host}',
  'ob.domain.keep.placeholder': 'mon-nom',
  'ob.domain.keep.body':
    'Vous n’avez rien à toucher : la publication, c’est pour nous. C’est ce que nous recommandons pour commencer.',
  'ob.domain.own.title': 'J’ai déjà un domaine et je veux l’utiliser (acme.com)',
  'ob.domain.own.body':
    'S’il est chez Cloudflare, nous le configurons pour vous. Sinon, nous vous indiquons l’enregistrement exact à ajouter et nous le vérifions ici même.',
  'ob.domain.search.title': 'Je n’ai pas encore de domaine',
  'ob.domain.search.body':
    'Nous vérifions si le nom que vous voulez est libre et combien il coûte par an. Ce n’est qu’une recherche : rien ne s’achète ici.',
  'ob.domain.hostLabel': 'Votre domaine',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Écrivez-le tel quel : acme.com, sans « https:// » et sans « www ».',
  'ob.domain.verified': '{host} vérifié',

  'ob.dns.legend': 'Où se trouve {domain} ?',
  'ob.dns.fallback': 'votre domaine',
  'ob.dns.cf.title': 'Chez Cloudflare — configurez-le pour moi',
  'ob.dns.cf.body':
    'Vous nous donnez une autorisation temporaire sur votre compte et nous créons les enregistrements nous-mêmes. Avant de toucher à quoi que ce soit, nous vous montrons ce que nous allons changer.',
  'ob.dns.cf.badge': 'Sans toucher au DNS',
  'ob.dns.manual.title': 'Chez un autre fournisseur — je m’en occupe',
  'ob.dns.manual.body':
    'Nous vous donnons l’enregistrement exact à copier dans votre panneau DNS et nous le vérifions ici dès que vous l’avez.',
  'ob.dns.manualSummary': 'Je préfère ajouter les enregistrements moi-même',

  'ob.people.title': 'Votre équipe',
  'ob.people.heading': 'Qui invitez-vous en premier ?',
  'ob.people.description':
    'Chaque personne reçoit un e-mail avec un lien personnel pour créer son mot de passe et se connecter. Vous pouvez sauter cette étape et le faire plus tard.',
  'ob.people.next': 'Continuer',
  'ob.people.emailsLabel': 'Adresses e-mail',
  'ob.people.emailsPlaceholder': 'anne@acme.com, bruno@acme.com',
  'ob.people.emailsHint':
    'Vous pouvez en coller plusieurs d’un coup, séparées par des virgules ou des sauts de ligne.',
  'ob.people.roleLabel': 'Avec quel rôle elles arrivent',
  'ob.people.roleHint': 'Cela peut être changé à tout moment depuis Sytue Admin.',
  'ob.people.send': 'Envoyer les invitations',
  'ob.people.sendNote': 'Rien n’est envoyé tant que vous n’appuyez pas ici.',
  'ob.people.pendingTitle': 'Invitations en attente d’acceptation',
  'ob.people.emptyTitle': 'Vous n’avez encore invité personne',
  'ob.people.emptyBody':
    'Vous pouvez le faire maintenant ou plus tard, depuis Sytue Admin. Votre organisation fonctionne pareil.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Invitation envoyée',
  'ob.invite.already': 'Déjà dans votre organisation',
  'ob.invite.invalid': 'Cela ne ressemble pas à une adresse e-mail',
  'ob.invite.notSent': 'L’e-mail n’a pas pu être envoyé',
  'ob.invite.failed': 'Impossible de les inviter',
  'ob.invite.default':
    'Elles recevront un e-mail avec un lien personnel pour créer leur mot de passe et se connecter.',

  'ob.billing.title': 'Facturation',
  'ob.billing.heading': 'À quel nom sont établies les factures ?',
  'ob.billing.description':
    'Ajoutez les données de facturation et choisissez les sièges payants. L’accès ne démarre qu’après confirmation de Mollie.',
  'ob.billing.next': 'Continuer',
  'ob.billing.pay': 'Payer en toute sécurité avec Mollie',
  'ob.billing.verify': 'Vérifier le paiement',
  'ob.billing.legalLabel': 'Raison sociale',
  'ob.billing.legalPlaceholder': 'ACME Europe SAS',
  'ob.billing.legalHint':
    'Le nom légal de l’entreprise, exactement tel qu’il figure sur vos factures.',
  'ob.billing.taxLabel': 'Identification fiscale',
  'ob.billing.taxPlaceholder': 'FR12345678901',
  'ob.billing.taxHint': 'Le numéro fiscal ou de TVA de votre pays.',
  'ob.billing.addressLabel': 'Adresse',
  'ob.billing.addressPlaceholder': '1 rue de Rivoli, 75001 Paris, France',
  'ob.billing.addressHint': 'L’adresse du siège qui figure sur la facture.',
  'ob.billing.emailLabel': 'E-mail pour les factures',
  'ob.billing.emailPlaceholder': 'factures@acme.com',
  'ob.billing.emailHint':
    'C’est là que nous envoyons les factures. Par défaut, l’e-mail de votre compte.',
  'ob.billing.note':
    'Nous ne demandons que le nécessaire pour vous facturer. Rien de tout cela n’est partagé avec qui que ce soit.',
  'ob.billing.seatsLabel': 'Sièges payants',
  'ob.billing.seatsHint': '{seats} siège(s) × {price}/Utilisateur/Mois · total {total}/Mois.',
  'ob.billing.active': 'Abonnement actif',
  'ob.billing.activeBody': 'Mollie a confirmé le paiement. Votre accès Sytue One est activé.',
  'ob.billing.pending': 'Nous vérifions le paiement',
  'ob.billing.pendingBody':
    'Mollie n’a pas encore confirmé le paiement. Attendez quelques secondes puis vérifiez à nouveau ; vous ne serez pas débité deux fois.',

  'ob.roles.title': 'Autorisations',
  'ob.roles.heading': 'Voulez-vous répartir les tâches d’administration ?',
  'ob.roles.description':
    'Ce sont des rôles tout prêts : vous choisissez ceux que vous voulez et nous les créons. Ensuite, vous les attribuez à qui vous voulez depuis Sytue Admin. Vous pouvez sauter cette étape.',
  'ob.roles.next': 'Continuer',
  'ob.roles.already': 'Déjà créé',
  'ob.roles.seePermissions': 'Voir les autorisations exactes',
  'ob.roles.helpTitle': 'Vous gardez la main',
  'ob.roles.helpBody':
    'Votre compte a la propriété de l’organisation : ces rôles ne lui retirent rien. Vous pouvez les modifier ou les supprimer quand vous voulez dans Sytue Admin → Rôles.',

  'ob.done.title': 'Terminé',
  'ob.done.headingCompleted': 'Tout est prêt',
  'ob.done.heading': 'Voici ce que vous avez configuré',
  'ob.done.description': 'Cela vient de votre compte réel, pas de ce que vous avez saisi ici.',
  'ob.done.next': 'Entrer dans Sytue',
  'ob.done.warnTitle': '{host} n’est pas encore vérifié',
  'ob.done.warnBody':
    'Sytue fonctionne pareil en attendant, avec {subdomain}. Quand vous aurez ajouté l’enregistrement à votre domaine, revenez à l’étape « {step} » et appuyez sur « Vérifier maintenant ».',
  'ob.done.nextTitle': 'Où aller ensuite',
  'ob.next.devices.title': 'Connectez votre téléphone et votre ordinateur',
  'ob.next.devices.body':
    'Laissez la messagerie, l’agenda et les contacts fonctionner dans les applications de toujours.',
  'ob.next.migrations.title': 'Récupérez vos contenus de Microsoft 365 ou de Google',
  'ob.next.migrations.body':
    'Nous copions les e-mails, les fichiers et les agendas sans que vous perdiez quoi que ce soit en chemin.',
  'ob.next.apps.title': 'Connectez vos applications',
  'ob.next.apps.body': 'Les outils que votre équipe utilise déjà, reliés à votre compte Sytue.',
  'ob.next.users.title': 'Continuez à inscrire votre équipe',
  'ob.next.users.body': 'Invitez celles et ceux qui manquent et vérifiez qui a quel rôle.',

  'ob.sum.org': 'Organisation',
  'ob.sum.account': 'Votre compte',
  'ob.sum.address': 'Adresse',
  'ob.sum.addressLive': '{host} · publiée',
  'ob.sum.addressPending': '{host} · en attente de publication',
  'ob.sum.ownDomain': 'Domaine personnalisé',
  'ob.sum.domainVerified': '{host} · vérifié',
  'ob.sum.domainPending': '{host} · en attente de l’enregistrement',
  'ob.sum.noDomain': 'Pas de domaine personnalisé',
  'ob.sum.invitations': 'Invitations',
  // `one` cubre también el 0 en francés, así que aquí NUNCA va un «1» literal.
  'ob.sum.invitationsPending': {
    one: '{count} en attente d’acceptation',
    other: '{count} en attente d’acceptation',
  },
  'ob.sum.noInvitations': 'Aucune pour l’instant',
  'ob.sum.billing': 'Facturation',
  'ob.sum.billingEmpty': 'Non renseignée (pas encore nécessaire)',
  'ob.sum.roles': 'Rôles créés',
  'ob.sum.edit': 'Modifier',
  'ob.sum.noRoles': 'Aucun',

  'ob.sub.liveTitle': 'Votre adresse est déjà publiée',
  'ob.sub.liveBody': '{host} résout sur internet. Vous n’avez rien d’autre à faire.',
  'ob.sub.failedTitle': 'Votre adresse n’est pas encore publiée',
  'ob.sub.failedDefault': 'Nous n’avons pas pu la publier pour le moment.',
  'ob.sub.failedTail':
    'Votre compte est créé : poursuivez l’inscription et revenez dans un moment.',
  'ob.sub.reservedTitle': 'Votre adresse est réservée',
  'ob.sub.reservedBody':
    '{host} est à vous. La publier sur internet, c’est pour nous : vous n’avez rien à faire.',

  'ob.search.label': 'Le domaine qui vous plairait',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Écrivez-le en entier, avec son extension : acme.com, acme.fr, acme.eu…',
  'ob.search.check': 'Vérifier la disponibilité',
  'ob.search.useOwn': 'Je l’ai déjà enregistré : reliez-le',
  'ob.search.note':
    'Les domaines ne se vendent pas chez Sytue : ici, nous vous disons seulement s’il est libre et combien il coûte. Enregistrez-le où vous préférez et revenez à cette étape, sous « J’ai déjà un domaine », pour le relier à votre compte.',
  'ob.avail.freeTitle': '{domain} est libre',
  'ob.avail.freePrice':
    'Il coûte {registration} la première année et {renewal} à chaque renouvellement. C’est le prix du jour chez Cloudflare ; là où vous l’enregistrerez, il peut être autre.',
  'ob.avail.freeNoPrice':
    'Il peut être enregistré. Le bureau d’enregistrement où vous l’achèterez vous indiquera le prix.',
  'ob.avail.takenTitle': '{domain} est déjà enregistré',
  'ob.avail.takenBody':
    'Quelqu’un l’a. Essayez un autre nom, ou une autre extension (.fr, .eu, .net…). Si le domaine est le vôtre, choisissez « J’ai déjà un domaine » ci-dessus et nous le relierons.',
  'ob.avail.unsupportedTitle': 'Nous ne pouvons pas consulter cette extension',
  'ob.avail.unsupportedBody':
    'Nous ne pouvons pas vous dire s’il est libre ni combien il coûte : vérifiez-le auprès de votre bureau d’enregistrement. Quand vous l’aurez, revenez ici et choisissez « J’ai déjà un domaine ».',
  'ob.avail.invalidTitle': 'Cela ne ressemble pas à un domaine',
  'ob.avail.invalidBody':
    'Écrivez-le comme acme.com : un nom, un point et l’extension. Sans « https:// » et sans « www ».',
  'ob.avail.unconfiguredTitle': 'Nous ne consultons pas les prix des domaines ici',
  'ob.avail.unconfiguredBody':
    'Cherchez-le auprès du bureau d’enregistrement de votre choix. Quand vous l’aurez, revenez à cette étape et choisissez « J’ai déjà un domaine » pour le relier.',
  'ob.avail.unknownTitle': 'Nous n’avons pas pu le vérifier pour le moment',
  'ob.avail.unknownBody':
    'Ce n’est pas la faute de ce que vous avez saisi : nous n’avons obtenu aucune réponse. Réessayez dans un instant.',

  'ob.dnsRec.title': 'Ajoutez cet enregistrement là où votre domaine est enregistré',
  'ob.dnsRec.body':
    'Cherchez la section « DNS » ou « Enregistrements » chez votre fournisseur, créez un enregistrement TXT et copiez ces deux valeurs. La marche à suivre est la même partout.',
  'ob.dnsRec.type': 'Type',
  'ob.dnsRec.name': 'Nom',
  'ob.dnsRec.value': 'Valeur',
  'ob.dnsRec.verifiedTitle': 'Domaine vérifié',
  'ob.dnsRec.verifiedBody':
    'Nous avons trouvé l’enregistrement : {host} est à vous et il est inscrit comme tel.',
  'ob.dnsRec.mismatchTitle': 'Il y a un enregistrement, mais pas celui-ci',
  'ob.dnsRec.mismatchBody':
    'Sur {name}, nous avons trouvé « {found} ». Vérifiez que vous avez copié la valeur en entier et qu’aucun ancien enregistrement ne traîne.',
  'ob.dnsRec.unreachableTitle': 'Nous n’avons pas pu interroger votre DNS',
  'ob.dnsRec.unreachableBody':
    'Ce n’est pas la faute de votre enregistrement : le serveur de noms n’a pas répondu. Réessayez dans un instant.',
  'ob.dnsRec.pendingTitle': 'Nous ne voyons pas encore l’enregistrement',
  'ob.dnsRec.pendingBody':
    'Si vous venez de le créer, c’est normal : les changements DNS mettent de quelques minutes à quelques heures à apparaître. Vous pouvez poursuivre l’inscription et revérifier quand vous voulez.',
  'ob.dnsRec.uncheckedTitle': 'Pas encore vérifié',
  'ob.dnsRec.uncheckedBody':
    'Quand vous aurez ajouté l’enregistrement chez votre fournisseur, appuyez sur « Vérifier maintenant » et nous regardons votre DNS aussitôt.',
  'ob.dnsRec.empty':
    'Quand vous appuierez sur « Enregistrer et vérifier », nous vous donnons l’enregistrement exact à ajouter.',
  'ob.dnsRec.check': 'Vérifier maintenant',
  'ob.dnsRec.save': 'Enregistrer et vérifier',

  'ob.cf.state.ok': 'Déjà en place',
  'ob.cf.state.created': 'Créé',
  'ob.cf.state.replaced': 'Remplacé',
  'ob.cf.state.missing': 'Nous l’ajoutons',
  'ob.cf.state.conflict': 'Il y a déjà quelque chose',
  'ob.cf.state.failed': 'Échec',
  'ob.cf.state.pending': 'Pas encore',
  'ob.cf.permTitle': 'Donnez-nous l’autorisation pour votre DNS uniquement',
  'ob.cf.step1': 'Ouvrez vos jetons d’API Cloudflare et appuyez sur « Create token ».',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2':
    'Choisissez « Create custom token » et cochez une seule autorisation : Zone · DNS · Edit.',
  'ob.cf.step3': 'Sous « Zone resources », choisissez {domain}, créez le jeton et collez-le ici.',
  'ob.cf.tokenLabel': 'Jeton Cloudflare',
  'ob.cf.tokenPlaceholder': 'Le jeton que vous venez de créer',
  'ob.cf.tokenHint':
    'Il ne fonctionne que pour le DNS de ce domaine. Nous l’utilisons puis nous le jetons : il n’est pas conservé sauf si vous nous le demandez ci-dessous, et il ne quitte jamais nos serveurs.',
  'ob.cf.mailLabel': 'Recevoir aussi dans Sytue Mail les e-mails de @{domain}',
  'ob.cf.mailHintOn':
    'Cela change l’endroit où arrivent les e-mails de votre domaine. Si vous les recevez ailleurs aujourd’hui, nous vous préviendrons avant de remplacer quoi que ce soit.',
  'ob.cf.mailHintOff':
    'Ne l’activez que si vous voulez que vos boîtes aux lettres @votre-domaine déménagent vers Sytue.',
  'ob.cf.rememberLabel': 'Retenir cette autorisation pour les prochains changements',
  'ob.cf.rememberHint':
    'Si vous ne cochez pas, il faudra le recoller la prochaine fois. Cocher le conserve chiffré ; décocher l’efface.',
  'ob.cf.preview': 'Voir ce que nous allons changer',
  'ob.cf.previewAgain': 'Regarder à nouveau',
  'ob.cf.nothing': 'Plus rien à faire',
  'ob.cf.apply': { one: 'Appliquer {count} changement', other: 'Appliquer {count} changements' },
  'ob.cf.zone': 'Zone trouvée',
  'ob.cf.token': 'Votre jeton',
  'ob.cf.tokenStored': 'Conservé chiffré pour les prochains changements',
  'ob.cf.tokenDiscarded': 'Utilisé puis jeté (nous ne l’avons pas conservé)',
  'ob.cf.domainState': 'État du domaine',
  'ob.cf.verified': 'Vérifié',
  'ob.cf.pending': 'En attente de vérification',
  'ob.cf.nowThere': 'Il y a actuellement : {current}',
  'ob.cf.replaceLabel': 'Le remplacer par celui de Sytue',
  'ob.cf.failTitle': 'Quelque chose n’a pas été configuré',
  'ob.cf.failBody':
    'Le reste est en place. Ci-dessus, vous avez la raison de chaque enregistrement en échec et sa valeur, pour l’ajouter à la main depuis Cloudflare.',
  'ob.cf.conflictTitle': 'Il y a des enregistrements à vous que nous ne toucherons pas',
  'ob.cf.conflictBody':
    'Nous ne remplaçons jamais ce que vous avez déjà sauf si vous le cochez. Si vous préférez les combiner à la main, vous avez là-haut la valeur actuelle et la nôtre.',
  'ob.cf.okTitle': 'Domaine vérifié',
  'ob.cf.okBody':
    'Nous avons vu l’enregistrement dans votre propre zone : {host} est à vous et il est inscrit comme tel.',
  'ob.cf.todoTitle': 'Les changements restent à appliquer',
  'ob.cf.todoBody':
    'Nous n’avons pas encore touché à votre zone. Appuyez sur « Appliquer » quand ce qui précède vous convient.',
  'ob.cf.someFailed': {
    one: '{count} enregistrement n’a pas été configuré',
    other: '{count} enregistrements n’ont pas été configurés',
  },
  'ob.cf.someFailedBody':
    'Ci-dessous, vous avez la raison de chacun et les valeurs pour le faire à la main.',
  'ob.cf.configured': '{host} configuré et vérifié',
  'ob.cf.created': 'Enregistrements créés dans votre zone',
  'today.title': 'Aujourd’hui',
  'today.openCalendar': 'Voir le calendrier',
  'today.noEvents': 'Rien à l’agenda aujourd’hui.',
  'today.unavailable': 'L’agenda n’est pas disponible pour le moment.',
  'today.allDay': 'Toute la journée',
  'today.untitled': '(sans titre)',
  'today.unread': { one: '{count} courriel non lu', other: '{count} courriels non lus' },
  'today.inboxZero': 'Boîte de réception à jour',
  'launcher.quickActions': 'Accès rapides',
  'launcher.recentAll': 'Récent dans toute la suite',
  'launcher.activityError': 'Impossible de charger votre activité',
  'launcher.noActivity': 'Aucune activité récente',
  'ob.dns.verified': 'Domaine vérifié',
  'ob.dns.mismatch': 'Il y a un enregistrement, mais pas celui-ci',
  'ob.dns.unreachable': "Nous n'avons pas pu interroger votre DNS",
  'ob.dns.notYet': "Nous ne voyons pas encore l'enregistrement",
  'ob.dns.unchecked': 'Pas encore vérifié',

  // --- Onboarding: rótulos que estaban en duro ---
  'ob.dns.type': 'Type',
  'ob.dns.name': 'Nom',
  'ob.dns.value': 'Valeur',
  'ob.cf.tokenField': 'Jeton Cloudflare',
  'ob.cf.remember': 'Mémoriser cette autorisation pour les modifications futures',
  'ob.cf.replaceWithSytue': 'Le remplacer par celui de Sytue',
  'recent.hubDown':
    'Le Hub ne répond pas pour le moment. Votre travail est en sécurité ; réessayez.',
  'search.partial':
    '{sources} n’a pas répondu, il peut donc manquer des résultats. Réessayez dans un instant.',
  'search.partialJoin': ' et ',
  'search.showing': '{count} résultats affichés sur {total}.',
  'privacy.notice':
    'Nous ne vendons pas vos données et ne les cédons pas à des tiers. Nous ne les utilisons ni pour vous profiler, ni pour la publicité, ni pour entraîner ou améliorer nos services ou ceux d’autrui. Elles sont chiffrées en transit et au repos, et ne quittent notre infrastructure européenne que pour le strict nécessaire de la facturation et dans les intégrations que vous activez, où le tiers vous demandera votre accord sur les données utilisées.',
};
