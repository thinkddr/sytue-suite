/**
 * Suomenkieliset viestit tekoälyyn (ia).
 * Etuliite: ai.*
 */
export const ia = {
  // Asetukset
  'ai.settings.provider': 'Tekoälypalveluntarjoaja',
  'ai.settings.model': 'Malli',
  'ai.settings.key': 'Oma API-avain (BYOK)',
  'ai.settings.key_placeholder': 'Syötä salainen API-avaimesi…',
  'ai.settings.key_help':
    'Avaimesi tallennetaan AES-256-GCM-salattuna. Sitä ei koskaan palauteta selaimeen.',
  'ai.settings.check_key': 'Testaa avain',
  'ai.settings.delete_key': 'Poista avain',
  'ai.settings.status_configured': 'Avain määritetty',
  'ai.settings.tested_at': 'Vahvistettu',
  'ai.settings.quota_title': 'Tekoälyn kuukausikiintiö',
  'ai.settings.quota_reset': 'Nollautuu {date}',
  'ai.settings.notice_eu_title': 'Ilmoitus tietojen siirrosta ETA:n ulkopuolelle',
  'ai.settings.notice_eu_desc':
    'Valittu palveluntarjoaja ({provider}) käsittelee tietoja Euroopan talousalueen ulkopuolella. GDPR:n 49 artiklan 1 kohdan a alakohdan mukaisesti sinun on annettava nimenomainen suostumus tälle siirrolle jatkaaksesi.',
  'ai.settings.notice_eu_accept':
    'Hyväksyn tietojen kansainvälisen siirron ETA:n ulkopuolelle tämän palveluntarjoajan tekoälypyynnöissä.',
  'ai.settings.success_save': 'Avain tallennettu ja vahvistettu onnistuneesti.',
  'ai.settings.error_check': 'Virhe vahvistettaessa avainta palveluntarjoajalta.',
  'ai.settings.form_title': 'Määritä oma avain (BYOK)',
  'ai.settings.form_help':
    'Käytä omia Mistral AI-, OpenAI-, Anthropic- tai Zhipu AI -tunnuksiasi, jotta et ole riippuvainen kuukausikiintiöstä.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (Yhdysvallat)',
  'ai.settings.provider_anthropic': 'Anthropic (Yhdysvallat)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Ranska',
  'ai.region.us': 'Yhdysvallat',
  'ai.region.intl': 'Kansainvälinen',
  'ai.settings.show_key': 'Näytä avain',
  'ai.settings.hide_key': 'Piilota avain',
  'ai.settings.key_required': 'Syötä avain.',
  'ai.settings.notice_required': 'Sinun on hyväksyttävä ilmoitus siirroista ETA:n ulkopuolelle.',
  'ai.settings.check_ok': 'Avain tarkistettu onnistuneesti palveluntarjoajan kanssa.',
  'ai.settings.error_save': 'Avainta ei voitu tallentaa.',
  'ai.settings.error_delete': 'Avainta ei voitu poistaa.',
  'ai.settings.checking': 'Tarkistetaan…',
  'ai.settings.save_activate': 'Tallenna ja ota käyttöön',
  'ai.settings.key_deleted': 'Avain poistettu. Oletusarvoista kuukausikiintiötä käytetään.',
  'ai.settings.active_provider': 'Aktiivinen palveluntarjoaja: {provider} ({model})',
  'ai.settings.no_key_default': 'Omaa avainta ei ole asetettu. Tilauksen kuukausiraja on voimassa.',
  'ai.settings.status_plan': 'Tilauksen kiintiö',
  'ai.settings.quota_unlimited': 'Rajoittamaton',
  'ai.settings.quota_unlimited_help': 'Pyynnöt käsitellään omalla API-avaimellasi.',
  'ai.settings.quota_jobs': 'Suunnittelu-, käännös- ja vientityöt.',
  'ai.settings.loading': 'Ladataan tekoälyasetuksia',
  'ai.settings.load_error': 'Tekoälyasetuksia ei voitu ladata.',
  'ai.settings.load_error_help': 'Palveluun ei saatu yhteyttä. Asetuksesi eivät ole muuttuneet.',
  'ai.settings.key_invalid':
    'Palveluntarjoaja hylkäsi avaimen. Tarkista, että se on oikein ja voimassa.',
  'ai.settings.encryption_unavailable':
    'Palvelu ei ole vielä valmis tallentamaan avaimia turvallisesti. Ota yhteyttä tukeen.',
  'ai.settings.invalid_model': 'Valitse tälle palveluntarjoajalle kelvollinen malli.',
  'ai.settings.key_state': 'Avaimen tila',

  'ai.common.cancel': 'Peruuta',

  // Yhteinen konteksti: kiintiö, GDPR-ilmoitus, virheet ja työn tila
  'ai.context.quota_label': 'Tekoälyn kuukausikiintiö',
  'ai.context.quota_used': '{used}/{limit} työtä tässä kuussa',
  'ai.context.quota_reset': 'Uusiutuu {date}.',
  'ai.context.quota_exhausted_title': 'Olet käyttänyt tekoälyn kuukausikiintiön loppuun',
  'ai.context.quota_exhausted_desc':
    'Voit käynnistää lisää töitä {date}. Ylläpito voi poistaa rajoituksen lisäämällä oman API-avaimen tekoälyasetuksissa.',
  'ai.context.byok': 'Organisaatiosi käyttää omaa API-avaintaan: tämä työ ei kuluta kiintiötä.',
  'ai.context.loading': 'Tarkistetaan tekoälykiintiötä',
  'ai.context.load_error': 'Tekoälykiintiötä ei voitu tarkistaa. Voit silti yrittää.',
  'ai.context.retry': 'Yritä uudelleen',
  'ai.notice.eu': 'Tekstit käsitellään EU:n sisällä GDPR:n mukaisesti.',
  'ai.notice.non_eu_title': 'Tämä työ siirtyy Euroopan talousalueen ulkopuolelle',
  'ai.notice.non_eu_desc':
    'Organisaatiosi oma avain kuuluu palveluntarjoajalle {provider}, joka käsittelee tietoja ETA:n ulkopuolella. Organisaatiosi hyväksyi kyseisen siirron (GDPR 49 art. 1 k. a alak.) avainta tallentaessaan.',
  'ai.error.quota': 'Tekoälyn kuukausikiintiö on käytetty loppuun. Se uusiutuu {date}.',
  'ai.error.quota_nodate': 'Tekoälyn kuukausikiintiö on käytetty loppuun.',
  'ai.error.busy': 'Tekoälytöitä on jo käynnissä. Odota niiden valmistumista ja yritä uudelleen.',
  'ai.error.admin': 'Vain organisaation ylläpito voi tehdä tämän.',
  'ai.error.prompt': 'Kuvaile sivustoasi ennen ehdotuksen luomista.',
  'ai.error.languages': 'Valitse vähintään yksi kohdekieli.',
  'ai.error.session': 'Istuntosi on vanhentunut. Lataa sivu uudelleen ja kirjaudu sisään.',
  'ai.error.generic': 'Työtä ei voitu käynnistää. Yritä muutaman minuutin kuluttua uudelleen.',
  'ai.error.draft_not_saved':
    'Luonnosta ei voitu tallentaa. Ratkaise tallennusongelma ja käynnistä tekoäly uudelleen.',
  'ai.job.state_queued': 'Jonossa',
  'ai.job.state_running': 'Käynnissä',
  'ai.job.state_done': 'Valmis',
  'ai.job.state_failed': 'Epäonnistui',
  'ai.job.queued_text': 'Odotetaan, että palvelin ottaa työn vastaan…',
  'ai.job.running_text': 'Tekoäly työskentelee. Tämä voi kestää muutaman minuutin.',
  'ai.job.done_text': 'Työ on valmis.',
  'ai.job.failed_text': 'Työtä ei voitu suorittaa loppuun.',
  'ai.job.lost_text': 'Yhteys palvelimeen katkesi.',
  'ai.job.elapsed': 'Kulunut aika: {time}',
  'ai.job.detail': 'Tekninen tieto: {detail}',
  'ai.job.model': 'Malli: {model}',
  'ai.job.badge_eu': 'EU:n sisällä',
  'ai.job.badge_non_eu': 'ETA:n ulkopuolella',
  'ai.job.badge_byok': 'Oma avain',
  'ai.job.back': 'Takaisin',
  'ai.job.retry': 'Yritä uudelleen',
  'ai.job.stop': 'Lopeta odottaminen',
  'ai.job.stop_help': 'Työ jatkuu palvelimella ja lasketaan mukaan kiintiöösi.',

  // Suunnittelu
  'ai.design.title': 'Suunnittele tekoälyllä',
  'ai.design.desc':
    'Kuvaile yritystäsi, niin tekoäly ehdottaa sivun osioita ja muokkausvalmiita tekstejä.',
  'ai.design.prompt_label': 'Kuvaile sivustoasi tai yritystäsi',
  'ai.design.prompt_placeholder':
    'Esim. Vastuullinen naisten muotiliike, jolla on kuvasto ja ajanvaraus…',
  'ai.design.prompt_help':
    'Mitä tarkemmin kerrot (mitä tarjoat, kenelle ja mitä kävijöiden tulisi tehdä), sitä parempi ehdotus on.',
  'ai.design.ideas_label': 'Ideoita alkuun',
  'ai.design.idea_1': 'Artesaanijuustojen ja -viinien herkkukauppa verkkotilauksilla',
  'ai.design.idea_2': 'Arkkitehti- ja remonttitoimisto referenssigallerialla',
  'ai.design.idea_3': 'Hammaslääkäriasema ajanvarauksella, tiimiesittelyllä ja UKK-osiolla',
  'ai.design.idea_4': 'Italialainen ravintola maistelumenulla ja pöytävarauksella',
  'ai.design.style_label': 'Visuaalinen tyyli',
  'ai.design.style_modern': 'Moderni ja selkeä',
  'ai.design.style_elegant': 'Tyylikäs ja hienostunut',
  'ai.design.style_bold': 'Rohkea ja eloisa',
  'ai.design.style_minimal': 'Minimalistinen',
  'ai.design.lang_label': 'Sisällön kieli',
  'ai.design.button_generate': 'Luo ehdotus',
  'ai.design.job_title': 'Suunnitellaan ehdotusta',
  'ai.design.result_title': 'Ehdotus valmis',
  'ai.design.result_sections': {
    one: '{count} osio valmiina lisättäväksi pohjaan.',
    other: '{count} osiota valmiina lisättäväksi pohjaan.',
  },
  'ai.design.result_help': 'Ne lisätään muokkaamallesi sivulle ja voit muokata niitä sen jälkeen.',
  'ai.design.result_empty': 'Tekoäly ei palauttanut yhtään käyttökelpoista osiota.',
  'ai.design.button_apply': 'Lisää pohjaan',
  'ai.design.button_again': 'Kokeile toista ideaa',

  // Kääntäminen
  'ai.translate.title': 'Käännä tekoälyllä',
  'ai.translate.desc': 'Käännä sivuston tekstit muille kielille koskematta ulkoasuun.',
  'ai.translate.from_label': 'Lähdekieli',
  'ai.translate.to_label': 'Kohdekielet',
  'ai.translate.to_help':
    'Valitse vähintään yksi. Lähdekieli ei voi olla kohdekieli. Tuloksen käyttöönottaminen lisää uudet kielet sivustolle.',
  'ai.translate.scope_label': 'Käännöksen laajuus',
  'ai.translate.scope_site': 'Koko sivusto',
  'ai.translate.scope_page': 'Nykyinen sivu',
  'ai.translate.scope_store': 'Vain kaupan tuoteluettelo',
  'ai.translate.store_unavailable': 'Kaupan tuoteluettelon kääntäminen ei ole vielä käytettävissä.',
  'ai.translate.scope_invalid': 'Valitse käännettäväksi olemassa oleva sivu tai koko sivusto.',
  'ai.translate.overwrite_label': 'Korvaa jo olemassa olevat käännökset',
  'ai.translate.button_translate': 'Käännä',
  'ai.translate.job_title': 'Käännetään tekstejä',
  'ai.translate.result_title': 'Käännökset valmiina',
  'ai.translate.result_texts': {
    one: '{count} teksti käännetty.',
    other: '{count} tekstiä käännetty.',
  },
  'ai.translate.result_help': 'Ne lisätään luonnokseen; ulkoasu ei muutu.',
  'ai.translate.result_empty': 'Tekoäly ei palauttanut yhtään käännöstä.',
  'ai.translate.button_apply': 'Ota käännökset käyttöön',
  'ai.translate.button_again': 'Käännä muille kielille',

  // Vienti
  'ai.export.title': 'Vie tekoälyllä',
  'ai.export.desc':
    'Luo paketti, jolla sivustosi on valmis siirrettäväksi toiselle alustalle ilman riippuvuutta Sytuesta.',
  'ai.export.destination_label': 'Kohdealusta',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Täydellinen teema JSON-sivupohjilla ja Liquid-osioilla sekä tuoteluettelo CSV-tiedostona Shopifyn hallintapaneelista tuotavaksi.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Yksisivuinen projekti, joka on yhdistetty Sytuen julkiseen API-rajapintaan ja sisältää kassan ja lomakkeet.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Palvelimella renderöitävä tai staattinen sovellus sisäänrakennetulla maksupalvelulla.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Natiivi WordPress-lohkoteema tuotteiden ja ominaisuuksien viennillä.',
  'ai.export.badge_available': 'Saatavilla',
  'ai.export.badge_soon': 'Tulossa pian',
  'ai.export.button_export': 'Luo paketti',
  'ai.export.job_title': 'Luodaan pakettia alustalle {destination}',
  'ai.export.result_title': 'Paketti valmis',
  'ai.export.result_help':
    'Teema ja tuoteluettelon CSV-tiedosto ovat yhdessä samassa ZIP-paketissa.',
  'ai.export.steps_title': 'Asentaminen Shopifyhin',
  'ai.export.step_1': 'Lataa ZIP-tiedosto.',
  'ai.export.step_2':
    'Siirry Shopifyn hallintapaneelissa kohtaan Online Store > Themes ja valitse ”Upload zip file”.',
  'ai.export.step_3':
    'Tuotteita varten siirry kohtaan Products > Import ja lähetä ZIP-paketin sisältämä products_export_shopify.csv-tiedosto.',
  'ai.export.button_download': 'Lataa ZIP',
  'ai.export.button_again': 'Luo toinen vienti',
};
