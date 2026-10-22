/**
 * Mesaje în română pentru zona nucleu a Sytue Web (web).
 * Prefixe: nav.*, sites.*, new.*, publish.*, versions.*, domains.*, analytics.*, export.*, legal.*, errors.*, settings.*
 */
export const nucleo = {
  'ficha.address_unavailable': 'Adresa publică a site-ului nu a putut fi obținută.',
  'nav.title': 'Web',
  'nav.sites': 'Site-urile mele',
  'nav.new': 'Site nou',
  'nav.settings': 'Setări',

  'sites.title': 'Site-urile mele',
  'sites.empty': 'Nu ați creat încă niciun site.',
  'sites.create': 'Creați un site',
  'sites.status.published': 'Publicat',
  'sites.status.draft': 'Ciornă',
  'sites.restore': 'Restaurați',
  'sites.trash': 'Site-uri în coșul de gunoi',
  'sites.trash_help':
    'Site-urile șterse sunt păstrate timp de 30 de zile înainte de ștergerea definitivă.',
  'sites.open_editor': 'Deschideți editorul',
  'sites.manage': 'Gestionați',
  'sites.last_updated': 'Actualizat pe {fecha}',
  'sites.filter_all': 'Toate',
  'sites.filter_published': 'Publicate',
  'sites.filter_draft': 'Ciorne',
  'sites.count': {
    one: '{count} site',
    few: '{count} site-uri',
    other: '{count} de site-uri',
  },
  'sites.empty_help':
    'Alegeți un șablon sau începeți de la o pagină goală. Îl veți putea publica oricând doriți.',
  'sites.filter_label': 'Filtrați după stare',
  'sites.filter_empty': 'Niciun site nu corespunde acestui filtru.',
  'sites.loading': 'Se încarcă site-urile',
  'sites.load_error': 'Site-urile nu au putut fi încărcate',
  'sites.load_error_help': 'Verificați conexiunea și încercați din nou.',
  'sites.retry': 'Încercați din nou',
  'sites.restore_error': 'Site-ul nu a putut fi restaurat. Încercați din nou.',
  'sites.days_left': {
    one: 'mai rămâne {count} zi',
    few: 'mai rămân {count} zile',
    other: 'mai rămân {count} de zile',
  },

  'new.title': 'Creați un site nou',
  'new.template_step': 'Alegeți un șablon',
  'new.details_step': 'Detaliile site-ului',
  'new.name_label': 'Numele site-ului',
  'new.slug_label': 'Identificator (slug)',
  'new.languages_label': 'Limbile site-ului',
  'new.submit': 'Creați și deschideți editorul',
  'new.step_template': 'Șablon',
  'new.step_info': 'Informații de bază',
  'new.step_languages': 'Limbi',
  'new.primary_language': 'Limba principală',
  'new.step_review': 'Verificare',
  'new.details_help':
    'Dați un nume site-ului dumneavoastră. Vom pregăti automat adresa acestuia și vă veți putea conecta propriul domeniu mai târziu.',
  'new.review_heading': 'Totul este pregătit pentru a începe',
  'new.review_help': 'Verificați aceste opțiuni înainte de a crea site-ul.',
  'new.draft_notice':
    'Se va crea o ciornă privată. În editor veți putea modifica textele, fotografiile și designul. Apoi vă vom ghida pentru verificare, conectarea domeniului și publicare.',
  'new.template_blank': 'Pagină goală',
  'new.template_blank_desc': 'Începeți cu o pagină goală și construiți designul liber.',
  'new.languages_notice': 'Mai multe limbi în curând',
  'new.slug_available': 'Identificatorul este disponibil',
  'new.slug_unavailable': 'Acest identificator nu este disponibil',
  'new.slug_checking': 'Se verifică identificatorul…',
  'new.next': 'Continuă',
  'new.back': 'Înapoi',
  'new.templates_error': 'Șabloanele nu au putut fi încărcate. Puteți începe cu o pagină goală.',
  'new.name_placeholder': 'Magazinul meu online',
  'new.slug_placeholder': 'magazinul-meu',
  'new.slug_help': 'Doar litere mici, cifre și cratime. Formează adresa site-ului.',
  'new.slug_unverified':
    'Identificatorul nu a putut fi verificat acum; va fi validat la crearea site-ului.',
  'new.main_language': 'Limba principală',
  'new.lang_es': 'Español',
  'new.lang_en': 'English',
  'new.bilingual': 'Activați versiunea bilingvă ({idioma})',
  'new.create_error': 'Site-ul nu a putut fi creat. Încercați din nou.',
  'new.err_site_limit':
    'Organizația dumneavoastră are deja 20 de site-uri sau a creat deja 5 astăzi. Ștergeți unul sau reveniți mâine.',
  'new.cancel': 'Anulați',
  'new.progress': 'Pași pentru crearea site-ului',
  'new.go_to_step': 'Mergeți la pasul {n}',
  'new.step_of': 'Pasul {n} din {total}',
  'app_settings.title': 'Setări Sytue Web',
  'new.search_title': 'Începeți de la tipul afacerii dumneavoastră',
  'new.search_help':
    'Scrieți cu ce vă ocupați sau alegeți o categorie. Fiecare șablon conține deja textele potrivite domeniului: trebuie doar să schimbați numele, telefonul, prețurile și fotografiile.',
  'new.search_label': 'Tipul afacerii dumneavoastră',
  'new.search_placeholder': 'Coafor, cafenea, instalator, magazin de haine…',
  'new.all_groups': 'Toate',
  'new.results': {
    one: '{count} șablon',
    few: '{count} șabloane',
    other: '{count} de șabloane',
  },
  'new.no_results': 'Nu există niciun șablon pentru „{q}”.',
  'new.no_results_help':
    'Încercați alt cuvânt (de exemplu „magazin” sau „clinică”) sau începeți cu o pagină goală.',
  'new.clear_search': 'Ștergeți căutarea',
  'new.select': 'Alegeți acest șablon',
  'new.selected': 'Ales',
  'new.preview': 'Previzualizare',
  'new.preview_close': 'Închideți',
  'new.preview_loading': 'Se încarcă șablonul…',
  'new.preview_error': 'Previzualizarea nu a putut fi încărcată.',
  'new.preview_desktop': 'Computer',
  'new.preview_mobile': 'Telefon',
  'new.preview_pages': 'Paginile șablonului',
  'new.preview_note':
    'Vizualizare orientativă: fotografiile sunt imagini provizorii pe care le veți înlocui cu ale dumneavoastră, iar datele demonstrative se modifică în editor.',
  'new.badge_store': 'Cu magazin',
  'new.badge_custom': 'Personalizat',
  'new.cat.belleza': 'Frumusețe și îngrijire',
  'new.cat.comida': 'Mâncare și băuturi',
  'new.cat.comercio': 'Magazine și comerț',
  'new.cat.oficios': 'Meserii și locuință',
  'new.cat.motor': 'Auto și moto',
  'new.cat.profesionales': 'Companii și servicii profesionale',
  'new.cat.salud': 'Sănătate',
  'new.cat.deporte': 'Sport',
  'new.cat.educacion': 'Educație și formare',
  'new.cat.creativos': 'Domenii creative și evenimente',
  'new.cat.turismo': 'Turism și cazare',
  'new.cat.local': 'Produse locale',
  'new.cat.comunidad': 'Asociații și comunitate',
  'new.cat.web': 'Pagini simple',
  'new.cat.otras': 'Altele',

  'publish.title': 'Publicați site-ul',
  'publish.action': 'Publicați',
  'publish.checklist': 'Listă de verificare înainte de publicare',
  'publish.preview': 'Previzualizare',

  'versions.title': 'Versiuni',
  'versions.create': 'Creați o versiune',
  'versions.restore': 'Restaurați această versiune',
  'versions.empty': 'Nu există încă versiuni salvate.',
  'versions.name': 'Numele versiunii',
  'versions.rev': 'Revizie',
  'versions.restored': 'Versiunea a fost restaurată',

  'domains.title': 'Domenii',
  'domains.add': 'Adăugați un domeniu',

  'analytics.title': 'Statistici',
  'analytics.visits': 'Vizite',

  'export.title': 'Export',
  'export.origins_label': 'Unde veți găzdui acest cod (origini permise)',
  'export.origins_help':
    'Introduceți câte un URL https:// pe linie de unde va fi apelat API-ul public.',
  'export.schemes_label': 'Scheme ale aplicației (pentru aplicații native)',
  'export.schemes_help':
    'Scheme URL personalizate (câte una pe linie), de exemplu: aplicatiamea, sytue.',
  'export.save': 'Salvați setările de export',
  'export.saved': 'Setările de export au fost salvate',

  'legal.title': 'Date legale',
  'legal.holder_name': 'Denumirea firmei sau titularul',
  'legal.tax_id': 'CUI / CIF / cod TVA',
  'legal.country': 'Țară',
  'legal.street': 'Adresă (stradă și număr)',
  'legal.postal_code': 'Cod poștal',
  'legal.city': 'Localitate',
  'legal.region': 'Județ sau regiune',
  'legal.email': 'E-mail de contact juridic',
  'legal.phone': 'Telefon',
  'legal.registry': 'Date din Registrul Comerțului (nr. de ordine, an)',
  'legal.privacy_contact': 'Contact pentru confidențialitate (DPO)',
  'legal.save': 'Salvați datele legale',
  'legal.saved': 'Datele legale au fost salvate',

  'settings.title': 'Setările site-ului',
  'settings.name': 'Numele site-ului',
  'settings.slug': 'Identificator (slug)',
  'settings.analytics': 'Activați statisticile integrate',
  'settings.analytics_help':
    'Înregistrează vizualizările de pagină respectând confidențialitatea, fără cookie-uri de la terți.',
  'settings.save': 'Salvați setările',
  'settings.saved': 'Setările au fost salvate',
  'settings.danger_zone': 'Zonă periculoasă',
  'settings.delete_title': 'Ștergeți acest site',
  'settings.delete_desc':
    'Site-ul va fi mutat în coșul de gunoi timp de 30 de zile, perioadă în care îl puteți restaura.',
  'settings.delete_confirm_prompt': 'Scrieți numele site-ului ({name}) pentru confirmare:',
  'settings.delete_button': 'Ștergeți site-ul',

  'errors.not_found': 'Site-ul nu a fost găsit',
  'errors.unauthorized': 'Nu aveți permisiunea de a vedea acest site',

  'ficha.page_title': 'Fișa site-ului',
  'ficha.back': 'Înapoi la site-urile mele',
  'ficha.sections': 'Secțiunile site-ului',
  'ficha.loading': 'Se încarcă site-ul',
  'ficha.load_error': 'Site-ul nu a putut fi încărcat',
  'ficha.open_site': 'Deschideți site-ul',
  'ficha.not_found_help': 'Este posibil să fi fost șters sau să nu aveți acces la el.',

  'publish.tab': 'Publicare',
  'publish.intro':
    'Verificați lista și publicați când totul este gata. O previzualizare nu modifică ceea ce vede publicul.',
  'publish.status': 'Stare actuală',
  'publish.check_ok': 'Gata',
  'publish.check_missing': 'Lipsește',
  'publish.check_legal': 'Date legale complete',
  'publish.check_legal_missing': 'Lipsesc: {campos}',
  'publish.check_legal_fix': 'Completați',
  'publish.check_demo': 'Fără texte demonstrative din șablon',
  'publish.check_ir': 'Pagini corect structurate',
  'publish.check_server':
    'La publicare se verifică și dacă nu au rămas texte demonstrative din șablon și dacă paginile sunt corect structurate.',
  'publish.done_preview': 'Previzualizarea a fost creată.',
  'publish.done_live': 'Site-ul a fost publicat.',
  'publish.open_preview': 'Deschideți previzualizarea',
  'publish.err_legal': 'Lipsesc date legale pentru publicare. Completați-le în fila „Date legale”.',
  'publish.err_demo':
    'Site-ul conține încă texte demonstrative din șablon. Înlocuiți-le în editor și publicați din nou.',
  'publish.err_ir':
    'O pagină nu este corect structurată. Deschideți-o în editor pentru a vedea ce trebuie corectat.',
  'publish.err_previews':
    'Există deja 5 previzualizări active. Așteptați să expire una înainte de a crea alta.',
  'publish.err_too_large':
    'Site-ul depășește dimensiunea maximă care poate fi publicată (64 MiB sau 5.000 de fișiere). Reduceți imaginile sau fișierele.',
  'publish.err_forbidden': 'Doar persoanele care administrează organizația pot publica site-ul.',
  'publish.err_conflict':
    'Site-ul nu a putut fi publicat deoarece s-a modificat sau este suspendat. Reîncărcați pagina și încercați din nou.',
  'publish.err_invalid': 'Publicarea nu este validă. Verificați conținutul site-ului în editor.',
  'publish.err_generic': 'Site-ul nu a putut fi publicat. Încercați din nou.',

  'versions.intro':
    'O versiune este o copie a ciornei la care puteți reveni. Creați una înainte de o modificare importantă.',
  'versions.name_placeholder': 'Înainte de modificarea paginii principale',
  'versions.manual_name': 'Versiune manuală',
  'versions.unnamed': 'Fără nume',
  'versions.loading': 'Se încarcă versiunile',
  'versions.load_error': 'Versiunile nu au putut fi încărcate',
  'versions.empty_help': 'Când creați o versiune sau publicați site-ul, aceasta va apărea aici.',
  'versions.created': 'Versiunea a fost creată',
  'versions.create_error': 'Versiunea nu a putut fi creată. Încercați din nou.',
  'versions.restore_error': 'Versiunea nu a putut fi restaurată. Încercați din nou.',
  'versions.restore_confirm':
    'Ciorna va reveni la starea „{nombre}”. Starea actuală este salvată mai întâi ca „Înainte de restaurare”, astfel încât să puteți reveni la ea.',
  'versions.kind.manual': 'Manuală',
  'versions.kind.publish': 'Publicare',
  'versions.kind.preview': 'Previzualizare',
  'versions.kind.export': 'Export',
  'versions.kind.before_restore': 'Înainte de restaurare',

  'domains.intro':
    'Conectați propriul domeniu. După adăugare, creați la furnizorul dumneavoastră DNS înregistrările indicate și verificați configurarea.',
  'domains.host_label': 'Domeniu',
  'domains.host_placeholder': 'www.exemplu.ro',
  'domains.host_help': 'Doar numele domeniului, fără https:// sau bare oblice.',
  'domains.empty': 'Nu există încă domenii proprii configurate.',
  'domains.loading': 'Se încarcă domeniile',
  'domains.load_error': 'Domeniile nu au putut fi încărcate',
  'domains.primary': 'Principal',
  'domains.status.pending': 'În așteptarea verificării',
  'domains.status.verified': 'Verificat',
  'domains.status.active': 'Activ',
  'domains.status.failed': 'Cu eroare',
  'domains.dns_title': 'Înregistrări DNS',
  'domains.dns_txt': 'TXT (verificare)',
  'domains.dns_cname': 'CNAME (subdomenii precum www)',
  'domains.dns_a': 'A (domeniu rădăcină)',
  'domains.check': 'Verificați DNS și HTTPS',
  'domains.make_primary': 'Setați ca principal',
  'domains.remove': 'Eliminați',
  'domains.remove_title': 'Eliminați domeniul',
  'domains.remove_confirm':
    '„{host}” nu va mai indica spre acest site. Va trebui să îl adăugați din nou pentru a-l refolosi.',
  'domains.added':
    'Domeniul a fost adăugat. Creați la furnizorul dumneavoastră DNS înregistrările afișate mai jos.',
  'domains.updated': 'Domeniile au fost actualizate.',
  'domains.action_error': 'Acțiunea nu a putut fi finalizată. Încercați din nou.',
  'domains.err_invalid': 'Numele domeniului nu este valid.',
  'domains.err_reserved': 'Acest domeniu este rezervat și nu poate fi folosit.',
  'domains.err_taken': 'Acest domeniu este deja folosit de un alt site.',
  'domains.err_limit': 'Ați atins numărul maxim de domenii pentru acest site.',
  'domains.err_suspended': 'Site-ul este suspendat și nu permite modificări de domeniu.',
  'domains.err_forbidden': 'Doar persoanele care administrează organizația pot gestiona domeniile.',
  'domains.err_generic': 'Operațiunea pentru domeniu nu a putut fi finalizată. Încercați din nou.',
  'domains.start_title': 'Conectați-vă domeniul, pas cu pas',
  'domains.start_help':
    'Introduceți domeniul pe care doriți să îl folosiți. Veți avea nevoie de acces la panoul DNS al furnizorului de la care l-ați achiziționat.',
  'domains.progress': 'Progresul conectării domeniului',
  'domains.step.registered': 'Adăugat în Sytue Web',
  'domains.step.dns': 'DNS verificat',
  'domains.step.https': 'HTTPS pregătit',
  'domains.step.primary': 'Adresă principală',
  'domains.step.done': 'Finalizat',
  'domains.step.pending': 'În așteptare',
  'domains.status.review': 'Necesită verificare',
  'domains.record_name': 'Numele înregistrării',
  'domains.record_value': 'Valoarea înregistrării',
  'domains.routing_title': 'Adresa site-ului',
  'domains.dns_help':
    'Deschideți setările DNS la furnizorul dumneavoastră și adăugați aceste două înregistrări. Copiați fiecare nume și valoare exact așa cum apar.',
  'domains.dns_names_help':
    'Unii furnizori folosesc @ pentru domeniul rădăcină sau doar www pentru acel subdomeniu. Numele complet al înregistrării este afișat mai sus.',
  'domains.routing_legacy_help':
    'Folosiți A pentru domeniul rădăcină sau CNAME pentru un subdomeniu precum www, cu destinația corespunzătoare indicată de serviciu.',
  'domains.dns_propagation':
    'După salvarea înregistrărilor, apăsați pe „Verificați DNS și HTTPS”. Dacă nu apar încă, așteptați ca furnizorul să propage modificarea și verificați din nou. Păstrați înregistrările de e-mail și ale altor servicii.',
  'domains.records_unavailable':
    'Serviciul nu a furnizat încă toate datele acestei înregistrări. Nu modificați DNS-ul până când nu sunt disponibile.',
  'domains.copy': 'Copiați',
  'domains.copied': 'Copiat',
  'domains.copy_field': 'Copiați {campo}',
  'domains.copy_error':
    'Valoarea nu a putut fi copiată în clipboard. Selectați valoarea câmpului și copiați-o manual.',
  'domains.last_check': 'Ultima verificare: {fecha}',
  'domains.publish_help':
    'Când adresa principală este pregătită, publicați din nou site-ul pentru ca linkurile și redirecționările să folosească domeniul dumneavoastră.',
  'domains.publish_action': 'Mergeți la publicare',
  'domains.detail.pending':
    'Domeniul este adăugat. Configurați înregistrările DNS și verificați conexiunea pentru a continua.',
  'domains.detail.txt_missing':
    'Înregistrarea TXT nu a fost găsită încă. Adăugați-o la furnizorul DNS și verificați din nou după propagare.',
  'domains.detail.txt_mismatch':
    'Valoarea TXT nu corespunde. Înlocuiți-o cu valoarea exactă afișată pe acest ecran.',
  'domains.detail.dns_unreachable':
    'DNS-ul acestui domeniu nu a putut fi interogat. Verificați domeniul și serverele sale de nume, apoi încercați din nou.',
  'domains.detail.routing':
    'Domeniul nu indică încă spre site. Verificați înregistrarea A sau CNAME indicată și încercați din nou.',
  'domains.detail.certificate':
    'Înregistrările DNS sunt verificate. Certificatul HTTPS nu este încă pregătit; verificați din nou în câteva minute.',
  'domains.detail.active': 'DNS-ul și conexiunea HTTPS sunt verificate. Domeniul este pregătit.',
  'domains.detail.unknown':
    'Ultima verificare necesită atenție. Verificați înregistrările și testați conexiunea din nou.',

  'analytics.intro':
    'Vizitatorii reprezintă o estimare zilnică pe durata pilotului; nu reprezintă utilizatori unici pe întreaga perioadă.',
  'analytics.visitors': 'Vizitatori estimați',
  'analytics.loading': 'Se încarcă statisticile',
  'analytics.load_error': 'Statisticile nu au putut fi încărcate',
  'analytics.unavailable':
    'Serviciul de statistici nu este disponibil în acest moment. Încercați din nou în câteva minute.',
  'analytics.disabled': 'Statisticile sunt dezactivate',
  'analytics.disabled_help':
    'Activați-le din setările site-ului pentru a începe numărarea vizitelor.',
  'analytics.go_settings': 'Mergeți la setări',
  'analytics.refresh': 'Reîmprospătați',

  'export.intro':
    'Dacă găzduiți codul site-ului în afara Sytue, indicați de unde va fi apelat API-ul public.',
  'export.origins_placeholder': 'https://magazinulmeu.exemplu.ro',
  'export.schemes_placeholder': 'aplicatiamea',
  'export.origins_invalid':
    'Fiecare linie trebuie să fie o adresă https:// validă (linia {n} nu este).',
  'export.schemes_invalid':
    'Fiecare linie trebuie să fie o schemă validă: începe cu o literă și conține doar litere, cifre, „+”, „-” sau „.” (linia {n} nu este).',
  'export.err_generic': 'Setările nu au putut fi salvate. Încercați din nou.',

  'legal.intro': 'Aceste date sunt obligatorii pentru publicarea site-ului.',
  'legal.required_note': 'Câmpurile marcate cu * sunt obligatorii.',
  'legal.saved_incomplete': 'Datele au fost salvate. Mai lipsesc pentru publicare: {campos}.',
  'legal.country_help': 'Cod din două litere, de exemplu RO.',
  'legal.registry_help':
    'Obligatoriu pentru societățile comerciale (de exemplu numărul de înregistrare la Registrul Comerțului).',
  'legal.ph_holder': 'Exemplu S.R.L.',
  'legal.ph_street': 'Strada Principală nr. 12, et. 1',
  'legal.ph_city': 'București',
  'legal.ph_email': 'juridic@exemplu.ro',
  'legal.ph_privacy': 'dpo@exemplu.ro',
  'legal.ph_registry': 'Reg. Com. București, J40/1234/2026',
  'legal.err_generic': 'Datele legale nu au putut fi salvate. Încercați din nou.',

  'settings.tab': 'Setări',
  'settings.intro': 'Numele, adresa și opțiunile generale ale site-ului.',
  'settings.slug_help':
    'Doar litere mici, cifre și cratime. Dacă îl modificați, se schimbă adresa site-ului.',
  'settings.admin_only':
    'Doar persoanele care administrează organizația pot modifica această opțiune.',
  'settings.name_required': 'Numele site-ului nu poate fi gol.',
  'settings.err_slug_taken': 'Un alt site folosește deja acest identificator.',
  'settings.err_invalid':
    'Verificați numele, identificatorul și adresa: unul dintre aceste câmpuri nu este valid.',
  'settings.err_generic': 'Setările nu au putut fi salvate. Încercați din nou.',
  'settings.delete_error': 'Site-ul nu a putut fi șters. Încercați din nou.',

  'forms.empty': 'Nu există încă răspunsuri',
  'forms.empty_help':
    'Când cineva trimite un formular de pe site-ul dumneavoastră, îl veți vedea aici.',
};
