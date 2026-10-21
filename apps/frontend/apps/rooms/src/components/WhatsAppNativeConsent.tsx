import { useEffect, useRef, useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Locale } from '@sytue/i18n';
import { Button, Icon, Spinner, withBase } from '@sytue/ui';
import type { WhatsAppSignup } from '@sytue/api-clients';
import { openMetaSignup } from '../lib/metaWhatsAppSignup.js';
import { roomsT } from '../i18n/index.js';

/** Meta requires its web SDK. Team, number, PIN and activation remain in Fyne. */
export function WhatsAppNativeConsent({
  signup,
  mismatch,
  locale,
}: {
  signup: WhatsAppSignup | null;
  mismatch: boolean;
  locale: Locale;
}) {
  const t = roomsT(locale);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<'failed' | 'account' | null>(null);
  const pendingRef = useRef<AbortController | null>(null);
  useEffect(() => {
    window.history.replaceState(window.history.state, '', window.location.pathname);
    return () => pendingRef.current?.abort();
  }, []);
  function authorize(): void {
    if (busy || !signup) return;
    const controller = new AbortController();
    pendingRef.current = controller;
    setBusy(true);
    setError(null);
    void openMetaSignup(
      signup,
      {
        continue: t('appConnection.continue', { provider: 'Meta' }),
        loading: t('state.loading'),
        failed: t('appConnection.failed'),
        done: t('action.done'),
      },
      controller.signal,
    )
      .then(async (consent) => {
        const response = await fetch(
          withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/whatsapp-connection'),
          {
            method: 'POST',
            credentials: 'include',
            signal: controller.signal,
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ action: 'finish', ...consent }),
          },
        );
        if (!response.ok)
          throw new Error(
            response.status === 401 || response.status === 403 ? 'account' : 'exchange',
          );
        const result = (await response.json()) as { instance_id: string };
        if (result.instance_id !== signup.instance_id) throw new Error('instance');
        if (!controller.signal.aborted) setDone(true);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          setError(
            failure instanceof Error && failure.message === 'account' ? 'account' : 'failed',
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
  }
  return (
    <I18nProvider locale={locale}>
      <main class="mx-auto flex w-full max-w-lg flex-col items-start gap-5 p-6">
        <h1 class="sytue-h2">WhatsApp Business</h1>
        <p class="text-text-muted text-base">
          {t(
            !signup
              ? mismatch
                ? 'whatsappConnection.sameAccount'
                : 'whatsappConnection.unavailable'
              : done
                ? 'whatsappConnection.numberBody'
                : 'whatsappConnection.authorizeBody',
          )}
        </p>
        {error ? (
          <p class="text-danger-text text-sm" role="alert">
            {t(error === 'account' ? 'whatsappConnection.sameAccount' : 'appConnection.failed')}
          </p>
        ) : null}
        {busy ? (
          <p class="text-text-muted flex items-center gap-2" role="status">
            <Spinner size={18} />
            {t('state.loading')}
          </p>
        ) : null}
        {(done || error) && signup ? (
          <>
            {done ? <Icon name="check" size={32} class="text-success-text" /> : null}
            <Button
              href={`sytue-rooms://connect/whatsapp?instance=${encodeURIComponent(signup.instance_id)}`}
            >
              {t('appConnection.continue', { provider: 'Sytue Rooms' })}
            </Button>
          </>
        ) : signup ? (
          <Button disabled={busy} onClick={authorize}>
            {t('whatsappConnection.authorize')}
          </Button>
        ) : null}
      </main>
    </I18nProvider>
  );
}
