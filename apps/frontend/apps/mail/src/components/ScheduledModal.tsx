/**
 * ScheduledModal — vista/gestión de los envíos PROGRAMADOS de Sytue Mail. Lista
 * (paginada por keyset) los correos que el usuario dejó programados, permite
 * EDITAR su hora o CANCELARLOS (el servidor los reabre como borrador). Solo del
 * dueño (el BFF los pide con el token de sesión). docs/CONVENTIONS.md §5.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, FieldGroup, Input, Modal, Icon, Spinner, useToast } from '@sytue/ui';
import {
  listScheduled,
  cancelScheduled,
  rescheduleScheduled,
  type ScheduledItem,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { fullDate, toLocalInput } from './utils.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  open: boolean;
  onClose: () => void;
}

/** Lo más pronto que se puede reprogramar: dentro de un minuto, como valor del
 *  `<input type="datetime-local">`. Solo lo llama el manejador que abre la edición. */
function minimoReprogramable(): string {
  return toLocalInput(new Date(Date.now() + 60_000));
}

export function ScheduledModal({ locale, open, onClose }: Props): JSX.Element | null {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<ScheduledItem[]>([]);
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [reload, setReload] = useState(0);
  // Edición de hora en curso: id + valor del datetime-local.
  const [editId, setEditId] = useState<string | null>(null);
  const [editAt, setEditAt] = useState('');
  // Mínimo del selector (dentro de un minuto), fijado al abrir la edición: el render
  // no puede leer el reloj. Lo que manda es la comprobación de `save`, que sí lo lee.
  const [editMin, setEditMin] = useState('');
  const [busyId, setBusyId] = useState<string | null>(null);
  // Foco accesible al abrir/cerrar el editor de hora (los controles de
  // @sytue/ui no reenvían ref, así que se localizan por id).
  const lastEditRef = useRef<string | null>(null);

  // Carga la primera página al abrir (o al reintentar).
  useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setError(false);
    setEditId(null);
    listScheduled()
      .then(({ scheduled, nextCursor }) => {
        if (!active) return;
        setItems(scheduled);
        setCursor(nextCursor);
      })
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, [open, reload]);

  // Al revelar el editor, lleva el foco al input; al cerrarlo, lo devuelve al
  // botón «Editar hora» de esa fila.
  useEffect(() => {
    if (editId) {
      document.getElementById(`sched-at-${editId}`)?.focus();
      lastEditRef.current = editId;
    } else if (lastEditRef.current) {
      document.getElementById(`sched-edit-${lastEditRef.current}`)?.focus();
      lastEditRef.current = null;
    }
  }, [editId]);

  function handleLoadMore(): void {
    if (loadingMore || !cursor) return;
    setLoadingMore(true);
    listScheduled(cursor)
      .then(({ scheduled, nextCursor }) => {
        setItems((prev) => [...prev, ...scheduled.filter((s) => !prev.some((p) => p.id === s.id))]);
        setCursor(nextCursor);
      })
      .catch(() => setCursor(''))
      .finally(() => setLoadingMore(false));
  }

  async function handleCancel(id: string): Promise<void> {
    setBusyId(id);
    try {
      await cancelScheduled(id);
      setItems((prev) => prev.filter((s) => s.id !== id));
      toast.notify({ tone: 'success', title: t('scheduled.canceled') });
    } catch {
      // 409 = el worker ya lo liberó (salió). Refresca para reflejarlo.
      toast.notify({ tone: 'danger', title: t('scheduled.cancelGone') });
      setReload((n) => n + 1);
    } finally {
      setBusyId(null);
    }
  }

  function startEdit(item: ScheduledItem): void {
    setEditId(item.id);
    setEditAt(toLocalInput(new Date(item.releaseAt)));
    setEditMin(minimoReprogramable());
  }

  async function saveEdit(id: string): Promise<void> {
    const when = new Date(editAt);
    if (!editAt || Number.isNaN(when.getTime()) || when.getTime() <= Date.now()) {
      toast.notify({ tone: 'danger', title: t('compose.scheduleFuture') });
      return;
    }
    setBusyId(id);
    try {
      const { releaseAt } = await rescheduleScheduled(id, when.toISOString());
      setItems((prev) =>
        prev
          .map((s) => (s.id === id ? { ...s, releaseAt: releaseAt || when.toISOString() } : s))
          .sort((a, b) => a.releaseAt.localeCompare(b.releaseAt)),
      );
      setEditId(null);
      toast.notify({ tone: 'success', title: t('scheduled.timeUpdated') });
    } catch {
      toast.notify({ tone: 'danger', title: t('scheduled.rescheduleGone') });
      setReload((n) => n + 1);
    } finally {
      setBusyId(null);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('scheduled.title')}
      size="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.close')}
          </Button>
        </>
      }
    >
      {loading ? (
        <div class="text-text-subtle flex items-center gap-2 py-6">
          <Spinner size={16} label={t('scheduled.loading')} />
          <span class="text-sm" aria-hidden="true">
            {t('common.loading')}
          </span>
        </div>
      ) : error ? (
        <div class="flex flex-col items-start gap-2 py-2" role="alert">
          <p class="text-text flex items-start gap-2 text-sm">
            <Icon name="warning" size={16} class="mt-0.5 shrink-0" />
            {t('scheduled.loadError')}
          </p>
          <Button size="sm" variant="secondary" onClick={() => setReload((n) => n + 1)}>
            {t('common.retry')}
          </Button>
        </div>
      ) : items.length === 0 ? (
        <p class="text-text-subtle py-6 text-sm">{t('scheduled.empty')}</p>
      ) : (
        <ul class="divide-border border-border flex flex-col divide-y rounded-md border">
          {items.map((s) => (
            <li key={s.id} class="flex flex-col gap-2 px-3 py-3">
              {/* En el teléfono, el mensaje entero arriba y sus acciones debajo, a la
                  derecha: al lado le quedaban 131 px y la fecha partía en dos. */}
              <div class="flex items-start justify-between gap-3 max-md:flex-col max-md:items-stretch max-md:gap-2">
                <div class="min-w-0">
                  <p class="text-text truncate text-sm font-medium">
                    {s.subject || t('common.noSubject')}
                  </p>
                  <p class="sytue-meta truncate">
                    {t('scheduled.recipients', { addresses: s.recipients.join(', ') })}
                  </p>
                  <p class="sytue-meta mt-0.5 flex items-center gap-1.5">
                    <Icon name="bell" size={14} class="shrink-0" />
                    <span class="tabular-nums">{fullDate(locale, s.releaseAt)}</span>
                  </p>
                </div>
                {editId !== s.id ? (
                  <div class="flex shrink-0 items-center gap-2 max-md:justify-end">
                    <Button
                      id={`sched-edit-${s.id}`}
                      size="sm"
                      variant="ghost"
                      disabled={busyId === s.id}
                      onClick={() => startEdit(s)}
                    >
                      {t('scheduled.editTime')}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      loading={busyId === s.id}
                      onClick={() => void handleCancel(s.id)}
                    >
                      {t('common.cancel')}
                    </Button>
                  </div>
                ) : null}
              </div>
              {editId === s.id ? (
                <div class="border-border flex flex-wrap items-end gap-3 border-t pt-3">
                  {/* `max-md:min-w-0`: aquí el hueco a 320px son 214px —el diálogo son
                      288, menos el `px-6` del cuerpo, el borde de la lista y el `px-3`
                      de la fila—, o sea 26 menos que los 240 de `min-w-[15rem]`. Un
                      mínimo mayor que su contenedor no encoge: sacaba el cuerpo del
                      diálogo a scroll lateral. */}
                  <div class="min-w-[15rem] flex-1 max-md:min-w-0 max-md:basis-full">
                    <FieldGroup label={t('scheduled.newDateTime')} htmlFor={`sched-at-${s.id}`}>
                      <Input
                        id={`sched-at-${s.id}`}
                        type="datetime-local"
                        value={editAt}
                        min={editMin}
                        onInput={(e) => setEditAt(e.currentTarget.value)}
                      />
                    </FieldGroup>
                  </div>
                  <div class="flex gap-2 max-md:w-full max-md:justify-end">
                    <Button size="sm" variant="ghost" onClick={() => setEditId(null)}>
                      {t('common.discard')}
                    </Button>
                    <Button
                      size="sm"
                      variant="primary"
                      loading={busyId === s.id}
                      onClick={() => void saveEdit(s.id)}
                    >
                      {t('scheduled.saveTime')}
                    </Button>
                  </div>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {!loading && cursor ? (
        <div class="mt-3 flex justify-center">
          <Button size="sm" variant="secondary" loading={loadingMore} onClick={handleLoadMore}>
            {t('common.loadMore')}
          </Button>
        </div>
      ) : null}
    </Modal>
  );
}
