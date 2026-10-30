/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const lv: AppCatalog<typeof en> = {
  'skip.content': 'Pāriet uz saturu',
  'nav.products': 'Produkti',
  'nav.sovereignty': 'Suverenitāte',
  'nav.standards': 'Atvērtie standarti',
  'nav.pricing': 'Cenas',
  'footer.pricing': 'Cenas',
  'home.pricing.title': 'Trīs plāni, viss komplekts',
  'home.pricing.body':
    'Visos trijos tie paši produkti: mainās tikai krātuve vienam lietotājam. Nekādu apcirstu versiju.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'bez PVN',
  'home.pricing.per': '/Lietotājam/Mēn.',
  'home.pricing.cta': 'Sazināties ar pārdošanas nodaļu',
  'home.pricing.commitment.title': 'Apņemšanās uz mūžu',
  'home.pricing.commitment.body':
    'Cena vienam lietotājam ik pēc četriem gadiem pieaug par 0,50 € un ne vairāk: uz mūžu. Tas ir zināms jau pievienošanās dienā un nemainās.',
  'home.pricing.included.title': 'Viena vienīga versija',
  'home.pricing.included.body':
    'Nav plānu pēc funkcijām, nav līmeņu, nav apcirstu versiju: viss komplekts ir iekļauts cenā vienam lietotājam, ar vienādām funkcijām visiem.',
  'home.pricing.metered.title': 'Par ko maksā atsevišķi',
  'home.pricing.metered.body': 'Par papildu krātuvi: pēc lietojuma vai blokos.',
  'home.pricing.storage.title': 'Krātuve ir kopīga',
  'home.pricing.storage.body':
    'Katras personas kvota tiek pieskaitīta organizācijas kvotai: neizmantotā kvota paliek pārējai organizācijai. Kvotu vienam lietotājam var arī pielāgot organizācijas līmenī.',
  'nav.downloads': 'Lejupielādes',
  'nav.aria': 'Vietnes navigācija',
  'nav.ariaMobile': 'Mobilā navigācija',
  'nav.home': 'Sytue One — Sākums',
  'nav.openMenu': 'Atvērt izvēlni',
  'nav.closeMenu': 'Aizvērt izvēlni',
  'theme.toLight': 'Pārslēgt uz gaišo režīmu',
  'theme.toDark': 'Pārslēgt uz tumšo režīmu',
  'lang.label': 'Valoda',
  'cta.start': 'Sākt bez maksas',
  'cta.signIn': 'Pierakstīties',
  'cta.seeSuite': 'Apskatīt komplektu',
  'meta.home.title': 'Sytue One — suverēns Eiropas produktivitātes komplekts',
  'meta.home.description':
    'Sytue One: suverēns Eiropas produktivitātes komplekts. Identitāte, pasts, faili, dokumenti, kalendārs un tērzēšana, 100% ES un veidota uz atvērtiem standartiem.',
  'home.hero.lead': 'Jūsu produktivitāte,',
  'home.hero.accent': 'suverēna un eiropeiska',
  'home.hero.subtitle':
    'Pasts, faili, dokumenti, kalendārs, tērzēšana un videozvani — apvienoti vienā lietotnē un mitināti tikai ES.',
  'home.hero.note': 'Karte nav vajadzīga. Dati Scaleway (ES). Atbalsts migrācijā.',
  'home.privacy.title': 'Jūsu darbs pieder jums. Arī jūsu privātums.',
  'home.privacy.body': 'Mēs nepārdodam jūsu datus un neizmantojam tos reklāmas profilu veidošanai.',
  'home.privacy.anonymity':
    'Sytue Analytics aizsargā jūsu privātumu ar apkopotu statistiku, bez izsekošanas sīkdatnēm un ar pseidonimizētiem apmeklētāju identifikatoriem, kas mainās katru dienu un katrai vietnei ir atsevišķi.',
  'home.privacy.account':
    'Lai izmantotu komplektu, mums ir nepieciešami konta dati un piekļuves žurnāli, kas ļauj sniegt pakalpojumu un uzturēt tā drošību.',
  'home.privacy.link': 'Kā mēs aizsargājam jūsu privātumu',
  'home.stats.aria': 'Klātbūtne Eiropā',
  'home.stats.regions': 'ES reģioni (fr-par · nl-ams · pašu zona Madridē)',
  'home.stats.apps': 'Lietotnes aiz vienas identitātes',
  'home.stats.standards': 'Atvērtie standarti: OIDC, JMAP, CalDAV, CardDAV un WebDAV',
  'home.products.title': 'Viens komplekts, viena pieredze',
  'home.products.subtitle':
    'Visiem produktiem ir vienāds dizains, identitāte un apvalks. Tie izskatās un darbojas kā viena lietotne.',
  'home.integration.title': 'Integrācija, kādai jābūt komplektā',
  'home.integration.subtitle':
    'Viena reģistrācija atver visu. Automātiska lietotāju nodrošināšana (SCIM) no Sytue ID.',
  'home.integration.sso.title': 'Vienotā pieteikšanās',
  'home.integration.sso.body': 'Viena identitāte (Sytue ID) visam komplektam. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Kopīgs apvalks',
  'home.integration.shell.body':
    'Augšējā josla, lietotņu pārslēdzējs, meklēšana un vispārīgi paziņojumi katrā produktā.',
  'home.integration.cross.title': 'Darbības starp lietotnēm',
  'home.integration.cross.body':
    'Pievienojiet failu no Drive lietotnē Mail, pārvērtiet e-pasta ziņojumu par Calendar notikumu un kopīgojiet to Rooms, nepārtraucot darbu.',
  'home.sovereignty.title': 'Īsta suverenitāte',
  'home.sovereignty.body':
    'Tas nav mārketinga solījums, bet arhitektūra. Infrastruktūra darbojas tikai Eiropas reģionos, un viss ir veidots uz atvērtiem, auditējamiem protokoliem bez sētas durvīm.',
  'home.sovereignty.link': 'Kāpēc atvērtie standarti',
  'home.sovereignty.eu.title': '100% ES',
  'home.sovereignty.eu.body':
    'Visa infrastruktūra ES: Scaleway (fr-par / nl-ams) un pašu zona Madridē. Personas datiem nekādu ASV pakalpojumu sniedzēju.',
  'home.sovereignty.gdpr.title': 'Atbilstība VDAR',
  'home.sovereignty.gdpr.body':
    'Datu apstrādes līgums, apstrādes darbību reģistrs, datu subjektu pieprasījumu procedūra un glabāšanas termiņi. Šifrēts pārsūtīšanas un glabāšanas laikā (TLS 1.3).',
  'home.sovereignty.lockin.title': 'Bez piesaistes',
  'home.sovereignty.lockin.body':
    'Veidots uz atvērtiem standartiem: OIDC, JMAP, CalDAV, CardDAV un WebDAV. Jūsu dati paliek pārnesami.',
  'home.standards.overline': 'Sytue oriģināls darbs',
  'home.standards.title': 'Veidots uz atvērtiem standartiem',
  'home.standards.body':
    'Katrs Sytue produkts ir oriģināls darbs, rakstīts Go un Astro + Preact uz atvērtiem protokoliem (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Bez trešo pušu dzinējiem: jūsu dati ir pārnesami, un jūsu komplekts ir suverēns.',
  'home.cta.title': 'Atgūstiet kontroli pār savu darbu',
  'home.cta.body':
    'Sāciet ar savu organizāciju jau šodien. Atbalsts migrācijā no Google Workspace vai Microsoft 365.',
  'home.cta.primary': 'Izveidot organizāciju',
  'home.cta.secondary': 'Sazināties ar pārdošanas nodaļu',
  'grid.discover': 'Uzzināt vairāk',
  'product.mail.blurb': 'Privāts Eiropas pasts ar JMAP — bez izsekotājiem un reklāmām.',
  'product.drive.blurb': 'Faili un sadarbība ar jūsu zīmolu.',
  'product.docs.blurb': 'Dokumenti, izklājlapas un slaidi ar kopīgu rediģēšanu reāllaikā.',
  'product.calendar.blurb': 'CalDAV kalendārs, kas saistīts ar pārējo komplektu.',
  'product.contacts.blurb': 'CardDAV kontakti, kopīgoti visā organizācijā.',
  'product.notes.blurb':
    'Piezīmju grāmatiņas, kurās ietilpst viss: teksts, faili, notikumi, veseli dokumenti.',
  'product.photos.blurb': 'Fotoattēlu galerija un redaktors: oriģināli Drive, labojumi kā recepte.',
  'product.rooms.blurb': 'Kanāli, ziņojumi un komandas videozvani vienuviet.',
  'product.cast.blurb':
    'Video pēc pieprasījuma un tiešraides: augšupielādējiet, raidiet un kopīgojiet, ES.',
  'product.pages.blurb':
    'Vietnes un tiešsaistes veikali jūsu uzņēmumam, izstrādāti pēc jūsu vēlmēm un publicēti ES.',
  'product.bases.blurb': 'Kopīgas relāciju tabulas klientiem, pasūtījumiem vai krājumiem, ES.',
  'product.cloud.blurb': 'Izvietošanas un pārvaldītu datu platforma ES.',
  'product.id.blurb': 'Viena identitāte (SSO + MFA) ar Sytue ID.',
  'product.one.blurb':
    'Komplekta sākumlapa: lietotnes, ātrās darbības, nesenās darbības un paziņojumi.',
  'product.sheets.blurb':
    'Izklājlapas ar formulām, ārējiem datiem un diagrammām, ko rediģē daudzi.',
  'product.slides.blurb': 'Prezentācijas ar jūsu zīmolu, motīviem un runātāja skatu.',
  'product.pdf.blurb':
    'Lasiet, anotējiet, parakstiet un kārtojiet PDF failus, tiem nepametot Eiropu.',
  'product.console.blurb': 'Organizācijas administrēšana: cilvēki, piekļuve, atbilstība un audits.',
  'product.erp.blurb':
    'Divdesmit četri moduļi un divkāršā grāmatvedība, ko datubāze neļauj izjaukt.',
  'productPage.back': 'Viss komplekts',
  'productPage.features': 'Viss, ko dara {name}',
  'productPage.featuresPlanned': 'Ko darīs {name}',
  'productPage.builtOn': 'Veidots uz {standard}',
  'productPage.fromScratch':
    'Docus, Calcus, Diapos un CertiPDF redaktors ir uzrakstīts no nulles: pašu Sytue dzinējs, nevis trešās puses integrācija.',
  'productPage.shot': '{name} galvenais ekrāns tieši tāds, kāds tas redzams pēc pierakstīšanās.',
  'productPage.shotFeatures': '{name} darbībā, ar funkcijām ekrānā.',
  'productPage.open': 'Atvērt {name}',
  'productPage.enDesarrollo': 'Šis produkts ir izstrādē: tas vēl nav pieejams.',
  'productPage.included':
    'Iekļauts Sytue One: viena identitāte (Sytue ID) un viena reģistrācija dod piekļuvi visiem produktiem. Dati 100% ES (Scaleway), pārnesami ar atvērtajiem standartiem.',
  'product.mail.tagline': 'Privāts Eiropas pasts bez izsekotājiem un reklāmām.',
  'product.mail.standard': 'JMAP (atvērts standarts)',
  'product.mail.p1': 'Ātra iesūtne ar pavedieniem, iezīmēm, filtriem un meklēšanu.',
  'product.mail.p2': 'Failu pievienošana tieši no Sytue Drive.',
  'product.mail.p3': 'E-pasta ziņojuma pārvēršana par Calendar notikumu.',
  'product.mail.p4':
    'Šifrēts pārsūtīšanas laikā (TLS 1.3). Sytue infrastruktūra atrodas ES. Ārējo pakalpojumu atrašanās vieta ir atkarīga no to sniedzēja.',
  'product.drive.tagline': 'Jūsu faili un sadarbība ar jūsu organizācijas zīmolu.',
  'product.drive.standard': 'WebDAV (atvērts standarts)',
  'product.drive.p1':
    'Augšupielāde, kārtošana un kopīgošana ar kontrolētām saitēm (atļaujas un derīguma termiņš).',
  'product.drive.p2': 'Dokumentu un attēlu priekšskatījums tieši lapā.',
  'product.drive.p3': 'Pievienošana Mail bez faila dublēšanas.',
  'product.drive.p4': 'S3 krātuve Eiropas reģionos.',
  'product.docs.tagline': 'Dokumenti, izklājlapas un slaidi ar kopīgu rediģēšanu reāllaikā.',
  'product.docs.standard': 'Kopīga rediģēšana reāllaikā',
  'product.docs.p1': 'Rakstiet kopā ar klātbūtni reāllaikā un tūlītējām izmaiņām.',
  'product.docs.p2': 'Izklājlapas ar formulām un nosacījumformatēšanu.',
  'product.docs.p3': 'Prezentācijas ar jūsu zīmolu.',
  'product.docs.p4': 'Viss saglabāts jūsu Eiropas Drive.',
  'product.calendar.tagline': 'Jūsu kalendārs, saistīts ar pārējo komplektu.',
  'product.calendar.standard': 'CalDAV (atvērts standarts)',
  'product.calendar.p1': 'Dienas, nedēļas un mēneša skati.',
  'product.calendar.p2': 'Atkārtoti notikumi un atgādinājumi.',
  'product.calendar.p3': 'Notikumu izveide tieši no e-pasta ziņojuma Mail.',
  'product.calendar.p4': 'Darbojas ar jebkuru CalDAV klientu.',
  'product.contacts.tagline': 'Jūsu organizācijas kontakti, kopīgoti un pārnesami.',
  'product.contacts.standard': 'CardDAV (atvērts standarts)',
  'product.contacts.p1': 'Kopīga adrešu grāmata visai organizācijai.',
  'product.contacts.p2': 'Importēšana un eksportēšana vCard formātā.',
  'product.contacts.p3': 'Dublikātu apvienošana bez datu zaudēšanas.',
  'product.contacts.p4': 'Darbojas ar jebkuru CardDAV klientu.',
  'product.notes.tagline': 'Viss, ko vēlaties saglabāt, vienuviet un jūsu rokās.',
  'product.notes.standard': 'Savs modelis uz atvērtiem standartiem (sava REST API)',
  'product.notes.p1': 'Ligzdotas piezīmju grāmatiņas uz brīva audekla, kolonnās vai sarakstā.',
  'product.notes.p2':
    'Jebkādas piezīmes: teksts, uzdevumi, saites, faili, Calendar notikumi, Docs dokumenti.',
  'product.notes.p3': 'Pilnīga šifrēšana: ieejas frāze nekad nepamet jūsu pārlūku.',
  'product.notes.p4': 'Jūs izlemjat, kas nonāk meklēšanā — un kas nekad.',
  'product.rooms.tagline': 'Kanāli, ziņojumi un komandas videozvani vienuviet.',
  'product.rooms.standard': 'Ziņojumi un zvani reāllaikā',
  'product.rooms.p1': 'Kanāli pa komandām vai projektiem.',
  'product.rooms.p2': 'Videozvans iebūvēts katrā kanālā.',
  'product.rooms.p3': 'Pieminējumi un paziņojumi vienoti visā komplektā.',
  'product.rooms.p4': 'Meklējama vēsture no jebkuras Sytue One vietas.',
  'product.cloud.p4': 'Pielāgoti domēni ar automātisku TLS.',
  'product.cloud.p3': 'Pārvaldīta S3 krātuve un funkcijas.',
  'product.cloud.p2': 'Postgres datubāze ar automātisku REST API.',
  'product.cloud.p1': 'Priekšskatījums katram PR un tūlītēja atgriešana.',
  'product.cloud.standard': 'Kubernetes vietējā PaaS',
  'product.cloud.tagline': 'Jūsu kods izvietots un dati pārvaldīti Eiropas Savienībā.',
  'product.id.tagline': 'Viena identitāte visam komplektam. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (atvērts standarts)',
  'product.id.p1': 'Vienotā pieteikšanās visos produktos.',
  'product.id.p2': 'MFA un piekļuves atslēgas (WebAuthn).',
  'product.id.p3': 'Automātiska lietotāju nodrošināšana (SCIM) no jūsu direktorija.',
  'product.id.p4': 'Profils un drošība vienuviet.',
  'product.one.tagline': 'Viss Sytue One viena klikšķa attālumā, pa tām pašām durvīm.',
  'product.one.standard': 'Viena pierakstīšanās (Sytue ID) visam komplektam',
  'product.one.p1': 'Palaidējs ar visām jūsu lietotnēm un tām, ko pievieno jūsu organizācija.',
  'product.one.p2': 'Meklēšana visā komplektā (⌘K) no jebkura ekrāna.',
  'product.one.p3':
    'Šodienas grafiks un nelasītais pasts sākumlapā, blakus komplekta jaunumiem un jūsu paziņojumiem.',
  'product.one.p4': 'Pievienojiet savas ierīces un lejupielādējiet vietējās lietotnes.',
  'product.sheets.tagline': 'Jēgpilni skaitļi tabulā, kas strādā kopā ar jums.',
  'product.sheets.standard': 'ODF/OOXML formāti un kopīga rediģēšana reāllaikā',
  'product.sheets.p1': 'Formulas, nosacījumformatēšana un datu validācija.',
  'product.sheets.p2': 'Ienesiet ārējus datus un veiciet vaicājumus, nepametot tabulu.',
  'product.sheets.p3': 'Diagrammas, mini diagrammas, sagriezumi un datu tabulas.',
  'product.sheets.p4': 'Eksports uz ODS un XLSX; Excel failu atvēršana.',
  'product.slides.tagline': 'Stāstiet stāstu; slaids tur līdzi.',
  'product.slides.standard': 'ODF/OOXML formāti un kopīga rediģēšana reāllaikā',
  'product.slides.p1': 'Prezentāciju motīvi ar jūsu organizācijas identitāti.',
  'product.slides.p2': 'Runātāja skats ar piezīmēm un nākamo slaidu.',
  'product.slides.p3': 'Attēli, formas, diagrammas un SmartArt.',
  'product.slides.p4': 'Eksports uz ODP, PPTX un PDF; PowerPoint failu atvēršana.',
  'product.pdf.tagline': 'PDF no sākuma līdz beigām jūsu pārlūkā.',
  'product.pdf.standard': 'PDF (ISO 32000) ar standarta anotācijām un veidlapām',
  'product.pdf.p1': 'Iezīmēšana, pasvītrošana, komentāri un zīmēšana dokumentā.',
  'product.pdf.p2': 'Veidlapas: lauku aizpildīšana vai savu veidošana.',
  'product.pdf.p3': 'Apvienošana, izgūšana, pagriešana, numurēšana un ūdenszīme.',
  'product.pdf.p4':
    'Parakstu panelis, neatgriezeniska aizklāšana un ar paroli aizsargāti dokumenti.',
  'product.console.tagline': 'Pārvaldiet komplektu no viena ekrāna.',
  'product.console.standard': 'SCIM, SAML un OpenID Connect (atvērtie standarti)',
  'product.console.p1': 'Personas, grupas (arī dinamiskās) un lomas.',
  'product.console.p2': 'SSO ar jūsu pakalpojumu sniedzēju un nodrošināšana ar SCIM.',
  'product.console.p3':
    'Nosacījumpiekļuves noteikumi, ierīces un priviliģēta piekļuve ar apstiprinājumu.',
  'product.console.p4': 'Audita žurnāls, ko var eksportēt uz jūsu SIEM, un juridiskā aizturēšana.',
  'product.erp.tagline': 'Viss ERP ar modeļu katalogu kā datu failu.',
  'product.erp.standard': 'Divkāršā grāmatvedība, JSON modeļu katalogs un viena REST API',
  'product.erp.p1':
    'Divdesmit četri moduļi un trīsdesmit seši modeļi: kontakti, pārdošana, iepirkumi, krājumi, ražošana, rēķini, grāmatvedība, projekti un personāls.',
  'product.erp.p2':
    'Moduļa pievienošana nozīmē JSON faila rakstīšanu: bez migrācijas, bez jauna maršruta un bez darba pie saskarnes.',
  'product.erp.p3':
    'Naudas summas tiek glabātas veselos centos, nekad peldošajā komatā, un bilanci nodrošina datubāze.',
  'product.erp.p4':
    'Iegrāmatotie ieraksti ir neaizskarami: tos labo ar citu ierakstu, un tieši tas atšķir virsgrāmatu no izklājlapas.',
  'product.one.f1':
    'Palaidējs ar komplekta lietotnēm un trešo pušu lietotnēm, ko pievieno jūsu organizācija.',
  'product.one.f2':
    'Ātrās darbības: rakstīt e-pastu, jauns dokuments, augšupielādēt Drive, izveidot notikumu.',
  'product.one.f3': 'Nesen komplektā: pēdējie vienumi no Drive, Docus, Mail vai Rooms.',
  'product.one.f4': 'Globālā meklēšana ar ⌘K vai Ctrl K, ar rezultātiem no visām lietotnēm.',
  'product.one.f5': 'Paziņojumu centrs un klātbūtne, kopīgi visam komplektam.',
  'product.one.f6': 'Ierīču pievienošana: profils iPhone un Mac, Android un datoram, divos soļos.',
  'product.one.f7': 'Vietējo datora un mobilo lietotņu lejupielādes.',
  'product.one.f8': 'Sagaidīšanas vednis, kas sagatavo organizāciju bez rokasgrāmatām.',
  'product.mail.f1': 'Sarunu pavedieni ar lasīšanu, atbildēšanu un pārsūtīšanu ar vienu klikšķi.',
  'product.mail.f2':
    'Mapes, krāsainas iezīmes un meklēšana ar operatoriem (from:, has:attachment, is:unread…).',
  'product.mail.f3': 'Automātiski noteikumi, kas ienākošos ziņojumus arhivē, iezīmē vai pārvieto.',
  'product.mail.f4': 'Ieplānota sūtīšana un “Atsaukt sūtīšanu”.',
  'product.mail.f5': 'Paraksti, adrešu aizstājvārdi un automātiska prombūtnes atbilde.',
  'product.mail.f6': 'Kopīgotas un deleģētas pastkastes ar lasīšanas un sūtīšanas atļaujām.',
  'product.mail.f7':
    'Formatēta teksta redaktors ar pielikumiem no Sytue Drive bez failu dublēšanas.',
  'product.mail.f8':
    'Attālais saturs pēc noklusējuma bloķēts: neviens neuzzina, kad atverat e-pasta ziņojumu.',
  'product.mail.f9':
    'Parakstīts pasts (PGP un S/MIME) tiek atpazīts un atzīmēts; TLS pārsūtīšanas laikā.',
  'product.mail.f10':
    'Piekļuve arī no Thunderbird, Outlook, Apple Mail vai tālruņa ar lietotņu parolēm.',
  'product.mail.f11':
    'Adresāti kā žetoni, Contacts adrešu grāmata viena klikšķa attālumā un brīdinājums, ja uzrakstāt “pielikumā”, bet neko nepievienojat.',
  'product.mail.f12':
    'Noteikumi tieši no atvērta ziņojuma, atkritnes iztukšošana vienā reizē un īsinājumtaustiņi ar “?”.',
  'product.drive.f1': 'Mans disks, komandu diski, kopīgoti ar mani, nesenie, izlase un atkritne.',
  'product.drive.f2':
    'Augšupielāde ar vilkšanu; mapes, pārvietošana, kopēšana, pārdēvēšana un vairāku vienumu atlase.',
  'product.drive.f3':
    'Kopīgošanas saites ar diviem līmeņiem (skatīt vai rediģēt), paroli un derīguma termiņu.',
  'product.drive.f4': 'Versiju vēsture ar atjaunošanu.',
  'product.drive.f5': 'Dokumentu, attēlu un PDF priekšskatījums bez lejupielādes.',
  'product.drive.f6':
    'Biroja dokumentu atvēršana un rediģēšana Docus, Calcus un Diapos; PDF – CertiPDF.',
  'product.drive.f7': 'Sensitivitātes iezīmes, kas bloķē publiskas saites tur, kur tām nav vietas.',
  'product.drive.f8': 'Kopīgošana Rooms kanālā vai pievienošana Mail bez faila dublēšanas.',
  'product.drive.f9': 'WebDAV piekļuve un datora klients, lai redzētu failus savā datorā.',
  'product.drive.f10': 'S3 krātuve Eiropas reģionos, kvota vienmēr redzama.',
  'product.docs.f1': 'Kopīga rediģēšana reāllaikā ar klātbūtni un bez savstarpējas traucēšanas.',
  'product.docs.f2': 'Lente ar cilnēm: Sākums, Ievietošana, Atsauces, Sūtījumi un Pārskatīšana.',
  'product.docs.f3': 'Tabulas, attēli, formas, SmartArt, diagrammas, simboli un ūdenszīmes.',
  'product.docs.f4': 'Atsauces: satura rādītājs, vēres, citāti un bibliogrāfija.',
  'product.docs.f5': 'Sūtījumi: datu sapludināšana vēstulēm un uzlīmēm.',
  'product.docs.f6': 'Komentāru pavedieni, izmaiņu pārskatīšana un versijas.',
  'product.docs.f7':
    'Neobligāts MI palīgs pārrakstīšanai, tulkošanai vai kopsavilkumam, tikai ja jūsu organizācija to iespējo.',
  'product.docs.f8': 'Eksports uz ODT, DOCX un PDF; Word failu atvēršana no Drive.',
  'product.docs.f9': 'Galvenes, kājenes, lappušu numerācija un atkārtoti izmantojami bloki.',
  'product.docs.f10': 'Automātiska saglabāšana jūsu Eiropas Drive ar vēsturi.',
  'product.docs.f11':
    'Lejupielāde no sākumlapas PDF, HTML, Markdown, OpenDocument vai Microsoft Office formātā.',
  'product.sheets.f1': 'Formulas ar kontekstuālu palīdzību un tūlītēju pārrēķinu.',
  'product.sheets.f2':
    'Nosacījumformatēšana, datu validācija un reģionālajiem iestatījumiem atbilstoši skaitļu formāti.',
  'product.sheets.f3': 'Dažādu veidu diagrammas, mini diagrammas un sagriezumi.',
  'product.sheets.f4': 'Datu tabulas ar kopsummām un kārtošanu; rakurstabula.',
  'product.sheets.f5': 'Ārējie dati: pievienojiet avotu un veiciet vaicājumus no tabulas.',
  'product.sheets.f6': 'Risinātājs vērtības atrašanai, kas atbilst mērķim.',
  'product.sheets.f7': 'Makro atkārtotu uzdevumu automatizēšanai.',
  'product.sheets.f8': 'Sadarbība reāllaikā ar klātbūtni un komentāriem.',
  'product.sheets.f9': 'Filtri, sagriezumi un meklēšana tabulā.',
  'product.sheets.f10': 'Eksports uz ODS, XLSX un PDF; Excel failu atvēršana no Drive.',
  'product.slides.f1': 'Motīvi un paletes atbilstoši jūsu organizācijas zīmolam.',
  'product.slides.f2': 'Audekls ar formām, attēliem, diagrammām, SmartArt un savienotājiem.',
  'product.slides.f3': 'Prezentēšana pilnekrāna režīmā ar runātāja skatu un piezīmēm.',
  'product.slides.f4': 'Sadarbība reāllaikā ar klātbūtni un komentāriem.',
  'product.slides.f5': 'Slaidi ar foniem, plakātiem un izkārtojumiem.',
  'product.slides.f6': 'Eksports uz ODP, PPTX un PDF.',
  'product.slides.f7': 'PowerPoint failu atvēršana no Drive.',
  'product.slides.f8': 'Automātiska saglabāšana ar versijām.',
  'product.pdf.f1':
    'Ātrs skatītājs ar struktūru, sīktēliem, meklēšanu un tālummaiņu pēc lappuses vai platuma.',
  'product.pdf.f2':
    'Anotācijas: iezīmēšana, pasvītrošana, pārsvītrošana, komentāri, zīmējumi, formas, bultas un zīmogi.',
  'product.pdf.f3': 'Teksta lodziņi, mērījumi un apgabala momentuzņēmumi.',
  'product.pdf.f4': 'Veidlapas: esošo lauku aizpildīšana vai savu lauku pievienošana.',
  'product.pdf.f5': 'Dokumentu parakstīšanas panelis.',
  'product.pdf.f6': 'Neatgriezeniska aizklāšana: atzīmētais pazūd no faila, ne tikai no skata.',
  'product.pdf.f7':
    'Lappuses: apvienošana, izgūšana, dzēšana, pagriešana, numurēšana (arī Bates), galvenes un kājenes.',
  'product.pdf.f8': 'Ūdenszīme, fons un rediģējami metadati.',
  'product.pdf.f9': 'Ar paroli aizsargāti dokumenti un atļaujas.',
  'product.pdf.f10': 'Nolasīšana balsī un komentāru pavedieni.',
  'product.calendar.f1':
    'Mēneša, nedēļas, dienas un darba kārtības skati ar mazo kalendāru un pogu “Šodien”.',
  'product.calendar.f2':
    'Ikdienas, iknedēļas vai ikmēneša atkārtojums ar beigām; rediģēt tikai šo, šo un nākamos vai visus.',
  'product.calendar.f3': 'Atgādinājumi katram notikumam un paziņojumi komplektā.',
  'product.calendar.f4': 'Ielūgumi ar atbildi (piedalīšos, varbūt, nē) un izsekošanu.',
  'product.calendar.f5': '“Atrast laiku”, kas brīvs visām uzaicinātajām personām.',
  'product.calendar.f6': 'Rezervējamas telpas un resursi ar pieejamību.',
  'product.calendar.f7': 'Kopīgoti kalendāri ar atļaujām un ar jums kopīgotie kalendāri.',
  'product.calendar.f8': 'Ārēju kalendāru (ICS) abonēšana un vairāki krāsās iezīmēti kalendāri.',
  'product.calendar.f9': 'Notikuma izveide no Mail ziņojuma vai piezīmes.',
  'product.calendar.f10':
    'Darbojas ar jebkuru CalDAV klientu: Apple Calendar, Thunderbird un citiem.',
  'product.calendar.f11':
    'Notikuma dublēšana vai kopēšana uz jūsu kalendāru un lejupielāde kā .ics personām ārpus Sytue.',
  'product.contacts.f1': 'Adrešu grāmatas, tostarp jūsu organizācijas kopīgā.',
  'product.contacts.f2':
    'Pilna kartīte: tālruņi, e-pasti, adreses, organizācija, dzimšanas diena, piezīmes un foto.',
  'product.contacts.f3': 'Iezīmes un izlase grupēšanai un atrašanai.',
  'product.contacts.f4': 'Dublikātu noteikšana un apvienošana bez datu zaudēšanas.',
  'product.contacts.f5': 'vCard un CSV importēšana un eksportēšana; viena kontakta eksportēšana.',
  'product.contacts.f6': 'Lielapjoma darbības un atkritne ar atkopšanu.',
  'product.contacts.f7': 'Tūlītēja meklēšana un automātiska pabeigšana, rakstot Mail.',
  'product.contacts.f8': 'Darbojas ar jebkuru CardDAV klientu.',
  'product.contacts.f9': 'Gaidāmās dzimšanas dienas no adrešu grāmatas, izceļot šī mēneša.',
  'product.contacts.f10':
    'Sapulces izveide ar kontaktu vai kontakta e-pasta adreses kopēšana, nepametot kartīti.',
  'product.notes.f1':
    'Ligzdotas piezīmju grāmatiņas ar ikonu un krāsu; trīs izkārtojumi: brīvs, kolonnas un saraksts.',
  'product.notes.f2':
    'Divpadsmit piezīmju veidi: teksts, uzdevumi, saites, krāsa, attēls un atsauces uz Drive, Docus, Calendar, Rooms un citām piezīmēm.',
  'product.notes.f3':
    'Redaktors ar formatēšanu (treknraksts, saraksti, saites), kas saglabā, aizejot.',
  'product.notes.f4':
    'Birkas, piespraušana, dublēšana, pārvietošana starp grāmatiņām, grupēšana un vilkšana uz audekla.',
  'product.notes.f5':
    'Redzamība katrai grāmatiņai: privāta, personiska, organizācijas vai publiska saite.',
  'product.notes.f6': 'Šifrēšana pārlūkā sensitīvām grāmatiņām: Sytue tās nevar izlasīt.',
  'product.notes.f7': 'Jūs izlemjat, kuras grāmatiņas nonāk komplekta meklēšanā.',
  'product.notes.f8': 'Atpakaļsaites: kuras piezīmes norāda uz šo.',
  'product.notes.f9':
    'Visas grāmatiņas eksports uz HTML, PDF, Markdown un citiem formātiem; kopiju importēšana.',
  'product.notes.f10': 'Piezīmes kopīgošana Rooms kanālā; piezīmju izveide, nometot vai ielīmējot.',
  'product.notes.f11':
    'Tabulas redaktorā un Markdown saīsnes rakstīšanas laikā: “- ”, “# ”, “> ” un “```”.',
  'product.notes.f12':
    'Piezīmes pārvietošana uz citu grāmatiņu, vienas piezīmes eksports un grāmatiņas kārtošana pēc izvēles.',
  'product.photos.tagline': 'Jūsu fotoattēli sakārtoti un pēc jūsu gaumes, nepametot Eiropu.',
  'product.photos.standard': 'Oriģināli Sytue Drive; rediģēšanas dzinējs jūsu pārlūkā',
  'product.photos.p1': 'Laika josla pa mēnešiem ar meklēšanu pēc nosaukuma.',
  'product.photos.p2': 'Albumi ar vāku; pilnekrāna skatītājs.',
  'product.photos.p3':
    'Redaktors gaismai, krāsām, pagriešanai, spoguļošanai un apgriešanai reāllaikā.',
  'product.photos.p4': 'Katrs labojums ir recepte: oriģināls netiek aizskarts.',
  'product.photos.f1':
    'Laika josla, grupēta pa mēnešiem, ar meklēšanu pēc nosaukuma un asiem sīktēliem.',
  'product.photos.f2':
    'Albumi: izveide, pārdēvēšana, vāka izvēle, pievienošana no skatītāja un noņemšana.',
  'product.photos.f3': 'Pilnekrāna skatītājs ar navigāciju ar tastatūru.',
  'product.photos.f4':
    'Redaktors ar četriem slīdņiem (ekspozīcija, kontrasts, piesātinājums, temperatūra), pagriešanu un spoguļošanu.',
  'product.photos.f5':
    'Apgriešana ar vilkšanu un īsinājumtaustiņi; GPU priekšskatījums ar CPU rezerves variantu.',
  'product.photos.f6':
    'Katrs labojums ir atsevišķi glabāta recepte: oriģināls Drive nekad netiek mainīts.',
  'product.photos.f7':
    'Eksports pilnā izšķirtspējā, renderēts serverī ar tām pašām formulām kā priekšskatījumā.',
  'product.photos.f8': 'Atkritne ar atjaunošanu.',
  'product.photos.f9': 'Oriģināli atrodas jūsu Sytue Drive; atvasinājumi ir atjaunojama kešatmiņa.',
  'product.photos.f10': 'EXIF dati tiek nolasīti no paša faila; viss Eiropas Savienībā.',
  'product.photos.f11':
    'Vairāku attēlu atlase pa mēnešiem: uz jaunu albumu, uz atkritni vai lejupielāde vienā reizē.',
  'product.photos.f12':
    'Slaidrāde, pilns izmērs, pagriešana no skatītāja un apgriešana ar fiksētu malu attiecību.',
  'product.rooms.f1': 'Publiski un privāti kanāli kategorijās; tiešie ziņojumi un grupas.',
  'product.rooms.f2':
    'Pavedieni, reakcijas, pieminējumi, piespraustie un saglabātie ziņojumi un melnraksti.',
  'product.rooms.f3':
    'Formatēšana ar joslu “Aa” un īsinājumtaustiņiem: treknraksts, slīpraksts, kods, citāti, saraksti un saites.',
  'product.rooms.f4': 'Pielikumi, attēli, GIF un pielāgotas emocijzīmes; faili no Drive.',
  'product.rooms.f5': 'Ieplānota sūtīšana, prioritāte, izlasīšanas apstiprinājums un “redzēja”.',
  'product.rooms.f6': 'Slīpsvītras komandas un kartītes: aptaujas un piešķirami uzdevumi.',
  'product.rooms.f7': 'Videozvani kanālā: kamera, ekrāns, pacelta roka, tērzēšana un ierakstīšana.',
  'product.rooms.f8': 'Ieplānotas sapulces un uzaicinājuma saites.',
  'product.rooms.f9': 'Roboti, tīmekļa āķi un integrācijas ar citiem rīkiem.',
  'product.rooms.f10': 'Klātbūtne un pielāgots statuss; paziņojumu iestatījumi katram kanālam.',
  'product.rooms.f11': 'Meklēšana vēsturē, arī no visa komplekta.',
  'product.rooms.f12':
    'Reāllaiks ar WebSocket, ar atkārtotu savienošanu un bez pazaudētiem ziņojumiem.',
  'product.rooms.f13':
    'Ātra kanālu pārslēgšana ar Ctrl+G, visa atzīmēšana kā izlasīta un īsinājumtaustiņi ar “?”.',
  'product.cast.tagline':
    'Jūsu video un jūsu tiešraides, pēc pieprasījuma un reāllaikā, nepametot Eiropu.',
  'product.cast.standard': 'HLS atskaņošana pārlūkā; RTMP ievade no jebkuras raidīšanas programmas',
  'product.cast.p1':
    'Augšupielādējiet video, un tas tiek pārkodēts vairākās kvalitātēs jebkuram ekrānam.',
  'product.cast.p2': 'Raidiet tiešraidē no raidīšanas programmas, no pārlūka vai no lietotnes.',
  'product.cast.p3':
    'Četri redzamības līmeņi: publisks, nesarakstīts, organizācija vai konkrētas personas.',
  'product.cast.p4': 'Tiešraides tērzēšana un komentāri, ar publicētāja pamata moderāciju.',
  'product.cast.f1': 'Augšupielādējiet video līdz 8 GiB; oriģināls tiek saglabāts.',
  'product.cast.f2': 'Adaptīva atskaņošana trīs kvalitātēs; pārlūks izvēlas labāko.',
  'product.cast.f3':
    'Tiešraide no raidīšanas programmas caur RTMP, ar straumes atslēgu katrai personai.',
  'product.cast.f4':
    'Tiešraidē no pārlūka vai lietotnes: kamera un mikrofons. Ekrāna kopīgošana atbalstītos pārlūkos, darbvirsmas sistēmās un Android; nav pieejama iOS/iPadOS lietotnē.',
  'product.cast.f5': 'Tiešraižu ieraksti automātiski kļūst par video pēc pieprasījuma.',
  'product.cast.f6': 'Publiska sākumlapa un kanāls katrai organizācijai, indeksējami.',
  'product.cast.f7':
    'Nesarakstītas saites, video tikai organizācijai vai piekļuve konkrētām personām pēc @lietotājvārda.',
  'product.cast.f8':
    'Tiešraides tērzēšana un komentāri pavedienos, ko moderē publicētājs un organizācijas administrācija.',
  'product.cast.f9': 'Iegulstams atskaņotājs un oEmbed publiskiem un nesarakstītiem video.',
  'product.cast.f10': 'Krātuve tiek ieskaitīta jūsu Sytue Drive kvotā; viss Eiropas Savienībā.',
  'product.cast.f11':
    '“Augšupielādēt pakalpojumā Cast” no Sytue Drive: video kopija nonāk Cast, bet fails paliek Drive.',
  'product.pages.tagline':
    'Jūsu uzņēmuma vietne un tās veikals, izveidoti vizuālajā redaktorā un publicēti, neatstājot Eiropu.',
  'product.pages.standard':
    'Statisks, pieejams HTML un CSS bez trešo pušu sīkdatnēm; eksports uz Astro',
  'product.pages.p1':
    'Vizuālais redaktors ar rāmjiem, kas paši pielāgojas datoram, planšetdatoram un tālrunim.',
  'product.pages.p2': 'Desmit nozaru veidnes ar īstiem tekstiem un publiskā īpašuma fotogrāfijām.',
  'product.pages.p3': 'Veikals ar katalogu, grozu un maksājumiem uz jūsu uzņēmuma kontu.',
  'product.pages.p4':
    'Jūsu pašu domēns ar automātisku sertifikātu vai adrese vietnē sites.sytue.eu.',
  'product.pages.f1':
    'Uz rāmjiem balstīts vizuālais redaktors ar automātisku izkārtojumu, atkārtoti izmantojamiem komponentiem un gataviem blokiem.',
  'product.pages.f2':
    'Trīs pārtraukuma punkti (dators, planšetdators un tālrunis), katram savi pielāgojumi.',
  'product.pages.f3': 'Apmeklētāju analītika bez sīkdatnēm, vietnes informācijas panelī.',
  'product.pages.f4':
    'Daudzvalodu vietnes ar adresēm katrā valodā, hreflang un tulkojumu ar Sytue AI palīdzību.',
  'product.pages.f5':
    'Attēli no jūsu datora, Sytue Drive vai Sytue Photos, ar alternatīvo tekstu, ko pieprasa ievietošanas brīdī.',
  'product.pages.f6':
    'Veikals ar variantiem, krājumiem, PVN par katru produktu un maksājumu ar Mollie, Adyen, Redsys vai Stripe.',
  'product.pages.f7':
    'Pasūtījumi ar vienkāršotu rēķinu, ko izraksta Sytue ERP, un atteikšanās tieši no pašas vietnes.',
  'product.pages.f8':
    'Veidlapas ar piekrišanu, ziņojumu iesūtne un paziņojumi jūsu organizācijas cilvēkiem.',
  'product.pages.f9':
    'Nosauktas versijas, priekšskatījums aiz slepenas saites un atgriešanās pie jebkuras iepriekšējas publikācijas.',
  'product.pages.f10':
    'Koda eksports uz Astro, React, Next.js un vietējām lietotnēm, kā arī kataloga eksports uz Shopify, WooCommerce un citām platformām.',
  'product.bases.tagline':
    'Saistītas tabulas klientu, pasūtījumu vai krājumu uzskaitei, ar atļaujām un katras izmaiņas žurnālu.',
  'product.bases.standard': 'Atvērts CSV un JSON; shēmu un attiecības nodrošina pati datubāze',
  'product.bases.p1':
    'Klienti, pasūtījumi vai krājumi saistītās tabulās, nezinot neko par datubāzēm.',
  'product.bases.p2': 'Kopīgojiet ar savu organizāciju ar izvēlēto lomu.',
  'product.bases.p3': 'Katra izmaiņa tiek reģistrēta: kas to izdarīja un kas bija iepriekš.',
  'product.bases.p4': 'Jūsu dati paliek ES, un jūs tos eksportējat, kad vēlaties.',
  'product.bases.f1':
    'Tabulas ar teksta, skaitļa, datuma, atzīmes lodziņa, vienas izvēles un vairāku izvēļu laukiem.',
  'product.bases.f2':
    'Attiecības starp tabulām: sasaistiet pasūtījumu ar klientu, un datubāze neļauj saiti pārtraukt.',
  'product.bases.f3':
    'Rediģējams režģis ar tastatūru, lasāms ekrāna lasītājiem un izmantojams tālrunī.',
  'product.bases.f4':
    'Atļaujas pa datubāzei un tabulai: īpašnieks, redaktors, komentētājs un lasītājs.',
  'product.bases.f5': 'Importējiet CSV failu kā jaunu tabulu un eksportējiet uz CSV vai JSON.',
  'product.bases.f6': 'Katras izmaiņas žurnāls: kas, kurš ieraksts un lauks, pirms un pēc.',
  'product.cloud.f12': 'Lietojums, izmaksas un Eiropas rēķini tajā pašā konsolē.',
  'product.cloud.f11': 'Personas, API marķieri un eksportējams audita žurnāls.',
  'product.cloud.f10': 'Privātumu cienoša tīmekļa analītika bez sīkfailiem.',
  'product.cloud.f9': 'Ieplānoti uzdevumi un projektu veidnes.',
  'product.cloud.f8': 'Terminālis konteinerā; pakalpojuma restartēšana vai apturēšana.',
  'product.cloud.f7': 'Tiešsaistes žurnāli, metrikas un uzraudzība ar brīdinājumiem.',
  'product.cloud.f6': 'Vides mainīgie un noslēpumi katrai videi.',
  'product.cloud.f5': 'Pielāgoti domēni ar automātisku TLS; pāradresācijas un galvenes.',
  'product.cloud.f4': 'S3 krātuve un pārvaldītas funkcijas.',
  'product.cloud.f3': 'Postgres datubāzes ar automātisku REST API un SQL redaktoru.',
  'product.cloud.f2':
    'Priekšskatījums katram PR un atgriešana uz jebkuru izvietošanu ar vienu klikšķi.',
  'product.cloud.f1':
    'Projekti no jūsu repozitorija (GitHub, GitLab vai savs Git) ar izvietošanu pēc katra push.',
  'product.console.f1':
    'Informācijas panelis ar organizācijas stāvokli un to, kam šodien vērts pievērst uzmanību.',
  'product.console.f2':
    'Konti: pievienošana, apturēšana, paroles un MFA atiestatīšana, sesijas, aizstājvārdi un pastkaste.',
  'product.console.f3':
    'Statiskas grupas un uz noteikumiem balstītas dinamiskas grupas; lomas ar atļaujām.',
  'product.console.f4':
    'Priviliģēta piekļuve (PIM): laikā ierobežota aktivizēšana ar apstiprinājumu.',
  'product.console.f5':
    'SSO ar SAML vai OIDC pakalpojumu sniedzējiem, ar savienojuma pārbaudi; nodrošināšana ar SCIM.',
  'product.console.f6': 'Nosacījumpiekļuves noteikumi un uzticamo ierīču saraksts.',
  'product.console.f7':
    'Integrācijas: OIDC klienti, SAML un trešo pušu savienojumi; produktu aizstāšana.',
  'product.console.f8': 'Satura pārnešana: migrācija no citiem pakalpojumu sniedzējiem.',
  'product.console.f9': 'Dokumentu redaktors un pārvaldāmi Rooms kanāli.',
  'product.console.f10':
    'Pārvaldība: juridiskā aizturēšana, sensitivitātes iezīmes un paroļu politika.',
  'product.console.f11':
    'Audita žurnāls ar CSV eksportu un pārsūtīšanu uz jūsu SIEM; pakalpojumu žurnāli.',
  'product.console.f12': 'Savs domēns un zīmols: organizācijas logotips un pasta domēns.',
  'product.id.f1':
    'Vienotā pieteikšanās (OpenID Connect un OAuth 2.0 ar PKCE) komplektam un jūsu lietotnēm.',
  'product.id.f2': 'Piekļuves atslēgas (WebAuthn) un TOTP MFA ar atkopšanas kodiem.',
  'product.id.f3': 'Lietotņu paroles IMAP un klientiem, kas neatbalsta OIDC.',
  'product.id.f4': 'Sesijas un ierīces: atsauciet vienu vai visas pārējās.',
  'product.id.f5': 'SAML federācija ar jūsu IdP un Sytue kā SAML IdP citām lietotnēm.',
  'product.id.f6': 'Nodrošināšana ar SCIM no jūsu direktorija.',
  'product.id.f7': 'Ielūgumi, e-pasta apstiprināšana un paroles atkopšana.',
  'product.id.f8': 'Profils iPhone un Mac (.mobileconfig) ar vienu pieskārienu.',
  'product.id.f9': 'Organizācijas noteiktas paroļu politikas un piekļuves noteikumi.',
  'product.id.f10': 'Dinamiska klientu reģistrācija (DCR) un standarta atklāšana.',
  'product.erp.f1':
    'Divdesmit četri moduļi un trīsdesmit seši modeļi vienā pakalpojumā un vienā datubāzē.',
  'product.erp.f2':
    'Modeļu katalogs ir dati: katrs modulis deklarē savus laukus, stāvokļus un saraksta kolonnas.',
  'product.erp.f3':
    'Jauns modulis ir viens JSON fails — bez DDL, bez migrācijas un bez tabulu bloķēšanas.',
  'product.erp.f4':
    'Viena API visiem trīsdesmit sešiem modeļiem un viena no kataloga ģenerēta saskarne.',
  'product.erp.f5':
    'Divkāršā grāmatvedība ar summām veselos centos; neviena skaitļa peldošajā komatā.',
  'product.erp.f6':
    'Bilanci pārbauda datubāze transakcijas apstiprināšanas brīdī, nevis lietotnes kods.',
  'product.erp.f7':
    'Slēgtie ieraksti ir nemaināmi: tos nevar rediģēt, dzēst vai papildināt ar jaunām rindām.',
  'product.erp.f8':
    'Viens dokuments, viens ieraksts: atkārtots mēģinājums vai divas atvērtas cilnes nevar dubultot ieņēmumus.',
  'product.erp.f9':
    'Kopīgie nosaukumi ir pieejami visās 26 valodās, un netulkots lauks neļauj pakalpojumam startēt.',
  'product.erp.f10':
    'Indeksēti saraksti: pirmā lappuse no divdesmit tūkstošiem rindu tiek atgriezta ātrāk nekā milisekundē.',
  'meta.downloads.title': 'Lejupielādes — Sytue One datoram un mobilajām ierīcēm',
  'meta.downloads.description':
    'Vietējās Sytue One lietotnes macOS, Windows, Linux, iOS un Android sistēmām. Viss komplekts jūsu ierīcē ar katras sistēmas integrācijām.',
  'downloads.overline': 'Lietotnes',
  'downloads.h1': 'Sytue One jūsu ierīcē',
  'downloads.intro':
    'Īstas datora un mobilās lietotnes: tās tiek instalētas, atrodas Dock vai izvēlnē Sākt, informē ar sistēmas paziņojumiem un atver failus no jūsu ierīces.',
  'downloads.desktop.title': 'Dators',
  'downloads.desktop.body':
    'Katram produktam sava vietējā lietotne, kas pati zīmē savu saskarni ar grafisko karti: iekšā nav pārlūka, tāpēc tā ātri startē un patērē kā sistēmas lietotne.',
  'downloads.apps.title': 'Tikai tas, kas vajadzīgs',
  'downloads.apps.body':
    'Sytue One aptver visu darba dienu. Ja vajadzīgs tikai disks, Sytue Drive var instalēt atsevišķi.',
  'downloads.includes': 'Ietver:',
  'downloads.mobile.title': 'Mobilās ierīces',
  'downloads.mobile.body':
    'Vietējās iOS un Android lietotnes ar sistēmas paziņojumiem, kopīgošanu un piekļuvi failiem un kamerai.',
  'downloads.selfhost.title': 'Jūsu organizācijai ir savs domēns?',
  'downloads.selfhost.body':
    'Vietējās lietotnes savienojas ar sytue.eu. Ja jūsu organizācijai ir sava instalācija, piesakieties pārlūkā tās domēnā: komplekts tīmeklī darbojas tāpat.',
  'downloads.yourSystem': 'Jūsu sistēma',
  'downloads.signInToGet': 'Pierakstieties, lai lejupielādētu',
  'downloads.store': 'Skatīt: {format}',
  'downloads.soon': 'Drīzumā: {format}',
  'downloads.browser.text':
    'Dodat priekšroku pārlūkam? Viss komplekts tīmeklī darbojas tāpat, neko neinstalējot:',
  'downloads.browser.link': 'atvērt Sytue One',
  'platform.macos.req': 'macOS 11 vai jaunāka · Apple Silicon un Intel',
  'platform.windows.req': 'Windows 10 vai jaunāka · 64 bitu',
  'platform.linux.req': 'OpenGL 2.1 · 64 bitu',
  'platform.ios.req': 'iOS 14 vai jaunāka',
  'platform.android.req': 'Android 6.0 vai jaunāka',
  'footer.tagline':
    'Katra komplekta lietotne ir mūsu pašu pakalpojums, veidots uz atvērtiem, publicētiem protokoliem.',
  'footer.products': 'Produkti',
  'footer.resources': 'Resursi',
  'footer.legal': 'Juridiskā informācija',
  'footer.suite': 'Komplekts',
  'footer.downloads': 'Lejupielādēt lietotnes',
  'footer.sales': 'Sazināties ar pārdošanas nodaļu',
  'footer.privacy': 'Privātums un VDAR',
  'footer.accessibility': 'Piekļūstamība',
  'footer.openSource': 'Atvērtais kods',
  'footer.legalNotice': 'Juridisks paziņojums',
  'footer.terms': 'Noteikumi',
  'footer.copyright':
    '© {year} Sytue · Dati mitināti ES (Scaleway fr-par / nl-ams · pašu zona Madridē).',
  'footer.privacyNotice':
    'Mēs nepārdodam jūsu datus un nenododam tos trešajām personām. Mēs tos neizmantojam profilēšanai, reklāmai vai savu vai citu pakalpojumu apmācībai vai uzlabošanai. Tie ir šifrēti pārsūtīšanas un glabāšanas laikā un atstāj mūsu Eiropas infrastruktūru tikai tad, ja tas ir noteikti nepieciešams rēķina izrakstīšanai, paziņojumiem, kamēr lietotne ir aizvērta (push pakalpojums redz tikai jūsu ierīci un laiku, nekad saturu), un jūsu ieslēgtajās integrācijās, kurās trešā persona lūgs jūsu piekrišanu datiem, ko tā izmantos.',
  'meta.notFound.title': 'Lapa nav atrasta — Sytue One',
  'notFound.overline': 'Kļūda 404',
  'notFound.title': 'Šī lapa netika atrasta',
  'notFound.body':
    'Iespējams, saite ir mainījusies. Atgriezieties sākumlapā vai iepazīstiet komplektu.',
  'notFound.home': 'Atpakaļ uz sākumu',
  'meta.legal.title': 'Juridisks paziņojums — Sytue One',
  'meta.legal.description':
    'www.sytue.eu un pakalpojuma Sytue One īpašnieka identifikācijas dati: Sytue Projects, S.L.',
  'aviso.title': 'Juridisks paziņojums',
  'aviso.intro':
    'Saskaņā ar Spānijas 11. jūlija Likuma Nr. 34/2002 par informācijas sabiedrības pakalpojumiem un elektronisko komerciju (LSSI) 10. pantu šeit sniegti šīs tīmekļa vietnes un pakalpojuma Sytue One īpašnieka identifikācijas dati.',
  'aviso.review.body':
    'Reģistra dati šajā paziņojumā ir pārbaudīti Komercreģistrā. Pārējais teksts tiek juridiski pārskatīts, un tā formulējums var mainīties, nemainot aprakstīto.',
  'aviso.owner.title': 'Īpašnieks',
  'aviso.owner.name': 'Nosaukums',
  'aviso.owner.form': 'Juridiskā forma',
  'aviso.owner.formValue':
    'Viena dalībnieka sabiedrība ar ierobežotu atbildību (sociedad limitada unipersonal) saskaņā ar Spānijas tiesībām',
  'aviso.owner.nif': 'Nodokļu maksātāja numurs (NIF)',
  'aviso.owner.address': 'Juridiskā adrese',
  'aviso.owner.registry': 'Reģistra ieraksts',
  'aviso.owner.registryValue':
    'Madrides Komercreģistrs (Registro Mercantil de Madrid), 8. nodaļa, lapa M-887153, 1. ieraksts',
  'aviso.owner.email': 'E-pasts',
  'aviso.owner.web': 'Tīmekļa vietne',
  'aviso.scope.title': 'Mērķis',
  'aviso.scope.body':
    'Šī tīmekļa vietne iepazīstina ar komplektu Sytue One un nodrošina piekļuvi tā lietotnēm. Pakalpojuma lietošanu regulē Pakalpojuma noteikumi, bet personas datu apstrādi — Privātuma politika; saites uz abiem dokumentiem ir katras lapas kājenē.',
  'aviso.ip.title': 'Intelektuālais un rūpnieciskais īpašums',
  'aviso.ip.body':
    'Šīs vietnes saturs (teksti, dizains, kods, kā arī Sytue un Sytue One preču zīmes un logotipi) pieder Sytue Projects, S.L. vai tiek izmantots saskaņā ar licenci. Trešo pušu produktu nosaukumi, kas norādīti kā saderības atsauces, ir to īpašnieku preču zīmes.',
  'aviso.liability.title': 'Atbildība',
  'aviso.liability.body':
    'Sytue Projects, S.L. cenšas uzturēt šo vietni pieejamu un aktuālu, negarantējot, ka nebūs pārtraukumu vai kļūdu. Sabiedrība neatbild par trešo pušu vietņu saturu, uz kurām ved saites.',
  'aviso.law.title': 'Piemērojamās tiesības',
  'aviso.law.body':
    'Šo paziņojumu regulē Spānijas tiesības. Patērētājiem saglabājas tiesības un jurisdikcija, ko piešķir viņu dzīvesvietas valsts tiesības Eiropas Savienībā.',
  'aviso.updated': 'Pēdējoreiz atjaunināts: 2026. gada septembrī.',
  'meta.accessibility.title': 'Piekļūstamības paziņojums — Sytue One',
  'meta.accessibility.description':
    'Cik piekļūstami ir komplekts Sytue One un šī tīmekļa vietne, ko mēs mērām, kas vēl nav pietiekami un kā mums par to paziņot.',
  'a11y.title': 'Piekļūstamības paziņojums',
  'a11y.intro':
    'Sytue vēlas, lai ikviens varētu lietot Sytue One un šo tīmekļa vietni neatkarīgi no izmantotajām palīgtehnoloģijām. Šis paziņojums attiecas uz www.sytue.eu un komplekta tīmekļa lietotnēm.',
  'a11y.standard.title': 'Standarts, pēc kura strādājam',
  'a11y.standard.body':
    'Mēs projektējam un izstrādājam atbilstoši WCAG 2.1 AA līmenim, uz kuru atsaucas Eiropas standarts EN 301 549 un Eiropas Piekļūstamības akts (Direktīva (ES) 2019/882).',
  'a11y.status.title': 'Atbilstības statuss',
  'a11y.status.body':
    'Daļēja atbilstība: lielākā komplekta daļa atbilst standartam, bet tālāk uzskaitītās daļas tam vēl pilnībā neatbilst. Neatkarīgs audits vēl nav pasūtīts.',
  'a11y.measures.title': 'Ko mēs darām, lai to panāktu',
  'a11y.measures.design':
    'Viena dizaina sistēma visam komplektam: teksta krāsas ar pārbaudītu kontrastu, redzams fokusa rāmis uz katras vadīklas un izvēlņu, dialoglodziņu un sarakstu vadība ar tastatūru.',
  'a11y.measures.ci':
    'Automātiski piekļūstamības auditi (Lighthouse) tiek palaisti integrācijas procesā pēc katras publiskās vietnes izmaiņas.',
  'a11y.measures.system':
    'Saskarne ievēro jūsu sistēmas iestatījumus: tumšo vai gaišo motīvu, samazinātu kustību un teksta lielumu, bez fiksētiem pikseļu izmēriem veidlapu laukos.',
  'a11y.measures.text':
    'Katram attēlam, ikonai un tukšam stāvoklim ir teksta alternatīva, un katra lapa norāda savu valodu, lai ekrāna lasītāji to izrunātu pareizi.',
  'a11y.limits.title': 'Zināmie ierobežojumi',
  'a11y.limits.body':
    'Izklājlapu režģis, slaidu audekls un PDF anotēšana ir sarežģīti redaktori, pie kuriem vēl jāstrādā, lai tos varētu pilnībā lietot ar tastatūru un ekrāna lasītāju. Personu augšupielādētie dokumenti un faili tiek rādīti tādi, kādi tie ir, un mēs tos nepielāgojam.',
  'a11y.feedback.title': 'Pastāstiet, kas nedarbojas',
  'a11y.feedback.body':
    'Ja kaut ko komplektā vai šajā vietnē jums ir grūti vai neiespējami lietot, rakstiet mums — mēs to izpētīsim un sniegsim atbildi:',
  'a11y.enforcement.title': 'Ja atbilde neapmierina',
  'a11y.enforcement.body':
    'Ja atbilde uz ziņoto šķērsli neapmierina, varat vērsties iestādē, kas jūsu valstī atbild par piekļūstamību saskaņā ar Eiropas Piekļūstamības aktu.',
  'a11y.updated':
    'Šis paziņojums sagatavots 2026. gada septembrī, un to pārskata, kad komplekts būtiski mainās.',
  'meta.openSource.title': 'Atvērtais kods — Sytue One',
  'meta.openSource.description':
    'Sytue veido gandrīz visu no nulles, taču neizgudro no jauna to, kas jau darbojas labi. Atvērtā koda projekti, uz kuriem balstās komplekts.',
  'oss.title': 'Atvērtais kods',
  'oss.intro':
    'Sytue veido gandrīz visu no nulles: pastu, redaktorus, kalendāru. Taču tas neizgudro no jauna to, kas jau darbojas labi. Šie ir atvērtā koda projekti, uz kuriem balstās dažas komplekta daļas.',
  'oss.livekit.body': 'Reāllaika audio un video Sytue Rooms zvaniem darbojas uz LiveKit.',
  'oss.postgres.body':
    'Katrs komplekta pakalpojums glabā savus datus PostgreSQL, viena datubāze katrai organizācijai.',
  'meta.privacy.title': 'Privātums un VDAR — Sytue One',
  'meta.privacy.description':
    'Kā Sytue One apstrādā personas datus: pārzinis, dati, tiesiskais pamats, glabāšana, apstrādātāji, drošība, starptautiskā nosūtīšana un jūsu tiesības saskaņā ar VDAR.',
  'legal.overline': 'Juridiskā informācija',
  'legal.updated': 'Pēdējoreiz atjaunināts: 2026. gada jūlijā.',
  'legal.review.title': 'Tiek juridiski pārskatīts.',
  'privacy.title': 'Privātums un VDAR',
  'privacy.review.body':
    'Šī politika precīzi apraksta, kā mēs apstrādājam datus šodien. Konkrētie glabāšanas termiņi tiks papildināti galīgajā versijā, ko pārskatīs mūsu juristu komanda.',
  'privacy.controller.title': 'Datu pārzinis',
  'privacy.controller.body':
    'Sytue Projects, S.L. (Spānijas nodokļu maksātāja numurs B88729900; Calle Atenas, 10, 28822 Coslada, Madride, Spānija; reģistrēta Madrides Komercreģistrā, 8. nodaļa, lapa M-887153) kā komplekta Sytue One sniedzēja ir pakalpojuma kontu un organizāciju datu pārzine. Kontakts datu aizsardzības jautājumos:',
  'privacy.data.title': 'Kādus datus mēs apstrādājam',
  'privacy.data.account.label': 'Konta dati:',
  'privacy.data.account.body':
    'vārds, e-pasta adrese un organizācija, ko pārvalda Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Jūsu organizācijas saturs:',
  'privacy.data.content.body':
    'pasts, faili, dokumenti, kalendārs un ziņojumi, ko jūsu organizācija mitina komplektā. Šis saturs pieder jūsu organizācijai.',
  'privacy.data.technical.label': 'Tehniskie darbības dati:',
  'privacy.data.technical.body':
    'piekļuves žurnāli un savienojuma dati, kas nepieciešami drošībai, ļaunprātīgas izmantošanas novēršanai un pakalpojuma darbībai.',
  'privacy.data.note':
    'Mēs nepārdodam datus un neizmantojam tos reklāmai. Sytue pārvaldītā infrastruktūra un datu glabāšana atrodas Eiropas Savienībā (Scaleway, reģionos fr-par un nl-ams, kā arī lokālā infrastruktūrā Madridē), ar šifrēšanu pārsūtīšanas laikā (TLS 1.3) un glabāšanas laikā. Savienojumi ar ārējiem pakalpojumiem nosūta tavus atļautos datus to sniedzējam saskaņā ar tā atrašanās vietu un noteikumiem.',
  'privacy.basis.title': 'Tiesiskais pamats',
  'privacy.basis.body':
    'Apstrāde balstās uz VDAR 6. panta tiesiskajiem pamatiem: pakalpojuma līguma izpildi ar jūsu organizāciju, mūsu leģitīmajām interesēm drošības un ļaunprātīgas izmantošanas novēršanas jomā, juridisko pienākumu izpildi un, ja tas ir skaidri pieprasīts, jūsu piekrišanu. Sīkāka informācija par katru pamatu un apstrādes darbību būs iekļauta galīgajā tekstā.',
  'privacy.retention.title': 'Glabāšana',
  'privacy.retention.body':
    'Konta datus un saturu glabājam, kamēr jūsu organizācija uztur pakalpojumu aktīvu. Slēdzot kontu vai organizāciju, dati saprātīgā termiņā tiek dzēsti vai anonimizēti, izņemot to, kas mums jāglabā saskaņā ar likumu (piemēram, rēķini). Tehniskie darbības žurnāli tiek periodiski tīrīti. Konkrētie termiņi pa kategorijām tiek norādīti galīgajā versijā. Ziņojumi par saturu tiek glabāti, kamēr tie ir atvērti, un tiek dzēsti divpadsmit mēnešus pēc to atrisināšanas.',
  'privacy.processors.title': 'Apstrādātāji un apakšapstrādātāji',
  'privacy.processors.body':
    'Mūsu infrastruktūras nodrošinātājs ir Scaleway (reģioni fr-par un nl-ams), kas darbojas kā apstrādātājs tikai Eiropas Savienībā; turklāt mēs uzturam pašu zonu Madridē (Spānijā) savās telpās. Abonēšanas maksājumiem izmantojam Mollie B.V. (Nīderlande), kas apstrādā tikai maksājumam nepieciešamos datus. Mēs neizmantojam apakšapstrādātājus ārpus EEZ. Apakšapstrādātāju saraksts tiks publicēts un uzturēts aktuāls, par katru papildinājumu paziņojot iepriekš.',
  'privacy.security.title': 'Drošība',
  'privacy.security.body':
    'Mēs piemērojam VDAR atbilstošus tehniskos un organizatoriskos pasākumus: šifrēšanu pārsūtīšanas (TLS 1.3) un glabāšanas laikā, organizāciju nošķiršanu (multi-tenant ar rindu līmeņa drošību), stipru autentifikāciju (MFA/WebAuthn), piekļuves reģistrēšanu un minimālo privilēģiju principu. Neviena sistēma nav nekļūdīga: ja pārkāpums skars jūs, mēs par to paziņosim Regulā noteiktajos termiņos.',
  'privacy.cookies.title': 'Sīkfaili',
  'privacy.cookies.body':
    'Mēs izmantojam tikai tehniskos sīkfailus, kas nepieciešami vienotajai pieteikšanās sistēmai un sesijas drošībai. Mēs neizmantojam reklāmas vai trešo pušu izsekošanas sīkfailus, tāpēc nav piekrišanas sienas izsekošanai.',
  'privacy.transfers.title': 'Starptautiskā nosūtīšana',
  'privacy.transfers.body':
    'Sytue pārvaldītā apstrāde un mitināšana notiek Eiropas Savienības reģionos. Lietotāja iespējotie ārējie savienojumi nosūta atļautos datus to sniedzējam; to atrašanās vieta ir atkarīga no sniedzēja un tā konfigurācijas. Paziņojumiem, kas tiek sūtīti, kamēr lietotne vai pārlūks ir aizvērts, arī var izmantot ārējus pakalpojumus. Ierīces push pakalpojums (Google, Mozilla vai Apple atkarībā no jūsu pārlūka vai tālruņa) redz jūsu ierīces anonīmu identifikatoru un katra paziņojuma laiku — nekad saturu, kas tiek pārsūtīts ar pilnīgu šifrēšanu un iOS sistēmā tiek nosūtīts tukšs. Paziņojumus var izslēgt, un tad netiek nosūtīts pat tas.',
  'privacy.rights.title': 'Jūsu tiesības',
  'privacy.rights.body1':
    'VDAR piešķir jums tiesības piekļūt datiem, labot tos, dzēst, iebilst, ierobežot apstrādi un pārnest datus. Šīs tiesības varat izmantot, rakstot uz',
  'privacy.rights.body2':
    '; atbildam Regulā noteiktajos termiņos. Jums ir arī tiesības iesniegt sūdzību',
  'privacy.rights.authority': 'uzraudzības iestādei',
  'privacy.dpo.title': 'Datu aizsardzības speciālists (DPO)',
  'privacy.dpo.body': 'Jebkuros datu aizsardzības jautājumos vai lai sazinātos ar DPO:',
  'privacy.minors.title': 'Nepilngadīgie',
  'privacy.minors.body':
    'Sytue One ir pakalpojums organizācijām un profesionālai lietošanai; tas nav paredzēts nepilngadīgajiem, un mēs apzināti nevācam nepilngadīgo datus.',
  'privacy.changes.title': 'Izmaiņas šajā politikā',
  'privacy.changes.body':
    'Ja atjaunināsim šo politiku, mainīsim “pēdējās atjaunināšanas” datumu, un, ja izmaiņas būs būtiskas, paziņosim par tām pakalpojuma kanālos pirms to stāšanās spēkā.',
  'meta.terms.title': 'Pakalpojuma noteikumi — Sytue One',
  'meta.terms.description':
    'Sytue One pakalpojuma noteikumi: sniedzējs, apjoms, konti un organizācijas, pieņemama lietošana, pieejamība, jūsu dati, intelektuālais īpašums un piemērojamās tiesības.',
  'terms.title': 'Pakalpojuma noteikumi',
  'terms.review.body':
    'Šie noteikumi apraksta pakalpojumu un tā lietošanu tieši tā, kā tas darbojas šodien. Konkrētās līguma klauzulas (pakalpojuma līmenis, atbildība un jurisdikcija) tiks papildinātas galīgajā versijā, ko pārskatīs mūsu juristu komanda.',
  'terms.provider.title': 'Kas sniedz pakalpojumu',
  'terms.provider.body':
    'Sytue One sniedz Sytue Projects, S.L. (Spānijas nodokļu maksātāja numurs B88729900; Calle Atenas, 10, 28822 Coslada, Madride, Spānija; reģistrēta Madrides Komercreģistrā, 8. nodaļa, lapa M-887153) no infrastruktūras, kas atrodas tikai Eiropas Savienībā (Scaleway, reģioni fr-par un nl-ams, un pašu zona Madridē), un pakalpojumu regulē ES tiesības.',
  'terms.scope.title': 'Pakalpojuma apjoms',
  'terms.scope.body':
    'Sytue One ir produktivitātes komplekts organizācijām: identitāte (Sytue ID), pasts, faili, dokumenti, kalendārs, kontakti un ziņojumi ar videozvaniem. Pakalpojumu līgst katrai organizācijai.',
  'terms.accounts.title': 'Konti un organizācijas',
  'terms.accounts.body':
    'Katra organizācija pārvalda savus kontus, grupas un atļaujas, izmantojot Sytue ID (SSO + MFA). Organizācijas administrācija atbild par kontu pievienošanu un noņemšanu un par to, kā pakalpojums tiek lietots ar šiem kontiem.',
  'terms.use.title': 'Pieņemama lietošana',
  'terms.use.body':
    'Jūs apņematies lietot pakalpojumu likumīgi un neizmantot to nelikumīgām darbībām, surogātpastam, ļaunprogrammatūras izplatīšanai, trešo pušu tiesību pārkāpšanai vai jebkādai lietošanai, kas apdraud pakalpojuma drošību vai pieejamību citām organizācijām.',
  'terms.availability.title': 'Pieejamība un atbalsts',
  'terms.availability.body1':
    'Mēs strādājam, lai nodrošinātu augstu pieejamību, un pastāvīgi veicam apkopi un uzlabojumus. Konkrētās pakalpojuma līmeņa saistības (SLA), ja tās attiecas uz jūsu plānu, ir noteiktas pakalpojuma līgumā. Atbalsts tiek sniegts, izmantojot',
  'terms.data.title': 'Jūsu dati',
  'terms.data.body1':
    'Saturs, ko jūsu organizācija mitina komplektā, pieder jūsu organizācijai. Komplekts ir veidots uz atvērtiem standartiem (OIDC, JMAP, CalDAV, CardDAV, WebDAV), tāpēc jūsu dati paliek pārnesami, un tos var paņemt līdzi. Personas datu apstrāde ir aprakstīta lapā',
  'terms.ip.title': 'Intelektuālais īpašums',
  'terms.ip.body':
    'Programmatūra Sytue One ir Sytue oriģināls darbs, un to aizsargā attiecīgās intelektuālā īpašuma tiesības. Īpašumtiesības uz saturu, ko jūsu organizācija mitina komplektā, paliek jums; mēs to apstrādājam tikai pakalpojuma sniegšanai.',
  'terms.liability.title': 'Atbildība',
  'terms.liability.body':
    'Mēs darām visu, lai pakalpojums būtu uzticams un drošs. Piemērojamās garantijas un atbildības ierobežojumi ir noteikti pakalpojuma līgumā; nekas šajos noteikumos neizslēdz atbildību, kuras izslēgšanu likums neatļauj.',
  'terms.contact.title': 'Līgumi un kontakti',
  'terms.contact.body1':
    'Par līguma nosacījumiem, cenām organizācijai vai jebkuru jautājumu par pakalpojumu rakstiet uz',
  'terms.changes.title': 'Izmaiņas',
  'terms.changes.body':
    'Mēs varam atjaunināt šos noteikumus, lai atspoguļotu izmaiņas pakalpojumā vai tiesību aktos. Ja izmaiņas būs būtiskas, paziņosim par tām pakalpojuma kanālos pirms to stāšanās spēkā un atjaunināsim “pēdējās atjaunināšanas” datumu.',
  'terms.law.title': 'Piemērojamās tiesības',
  'terms.law.body':
    'Pakalpojumu regulē Eiropas Savienības tiesības un sniedzēja valsts tiesības. Konkrētā strīdu izšķiršanas jurisdikcija tiek noteikta galīgajā tekstā. Neatkarīgi no tā jums saglabājas tiesības iesniegt sūdzību datu aizsardzības uzraudzības iestādei.',
  'seo.org.description':
    'Sytue One: suverēns Eiropas produktivitātes komplekts. Identitāte, pasts, faili, dokumenti, kalendārs, tērzēšana, videozvani un PaaS, 100% ES.',
  'seo.org.slogan': 'Jūsu produktivitāte, suverēna un eiropeiska.',
  'seo.software.description':
    'Suverēns Eiropas produktivitātes komplekts: pasts (JMAP), faili (WebDAV), kopīgi dokumenti, kalendārs (CalDAV), kontakti (CardDAV), tērzēšana, videozvani un PaaS. SSO ar Sytue ID.',
  'seo.software.f1': 'Vienotā pieteikšanās (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Pasts, faili, dokumenti, kalendārs, kontakti, tērzēšana un videozvani',
  'seo.software.f3': 'Izvietošanas un pārvaldītu datu platforma',
  'seo.software.f4': 'Dati 100% ES (Scaleway)',
  'seo.offer.description': 'Bezmaksas izmēģinājums, karte nav vajadzīga.',
  'seo.downloads.description':
    'Komplekta Sytue One vietējās lietotnes datoram (macOS, Windows, Linux) un mobilajām ierīcēm (iOS, Android).',
  'home.pricing.personal.title': 'Bezmaksas',
  'home.pricing.personal.freeStorage': '5 GB vienai personai',
  'home.pricing.personal.freeBody':
    'Pasts, faili, fotoattēli, dokumenti, kalendārs, kontakti, piezīmes un telpas. Bez kartes un bez laika ierobežojuma.',
  'home.pricing.personal.note':
    'Jūsu organizācija ir jūsu e-pasts: bez organizācijas administrēšanas un bez ERP, bet viss pārējais, ieskaitot Sytue Admin jūsu integrācijām un migrācijām.',
  'home.pricing.personal.cta': 'Izveidot bezmaksas kontu',
  'grid.orgOnly': 'Tikai organizācijām',
  'grid.enDesarrollo': 'Izstrādē',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB vienam lietotājam',
  'home.pricing.paidBody':
    'Organizācijām, kā arī vienai personai. Viss komplekts ar Sytue ERP un Sytue Admin jūsu organizācijas vadībai.',
  'home.pricing.thirdParty':
    'Katrs plāns ļauj pievienot savu krātuvi: Google Drive, Microsoft OneDrive, S3 spaini (Scaleway, OVH, Hetzner, Wasabi, MinIO), WebDAV serveri, piemēram, Nextcloud vai Internxt, vai jūsu NAS jūsu telpās — bez atvērtiem portiem, caur Sytue tuneli. Par pievienotajās krātuvēs glabāto datu dublēšanu un atkopšanu atbild klients (jūs), un Sytue® tiek atbrīvots no jebkādas atbildības par datu zudumu ar Sytue® savienotās krātuvēs.',
  'home.pricing.soon': 'Drīzumā',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB vienam lietotājam',
  'home.pricing.enterprise.support': 'Ar Premium atbalstu.',
  'home.pricing.education': 'Izglītības iestādēm: 50% atlaide Pro plānam.',
};
