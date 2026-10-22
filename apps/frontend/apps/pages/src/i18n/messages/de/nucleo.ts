/**
 * Deutsche Texte für den Kern von Sytue Web (web).
 * Präfixe: nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Die öffentliche Adresse der Website konnte nicht abgerufen werden.',
  'nav.title': 'Web',
  'nav.sites': 'Meine Websites',
  'nav.new': 'Neue Website',
  'nav.settings': 'Einstellungen',

  'sites.title': 'Meine Websites',
  'sites.empty': 'Sie haben noch keine Website erstellt.',
  'sites.create': 'Website erstellen',
  'sites.status.published': 'Veröffentlicht',
  'sites.status.draft': 'Entwurf',
  'sites.restore': 'Wiederherstellen',
  'sites.trash': 'Websites im Papierkorb',
  'sites.trash_help':
    'Gelöschte Websites werden 30 Tage aufbewahrt, bevor sie endgültig entfernt werden.',
  'sites.open_editor': 'Editor öffnen',
  'sites.manage': 'Verwalten',
  'sites.last_updated': 'Aktualisiert am {fecha}',
  'sites.filter_all': 'Alle',
  'sites.filter_published': 'Veröffentlicht',
  'sites.filter_draft': 'Entwürfe',
  'sites.count': { one: '{count} Website', other: '{count} Websites' },
  'sites.empty_help':
    'Wählen Sie eine Vorlage oder beginnen Sie mit einer leeren Seite. Veröffentlichen können Sie, wann immer Sie möchten.',
  'sites.filter_label': 'Nach Status filtern',
  'sites.filter_empty': 'Keine Website entspricht diesem Filter.',
  'sites.loading': 'Websites werden geladen',
  'sites.load_error': 'Die Websites konnten nicht geladen werden',
  'sites.load_error_help': 'Prüfen Sie Ihre Verbindung und versuchen Sie es erneut.',
  'sites.retry': 'Erneut versuchen',
  'sites.restore_error':
    'Die Website konnte nicht wiederhergestellt werden. Versuchen Sie es erneut.',
  'sites.days_left': { one: 'noch {count} Tag', other: 'noch {count} Tage' },

  'new.title': 'Neue Website erstellen',
  'new.template_step': 'Vorlage auswählen',
  'new.details_step': 'Angaben zur Website',
  'new.name_label': 'Name der Website',
  'new.slug_label': 'Kurzname (Slug)',
  'new.languages_label': 'Sprachen der Website',
  'new.submit': 'Erstellen und Editor öffnen',
  'new.step_template': 'Vorlage',
  'new.step_info': 'Grunddaten',
  'new.step_languages': 'Sprachen',
  'new.primary_language': 'Hauptsprache',
  'new.step_review': 'Prüfen',
  'new.details_help':
    'Geben Sie Ihrer Website einen Namen. Die Adresse bereiten wir automatisch vor; Ihre eigene Domain können Sie später verbinden.',
  'new.review_heading': 'Bereit zum Start',
  'new.review_help': 'Prüfen Sie diese Angaben, bevor Sie die Website erstellen.',
  'new.draft_notice':
    'Dabei entsteht ein privater Entwurf. Im Editor können Sie Texte, Fotos und Design ändern. Danach führen wir Sie durch Prüfung, Domain-Verbindung und Veröffentlichung.',
  'new.template_blank': 'Leer',
  'new.template_blank_desc': 'Mit einer leeren Seite beginnen und frei gestalten.',
  'new.languages_notice': 'Weitere Sprachen folgen in Kürze',
  'new.slug_available': 'Der Kurzname ist verfügbar',
  'new.slug_unavailable': 'Dieser Kurzname ist nicht verfügbar',
  'new.slug_checking': 'Kurzname wird geprüft …',
  'new.next': 'Weiter',
  'new.back': 'Zurück',
  'new.templates_error':
    'Die Vorlagen konnten nicht geladen werden. Sie können mit einer leeren Seite beginnen.',
  'new.name_placeholder': 'Mein Onlineshop',
  'new.slug_placeholder': 'mein-shop',
  'new.slug_help':
    'Nur Kleinbuchstaben, Ziffern und Bindestriche. Daraus wird die Adresse der Website gebildet.',
  'new.slug_unverified':
    'Der Kurzname konnte gerade nicht geprüft werden; er wird beim Erstellen der Website kontrolliert.',
  'new.main_language': 'Hauptsprache',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Zweisprachige Version aktivieren ({idioma})',
  'new.create_error': 'Die Website konnte nicht erstellt werden. Versuchen Sie es erneut.',
  'new.err_site_limit':
    'Ihre Organisation hat bereits 20 Websites oder heute schon 5 erstellt. Löschen Sie eine oder versuchen Sie es morgen wieder.',
  'new.cancel': 'Abbrechen',
  'new.progress': 'Schritte zum Erstellen der Website',
  'new.go_to_step': 'Zu Schritt {n}',
  'new.step_of': 'Schritt {n} von {total}',
  'app_settings.title': 'Einstellungen von Sytue Web',
  'new.search_title': 'Beginnen Sie mit Ihrer Branche',
  'new.search_help':
    'Geben Sie ein, was Sie tun, oder wählen Sie eine Gruppe. Jede Vorlage enthält bereits die passenden Texte für Ihre Branche: Sie ändern nur Namen, Telefonnummer, Preise und Fotos.',
  'new.search_label': 'Ihre Branche',
  'new.search_placeholder': 'Friseursalon, Bar, Installateurbetrieb, Modegeschäft …',
  'new.all_groups': 'Alle',
  'new.results': { one: '{count} Vorlage', other: '{count} Vorlagen' },
  'new.no_results': 'Keine Vorlage passt zu „{q}“.',
  'new.no_results_help':
    'Versuchen Sie ein anderes Wort (zum Beispiel „Geschäft“ oder „Praxis“) oder beginnen Sie mit einer leeren Seite.',
  'new.clear_search': 'Suche zurücksetzen',
  'new.select': 'Diese Vorlage wählen',
  'new.selected': 'Ausgewählt',
  'new.preview': 'Vorschau',
  'new.preview_close': 'Schließen',
  'new.preview_loading': 'Vorlage wird geladen …',
  'new.preview_error': 'Die Vorschau konnte nicht geladen werden.',
  'new.preview_desktop': 'Computer',
  'new.preview_mobile': 'Smartphone',
  'new.preview_pages': 'Seiten der Vorlage',
  'new.preview_note':
    'Ungefähre Ansicht: Die Fotos sind Platzhalter, die Sie durch eigene ersetzen, und die Beispielangaben ändern Sie im Editor.',
  'new.badge_store': 'Mit Shop',
  'new.badge_custom': 'Eigene',
  'new.cat.belleza': 'Schönheit und Wellness',
  'new.cat.comida': 'Essen und Trinken',
  'new.cat.comercio': 'Geschäfte und Handel',
  'new.cat.oficios': 'Handwerk und Haus',
  'new.cat.motor': 'Auto und Kfz',
  'new.cat.profesionales': 'Unternehmen und Freiberufler',
  'new.cat.salud': 'Gesundheit',
  'new.cat.deporte': 'Sport',
  'new.cat.educacion': 'Bildung und Weiterbildung',
  'new.cat.creativos': 'Kreative und Events',
  'new.cat.turismo': 'Tourismus und Unterkünfte',
  'new.cat.local': 'Regionale Produkte',
  'new.cat.comunidad': 'Vereine und Gemeinschaft',
  'new.cat.web': 'Einfache Seiten',
  'new.cat.otras': 'Sonstige',

  'publish.title': 'Website veröffentlichen',
  'publish.action': 'Veröffentlichen',
  'publish.checklist': 'Checkliste vor dem Veröffentlichen',
  'publish.preview': 'Vorschau',

  'versions.title': 'Versionen',
  'versions.create': 'Version erstellen',
  'versions.restore': 'Diese Version wiederherstellen',
  'versions.empty': 'Noch keine Versionen gespeichert.',
  'versions.name': 'Name der Version',
  'versions.rev': 'Revision',
  'versions.restored': 'Version wiederhergestellt',

  'domains.title': 'Domains',
  'domains.add': 'Domain hinzufügen',

  'analytics.title': 'Statistik',
  'analytics.visits': 'Besuche',

  'export.title': 'Export',
  'export.origins_label': 'Erlaubte Ursprünge (wo der Code gehostet wird)',
  'export.origins_help':
    'Geben Sie pro Zeile eine https://-Adresse an, von der aus die öffentliche API aufgerufen wird.',
  'export.schemes_label': 'App-Schemata (für native Apps)',
  'export.schemes_help': 'Eigene URL-Schemata (eines pro Zeile), zum Beispiel: meineapp, sytue.',
  'export.save': 'Exporteinstellungen speichern',
  'export.saved': 'Exporteinstellungen gespeichert',

  'legal.title': 'Rechtliche Angaben',
  'legal.holder_name': 'Name des Unternehmens oder der verantwortlichen Person',
  'legal.tax_id': 'Steuernummer / USt-IdNr.',
  'legal.country': 'Land',
  'legal.street': 'Straße und Hausnummer',
  'legal.postal_code': 'Postleitzahl',
  'legal.city': 'Ort',
  'legal.region': 'Region / Bundesland',
  'legal.email': 'E-Mail für rechtliche Anfragen',
  'legal.phone': 'Telefon',
  'legal.registry': 'Registerangaben (Buch, Band, Blatt)',
  'legal.privacy_contact': 'Kontakt für Datenschutz (DSB)',
  'legal.save': 'Rechtliche Angaben speichern',
  'legal.saved': 'Rechtliche Angaben gespeichert',

  'settings.title': 'Website-Einstellungen',
  'settings.name': 'Name der Website',
  'settings.slug': 'Kurzname (Slug)',
  'settings.analytics': 'Integrierte Statistik aktivieren',
  'settings.analytics_help':
    'Zählt Seitenaufrufe datenschutzfreundlich und ohne Cookies von Drittanbietern.',
  'settings.save': 'Einstellungen speichern',
  'settings.saved': 'Einstellungen gespeichert',
  'settings.danger_zone': 'Gefahrenbereich',
  'settings.delete_title': 'Diese Website löschen',
  'settings.delete_desc':
    'Die Website wird für 30 Tage in den Papierkorb verschoben und kann in dieser Zeit wiederhergestellt werden.',
  'settings.delete_confirm_prompt': 'Geben Sie zur Bestätigung den Namen der Website ({name}) ein:',
  'settings.delete_button': 'Website löschen',

  'errors.not_found': 'Website nicht gefunden',
  'errors.unauthorized': 'Sie haben keine Berechtigung, diese Website anzusehen',

  // Website-Übersicht (islands/SitioDetalleApp.tsx, formularios/Bandeja.tsx, tienda/Pedidos.tsx)
  'ficha.page_title': 'Website-Details',
  'ficha.back': 'Zurück zu meinen Websites',
  'ficha.sections': 'Bereiche der Website',
  'ficha.loading': 'Website wird geladen',
  'ficha.load_error': 'Die Website konnte nicht geladen werden',
  'ficha.open_site': 'Website öffnen',
  'ficha.not_found_help':
    'Sie wurde möglicherweise gelöscht, oder Sie haben keinen Zugriff darauf.',

  'publish.tab': 'Veröffentlichen',
  'publish.intro':
    'Gehen Sie die Checkliste durch und veröffentlichen Sie, wenn alles bereit ist. Eine Vorschau ändert nichts an dem, was die Öffentlichkeit sieht.',
  'publish.status': 'Aktueller Status',
  'publish.check_ok': 'Bereit',
  'publish.check_missing': 'Fehlt',
  'publish.check_legal': 'Rechtliche Angaben vollständig',
  'publish.check_legal_missing': 'Es fehlt: {campos}',
  'publish.check_legal_fix': 'Ergänzen',
  'publish.check_demo': 'Kein Beispieltext der Vorlage mehr',
  'publish.check_ir': 'Die Seiten sind korrekt aufgebaut',
  'publish.check_server':
    'Beim Veröffentlichen wird außerdem geprüft, dass kein Beispieltext der Vorlage übrig ist und die Seiten korrekt aufgebaut sind.',
  'publish.done_preview': 'Vorschau erstellt.',
  'publish.done_live': 'Website veröffentlicht.',
  'publish.open_preview': 'Vorschau öffnen',
  'publish.err_legal':
    'Es fehlen rechtliche Angaben. Ergänzen Sie sie im Tab „Rechtliche Angaben“.',
  'publish.err_demo':
    'Die Website enthält noch Beispieltext der Vorlage. Ersetzen Sie ihn im Editor und veröffentlichen Sie erneut.',
  'publish.err_ir':
    'Eine Seite ist nicht korrekt aufgebaut. Öffnen Sie sie im Editor, um zu sehen, was zu korrigieren ist.',
  'publish.err_previews':
    'Es gibt bereits 5 aktive Vorschauen. Warten Sie, bis eine abläuft, bevor Sie eine weitere erstellen.',
  'publish.err_too_large':
    'Die Website ist größer als das Maximum, das veröffentlicht werden kann (64 MiB oder 5.000 Dateien). Verkleinern Sie Bilder oder Dateien.',
  'publish.err_forbidden': 'Die Website kann nur mit Administrationsrechten veröffentlicht werden.',
  'publish.err_conflict':
    'Die Website konnte nicht veröffentlicht werden, weil sie sich geändert hat oder gesperrt ist. Laden Sie die Seite neu und versuchen Sie es erneut.',
  'publish.err_invalid':
    'Die Veröffentlichung ist ungültig. Prüfen Sie den Inhalt der Website im Editor.',
  'publish.err_generic': 'Die Website konnte nicht veröffentlicht werden. Versuchen Sie es erneut.',

  'versions.intro':
    'Eine Version ist eine Kopie des Entwurfs, zu der Sie zurückkehren können. Erstellen Sie eine vor größeren Änderungen.',
  'versions.name_placeholder': 'Vor der Änderung der Startseite',
  'versions.manual_name': 'Manuelle Version',
  'versions.unnamed': 'Ohne Namen',
  'versions.loading': 'Versionen werden geladen',
  'versions.load_error': 'Die Versionen konnten nicht geladen werden',
  'versions.empty_help':
    'Wenn Sie eine Version erstellen oder die Website veröffentlichen, erscheint sie hier.',
  'versions.created': 'Version erstellt',
  'versions.create_error': 'Die Version konnte nicht erstellt werden. Versuchen Sie es erneut.',
  'versions.restore_error':
    'Die Version konnte nicht wiederhergestellt werden. Versuchen Sie es erneut.',
  'versions.restore_confirm':
    'Der Entwurf wird auf den Stand von „{nombre}“ zurückgesetzt. Der aktuelle Stand wird vorher als „Vor der Wiederherstellung“ gesichert, sodass Sie zu ihm zurückkehren können.',
  'versions.kind.manual': 'Manuell',
  'versions.kind.publish': 'Veröffentlichung',
  'versions.kind.preview': 'Vorschau',
  'versions.kind.export': 'Export',
  'versions.kind.before_restore': 'Vor der Wiederherstellung',

  'domains.intro':
    'Verbinden Sie Ihre eigene Domain. Legen Sie danach die angezeigten Einträge bei Ihrem DNS-Anbieter an und prüfen Sie die Einrichtung.',
  'domains.host_label': 'Domain',
  'domains.host_placeholder': 'www.beispiel.de',
  'domains.host_help': 'Nur der Domainname, ohne https:// und ohne Schrägstriche.',
  'domains.empty': 'Es sind noch keine eigenen Domains eingerichtet.',
  'domains.loading': 'Domains werden geladen',
  'domains.load_error': 'Die Domains konnten nicht geladen werden',
  'domains.primary': 'Hauptdomain',
  'domains.status.pending': 'Bestätigung ausstehend',
  'domains.status.verified': 'Bestätigt',
  'domains.status.active': 'Aktiv',
  'domains.status.failed': 'Fehlgeschlagen',
  'domains.dns_title': 'DNS-Einträge',
  'domains.dns_txt': 'TXT (Bestätigung)',
  'domains.dns_cname': 'CNAME (Subdomains wie www)',
  'domains.dns_a': 'A (Hauptdomain)',
  'domains.check': 'DNS und HTTPS prüfen',
  'domains.make_primary': 'Als Hauptdomain festlegen',
  'domains.remove': 'Entfernen',
  'domains.remove_title': 'Domain entfernen',
  'domains.remove_confirm':
    '„{host}“ verweist dann nicht mehr auf diese Website. Um sie wieder zu nutzen, müssen Sie sie erneut hinzufügen.',
  'domains.added':
    'Domain hinzugefügt. Legen Sie die unten angezeigten Einträge bei Ihrem DNS-Anbieter an.',
  'domains.updated': 'Domains aktualisiert.',
  'domains.action_error': 'Die Aktion konnte nicht ausgeführt werden. Versuchen Sie es erneut.',
  'domains.err_invalid': 'Der Domainname ist ungültig.',
  'domains.err_reserved': 'Diese Domain ist reserviert und kann nicht verwendet werden.',
  'domains.err_taken': 'Diese Domain wird bereits von einer anderen Website verwendet.',
  'domains.err_limit': 'Sie haben die maximale Anzahl an Domains für diese Website erreicht.',
  'domains.err_suspended': 'Die Website ist gesperrt und erlaubt keine Domain-Änderungen.',
  'domains.err_forbidden': 'Domains können nur mit Administrationsrechten verwaltet werden.',
  'domains.err_generic':
    'Der Vorgang an der Domain konnte nicht abgeschlossen werden. Versuchen Sie es erneut.',
  'domains.start_title': 'Ihre Domain Schritt für Schritt verbinden',
  'domains.start_help':
    'Geben Sie die Domain ein, die Sie verwenden möchten. Sie benötigen Zugriff auf die DNS-Einstellungen bei dem Anbieter, bei dem Sie sie gekauft haben.',
  'domains.progress': 'Fortschritt der Domain-Verbindung',
  'domains.step.registered': 'Zu Sytue Web hinzugefügt',
  'domains.step.dns': 'DNS bestätigt',
  'domains.step.https': 'HTTPS bereit',
  'domains.step.primary': 'Hauptadresse',
  'domains.step.done': 'Abgeschlossen',
  'domains.step.pending': 'Ausstehend',
  'domains.status.review': 'Prüfung nötig',
  'domains.record_name': 'Name des Eintrags',
  'domains.record_value': 'Wert des Eintrags',
  'domains.routing_title': 'Adresse der Website',
  'domains.dns_help':
    'Öffnen Sie die DNS-Einstellungen bei Ihrem Anbieter und fügen Sie diese beiden Einträge hinzu. Übernehmen Sie Name und Wert jeweils genau wie angezeigt.',
  'domains.dns_names_help':
    'Manche Anbieter verwenden @ für die Hauptdomain oder nur www für diese Subdomain. Der vollständige Name des Eintrags steht oben.',
  'domains.routing_legacy_help':
    'Verwenden Sie A für die Hauptdomain oder CNAME für eine Subdomain wie www, jeweils mit dem Ziel, das der Dienst angibt.',
  'domains.dns_propagation':
    'Wählen Sie nach dem Speichern der Einträge „DNS und HTTPS prüfen“. Wenn sie noch nicht erscheinen, warten Sie, bis Ihr Anbieter die Änderung verteilt hat, und prüfen Sie erneut. Behalten Sie die Einträge für E-Mail und andere Dienste bei.',
  'domains.records_unavailable':
    'Der Dienst hat noch nicht alle Angaben für diesen Eintrag geliefert. Warten Sie darauf, bevor Sie Ihr DNS ändern.',
  'domains.copy': 'Kopieren',
  'domains.copied': 'Kopiert',
  'domains.copy_field': '{campo} kopieren',
  'domains.copy_error':
    'Der Wert konnte nicht in die Zwischenablage kopiert werden. Markieren Sie den Wert im Feld und kopieren Sie ihn von Hand.',
  'domains.last_check': 'Letzte Prüfung: {fecha}',
  'domains.publish_help':
    'Sobald Ihre Hauptadresse bereit ist, veröffentlichen Sie die Website erneut, damit Links und Weiterleitungen Ihre Domain verwenden.',
  'domains.publish_action': 'Zum Veröffentlichen',
  'domains.detail.pending':
    'Ihre Domain ist hinzugefügt. Richten Sie die DNS-Einträge ein und prüfen Sie die Verbindung, um fortzufahren.',
  'domains.detail.txt_missing':
    'Der TXT-Eintrag wurde noch nicht gefunden. Legen Sie ihn bei Ihrem DNS-Anbieter an und prüfen Sie erneut, sobald die Änderung verteilt ist.',
  'domains.detail.txt_mismatch':
    'Der TXT-Wert stimmt nicht überein. Ersetzen Sie ihn durch genau den Wert, der hier angezeigt wird.',
  'domains.detail.dns_unreachable':
    'Das DNS der Domain konnte nicht abgefragt werden. Prüfen Sie die Domain und ihre Nameserver und versuchen Sie es dann erneut.',
  'domains.detail.routing':
    'Die Domain verweist noch nicht auf die Website. Prüfen Sie den angezeigten A- oder CNAME-Eintrag und versuchen Sie es erneut.',
  'domains.detail.certificate':
    'Ihre DNS-Einträge sind bestätigt. Das HTTPS-Zertifikat ist noch nicht bereit; prüfen Sie in ein paar Minuten erneut.',
  'domains.detail.active': 'DNS und HTTPS-Verbindung sind bestätigt. Ihre Domain ist bereit.',
  'domains.detail.unknown':
    'Die letzte Prüfung erfordert Ihre Aufmerksamkeit. Kontrollieren Sie die Einträge und prüfen Sie die Verbindung erneut.',

  'analytics.intro':
    'Die Besucherzahlen sind während der Pilotphase eine tägliche Schätzung; sie entsprechen nicht eindeutigen Personen im gesamten Zeitraum.',
  'analytics.visitors': 'Geschätzte Besuche',
  'analytics.loading': 'Statistik wird geladen',
  'analytics.load_error': 'Die Statistik konnte nicht geladen werden',
  'analytics.unavailable':
    'Der Statistikdienst ist gerade nicht verfügbar. Versuchen Sie es in ein paar Minuten erneut.',
  'analytics.disabled': 'Die Statistik ist ausgeschaltet',
  'analytics.disabled_help':
    'Schalten Sie sie in den Website-Einstellungen ein, um Besuche zu zählen.',
  'analytics.go_settings': 'Zu den Einstellungen',
  'analytics.refresh': 'Aktualisieren',

  'export.intro':
    'Wenn Sie den Code der Website außerhalb von Sytue hosten, geben Sie an, von wo aus die öffentliche API aufgerufen wird.',
  'export.origins_placeholder': 'https://meinshop.beispiel.de',
  'export.schemes_placeholder': 'meineapp',
  'export.origins_invalid':
    'Jede Zeile muss eine gültige https://-Adresse sein (Zeile {n} ist es nicht).',
  'export.schemes_invalid':
    'Jede Zeile muss ein gültiges Schema sein: Es beginnt mit einem Buchstaben und enthält nur Buchstaben, Ziffern, „+“, „-“ oder „.“ (Zeile {n} nicht).',
  'export.err_generic':
    'Die Einstellungen konnten nicht gespeichert werden. Versuchen Sie es erneut.',

  'legal.intro': 'Sie sind zum Veröffentlichen der Website erforderlich.',
  'legal.required_note': 'Mit * markierte Felder sind Pflichtfelder.',
  'legal.saved_incomplete': 'Angaben gespeichert. Zum Veröffentlichen fehlt noch: {campos}.',
  'legal.country_help': 'Zweistelliger Code, zum Beispiel DE.',
  'legal.registry_help':
    'Pflicht für spanische Unternehmen (Steuernummer beginnend mit A, B, C, D oder F).',
  'legal.ph_holder': 'Muster GmbH',
  'legal.ph_street': 'Hauptstraße 12, 1. OG',
  'legal.ph_city': 'Berlin',
  'legal.ph_email': 'recht@beispiel.de',
  'legal.ph_privacy': 'datenschutz@beispiel.de',
  'legal.ph_registry': 'Handelsregister Madrid, Band 1234, Folio 56, Blatt M-789',
  'legal.err_generic':
    'Die rechtlichen Angaben konnten nicht gespeichert werden. Versuchen Sie es erneut.',

  'settings.tab': 'Einstellungen',
  'settings.intro': 'Name, Adresse und allgemeine Optionen der Website.',
  'settings.slug_help':
    'Nur Kleinbuchstaben, Ziffern und Bindestriche. Wenn Sie ihn ändern, ändert sich die Adresse der Website.',
  'settings.admin_only': 'Das kann nur mit Administrationsrechten geändert werden.',
  'settings.name_required': 'Der Name der Website darf nicht leer sein.',
  'settings.err_slug_taken': 'Eine andere Website verwendet diesen Kurznamen bereits.',
  'settings.err_invalid': 'Prüfen Sie Name, Kurzname und Adresse: Eines davon ist ungültig.',
  'settings.err_generic':
    'Die Einstellungen konnten nicht gespeichert werden. Versuchen Sie es erneut.',
  'settings.delete_error': 'Die Website konnte nicht gelöscht werden. Versuchen Sie es erneut.',

  'forms.empty': 'Noch keine Antworten',
  'forms.empty_help': 'Wenn jemand ein Formular auf Ihrer Website absendet, sehen Sie es hier.',
};
