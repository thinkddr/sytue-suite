/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const lt: AppCatalog<typeof en> = {
  'skip.content': 'Pereiti prie turinio',
  'nav.products': 'Produktai',
  'nav.sovereignty': 'Suverenumas',
  'nav.standards': 'Atvirieji standartai',
  'nav.pricing': 'Kainos',
  'footer.pricing': 'Kainos',
  'home.pricing.title': 'Trys planai, visas rinkinys',
  'home.pricing.body':
    'Visuose trijuose tie patys produktai: skiriasi tik saugyklos dydis vienam naudotojui. Jokių apkarpytų versijų.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'be PVM',
  'home.pricing.per': '/Naudotojui/Mėn.',
  'home.pricing.cta': 'Susisiekti su pardavimų skyriumi',
  'home.pricing.commitment.title': 'Įsipareigojimas visam laikui',
  'home.pricing.commitment.body':
    'Kaina vienam naudotojui kas ketverius metus padidėja 0,50 € ir nieko daugiau: visam laikui. Tai žinote nuo prisijungimo dienos, ir tai nesikeičia.',
  'home.pricing.included.title': 'Viena vienintelė versija',
  'home.pricing.included.body':
    'Jokių planų pagal funkcijas, jokių lygių, jokių apkarpytų versijų: visas rinkinys įskaičiuotas į kainą vienam naudotojui, su tomis pačiomis funkcijomis visiems.',
  'home.pricing.metered.title': 'Kas apmokestinama atskirai',
  'home.pricing.metered.body': 'Papildoma saugykla: pagal naudojimą arba blokais.',
  'home.pricing.storage.title': 'Saugykla bendra',
  'home.pricing.storage.body':
    'Kiekvieno asmens kvota pridedama prie organizacijos kvotos: nepanaudota kvota atitenka likusiai organizacijai. Be to, kvotą vienam naudotojui galima koreguoti organizacijos lygiu.',
  'nav.downloads': 'Atsisiuntimai',
  'nav.aria': 'Svetainės naršymas',
  'nav.ariaMobile': 'Mobilusis naršymas',
  'nav.home': 'Sytue One — Pradžia',
  'nav.openMenu': 'Atidaryti meniu',
  'nav.closeMenu': 'Uždaryti meniu',
  'theme.toLight': 'Perjungti į šviesų režimą',
  'theme.toDark': 'Perjungti į tamsų režimą',
  'lang.label': 'Kalba',
  'cta.start': 'Pradėti nemokamai',
  'cta.signIn': 'Prisijungti',
  'cta.seeSuite': 'Peržiūrėti rinkinį',
  'meta.home.title': 'Sytue One — suverenus Europos produktyvumo rinkinys',
  'meta.home.description':
    'Sytue One: suverenus Europos produktyvumo rinkinys. Tapatybė, paštas, failai, dokumentai, kalendorius ir pokalbiai, 100 % ES ir sukurta remiantis atviraisiais standartais.',
  'home.hero.lead': 'Jūsų produktyvumas,',
  'home.hero.accent': 'suverenus ir europietiškas',
  'home.hero.subtitle':
    'Paštas, failai, dokumentai, kalendorius, pokalbiai ir vaizdo skambučiai — sujungti į vieną programą ir talpinami tik ES.',
  'home.hero.note': 'Kortelės nereikia. Duomenys „Scaleway“ (ES). Pagalba perkeliant.',
  'home.privacy.title': 'Jūsų darbas priklauso jums. Jūsų privatumas taip pat.',
  'home.privacy.body': 'Neparduodame jūsų duomenų ir nenaudojame jų reklamos profiliams kurti.',
  'home.privacy.anonymity':
    '„Sytue Analytics“ saugo jūsų privatumą naudodama apibendrintą statistiką, be sekimo slapukų ir su pseudoniminiais lankytojų identifikatoriais, kurie keičiasi kasdien ir yra atskiri kiekvienai svetainei.',
  'home.privacy.account':
    'Norint naudotis programų rinkiniu, reikia paskyros duomenų ir prieigos žurnalų, kad galėtume teikti paslaugą ir užtikrinti jos saugumą.',
  'home.privacy.link': 'Kaip saugome jūsų privatumą',
  'home.stats.aria': 'Europos buvimas',
  'home.stats.regions': 'ES regionai (fr-par · nl-ams · nuosava zona Madride)',
  'home.stats.apps': 'Programos vienos tapatybės pagrindu',
  'home.stats.standards': 'Atvirieji standartai: OIDC, JMAP, CalDAV, CardDAV ir WebDAV',
  'home.products.title': 'Vienas rinkinys, viena patirtis',
  'home.products.subtitle':
    'Visi produktai turi tą patį dizainą, tapatybę ir apvalkalą. Jie atrodo ir veikia kaip viena programa.',
  'home.integration.title': 'Integracija, kokia turi būti rinkinyje',
  'home.integration.subtitle':
    'Viena registracija atveria viską. Automatinis naudotojų teikimas (SCIM) iš Sytue ID.',
  'home.integration.sso.title': 'Vienkartinis prisijungimas',
  'home.integration.sso.body': 'Viena tapatybė (Sytue ID) visam rinkiniui. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Bendras apvalkalas',
  'home.integration.shell.body':
    'Viršutinė juosta, programų perjungiklis, paieška ir bendri pranešimai kiekviename produkte.',
  'home.integration.cross.title': 'Veiksmai tarp programų',
  'home.integration.cross.body':
    'Pridėkite failą iš Drive programoje Mail, paverskite laišką Calendar įvykiu ir pasidalykite juo Rooms, nenutraukiant darbo.',
  'home.sovereignty.title': 'Tikras suverenumas',
  'home.sovereignty.body':
    'Tai ne rinkodaros pažadas, o architektūra. Infrastruktūra veikia tik Europos regionuose, o viskas sukurta remiantis atvirais, audituojamais protokolais be užpakalinių durų.',
  'home.sovereignty.link': 'Kodėl atvirieji standartai',
  'home.sovereignty.eu.title': '100 % ES',
  'home.sovereignty.eu.body':
    'Visa infrastruktūra ES: „Scaleway“ (fr-par / nl-ams) ir nuosava zona Madride. Asmens duomenims – jokių JAV paslaugų teikėjų.',
  'home.sovereignty.gdpr.title': 'BDAR atitiktis',
  'home.sovereignty.gdpr.body':
    'Duomenų tvarkymo sutartis, tvarkymo veiklos įrašai, duomenų subjektų prašymų procedūra ir saugojimo terminai. Šifruojama perduodant (TLS 1.3) ir saugant.',
  'home.sovereignty.lockin.title': 'Jokio susaistymo',
  'home.sovereignty.lockin.body':
    'Sukurta remiantis atviraisiais standartais: OIDC, JMAP, CalDAV, CardDAV ir WebDAV. Jūsų duomenys lieka perkeliami.',
  'home.standards.overline': 'Originalus Sytue darbas',
  'home.standards.title': 'Sukurta remiantis atviraisiais standartais',
  'home.standards.body':
    'Kiekvienas Sytue produktas yra originalus darbas, parašytas Go ir Astro + Preact kalbomis ant atvirųjų protokolų (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Jokių trečiųjų šalių variklių: jūsų duomenys perkeliami, o rinkinys – suverenus.',
  'home.cta.title': 'Susigrąžinkite savo darbo kontrolę',
  'home.cta.body':
    'Pradėkite su savo organizacija jau šiandien. Pagalba perkeliant iš Google Workspace arba Microsoft 365.',
  'home.cta.primary': 'Sukurti organizaciją',
  'home.cta.secondary': 'Susisiekti su pardavimų skyriumi',
  'grid.discover': 'Sužinoti daugiau',
  'product.mail.blurb': 'Privatus europietiškas paštas per JMAP — be sekiklių ir be reklamų.',
  'product.drive.blurb': 'Failai ir bendradarbiavimas su jūsų prekių ženklu.',
  'product.docs.blurb':
    'Dokumentai, skaičiuoklės ir skaidrės su bendru redagavimu realiuoju laiku.',
  'product.calendar.blurb': 'CalDAV kalendorius, susietas su visu rinkiniu.',
  'product.contacts.blurb': 'CardDAV kontaktai, bendrinami visoje organizacijoje.',
  'product.notes.blurb':
    'Užrašų knygelės, talpinančios bet ką: tekstą, failus, įvykius, ištisus dokumentus.',
  'product.photos.blurb':
    'Nuotraukų galerija ir redaktorius: originalai Drive, pakeitimai – kaip receptas.',
  'product.rooms.blurb': 'Kanalai, žinutės ir komandos vaizdo skambučiai vienoje vietoje.',
  'product.cast.blurb':
    'Vaizdo įrašai pagal užklausą ir tiesioginės transliacijos: įkelkite, transliuokite ir dalykitės, ES.',
  'product.pages.blurb':
    'Svetainės ir internetinės parduotuvės jūsų verslui, sukurtos pagal jūsų poreikius ir paskelbtos ES.',
  'product.bases.blurb': 'Bendros reliacinės lentelės klientams, užsakymams ar atsargoms, ES.',
  'product.cloud.blurb': 'Diegimo ir valdomų duomenų platforma ES.',
  'product.id.blurb': 'Viena tapatybė (SSO + MFA) su Sytue ID.',
  'product.one.blurb':
    'Rinkinio pradžios puslapis: programos, greitieji veiksmai, naujausia veikla ir pranešimai.',
  'product.sheets.blurb':
    'Skaičiuoklės su formulėmis, išoriniais duomenimis ir diagramomis, redaguojamos daugelio žmonių.',
  'product.slides.blurb': 'Pristatymai su jūsų prekių ženklu, temomis ir pranešėjo rodiniu.',
  'product.pdf.blurb':
    'Skaitykite, komentuokite, pasirašykite ir tvarkykite PDF failus, jiems neišvykstant iš Europos.',
  'product.console.blurb': 'Organizacijos administravimas: žmonės, prieiga, atitiktis ir auditas.',
  'product.erp.blurb':
    'Dvidešimt keturi moduliai ir dvejybinė apskaita, kurios duomenų bazė neleidžia išbalansuoti.',
  'productPage.back': 'Visas rinkinys',
  'productPage.features': 'Viskas, ką daro {name}',
  'productPage.featuresPlanned': 'Ką darys {name}',
  'productPage.builtOn': 'Sukurta pagal {standard}',
  'productPage.fromScratch':
    'Docus, Calcus, Diapos ir CertiPDF redaktorius parašytas nuo nulio: nuosavas Sytue variklis, o ne trečiosios šalies integracija.',
  'productPage.shot': 'Pagrindinis {name} ekranas, lygiai toks, koks rodomas prisijungus.',
  'productPage.shotFeatures': '{name} veikia, funkcijos matomos ekrane.',
  'productPage.open': 'Atidaryti {name}',
  'productPage.enDesarrollo': 'Šis produktas dar kuriamas: jis dar nepasiekiamas.',
  'productPage.included':
    'Įtraukta į Sytue One: viena tapatybė (Sytue ID) ir viena registracija suteikia prieigą prie visų produktų. Duomenys 100 % ES („Scaleway“) ir perkeliami naudojant atviruosius standartus.',
  'product.mail.tagline': 'Privatus europietiškas paštas be sekiklių ir be reklamų.',
  'product.mail.standard': 'JMAP (atvirasis standartas)',
  'product.mail.p1': 'Greita pašto dėžutė su gijomis, žymomis, filtrais ir paieška.',
  'product.mail.p2': 'Failų pridėjimas tiesiai iš Sytue Drive.',
  'product.mail.p3': 'Laiško pavertimas Calendar įvykiu.',
  'product.mail.p4':
    'Šifruojama perduodant (TLS 1.3). „Sytue“ infrastruktūra yra ES. Išorinių paslaugų vieta priklauso nuo jų teikėjo.',
  'product.drive.tagline': 'Jūsų failai ir bendradarbiavimas su jūsų organizacijos prekių ženklu.',
  'product.drive.standard': 'WebDAV (atvirasis standartas)',
  'product.drive.p1':
    'Įkėlimas, tvarkymas ir bendrinimas kontroliuojamomis nuorodomis (teisės ir galiojimo pabaiga).',
  'product.drive.p2': 'Dokumentų ir vaizdų peržiūra tiesiai puslapyje.',
  'product.drive.p3': 'Pridėjimas programoje Mail, nesukuriant failo kopijos.',
  'product.drive.p4': 'S3 pagrindu veikianti saugykla Europos regionuose.',
  'product.docs.tagline':
    'Dokumentai, skaičiuoklės ir skaidrės su bendru redagavimu realiuoju laiku.',
  'product.docs.standard': 'Bendras redagavimas realiuoju laiku',
  'product.docs.p1':
    'Rašykite kartu, matant tiesioginį buvimą ir akimirksniu atsirandančius pakeitimus.',
  'product.docs.p2': 'Skaičiuoklės su formulėmis ir sąlyginiu formatavimu.',
  'product.docs.p3': 'Pristatymai su jūsų prekių ženklu.',
  'product.docs.p4': 'Viskas išsaugoma jūsų europietiškame Drive.',
  'product.calendar.tagline': 'Jūsų kalendorius, susietas su visu rinkiniu.',
  'product.calendar.standard': 'CalDAV (atvirasis standartas)',
  'product.calendar.p1': 'Dienos, savaitės ir mėnesio rodiniai.',
  'product.calendar.p2': 'Pasikartojantys įvykiai ir priminimai.',
  'product.calendar.p3': 'Įvykių kūrimas tiesiai iš laiško programoje Mail.',
  'product.calendar.p4': 'Veikia su bet kuria CalDAV kliento programa.',
  'product.contacts.tagline': 'Jūsų organizacijos kontaktai, bendrinami ir perkeliami.',
  'product.contacts.standard': 'CardDAV (atvirasis standartas)',
  'product.contacts.p1': 'Bendra adresų knyga visai organizacijai.',
  'product.contacts.p2': 'Importavimas ir eksportavimas vCard formatu.',
  'product.contacts.p3': 'Dublikatų suliejimas neprarandant duomenų.',
  'product.contacts.p4': 'Veikia su bet kuria CardDAV kliento programa.',
  'product.notes.tagline': 'Viskas, ką norite išsaugoti, vienoje vietoje ir jūsų rankose.',
  'product.notes.standard': 'Nuosavas modelis ant atvirųjų standartų (nuosava REST API)',
  'product.notes.p1': 'Įdėtinės užrašų knygelės laisvoje drobėje, stulpeliuose arba sąraše.',
  'product.notes.p2':
    'Bet kokie užrašai: tekstas, užduotys, nuorodos, failai, Calendar įvykiai, Docs dokumentai.',
  'product.notes.p3': 'Ištisinis šifravimas: slaptafrazė niekada nepalieka jūsų naršyklės.',
  'product.notes.p4': 'Jūs sprendžiate, kas patenka į paiešką — ir kas niekada.',
  'product.rooms.tagline': 'Kanalai, žinutės ir komandos vaizdo skambučiai vienoje vietoje.',
  'product.rooms.standard': 'Žinutės ir skambučiai realiuoju laiku',
  'product.rooms.p1': 'Kanalai pagal komandą ar projektą.',
  'product.rooms.p2': 'Vaizdo skambutis, įtaisytas kiekviename kanale.',
  'product.rooms.p3': 'Paminėjimai ir pranešimai, suvienodinti visame rinkinyje.',
  'product.rooms.p4': 'Istorija, kurioje galima ieškoti iš bet kurios Sytue One vietos.',
  'product.cloud.p4': 'Nuosavi domenai su automatiniu TLS.',
  'product.cloud.p3': 'Valdoma S3 saugykla ir funkcijos.',
  'product.cloud.p2': 'Postgres duomenų bazė su automatine REST API.',
  'product.cloud.p1': 'Peržiūra kiekvienam PR ir akimirksnio grąžinimas.',
  'product.cloud.standard': 'Kubernetes pagrindu sukurta PaaS',
  'product.cloud.tagline': 'Jūsų kodas įdiegtas, o duomenys valdomi Europos Sąjungoje.',
  'product.id.tagline': 'Viena tapatybė visam rinkiniui. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (atvirasis standartas)',
  'product.id.p1': 'Vienkartinis prisijungimas prie visų produktų.',
  'product.id.p2': 'MFA ir prieigos raktai (WebAuthn).',
  'product.id.p3': 'Automatinis naudotojų teikimas (SCIM) iš jūsų katalogo.',
  'product.id.p4': 'Profilis ir sauga vienoje vietoje.',
  'product.one.tagline': 'Visas Sytue One vieno spustelėjimo atstumu, pro tas pačias duris.',
  'product.one.standard': 'Vienas prisijungimas (Sytue ID) visam rinkiniui',
  'product.one.p1':
    'Paleidiklis su visomis jūsų programomis ir tomis, kurias prijungia jūsų organizacija.',
  'product.one.p2': 'Paieška visame rinkinyje (⌘K) iš bet kurio ekrano.',
  'product.one.p3':
    'Šiandienos darbotvarkė ir neskaityti laiškai pradžios puslapyje, šalia naujausios rinkinio veiklos ir jūsų pranešimų.',
  'product.one.p4': 'Prijunkite įrenginius ir atsisiųskite vietines programas.',
  'product.sheets.tagline': 'Prasmingi skaičiai lentelėje, kuri dirba kartu su jumis.',
  'product.sheets.standard': 'ODF/OOXML formatai ir bendras redagavimas realiuoju laiku',
  'product.sheets.p1': 'Formulės, sąlyginis formatavimas ir duomenų tikrinimas.',
  'product.sheets.p2': 'Įkelkite išorinius duomenis ir teikite užklausas neišeinant iš lentelės.',
  'product.sheets.p3': 'Diagramos, mini diagramos, filtravimo juostos ir duomenų lentelės.',
  'product.sheets.p4': 'Eksportavimas į ODS ir XLSX; Excel failų atidarymas.',
  'product.slides.tagline': 'Pasakokite istoriją; skaidrė neatsilieka.',
  'product.slides.standard': 'ODF/OOXML formatai ir bendras redagavimas realiuoju laiku',
  'product.slides.p1': 'Pristatymų temos su jūsų organizacijos identitetu.',
  'product.slides.p2': 'Pranešėjo rodinys su pastabomis ir kita skaidre.',
  'product.slides.p3': 'Vaizdai, figūros, diagramos ir SmartArt.',
  'product.slides.p4': 'Eksportavimas į ODP, PPTX ir PDF; PowerPoint failų atidarymas.',
  'product.pdf.tagline': 'PDF nuo pradžios iki galo, jūsų naršyklėje.',
  'product.pdf.standard': 'PDF (ISO 32000) su standartinėmis anotacijomis ir formomis',
  'product.pdf.p1': 'Žymėjimas, pabraukimas, komentarai ir piešimas dokumente.',
  'product.pdf.p2': 'Formos: laukų pildymas arba savų formų kūrimas.',
  'product.pdf.p3': 'Suliejimas, išskyrimas, pasukimas, numeravimas ir vandens ženklas.',
  'product.pdf.p4':
    'Parašų skydelis, negrįžtamas užtušavimas ir slaptažodžiu apsaugoti dokumentai.',
  'product.console.tagline': 'Valdykite rinkinį iš vieno ekrano.',
  'product.console.standard': 'SCIM, SAML ir OpenID Connect (atvirieji standartai)',
  'product.console.p1': 'Žmonės, grupės (taip pat dinaminės) ir vaidmenys.',
  'product.console.p2': 'SSO su jūsų paslaugų teikėju ir naudotojų teikimas per SCIM.',
  'product.console.p3':
    'Sąlyginės prieigos taisyklės, įrenginiai ir privilegijuota prieiga su patvirtinimu.',
  'product.console.p4':
    'Audito žurnalas, kurį galima eksportuoti į jūsų SIEM, ir teisinis sulaikymas.',
  'product.erp.tagline': 'Visas ERP, kurio modelių katalogas – duomenų failas.',
  'product.erp.standard': 'Dvejybinė apskaita, JSON modelių katalogas ir viena REST API',
  'product.erp.p1':
    'Dvidešimt keturi moduliai ir trisdešimt šeši modeliai: kontaktai, pardavimas, pirkimas, atsargos, gamyba, sąskaitų išrašymas, apskaita, projektai ir personalas.',
  'product.erp.p2':
    'Pridėti modulį reiškia parašyti JSON failą: be migracijos, be naujo maršruto ir be sąsajos darbų.',
  'product.erp.p3':
    'Pinigai saugomi sveikaisiais centais, niekada slankiuoju kableliu, o balansą užtikrina duomenų bazė.',
  'product.erp.p4':
    'Užregistruoti įrašai neliečiami: jie taisomi kitu įrašu, ir būtent tai skiria didžiąją knygą nuo skaičiuoklės.',
  'product.one.f1':
    'Paleidiklis su rinkinio programomis ir trečiųjų šalių programomis, kurias prijungia jūsų organizacija.',
  'product.one.f2':
    'Greitieji veiksmai: rašyti laišką, naujas dokumentas, įkelti į Drive, sukurti įvykį.',
  'product.one.f3': 'Naujausi rinkinyje: paskutiniai elementai iš Drive, Docus, Mail ar Rooms.',
  'product.one.f4': 'Visuotinė paieška klavišais ⌘K arba Ctrl K, su rezultatais iš visų programų.',
  'product.one.f5': 'Pranešimų centras ir buvimo būsena, bendri visam rinkiniui.',
  'product.one.f6':
    'Įrenginių prijungimas: profilis iPhone ir Mac, Android ir kompiuteriui, dviem žingsniais.',
  'product.one.f7': 'Vietinių kompiuterio ir mobiliųjų programų atsisiuntimai.',
  'product.one.f8': 'Pasveikinimo vedlys, kuris paruošia organizaciją be vadovų.',
  'product.mail.f1': 'Pokalbių gijos su skaitymu, atsakymu ir persiuntimu vienu spustelėjimu.',
  'product.mail.f2':
    'Aplankai, spalvotos žymos ir paieška su operatoriais (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Automatinės taisyklės, kurios gaunamus laiškus archyvuoja, pažymi arba perkelia.',
  'product.mail.f4': 'Suplanuotas siuntimas ir „Atšaukti siuntimą“.',
  'product.mail.f5': 'Parašai, adresų slapyvardžiai ir automatinis atsakymas atostogų metu.',
  'product.mail.f6': 'Bendrinamos ir deleguotos pašto dėžutės su skaitymo ir siuntimo teisėmis.',
  'product.mail.f7':
    'Formatuoto teksto rengyklė su priedais iš Sytue Drive, nesukuriant failų kopijų.',
  'product.mail.f8':
    'Nuotolinis turinys pagal numatytuosius nustatymus blokuojamas: niekas nesužino, kada atidarote laišką.',
  'product.mail.f9': 'Pasirašyti laiškai (PGP ir S/MIME) atpažįstami ir pažymimi; TLS perduodant.',
  'product.mail.f10':
    'Prieiga ir iš Thunderbird, Outlook, Apple Mail ar telefono, naudojant programų slaptažodžius.',
  'product.mail.f11':
    'Adresai kaip žymos, Contacts adresų knyga vienu spustelėjimu ir įspėjimas, jei parašote „prisegta“, bet nieko neprisegate.',
  'product.mail.f12':
    'Taisyklės tiesiai iš atidaryto laiško, šiukšlinės išvalymas vienu kartu ir spartieji klavišai su „?“.',
  'product.drive.f1':
    'Mano diskas, komandos diskai, bendrinama su manimi, naujausi, mėgstamiausi ir šiukšlinė.',
  'product.drive.f2':
    'Įkėlimas nutempiant; aplankai, perkėlimas, kopijavimas, pervardijimas ir kelių elementų pasirinkimas.',
  'product.drive.f3':
    'Bendrinimo nuorodos su dviem lygiais (peržiūra arba redagavimas), slaptažodžiu ir galiojimo pabaiga.',
  'product.drive.f4': 'Versijų istorija su atkūrimu.',
  'product.drive.f5': 'Dokumentų, vaizdų ir PDF peržiūra neatsisiunčiant.',
  'product.drive.f6':
    'Biuro dokumentų atidarymas ir redagavimas Docus, Calcus ir Diapos; PDF – CertiPDF.',
  'product.drive.f7':
    'Konfidencialumo žymos, blokuojančios viešas nuorodas ten, kur joms ne vieta.',
  'product.drive.f8':
    'Bendrinimas Rooms kanale arba pridėjimas programoje Mail, nesukuriant failo kopijos.',
  'product.drive.f9': 'WebDAV prieiga ir kompiuterio kliento programa failams matyti kompiuteryje.',
  'product.drive.f10': 'S3 saugykla Europos regionuose, kvota visada matoma.',
  'product.docs.f1':
    'Bendras redagavimas realiuoju laiku, matant buvimą ir netrukdant vieni kitiems.',
  'product.docs.f2':
    'Juostelė su skirtukais: Pagrindinis, Įterpimas, Nuorodos, Laiškai ir Peržiūra.',
  'product.docs.f3':
    'Lentelės, vaizdai, figūros, SmartArt, diagramos, simboliai ir vandens ženklai.',
  'product.docs.f4': 'Nuorodos: turinys, išnašos, citatos ir bibliografija.',
  'product.docs.f5': 'Laiškai: duomenų suliejimas laiškams ir etiketėms.',
  'product.docs.f6': 'Komentarų gijos, keitimų peržiūra ir versijos.',
  'product.docs.f7':
    'Pasirenkamas DI asistentas perrašymui, vertimui ar santraukai, tik jei jūsų organizacija jį įjungia.',
  'product.docs.f8': 'Eksportavimas į ODT, DOCX ir PDF; Word failų atidarymas iš Drive.',
  'product.docs.f9': 'Antraštės, poraštės, puslapių numeravimas ir pakartotinai naudojami blokai.',
  'product.docs.f10': 'Automatinis išsaugojimas jūsų europietiškame Drive su istorija.',
  'product.docs.f11':
    'Atsisiuntimas iš pradžios puslapio PDF, HTML, Markdown, OpenDocument arba Microsoft Office formatu.',
  'product.sheets.f1': 'Formulės su kontekstine pagalba ir akimirksnio perskaičiavimu.',
  'product.sheets.f2':
    'Sąlyginis formatavimas, duomenų tikrinimas ir skaičių formatai pagal lokalę.',
  'product.sheets.f3': 'Įvairių tipų diagramos, mini diagramos ir filtravimo juostos.',
  'product.sheets.f4': 'Duomenų lentelės su sumomis ir rikiavimu; suvestinės lentelė.',
  'product.sheets.f5': 'Išoriniai duomenys: prijunkite šaltinį ir teikite užklausas iš lentelės.',
  'product.sheets.f6': 'Sprendiklis vertei, atitinkančiai tikslą, rasti.',
  'product.sheets.f7': 'Makrokomandos pasikartojančioms užduotims automatizuoti.',
  'product.sheets.f8': 'Bendradarbiavimas realiuoju laiku su buvimo būsena ir komentarais.',
  'product.sheets.f9': 'Filtrai, filtravimo juostos ir paieška lentelėje.',
  'product.sheets.f10': 'Eksportavimas į ODS, XLSX ir PDF; Excel failų atidarymas iš Drive.',
  'product.slides.f1': 'Temos ir paletės pagal jūsų organizacijos prekių ženklą.',
  'product.slides.f2': 'Drobė su figūromis, vaizdais, diagramomis, SmartArt ir jungtimis.',
  'product.slides.f3': 'Pristatymas visame ekrane su pranešėjo rodiniu ir pastabomis.',
  'product.slides.f4': 'Bendradarbiavimas realiuoju laiku su buvimo būsena ir komentarais.',
  'product.slides.f5': 'Skaidrės su fonais, plakatais ir maketais.',
  'product.slides.f6': 'Eksportavimas į ODP, PPTX ir PDF.',
  'product.slides.f7': 'PowerPoint failų atidarymas iš Drive.',
  'product.slides.f8': 'Automatinis išsaugojimas su versijomis.',
  'product.pdf.f1':
    'Greita peržiūros programa su struktūra, miniatiūromis, paieška ir mastelio keitimu pagal puslapį ar plotį.',
  'product.pdf.f2':
    'Anotacijos: žymėjimas, pabraukimas, perbraukimas, komentarai, piešimas, figūros, rodyklės ir antspaudai.',
  'product.pdf.f3': 'Teksto laukai, matavimai ir srities momentinės kopijos.',
  'product.pdf.f4': 'Formos: esamų laukų pildymas arba savų laukų pridėjimas.',
  'product.pdf.f5': 'Dokumentų pasirašymo skydelis.',
  'product.pdf.f6':
    'Negrįžtamas užtušavimas: tai, ką pažymite, dingsta iš failo, o ne tik iš rodinio.',
  'product.pdf.f7':
    'Puslapiai: suliejimas, išskyrimas, šalinimas, pasukimas, numeravimas (taip pat Bates), antraštės ir poraštės.',
  'product.pdf.f8': 'Vandens ženklas, fonas ir redaguojami metaduomenys.',
  'product.pdf.f9': 'Slaptažodžiu apsaugoti dokumentai ir teisės.',
  'product.pdf.f10': 'Skaitymas balsu ir komentarų gijos.',
  'product.calendar.f1':
    'Mėnesio, savaitės, dienos ir darbotvarkės rodiniai su mažu kalendoriumi ir „Šiandien“.',
  'product.calendar.f2':
    'Kasdienis, kassavaitinis ar kasmėnesinis kartojimas su pabaiga; redaguoti tik šį, šį ir paskesnius arba visus.',
  'product.calendar.f3': 'Priminimai kiekvienam įvykiui ir pranešimai rinkinyje.',
  'product.calendar.f4': 'Kvietimai su atsakymu (dalyvausiu, galbūt, ne) ir stebėsena.',
  'product.calendar.f5': '„Rasti laiką“, laisvą visiems pakviestiems asmenims.',
  'product.calendar.f6': 'Rezervuojamos patalpos ir ištekliai su užimtumu.',
  'product.calendar.f7': 'Bendrinami kalendoriai su teisėmis ir su jumis bendrinami kalendoriai.',
  'product.calendar.f8':
    'Išorinių kalendorių (ICS) prenumerata ir keli spalvomis pažymėti kalendoriai.',
  'product.calendar.f9': 'Įvykio kūrimas iš Mail laiško arba iš užrašo.',
  'product.calendar.f10':
    'Veikia su bet kuria CalDAV kliento programa: Apple Calendar, Thunderbird ir kitomis.',
  'product.calendar.f11':
    'Įvykio dubliavimas arba kopijavimas į jūsų kalendorių ir atsisiuntimas .ics formatu asmenims už Sytue ribų.',
  'product.contacts.f1': 'Adresų knygos, įskaitant bendrą jūsų organizacijos adresų knygą.',
  'product.contacts.f2':
    'Išsami kortelė: telefonai, el. pašto adresai, adresai, organizacija, gimtadienis, pastabos ir nuotrauka.',
  'product.contacts.f3': 'Žymos ir mėgstamiausi grupavimui ir paieškai.',
  'product.contacts.f4': 'Dublikatų aptikimas ir suliejimas neprarandant duomenų.',
  'product.contacts.f5':
    'vCard ir CSV importavimas ir eksportavimas; vieno kontakto eksportavimas.',
  'product.contacts.f6': 'Masiniai veiksmai ir šiukšlinė su atkūrimu.',
  'product.contacts.f7': 'Akimirksnio paieška ir automatinis užbaigimas rašant Mail.',
  'product.contacts.f8': 'Veikia su bet kuria CardDAV kliento programa.',
  'product.contacts.f9': 'Artėjantys gimtadieniai iš adresų knygos, pažymint šio mėnesio.',
  'product.contacts.f10':
    'Susitikimo su kontaktu sukūrimas arba kontakto el. pašto adreso kopijavimas neišeinant iš kortelės.',
  'product.notes.f1':
    'Įdėtinės užrašų knygelės su piktograma ir spalva; trys išdėstymai: laisvas, stulpeliai ir sąrašas.',
  'product.notes.f2':
    'Dvylika užrašų tipų: tekstas, užduotys, nuorodos, spalva, vaizdas ir nuorodos į Drive, Docus, Calendar, Rooms bei kitus užrašus.',
  'product.notes.f3':
    'Rengyklė su formatavimu (paryškinimas, sąrašai, nuorodos), išsauganti išeinant.',
  'product.notes.f4':
    'Žymos, prisegimas, dubliavimas, perkėlimas tarp užrašų knygelių, grupavimas ir tempimas drobėje.',
  'product.notes.f5':
    'Matomumas kiekvienai užrašų knygelei: privati, asmeninė, organizacijos arba vieša nuoroda.',
  'product.notes.f6':
    'Šifravimas naršyklėje jautrioms užrašų knygelėms: Sytue negali jų perskaityti.',
  'product.notes.f7': 'Jūs sprendžiate, kurios užrašų knygelės patenka į rinkinio paiešką.',
  'product.notes.f8': 'Atgalinės nuorodos: kurie užrašai nurodo į šį.',
  'product.notes.f9':
    'Visos užrašų knygelės eksportavimas į HTML, PDF, Markdown ir kitus formatus; kopijų importavimas.',
  'product.notes.f10': 'Užrašo bendrinimas Rooms kanale; užrašų kūrimas numetant arba įklijuojant.',
  'product.notes.f11':
    'Lentelės rengyklėje ir Markdown spartieji keliai rašant: „- “, „# “, „> “ ir „```“.',
  'product.notes.f12':
    'Užrašo perkėlimas į kitą užrašų knygelę, vieno užrašo eksportavimas ir užrašų knygelės rikiavimas pagal poreikį.',
  'product.photos.tagline':
    'Jūsų nuotraukos tvarkingos ir pagal jūsų skonį, neišvykstant iš Europos.',
  'product.photos.standard': 'Originalai Sytue Drive; redagavimo variklis jūsų naršyklėje',
  'product.photos.p1': 'Laiko juosta pagal mėnesius su paieška pagal pavadinimą.',
  'product.photos.p2': 'Albumai su viršeliu; peržiūra visame ekrane.',
  'product.photos.p3':
    'Šviesos, spalvų, pasukimo, veidrodinio atvaizdo ir apkarpymo redaktorius realiuoju laiku.',
  'product.photos.p4': 'Kiekvienas pakeitimas yra receptas: originalas niekada neliečiamas.',
  'product.photos.f1':
    'Laiko juosta, sugrupuota pagal mėnesius, su paieška pagal pavadinimą ir ryškiomis miniatiūromis.',
  'product.photos.f2':
    'Albumai: kūrimas, pervardijimas, viršelio pasirinkimas, pridėjimas iš peržiūros ir šalinimas.',
  'product.photos.f3': 'Peržiūra visame ekrane su naršymu klaviatūra.',
  'product.photos.f4':
    'Redaktorius su keturiais slankikliais (ekspozicija, kontrastas, sodrumas, temperatūra), pasukimu ir veidrodiniu atvaizdu.',
  'product.photos.f5':
    'Apkarpymas tempiant ir spartieji klavišai; GPU peržiūra su CPU atsarginiu variantu.',
  'product.photos.f6':
    'Kiekvienas pakeitimas yra atskirai saugomas receptas: originalas Drive niekada nekeičiamas.',
  'product.photos.f7':
    'Eksportavimas visa raiška, atvaizduojamas serveryje tomis pačiomis formulėmis kaip peržiūroje.',
  'product.photos.f8': 'Šiukšlinė su atkūrimu.',
  'product.photos.f9':
    'Originalai saugomi jūsų Sytue Drive; išvestinės versijos yra atkuriama talpykla.',
  'product.photos.f10': 'EXIF duomenys skaitomi iš paties failo; viskas Europos Sąjungoje.',
  'product.photos.f11':
    'Kelių nuotraukų pasirinkimas pagal mėnesį: į naują albumą, į šiukšlinę arba atsisiuntimas vienu kartu.',
  'product.photos.f12':
    'Skaidrių peržiūra, visas dydis, pasukimas iš peržiūros ir apkarpymas fiksuotu santykiu.',
  'product.rooms.f1': 'Vieši ir privatūs kanalai kategorijose; tiesioginės žinutės ir grupės.',
  'product.rooms.f2':
    'Gijos, reakcijos, paminėjimai, prisegtos ir išsaugotos žinutės bei juodraščiai.',
  'product.rooms.f3':
    'Formatavimas su „Aa“ juosta ir sparčiaisiais klavišais: paryškinimas, kursyvas, kodas, citatos, sąrašai ir nuorodos.',
  'product.rooms.f4': 'Priedai, vaizdai, GIF ir nuosavi jaustukai; failai iš Drive.',
  'product.rooms.f5': 'Suplanuotas siuntimas, prioritetas, perskaitymo patvirtinimas ir „matė“.',
  'product.rooms.f6':
    'Pasvirojo brūkšnio komandos ir kortelės: apklausos ir priskiriamos užduotys.',
  'product.rooms.f7':
    'Vaizdo skambučiai kanale: kamera, ekranas, pakelta ranka, pokalbis ir įrašymas.',
  'product.rooms.f8': 'Suplanuoti susitikimai ir kvietimo nuorodos.',
  'product.rooms.f9': 'Robotai, žiniatinklio kabliukai ir integracijos su kitais įrankiais.',
  'product.rooms.f10':
    'Buvimo būsena ir pasirinktinė būsena; pranešimų nustatymai kiekvienam kanalui.',
  'product.rooms.f11': 'Paieška istorijoje, taip pat iš viso rinkinio.',
  'product.rooms.f12':
    'Realusis laikas per WebSocket, su pakartotiniu prisijungimu ir be prarastų žinučių.',
  'product.rooms.f13':
    'Greitas kanalų perjungimas Ctrl+G, visko pažymėjimas kaip perskaityto ir spartieji klavišai su „?“.',
  'product.cast.tagline':
    'Jūsų vaizdo įrašai ir tiesioginės transliacijos, pagal užklausą ir realiuoju laiku, neišvykstant iš Europos.',
  'product.cast.standard':
    'HLS atkūrimas naršyklėje; RTMP įvestis iš bet kurios transliavimo programos',
  'product.cast.p1':
    'Įkelkite vaizdo įrašą ir jis perkoduojamas keliomis kokybėmis bet kokiam ekranui.',
  'product.cast.p2':
    'Transliuokite tiesiogiai iš transliavimo programos, naršyklės ar programėlės.',
  'product.cast.p3':
    'Keturi matomumo lygiai: viešas, neįtrauktas į sąrašą, organizacija arba konkretūs asmenys.',
  'product.cast.p4': 'Tiesioginis pokalbis ir komentarai, su pagrindiniu paskelbusiojo moderavimu.',
  'product.cast.f1': 'Įkelkite iki 8 GiB vaizdo įrašus; originalas išsaugomas.',
  'product.cast.f2': 'Adaptyvus atkūrimas trimis kokybėmis; naršyklė pasirenka geriausią.',
  'product.cast.f3':
    'Tiesioginė transliacija iš transliavimo programos per RTMP, su srauto raktu kiekvienam asmeniui.',
  'product.cast.f4': 'Tiesiogiai iš naršyklės ar programėlės: kamera, ekranas ir mikrofonas.',
  'product.cast.f5':
    'Tiesioginių transliacijų įrašai automatiškai tampa vaizdo įrašais pagal užklausą.',
  'product.cast.f6': 'Viešas pradinis puslapis ir kanalas kiekvienai organizacijai, indeksuojami.',
  'product.cast.f7':
    'Neįtrauktos į sąrašą nuorodos, vaizdo įrašai tik organizacijai arba prieiga konkretiems asmenims pagal @naudotoją.',
  'product.cast.f8':
    'Tiesioginis pokalbis ir komentarai gijomis, moderuojami paskelbusiojo ir organizacijos administracijos.',
  'product.cast.f9':
    'Įterpiamas grotuvas ir „oEmbed“ viešiems ir neįtrauktiems į sąrašą vaizdo įrašams.',
  'product.cast.f10': 'Saugykla skaičiuojama į jūsų „Sytue Drive“ kvotą; viskas Europos Sąjungoje.',
  'product.cast.f11':
    '„Įkelti į Cast“ iš „Sytue Drive“: vaizdo įrašo kopija perkeliama į „Cast“, o failas lieka „Drive“.',
  'product.pages.tagline':
    'Jūsų įmonės svetainė ir jos parduotuvė, sukurtos vaizdiniame redaktoriuje ir paskelbtos nepaliekant Europos.',
  'product.pages.standard':
    'Statinis, prieinamas HTML ir CSS be trečiųjų šalių slapukų; eksportas į Astro',
  'product.pages.p1':
    'Vaizdinis redaktorius su rėmeliais, kurie patys prisitaiko prie kompiuterio, planšetės ir telefono.',
  'product.pages.p2':
    'Dešimt šablonų pagal sektorių, su tikrais tekstais ir viešojo naudojimo nuotraukomis.',
  'product.pages.p3': 'Parduotuvė su katalogu, krepšeliu ir mokėjimais į jūsų įmonės sąskaitą.',
  'product.pages.p4': 'Jūsų pačių domenas su automatiniu sertifikatu arba adresas sites.sytue.eu.',
  'product.pages.f1':
    'Rėmeliais pagrįstas vaizdinis redaktorius su automatiniu išdėstymu, pakartotinai naudojamais komponentais ir paruoštomis sekcijomis.',
  'product.pages.f2':
    'Trys lūžio taškai (kompiuteris, planšetė ir telefonas), kiekvienas su savo nustatymais.',
  'product.pages.f3': 'Lankytojų analitika be slapukų, svetainės skydelyje.',
  'product.pages.f4':
    'Daugiakalbės svetainės su kiekvienos kalbos adresais, hreflang ir Sytue AI padedamu vertimu.',
  'product.pages.f5':
    'Vaizdai iš jūsų kompiuterio, Sytue Drive arba Sytue Photos, su alternatyviuoju tekstu, kurio prašoma įterpiant.',
  'product.pages.f6':
    'Parduotuvė su variantais, atsargomis, PVM pagal produktą ir mokėjimu per Mollie, Adyen, Redsys ar Stripe.',
  'product.pages.f7':
    'Užsakymai su supaprastinta sąskaita faktūra, kurią išrašo Sytue ERP, ir atsisakymas tiesiai iš svetainės.',
  'product.pages.f8':
    'Formos su sutikimu, žinučių dėžutė ir pranešimai jūsų organizacijos žmonėms.',
  'product.pages.f9':
    'Pavadintos versijos, peržiūra už slaptos nuorodos ir grįžimas prie bet kurio ankstesnio paskelbimo.',
  'product.pages.f10':
    'Kodo eksportas į Astro, React, Next.js ir savąsias programėles, o katalogo – į Shopify, WooCommerce ir kitas platformas.',
  'product.bases.tagline':
    'Susietos lentelės klientams, užsakymams ar atsargoms tvarkyti, su leidimais ir kiekvieno pakeitimo žurnalu.',
  'product.bases.standard': 'Atviri CSV ir JSON; schemą ir ryšius užtikrina pati duomenų bazė',
  'product.bases.p1':
    'Klientai, užsakymai ar atsargos susietose lentelėse, nieko neišmanant apie duomenų bazes.',
  'product.bases.p2': 'Bendrinkite su organizacija pasirinktu vaidmeniu.',
  'product.bases.p3': 'Kiekvienas pakeitimas užfiksuojamas: kas jį atliko ir kas buvo anksčiau.',
  'product.bases.p4': 'Jūsų duomenys lieka ES, o eksportuoti galite, kada norite.',
  'product.bases.f1':
    'Lentelės su teksto, skaičiaus, datos, žymimojo langelio, vieno ir kelių pasirinkimų laukais.',
  'product.bases.f2':
    'Ryšiai tarp lentelių: susiekite užsakymą su klientu, o duomenų bazė neleis nutraukti ryšio.',
  'product.bases.f3':
    'Klaviatūra redaguojamas tinklelis, skaitomas ekrano skaitytuvais ir tinkamas telefone.',
  'product.bases.f4':
    'Leidimai pagal duomenų bazę ir lentelę: savininkas, redaktorius, komentatorius ir skaitytojas.',
  'product.bases.f5': 'Importuokite CSV failą kaip naują lentelę ir eksportuokite į CSV ar JSON.',
  'product.bases.f6': 'Kiekvieno pakeitimo žurnalas: kas, kuris įrašas ir laukas, prieš ir po.',
  'product.cloud.f12':
    'Naudojimas, išlaidos ir europietiškas sąskaitų išrašymas toje pačioje konsolėje.',
  'product.cloud.f11': 'Žmonės, API prieigos raktai ir eksportuojamas audito žurnalas.',
  'product.cloud.f10': 'Privatumą gerbianti žiniatinklio analitika be slapukų.',
  'product.cloud.f9': 'Suplanuotos užduotys ir projektų šablonai.',
  'product.cloud.f8': 'Terminalas konteineryje; paslaugos paleidimas iš naujo arba pristabdymas.',
  'product.cloud.f7': 'Tiesioginiai žurnalai, metrikos ir stebėsena su įspėjimais.',
  'product.cloud.f6': 'Aplinkos kintamieji ir paslaptys kiekvienai aplinkai.',
  'product.cloud.f5': 'Nuosavi domenai su automatiniu TLS; peradresavimai ir antraštės.',
  'product.cloud.f4': 'S3 saugykla ir valdomos funkcijos.',
  'product.cloud.f3': 'Postgres duomenų bazės su automatine REST API ir SQL rengykle.',
  'product.cloud.f2': 'Peržiūra kiekvienam PR ir grąžinimas į bet kurį diegimą vienu spustelėjimu.',
  'product.cloud.f1':
    'Projektai iš jūsų saugyklos (GitHub, GitLab ar nuosavo Git) su diegimu po kiekvieno push.',
  'product.console.f1': 'Skydelis su organizacijos būkle ir tuo, kas šiandien verta dėmesio.',
  'product.console.f2':
    'Paskyros: įtraukimas, sustabdymas, slaptažodžio ir MFA atstatymas, seansai, slapyvardžiai ir pašto dėžutė.',
  'product.console.f3':
    'Statinės grupės ir taisyklėmis pagrįstos dinaminės grupės; vaidmenys su teisėmis.',
  'product.console.f4': 'Privilegijuota prieiga (PIM): laikinas aktyvinimas su patvirtinimu.',
  'product.console.f5':
    'SSO su SAML ar OIDC paslaugų teikėjais ir ryšio patikra; naudotojų teikimas per SCIM.',
  'product.console.f6': 'Sąlyginės prieigos taisyklės ir patikimų įrenginių sąrašas.',
  'product.console.f7':
    'Integracijos: OIDC klientai, SAML ir trečiųjų šalių ryšiai; produktų pakeitimas.',
  'product.console.f8': 'Turinio perkėlimas: migracija iš kitų paslaugų teikėjų.',
  'product.console.f9': 'Dokumentų rengyklė ir administruojami Rooms kanalai.',
  'product.console.f10':
    'Valdysena: teisinis sulaikymas, konfidencialumo žymos ir slaptažodžių politika.',
  'product.console.f11':
    'Audito žurnalas su eksportavimu į CSV ir persiuntimu į jūsų SIEM; paslaugų žurnalai.',
  'product.console.f12':
    'Nuosavas domenas ir prekių ženklas: organizacijos logotipas ir pašto domenas.',
  'product.id.f1':
    'Vienkartinis prisijungimas (OpenID Connect ir OAuth 2.0 su PKCE) rinkiniui ir jūsų programoms.',
  'product.id.f2': 'Prieigos raktai (WebAuthn) ir TOTP MFA su atkūrimo kodais.',
  'product.id.f3': 'Programų slaptažodžiai IMAP ir OIDC nepalaikančioms kliento programoms.',
  'product.id.f4': 'Seansai ir įrenginiai: atšaukite vieną arba visus kitus.',
  'product.id.f5': 'SAML federacija su jūsų IdP ir Sytue kaip SAML IdP kitoms programoms.',
  'product.id.f6': 'Naudotojų teikimas per SCIM iš jūsų katalogo.',
  'product.id.f7': 'Kvietimai, el. pašto patvirtinimas ir slaptažodžio atkūrimas.',
  'product.id.f8': 'Profilis iPhone ir Mac (.mobileconfig) vienu palietimu.',
  'product.id.f9': 'Organizacijos nustatytos slaptažodžių politikos ir prieigos taisyklės.',
  'product.id.f10': 'Dinaminė klientų registracija (DCR) ir standartinis aptikimas.',
  'product.erp.f1':
    'Dvidešimt keturi moduliai ir trisdešimt šeši modeliai vienoje paslaugoje ir vienoje duomenų bazėje.',
  'product.erp.f2':
    'Modelių katalogas yra duomenys: kiekvienas modulis aprašo savo laukus, būsenas ir sąrašo stulpelius.',
  'product.erp.f3':
    'Naujas modulis – tai vienas JSON failas — be DDL, be migracijos ir be lentelių užrakinimo.',
  'product.erp.f4':
    'Viena API visiems trisdešimt šešiems modeliams ir viena iš katalogo sugeneruota sąsaja.',
  'product.erp.f5':
    'Dvejybinė apskaita su sumomis sveikaisiais centais; nė vieno skaičiaus slankiuoju kableliu.',
  'product.erp.f6': 'Balansą tikrina duomenų bazė patvirtinant operaciją, o ne programos kodas.',
  'product.erp.f7':
    'Uždaryti įrašai nekeičiami: jų negalima redaguoti, šalinti ar papildyti naujomis eilutėmis.',
  'product.erp.f8':
    'Vienas dokumentas, vienas įrašas: pakartotinis bandymas ar dvi atviros kortelės negali padvigubinti pajamų.',
  'product.erp.f9':
    'Bendri pavadinimai pateikiami visomis 26 kalbomis, o neišverstas laukas neleidžia paslaugai pasileisti.',
  'product.erp.f10':
    'Indeksuoti sąrašai: pirmasis dvidešimties tūkstančių eilučių puslapis pateikiamas greičiau nei per milisekundę.',
  'meta.downloads.title': 'Atsisiuntimai — Sytue One kompiuteriui ir mobiliesiems',
  'meta.downloads.description':
    'Vietinės Sytue One programos macOS, Windows, Linux, iOS ir Android sistemoms. Visas rinkinys jūsų įrenginyje, su kiekvienos sistemos integracijomis.',
  'downloads.overline': 'Programos',
  'downloads.h1': 'Sytue One jūsų įrenginyje',
  'downloads.intro':
    'Tikros kompiuterio ir mobiliosios programos: jos įdiegiamos, laikomos Dock arba meniu Pradžia, praneša sistemos pranešimais ir atidaro failus iš jūsų įrenginio.',
  'downloads.desktop.title': 'Kompiuteris',
  'downloads.desktop.body':
    'Kiekvienam produktui – atskira vietinė programa, kuri pati piešia savo sąsają vaizdo plokšte: viduje nėra naršyklės, todėl ji greitai pasileidžia ir naudoja išteklius kaip sistemos programa.',
  'downloads.apps.title': 'Tik tai, ko reikia',
  'downloads.apps.body':
    'Sytue One apima visą darbo dieną. Jei norite tik disko, Sytue Drive galima įdiegti atskirai.',
  'downloads.includes': 'Apima:',
  'downloads.mobile.title': 'Mobilieji',
  'downloads.mobile.body':
    'Vietinės iOS ir Android programos su sistemos pranešimais, bendrinimu ir prieiga prie failų bei kameros.',
  'downloads.selfhost.title': 'Jūsų organizacija turi nuosavą domeną?',
  'downloads.selfhost.body':
    'Vietinės programos jungiasi prie sytue.eu. Jei jūsų organizacija turi nuosavą diegimą, prisijunkite naršyklėje jos domene: paketas žiniatinklyje veikia taip pat.',
  'downloads.yourSystem': 'Jūsų sistema',
  'downloads.signInToGet': 'Prisijunkite, kad atsisiųstumėte',
  'downloads.store': 'Žiūrėti: {format}',
  'downloads.soon': 'Netrukus: {format}',
  'downloads.browser.text':
    'Mieliau naudojate naršyklę? Visas rinkinys žiniatinklyje veikia taip pat, nieko neįdiegiant:',
  'downloads.browser.link': 'atidaryti Sytue One',
  'platform.macos.req': 'macOS 11 arba naujesnė · Apple Silicon ir Intel',
  'platform.windows.req': 'Windows 10 arba naujesnė · 64 bitų',
  'platform.linux.req': 'OpenGL 2.1 · 64 bitų',
  'platform.ios.req': 'iOS 14 arba naujesnė',
  'platform.android.req': 'Android 6.0 arba naujesnė',
  'footer.tagline':
    'Kiekviena rinkinio programa yra mūsų pačių paslauga, sukurta remiantis atvirais, paskelbtais protokolais.',
  'footer.products': 'Produktai',
  'footer.resources': 'Ištekliai',
  'footer.legal': 'Teisinė informacija',
  'footer.suite': 'Rinkinys',
  'footer.downloads': 'Atsisiųsti programas',
  'footer.sales': 'Susisiekti su pardavimų skyriumi',
  'footer.privacy': 'Privatumas ir BDAR',
  'footer.accessibility': 'Prieinamumas',
  'footer.openSource': 'Atvirasis kodas',
  'footer.legalNotice': 'Teisinis pranešimas',
  'footer.terms': 'Sąlygos',
  'footer.copyright':
    '© {year} Sytue · Duomenys talpinami ES („Scaleway“ fr-par / nl-ams · nuosava zona Madride).',
  'footer.privacyNotice':
    'Mes neparduodame jūsų duomenų ir nesidalijame jais su trečiosiomis šalimis. Nenaudojame jų profiliavimui, reklamai ar savo arba kitų paslaugų mokymui ar tobulinimui. Jie šifruojami perduodant ir saugant, o mūsų Europos infrastruktūrą palieka tik tiek, kiek būtina sąskaitoms išrašyti, pranešimams siųsti, kai programa uždaryta (tiesioginių pranešimų paslauga mato tik jūsų įrenginį ir laiką, niekada – turinį), ir jūsų įjungtose integracijose, kuriose trečioji šalis paprašys jūsų sutikimo dėl naudojamų duomenų.',
  'meta.notFound.title': 'Puslapis nerastas — Sytue One',
  'notFound.overline': 'Klaida 404',
  'notFound.title': 'Šis puslapis nerastas',
  'notFound.body':
    'Nuoroda galėjo pasikeisti. Grįžkite į pradžios puslapį arba susipažinkite su rinkiniu.',
  'notFound.home': 'Grįžti į pradžią',
  'meta.legal.title': 'Teisinis pranešimas — Sytue One',
  'meta.legal.description':
    'www.sytue.eu ir Sytue One paslaugos savininko identifikaciniai duomenys: Sytue Projects, S.L.',
  'aviso.title': 'Teisinis pranešimas',
  'aviso.intro':
    'Vadovaujantis Ispanijos liepos 11 d. įstatymo Nr. 34/2002 dėl informacinės visuomenės paslaugų ir elektroninės prekybos (LSSI) 10 straipsniu, pateikiami šios svetainės ir Sytue One paslaugos savininko identifikaciniai duomenys.',
  'aviso.review.body':
    'Šiame pranešime pateikti registro duomenys patikrinti Juridinių asmenų registre. Likęs tekstas yra teisiškai peržiūrimas, ir jo formuluotė gali keistis, nekeičiant to, kas aprašyta.',
  'aviso.owner.title': 'Savininkas',
  'aviso.owner.name': 'Pavadinimas',
  'aviso.owner.form': 'Teisinė forma',
  'aviso.owner.formValue':
    'Vienanarė ribotos atsakomybės bendrovė (sociedad limitada unipersonal) pagal Ispanijos teisę',
  'aviso.owner.nif': 'Mokesčių mokėtojo kodas (NIF)',
  'aviso.owner.address': 'Buveinė',
  'aviso.owner.registry': 'Įrašas registre',
  'aviso.owner.registryValue':
    'Madrido juridinių asmenų registras (Registro Mercantil de Madrid), 8 skyrius, lapas M-887153, 1 įrašas',
  'aviso.owner.email': 'El. paštas',
  'aviso.owner.web': 'Svetainė',
  'aviso.scope.title': 'Paskirtis',
  'aviso.scope.body':
    'Ši svetainė pristato Sytue One rinkinį ir suteikia prieigą prie jo programų. Naudojimąsi paslauga reglamentuoja Paslaugų teikimo sąlygos, o asmens duomenų tvarkymą – Privatumo politika; nuorodos į abu dokumentus pateikiamos kiekvieno puslapio poraštėje.',
  'aviso.ip.title': 'Intelektinė ir pramoninė nuosavybė',
  'aviso.ip.body':
    'Šios svetainės turinys (tekstai, dizainas, kodas, Sytue ir Sytue One prekių ženklai bei logotipai) priklauso Sytue Projects, S.L. arba naudojamas pagal licenciją. Trečiųjų šalių produktų pavadinimai, nurodyti kaip suderinamumo nuorodos, yra jų savininkų prekių ženklai.',
  'aviso.liability.title': 'Atsakomybė',
  'aviso.liability.body':
    'Sytue Projects, S.L. stengiasi, kad ši svetainė būtų pasiekiama ir atnaujinta, tačiau negarantuoja, kad nebus sutrikimų ar klaidų. Bendrovė neatsako už trečiųjų šalių svetainių, į kurias pateikiamos nuorodos, turinį.',
  'aviso.law.title': 'Taikytina teisė',
  'aviso.law.body':
    'Šiam pranešimui taikoma Ispanijos teisė. Vartotojams išlieka teisės ir jurisdikcija, kurias suteikia jų gyvenamosios vietos Europos Sąjungos valstybės teisė.',
  'aviso.updated': 'Paskutinį kartą atnaujinta: 2026 m. rugsėjo mėn.',
  'meta.accessibility.title': 'Prieinamumo pareiškimas — Sytue One',
  'meta.accessibility.description':
    'Kiek prieinami yra Sytue One rinkinys ir ši svetainė, ką matuojame, ko dar trūksta ir kaip mums apie tai pranešti.',
  'a11y.title': 'Prieinamumo pareiškimas',
  'a11y.intro':
    'Sytue siekia, kad Sytue One ir šia svetaine galėtų naudotis kiekvienas, nepriklausomai nuo naudojamų pagalbinių technologijų. Šis pareiškimas taikomas www.sytue.eu ir rinkinio žiniatinklio programoms.',
  'a11y.standard.title': 'Standartas, kurio laikomės',
  'a11y.standard.body':
    'Kuriame ir projektuojame pagal WCAG 2.1 AA lygį, į kurį nurodo Europos standartas EN 301 549 ir Europos prieinamumo aktas (Direktyva (ES) 2019/882).',
  'a11y.status.title': 'Atitikties būsena',
  'a11y.status.body':
    'Iš dalies atitinka: didžioji rinkinio dalis atitinka standartą, o toliau išvardytos dalys dar ne visiškai. Nepriklausomas auditas dar neužsakytas.',
  'a11y.measures.title': 'Ką darome, kad to pasiektume',
  'a11y.measures.design':
    'Viena dizaino sistema visam rinkiniui: teksto spalvos su patikrintu kontrastu, matomas fokusavimo žymeklis ant kiekvieno valdiklio ir meniu, dialogų bei sąrašų valdymas klaviatūra.',
  'a11y.measures.ci':
    'Automatiniai prieinamumo auditai („Lighthouse“) vykdomi integracijos procese po kiekvieno viešosios svetainės pakeitimo.',
  'a11y.measures.system':
    'Sąsaja paiso jūsų sistemos nustatymų: tamsios ar šviesios temos, sumažinto judesio ir teksto dydžio, be fiksuotų pikselių dydžių formų laukuose.',
  'a11y.measures.text':
    'Kiekvienas vaizdas, piktograma ir tuščia būsena turi tekstinę alternatyvą, o kiekvienas puslapis nurodo savo kalbą, kad ekrano skaitytuvai ją ištartų teisingai.',
  'a11y.limits.title': 'Žinomi apribojimai',
  'a11y.limits.body':
    'Skaičiuoklės tinklelis, skaidrių drobė ir PDF anotavimas yra sudėtingi redaktoriai, kuriuos dar reikia tobulinti, kad jais būtų galima visiškai naudotis klaviatūra ir ekrano skaitytuvu. Žmonių įkelti dokumentai ir failai rodomi tokie, kokie yra, ir mes jų nepritaikome.',
  'a11y.feedback.title': 'Praneškite, kas neveikia',
  'a11y.feedback.body':
    'Jei kuo nors rinkinyje ar šioje svetainėje naudotis sunku ar neįmanoma, parašykite mums — išnagrinėsime ir atsakysime:',
  'a11y.enforcement.title': 'Jei atsakymas netenkina',
  'a11y.enforcement.body':
    'Jei atsakymas į praneštą kliūtį netenkina, galite kreiptis į už prieinamumą atsakingą instituciją savo šalyje pagal Europos prieinamumo aktą.',
  'a11y.updated':
    'Šis pareiškimas parengtas 2026 m. rugsėjo mėn. ir peržiūrimas, kai rinkinys iš esmės pasikeičia.',
  'meta.openSource.title': 'Atvirasis kodas — Sytue One',
  'meta.openSource.description':
    'Sytue kuria beveik viską nuo nulio, tačiau iš naujo neišranda to, kas jau gerai veikia. Atvirojo kodo projektai, kuriais remiasi paketas.',
  'oss.title': 'Atvirasis kodas',
  'oss.intro':
    'Sytue kuria beveik viską nuo nulio: paštą, redaktorius, kalendorių. Tačiau iš naujo neišranda to, kas jau gerai veikia. Tai atvirojo kodo projektai, kuriais remiasi kai kurios paketo dalys.',
  'oss.livekit.body':
    'Realaus laiko garsas ir vaizdas Sytue Rooms skambučiams veikia naudojant LiveKit.',
  'oss.postgres.body':
    'Kiekviena paketo paslauga saugo savo duomenis PostgreSQL, atskira duomenų bazė kiekvienai organizacijai.',
  'meta.privacy.title': 'Privatumas ir BDAR — Sytue One',
  'meta.privacy.description':
    'Kaip Sytue One tvarko asmens duomenis: duomenų valdytojas, duomenys, teisiniai pagrindai, saugojimas, duomenų tvarkytojai, sauga, tarptautinis perdavimas ir jūsų teisės pagal BDAR.',
  'legal.overline': 'Teisinė informacija',
  'legal.updated': 'Paskutinį kartą atnaujinta: 2026 m. liepos mėn.',
  'legal.review.title': 'Teisiškai peržiūrima.',
  'privacy.title': 'Privatumas ir BDAR',
  'privacy.review.body':
    'Ši politika tiksliai aprašo, kaip šiandien tvarkome duomenis. Konkretūs saugojimo terminai bus papildyti galutinėje versijoje, kurią peržiūrės mūsų teisininkų komanda.',
  'privacy.controller.title': 'Duomenų valdytojas',
  'privacy.controller.body':
    'Sytue Projects, S.L. (Ispanijos mokesčių mokėtojo kodas B88729900; Calle Atenas, 10, 28822 Coslada, Madridas, Ispanija; įregistruota Madrido juridinių asmenų registre, 8 skyrius, lapas M-887153), kaip Sytue One rinkinio teikėja, yra paslaugos paskyrų ir organizacijų duomenų valdytoja. Duomenų apsaugos kontaktas:',
  'privacy.data.title': 'Kokius duomenis tvarkome',
  'privacy.data.account.label': 'Paskyros duomenys:',
  'privacy.data.account.body':
    'vardas, pavardė, el. pašto adresas ir organizacija, kuriuos valdo Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Jūsų organizacijos turinys:',
  'privacy.data.content.body':
    'paštas, failai, dokumentai, kalendorius ir žinutės, kuriuos jūsų organizacija talpina rinkinyje. Šis turinys priklauso jūsų organizacijai.',
  'privacy.data.technical.label': 'Techniniai veikimo duomenys:',
  'privacy.data.technical.body':
    'prieigos žurnalai ir ryšio duomenys, reikalingi saugai, piktnaudžiavimo prevencijai ir paslaugos veikimui.',
  'privacy.data.note':
    'Neparduodame duomenų ir nenaudojame jų reklamai. „Sytue“ valdoma infrastruktūra ir duomenų saugyklos yra Europos Sąjungoje („Scaleway“, fr-par ir nl-ams regionai bei vietinė infrastruktūra Madride), duomenys šifruojami perduodant (TLS 1.3) ir saugant. Ryšiai su išorinėmis paslaugomis siunčia jūsų leistus duomenis jų teikėjui pagal jo vietą ir sąlygas.',
  'privacy.basis.title': 'Teisinis pagrindas',
  'privacy.basis.body':
    'Duomenys tvarkomi remiantis BDAR 6 straipsnio teisiniais pagrindais: paslaugų sutarties su jūsų organizacija vykdymu, mūsų teisėtu interesu užtikrinti saugą ir užkirsti kelią piktnaudžiavimui, teisinių prievolių vykdymu ir, kai to aiškiai prašoma, jūsų sutikimu. Išsami informacija pagal kiekvieną pagrindą ir tvarkymo veiklą bus pateikta galutiniame tekste.',
  'privacy.retention.title': 'Saugojimas',
  'privacy.retention.body':
    'Paskyros duomenis ir turinį saugome tol, kol jūsų organizacija naudojasi aktyvia paslauga. Uždarius paskyrą ar organizaciją, duomenys per protingą laikotarpį ištrinami arba nuasmeninami, išskyrus tuos, kuriuos privalome saugoti pagal įstatymus (pavyzdžiui, sąskaitas). Techniniai veikimo žurnalai periodiškai valomi. Konkretūs terminai pagal kategorijas nurodomi galutinėje versijoje. Pranešimai apie turinį saugomi, kol yra atviri, ir ištrinami praėjus dvylikai mėnesių po jų išsprendimo.',
  'privacy.processors.title': 'Duomenų tvarkytojai ir subtvarkytojai',
  'privacy.processors.body':
    'Mūsų infrastruktūros teikėja yra „Scaleway“ (fr-par ir nl-ams regionai), veikianti kaip duomenų tvarkytoja tik Europos Sąjungoje; be to, valdome nuosavą zoną Madride (Ispanija) savo patalpose. Prenumeratos mokėjimams naudojame Mollie B.V. (Nyderlandai), kuri tvarko tik mokėjimui reikalingus duomenis. Nenaudojame subtvarkytojų už EEE ribų. Subtvarkytojų sąrašas bus paskelbtas ir nuolat atnaujinamas, apie kiekvieną papildymą pranešant iš anksto.',
  'privacy.security.title': 'Sauga',
  'privacy.security.body':
    'Taikome BDAR atitinkančias technines ir organizacines priemones: šifravimą perduodant (TLS 1.3) ir saugant, organizacijų atskyrimą (multi-tenant su eilutės lygmens sauga), stiprų autentifikavimą (MFA/WebAuthn), prieigos registravimą ir mažiausių privilegijų principą. Nė viena sistema nėra neklystanti: jei pažeidimas palies jus, pranešime per Reglamente nustatytus terminus.',
  'privacy.cookies.title': 'Slapukai',
  'privacy.cookies.body':
    'Naudojame tik techninius slapukus, reikalingus vienkartiniam prisijungimui ir seanso saugai. Nenaudojame reklaminių ar trečiųjų šalių sekimo slapukų, todėl nėra sutikimo sekti sienos.',
  'privacy.transfers.title': 'Tarptautinis perdavimas',
  'privacy.transfers.body':
    '„Sytue“ valdomas duomenų tvarkymas ir talpinimas vyksta Europos Sąjungos regionuose. Naudotojo įjungti išoriniai ryšiai siunčia leistus duomenis jų teikėjui; jų vieta priklauso nuo teikėjo ir jo konfigūracijos. Pranešimai, siunčiami, kai programa ar naršyklė uždaryta, taip pat gali naudoti išorines paslaugas. Įrenginio tiesioginių pranešimų paslauga („Google“, „Mozilla“ arba „Apple“, priklausomai nuo jūsų naršyklės ar telefono) mato anoniminį jūsų įrenginio identifikatorių ir kiekvieno pranešimo laiką — niekada turinio, kuris keliauja ištisai užšifruotas, o iOS sistemoje siunčiamas tuščias. Pranešimus galite išjungti, ir tuomet neišeina net tai.',
  'privacy.rights.title': 'Jūsų teisės',
  'privacy.rights.body1':
    'BDAR suteikia jums teisę susipažinti su duomenimis, juos ištaisyti, ištrinti, nesutikti su tvarkymu, apriboti tvarkymą ir perkelti duomenis. Šiomis teisėmis galima pasinaudoti rašant adresu',
  'privacy.rights.body2':
    '; atsakome per Reglamente nustatytus terminus. Taip pat turite teisę pateikti skundą',
  'privacy.rights.authority': 'priežiūros institucijai',
  'privacy.dpo.title': 'Duomenų apsaugos pareigūnas (DPO)',
  'privacy.dpo.body': 'Dėl bet kokio duomenų apsaugos klausimo arba norint susisiekti su DPO:',
  'privacy.minors.title': 'Nepilnamečiai',
  'privacy.minors.body':
    'Sytue One yra paslauga organizacijoms ir profesiniam naudojimui; ji neskirta nepilnamečiams, ir mes sąmoningai nerenkame nepilnamečių duomenų.',
  'privacy.changes.title': 'Šios politikos pakeitimai',
  'privacy.changes.body':
    'Jei atnaujinsime šią politiką, pakeisime „paskutinio atnaujinimo“ datą, o esant esminiams pakeitimams, apie juos pranešime paslaugos kanalais prieš jiems įsigaliojant.',
  'meta.terms.title': 'Paslaugų teikimo sąlygos — Sytue One',
  'meta.terms.description':
    'Sytue One paslaugų teikimo sąlygos: teikėjas, apimtis, paskyros ir organizacijos, priimtinas naudojimas, pasiekiamumas, jūsų duomenys, intelektinė nuosavybė ir taikytina teisė.',
  'terms.title': 'Paslaugų teikimo sąlygos',
  'terms.review.body':
    'Šios sąlygos aprašo paslaugą ir jos naudojimą tiksliai taip, kaip ji veikia šiandien. Konkrečios sutarties nuostatos (paslaugų lygis, atsakomybė ir jurisdikcija) bus papildytos galutinėje versijoje, kurią peržiūrės mūsų teisininkų komanda.',
  'terms.provider.title': 'Kas teikia paslaugą',
  'terms.provider.body':
    'Sytue One teikia Sytue Projects, S.L. (Ispanijos mokesčių mokėtojo kodas B88729900; Calle Atenas, 10, 28822 Coslada, Madridas, Ispanija; įregistruota Madrido juridinių asmenų registre, 8 skyrius, lapas M-887153) iš infrastruktūros, esančios tik Europos Sąjungoje („Scaleway“, fr-par ir nl-ams regionai, ir nuosava zona Madride), o paslaugai taikoma ES teisė.',
  'terms.scope.title': 'Paslaugos apimtis',
  'terms.scope.body':
    'Sytue One yra produktyvumo rinkinys organizacijoms: tapatybė (Sytue ID), paštas, failai, dokumentai, kalendorius, kontaktai ir žinutės su vaizdo skambučiais. Paslauga užsakoma kiekvienai organizacijai.',
  'terms.accounts.title': 'Paskyros ir organizacijos',
  'terms.accounts.body':
    'Kiekviena organizacija valdo savo paskyras, grupes ir teises per Sytue ID (SSO + MFA). Organizacijos administravimas atsako už paskyrų pridėjimą ir šalinimą bei už tai, kaip paslauga naudojama per tas paskyras.',
  'terms.use.title': 'Priimtinas naudojimas',
  'terms.use.body':
    'Įsipareigojate naudotis paslauga teisėtai ir nenaudoti jos neteisėtai veiklai, šlamštui, kenkėjiškų programų platinimui, trečiųjų šalių teisių pažeidimui ar bet kokiam naudojimui, keliančiam grėsmę paslaugos saugai ar pasiekiamumui kitoms organizacijoms.',
  'terms.availability.title': 'Pasiekiamumas ir pagalba',
  'terms.availability.body1':
    'Siekiame aukšto pasiekiamumo ir nuolat atliekame priežiūros darbus bei tobuliname paslaugą. Konkretūs paslaugų lygio įsipareigojimai (SLA), kai jie taikomi jūsų planui, nurodyti paslaugų sutartyje. Pagalba teikiama per',
  'terms.data.title': 'Jūsų duomenys',
  'terms.data.body1':
    'Turinys, kurį jūsų organizacija talpina rinkinyje, priklauso jūsų organizacijai. Rinkinys sukurtas remiantis atviraisiais standartais (OIDC, JMAP, CalDAV, CardDAV, WebDAV), todėl jūsų duomenys lieka perkeliami ir galite juos pasiimti. Asmens duomenų tvarkymas aprašytas puslapyje',
  'terms.ip.title': 'Intelektinė nuosavybė',
  'terms.ip.body':
    'Sytue One programinė įranga yra originalus Sytue kūrinys, saugomas atitinkamų intelektinės nuosavybės teisių. Jums lieka nuosavybė į turinį, kurį jūsų organizacija talpina rinkinyje; jį tvarkome tik paslaugai teikti.',
  'terms.liability.title': 'Atsakomybė',
  'terms.liability.body':
    'Dedame visas pastangas, kad paslauga būtų patikima ir saugi. Taikomos garantijos ir atsakomybės apribojimai nurodyti paslaugų sutartyje; niekas šiose sąlygose nepanaikina atsakomybės, kurios panaikinti įstatymai neleidžia.',
  'terms.contact.title': 'Sutartys ir kontaktai',
  'terms.contact.body1':
    'Dėl sutarties sąlygų, kainų organizacijai ar bet kokio klausimo apie paslaugą rašykite adresu',
  'terms.changes.title': 'Pakeitimai',
  'terms.changes.body':
    'Šias sąlygas galime atnaujinti, kad jos atspindėtų paslaugos ar teisės aktų pokyčius. Esant esminiams pakeitimams, apie juos pranešime paslaugos kanalais prieš jiems įsigaliojant ir atnaujinsime „paskutinio atnaujinimo“ datą.',
  'terms.law.title': 'Taikytina teisė',
  'terms.law.body':
    'Paslaugai taikoma Europos Sąjungos teisė ir teikėjo šalies teisė. Konkreti ginčų sprendimo jurisdikcija nustatoma galutiniame tekste. Nepriklausomai nuo to, išlieka jūsų teisė pateikti skundą duomenų apsaugos priežiūros institucijai.',
  'seo.org.description':
    'Sytue One: suverenus Europos produktyvumo rinkinys. Tapatybė, paštas, failai, dokumentai, kalendorius, pokalbiai, vaizdo skambučiai ir PaaS, 100 % ES.',
  'seo.org.slogan': 'Jūsų produktyvumas, suverenus ir europietiškas.',
  'seo.software.description':
    'Suverenus Europos produktyvumo rinkinys: paštas (JMAP), failai (WebDAV), bendri dokumentai, kalendorius (CalDAV), kontaktai (CardDAV), pokalbiai, vaizdo skambučiai ir PaaS. SSO su Sytue ID.',
  'seo.software.f1': 'Vienkartinis prisijungimas (SSO) + MFA/WebAuthn',
  'seo.software.f2':
    'Paštas, failai, dokumentai, kalendorius, kontaktai, pokalbiai ir vaizdo skambučiai',
  'seo.software.f3': 'Diegimo ir valdomų duomenų platforma',
  'seo.software.f4': 'Duomenys 100 % ES („Scaleway“)',
  'seo.offer.description': 'Nemokamas bandomasis laikotarpis, kortelės nereikia.',
  'seo.downloads.description':
    'Vietinės Sytue One rinkinio programos kompiuteriui (macOS, Windows, Linux) ir mobiliesiems (iOS, Android).',
  'home.pricing.personal.title': 'Nemokamas',
  'home.pricing.personal.freeStorage': '5 GB, vienam asmeniui',
  'home.pricing.personal.freeBody':
    'Paštas, failai, nuotraukos, dokumentai, kalendorius, kontaktai, užrašai ir kambariai. Be kortelės ir be laiko apribojimų.',
  'home.pricing.personal.note':
    'Jūsų organizacija yra jūsų el. paštas: be organizacijos administravimo ir be ERP, o visa kita – įskaitant Sytue Admin jūsų integracijoms ir perkėlimams.',
  'home.pricing.personal.cta': 'Sukurti nemokamą paskyrą',
  'grid.orgOnly': 'Tik organizacijoms',
  'grid.enDesarrollo': 'Kuriama',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB vienam naudotojui',
  'home.pricing.paidBody':
    'Organizacijoms, taip pat ir vienam asmeniui. Visas rinkinys su Sytue ERP ir Sytue Admin jūsų organizacijai valdyti.',
  'home.pricing.thirdParty':
    'Kiekvienas planas leidžia prijungti nuosavą saugyklą: Google Drive, Microsoft OneDrive, S3 talpyklą (Scaleway, OVH, Hetzner, Wasabi, MinIO), WebDAV serverį, pvz., Nextcloud ar Internxt, arba jūsų NAS jūsų patalpose — be atvirų prievadų, per Sytue tunelį. Už prijungtose saugyklose laikomų duomenų atsargines kopijas ir atkūrimą atsako klientas (jūs), o Sytue® atleidžiama nuo bet kokios atsakomybės už duomenų praradimą su Sytue® sujungtose saugyklose.',
  'home.pricing.soon': 'Netrukus',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB vienam naudotojui',
  'home.pricing.enterprise.support': 'Su „Premium“ pagalba.',
  'home.pricing.education': 'Švietimo įstaigoms: 50 % nuolaida Pro planui.',
};
