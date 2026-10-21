import { useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import { Button, SettingsShell, ToastProvider } from '@sytue/ui';
import ConnectAppWizard from '../ConnectAppWizard.js';
import { useT } from '../../i18n/index.js';
import type { CatalogApp } from '../../lib/gateway.js';

/** Uses the same shell and wizard as AppsPanel; Playwright supplies the Gateway. */
export function ConnectAppDemo({ configured }: { configured: boolean }) {
  const t = useT('en');
  const [open, setOpen] = useState(true);
  const app: CatalogApp = {
    key: 'slack',
    name: 'Slack',
    description: 'Connect Slack',
    color: '#611f69',
    iconUrl: '',
    identity: 'none',
    provisioning: 'none',
    hasData: false,
    fields: configured ? [{ key: 'workspace', label: 'Workspace', required: true }] : [],
    // OAuth must suppress legacy static credentials in every step.
    secret: { label: 'Legacy static token', help: 'Legacy credential instructions' },
    oauth: { scopes: ['chat:write'] },
    tier: 'tier2',
  };
  return (
    <I18nProvider locale="en">
      <ToastProvider>
        <main>
          <SettingsShell title={t('apps.connect')} description={t('apps.wizard.description')}>
            {open ? (
              <ConnectAppWizard
                t={t}
                locale="en"
                catalog={[app]}
                connected={{}}
                initialApp={app}
                onCancel={() => setOpen(false)}
                onConnected={() => setOpen(false)}
                onDone={() => setOpen(false)}
              />
            ) : (
              <Button onClick={() => setOpen(true)}>{t('apps.connect')}</Button>
            )}
          </SettingsShell>
        </main>
      </ToastProvider>
    </I18nProvider>
  );
}
