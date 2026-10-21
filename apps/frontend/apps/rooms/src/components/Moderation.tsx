/**
 * Denunciar y bloquear (ADR 0073): los tres diálogos.
 *
 *  - ReportModal: motivo de una lista corta y cerrada (los `Radio` de @sytue/ui dentro de
 *    un fieldset: el lector anuncia el grupo y las flechas cambian de opción) y una nota opcional.
 *  - BlockedModal: a quién se tiene bloqueado, con «Desbloquear».
 *  - ReportsModal: la cola de quien modera, con las tres acciones.
 *
 * El aviso de resultado va por toast (role=status): lo anuncia el lector sin
 * mover el foco.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Modal, Radio, Skeleton, Textarea, useToast } from '@sytue/ui';
import {
  ChatHttpError,
  REPORT_REASONS,
  listReports,
  resolveReport,
  sendReport,
  type BlockedPerson,
  type ModerationReport,
  type ReportAction,
  type ReportReason,
  type ReportTarget,
} from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

const REASON_KEY = {
  harassment: 'report.reasonHarassment',
  sexual: 'report.reasonSexual',
  violence: 'report.reasonViolence',
  spam: 'report.reasonSpam',
  other: 'report.reasonOther',
} as const;

function reasonLabel(t: T, reason: string): string {
  return reason in REASON_KEY ? t(REASON_KEY[reason as ReportReason]) : reason;
}

/** Mensaje de una persona bloqueada: plegado, con un botón para leerlo. El texto
 *  dice POR QUÉ no se ve; no es un hueco. */
export function BlockedFold({ t, onShow }: { t: T; onShow: () => void }): JSX.Element {
  return (
    <p class="text-text-subtle text-sm">
      {t('block.folded')}{' '}
      <button
        type="button"
        class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
        onClick={onShow}
      >
        {t('block.show')}
      </button>
    </p>
  );
}

export function ReportModal({
  target,
  name,
  onClose,
  t,
}: {
  /** Qué se denuncia; null = cerrado. */
  target: ReportTarget | null;
  /** Nombre de la persona denunciada (o autora del mensaje). */
  name: string;
  onClose: () => void;
  t: T;
}): JSX.Element {
  const toast = useToast();
  const [reason, setReason] = useState<ReportReason | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!target) return;
    setReason(null);
    setNote('');
  }, [target]);

  function submit(): void {
    if (!target || !reason) return;
    setBusy(true);
    sendReport(target, reason, note.trim())
      .then(() => {
        toast.notify({ tone: 'success', title: t('report.sent') });
        onClose();
      })
      .catch((err: unknown) => {
        const tope = err instanceof ChatHttpError && err.status === 429;
        toast.notify({ tone: 'danger', title: t(tope ? 'report.tooMany' : 'report.failed') });
      })
      .finally(() => setBusy(false));
  }

  return (
    <Modal
      open={target !== null}
      onClose={onClose}
      title={'messageId' in (target ?? {}) ? t('report.message') : t('report.person', { name })}
      footer={
        <>
          {/* Fantasma, como el resto de «Cancelar» de Rooms: con relleno se leía como
              una segunda acción principal junto a «Denunciar». */}
          <Button variant="ghost" onClick={onClose}>
            {t('action.cancel')}
          </Button>
          <Button onClick={submit} disabled={!reason} loading={busy}>
            {t('report.submit')}
          </Button>
        </>
      }
    >
      <p class="text-text-muted mb-3 text-sm leading-relaxed">{t('report.intro')}</p>
      <fieldset class="mb-3">
        <legend class="text-text mb-1 text-sm font-semibold">{t('report.reason')}</legend>
        {REPORT_REASONS.map((r) => (
          <Radio
            key={r}
            name="report-reason"
            value={r}
            checked={reason === r}
            onChange={() => setReason(r)}
            label={reasonLabel(t, r)}
            class="hover:bg-bg-subtle w-full rounded-md px-2 py-2 text-sm"
          />
        ))}
      </fieldset>
      <label class="text-text block text-sm font-semibold" for="report-note">
        {t('report.note')}
      </label>
      <Textarea
        id="report-note"
        rows={3}
        maxLength={500}
        value={note}
        onInput={(e) => setNote((e.target as HTMLTextAreaElement).value)}
        class="mt-1 w-full"
      />
    </Modal>
  );
}

