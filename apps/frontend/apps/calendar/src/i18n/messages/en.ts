/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo INGLÉS de Sytue Calendar — FUENTE DE VERDAD (docs/CONVENTIONS.md
 * §13.1). Las cadenas nacen aquí y los otros cuatro idiomas las traducen. Una
 * clave nueva aquí rompe la compilación de es/fr/de/it hasta que la traduzcan.
 *
 * Inglés europeo (en-GB): «colour», «organisation». El formateador ya usa en-GB
 * (`intlTag`) y mezclar ortografías en la misma interfaz canta.
 *
 * Terminología: la de Google Calendar / Outlook en cada lengua, y la MISMA que
 * Mail y Contacts («calendar», «guest», «reminder»): las tres apps se usan
 * seguidas y un invitado tiene que llamarse igual en las tres.
 *
 * NADA de frases pegadas con `+`: lo que en español es «Todo el día · 3 de
 * julio» es una PLANTILLA completa aquí, porque el orden de las palabras y el
 * separador cambian entre lenguas.
 */
import type { MessageEntry } from '@sytue/i18n';

export const en = {
  // --- Comunes ------------------------------------------------------------
  'common.cancel': 'Cancel',
  'common.save': 'Save',
  'common.delete': 'Delete',
  'common.edit': 'Edit',
  'common.close': 'Close',
  'common.remove': 'Remove',
  'common.retry': 'Retry',
  'common.done': 'Done',
  'common.loading': 'Loading…',
  'common.colour': 'Colour',

  // --- Página / layout ----------------------------------------------------
  'page.description': 'Sytue Calendar — the sovereign calendar of the Sytue One suite.',
  'page.newEvent': 'Sytue Calendar — New event',

  // --- Barra lateral ------------------------------------------------------
  'nav.createEvent': 'Create event',
  'nav.myCalendars': 'My calendars',
  'nav.manageResources': 'Manage rooms and equipment',
  'nav.resourcesTitle': 'Rooms and equipment',
  'nav.subscribeAria': 'Subscribe to a calendar by URL',
  'nav.subscribeTitle': 'Subscribe by URL (.ics)',
  'nav.createCalendar': 'Create calendar',
  'nav.noCalendars': 'No calendars',
  'nav.showCalendar': 'Show {name}',
  'nav.subscribedReadOnly': 'Subscribed calendar (read-only)',
  'nav.calendarOptions': 'Options for {name}',
  'nav.calendarOptionsTitle': 'Calendar options',
  'nav.refreshNow': 'Refresh now',
  'nav.rename': 'Rename',
  'nav.changeColour': 'Change colour',
  'nav.share': 'Share…',
  'nav.deleteSubscription': 'Remove subscription',
  'nav.sharedWithMe': 'Shared with me',
  'nav.sharedBy': 'Shared by {name}',
  'nav.permWriter': 'Editor',
  'nav.permReader': 'Viewer',
  'nav.permWriterTitle': 'You can edit',
  'nav.permReaderTitle': 'Read-only',
  'nav.prevMonth': 'Previous month',
  'nav.nextMonth': 'Next month',

  // --- Cabecera de la vista -----------------------------------------------
  'header.today': 'Today',
  'header.shortcut': '{label} ({key})',
  'header.previous': 'Previous',
  'header.next': 'Next',
  'header.search': 'Search events',
  'header.invitations': 'Invitations',
  'header.pending': { one: '{count} pending', other: '{count} pending' },
  'header.viewGroup': 'Calendar view',
  'view.month': 'Month',
  'view.week': 'Week',
  'view.day': 'Day',
  'view.agenda': 'Agenda',

  // --- Rejilla ------------------------------------------------------------
  'grid.loading': 'Loading the calendar',
  'grid.loadError': 'The calendar could not be loaded',
  'grid.createOn': 'Create an event on {date}',
  'grid.more': { one: '+{count} more', other: '+{count} more' },
  'grid.continued': '{title} (continued from earlier)',
  'grid.blockAria': '{title}, {time}',
  'grid.snapAria': {
    one: 'Snap dragging to {count} minute. Press to change.',
    other: 'Snap dragging to {count} minutes. Press to change.',
  },
  'grid.snapTitle': { one: 'Snapped to {count} min', other: 'Snapped to {count} min' },
  'grid.snapValue': { one: '{count} min', other: '{count} min' },

  // --- Vista agenda -------------------------------------------------------
  'agenda.search': 'Search events',
  'agenda.noMatches': 'No matches',
  'agenda.empty': 'No upcoming events',

  // --- Evento (comunes a todas las vistas) --------------------------------
  'event.untitled': '(no title)',
  'event.untitledLong': 'untitled event',
  'event.allDay': 'All day',

  // --- «Cuándo»: plantillas COMPLETAS, nunca trozos pegados ---------------
  'when.allDay': 'All day · {date}',
  'when.allDayRange': 'All day · {start} – {end}',
  'when.at': '{date}, {time}',
  'when.range': '{date}, {start} – {end}',
  'when.rangeAcross': '{startDate} {startTime} – {endDate} {endTime}',
  'when.timeRange': '{start} – {end}',

  // --- Vista rápida (popover) ---------------------------------------------
  'popover.details': 'Details of {title}',
  'popover.when': 'When',
  'popover.location': 'Location',
  'popover.conference': 'Video call',
  'popover.description': 'Description',
  'popover.join': 'Join the video call',
  'popover.duplicate': 'Duplicate',
  'popover.copyToMine': 'Copy to my calendar',
  'popover.readOnly': 'Subscribed calendar (read-only).',
  'notes.linked': 'Notes',
  'notes.create': 'Write a note for this event',
  'notes.untitled': 'Untitled note',

  // --- Modal de evento ----------------------------------------------------
  'event.viewTitle': 'View event',
  'event.editTitle': 'Edit event',
  'event.newTitle': 'New event',
  'event.readOnlyNotice':
    'This event belongs to a subscribed calendar (read-only). It cannot be edited.',
  'event.title': 'Title',
  'event.titlePlaceholder': 'Add a title',
  'event.ai': 'Describe the event',
  'event.aiPlaceholder': 'Lunch with Ana on Thursday at 2pm at Casa Lucio',
  'event.aiFill': 'Fill in',
  'event.aiError': "Couldn't work out an event from that. Try adding a day and a time.",
  'event.titleRequired': 'A title is required.',
  'event.calendar': 'Calendar',
  'event.start': 'Starts',
  'event.end': 'Ends',
  'event.endBeforeStart': 'The end cannot be earlier than the start.',
  'event.timeZone': 'Time zone',
  'event.findTimeShow': 'Find a time',
  'event.findTimeHide': 'Hide “find a time”',
  'event.repeat': 'Repeat',
  'event.repeatNever': 'Does not repeat',
  'event.repeatDaily': 'Daily',
  'event.repeatWeekly': 'Weekly',
  'event.repeatMonthly': 'Monthly',
  'event.repeatEnds': 'Ends',
  'event.repeatEndNever': 'Never',
  'event.repeatEndCount': 'After a number of times',
  'event.repeatEndUntil': 'On a date',
  'event.repeatCount': 'Number of repeats',
  'event.repeatUntil': 'Until date',
  'event.location': 'Location',
  'event.locationPlaceholder': 'Add a location (optional)',
  'event.conference': 'Video call',
  'event.conferenceRemoveAria': 'Remove the video call',
  'event.conferenceAdd': 'Add video call',
  'event.conferenceCreating': 'Creating…',
  'event.conferenceHint':
    'Creates a Sytue Rooms meeting for this event. Guests join from their own calendar, with nothing to install.',
  'event.conferenceError': 'The video call could not be created.',
  'event.conferenceDefaultSummary': 'Meeting',
  'event.resources': 'Rooms and equipment',
  'event.guests': 'Guests',
  'event.guestsAdded': 'Guests added',
  'event.guestRemove': 'Remove {email}',
  'event.guestPlaceholder': 'Add an email address and press Enter',
  'event.guestInvalid': 'Enter a valid email address.',
  'event.description': 'Description',
  'event.descriptionPlaceholder': 'Add notes or a description (optional)',
  'event.reminders': 'Reminders',
  'event.remindersNone': 'No reminders',
  'event.remindersList': 'Event reminders',
  'event.reminderAdd': 'Add reminder',
  'event.reminderSelect': 'Reminder {index}',
  'event.reminderAmount': 'Reminder {index} amount',
  'event.reminderUnit': 'Reminder {index} unit',
  'event.reminderRemove': 'Remove reminder {index}',
  'event.reminderCustom': 'Custom…',
  'event.unitMinutes': 'minutes before',
  'event.unitHours': 'hours before',
  'event.unitDays': 'days before',

  // --- Recordatorios: la etiqueta sale de la cantidad, no de una tabla ----
  'reminder.atTime': 'At the time of the event',
  'reminder.minutes': { one: '{count} minute before', other: '{count} minutes before' },
  'reminder.hours': { one: '{count} hour before', other: '{count} hours before' },
  'reminder.days': { one: '{count} day before', other: '{count} days before' },
  'reminder.weeks': { one: '{count} week before', other: '{count} weeks before' },

  // --- Alcance de una serie recurrente ------------------------------------
  'scope.editTitle': 'Edit recurring event',
  'scope.deleteTitle': 'Delete recurring event',
  'scope.legend': 'This event repeats. Which events does the change apply to?',
  'scope.this': 'This event only',
  'scope.following': 'This and following events',
  'scope.all': 'All events',

  // --- Modal de calendario -------------------------------------------------
  'calendar.newTitle': 'New calendar',
  'calendar.editTitle': 'Edit calendar',
  'calendar.helpTitle': 'What another calendar is for',
  'calendar.helpBody':
    'Keep apart what you do not want mixed up (Work, Personal, On call). It sits alongside the rest of your schedule, you hide it with one click and you share it separately.',
  'calendar.name': 'Name',
  'calendar.namePlaceholder': 'E.g. Work',
  'calendar.nameRequired': 'A name is required.',
  'calendar.nameHint':
    'This is how you will see it in your calendar list. You can change it later.',
  'calendar.colourHint':
    'Its events are drawn in this colour on the grid, so you can tell them apart at a glance.',
  'calendar.deleteTitle': 'Delete calendar',
  'calendar.deleteBody':
    '“{name}” will be deleted along with all its events, and anyone you shared it with will stop seeing it. This cannot be undone.',

  // --- Paleta de colores ---------------------------------------------------
  'colour.inherit': 'Calendar colour',
  'colour.terracotta': 'Terracotta',
  'colour.slate': 'Slate',
  'colour.pine': 'Pine',
  'colour.inkBlue': 'Ink blue',
  'colour.jade': 'Jade',
  'colour.ochre': 'Ochre',
  'colour.garnet': 'Garnet',
  'colour.plum': 'Plum',
  'colour.petrol': 'Petrol',
  'colour.orchid': 'Orchid',
  'colour.purple': 'Purple',
  'colour.indigo': 'Indigo',
  'colour.steel': 'Steel',
  'colour.viridian': 'Viridian',

  // --- Compartir calendario ------------------------------------------------
  'share.title': 'Share “{name}”',
  'share.titleFallback': 'Share calendar',
  'share.helpTitle': 'What will happen',
  'share.helpBody':
    'The other person will see this calendar next to their own, without having to accept anything. You can withdraw access whenever you want, and it only affects this calendar, not the rest of your schedule.',
  'share.email': 'Recipient’s email address',
  'share.emailPlaceholder': 'person@sytue.eu',
  'share.emailInvalid': 'Enter a valid email address.',
  'share.emailHint':
    'Someone from your organisation. If you enter the address of a list or a group, everyone in it will see your calendar.',
  'share.legend': 'What they will be able to do',
  'share.readerLabel': 'See my events',
  'share.readerHint':
    'Sees the title, the time and the details of each event, but cannot change anything.',
  'share.writerLabel': 'See and edit my events',
  'share.writerHint':
    'As well as seeing them, creates, changes and deletes events in this calendar.',
  'share.manageLabel': 'Manage my calendar for me',
  'share.manageHint':
    'Everything above and, on top of that, organises and replies on your behalf. For whoever runs your schedule day to day.',
  'share.warnTitle': 'They will be able to change your events',
  'share.warnWriter':
    'They will be able to create, move and delete events in this calendar. The changes show up in your schedule straight away.',
  'share.warnManage':
    'Creating, moving and deleting events, and answering invitations as if they were you. Only give this to whoever runs your schedule.',
  'share.submit': 'Share',
  'share.whoSees': 'Who can see it now',
  'share.emptyTitle': 'Only you can see this calendar',
  'share.emptyDescription':
    'Once you share it, you will see here who with and what each person can do.',
  'share.badgeReader': 'Only sees',
  'share.badgeWriter': 'Sees and edits',
  'share.badgeManage': 'Runs your schedule',
  'share.revoke': 'Stop sharing with {email}',
  'share.revokeTitle': 'Revoke',

  // --- Suscripción a un .ics -----------------------------------------------
  'subscribe.title': 'Subscribe to a calendar',
  'subscribe.submit': 'Subscribe',
  'subscribe.intro':
    'Add a public calendar by its URL (an .ics file, such as a country’s public holidays or a team calendar). It is added as read-only and updates automatically.',
  'subscribe.url': 'Calendar URL (.ics)',
  'subscribe.urlPlaceholder': 'https://example.com/holidays.ics',
  'subscribe.urlInvalid': 'Enter a valid http(s) URL of an .ics file.',
  'subscribe.name': 'Name',
  'subscribe.namePlaceholder': 'E.g. Public holidays',
  'subscribe.defaultName': 'Subscribed calendar',

  // --- Invitaciones ---------------------------------------------------------
  'invitations.title': 'Invitations',
  'invitations.loading': 'Loading invitations',
  'invitations.emptyTitle': 'You have no pending invitations',
  'invitations.emptyDescription':
    'When someone invites you to an event, it will show up here for you to answer.',
  'invitations.help':
    'Answer so that whoever is organising knows whether to count on you. If you say yes, the event is saved to your calendar; you can change your answer later from the event itself.',
  'invitations.untitled': 'Untitled event',
  'invitations.invitedBy': '{name} has invited you',
  'invitations.invitedByFrom': '{name} ({org}) has invited you',
  'invitations.generic': 'Calendar invitation',
  'invitations.whoWhen': '{who} · {when}',
  'invitations.accept': 'Yes, I’ll be there',
  'invitations.acceptHint': 'It is saved to your calendar',
  'invitations.tentative': 'Maybe',
  'invitations.tentativeHint': 'Saved, but marked as tentative',
  'invitations.decline': 'I can’t',
  'invitations.declineHint': 'It is not saved to your calendar',

  // --- Centro de recordatorios ---------------------------------------------
  'reminders.title': 'Reminders',
  'reminders.bell': 'Reminders',
  'reminders.bellCount': { one: 'Reminders ({count})', other: 'Reminders ({count})' },
  'reminders.loading': 'Loading',
  'reminders.loadError': 'Reminders could not be loaded.',
  'reminders.empty': 'No pending reminders',
  'reminders.dismiss': 'Dismiss the reminder for {title}',
  'reminders.eventFallback': 'event',
  'reminders.enableNotifications': 'Turn on browser alerts',
  'reminders.fallbackTitle': 'Reminder',
  'reminders.body': 'Event',
  'reminders.bodyAt': 'Event · {time}',
  'reminders.view': 'View',
  'reminders.today': 'Today',
  'reminders.when': '{day} · {time}',

  // --- Búsqueda -------------------------------------------------------------
  'search.title': 'Search events',
  'search.placeholder': 'Title, description, location or guest…',
  'search.minChars': {
    one: 'Type at least {count} character to search your calendars.',
    other: 'Type at least {count} characters to search your calendars.',
  },
  'search.searching': 'Searching…',
  'search.error': 'The search could not be completed.',
  'search.noResults': 'No results for “{term}”.',
  'search.series': 'Series',
  'search.loadMore': 'Load more',

  // --- Buscar un hueco ------------------------------------------------------
  'findTime.summary': 'Common free slots · Mon–Fri {start}–{end} · {duration} min',
  'findTime.weekdaysOnly': 'Weekdays only',
  'findTime.window': 'Search window',
  'findTime.windowDays': { one: '{count} day', other: '{count} days' },
  'findTime.loading': 'Searching for free slots',
  'findTime.error': 'Availability could not be checked.',
  'findTime.empty': 'No free slots in this window. Try widening it or making the event shorter.',

  // --- Salas y recursos (gestión) -------------------------------------------
  'resources.title': 'Rooms and equipment',
  'resources.kindRoom': 'Room',
  'resources.kindEquipment': 'Equipment',
  'resources.helpTitle': 'What a bookable resource is',
  'resources.helpBody':
    'A room or a piece of equipment that people add to their meetings. When creating an event it shows up under “Rooms and equipment” with its capacity and its location, and only one person at a time can book it: if it is already taken at that time, it shows as busy.',
  'resources.formNewTitle': 'New resource',
  'resources.formEditTitle': 'Edit resource',
  'resources.formNewDesc': 'This is how your team will see it when looking for somewhere to meet.',
  'resources.formEditDesc':
    'The changes show up straight away in new bookings; the ones already made stay as they are.',
  'resources.kindFixed': '{kind} · the type cannot be changed once it has been created',
  'resources.legend': 'What you are adding',
  'resources.roomTitle': 'A room',
  'resources.roomDesc':
    'A physical space where people meet. It is shown with its capacity and its floor.',
  'resources.equipmentTitle': 'A piece of equipment',
  'resources.equipmentDesc': 'Something taken to the meeting: a projector, a car, a loan laptop.',
  'resources.name': 'Name',
  'resources.namePlaceholder': 'Marie Curie room',
  'resources.nameRequired': 'A name is required.',
  'resources.nameHint': 'This is what people read when booking: make it recognisable on its own.',
  'resources.capacity': 'Capacity',
  'resources.capacityPlaceholder': 'Optional',
  'resources.capacityHint':
    'How many people fit. It helps avoid booking a room for 4 when there are 12.',
  'resources.location': 'Location',
  'resources.locationPlaceholder': 'Floor 2',
  'resources.locationHint': 'Where it is, for whoever does not know: “Floor 2”, “Building B”.',
  'resources.email': 'Resource email address',
  'resources.emailPlaceholder': 'room@sytue.eu',
  'resources.emailHint':
    'Optional. It lets people invite it from outside Sytue, like any other guest.',
  'resources.autoAccept': 'Accept automatically when free',
  'resources.autoAcceptOn':
    'Whoever books it has it confirmed straight away if nobody has taken it first.',
  'resources.autoAcceptOff': 'Every booking stays pending until someone approves it by hand.',
  'resources.create': 'Create resource',
  'resources.catalogueTitle': 'Rooms and equipment on file',
  'resources.catalogueDesc': 'This is what your team can book today.',
  'resources.emptyTitle': 'Nothing to book yet',
  'resources.emptyDescription':
    'Add the first room or the first piece of equipment above: it will show up in the resource picker when creating an event.',
  'resources.emptyAction': 'Add a room',
  'resources.people': { one: '{count} person', other: '{count} people' },
  'resources.edit': 'Edit {name}',
  'resources.remove': 'Remove {name}',
  'resources.removeTitle': 'Remove',

  // --- Selector de recursos (dentro del evento) -----------------------------
  'picker.error': 'Resource availability could not be checked.',
  'picker.list': 'Booked resources',
  'picker.remove': 'Remove {name}',
  'picker.removeTitle': 'Remove',
  'picker.fallbackName': 'resource',
  'picker.add': 'Add a room or resource',
  'picker.addPlaceholder': 'Add a room or resource…',
  'picker.option': '{name} — {state}',
  'picker.optionWithCapacity': '{name} · {capacity} — {state}',
  'picker.free': 'free',
  'picker.busy': 'busy',
  'picker.allAdded': 'Every resource has been added.',
  'picker.badgeBooked': 'Booked',
  'picker.badgeBusy': 'Busy',
  'picker.badgePending': 'Pending',
  'picker.badgeFree': 'Free',

  // --- Reservas de recursos al guardar --------------------------------------
  'booking.booked': { one: '{count} resource booked', other: '{count} resources booked' },
  'booking.busy': { one: '{count} unavailable', other: '{count} unavailable' },
  'booking.saved': 'Event saved · {booked}',
  'booking.savedBusy': 'Event saved · {booked}, {busy}',

  // --- Errores de carga -----------------------------------------------------
  'error.calendarsLoad': 'The calendars could not be loaded.',
  'error.eventsLoad': 'The events could not be loaded.',

  // --- Avisos (toasts) ------------------------------------------------------
  'toast.selectCalendar': 'Select a calendar',
  'toast.eventCreated': 'Event created',
  'toast.eventUpdated': 'Event updated',
  'toast.eventSaveFailed': 'The event could not be saved',
  'toast.eventDeleted': 'Event deleted',
  'toast.eventDeleteFailed': 'The event could not be deleted',
  'toast.subscriptionRefreshed': 'Subscription updated',
  'toast.subscriptionRefreshFailed': 'The subscription could not be updated',
  'toast.calendarCreated': 'Calendar created',
  'toast.calendarUpdated': 'Calendar updated',
  'toast.calendarSaveFailed': 'The calendar could not be saved',
  'toast.calendarDeleted': 'Calendar deleted',
  'toast.calendarDeleteFailed': 'The calendar could not be deleted',
  'toast.calendarSubscribed': 'Calendar subscribed',
  'toast.subscribeFailed': 'Could not subscribe',
  'toast.subscribeFailedHint': 'Check the URL: it has to be a public .ics reachable over http(s).',
  'toast.sharesLoadFailed': 'The sharing settings could not be loaded',
  'toast.calendarShared': 'Calendar shared',
  'toast.shareFailed': 'The calendar could not be shared',
  'toast.shareRevoked': 'Sharing revoked',
  'toast.revokeFailed': 'Could not revoke',
  'toast.resourcesLoadFailed': 'The resources could not be loaded',
  'toast.resourceCreated': 'Resource created',
  'toast.resourceUpdated': 'Resource updated',
  'toast.resourceSaveFailed': 'The resource could not be saved',
  'toast.resourceRemoved': 'Resource removed',
  'toast.resourceRemoveFailed': 'The resource could not be removed',
  'toast.invitationsLoadFailed': 'The invitations could not be loaded',
  'toast.invitationAccepted': 'Done: the event is in your calendar',
  'toast.invitationDeclined': 'Done: you said you can’t make it',
  'toast.invitationTentative': 'Done: you answered “maybe”',
  'toast.invitationFailed': 'The invitation could not be answered',
  'toast.reminderDismissFailed': 'The reminder could not be dismissed',
  'nav.prev': 'Previous',
  'nav.next': 'Next',
  'cal.create': 'Create calendar',
  'cal.options': 'Calendar options',
  'cal.refresh': 'Refresh now',
  'cal.share': 'Share…',
  'cal.downloadIcs': 'Download .ics',
  'cal.sharedWithMe': 'Shared with me',
  'resources.manage': 'Manage rooms and resources',
  'subscribe.aria': 'Subscribe to a calendar by URL',
  'subscribe.byUrl': 'Subscribe by URL (.ics)',
  'search.events': 'Search events',
  'view.label': 'Calendar view',
  'load.error': 'The calendar could not be loaded',
  'loading.calendar': 'Loading the calendar',
  'page.title': 'Sytue Calendar',
  'page.newEventTitle': 'Sytue Calendar — New event',
  'share.whatHappens': 'What will happen',
  'share.canEdit': 'They will be able to change your events',
  'share.whoSeesNow': 'Who sees it now',
  'share.onlyYou': 'Only you see this calendar',
  'share.revokeAction': 'Revoke',

  // --- Rótulos que estaban en duro ---
  'share.recipientEmail': 'Recipient email',
  'cal.subscribedReadOnly': 'Subscribed calendar (read-only)',

  // --- Errores y textos de permisos que estaban en duro ---
  'cal.fromEarlier': ' (from earlier)',
  'cal.noUpcoming': 'No upcoming events',
  'cal.sub.delete': 'Delete subscription',
  'cal.sub.updated': 'Subscription updated',
  'cal.error.loadCalendars': 'Could not load the calendars.',
  'cal.error.loadEvents': 'Could not load the events.',
  'cal.error.noAccess': 'You no longer have access to “{name}”.',
  'cal.error.updateEvent': 'Could not update the event',
  'cal.error.saveEvent': 'Could not save the event',
  'cal.error.deleteEvent': 'Could not delete the event',
  'cal.error.updateSub': 'Could not update the subscription',
  'share.recipient.hint':
    'Someone in your organisation. If you enter the address of a list or a group, everyone in it will see your calendar.',
  'share.role.manage.hint':
    'Create, move and delete events, and reply to invitations as you. Only give this to whoever manages your diary.',
  'share.role.write.hint':
    'They will be able to create, move and delete events in this calendar. Changes show in your diary straight away.',
  'nav.createEventOn': 'Create event on {date}',
  'share.empty.hint':
    'Once you share it, this is where you will see who has access and what each person can do.',
  'popover.respond': 'Are you going?',
  'popover.yes': 'Yes',
  'popover.maybe': 'Maybe',
  'popover.no': 'No',
  'popover.responded': 'Reply sent.',
  'popover.respondFailed': 'The reply could not be sent.',
} satisfies Record<string, MessageEntry>;
