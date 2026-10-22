/**
 * Съобщения на български за изкуствения интелект (ia).
 * Префикс: ai.*
 */
export const ia = {
  // Настройки
  'ai.settings.provider': 'Доставчик на ИИ',
  'ai.settings.model': 'Модел',
  'ai.settings.key': 'Собствен API ключ (BYOK)',
  'ai.settings.key_placeholder': 'Въведете тайния си API ключ…',
  'ai.settings.key_help':
    'Ключът ви се съхранява шифрован с AES-256-GCM. Никога не се връща към браузъра.',
  'ai.settings.check_key': 'Тестване на ключа',
  'ai.settings.delete_key': 'Изтриване на ключа',
  'ai.settings.status_configured': 'Ключът е настроен',
  'ai.settings.tested_at': 'Проверен на',
  'ai.settings.quota_title': 'Месечен лимит за ИИ',
  'ai.settings.quota_reset': 'Нулира се на {date}',
  'ai.settings.notice_eu_title': 'Известие за предаване на данни извън ЕИП',
  'ai.settings.notice_eu_desc':
    'Избраният доставчик ({provider}) обработва данни извън Европейското икономическо пространство. Съгласно чл. 49, пар. 1, б. „а“ от ОРЗД (GDPR) трябва изрично да се съгласите с това предаване, за да продължите.',
  'ai.settings.notice_eu_accept':
    'Съгласявам се с международното предаване на данни извън ЕИП за заявки към ИИ при този доставчик.',
  'ai.settings.success_save': 'Ключът е запазен и проверен успешно.',
  'ai.settings.error_check': 'Грешка при проверката на ключа при доставчика.',
  'ai.settings.form_title': 'Настройване на собствен ключ (BYOK)',
  'ai.settings.form_help':
    'Използвайте собствени данни за достъп до Mistral AI, OpenAI, Anthropic или Zhipu AI, за да не зависите от месечния лимит.',
  'ai.settings.provider_mistral': 'Mistral AI (ЕС, ОРЗД)',
  'ai.settings.provider_openai': 'OpenAI (САЩ)',
  'ai.settings.provider_anthropic': 'Anthropic (САЩ)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'ЕС, Франция',
  'ai.region.us': 'САЩ',
  'ai.region.intl': 'Международен',
  'ai.settings.show_key': 'Показване на ключа',
  'ai.settings.hide_key': 'Скриване на ключа',
  'ai.settings.key_required': 'Въведете ключ.',
  'ai.settings.notice_required': 'Трябва да приемете известието за предаване извън ЕИП.',
  'ai.settings.check_ok': 'Ключът е проверен успешно при доставчика.',
  'ai.settings.error_save': 'Ключът не можа да бъде запазен.',
  'ai.settings.error_delete': 'Ключът не можа да бъде изтрит.',
  'ai.settings.checking': 'Проверка…',
  'ai.settings.save_activate': 'Запазване и активиране',
  'ai.settings.key_deleted': 'Ключът е изтрит. Ще се прилага стандартният месечен лимит.',
  'ai.settings.active_provider': 'Активен доставчик: {provider} ({model})',
  'ai.settings.no_key_default': 'Няма настроен собствен ключ. Прилага се месечният лимит на плана.',
  'ai.settings.status_plan': 'Лимит по плана',
  'ai.settings.quota_unlimited': 'Неограничен',
  'ai.settings.quota_unlimited_help': 'Заявките се изпълняват със собствения ви API ключ.',
  'ai.settings.quota_jobs': 'Задачи за дизайн, превод и експортиране.',
  'ai.settings.loading': 'Зареждане на настройките за ИИ',
  'ai.settings.load_error': 'Настройките за ИИ не можаха да се заредят.',
  'ai.settings.load_error_help': 'Няма връзка с услугата. Настройките ви не са променени.',
  'ai.settings.key_invalid': 'Доставчикът отхвърли ключа. Проверете дали е правилен и активен.',
  'ai.settings.encryption_unavailable':
    'Услугата все още не е готова да съхранява ключове по защитен начин. Свържете се с поддръжката.',
  'ai.settings.invalid_model': 'Изберете валиден модел за този доставчик.',
  'ai.settings.key_state': 'Състояние на ключа',

  'ai.common.cancel': 'Отказ',

  // Общ контекст: лимит, известие по ОРЗД, грешки и състояние на задачата
  'ai.context.quota_label': 'Месечен лимит за ИИ',
  'ai.context.quota_used': '{used} от {limit} задачи този месец',
  'ai.context.quota_reset': 'Подновява се на {date}.',
  'ai.context.quota_exhausted_title': 'Месечният лимит за ИИ е изчерпан',
  'ai.context.quota_exhausted_desc':
    'Ще можете да стартирате нови задачи на {date}. Администрацията на организацията може да премахне ограничението, като добави собствен API ключ в настройките за ИИ.',
  'ai.context.byok': 'Организацията ви използва собствен API ключ: тази задача не черпи от лимита.',
  'ai.context.loading': 'Проверка на лимита за ИИ',
  'ai.context.load_error':
    'Лимитът за ИИ не можа да бъде проверен. Въпреки това можете да опитате.',
  'ai.context.retry': 'Опитайте отново',
  'ai.notice.eu': 'Текстовете се обработват в рамките на ЕС съгласно ОРЗД.',
  'ai.notice.non_eu_title': 'Тази задача излиза извън Европейското икономическо пространство',
  'ai.notice.non_eu_desc':
    'Собственият ключ на организацията ви принадлежи на {provider}, който обработва данните извън ЕИП. Организацията ви прие това предаване (чл. 49, пар. 1, б. „а“ от ОРЗД) при запазването на ключа.',
  'ai.error.quota': 'Месечният лимит за ИИ е изчерпан. Подновява се на {date}.',
  'ai.error.quota_nodate': 'Месечният лимит за ИИ е изчерпан.',
  'ai.error.busy': 'Вече се изпълняват задачи с ИИ. Изчакайте да завършат и опитайте отново.',
  'ai.error.admin': 'Само администрацията на организацията може да направи това.',
  'ai.error.prompt': 'Опишете сайта си, преди да генерирате предложението.',
  'ai.error.languages': 'Изберете поне един целеви език.',
  'ai.error.session': 'Сесията ви е изтекла. Презаредете страницата и влезте отново.',
  'ai.error.generic': 'Задачата не можа да стартира. Опитайте отново след няколко минути.',
  'ai.error.draft_not_saved':
    'Черновата не можа да бъде запазена. Отстранете проблема със запазването и стартирайте ИИ отново.',
  'ai.job.state_queued': 'На опашка',
  'ai.job.state_running': 'В ход',
  'ai.job.state_done': 'Готово',
  'ai.job.state_failed': 'С грешка',
  'ai.job.queued_text': 'Изчакване сървърът да приеме задачата…',
  'ai.job.running_text': 'ИИ работи. Това може да отнеме няколко минути.',
  'ai.job.done_text': 'Задачата завърши.',
  'ai.job.failed_text': 'Задачата не можа да бъде завършена.',
  'ai.job.lost_text': 'Връзката със сървъра беше прекъсната.',
  'ai.job.elapsed': 'Изминало време: {time}',
  'ai.job.detail': 'Технически детайл: {detail}',
  'ai.job.model': 'Модел: {model}',
  'ai.job.badge_eu': 'В рамките на ЕС',
  'ai.job.badge_non_eu': 'Извън ЕИП',
  'ai.job.badge_byok': 'Собствен ключ',
  'ai.job.back': 'Назад',
  'ai.job.retry': 'Опитайте отново',
  'ai.job.stop': 'Спри чакането',
  'ai.job.stop_help': 'Задачата ще продължи на сървъра и ще се отчете към вашия лимит.',

  // Дизайн
  'ai.design.title': 'Дизайн с ИИ',
  'ai.design.desc':
    'Опишете бизнеса си и ИИ ще предложи секциите на страницата с текстове, готови за редактиране.',
  'ai.design.prompt_label': 'Опишете вашия сайт или бизнес',
  'ai.design.prompt_placeholder':
    'Напр. Магазин за натурална веган козметика с каталог и записване на часове…',
  'ai.design.prompt_help':
    'Колкото по-конкретно опишете (какво предлагате, за кого и какво искате да направят посетителите), толкова по-добро ще бъде предложението.',
  'ai.design.ideas_label': 'Идеи за начало',
  'ai.design.idea_1': 'Гурме магазин за занаятчийски сирена и вина с онлайн поръчки',
  'ai.design.idea_2': 'Студио за архитектура и цялостни ремонти с галерия от проекти',
  'ai.design.idea_3': 'Дентална клиника със записване на час, медицински екип и чести въпроси',
  'ai.design.idea_4': 'Италиански ресторант с дегустационно меню и резервация на маса',
  'ai.design.style_label': 'Визуален стил',
  'ai.design.style_modern': 'Модерен и изчистен',
  'ai.design.style_elegant': 'Елегантен и изискан',
  'ai.design.style_bold': 'Смел и впечатляващ',
  'ai.design.style_minimal': 'Минималистичен',
  'ai.design.lang_label': 'Език на съдържанието',
  'ai.design.button_generate': 'Генериране на предложение',
  'ai.design.job_title': 'Проектиране на предложението',
  'ai.design.result_title': 'Предложението е готово',
  'ai.design.result_sections': {
    one: '{count} секция е готова за добавяне към платното.',
    other: '{count} секции са готови за добавяне към платното.',
  },
  'ai.design.result_help':
    'Ще бъдат добавени към страницата, която редактирате, и ще можете да ги промените след това.',
  'ai.design.result_empty': 'ИИ не върна използваема секция.',
  'ai.design.button_apply': 'Добавяне към платното',
  'ai.design.button_again': 'Опитайте друга идея',

  // Превод
  'ai.translate.title': 'Превод с ИИ',
  'ai.translate.desc': 'Преведете текстовете на сайта на други езици, без да променяте дизайна.',
  'ai.translate.from_label': 'Изходен език',
  'ai.translate.to_label': 'Целеви езици',
  'ai.translate.to_help':
    'Изберете поне един. Изходният език не може да бъде избран като целеви. При прилагане новите езици ще се добавят към сайта.',
  'ai.translate.scope_label': 'Обхват на превода',
  'ai.translate.scope_site': 'Целия сайт',
  'ai.translate.scope_page': 'Текущата страница',
  'ai.translate.scope_store': 'Само каталога на магазина',
  'ai.translate.store_unavailable': 'Преводът на каталога на магазина все още не е наличен.',
  'ai.translate.scope_invalid': 'Изберете съществуваща страница или целия сайт за превод.',
  'ai.translate.overwrite_label': 'Презаписване на вече съществуващите преводи',
  'ai.translate.button_translate': 'Превеждане',
  'ai.translate.job_title': 'Превеждане на текстовете',
  'ai.translate.result_title': 'Преводите са готови',
  'ai.translate.result_texts': {
    one: '{count} преведен текст.',
    other: '{count} преведени текста.',
  },
  'ai.translate.result_help': 'Ще бъдат добавени в черновата; дизайнът не се променя.',
  'ai.translate.result_empty': 'ИИ не върна никакъв превод.',
  'ai.translate.button_apply': 'Прилагане на преводите',
  'ai.translate.button_again': 'Превод на други езици',

  // Експортиране
  'ai.export.title': 'Експортиране с ИИ',
  'ai.export.desc':
    'Генерирайте пакет с вашия сайт, готов за пренасяне в друга платформа, без зависимост от Sytue.',
  'ai.export.destination_label': 'Целева платформа',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Пълна тема с JSON шаблони и Liquid секции, плюс каталога в CSV за импортиране от панела на Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Едностраничен проект, свързан с публичния API на Sytue, с плащане и формуляри.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Приложение със сървърно рендиране или статично, с вграден платежен портал.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Нативна блокова тема за WordPress с експортиране на продукти и атрибути.',
  'ai.export.badge_available': 'Налично',
  'ai.export.badge_soon': 'Очаквайте скоро',
  'ai.export.button_export': 'Генериране на пакет',
  'ai.export.job_title': 'Генериране на пакета за {destination}',
  'ai.export.result_title': 'Пакетът е готов',
  'ai.export.result_help': 'Темата и CSV файлът с каталога са събрани в един общ ZIP архив.',
  'ai.export.steps_title': 'Как да го инсталирате в Shopify',
  'ai.export.step_1': 'Изтеглете ZIP файла.',
  'ai.export.step_2':
    'В административния панел на Shopify отидете в Online Store > Themes и изберете „Upload zip file“.',
  'ai.export.step_3':
    'За продуктите отидете в Products > Import и качете файла products_export_shopify.csv от ZIP архива.',
  'ai.export.button_download': 'Изтегляне на ZIP',
  'ai.export.button_again': 'Генериране на друг експорт',
};