export function BlockedModal({
  open,
  blocks,
  onUnblock,
  onClose,
  t,
}: {
  open: boolean;
  blocks: BlockedPerson[];
  onUnblock: (b: BlockedPerson) => void;
  onClose: () => void;
  t: T;
}): JSX.Element {
  return (
    <Modal open={open} onClose={onClose} title={t('block.list')}>
      <p class="text-text-muted mb-3 text-sm leading-relaxed">{t('block.explain')}</p>
      {blocks.length === 0 ? (
        <p class="text-text-subtle text-sm">{t('block.listEmpty')}</p>
      ) : (
        <ul class="divide-border divide-y">
          {blocks.map((b) => (
            <li key={b.sub} class="flex items-center gap-3 py-2">
              <span class="text-text min-w-0 flex-1 truncate text-sm font-semibold">{b.name}</span>
              <Button
                size="sm"
                variant="secondary"
                aria-label={t('block.unblock', { name: b.name })}
                onClick={() => onUnblock(b)}
              >
                {t('block.unblockShort')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}

export function ReportsModal({
  open,
  onClose,
  t,
}: {
  open: boolean;
  onClose: () => void;
  t: T;
}): JSX.Element {
  const toast = useToast();
  const [reports, setReports] = useState<ModerationReport[] | null>(null);
  const [error, setError] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const ac = new AbortController();
    setReports(null);
    setError(false);
    listReports(ac.signal)
      .then((d) => setReports(d.reports))
      .catch(() => {
        if (!ac.signal.aborted) setError(true);
      });
    return () => ac.abort();
  }, [open]);

  function resolve(r: ModerationReport, action: ReportAction): void {
    setBusyId(r.id);
    resolveReport(r.id, action)
      .then(() => {
        setReports((prev) => (prev ?? []).filter((x) => x.id !== r.id));
        toast.notify({ tone: 'success', title: t('moderation.resolved') });
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('block.failed') }))
      .finally(() => setBusyId(null));
  }

  return (
    <Modal open={open} onClose={onClose} title={t('moderation.title')} size="lg">
      {error ? (
        <p class="text-danger text-sm" role="alert">
          {t('moderation.loadFailed')}
        </p>
      ) : reports === null ? (
        <Skeleton class="h-16 w-full" />
      ) : reports.length === 0 ? (
        <p class="text-text-subtle text-sm">{t('moderation.empty')}</p>
      ) : (
        <ul class="divide-border divide-y">
          {reports.map((r) => (
            <li key={r.id} class="py-3">
              <p class="text-text text-sm font-semibold">{r.targetName}</p>
              <p class="sytue-meta">
                {reasonLabel(t, r.reason)}
                {' · '}
                {r.channelKind === 'direct'
                  ? t('moderation.inDirect')
                  : t('moderation.inChannel', { channel: r.channelName })}
              </p>
              {r.messageBody ? (
                <blockquote class="border-border text-text mt-1 border-l-2 pl-2 text-sm whitespace-pre-wrap">
                  {r.messageBody}
                </blockquote>
              ) : (
                <p class="text-text-subtle mt-1 text-sm">
                  {r.messageId ? t('moderation.messageGone') : t('moderation.personOnly')}
                </p>
              )}
              {r.note ? (
                <p class="text-text-muted mt-1 text-sm">{t('moderation.note', { note: r.note })}</p>
              ) : null}
              <div class="mt-2 flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={busyId === r.id}
                  onClick={() => resolve(r, 'dismissed')}
                >
                  {t('moderation.dismiss')}
                </Button>
                {r.messageBody ? (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={busyId === r.id}
                    onClick={() => resolve(r, 'message_deleted')}
                  >
                    {t('moderation.deleteMessage')}
                  </Button>
                ) : null}
                {r.channelKind !== 'direct' ? (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={busyId === r.id}
                    onClick={() => resolve(r, 'member_removed')}
                  >
                    {t('moderation.removeMember')}
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
