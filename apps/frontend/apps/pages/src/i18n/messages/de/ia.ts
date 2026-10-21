/**
 * Deutsche Texte für die künstliche Intelligenz (ia).
 * Präfix: ai.*
 */
export const ia = {
  // Einstellungen
  'ai.settings.provider': 'KI-Anbieter',
  'ai.settings.model': 'Modell',
  'ai.settings.key': 'Eigener API-Schlüssel (BYOK)',
  'ai.settings.key_placeholder': 'Geheimen API-Schlüssel eingeben …',
  'ai.settings.key_help':
    'Ihr Schlüssel wird mit AES-256-GCM verschlüsselt gespeichert und nie an den Browser zurückgegeben.',
  'ai.settings.check_key': 'Schlüssel testen',
  'ai.settings.delete_key': 'Schlüssel löschen',
  'ai.settings.status_configured': 'Schlüssel eingerichtet',
  'ai.settings.tested_at': 'Geprüft am',
  'ai.settings.quota_title': 'Monatliches KI-Kontingent',
  'ai.settings.quota_reset': 'Wird am {date} zurückgesetzt',
  'ai.settings.notice_eu_title': 'Hinweis zur Datenübermittlung außerhalb des EWR',
  'ai.settings.notice_eu_desc':
    'Der gewählte Anbieter ({provider}) verarbeitet Daten außerhalb des Europäischen Wirtschaftsraums. Nach Art. 49 Abs. 1 lit. a DSGVO müssen Sie dieser Übermittlung ausdrücklich zustimmen, um fortzufahren.',
  'ai.settings.notice_eu_accept':
    'Ich stimme der internationalen Übermittlung von Daten außerhalb des EWR für KI-Anfragen an diesen Anbieter zu.',
  'ai.settings.success_save': 'Schlüssel gespeichert und geprüft.',
  'ai.settings.error_check': 'Fehler beim Prüfen des Schlüssels beim Anbieter.',
  'ai.settings.form_title': 'Eigenen Schlüssel einrichten (BYOK)',
  'ai.settings.form_help':
    'Verwenden Sie Ihre eigenen Zugangsdaten für Mistral AI, OpenAI, Anthropic oder Zhipu AI, um nicht vom monatlichen Kontingent abzuhängen.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, DSGVO)',
  'ai.settings.provider_openai': 'OpenAI (USA)',
  'ai.settings.provider_anthropic': 'Anthropic (USA)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, Frankreich',
  'ai.region.us': 'USA',
  'ai.region.intl': 'International',
  'ai.settings.show_key': 'Schlüssel anzeigen',
  'ai.settings.hide_key': 'Schlüssel ausblenden',
  'ai.settings.key_required': 'Geben Sie einen Schlüssel ein.',
  'ai.settings.notice_required':
    'Sie müssen den Hinweis zur Übermittlung außerhalb des EWR akzeptieren.',
  'ai.settings.check_ok': 'Schlüssel beim Anbieter erfolgreich geprüft.',
  'ai.settings.error_save': 'Der Schlüssel konnte nicht gespeichert werden.',
  'ai.settings.error_delete': 'Der Schlüssel konnte nicht gelöscht werden.',
  'ai.settings.checking': 'Wird geprüft …',
  'ai.settings.save_activate': 'Speichern und aktivieren',
  'ai.settings.key_deleted':
    'Schlüssel gelöscht. Es gilt wieder das monatliche Standardkontingent.',
  'ai.settings.active_provider': 'Aktiver Anbieter: {provider} ({model})',
  'ai.settings.no_key_default':
    'Kein eigener Schlüssel eingerichtet. Es gilt das monatliche Limit des Tarifs.',
  'ai.settings.status_plan': 'Kontingent des Tarifs',
  'ai.settings.quota_unlimited': 'Unbegrenzt',
  'ai.settings.quota_unlimited_help': 'Anfragen werden mit Ihrem eigenen API-Schlüssel bearbeitet.',
  'ai.settings.quota_jobs': 'Aufträge für Design, Übersetzung und Export.',
  'ai.settings.loading': 'KI-Einstellungen werden geladen',
  'ai.settings.load_error': 'Die KI-Einstellungen konnten nicht geladen werden.',
  'ai.settings.load_error_help':
    'Der Dienst war nicht erreichbar. Ihre Einstellungen wurden nicht geändert.',
  'ai.settings.key_invalid':
    'Der Anbieter hat den Schlüssel abgelehnt. Prüfen Sie, ob er korrekt und aktiv ist.',
  'ai.settings.encryption_unavailable':
    'Der Dienst kann Schlüssel noch nicht sicher speichern. Wenden Sie sich an den Support.',
  'ai.settings.invalid_model': 'Wählen Sie ein gültiges Modell für diesen Anbieter.',
  'ai.settings.key_state': 'Status des Schlüssels',

  'ai.common.cancel': 'Abbrechen',

  // Gemeinsamer Kontext: Kontingent, DSGVO-Hinweis, Fehler und Auftragsstatus
  'ai.context.quota_label': 'Monatliches KI-Kontingent',
  'ai.context.quota_used': '{used} von {limit} Aufträgen in diesem Monat',
  'ai.context.quota_reset': 'Wird am {date} erneuert.',
  'ai.context.quota_exhausted_title': 'Das monatliche KI-Kontingent ist aufgebraucht',
  'ai.context.quota_exhausted_desc':
    'Ab dem {date} können Sie wieder Aufträge starten. Mit Administrationsrechten lässt sich das Limit aufheben, indem in den KI-Einstellungen ein eigener API-Schlüssel hinterlegt wird.',
  'ai.context.byok':
    'Ihre Organisation verwendet einen eigenen API-Schlüssel: Dieser Auftrag verbraucht kein Kontingent.',
  'ai.context.loading': 'KI-Kontingent wird geprüft',
  'ai.context.load_error':
    'Das KI-Kontingent konnte nicht geprüft werden. Sie können es trotzdem versuchen.',
  'ai.context.retry': 'Erneut versuchen',
  'ai.notice.eu': 'Die Texte werden DSGVO-konform innerhalb der EU verarbeitet.',
  'ai.notice.non_eu_title': 'Dieser Auftrag verlässt den Europäischen Wirtschaftsraum',
  'ai.notice.non_eu_desc':
    'Der eigene Schlüssel Ihrer Organisation gehört zu {provider}, das Daten außerhalb des EWR verarbeitet. Ihre Organisation hat dieser Übermittlung (Art. 49 Abs. 1 lit. a DSGVO) beim Speichern des Schlüssels zugestimmt.',
  'ai.error.quota': 'Das monatliche KI-Kontingent ist aufgebraucht. Es wird am {date} erneuert.',
  'ai.error.quota_nodate': 'Das monatliche KI-Kontingent ist aufgebraucht.',
  'ai.error.busy':
    'Es laufen bereits KI-Aufträge. Warten Sie, bis sie fertig sind, und versuchen Sie es erneut.',
  'ai.error.admin': 'Das ist nur mit Administrationsrechten der Organisation möglich.',
  'ai.error.prompt': 'Beschreiben Sie Ihre Website, bevor Sie den Vorschlag erzeugen.',
  'ai.error.languages': 'Wählen Sie mindestens eine Zielsprache.',
  'ai.error.session':
    'Ihre Sitzung ist abgelaufen. Laden Sie die Seite neu und melden Sie sich erneut an.',
  'ai.error.generic':
    'Der Auftrag konnte nicht gestartet werden. Versuchen Sie es in ein paar Minuten erneut.',
  'ai.error.draft_not_saved':
    'Der Entwurf konnte nicht gespeichert werden. Beheben Sie das Speicherproblem und starten Sie die KI erneut.',
  'ai.job.state_queued': 'In der Warteschlange',
  'ai.job.state_running': 'Läuft',
  'ai.job.state_done': 'Fertig',
  'ai.job.state_failed': 'Fehlgeschlagen',
  'ai.job.queued_text': 'Warten, bis der Server den Auftrag annimmt …',
  'ai.job.running_text': 'Die KI arbeitet. Das kann einige Minuten dauern.',
  'ai.job.done_text': 'Der Auftrag ist abgeschlossen.',
  'ai.job.failed_text': 'Der Auftrag konnte nicht abgeschlossen werden.',
  'ai.job.lost_text': 'Die Verbindung zum Server wurde unterbrochen.',
  'ai.job.elapsed': 'Vergangene Zeit: {time}',
  'ai.job.detail': 'Technische Details: {detail}',
  'ai.job.model': 'Modell: {model}',
  'ai.job.badge_eu': 'Innerhalb der EU',
  'ai.job.badge_non_eu': 'Außerhalb des EWR',
  'ai.job.badge_byok': 'Eigener Schlüssel',
  'ai.job.back': 'Zurück',
  'ai.job.retry': 'Erneut versuchen',
  'ai.job.stop': 'Nicht mehr warten',
  'ai.job.stop_help':
    'Der Auftrag läuft auf dem Server weiter und wird auf Ihr Kontingent angerechnet.',

  // Design
  'ai.design.title': 'Mit KI gestalten',
  'ai.design.desc':
    'Beschreiben Sie Ihr Unternehmen, und die KI schlägt die Abschnitte der Seite vor, mit Texten, die Sie bearbeiten können.',
  'ai.design.prompt_label': 'Beschreiben Sie Ihre Website oder Ihr Unternehmen',
  'ai.design.prompt_placeholder':
    'Z. B. Nachhaltiges Modegeschäft für Damen mit Katalog und Terminvereinbarung …',
  'ai.design.prompt_help':
    'Je genauer Sie sind (was Sie anbieten, für wen und was Besuchende tun sollen), desto besser wird der Vorschlag.',
  'ai.design.ideas_label': 'Ideen für den Anfang',
  'ai.design.idea_1': 'Feinkostladen für handgemachten Käse und Wein mit Online-Bestellung',
  'ai.design.idea_2': 'Architektur- und Renovierungsbüro mit Projektgalerie',
  'ai.design.idea_3': 'Zahnarztpraxis mit Terminbuchung, Praxisteam und häufigen Fragen',
  'ai.design.idea_4': 'Italienisches Restaurant mit Degustationsmenü und Tischreservierung',
  'ai.design.style_label': 'Visueller Stil',
  'ai.design.style_modern': 'Modern und klar',
  'ai.design.style_elegant': 'Elegant und edel',
  'ai.design.style_bold': 'Kräftig und lebendig',
  'ai.design.style_minimal': 'Minimalistisch',
  'ai.design.lang_label': 'Sprache der Inhalte',
  'ai.design.button_generate': 'Vorschlag erzeugen',
  'ai.design.job_title': 'Vorschlag wird gestaltet',
  'ai.design.result_title': 'Vorschlag bereit',
  'ai.design.result_sections': {
    one: '{count} Abschnitt bereit für die Arbeitsfläche.',
    other: '{count} Abschnitte bereit für die Arbeitsfläche.',
  },
  'ai.design.result_help':
    'Sie werden der Seite hinzugefügt, die Sie gerade bearbeiten, und lassen sich danach anpassen.',
  'ai.design.result_empty': 'Die KI hat keinen verwendbaren Abschnitt geliefert.',
  'ai.design.button_apply': 'Zur Arbeitsfläche hinzufügen',
  'ai.design.button_again': 'Andere Idee ausprobieren',

  // Übersetzen
  'ai.translate.title': 'Mit KI übersetzen',
  'ai.translate.desc':
    'Übersetzen Sie die Texte der Website in andere Sprachen, ohne das Design zu verändern.',
  'ai.translate.from_label': 'Ausgangssprache',
  'ai.translate.to_label': 'Zielsprachen',
  'ai.translate.to_help':
    'Wählen Sie mindestens eine. Die Ausgangssprache kann keine Zielsprache sein. Wenn Sie das Ergebnis übernehmen, kommen neue Sprachen zur Website hinzu.',
  'ai.translate.scope_label': 'Umfang der Übersetzung',
  'ai.translate.scope_site': 'Gesamte Website',
  'ai.translate.scope_page': 'Aktuelle Seite',
  'ai.translate.scope_store': 'Nur Shop-Katalog',
  'ai.translate.store_unavailable': 'Die Übersetzung des Shop-Katalogs ist noch nicht verfügbar.',
  'ai.translate.scope_invalid':
    'Wählen Sie eine vorhandene Seite oder die gesamte Website zum Übersetzen.',
  'ai.translate.overwrite_label': 'Vorhandene Übersetzungen überschreiben',
  'ai.translate.button_translate': 'Übersetzen',
  'ai.translate.job_title': 'Texte werden übersetzt',
  'ai.translate.result_title': 'Übersetzungen bereit',
  'ai.translate.result_texts': {
    one: '{count} Text übersetzt.',
    other: '{count} Texte übersetzt.',
  },
  'ai.translate.result_help': 'Sie werden dem Entwurf hinzugefügt; das Design bleibt unverändert.',
  'ai.translate.result_empty': 'Die KI hat keine Übersetzung geliefert.',
  'ai.translate.button_apply': 'Übersetzungen übernehmen',
  'ai.translate.button_again': 'In weitere Sprachen übersetzen',

  // Export
  'ai.export.title': 'Mit KI exportieren',
  'ai.export.desc':
    'Erzeugen Sie ein Paket mit Ihrer Website, bereit für eine andere Plattform, ohne von Sytue abzuhängen.',
  'ai.export.destination_label': 'Zielplattform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Vollständiges Theme mit JSON-Vorlagen und Liquid-Abschnitten sowie der Katalog als CSV zum Import im Shopify-Adminbereich.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Single-Page-Projekt, verbunden mit der öffentlichen Sytue-API, mit Checkout und Formularen.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Serverseitig gerenderte oder statische Anwendung mit integriertem Zahlungsanbieter.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Natives WordPress-Block-Theme mit Export von Produkten und Attributen.',
  'ai.export.badge_available': 'Verfügbar',
  'ai.export.badge_soon': 'Demnächst',
  'ai.export.button_export': 'Paket erzeugen',
  'ai.export.job_title': 'Paket für {destination} wird erzeugt',
  'ai.export.result_title': 'Paket bereit',
  'ai.export.result_help': 'Theme und Katalog-CSV stecken zusammen in einer ZIP-Datei.',
  'ai.export.steps_title': 'So installieren Sie es in Shopify',
  'ai.export.step_1': 'Laden Sie die ZIP-Datei herunter.',
  'ai.export.step_2':
    'Gehen Sie im Shopify-Adminbereich zu Onlineshop > Themes und wählen Sie „ZIP-Datei hochladen“.',
  'ai.export.step_3':
    'Für die Produkte gehen Sie zu Produkte > Importieren und laden die Datei products_export_shopify.csv aus der ZIP-Datei hoch.',
  'ai.export.button_download': 'ZIP herunterladen',
  'ai.export.button_again': 'Weiteren Export erzeugen',
};
