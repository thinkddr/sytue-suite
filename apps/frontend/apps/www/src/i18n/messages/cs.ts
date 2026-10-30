/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const cs: AppCatalog<typeof en> = {
  'skip.content': 'Přejít na obsah',
  'nav.products': 'Produkty',
  'nav.sovereignty': 'Suverenita',
  'nav.standards': 'Otevřené standardy',
  'nav.pricing': 'Ceny',
  'footer.pricing': 'Ceny',
  'home.pricing.title': 'Tři tarify, celá sada',
  'home.pricing.body':
    'Ve všech třech stejné produkty: mění se jen úložiště na uživatele. Žádné osekané edice.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'bez DPH',
  'home.pricing.per': '/Uživatel/Měsíc',
  'home.pricing.cta': 'Kontaktovat obchod',
  'home.pricing.commitment.title': 'Závazek na celý život',
  'home.pricing.commitment.body':
    'Cena za uživatele se každé čtyři roky zvýší o 0,50 € a nic víc: po celý život. Víte to od prvního dne a nemění se to.',
  'home.pricing.included.title': 'Jediná edice',
  'home.pricing.included.body':
    'Žádné tarify podle funkcí, žádné úrovně, žádné osekané edice: celá sada je v ceně za uživatele, se stejnými funkcemi pro všechny.',
  'home.pricing.metered.title': 'Co se účtuje zvlášť',
  'home.pricing.metered.body': 'Dodatečné úložiště: podle spotřeby nebo v blocích.',
  'home.pricing.storage.title': 'Úložiště je sdílené',
  'home.pricing.storage.body':
    'Kvóta každé osoby se přičítá ke kvótě organizace: nevyužitá kapacita jedné osoby připadne ostatním. Kvótu na uživatele lze navíc upravit v rámci organizace.',
  'nav.downloads': 'Ke stažení',
  'nav.aria': 'Navigace webu',
  'nav.ariaMobile': 'Mobilní navigace',
  'nav.home': 'Sytue One — Úvod',
  'nav.openMenu': 'Otevřít nabídku',
  'nav.closeMenu': 'Zavřít nabídku',
  'theme.toLight': 'Přepnout na světlý režim',
  'theme.toDark': 'Přepnout na tmavý režim',
  'lang.label': 'Jazyk',
  'cta.start': 'Začít zdarma',
  'cta.signIn': 'Přihlásit se',
  'cta.seeSuite': 'Prohlédnout sadu',
  'meta.home.title': 'Sytue One — Suverénní evropská sada pro produktivitu',
  'meta.home.description':
    'Sytue One: suverénní evropská sada pro produktivitu. Identita, pošta, soubory, dokumenty, kalendář a chat, 100% v EU a postavené na otevřených standardech.',
  'home.hero.lead': 'Vaše produktivita,',
  'home.hero.accent': 'suverénní a evropská',
  'home.hero.subtitle':
    'Pošta, soubory, dokumenty, kalendář, chat a videohovory — propojené v jedné aplikaci a hostované výhradně v EU.',
  'home.hero.note': 'Bez platební karty. Data ve Scaleway (EU). Asistovaná migrace.',
  'home.privacy.title': 'Vaše práce je vaše. Vaše soukromí také.',
  'home.privacy.body': 'Vaše údaje neprodáváme ani je nepoužíváme k vytváření reklamních profilů.',
  'home.privacy.anonymity':
    'Sytue Analytics chrání vaše soukromí pomocí souhrnných statistik, bez sledovacích cookies a s pseudonymními identifikátory návštěvníků, které se každý den mění a jsou pro každý web samostatné.',
  'home.privacy.account':
    'K používání sady potřebujeme údaje o účtu a záznamy o přístupu, abychom mohli službu poskytovat a udržovat ji bezpečnou.',
  'home.privacy.link': 'Jak chráníme vaše soukromí',
  'home.stats.aria': 'Evropská přítomnost',
  'home.stats.regions': 'Regiony EU (fr-par · nl-ams · vlastní zóna v Madridu)',
  'home.stats.apps': 'Aplikace za jedinou identitou',
  'home.stats.standards': 'Otevřené standardy: OIDC, JMAP, CalDAV, CardDAV a WebDAV',
  'home.products.title': 'Jedna sada, jeden zážitek',
  'home.products.subtitle':
    'Všechny produkty sdílejí stejný design, identitu a rozhraní. Vypadají a chovají se jako jediná aplikace.',
  'home.integration.title': 'Integrace, jaká má sada být',
  'home.integration.subtitle':
    'Jedna registrace otevře vše. Automatické zřizování (SCIM) ze Sytue ID.',
  'home.integration.sso.title': 'Jednotné přihlášení',
  'home.integration.sso.body': 'Jedna identita (Sytue ID) pro celou sadu. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Společné rozhraní',
  'home.integration.shell.body':
    'Horní lišta, přepínač aplikací, vyhledávání a globální oznámení v každém produktu.',
  'home.integration.cross.title': 'Akce napříč aplikacemi',
  'home.integration.cross.body':
    'Přiložte soubor z Drive v Mail, udělejte z e-mailu událost v Calendar a sdílejte ji v Rooms bez přerušení práce.',
  'home.sovereignty.title': 'Skutečná suverenita',
  'home.sovereignty.body':
    'Nejde o marketingový slib, ale o architekturu. Infrastruktura běží pouze v evropských regionech a vše stojí na otevřených, auditovatelných protokolech bez zadních vrátek.',
  'home.sovereignty.link': 'Proč otevřené standardy',
  'home.sovereignty.eu.title': '100% v EU',
  'home.sovereignty.eu.body':
    'Veškerá infrastruktura v EU: Scaleway (fr-par / nl-ams) a vlastní zóna v Madridu. Žádní poskytovatelé z USA pro osobní údaje.',
  'home.sovereignty.gdpr.title': 'Soulad s GDPR',
  'home.sovereignty.gdpr.body':
    'Smlouva o zpracování, záznamy o činnostech zpracování, postup pro žádosti subjektů údajů a doby uchování. Šifrování při přenosu (TLS 1.3) i v klidu.',
  'home.sovereignty.lockin.title': 'Bez závislosti na dodavateli',
  'home.sovereignty.lockin.body':
    'Postaveno na otevřených standardech: OIDC, JMAP, CalDAV, CardDAV a WebDAV. Vaše data zůstávají přenositelná.',
  'home.standards.overline': 'Vlastní vývoj Sytue',
  'home.standards.title': 'Postaveno na otevřených standardech',
  'home.standards.body':
    'Každý produkt Sytue je vlastní vývoj, napsaný v Go a Astro + Preact nad otevřenými protokoly (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Žádná cizí jádra: vaše data jsou přenositelná a vaše sada suverénní.',
  'home.cta.title': 'Převezměte znovu kontrolu nad svou prací',
  'home.cta.body':
    'Začněte se svou organizací ještě dnes. Asistovaná migrace z Google Workspace nebo Microsoft 365.',
  'home.cta.primary': 'Vytvořit organizaci',
  'home.cta.secondary': 'Kontaktovat obchod',
  'grid.discover': 'Zjistit více',
  'product.mail.blurb': 'Soukromá evropská pošta přes JMAP — bez sledování a bez reklam.',
  'product.drive.blurb': 'Soubory a spolupráce pod vaší vlastní značkou.',
  'product.docs.blurb': 'Dokumenty, tabulky a prezentace se společnými úpravami v reálném čase.',
  'product.calendar.blurb': 'Kalendář přes CalDAV propojený se zbytkem sady.',
  'product.contacts.blurb': 'Kontakty přes CardDAV sdílené v celé organizaci.',
  'product.notes.blurb':
    'Sešity, do kterých se vejde cokoli: text, soubory, události i celé dokumenty.',
  'product.photos.blurb': 'Galerie a editor fotek: originály v Drive, úpravy jako recept.',
  'product.rooms.blurb': 'Kanály, zprávy a týmové videohovory na jednom místě.',
  'product.cast.blurb':
    'Video na vyžádání a živé vysílání: nahrávejte, vysílejte a sdílejte, v EU.',
  'product.pages.blurb':
    'Weby a internetové obchody pro vaši firmu, navržené podle vašich představ a publikované v EU.',
  'product.bases.blurb': 'Společné relační tabulky pro zákazníky, objednávky nebo zásoby, v EU.',
  'product.cloud.blurb': 'Platforma pro nasazení a spravovaná data v EU.',
  'product.id.blurb': 'Jediná identita (SSO + MFA) se Sytue ID.',
  'product.one.blurb': 'Úvodní stránka sady: aplikace, rychlé akce, nedávná aktivita a upozornění.',
  'product.sheets.blurb': 'Tabulky se vzorci, externími daty a grafy, upravované mnoha lidmi.',
  'product.slides.blurb': 'Prezentace s vaší značkou, motivy a zobrazením přednášejícího.',
  'product.pdf.blurb': 'Čtěte, anotujte, podepisujte a uspořádejte PDF bez opuštění Evropy.',
  'product.console.blurb': 'Správa organizace: lidé, přístupy, dodržování předpisů a audit.',
  'product.erp.blurb':
    'Dvacet čtyři modulů a podvojné účetnictví, které databáze nedovolí rozvážit.',
  'productPage.back': 'Celá sada',
  'productPage.features': 'Vše, co {name} umí',
  'productPage.featuresPlanned': 'Co bude {name} umět',
  'productPage.builtOn': 'Postaveno na {standard}',
  'productPage.fromScratch':
    'Editor Docus, Calcus, Diapos a CertiPDF je napsaný od nuly: vlastní engine Sytue, ne integrace třetí strany.',
  'productPage.shot': 'Hlavní obrazovka {name} přesně tak, jak vypadá po přihlášení.',
  'productPage.shotFeatures': '{name} v akci, s funkcemi na obrazovce.',
  'productPage.open': 'Otevřít {name}',
  'productPage.enDesarrollo': 'Tento produkt je ve vývoji: zatím není k dispozici.',
  'productPage.included':
    'Součást Sytue One: jedna identita (Sytue ID) a jediná registrace dávají přístup ke všem produktům. Data 100% v EU (Scaleway), přenositelná díky otevřeným standardům.',
  'product.mail.tagline': 'Soukromá evropská pošta bez sledování a bez reklam.',
  'product.mail.standard': 'JMAP (otevřený standard)',
  'product.mail.p1': 'Rychlá schránka s vlákny, štítky, filtry a vyhledáváním.',
  'product.mail.p2': 'Přílohy přímo ze Sytue Drive.',
  'product.mail.p3': 'Z e-mailu událost v Calendar.',
  'product.mail.p4':
    'Šifrování při přenosu (TLS 1.3). Infrastruktura Sytue je v EU. Umístění externích služeb závisí na jejich poskytovateli.',
  'product.drive.tagline': 'Vaše soubory a spolupráce pod značkou vaší organizace.',
  'product.drive.standard': 'WebDAV (otevřený standard)',
  'product.drive.p1':
    'Nahrávání, třídění a sdílení přes kontrolované odkazy (oprávnění a platnost).',
  'product.drive.p2': 'Náhled dokumentů a obrázků přímo na stránce.',
  'product.drive.p3': 'Přílohy v Mail bez duplikování souboru.',
  'product.drive.p4': 'Úložiště S3 v evropských regionech.',
  'product.docs.tagline': 'Dokumenty, tabulky a prezentace se společnými úpravami v reálném čase.',
  'product.docs.standard': 'Společné úpravy v reálném čase',
  'product.docs.p1': 'Pište společně, s živou přítomností a okamžitými změnami.',
  'product.docs.p2': 'Tabulky se vzorci a podmíněným formátováním.',
  'product.docs.p3': 'Prezentace s vaší značkou.',
  'product.docs.p4': 'Vše uloženo ve vašem evropském Drive.',
  'product.calendar.tagline': 'Váš kalendář propojený se zbytkem sady.',
  'product.calendar.standard': 'CalDAV (otevřený standard)',
  'product.calendar.p1': 'Zobrazení dne, týdne a měsíce.',
  'product.calendar.p2': 'Opakované události a připomenutí.',
  'product.calendar.p3': 'Události přímo z e-mailu v Mail.',
  'product.calendar.p4': 'Funguje s jakýmkoli klientem CalDAV.',
  'product.contacts.tagline': 'Kontakty vaší organizace, sdílené a přenositelné.',
  'product.contacts.standard': 'CardDAV (otevřený standard)',
  'product.contacts.p1': 'Sdílený adresář pro celou organizaci.',
  'product.contacts.p2': 'Import a export ve formátu vCard.',
  'product.contacts.p3': 'Slučování duplicit bez ztráty dat.',
  'product.contacts.p4': 'Funguje s jakýmkoli klientem CardDAV.',
  'product.notes.tagline': 'Vše, co chcete uchovat, na jednom místě a ve vašich rukou.',
  'product.notes.standard': 'Vlastní model nad otevřenými standardy (vlastní REST API)',
  'product.notes.p1': 'Vnořené sešity na volném plátně, ve sloupcích nebo jako seznam.',
  'product.notes.p2':
    'Poznámky všeho druhu: text, úkoly, odkazy, soubory, události z Calendar, dokumenty z Docs.',
  'product.notes.p3': 'Koncové šifrování: heslo nikdy neopustí váš prohlížeč.',
  'product.notes.p4': 'Rozhodujete, co se dostane do vyhledávání — a co nikdy.',
  'product.rooms.tagline': 'Kanály, zprávy a týmové videohovory na jednom místě.',
  'product.rooms.standard': 'Zprávy a hovory v reálném čase',
  'product.rooms.p1': 'Kanály podle týmu nebo projektu.',
  'product.rooms.p2': 'Videohovor zabudovaný v každém kanálu.',
  'product.rooms.p3': 'Zmínky a oznámení sjednocené v celé sadě.',
  'product.rooms.p4': 'Prohledávatelná historie odkudkoli v Sytue One.',
  'product.cloud.p4': 'Vlastní domény s automatickým TLS.',
  'product.cloud.p3': 'Spravované úložiště S3 a funkce.',
  'product.cloud.p2': 'Databáze Postgres s automatickým REST API.',
  'product.cloud.p1': 'Náhled pro každý PR a okamžitý návrat zpět.',
  'product.cloud.standard': 'PaaS nativní pro Kubernetes',
  'product.cloud.tagline': 'Váš kód nasazený a vaše data spravovaná v Evropské unii.',
  'product.id.tagline': 'Jedna identita pro celou sadu. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (otevřený standard)',
  'product.id.p1': 'Jednotné přihlášení do všech produktů.',
  'product.id.p2': 'MFA a přístupové klíče (WebAuthn).',
  'product.id.p3': 'Automatické zřizování (SCIM) z vašeho adresáře.',
  'product.id.p4': 'Profil a zabezpečení na jednom místě.',
  'product.one.tagline': 'Celý Sytue One na jedno kliknutí, jedněmi dveřmi.',
  'product.one.standard': 'Jedno přihlášení (Sytue ID) pro celou sadu',
  'product.one.p1': 'Spouštěč se všemi vašimi aplikacemi i těmi, které připojí vaše organizace.',
  'product.one.p2': 'Vyhledávání v celé sadě (⌘K) z jakékoli obrazovky.',
  'product.one.p3':
    'Dnešní program a nepřečtená pošta na úvodní stránce, vedle novinek z celé sady a vašich oznámení.',
  'product.one.p4': 'Připojte svá zařízení a stáhněte si nativní aplikace.',
  'product.sheets.tagline': 'Čísla, která dávají smysl, v tabulce, která pracuje s vámi.',
  'product.sheets.standard': 'Formáty ODF/OOXML a společné úpravy v reálném čase',
  'product.sheets.p1': 'Vzorce, podmíněné formátování a ověřování dat.',
  'product.sheets.p2': 'Načtěte externí data a dotazujte se na ně bez opuštění tabulky.',
  'product.sheets.p3': 'Grafy, minigrafy, průřezy a datové tabulky.',
  'product.sheets.p4': 'Export do ODS a XLSX; otevírání souborů Excel.',
  'product.slides.tagline': 'Vyprávějte příběh; snímek drží krok.',
  'product.slides.standard': 'Formáty ODF/OOXML a společné úpravy v reálném čase',
  'product.slides.p1': 'Motivy prezentací s identitou vaší organizace.',
  'product.slides.p2': 'Zobrazení přednášejícího s poznámkami a dalším snímkem.',
  'product.slides.p3': 'Obrázky, tvary, grafy a SmartArt.',
  'product.slides.p4': 'Export do ODP, PPTX a PDF; otevírání souborů PowerPoint.',
  'product.pdf.tagline': 'PDF od začátku do konce, v prohlížeči.',
  'product.pdf.standard': 'PDF (ISO 32000) se standardními anotacemi a formuláři',
  'product.pdf.p1': 'Zvýrazňování, podtrhávání, komentáře a kreslení v dokumentu.',
  'product.pdf.p2': 'Formuláře: vyplňování polí nebo tvorba vlastních.',
  'product.pdf.p3': 'Slučování, extrakce, otáčení, číslování a vodoznaky.',
  'product.pdf.p4': 'Panel podpisů, trvalé začernění a dokumenty chráněné heslem.',
  'product.console.tagline': 'Spravujte sadu z jediné obrazovky.',
  'product.console.standard': 'SCIM, SAML a OpenID Connect (otevřené standardy)',
  'product.console.p1': 'Lidé, skupiny (i dynamické) a role.',
  'product.console.p2': 'SSO s vaším poskytovatelem a zřizování přes SCIM.',
  'product.console.p3':
    'Pravidla podmíněného přístupu, zařízení a privilegovaný přístup se schválením.',
  'product.console.p4': 'Auditní protokol exportovatelný do vašeho SIEM a právní blokace.',
  'product.erp.tagline': 'Celý ERP, s katalogem modelů jako datovým souborem.',
  'product.erp.standard': 'Podvojné účetnictví, katalog modelů v JSON a jediné REST API',
  'product.erp.p1':
    'Dvacet čtyři modulů a třicet šest modelů: kontakty, prodej, nákup, sklad, výroba, fakturace, účetnictví, projekty a personalistika.',
  'product.erp.p2':
    'Přidat modul znamená napsat soubor JSON: bez migrace, bez nové trasy a bez práce na rozhraní.',
  'product.erp.p3':
    'Peníze se ukládají v celých centech, nikdy v plovoucí čárce, a vyrovnanost hlídá databáze.',
  'product.erp.p4':
    'Zaúčtované zápisy jsou nedotknutelné: opravují se dalším zápisem, a to odlišuje účetní knihu od tabulky.',
  'product.one.f1':
    'Spouštěč s aplikacemi sady a aplikacemi třetích stran, které připojí vaše organizace.',
  'product.one.f2': 'Rychlé akce: napsat e-mail, nový dokument, nahrát do Drive, vytvořit událost.',
  'product.one.f3': 'Nedávno v celé sadě: poslední položky z Drive, Docus, Mail nebo Rooms.',
  'product.one.f4': 'Globální vyhledávání přes ⌘K nebo Ctrl K, s výsledky ze všech aplikací.',
  'product.one.f5': 'Centrum oznámení a přítomnost sdílené v celé sadě.',
  'product.one.f6':
    'Připojení zařízení: profil pro iPhone a Mac, Android a počítač, ve dvou krocích.',
  'product.one.f7': 'Ke stažení nativní aplikace pro počítač a mobil.',
  'product.one.f8': 'Uvítací průvodce, který připraví organizaci bez manuálů.',
  'product.mail.f1': 'Konverzace ve vláknech se čtením, odpovědí a přeposláním na jedno kliknutí.',
  'product.mail.f2':
    'Složky, barevné štítky a vyhledávání s operátory (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Automatická pravidla, která poštu při doručení archivují, označí štítkem nebo přesunou.',
  'product.mail.f4': 'Plánované odeslání a „Vrátit odeslání“.',
  'product.mail.f5': 'Podpisy, aliasy adres a automatická odpověď v nepřítomnosti.',
  'product.mail.f6': 'Sdílené a delegované schránky s oprávněním ke čtení a odesílání.',
  'product.mail.f7':
    'Editor formátovaného textu s přílohami ze Sytue Drive bez duplikování souborů.',
  'product.mail.f8':
    'Vzdálený obsah je ve výchozím stavu blokovaný: nikdo se nedozví, kdy e-mail otevřete.',
  'product.mail.f9': 'Podepsaná pošta (PGP a S/MIME) se rozpozná a označí; TLS při přenosu.',
  'product.mail.f10':
    'Přístup i z Thunderbirdu, Outlooku, Apple Mail nebo telefonu, s hesly pro aplikace.',
  'product.mail.f11':
    'Adresy jako štítky s adresářem Contacts na jedno kliknutí a upozornění, když napíšete „v příloze“ a nic nepřiložíte.',
  'product.mail.f12':
    'Pravidla přímo z otevřeného e-mailu, vysypání koše najednou a klávesové zkratky přes „?“.',
  'product.drive.f1': 'Můj disk, týmové disky, sdíleno se mnou, nedávné, oblíbené a koš.',
  'product.drive.f2':
    'Nahrávání přetažením; složky, přesun, kopírování, přejmenování a vícenásobný výběr.',
  'product.drive.f3':
    'Odkazy ke sdílení se dvěma úrovněmi (zobrazení nebo úpravy), heslem a platností.',
  'product.drive.f4': 'Historie verzí s obnovením.',
  'product.drive.f5': 'Náhled dokumentů, obrázků a PDF bez stahování.',
  'product.drive.f6':
    'Otevírání a úpravy kancelářských dokumentů v Docus, Calcus a Diapos; PDF v CertiPDF.',
  'product.drive.f7': 'Štítky citlivosti, které blokují veřejné odkazy tam, kam nepatří.',
  'product.drive.f8': 'Sdílení do kanálu Rooms nebo příloha v Mail bez duplikování souboru.',
  'product.drive.f9': 'Přístup přes WebDAV a desktopový klient pro zobrazení souborů v počítači.',
  'product.drive.f10': 'Úložiště S3 v evropských regionech, s kvótou vždy na očích.',
  'product.docs.f1': 'Společné úpravy v reálném čase, s přítomností a bez vzájemného přepisování.',
  'product.docs.f2': 'Pás karet: Domů, Vložení, Reference, Korespondence a Revize.',
  'product.docs.f3': 'Tabulky, obrázky, tvary, SmartArt, grafy, symboly a vodoznaky.',
  'product.docs.f4': 'Reference: obsah, poznámky pod čarou, citace a bibliografie.',
  'product.docs.f5': 'Korespondence: hromadná korespondence pro dopisy a štítky.',
  'product.docs.f6': 'Komentáře ve vláknech, revize změn a verze.',
  'product.docs.f7':
    'Volitelný asistent AI pro přepis, překlad nebo shrnutí, jen pokud ho vaše organizace povolí.',
  'product.docs.f8': 'Export do ODT, DOCX a PDF; otevírání souborů Word z Drive.',
  'product.docs.f9': 'Záhlaví, zápatí, číslování stránek a opakovaně použitelné bloky.',
  'product.docs.f10': 'Automatické ukládání do vašeho evropského Drive, s historií.',
  'product.docs.f11':
    'Stažení z úvodní stránky jako PDF, HTML, Markdown, OpenDocument nebo Microsoft Office.',
  'product.sheets.f1': 'Vzorce s kontextovou nápovědou a okamžitým přepočtem.',
  'product.sheets.f2':
    'Podmíněné formátování, ověřování dat a formáty čísel podle národního prostředí.',
  'product.sheets.f3': 'Grafy různých typů, minigrafy a průřezy.',
  'product.sheets.f4': 'Datové tabulky se součty a řazením; kontingenční tabulka.',
  'product.sheets.f5': 'Externí data: připojte zdroj a dotazujte se na něj z tabulky.',
  'product.sheets.f6': 'Řešitel pro nalezení hodnoty, která splní cíl.',
  'product.sheets.f7': 'Makra pro automatizaci opakovaných úloh.',
  'product.sheets.f8': 'Spolupráce v reálném čase s přítomností a komentáři.',
  'product.sheets.f9': 'Filtry, průřezy a vyhledávání v tabulce.',
  'product.sheets.f10': 'Export do ODS, XLSX a PDF; otevírání souborů Excel z Drive.',
  'product.slides.f1': 'Motivy a palety v souladu se značkou vaší organizace.',
  'product.slides.f2': 'Plátno s tvary, obrázky, grafy, SmartArt a spojnicemi.',
  'product.slides.f3': 'Prezentace na celou obrazovku se zobrazením přednášejícího a poznámkami.',
  'product.slides.f4': 'Spolupráce v reálném čase s přítomností a komentáři.',
  'product.slides.f5': 'Snímky s pozadími, plakáty a rozvrženími.',
  'product.slides.f6': 'Export do ODP, PPTX a PDF.',
  'product.slides.f7': 'Otevírání souborů PowerPoint z Drive.',
  'product.slides.f8': 'Automatické ukládání s verzemi.',
  'product.pdf.f1':
    'Rychlý prohlížeč s osnovou, miniaturami, vyhledáváním a přiblížením na stránku nebo šířku.',
  'product.pdf.f2':
    'Anotace: zvýraznění, podtržení, přeškrtnutí, komentáře, kresba, tvary, šipky a razítka.',
  'product.pdf.f3': 'Textová pole, měření a snímky oblasti.',
  'product.pdf.f4': 'Formuláře: vyplňování existujících polí nebo přidání vlastních.',
  'product.pdf.f5': 'Panel podpisů dokumentu.',
  'product.pdf.f6': 'Trvalé začernění: označený obsah zmizí ze souboru, nejen z pohledu.',
  'product.pdf.f7':
    'Stránky: sloučení, extrakce, odstranění, otočení, číslování (i Bates), záhlaví a zápatí.',
  'product.pdf.f8': 'Vodoznak, pozadí a upravitelná metadata.',
  'product.pdf.f9': 'Dokumenty chráněné heslem a oprávnění.',
  'product.pdf.f10': 'Předčítání nahlas a komentáře ve vláknech.',
  'product.calendar.f1':
    'Zobrazení měsíce, týdne, dne a programu, s minikalendářem a tlačítkem „Dnes“.',
  'product.calendar.f2':
    'Denní, týdenní nebo měsíční opakování s koncem; úprava jen této, této a následujících, nebo všech.',
  'product.calendar.f3': 'Připomenutí pro každou událost a oznámení v rámci sady.',
  'product.calendar.f4': 'Pozvánky s odpovědí (zúčastním se, možná, ne) a přehledem odpovědí.',
  'product.calendar.f5': '„Najít termín“, který vyhovuje všem pozvaným osobám.',
  'product.calendar.f6': 'Rezervovatelné místnosti a prostředky s dostupností.',
  'product.calendar.f7': 'Sdílené kalendáře s oprávněními a kalendáře sdílené s vámi.',
  'product.calendar.f8': 'Odběr externích kalendářů (ICS) a několik barevně odlišených kalendářů.',
  'product.calendar.f9': 'Vytvoření události ze zprávy v Mail nebo z poznámky.',
  'product.calendar.f10':
    'Funguje s jakýmkoli klientem CalDAV: Apple Calendar, Thunderbird a další.',
  'product.calendar.f11':
    'Duplikování události nebo její zkopírování do vašeho kalendáře a stažení jako .ics pro kohokoli mimo Sytue.',
  'product.contacts.f1': 'Adresáře, včetně sdíleného adresáře vaší organizace.',
  'product.contacts.f2':
    'Úplná karta: telefony, e-maily, adresy, organizace, narozeniny, poznámky a fotka.',
  'product.contacts.f3': 'Štítky a oblíbené pro seskupení a hledání.',
  'product.contacts.f4': 'Rozpoznání a sloučení duplicit bez ztráty dat.',
  'product.contacts.f5': 'Import a export vCard a CSV; export jednoho kontaktu.',
  'product.contacts.f6': 'Hromadné akce a koš s obnovením.',
  'product.contacts.f7': 'Okamžité vyhledávání a automatické doplňování při psaní v Mail.',
  'product.contacts.f8': 'Funguje s jakýmkoli klientem CardDAV.',
  'product.contacts.f9': 'Blížící se narozeniny z adresáře, se zvýrazněním těch v tomto měsíci.',
  'product.contacts.f10':
    'Naplánování schůzky s kontaktem nebo zkopírování e-mailu kontaktu bez opuštění karty.',
  'product.notes.f1': 'Vnořené sešity s ikonou a barvou; tři rozvržení: volné, sloupce a seznam.',
  'product.notes.f2':
    'Dvanáct typů poznámek: text, úkoly, odkazy, barva, obrázek a odkazy na Drive, Docus, Calendar, Rooms a další poznámky.',
  'product.notes.f3':
    'Editor s formátováním (tučné písmo, seznamy, odkazy), který ukládá při odchodu.',
  'product.notes.f4':
    'Štítky, připnutí, duplikování, přesun mezi sešity, seskupování a přetahování na plátně.',
  'product.notes.f5':
    'Viditelnost pro každý sešit: soukromý, osobní, organizace nebo veřejný odkaz.',
  'product.notes.f6': 'Šifrování v prohlížeči pro citlivé sešity: Sytue je nemůže číst.',
  'product.notes.f7': 'Rozhodujete, které sešity se dostanou do vyhledávání v sadě.',
  'product.notes.f8': 'Zpětné odkazy: které poznámky odkazují na tuto.',
  'product.notes.f9': 'Export celého sešitu do HTML, PDF, Markdown a dalších; import kopií.',
  'product.notes.f10':
    'Sdílení poznámky do kanálu Rooms; přetažením nebo vložením vznikají poznámky.',
  'product.notes.f11': 'Tabulky v editoru a zkratky Markdown při psaní: „- “, „# “, „> “ a „```“.',
  'product.notes.f12':
    'Přesun poznámky do jiného sešitu, export jedné poznámky a řazení sešitu podle potřeby.',
  'product.photos.tagline': 'Vaše fotky, v pořádku a podle vašeho vkusu, bez opuštění Evropy.',
  'product.photos.standard': 'Originály v Sytue Drive; editační jádro ve vašem prohlížeči',
  'product.photos.p1': 'Časová osa po měsících s hledáním podle názvu.',
  'product.photos.p2': 'Alba s obálkou; prohlížeč na celou obrazovku.',
  'product.photos.p3': 'Editor světla, barev, otočení, zrcadlení a ořezu v reálném čase.',
  'product.photos.p4': 'Každá úprava je recept: originál zůstává nedotčený.',
  'product.photos.f1':
    'Časová osa seskupená po měsících, s hledáním podle názvu a ostrými miniaturami.',
  'product.photos.f2':
    'Alba: vytvoření, přejmenování, výběr obálky, přidání z prohlížeče a odebrání.',
  'product.photos.f3': 'Prohlížeč na celou obrazovku s ovládáním klávesnicí.',
  'product.photos.f4':
    'Editor se čtyřmi posuvníky (expozice, kontrast, sytost, teplota), otočením a zrcadlením.',
  'product.photos.f5':
    'Ořez přetažením a klávesové zkratky; náhled na GPU se záložním výpočtem na CPU.',
  'product.photos.f6': 'Každá úprava je recept uložený zvlášť: originál v Drive se nikdy nemění.',
  'product.photos.f7':
    'Export v plném rozlišení, vykreslený na serveru stejnými vzorci jako náhled.',
  'product.photos.f8': 'Koš s obnovením.',
  'product.photos.f9':
    'Originály jsou ve vašem Sytue Drive; odvozené verze jsou obnovitelná mezipaměť.',
  'product.photos.f10': 'Údaje EXIF čtené přímo ze souboru; vše v Evropské unii.',
  'product.photos.f11':
    'Vícenásobný výběr po měsících: do nového alba, do koše nebo stažení najednou.',
  'product.photos.f12':
    'Prezentace, plná velikost, otočení z prohlížeče a ořez s pevným poměrem stran.',
  'product.rooms.f1': 'Veřejné a soukromé kanály v kategoriích; přímé zprávy a skupiny.',
  'product.rooms.f2': 'Vlákna, reakce, zmínky, připnuté a uložené zprávy a koncepty.',
  'product.rooms.f3':
    'Formátování s lištou „Aa“ a zkratkami: tučné písmo, kurzíva, kód, citace, seznamy a odkazy.',
  'product.rooms.f4': 'Přílohy, obrázky, GIFy a vlastní emoji; soubory z Drive.',
  'product.rooms.f5': 'Plánované odeslání, priorita, potvrzení přečtení a „zobrazeno“.',
  'product.rooms.f6': 'Příkazy s lomítkem a karty: ankety a přiřaditelné úkoly.',
  'product.rooms.f7': 'Videohovory v kanálu: kamera, obrazovka, zvednutá ruka, chat a nahrávání.',
  'product.rooms.f8': 'Plánované schůzky a odkazy s pozvánkou.',
  'product.rooms.f9': 'Boti, webhooky a integrace s dalšími nástroji.',
  'product.rooms.f10': 'Přítomnost a vlastní stav; nastavení oznámení pro každý kanál.',
  'product.rooms.f11': 'Vyhledávání v historii, i z celé sady.',
  'product.rooms.f12': 'Reálný čas přes WebSocket, s opětovným připojením a bez ztracených zpráv.',
  'product.rooms.f13':
    'Rychlý přepínač kanálů přes Ctrl+G, označení všeho jako přečteného a zkratky přes „?“.',
  'product.cast.tagline':
    'Vaše videa a vaše živé přenosy, na vyžádání i v reálném čase, aniž byste opustili Evropu.',
  'product.cast.standard':
    'Přehrávání HLS v prohlížeči; příjem RTMP z libovolného vysílacího programu',
  'product.cast.p1': 'Nahrajte video a překóduje se do několika kvalit pro jakoukoli obrazovku.',
  'product.cast.p2': 'Vysílejte živě z vysílacího programu, z prohlížeče nebo z aplikace.',
  'product.cast.p3':
    'Čtyři úrovně viditelnosti: veřejné, nezařazené, organizace nebo konkrétní osoby.',
  'product.cast.p4': 'Živý chat a komentáře se základní moderací od osoby, která vysílá.',
  'product.cast.f1': 'Nahrávejte videa do 8 GiB; originál se zachová.',
  'product.cast.f2': 'Adaptivní přehrávání ve třech kvalitách; prohlížeč zvolí tu nejlepší.',
  'product.cast.f3':
    'Živé vysílání z vysílacího programu přes RTMP, s vysílacím klíčem pro každou osobu.',
  'product.cast.f4':
    'Živě z prohlížeče nebo aplikace: kamera a mikrofon. Sdílení obrazovky v podporovaných prohlížečích, počítačových systémech a Androidu; v aplikaci pro iOS/iPadOS není dostupné.',
  'product.cast.f5': 'Záznamy živých přenosů se automaticky stávají videi na vyžádání.',
  'product.cast.f6': 'Veřejná úvodní stránka a kanál pro každou organizaci, indexovatelné.',
  'product.cast.f7':
    'Nezařazené odkazy, videa jen pro organizaci nebo přístup pro konkrétní osoby podle @uživatelského jména.',
  'product.cast.f8':
    'Živý chat a komentáře ve vláknech, moderované osobou, která zveřejňuje, a správou organizace.',
  'product.cast.f9': 'Vložitelný přehrávač a oEmbed pro veřejná a nezařazená videa.',
  'product.cast.f10': 'Úložiště se počítá do vaší kvóty v Sytue Drive; vše v Evropské unii.',
  'product.cast.f11':
    '„Nahrát do Cast“ ze Sytue Drive: kopie videa se přenese do Cast a soubor zůstane v Drive.',
  'product.pages.tagline':
    'Web vaší firmy a jeho obchod, navržené ve vizuálním editoru a publikované, aniž by opustily Evropu.',
  'product.pages.standard':
    'Statické přístupné HTML a CSS bez cookies třetích stran; export do Astro',
  'product.pages.p1':
    'Vizuální editor s rámy, které se samy přizpůsobí počítači, tabletu i mobilu.',
  'product.pages.p2':
    'Deset šablon podle oboru, se skutečnými texty a fotografiemi z public domain.',
  'product.pages.p3': 'Obchod s katalogem, košíkem a platbou na účet vaší firmy.',
  'product.pages.p4': 'Vlastní doména s automatickým certifikátem, nebo adresa na sites.sytue.eu.',
  'product.pages.f1':
    'Vizuální editor s rámy, automatickým rozvržením, opakovaně použitelnými komponentami a hotovými sekcemi.',
  'product.pages.f2': 'Tři zlomové body (počítač, tablet a mobil), každý s vlastním nastavením.',
  'product.pages.f3': 'Analytika návštěvnosti bez cookies, přímo v panelu webu.',
  'product.pages.f4':
    'Vícejazyčné weby s adresami pro každý jazyk, hreflang a překladem s pomocí Sytue AI.',
  'product.pages.f5':
    'Obrázky z vašeho počítače, ze Sytue Drive nebo ze Sytue Photos, s alternativním textem, na který se ptáme při vkládání.',
  'product.pages.f6':
    'Obchod s variantami, zásobami, DPH podle produktu a platbou přes Mollie, Adyen, Redsys nebo Stripe.',
  'product.pages.f7':
    'Objednávky se zjednodušenou fakturou vystavenou Sytue ERP a odstoupením od smlouvy přímo z webu.',
  'product.pages.f8':
    'Formuláře se souhlasem, schránka zpráv a upozornění pro lidi ve vaší organizaci.',
  'product.pages.f9':
    'Pojmenované verze, náhled za tajným odkazem a návrat k jakémukoli dřívějšímu zveřejnění.',
  'product.pages.f10':
    'Export kódu do Astro, Reactu, Next.js a nativních aplikací a katalogu do Shopify, WooCommerce a dalších platforem.',
  'product.bases.tagline':
    'Propojené tabulky pro vedení zákazníků, objednávek nebo zásob, s oprávněními a záznamem každé změny.',
  'product.bases.standard': 'Otevřené formáty CSV a JSON; schéma a vazby hlídá sama databáze',
  'product.bases.p1':
    'Zákazníci, objednávky nebo zásoby v propojených tabulkách, bez znalosti databází.',
  'product.bases.p2': 'Sdílejte se svou organizací s rolí, kterou zvolíte.',
  'product.bases.p3': 'Každá změna se zaznamená: kdo ji provedl a jaká byla předchozí hodnota.',
  'product.bases.p4': 'Vaše data zůstávají v EU a exportujete je, kdy chcete.',
  'product.bases.f1':
    'Tabulky s poli pro text, číslo, datum, zaškrtávací pole, jednu volbu i více voleb.',
  'product.bases.f2':
    'Vazby mezi tabulkami: propojte objednávku se zákazníkem a databáze nedovolí vazbu přetrhnout.',
  'product.bases.f3':
    'Upravitelná mřížka ovládaná klávesnicí, čitelná pro čtečky obrazovky a použitelná na telefonu.',
  'product.bases.f4':
    'Oprávnění pro databázi i pro tabulku: vlastník, editor, komentující a čtenář.',
  'product.bases.f5': 'Importujte soubor CSV jako novou tabulku a exportujte do CSV nebo JSON.',
  'product.bases.f6': 'Záznam každé změny: kdo, který záznam a pole, před a po.',
  'product.cloud.f12': 'Spotřeba, náklady a evropská fakturace v jedné konzoli.',
  'product.cloud.f11': 'Lidé, tokeny API a exportovatelný auditní protokol.',
  'product.cloud.f10': 'Webová analytika šetrná k soukromí, bez cookies.',
  'product.cloud.f9': 'Plánované úlohy a šablony projektů.',
  'product.cloud.f8': 'Terminál do kontejneru; restart nebo pozastavení služby.',
  'product.cloud.f7': 'Živé protokoly, metriky a monitorování s upozorněními.',
  'product.cloud.f6': 'Proměnné prostředí a tajné klíče pro každé prostředí.',
  'product.cloud.f5': 'Vlastní domény s automatickým TLS; přesměrování a hlavičky.',
  'product.cloud.f4': 'Úložiště S3 a spravované funkce.',
  'product.cloud.f3': 'Databáze Postgres s automatickým REST API a editorem SQL.',
  'product.cloud.f2': 'Náhled pro každý PR a návrat k libovolnému nasazení jedním kliknutím.',
  'product.cloud.f1':
    'Projekty z vašeho repozitáře (GitHub, GitLab nebo vlastní Git) s nasazením při každém push.',
  'product.console.f1': 'Přehled se stavem organizace a tím, co si dnes zaslouží pozornost.',
  'product.console.f2': 'Účty: nástup, pozastavení, reset hesla a MFA, relace, aliasy a schránka.',
  'product.console.f3': 'Statické skupiny a dynamické skupiny podle pravidel; role s oprávněními.',
  'product.console.f4': 'Privilegovaný přístup (PIM): časově omezená aktivace se schválením.',
  'product.console.f5':
    'SSO s poskytovateli SAML nebo OIDC, s testem připojení; zřizování přes SCIM.',
  'product.console.f6': 'Pravidla podmíněného přístupu a přehled důvěryhodných zařízení.',
  'product.console.f7':
    'Integrace: klienti OIDC, SAML a připojení třetích stran; náhrada produktů.',
  'product.console.f8': 'Převod obsahu: migrace od jiných poskytovatelů.',
  'product.console.f9': 'Editor dokumentů a spravovatelné kanály Rooms.',
  'product.console.f10': 'Správa: právní blokace, štítky citlivosti a zásady hesel.',
  'product.console.f11':
    'Auditní protokol s exportem do CSV a přeposíláním do vašeho SIEM; protokoly služeb.',
  'product.console.f12': 'Vlastní doména a značka: logo organizace a poštovní doména.',
  'product.id.f1':
    'Jednotné přihlášení (OpenID Connect a OAuth 2.0 s PKCE) pro sadu i vaše aplikace.',
  'product.id.f2': 'Přístupové klíče (WebAuthn) a MFA přes TOTP s kódy pro obnovení.',
  'product.id.f3': 'Hesla pro aplikace pro IMAP a klienty, kteří neumí OIDC.',
  'product.id.f4': 'Relace a zařízení: odvolání jedné nebo všech ostatních.',
  'product.id.f5': 'Federace SAML s vaším IdP a Sytue jako SAML IdP pro další aplikace.',
  'product.id.f6': 'Zřizování přes SCIM z vašeho adresáře.',
  'product.id.f7': 'Pozvánky, ověření e-mailu a obnovení hesla.',
  'product.id.f8': 'Profil pro iPhone a Mac (.mobileconfig) jedním klepnutím.',
  'product.id.f9': 'Zásady hesel a pravidla přístupu stanovená organizací.',
  'product.id.f10': 'Dynamická registrace klientů (DCR) a standardní zjišťování.',
  'product.erp.f1': 'Dvacet čtyři modulů a třicet šest modelů v jediné službě a jediné databázi.',
  'product.erp.f2':
    'Katalog modelů jsou data: každý modul deklaruje svá pole, stavy a sloupce seznamu.',
  'product.erp.f3':
    'Nový modul je jeden soubor JSON — bez DDL, bez migrace a bez zamykání tabulek.',
  'product.erp.f4':
    'Jedno API pro všech třicet šest modelů a jedno rozhraní generované z katalogu.',
  'product.erp.f5':
    'Podvojné účetnictví s částkami v celých centech; ani jedno číslo v plovoucí čárce.',
  'product.erp.f6': 'Vyrovnanost kontroluje databáze při potvrzení transakce, ne kód aplikace.',
  'product.erp.f7':
    'Uzavřené zápisy jsou neměnné: nelze je upravit, smazat ani do nich přidat řádky.',
  'product.erp.f8':
    'Jeden doklad, jeden zápis: opakovaný pokus ani dvě otevřené karty nemohou zdvojit výnos.',
  'product.erp.f9':
    'Společné popisky jsou ve všech 26 jazycích a nepřeložené pole nedovolí službě spustit se.',
  'product.erp.f10':
    'Výpisy nad indexy: první stránka z dvaceti tisíc řádků se vrátí za méně než milisekundu.',
  'meta.downloads.title': 'Ke stažení — Sytue One pro počítač a mobil',
  'meta.downloads.description':
    'Nativní aplikace Sytue One pro macOS, Windows, Linux, iOS a Android. Celá sada ve vašem zařízení, s integracemi každého systému.',
  'downloads.overline': 'Aplikace',
  'downloads.h1': 'Sytue One ve vašem zařízení',
  'downloads.intro':
    'Skutečné aplikace pro počítač a mobil: nainstalují se, sídlí v Docku nebo v nabídce Start, upozorňují systémovými oznámeními a otevírají soubory z vašeho zařízení.',
  'downloads.desktop.title': 'Počítač',
  'downloads.desktop.body':
    'Jedna nativní aplikace na každý produkt, která si kreslí vlastní rozhraní pomocí grafické karty: bez prohlížeče uvnitř, takže startuje rychle a spotřebuje jako systémová aplikace.',
  'downloads.apps.title': 'Jen to, co potřebujete',
  'downloads.apps.body':
    'Sytue One pokryje celý pracovní den. Pokud chcete jen disk, Sytue Drive se nainstaluje samostatně.',
  'downloads.includes': 'Obsahuje:',
  'downloads.mobile.title': 'Mobil',
  'downloads.mobile.body':
    'Nativní aplikace pro iOS a Android se systémovými oznámeními, sdílením a přístupem k souborům a fotoaparátu.',
  'downloads.selfhost.title': 'Je vaše organizace na vlastní doméně?',
  'downloads.selfhost.body':
    'Nativní aplikace se připojují k sytue.eu. Pokud má vaše organizace vlastní instalaci, přihlaste se v prohlížeči na její doméně: sada funguje na webu stejně.',
  'downloads.yourSystem': 'Váš systém',
  'downloads.signInToGet': 'Přihlaste se ke stažení',
  'downloads.store': 'Zobrazit v {format}',
  'downloads.soon': 'Brzy v {format}',
  'downloads.browser.text':
    'Dáváte přednost prohlížeči? Celá sada funguje na webu stejně, bez instalace:',
  'downloads.browser.link': 'otevřít Sytue One',
  'platform.macos.req': 'macOS 11 nebo novější · Apple Silicon a Intel',
  'platform.windows.req': 'Windows 10 nebo novější · 64 bitů',
  'platform.linux.req': 'OpenGL 2.1 · 64 bitů',
  'platform.ios.req': 'iOS 14 nebo novější',
  'platform.android.req': 'Android 6.0 nebo novější',
  'footer.tagline':
    'Každá aplikace sady je naše vlastní služba postavená na otevřených, zveřejněných protokolech.',
  'footer.products': 'Produkty',
  'footer.resources': 'Zdroje',
  'footer.legal': 'Právní informace',
  'footer.suite': 'Sada',
  'footer.downloads': 'Stáhnout aplikace',
  'footer.sales': 'Kontaktovat obchod',
  'footer.privacy': 'Soukromí a GDPR',
  'footer.accessibility': 'Přístupnost',
  'footer.openSource': 'Otevřený zdrojový kód',
  'footer.legalNotice': 'Právní upozornění',
  'footer.terms': 'Podmínky',
  'footer.copyright':
    '© {year} Sytue · Data hostovaná v EU (Scaleway fr-par / nl-ams · vlastní zóna v Madridu).',
  'footer.privacyNotice':
    'Vaše údaje neprodáváme ani je nepředáváme třetím stranám. Nepoužíváme je k profilování, k reklamě ani k trénování či vylepšování našich služeb nebo služeb kohokoli jiného. Jsou šifrované při přenosu i v klidu a naši evropskou infrastrukturu opouštějí jen v nezbytně nutném rozsahu kvůli fakturaci, kvůli oznámením při zavřené aplikaci (služba push vidí jen vaše zařízení a čas, nikdy obsah) a v integracích, které si zapnete — v nich vás třetí strana požádá o souhlas s údaji, které použije.',
  'meta.notFound.title': 'Stránka nenalezena — Sytue One',
  'notFound.overline': 'Chyba 404',
  'notFound.title': 'Tuto stránku se nepodařilo najít',
  'notFound.body': 'Odkaz se mohl změnit. Vraťte se na úvod nebo prozkoumejte sadu.',
  'notFound.home': 'Zpět na úvod',
  'meta.legal.title': 'Právní upozornění — Sytue One',
  'meta.legal.description':
    'Identifikační údaje provozovatele www.sytue.eu a služby Sytue One: Sytue Projects, S.L.',
  'aviso.title': 'Právní upozornění',
  'aviso.intro':
    'V souladu s článkem 10 španělského zákona 34/2002 ze dne 11. července o službách informační společnosti a elektronickém obchodu (LSSI) uvádíme identifikační údaje provozovatele tohoto webu a služby Sytue One.',
  'aviso.review.body':
    'Rejstříkové údaje v tomto upozornění byly ověřeny v obchodním rejstříku. Zbytek textu prochází právní revizí a jeho znění se může změnit, aniž by se změnilo to, co popisuje.',
  'aviso.owner.title': 'Provozovatel',
  'aviso.owner.name': 'Obchodní firma',
  'aviso.owner.form': 'Právní forma',
  'aviso.owner.formValue':
    'Jednočlenná společnost s ručením omezeným (sociedad limitada unipersonal) podle španělského práva',
  'aviso.owner.nif': 'Daňové identifikační číslo (NIF)',
  'aviso.owner.address': 'Sídlo',
  'aviso.owner.registry': 'Zápis v rejstříku',
  'aviso.owner.registryValue':
    'Obchodní rejstřík v Madridu (Registro Mercantil de Madrid), oddíl 8, list M-887153, zápis 1',
  'aviso.owner.email': 'E-mail',
  'aviso.owner.web': 'Web',
  'aviso.scope.title': 'Účel',
  'aviso.scope.body':
    'Tento web představuje sadu Sytue One a zpřístupňuje její aplikace. Používání služby se řídí Podmínkami služby a zpracování osobních údajů Zásadami ochrany osobních údajů; oba dokumenty jsou odkázány v zápatí každé stránky.',
  'aviso.ip.title': 'Duševní a průmyslové vlastnictví',
  'aviso.ip.body':
    'Obsah tohoto webu (texty, design, kód a ochranné známky a loga Sytue a Sytue One) patří společnosti Sytue Projects, S.L. nebo je užíván na základě licence. Názvy produktů třetích stran uvedené jako odkazy na kompatibilitu jsou ochrannými známkami jejich vlastníků.',
  'aviso.liability.title': 'Odpovědnost',
  'aviso.liability.body':
    'Společnost Sytue Projects, S.L. usiluje o to, aby byl tento web dostupný a aktuální, aniž by zaručovala absenci výpadků či chyb. Neodpovídá za obsah webů třetích stran, na které odkazuje.',
  'aviso.law.title': 'Rozhodné právo',
  'aviso.law.body':
    'Toto upozornění se řídí španělským právem. Spotřebitelům zůstávají práva a příslušnost soudů, které jim přiznává právo země jejich bydliště v Evropské unii.',
  'aviso.updated': 'Poslední aktualizace: září 2026.',
  'meta.accessibility.title': 'Prohlášení o přístupnosti — Sytue One',
  'meta.accessibility.description':
    'Jak přístupné jsou sada Sytue One a tento web, co měříme, co zatím nestačí a jak nám dát vědět.',
  'a11y.title': 'Prohlášení o přístupnosti',
  'a11y.intro':
    'Sytue chce, aby Sytue One a tento web mohl používat každý, bez ohledu na asistivní technologie. Toto prohlášení se vztahuje na www.sytue.eu a webové aplikace sady.',
  'a11y.standard.title': 'Standard, podle kterého pracujeme',
  'a11y.standard.body':
    'Navrhujeme a vyvíjíme podle WCAG 2.1 na úrovni AA, na kterou odkazují evropská norma EN 301 549 a evropský akt o přístupnosti (směrnice (EU) 2019/882).',
  'a11y.status.title': 'Stav souladu',
  'a11y.status.body':
    'Částečný soulad: většina sady standard splňuje a níže uvedené části jej zatím nesplňují zcela. Nezávislý audit zatím nebyl zadán.',
  'a11y.measures.title': 'Co pro to děláme',
  'a11y.measures.design':
    'Jeden designový systém pro celou sadu: barvy textu s ověřeným kontrastem, viditelný fokus na každém ovládacím prvku a ovládání nabídek, dialogů a seznamů klávesnicí.',
  'a11y.measures.ci':
    'Automatické audity přístupnosti (Lighthouse) běží v integrační pipeline při každé změně veřejného webu.',
  'a11y.measures.system':
    'Rozhraní respektuje nastavení systému: tmavý nebo světlý motiv, omezený pohyb a velikost textu, bez pevných velikostí v pixelech ve formulářových polích.',
  'a11y.measures.text':
    'Každý obrázek, ikona i prázdný stav má textovou alternativu a každá stránka deklaruje svůj jazyk, aby jej čtečky obrazovky vyslovovaly správně.',
  'a11y.limits.title': 'Známá omezení',
  'a11y.limits.body':
    'Mřížka tabulek, plátno snímků a anotace PDF jsou složité editory, které ještě vyžadují práci, aby byly plně ovladatelné klávesnicí a čtečkou obrazovky. Dokumenty a soubory, které lidé nahrají, se zobrazují tak, jak jsou, a neupravujeme je.',
  'a11y.feedback.title': 'Dejte nám vědět, co nefunguje',
  'a11y.feedback.body':
    'Pokud je pro vás něco v sadě nebo na tomto webu obtížně použitelné nebo nepoužitelné, napište nám; prověříme to a ozveme se:',
  'a11y.enforcement.title': 'Pokud odpověď nestačí',
  'a11y.enforcement.body':
    'Pokud po nahlášení překážky naše odpověď nebude uspokojivá, můžete se obrátit na orgán odpovědný za přístupnost ve vaší zemi podle evropského aktu o přístupnosti.',
  'a11y.updated':
    'Toto prohlášení vzniklo v září 2026 a reviduje se, když se sada podstatně změní.',
  'meta.openSource.title': 'Otevřený zdrojový kód — Sytue One',
  'meta.openSource.description':
    'Sytue staví téměř vše od nuly, ale znovu nevymýšlí to, co už dobře funguje. Open source projekty, na kterých balík staví.',
  'oss.title': 'Otevřený zdrojový kód',
  'oss.intro':
    'Sytue staví téměř vše od nuly: poštu, editory, kalendář. Ale znovu nevymýšlí to, co už dobře funguje. Toto jsou open source projekty, na kterých staví některé části balíku.',
  'oss.livekit.body': 'Zvuk a video v reálném čase pro hovory v Sytue Rooms běží na LiveKit.',
  'oss.postgres.body':
    'Každá služba balíku ukládá svá data do PostgreSQL, jedna databáze na organizaci.',
  'meta.privacy.title': 'Soukromí a GDPR — Sytue One',
  'meta.privacy.description':
    'Jak Sytue One zpracovává osobní údaje: správce, údaje, právní základy, uchovávání, zpracovatelé, zabezpečení, mezinárodní předávání a vaše práva podle GDPR.',
  'legal.overline': 'Právní informace',
  'legal.updated': 'Poslední aktualizace: červenec 2026.',
  'legal.review.title': 'Prochází právní revizí.',
  'privacy.title': 'Soukromí a GDPR',
  'privacy.review.body':
    'Tyto zásady přesně popisují, jak údaje zpracováváme dnes. Konkrétní doby uchování doplní konečná verze, kterou zreviduje náš právní tým.',
  'privacy.controller.title': 'Správce údajů',
  'privacy.controller.body':
    'Sytue Projects, S.L. (španělské DIČ B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Španělsko; zapsaná v obchodním rejstříku v Madridu, oddíl 8, list M-887153) je jako poskytovatel sady Sytue One správcem údajů účtů a organizací služby. Kontakt pro ochranu údajů:',
  'privacy.data.title': 'Jaké údaje zpracováváme',
  'privacy.data.account.label': 'Údaje o účtu:',
  'privacy.data.account.body':
    'jméno, e-mailová adresa a organizace, spravované službou Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Obsah vaší organizace:',
  'privacy.data.content.body':
    'pošta, soubory, dokumenty, kalendář a zprávy, které vaše organizace v sadě hostuje. Tento obsah patří vaší organizaci.',
  'privacy.data.technical.label': 'Technické provozní údaje:',
  'privacy.data.technical.body':
    'přístupové protokoly a údaje o připojení nezbytné pro zabezpečení, prevenci zneužití a provoz služby.',
  'privacy.data.note':
    'Data neprodáváme ani je nepoužíváme k reklamě. Infrastruktura a úložiště spravované společností Sytue se nacházejí v Evropské unii (Scaleway, regiony fr-par a nl-ams a vlastní infrastruktura v Madridu), se šifrováním při přenosu (TLS 1.3) i při uložení. Připojení k externím službám odesílají vámi schválená data jejich poskytovateli, podle jeho umístění a podmínek.',
  'privacy.basis.title': 'Právní základ',
  'privacy.basis.body':
    'Zpracování se opírá o právní základy podle článku 6 GDPR: plnění smlouvy o poskytování služby s vaší organizací, náš oprávněný zájem na zabezpečení a prevenci zneužití, plnění právních povinností a tam, kde je o to výslovně požádáno, váš souhlas. Podrobnosti podle jednotlivých základů a činností zpracování budou součástí konečného textu.',
  'privacy.retention.title': 'Uchovávání',
  'privacy.retention.body':
    'Údaje o účtu a obsah uchováváme, dokud má vaše organizace službu aktivní. Po zrušení účtu nebo organizace se údaje v přiměřené lhůtě smažou nebo anonymizují, s výjimkou toho, co musíme uchovávat ze zákona (například fakturace). Technické provozní protokoly se pravidelně mažou. Konkrétní lhůty podle kategorií uvede konečná verze. Hlášení obsahu uchováváme, dokud jsou otevřená, a mažeme je dvanáct měsíců po jejich vyřízení.',
  'privacy.processors.title': 'Zpracovatelé a další zpracovatelé',
  'privacy.processors.body':
    'Poskytovatelem naší infrastruktury je Scaleway (regiony fr-par a nl-ams), který jedná jako zpracovatel a působí výhradně v Evropské unii; kromě toho provozujeme vlastní zónu v Madridu (Španělsko) ve vlastních prostorách. Pro platby předplatného využíváme Mollie B.V. (Nizozemsko), která zpracovává pouze údaje nezbytné pro platbu. Nevyužíváme žádné další zpracovatele mimo EHP. Seznam dalších zpracovatelů bude zveřejněn a aktualizován, s předchozím oznámením každého doplnění.',
  'privacy.security.title': 'Zabezpečení',
  'privacy.security.body':
    'Uplatňujeme technická a organizační opatření v souladu s GDPR: šifrování při přenosu (TLS 1.3) i v klidu, oddělení organizací (multi-tenant se zabezpečením na úrovni řádků), silné ověřování (MFA/WebAuthn), protokolování přístupů a minimální oprávnění. Žádný systém není neomylný: pokud by se vás týkalo porušení zabezpečení, oznámíme vám to ve lhůtách stanovených nařízením.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'Používáme pouze technické cookies nezbytné pro jednotné přihlášení a zabezpečení relace. Nepoužíváme reklamní ani sledovací cookies třetích stran, takže není potřeba žádná souhlasová lišta pro sledování.',
  'privacy.transfers.title': 'Mezinárodní předávání',
  'privacy.transfers.body':
    'Zpracování a hostování spravované společností Sytue probíhá v regionech Evropské unie. Externí připojení povolená uživatelem odesílají schválená data jejich poskytovateli; jejich umístění závisí na poskytovateli a jeho konfiguraci. Oznámení odeslaná při zavřené aplikaci nebo prohlížeči mohou také využívat externí služby. Služba push zařízení (Google, Mozilla nebo Apple, podle prohlížeče nebo telefonu) vidí anonymní identifikátor vašeho zařízení a čas každého oznámení — nikdy obsah, který putuje koncově šifrovaný a v iOS se odesílá prázdný. Oznámení můžete vypnout a pak neodchází ani to.',
  'privacy.rights.title': 'Vaše práva',
  'privacy.rights.body1':
    'GDPR vám přiznává právo na přístup, opravu, výmaz, vznesení námitky, omezení zpracování a přenositelnost. Uplatnit je můžete na adrese',
  'privacy.rights.body2':
    '; odpovídáme ve lhůtách stanovených nařízením. Máte také právo podat stížnost u',
  'privacy.rights.authority': 'vašeho dozorového úřadu',
  'privacy.dpo.title': 'Pověřenec pro ochranu osobních údajů (DPO)',
  'privacy.dpo.body': 'S jakýmkoli dotazem k ochraně osobních údajů nebo pro kontakt s DPO:',
  'privacy.minors.title': 'Nezletilí',
  'privacy.minors.body':
    'Sytue One je služba pro organizace a profesní použití; není určena nezletilým a údaje nezletilých vědomě neshromažďujeme.',
  'privacy.changes.title': 'Změny těchto zásad',
  'privacy.changes.body':
    'Pokud tyto zásady aktualizujeme, změníme datum „poslední aktualizace“ a u podstatných změn vás budeme informovat prostřednictvím kanálů služby ještě před jejich účinností.',
  'meta.terms.title': 'Podmínky služby — Sytue One',
  'meta.terms.description':
    'Podmínky služby Sytue One: poskytovatel, rozsah, účty a organizace, přijatelné použití, dostupnost, vaše data, duševní vlastnictví a rozhodné právo.',
  'terms.title': 'Podmínky služby',
  'terms.review.body':
    'Tyto podmínky popisují službu a její používání přesně tak, jak dnes funguje. Konkrétní smluvní ujednání (úroveň služeb, odpovědnost a příslušnost soudů) doplní konečná verze, kterou zreviduje náš právní tým.',
  'terms.provider.title': 'Kdo službu poskytuje',
  'terms.provider.body':
    'Sytue One poskytuje Sytue Projects, S.L. (španělské DIČ B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Španělsko; zapsaná v obchodním rejstříku v Madridu, oddíl 8, list M-887153) z infrastruktury umístěné výhradně v Evropské unii (Scaleway, regiony fr-par a nl-ams, a vlastní zóna v Madridu); služba se řídí právem EU.',
  'terms.scope.title': 'Rozsah služby',
  'terms.scope.body':
    'Sytue One je sada pro produktivitu organizací: identita (Sytue ID), pošta, soubory, dokumenty, kalendář, kontakty a zprávy s videohovory. Služba se sjednává pro každou organizaci.',
  'terms.accounts.title': 'Účty a organizace',
  'terms.accounts.body':
    'Každá organizace spravuje své účty, skupiny a oprávnění prostřednictvím Sytue ID (SSO + MFA). Správa organizace odpovídá za přidávání a odebírání účtů a za způsob, jakým se služba v rámci těchto účtů používá.',
  'terms.use.title': 'Přijatelné použití',
  'terms.use.body':
    'Zavazujete se používat službu v souladu se zákonem a nepoužívat ji k nezákonné činnosti, spamu, šíření malwaru, porušování práv třetích stran ani k žádnému použití, které ohrožuje zabezpečení nebo dostupnost služby pro jiné organizace.',
  'terms.availability.title': 'Dostupnost a podpora',
  'terms.availability.body1':
    'Usilujeme o vysokou dostupnost a průběžně provádíme údržbu a vylepšení. Konkrétní závazky úrovně služeb (SLA), pokud se na váš tarif vztahují, jsou uvedeny ve smlouvě o poskytování služby. Podpora probíhá prostřednictvím',
  'terms.data.title': 'Vaše data',
  'terms.data.body1':
    'Obsah, který vaše organizace v sadě hostuje, patří vaší organizaci. Sada stojí na otevřených standardech (OIDC, JMAP, CalDAV, CardDAV, WebDAV), takže vaše data zůstávají přenositelná a můžete si je odnést. Zpracování osobních údajů popisuje stránka',
  'terms.ip.title': 'Duševní vlastnictví',
  'terms.ip.body':
    'Software Sytue One je vlastní dílo společnosti Sytue a je chráněn příslušnými právy duševního vlastnictví. Vlastnictví obsahu, který vaše organizace v sadě hostuje, zůstává vám; zpracováváme jej pouze za účelem poskytování služby.',
  'terms.liability.title': 'Odpovědnost',
  'terms.liability.body':
    'Vynakládáme veškeré úsilí na spolehlivou a bezpečnou službu. Příslušné záruky a omezení odpovědnosti jsou uvedeny ve smlouvě o poskytování služby; nic v těchto podmínkách nevylučuje odpovědnost, jejíž vyloučení zákon nepřipouští.',
  'terms.contact.title': 'Sjednání a kontakt',
  'terms.contact.body1':
    'Ohledně smluvních podmínek, ceny pro organizaci nebo jakéhokoli dotazu ke službě pište na',
  'terms.changes.title': 'Změny',
  'terms.changes.body':
    'Tyto podmínky můžeme aktualizovat, aby odrážely změny služby nebo právních předpisů. U podstatných změn vás budeme informovat prostřednictvím kanálů služby ještě před jejich účinností a aktualizujeme datum „poslední aktualizace“.',
  'terms.law.title': 'Rozhodné právo',
  'terms.law.body':
    'Služba se řídí právem Evropské unie a právem země poskytovatele. Konkrétní příslušnost soudů pro řešení sporů stanoví konečný text. Bez ohledu na to vám zůstává právo podat stížnost u dozorového úřadu pro ochranu osobních údajů.',
  'seo.org.description':
    'Sytue One: suverénní evropská sada pro produktivitu. Identita, pošta, soubory, dokumenty, kalendář, chat, videohovory a PaaS, 100% v EU.',
  'seo.org.slogan': 'Vaše produktivita, suverénní a evropská.',
  'seo.software.description':
    'Suverénní evropská sada pro produktivitu: pošta (JMAP), soubory (WebDAV), společné dokumenty, kalendář (CalDAV), kontakty (CardDAV), chat, videohovory a PaaS. SSO se Sytue ID.',
  'seo.software.f1': 'Jednotné přihlášení (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Pošta, soubory, dokumenty, kalendář, kontakty, chat a videohovory',
  'seo.software.f3': 'Platforma pro nasazení a spravovaná data',
  'seo.software.f4': 'Data 100% v EU (Scaleway)',
  'seo.offer.description': 'Zkušební verze zdarma, bez platební karty.',
  'seo.downloads.description':
    'Nativní aplikace sady Sytue One pro počítač (macOS, Windows, Linux) a mobil (iOS, Android).',
  'home.pricing.personal.title': 'Zdarma',
  'home.pricing.personal.freeStorage': '5 GB pro jednu osobu',
  'home.pricing.personal.freeBody':
    'Pošta, soubory, fotky, dokumenty, kalendář, kontakty, poznámky a místnosti. Bez platební karty a bez časového omezení.',
  'home.pricing.personal.note':
    'Vaší organizací je váš e-mail: bez správy organizace a bez ERP, a vše ostatní včetně Sytue Admin pro vaše integrace a migrace.',
  'home.pricing.personal.cta': 'Vytvořit bezplatný účet',
  'grid.orgOnly': 'Jen pro organizace',
  'grid.enDesarrollo': 'Ve vývoji',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB na uživatele',
  'home.pricing.paidBody':
    'Pro organizace i pro jednotlivce. Celá sada, se Sytue ERP a Sytue Admin pro řízení vaší organizace.',
  'home.pricing.thirdParty':
    'Každý tarif umožňuje připojit vlastní úložiště: Google Drive, Microsoft OneDrive, bucket S3 (Scaleway, OVH, Hetzner, Wasabi, MinIO), server WebDAV jako Nextcloud nebo Internxt, nebo váš NAS ve vlastních prostorách — bez otevřených portů, přes tunel Sytue. Zálohy a obnova dat uložených v připojených úložištích jsou odpovědností zákazníka (vás) a společnost Sytue® je zproštěna jakékoli odpovědnosti za ztrátu dat v úložištích připojených k Sytue®.',
  'home.pricing.soon': 'Již brzy',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB na uživatele',
  'home.pricing.enterprise.support': 'S prémiovou podporou.',
  'home.pricing.education': 'Vzdělávací instituce: sleva 50 % na Pro.',
};
