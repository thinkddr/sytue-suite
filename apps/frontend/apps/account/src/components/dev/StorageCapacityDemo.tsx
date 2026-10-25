/* SPDX-License-Identifier: Apache-2.0 */
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { PlanCard } from '../PlanCard';
import { useT } from '../../i18n';

export function StorageCapacityDemo({ locale }: { locale: Locale }) {
  const t = useT(locale);
  return (
    <I18nProvider locale={locale}>
      <main class="mx-auto max-w-xl p-6">
        <h1 class="sytue-h1 mb-6">Sytue Account</h1>
        <PlanCard t={t} locale={locale} />
      </main>
    </I18nProvider>
  );
}
