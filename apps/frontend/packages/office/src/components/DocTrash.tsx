/**
 * DocTrash — la papelera de Sytue Docs (decidido por el dueño, 2026-09-28).
 *
 * Borrar un documento lo mandaba a una papelera que nadie podía ver. Aquí se ve,
 * se restaura y se vacía; y como cada documento vive como su .odt/.ods/.odp en
 * Sytue Drive, el servicio hace lo mismo con ese fichero en la papelera de Drive.
 *
 * Mismo patrón que la papelera de Drive (`TrashView` de apps/drive): lista,
 * «Restaurar», «Eliminar definitivamente» con confirmación, y aquí además
 * «Vaciar papelera», también con confirmación. Habla solo con el BFF (/api/trash).
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import {
  Button,
  EmptyState,
  Icon,
  Illustration,
  InlineHelp,
  Modal,
  Skeleton,
  useToast,
} from '@sytue/ui';
import type { DocKind } from '@sytue/api-clients';
import { formatDate, type Locale } from '@sytue/i18n';
import { useConnectivity } from '@sytue/offline/preact';
import { docsT } from '../i18n';
import { r } from '../lib/rutas';
import { KindGlyph } from './DocList';

/** Un documento de la papelera, tal y como lo devuelve el servicio (snake_case). */
interface DocEnPapelera {
  id: string;
  kind: DocKind;
  title: string;
  deleted_at?: string;
}

interface DocTrashProps {
  locale: Locale;
  /** Tipo fijado por la URL (Calcus solo ve hojas): el mismo `?kind=` que la lista. */
  kind?: DocKind | '';
  /** Organización de la URL, que el servidor conoce (ADR 0056). */
  org?: string | null;
}

/** Lo que contesta el servicio al fallar, para elegir la frase. */
async function codigoDe(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: { code?: string } | string; code?: string };
    if (typeof body.error === 'object' && body.error?.code) return body.error.code;
    return body.code ?? '';
  } catch {
    return '';
  }
}

