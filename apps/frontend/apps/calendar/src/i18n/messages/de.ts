/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ALEMÁN de Sytue Calendar. Terminología de Google Kalender / Outlook:
 * «Termin», «Teilnehmer», «Erinnerung», «Ganztägig», «Kalender», «Serientermin».
 * La misma que Mail y Contacts. Comillas „…“.
 *
 * El alemán es ~30 % más largo que el inglés y esta interfaz es una REJILLA
 * DENSA: las etiquetas que caen en una celda de día, en un bloque de evento o en
 * el conmutador de vista se han mantenido cortas a propósito («Übersicht» en vez
 * de «Terminübersicht», «Min.» en vez de «Minuten»), porque ahí no hay sitio y
 * el contexto ya lo dice. docs/CONVENTIONS.md §13.1.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const de: AppCatalog<typeof en> = {
  // --- Comunes ------------------------------------------------------------
  'common.cancel': 'Abbrechen',
  'common.save': 'Speichern',
  'common.delete': 'Löschen',
  'common.edit': 'Bearbeiten',
  'common.close': 'Schließen',
  'common.remove': 'Entfernen',
  'common.retry': 'Erneut versuchen',
  'common.done': 'Fertig',
  'common.loading': 'Wird geladen…',
  'common.colour': 'Farbe',

  // --- Página / layout ----------------------------------------------------
  'page.description': 'Sytue Calendar — der souveräne Kalender der Sytue-One-Suite.',
  'page.newEvent': 'Sytue Calendar — Neuer Termin',

  // --- Barra lateral ------------------------------------------------------
  'nav.createEvent': 'Termin erstellen',
  'nav.myCalendars': 'Meine Kalender',
  'nav.manageResources': 'Räume und Ressourcen verwalten',
  'nav.resourcesTitle': 'Räume und Ressourcen',
  'nav.subscribeAria': 'Kalender per URL abonnieren',
  'nav.subscribeTitle': 'Per URL abonnieren (.ics)',
  'nav.createCalendar': 'Kalender erstellen',
  'nav.noCalendars': 'Keine Kalender',
  'nav.showCalendar': '{name} anzeigen',
  'nav.subscribedReadOnly': 'Abonnierter Kalender (nur lesen)',
  'nav.calendarOptions': 'Optionen für {name}',
  'nav.calendarOptionsTitle': 'Kalenderoptionen',
  'nav.refreshNow': 'Jetzt aktualisieren',
  'nav.rename': 'Umbenennen',
  'nav.changeColour': 'Farbe ändern',
  'nav.share': 'Freigeben…',
  'nav.deleteSubscription': 'Abo entfernen',
  'nav.sharedWithMe': 'Für mich freigegeben',
  'nav.sharedBy': 'Freigegeben von {name}',
  'nav.permWriter': 'Bearbeiten',
  'nav.permReader': 'Lesen',
  'nav.permWriterTitle': 'Du kannst bearbeiten',
  'nav.permReaderTitle': 'Nur lesen',
  'nav.prevMonth': 'Voriger Monat',
  'nav.nextMonth': 'Nächster Monat',

  // --- Cabecera de la vista -----------------------------------------------
  'header.today': 'Heute',
  'header.shortcut': '{label} ({key})',
  'header.previous': 'Zurück',
  'header.next': 'Weiter',
  'header.search': 'Termine suchen',
  'header.invitations': 'Einladungen',
  'header.pending': { one: '{count} offen', other: '{count} offen' },
  'header.viewGroup': 'Kalenderansicht',
  'view.month': 'Monat',
  'view.week': 'Woche',
  'view.day': 'Tag',
  'view.agenda': 'Übersicht',

  // --- Rejilla ------------------------------------------------------------
  'grid.loading': 'Kalender wird geladen',
  'grid.loadError': 'Der Kalender konnte nicht geladen werden',
  'grid.createOn': 'Termin am {date} erstellen',
  'grid.more': { one: '+{count} weiterer', other: '+{count} weitere' },
  'grid.continued': '{title} (Fortsetzung)',
  'grid.blockAria': '{title}, {time}',
  'grid.snapAria': {
    one: 'Ziehen auf {count} Minute einrasten. Zum Ändern drücken.',
    other: 'Ziehen auf {count} Minuten einrasten. Zum Ändern drücken.',
  },
  'grid.snapTitle': { one: 'Raster: {count} Min.', other: 'Raster: {count} Min.' },
  'grid.snapValue': { one: '{count} Min.', other: '{count} Min.' },

  // --- Vista agenda -------------------------------------------------------
  'agenda.search': 'Termine suchen',
  'agenda.noMatches': 'Keine Treffer',
  'agenda.empty': 'Keine anstehenden Termine',

  // --- Evento (comunes a todas las vistas) --------------------------------
  'event.untitled': '(ohne Titel)',
  'event.untitledLong': 'Termin ohne Titel',
  'event.allDay': 'Ganztägig',

  // --- «Cuándo» -----------------------------------------------------------
  'when.allDay': 'Ganztägig · {date}',
  'when.allDayRange': 'Ganztägig · {start} – {end}',
  'when.at': '{date}, {time}',
  'when.range': '{date}, {start} – {end}',
  'when.rangeAcross': '{startDate} {startTime} – {endDate} {endTime}',
  'when.timeRange': '{start} – {end}',

  // --- Vista rápida (popover) ---------------------------------------------
  'popover.details': 'Details zu {title}',
  'popover.when': 'Wann',
  'popover.location': 'Ort',
  'popover.conference': 'Videoanruf',
  'popover.description': 'Beschreibung',
  'popover.join': 'Am Videoanruf teilnehmen',
  'popover.duplicate': 'Duplizieren',
  'popover.copyToMine': 'In meinen Kalender kopieren',
  'popover.readOnly': 'Abonnierter Kalender (nur lesen).',
  'notes.linked': 'Notizen',
  'notes.create': 'Notiz zu diesem Termin schreiben',
  'notes.untitled': 'Notiz ohne Titel',

  // --- Modal de evento ----------------------------------------------------
  'event.viewTitle': 'Termin ansehen',
  'event.editTitle': 'Termin bearbeiten',
  'event.newTitle': 'Neuer Termin',
  'event.readOnlyNotice':
    'Dieser Termin gehört zu einem abonnierten Kalender (nur lesen). Er kann nicht bearbeitet werden.',
  'event.title': 'Titel',
  'event.titlePlaceholder': 'Titel hinzufügen',
  'event.ai': 'Beschreiben Sie den Termin',
  'event.aiPlaceholder': 'Mittagessen mit Ana am Donnerstag um 14:00 Uhr im Casa Lucio',
  'event.aiFill': 'Ausfüllen',
  'event.aiError': 'Daraus ließ sich kein Termin ableiten. Geben Sie Tag und Uhrzeit an.',
  'event.titleRequired': 'Ein Titel ist erforderlich.',
  'event.calendar': 'Kalender',
  'event.start': 'Beginn',
  'event.end': 'Ende',
  'event.endBeforeStart': 'Das Ende darf nicht vor dem Beginn liegen.',
  'event.timeZone': 'Zeitzone',
  'event.findTimeShow': 'Zeit finden',
  'event.findTimeHide': '„Zeit finden“ ausblenden',
  'event.repeat': 'Wiederholen',
  'event.repeatNever': 'Wiederholt sich nicht',
  'event.repeatDaily': 'Täglich',
  'event.repeatWeekly': 'Wöchentlich',
  'event.repeatMonthly': 'Monatlich',
  'event.repeatEnds': 'Endet',
  'event.repeatEndNever': 'Nie',
  'event.repeatEndCount': 'Nach einer Anzahl von Malen',
  'event.repeatEndUntil': 'An einem Datum',
  'event.repeatCount': 'Anzahl der Wiederholungen',
  'event.repeatUntil': 'Bis zum Datum',
  'event.location': 'Ort',
  'event.locationPlaceholder': 'Ort hinzufügen (optional)',
  'event.conference': 'Videoanruf',
  'event.conferenceRemoveAria': 'Videoanruf entfernen',
  'event.conferenceAdd': 'Videoanruf hinzufügen',
  'event.conferenceCreating': 'Wird erstellt…',
  'event.conferenceHint':
    'Erstellt einen Sytue-Rooms-Raum für diesen Termin. Die Eingeladenen treten aus ihrem Kalender bei, ohne etwas zu installieren.',
  'event.conferenceError': 'Der Videoanruf konnte nicht erstellt werden.',
  'event.conferenceDefaultSummary': 'Besprechung',
  'event.resources': 'Räume und Ressourcen',
  'event.guests': 'Eingeladene',
  'event.guestsAdded': 'Eingeladene hinzugefügt',
  'event.guestRemove': '{email} entfernen',
  'event.guestPlaceholder': 'E-Mail-Adresse eingeben und Enter drücken',
  'event.guestInvalid': 'Gib eine gültige E-Mail-Adresse ein.',
  'event.description': 'Beschreibung',
  'event.descriptionPlaceholder': 'Notizen oder eine Beschreibung hinzufügen (optional)',
  'event.reminders': 'Erinnerungen',
  'event.remindersNone': 'Keine Erinnerungen',
  'event.remindersList': 'Erinnerungen des Termins',
  'event.reminderAdd': 'Erinnerung hinzufügen',
  'event.reminderSelect': 'Erinnerung {index}',
  'event.reminderAmount': 'Wert der Erinnerung {index}',
  'event.reminderUnit': 'Einheit der Erinnerung {index}',
  'event.reminderRemove': 'Erinnerung {index} entfernen',
  'event.reminderCustom': 'Benutzerdefiniert…',
  'event.unitMinutes': 'Minuten vorher',
  'event.unitHours': 'Stunden vorher',
  'event.unitDays': 'Tage vorher',

  // --- Recordatorios ------------------------------------------------------
  'reminder.atTime': 'Zum Zeitpunkt des Termins',
  'reminder.minutes': { one: '{count} Minute vorher', other: '{count} Minuten vorher' },
  'reminder.hours': { one: '{count} Stunde vorher', other: '{count} Stunden vorher' },
  'reminder.days': { one: '{count} Tag vorher', other: '{count} Tage vorher' },
  'reminder.weeks': { one: '{count} Woche vorher', other: '{count} Wochen vorher' },

  // --- Alcance de una serie recurrente ------------------------------------
  'scope.editTitle': 'Serientermin bearbeiten',
  'scope.deleteTitle': 'Serientermin löschen',
  'scope.legend': 'Dieser Termin wiederholt sich. Für welche Termine gilt die Änderung?',
  'scope.this': 'Nur diesen Termin',
  'scope.following': 'Diesen und alle folgenden',
  'scope.all': 'Alle Termine',

  // --- Modal de calendario -------------------------------------------------
  'calendar.newTitle': 'Neuer Kalender',
  'calendar.editTitle': 'Kalender bearbeiten',
  'calendar.helpTitle': 'Wozu ein weiterer Kalender gut ist',
  'calendar.helpBody':
    'Trenne, was du nicht vermischen willst (Arbeit, Privat, Bereitschaft). Er erscheint neben dem Rest deines Kalenders, du blendest ihn mit einem Klick aus und gibst ihn separat frei.',
  'calendar.name': 'Name',
  'calendar.namePlaceholder': 'z. B. Arbeit',
  'calendar.nameRequired': 'Ein Name ist erforderlich.',
  'calendar.nameHint': 'So siehst du ihn in deiner Kalenderliste. Du kannst ihn später ändern.',
  'calendar.colourHint':
    'In dieser Farbe erscheinen seine Termine im Raster, damit du sie auf einen Blick unterscheidest.',
  'calendar.deleteTitle': 'Kalender löschen',
  'calendar.deleteBody':
    '„{name}“ wird mit allen Terminen gelöscht, und wer ihn freigegeben hatte, sieht ihn nicht mehr. Das lässt sich nicht rückgängig machen.',

  // --- Paleta de colores ---------------------------------------------------
  'colour.inherit': 'Farbe des Kalenders',
  'colour.terracotta': 'Terrakotta',
  'colour.slate': 'Schiefer',
  'colour.pine': 'Kiefer',
  'colour.inkBlue': 'Tintenblau',
  'colour.jade': 'Jade',
  'colour.ochre': 'Ocker',
  'colour.garnet': 'Granat',
  'colour.plum': 'Pflaume',
  'colour.petrol': 'Petrol',
  'colour.orchid': 'Orchidee',
  'colour.purple': 'Purpur',
  'colour.indigo': 'Indigo',
  'colour.steel': 'Stahl',
  'colour.viridian': 'Viridian',

  // --- Compartir calendario ------------------------------------------------
  'share.title': '„{name}“ freigeben',
  'share.titleFallback': 'Kalender freigeben',
  'share.helpTitle': 'Was passieren wird',
  'share.helpBody':
    'Die Person sieht diesen Kalender neben ihrem eigenen, ohne etwas annehmen zu müssen. Du kannst den Zugriff jederzeit entziehen, und es betrifft nur diesen Kalender, nicht den Rest deines Kalenders.',
  'share.email': 'E-Mail-Adresse der Person, mit der Sie teilen',
  'share.emailPlaceholder': 'person@sytue.eu',
  'share.emailInvalid': 'Gib eine gültige E-Mail-Adresse ein.',
  'share.emailHint':
    'Eine Person aus deiner Organisation. Wenn du die Adresse einer Liste oder Gruppe angibst, sehen alle darin deinen Kalender.',
  'share.legend': 'Was die Person tun darf',
  'share.readerLabel': 'Meine Termine sehen',
  'share.readerHint': 'Sieht Titel, Uhrzeit und Details jedes Termins, kann aber nichts ändern.',
  'share.writerLabel': 'Meine Termine sehen und bearbeiten',
  'share.writerHint':
    'Sieht sie nicht nur, sondern erstellt, ändert und löscht auch Termine in diesem Kalender.',
  'share.manageLabel': 'Meinen Kalender für mich führen',
  'share.manageHint':
    'Alles Vorige und zusätzlich: organisiert und antwortet in deinem Namen. Für die Person, die deinen Kalender täglich führt.',
  'share.warnTitle': 'Die Person kann deine Termine ändern',
  'share.warnWriter':
    'Sie kann Termine in diesem Kalender erstellen, verschieben und löschen. Die Änderungen erscheinen sofort in deinem Kalender.',
  'share.warnManage':
    'Termine erstellen, verschieben und löschen sowie Einladungen beantworten, als wärst du es. Gib das nur der Person, die deinen Kalender führt.',
  'share.submit': 'Freigeben',
  'share.whoSees': 'Wer ihn jetzt sieht',
  'share.emptyTitle': 'Nur du siehst diesen Kalender',
  'share.emptyDescription':
    'Sobald du ihn freigibst, siehst du hier, mit wem und was jede Person tun darf.',
  'share.badgeReader': 'Sieht nur',
  'share.badgeWriter': 'Sieht und bearbeitet',
  'share.badgeManage': 'Führt deinen Kalender',
  'share.revoke': 'Freigabe für {email} beenden',
  'share.revokeTitle': 'Entziehen',

  // --- Suscripción a un .ics -----------------------------------------------
  'subscribe.title': 'Kalender abonnieren',
  'subscribe.submit': 'Abonnieren',
  'subscribe.intro':
    'Füge einen öffentlichen Kalender über seine URL hinzu (eine .ics-Datei, etwa die Feiertage eines Landes oder der Kalender eines Teams). Er wird nur zum Lesen hinzugefügt und aktualisiert sich automatisch.',
  'subscribe.url': 'Kalender-URL (.ics)',
  'subscribe.urlPlaceholder': 'https://beispiel.de/feiertage.ics',
  'subscribe.urlInvalid': 'Gib eine gültige http(s)-URL einer .ics-Datei ein.',
  'subscribe.name': 'Name',
  'subscribe.namePlaceholder': 'z. B. Feiertage',
  'subscribe.defaultName': 'Abonnierter Kalender',

  // --- Invitaciones ---------------------------------------------------------
  'invitations.title': 'Einladungen',
  'invitations.loading': 'Einladungen werden geladen',
  'invitations.emptyTitle': 'Du hast keine offenen Einladungen',
  'invitations.emptyDescription':
    'Wenn dich jemand zu einem Termin einlädt, erscheint die Einladung hier zum Beantworten.',
  'invitations.help':
    'Antworte, damit die organisierende Person weiß, ob sie mit dir rechnen kann. Wenn du zusagst, wird der Termin in deinem Kalender gespeichert; du kannst deine Antwort später am Termin selbst ändern.',
  'invitations.untitled': 'Termin ohne Titel',
  'invitations.invitedBy': '{name} lädt dich ein',
  'invitations.invitedByFrom': '{name} ({org}) lädt dich ein',
  'invitations.generic': 'Kalendereinladung',
  'invitations.whoWhen': '{who} · {when}',
  'invitations.accept': 'Ja, ich komme',
  'invitations.acceptHint': 'Wird in deinem Kalender gespeichert',
  'invitations.tentative': 'Vielleicht',
  'invitations.tentativeHint': 'Wird gespeichert, aber als unter Vorbehalt markiert',
  'invitations.decline': 'Ich kann nicht',
  'invitations.declineHint': 'Wird nicht in deinem Kalender gespeichert',

  // --- Centro de recordatorios ---------------------------------------------
  'reminders.title': 'Erinnerungen',
  'reminders.bell': 'Erinnerungen',
  'reminders.bellCount': { one: 'Erinnerungen ({count})', other: 'Erinnerungen ({count})' },
  'reminders.loading': 'Wird geladen',
  'reminders.loadError': 'Die Erinnerungen konnten nicht geladen werden.',
  'reminders.empty': 'Keine offenen Erinnerungen',
  'reminders.dismiss': 'Erinnerung für {title} verwerfen',
  'reminders.eventFallback': 'Termin',
  'reminders.enableNotifications': 'Browser-Hinweise aktivieren',
  'reminders.fallbackTitle': 'Erinnerung',
  'reminders.body': 'Termin',
  'reminders.bodyAt': 'Termin · {time}',
  'reminders.view': 'Ansehen',
  'reminders.today': 'Heute',
  'reminders.when': '{day} · {time}',

  // --- Búsqueda -------------------------------------------------------------
  'search.title': 'Termine suchen',
  'search.placeholder': 'Titel, Beschreibung, Ort oder eingeladene Person…',
  'search.minChars': {
    one: 'Gib mindestens {count} Zeichen ein, um deine Kalender zu durchsuchen.',
    other: 'Gib mindestens {count} Zeichen ein, um deine Kalender zu durchsuchen.',
  },
  'search.searching': 'Wird gesucht…',
  'search.error': 'Die Suche konnte nicht abgeschlossen werden.',
  'search.noResults': 'Keine Treffer für „{term}“.',
  'search.series': 'Serie',
  'search.loadMore': 'Mehr laden',

  // --- Buscar un hueco ------------------------------------------------------
  'findTime.summary': 'Gemeinsame Zeiten · Mo-Fr {start}–{end} · {duration} Min.',
  'findTime.weekdaysOnly': 'Nur Werktage',
  'findTime.window': 'Suchzeitraum',
  'findTime.windowDays': { one: '{count} Tag', other: '{count} Tage' },
  'findTime.loading': 'Freie Zeiten werden gesucht',
  'findTime.error': 'Die Verfügbarkeit konnte nicht abgefragt werden.',
  'findTime.empty': 'Keine freien Zeiten in diesem Zeitraum. Erweitere ihn oder kürze den Termin.',

  // --- Salas y recursos (gestión) -------------------------------------------
  'resources.title': 'Räume und Ressourcen',
  'resources.kindRoom': 'Raum',
  'resources.kindEquipment': 'Ausstattung',
  'resources.helpTitle': 'Was eine buchbare Ressource ist',
  'resources.helpBody':
    'Ein Raum oder ein Gerät, das Leute zu ihren Besprechungen hinzufügen. Beim Erstellen eines Termins erscheint es unter „Räume und Ressourcen“ mit Kapazität und Ort, und nur eine Person kann es gleichzeitig buchen: Ist es zu dieser Zeit schon vergeben, erscheint es als belegt.',
  'resources.formNewTitle': 'Neue Ressource',
  'resources.formEditTitle': 'Ressource bearbeiten',
  'resources.formNewDesc': 'So sieht dein Team sie, wenn es einen Ort zum Treffen sucht.',
  'resources.formEditDesc':
    'Die Änderungen gelten sofort für neue Buchungen; die bereits bestehenden bleiben unverändert.',
  'resources.kindFixed': '{kind} · Der Typ lässt sich nach dem Anlegen nicht mehr ändern',
  'resources.legend': 'Was du anlegst',
  'resources.roomTitle': 'Ein Raum',
  'resources.roomDesc':
    'Ein physischer Ort, an dem sich Leute treffen. Er wird mit Kapazität und Etage angezeigt.',
  'resources.equipmentTitle': 'Ein Gerät',
  'resources.equipmentDesc':
    'Etwas, das zur Besprechung mitgebracht wird: ein Beamer, ein Auto, ein Leihlaptop.',
  'resources.name': 'Name',
  'resources.namePlaceholder': 'Raum Marie Curie',
  'resources.nameRequired': 'Ein Name ist erforderlich.',
  'resources.nameHint': 'Das liest man beim Buchen: Er sollte ohne Erklärung erkennbar sein.',
  'resources.capacity': 'Kapazität',
  'resources.capacityPlaceholder': 'Optional',
  'resources.capacityHint':
    'Wie viele Personen hineinpassen. So bucht niemand einen Raum für 4, wenn 12 kommen.',
  'resources.location': 'Ort',
  'resources.locationPlaceholder': 'Etage 2',
  'resources.locationHint': 'Wo er liegt, für alle, die es nicht wissen: „Etage 2“, „Gebäude B“.',
  'resources.email': 'E-Mail-Adresse der Ressource',
  'resources.emailPlaceholder': 'raum@sytue.eu',
  'resources.emailHint':
    'Optional. Damit lässt sie sich von außerhalb von Sytue einladen, wie jede andere Einladung.',
  'resources.autoAccept': 'Automatisch annehmen, wenn frei',
  'resources.autoAcceptOn':
    'Wer sie bucht, hat sie sofort bestätigt, sofern sie niemand vorher genommen hat.',
  'resources.autoAcceptOff': 'Jede Buchung bleibt offen, bis sie jemand von Hand freigibt.',
  'resources.create': 'Ressource anlegen',
  'resources.catalogueTitle': 'Angelegte Räume und Ressourcen',
  'resources.catalogueDesc': 'Das kann dein Team heute buchen.',
  'resources.emptyTitle': 'Noch nichts zu buchen',
  'resources.emptyDescription':
    'Lege oben den ersten Raum oder das erste Gerät an: Es erscheint beim Erstellen eines Termins in der Ressourcenauswahl.',
  'resources.emptyAction': 'Einen Raum anlegen',
  'resources.people': { one: '{count} Person', other: '{count} Personen' },
  'resources.edit': '{name} bearbeiten',
  'resources.remove': '{name} entfernen',
  'resources.removeTitle': 'Entfernen',

  // --- Selector de recursos (dentro del evento) -----------------------------
  'picker.error': 'Die Verfügbarkeit der Ressourcen konnte nicht abgefragt werden.',
  'picker.list': 'Gebuchte Ressourcen',
  'picker.remove': '{name} entfernen',
  'picker.removeTitle': 'Entfernen',
  'picker.fallbackName': 'Ressource',
  'picker.add': 'Raum oder Ressource hinzufügen',
  'picker.addPlaceholder': 'Raum oder Ressource hinzufügen…',
  'picker.option': '{name} — {state}',
  'picker.optionWithCapacity': '{name} · {capacity} — {state}',
  'picker.free': 'frei',
  'picker.busy': 'belegt',
  'picker.allAdded': 'Alle Ressourcen sind hinzugefügt.',
  'picker.badgeBooked': 'Gebucht',
  'picker.badgeBusy': 'Belegt',
  'picker.badgePending': 'Offen',
  'picker.badgeFree': 'Frei',

  // --- Reservas de recursos al guardar --------------------------------------
  'booking.booked': { one: '{count} Ressource gebucht', other: '{count} Ressourcen gebucht' },
  'booking.busy': { one: '{count} belegt', other: '{count} belegt' },
  'booking.saved': 'Termin gespeichert · {booked}',
  'booking.savedBusy': 'Termin gespeichert · {booked}, {busy}',

  // --- Errores de carga -----------------------------------------------------
  'error.calendarsLoad': 'Die Kalender konnten nicht geladen werden.',
  'error.eventsLoad': 'Die Termine konnten nicht geladen werden.',

  // --- Avisos (toasts) ------------------------------------------------------
  'toast.selectCalendar': 'Wähle einen Kalender',
  'toast.eventCreated': 'Termin erstellt',
  'toast.eventUpdated': 'Termin aktualisiert',
  'toast.eventSaveFailed': 'Der Termin konnte nicht gespeichert werden',
  'toast.eventDeleted': 'Termin gelöscht',
  'toast.eventDeleteFailed': 'Der Termin konnte nicht gelöscht werden',
  'toast.subscriptionRefreshed': 'Abo aktualisiert',
  'toast.subscriptionRefreshFailed': 'Das Abo konnte nicht aktualisiert werden',
  'toast.calendarCreated': 'Kalender erstellt',
  'toast.calendarUpdated': 'Kalender aktualisiert',
  'toast.calendarSaveFailed': 'Der Kalender konnte nicht gespeichert werden',
  'toast.calendarDeleted': 'Kalender gelöscht',
  'toast.calendarDeleteFailed': 'Der Kalender konnte nicht gelöscht werden',
  'toast.calendarSubscribed': 'Kalender abonniert',
  'toast.subscribeFailed': 'Abonnieren nicht möglich',
  'toast.subscribeFailedHint':
    'Prüfe die URL: Es muss eine öffentliche .ics-Datei sein, die über http(s) erreichbar ist.',
  'toast.sharesLoadFailed': 'Die Freigaben konnten nicht geladen werden',
  'toast.calendarShared': 'Kalender freigegeben',
  'toast.shareFailed': 'Der Kalender konnte nicht freigegeben werden',
  'toast.shareRevoked': 'Freigabe entzogen',
  'toast.revokeFailed': 'Entziehen nicht möglich',
  'toast.resourcesLoadFailed': 'Die Ressourcen konnten nicht geladen werden',
  'toast.resourceCreated': 'Ressource angelegt',
  'toast.resourceUpdated': 'Ressource aktualisiert',
  'toast.resourceSaveFailed': 'Die Ressource konnte nicht gespeichert werden',
  'toast.resourceRemoved': 'Ressource entfernt',
  'toast.resourceRemoveFailed': 'Die Ressource konnte nicht entfernt werden',
  'toast.invitationsLoadFailed': 'Die Einladungen konnten nicht geladen werden',
  'toast.invitationAccepted': 'Erledigt: Der Termin steht in deinem Kalender',
  'toast.invitationDeclined': 'Erledigt: Du hast abgesagt',
  'toast.invitationTentative': 'Erledigt: Du hast mit „vielleicht“ geantwortet',
  'toast.invitationFailed': 'Die Einladung konnte nicht beantwortet werden',
  'toast.reminderDismissFailed': 'Die Erinnerung konnte nicht verworfen werden',
  'nav.prev': 'Zurück',
  'nav.next': 'Weiter',
  'cal.create': 'Kalender erstellen',
  'cal.options': 'Kalenderoptionen',
  'cal.refresh': 'Jetzt aktualisieren',
  'cal.share': 'Teilen…',
  'cal.downloadIcs': '.ics herunterladen',
  'cal.sharedWithMe': 'Mit mir geteilt',
  'resources.manage': 'Räume und Ressourcen verwalten',
  'subscribe.aria': 'Kalender per URL abonnieren',
  'subscribe.byUrl': 'Per URL abonnieren (.ics)',
  'search.events': 'Termine suchen',
  'view.label': 'Kalenderansicht',
  'load.error': 'Der Kalender konnte nicht geladen werden',
  'loading.calendar': 'Kalender wird geladen',
  'page.title': 'Sytue Calendar',
  'page.newEventTitle': 'Sytue Calendar — Neuer Termin',
  'share.whatHappens': 'Was passieren wird',
  'share.canEdit': 'Diese Person kann Ihre Termine ändern',
  'share.whoSeesNow': 'Wer es jetzt sieht',
  'share.onlyYou': 'Nur Sie sehen diesen Kalender',
  'share.revokeAction': 'Entziehen',

  // --- Rótulos que estaban en duro ---
  'share.recipientEmail': 'E-Mail der Person, mit der Sie teilen',
  'cal.subscribedReadOnly': 'Abonnierter Kalender (schreibgeschützt)',

  // --- Errores y textos de permisos que estaban en duro ---
  'cal.fromEarlier': ' (von vorher)',
  'cal.noUpcoming': 'Keine bevorstehenden Termine',
  'cal.sub.delete': 'Abonnement löschen',
  'cal.sub.updated': 'Abonnement aktualisiert',
  'cal.error.loadCalendars': 'Die Kalender konnten nicht geladen werden.',
  'cal.error.loadEvents': 'Die Termine konnten nicht geladen werden.',
  'cal.error.noAccess': 'Du hast keinen Zugriff mehr auf „{name}“.',
  'cal.error.updateEvent': 'Der Termin konnte nicht aktualisiert werden',
  'cal.error.saveEvent': 'Der Termin konnte nicht gespeichert werden',
  'cal.error.deleteEvent': 'Der Termin konnte nicht gelöscht werden',
  'cal.error.updateSub': 'Das Abonnement konnte nicht aktualisiert werden',
  'share.recipient.hint':
    'Eine Person aus Ihrer Organisation. Wenn Sie die Adresse einer Liste oder Gruppe eingeben, sehen alle darin enthaltenen Personen Ihren Kalender.',
  'share.role.manage.hint':
    'Termine erstellen, verschieben und löschen sowie Einladungen in Ihrem Namen beantworten. Geben Sie dies nur der Person, die Ihren Kalender führt.',
  'share.role.write.hint':
    'Diese Person kann Termine in diesem Kalender erstellen, verschieben und löschen. Änderungen erscheinen sofort in Ihrem Kalender.',
  'nav.createEventOn': 'Termin am {date} erstellen',
  'share.empty.hint':
    'Sobald Sie ihn freigeben, sehen Sie hier, wer Zugriff hat und was jede Person tun darf.',
  'popover.respond': 'Nehmen Sie teil?',
  'popover.yes': 'Ja',
  'popover.maybe': 'Vielleicht',
  'popover.no': 'Nein',
  'popover.responded': 'Antwort gesendet.',
  'popover.respondFailed': 'Die Antwort konnte nicht gesendet werden.',
};
