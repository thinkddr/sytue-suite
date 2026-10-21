import { useEffect, useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { Button, ToastProvider } from '@sytue/ui';
import { ExternalConversations } from '../ExternalConversations.js';
import { IntegrationsModal } from '../IntegrationsModal.js';
import type { Channel } from '../../client/chatApi.js';
import { roomsT } from '../../i18n/index.js';

/** Browser fixture mounts the real modal; Playwright supplies API responses. */
export function AppConnectionsDemo({ locale }: { locale: Locale }) {
  const t = roomsT(locale);
  const [open, setOpen] = useState(false);
  const [external, setExternal] = useState(false);
  useEffect(() => {
    if (new URLSearchParams(window.location.search).get('connect-app') === '1') setOpen(true);
    if (new URLSearchParams(window.location.search).get('connect-external') === '1')
      setExternal(true);
  }, []);
  async function shareMeeting(channelId: string, url: string, clientId: string) {
    // Development-only replacement for the encrypted Matrix client boundary.
    const response = await fetch('/rooms/api/dev/meeting-share', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ channelId, url, clientId }),
    });
    if (!response.ok) throw new Error('encrypted_share');
  }
  const channels: Channel[] = [
    { id: 'local-general', name: 'general', kind: 'public' },
    { id: 'local-design', name: 'diseño y planificación del equipo', kind: 'private' },
  ];
  return (
    <I18nProvider locale={locale}>
      <ToastProvider>
        <main class="mx-auto max-w-xl p-6">
          <h1 class="sytue-h1 mb-6">Sytue Rooms</h1>
          <Button onClick={() => setOpen(true)}>{t('integrations.title')}</Button>
          <Button variant="secondary" onClick={() => setExternal(true)}>
            {t('external.title')}
          </Button>
          {external ? (
            <ExternalConversations
              onClose={() => setExternal(false)}
              t={t}
              meetingChannels={channels}
              activeChannelId="local-general"
              captureMeetingScope={() => ({ assertCurrent() {}, share: shareMeeting })}
            />
          ) : null}
          <IntegrationsModal
            isOrgAdmin
            open={open}
            onClose={() => setOpen(false)}
            channels={channels}
            activeChannelId="local-general"
            t={t}
          />
        </main>
      </ToastProvider>
    </I18nProvider>
  );
}
