/**
 * Zprávy v češtině pro oblast umělé inteligence (ia).
 * Prefix: ai.*
 */
export const ia = {
  // Nastavení
  'ai.settings.provider': 'Poskytovatel AI',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Vlastní klíč API (BYOK)',
  'ai.settings.key_placeholder': 'Zadejte svůj tajný klíč API…',
  'ai.settings.key_help':
    'Váš klíč je uložen zašifrovaný pomocí AES-256-GCM. Nikdy se nevrací zpět do prohlížeče.',
  'ai.settings.check_key': 'Otestovat klíč',
  'ai.settings.delete_key': 'Smazat klíč',
  'ai.settings.status_configured': 'Klíč je nastaven',
  'ai.settings.tested_at': 'Ověřeno dne',
  'ai.settings.quota_title': 'Měsíční limit AI',
  'ai.settings.quota_reset': 'Obnoví se {date}',
  'ai.settings.notice_eu_title': 'Upozornění na předávání údajů mimo EHP',
  'ai.settings.notice_eu_desc':
    'Vybraný poskytovatel ({provider}) zpracovává údaje mimo Evropský hospodářský prostor. Podle čl. 49 odst. 1 písm. a) GDPR musíte s tímto předáváním výslovně souhlasit, abyste mohli pokračovat.',
  'ai.settings.notice_eu_accept':
    'Souhlasím s mezinárodním předáváním údajů mimo EHP pro požadavky AI u tohoto poskytovatele.',
  'ai.settings.success_save': 'Klíč byl úspěšně uložen a ověřen.',
  'ai.settings.error_check': 'Chyba při ověřování klíče u poskytovatele.',
  'ai.settings.form_title': 'Nastavit vlastní klíč (BYOK)',
  'ai.settings.form_help':
    'Použijte vlastní přihlašovací údaje pro Mistral AI, OpenAI, Anthropic nebo Zhipu AI, abyste nebyli závislí na měsíčním limitu.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Francie',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Mezinárodní',
  'ai.settings.show_key': 'Zobrazit klíč',
  'ai.settings.hide_key': 'Skrýt klíč',
  'ai.settings.key_required': 'Zadejte klíč.',
  'ai.settings.notice_required': 'Musíte potvrdit upozornění o předávání údajů mimo EHP.',
  'ai.settings.check_ok': 'Klíč byl u poskytovatele úspěšně ověřen.',
  'ai.settings.error_save': 'Klíč se nepodařilo uložit.',
  'ai.settings.error_delete': 'Klíč se nepodařilo smazat.',
  'ai.settings.checking': 'Ověřování…',
  'ai.settings.save_activate': 'Uložit a aktivovat',
  'ai.settings.key_deleted': 'Klíč byl smazán. Použije se výchozí měsíční limit.',
  'ai.settings.active_provider': 'Aktivní poskytovatel: {provider} ({model})',
  'ai.settings.no_key_default': 'Není nastaven vlastní klíč. Platí měsíční limit tarifu.',
  'ai.settings.status_plan': 'Limit tarifu',
  'ai.settings.quota_unlimited': 'Neomezeně',
  'ai.settings.quota_unlimited_help':
    'Požadavky jsou vyřizovány pomocí vašeho vlastního klíče API.',
  'ai.settings.quota_jobs': 'Úlohy návrhu, překladu a exportu.',
  'ai.settings.loading': 'Načítání nastavení AI',
  'ai.settings.load_error': 'Nastavení AI se nepodařilo načíst.',
  'ai.settings.load_error_help':
    'Se službou se nepodařilo spojit. Vaše nastavení zůstalo beze změny.',
  'ai.settings.key_invalid': 'Poskytovatel klíč odmítl. Zkontrolujte, zda je správný a aktivní.',
  'ai.settings.encryption_unavailable':
    'Služba zatím není připravena bezpečně ukládat klíče. Kontaktujte podporu.',
  'ai.settings.invalid_model': 'Vyberte platný model pro tohoto poskytovatele.',
  'ai.settings.key_state': 'Stav klíče',

  'ai.common.cancel': 'Zrušit',

  // Společný kontext: limit, upozornění GDPR, chyby a stav úlohy
  'ai.context.quota_label': 'Měsíční limit AI',
  'ai.context.quota_used': '{used} z {limit} úloh tento měsíc',
  'ai.context.quota_reset': 'Obnovuje se {date}.',
  'ai.context.quota_exhausted_title': 'Měsíční limit AI byl vyčerpán',
  'ai.context.quota_exhausted_desc':
    'Další úlohy budete moci spustit {date}. Správa organizace může limit zrušit přidáním vlastního klíče API v nastavení AI.',
  'ai.context.byok': 'Vaše organizace používá vlastní klíč API: tato úloha nečerpá limit.',
  'ai.context.loading': 'Kontrola limitu AI',
  'ai.context.load_error': 'Limit AI se nepodařilo zjistit. Přesto to můžete zkusit.',
  'ai.context.retry': 'Zkusit znovu',
  'ai.notice.eu': 'Texty se zpracovávají v rámci EU v souladu s GDPR.',
  'ai.notice.non_eu_title': 'Tato úloha opouští Evropský hospodářský prostor',
  'ai.notice.non_eu_desc':
    'Vlastní klíč vaší organizace patří poskytovateli {provider}, který zpracovává údaje mimo EHP. Vaše organizace toto předávání (čl. 49 odst. 1 písm. a) GDPR) odsouhlasila při uložení klíče.',
  'ai.error.quota': 'Měsíční limit AI je vyčerpán. Obnovuje se {date}.',
  'ai.error.quota_nodate': 'Měsíční limit AI je vyčerpán.',
  'ai.error.busy': 'Úlohy AI již probíhají. Počkejte na jejich dokončení a zkuste to znovu.',
  'ai.error.admin': 'Toto může provést pouze správa organizace.',
  'ai.error.prompt': 'Před vygenerováním návrhu popište svůj web.',
  'ai.error.languages': 'Vyberte alespoň jeden cílový jazyk.',
  'ai.error.session': 'Vaše relace vypršela. Obnovte stránku a znovu se přihlaste.',
  'ai.error.generic': 'Úlohu se nepodařilo spustit. Zkuste to znovu za několik minut.',
  'ai.error.draft_not_saved':
    'Koncept se nepodařilo uložit. Vyřešte potíže s ukládáním a spusťte AI znovu.',
  'ai.job.state_queued': 'Ve frontě',
  'ai.job.state_running': 'Probíhá',
  'ai.job.state_done': 'Hotovo',
  'ai.job.state_failed': 'Chyba',
  'ai.job.queued_text': 'Čeká se na přijetí úlohy serverem…',
  'ai.job.running_text': 'AI pracuje. Může to trvat několik minut.',
  'ai.job.done_text': 'Úloha byla dokončena.',
  'ai.job.failed_text': 'Úlohu se nepodařilo dokončit.',
  'ai.job.lost_text': 'Spojení se serverem bylo přerušeno.',
  'ai.job.elapsed': 'Uplynulý čas: {time}',
  'ai.job.detail': 'Technický detail: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'V rámci EU',
  'ai.job.badge_non_eu': 'Mimo EHP',
  'ai.job.badge_byok': 'Vlastní klíč',
  'ai.job.back': 'Zpět',
  'ai.job.retry': 'Zkusit znovu',
  'ai.job.stop': 'Přestat čekat',
  'ai.job.stop_help': 'Úloha poběží na serveru dál a započítá se do vašeho limitu.',

  // Navrhování
  'ai.design.title': 'Navrhnout pomocí AI',
  'ai.design.desc':
    'Popište své podnikání a AI navrhne sekce stránky s texty připravenými k úpravám.',
  'ai.design.prompt_label': 'Popište svůj web nebo podnikání',
  'ai.design.prompt_placeholder':
    'Např. Obchod s přírodní veganskou kosmetikou, katalogem a rezervací termínů…',
  'ai.design.prompt_help':
    'Čím konkrétnější popis uvedete (co nabízíte, komu a co mají příchozí na webu udělat), tím lepší bude návrh.',
  'ai.design.ideas_label': 'Nápady pro začátek',
  'ai.design.idea_1': 'Delikatesy s řemeslnými sýry a víny s online objednávkami',
  'ai.design.idea_2': 'Architektonické a rekonstrukční studio s galerií realizací',
  'ai.design.idea_3': 'Zubní ordinace s objednáváním termínů, lékařským týmem a častými dotazy',
  'ai.design.idea_4': 'Italská restaurace s degustačním menu a rezervací stolů',
  'ai.design.style_label': 'Vizuální styl',
  'ai.design.style_modern': 'Moderní a čistý',
  'ai.design.style_elegant': 'Elegantní a vkusný',
  'ai.design.style_bold': 'Výrazný a odvážný',
  'ai.design.style_minimal': 'Minimalistický',
  'ai.design.lang_label': 'Jazyk obsahu',
  'ai.design.button_generate': 'Vygenerovat návrh',
  'ai.design.job_title': 'Navrhování stránky',
  'ai.design.result_title': 'Návrh je připraven',
  'ai.design.result_sections': {
    one: '{count} sekce připravena k přidání na plátno.',
    few: '{count} sekce připraveny k přidání na plátno.',
    many: '{count} sekce připravené k přidání na plátno.',
    other: '{count} sekcí připraveno k přidání na plátno.',
  },
  'ai.design.result_help':
    'Přidají se na právě upravovanou stránku a poté si je můžete přizpůsobit.',
  'ai.design.result_empty': 'AI nevrátila žádnou použitelnou sekci.',
  'ai.design.button_apply': 'Přidat na plátno',
  'ai.design.button_again': 'Vyzkoušet jiný nápad',

  // Překlad
  'ai.translate.title': 'Přeložit pomocí AI',
  'ai.translate.desc': 'Přeložte texty webu do dalších jazyků beze změny vzhledu.',
  'ai.translate.from_label': 'Výchozí jazyk',
  'ai.translate.to_label': 'Cílové jazyky',
  'ai.translate.to_help':
    'Vyberte alespoň jeden. Výchozí jazyk nelze vybrat jako cílový. Po použití se nové jazyky přidají na web.',
  'ai.translate.scope_label': 'Rozsah překladu',
  'ai.translate.scope_site': 'Celý web',
  'ai.translate.scope_page': 'Aktuální stránka',
  'ai.translate.scope_store': 'Pouze katalog obchodu',
  'ai.translate.store_unavailable': 'Překlad katalogu obchodu zatím není k dispozici.',
  'ai.translate.scope_invalid': 'Pro překlad vyberte existující stránku nebo celý web.',
  'ai.translate.overwrite_label': 'Přepsat již existující překlady',
  'ai.translate.button_translate': 'Přeložit',
  'ai.translate.job_title': 'Překládání textů',
  'ai.translate.result_title': 'Překlady jsou připraveny',
  'ai.translate.result_texts': {
    one: 'Přeložen {count} text.',
    few: 'Přeloženy {count} texty.',
    many: 'Přeloženo {count} textu.',
    other: 'Přeloženo {count} textů.',
  },
  'ai.translate.result_help': 'Přidají se do konceptu; vzhled zůstane beze změny.',
  'ai.translate.result_empty': 'AI nevrátila žádný překlad.',
  'ai.translate.button_apply': 'Použít překlady',
  'ai.translate.button_again': 'Přeložit do dalších jazyků',

  // Export
  'ai.export.title': 'Exportovat pomocí AI',
  'ai.export.desc':
    'Vygenerujte balíček se svým webem připravený k přenosu na jinou platformu, bez závislosti na Sytue.',
  'ai.export.destination_label': 'Cílová platforma',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Kompletní šablona s JSON šablonami a sekcemi Liquid, včetně katalogu v CSV pro import v administraci Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Jednostránkový projekt napojený na veřejné API Sytue, s pokladnou a formuláři.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Serverově vykreslovaná nebo statická aplikace se zabudovanou platební bránou.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Nativní bloková šablona WordPressu s exportem produktů a atributů.',
  'ai.export.badge_available': 'Dostupné',
  'ai.export.badge_soon': 'Již brzy',
  'ai.export.button_export': 'Vygenerovat balíček',
  'ai.export.job_title': 'Generování balíčku pro {destination}',
  'ai.export.result_title': 'Balíček je připraven',
  'ai.export.result_help': 'Šablona i CSV soubor katalogu jsou společně v jednom souboru ZIP.',
  'ai.export.steps_title': 'Jak nainstalovat do Shopify',
  'ai.export.step_1': 'Stáhněte soubor ZIP.',
  'ai.export.step_2':
    'V administraci Shopify přejděte do Online Store > Themes a zvolte „Upload zip file“.',
  'ai.export.step_3':
    'Pro produkty přejděte do Products > Import a nahrajte soubor products_export_shopify.csv, který je uvnitř ZIPu.',
  'ai.export.button_download': 'Stáhnout ZIP',
  'ai.export.button_again': 'Vygenerovat další export',
};
