/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const fi: AppCatalog<typeof en> = {
  'skip.content': 'Siirry sisältöön',
  'nav.products': 'Tuotteet',
  'nav.sovereignty': 'Suvereniteetti',
  'nav.standards': 'Avoimet standardit',
  'nav.pricing': 'Hinnat',
  'footer.pricing': 'Hinnat',
  'home.pricing.title': 'Kolme pakettia, koko kokonaisuus',
  'home.pricing.body':
    'Samat tuotteet kaikissa kolmessa: vain käyttäjäkohtainen tallennustila vaihtelee. Ei riisuttuja versioita.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'alv 0 %',
  'home.pricing.per': '/Käyttäjä/Kuukausi',
  'home.pricing.cta': 'Ota yhteyttä myyntiin',
  'home.pricing.commitment.title': 'Sitoumus koko eliniäksi',
  'home.pricing.commitment.body':
    'Käyttäjäkohtainen hinta nousee 0,50 € joka neljäs vuosi eikä mitään muuta: koko eliniän. Tiedät sen liittymispäivästä alkaen, eikä se muutu.',
  'home.pricing.included.title': 'Yksi ainoa versio',
  'home.pricing.included.body':
    'Ei ominaisuuskohtaisia paketteja, ei tasoja, ei riisuttuja versioita: koko kokonaisuus sisältyy käyttäjäkohtaiseen hintaan, samoin ominaisuuksin kaikille.',
  'home.pricing.metered.title': 'Mitä laskutetaan erikseen',
  'home.pricing.metered.body': 'Lisätallennustila: käytön mukaan tai lohkoina.',
  'home.pricing.storage.title': 'Tallennustila on yhteinen',
  'home.pricing.storage.body':
    'Jokaisen henkilön kiintiö lisätään organisaation kiintiöön: minkä yksi jättää käyttämättä, sen muut saavat käyttöönsä. Käyttäjäkohtaista kiintiötä voi myös säätää organisaatiossa.',
  'nav.downloads': 'Lataukset',
  'nav.aria': 'Sivuston navigointi',
  'nav.ariaMobile': 'Mobiilinavigointi',
  'nav.home': 'Sytue One — Etusivu',
  'nav.openMenu': 'Avaa valikko',
  'nav.closeMenu': 'Sulje valikko',
  'theme.toLight': 'Vaihda vaaleaan teemaan',
  'theme.toDark': 'Vaihda tummaan teemaan',
  'lang.label': 'Kieli',
  'cta.start': 'Aloita maksutta',
  'cta.signIn': 'Kirjaudu sisään',
  'cta.seeSuite': 'Tutustu kokonaisuuteen',
  'meta.home.title': 'Sytue One — Suvereeni eurooppalainen tuottavuuspaketti',
  'meta.home.description':
    'Sytue One: suvereeni eurooppalainen tuottavuuspaketti. Identiteetti, sähköposti, tiedostot, asiakirjat, kalenteri ja chat, 100 % EU:ssa ja avoimiin standardeihin rakennettuna.',
  'home.hero.lead': 'Tuottavuutesi,',
  'home.hero.accent': 'suvereeni ja eurooppalainen',
  'home.hero.subtitle':
    'Sähköposti, tiedostot, asiakirjat, kalenteri, chat ja videopuhelut — yhdistettynä yhdeksi sovellukseksi ja isännöitynä kokonaan EU:ssa.',
  'home.hero.note': 'Korttia ei tarvita. Tiedot Scalewaylla (EU). Apua siirtoon.',
  'home.privacy.title': 'Työsi kuuluu sinulle. Niin myös yksityisyytesi.',
  'home.privacy.body': 'Emme myy tietojasi emmekä käytä niitä mainosprofiilien luomiseen.',
  'home.privacy.anonymity':
    'Sytue Analytics suojaa yksityisyyttäsi koostetuilla tilastoilla ilman seurantaevästeitä sekä pseudonyymisillä kävijätunnisteilla, jotka vaihtuvat päivittäin ja ovat erilliset jokaiselle sivustolle.',
  'home.privacy.account':
    'Ohjelmistokokonaisuuden käyttö edellyttää tilitietoja ja käyttölokeja, jotta voimme tarjota palvelun ja pitää sen turvallisena.',
  'home.privacy.link': 'Näin suojaamme yksityisyyttäsi',
  'home.stats.aria': 'Eurooppalainen läsnäolo',
  'home.stats.regions': 'EU-alueet (fr-par · nl-ams · oma vyöhyke Madridissa)',
  'home.stats.apps': 'Sovellusta yhden identiteetin takana',
  'home.stats.standards': 'Avoimet standardit: OIDC, JMAP, CalDAV, CardDAV ja WebDAV',
  'home.products.title': 'Yksi kokonaisuus, yksi kokemus',
  'home.products.subtitle':
    'Kaikilla tuotteilla on sama muotoilu, identiteetti ja kehys. Ne näyttävät ja toimivat kuin yksi sovellus.',
  'home.integration.title': 'Integraatio sellaisena kuin kokonaisuuden kuuluu toimia',
  'home.integration.subtitle':
    'Yksi rekisteröityminen avaa kaiken. Automaattinen käyttäjähallinta (SCIM) Sytue ID:stä.',
  'home.integration.sso.title': 'Kertakirjautuminen',
  'home.integration.sso.body':
    'Yksi identiteetti (Sytue ID) koko kokonaisuudelle. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Yhteinen kehys',
  'home.integration.shell.body':
    'Yläpalkki, sovellusvalitsin, haku ja yleiset ilmoitukset jokaisessa tuotteessa.',
  'home.integration.cross.title': 'Toiminnot sovellusten välillä',
  'home.integration.cross.body':
    'Liitä Drivesta Mailiin, muuta sähköposti Calendarin tapahtumaksi ja jaa se Roomsissa keskeyttämättä työtä.',
  'home.sovereignty.title': 'Suvereniteettia oikeasti',
  'home.sovereignty.body':
    'Tämä ei ole markkinointilupaus vaan arkkitehtuuri. Infrastruktuuri toimii vain eurooppalaisilla alueilla, ja kaikki on rakennettu avoimille, auditoitaville protokollille ilman takaovia.',
  'home.sovereignty.link': 'Miksi avoimet standardit',
  'home.sovereignty.eu.title': '100 % EU:ssa',
  'home.sovereignty.eu.body':
    'Koko infrastruktuuri EU:ssa: Scaleway (fr-par / nl-ams) sekä oma vyöhyke Madridissa. Ei yhdysvaltalaisia palveluntarjoajia henkilötiedoille.',
  'home.sovereignty.gdpr.title': 'GDPR-vaatimustenmukaisuus',
  'home.sovereignty.gdpr.body':
    'Käsittelysopimus, seloste käsittelytoimista, rekisteröityjen pyyntöjen menettely ja säilytysajat. Salattu siirron aikana (TLS 1.3) ja levossa.',
  'home.sovereignty.lockin.title': 'Ei lukittumista',
  'home.sovereignty.lockin.body':
    'Rakennettu avoimille standardeille: OIDC, JMAP, CalDAV, CardDAV ja WebDAV. Tietosi pysyvät siirrettävinä.',
  'home.standards.overline': 'Sytuen omaa työtä',
  'home.standards.title': 'Rakennettu avoimille standardeille',
  'home.standards.body':
    'Jokainen Sytue-tuote on omaa työtämme, kirjoitettu Go:lla ja Astro + Preactilla avoimien protokollien päälle (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Ei kolmansien osapuolten moottoreita: tietosi ovat siirrettäviä ja kokonaisuutesi suvereeni.',
  'home.cta.title': 'Ota työsi hallinta takaisin',
  'home.cta.body':
    'Aloita organisaatiosi kanssa jo tänään. Apua siirtoon Google Workspacesta tai Microsoft 365:stä.',
  'home.cta.primary': 'Luo organisaatio',
  'home.cta.secondary': 'Ota yhteyttä myyntiin',
  'grid.discover': 'Tutustu',
  'product.mail.blurb':
    'Yksityinen, eurooppalainen sähköposti JMAP:n kautta — ilman seurantaa ja mainoksia.',
  'product.drive.blurb': 'Tiedostot ja yhteistyö omalla brändilläsi.',
  'product.docs.blurb':
    'Asiakirjat, laskentataulukot ja diat reaaliaikaisella yhteismuokkauksella.',
  'product.calendar.blurb': 'CalDAV-kalenteri, joka on kytketty muuhun kokonaisuuteen.',
  'product.contacts.blurb': 'CardDAV-yhteystiedot jaettuna koko organisaatiossasi.',
  'product.notes.blurb':
    'Muistikirjat, joihin mahtuu mitä tahansa: tekstiä, tiedostoja, tapahtumia, kokonaisia asiakirjoja.',
  'product.photos.blurb': 'Kuvagalleria ja -editori: alkuperäiset Drivessa, muokkaukset reseptinä.',
  'product.rooms.blurb': 'Kanavat, viestit ja tiimin videopuhelut yhdessä paikassa.',
  'product.cast.blurb': 'Tilausvideo ja suoratoisto: lataa, lähetä ja jaa, EU:ssa.',
  'product.pages.blurb':
    'Verkkosivustot ja verkkokaupat yrityksellesi, suunniteltu sinun tarpeisiisi ja julkaistu EU:ssa.',
  'product.bases.blurb':
    'Yhteiset relaatiotaulukot asiakkaille, tilauksille tai varastolle, EU:ssa.',
  'product.cloud.blurb': 'Julkaisu- ja hallinnoitujen tietojen alusta EU:ssa.',
  'product.id.blurb': 'Yksi identiteetti (SSO + MFA) Sytue ID:llä.',
  'product.one.blurb':
    'Kokonaisuuden etusivu: sovellukset, pikatoiminnot, viimeaikainen toiminta ja ilmoitukset.',
  'product.sheets.blurb':
    'Laskentataulukot kaavoilla, ulkoisilla tiedoilla ja kaavioilla, monen käden muokattavina.',
  'product.slides.blurb': 'Esitykset brändilläsi, teemoilla ja esittäjänäkymällä.',
  'product.pdf.blurb':
    'Lue, merkitse, allekirjoita ja järjestä PDF-tiedostosi ilman, että ne poistuvat Euroopasta.',
  'product.console.blurb':
    'Organisaation hallinta: ihmiset, käyttöoikeudet, vaatimustenmukaisuus ja auditointi.',
  'product.erp.blurb':
    'Kaksikymmentäneljä moduulia ja kahdenkertainen kirjanpito, jota tietokanta ei päästä epätasapainoon.',
  'productPage.back': 'Koko kokonaisuus',
  'productPage.features': 'Kaikki, mitä {name} tekee',
  'productPage.featuresPlanned': 'Mitä {name} tulee tekemään',
  'productPage.builtOn': 'Rakennettu standardille {standard}',
  'productPage.fromScratch':
    'Docus-, Calcus-, Diapos- ja CertiPDF-editori on kirjoitettu alusta asti: Sytuen oma moottori, ei kolmannen osapuolen integraatio.',
  'productPage.shot': '{name}-päänäkymä täsmälleen sellaisena kuin se näkyy kirjautumisen jälkeen.',
  'productPage.shotFeatures': '{name} käytössä, ominaisuudet näytöllä.',
  'productPage.open': 'Avaa {name}',
  'productPage.enDesarrollo': 'Tämä tuote on kehitteillä: se ei ole vielä saatavilla.',
  'productPage.included':
    'Sisältyy Sytue Oneen: yksi identiteetti (Sytue ID) ja yksi rekisteröityminen antavat pääsyn kaikkiin tuotteisiin. Tiedot 100 % EU:ssa (Scaleway) ja siirrettävinä avoimien standardien ansiosta.',
  'product.mail.tagline': 'Yksityinen, eurooppalainen sähköposti ilman seurantaa ja mainoksia.',
  'product.mail.standard': 'JMAP (avoin standardi)',
  'product.mail.p1': 'Nopea saapuneet-kansio ketjuilla, tunnisteilla, suodattimilla ja haulla.',
  'product.mail.p2': 'Liitä tiedostoja suoraan Sytue Drivesta.',
  'product.mail.p3': 'Muuta sähköposti Calendarin tapahtumaksi.',
  'product.mail.p4':
    'Salattu siirron aikana (TLS 1.3). Sytuen infrastruktuuri sijaitsee EU:ssa. Ulkoisten palvelujen sijainti riippuu niiden tarjoajasta.',
  'product.drive.tagline': 'Tiedostosi ja yhteistyösi organisaatiosi brändillä.',
  'product.drive.standard': 'WebDAV (avoin standardi)',
  'product.drive.p1': 'Lataa, järjestä ja jaa hallituilla linkeillä (oikeudet ja vanhentuminen).',
  'product.drive.p2': 'Asiakirjojen ja kuvien esikatselu suoraan sivulla.',
  'product.drive.p3': 'Liitä Mailiin ilman tiedoston kopiointia.',
  'product.drive.p4': 'S3-pohjainen tallennus eurooppalaisilla alueilla.',
  'product.docs.tagline':
    'Asiakirjat, laskentataulukot ja diat reaaliaikaisella yhteismuokkauksella.',
  'product.docs.standard': 'Reaaliaikainen yhteismuokkaus',
  'product.docs.p1':
    'Kirjoittakaa yhdessä, reaaliaikaisella läsnäololla ja välittömillä muutoksilla.',
  'product.docs.p2': 'Laskentataulukot kaavoilla ja ehdollisella muotoilulla.',
  'product.docs.p3': 'Esitykset brändilläsi.',
  'product.docs.p4': 'Kaikki tallennettuna eurooppalaiseen Driveesi.',
  'product.calendar.tagline': 'Kalenterisi, kytkettynä muuhun kokonaisuuteen.',
  'product.calendar.standard': 'CalDAV (avoin standardi)',
  'product.calendar.p1': 'Päivä-, viikko- ja kuukausinäkymät.',
  'product.calendar.p2': 'Toistuvat tapahtumat ja muistutukset.',
  'product.calendar.p3': 'Luo tapahtumia suoraan Mailin sähköpostista.',
  'product.calendar.p4': 'Toimii minkä tahansa CalDAV-asiakasohjelman kanssa.',
  'product.contacts.tagline': 'Organisaatiosi yhteystiedot, jaettuina ja siirrettävinä.',
  'product.contacts.standard': 'CardDAV (avoin standardi)',
  'product.contacts.p1': 'Yhteinen osoitekirja koko organisaatiolle.',
  'product.contacts.p2': 'Tuonti ja vienti vCard-muodossa.',
  'product.contacts.p3': 'Yhdistä kaksoiskappaleet menettämättä tietoja.',
  'product.contacts.p4': 'Toimii minkä tahansa CardDAV-asiakasohjelman kanssa.',
  'product.notes.tagline': 'Kaikki, mitä haluat säilyttää, yhdessä paikassa ja omissa käsissäsi.',
  'product.notes.standard': 'Oma malli avoimien standardien päällä (oma REST-rajapinta)',
  'product.notes.p1': 'Sisäkkäiset muistikirjat vapaalla pohjalla, sarakkeissa tai luettelona.',
  'product.notes.p2':
    'Kaikenlaisia muistiinpanoja: tekstiä, tehtäviä, linkkejä, tiedostoja, Calendarin tapahtumia, Docsin asiakirjoja.',
  'product.notes.p3': 'Päästä päähän -salaus: tunnuslause ei koskaan poistu selaimestasi.',
  'product.notes.p4': 'Sinä päätät, mitä hakuun päätyy — ja mitä ei koskaan.',
  'product.rooms.tagline': 'Kanavat, viestit ja tiimin videopuhelut yhdessä.',
  'product.rooms.standard': 'Reaaliaikaiset viestit ja puhelut',
  'product.rooms.p1': 'Kanavat tiimeittäin tai projekteittain.',
  'product.rooms.p2': 'Videopuhelu upotettuna jokaiseen kanavaan.',
  'product.rooms.p3': 'Maininnat ja ilmoitukset yhtenäisinä koko kokonaisuudessa.',
  'product.rooms.p4': 'Haettava historia mistä tahansa Sytue Onessa.',
  'product.cloud.p4': 'Omat verkkotunnukset automaattisella TLS:llä.',
  'product.cloud.p3': 'Hallinnoitu S3-tallennus ja funktiot.',
  'product.cloud.p2': 'Postgres-tietokanta automaattisella REST-rajapinnalla.',
  'product.cloud.p1': 'Esikatselu jokaiselle PR:lle ja välitön palautus.',
  'product.cloud.standard': 'Kubernetes-natiivi PaaS',
  'product.cloud.tagline': 'Koodisi julkaistuna ja tietosi hallinnoituna Euroopan unionissa.',
  'product.id.tagline': 'Yksi identiteetti koko kokonaisuudelle. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (avoin standardi)',
  'product.id.p1': 'Kertakirjautuminen kaikkiin tuotteisiin.',
  'product.id.p2': 'MFA ja pääsyavaimet (WebAuthn).',
  'product.id.p3': 'Automaattinen käyttäjähallinta (SCIM) hakemistostasi.',
  'product.id.p4': 'Profiili ja tietoturva yhdessä paikassa.',
  'product.one.tagline': 'Koko Sytue One yhden napsautuksen päässä, saman oven kautta.',
  'product.one.standard': 'Yksi kirjautuminen (Sytue ID) koko kokonaisuudelle',
  'product.one.p1':
    'Käynnistin, jossa ovat kaikki sovelluksesi ja organisaatiosi yhdistämät sovellukset.',
  'product.one.p2': 'Haku koko kokonaisuudesta (⌘K) mistä tahansa näkymästä.',
  'product.one.p3':
    'Päivän ohjelma ja lukemattomat sähköpostit etusivulla, kokonaisuuden viimeaikaisten asioiden ja ilmoitustesi vieressä.',
  'product.one.p4': 'Yhdistä laitteesi ja lataa natiivisovellukset.',
  'product.sheets.tagline': 'Järkeviä lukuja taulukossa, joka tekee töitä kanssasi.',
  'product.sheets.standard': 'ODF/OOXML-muodot ja reaaliaikainen yhteismuokkaus',
  'product.sheets.p1': 'Kaavat, ehdollinen muotoilu ja tietojen kelpoisuustarkistus.',
  'product.sheets.p2': 'Tuo ulkoisia tietoja ja tee niihin kyselyjä poistumatta taulukosta.',
  'product.sheets.p3': 'Kaaviot, minikaaviot, osittajat ja tietotaulukot.',
  'product.sheets.p4': 'Vienti ODS- ja XLSX-muotoihin; Excel-tiedostojen avaaminen.',
  'product.slides.tagline': 'Kerro tarina; dia pysyy perässä.',
  'product.slides.standard': 'ODF/OOXML-muodot ja reaaliaikainen yhteismuokkaus',
  'product.slides.p1': 'Esitysteemat organisaatiosi ilmeellä.',
  'product.slides.p2': 'Esittäjänäkymä muistiinpanoineen ja seuraavine dioineen.',
  'product.slides.p3': 'Kuvat, muodot, kaaviot ja SmartArt.',
  'product.slides.p4': 'Vienti ODP-, PPTX- ja PDF-muotoihin; PowerPoint-tiedostojen avaaminen.',
  'product.pdf.tagline': 'PDF alusta loppuun, selaimessasi.',
  'product.pdf.standard': 'PDF (ISO 32000) vakiomuotoisilla merkinnöillä ja lomakkeilla',
  'product.pdf.p1': 'Korosta, alleviivaa, kommentoi ja piirrä asiakirjaan.',
  'product.pdf.p2': 'Lomakkeet: täytä kenttiä tai luo omia.',
  'product.pdf.p3': 'Yhdistä, poimi, kierrä, numeroi ja lisää vesileima.',
  'product.pdf.p4': 'Allekirjoituspaneeli, pysyvä peittäminen ja salasanalla suojatut asiakirjat.',
  'product.console.tagline': 'Hallitse kokonaisuutta poistumatta yhdeltä näytöltä.',
  'product.console.standard': 'SCIM, SAML ja OpenID Connect (avoimet standardit)',
  'product.console.p1': 'Käyttäjät, ryhmät (myös dynaamiset) ja roolit.',
  'product.console.p2': 'SSO palveluntarjoajasi kanssa ja käyttäjähallinta SCIM:llä.',
  'product.console.p3':
    'Ehdollisen pääsyn säännöt, laitteet ja hyväksyntää vaativa etuoikeutettu pääsy.',
  'product.console.p4':
    'Auditointiloki, jonka voi viedä SIEM-järjestelmääsi, sekä oikeudelliset säilytysvelvoitteet.',
  'product.erp.tagline': 'Koko ERP, jonka mallikatalogi on datatiedosto.',
  'product.erp.standard': 'Kahdenkertainen kirjanpito, JSON-mallikatalogi ja yksi REST-rajapinta',
  'product.erp.p1':
    'Kaksikymmentäneljä moduulia ja kolmekymmentäkuusi mallia: yhteystiedot, myynti, ostot, varasto, valmistus, laskutus, kirjanpito, projektit ja henkilöstöhallinto.',
  'product.erp.p2':
    'Moduulin lisääminen tarkoittaa JSON-tiedoston kirjoittamista: ei migraatiota, ei uutta reittiä eikä käyttöliittymätyötä.',
  'product.erp.p3':
    'Rahamäärät tallennetaan kokonaisina sentteinä, ei koskaan liukulukuina, ja tietokanta valvoo tasapainoa.',
  'product.erp.p4':
    'Kirjatut viennit ovat koskemattomia: ne korjataan uudella viennillä, ja juuri se erottaa pääkirjan laskentataulukosta.',
  'product.one.f1':
    'Käynnistin kokonaisuuden sovelluksille ja kolmansien osapuolten sovelluksille, jotka organisaatiosi yhdistää.',
  'product.one.f2':
    'Pikatoiminnot: kirjoita sähköposti, uusi asiakirja, lataa Driveen, luo tapahtuma.',
  'product.one.f3':
    'Viimeaikaiset koko kokonaisuudessa: viimeisimmät kohteet Drivesta, Docusista, Mailista tai Roomsista.',
  'product.one.f4': 'Yleinen haku näppäimillä ⌘K tai Ctrl K, tulokset kaikista sovelluksista.',
  'product.one.f5': 'Ilmoituskeskus ja läsnäolo yhteisinä koko kokonaisuudessa.',
  'product.one.f6':
    'Yhdistä laitteesi: profiili iPhonelle ja Macille, Androidille ja tietokoneelle, kahdessa vaiheessa.',
  'product.one.f7': 'Natiivien työpöytä- ja mobiilisovellusten lataukset.',
  'product.one.f8': 'Tervetuloapuri, joka valmistelee organisaation ilman käyttöoppaita.',
  'product.mail.f1':
    'Keskusteluketjut, lukeminen, vastaaminen ja edelleenlähetys yhdellä napsautuksella.',
  'product.mail.f2':
    'Kansiot, väritunnisteet ja haku operaattoreilla (from:, has:attachment, is:unread…).',
  'product.mail.f3':
    'Automaattiset säännöt, jotka arkistoivat, merkitsevät tai siirtävät saapuvan postin.',
  'product.mail.f4': 'Ajastettu lähetys ja ”Peru lähetys”.',
  'product.mail.f5': 'Allekirjoitukset, lähettäjäaliakset ja automaattinen lomavastaus.',
  'product.mail.f6': 'Jaetut ja delegoidut postilaatikot luku- ja lähetysoikeuksin.',
  'product.mail.f7':
    'Muotoillun tekstin editori ja liitteet Sytue Drivesta ilman tiedostojen kopiointia.',
  'product.mail.f8':
    'Etäsisältö on oletuksena estetty: kukaan ei saa tietää, milloin avaat sähköpostin.',
  'product.mail.f9':
    'Allekirjoitettu posti (PGP ja S/MIME) tunnistetaan ja merkitään; TLS siirron aikana.',
  'product.mail.f10':
    'Käyttö myös Thunderbirdista, Outlookista, Apple Mailista tai puhelimesta sovellussalasanoilla.',
  'product.mail.f11':
    'Vastaanottajat tunnistepaloina, Contactsin osoitekirja yhden napsautuksen päässä ja varoitus, jos kirjoitat ”liitteenä” liittämättä mitään.',
  'product.mail.f12':
    'Säännöt suoraan avatusta sähköpostista, roskakorin tyhjennys kerralla ja pikanäppäimet ”?”-näppäimellä.',
  'product.drive.f1':
    'Oma asema, tiimiasemat, kanssani jaetut, viimeaikaiset, suosikit ja roskakori.',
  'product.drive.f2':
    'Lataa vetämällä; kansiot, siirto, kopiointi, uudelleennimeäminen ja monivalinta.',
  'product.drive.f3':
    'Jakolinkit kahdella tasolla (katselu tai muokkaus), salasanalla ja vanhentumisella.',
  'product.drive.f4': 'Versiohistoria palautuksineen.',
  'product.drive.f5': 'Asiakirjojen, kuvien ja PDF-tiedostojen esikatselu lataamatta.',
  'product.drive.f6':
    'Toimistoasiakirjojen avaaminen ja muokkaus Docusissa, Calcusissa ja Diaposissa; PDF:t CertiPDF:ssä.',
  'product.drive.f7':
    'Luottamuksellisuustunnisteet, jotka estävät julkiset linkit sieltä, minne ne eivät kuulu.',
  'product.drive.f8': 'Jaa Roomsin kanavalle tai liitä Mailiin ilman tiedoston kopiointia.',
  'product.drive.f9': 'WebDAV-yhteys ja työpöytäsovellus, jolla näet tiedostosi tietokoneellasi.',
  'product.drive.f10': 'S3-tallennus eurooppalaisilla alueilla, kiintiö aina näkyvissä.',
  'product.docs.f1':
    'Reaaliaikainen yhteismuokkaus läsnäolon kera ja toisten muutoksia sotkematta.',
  'product.docs.f2':
    'Välilehtinen valintanauha: Aloitus, Lisää, Viittaukset, Postitukset ja Tarkista.',
  'product.docs.f3': 'Taulukot, kuvat, muodot, SmartArt, kaaviot, symbolit ja vesileimat.',
  'product.docs.f4': 'Viittaukset: sisällysluettelo, alaviitteet, lainaukset ja lähdeluettelo.',
  'product.docs.f5': 'Postitukset: joukkokirjeet ja tarrat tietojen yhdistämisellä.',
  'product.docs.f6': 'Kommenttiketjut, muutosten tarkistus ja versiot.',
  'product.docs.f7':
    'Valinnainen tekoälyavustaja uudelleenkirjoittamiseen, kääntämiseen tai tiivistämiseen, vain jos organisaatiosi ottaa sen käyttöön.',
  'product.docs.f8': 'Vienti ODT-, DOCX- ja PDF-muotoihin; Word-tiedostojen avaaminen Drivesta.',
  'product.docs.f9': 'Ylä- ja alatunnisteet, sivunumerointi ja uudelleenkäytettävät lohkot.',
  'product.docs.f10': 'Automaattinen tallennus eurooppalaiseen Driveesi historian kera.',
  'product.docs.f11':
    'Lataus etusivulta PDF-, HTML-, Markdown-, OpenDocument- tai Microsoft Office -muodossa.',
  'product.sheets.f1':
    'Kaavat asiayhteyteen sopivalla ohjeella ja välittömällä uudelleenlaskennalla.',
  'product.sheets.f2':
    'Ehdollinen muotoilu, tietojen kelpoisuustarkistus ja alueasetusten mukaiset lukumuodot.',
  'product.sheets.f3': 'Monenlaiset kaaviot, minikaaviot ja osittajat.',
  'product.sheets.f4': 'Tietotaulukot summineen ja lajitteluineen; pivot-taulukko.',
  'product.sheets.f5': 'Ulkoiset tiedot: yhdistä lähde ja tee siihen kyselyjä taulukosta.',
  'product.sheets.f6': 'Ratkaisin tavoitteen täyttävän arvon löytämiseen.',
  'product.sheets.f7': 'Makrot toistuvien tehtävien automatisointiin.',
  'product.sheets.f8': 'Reaaliaikainen yhteistyö läsnäolon ja kommenttien kera.',
  'product.sheets.f9': 'Suodattimet, osittajat ja haku taulukossa.',
  'product.sheets.f10':
    'Vienti ODS-, XLSX- ja PDF-muotoihin; Excel-tiedostojen avaaminen Drivesta.',
  'product.slides.f1': 'Teemat ja paletit organisaatiosi brändin mukaisesti.',
  'product.slides.f2': 'Pohja muodoille, kuville, kaavioille, SmartArtille ja yhdysviivoille.',
  'product.slides.f3': 'Esitä koko näytöllä esittäjänäkymän ja muistiinpanojen kera.',
  'product.slides.f4': 'Reaaliaikainen yhteistyö läsnäolon ja kommenttien kera.',
  'product.slides.f5': 'Diat taustoilla, julisteilla ja asetteluilla.',
  'product.slides.f6': 'Vienti ODP-, PPTX- ja PDF-muotoihin.',
  'product.slides.f7': 'PowerPoint-tiedostojen avaaminen Drivesta.',
  'product.slides.f8': 'Automaattinen tallennus versioineen.',
  'product.pdf.f1':
    'Nopea katseluohjelma jäsennyksellä, pikkukuvilla, haulla ja zoomauksella sivun tai leveyden mukaan.',
  'product.pdf.f2':
    'Merkinnät: korostus, alleviivaus, yliviivaus, kommentit, piirrokset, muodot, nuolet ja leimat.',
  'product.pdf.f3': 'Tekstiruudut, mittaukset ja alueen tilannekuvat.',
  'product.pdf.f4': 'Lomakkeet: täytä olemassa olevia kenttiä tai lisää omia.',
  'product.pdf.f5': 'Asiakirjojen allekirjoituspaneeli.',
  'product.pdf.f6':
    'Pysyvä peittäminen: merkitsemäsi sisältö katoaa tiedostosta, ei vain näkymästä.',
  'product.pdf.f7':
    'Sivut: yhdistä, poimi, poista, kierrä, numeroi (myös Bates), ylä- ja alatunnisteet.',
  'product.pdf.f8': 'Vesileima, tausta ja muokattavat metatiedot.',
  'product.pdf.f9': 'Salasanalla suojatut asiakirjat ja oikeudet.',
  'product.pdf.f10': 'Ääneenluku ja kommenttiketjut.',
  'product.calendar.f1':
    'Kuukausi-, viikko-, päivä- ja esityslistanäkymät, minikalenteri ja ”Tänään”.',
  'product.calendar.f2':
    'Päivittäinen, viikoittainen tai kuukausittainen toisto päättymispäivällä; muokkaa vain tätä, tätä ja seuraavia tai kaikkia.',
  'product.calendar.f3': 'Tapahtumakohtaiset muistutukset ja ilmoitukset kokonaisuudessa.',
  'product.calendar.f4': 'Kutsut vastauksineen (osallistun, ehkä, en) ja seurannalla.',
  'product.calendar.f5': '”Etsi aika”, joka sopii kaikille osallistujille.',
  'product.calendar.f6': 'Varattavat tilat ja resurssit saatavuustietoineen.',
  'product.calendar.f7': 'Jaetut kalenterit oikeuksineen sekä kanssasi jaetut kalenterit.',
  'product.calendar.f8':
    'Tilaa ulkoisia kalentereita (ICS) ja käytä useita värikoodattuja kalentereita.',
  'product.calendar.f9': 'Luo tapahtuma Mailin viestistä tai muistiinpanosta.',
  'product.calendar.f10':
    'Toimii minkä tahansa CalDAV-asiakasohjelman kanssa: Apple Calendar, Thunderbird ja muut.',
  'product.calendar.f11':
    'Monista tapahtuma tai kopioi se kalenteriisi ja lataa .ics-tiedostona Sytuen ulkopuolisille.',
  'product.contacts.f1': 'Osoitekirjat, myös organisaatiosi yhteinen osoitekirja.',
  'product.contacts.f2':
    'Täydellinen kortti: puhelinnumerot, sähköpostit, osoitteet, organisaatio, syntymäpäivä, muistiinpanot ja kuva.',
  'product.contacts.f3': 'Tunnisteet ja suosikit ryhmittelyyn ja löytämiseen.',
  'product.contacts.f4': 'Kaksoiskappaleiden tunnistus ja yhdistäminen tietoja menettämättä.',
  'product.contacts.f5': 'vCard- ja CSV-tuonti ja -vienti; yksittäisen yhteystiedon vienti.',
  'product.contacts.f6': 'Joukkotoiminnot ja palautuksen mahdollistava roskakori.',
  'product.contacts.f7': 'Välitön haku ja automaattinen täydennys, kun kirjoitat Mailissa.',
  'product.contacts.f8': 'Toimii minkä tahansa CardDAV-asiakasohjelman kanssa.',
  'product.contacts.f9': 'Tulevat syntymäpäivät osoitekirjastasi, tämän kuun päivät korostettuina.',
  'product.contacts.f10':
    'Luo tapaaminen yhteystiedon kanssa tai kopioi yhteystiedon sähköposti poistumatta kortilta.',
  'product.notes.f1':
    'Sisäkkäiset muistikirjat kuvakkeella ja värillä; kolme asettelua: vapaa, sarakkeet ja luettelo.',
  'product.notes.f2':
    'Kaksitoista muistiinpanotyyppiä: teksti, tehtävät, linkit, väri, kuva sekä viittaukset Driveen, Docusiin, Calendariin, Roomsiin ja muihin muistiinpanoihin.',
  'product.notes.f3':
    'Muotoiluja tukeva editori (lihavointi, luettelot, linkit), joka tallentaa, kun poistut.',
  'product.notes.f4':
    'Tunnisteet, kiinnitys, monistus, siirto muistikirjojen välillä, ryhmittely ja vetäminen pohjalla.',
  'product.notes.f5':
    'Näkyvyys muistikirjoittain: yksityinen, henkilökohtainen, organisaatio tai julkinen linkki.',
  'product.notes.f6':
    'Salaus selaimessa arkaluonteisille muistikirjoille: Sytue ei voi lukea niitä.',
  'product.notes.f7': 'Sinä päätät, mitkä muistikirjat päätyvät kokonaisuuden hakuun.',
  'product.notes.f8': 'Paluulinkit: näe, mitkä muistiinpanot viittaavat tähän.',
  'product.notes.f9':
    'Vie koko muistikirja HTML-, PDF-, Markdown- ja muihin muotoihin; tuo kopioita.',
  'product.notes.f10':
    'Jaa muistiinpano Roomsin kanavalle; pudota tai liitä luodaksesi muistiinpanoja.',
  'product.notes.f11':
    'Taulukot editorissa ja Markdown-pikakomennot kirjoittaessa: ”- ”, ”# ”, ”> ” ja ”```”.',
  'product.notes.f12':
    'Siirrä muistiinpano toiseen muistikirjaan, vie yksittäinen muistiinpano ja lajittele muistikirja haluamallasi tavalla.',
  'product.photos.tagline': 'Kuvasi järjestyksessä ja makusi mukaan, poistumatta Euroopasta.',
  'product.photos.standard': 'Alkuperäiset Sytue Drivessa; muokkausmoottori selaimessasi',
  'product.photos.p1': 'Kuukausittainen aikajana nimihaulla.',
  'product.photos.p2': 'Albumit kansikuvineen; koko näytön katselu.',
  'product.photos.p3':
    'Editori valolle, värille, kierrolle, peilaukselle ja rajaukselle reaaliajassa.',
  'product.photos.p4': 'Jokainen muokkaus on resepti: alkuperäiseen ei koskaan kosketa.',
  'product.photos.f1': 'Kuukausittain ryhmitelty aikajana nimihaulla ja terävillä pikkukuvilla.',
  'product.photos.f2':
    'Albumit: luo, nimeä uudelleen, valitse kansikuva, lisää katselusta ja poista.',
  'product.photos.f3': 'Koko näytön katselu näppäimistöllä selattavana.',
  'product.photos.f4':
    'Editori neljällä liukusäätimellä (valotus, kontrasti, kylläisyys, lämpötila), kierrolla ja peilauksella.',
  'product.photos.f5': 'Rajaa vetämällä ja käytä pikanäppäimiä; GPU-esikatselu ja CPU varalla.',
  'product.photos.f6':
    'Jokainen muokkaus on erikseen tallennettu resepti: Driven alkuperäistä ei koskaan muuteta.',
  'product.photos.f7':
    'Vienti täydellä resoluutiolla, renderöitynä palvelimella samoilla kaavoilla kuin esikatselussa.',
  'product.photos.f8': 'Roskakori palautuksineen.',
  'product.photos.f9':
    'Alkuperäiset ovat Sytue Drivessasi; johdannaiset ovat uudelleen luotava välimuisti.',
  'product.photos.f10': 'EXIF-tiedot luetaan itse tiedostosta; kaikki Euroopan unionissa.',
  'product.photos.f11':
    'Monivalinta kuukausittain: uuteen albumiin, roskakoriin tai ladattavaksi kerralla.',
  'product.photos.f12':
    'Diaesitys, täysi koko, kierto katselusta ja rajaus kiinteällä kuvasuhteella.',
  'product.rooms.f1': 'Julkiset ja yksityiset kanavat luokissa; yksityisviestit ja ryhmät.',
  'product.rooms.f2':
    'Ketjut, reaktiot, maininnat, kiinnitetyt ja tallennetut viestit sekä luonnokset.',
  'product.rooms.f3':
    'Muotoilu ”Aa”-palkilla ja pikanäppäimillä: lihavointi, kursiivi, koodi, lainaukset, luettelot ja linkit.',
  'product.rooms.f4': 'Liitteet, kuvat, GIFit ja omat emojit; tiedostot Drivesta.',
  'product.rooms.f5': 'Ajastettu lähetys, prioriteetti, lukukuittaus ja ”nähneet”.',
  'product.rooms.f6': 'Kauttaviivakomennot ja kortit: äänestykset ja osoitettavat tehtävät.',
  'product.rooms.f7': 'Videopuhelut kanavalla: kamera, näyttö, käden nosto, chat ja tallennus.',
  'product.rooms.f8': 'Ajastetut kokoukset ja kutsulinkit.',
  'product.rooms.f9': 'Botit, webhookit ja integraatiot muihin työkaluihin.',
  'product.rooms.f10': 'Läsnäolo ja mukautettu tila; kanavakohtaiset ilmoitusasetukset.',
  'product.rooms.f11': 'Hae historiasta, myös koko kokonaisuudesta.',
  'product.rooms.f12':
    'Reaaliaikaisuus WebSocketin kautta, uudelleenyhdistyksellä ja ilman kadonneita viestejä.',
  'product.rooms.f13':
    'Nopea kanavanvaihto Ctrl+G:llä, kaiken merkitseminen luetuksi ja pikanäppäimet ”?”-näppäimellä.',
  'product.cast.tagline':
    'Sinun videosi ja suorat lähetyksesi, tilauksesta ja reaaliajassa, poistumatta Euroopasta.',
  'product.cast.standard': 'HLS-toisto selaimessa; RTMP-syöttö mistä tahansa lähetysohjelmasta',
  'product.cast.p1': 'Lataa video, niin se koodataan useaan laatuun mille tahansa näytölle.',
  'product.cast.p2': 'Lähetä suorana lähetysohjelmasta, selaimesta tai sovelluksesta.',
  'product.cast.p3': 'Neljä näkyvyystasoa: julkinen, piilotettu, organisaatio tai tietyt henkilöt.',
  'product.cast.p4': 'Suora keskustelu ja kommentit, julkaisijan perusmoderoinnilla.',
  'product.cast.f1': 'Lataa enintään 8 GiB:n videoita; alkuperäinen säilytetään.',
  'product.cast.f2': 'Mukautuva toisto kolmessa laadussa; selain valitsee parhaan.',
  'product.cast.f3':
    'Suora lähetys lähetysohjelmasta RTMP:llä, henkilökohtaisella lähetysavaimella.',
  'product.cast.f4':
    'Suorana selaimesta tai sovelluksesta: kamera ja mikrofoni. Näytön jakaminen tuetuissa selaimissa, työpöytäjärjestelmissä ja Androidissa; ei saatavilla iOS/iPadOS-sovelluksessa.',
  'product.cast.f5': 'Suorien lähetysten tallenteet muuttuvat tilausvideoiksi automaattisesti.',
  'product.cast.f6': 'Julkinen etusivu ja kanava jokaiselle organisaatiolle, indeksoitavissa.',
  'product.cast.f7':
    'Piilotetut linkit, videot vain organisaatiolle tai pääsy tietyille henkilöille @käyttäjänimellä.',
  'product.cast.f8':
    'Suora keskustelu ja ketjutetut kommentit, joita moderoivat julkaisija ja organisaation ylläpitäjät.',
  'product.cast.f9': 'Upotettava soitin ja oEmbed julkisille ja piilotetuille videoille.',
  'product.cast.f10': 'Tallennustila lasketaan Sytue Drive -kiintiöösi; kaikki Euroopan unionissa.',
  'product.cast.f11':
    '”Lataa Castiin” Sytue Drivesta: kopio videosta siirtyy Castiin, ja tiedosto jää Driveen.',
  'product.pages.tagline':
    'Yrityksesi verkkosivusto ja verkkokauppa, suunniteltu visuaalisella editorilla ja julkaistu poistumatta Euroopasta.',
  'product.pages.standard':
    'Staattista, saavutettavaa HTML:ää ja CSS:ää ilman kolmansien osapuolten evästeitä; vienti Astroon',
  'product.pages.p1':
    'Visuaalinen editori kehyksillä, jotka mukautuvat itse työpöydälle, tabletille ja matkapuhelimelle.',
  'product.pages.p2':
    'Kymmenen toimialakohtaista mallipohjaa oikeine teksteineen ja vapaasti käytettävine valokuvineen.',
  'product.pages.p3':
    'Verkkokauppa, jossa on tuoteluettelo, ostoskori ja maksut yrityksesi tilille.',
  'product.pages.p4':
    'Oma verkkotunnus automaattisella varmenteella tai osoite sites.sytue.eu-palvelussa.',
  'product.pages.f1':
    'Kehyspohjainen visuaalinen editori automaattisella asettelulla, uudelleenkäytettävillä komponenteilla ja valmiilla osioilla.',
  'product.pages.f2':
    'Kolme katkaisukohtaa (työpöytä, tabletti ja matkapuhelin), joilla jokaisella on omat säätönsä.',
  'product.pages.f3': 'Kävijäanalytiikka ilman evästeitä sivuston hallintanäkymässä.',
  'product.pages.f4':
    'Monikieliset verkkosivustot kielikohtaisine osoitteineen, hreflang-merkintöineen ja Sytue AI:n avustamalla käännöksellä.',
  'product.pages.f5':
    'Kuvat tietokoneeltasi, Sytue Drivesta tai Sytue Photosista, ja vaihtoehtoinen teksti kysytään lisäyksen yhteydessä.',
  'product.pages.f6':
    'Verkkokauppa, jossa on tuotevaihtoehdot, varastosaldo, tuotekohtainen alv ja maksu Mollie-, Adyen-, Redsys- tai Stripe-palvelun kautta.',
  'product.pages.f7':
    'Tilaukset, joihin Sytue ERP laatii yksinkertaistetun laskun, ja peruuttaminen suoraan verkkosivustolta.',
  'product.pages.f8':
    'Lomakkeet suostumuksineen, viestien postilaatikko ja ilmoitukset organisaatiosi jäsenille.',
  'product.pages.f9':
    'Nimetyt versiot, esikatselu salaisen linkin takana ja palaaminen mihin tahansa aiempaan julkaisuun.',
  'product.pages.f10':
    'Koodin vienti Astroon, Reactiin, Next.js:ään ja natiivisovelluksiin sekä tuoteluettelon vienti Shopifyyn, WooCommerceen ja muille alustoille.',
  'product.bases.tagline':
    'Linkitetyt taulukot asiakkaiden, tilausten tai varaston hallintaan, käyttöoikeuksin ja jokaisen muutoksen lokilla.',
  'product.bases.standard': 'Avoimet CSV ja JSON; skeeman ja suhteet takaa itse tietokanta',
  'product.bases.p1':
    'Asiakkaat, tilaukset tai varasto linkitetyissä taulukoissa, ilman tietokantaosaamista.',
  'product.bases.p2': 'Jaa organisaatiosi kanssa valitsemallasi roolilla.',
  'product.bases.p3': 'Jokainen muutos kirjataan: kuka sen teki ja mikä arvo oli ennen.',
  'product.bases.p4': 'Tietosi pysyvät EU:ssa ja viet ne milloin haluat.',
  'product.bases.f1':
    'Taulukot, joissa on teksti-, luku-, päivämäärä-, valintaruutu-, yksittäis- ja monivalintakentät.',
  'product.bases.f2':
    'Taulukoiden väliset suhteet: linkitä tilaus asiakkaaseensa, eikä tietokanta anna katkaista linkkiä.',
  'product.bases.f3':
    'Näppäimistöllä muokattava ruudukko, joka toimii ruudunlukijoilla ja puhelimella.',
  'product.bases.f4':
    'Käyttöoikeudet tietokannoittain ja taulukoittain: omistaja, muokkaaja, kommentoija ja lukija.',
  'product.bases.f5': 'Tuo CSV-tiedosto uutena taulukkona ja vie CSV- tai JSON-muodossa.',
  'product.bases.f6': 'Loki jokaisesta muutoksesta: kuka, mikä tietue ja kenttä, ennen ja jälkeen.',
  'product.cloud.f12': 'Käyttö, kustannukset ja eurooppalainen laskutus samassa konsolissa.',
  'product.cloud.f11': 'Jäsenet, API-tunnukset ja vietävä auditointiloki.',
  'product.cloud.f10': 'Yksityisyyttä kunnioittava verkkoanalytiikka ilman evästeitä.',
  'product.cloud.f9': 'Ajastetut tehtävät ja projektipohjat.',
  'product.cloud.f8': 'Pääte konttiin; käynnistä palvelu uudelleen tai keskeytä se.',
  'product.cloud.f7': 'Reaaliaikaiset lokit, mittarit ja valvonta hälytyksineen.',
  'product.cloud.f6': 'Ympäristömuuttujat ja salaisuudet ympäristöittäin.',
  'product.cloud.f5':
    'Omat verkkotunnukset automaattisella TLS:llä; uudelleenohjaukset ja otsakkeet.',
  'product.cloud.f4': 'S3-tallennus ja hallinnoidut funktiot.',
  'product.cloud.f3': 'Postgres-tietokannat automaattisella REST-rajapinnalla ja SQL-editorilla.',
  'product.cloud.f2':
    'Esikatselu jokaiselle PR:lle ja palautus mihin tahansa julkaisuun yhdellä napsautuksella.',
  'product.cloud.f1':
    'Projektit repositoriostasi (GitHub, GitLab tai oma Git), julkaisu jokaisella pushilla.',
  'product.console.f1':
    'Koontinäyttö, jossa näkyy organisaation tila ja se, mikä ansaitsee huomion tänään.',
  'product.console.f2':
    'Käyttäjät: perehdytys, jäädytys, salasanan ja MFA:n nollaus, istunnot, aliakset ja postilaatikko.',
  'product.console.f3':
    'Staattiset ryhmät ja sääntöihin perustuvat dynaamiset ryhmät; roolit oikeuksineen.',
  'product.console.f4': 'Etuoikeutettu pääsy (PIM): määräaikainen aktivointi hyväksynnällä.',
  'product.console.f5':
    'SSO SAML- tai OIDC-palveluntarjoajien kanssa yhteystestillä; käyttäjähallinta SCIM:llä.',
  'product.console.f6': 'Ehdollisen pääsyn säännöt ja luotettujen laitteiden luettelo.',
  'product.console.f7':
    'Integraatiot: OIDC-asiakkaat, SAML ja kolmansien osapuolten yhteydet; tuotteiden korvaaminen.',
  'product.console.f8': 'Sisällön tuonti: siirto muilta palveluntarjoajilta.',
  'product.console.f9': 'Asiakirjaeditori ja hallittavat Rooms-kanavat.',
  'product.console.f10':
    'Hallinto: oikeudelliset säilytysvelvoitteet, luottamuksellisuustunnisteet ja salasanakäytäntö.',
  'product.console.f11':
    'Auditointiloki CSV-viennillä ja edelleenlähetyksellä SIEM-järjestelmääsi; palvelulokit.',
  'product.console.f12':
    'Oma verkkotunnus ja brändi: organisaation logo ja sähköpostin verkkotunnus.',
  'product.id.f1':
    'Kertakirjautuminen (OpenID Connect ja OAuth 2.0 PKCE:llä) kokonaisuuteen ja sovelluksiisi.',
  'product.id.f2': 'Pääsyavaimet (WebAuthn) ja TOTP-MFA palautuskoodeineen.',
  'product.id.f3': 'Sovellussalasanat IMAP:lle ja asiakasohjelmille, jotka eivät tue OIDC:tä.',
  'product.id.f4': 'Istunnot ja laitteet: peru yksi tai kaikki muut.',
  'product.id.f5': 'SAML-federointi IdP:si kanssa ja Sytue SAML-IdP:nä muille sovelluksille.',
  'product.id.f6': 'Käyttäjähallinta SCIM:llä hakemistostasi.',
  'product.id.f7': 'Kutsut, sähköpostin vahvistus ja salasanan palautus.',
  'product.id.f8': 'Profiili iPhonelle ja Macille (.mobileconfig) yhdellä napautuksella.',
  'product.id.f9': 'Organisaation asettamat salasanakäytännöt ja pääsysäännöt.',
  'product.id.f10': 'Dynaaminen asiakasrekisteröinti (DCR) ja standardinmukainen löytäminen.',
  'product.erp.f1':
    'Kaksikymmentäneljä moduulia ja kolmekymmentäkuusi mallia yhdessä palvelussa ja yhdessä tietokannassa.',
  'product.erp.f2':
    'Mallikatalogi on dataa: jokainen moduuli määrittelee kenttänsä, tilansa ja luettelosarakkeensa.',
  'product.erp.f3':
    'Uusi moduuli on yksi JSON-tiedosto — ei DDL:ää, ei migraatiota eikä taululukitusta.',
  'product.erp.f4':
    'Yksi rajapinta kaikille kolmellekymmenellekuudelle mallille ja yksi katalogista generoitu käyttöliittymä.',
  'product.erp.f5':
    'Kahdenkertainen kirjanpito, jossa summat ovat kokonaisina sentteinä; ei yhtään liukulukua.',
  'product.erp.f6': 'Tietokanta tarkistaa tasapainon tapahtuman vahvistuksessa, ei sovelluskoodi.',
  'product.erp.f7':
    'Suljetut viennit ovat muuttumattomia: niitä ei voi muokata, poistaa eikä niihin voi lisätä rivejä.',
  'product.erp.f8':
    'Yksi tosite, yksi vienti: uusintayritys tai kaksi avointa välilehteä ei voi kahdentaa tuottoja.',
  'product.erp.f9':
    'Yhteiset nimikkeet ovat kaikilla 26 kielellä, ja kääntämätön kenttä estää palvelua käynnistymästä.',
  'product.erp.f10':
    'Indekseihin perustuvat luettelot: kahdenkymmenentuhannen rivin ensimmäinen sivu palautetaan alle millisekunnissa.',
  'meta.downloads.title': 'Lataukset — Sytue One tietokoneelle ja mobiiliin',
  'meta.downloads.description':
    'Natiivit Sytue One -sovellukset macOS:lle, Windowsille, Linuxille, iOS:lle ja Androidille. Koko kokonaisuus laitteellasi kunkin järjestelmän omin integraatioin.',
  'downloads.overline': 'Sovellukset',
  'downloads.h1': 'Sytue One laitteellasi',
  'downloads.intro':
    'Oikeat työpöytä- ja mobiilisovellukset: ne asennetaan, ne asuvat Dockissa tai Käynnistä-valikossa, ilmoittavat järjestelmän ilmoituksilla ja avaavat tiedostoja laitteeltasi.',
  'downloads.desktop.title': 'Tietokone',
  'downloads.desktop.body':
    'Jokaiselle tuotteelle oma natiivisovellus, joka piirtää käyttöliittymänsä itse näytönohjaimella: sisällä ei ole selainta, joten se käynnistyy nopeasti ja kuluttaa kuin järjestelmän sovellus.',
  'downloads.apps.title': 'Vain se, mitä tarvitset',
  'downloads.apps.body':
    'Sytue One kattaa koko työpäivän. Jos haluat vain aseman, Sytue Driven voi asentaa erikseen.',
  'downloads.includes': 'Sisältää:',
  'downloads.mobile.title': 'Mobiili',
  'downloads.mobile.body':
    'Natiivit iOS- ja Android-sovellukset järjestelmäilmoituksilla, jakamisella sekä pääsyllä tiedostoihin ja kameraan.',
  'downloads.selfhost.title': 'Onko organisaatiollasi oma verkkotunnus?',
  'downloads.selfhost.body':
    'Natiivisovellukset muodostavat yhteyden osoitteeseen sytue.eu. Jos organisaatiollasi on oma asennus, kirjaudu selaimella sen verkkotunnuksessa: sovelluskokonaisuus toimii verkossa samalla tavalla.',
  'downloads.yourSystem': 'Järjestelmäsi',
  'downloads.signInToGet': 'Kirjaudu sisään ladataksesi',
  'downloads.store': 'Näytä kaupassa {format}',
  'downloads.soon': 'Tulossa kauppaan {format}',
  'downloads.browser.text':
    'Suositko selainta? Koko kokonaisuus toimii verkossa samoin, mitään asentamatta:',
  'downloads.browser.link': 'avaa Sytue One',
  'platform.macos.req': 'macOS 11 tai uudempi · Apple Silicon ja Intel',
  'platform.windows.req': 'Windows 10 tai uudempi · 64-bittinen',
  'platform.linux.req': 'OpenGL 2.1 · 64-bittinen',
  'platform.ios.req': 'iOS 14 tai uudempi',
  'platform.android.req': 'Android 6.0 tai uudempi',
  'footer.tagline':
    'Jokainen kokonaisuuden sovellus on oma palvelumme, rakennettu avoimille, julkaistuille protokollille.',
  'footer.products': 'Tuotteet',
  'footer.resources': 'Resurssit',
  'footer.legal': 'Oikeudelliset tiedot',
  'footer.suite': 'Kokonaisuus',
  'footer.downloads': 'Lataa sovellukset',
  'footer.sales': 'Ota yhteyttä myyntiin',
  'footer.privacy': 'Tietosuoja ja GDPR',
  'footer.accessibility': 'Saavutettavuus',
  'footer.openSource': 'Avoin lähdekoodi',
  'footer.legalNotice': 'Oikeudellinen ilmoitus',
  'footer.terms': 'Ehdot',
  'footer.copyright':
    '© {year} Sytue · Tiedot isännöity EU:ssa (Scaleway fr-par / nl-ams · oma vyöhyke Madridissa).',
  'footer.privacyNotice':
    'Emme myy tietojasi emmekä jaa niitä kolmansille osapuolille. Emme käytä niitä profilointiin, mainontaan emmekä omien tai muiden palvelujen kouluttamiseen tai parantamiseen. Ne on salattu siirron aikana ja levossa, ja ne poistuvat eurooppalaisesta infrastruktuuristamme vain siltä osin kuin laskutus välttämättä vaatii, ilmoitusten lähettämiseksi sovelluksen ollessa suljettuna (push-palvelu näkee vain laitteesi ja kellonajan, ei koskaan sisältöä) sekä käyttöön ottamissasi integraatioissa, joissa kolmas osapuoli pyytää hyväksyntäsi käyttämilleen tiedoille.',
  'meta.notFound.title': 'Sivua ei löytynyt — Sytue One',
  'notFound.overline': 'Virhe 404',
  'notFound.title': 'Tätä sivua ei löytynyt',
  'notFound.body': 'Linkki on voinut muuttua. Palaa etusivulle tai tutustu kokonaisuuteen.',
  'notFound.home': 'Takaisin etusivulle',
  'meta.legal.title': 'Oikeudellinen ilmoitus — Sytue One',
  'meta.legal.description':
    'Sivuston www.sytue.eu ja Sytue One -palvelun omistajan tunnistetiedot: Sytue Projects, S.L.',
  'aviso.title': 'Oikeudellinen ilmoitus',
  'aviso.intro':
    'Espanjan tietoyhteiskunnan palveluista ja sähköisestä kaupankäynnistä 11. heinäkuuta 2002 annetun lain 34/2002 (LSSI) 10 artiklan mukaisesti tässä ovat tämän verkkosivuston ja Sytue One -palvelun omistajan tunnistetiedot.',
  'aviso.review.body':
    'Tämän ilmoituksen rekisteritiedot on tarkistettu kaupparekisteristä. Muu teksti on oikeudellisessa tarkastuksessa, ja sen sanamuoto voi muuttua ilman, että kuvattu asia muuttuu.',
  'aviso.owner.title': 'Omistaja',
  'aviso.owner.name': 'Toiminimi',
  'aviso.owner.form': 'Oikeudellinen muoto',
  'aviso.owner.formValue':
    'Yhden osakkaan osakeyhtiö (sociedad limitada unipersonal) Espanjan lainsäädännön mukaan',
  'aviso.owner.nif': 'Verotunniste (NIF)',
  'aviso.owner.address': 'Kotipaikka',
  'aviso.owner.registry': 'Rekisterimerkintä',
  'aviso.owner.registryValue':
    'Madridin kaupparekisteri (Registro Mercantil de Madrid), osasto 8, lehti M-887153, merkintä 1',
  'aviso.owner.email': 'Sähköposti',
  'aviso.owner.web': 'Verkkosivusto',
  'aviso.scope.title': 'Tarkoitus',
  'aviso.scope.body':
    'Tämä verkkosivusto esittelee Sytue One -kokonaisuuden ja antaa pääsyn sen sovelluksiin. Palvelun käyttöä säätelevät palveluehdot ja henkilötietojen käsittelyä tietosuojakäytäntö; molempiin on linkki jokaisen sivun alatunnisteessa.',
  'aviso.ip.title': 'Immateriaalioikeudet',
  'aviso.ip.body':
    'Tämän sivuston sisältö (tekstit, muotoilu, koodi sekä Sytue- ja Sytue One -tavaramerkit ja -logot) kuuluu Sytue Projects, S.L:lle tai sitä käytetään lisenssillä. Yhteensopivuusviitteinä mainitut kolmansien osapuolten tuotenimet ovat omistajiensa tavaramerkkejä.',
  'aviso.liability.title': 'Vastuu',
  'aviso.liability.body':
    'Sytue Projects, S.L. pyrkii pitämään tämän sivuston saatavilla ja ajan tasalla takaamatta, ettei katkoksia tai virheitä esiinny. Yhtiö ei vastaa niiden kolmansien osapuolten sivustojen sisällöstä, joihin se linkittää.',
  'aviso.law.title': 'Sovellettava laki',
  'aviso.law.body':
    'Tähän ilmoitukseen sovelletaan Espanjan lakia. Jos olet kuluttaja, säilytät oikeudet ja oikeuspaikan, jotka Euroopan unionissa sijaitsevan asuinmaasi laki sinulle antaa.',
  'aviso.updated': 'Päivitetty viimeksi: syyskuu 2026.',
  'meta.accessibility.title': 'Saavutettavuusseloste — Sytue One',
  'meta.accessibility.description':
    'Kuinka saavutettavia Sytue One -kokonaisuus ja tämä verkkosivusto ovat, mitä mittaamme, mikä vielä puuttuu ja miten voit kertoa meille.',
  'a11y.title': 'Saavutettavuusseloste',
  'a11y.intro':
    'Sytue haluaa, että jokainen voi käyttää Sytue Onea ja tätä verkkosivustoa riippumatta käytössä olevasta apuvälineestä. Tämä seloste koskee sivustoa www.sytue.eu ja kokonaisuuden verkkosovelluksia.',
  'a11y.standard.title': 'Standardi, jonka mukaan toimimme',
  'a11y.standard.body':
    'Suunnittelemme ja toteutamme WCAG 2.1 -ohjeiden AA-tason mukaisesti; siihen viittaavat eurooppalainen standardi EN 301 549 ja Euroopan esteettömyyssäädös (direktiivi (EU) 2019/882).',
  'a11y.status.title': 'Vaatimustenmukaisuuden tila',
  'a11y.status.body':
    'Osittain vaatimusten mukainen: suurin osa kokonaisuudesta täyttää standardin, ja alla luetellut osat eivät vielä täytä sitä kokonaan. Riippumatonta auditointia ei ole vielä teetetty.',
  'a11y.measures.title': 'Mitä teemme tavoitteen eteen',
  'a11y.measures.design':
    'Yksi suunnittelujärjestelmä koko kokonaisuudelle: tekstivärit tarkistetulla kontrastilla, näkyvä kohdistuksen korostus jokaisessa säätimessä sekä valikoiden, valintaikkunoiden ja luetteloiden käyttö näppäimistöllä.',
  'a11y.measures.ci':
    'Automaattiset saavutettavuustarkastukset (Lighthouse) ajetaan integraatioputkessa jokaisen julkisen verkkosivuston muutoksen yhteydessä.',
  'a11y.measures.system':
    'Käyttöliittymä noudattaa järjestelmäsi asetuksia: tumma tai vaalea teema, vähennetty liike ja tekstin koko, ilman kiinteitä pikselikokoja lomakekentissä.',
  'a11y.measures.text':
    'Jokaisella kuvalla, kuvakkeella ja tyhjällä tilalla on tekstivastine, ja jokainen sivu ilmoittaa kielensä, jotta ruudunlukijat ääntävät sen oikein.',
  'a11y.limits.title': 'Tunnetut rajoitukset',
  'a11y.limits.body':
    'Laskentataulukon ruudukko, diapohja ja PDF-merkinnät ovat monimutkaisia editoreja, jotka vaativat vielä työtä, jotta niitä voi käyttää täysin näppäimistöllä ja ruudunlukijalla. Käyttäjien lataamat asiakirjat ja tiedostot näytetään sellaisinaan, emmekä muokkaa niitä.',
  'a11y.feedback.title': 'Kerro, mikä ei toimi',
  'a11y.feedback.body':
    'Jos jokin kokonaisuudessa tai tällä sivustolla on sinulle vaikeaa tai mahdotonta käyttää, kirjoita meille, niin selvitämme asian ja palaamme asiaan:',
  'a11y.enforcement.title': 'Jos et ole tyytyväinen',
  'a11y.enforcement.body':
    'Jos olet ilmoittanut esteestä etkä ole tyytyväinen vastaukseemme, voit ottaa yhteyttä maasi saavutettavuudesta vastaavaan viranomaiseen Euroopan esteettömyyssäädöksen mukaisesti.',
  'a11y.updated':
    'Tämä seloste on laadittu syyskuussa 2026, ja se tarkistetaan, kun kokonaisuus muuttuu olennaisesti.',
  'meta.openSource.title': 'Avoin lähdekoodi — Sytue One',
  'meta.openSource.description':
    'Sytue rakentaa lähes kaiken alusta asti, mutta ei keksi uudelleen sitä, mikä jo toimii hyvin. Avoimen lähdekoodin projektit, joihin sarja nojaa.',
  'oss.title': 'Avoin lähdekoodi',
  'oss.intro':
    'Sytue rakentaa lähes kaiken alusta asti: sähköpostin, editorit, kalenterin. Mutta se ei keksi uudelleen sitä, mikä jo toimii hyvin. Nämä ovat avoimen lähdekoodin projekteja, joihin jotkin sarjan osat nojaavat.',
  'oss.livekit.body': 'Sytue Roomsin puheluiden reaaliaikainen ääni ja video toimivat LiveKitillä.',
  'oss.postgres.body':
    'Jokainen sarjan palvelu tallentaa tietonsa PostgreSQL-tietokantaan, yksi tietokanta per organisaatio.',
  'meta.privacy.title': 'Tietosuoja ja GDPR — Sytue One',
  'meta.privacy.description':
    'Miten Sytue One käsittelee henkilötietoja: rekisterinpitäjä, tiedot, oikeusperusteet, säilytys, käsittelijät, tietoturva, kansainväliset siirrot ja GDPR:n mukaiset oikeutesi.',
  'legal.overline': 'Oikeudelliset tiedot',
  'legal.updated': 'Päivitetty viimeksi: heinäkuu 2026.',
  'legal.review.title': 'Oikeudellisessa tarkastuksessa.',
  'privacy.title': 'Tietosuoja ja GDPR',
  'privacy.review.body':
    'Tämä käytäntö kuvaa tarkasti, miten käsittelemme tietoja tänään. Täsmälliset säilytysajat täydennetään lopulliseen versioon, jonka lakitiimimme tarkastaa.',
  'privacy.controller.title': 'Rekisterinpitäjä',
  'privacy.controller.body':
    'Sytue Projects, S.L. (espanjalainen verotunniste B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Espanja; merkitty Madridin kaupparekisteriin, osasto 8, lehti M-887153) on Sytue One -kokonaisuuden tarjoajana palvelun tilien ja organisaatioiden tietojen rekisterinpitäjä. Tietosuojayhteystieto:',
  'privacy.data.title': 'Mitä tietoja käsittelemme',
  'privacy.data.account.label': 'Tilin tiedot:',
  'privacy.data.account.body':
    'nimi, sähköpostiosoite ja organisaatio, joita hallinnoi Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Organisaatiosi sisältö:',
  'privacy.data.content.body':
    'sähköposti, tiedostot, asiakirjat, kalenteri ja viestit, joita organisaatiosi isännöi kokonaisuudessa. Tämä sisältö kuuluu organisaatiollesi.',
  'privacy.data.technical.label': 'Tekniset käyttötiedot:',
  'privacy.data.technical.body':
    'käyttölokit ja yhteystiedot, jotka ovat tarpeen tietoturvan, väärinkäytösten estämisen ja palvelun toiminnan kannalta.',
  'privacy.data.note':
    'Emme myy tietoja emmekä käytä niitä mainontaan. Sytuen hallinnoima infrastruktuuri ja tallennus sijaitsevat Euroopan unionissa (Scaleway, alueet fr-par ja nl-ams sekä paikallinen infrastruktuuri Madridissa), ja tiedot salataan siirron aikana (TLS 1.3) ja tallennettuina. Yhteydet ulkoisiin palveluihin lähettävät hyväksymäsi tiedot niiden palveluntarjoajalle tämän sijainnin ja ehtojen mukaisesti.',
  'privacy.basis.title': 'Oikeusperuste',
  'privacy.basis.body':
    'Käsittely perustuu GDPR:n 6 artiklan oikeusperusteisiin: organisaatiosi kanssa tehdyn palvelusopimuksen täyttäminen, oikeutettu etumme tietoturvaan ja väärinkäytösten estämiseen, lakisääteisten velvoitteiden noudattaminen ja, kun sitä nimenomaisesti pyydetään, suostumuksesi. Yksityiskohdat perusteittain ja käsittelytoimittain sisältyvät lopulliseen tekstiin.',
  'privacy.retention.title': 'Säilytys',
  'privacy.retention.body':
    'Säilytämme tilin tiedot ja sisällön niin kauan kuin organisaatiosi pitää palvelun aktiivisena. Kun tili tai organisaatio suljetaan, tiedot poistetaan tai anonymisoidaan kohtuullisessa ajassa lukuun ottamatta sitä, mitä meidän on säilytettävä lain nojalla (esimerkiksi laskutus). Tekniset käyttölokit tyhjennetään säännöllisesti. Täsmälliset säilytysajat luokittain esitetään lopullisessa versiossa. Sisältöä koskevat ilmoitukset säilytetään niin kauan kuin ne ovat avoinna, ja ne poistetaan kaksitoista kuukautta käsittelyn jälkeen.',
  'privacy.processors.title': 'Käsittelijät ja alikäsittelijät',
  'privacy.processors.body':
    'Infrastruktuuripalvelujen tarjoajamme on Scaleway (alueet fr-par ja nl-ams), joka toimii henkilötietojen käsittelijänä yksinomaan Euroopan unionissa; lisäksi ylläpidämme omaa vyöhykettä Madridissa (Espanja) omissa tiloissamme. Tilausmaksuissa käytämme Mollie B.V:tä (Alankomaat), joka käsittelee vain maksuun tarvittavat tiedot. Emme käytä alikäsittelijöitä ETA:n ulkopuolella. Alikäsittelijöiden luettelo julkaistaan ja pidetään ajan tasalla, ja jokaisesta lisäyksestä ilmoitetaan etukäteen.',
  'privacy.security.title': 'Tietoturva',
  'privacy.security.body':
    'Sovellamme GDPR:n mukaisia teknisiä ja organisatorisia toimenpiteitä: salaus siirron aikana (TLS 1.3) ja levossa, organisaatioiden eristäminen (multi-tenant rivitason suojauksella), vahva tunnistautuminen (MFA/WebAuthn), käytön lokitus ja vähimpien oikeuksien periaate. Mikään järjestelmä ei ole erehtymätön: jos tietoturvaloukkaus koskee sinua, ilmoitamme siitä asetuksen määräajoissa.',
  'privacy.cookies.title': 'Evästeet',
  'privacy.cookies.body':
    'Käytämme vain teknisiä evästeitä, jotka ovat tarpeen kertakirjautumisen ja istunnon suojauksen kannalta. Emme käytä mainosevästeitä emmekä kolmansien osapuolten seurantaevästeitä, joten seurannalle ei tarvita suostumusmuuria.',
  'privacy.transfers.title': 'Kansainväliset siirrot',
  'privacy.transfers.body':
    'Sytuen hallinnoima tietojenkäsittely ja isännöinti tapahtuvat Euroopan unionin alueilla. Käyttäjän käyttöön ottamat ulkoiset yhteydet lähettävät hyväksytyt tiedot palveluntarjoajalleen; niiden sijainti riippuu palveluntarjoajasta ja sen asetuksista. Sovelluksen tai selaimen ollessa suljettuna lähetettävät ilmoitukset voivat myös käyttää ulkoisia palveluja. Laitteen push-palvelu (Google, Mozilla tai Apple selaimesi tai puhelimesi mukaan) näkee laitteesi anonyymin tunnisteen ja kunkin ilmoituksen kellonajan — ei koskaan sisältöä, joka kulkee päästä päähän salattuna ja lähetetään iOS:ssä tyhjänä. Voit poistaa ilmoitukset käytöstä, jolloin EU:sta ei poistu edes tämä.',
  'privacy.rights.title': 'Oikeutesi',
  'privacy.rights.body1':
    'GDPR antaa sinulle oikeuden tarkastaa, oikaista ja poistaa tietosi, vastustaa käsittelyä, rajoittaa sitä sekä siirtää tiedot järjestelmästä toiseen. Voit käyttää oikeuksiasi kirjoittamalla osoitteeseen',
  'privacy.rights.body2':
    '; vastaamme asetuksen määräajoissa. Sinulla on myös oikeus tehdä valitus',
  'privacy.rights.authority': 'valvontaviranomaisellesi',
  'privacy.dpo.title': 'Tietosuojavastaava (DPO)',
  'privacy.dpo.body':
    'Tietosuojaa koskevissa kysymyksissä tai tietosuojavastaavan tavoittamiseksi:',
  'privacy.minors.title': 'Alaikäiset',
  'privacy.minors.body':
    'Sytue One on palvelu organisaatioille ja ammatilliseen käyttöön; sitä ei ole suunnattu alaikäisille, emmekä tietoisesti kerää alaikäisten tietoja.',
  'privacy.changes.title': 'Tämän käytännön muutokset',
  'privacy.changes.body':
    'Jos päivitämme tätä käytäntöä, muutamme ”päivitetty viimeksi” -päivämäärän, ja jos muutokset ovat olennaisia, kerromme niistä palvelun kanavien kautta ennen niiden voimaantuloa.',
  'meta.terms.title': 'Palveluehdot — Sytue One',
  'meta.terms.description':
    'Sytue Onen palveluehdot: palveluntarjoaja, laajuus, tilit ja organisaatiot, hyväksyttävä käyttö, saatavuus, tietosi, immateriaalioikeudet ja sovellettava laki.',
  'terms.title': 'Palveluehdot',
  'terms.review.body':
    'Nämä ehdot kuvaavat palvelua ja sen käyttöä täsmälleen sellaisena kuin se tänään toimii. Täsmälliset sopimusehdot (palvelutaso, vastuu ja oikeuspaikka) täydennetään lopulliseen versioon, jonka lakitiimimme tarkastaa.',
  'terms.provider.title': 'Kuka palvelua tarjoaa',
  'terms.provider.body':
    'Sytue Onea tarjoaa Sytue Projects, S.L. (espanjalainen verotunniste B88729900; Calle Atenas, 10, 28822 Coslada, Madrid, Espanja; merkitty Madridin kaupparekisteriin, osasto 8, lehti M-887153) yksinomaan Euroopan unionissa sijaitsevasta infrastruktuurista (Scaleway, alueet fr-par ja nl-ams, sekä oma vyöhyke Madridissa), ja palveluun sovelletaan EU:n lainsäädäntöä.',
  'terms.scope.title': 'Palvelun laajuus',
  'terms.scope.body':
    'Sytue One on organisaatioiden tuottavuuspaketti: identiteetti (Sytue ID), sähköposti, tiedostot, asiakirjat, kalenteri, yhteystiedot ja viestintä videopuheluineen. Palvelusta sovitaan organisaatiokohtaisesti.',
  'terms.accounts.title': 'Tilit ja organisaatiot',
  'terms.accounts.body':
    'Jokainen organisaatio hallinnoi käyttäjiään, ryhmiään ja oikeuksiaan Sytue ID:n kautta (SSO + MFA). Organisaation hallinto vastaa tilien lisäämisestä ja poistamisesta sekä palvelun käytöstä niiden kautta.',
  'terms.use.title': 'Hyväksyttävä käyttö',
  'terms.use.body':
    'Sitoudut käyttämään palvelua lainmukaisesti etkä käytä sitä laittomaan toimintaan, roskapostiin, haittaohjelmien levittämiseen, kolmansien osapuolten oikeuksien loukkaamiseen tai mihinkään käyttöön, joka vaarantaa palvelun turvallisuuden tai saatavuuden muille organisaatioille.',
  'terms.availability.title': 'Saatavuus ja tuki',
  'terms.availability.body1':
    'Pyrimme korkeaan saatavuuteen ja teemme ylläpitoa ja parannuksia jatkuvasti. Täsmälliset palvelutasositoumukset (SLA), kun ne koskevat pakettiasi, on määritetty palvelusopimuksessa. Tukea annetaan osoitteessa',
  'terms.data.title': 'Tietosi',
  'terms.data.body1':
    'Sisältö, jota organisaatiosi isännöi kokonaisuudessa, kuuluu organisaatiollesi. Kokonaisuus on rakennettu avoimille standardeille (OIDC, JMAP, CalDAV, CardDAV, WebDAV), joten tietosi pysyvät siirrettävinä ja voit viedä ne mukanasi. Henkilötietojen käsittelyä kuvataan sivulla',
  'terms.ip.title': 'Immateriaalioikeudet',
  'terms.ip.body':
    'Sytue One -ohjelmisto on Sytuen alkuperäistä työtä, ja sitä suojaavat asianmukaiset immateriaalioikeudet. Säilytät omistusoikeuden sisältöön, jota organisaatiosi isännöi kokonaisuudessa; käsittelemme sitä vain palvelun tarjoamiseksi.',
  'terms.liability.title': 'Vastuu',
  'terms.liability.body':
    'Teemme kaikkemme luotettavan ja turvallisen palvelun eteen. Sovellettavat takuut ja vastuunrajoitukset on määritetty palvelusopimuksessa; mikään näissä ehdoissa ei rajaa pois vastuuta, jonka poissulkemista laki ei salli.',
  'terms.contact.title': 'Sopimukset ja yhteystiedot',
  'terms.contact.body1':
    'Sopimusehtoja, organisaatiokohtaisia hintoja tai muita palvelua koskevia kysymyksiä varten kirjoita osoitteeseen',
  'terms.changes.title': 'Muutokset',
  'terms.changes.body':
    'Voimme päivittää näitä ehtoja vastaamaan palvelun tai lainsäädännön muutoksia. Jos muutokset ovat olennaisia, kerromme niistä palvelun kanavien kautta ennen niiden voimaantuloa ja päivitämme ”päivitetty viimeksi” -päivämäärän.',
  'terms.law.title': 'Sovellettava laki',
  'terms.law.body':
    'Palveluun sovelletaan Euroopan unionin lainsäädäntöä ja palveluntarjoajan kotimaan lakia. Riitojen ratkaisun oikeuspaikka määritetään lopullisessa tekstissä. Tästä riippumatta säilytät oikeutesi tehdä valitus tietosuojaa valvovalle viranomaiselle.',
  'seo.org.description':
    'Sytue One: suvereeni eurooppalainen tuottavuuspaketti. Identiteetti, sähköposti, tiedostot, asiakirjat, kalenteri, chat, videopuhelut ja PaaS, 100 % EU:ssa.',
  'seo.org.slogan': 'Tuottavuutesi, suvereeni ja eurooppalainen.',
  'seo.software.description':
    'Suvereeni eurooppalainen tuottavuuspaketti: sähköposti (JMAP), tiedostot (WebDAV), yhteismuokattavat asiakirjat, kalenteri (CalDAV), yhteystiedot (CardDAV), chat, videopuhelut ja PaaS. SSO Sytue ID:llä.',
  'seo.software.f1': 'Kertakirjautuminen (SSO) + MFA/WebAuthn',
  'seo.software.f2':
    'Sähköposti, tiedostot, asiakirjat, kalenteri, yhteystiedot, chat ja videopuhelut',
  'seo.software.f3': 'Julkaisu- ja hallinnoitujen tietojen alusta',
  'seo.software.f4': 'Tiedot 100 % EU:ssa (Scaleway)',
  'seo.offer.description': 'Maksuton kokeilu, korttia ei tarvita.',
  'seo.downloads.description':
    'Sytue One -kokonaisuuden natiivisovellukset tietokoneelle (macOS, Windows, Linux) ja mobiiliin (iOS, Android).',
  'home.pricing.personal.title': 'Maksuton',
  'home.pricing.personal.freeStorage': '5 Gt, yhdelle henkilölle',
  'home.pricing.personal.freeBody':
    'Sähköposti, tiedostot, kuvat, asiakirjat, kalenteri, yhteystiedot, muistiinpanot ja huoneet. Ei korttia eikä aikarajaa.',
  'home.pricing.personal.note':
    'Organisaatiosi on sähköpostisi: ei organisaation hallintaa eikä ERP:tä, mutta kaikki muu, myös Sytue Admin integraatioillesi ja siirroillesi.',
  'home.pricing.personal.cta': 'Luo maksuton tili',
  'grid.orgOnly': 'Vain organisaatioille',
  'grid.enDesarrollo': 'Kehitteillä',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 Gt käyttäjää kohden',
  'home.pricing.paidBody':
    'Organisaatioille ja myös yksittäiselle henkilölle. Koko kokonaisuus, mukana Sytue ERP ja Sytue Admin organisaatiosi johtamiseen.',
  'home.pricing.thirdParty':
    'Jokainen paketti antaa yhdistää oman tallennustilan: Google Drive, Microsoft OneDrive, S3-säilö (Scaleway, OVH, Hetzner, Wasabi, MinIO), WebDAV-palvelin kuten Nextcloud tai Internxt, tai NAS omissa tiloissasi — ilman avoimia portteja, Sytuen tunnelin kautta. Yhdistettyihin tallennustiloihin tallennettujen tietojen varmuuskopiointi ja palautus ovat asiakkaan (sinun) vastuulla, ja Sytue® vapautetaan kaikesta vastuusta tietojen menetykseen Sytue®:en yhdistetyissä tallennustiloissa.',
  'home.pricing.soon': 'Tulossa pian',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 Tt käyttäjää kohden',
  'home.pricing.enterprise.support': 'Premium-tuella.',
  'home.pricing.education': 'Oppilaitokset: Pro 50 % edullisemmin.',
};
