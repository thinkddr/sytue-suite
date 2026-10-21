/**
 * NotifySettingsModal — preferencias de notificación del usuario (Ola 13): palabras clave
 * que disparan un aviso de escritorio (además de las @menciones) y ventana «No molestar»
 * (DND). Las palabras clave se editan como chips; el DND con dos selectores de hora
 * NATIVOS (<input type="time">). Carga las prefs al abrir y las guarda con putNotifyPrefs
 * (el backend las sanea: dedupe/recorte, rango DND). DND desactivado se guarda como
 * start==end (00:00–00:00); activado, con la franja elegida (cruce de medianoche válido).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Modal, Input, Button, Switch } from '@sytue/ui';
import { getNotifyPrefs, putNotifyPrefs, getPushKey, type NotifyPrefs } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { RemovableChip } from './RemovableChip.js';
import {
  pushSupported,
  isPushSubscribed,
  subscribeToPush,
  unsubscribeFromPush,
} from '../lib/push.js';

interface Props {
  open: boolean;
  onClose: () => void;
  /** Notifica al padre las prefs guardadas (para refrescar sus refs de aviso en vivo). */
  onSaved: (prefs: NotifyPrefs) => void;
  /** Traductor ya atado al idioma. */
  t: T;
}

/** "HH:MM" (24h) → minutos del día [0..1439]. */
function timeToMinutes(v: string): number {
  const parts = v.split(':');
  const h = parseInt(parts[0] ?? '', 10);
  const m = parseInt(parts[1] ?? '', 10);
  if (Number.isNaN(h) || Number.isNaN(m)) return 0;
  return Math.min(23, Math.max(0, h)) * 60 + Math.min(59, Math.max(0, m));
}

