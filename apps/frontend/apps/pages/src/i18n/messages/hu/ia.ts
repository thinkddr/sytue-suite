/**
 * Mensajes en húngaro (hu) para el área de inteligencia artificial (ia).
 * Prefijo: ai.*
 */
export const ia = {
  // Beállítások
  'ai.settings.provider': 'MI-szolgáltató',
  'ai.settings.model': 'Modell',
  'ai.settings.key': 'Saját API-kulcs (BYOK)',
  'ai.settings.key_placeholder': 'Adja meg a titkos API-kulcsát…',
  'ai.settings.key_help':
    'A kulcsát AES-256-GCM titkosítással tároljuk. Soha nem kerül vissza a böngészőbe.',
  'ai.settings.check_key': 'Kulcs tesztelése',
  'ai.settings.delete_key': 'Kulcs törlése',
  'ai.settings.status_configured': 'A kulcs beállítva',
  'ai.settings.tested_at': 'Ellenőrizve:',
  'ai.settings.quota_title': 'Havi MI-kvóta',
  'ai.settings.quota_reset': 'Megújulás: {date}',
  'ai.settings.notice_eu_title': 'Tájékoztatás EGT-n kívüli adattovábbításról',
  'ai.settings.notice_eu_desc':
    'A kiválasztott szolgáltató ({provider}) az Európai Gazdasági Térségen kívül dolgozza fel az adatokat. A GDPR 49. cikk (1) bekezdés a) pontja alapján a folytatáshoz kifejezetten hozzá kell járulnia ehhez az adattovábbításhoz.',
  'ai.settings.notice_eu_accept':
    'Hozzájárulok az adatok EGT-n kívülre történő nemzetközi továbbításához az ennél a szolgáltatónál indított MI-kérésekhez.',
  'ai.settings.success_save': 'A kulcs sikeresen mentve és ellenőrizve.',
  'ai.settings.error_check': 'Hiba történt a kulcs szolgáltatónál végzett ellenőrzésekor.',
  'ai.settings.form_title': 'Saját kulcs beállítása (BYOK)',
  'ai.settings.form_help':
    'Használja saját Mistral AI-, OpenAI-, Anthropic- vagy Zhipu AI-hitelesítő adatait, hogy ne függjön a havi kvótától.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Franciaország',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Nemzetközi',
  'ai.settings.show_key': 'Kulcs megjelenítése',
  'ai.settings.hide_key': 'Kulcs elrejtése',
  'ai.settings.key_required': 'Adjon meg egy kulcsot.',
  'ai.settings.notice_required':
    'El kell fogadnia az EGT-n kívüli adattovábbításról szóló tájékoztatást.',
  'ai.settings.check_ok': 'A kulcs sikeresen ellenőrizve a szolgáltatónál.',
  'ai.settings.error_save': 'A kulcsot nem sikerült menteni.',
  'ai.settings.error_delete': 'A kulcsot nem sikerült törölni.',
  'ai.settings.checking': 'Ellenőrzés…',
  'ai.settings.save_activate': 'Mentés és aktiválás',
  'ai.settings.key_deleted': 'A kulcs törölve. Az alapértelmezett havi kvóta lép érvénybe.',
  'ai.settings.active_provider': 'Aktív szolgáltató: {provider} ({model})',
  'ai.settings.no_key_default': 'Nincs saját kulcs beállítva. A csomag havi kerete érvényes.',
  'ai.settings.status_plan': 'A csomag kvótája',
  'ai.settings.quota_unlimited': 'Korlátlan',
  'ai.settings.quota_unlimited_help': 'Saját API-kulccsal kezelt kérések.',
  'ai.settings.quota_jobs': 'Tervezési, fordítási és exportálási feladatok.',
  'ai.settings.loading': 'MI-beállítások betöltése',
  'ai.settings.load_error': 'Az MI-beállításokat nem sikerült betölteni.',
  'ai.settings.load_error_help':
    'A szolgáltatást nem sikerült elérni. A beállításai nem változtak.',
  'ai.settings.key_invalid':
    'A szolgáltató elutasította a kulcsot. Ellenőrizze, hogy helyes és aktív-e.',
  'ai.settings.encryption_unavailable':
    'A szolgáltatás még nem áll készen a kulcsok biztonságos tárolására. Forduljon az ügyfélszolgálathoz.',
  'ai.settings.invalid_model': 'Válasszon érvényes modellt ehhez a szolgáltatóhoz.',
  'ai.settings.key_state': 'A kulcs állapota',

  'ai.common.cancel': 'Mégse',

  // Közös kontextus: kvóta, GDPR-tájékoztató, hibák és a feladat állapota
  'ai.context.quota_label': 'Havi MI-kvóta',
  'ai.context.quota_used': 'Ebben a hónapban {used} / {limit} feladat használva',
  'ai.context.quota_reset': 'Megújulás: {date}.',
  'ai.context.quota_exhausted_title': 'Elhasználta a havi MI-kvótát',
  'ai.context.quota_exhausted_desc':
    'További feladatokat {date} napjától indíthat. A szervezet adminisztrációja saját API-kulcs hozzáadásával feloldhatja a korlátot az MI-beállításokban.',
  'ai.context.byok':
    'A szervezete saját API-kulcsot használ: ez a feladat nem fogyasztja a kvótát.',
  'ai.context.loading': 'Az MI-kvóta ellenőrzése',
  'ai.context.load_error':
    'Az MI-kvótát nem sikerült ellenőrizni. Ettől függetlenül megpróbálhatja.',
  'ai.context.retry': 'Újra',
  'ai.notice.eu': 'A szövegek feldolgozása az EU-n belül, a GDPR-nak megfelelően történik.',
  'ai.notice.non_eu_title': 'Ez a feladat elhagyja az Európai Gazdasági Térséget',
  'ai.notice.non_eu_desc':
    'A szervezet saját kulcsa a(z) {provider} szolgáltatóhoz tartozik, amely az EGT-n kívül dolgozza fel az adatokat. A szervezete a kulcs mentésekor elfogadta ezt az adattovábbítást (GDPR 49. cikk (1) bekezdés a) pont).',
  'ai.error.quota': 'A havi MI-kvóta elfogyott. Megújulás: {date}.',
  'ai.error.quota_nodate': 'A havi MI-kvóta elfogyott.',
  'ai.error.busy': 'Már futnak MI-feladatok. Várja meg, amíg befejeződnek, majd próbálja újra.',
  'ai.error.admin': 'Ezt csak a szervezet adminisztrációja végezheti el.',
  'ai.error.prompt': 'A javaslat elkészítése előtt írja le a webhelyét.',
  'ai.error.languages': 'Válasszon legalább egy célnyelvet.',
  'ai.error.session': 'A munkamenete lejárt. Töltse újra az oldalt, és jelentkezzen be ismét.',
  'ai.error.generic': 'A feladatot nem sikerült elindítani. Próbálja újra néhány perc múlva.',
  'ai.error.draft_not_saved':
    'A piszkozatot nem sikerült menteni. Hárítsa el a mentési hibát, majd indítsa újra az MI-t.',
  'ai.job.state_queued': 'Sorban áll',
  'ai.job.state_running': 'Folyamatban',
  'ai.job.state_done': 'Kész',
  'ai.job.state_failed': 'Sikertelen',
  'ai.job.queued_text': 'Várakozás arra, hogy a szerver fogadja a feladatot…',
  'ai.job.running_text': 'Az MI dolgozik. Ez eltarthat néhány percig.',
  'ai.job.done_text': 'A feladat befejeződött.',
  'ai.job.failed_text': 'A feladatot nem sikerült befejezni.',
  'ai.job.lost_text': 'Megszakadt a kapcsolat a szerverrel.',
  'ai.job.elapsed': 'Eltelt idő: {time}',
  'ai.job.detail': 'Műszaki részletek: {detail}',
  'ai.job.model': 'Modell: {model}',
  'ai.job.badge_eu': 'EU-n belül',
  'ai.job.badge_non_eu': 'EGT-n kívül',
  'ai.job.badge_byok': 'Saját kulcs',
  'ai.job.back': 'Vissza',
  'ai.job.retry': 'Újra',
  'ai.job.stop': 'Várakozás leállítása',
  'ai.job.stop_help': 'A feladat tovább fut a szerveren, és beleszámít a kvótájába.',

  // Tervezés
  'ai.design.title': 'Tervezés MI-vel',
  'ai.design.desc':
    'Írja le a vállalkozását, és az MI javaslatot tesz az oldal szakaszaira, szerkesztésre kész szövegekkel.',
  'ai.design.prompt_label': 'Írja le a webhelyét vagy vállalkozását',
  'ai.design.prompt_placeholder':
    'Pl. fenntartható női divatüzlet katalógussal és időpontfoglalással…',
  'ai.design.prompt_help':
    'Minél pontosabb leírást ad (mit kínál, kinek, és mit tegyenek a látogatók), annál jobb lesz a javaslat.',
  'ai.design.ideas_label': 'Ötletek a kezdéshez',
  'ai.design.idea_1': 'Kézműves sajtokat és borokat kínáló ínyencbolt online rendeléssel',
  'ai.design.idea_2': 'Építészeti és felújítási stúdió projektgalériával',
  'ai.design.idea_3': 'Fogászati rendelő időpontfoglalással, orvoscsapattal és GYIK-kel',
  'ai.design.idea_4': 'Olasz étterem degusztációs menüvel és asztalfoglalással',
  'ai.design.style_label': 'Vizuális stílus',
  'ai.design.style_modern': 'Modern és letisztult',
  'ai.design.style_elegant': 'Elegáns és kifinomult',
  'ai.design.style_bold': 'Merész és élénk',
  'ai.design.style_minimal': 'Minimalista',
  'ai.design.lang_label': 'A tartalom nyelve',
  'ai.design.button_generate': 'Javaslat generálása',
  'ai.design.job_title': 'A javaslat tervezése',
  'ai.design.result_title': 'A javaslat elkészült',
  'ai.design.result_sections': {
    one: '{count} szakasz készen áll a vászonhoz való hozzáadásra.',
    other: '{count} szakasz készen áll a vászonhoz való hozzáadásra.',
  },
  'ai.design.result_help':
    'Hozzáadódnak az éppen szerkesztett oldalhoz, és utána tetszés szerint módosíthatja őket.',
  'ai.design.result_empty': 'Az MI nem adott vissza használható szakaszt.',
  'ai.design.button_apply': 'Hozzáadás a vászonhoz',
  'ai.design.button_again': 'Másik ötlet kipróbálása',

  // Fordítás
  'ai.translate.title': 'Fordítás MI-vel',
  'ai.translate.desc': 'Fordítsa le a webhely szövegeit más nyelvekre a dizájn érintése nélkül.',
  'ai.translate.from_label': 'Forrásnyelv',
  'ai.translate.to_label': 'Célnyelvek',
  'ai.translate.to_help':
    'Válasszon legalább egyet. A forrásnyelv nem lehet célnyelv. Az eredmény alkalmazásával az új nyelvek hozzáadódnak a webhelyhez.',
  'ai.translate.scope_label': 'A fordítás hatóköre',
  'ai.translate.scope_site': 'A teljes webhely',
  'ai.translate.scope_page': 'Az aktuális oldal',
  'ai.translate.scope_store': 'Csak az áruház katalógusa',
  'ai.translate.store_unavailable': 'Az áruházkatalógus fordítása még nem érhető el.',
  'ai.translate.scope_invalid':
    'Válasszon egy meglévő oldalt vagy a teljes webhelyet a fordításhoz.',
  'ai.translate.overwrite_label': 'A már meglévő fordítások felülírása',
  'ai.translate.button_translate': 'Fordítás',
  'ai.translate.job_title': 'A szövegek fordítása',
  'ai.translate.result_title': 'A fordítások elkészültek',
  'ai.translate.result_texts': {
    one: '{count} szöveg lefordítva.',
    other: '{count} szöveg lefordítva.',
  },
  'ai.translate.result_help': 'Bekerülnek a piszkozatba; a dizájn nem változik.',
  'ai.translate.result_empty': 'Az MI nem adott vissza fordítást.',
  'ai.translate.button_apply': 'Fordítások alkalmazása',
  'ai.translate.button_again': 'Fordítás további nyelvekre',

  // Exportálás
  'ai.export.title': 'Exportálás MI-vel',
  'ai.export.desc':
    'Készítsen egy csomagot a webhelyével, amely készen áll arra, hogy a Sytue-tól függetlenül másik platformra vigye.',
  'ai.export.destination_label': 'Célplatform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Teljes sablon JSON-sablonokkal és Liquid-szakaszokkal, valamint a katalógus CSV-fájlként a Shopify adminisztrációs felületéről való importáláshoz.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'A nyilvános Sytue API-hoz kapcsolódó egyoldalas projekt pénztárral és űrlapokkal.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Szerveroldalon renderelt vagy statikus alkalmazás beépített fizetési kapuval.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Natív WordPress-blokksablon termékek és attribútumok exportálásával.',
  'ai.export.badge_available': 'Elérhető',
  'ai.export.badge_soon': 'Hamarosan',
  'ai.export.button_export': 'Csomag generálása',
  'ai.export.job_title': 'Csomag generálása ehhez: {destination}',
  'ai.export.result_title': 'A csomag elkészült',
  'ai.export.result_help': 'A sablon és a katalógus CSV-fájlja együtt, egyetlen ZIP-ben található.',
  'ai.export.steps_title': 'Telepítés a Shopifyban',
  'ai.export.step_1': 'Töltse le a ZIP-fájlt.',
  'ai.export.step_2':
    'A Shopify adminisztrációs felületén lépjen az Online Store > Themes menüpontra, és válassza az „Upload zip file” lehetőséget.',
  'ai.export.step_3':
    'A termékekhez lépjen a Products > Import menüpontra, és töltse fel a ZIP-ben található products_export_shopify.csv fájlt.',
  'ai.export.button_download': 'ZIP letöltése',
  'ai.export.button_again': 'Újabb exportálás generálása',
};
