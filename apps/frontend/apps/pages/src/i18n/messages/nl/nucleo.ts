/**
 * Nederlandse teksten voor de kern van Sytue Web (web).
 * Prefixen: nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Het openbare adres van de site kon niet worden opgehaald.',
  'nav.title': 'Web',
  'nav.sites': 'Mijn sites',
  'nav.new': 'Nieuwe site',
  'nav.settings': 'Instellingen',

  'sites.title': 'Mijn sites',
  'sites.empty': 'Je hebt nog geen sites gemaakt.',
  'sites.create': 'Site maken',
  'sites.status.published': 'Gepubliceerd',
  'sites.status.draft': 'Concept',
  'sites.restore': 'Herstellen',
  'sites.trash': 'Sites in de prullenbak',
  'sites.trash_help':
    'Verwijderde sites blijven 30 dagen bewaard voordat ze definitief worden verwijderd.',
  'sites.open_editor': 'Editor openen',
  'sites.manage': 'Beheren',
  'sites.last_updated': 'Bijgewerkt op {fecha}',
  'sites.filter_all': 'Alle',
  'sites.filter_published': 'Gepubliceerd',
  'sites.filter_draft': 'Concepten',
  'sites.count': { one: '{count} site', other: '{count} sites' },
  'sites.empty_help':
    'Kies een sjabloon of begin met een lege pagina. Je kunt publiceren wanneer je wilt.',
  'sites.filter_label': 'Filteren op status',
  'sites.filter_empty': 'Geen enkele site past bij dit filter.',
  'sites.loading': 'Sites laden',
  'sites.load_error': 'De sites konden niet worden geladen',
  'sites.load_error_help': 'Controleer je verbinding en probeer het opnieuw.',
  'sites.retry': 'Opnieuw proberen',
  'sites.restore_error': 'De site kon niet worden hersteld. Probeer het opnieuw.',
  'sites.days_left': { one: 'nog {count} dag', other: 'nog {count} dagen' },

  'new.title': 'Nieuwe site maken',
  'new.template_step': 'Kies een sjabloon',
  'new.details_step': 'Gegevens van de site',
  'new.name_label': 'Naam van de site',
  'new.slug_label': 'Slug',
  'new.languages_label': 'Talen van de site',
  'new.submit': 'Maken en editor openen',
  'new.step_template': 'Sjabloon',
  'new.step_info': 'Basisgegevens',
  'new.step_languages': 'Talen',
  'new.primary_language': 'Hoofdtaal',
  'new.step_review': 'Controleren',
  'new.details_help':
    'Geef je site een naam. Wij maken het adres automatisch aan; je eigen domein kun je later koppelen.',
  'new.review_heading': 'Klaar om te beginnen',
  'new.review_help': 'Controleer deze opties voordat je de site maakt.',
  'new.draft_notice':
    'Er wordt een privéconcept gemaakt. In de editor kun je de teksten, foto’s en het ontwerp aanpassen. Daarna helpen we je bij het controleren, het koppelen van je domein en het publiceren.',
  'new.template_blank': 'Leeg',
  'new.template_blank_desc': 'Begin met een lege pagina en ontwerp vrij.',
  'new.languages_notice': 'Binnenkort meer talen',
  'new.slug_available': 'Slug is beschikbaar',
  'new.slug_unavailable': 'Deze slug is niet beschikbaar',
  'new.slug_checking': 'Slug controleren…',
  'new.next': 'Volgende',
  'new.back': 'Vorige',
  'new.templates_error':
    'De sjablonen konden niet worden geladen. Je kunt met een lege pagina beginnen.',
  'new.name_placeholder': 'Mijn webwinkel',
  'new.slug_placeholder': 'mijn-winkel',
  'new.slug_help':
    'Alleen kleine letters, cijfers en koppeltekens. Hiermee wordt het adres van de site gevormd.',
  'new.slug_unverified':
    'De slug kon nu niet worden gecontroleerd; dat gebeurt bij het maken van de site.',
  'new.main_language': 'Hoofdtaal',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Tweetalige versie inschakelen ({idioma})',
  'new.create_error': 'De site kon niet worden gemaakt. Probeer het opnieuw.',
  'new.err_site_limit':
    'Je organisatie heeft al 20 sites of heeft er vandaag al 5 gemaakt. Verwijder er een of kom morgen terug.',
  'new.cancel': 'Annuleren',
  'new.progress': 'Stappen om de site te maken',
  'new.go_to_step': 'Naar stap {n}',
  'new.step_of': 'Stap {n} van {total}',
  'app_settings.title': 'Instellingen van Sytue Web',
  'new.search_title': 'Begin met je soort bedrijf',
  'new.search_help':
    'Typ wat je doet of kies een groep. Elk sjabloon heeft al de teksten voor jouw vak: je past alleen de naam, het telefoonnummer, de prijzen en de foto’s aan.',
  'new.search_label': 'Je soort bedrijf',
  'new.search_placeholder': 'Kapsalon, café, loodgieter, kledingwinkel…',
  'new.all_groups': 'Alle',
  'new.results': { one: '{count} sjabloon', other: '{count} sjablonen' },
  'new.no_results': 'Geen sjabloon gevonden voor ‘{q}’.',
  'new.no_results_help':
    'Probeer een ander woord (bijvoorbeeld ‘winkel’ of ‘kliniek’) of begin met een lege pagina.',
  'new.clear_search': 'Zoekopdracht wissen',
  'new.select': 'Dit sjabloon kiezen',
  'new.selected': 'Gekozen',
  'new.preview': 'Voorbeeld',
  'new.preview_close': 'Sluiten',
  'new.preview_loading': 'Sjabloon laden…',
  'new.preview_error': 'Het voorbeeld kon niet worden geladen.',
  'new.preview_desktop': 'Computer',
  'new.preview_mobile': 'Telefoon',
  'new.preview_pages': 'Pagina’s van het sjabloon',
  'new.preview_note':
    'Weergave bij benadering: de foto’s zijn plaatshouders die je door je eigen foto’s vervangt, en de voorbeeldgegevens pas je aan in de editor.',
  'new.badge_store': 'Met webwinkel',
  'new.badge_custom': 'Aangepast',
  'new.cat.belleza': 'Beauty en welzijn',
  'new.cat.comida': 'Eten en drinken',
  'new.cat.comercio': 'Winkels en detailhandel',
  'new.cat.oficios': 'Vakmensen en huis',
  'new.cat.motor': 'Auto en motor',
  'new.cat.profesionales': 'Bedrijven en professionals',
  'new.cat.salud': 'Gezondheid',
  'new.cat.deporte': 'Sport',
  'new.cat.educacion': 'Onderwijs en opleiding',
  'new.cat.creativos': 'Creatief en evenementen',
  'new.cat.turismo': 'Toerisme en verblijf',
  'new.cat.local': 'Lokale producten',
  'new.cat.comunidad': 'Verenigingen en gemeenschap',
  'new.cat.web': 'Eenvoudige pagina’s',
  'new.cat.otras': 'Overig',

  'publish.title': 'Site publiceren',
  'publish.action': 'Publiceren',
  'publish.checklist': 'Controlelijst voor publicatie',
  'publish.preview': 'Voorbeeld',

  'versions.title': 'Versies',
  'versions.create': 'Versie maken',
  'versions.restore': 'Deze versie herstellen',
  'versions.empty': 'Nog geen versies opgeslagen.',
  'versions.name': 'Naam van de versie',
  'versions.rev': 'Revisie',
  'versions.restored': 'Versie hersteld',

  'domains.title': 'Domeinen',
  'domains.add': 'Domein toevoegen',

  'analytics.title': 'Statistieken',
  'analytics.visits': 'Bezoeken',

  'export.title': 'Exporteren',
  'export.origins_label': 'Waar je deze code host (toegestane origins)',
  'export.origins_help':
    'Vul per regel één https://-URL in van waaruit de openbare API wordt aangeroepen.',
  'export.schemes_label': 'App-schema’s (voor native apps)',
  'export.schemes_help': 'Eigen URL-schema’s (één per regel), bijvoorbeeld: mijnapp, sytue.',
  'export.save': 'Exportinstellingen opslaan',
  'export.saved': 'Exportinstellingen opgeslagen',

  'legal.title': 'Juridische gegevens',
  'legal.holder_name': 'Naam of bedrijfsnaam',
  'legal.tax_id': 'Btw-nummer / fiscaal nummer',
  'legal.country': 'Land',
  'legal.street': 'Adres (straat en huisnummer)',
  'legal.postal_code': 'Postcode',
  'legal.city': 'Plaats',
  'legal.region': 'Provincie of regio',
  'legal.email': 'E-mailadres voor juridisch contact',
  'legal.phone': 'Telefoon',
  'legal.registry': 'Registergegevens (deel, boek, blad)',
  'legal.privacy_contact': 'Contact voor privacy (FG)',
  'legal.save': 'Juridische gegevens opslaan',
  'legal.saved': 'Juridische gegevens opgeslagen',

  'settings.title': 'Instellingen van de site',
  'settings.name': 'Naam van de site',
  'settings.slug': 'Slug',
  'settings.analytics': 'Ingebouwde statistieken inschakelen',
  'settings.analytics_help':
    'Telt paginaweergaven met respect voor privacy, zonder cookies van derden.',
  'settings.save': 'Instellingen opslaan',
  'settings.saved': 'Instellingen opgeslagen',
  'settings.danger_zone': 'Gevarenzone',
  'settings.delete_title': 'Deze site verwijderen',
  'settings.delete_desc':
    'De site gaat 30 dagen naar de prullenbak; in die tijd kun je hem nog herstellen.',
  'settings.delete_confirm_prompt': 'Typ de naam van de site ({name}) om te bevestigen:',
  'settings.delete_button': 'Site verwijderen',

  'errors.not_found': 'Site niet gevonden',
  'errors.unauthorized': 'Je hebt geen toestemming om deze site te bekijken',

  // Sitegegevens (islands/SitioDetalleApp.tsx, formularios/Bandeja.tsx, tienda/Pedidos.tsx)
  'ficha.page_title': 'Sitegegevens',
  'ficha.back': 'Terug naar mijn sites',
  'ficha.sections': 'Onderdelen van de site',
  'ficha.loading': 'Site laden',
  'ficha.load_error': 'De site kon niet worden geladen',
  'ficha.open_site': 'Site openen',
  'ficha.not_found_help': 'Misschien is de site verwijderd of heb je er geen toegang toe.',

  'publish.tab': 'Publiceren',
  'publish.intro':
    'Loop de lijst na en publiceer als alles klaar is. Een voorbeeld verandert niets aan wat het publiek ziet.',
  'publish.status': 'Huidige status',
  'publish.check_ok': 'Klaar',
  'publish.check_missing': 'Ontbreekt',
  'publish.check_legal': 'Juridische gegevens volledig',
  'publish.check_legal_missing': 'Ontbreekt: {campos}',
  'publish.check_legal_fix': 'Aanvullen',
  'publish.check_demo': 'Geen voorbeeldteksten van het sjabloon',
  'publish.check_ir': 'Pagina’s correct opgebouwd',
  'publish.check_server':
    'Bij het publiceren wordt ook gecontroleerd dat er geen voorbeeldteksten van het sjabloon over zijn en dat de pagina’s correct zijn opgebouwd.',
  'publish.done_preview': 'Voorbeeld gemaakt.',
  'publish.done_live': 'Site gepubliceerd.',
  'publish.open_preview': 'Voorbeeld openen',
  'publish.err_legal':
    'Er ontbreken juridische gegevens. Vul ze aan op het tabblad ‘Juridische gegevens’.',
  'publish.err_demo':
    'De site bevat nog voorbeeldteksten van het sjabloon. Vervang ze in de editor en publiceer opnieuw.',
  'publish.err_ir':
    'Een pagina is niet correct opgebouwd. Open die pagina in de editor om te zien wat er moet worden verbeterd.',
  'publish.err_previews':
    'Er zijn al 5 actieve voorbeelden. Wacht tot er een verloopt voordat je een nieuw voorbeeld maakt.',
  'publish.err_too_large':
    'De site is groter dan wat maximaal kan worden gepubliceerd (64 MiB of 5000 bestanden). Verklein de afbeeldingen of bestanden.',
  'publish.err_forbidden': 'Alleen wie de organisatie beheert, kan de site publiceren.',
  'publish.err_conflict':
    'De site kon niet worden gepubliceerd omdat hij is gewijzigd of is opgeschort. Laad de pagina opnieuw en probeer het nog eens.',
  'publish.err_invalid':
    'De publicatie is ongeldig. Controleer de inhoud van de site in de editor.',
  'publish.err_generic': 'De site kon niet worden gepubliceerd. Probeer het opnieuw.',

  'versions.intro':
    'Een versie is een kopie van het concept waarnaar je kunt terugkeren. Maak er een voor een grote wijziging.',
  'versions.name_placeholder': 'Voor het aanpassen van de homepage',
  'versions.manual_name': 'Handmatige versie',
  'versions.unnamed': 'Naamloos',
  'versions.loading': 'Versies laden',
  'versions.load_error': 'De versies konden niet worden geladen',
  'versions.empty_help': 'Wanneer je een versie maakt of de site publiceert, verschijnt die hier.',
  'versions.created': 'Versie gemaakt',
  'versions.create_error': 'De versie kon niet worden gemaakt. Probeer het opnieuw.',
  'versions.restore_error': 'De versie kon niet worden hersteld. Probeer het opnieuw.',
  'versions.restore_confirm':
    'Het concept gaat terug naar de staat van ‘{nombre}’. De huidige staat wordt eerst opgeslagen als ‘Voor het herstellen’, zodat je ernaar kunt terugkeren.',
  'versions.kind.manual': 'Handmatig',
  'versions.kind.publish': 'Publicatie',
  'versions.kind.preview': 'Voorbeeld',
  'versions.kind.export': 'Export',
  'versions.kind.before_restore': 'Voor het herstellen',

  'domains.intro':
    'Koppel een eigen domein. Maak na het toevoegen bij je DNS-provider de vermelde records aan en controleer de instellingen.',
  'domains.host_label': 'Domein',
  'domains.host_placeholder': 'www.voorbeeld.nl',
  'domains.host_help': 'Alleen de domeinnaam, zonder https:// of schuine strepen.',
  'domains.empty': 'Er zijn nog geen eigen domeinen ingesteld.',
  'domains.loading': 'Domeinen laden',
  'domains.load_error': 'De domeinen konden niet worden geladen',
  'domains.primary': 'Primair',
  'domains.status.pending': 'Wacht op verificatie',
  'domains.status.verified': 'Geverifieerd',
  'domains.status.active': 'Actief',
  'domains.status.failed': 'Mislukt',
  'domains.dns_title': 'DNS-records',
  'domains.dns_txt': 'TXT (verificatie)',
  'domains.dns_cname': 'CNAME (subdomeinen zoals www)',
  'domains.dns_a': 'A (hoofddomein)',
  'domains.check': 'DNS en HTTPS controleren',
  'domains.make_primary': 'Primair maken',
  'domains.remove': 'Verwijderen',
  'domains.remove_title': 'Domein verwijderen',
  'domains.remove_confirm':
    '‘{host}’ verwijst dan niet meer naar deze site. Om het weer te gebruiken, moet je het opnieuw toevoegen.',
  'domains.added': 'Domein toegevoegd. Maak bij je DNS-provider de records hieronder aan.',
  'domains.updated': 'Domeinen bijgewerkt.',
  'domains.action_error': 'De actie kon niet worden voltooid. Probeer het opnieuw.',
  'domains.err_invalid': 'De domeinnaam is ongeldig.',
  'domains.err_reserved': 'Dat domein is gereserveerd en kan niet worden gebruikt.',
  'domains.err_taken': 'Dat domein wordt al door een andere site gebruikt.',
  'domains.err_limit': 'Je hebt het maximale aantal domeinen voor deze site bereikt.',
  'domains.err_suspended': 'De site is opgeschort en accepteert geen domeinwijzigingen.',
  'domains.err_forbidden': 'Alleen wie de organisatie beheert, kan domeinen beheren.',
  'domains.err_generic':
    'De bewerking op het domein kon niet worden voltooid. Probeer het opnieuw.',
  'domains.start_title': 'Koppel je domein, stap voor stap',
  'domains.start_help':
    'Vul het domein in dat je wilt gebruiken. Je hebt toegang nodig tot de DNS-instellingen bij de provider waar je het hebt gekocht.',
  'domains.progress': 'Voortgang van de domeinkoppeling',
  'domains.step.registered': 'Toegevoegd aan Sytue Web',
  'domains.step.dns': 'DNS geverifieerd',
  'domains.step.https': 'HTTPS klaar',
  'domains.step.primary': 'Primair adres',
  'domains.step.done': 'Voltooid',
  'domains.step.pending': 'In afwachting',
  'domains.status.review': 'Vraagt aandacht',
  'domains.record_name': 'Naam van het record',
  'domains.record_value': 'Waarde van het record',
  'domains.routing_title': 'Adres van de site',
  'domains.dns_help':
    'Open de DNS-instellingen bij je provider en voeg deze twee records toe. Kopieer elke naam en waarde precies zoals ze hier staan.',
  'domains.dns_names_help':
    'Sommige providers gebruiken @ voor het hoofddomein of alleen www voor dat subdomein. De volledige recordnaam staat hierboven.',
  'domains.routing_legacy_help':
    'Gebruik A voor het hoofddomein of CNAME voor een subdomein zoals www, met de bijbehorende bestemming die de dienst opgeeft.',
  'domains.dns_propagation':
    'Kies na het opslaan van de records ‘DNS en HTTPS controleren’. Zie je ze nog niet, wacht dan tot je provider de wijziging heeft doorgevoerd en controleer opnieuw. Laat de records voor e-mail en andere diensten staan.',
  'domains.records_unavailable':
    'De dienst heeft nog niet alle gegevens voor dit record geleverd. Wijzig je DNS pas als ze beschikbaar zijn.',
  'domains.copy': 'Kopiëren',
  'domains.copied': 'Gekopieerd',
  'domains.copy_field': '{campo} kopiëren',
  'domains.copy_error':
    'Kopiëren naar het klembord is niet gelukt. Selecteer de waarde van het veld en kopieer die handmatig.',
  'domains.last_check': 'Laatste controle: {fecha}',
  'domains.publish_help':
    'Zodra je primaire adres klaar is, publiceer je de site opnieuw zodat links en doorverwijzingen je domein gebruiken.',
  'domains.publish_action': 'Naar publiceren',
  'domains.detail.pending':
    'Het domein is toegevoegd. Stel de DNS-records in en controleer de verbinding om verder te gaan.',
  'domains.detail.txt_missing':
    'Het TXT-record is nog niet gevonden. Voeg het toe bij je DNS-provider en controleer opnieuw zodra de wijziging is doorgevoerd.',
  'domains.detail.txt_mismatch':
    'De TXT-waarde komt niet overeen. Vervang ze door de exacte waarde op dit scherm.',
  'domains.detail.dns_unreachable':
    'De DNS van dit domein kon niet worden opgevraagd. Controleer het domein en de nameservers en probeer het opnieuw.',
  'domains.detail.routing':
    'Het domein verwijst nog niet naar de site. Controleer het vermelde A- of CNAME-record en probeer het opnieuw.',
  'domains.detail.certificate':
    'De DNS-records zijn geverifieerd. Het HTTPS-certificaat is nog niet klaar; controleer over een paar minuten opnieuw.',
  'domains.detail.active': 'DNS en de HTTPS-verbinding zijn geverifieerd. Het domein is klaar.',
  'domains.detail.unknown':
    'De laatste controle vraagt aandacht. Controleer de records en verifieer de verbinding opnieuw.',

  'analytics.intro':
    'Tijdens de pilot zijn de bezoeken een dagelijkse schatting; ze staan niet voor unieke personen in de periode.',
  'analytics.visitors': 'Geschatte bezoeken',
  'analytics.loading': 'Statistieken laden',
  'analytics.load_error': 'De statistieken konden niet worden geladen',
  'analytics.unavailable':
    'De statistiekendienst is nu niet beschikbaar. Probeer het over een paar minuten opnieuw.',
  'analytics.disabled': 'Statistieken staan uit',
  'analytics.disabled_help': 'Zet ze aan in de instellingen van de site om bezoeken te tellen.',
  'analytics.go_settings': 'Naar de instellingen',
  'analytics.refresh': 'Vernieuwen',

  'export.intro':
    'Host je de code van de site buiten Sytue, geef dan aan van waaruit de openbare API wordt aangeroepen.',
  'export.origins_placeholder': 'https://mijnwinkel.voorbeeld.nl',
  'export.schemes_placeholder': 'mijnapp',
  'export.origins_invalid':
    'Elke regel moet een geldig https://-adres zijn (regel {n} is dat niet).',
  'export.schemes_invalid':
    'Elke regel moet een geldig schema zijn: het begint met een letter en bevat alleen letters, cijfers, ‘+’, ‘-’ of ‘.’ (regel {n} voldoet niet).',
  'export.err_generic': 'De instellingen konden niet worden opgeslagen. Probeer het opnieuw.',

  'legal.intro': 'Deze zijn verplicht om de site te publiceren.',
  'legal.required_note': 'Velden met een * zijn verplicht.',
  'legal.saved_incomplete': 'Gegevens opgeslagen. Nog nodig om te publiceren: {campos}.',
  'legal.country_help': 'Code van twee letters, bijvoorbeeld NL.',
  'legal.registry_help':
    'Verplicht voor Spaanse vennootschappen (fiscaal nummer dat begint met A, B, C, D of F).',
  'legal.ph_holder': 'Voorbeeld B.V.',
  'legal.ph_street': 'Hoofdstraat 12, 1e verdieping',
  'legal.ph_city': 'Amsterdam',
  'legal.ph_email': 'juridisch@voorbeeld.nl',
  'legal.ph_privacy': 'fg@voorbeeld.nl',
  'legal.ph_registry': 'Handelsregister Madrid, deel 1234, folio 56, blad M-789',
  'legal.err_generic': 'De juridische gegevens konden niet worden opgeslagen. Probeer het opnieuw.',

  'settings.tab': 'Instellingen',
  'settings.intro': 'Naam, adres en algemene opties van de site.',
  'settings.slug_help':
    'Alleen kleine letters, cijfers en koppeltekens. Als je dit wijzigt, verandert het adres van de site.',
  'settings.admin_only': 'Alleen wie de organisatie beheert, kan dit wijzigen.',
  'settings.name_required': 'De naam van de site mag niet leeg zijn.',
  'settings.err_slug_taken': 'Een andere site gebruikt die slug al.',
  'settings.err_invalid': 'Controleer de naam, de slug en het adres: een ervan is ongeldig.',
  'settings.err_generic': 'De instellingen konden niet worden opgeslagen. Probeer het opnieuw.',
  'settings.delete_error': 'De site kon niet worden verwijderd. Probeer het opnieuw.',

  'forms.empty': 'Nog geen reacties',
  'forms.empty_help': 'Als iemand een formulier op je website verstuurt, zie je dat hier.',
};