async function accion(body: {
  action: 'restore' | 'purge' | 'empty';
  id?: string;
  kind?: DocKind | '';
  after?: string;
}): Promise<Response> {
  return fetch('/api/trash', {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export function DocTrash({ locale, kind = '', org }: DocTrashProps) {
  const t = docsT(locale);
  const toast = useToast();
  const offline = useConnectivity() === 'offline';
  const [docs, setDocs] = useState<DocEnPapelera[] | null>(null);
  const [error, setError] = useState(false);
  const [busy, setBusy] = useState(false);
  const [pendingPurge, setPendingPurge] = useState<DocEnPapelera | null>(null);
  const [confirmEmpty, setConfirmEmpty] = useState(false);

  const load = useCallback(async () => {
    setDocs(null);
    setError(false);
    try {
      const res = await fetch(`/api/trash${kind ? `?kind=${kind}` : ''}`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(String(res.status));
      const body = (await res.json()) as { documents?: DocEnPapelera[] };
      setDocs(body.documents ?? []);
    } catch {
      setError(true);
    }
  }, [kind]);

  useEffect(() => {
    void load();
  }, [load]);

  const onRestore = async (doc: DocEnPapelera) => {
    if (offline || busy) return;
    setBusy(true);
    try {
      const res = await accion({ action: 'restore', id: doc.id });
      if (!res.ok) {
        const code = await codigoDe(res);
        toast.notify({
          tone: 'danger',
          title:
            code === 'file_name_taken'
              ? t('trash.restoreConflict', { title: doc.title })
              : code === 'already_restored'
                ? t('trash.restoreDuplicate', { title: doc.title })
                : t('trash.restoreFailed'),
        });
        return;
      }
      toast.notify({ tone: 'success', title: t('trash.restored', { title: doc.title }) });
      await load();
    } finally {
      setBusy(false);
    }
  };

  const onPurge = async (doc: DocEnPapelera) => {
    if (offline || busy) return;
    setBusy(true);
    try {
      const res = await accion({ action: 'purge', id: doc.id });
      if (!res.ok) {
        const code = await codigoDe(res);
        toast.notify({
          tone: 'danger',
          title:
            code === 'legal_hold'
              ? t('trash.purgeHold', { title: doc.title })
              : t('trash.purgeFailed'),
        });
        return;
      }
      toast.notify({ tone: 'success', title: t('trash.purged', { title: doc.title }) });
      await load();
    } finally {
      setBusy(false);
      setPendingPurge(null);
    }
  };

  const onEmpty = async () => {
    if (offline || busy) return;
    setBusy(true);
    let fallidos = 0;
    try {
      // El servicio vacía por páginas y dice por dónde seguir (`next`), saltando lo
      // que no pudo borrar: se repite mientras quede algo.
      let after = '';
      for (let vuelta = 0; vuelta < 200; vuelta++) {
        const res = await accion({ action: 'empty', kind, after });
        if (!res.ok) throw new Error(String(res.status));
        const out = (await res.json()) as { failed?: number; next?: string };
        fallidos += out.failed ?? 0;
        after = out.next ?? '';
        if (!after) break;
      }
      // Con `after` aún puesto se paró en el tope de vueltas: no está vacía.
      toast.notify(
        fallidos > 0 || after
          ? { tone: 'danger', title: t('trash.emptyPartial') }
          : { tone: 'success', title: t('trash.emptied') },
      );
    } catch {
      toast.notify({ tone: 'danger', title: t('trash.purgeFailed') });
    } finally {
      setBusy(false);
      setConfirmEmpty(false);
      await load();
    }
  };

  const volver = r(kind ? `/?kind=${kind}` : '/', org);

  return (
    <div class="mx-auto flex min-h-[calc(100dvh-var(--spacing-topbar))] w-full max-w-[var(--container-content,1280px)] flex-col px-6 pt-8 pb-10">
      <header class="mb-6 flex flex-wrap items-start gap-3">
        <div class="min-w-0 flex-1">
          <a
            href={volver}
            class="sytue-focus-ring text-text-muted hover:text-text mb-2 inline-flex items-center gap-1 rounded text-sm"
          >
            <Icon name="chevronLeft" size={16} />
            {t('trash.back')}
          </a>
          <h1 class="sytue-h2 text-text">{t('trash.title')}</h1>
          <p class="text-text-muted mt-1 max-w-2xl text-base">{t('trash.subtitle')}</p>
        </div>
        {docs && docs.length > 0 ? (
          <Button
            variant="danger"
            size="sm"
            disabled={busy || offline}
            onClick={() => setConfirmEmpty(true)}
          >
            {t('trash.empty')}
          </Button>
        ) : null}
      </header>

      {docs === null && !error ? (
        <div aria-busy="true" class="flex flex-col gap-2">
          <span class="sr-only">{t('list.loading')}</span>
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} height="56px" class="rounded-lg" />
          ))}
        </div>
      ) : error ? (
        <EmptyState
          title={t('trash.errorTitle')}
          description={t('list.error.description')}
          illustration={<Icon name="warning" size={40} class="text-danger-text" />}
          action={
            <Button variant="secondary" size="sm" onClick={() => void load()}>
              {t('list.retry')}
            </Button>
          }
        />
      ) : docs && docs.length === 0 ? (
        <EmptyState
          title={t('trash.emptyTitle')}
          description={t('trash.emptyBody')}
          illustration={<Illustration name="emptyDrive" />}
        />
      ) : (
        <ul>
          <li class="pb-2">
            <InlineHelp tone="neutral">{t('trash.help')}</InlineHelp>
          </li>
          {docs!.map((doc) => (
            // LA FILA ENVUELVE. Las dos acciones iban en un bloque que no encogía
            // (`shrink-0`) al lado de un título sin mínimo: a 320 px el bloque se salía
            // 25 px de la página, y a 360 cabía a costa de dejar el título en cero. Ahora
            // el título conserva 8rem, y cuando no caben los dos las acciones bajan a su
            // propia línea —y ahí, si hace falta, un botón debajo del otro—.
            <li
              key={doc.id}
              class="border-border/60 flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-3 py-2"
            >
              <KindGlyph kind={doc.kind} size={28} />
              <span class="min-w-32 flex-1">
                <span class="text-text block truncate font-medium" title={doc.title}>
                  {doc.title}
                </span>
                {doc.deleted_at ? (
                  <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                    {t('trash.deletedOn', {
                      date: formatDate(locale, new Date(doc.deleted_at), {
                        day: '2-digit',
                        month: 'short',
                        year: 'numeric',
                      }),
                    })}
                  </span>
                ) : null}
              </span>
              <span class="ml-auto flex flex-wrap items-center justify-end gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={busy || offline}
                  onClick={() => void onRestore(doc)}
                >
                  {t('trash.restore')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-danger-text"
                  disabled={busy || offline}
                  onClick={() => setPendingPurge(doc)}
                >
                  {t('trash.purge')}
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {/* Eliminar uno: irreversible, siempre con confirmación. */}
      <Modal
        open={pendingPurge !== null}
        onClose={() => setPendingPurge(null)}
        title={t('trash.purgeTitle')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingPurge(null)} disabled={busy}>
              {t('action.cancel')}
            </Button>
            <Button
              variant="danger"
              loading={busy}
              onClick={() => pendingPurge && void onPurge(pendingPurge)}
            >
              {t('trash.purge')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base leading-relaxed">
          {t('trash.purgeBody', { title: pendingPurge?.title ?? '' })}
        </p>
      </Modal>

      {/* Vaciar: irreversible, siempre con confirmación. */}
      <Modal
        open={confirmEmpty}
        onClose={() => setConfirmEmpty(false)}
        title={t('trash.emptyConfirmTitle')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmEmpty(false)} disabled={busy}>
              {t('action.cancel')}
            </Button>
            <Button variant="danger" loading={busy} onClick={() => void onEmpty()}>
              {t('trash.empty')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base leading-relaxed">{t('trash.emptyConfirmBody')}</p>
      </Modal>
    </div>
  );
}
