/**
 * IncomingCall — aviso de llamada entrante en un canal que NO estás mirando.
 *
 * El frame `call_started` del WebSocket ya llegaba; lo que hacía el cliente era
 * descartarlo si el canal no era el abierto, así que a quien te llamaba por
 * mensaje directo no lo veías hasta abrir esa conversación. Esto es la otra mitad:
 * lo que se ve —y, en un directo, lo que suena— cuando la llamada es para ti.
 *
 * Se pinta arriba a la derecha y no como un modal a propósito: una llamada no
 * puede secuestrar lo que estabas escribiendo. Con `role="alertdialog"` el lector
 * de pantalla lo anuncia sin mover el foco de donde estaba.
 */
import type { JSX } from 'preact';
import { useRef } from 'preact/hooks';
import { Button, useFitInViewport, useMobileBack } from '@sytue/ui';
import type { T } from '../i18n/index.js';

export interface IncomingCallProps {
  /** Nombre del canal o de la persona que llama. */
  label: string;
  /** Un directo se anuncia como llamada personal; un canal, como llamada del canal. */
  direct: boolean;
  onAccept: () => void;
  onDismiss: () => void;
  t: T;
}

export function IncomingCall({
  label,
  direct,
  onAccept,
  onDismiss,
  t,
}: IncomingCallProps): JSX.Element {
  const ref = useRef<HTMLDivElement>(null);
  useFitInViewport(ref, true, { vertical: true });
  // El aviso queda bajo los cajones y los diálogos que puedan estar abiertos.
  useMobileBack(true, onDismiss, { nivel: 0 });
  return (
    <div
      ref={ref}
      role="alertdialog"
      aria-label={
        direct
          ? t('call.incomingDirect', { name: label })
          : t('call.incomingChannel', { name: label })
      }
      class="sytue-popover-surface border-border fixed top-[max(1rem,var(--sytue-safe-top,0px))] right-[max(1rem,var(--sytue-safe-right,0px))] z-50 w-72 rounded-lg border p-3 shadow-[var(--shadow-lg)]"
    >
      <p class="text-text truncate text-sm font-semibold" title={label}>
        {label}
      </p>
      <p class="sytue-meta mt-0.5 mb-2.5">
        {direct ? t('call.incomingDirectShort') : t('call.incomingChannelShort')}
      </p>
      <div class="flex gap-2">
        {/* Contestar primero y a la izquierda: es lo que se busca con prisa. */}
        <Button size="sm" onClick={onAccept}>
          {t('call.answer')}
        </Button>
        <Button size="sm" variant="secondary" onClick={onDismiss}>
          {t('call.decline')}
        </Button>
      </div>
    </div>
  );
}
