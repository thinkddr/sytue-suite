/**
 * SmimeIdentityModal — instala el certificado S/MIME PROPIO del buzón (de una
 * CA pública, con su cadena y su clave privada), para que la firma se valide
 * SOLA fuera de Sytue sin que nadie tenga que instalar el ancla de la
 * organización a mano. Contrapartida de la descarga del ancla (nav.smimeAnchor):
 * aquella se BAJA, esta se SUBE.
 */
import type { JSX } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { Button, FieldHint, InlineHelp, Modal, Textarea, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

interface Props {
  locale: Locale;
  open: boolean;
  onClose: () => void;
}

export function SmimeIdentityModal({ locale, open, onClose }: Props): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const fileRef = useRef<HTMLInputElement>(null);
  const [pem, setPem] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function elegirFichero(input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    input.value = ''; // permite re-elegir el mismo fichero
    if (!file) return;
    setPem(await file.text());
    setError('');
  }

  async function instalar(): Promise<void> {
    if (busy || !pem.trim()) return;
    setBusy(true);
    setError('');
    try {
      const res = await fetch('/api/smime-identity', {
        method: 'PUT',
        headers: { 'content-type': 'application/x-pem-file' },
        body: pem,
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string };
        setError(body.message || t('smime.installFailed'));
        return;
      }
      toast.notify({ tone: 'success', title: t('smime.installed') });
      setPem('');
      onClose();
    } catch {
      setError(t('smime.installFailed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('smime.installTitle')}
      size="md"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!pem.trim()}
            onClick={() => void instalar()}
          >
            {t('smime.install')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        <InlineHelp tone="neutral" title={t('smime.installIntroTitle')}>
          {t('smime.installIntroBody')}
        </InlineHelp>
        <div class="flex flex-col gap-2">
          <input
            ref={fileRef}
            type="file"
            accept=".pem,.crt,.cer,.txt,text/plain,application/x-pem-file"
            class="hidden"
            aria-hidden="true"
            tabindex={-1}
            onChange={(e) => void elegirFichero(e.currentTarget)}
          />
          <Button variant="secondary" onClick={() => fileRef.current?.click()}>
            {t('smime.chooseFile')}
          </Button>
          <Textarea
            value={pem}
            onInput={(e) => {
              setPem(e.currentTarget.value);
              setError('');
            }}
            placeholder={t('smime.pastePlaceholder')}
            rows={8}
            class="font-mono text-xs"
          />
          <FieldHint tone={error ? 'danger' : 'neutral'}>{error || t('smime.hint')}</FieldHint>
        </div>
      </div>
    </Modal>
  );
}

export default SmimeIdentityModal;
