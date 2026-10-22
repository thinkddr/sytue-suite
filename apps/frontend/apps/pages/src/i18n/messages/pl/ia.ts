/**
 * Komunikaty w języku polskim dla obszaru sztucznej inteligencji (ia).
 * Prefiks: ai.*
 */
export const ia = {
  // Ustawienia
  'ai.settings.provider': 'Dostawca AI',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Własny klucz API (BYOK)',
  'ai.settings.key_placeholder': 'Wprowadź swój tajny klucz API…',
  'ai.settings.key_help':
    'Twój klucz jest przechowywany w postaci zaszyfrowanej algorytmem AES-256-GCM. Nigdy nie jest zwracany do przeglądarki.',
  'ai.settings.check_key': 'Przetestuj klucz',
  'ai.settings.delete_key': 'Usuń klucz',
  'ai.settings.status_configured': 'Klucz skonfigurowany',
  'ai.settings.tested_at': 'Zweryfikowano dnia',
  'ai.settings.quota_title': 'Miesięczny limit AI',
  'ai.settings.quota_reset': 'Odnowienie: {date}',
  'ai.settings.notice_eu_title': 'Informacja o przekazywaniu danych poza EOG',
  'ai.settings.notice_eu_desc':
    'Wybrany dostawca ({provider}) przetwarza dane poza Europejskim Obszarem Gospodarczym. Zgodnie z art. 49 ust. 1 lit. a RODO musisz wyrazić wyraźną zgodę na ten transfer, aby kontynuować.',
  'ai.settings.notice_eu_accept':
    'Wyrażam zgodę na międzynarodowe przekazywanie danych poza EOG na potrzeby zapytań AI u tego dostawcy.',
  'ai.settings.success_save': 'Klucz został pomyślnie zapisany i zweryfikowany.',
  'ai.settings.error_check': 'Błąd podczas weryfikacji klucza u dostawcy.',
  'ai.settings.form_title': 'Skonfiguruj własny klucz (BYOK)',
  'ai.settings.form_help':
    'Użyj własnych danych uwierzytelniających Mistral AI, OpenAI, Anthropic lub Zhipu AI, aby uniezależnić się od miesięcznego limitu.',
  'ai.settings.provider_mistral': 'Mistral AI (UE, RODO)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'UE, Francja',
  'ai.region.us': 'USA',
  'ai.region.intl': 'Międzynarodowy',
  'ai.settings.show_key': 'Pokaż klucz',
  'ai.settings.hide_key': 'Ukryj klucz',
  'ai.settings.key_required': 'Wprowadź klucz.',
  'ai.settings.notice_required': 'Musisz zaakceptować informację o przekazywaniu danych poza EOG.',
  'ai.settings.check_ok': 'Klucz został pomyślnie zweryfikowany u dostawcy.',
  'ai.settings.error_save': 'Nie udało się zapisać klucza.',
  'ai.settings.error_delete': 'Nie udało się usunąć klucza.',
  'ai.settings.checking': 'Sprawdzanie…',
  'ai.settings.save_activate': 'Zapisz i aktywuj',
  'ai.settings.key_deleted': 'Klucz usunięty. Będzie obowiązywać domyślny limit miesięczny.',
  'ai.settings.active_provider': 'Aktywny dostawca: {provider} ({model})',
  'ai.settings.no_key_default':
    'Brak skonfigurowanego własnego klucza. Obowiązuje miesięczny limit planu.',
  'ai.settings.status_plan': 'Limit planu',
  'ai.settings.quota_unlimited': 'Bez limitu',
  'ai.settings.quota_unlimited_help':
    'Zapytania obsługiwane za pomocą Twojego własnego klucza API.',
  'ai.settings.quota_jobs': 'Zadania projektowania, tłumaczenia i eksportu.',
  'ai.settings.loading': 'Wczytywanie ustawień AI',
  'ai.settings.load_error': 'Nie udało się wczytać ustawień AI.',
  'ai.settings.load_error_help':
    'Nie udało się połączyć z usługą. Twoja konfiguracja nie uległa zmianie.',
  'ai.settings.key_invalid': 'Dostawca odrzucił klucz. Sprawdź, czy jest poprawny i aktywny.',
  'ai.settings.encryption_unavailable':
    'Usługa nie jest jeszcze gotowa do bezpiecznego przechowywania kluczy. Skontaktuj się z pomocą techniczną.',
  'ai.settings.invalid_model': 'Wybierz prawidłowy model dla tego dostawcy.',
  'ai.settings.key_state': 'Stan klucza',

  'ai.common.cancel': 'Anuluj',

  // Wspólny kontekst: limit, informacja RODO, błędy i stan zadania
  'ai.context.quota_label': 'Miesięczny limit AI',
  'ai.context.quota_used': '{used} z {limit} zadań w tym miesiącu',
  'ai.context.quota_reset': 'Odnawia się {date}.',
  'ai.context.quota_exhausted_title': 'Wykorzystano miesięczny limit AI',
  'ai.context.quota_exhausted_desc':
    'Kolejne zadania będzie można uruchomić {date}. Administracja organizacji może zdjąć ten limit, dodając własny klucz API w ustawieniach AI.',
  'ai.context.byok':
    'Twoja organizacja korzysta z własnego klucza API: to zadanie nie zużywa limitu.',
  'ai.context.loading': 'Sprawdzanie limitu AI',
  'ai.context.load_error': 'Nie udało się sprawdzić limitu AI. Mimo to możesz spróbować.',
  'ai.context.retry': 'Spróbuj ponownie',
  'ai.notice.eu': 'Teksty są przetwarzane na terenie UE, zgodnie z RODO.',
  'ai.notice.non_eu_title': 'To zadanie wychodzi poza Europejski Obszar Gospodarczy',
  'ai.notice.non_eu_desc':
    'Własny klucz Twojej organizacji należy do dostawcy {provider}, który przetwarza dane poza EOG. Twoja organizacja zaakceptowała to przekazywanie (art. 49 ust. 1 lit. a RODO) podczas zapisywania klucza.',
  'ai.error.quota': 'Miesięczny limit AI został wyczerpany. Odnawia się {date}.',
  'ai.error.quota_nodate': 'Miesięczny limit AI został wyczerpany.',
  'ai.error.busy': 'Zadania AI są już w toku. Poczekaj na ich zakończenie i spróbuj ponownie.',
  'ai.error.admin': 'Tylko administracja organizacji może to zrobić.',
  'ai.error.prompt': 'Opisz swoją witrynę przed wygenerowaniem propozycji.',
  'ai.error.languages': 'Wybierz co najmniej jeden język docelowy.',
  'ai.error.session': 'Twoja sesja wygasła. Odśwież stronę i zaloguj się ponownie.',
  'ai.error.generic': 'Nie udało się uruchomić zadania. Spróbuj ponownie za kilka minut.',
  'ai.error.draft_not_saved':
    'Nie udało się zapisać wersji roboczej. Rozwiąż problem z zapisem i uruchom AI ponownie.',
  'ai.job.state_queued': 'W kolejce',
  'ai.job.state_running': 'W toku',
  'ai.job.state_done': 'Gotowe',
  'ai.job.state_failed': 'Błąd',
  'ai.job.queued_text': 'Oczekiwanie na przyjęcie zadania przez serwer…',
  'ai.job.running_text': 'AI pracuje. Może to potrwać kilka minut.',
  'ai.job.done_text': 'Zadanie zostało zakończone.',
  'ai.job.failed_text': 'Nie udało się ukończyć zadania.',
  'ai.job.lost_text': 'Utracono połączenie z serwerem.',
  'ai.job.elapsed': 'Upływający czas: {time}',
  'ai.job.detail': 'Szczegóły techniczne: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'Wewnątrz UE',
  'ai.job.badge_non_eu': 'Poza EOG',
  'ai.job.badge_byok': 'Własny klucz',
  'ai.job.back': 'Wstecz',
  'ai.job.retry': 'Spróbuj ponownie',
  'ai.job.stop': 'Przestań czekać',
  'ai.job.stop_help': 'Zadanie będzie dalej wykonywane na serwerze i wliczy się do Twojego limitu.',

  // Projektowanie
  'ai.design.title': 'Zaprojektuj z AI',
  'ai.design.desc':
    'Opisz swoją firmę, a AI zaproponuje sekcje strony wraz z tekstami gotowymi do edycji.',
  'ai.design.prompt_label': 'Opisz swoją witrynę lub firmę',
  'ai.design.prompt_placeholder':
    'Np. Sklep z naturalnymi kosmetykami wegańskimi, z katalogiem i rezerwacją wizyt…',
  'ai.design.prompt_help':
    'Im dokładniejszy opis (co oferujesz, dla kogo i co mają zrobić osoby odwiedzające stronę), tym lepsza będzie propozycja.',
  'ai.design.ideas_label': 'Pomysły na początek',
  'ai.design.idea_1':
    'Sklep delikatesowy z rzemieślniczymi serami i winami oraz zamówieniami online',
  'ai.design.idea_2': 'Pracownia architektoniczna i remontowa z galerią realizacji',
  'ai.design.idea_3': 'Klinika stomatologiczna z rezerwacją wizyt, zespołem lekarskim i sekcją FAQ',
  'ai.design.idea_4': 'Włoska restauracja z menu degustacyjnym i rezerwacją stolików',
  'ai.design.style_label': 'Styl wizualny',
  'ai.design.style_modern': 'Nowoczesny i przejrzysty',
  'ai.design.style_elegant': 'Elegancki i wyrafinowany',
  'ai.design.style_bold': 'Odważny i wyrazisty',
  'ai.design.style_minimal': 'Minimalistyczny',
  'ai.design.lang_label': 'Język treści',
  'ai.design.button_generate': 'Wygeneruj propozycję',
  'ai.design.job_title': 'Projektowanie propozycji',
  'ai.design.result_title': 'Propozycja gotowa',
  'ai.design.result_sections': {
    one: '{count} sekcja gotowa do dodania na płótno.',
    few: '{count} sekcje gotowe do dodania na płótno.',
    many: '{count} sekcji gotowych do dodania na płótno.',
    other: '{count} sekcji gotowej do dodania na płótno.',
  },
  'ai.design.result_help':
    'Zostaną dodane do edytowanej strony i będzie można je później dostosować.',
  'ai.design.result_empty': 'AI nie zwróciła żadnej sekcji nadającej się do użycia.',
  'ai.design.button_apply': 'Dodaj na płótno',
  'ai.design.button_again': 'Wypróbuj inny pomysł',

  // Tłumaczenie
  'ai.translate.title': 'Przetłumacz z AI',
  'ai.translate.desc': 'Przetłumacz teksty witryny na inne języki bez naruszania układu strony.',
  'ai.translate.from_label': 'Język źródłowy',
  'ai.translate.to_label': 'Języki docelowe',
  'ai.translate.to_help':
    'Wybierz co najmniej jeden. Język źródłowy nie może być wybrany jako docelowy. Po zastosowaniu nowe języki zostaną dodane do witryny.',
  'ai.translate.scope_label': 'Zakres tłumaczenia',
  'ai.translate.scope_site': 'Cała witryna',
  'ai.translate.scope_page': 'Bieżąca strona',
  'ai.translate.scope_store': 'Tylko katalog sklepu',
  'ai.translate.store_unavailable': 'Tłumaczenie katalogu sklepu nie jest jeszcze dostępne.',
  'ai.translate.scope_invalid': 'Wybierz istniejącą stronę lub całą witrynę do przetłumaczenia.',
  'ai.translate.overwrite_label': 'Nadpisz istniejące już tłumaczenia',
  'ai.translate.button_translate': 'Przetłumacz',
  'ai.translate.job_title': 'Tłumaczenie tekstów',
  'ai.translate.result_title': 'Tłumaczenia gotowe',
  'ai.translate.result_texts': {
    one: 'Przetłumaczono {count} tekst.',
    few: 'Przetłumaczono {count} teksty.',
    many: 'Przetłumaczono {count} tekstów.',
    other: 'Przetłumaczono {count} tekstu.',
  },
  'ai.translate.result_help': 'Zostaną dodane do wersji roboczej; układ strony się nie zmienia.',
  'ai.translate.result_empty': 'AI nie zwróciła żadnego tłumaczenia.',
  'ai.translate.button_apply': 'Zastosuj tłumaczenia',
  'ai.translate.button_again': 'Przetłumacz na inne języki',

  // Eksport
  'ai.export.title': 'Eksportuj z AI',
  'ai.export.desc':
    'Wygeneruj pakiet ze swoją witryną gotowy do przeniesienia na inną platformę, bez zależności od Sytue.',
  'ai.export.destination_label': 'Platforma docelowa',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Kompletny motyw z szablonami JSON i sekcjami Liquid oraz katalog w pliku CSV do zaimportowania w panelu Shopify.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Projekt jednostronicowy połączony z publicznym API Sytue, z obsługą płatności i formularzy.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Aplikacja renderowana po stronie serwera lub statyczna, ze zintegrowaną bramką płatności.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Natywny motyw blokowy WordPressa z eksportem produktów i atrybutów.',
  'ai.export.badge_available': 'Dostępne',
  'ai.export.badge_soon': 'Wkrótce',
  'ai.export.button_export': 'Wygeneruj pakiet',
  'ai.export.job_title': 'Generowanie pakietu dla {destination}',
  'ai.export.result_title': 'Pakiet gotowy',
  'ai.export.result_help': 'Motyw oraz plik CSV z katalogiem znajdują się w jednym archiwum ZIP.',
  'ai.export.steps_title': 'Jak zainstalować w Shopify',
  'ai.export.step_1': 'Pobierz plik ZIP.',
  'ai.export.step_2':
    'W panelu administracyjnym Shopify przejdź do Sklep online > Motywy i wybierz „Prześlij plik zip”.',
  'ai.export.step_3':
    'Aby dodać produkty, przejdź do Produkty > Importuj i prześlij plik products_export_shopify.csv znajdujący się wewnątrz archiwum ZIP.',
  'ai.export.button_download': 'Pobierz ZIP',
  'ai.export.button_again': 'Wygeneruj kolejny eksport',
};
