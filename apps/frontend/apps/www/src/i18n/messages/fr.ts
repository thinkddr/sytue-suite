/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo FRANCÉS del sitio público. Traducción del inglés (`en.ts`), que es la
 * fuente de verdad: el tipo se deriva de él y una clave de más o de menos rompe
 * la compilación (docs/CONVENTIONS.md §13.1).
 *
 * Tipografía francesa: espacio antes de « : » y « ; », guillemets « » y espacio
 * antes del signo de porcentaje. Términos RGPD según el vocabulario oficial del
 * Reglamento en francés (responsable du traitement, sous-traitant, etc.).
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fr: AppCatalog<typeof en> = {
  // --- Cromo del sitio ---------------------------------------------------
  'skip.content': 'Aller au contenu',
  'nav.products': 'Produits',
  'nav.sovereignty': 'Souveraineté',
  'nav.standards': 'Standards ouverts',
  'nav.pricing': 'Tarifs',
  'footer.pricing': 'Tarifs',
  'home.pricing.title': 'Trois forfaits, toute la suite',
  'home.pricing.body':
    'Les mêmes produits dans les trois : ce qui change, c’est l’espace par utilisateur. Pas d’édition au rabais.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'hors TVA',
  'home.pricing.per': '/Utilisateur/Mois',
  'home.pricing.cta': 'Parler aux ventes',
  'home.pricing.commitment.title': 'Un engagement à vie',
  'home.pricing.commitment.body':
    'Le prix par utilisateur augmente de 0,50 € tous les quatre ans, et rien d’autre : à vie. Vous le savez le jour où vous arrivez et cela ne change pas.',
  'home.pricing.included.title': 'Une seule édition',
  'home.pricing.included.body':
    'Pas de forfaits par fonctionnalité, pas de paliers, pas d’éditions allégées : la suite complète est comprise dans le prix par utilisateur, avec les mêmes fonctions pour tout le monde.',
  'home.pricing.metered.title': 'Ce qui est facturé à part',
  'home.pricing.metered.body': 'Le stockage supplémentaire : à l’usage ou par blocs.',
  'home.pricing.storage.title': 'Le stockage est partagé',
  'home.pricing.storage.body':
    'Le quota de chaque personne s’ajoute à celui de l’organisation : ce que l’une n’utilise pas, les autres en profitent. Et le quota par utilisateur se règle depuis l’organisation.',
  'nav.downloads': 'Téléchargements',
  'nav.aria': 'Navigation du site',
  'nav.ariaMobile': 'Navigation mobile',
  'nav.home': 'Sytue One — Accueil',
  'nav.openMenu': 'Ouvrir le menu',
  'nav.closeMenu': 'Fermer le menu',
  'theme.toLight': 'Passer en mode clair',
  'theme.toDark': 'Passer en mode sombre',
  'lang.label': 'Langue',
  'cta.start': 'Commencer gratuitement',
  'cta.signIn': 'Se connecter',
  'cta.seeSuite': 'Voir la suite',

  // --- Portada -----------------------------------------------------------
  'meta.home.title': 'Sytue One — La suite bureautique souveraine européenne',
  'meta.home.description':
    'Sytue One : la suite bureautique souveraine européenne. Identité, messagerie, fichiers, documents, agenda et discussions, 100 % dans l’UE et sur des standards ouverts.',
  'home.hero.lead': 'Votre productivité,',
  'home.hero.accent': 'souveraine et européenne',
  'home.hero.subtitle':
    'Messagerie, fichiers, documents, agenda, discussions et visioconférence, intégrés comme une seule application et hébergés intégralement dans l’UE.',
  'home.hero.note': 'Sans carte bancaire. Données chez Scaleway (UE). Migration accompagnée.',
  'home.privacy.title': 'Votre travail vous appartient. Votre vie privée aussi.',
  'home.privacy.body':
    'Nous ne vendons pas vos données et ne les utilisons pas pour créer des profils publicitaires.',
  'home.privacy.anonymity':
    'Sytue Analytics protège votre vie privée avec des statistiques agrégées, sans cookies de suivi et avec des identifiants de visiteurs pseudonymes qui changent chaque jour et sont distincts pour chaque site.',
  'home.privacy.account':
    'L’utilisation de la suite nécessite des données de compte et des journaux d’accès afin que nous puissions fournir le service et assurer sa sécurité.',
  'home.privacy.link': 'Comment nous protégeons votre vie privée',

  'home.stats.aria': 'Empreinte européenne',
  'home.stats.regions': 'Régions UE (fr-par · nl-ams · on-premise Madrid)',
  'home.stats.apps': 'Applications avec une seule identité',
  'home.stats.standards': 'Standards ouverts : OIDC, JMAP, CalDAV, CardDAV et WebDAV',

  'home.products.title': 'Une suite, une expérience',
  'home.products.subtitle':
    'Chaque produit partage le même design, la même identité et la même interface. Ils s’utilisent comme une seule application.',

  'home.integration.title': 'L’intégration qu’on attend d’une vraie suite',
  'home.integration.subtitle':
    'Une seule inscription ouvre tout. Provisionnement automatique (SCIM) depuis Sytue ID.',
  'home.integration.sso.title': 'Authentification unique',
  'home.integration.sso.body': 'Une identité (Sytue ID) pour toute la suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Interface commune',
  'home.integration.shell.body':
    'Barre supérieure, sélecteur d’applications, recherche et notifications globales dans chaque produit.',
  'home.integration.cross.title': 'Actions transversales',
  'home.integration.cross.body':
    'Joignez un fichier Drive depuis Mail, transformez un e-mail en évènement Calendar et partagez-le dans Rooms sans quitter votre flux.',

  'home.sovereignty.title': 'La souveraineté, vraiment',
  'home.sovereignty.body':
    'Ce n’est pas une promesse marketing : c’est l’architecture. L’infrastructure ne vit que dans des régions européennes et tout repose sur des protocoles ouverts et auditables, sans porte dérobée.',
  'home.sovereignty.link': 'Pourquoi des standards ouverts',
  'home.sovereignty.eu.title': '100 % dans l’UE',
  'home.sovereignty.eu.body':
    'Toute l’infrastructure dans l’UE : Scaleway (fr-par / nl-ams) et une zone on-premise à Madrid. Aucun fournisseur américain pour les données personnelles.',
  'home.sovereignty.gdpr.title': 'Conformité RGPD',
  'home.sovereignty.gdpr.body':
    'DPA, registre des traitements, procédure de réponse aux demandes d’exercice des droits et durées de conservation. Chiffrement en transit (TLS 1.3) et au repos.',
  'home.sovereignty.lockin.title': 'Aucune dépendance',
  'home.sovereignty.lockin.body':
    'Construite sur des standards ouverts : OIDC, JMAP, CalDAV, CardDAV et WebDAV. Vos données restent portables.',

  'home.standards.overline': 'Œuvre originale de Sytue',
  'home.standards.title': 'Bâtie sur des standards ouverts',
  'home.standards.body':
    'Chaque produit Sytue est une œuvre originale, écrite en Go et en Astro + Preact sur des protocoles ouverts (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Aucun moteur tiers : vos données sont portables et votre suite, souveraine.',

  'home.cta.title': 'Reprenez la main sur votre travail',
  'home.cta.body':
    'Lancez votre organisation dès aujourd’hui. Migration accompagnée depuis Google Workspace ou Microsoft 365.',
  'home.cta.primary': 'Créer mon organisation',
  'home.cta.secondary': 'Parler à un commercial',

  // --- Rejilla de productos ----------------------------------------------
  'grid.discover': 'Découvrir',
  'product.mail.blurb': 'Messagerie privée et européenne en JMAP, sans traqueurs ni publicité.',
  'product.drive.blurb': 'Fichiers et collaboration, aux couleurs de votre organisation.',
  'product.docs.blurb': 'Documents, tableurs et présentations en coédition temps réel.',
  'product.calendar.blurb': 'Un agenda CalDAV relié au reste de la suite.',
  'product.contacts.blurb': 'Contacts CardDAV, partagés dans votre organisation.',
  'product.notes.blurb':
    'Des carnets qui gardent tout : texte, fichiers, événements, documents entiers.',
  'product.photos.blurb':
    'Galerie et éditeur de photos : vos originaux dans Drive, vos réglages sous forme de recette.',
  'product.rooms.blurb': 'Canaux, messages et visioconférences d’équipe au même endroit.',
  'product.cast.blurb':
    'Vidéo à la demande et direct : téléverser, diffuser et partager, dans l’UE.',
  'product.pages.blurb':
    'Sites web et boutiques en ligne pour votre entreprise, conçus à votre mesure et publiés dans l’UE.',
  'product.bases.blurb':
    'Tables relationnelles collaboratives pour clients, commandes ou stocks, dans l’UE.',
  'product.cloud.blurb': 'Plateforme de déploiement et de données gérées, dans l’UE.',
  'product.id.blurb': 'Une identité unique (SSO + MFA) avec Sytue ID.',
  'product.one.blurb':
    'Votre page d’accueil de la suite : applications, accès rapides, activité récente et avis.',
  'product.sheets.blurb':
    'Des feuilles de calcul avec formules, données externes et graphiques, éditées à plusieurs mains.',
  'product.slides.blurb': 'Des présentations à votre marque, avec thèmes et vue du présentateur.',
  'product.pdf.blurb': 'Lisez, annotez, signez et ordonnez vos PDF sans qu’ils quittent l’Europe.',
  'product.console.blurb':
    'Administration de l’organisation : personnes, accès, conformité et audit.',
  'product.erp.blurb':
    'Vingt-quatre modules et un livre en partie double que la base de données refuse de déséquilibrer.',

  // --- Fichas de producto -------------------------------------------------
  'productPage.back': 'Toute la suite',
  'productPage.features': 'Tout ce que fait {name}',
  'productPage.featuresPlanned': 'Ce que fera {name}',
  'productPage.builtOn': 'Bâti sur {standard}',
  'productPage.fromScratch':
    "L'éditeur de Docus, Calcus, Diapos et CertiPDF est écrit depuis zéro : un moteur propre à Sytue, pas une intégration tierce.",
  'productPage.shot': 'L’écran principal de {name}, tel qu’il apparaît à la connexion.',
  'productPage.shotFeatures': '{name} en action, ses fonctions à l’écran.',
  'productPage.open': 'Ouvrir {name}',
  'productPage.enDesarrollo': 'Ce produit est en développement : il n’est pas encore disponible.',
  'productPage.included':
    'Inclus dans Sytue One : une seule identité (Sytue ID) et une seule inscription donnent accès à tous les produits. Données 100 % dans l’UE (Scaleway) et portables grâce aux standards ouverts.',

  'product.mail.tagline': 'Une messagerie privée et européenne, sans traqueurs ni publicité.',
  'product.mail.standard': 'JMAP (standard ouvert)',
  'product.mail.p1': 'Boîte de réception rapide : fils, libellés, filtres et recherche.',
  'product.mail.p2': 'Joignez des fichiers directement depuis Sytue Drive.',
  'product.mail.p3': 'Transformez un e-mail en évènement Calendar.',
  'product.mail.p4':
    'Chiffrement en transit (TLS 1.3). L’infrastructure de Sytue se trouve dans l’UE. La localisation des services externes dépend de leur fournisseur.',

  'product.drive.tagline':
    'Vos fichiers et votre collaboration, aux couleurs de votre organisation.',
  'product.drive.standard': 'WebDAV (standard ouvert)',
  'product.drive.p1': 'Déposez, organisez et partagez par liens maîtrisés (droits et expiration).',
  'product.drive.p2': 'Aperçu en ligne des documents et des images.',
  'product.drive.p3': 'Joignez à Mail sans dupliquer le fichier.',
  'product.drive.p4': 'Stockage adossé à S3, en régions européennes.',

  'product.docs.tagline': 'Documents, tableurs et présentations en coédition temps réel.',
  'product.docs.standard': 'Coédition en temps réel',
  'product.docs.p1': 'Écrivez à plusieurs, avec présence et modifications instantanées.',
  'product.docs.p2': 'Tableurs avec formules et mise en forme conditionnelle.',
  'product.docs.p3': 'Présentations à l’identité de votre marque.',
  'product.docs.p4': 'Tout est enregistré dans votre Drive européen.',

  'product.calendar.tagline': 'Votre agenda, relié au reste de la suite.',
  'product.calendar.standard': 'CalDAV (standard ouvert)',
  'product.calendar.p1': 'Vues jour, semaine et mois.',
  'product.calendar.p2': 'Évènements récurrents et rappels.',
  'product.calendar.p3': 'Créez un évènement directement depuis un e-mail dans Mail.',
  'product.calendar.p4': 'Compatible avec n’importe quel client CalDAV.',

  'product.contacts.tagline': 'Les contacts de votre organisation, partagés et portables.',
  'product.contacts.standard': 'CardDAV (standard ouvert)',
  'product.contacts.p1': 'Carnet d’adresses partagé dans toute l’organisation.',
  'product.contacts.p2': 'Import et export au format vCard.',
  'product.contacts.p3': 'Fusionnez les doublons sans rien perdre.',
  'product.contacts.p4': 'Compatible avec n’importe quel client CardDAV.',
  'product.notes.tagline': 'Tout ce que vous voulez garder, au même endroit et entre vos mains.',
  'product.notes.standard': 'Modèle propre sur des standards ouverts (API REST propre)',
  'product.notes.p1': 'Carnets imbriqués, en toile libre, en colonnes ou en liste.',
  'product.notes.p2':
    'Des notes de tout type : texte, tâches, liens, fichiers, événements d’Agenda, documents Docs.',
  'product.notes.p3':
    'Chiffrement de bout en bout : la phrase secrète ne quitte pas votre navigateur.',
  'product.notes.p4': 'Vous décidez ce qui entre dans la recherche, et ce qui n’y entre jamais.',

  'product.rooms.tagline': 'Canaux, messages et visioconférences d’équipe, réunis.',
  'product.rooms.standard': 'Messagerie et appels en temps réel',
  'product.rooms.p1': 'Des canaux par équipe ou par projet.',
  'product.rooms.p2': 'Une visioconférence intégrée à chaque canal.',
  'product.rooms.p3': 'Mentions et notifications unifiées dans toute la suite.',
  'product.rooms.p4': 'Historique consultable depuis tout Sytue One.',

  'product.cloud.p4': 'Domaines personnalisés avec TLS automatique.',

  'product.cloud.p3': 'Stockage S3 et fonctions gérées.',
  'product.cloud.p2': 'Base Postgres avec API REST automatique.',
  'product.cloud.p1': 'Une préversion à chaque PR et un retour arrière instantané.',
  'product.cloud.standard': 'PaaS native Kubernetes',
  'product.cloud.tagline': 'Votre code déployé et vos données gérées, dans l’Union européenne.',
  'product.id.tagline': 'Une identité pour toute la suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (standard ouvert)',
  'product.id.p1': 'Authentification unique sur tous les produits.',
  'product.id.p2': 'MFA et clés d’accès (WebAuthn).',
  'product.id.p3': 'Provisionnement automatique (SCIM) depuis votre annuaire.',
  'product.id.p4': 'Profil et sécurité au même endroit.',
  'product.one.tagline': 'Tout Sytue One en un clic, par la même porte.',
  'product.one.standard': 'Une seule connexion (Sytue ID) pour toute la suite',
  'product.one.p1':
    'Un lanceur avec toutes vos applications et celles que votre organisation connecte.',
  'product.one.p2': 'Recherche dans toute la suite (⌘K) depuis n’importe quel écran.',
  'product.one.p3':
    'Votre agenda du jour et le courrier non lu en page d’accueil, à côté des éléments récents de toute la suite et des notifications.',
  'product.one.p4': 'Connectez vos appareils et téléchargez les applications natives.',
  'product.sheets.tagline':
    'Des chiffres qui se comprennent, dans une feuille qui travaille avec vous.',
  'product.sheets.standard': 'Formats ODF/OOXML et édition collaborative en temps réel',
  'product.sheets.p1': 'Formules, mise en forme conditionnelle et validation des données.',
  'product.sheets.p2': 'Importez des données externes et interrogez-les sans quitter la feuille.',
  'product.sheets.p3': 'Graphiques, graphiques sparkline, segments et tableaux de données.',
  'product.sheets.p4': 'Export en ODS et XLSX ; ouverture des fichiers Excel.',
  'product.slides.tagline': 'Racontez l’histoire ; la diapositive suit.',
  'product.slides.standard': 'Formats ODF/OOXML et édition collaborative en temps réel',
  'product.slides.p1': 'Des thèmes de présentation à l’identité de votre organisation.',
  'product.slides.p2': 'Vue du présentateur avec notes et diapositive suivante.',
  'product.slides.p3': 'Images, formes, graphiques et SmartArt.',
  'product.slides.p4': 'Export en ODP, PPTX et PDF ; ouverture des fichiers PowerPoint.',
  'product.pdf.tagline': 'Le PDF, de bout en bout, dans votre navigateur.',
  'product.pdf.standard': 'PDF (ISO 32000) avec annotations et formulaires standard',
  'product.pdf.p1': 'Surligner, souligner, commenter et dessiner sur le document.',
  'product.pdf.p2': 'Formulaires : remplissez les champs ou créez les vôtres.',
  'product.pdf.p3': 'Fusionner, extraire, pivoter, numéroter et filigraner.',
  'product.pdf.p4':
    'Panneau des signatures, caviardage définitif et documents protégés par mot de passe.',
  'product.console.tagline': 'Gouvernez la suite sans quitter un écran.',
  'product.console.standard': 'SCIM, SAML et OpenID Connect (standards ouverts)',
  'product.console.p1': 'Personnes, groupes (y compris dynamiques) et rôles.',
  'product.console.p2': 'SSO avec votre fournisseur et provisionnement SCIM.',
  'product.console.p3':
    'Règles d’accès conditionnel, appareils et accès privilégié avec approbation.',
  'product.console.p4': 'Audit exportable vers votre SIEM et conservations légales.',
  'product.erp.tagline': 'Tout l’ERP, et le catalogue de modèles tient dans un fichier de données.',
  'product.erp.standard':
    'Comptabilité en partie double, catalogue de modèles en JSON et une seule API REST',
  'product.erp.p1':
    'Vingt-quatre modules et trente-six modèles : contacts, ventes, achats, stocks, production, facturation, comptabilité, projets et RH.',
  'product.erp.p2':
    'Ajouter un module, c’est écrire un JSON : sans migration, sans nouvelle route et sans toucher à l’interface.',
  'product.erp.p3':
    'Les montants sont en centimes entiers, jamais en virgule flottante, et l’équilibre est imposé par la base.',
  'product.erp.p4':
    'Ce qui est comptabilisé ne se retouche pas : on corrige par une autre écriture, ce qui distingue un livre d’un tableur.',
  'product.one.f1':
    'Un lanceur avec les applications de la suite et les applications tierces que votre organisation connecte.',
  'product.one.f2':
    'Accès rapides : écrire un courriel, nouveau document, envoyer vers Drive, créer un événement.',
  'product.one.f3':
    'Récents de toute la suite : vos dernières actions dans Drive, Docus, Mail ou Rooms.',
  'product.one.f4':
    'Recherche globale avec ⌘K ou Ctrl K, avec des résultats de toutes les applications.',
  'product.one.f5': 'Centre de notifications et présence partagés par toute la suite.',
  'product.one.f6':
    'Connectez vos appareils : profil pour iPhone et Mac, Android et bureau, en deux étapes.',
  'product.one.f7': 'Téléchargement des applications natives de bureau et mobiles.',
  'product.one.f8': 'Un assistant de bienvenue qui prépare l’organisation sans manuel.',
  'product.mail.f1': 'Fils de conversation avec lecture, réponse et transfert en un clic.',
  'product.mail.f2':
    'Dossiers, étiquettes de couleur et recherche avec opérateurs (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Règles automatiques qui archivent, étiquettent ou déplacent le courrier à l’arrivée.',
  'product.mail.f4': 'Envoi programmé et « Annuler l’envoi ».',
  'product.mail.f5': 'Signatures, alias d’adresse et réponse automatique d’absence.',
  'product.mail.f6': 'Boîtes partagées et déléguées avec droits de lecture et d’envoi.',
  'product.mail.f7':
    'Éditeur de texte enrichi avec pièces jointes depuis Sytue Drive sans dupliquer les fichiers.',
  'product.mail.f8':
    'Contenu distant bloqué par défaut : personne ne sait quand vous ouvrez un courriel.',
  'product.mail.f9': 'Courrier signé (PGP et S/MIME) reconnu et signalé ; TLS en transit.',
  'product.mail.f10':
    'Accès aussi depuis Thunderbird, Outlook, Apple Mail ou le mobile, avec des mots de passe d’application.',
  'product.mail.f11':
    'Adresses en pastilles avec le carnet d’adresses de Contacts à un clic, et alerte si vous dites « ci-joint » sans rien joindre.',
  'product.mail.f12':
    'Règles depuis le courriel ouvert, vider la corbeille d’un coup et raccourcis clavier avec « ? ».',
  'product.drive.f1':
    'Mon espace, espaces d’équipe, partagés avec moi, récents, favoris et corbeille.',
  'product.drive.f2':
    'Envoi par glisser-déposer ; dossiers, déplacer, copier, renommer et sélection multiple.',
  'product.drive.f3':
    'Liens de partage à deux niveaux (consulter ou modifier), mot de passe et expiration.',
  'product.drive.f4': 'Historique des versions avec restauration.',
  'product.drive.f5': 'Aperçu des documents, images et PDF sans téléchargement.',
  'product.drive.f6':
    'Ouvrez et modifiez les documents bureautiques dans Docus, Calcus et Diapos ; les PDF dans CertiPDF.',
  'product.drive.f7':
    'Étiquettes de confidentialité qui interdisent les liens publics là où il ne faut pas.',
  'product.drive.f8': 'Partagez vers un canal Rooms ou joignez à Mail sans dupliquer le fichier.',
  'product.drive.f9': 'Accès WebDAV et client de bureau pour voir vos fichiers sur l’ordinateur.',
  'product.drive.f10': 'Stockage S3 dans des régions européennes, avec le quota toujours visible.',
  'product.docs.f1':
    'Édition collaborative en temps réel, avec présence et sans se marcher dessus.',
  'product.docs.f2':
    'Un ruban à onglets : Accueil, Insertion, Références, Publipostage et Révision.',
  'product.docs.f3': 'Tableaux, images, formes, SmartArt, graphiques, symboles et filigranes.',
  'product.docs.f4':
    'Références : table des matières, notes de bas de page, citations et bibliographie.',
  'product.docs.f5': 'Publipostage : fusion de données pour lettres et étiquettes.',
  'product.docs.f6': 'Commentaires en fil, révision des modifications et versions.',
  'product.docs.f7':
    'Assistant IA optionnel pour réécrire, traduire ou résumer, seulement si votre organisation l’active.',
  'product.docs.f8': 'Export en ODT, DOCX et PDF ; ouverture des fichiers Word depuis Drive.',
  'product.docs.f9': 'En-têtes, pieds de page, numérotation et blocs réutilisables.',
  'product.docs.f10': 'Enregistrement automatique dans votre Drive européen, avec historique.',
  'product.docs.f11':
    'Téléchargez depuis la page d’accueil en PDF, HTML, Markdown, OpenDocument ou Microsoft Office.',
  'product.sheets.f1': 'Formules avec aide contextuelle et recalcul instantané.',
  'product.sheets.f2':
    'Mise en forme conditionnelle, validation des données et formats numériques régionaux.',
  'product.sheets.f3': 'Graphiques de plusieurs types, sparklines et segments.',
  'product.sheets.f4': 'Tableaux de données avec totaux et tri ; tableau croisé dynamique.',
  'product.sheets.f5':
    'Données externes : connectez une source et interrogez-la depuis la feuille.',
  'product.sheets.f6': 'Solveur pour trouver la valeur qui atteint un objectif.',
  'product.sheets.f7': 'Macros pour automatiser les tâches répétitives.',
  'product.sheets.f8': 'Collaboration en temps réel avec présence et commentaires.',
  'product.sheets.f9': 'Filtres, segments et recherche dans la feuille.',
  'product.sheets.f10': 'Export en ODS, XLSX et PDF ; ouverture des fichiers Excel depuis Drive.',
  'product.slides.f1': 'Thèmes et palettes cohérents avec la marque de votre organisation.',
  'product.slides.f2': 'Un canevas avec formes, images, graphiques, SmartArt et connecteurs.',
  'product.slides.f3': 'Présentez en plein écran avec vue du présentateur et notes.',
  'product.slides.f4': 'Collaboration en temps réel avec présence et commentaires.',
  'product.slides.f5': 'Diapositives avec arrière-plan, affiches et dispositions.',
  'product.slides.f6': 'Export en ODP, PPTX et PDF.',
  'product.slides.f7': 'Ouverture des fichiers PowerPoint depuis Drive.',
  'product.slides.f8': 'Enregistrement automatique avec versions.',
  'product.pdf.f1':
    'Une visionneuse rapide avec plan, vignettes, recherche et zoom à la page ou à la largeur.',
  'product.pdf.f2':
    'Annotations : surligner, souligner, barrer, commenter, dessiner, formes, flèches et tampons.',
  'product.pdf.f3': 'Zones de texte, mesures et captures de zone.',
  'product.pdf.f4': 'Formulaires : remplissez les champs existants ou ajoutez les vôtres.',
  'product.pdf.f5': 'Panneau des signatures du document.',
  'product.pdf.f6':
    'Caviardage définitif : ce qui est marqué disparaît du fichier, pas seulement de l’affichage.',
  'product.pdf.f7':
    'Pages : fusionner, extraire, supprimer, pivoter, numéroter (Bates aussi), en-têtes et pieds de page.',
  'product.pdf.f8': 'Filigrane, arrière-plan et métadonnées modifiables.',
  'product.pdf.f9': 'Documents protégés par mot de passe et permissions.',
  'product.pdf.f10': 'Lecture à voix haute et commentaires en fil.',
  'product.calendar.f1':
    'Vues mois, semaine, jour et agenda, avec mini-calendrier et « Aujourd’hui ».',
  'product.calendar.f2':
    'Répétition quotidienne, hebdomadaire ou mensuelle avec fin ; modifiez celui-ci, celui-ci et les suivants, ou tous.',
  'product.calendar.f3': 'Rappels par événement et notifications dans la suite.',
  'product.calendar.f4': 'Invitations avec réponse (présent, peut-être, non) et suivi.',
  'product.calendar.f5': '« Trouver un créneau » libre pour toutes les personnes invitées.',
  'product.calendar.f6': 'Salles et ressources réservables avec disponibilité.',
  'product.calendar.f7':
    'Calendriers partagés avec permissions, et ceux que d’autres partagent avec vous.',
  'product.calendar.f8':
    'Abonnement à des calendriers externes (ICS) et plusieurs calendriers en couleur.',
  'product.calendar.f9': 'Créez un événement depuis un courriel de Mail ou depuis une note.',
  'product.calendar.f10':
    'Compatible avec tout client CalDAV : Apple Calendrier, Thunderbird et plus.',
  'product.calendar.f11':
    'Dupliquez un événement ou copiez-le dans votre calendrier, et téléchargez-le en .ics pour les personnes qui ne sont pas sur Sytue.',
  'product.contacts.f1': 'Carnets d’adresses, y compris celui partagé de votre organisation.',
  'product.contacts.f2':
    'Fiche complète : téléphones, courriels, adresses, organisation, anniversaire, notes et photo.',
  'product.contacts.f3': 'Étiquettes et favoris pour regrouper et retrouver.',
  'product.contacts.f4': 'Détection et fusion des doublons sans perte de données.',
  'product.contacts.f5': 'Import et export vCard et CSV ; export d’un contact seul.',
  'product.contacts.f6': 'Actions par lot et corbeille avec récupération.',
  'product.contacts.f7': 'Recherche instantanée et saisie automatique dans Mail.',
  'product.contacts.f8': 'Compatible avec tout client CardDAV.',
  'product.contacts.f9': 'Prochains anniversaires de votre carnet, avec ceux du mois en évidence.',
  'product.contacts.f10':
    'Créez une réunion avec un contact ou copiez son adresse sans quitter sa fiche.',
  'product.notes.f1':
    'Carnets imbriqués avec icône et couleur ; trois dispositions : libre, colonnes et liste.',
  'product.notes.f2':
    'Douze types de notes : texte, tâches, liens, couleur, image et références vers Drive, Docus, Calendar, Rooms et d’autres notes.',
  'product.notes.f3':
    'Un éditeur avec mise en forme (gras, listes, liens) qui enregistre à la sortie.',
  'product.notes.f4':
    'Étiquettes, épingler, dupliquer, déplacer entre carnets, grouper et glisser sur le canevas.',
  'product.notes.f5': 'Visibilité par carnet : privé, personnel, organisation ou lien public.',
  'product.notes.f6':
    'Chiffrement dans votre navigateur pour les carnets sensibles : Sytue ne peut pas les lire.',
  'product.notes.f7': 'Vous décidez quels carnets entrent dans la recherche de la suite.',
  'product.notes.f8': 'Rétroliens : voyez quelles notes pointent vers celle-ci.',
  'product.notes.f9':
    'Exportez un carnet entier en HTML, PDF, Markdown et plus ; importez des copies.',
  'product.notes.f10':
    'Partagez une note vers un canal Rooms ; déposez ou collez pour créer des notes.',
  'product.notes.f11':
    'Tableaux dans l’éditeur et raccourcis Markdown en écrivant : « - », « # », « > » et « ``` ».',
  'product.notes.f12':
    'Déplacez une note vers un autre carnet, exportez une seule note et triez le carnet à votre guise.',
  'product.photos.tagline': 'Vos photos, en ordre et à votre goût, sans quitter l’Europe.',
  'product.photos.standard': 'Originaux dans Sytue Drive ; moteur d’édition dans votre navigateur',
  'product.photos.p1': 'Une frise par mois avec recherche par nom.',
  'product.photos.p2': 'Albums avec couverture ; visionneuse plein écran.',
  'product.photos.p3':
    'Un éditeur avec lumière, couleur, rotation, miroir et recadrage, en temps réel.',
  'product.photos.p4': 'Chaque retouche est une recette : l’original n’est jamais touché.',
  'product.photos.f1': 'Une frise groupée par mois, avec recherche par nom et vignettes nettes.',
  'product.photos.f2':
    'Albums : créer, renommer, choisir la couverture, ajouter depuis la visionneuse et retirer.',
  'product.photos.f3': 'Visionneuse plein écran avec navigation au clavier.',
  'product.photos.f4':
    'Un éditeur à quatre curseurs (exposition, contraste, saturation, température), rotation et miroir.',
  'product.photos.f5':
    'Recadrage par glissement et raccourcis clavier ; aperçu GPU avec repli CPU.',
  'product.photos.f6':
    'Chaque retouche est une recette enregistrée à part : l’original de Drive n’est jamais modifié.',
  'product.photos.f7':
    'Export en pleine résolution, calculé sur le serveur avec les mêmes formules que celles affichées.',
  'product.photos.f8': 'Corbeille avec restauration.',
  'product.photos.f9':
    'Les originaux vivent dans votre Sytue Drive ; les dérivés sont un cache régénérable.',
  'product.photos.f10':
    'Données EXIF lues dans le fichier lui-même ; tout dans l’Union européenne.',
  'product.photos.f11':
    'Sélection multiple par mois : vers un nouvel album, vers la corbeille ou téléchargées d’un coup.',
  'product.photos.f12':
    'Diaporama, taille réelle, rotation depuis la visionneuse et recadrage à proportion fixe.',
  'product.rooms.f1': 'Canaux publics et privés en catégories ; messages directs et groupes.',
  'product.rooms.f2': 'Fils, réactions, mentions, épinglés, enregistrés et brouillons.',
  'product.rooms.f3':
    'Mise en forme avec la barre « Aa » et des raccourcis : gras, italique, code, citations, listes et liens.',
  'product.rooms.f4': 'Pièces jointes, images, GIF et emoji personnalisés ; fichiers depuis Drive.',
  'product.rooms.f5': 'Envoi programmé, priorité, accusé de réception et « vu par ».',
  'product.rooms.f6': 'Commandes « / » et cartes : sondages et tâches attribuables.',
  'product.rooms.f7':
    'Appels vidéo dans le canal : caméra, écran, main levée, chat et enregistrement.',
  'product.rooms.f8': 'Réunions programmées et liens d’invitation.',
  'product.rooms.f9': 'Bots, webhooks et intégrations avec d’autres outils.',
  'product.rooms.f10': 'Présence et statut personnalisé ; notifications par canal sur mesure.',
  'product.rooms.f11': 'Recherche dans l’historique et depuis toute la suite.',
  'product.rooms.f12': 'Temps réel par WebSocket, avec reconnexion sans perdre de messages.',
  'product.rooms.f13':
    'Sélecteur rapide de canaux avec Ctrl+G, tout marquer comme lu et raccourcis avec « ? ».',
  'product.cast.tagline':
    'Vos vidéos et vos directs, à la demande et en temps réel, sans quitter l’Europe.',
  'product.cast.standard':
    'Lecture HLS dans le navigateur ; ingestion RTMP depuis tout logiciel de diffusion',
  'product.cast.p1':
    'Téléversez une vidéo : elle est transcodée en plusieurs qualités pour tout écran.',
  'product.cast.p2':
    'Passez en direct depuis un logiciel de diffusion, le navigateur ou l’application.',
  'product.cast.p3':
    'Quatre niveaux de visibilité : public, non répertorié, organisation ou personnes précises.',
  'product.cast.p4':
    'Tchat en direct et commentaires, avec une modération de base par la personne qui publie.',
  'product.cast.f1': 'Téléversez des vidéos jusqu’à 8 Gio ; l’original est conservé.',
  'product.cast.f2':
    'Lecture adaptative en trois qualités, le navigateur choisissant la meilleure.',
  'product.cast.f3':
    'Direct depuis un logiciel de diffusion via RTMP, avec une clé de flux par personne.',
  'product.cast.f4':
    'Direct depuis le navigateur ou l’application : caméra et micro. Partage d’écran dans les navigateurs, systèmes de bureau et Android compatibles ; indisponible dans l’app iOS/iPadOS.',
  'product.cast.f5':
    'Les enregistrements des directs deviennent automatiquement des vidéos à la demande.',
  'product.cast.f6': 'Une page d’accueil publique et une chaîne par organisation, indexables.',
  'product.cast.f7':
    'Liens non répertoriés, vidéos réservées à l’organisation ou accès à des personnes précises par @identifiant.',
  'product.cast.f8':
    'Tchat en direct et commentaires en fil, modérés par la personne qui publie et l’administration de l’organisation.',
  'product.cast.f9': 'Lecteur intégrable et oEmbed pour les vidéos publiques et non répertoriées.',
  'product.cast.f10':
    'Le stockage est décompté de votre quota Sytue Drive ; tout dans l’Union européenne.',
  'product.cast.f11':
    '« Importer dans Cast » depuis Sytue Drive : une copie de la vidéo part dans Cast et le fichier reste dans Drive.',
  'product.pages.tagline':
    'Le site web de votre entreprise et sa boutique, conçus dans un éditeur visuel et publiés sans quitter l’Europe.',
  'product.pages.standard':
    'HTML et CSS statiques et accessibles, sans cookies tiers ; export vers Astro',
  'product.pages.p1':
    'Un éditeur visuel avec des cadres qui s’adaptent seuls à l’ordinateur, à la tablette et au mobile.',
  'product.pages.p2':
    'Dix modèles par secteur, avec de vrais textes et des photos du domaine public.',
  'product.pages.p3':
    'Une boutique avec catalogue, panier et encaissement sur le compte de votre entreprise.',
  'product.pages.p4':
    'Votre propre domaine avec certificat automatique, ou une adresse sur sites.sytue.eu.',
  'product.pages.f1':
    'Éditeur visuel par cadres avec mise en page automatique, composants réutilisables et sections prêtes à l’emploi.',
  'product.pages.f2':
    'Trois points de rupture (ordinateur, tablette et mobile), chacun avec ses propres réglages.',
  'product.pages.f3': 'Analyse des visites sans cookies, dans le tableau de bord du site.',
  'product.pages.f4':
    'Sites multilingues avec des adresses par langue, hreflang et traduction assistée par Sytue AI.',
  'product.pages.f5':
    'Images depuis votre ordinateur, Sytue Drive ou Sytue Photos, avec un texte alternatif demandé à l’insertion.',
  'product.pages.f6':
    'Boutique avec variantes, stock, TVA par produit et paiement via Mollie, Adyen, Redsys ou Stripe.',
  'product.pages.f7':
    'Commandes avec une facture simplifiée émise par Sytue ERP, et rétractation depuis le site lui-même.',
  'product.pages.f8':
    'Formulaires avec consentement, boîte de réception des messages et notifications aux personnes de votre organisation.',
  'product.pages.f9':
    'Versions nommées, aperçu derrière un lien secret et retour à n’importe quelle publication antérieure.',
  'product.pages.f10':
    'Export du code vers Astro, React, Next.js et des applications natives, et du catalogue vers Shopify, WooCommerce et d’autres plateformes.',
  'product.bases.tagline':
    'Des tables liées pour suivre clients, commandes ou stocks, avec droits d’accès et historique de chaque modification.',
  'product.bases.standard':
    'CSV et JSON ouverts ; schéma et relations garantis par la base de données elle-même',
  'product.bases.p1':
    'Clients, commandes ou stocks dans des tables liées, sans rien connaître aux bases de données.',
  'product.bases.p2': 'Partagez avec votre organisation, avec le rôle de votre choix.',
  'product.bases.p3':
    'Chaque modification est enregistrée : qui l’a faite et quelle était la valeur précédente.',
  'product.bases.p4': 'Vos données restent dans l’UE et vous les exportez quand vous voulez.',
  'product.bases.f1':
    'Tables avec champs texte, nombre, date, case à cocher, choix unique et choix multiple.',
  'product.bases.f2':
    'Relations entre tables : liez une commande à son client et la base de données refuse de rompre le lien.',
  'product.bases.f3':
    'Grille modifiable au clavier, lisible avec un lecteur d’écran et utilisable sur mobile.',
  'product.bases.f4':
    'Droits par base et par table : propriétaire, éditeur, commentateur et lecteur.',
  'product.bases.f5': 'Importez un CSV comme nouvelle table et exportez en CSV ou JSON.',
  'product.bases.f6':
    'Journal de chaque modification : qui, quel enregistrement et quel champ, avant et après.',
  'product.cloud.f12': 'Usage, coûts et facturation européenne dans la même console.',
  'product.cloud.f11': 'Personnes, jetons d’API et audit exportable.',
  'product.cloud.f10': 'Analyse web respectueuse de la vie privée, sans cookies.',
  'product.cloud.f9': 'Tâches planifiées et modèles de projet.',
  'product.cloud.f8': 'Terminal dans le conteneur ; redémarrez ou mettez le service en pause.',
  'product.cloud.f7': 'Journaux en direct, métriques et supervision avec alertes.',
  'product.cloud.f6': 'Variables d’environnement et secrets par environnement.',
  'product.cloud.f5': 'Domaines personnalisés avec TLS automatique ; redirections et en-têtes.',
  'product.cloud.f4': 'Stockage S3 et fonctions gérées.',
  'product.cloud.f3': 'Bases Postgres avec API REST automatique et éditeur SQL.',
  'product.cloud.f2': 'Un aperçu par PR et retour à n’importe quel déploiement en un clic.',
  'product.cloud.f1':
    'Projets depuis votre dépôt (GitHub, GitLab ou votre Git) avec un déploiement à chaque push.',
  'product.console.f1':
    'Un tableau de bord avec l’état de l’organisation et ce qui mérite un coup d’œil aujourd’hui.',
  'product.console.f2':
    'Comptes : création, suspension, réinitialisation du mot de passe et de la MFA, sessions, alias et boîte.',
  'product.console.f3': 'Groupes statiques et dynamiques par règles ; rôles avec permissions.',
  'product.console.f4': 'Accès privilégié (PIM) : activation temporaire avec approbation.',
  'product.console.f5':
    'SSO avec des fournisseurs SAML ou OIDC, avec test de connexion ; provisionnement SCIM.',
  'product.console.f6': 'Règles d’accès conditionnel et inventaire des appareils de confiance.',
  'product.console.f7':
    'Intégrations : clients OIDC, SAML et connexions tierces ; substitution de produits.',
  'product.console.f8': 'Importer du contenu : migration depuis d’autres fournisseurs.',
  'product.console.f9': 'Éditeur de documents et canaux Rooms administrables.',
  'product.console.f10':
    'Gouvernance : conservations légales, étiquettes de confidentialité et politique de mots de passe.',
  'product.console.f11': 'Audit avec export CSV et envoi vers votre SIEM ; journaux de service.',
  'product.console.f12':
    'Domaine et marque propres : logo de l’organisation et domaine de courrier.',
  'product.id.f1':
    'Authentification unique (OpenID Connect et OAuth 2.0 avec PKCE) pour la suite et vos applications.',
  'product.id.f2': 'Clés d’accès (WebAuthn) et MFA par TOTP avec codes de récupération.',
  'product.id.f3': 'Mots de passe d’application pour IMAP et les clients qui ne parlent pas OIDC.',
  'product.id.f4': 'Sessions et appareils : révoquez-en un ou tous les autres.',
  'product.id.f5':
    'Fédération SAML avec votre IdP, et Sytue comme IdP SAML pour d’autres applications.',
  'product.id.f6': 'Provisionnement SCIM depuis votre annuaire.',
  'product.id.f7': 'Invitations, vérification du courriel et récupération du mot de passe.',
  'product.id.f8': 'Profil pour iPhone et Mac (.mobileconfig) en un geste.',
  'product.id.f9': 'Politiques de mot de passe et règles d’accès définies par l’organisation.',
  'product.id.f10': 'Enregistrement dynamique des clients (DCR) et découverte standard.',
  'product.erp.f1':
    'Vingt-quatre modules et trente-six modèles dans un seul service et une seule base de données.',
  'product.erp.f2':
    'Le catalogue de modèles est une donnée : chaque module déclare ses champs, ses états et ses colonnes.',
  'product.erp.f3':
    'Un nouveau module tient dans un fichier JSON — sans DDL, sans migration et sans verrou de table.',
  'product.erp.f4':
    'Une seule API pour les trente-six modèles, et une interface engendrée depuis le catalogue.',
  'product.erp.f5':
    'Comptabilité en partie double avec des montants en centimes entiers ; pas un seul flottant.',
  'product.erp.f6':
    'L’équilibre est vérifié par la base au moment de valider la transaction, pas par le code applicatif.',
  'product.erp.f7':
    'Les écritures clôturées sont immuables : ni modification, ni suppression, ni ligne supplémentaire.',
  'product.erp.f8':
    'Un document, une écriture : une nouvelle tentative ou deux onglets ne peuvent pas dupliquer un produit.',
  'product.erp.f9':
    'Les libellés communs existent dans les 26 langues, et un champ non traduit empêche le démarrage.',
  'product.erp.f10':
    'Listes servies par index : la première page de vingt mille lignes arrive en moins d’une milliseconde.',

  // --- Descargas ----------------------------------------------------------
  'meta.downloads.title': 'Téléchargements — Sytue One pour ordinateur et mobile',
  'meta.downloads.description':
    'Applications natives Sytue One pour macOS, Windows, Linux, iOS et Android. Toute la suite sur votre machine, avec les intégrations propres à chaque système.',
  'downloads.overline': 'Applications',
  'downloads.h1': 'Sytue One sur votre machine',
  'downloads.intro':
    'De vraies applications de bureau et mobiles : elles s’installent, vivent dans le Dock ou le menu Démarrer, vous alertent par les notifications du système et ouvrent les fichiers de votre machine.',
  'downloads.desktop.title': 'Ordinateur',
  'downloads.desktop.body':
    'Une application native par produit, qui dessine elle-même son interface avec la carte graphique : pas de navigateur à l’intérieur, elle démarre vite et consomme comme une application du système.',
  'downloads.apps.title': "Seulement ce qu'il vous faut",
  'downloads.apps.body':
    'Sytue One apporte toute la journée de travail. Si vous ne voulez que le disque, Sytue Drive s’installe séparément.',
  'downloads.includes': 'Comprend :',
  'downloads.mobile.title': 'Mobile',
  'downloads.mobile.body':
    'Application native iOS et Android, avec notifications système, partage et accès aux fichiers et à l’appareil photo.',
  'downloads.selfhost.title': 'Votre organisation est sur son propre domaine ?',
  'downloads.selfhost.body':
    'Les applications natives se connectent à sytue.eu. Si votre organisation a sa propre installation, connectez-vous avec le navigateur sur son domaine : la suite fonctionne de la même façon sur le web.',
  'downloads.yourSystem': 'Votre système',
  'downloads.signInToGet': 'Connectez-vous pour télécharger',
  'downloads.store': 'Voir sur {format}',
  'downloads.soon': 'Bientôt sur {format}',
  'downloads.browser.text':
    'Vous préférez le navigateur ? Toute la suite fonctionne à l’identique sur le web, sans rien installer :',
  'downloads.browser.link': 'ouvrir Sytue One',
  'platform.macos.req': 'macOS 11 ou ultérieur · Apple Silicon et Intel',
  'platform.windows.req': 'Windows 10 ou ultérieur · 64 bits',
  'platform.linux.req': 'OpenGL 2.1 · 64 bits',
  'platform.ios.req': 'iOS 14 ou ultérieur',
  'platform.android.req': 'Android 6.0 ou ultérieur',

  // --- Pie ----------------------------------------------------------------
  'footer.tagline':
    'Chaque application de la suite est notre propre service, bâti sur des protocoles ouverts et publiés.',
  'footer.products': 'Produits',
  'footer.resources': 'Ressources',
  'footer.legal': 'Mentions légales',
  'footer.suite': 'La suite',
  'footer.downloads': 'Télécharger les applications',
  'footer.sales': 'Parler à un commercial',
  'footer.privacy': 'Confidentialité et RGPD',
  'footer.accessibility': 'Accessibilité',
  'footer.openSource': 'Open source',
  'footer.legalNotice': 'Mentions légales',
  'footer.terms': 'Conditions',
  'footer.copyright':
    '© {year} Sytue · Données hébergées dans l’UE (Scaleway fr-par / nl-ams · on-premise Madrid).',
  'footer.privacyNotice':
    'Nous ne vendons pas vos données et ne les cédons pas à des tiers. Nous ne les utilisons ni pour vous profiler, ni pour la publicité, ni pour entraîner ou améliorer nos services ou ceux d’autrui. Elles sont chiffrées en transit et au repos, et ne quittent notre infrastructure européenne que pour le strict nécessaire de la facturation, pour vous notifier lorsque l’application est fermée (le service de push ne voit que votre appareil et l’heure, jamais le contenu) et dans les intégrations que vous activez, où le tiers vous demandera votre accord sur les données utilisées.',

  // --- 404 ----------------------------------------------------------------
  'meta.notFound.title': 'Page introuvable — Sytue One',
  'notFound.overline': 'Erreur 404',
  'notFound.title': 'Nous n’avons pas trouvé cette page',
  'notFound.body': 'Le lien a peut-être changé. Revenez à l’accueil ou explorez la suite.',
  'notFound.home': 'Revenir à l’accueil',

  // --- Legal: privacidad ---------------------------------------------------
  'meta.legal.title': 'Mentions légales — Sytue One',
  'meta.legal.description':
    'Coordonnées de l’éditeur de www.sytue.eu et du service Sytue One : Sytue Projects, S.L.',
  'aviso.title': 'Mentions légales',
  'aviso.intro':
    'Conformément à l’article 10 de la loi espagnole 34/2002 du 11 juillet sur les services de la société de l’information et le commerce électronique (LSSI), voici les coordonnées de l’éditeur de ce site et du service Sytue One.',
  'aviso.review.body':
    'Les données d’immatriculation de cet avis ont été vérifiées au registre du commerce. Le reste du texte est en cours de relecture juridique ; sa rédaction peut changer sans que change ce qu’il décrit.',
  'aviso.owner.title': 'Éditeur',
  'aviso.owner.name': 'Dénomination sociale',
  'aviso.owner.form': 'Forme juridique',
  'aviso.owner.formValue': 'Société à responsabilité limitée unipersonnelle de droit espagnol',
  'aviso.owner.nif': 'Numéro fiscal (NIF)',
  'aviso.owner.address': 'Siège social',
  'aviso.owner.registry': 'Immatriculation',
  'aviso.owner.registryValue':
    'Registre du commerce de Madrid (Registro Mercantil de Madrid), section 8, feuillet M-887153, inscription 1',
  'aviso.owner.email': 'Courriel',
  'aviso.owner.web': 'Site web',
  'aviso.scope.title': 'Objet',
  'aviso.scope.body':
    'Ce site présente la suite Sytue One et donne accès à ses applications. L’usage du service est régi par les Conditions d’utilisation, et le traitement des données personnelles par la Politique de confidentialité, toutes deux accessibles en pied de page.',
  'aviso.ip.title': 'Propriété intellectuelle et industrielle',
  'aviso.ip.body':
    'Les contenus de ce site (textes, design, code, marques et logos Sytue et Sytue One) appartiennent à Sytue Projects, S.L. ou sont utilisés sous licence. Les noms de produits tiers cités comme références de compatibilité sont des marques de leurs titulaires.',
  'aviso.liability.title': 'Responsabilité',
  'aviso.liability.body':
    'Sytue Projects, S.L. s’efforce de maintenir ce site disponible et à jour, sans garantir l’absence d’interruptions ou d’erreurs. Elle ne répond pas du contenu des sites tiers vers lesquels elle renvoie.',
  'aviso.law.title': 'Droit applicable',
  'aviso.law.body':
    'Cet avis est régi par le droit espagnol. Si vous êtes consommateur, vous conservez les droits et la juridiction que vous reconnaît la législation de votre pays de résidence dans l’Union européenne.',
  'aviso.updated': 'Dernière mise à jour : septembre 2026.',
  'meta.accessibility.title': 'Déclaration d’accessibilité — Sytue One',
  'meta.accessibility.description':
    'Le niveau d’accessibilité de la suite Sytue One et de ce site, ce que nous mesurons, ce qui manque encore et comment nous le signaler.',
  'a11y.title': 'Déclaration d’accessibilité',
  'a11y.intro':
    'Sytue veut que toute personne puisse utiliser Sytue One et ce site, quelle que soit la technologie d’assistance dont elle a besoin. Cette déclaration couvre www.sytue.eu et les applications web de la suite.',
  'a11y.standard.title': 'Norme de référence',
  'a11y.standard.body':
    'Nous concevons et développons selon les WCAG 2.1 niveau AA, le niveau visé par la norme européenne EN 301 549 et par l’Acte législatif européen sur l’accessibilité (directive (UE) 2019/882).',
  'a11y.status.title': 'État de conformité',
  'a11y.status.body':
    'Partiellement conforme : l’essentiel de la suite respecte la norme, et les parties listées plus bas ne la respectent pas encore entièrement. Aucun audit indépendant n’a encore été commandé.',
  'a11y.measures.title': 'Ce que nous faisons pour y arriver',
  'a11y.measures.design':
    'Un seul système de design pour toute la suite : couleurs de texte au contraste vérifié, anneau de focus visible sur chaque contrôle et utilisation au clavier des menus, dialogues et listes.',
  'a11y.measures.ci':
    'Des audits d’accessibilité automatisés (Lighthouse) dans la chaîne d’intégration à chaque modification du site public.',
  'a11y.measures.system':
    'L’interface suit les réglages de votre système : thème sombre ou clair, réduction des animations et taille du texte, sans taille fixe en pixels sur les champs de formulaire.',
  'a11y.measures.text':
    'Chaque image, icône et état vide a une alternative textuelle, et chaque page déclare sa langue pour que les lecteurs d’écran la prononcent correctement.',
  'a11y.limits.title': 'Limites connues',
  'a11y.limits.body':
    'La grille du tableur, le canevas des présentations et l’annotation de PDF sont des éditeurs complexes qui demandent encore du travail pour être pleinement utilisables au clavier et au lecteur d’écran. Les documents et fichiers déposés par les personnes sont affichés tels quels, sans correction de notre part.',
  'a11y.feedback.title': 'Dites-nous ce qui ne fonctionne pas',
  'a11y.feedback.body':
    'Si quelque chose dans la suite ou sur ce site vous est difficile ou impossible à utiliser, écrivez-nous : nous l’examinons et nous vous répondons.',
  'a11y.enforcement.title': 'Si la réponse ne vous satisfait pas',
  'a11y.enforcement.body':
    'Si vous avez signalé un obstacle et que notre réponse ne vous satisfait pas, vous pouvez saisir l’autorité chargée de l’accessibilité dans votre pays au titre de l’Acte législatif européen sur l’accessibilité.',
  'a11y.updated':
    'Cette déclaration a été rédigée en septembre 2026 et est revue lorsque la suite change de manière substantielle.',
  'meta.openSource.title': 'Open source — Sytue One',
  'meta.openSource.description':
    "Sytue construit presque tout depuis zéro, mais ne réinvente pas ce qui fonctionne déjà bien. Les projets open source sur lesquels s'appuie la suite.",
  'oss.title': 'Open source',
  'oss.intro':
    "Sytue construit presque tout depuis zéro : la messagerie, les éditeurs, le calendrier. Mais elle ne réinvente pas ce qui fonctionne déjà bien. Voici les projets open source sur lesquels s'appuient certaines pièces de la suite.",
  'oss.livekit.body':
    "L'audio et la vidéo en temps réel des appels de Sytue Rooms reposent sur LiveKit.",
  'oss.postgres.body':
    'Chaque service de la suite stocke ses données dans PostgreSQL, une base par organisation.',
  'meta.privacy.title': 'Confidentialité et RGPD — Sytue One',
  'meta.privacy.description':
    'Comment Sytue One traite les données personnelles : responsable du traitement, données, bases légales, conservation, sous-traitants, sécurité, transferts internationaux et vos droits RGPD.',
  'legal.overline': 'Mentions légales',
  'legal.updated': 'Dernière mise à jour : juillet 2026.',
  'legal.review.title': 'En cours de revue juridique.',
  'privacy.title': 'Confidentialité et RGPD',
  'privacy.review.body':
    'Cette politique décrit avec exactitude la manière dont nous traitons les données aujourd’hui. Les durées de conservation précises seront complétées dans la version finale, revue par notre équipe juridique.',
  'privacy.controller.title': 'Responsable du traitement',
  'privacy.controller.body':
    'Sytue Projects, S.L. (numéro fiscal espagnol B88729900 ; Calle Atenas, 10, 28822 Coslada, Madrid, Espagne ; immatriculée au registre du commerce de Madrid, section 8, feuillet M-887153), en tant que fournisseur de la suite Sytue One, est le responsable du traitement des données des comptes et des organisations du service. Contact en matière de protection des données :',
  'privacy.data.title': 'Quelles données nous traitons',
  'privacy.data.account.label': 'Données de compte :',
  'privacy.data.account.body':
    'nom, adresse e-mail et organisation, gérés par Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Contenu de votre organisation :',
  'privacy.data.content.body':
    'messages, fichiers, documents, agenda et discussions que votre organisation héberge dans la suite. Ce contenu appartient à votre organisation.',
  'privacy.data.technical.label': 'Données techniques d’exploitation :',
  'privacy.data.technical.body':
    'journaux d’accès et données de connexion nécessaires à la sécurité, à la prévention des abus et au fonctionnement du service.',
  'privacy.data.note':
    'Nous ne vendons pas les données et ne les utilisons pas à des fins publicitaires. L’infrastructure et le stockage gérés par Sytue se trouvent dans l’Union européenne (Scaleway, régions fr-par et nl-ams, et une zone sur site à Madrid), avec chiffrement en transit (TLS 1.3) et au repos. Les connexions aux services externes transmettent les données que tu autorises à leur fournisseur, selon sa localisation et ses conditions.',
  'privacy.basis.title': 'Base légale',
  'privacy.basis.body':
    'Les traitements reposent sur les bases de l’article 6 du RGPD : l’exécution du contrat de service conclu avec votre organisation, l’intérêt légitime à la sécurité et à la prévention des abus, le respect d’obligations légales et, lorsqu’il est expressément recueilli, votre consentement. Le détail base par base, traitement par traitement, fera partie du texte définitif.',
  'privacy.retention.title': 'Conservation',
  'privacy.retention.body':
    'Nous conservons les données de compte et le contenu tant que votre organisation maintient le service actif. À la clôture d’un compte ou d’une organisation, les données sont supprimées ou anonymisées dans un délai raisonnable, sauf ce que nous devons conserver en vertu d’une obligation légale (la facturation, par exemple). Les journaux techniques d’exploitation sont purgés périodiquement. Les durées précises par catégorie figureront dans la version finale. Les signalements de contenu sont conservés tant qu’ils sont ouverts et supprimés douze mois après leur traitement.',
  'privacy.processors.title': 'Sous-traitants et sous-traitants ultérieurs',
  'privacy.processors.body':
    'Notre fournisseur d’infrastructure est Scaleway (régions fr-par et nl-ams), qui agit en qualité de sous-traitant et opère exclusivement dans l’Union européenne ; nous exploitons en outre une zone on-premise à Madrid (Espagne), dans nos propres locaux. Pour l’encaissement des abonnements, nous recourons à Mollie B.V. (Pays-Bas), qui ne traite que les données nécessaires au paiement. Pour envoyer le SMS contenant le code qui vérifie le numéro de mobile lors de la création d’un compte, nous recourons à Sweego (Mindbaz SAS, France), qui ne reçoit que ce numéro et le code. Nous ne recourons à aucun sous-traitant ultérieur hors de l’EEE. La liste des sous-traitants sera publiée et tenue à jour, avec information préalable de tout ajout.',
  'privacy.security.title': 'Sécurité',
  'privacy.security.body':
    'Nous appliquons des mesures techniques et organisationnelles conformes au RGPD : chiffrement en transit (TLS 1.3) et au repos, isolation entre organisations (multi-tenant avec sécurité au niveau des lignes), authentification renforcée (MFA/WebAuthn), journalisation des accès et principe du moindre privilège. Aucun système n’est infaillible : en cas de violation vous concernant, nous vous en informerons dans les délais fixés par le Règlement.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'Nous n’utilisons que des cookies techniques nécessaires à l’authentification unique et à la sécurité de la session. Aucun cookie publicitaire ni de suivi tiers : il n’y a donc pas de bandeau de consentement au traçage.',
  'privacy.transfers.title': 'Transferts internationaux',
  'privacy.transfers.body':
    'Le traitement et l’hébergement gérés par Sytue ont lieu dans des régions de l’Union européenne. Les connexions externes activées par l’utilisateur transmettent les données autorisées à leur fournisseur ; leur localisation dépend de ce fournisseur et de sa configuration. Les notifications envoyées lorsque l’application ou le navigateur sont fermés peuvent aussi utiliser des services externes. Le service de push de votre appareil (Google, Mozilla ou Apple, selon votre navigateur ou votre téléphone) voit un identifiant anonyme de l’appareil et l’heure de chaque notification — jamais le contenu, qui circule chiffré de bout en bout et qui, sur iOS, part carrément vide. Vous pouvez désactiver les notifications : alors même cela ne sort plus.',
  'privacy.rights.title': 'Vos droits',
  'privacy.rights.body1':
    'Le RGPD vous reconnaît des droits d’accès, de rectification, d’effacement, d’opposition, de limitation du traitement et à la portabilité. Vous pouvez les exercer en écrivant à',
  'privacy.rights.body2':
    ' ; nous répondons dans les délais fixés par le Règlement. Vous avez également le droit d’introduire une réclamation auprès de',
  'privacy.rights.authority': 'votre autorité de contrôle',
  'privacy.dpo.title': 'Délégué à la protection des données (DPO)',
  'privacy.dpo.body':
    'Pour toute question relative à la protection des données ou pour contacter notre délégué à la protection des données :',
  'privacy.minors.title': 'Mineurs',
  'privacy.minors.body':
    'Sytue One est un service destiné aux organisations et aux personnes qui y travaillent ; il ne s’adresse pas aux mineurs et nous ne collectons pas sciemment de données les concernant.',
  'privacy.changes.title': 'Modifications de cette politique',
  'privacy.changes.body':
    'Si nous mettons à jour cette politique, nous modifierons la date de « dernière mise à jour » et, lorsque les changements sont importants, nous vous en informerons par les canaux du service avant leur entrée en vigueur.',

  // --- Legal: términos -----------------------------------------------------
  'meta.terms.title': 'Conditions de service — Sytue One',
  'meta.terms.description':
    'Conditions de service de Sytue One : prestataire, objet, comptes et organisations, usage acceptable, disponibilité, vos données, propriété intellectuelle et droit applicable.',
  'terms.title': 'Conditions de service',
  'terms.review.body':
    'Ces conditions décrivent avec exactitude le service et son usage. Les clauses contractuelles précises (niveau de service, responsabilité et juridiction) seront complétées dans la version finale, revue par notre équipe juridique.',
  'terms.provider.title': 'Qui fournit le service',
  'terms.provider.body':
    'Sytue One est fourni par Sytue Projects, S.L. (numéro fiscal espagnol B88729900 ; Calle Atenas, 10, 28822 Coslada, Madrid, Espagne ; immatriculée au registre du commerce de Madrid, section 8, feuillet M-887153) depuis une infrastructure située exclusivement dans l’Union européenne (Scaleway, régions fr-par et nl-ams, et une zone on-premise à Madrid), et relève du droit de l’UE.',
  'terms.scope.title': 'Objet du service',
  'terms.scope.body':
    'Sytue One est une suite bureautique pour les organisations : identité (Sytue ID), messagerie, fichiers, documents, agenda, contacts et messagerie instantanée avec visioconférence. Le service se souscrit par organisation.',
  'terms.accounts.title': 'Comptes et organisations',
  'terms.accounts.body':
    'Chaque organisation administre ses comptes, ses groupes et ses droits via Sytue ID (SSO + MFA). L’administration de l’organisation est responsable des créations et suppressions de comptes ainsi que de l’usage qui est fait du service avec ces comptes.',
  'terms.use.title': 'Usage acceptable',
  'terms.use.body':
    'Vous vous engagez à utiliser le service conformément à la loi et à ne pas l’employer à des fins illicites, pour l’envoi de spam, la diffusion de logiciels malveillants, l’atteinte aux droits de tiers ou tout usage compromettant la sécurité ou la disponibilité du service pour d’autres organisations.',
  'terms.availability.title': 'Disponibilité et assistance',
  'terms.availability.body1':
    'Nous travaillons à offrir une haute disponibilité et réalisons maintenances et améliorations en continu. Les engagements de niveau de service (SLA), lorsqu’ils s’appliquent à votre offre, sont fixés dans le contrat de service. L’assistance est assurée via',
  'terms.data.title': 'Vos données',
  'terms.data.body1':
    'Le contenu que votre organisation héberge dans la suite lui appartient. Il repose sur des standards ouverts (OIDC, JMAP, CalDAV, CardDAV, WebDAV), de sorte que vos données restent toujours portables et que vous pouvez les emporter. Le traitement des données personnelles est décrit sur la page',
  'terms.ip.title': 'Propriété intellectuelle',
  'terms.ip.body':
    'Le logiciel Sytue One est une œuvre originale de Sytue, protégée par les droits de propriété intellectuelle correspondants. Vous conservez la titularité du contenu que votre organisation héberge dans la suite ; nous ne le traitons que pour vous fournir le service.',
  'terms.liability.title': 'Responsabilité',
  'terms.liability.body':
    'Nous mettons tout en œuvre pour un service fiable et sûr. Les garanties et limitations de responsabilité applicables sont fixées dans le contrat de service ; rien dans ces conditions n’exclut les responsabilités que la loi ne permet pas d’exclure.',
  'terms.contact.title': 'Souscription et contact',
  'terms.contact.body1':
    'Pour les conditions de souscription, les tarifs par organisation ou toute question sur le service, écrivez-nous à',
  'terms.changes.title': 'Modifications',
  'terms.changes.body':
    'Nous pouvons mettre à jour ces conditions pour refléter des évolutions du service ou de la réglementation. Lorsque les changements sont importants, nous vous en informerons par les canaux du service avant leur entrée en vigueur, en actualisant la date de « dernière mise à jour ».',
  'terms.law.title': 'Droit applicable',
  'terms.law.body':
    'Le service est régi par le droit de l’Union européenne et par la réglementation du pays du prestataire. La juridiction compétente pour le règlement des litiges sera précisée dans le texte définitif. Indépendamment de cela, vous conservez votre droit d’introduire une réclamation auprès de votre autorité de contrôle en matière de protection des données.',

  // --- Datos estructurados (JSON-LD) --------------------------------------
  'seo.org.description':
    'Sytue One : la suite bureautique souveraine européenne. Identité, messagerie, fichiers, documents, agenda, discussions, visioconférence et une PaaS, 100 % dans l’UE.',
  'seo.org.slogan': 'Votre productivité, souveraine et européenne.',
  'seo.software.description':
    'Suite bureautique souveraine européenne : messagerie (JMAP), fichiers (WebDAV), documents collaboratifs, agenda (CalDAV), contacts (CardDAV), discussions, visioconférence et PaaS. SSO avec Sytue ID.',
  'seo.software.f1': 'Authentification unique (SSO) + MFA/WebAuthn',
  'seo.software.f2':
    'Messagerie, fichiers, documents, agenda, contacts, discussions et visioconférence',
  'seo.software.f3': 'Plateforme de déploiement et de données gérées',
  'seo.software.f4': 'Données 100 % dans l’UE (Scaleway)',
  'seo.offer.description': 'Essai gratuit, sans carte bancaire.',
  'seo.downloads.description':
    'Applications natives de la suite Sytue One pour ordinateur (macOS, Windows, Linux) et mobile (iOS, Android).',

  // --- Precios: cuenta personal ---
  'home.pricing.personal.title': 'Gratuit',
  'home.pricing.personal.freeStorage': '5 Go, pour une personne',
  'home.pricing.personal.freeBody':
    'Courrier, fichiers, photos, documents, agenda, contacts, notes et salles. Sans carte et sans limite de temps.',
  'home.pricing.personal.note':
    'Votre organisation, c’est votre e-mail : pas d’administration d’organisation ni d’ERP, et tout le reste, y compris Sytue Admin pour vos intégrations et migrations.',
  'home.pricing.personal.cta': 'Créer un compte gratuit',
  'grid.orgOnly': 'Réservé aux organisations',
  'grid.enDesarrollo': 'En développement',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 Go par utilisateur',
  'home.pricing.paidBody':
    'Pour les organisations, et aussi pour une personne seule. Toute la suite, avec Sytue ERP et Sytue Admin pour gérer votre organisation.',
  'home.pricing.thirdParty':
    'Tous les forfaits permettent de connecter votre propre stockage : Google Drive, Microsoft OneDrive, un bucket S3 (Scaleway, OVH, Hetzner, Wasabi, MinIO), un serveur WebDAV comme Nextcloud ou Internxt, ou votre NAS dans vos locaux —sans ouvrir de ports, via un tunnel Sytue—. Les sauvegardes et la récupération des données stockées dans un stockage connecté relèvent de la responsabilité du client (vous), et Sytue® est dégagée de toute responsabilité en cas de perte de données dans les stockages connectés à Sytue®.',
  'home.pricing.soon': 'Bientôt',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 To par utilisateur',
  'home.pricing.enterprise.support': 'Avec Support Premium.',
  'home.pricing.education': 'Établissements d’enseignement : 50 % de réduction sur Pro.',
};
