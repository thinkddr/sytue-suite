/* SPDX-License-Identifier: Apache-2.0
 * Browser fixture: real component, API responses supplied by Playwright.
 */
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { ToastProvider } from '@sytue/ui';
import { CloudsSidebar } from '../PersonalClouds';
import { useT } from '../../i18n';

const noop = () => {};

export function ExternalStorageDemo({ locale }: { locale: Locale }) {
  const t = useT(locale);
  return (
    <I18nProvider locale={locale}>
      <ToastProvider>
        <main class="mx-auto max-w-xl p-6">
          <h1 class="sytue-h1 mb-6">Sytue Drive</h1>
          <div class="border-border bg-surface rounded-lg border p-4">
            <CloudsSidebar t={t} active={null} onSelect={noop} />
          </div>
        </main>
      </ToastProvider>
    </I18nProvider>
  );
}
