/**
 * Textes français du cœur de Sytue Web (web).
 * Préfixes : nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Impossible de récupérer l’adresse publique du site.',
  'nav.title': 'Web',
  'nav.sites': 'Mes sites',
  'nav.new': 'Nouveau site',
  'nav.settings': 'Paramètres',

  'sites.title': 'Mes sites',
  'sites.empty': 'Vous n’avez encore créé aucun site.',
  'sites.create': 'Créer un site',
  'sites.status.published': 'Publié',
  'sites.status.draft': 'Brouillon',
  'sites.restore': 'Restaurer',
  'sites.trash': 'Sites dans la corbeille',
  'sites.trash_help':
    'Les sites supprimés sont conservés 30 jours avant leur suppression définitive.',
  'sites.open_editor': 'Ouvrir l’éditeur',
  'sites.manage': 'Gérer',
  'sites.last_updated': 'Mis à jour le {fecha}',
  'sites.filter_all': 'Tous',
  'sites.filter_published': 'Publiés',
  'sites.filter_draft': 'Brouillons',
  'sites.count': { one: '{count} site', many: '{count} sites', other: '{count} sites' },
  'sites.empty_help':
    'Choisissez un modèle ou partez d’une page vierge. Vous pourrez le publier quand vous voudrez.',
  'sites.filter_label': 'Filtrer par état',
  'sites.filter_empty': 'Aucun site ne correspond à ce filtre.',
  'sites.loading': 'Chargement des sites',
  'sites.load_error': 'Impossible de charger les sites',
  'sites.load_error_help': 'Vérifiez votre connexion et réessayez.',
  'sites.retry': 'Réessayer',
  'sites.restore_error': 'Impossible de restaurer le site. Réessayez.',
  'sites.days_left': {
    one: '{count} jour restant',
    many: '{count} jours restants',
    other: '{count} jours restants',
  },

  'new.title': 'Créer un site',
  'new.template_step': 'Choisir un modèle',
  'new.details_step': 'Informations du site',
  'new.name_label': 'Nom du site',
  'new.slug_label': 'Identifiant (slug)',
  'new.languages_label': 'Langues du site',
  'new.submit': 'Créer et ouvrir l’éditeur',
  'new.step_template': 'Modèle',
  'new.step_info': 'Informations de base',
  'new.step_languages': 'Langues',
  'new.primary_language': 'Langue principale',
  'new.step_review': 'Vérification',
  'new.details_help':
    'Donnez un nom à votre site. Nous préparons son adresse automatiquement ; vous pourrez connecter votre propre domaine plus tard.',
  'new.review_heading': 'Prêt à démarrer',
  'new.review_help': 'Vérifiez ces options avant de créer votre site.',
  'new.draft_notice':
    'Cela crée un brouillon privé. Dans l’éditeur, vous pourrez modifier les textes, les photos et le design. Nous vous guiderons ensuite pour la vérification, la connexion de votre domaine et la publication.',
  'new.template_blank': 'Vierge',
  'new.template_blank_desc': 'Partez d’une page vide et concevez librement.',
  'new.languages_notice': 'D’autres langues arrivent bientôt',
  'new.slug_available': 'L’identifiant est disponible',
  'new.slug_unavailable': 'Cet identifiant n’est pas disponible',
  'new.slug_checking': 'Vérification de l’identifiant…',
  'new.next': 'Suivant',
  'new.back': 'Retour',
  'new.templates_error': 'Impossible de charger les modèles. Vous pouvez partir d’une page vierge.',
  'new.name_placeholder': 'Ma boutique en ligne',
  'new.slug_placeholder': 'ma-boutique',
  'new.slug_help':
    'Uniquement des lettres minuscules, des chiffres et des tirets. Il forme l’adresse du site.',
  'new.slug_unverified':
    'L’identifiant n’a pas pu être vérifié pour le moment ; il sera validé à la création du site.',
  'new.main_language': 'Langue principale',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Activer la version bilingue ({idioma})',
  'new.create_error': 'Impossible de créer le site. Réessayez.',
  'new.err_site_limit':
    'Votre organisation a déjà 20 sites, ou en a créé 5 aujourd’hui. Supprimez-en un ou revenez demain.',
  'new.cancel': 'Annuler',
  'new.progress': 'Étapes de création du site',
  'new.go_to_step': 'Aller à l’étape {n}',
  'new.step_of': 'Étape {n} sur {total}',
  'app_settings.title': 'Paramètres de Sytue Web',
  'new.search_title': 'Commencez par votre type d’activité',
  'new.search_help':
    'Saisissez ce que vous faites ou choisissez un groupe. Chaque modèle contient déjà les textes de votre métier : il vous suffit de changer le nom, le téléphone, les prix et les photos.',
  'new.search_label': 'Votre type d’activité',
  'new.search_placeholder': 'Salon de coiffure, bar, plombier, boutique de vêtements…',
  'new.all_groups': 'Tous',
  'new.results': {
    one: '{count} modèle',
    many: '{count} modèles',
    other: '{count} modèles',
  },
  'new.no_results': 'Aucun modèle ne correspond à « {q} ».',
  'new.no_results_help':
    'Essayez un autre mot (par exemple « boutique » ou « cabinet ») ou partez d’une page vierge.',
  'new.clear_search': 'Effacer la recherche',
  'new.select': 'Choisir ce modèle',
  'new.selected': 'Choisi',
  'new.preview': 'Aperçu',
  'new.preview_close': 'Fermer',
  'new.preview_loading': 'Chargement du modèle…',
  'new.preview_error': 'Impossible de charger l’aperçu.',
  'new.preview_desktop': 'Ordinateur',
  'new.preview_mobile': 'Téléphone',
  'new.preview_pages': 'Pages du modèle',
  'new.preview_note':
    'Vue approximative : les photos sont provisoires et seront remplacées par les vôtres, et les données d’exemple se modifient dans l’éditeur.',
  'new.badge_store': 'Avec boutique',
  'new.badge_custom': 'Personnalisé',
  'new.cat.belleza': 'Beauté et bien-être',
  'new.cat.comida': 'Restauration',
  'new.cat.comercio': 'Commerces',
  'new.cat.oficios': 'Artisans et maison',
  'new.cat.motor': 'Automobile',
  'new.cat.profesionales': 'Entreprises et professions libérales',
  'new.cat.salud': 'Santé',
  'new.cat.deporte': 'Sport',
  'new.cat.educacion': 'Éducation et formation',
  'new.cat.creativos': 'Création et événements',
  'new.cat.turismo': 'Tourisme et hébergement',
  'new.cat.local': 'Produits locaux',
  'new.cat.comunidad': 'Associations et vie locale',
  'new.cat.web': 'Pages simples',
  'new.cat.otras': 'Autres',

  'publish.title': 'Publier le site',
  'publish.action': 'Publier',
  'publish.checklist': 'Vérifications avant publication',
  'publish.preview': 'Aperçu',

  'versions.title': 'Versions',
  'versions.create': 'Créer une version',
  'versions.restore': 'Restaurer cette version',
  'versions.empty': 'Aucune version enregistrée pour l’instant.',
  'versions.name': 'Nom de la version',
  'versions.rev': 'Révision',
  'versions.restored': 'Version restaurée',

  'domains.title': 'Domaines',
  'domains.add': 'Ajouter un domaine',

  'analytics.title': 'Statistiques',
  'analytics.visits': 'Visites',

  'export.title': 'Exporter',
  'export.origins_label': 'Origines autorisées (là où le code sera hébergé)',
  'export.origins_help':
    'Indiquez une adresse https:// par ligne depuis laquelle l’API publique est appelée.',
  'export.schemes_label': 'Schémas d’application (pour les apps natives)',
  'export.schemes_help': 'Schémas d’URL personnalisés (un par ligne), par exemple : monapp, sytue.',
  'export.save': 'Enregistrer les paramètres d’export',
  'export.saved': 'Paramètres d’export enregistrés',

  'legal.title': 'Mentions légales',
  'legal.holder_name': 'Nom de l’entreprise ou du titulaire',
  'legal.tax_id': 'N° d’identification fiscale / TVA',
  'legal.country': 'Pays',
  'legal.street': 'Adresse',
  'legal.postal_code': 'Code postal',
  'legal.city': 'Ville',
  'legal.region': 'Région / Département',
  'legal.email': 'E-mail de contact juridique',
  'legal.phone': 'Téléphone',
  'legal.registry': 'Données d’immatriculation (registre, volume, feuillet)',
  'legal.privacy_contact': 'Contact protection des données (DPD)',
  'legal.save': 'Enregistrer les mentions légales',
  'legal.saved': 'Mentions légales enregistrées',

  'settings.title': 'Paramètres du site',
  'settings.name': 'Nom du site',
  'settings.slug': 'Identifiant (slug)',
  'settings.analytics': 'Activer les statistiques intégrées',
  'settings.analytics_help':
    'Mesure les pages vues dans le respect de la vie privée, sans cookies tiers.',
  'settings.save': 'Enregistrer les paramètres',
  'settings.saved': 'Paramètres enregistrés',
  'settings.danger_zone': 'Zone de danger',
  'settings.delete_title': 'Supprimer ce site',
  'settings.delete_desc':
    'Le site sera placé dans la corbeille pendant 30 jours, période pendant laquelle il pourra être restauré.',
  'settings.delete_confirm_prompt': 'Saisissez le nom du site ({name}) pour confirmer :',
  'settings.delete_button': 'Supprimer le site',

  'errors.not_found': 'Site introuvable',
  'errors.unauthorized': 'Vous n’avez pas l’autorisation de voir ce site',

  // Fiche du site (islands/SitioDetalleApp.tsx, formularios/Bandeja.tsx, tienda/Pedidos.tsx)
  'ficha.page_title': 'Détails du site',
  'ficha.back': 'Retour à mes sites',
  'ficha.sections': 'Sections du site',
  'ficha.loading': 'Chargement du site',
  'ficha.load_error': 'Impossible de charger le site',
  'ficha.open_site': 'Ouvrir le site',
  'ficha.not_found_help': 'Il a peut-être été supprimé, ou vous n’y avez pas accès.',

  'publish.tab': 'Publier',
  'publish.intro':
    'Passez en revue les vérifications et publiez lorsque tout est prêt. Un aperçu ne change rien à ce que voit le public.',
  'publish.status': 'État actuel',
  'publish.check_ok': 'Prêt',
  'publish.check_missing': 'Manquant',
  'publish.check_legal': 'Mentions légales complètes',
  'publish.check_legal_missing': 'Manquant : {campos}',
  'publish.check_legal_fix': 'Compléter',
  'publish.check_demo': 'Plus aucun texte d’exemple du modèle',
  'publish.check_ir': 'Les pages sont correctement construites',
  'publish.check_server':
    'La publication vérifie aussi qu’il ne reste aucun texte d’exemple du modèle et que les pages sont correctement construites.',
  'publish.done_preview': 'Aperçu créé.',
  'publish.done_live': 'Site publié.',
  'publish.open_preview': 'Ouvrir l’aperçu',
  'publish.err_legal':
    'Des mentions légales manquent. Complétez-les dans l’onglet « Mentions légales ».',
  'publish.err_demo':
    'Le site contient encore du texte d’exemple du modèle. Remplacez-le dans l’éditeur et publiez à nouveau.',
  'publish.err_ir':
    'Une page n’est pas correctement construite. Ouvrez-la dans l’éditeur pour voir ce qu’il faut corriger.',
  'publish.err_previews':
    'Il y a déjà 5 aperçus actifs. Attendez qu’un aperçu expire avant d’en créer un autre.',
  'publish.err_too_large':
    'Le site dépasse la taille maximale publiable (64 Mio ou 5 000 fichiers). Réduisez les images ou les fichiers.',
  'publish.err_forbidden': 'La publication du site est réservée à l’administration.',
  'publish.err_conflict':
    'Impossible de publier le site, car il a changé ou il est suspendu. Rechargez la page et réessayez.',
  'publish.err_invalid':
    'La publication n’est pas valide. Vérifiez le contenu du site dans l’éditeur.',
  'publish.err_generic': 'Impossible de publier le site. Réessayez.',

  'versions.intro':
    'Une version est une copie du brouillon à laquelle vous pouvez revenir. Créez-en une avant un changement important.',
  'versions.name_placeholder': 'Avant de modifier la page d’accueil',
  'versions.manual_name': 'Version manuelle',
  'versions.unnamed': 'Sans nom',
  'versions.loading': 'Chargement des versions',
  'versions.load_error': 'Impossible de charger les versions',
  'versions.empty_help':
    'Lorsque vous créerez une version ou publierez le site, elle apparaîtra ici.',
  'versions.created': 'Version créée',
  'versions.create_error': 'Impossible de créer la version. Réessayez.',
  'versions.restore_error': 'Impossible de restaurer la version. Réessayez.',
  'versions.restore_confirm':
    'Le brouillon reviendra à l’état de « {nombre} ». L’état actuel est d’abord enregistré sous « Avant restauration », pour que vous puissiez y revenir.',
  'versions.kind.manual': 'Manuelle',
  'versions.kind.publish': 'Publication',
  'versions.kind.preview': 'Aperçu',
  'versions.kind.export': 'Export',
  'versions.kind.before_restore': 'Avant restauration',

  'domains.intro':
    'Connectez votre propre domaine. Après l’avoir ajouté, créez les enregistrements indiqués chez votre fournisseur DNS et vérifiez la configuration.',
  'domains.host_label': 'Domaine',
  'domains.host_placeholder': 'www.exemple.fr',
  'domains.host_help': 'Uniquement le nom de domaine, sans https:// ni barres obliques.',
  'domains.empty': 'Aucun domaine personnalisé n’est encore configuré.',
  'domains.loading': 'Chargement des domaines',
  'domains.load_error': 'Impossible de charger les domaines',
  'domains.primary': 'Principal',
  'domains.status.pending': 'Vérification en attente',
  'domains.status.verified': 'Vérifié',
  'domains.status.active': 'Actif',
  'domains.status.failed': 'Échec',
  'domains.dns_title': 'Enregistrements DNS',
  'domains.dns_txt': 'TXT (vérification)',
  'domains.dns_cname': 'CNAME (sous-domaines comme www)',
  'domains.dns_a': 'A (domaine racine)',
  'domains.check': 'Vérifier le DNS et le HTTPS',
  'domains.make_primary': 'Définir comme principal',
  'domains.remove': 'Supprimer',
  'domains.remove_title': 'Supprimer le domaine',
  'domains.remove_confirm':
    '« {host} » ne pointera plus vers ce site. Vous devrez l’ajouter à nouveau pour l’utiliser.',
  'domains.added':
    'Domaine ajouté. Créez les enregistrements indiqués ci-dessous chez votre fournisseur DNS.',
  'domains.updated': 'Domaines mis à jour.',
  'domains.action_error': 'L’action n’a pas pu être effectuée. Réessayez.',
  'domains.err_invalid': 'Le nom de domaine n’est pas valide.',
  'domains.err_reserved': 'Ce domaine est réservé et ne peut pas être utilisé.',
  'domains.err_taken': 'Ce domaine est déjà utilisé par un autre site.',
  'domains.err_limit': 'Vous avez atteint le nombre maximal de domaines pour ce site.',
  'domains.err_suspended': 'Le site est suspendu et n’accepte pas de changement de domaine.',
  'domains.err_forbidden': 'La gestion des domaines est réservée à l’administration.',
  'domains.err_generic': 'L’opération sur le domaine n’a pas pu aboutir. Réessayez.',
  'domains.start_title': 'Connectez votre domaine, étape par étape',
  'domains.start_help':
    'Saisissez le domaine que vous voulez utiliser. Vous aurez besoin d’accéder aux paramètres DNS chez le fournisseur où vous l’avez acheté.',
  'domains.progress': 'Progression de la connexion du domaine',
  'domains.step.registered': 'Ajouté à Sytue Web',
  'domains.step.dns': 'DNS vérifié',
  'domains.step.https': 'HTTPS prêt',
  'domains.step.primary': 'Adresse principale',
  'domains.step.done': 'Terminé',
  'domains.step.pending': 'En attente',
  'domains.status.review': 'À vérifier',
  'domains.record_name': 'Nom de l’enregistrement',
  'domains.record_value': 'Valeur de l’enregistrement',
  'domains.routing_title': 'Adresse du site',
  'domains.dns_help':
    'Ouvrez les paramètres DNS chez votre fournisseur et ajoutez ces deux enregistrements. Copiez chaque nom et chaque valeur exactement comme indiqué.',
  'domains.dns_names_help':
    'Certains fournisseurs utilisent @ pour le domaine racine ou simplement www pour ce sous-domaine. Le nom complet de l’enregistrement est indiqué ci-dessus.',
  'domains.routing_legacy_help':
    'Utilisez A pour le domaine racine ou CNAME pour un sous-domaine comme www, avec la destination correspondante fournie par le service.',
  'domains.dns_propagation':
    'Après avoir enregistré les entrées, choisissez « Vérifier le DNS et le HTTPS ». Si elles n’apparaissent pas encore, attendez que votre fournisseur propage la modification et vérifiez à nouveau. Conservez les enregistrements de votre messagerie et de vos autres services.',
  'domains.records_unavailable':
    'Le service n’a pas encore fourni toutes les données de cet enregistrement. Attendez-les avant de modifier votre DNS.',
  'domains.copy': 'Copier',
  'domains.copied': 'Copié',
  'domains.copy_field': 'Copier {campo}',
  'domains.copy_error':
    'Impossible de copier la valeur dans le presse-papiers. Sélectionnez la valeur du champ et copiez-la manuellement.',
  'domains.last_check': 'Dernière vérification : {fecha}',
  'domains.publish_help':
    'Une fois votre adresse principale prête, publiez à nouveau le site pour que les liens et les redirections utilisent votre domaine.',
  'domains.publish_action': 'Aller à la publication',
  'domains.detail.pending':
    'Votre domaine est ajouté. Configurez ses enregistrements DNS et vérifiez la connexion pour continuer.',
  'domains.detail.txt_missing':
    'L’enregistrement TXT est introuvable pour l’instant. Ajoutez-le chez votre fournisseur DNS et vérifiez à nouveau une fois la modification propagée.',
  'domains.detail.txt_mismatch':
    'La valeur TXT ne correspond pas. Remplacez-la par la valeur exacte affichée sur cet écran.',
  'domains.detail.dns_unreachable':
    'Impossible d’interroger le DNS du domaine. Vérifiez le domaine et ses serveurs de noms, puis réessayez.',
  'domains.detail.routing':
    'Le domaine ne pointe pas encore vers le site. Vérifiez l’enregistrement A ou CNAME indiqué et réessayez.',
  'domains.detail.certificate':
    'Vos enregistrements DNS sont vérifiés. Le certificat HTTPS n’est pas encore prêt ; vérifiez à nouveau dans quelques minutes.',
  'domains.detail.active': 'Le DNS et la connexion HTTPS sont vérifiés. Votre domaine est prêt.',
  'domains.detail.unknown':
    'La dernière vérification demande votre attention. Contrôlez les enregistrements et vérifiez à nouveau la connexion.',

  'analytics.intro':
    'Pendant la phase pilote, la fréquentation est une estimation quotidienne ; elle ne correspond pas à des personnes uniques sur la période.',
  'analytics.visitors': 'Fréquentation estimée',
  'analytics.loading': 'Chargement des statistiques',
  'analytics.load_error': 'Impossible de charger les statistiques',
  'analytics.unavailable':
    'Le service de statistiques n’est pas disponible pour le moment. Réessayez dans quelques minutes.',
  'analytics.disabled': 'Les statistiques sont désactivées',
  'analytics.disabled_help':
    'Activez-les dans les paramètres du site pour commencer à compter les visites.',
  'analytics.go_settings': 'Aller aux paramètres',
  'analytics.refresh': 'Actualiser',

  'export.intro':
    'Si vous hébergez le code du site en dehors de Sytue, indiquez d’où l’API publique sera appelée.',
  'export.origins_placeholder': 'https://maboutique.exemple.fr',
  'export.schemes_placeholder': 'monapp',
  'export.origins_invalid':
    'Chaque ligne doit être une adresse https:// valide (la ligne {n} ne l’est pas).',
  'export.schemes_invalid':
    'Chaque ligne doit être un schéma valide : il commence par une lettre et ne contient que des lettres, des chiffres, « + », « - » ou « . » (la ligne {n} ne l’est pas).',
  'export.err_generic': 'Impossible d’enregistrer les paramètres. Réessayez.',

  'legal.intro': 'Elles sont obligatoires pour publier le site.',
  'legal.required_note': 'Les champs marqués d’un * sont obligatoires.',
  'legal.saved_incomplete': 'Données enregistrées. Il manque encore pour publier : {campos}.',
  'legal.country_help': 'Code à deux lettres, par exemple FR.',
  'legal.registry_help':
    'Obligatoire pour les sociétés espagnoles (identifiant fiscal commençant par A, B, C, D ou F).',
  'legal.ph_holder': 'Exemple SARL',
  'legal.ph_street': '12 rue Principale, 1er étage',
  'legal.ph_city': 'Paris',
  'legal.ph_email': 'juridique@exemple.fr',
  'legal.ph_privacy': 'dpd@exemple.fr',
  'legal.ph_registry': 'Registre du commerce de Madrid, volume 1234, folio 56, feuille M-789',
  'legal.err_generic': 'Impossible d’enregistrer les mentions légales. Réessayez.',

  'settings.tab': 'Paramètres',
  'settings.intro': 'Nom, adresse et options générales du site.',
  'settings.slug_help':
    'Uniquement des lettres minuscules, des chiffres et des tirets. Si vous le modifiez, l’adresse du site change.',
  'settings.admin_only': 'Cette modification est réservée à l’administration.',
  'settings.name_required': 'Le nom du site ne peut pas être vide.',
  'settings.err_slug_taken': 'Un autre site utilise déjà cet identifiant.',
  'settings.err_invalid':
    'Vérifiez le nom, l’identifiant et l’adresse : l’un d’eux n’est pas valide.',
  'settings.err_generic': 'Impossible d’enregistrer les paramètres. Réessayez.',
  'settings.delete_error': 'Impossible de supprimer le site. Réessayez.',

  'forms.empty': 'Aucune réponse pour l’instant',
  'forms.empty_help': 'Lorsque quelqu’un enverra un formulaire de votre site, vous le verrez ici.',
};
