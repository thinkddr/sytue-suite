/**
 * Textes français de l’intelligence artificielle (ia).
 * Préfixe : ai.*
 */
export const ia = {
  // Paramètres
  'ai.settings.provider': 'Fournisseur d’IA',
  'ai.settings.model': 'Modèle',
  'ai.settings.key': 'Clé API personnelle (BYOK)',
  'ai.settings.key_placeholder': 'Saisissez votre clé API secrète…',
  'ai.settings.key_help':
    'Votre clé est stockée chiffrée avec AES-256-GCM. Elle n’est jamais renvoyée au navigateur.',
  'ai.settings.check_key': 'Tester la clé',
  'ai.settings.delete_key': 'Supprimer la clé',
  'ai.settings.status_configured': 'Clé configurée',
  'ai.settings.tested_at': 'Vérifiée le',
  'ai.settings.quota_title': 'Quota mensuel d’IA',
  'ai.settings.quota_reset': 'Réinitialisé le {date}',
  'ai.settings.notice_eu_title': 'Avis de transfert de données hors de l’EEE',
  'ai.settings.notice_eu_desc':
    'Le fournisseur choisi ({provider}) traite les données en dehors de l’Espace économique européen. Conformément à l’article 49, paragraphe 1, point a), du RGPD, vous devez consentir explicitement à ce transfert pour continuer.',
  'ai.settings.notice_eu_accept':
    'Je consens au transfert international de données hors de l’EEE pour les requêtes d’IA auprès de ce fournisseur.',
  'ai.settings.success_save': 'Clé enregistrée et vérifiée.',
  'ai.settings.error_check': 'Erreur lors de la vérification de la clé auprès du fournisseur.',
  'ai.settings.form_title': 'Configurer votre propre clé (BYOK)',
  'ai.settings.form_help':
    'Utilisez vos propres identifiants Mistral AI, OpenAI, Anthropic ou Zhipu AI pour ne pas dépendre du quota mensuel.',
  'ai.settings.provider_mistral': 'Mistral AI (UE, RGPD)',
  'ai.settings.provider_openai': 'OpenAI (États-Unis)',
  'ai.settings.provider_anthropic': 'Anthropic (États-Unis)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'UE, France',
  'ai.region.us': 'États-Unis',
  'ai.region.intl': 'International',
  'ai.settings.show_key': 'Afficher la clé',
  'ai.settings.hide_key': 'Masquer la clé',
  'ai.settings.key_required': 'Saisissez une clé.',
  'ai.settings.notice_required': 'Vous devez accepter l’avis sur les transferts hors de l’EEE.',
  'ai.settings.check_ok': 'Clé vérifiée avec succès auprès du fournisseur.',
  'ai.settings.error_save': 'Impossible d’enregistrer la clé.',
  'ai.settings.error_delete': 'Impossible de supprimer la clé.',
  'ai.settings.checking': 'Vérification…',
  'ai.settings.save_activate': 'Enregistrer et activer',
  'ai.settings.key_deleted': 'Clé supprimée. Le quota mensuel par défaut s’appliquera.',
  'ai.settings.active_provider': 'Fournisseur actif : {provider} ({model})',
  'ai.settings.no_key_default':
    'Aucune clé personnelle configurée. La limite mensuelle de l’offre s’applique.',
  'ai.settings.status_plan': 'Quota de l’offre',
  'ai.settings.quota_unlimited': 'Illimité',
  'ai.settings.quota_unlimited_help': 'Requêtes traitées avec votre propre clé API.',
  'ai.settings.quota_jobs': 'Tâches de design, de traduction et d’export.',
  'ai.settings.loading': 'Chargement des paramètres d’IA',
  'ai.settings.load_error': 'Impossible de charger les paramètres d’IA.',
  'ai.settings.load_error_help':
    'Le service n’a pas pu être joint. Vos paramètres n’ont pas changé.',
  'ai.settings.key_invalid':
    'Le fournisseur a refusé la clé. Vérifiez qu’elle est correcte et active.',
  'ai.settings.encryption_unavailable':
    'Le service n’est pas encore prêt à stocker des clés de manière sécurisée. Contactez l’assistance.',
  'ai.settings.invalid_model': 'Choisissez un modèle valide pour ce fournisseur.',
  'ai.settings.key_state': 'État de la clé',

  'ai.common.cancel': 'Annuler',

  // Contexte commun : quota, avis RGPD, erreurs et état des tâches
  'ai.context.quota_label': 'Quota mensuel d’IA',
  'ai.context.quota_used': '{used} tâches sur {limit} ce mois-ci',
  'ai.context.quota_reset': 'Renouvelé le {date}.',
  'ai.context.quota_exhausted_title': 'Le quota mensuel d’IA est épuisé',
  'ai.context.quota_exhausted_desc':
    'Vous pourrez lancer d’autres tâches le {date}. L’administration peut lever la limite en ajoutant sa propre clé API dans les paramètres d’IA.',
  'ai.context.byok':
    'Votre organisation utilise sa propre clé API : cette tâche ne consomme pas de quota.',
  'ai.context.loading': 'Vérification du quota d’IA',
  'ai.context.load_error':
    'Impossible de vérifier le quota d’IA. Vous pouvez tout de même essayer.',
  'ai.context.retry': 'Réessayer',
  'ai.notice.eu': 'Les textes sont traités dans l’UE, conformément au RGPD.',
  'ai.notice.non_eu_title': 'Cette tâche sort de l’Espace économique européen',
  'ai.notice.non_eu_desc':
    'La clé propre de votre organisation appartient à {provider}, qui traite les données en dehors de l’EEE. Votre organisation a accepté ce transfert (art. 49, par. 1, point a), du RGPD) en enregistrant la clé.',
  'ai.error.quota': 'Le quota mensuel d’IA est épuisé. Il sera renouvelé le {date}.',
  'ai.error.quota_nodate': 'Le quota mensuel d’IA est épuisé.',
  'ai.error.busy':
    'Des tâches d’IA sont déjà en cours. Attendez qu’elles se terminent et réessayez.',
  'ai.error.admin': 'Cette action est réservée à l’administration de l’organisation.',
  'ai.error.prompt': 'Décrivez votre site avant de générer la proposition.',
  'ai.error.languages': 'Choisissez au moins une langue cible.',
  'ai.error.session': 'Votre session a expiré. Rechargez la page et reconnectez-vous.',
  'ai.error.generic': 'Impossible de lancer la tâche. Réessayez dans quelques minutes.',
  'ai.error.draft_not_saved':
    'Impossible d’enregistrer le brouillon. Résolvez le problème d’enregistrement et relancez l’IA.',
  'ai.job.state_queued': 'En file d’attente',
  'ai.job.state_running': 'En cours',
  'ai.job.state_done': 'Terminée',
  'ai.job.state_failed': 'Échec',
  'ai.job.queued_text': 'En attente de l’acceptation de la tâche par le serveur…',
  'ai.job.running_text': 'L’IA travaille. Cela peut prendre quelques minutes.',
  'ai.job.done_text': 'La tâche est terminée.',
  'ai.job.failed_text': 'La tâche n’a pas pu aboutir.',
  'ai.job.lost_text': 'La connexion au serveur a été perdue.',
  'ai.job.elapsed': 'Temps écoulé : {time}',
  'ai.job.detail': 'Détail technique : {detail}',
  'ai.job.model': 'Modèle : {model}',
  'ai.job.badge_eu': 'Dans l’UE',
  'ai.job.badge_non_eu': 'Hors EEE',
  'ai.job.badge_byok': 'Clé personnelle',
  'ai.job.back': 'Retour',
  'ai.job.retry': 'Réessayer',
  'ai.job.stop': 'Ne plus attendre',
  'ai.job.stop_help': 'La tâche continuera sur le serveur et sera décomptée de votre quota.',

  // Design
  'ai.design.title': 'Concevoir avec l’IA',
  'ai.design.desc':
    'Décrivez votre activité et l’IA proposera les sections de la page, avec des textes prêts à modifier.',
  'ai.design.prompt_label': 'Décrivez votre site ou votre activité',
  'ai.design.prompt_placeholder':
    'Ex. : boutique de mode féminine durable avec catalogue et prise de rendez-vous…',
  'ai.design.prompt_help':
    'Plus la description est précise (ce que vous proposez, à qui, et ce que doivent faire les personnes qui visitent le site), meilleure sera la proposition.',
  'ai.design.ideas_label': 'Idées pour commencer',
  'ai.design.idea_1': 'Épicerie fine de fromages et vins artisanaux avec commande en ligne',
  'ai.design.idea_2': 'Agence d’architecture et de rénovation avec galerie de projets',
  'ai.design.idea_3': 'Cabinet dentaire avec prise de rendez-vous, équipe soignante et FAQ',
  'ai.design.idea_4': 'Restaurant italien avec menu dégustation et réservation de table',
  'ai.design.style_label': 'Style visuel',
  'ai.design.style_modern': 'Moderne et épuré',
  'ai.design.style_elegant': 'Élégant et raffiné',
  'ai.design.style_bold': 'Audacieux et vibrant',
  'ai.design.style_minimal': 'Minimaliste',
  'ai.design.lang_label': 'Langue du contenu',
  'ai.design.button_generate': 'Générer la proposition',
  'ai.design.job_title': 'Conception de la proposition',
  'ai.design.result_title': 'Proposition prête',
  'ai.design.result_sections': {
    one: '{count} section prête à ajouter au canevas.',
    many: '{count} sections prêtes à ajouter au canevas.',
    other: '{count} sections prêtes à ajouter au canevas.',
  },
  'ai.design.result_help':
    'Elles seront ajoutées à la page en cours de modification et vous pourrez les ajuster ensuite.',
  'ai.design.result_empty': 'L’IA n’a renvoyé aucune section utilisable.',
  'ai.design.button_apply': 'Ajouter au canevas',
  'ai.design.button_again': 'Essayer une autre idée',

  // Traduction
  'ai.translate.title': 'Traduire avec l’IA',
  'ai.translate.desc': 'Traduisez les textes du site dans d’autres langues sans toucher au design.',
  'ai.translate.from_label': 'Langue source',
  'ai.translate.to_label': 'Langues cibles',
  'ai.translate.to_help':
    'Choisissez-en au moins une. La langue source ne peut pas être une cible. Appliquer le résultat ajoute de nouvelles langues au site.',
  'ai.translate.scope_label': 'Portée de la traduction',
  'ai.translate.scope_site': 'Site entier',
  'ai.translate.scope_page': 'Page actuelle',
  'ai.translate.scope_store': 'Catalogue de la boutique uniquement',
  'ai.translate.store_unavailable':
    'La traduction du catalogue de la boutique n’est pas encore disponible.',
  'ai.translate.scope_invalid': 'Choisissez une page existante ou le site entier à traduire.',
  'ai.translate.overwrite_label': 'Écraser les traductions existantes',
  'ai.translate.button_translate': 'Traduire',
  'ai.translate.job_title': 'Traduction des textes',
  'ai.translate.result_title': 'Traductions prêtes',
  'ai.translate.result_texts': {
    one: '{count} texte traduit.',
    many: '{count} textes traduits.',
    other: '{count} textes traduits.',
  },
  'ai.translate.result_help': 'Elles seront ajoutées au brouillon ; le design ne change pas.',
  'ai.translate.result_empty': 'L’IA n’a renvoyé aucune traduction.',
  'ai.translate.button_apply': 'Appliquer les traductions',
  'ai.translate.button_again': 'Traduire dans d’autres langues',

  // Export
  'ai.export.title': 'Exporter avec l’IA',
  'ai.export.desc':
    'Générez un paquet contenant votre site, prêt à être déplacé vers une autre plateforme, sans dépendre de Sytue.',
  'ai.export.destination_label': 'Plateforme de destination',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Thème complet avec modèles JSON et sections Liquid, plus le catalogue en CSV à importer depuis l’administration Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Projet monopage connecté à l’API publique de Sytue, avec paiement et formulaires.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Application rendue côté serveur ou statique, avec la passerelle de paiement intégrée.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Thème de blocs WordPress natif, avec export des produits et des attributs.',
  'ai.export.badge_available': 'Disponible',
  'ai.export.badge_soon': 'Bientôt',
  'ai.export.button_export': 'Générer le paquet',
  'ai.export.job_title': 'Génération du paquet pour {destination}',
  'ai.export.result_title': 'Paquet prêt',
  'ai.export.result_help': 'Le thème et le CSV du catalogue sont réunis dans un seul ZIP.',
  'ai.export.steps_title': 'Comment l’installer dans Shopify',
  'ai.export.step_1': 'Téléchargez le fichier ZIP.',
  'ai.export.step_2':
    'Dans l’administration Shopify, allez dans Boutique en ligne > Thèmes et choisissez « Importer un fichier zip ».',
  'ai.export.step_3':
    'Pour les produits, allez dans Produits > Importer et importez le fichier products_export_shopify.csv contenu dans le ZIP.',
  'ai.export.button_download': 'Télécharger le ZIP',
  'ai.export.button_again': 'Générer un autre export',
};
