/* SPDX-License-Identifier: Apache-2.0 */
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { LanguageSelect } from '@sytue/ui';

export function LanguagePreferenceDemo({ locale }: { locale: Locale }) {
  return (
    <I18nProvider locale={locale}>
      <main class="bg-surface text-text mx-auto max-w-md p-6">
        <LanguageSelect />
      </main>
    </I18nProvider>
  );
}
