/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo FRANCÉS de Sytue Calendar. Terminología de Google Agenda / Outlook
 * en francés: «Événement», «Participants», «Rappel», «Toute la journée»,
 * «Agenda». La misma que Mail y Contacts. Apóstrofo tipográfico (’) y comillas
 * « » con espacio, como en el resto de la suite. docs/CONVENTIONS.md §13.1.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fr: AppCatalog<typeof en> = {
  // --- Comunes ------------------------------------------------------------
  'common.cancel': 'Annuler',
  'common.save': 'Enregistrer',
  'common.delete': 'Supprimer',
  'common.edit': 'Modifier',
  'common.close': 'Fermer',
  'common.remove': 'Retirer',
  'common.retry': 'Réessayer',
  'common.done': 'Terminé',
  'common.loading': 'Chargement…',
  'common.colour': 'Couleur',

  // --- Página / layout ----------------------------------------------------
  'page.description': 'Sytue Calendar — l’agenda souverain de la suite Sytue One.',
  'page.newEvent': 'Sytue Calendar — Nouvel événement',

  // --- Barra lateral ------------------------------------------------------
  'nav.createEvent': 'Créer un événement',
  'nav.myCalendars': 'Mes agendas',
  'nav.manageResources': 'Gérer les salles et les équipements',
  'nav.resourcesTitle': 'Salles et équipements',
  'nav.subscribeAria': 'S’abonner à un agenda par URL',
  'nav.subscribeTitle': 'S’abonner par URL (.ics)',
  'nav.createCalendar': 'Créer un agenda',
  'nav.noCalendars': 'Aucun agenda',
  'nav.showCalendar': 'Afficher {name}',
  'nav.subscribedReadOnly': 'Agenda abonné (lecture seule)',
  'nav.calendarOptions': 'Options de {name}',
  'nav.calendarOptionsTitle': 'Options de l’agenda',
  'nav.refreshNow': 'Actualiser maintenant',
  'nav.rename': 'Renommer',
  'nav.changeColour': 'Changer la couleur',
  'nav.share': 'Partager…',
  'nav.deleteSubscription': 'Supprimer l’abonnement',
  'nav.sharedWithMe': 'Partagés avec moi',
  'nav.sharedBy': 'Partagé par {name}',
  'nav.permWriter': 'Modification',
  'nav.permReader': 'Lecture',
  'nav.permWriterTitle': 'Vous pouvez modifier',
  'nav.permReaderTitle': 'Lecture seule',
  'nav.prevMonth': 'Mois précédent',
  'nav.nextMonth': 'Mois suivant',

  // --- Cabecera de la vista -----------------------------------------------
  'header.today': 'Aujourd’hui',
  'header.shortcut': '{label} ({key})',
  'header.previous': 'Précédent',
  'header.next': 'Suivant',
  'header.search': 'Rechercher des événements',
  'header.invitations': 'Invitations',
  'header.pending': { one: '{count} en attente', other: '{count} en attente' },
  'header.viewGroup': 'Affichage de l’agenda',
  'view.month': 'Mois',
  'view.week': 'Semaine',
  'view.day': 'Jour',
  'view.agenda': 'Planning',

  // --- Rejilla ------------------------------------------------------------
  'grid.loading': 'Chargement de l’agenda',
  'grid.loadError': 'Impossible de charger l’agenda',
  'grid.createOn': 'Créer un événement le {date}',
  'grid.more': { one: '+{count} autre', other: '+{count} autres' },
  'grid.continued': '{title} (suite)',
  'grid.blockAria': '{title}, {time}',
  'grid.snapAria': {
    one: 'Aligner le déplacement sur {count} minute. Appuyez pour changer.',
    other: 'Aligner le déplacement sur {count} minutes. Appuyez pour changer.',
  },
  'grid.snapTitle': { one: 'Aligné sur {count} min', other: 'Aligné sur {count} min' },
  'grid.snapValue': { one: '{count} min', other: '{count} min' },

  // --- Vista agenda -------------------------------------------------------
  'agenda.search': 'Rechercher des événements',
  'agenda.noMatches': 'Aucune correspondance',
  'agenda.empty': 'Aucun événement à venir',

  // --- Evento (comunes a todas las vistas) --------------------------------
  'event.untitled': '(sans titre)',
  'event.untitledLong': 'événement sans titre',
  'event.allDay': 'Toute la journée',

  // --- «Cuándo» -----------------------------------------------------------
  'when.allDay': 'Toute la journée · {date}',
  'when.allDayRange': 'Toute la journée · {start} – {end}',
  'when.at': '{date}, {time}',
  'when.range': '{date}, {start} – {end}',
  'when.rangeAcross': '{startDate} {startTime} – {endDate} {endTime}',
  'when.timeRange': '{start} – {end}',

  // --- Vista rápida (popover) ---------------------------------------------
  'popover.details': 'Détails de {title}',
  'popover.when': 'Quand',
  'popover.location': 'Lieu',
  'popover.conference': 'Visioconférence',
  'popover.description': 'Description',
  'popover.join': 'Rejoindre la visioconférence',
  'popover.duplicate': 'Dupliquer',
  'popover.copyToMine': 'Copier dans mon calendrier',
  'popover.readOnly': 'Agenda abonné (lecture seule).',
  'notes.linked': 'Notes',
  'notes.create': 'Écrire une note pour cet événement',
  'notes.untitled': 'Note sans titre',

  // --- Modal de evento ----------------------------------------------------
  'event.viewTitle': 'Voir l’événement',
  'event.editTitle': 'Modifier l’événement',
  'event.newTitle': 'Nouvel événement',
  'event.readOnlyNotice':
    'Cet événement appartient à un agenda abonné (lecture seule). Il ne peut pas être modifié.',
  'event.title': 'Titre',
  'event.titlePlaceholder': 'Ajoutez un titre',
  'event.ai': "Décrivez l'événement",
  'event.aiPlaceholder': 'Déjeuner avec Ana jeudi à 14h00 chez Casa Lucio',
  'event.aiFill': 'Remplir',
  'event.aiError': "Aucun événement n'a été compris. Précisez le jour et l'heure.",
  'event.titleRequired': 'Le titre est obligatoire.',
  'event.calendar': 'Agenda',
  'event.start': 'Début',
  'event.end': 'Fin',
  'event.endBeforeStart': 'La fin ne peut pas être antérieure au début.',
  'event.timeZone': 'Fuseau horaire',
  'event.findTimeShow': 'Trouver un créneau',
  'event.findTimeHide': 'Masquer « trouver un créneau »',
  'event.repeat': 'Récurrence',
  'event.repeatNever': 'Ne se répète pas',
  'event.repeatDaily': 'Tous les jours',
  'event.repeatWeekly': 'Toutes les semaines',
  'event.repeatMonthly': 'Tous les mois',
  'event.repeatEnds': 'Se termine',
  'event.repeatEndNever': 'Jamais',
  'event.repeatEndCount': 'Après un nombre d’occurrences',
  'event.repeatEndUntil': 'À une date',
  'event.repeatCount': 'Nombre d’occurrences',
  'event.repeatUntil': 'Jusqu’à la date',
  'event.location': 'Lieu',
  'event.locationPlaceholder': 'Ajoutez un lieu (facultatif)',
  'event.conference': 'Visioconférence',
  'event.conferenceRemoveAria': 'Retirer la visioconférence',
  'event.conferenceAdd': 'Ajouter une visioconférence',
  'event.conferenceCreating': 'Création…',
  'event.conferenceHint':
    'Crée une salle Sytue Rooms pour cet événement. Les personnes invitées la rejoignent depuis leur agenda, sans rien installer.',
  'event.conferenceError': 'Impossible de créer la visioconférence.',
  'event.conferenceDefaultSummary': 'Réunion',
  'event.resources': 'Salles et équipements',
  'event.guests': 'Personnes invitées',
  'event.guestsAdded': 'Personnes invitées ajoutées',
  'event.guestRemove': 'Retirer {email}',
  'event.guestPlaceholder': 'Ajoutez une adresse e-mail et appuyez sur Entrée',
  'event.guestInvalid': 'Saisissez une adresse e-mail valide.',
  'event.description': 'Description',
  'event.descriptionPlaceholder': 'Ajoutez des notes ou une description (facultatif)',
  'event.reminders': 'Rappels',
  'event.remindersNone': 'Aucun rappel',
  'event.remindersList': 'Rappels de l’événement',
  'event.reminderAdd': 'Ajouter un rappel',
  'event.reminderSelect': 'Rappel {index}',
  'event.reminderAmount': 'Valeur du rappel {index}',
  'event.reminderUnit': 'Unité du rappel {index}',
  'event.reminderRemove': 'Retirer le rappel {index}',
  'event.reminderCustom': 'Personnalisé…',
  'event.unitMinutes': 'minutes avant',
  'event.unitHours': 'heures avant',
  'event.unitDays': 'jours avant',

  // --- Recordatorios ------------------------------------------------------
  'reminder.atTime': 'Au moment de l’événement',
  'reminder.minutes': { one: '{count} minute avant', other: '{count} minutes avant' },
  'reminder.hours': { one: '{count} heure avant', other: '{count} heures avant' },
  'reminder.days': { one: '{count} jour avant', other: '{count} jours avant' },
  'reminder.weeks': { one: '{count} semaine avant', other: '{count} semaines avant' },

  // --- Alcance de una serie recurrente ------------------------------------
  'scope.editTitle': 'Modifier un événement récurrent',
  'scope.deleteTitle': 'Supprimer un événement récurrent',
  'scope.legend': 'Cet événement se répète. À quels événements la modification s’applique-t-elle ?',
  'scope.this': 'Cet événement uniquement',
  'scope.following': 'Cet événement et les suivants',
  'scope.all': 'Tous les événements',

  // --- Modal de calendario -------------------------------------------------
  'calendar.newTitle': 'Nouvel agenda',
  'calendar.editTitle': 'Modifier l’agenda',
  'calendar.helpTitle': 'À quoi sert un autre agenda',
  'calendar.helpBody':
    'Séparez ce que vous ne voulez pas mélanger (Travail, Personnel, Astreintes). Il s’affiche à côté du reste de votre planning, vous le masquez d’un clic et vous le partagez séparément.',
  'calendar.name': 'Nom',
  'calendar.namePlaceholder': 'Ex. Travail',
  'calendar.nameRequired': 'Le nom est obligatoire.',
  'calendar.nameHint':
    'C’est ainsi que vous le verrez dans votre liste d’agendas. Vous pourrez le changer plus tard.',
  'calendar.colourHint':
    'Ses événements sont affichés dans cette couleur sur la grille, pour les distinguer d’un coup d’œil.',
  'calendar.deleteTitle': 'Supprimer l’agenda',
  'calendar.deleteBody':
    '« {name} » sera supprimé avec tous ses événements, et les personnes avec qui vous l’aviez partagé cesseront de le voir. Cette action est irréversible.',

  // --- Paleta de colores ---------------------------------------------------
  'colour.inherit': 'Couleur de l’agenda',
  'colour.terracotta': 'Terre cuite',
  'colour.slate': 'Ardoise',
  'colour.pine': 'Pin',
  'colour.inkBlue': 'Bleu encre',
  'colour.jade': 'Jade',
  'colour.ochre': 'Ocre',
  'colour.garnet': 'Grenat',
  'colour.plum': 'Prune',
  'colour.petrol': 'Bleu pétrole',
  'colour.orchid': 'Orchidée',
  'colour.purple': 'Pourpre',
  'colour.indigo': 'Indigo',
  'colour.steel': 'Acier',
  'colour.viridian': 'Viridian',

  // --- Compartir calendario ------------------------------------------------
  'share.title': 'Partager « {name} »',
  'share.titleFallback': 'Partager l’agenda',
  'share.helpTitle': 'Ce qui va se passer',
  'share.helpBody':
    'La personne verra cet agenda à côté du sien, sans avoir à accepter quoi que ce soit. Vous pouvez lui retirer l’accès quand vous voulez, et cela ne concerne que cet agenda, pas le reste de votre planning.',
  'share.email': 'Adresse e-mail de la personne avec qui vous partagez',
  'share.emailPlaceholder': 'personne@sytue.eu',
  'share.emailInvalid': 'Saisissez une adresse e-mail valide.',
  'share.emailHint':
    'Une personne de votre organisation. Si vous indiquez l’adresse d’une liste ou d’un groupe, toutes les personnes qui en font partie verront votre agenda.',
  'share.legend': 'Ce qu’elle pourra faire',
  'share.readerLabel': 'Voir mes événements',
  'share.readerHint':
    'Voit le titre, l’horaire et les détails de chaque événement, mais ne peut rien modifier.',
  'share.writerLabel': 'Voir et modifier mes événements',
  'share.writerHint':
    'En plus de les voir, crée, modifie et supprime des événements de cet agenda.',
  'share.manageLabel': 'Gérer mon planning à ma place',
  'share.manageHint':
    'Tout ce qui précède et, en plus, organise et répond en votre nom. Pour la personne qui gère votre planning au quotidien.',
  'share.warnTitle': 'Elle pourra modifier vos événements',
  'share.warnWriter':
    'Elle pourra créer, déplacer et supprimer des événements de cet agenda. Les changements apparaissent immédiatement dans votre planning.',
  'share.warnManage':
    'Créer, déplacer et supprimer des événements, et répondre aux invitations comme si c’était vous. Ne l’accordez qu’à la personne qui gère votre planning.',
  'share.submit': 'Partager',
  'share.whoSees': 'Qui le voit actuellement',
  'share.emptyTitle': 'Vous seul voyez cet agenda',
  'share.emptyDescription':
    'Une fois partagé, vous verrez ici avec qui et ce que chaque personne peut faire.',
  'share.badgeReader': 'Voit seulement',
  'share.badgeWriter': 'Voit et modifie',
  'share.badgeManage': 'Gère votre planning',
  'share.revoke': 'Ne plus partager avec {email}',
  'share.revokeTitle': 'Révoquer',

  // --- Suscripción a un .ics -----------------------------------------------
  'subscribe.title': 'S’abonner à un agenda',
  'subscribe.submit': 'S’abonner',
  'subscribe.intro':
    'Ajoutez un agenda public par son URL (un fichier .ics, comme les jours fériés d’un pays ou l’agenda d’une équipe). Il est ajouté en lecture seule et se met à jour automatiquement.',
  'subscribe.url': 'URL de l’agenda (.ics)',
  'subscribe.urlPlaceholder': 'https://exemple.com/jours-feries.ics',
  'subscribe.urlInvalid': 'Saisissez une URL http(s) valide d’un fichier .ics.',
  'subscribe.name': 'Nom',
  'subscribe.namePlaceholder': 'Ex. Jours fériés',
  'subscribe.defaultName': 'Agenda abonné',

  // --- Invitaciones ---------------------------------------------------------
  'invitations.title': 'Invitations',
  'invitations.loading': 'Chargement des invitations',
  'invitations.emptyTitle': 'Vous n’avez aucune invitation en attente',
  'invitations.emptyDescription':
    'Lorsque quelqu’un vous invitera à un événement, l’invitation apparaîtra ici pour que vous y répondiez.',
  'invitations.help':
    'Répondez pour que la personne qui organise sache si elle peut compter sur vous. Si vous dites oui, l’événement est enregistré dans votre agenda ; vous pourrez changer de réponse plus tard depuis l’événement lui-même.',
  'invitations.untitled': 'Événement sans titre',
  'invitations.invitedBy': '{name} vous invite',
  'invitations.invitedByFrom': '{name} ({org}) vous invite',
  'invitations.generic': 'Invitation d’agenda',
  'invitations.whoWhen': '{who} · {when}',
  'invitations.accept': 'Oui, j’y serai',
  'invitations.acceptHint': 'Enregistré dans votre agenda',
  'invitations.tentative': 'Peut-être',
  'invitations.tentativeHint': 'Enregistré, mais marqué comme provisoire',
  'invitations.decline': 'Je ne peux pas',
  'invitations.declineHint': 'Non enregistré dans votre agenda',

  // --- Centro de recordatorios ---------------------------------------------
  'reminders.title': 'Rappels',
  'reminders.bell': 'Rappels',
  'reminders.bellCount': { one: 'Rappels ({count})', other: 'Rappels ({count})' },
  'reminders.loading': 'Chargement',
  'reminders.loadError': 'Impossible de charger les rappels.',
  'reminders.empty': 'Aucun rappel en attente',
  'reminders.dismiss': 'Ignorer le rappel de {title}',
  'reminders.eventFallback': 'événement',
  'reminders.enableNotifications': 'Activer les alertes du navigateur',
  'reminders.fallbackTitle': 'Rappel',
  'reminders.body': 'Événement',
  'reminders.bodyAt': 'Événement · {time}',
  'reminders.view': 'Voir',
  'reminders.today': 'Aujourd’hui',
  'reminders.when': '{day} · {time}',

  // --- Búsqueda -------------------------------------------------------------
  'search.title': 'Rechercher des événements',
  'search.placeholder': 'Titre, description, lieu ou personne invitée…',
  'search.minChars': {
    one: 'Saisissez au moins {count} caractère pour chercher dans vos agendas.',
    other: 'Saisissez au moins {count} caractères pour chercher dans vos agendas.',
  },
  'search.searching': 'Recherche…',
  'search.error': 'Impossible de terminer la recherche.',
  'search.noResults': 'Aucun résultat pour « {term} ».',
  'search.series': 'Série',
  'search.loadMore': 'Charger plus',

  // --- Buscar un hueco ------------------------------------------------------
  'findTime.summary': 'Créneaux communs · lun-ven {start}–{end} · {duration} min',
  'findTime.weekdaysOnly': 'Jours ouvrés uniquement',
  'findTime.window': 'Fenêtre de recherche',
  'findTime.windowDays': { one: '{count} jour', other: '{count} jours' },
  'findTime.loading': 'Recherche de créneaux',
  'findTime.error': 'Impossible de consulter les disponibilités.',
  'findTime.empty':
    'Aucun créneau libre dans cette fenêtre. Essayez de l’élargir ou de raccourcir l’événement.',

  // --- Salas y recursos (gestión) -------------------------------------------
  'resources.title': 'Salles et équipements',
  'resources.kindRoom': 'Salle',
  'resources.kindEquipment': 'Équipement',
  'resources.helpTitle': 'Ce qu’est une ressource réservable',
  'resources.helpBody':
    'Une salle ou un équipement que les gens ajoutent à leurs réunions. À la création d’un événement, elle apparaît dans « Salles et équipements » avec sa capacité et son emplacement, et une seule personne à la fois peut la réserver : si elle est déjà prise à cette heure-là, elle apparaît comme occupée.',
  'resources.formNewTitle': 'Nouvelle ressource',
  'resources.formEditTitle': 'Modifier la ressource',
  'resources.formNewDesc':
    'C’est ainsi que votre équipe la verra quand elle cherchera où se réunir.',
  'resources.formEditDesc':
    'Les changements sont visibles immédiatement dans les nouvelles réservations ; celles déjà faites restent inchangées.',
  'resources.kindFixed': '{kind} · le type ne peut plus être changé après la création',
  'resources.legend': 'Ce que vous ajoutez',
  'resources.roomTitle': 'Une salle',
  'resources.roomDesc':
    'Un espace physique où les gens se réunissent. Elle est affichée avec sa capacité et son étage.',
  'resources.equipmentTitle': 'Un équipement',
  'resources.equipmentDesc':
    'Quelque chose que l’on apporte à la réunion : un vidéoprojecteur, une voiture, un portable de prêt.',
  'resources.name': 'Nom',
  'resources.namePlaceholder': 'Salle Marie Curie',
  'resources.nameRequired': 'Le nom est obligatoire.',
  'resources.nameHint':
    'C’est ce que l’on lit au moment de réserver : qu’il soit reconnaissable sans explication.',
  'resources.capacity': 'Capacité',
  'resources.capacityPlaceholder': 'Facultatif',
  'resources.capacityHint':
    'Combien de personnes peuvent y tenir. Cela évite de réserver une salle de 4 pour 12.',
  'resources.location': 'Emplacement',
  'resources.locationPlaceholder': 'Étage 2',
  'resources.locationHint':
    'Où elle se trouve, pour qui ne le sait pas : « Étage 2 », « Bâtiment B ».',
  'resources.email': 'Adresse e-mail de la ressource',
  'resources.emailPlaceholder': 'salle@sytue.eu',
  'resources.emailHint':
    'Facultatif. Cela permet de l’inviter depuis l’extérieur de Sytue, comme n’importe quelle autre invitation.',
  'resources.autoAccept': 'Accepter automatiquement si elle est libre',
  'resources.autoAcceptOn':
    'La personne qui la réserve l’aura confirmée immédiatement si personne ne l’a prise avant.',
  'resources.autoAcceptOff':
    'Chaque réservation reste en attente jusqu’à ce que quelqu’un l’approuve à la main.',
  'resources.create': 'Créer la ressource',
  'resources.catalogueTitle': 'Salles et équipements enregistrés',
  'resources.catalogueDesc': 'Voilà ce que votre équipe peut réserver aujourd’hui.',
  'resources.emptyTitle': 'Rien à réserver pour l’instant',
  'resources.emptyDescription':
    'Enregistrez la première salle ou le premier équipement ci-dessus : il apparaîtra dans le sélecteur de ressources à la création d’un événement.',
  'resources.emptyAction': 'Enregistrer une salle',
  'resources.people': { one: '{count} personne', other: '{count} personnes' },
  'resources.edit': 'Modifier {name}',
  'resources.remove': 'Retirer {name}',
  'resources.removeTitle': 'Retirer',

  // --- Selector de recursos (dentro del evento) -----------------------------
  'picker.error': 'Impossible de consulter la disponibilité des ressources.',
  'picker.list': 'Ressources réservées',
  'picker.remove': 'Retirer {name}',
  'picker.removeTitle': 'Retirer',
  'picker.fallbackName': 'ressource',
  'picker.add': 'Ajouter une salle ou une ressource',
  'picker.addPlaceholder': 'Ajouter une salle ou une ressource…',
  'picker.option': '{name} — {state}',
  'picker.optionWithCapacity': '{name} · {capacity} — {state}',
  'picker.free': 'libre',
  'picker.busy': 'occupée',
  'picker.allAdded': 'Toutes les ressources ont été ajoutées.',
  'picker.badgeBooked': 'Réservée',
  'picker.badgeBusy': 'Occupée',
  'picker.badgePending': 'En attente',
  'picker.badgeFree': 'Libre',

  // --- Reservas de recursos al guardar --------------------------------------
  'booking.booked': { one: '{count} ressource réservée', other: '{count} ressources réservées' },
  'booking.busy': { one: '{count} occupée', other: '{count} occupées' },
  'booking.saved': 'Événement enregistré · {booked}',
  'booking.savedBusy': 'Événement enregistré · {booked}, {busy}',

  // --- Errores de carga -----------------------------------------------------
  'error.calendarsLoad': 'Impossible de charger les agendas.',
  'error.eventsLoad': 'Impossible de charger les événements.',

  // --- Avisos (toasts) ------------------------------------------------------
  'toast.selectCalendar': 'Sélectionnez un agenda',
  'toast.eventCreated': 'Événement créé',
  'toast.eventUpdated': 'Événement mis à jour',
  'toast.eventSaveFailed': 'Impossible d’enregistrer l’événement',
  'toast.eventDeleted': 'Événement supprimé',
  'toast.eventDeleteFailed': 'Impossible de supprimer l’événement',
  'toast.subscriptionRefreshed': 'Abonnement mis à jour',
  'toast.subscriptionRefreshFailed': 'Impossible de mettre à jour l’abonnement',
  'toast.calendarCreated': 'Agenda créé',
  'toast.calendarUpdated': 'Agenda mis à jour',
  'toast.calendarSaveFailed': 'Impossible d’enregistrer l’agenda',
  'toast.calendarDeleted': 'Agenda supprimé',
  'toast.calendarDeleteFailed': 'Impossible de supprimer l’agenda',
  'toast.calendarSubscribed': 'Abonnement à l’agenda effectué',
  'toast.subscribeFailed': 'Impossible de s’abonner',
  'toast.subscribeFailedHint':
    'Vérifiez l’URL : ce doit être un fichier .ics public accessible en http(s).',
  'toast.sharesLoadFailed': 'Impossible de charger les partages',
  'toast.calendarShared': 'Agenda partagé',
  'toast.shareFailed': 'Impossible de partager l’agenda',
  'toast.shareRevoked': 'Partage révoqué',
  'toast.revokeFailed': 'Impossible de révoquer',
  'toast.resourcesLoadFailed': 'Impossible de charger les ressources',
  'toast.resourceCreated': 'Ressource créée',
  'toast.resourceUpdated': 'Ressource mise à jour',
  'toast.resourceSaveFailed': 'Impossible d’enregistrer la ressource',
  'toast.resourceRemoved': 'Ressource retirée',
  'toast.resourceRemoveFailed': 'Impossible de retirer la ressource',
  'toast.invitationsLoadFailed': 'Impossible de charger les invitations',
  'toast.invitationAccepted': 'C’est fait : l’événement est dans votre agenda',
  'toast.invitationDeclined': 'C’est fait : vous avez répondu que vous ne pouviez pas',
  'toast.invitationTentative': 'C’est fait : vous avez répondu « peut-être »',
  'toast.invitationFailed': 'Impossible de répondre à l’invitation',
  'toast.reminderDismissFailed': 'Impossible d’ignorer le rappel',
  'nav.prev': 'Précédent',
  'nav.next': 'Suivant',
  'cal.create': 'Créer un calendrier',
  'cal.options': 'Options du calendrier',
  'cal.refresh': 'Actualiser maintenant',
  'cal.share': 'Partager…',
  'cal.downloadIcs': 'Télécharger .ics',
  'cal.sharedWithMe': 'Partagés avec moi',
  'resources.manage': 'Gérer les salles et ressources',
  'subscribe.aria': "S'abonner à un calendrier par URL",
  'subscribe.byUrl': "S'abonner par URL (.ics)",
  'search.events': 'Rechercher des événements',
  'view.label': 'Vue du calendrier',
  'load.error': "Le calendrier n'a pas pu être chargé",
  'loading.calendar': 'Chargement du calendrier',
  'page.title': 'Sytue Calendar',
  'page.newEventTitle': 'Sytue Calendar — Nouvel événement',
  'share.whatHappens': 'Ce qui va se passer',
  'share.canEdit': 'Cette personne pourra modifier vos événements',
  'share.whoSeesNow': 'Qui le voit actuellement',
  'share.onlyYou': 'Vous seul voyez ce calendrier',
  'share.revokeAction': 'Révoquer',

  // --- Rótulos que estaban en duro ---
  'share.recipientEmail': 'E-mail de la personne avec qui vous partagez',
  'cal.subscribedReadOnly': 'Calendrier abonné (lecture seule)',

  // --- Errores y textos de permisos que estaban en duro ---
  'cal.fromEarlier': ' (depuis avant)',
  'cal.noUpcoming': 'Aucun événement à venir',
  'cal.sub.delete': 'Supprimer l’abonnement',
  'cal.sub.updated': 'Abonnement mis à jour',
  'cal.error.loadCalendars': 'Impossible de charger les calendriers.',
  'cal.error.loadEvents': 'Impossible de charger les événements.',
  'cal.error.noAccess': 'Vous n’avez plus accès à « {name} ».',
  'cal.error.updateEvent': 'Impossible de mettre à jour l’événement',
  'cal.error.saveEvent': 'Impossible d’enregistrer l’événement',
  'cal.error.deleteEvent': 'Impossible de supprimer l’événement',
  'cal.error.updateSub': 'Impossible de mettre à jour l’abonnement',
  'share.recipient.hint':
    'Une personne de votre organisation. Si vous saisissez l’adresse d’une liste ou d’un groupe, toutes les personnes qui en font partie verront votre calendrier.',
  'share.role.manage.hint':
    'Créer, déplacer et supprimer des événements, et répondre aux invitations en votre nom. Ne l’accordez qu’à la personne qui gère votre agenda.',
  'share.role.write.hint':
    'Cette personne pourra créer, déplacer et supprimer des événements de ce calendrier. Les modifications apparaissent immédiatement dans votre agenda.',
  'nav.createEventOn': 'Créer un événement le {date}',
  'share.empty.hint':
    'Une fois partagé, vous verrez ici avec qui et ce que chaque personne peut faire.',
  'popover.respond': 'Vous y allez ?',
  'popover.yes': 'Oui',
  'popover.maybe': 'Peut-être',
  'popover.no': 'Non',
  'popover.responded': 'Réponse envoyée.',
  'popover.respondFailed': 'La réponse n’a pas pu être envoyée.',
};
