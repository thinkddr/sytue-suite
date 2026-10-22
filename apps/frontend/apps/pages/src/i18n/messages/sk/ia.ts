/**
 * Správy v slovenčine pre oblasť umelej inteligencie (ia).
 * Prefix: ai.*
 */
export const ia = {
  // Nastavenia
  'ai.settings.provider': 'Poskytovateľ AI',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Vlastný kľúč API (BYOK)',
  'ai.settings.key_placeholder': 'Zadajte svoj tajný kľúč API…',
  'ai.settings.key_help':
    'Váš kľúč je uložený zašifrovaný pomocou AES-256-GCM. Nikdy sa nevracia späť do prehliadača.',
  'ai.settings.check_key': 'Otestovať kľúč',
  'ai.settings.delete_key': 'Odstrániť kľúč',
  'ai.settings.status_configured': 'Kľúč je nastavený',
  'ai.settings.tested_at': 'Overené dňa',
  'ai.settings.quota_title': 'Mesačný limit AI',
  'ai.settings.quota_reset': 'Obnoví sa {date}',
  'ai.settings.notice_eu_title': 'Upozornenie na prenos údajov mimo EHP',
  'ai.settings.notice_eu_desc':
    'Vybraný poskytovateľ ({provider}) spracúva údaje mimo Európskeho hospodárskeho priestoru. Podľa čl. 49 ods. 1 písm. a) GDPR musíte s týmto prenosom výslovne súhlasiť, aby ste mohli pokračovať.',
  'ai.settings.notice_eu_accept':
    'Súhlasím s medzinárodným prenosom údajov mimo EHP pre požiadavky AI u tohto poskytovateľa.',
  'ai.settings.success_save': 'Kľúč bol úspešne uložený a overený.',
  'ai.settings.error_check': 'Chyba pri overovaní kľúča u poskytovateľa.',
  'ai.settings.form_title': 'Nastaviť vlastný kľúč (BYOK)',
  'ai.settings.form_help':
    'Použite vlastné prihlasovacie údaje pre Mistral AI, OpenAI, Anthropic alebo Zhipu AI, aby ste neboli závislí od mesačného limitu.',
  'ai.settings.provider_mistral': 'Mistral AI (EÚ, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EÚ, Francúzsko',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Medzinárodné',
  'ai.settings.show_key': 'Zobraziť kľúč',
  'ai.settings.hide_key': 'Skryť kľúč',
  'ai.settings.key_required': 'Zadajte kľúč.',
  'ai.settings.notice_required': 'Musíte potvrdiť upozornenie o prenose údajov mimo EHP.',
  'ai.settings.check_ok': 'Kľúč bol u poskytovateľa úspešne overený.',
  'ai.settings.error_save': 'Kľúč sa nepodarilo uložiť.',
  'ai.settings.error_delete': 'Kľúč sa nepodarilo odstrániť.',
  'ai.settings.checking': 'Overuje sa…',
  'ai.settings.save_activate': 'Uložiť a aktivovať',
  'ai.settings.key_deleted': 'Kľúč bol odstránený. Použije sa predvolený mesačný limit.',
  'ai.settings.active_provider': 'Aktívny poskytovateľ: {provider} ({model})',
  'ai.settings.no_key_default': 'Nie je nastavený vlastný kľúč. Platí mesačný limit programu.',
  'ai.settings.status_plan': 'Limit programu',
  'ai.settings.quota_unlimited': 'Neobmedzene',
  'ai.settings.quota_unlimited_help': 'Požiadavky sú vybavované pomocou vášho vlastného kľúča API.',
  'ai.settings.quota_jobs': 'Úlohy návrhu, prekladu a exportu.',
  'ai.settings.loading': 'Načítavajú sa nastavenia AI',
  'ai.settings.load_error': 'Nastavenia AI sa nepodarilo načítať.',
  'ai.settings.load_error_help':
    'So službou sa nepodarilo spojiť. Vaše nastavenie zostalo bez zmeny.',
  'ai.settings.key_invalid': 'Poskytovateľ kľúč odmietol. Skontrolujte, či je správny a aktívny.',
  'ai.settings.encryption_unavailable':
    'Služba zatiaľ nie je pripravená bezpečne ukladať kľúče. Kontaktujte podporu.',
  'ai.settings.invalid_model': 'Vyberte platný model pre tohto poskytovateľa.',
  'ai.settings.key_state': 'Stav kľúča',

  'ai.common.cancel': 'Zrušiť',

  // Spoločný kontext: limit, upozornenie GDPR, chyby a stav úlohy
  'ai.context.quota_label': 'Mesačný limit AI',
  'ai.context.quota_used': '{used} z {limit} úloh tento mesiac',
  'ai.context.quota_reset': 'Obnovuje sa {date}.',
  'ai.context.quota_exhausted_title': 'Mesačný limit AI bol vyčerpaný',
  'ai.context.quota_exhausted_desc':
    'Ďalšie úlohy budete môcť spustiť {date}. Správa organizácie môže limit zrušiť pridaním vlastného kľúča API v nastaveniach AI.',
  'ai.context.byok': 'Vaša organizácia používa vlastný kľúč API: táto úloha nečerpá limit.',
  'ai.context.loading': 'Kontroluje sa limit AI',
  'ai.context.load_error': 'Limit AI sa nepodarilo zistiť. Napriek tomu to môžete skúsiť.',
  'ai.context.retry': 'Skúsiť znova',
  'ai.notice.eu': 'Texty sa spracúvajú v rámci EÚ v súlade s GDPR.',
  'ai.notice.non_eu_title': 'Táto úloha opúšťa Európsky hospodársky priestor',
  'ai.notice.non_eu_desc':
    'Vlastný kľúč vašej organizácie patrí poskytovateľovi {provider}, ktorý spracúva údaje mimo EHP. Vaša organizácia tento prenos (čl. 49 ods. 1 písm. a) GDPR) odsúhlasila pri uložení kľúča.',
  'ai.error.quota': 'Mesačný limit AI je vyčerpaný. Obnovuje sa {date}.',
  'ai.error.quota_nodate': 'Mesačný limit AI je vyčerpaný.',
  'ai.error.busy': 'Úlohy AI už prebiehajú. Počkajte na ich dokončenie a skúste to znova.',
  'ai.error.admin': 'Toto môže vykonať iba správa organizácie.',
  'ai.error.prompt': 'Pred vygenerovaním návrhu opíšte svoj web.',
  'ai.error.languages': 'Vyberte aspoň jeden cieľový jazyk.',
  'ai.error.session': 'Vaša relácia vypršala. Obnovte stránku a znova sa prihláste.',
  'ai.error.generic': 'Úlohu sa nepodarilo spustiť. Skúste to znova o niekoľko minút.',
  'ai.error.draft_not_saved':
    'Koncept sa nepodarilo uložiť. Vyriešte problém s ukladaním a spustite AI znova.',
  'ai.job.state_queued': 'V rade',
  'ai.job.state_running': 'Prebieha',
  'ai.job.state_done': 'Hotovo',
  'ai.job.state_failed': 'Chyba',
  'ai.job.queued_text': 'Čaká sa na prijatie úlohy serverom…',
  'ai.job.running_text': 'AI pracuje. Môže to trvať niekoľko minút.',
  'ai.job.done_text': 'Úloha bola dokončená.',
  'ai.job.failed_text': 'Úlohu sa nepodarilo dokončiť.',
  'ai.job.lost_text': 'Spojenie so serverom bolo prerušené.',
  'ai.job.elapsed': 'Uplynutý čas: {time}',
  'ai.job.detail': 'Technický detail: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'V rámci EÚ',
  'ai.job.badge_non_eu': 'Mimo EHP',
  'ai.job.badge_byok': 'Vlastný kľúč',
  'ai.job.back': 'Späť',
  'ai.job.retry': 'Skúsiť znova',
  'ai.job.stop': 'Prestať čakať',
  'ai.job.stop_help': 'Úloha pobeží na serveri ďalej a započíta sa do vášho limitu.',

  // Navrhovanie
  'ai.design.title': 'Navrhnúť pomocou AI',
  'ai.design.desc':
    'Opíšte svoje podnikanie a AI navrhne sekcie stránky s textami pripravenými na úpravy.',
  'ai.design.prompt_label': 'Opíšte svoj web alebo podnikanie',
  'ai.design.prompt_placeholder':
    'Napr. Obchod s prírodnou vegánskou kozmetikou, katalógom a rezerváciou termínov…',
  'ai.design.prompt_help':
    'Čím konkrétnejší opis uvediete (čo ponúkate, komu a čo majú prichádzajúce osoby na webe urobiť), tým lepší bude návrh.',
  'ai.design.ideas_label': 'Nápady na začiatok',
  'ai.design.idea_1': 'Lahôdky s remeselnými syrmi a vínami s online objednávkami',
  'ai.design.idea_2': 'Architektonické a rekonštrukčné štúdio s galériou realizácií',
  'ai.design.idea_3':
    'Zubná ambulancia s objednávaním termínov, lekárskym tímom a častými otázkami',
  'ai.design.idea_4': 'Talianska reštaurácia s degustačným menu a rezerváciou stolov',
  'ai.design.style_label': 'Vizuálny štýl',
  'ai.design.style_modern': 'Moderný a čistý',
  'ai.design.style_elegant': 'Elegantný a vkusný',
  'ai.design.style_bold': 'Výrazný a odvážny',
  'ai.design.style_minimal': 'Minimalistický',
  'ai.design.lang_label': 'Jazyk obsahu',
  'ai.design.button_generate': 'Vygenerovať návrh',
  'ai.design.job_title': 'Navrhuje sa stránka',
  'ai.design.result_title': 'Návrh je pripravený',
  'ai.design.result_sections': {
    one: '{count} sekcia pripravená na pridanie na plátno.',
    few: '{count} sekcie pripravené na pridanie na plátno.',
    many: '{count} sekcie pripravenej na pridanie na plátno.',
    other: '{count} sekcií pripravených na pridanie na plátno.',
  },
  'ai.design.result_help':
    'Pridajú sa na práve upravovanú stránku a potom si ich môžete prispôsobiť.',
  'ai.design.result_empty': 'AI nevrátila žiadnu použiteľnú sekciu.',
  'ai.design.button_apply': 'Pridať na plátno',
  'ai.design.button_again': 'Vyskúšať iný nápad',

  // Preklad
  'ai.translate.title': 'Preložiť pomocou AI',
  'ai.translate.desc': 'Preložte texty webu do ďalších jazykov bez zmeny vzhľadu.',
  'ai.translate.from_label': 'Zdrojový jazyk',
  'ai.translate.to_label': 'Cieľové jazyky',
  'ai.translate.to_help':
    'Vyberte aspoň jeden. Zdrojový jazyk nemožno vybrať ako cieľový. Po použití sa nové jazyky pridajú na web.',
  'ai.translate.scope_label': 'Rozsah prekladu',
  'ai.translate.scope_site': 'Celý web',
  'ai.translate.scope_page': 'Aktuálna stránka',
  'ai.translate.scope_store': 'Iba katalóg obchodu',
  'ai.translate.store_unavailable': 'Preklad katalógu obchodu zatiaľ nie je k dispozícii.',
  'ai.translate.scope_invalid': 'Na preklad vyberte existujúcu stránku alebo celý web.',
  'ai.translate.overwrite_label': 'Prepísať už existujúce preklady',
  'ai.translate.button_translate': 'Preložiť',
  'ai.translate.job_title': 'Prekladajú sa texty',
  'ai.translate.result_title': 'Preklady sú pripravené',
  'ai.translate.result_texts': {
    one: 'Preložený {count} text.',
    few: 'Preložené {count} texty.',
    many: 'Preloženého {count} textu.',
    other: 'Preložených {count} textov.',
  },
  'ai.translate.result_help': 'Pridajú sa do konceptu; vzhľad zostane bez zmeny.',
  'ai.translate.result_empty': 'AI nevrátila žiadny preklad.',
  'ai.translate.button_apply': 'Použiť preklady',
  'ai.translate.button_again': 'Preložiť do ďalších jazykov',

  // Export
  'ai.export.title': 'Exportovať pomocou AI',
  'ai.export.desc':
    'Vygenerujte balíček so svojím webom pripravený na prenos na inú platformu, bez závislosti od Sytue.',
  'ai.export.destination_label': 'Cieľová platforma',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Kompletná téma s JSON šablónami a sekciami Liquid, vrátane katalógu v CSV na import v administrácii Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Jednostránkový projekt napojený na verejné API Sytue, s pokladňou a formulármi.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Serverovo vykresľovaná alebo statická aplikácia so zabudovanou platobnou bránou.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Natívna bloková téma WordPressu s exportom produktov a atribútov.',
  'ai.export.badge_available': 'Dostupné',
  'ai.export.badge_soon': 'Už čoskoro',
  'ai.export.button_export': 'Vygenerovať balíček',
  'ai.export.job_title': 'Generuje sa balíček pre {destination}',
  'ai.export.result_title': 'Balíček je pripravený',
  'ai.export.result_help': 'Téma aj CSV súbor katalógu sú spoločne v jednom súbore ZIP.',
  'ai.export.steps_title': 'Ako nainštalovať do Shopify',
  'ai.export.step_1': 'Stiahnite súbor ZIP.',
  'ai.export.step_2':
    'V administrácii Shopify prejdite do Online Store > Themes a zvoľte „Upload zip file“.',
  'ai.export.step_3':
    'Pre produkty prejdite do Products > Import a nahrajte súbor products_export_shopify.csv, ktorý je vo vnútri ZIPu.',
  'ai.export.button_download': 'Stiahnuť ZIP',
  'ai.export.button_again': 'Vygenerovať ďalší export',
};
