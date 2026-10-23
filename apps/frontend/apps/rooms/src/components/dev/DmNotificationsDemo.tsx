/* SPDX-License-Identifier: Apache-2.0 */
import { useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { Button } from '@sytue/ui';
import { roomsT } from '../../i18n/index.js';
import { useDmNotifications } from '../../lib/useDmNotifications.js';
import { StatusMenu } from '../StatusMenu.js';

/** Real hook/menu; only the identity and Hub responses are fixtures. */
export function DmNotificationsDemo({ locale }: { locale: Locale }) {
  const [tenant, setTenant] = useState('org-a');
  const preference = useDmNotifications({ id: 'person', tenant });
  const t = roomsT(locale);
  return (
    <I18nProvider locale={locale}>
      <main class="bg-surface text-text mx-auto max-w-md p-6">
        <StatusMenu
          state="online"
          status=""
          onChange={() => {}}
          dmNotify={preference.enabled}
          dmLoading={preference.loading}
          dmSaving={preference.saving}
          dmError={preference.error}
          dmLegacy={preference.hasLegacy}
          onRefreshDmNotify={preference.refresh}
          onDiscardLegacyDmNotify={preference.discardLegacy}
          onToggleDmNotify={preference.toggle}
          onOpenNotifySettings={() => {}}
          onOpenIntegrations={() => {}}
          onMarkAllRead={() => {}}
          canManageIntegrations={false}
          t={t}
        />
        <output data-testid="browser-enabled">{String(preference.browserEnabled)}</output>
        <output data-testid="preference-state">
          {preference.loading
            ? 'loading'
            : preference.saving
              ? 'saving'
              : preference.error
                ? 'error'
                : 'ready'}
        </output>
        <Button onClick={() => setTenant((value) => (value === 'org-a' ? 'org-b' : 'org-a'))}>
          Organization
        </Button>
      </main>
    </I18nProvider>
  );
}
