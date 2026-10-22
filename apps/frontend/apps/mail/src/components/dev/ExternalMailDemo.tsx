/* SPDX-License-Identifier: Apache-2.0 */
import { useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { Button } from '@sytue/ui';
import { ExternalMail } from '../ExternalMail.js';
import { useT } from '../../i18n/index.js';
export function ExternalMailDemo({ locale }: { locale: Locale }) {
  const t = useT(locale);
  const [open, setOpen] = useState(false);
  return (
    <I18nProvider locale={locale}>
      <main class="mx-auto max-w-xl p-6">
        <h1 class="sytue-h1 mb-6">Sytue Mail</h1>
        <Button onClick={() => setOpen(true)}>{t('externalMail.add')}</Button>
        {open ? <ExternalMail t={t} onClose={() => setOpen(false)} /> : null}
      </main>
    </I18nProvider>
  );
}
