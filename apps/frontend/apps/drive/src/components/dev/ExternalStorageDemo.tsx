/* SPDX-License-Identifier: Apache-2.0
 * Browser fixture: real component, API responses supplied by Playwright.
 */
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { ToastProvider } from '@sytue/ui';
import { useCallback, useState } from 'preact/hooks';
import { CloudsSidebar, CloudView, type Cloud } from '../PersonalClouds';
import { StorageMeter } from '../DriveSidebar';
import { useT } from '../../i18n';

export function ExternalStorageDemo({ locale }: { locale: Locale }) {
  const t = useT(locale);
  const [revision, setRevision] = useState(0);
  const [cloud, setCloud] = useState<Cloud | null>(null);
  const refreshQuota = useCallback(() => setRevision((value) => value + 1), []);
  return (
    <I18nProvider locale={locale}>
      <ToastProvider>
        <main class="mx-auto max-w-xl p-6">
          <h1 class="sytue-h1 mb-6">Sytue Drive</h1>
          <div class="border-border bg-sidebar rounded-lg border p-4">
            <CloudsSidebar
              t={t}
              active={cloud?.instance_id ?? null}
              onSelect={setCloud}
              onStorageChange={refreshQuota}
            />
            <StorageMeter locale={locale} t={t} revision={revision} />
          </div>
          {cloud ? <CloudView cloud={cloud} onDisconnected={() => setCloud(null)} /> : null}
        </main>
      </ToastProvider>
    </I18nProvider>
  );
}
