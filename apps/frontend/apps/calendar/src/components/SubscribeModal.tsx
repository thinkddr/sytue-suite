/**
 * SubscribeModal — «Añadir calendario por URL»: suscribe un feed ICS externo
 * (festivos, equipo, …) como un calendario de SOLO LECTURA. Reutiliza el design
 * system Sytue (Modal + Form) y el selector de color de CalendarModal. La URL se
 * valida y se obtiene SSRF-safe en el servicio Go; aquí solo se comprueba la
 * forma (http/https) para dar feedback inmediato. docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, DataFlow, FieldGroup, Input, Modal, describedBy, useToast } from '@sytue/ui';
import { subscribeIcs } from '../client/calendarApi.js';
import { CALENDAR_PALETTE, ColorSwatches } from './CalendarModal.js';
import type { T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  open: boolean;
  onClose: () => void;
  /** Tras suscribir con éxito (el llamante recarga la lista). */
  onSaved: () => void;
}

/** Forma mínima de una URL de feed: http/https con host. El servicio Go valida de verdad. */
function looksLikeUrl(v: string): boolean {
  return /^https?:\/\/[^\s/]+/i.test(v.trim());
}

export function SubscribeModal({ t, open, onClose, onSaved }: Props): JSX.Element | null {
  const toast = useToast();
  const [url, setUrl] = useState('');
  const [name, setName] = useState('');
  const [color, setColor] = useState(CALENDAR_PALETTE[7]!.value); // Ciruela (calendar)
  const [busy, setBusy] = useState(false);
  const [urlError, setUrlError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setUrl('');
    setName('');
    setColor(CALENDAR_PALETTE[7]!.value);
    setUrlError(null);
  }, [open]);

  async function handleSave(): Promise<void> {
    const trimmedUrl = url.trim();
    if (!looksLikeUrl(trimmedUrl)) {
      setUrlError(t('subscribe.urlInvalid'));
      document.getElementById('sub-url')?.focus();
      return;
    }
    setBusy(true);
    try {
      await subscribeIcs({
        url: trimmedUrl,
        displayName: name.trim() || t('subscribe.defaultName'),
        color,
      });
      toast.notify({ tone: 'success', title: t('toast.calendarSubscribed') });
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('toast.subscribeFailed'),
        description: t('toast.subscribeFailedHint'),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('subscribe.title')}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={() => void handleSave()} loading={busy}>
            {t('subscribe.submit')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-4">
        {/* La frase va ENTERA al catálogo: partirla para meter un <code> alrededor
            de «.ics» obligaría a pegar trozos, y el orden cambia entre lenguas. */}
        <p class="text-text-muted text-sm">{t('subscribe.intro')}</p>
        {/* Se lee el .ics de esa dirección; de aquí no sale nada hacia ella. */}
        <DataFlow compact incoming={['events']} />
        <FieldGroup
          label={t('subscribe.url')}
          htmlFor="sub-url"
          required
          {...(urlError ? { error: urlError } : {})}
        >
          <Input
            id="sub-url"
            type="url"
            value={url}
            placeholder={t('subscribe.urlPlaceholder')}
            invalid={Boolean(urlError)}
            aria-describedby={describedBy('sub-url', undefined, urlError ?? undefined)}
            onInput={(e) => {
              setUrl(e.currentTarget.value);
              if (urlError) setUrlError(null);
            }}
          />
        </FieldGroup>
        <FieldGroup label={t('subscribe.name')} htmlFor="sub-name">
          <Input
            id="sub-name"
            value={name}
            placeholder={t('subscribe.namePlaceholder')}
            onInput={(e) => setName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void handleSave();
              }
            }}
          />
        </FieldGroup>
        <div class="flex flex-col gap-1.5">
          <span id="sub-color-label" class="text-text text-sm font-semibold">
            {t('common.colour')}
          </span>
          <ColorSwatches
            t={t}
            value={color}
            onChange={setColor}
            idPrefix="sub-color"
            labelledBy="sub-color-label"
          />
        </div>
      </div>
    </Modal>
  );
}