/** minutos del día → "HH:MM" (24h, cero-padded). */
function minutesToTime(min: number): string {
  const h = Math.floor(min / 60) % 24;
  const m = min % 60;
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

export function NotifySettingsModal({ open, onClose, onSaved, t }: Props): JSX.Element | null {
  const [keywords, setKeywords] = useState<string[]>([]);
  const [draft, setDraft] = useState('');
  const [dndOn, setDndOn] = useState(false);
  const [dndStart, setDndStart] = useState(22 * 60); // 22:00 por defecto al activar
  const [dndEnd, setDndEnd] = useState(7 * 60); // 07:00
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  // Web Push (Ola 18): clave VAPID (vacía = push no configurado en el servidor) + estado
  // de la suscripción de ESTE navegador. El toggle solo aparece si hay soporte y clave.
  const [vapidKey, setVapidKey] = useState('');
  const [pushOn, setPushOn] = useState(false);
  const [pushBusy, setPushBusy] = useState(false);

  // Carga las prefs al abrir (reinicia el estado en cada apertura).
  useEffect(() => {
    if (!open) return;
    let live = true;
    setLoading(true);
    setError(false);
    setDraft('');
    getNotifyPrefs()
      .then(({ prefs }) => {
        if (!live) return;
        setKeywords(prefs.keywords);
        const on = prefs.dndStart !== prefs.dndEnd;
        setDndOn(on);
        if (on) {
          setDndStart(prefs.dndStart);
          setDndEnd(prefs.dndEnd);
        }
      })
      .catch(() => live && setError(true))
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [open]);

  // Estado del Web Push al abrir: clave VAPID del servidor + ¿suscrito ya este navegador?
  // Degrada en silencio (sin soporte o sin clave, el toggle no aparece).
  useEffect(() => {
    if (!open || !pushSupported()) return;
    let live = true;
    void Promise.all([
      getPushKey().catch(() => ({ key: '' })),
      isPushSubscribed().catch(() => false),
    ]).then(([{ key }, subscribed]) => {
      if (!live) return;
      setVapidKey(key);
      setPushOn(subscribed);
    });
    return () => {
      live = false;
    };
  }, [open]);

  // Activa/desactiva el push de este navegador: pide permiso + suscribe, o desuscribe.
  async function togglePush(): Promise<void> {
    if (pushBusy) return;
    setPushBusy(true);
    try {
      if (pushOn) {
        await unsubscribeFromPush();
        setPushOn(false);
      } else {
        setPushOn(await subscribeToPush(vapidKey));
      }
    } catch {
      // Permiso denegado o fallo de red: deja el estado como estaba.
    } finally {
      setPushBusy(false);
    }
  }

  function addKeyword(): void {
    const k = draft.trim().toLowerCase();
    setDraft('');
    if (!k) return;
    setKeywords((prev) => (prev.includes(k) ? prev : [...prev, k]));
  }

  function removeKeyword(k: string): void {
    setKeywords((prev) => prev.filter((x) => x !== k));
  }

  function save(): void {
    if (busy) return;
    setBusy(true);
    setError(false);
    // DND desactivado ⇒ start==end (deshabilitado). Activado ⇒ la franja elegida.
    const prefs: NotifyPrefs = dndOn
      ? { keywords, dndStart, dndEnd }
      : { keywords, dndStart: 0, dndEnd: 0 };
    putNotifyPrefs(prefs)
      .then(({ prefs: saved }) => {
        onSaved(saved);
        onClose();
      })
      .catch(() => setError(true))
      .finally(() => setBusy(false));
  }

  if (!open) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('notify.title')}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('action.cancel')}
          </Button>
          <Button variant="primary" onClick={save} loading={busy} disabled={loading}>
            {t('action.save')}
          </Button>
        </>
      }
    >
      <div class="flex flex-col gap-5">
        {error ? (
          <p class="text-danger-text text-sm" role="alert">
            {t('notify.failed')}
          </p>
        ) : null}

        {/* Palabras clave */}
        <div>
          <label class="text-text mb-1 block text-sm font-semibold" for="notify-kw">
            {t('notify.keywords')}
          </label>
          <p class="text-text-muted mb-2 text-xs">{t('notify.keywordsHelp')}</p>
          {keywords.length > 0 ? (
            <ul class="mb-2 flex flex-wrap gap-1.5" aria-label={t('notify.keywords')}>
              {keywords.map((k) => (
                <li key={k}>
                  <RemovableChip
                    label={k}
                    ariaLabel={t('action.removeItem', { name: k })}
                    onRemove={() => removeKeyword(k)}
                  />
                </li>
              ))}
            </ul>
          ) : null}
          <Input
            id="notify-kw"
            type="text"
            value={draft}
            maxLength={64}
            placeholder={t('notify.keywordPlaceholder')}
            disabled={loading}
            onInput={(e) => setDraft(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ',') {
                e.preventDefault();
                addKeyword();
              }
            }}
            aria-label={t('notify.addKeyword')}
          />
        </div>

        {/* No molestar (DND) */}
        <div>
          <div class="flex items-start justify-between gap-3">
            <p class="text-text-muted text-xs">{t('notify.dndHelp')}</p>
            <Switch
              label={t('presence.dnd')}
              checked={dndOn}
              disabled={loading}
              onChange={() => setDndOn((v) => !v)}
            />
          </div>
          {dndOn ? (
            <div class="mt-3 flex flex-wrap items-center gap-2">
              {/* «De [hora] a [hora]» en una línea: con el `w-full` del campo, cada rótulo
                  quedaba suelto encima de un campo de 342 y la franja se leía rota. */}
              <label class="text-text-muted text-sm" for="dnd-start">
                {t('notify.from')}
              </label>
              <Input
                id="dnd-start"
                type="time"
                class="w-32"
                value={minutesToTime(dndStart)}
                onInput={(e) => setDndStart(timeToMinutes(e.currentTarget.value))}
                aria-label={t('notify.dndStart')}
              />
              <label class="text-text-muted text-sm" for="dnd-end">
                {t('notify.to')}
              </label>
              <Input
                id="dnd-end"
                type="time"
                class="w-32"
                value={minutesToTime(dndEnd)}
                onInput={(e) => setDndEnd(timeToMinutes(e.currentTarget.value))}
                aria-label={t('notify.dndEnd')}
              />
            </div>
          ) : null}
        </div>

        {/* Notificaciones push (Ola 18): avisos con la app cerrada (Web Push). Solo si hay
            soporte del navegador y el servidor tiene VAPID configurado (vapidKey no vacía). */}
        {pushSupported() && vapidKey ? (
          <div>
            <div class="flex items-start justify-between gap-3">
              <p class="text-text-muted text-xs">{t('notify.pushHelp')}</p>
              <Switch
                label={t('notify.push')}
                checked={pushOn}
                disabled={pushBusy}
                onChange={() => void togglePush()}
              />
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
