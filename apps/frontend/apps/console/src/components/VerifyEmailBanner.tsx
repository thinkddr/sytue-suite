/**
 * VerifyEmailBanner — aviso «email sin verificar» con acción de reenvío. Se monta
 * en el layout cuando la sesión del usuario tiene email_verified=false. Reenvía el
 * enlace vía el BFF (POST /api/me/email/verify → Sytue ID); el usuario confirma
 * pulsando el enlace del correo (página server-rendered de Sytue ID). Sobrio,
 * tono de aviso, sin degradados; feedback accesible con aria-live.
 */
import { useState } from 'preact/hooks';
import { Button, Icon } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

type Status = 'idle' | 'sending' | 'sent' | 'error';

// Marca interna con la que se interpola el correo para poder partir la frase
// traducida en «antes / correo / después» sin concatenar frases.
const MARK = '\u0000';

function highlight(sentence: string, email: string) {
  const [before = sentence, after = ''] = sentence.split(MARK);
  return (
    <>
      {before}
      <b class="font-semibold">{email}</b>
      {after}
    </>
  );
}

export default function VerifyEmailBanner({ email, locale }: { email: string; locale: Locale }) {
  const t = useT(locale);
  const [status, setStatus] = useState<Status>('idle');

  async function resend() {
    if (status === 'sending') return;
    setStatus('sending');
    try {
      const res = await fetch('/api/me/email/verify', { method: 'POST' });
      if (!res.ok) throw new Error(String(res.status));
      setStatus('sent');
    } catch {
      setStatus('error');
    }
  }

  return (
    <div
      role="region"
      aria-label={t('verifyEmail.region')}
      class="bg-warning-bg text-warning-text border-border flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5"
    >
      <Icon name="warning" size={16} class="shrink-0" />
      <p class="min-w-0 flex-1 text-sm">
        {/* El correo se resalta partiendo la plantilla por su hueco: así la frase
            sigue siendo UNA cadena traducible y no dos trozos concatenados. */}
        {highlight(
          t(status === 'sent' ? 'verifyEmail.sent' : 'verifyEmail.pending', { email: MARK }),
          email,
        )}
      </p>
      {/* aria-live: el lector anuncia el resultado del reenvío sin mover el foco. */}
      <span aria-live="polite" class="sr-only">
        {status === 'sent' ? t('verifyEmail.sentAnnouncement') : ''}
        {status === 'error' ? t('verifyEmail.errorAnnouncement') : ''}
      </span>
      {status !== 'sent' ? (
        <Button
          variant="secondary"
          size="sm"
          class="shrink-0"
          loading={status === 'sending'}
          onClick={() => void resend()}
        >
          {status === 'error' ? t('common.retry') : t('verifyEmail.resend')}
        </Button>
      ) : null}
      {status === 'error' ? (
        <span class="text-danger-text w-full text-xs">{t('verifyEmail.errorHint')}</span>
      ) : null}
    </div>
  );
}
