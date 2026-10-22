/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * VersionsDialog — historial de versiones de un fichero de Sytue Drive. Lista las
 * versiones (la ACTUAL primero, luego las archivadas más nueva primero) con su
 * autor, fecha y tamaño, y permite «Descargar esta versión» y «Restaurar» (hace
 * actual una versión anterior de forma NO destructiva: la actual pasa a ser una
 * versión más). Antes de restaurar se confirma EN LA FILA diciendo la consecuencia
 * real (qué contenido queda vivo y qué no se pierde); el encabezado explica qué es
 * una versión y cuánto se conserva. Habla SOLO con el BFF de esta app (/api/versions,
 * /api/version-content) por cookie de sesión httpOnly; el token nunca llega al
 * cliente. Un viewer no puede restaurar (el servicio lo deniega con 403).
 *
 * Idioma: lo publica <SytueShell> en el contexto de i18n a partir del locale que
 * negoció el servidor; aquí solo se LEE (`useI18n`), nunca se detecta. Este
 * diálogo lo monta el explorador —no una página—, y por eso no llega por props.
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, Modal, Spinner, EmptyState, InlineHelp, useToast, cx, withBase } from '@sytue/ui';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { useI18n } from '@sytue/i18n/preact';
import type { DriveResource } from '@sytue/api-clients';
import { useT } from '../i18n';
import { Glyph } from './glyphs';
import { formatBytes } from '@sytue/ui';
import { toVersion, type FileVersion, type RawVersion } from '../lib/versions';

/** Error con el código de estado HTTP del BFF (401 = sesión caducada). */
class BffError extends Error {
  constructor(public readonly status: number) {
    super(`bff_${status}`);
  }
}

async function bffJson<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const res = await fetch(input, { credentials: 'include', ...init });
  if (!res.ok) throw new BffError(res.status);
  return (await res.json()) as T;
}

/** Fecha y hora en el formato del idioma activo (cambian el orden y el reloj). */
function versionDate(locale: Locale, iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return formatDateTime(locale, d, {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** Href de descarga: la versión actual va por ruta; las archivadas por id+versión. */
function downloadHref(item: DriveResource, v: FileVersion): string {
  if (v.isCurrent)
    return withBase(import.meta.env.BASE_URL, `/api/content?path=${encodeURIComponent(item.path)}`);
  return withBase(
    import.meta.env.BASE_URL,
    `/api/version-content?id=${encodeURIComponent(item.id ?? '')}&version=${v.version}`,
  );
}

export function VersionsDialog({
  item,
  onClose,
  onRestored,
}: {
  item: DriveResource;
  onClose: () => void;
  // Tras restaurar: el explorador recarga la carpeta (el contenido actual cambió).
  onRestored: () => void;
}) {
  const toast = useToast();
  const { locale } = useI18n();
  const t = useT(locale);
  const [versions, setVersions] = useState<FileVersion[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [error, setError] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  // Versión en curso de restauración (deshabilita el resto de acciones).
  const [restoring, setRestoring] = useState<number | null>(null);
  // Versión con la confirmación abierta: restaurar cambia el contenido vivo del
  // archivo, así que antes se dice EXACTAMENTE qué va a pasar (y qué no).
  const [confirming, setConfirming] = useState<number | null>(null);

  const load = useCallback(
    async (cursor?: string) => {
      if (!item.id) {
        setError(true);
        return;
      }
      if (cursor) setLoadingMore(true);
      else {
        setVersions(null);
        setError(false);
      }
      try {
        const q = `id=${encodeURIComponent(item.id)}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`;
        const page = await bffJson<{ versions: FileVersion[] | RawVersion[]; nextCursor?: string }>(
          `/api/versions?${q}`,
        );
        // El BFF ya mapea a camelCase; el segundo map es idempotente y defensivo.
        const mapped = (page.versions as FileVersion[]).map((v) =>
          'sizeBytes' in v ? v : toVersion(v as RawVersion),
        );
        setVersions((prev) => (cursor ? [...(prev ?? []), ...mapped] : mapped));
        setNextCursor(page.nextCursor ?? null);
      } catch {
        if (!cursor) setError(true);
        else toast.notify({ tone: 'danger', title: t('versions.loadMoreError') });
      } finally {
        setLoadingMore(false);
      }
    },
    [item.id, toast, t],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const onRestore = async (v: FileVersion) => {
    if (!item.id || restoring !== null) return;
    setRestoring(v.version);
    try {
      await bffJson('/api/versions', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ id: item.id, version: v.version }),
      });
      toast.notify({
        tone: 'success',
        title: t('versions.restored', { name: item.name, version: v.version }),
      });
      setConfirming(null);
      onRestored();
    } catch (e) {
      toast.notify({
        tone: 'danger',
        title:
          e instanceof BffError && e.status === 403
            ? t('versions.restoreDenied')
            : t('versions.restoreFailed'),
      });
      setRestoring(null);
    }
  };

  // Solo hay versiones anteriores si la lista trae algo más que la actual.
  const hasHistory = (versions?.length ?? 0) > 1;
  const currentVersion = versions?.find((v) => v.isCurrent)?.version;

  return (
    <Modal
      open
      onClose={onClose}
      title={t('versions.title', { name: item.name })}
      footer={<Button onClick={onClose}>{t('versions.close')}</Button>}
    >
      {versions === null && !error ? (
        <div class="grid h-40 place-items-center" aria-busy="true">
          <Spinner size={22} />
          <span class="sr-only" role="status">
            {t('versions.loading')}
          </span>
        </div>
      ) : error ? (
        <div class="grid h-40 place-items-center p-4 text-center">
          <div>
            <p class="text-text-muted text-base">{t('versions.loadError')}</p>
            <Button variant="secondary" size="sm" class="mt-2" onClick={() => void load()}>
              {t('versions.retry')}
            </Button>
          </div>
        </div>
      ) : (
        <>
          {/* Qué es una versión y cuánto dura, antes de la lista: nadie debería
              tener que deducirlo de los números. */}
          <InlineHelp tone="neutral" class="mb-3" title={t('versions.helpTitle')}>
            {t('versions.helpBody')}
          </InlineHelp>

          {!hasHistory ? (
            <EmptyState
              class="py-6"
              title={t('versions.emptyTitle')}
              description={t('versions.emptyBody')}
              illustration={<Glyph name="clock" size={36} class="text-text-subtle" />}
            />
          ) : null}
          <ul class="divide-border/60 border-border divide-y rounded-md border">
            {versions!.map((v) => (
              <li key={v.version} class="px-3 py-2.5">
                <div class="flex items-center gap-3">
                  <span
                    class="bg-bg-subtle text-text-muted grid h-9 w-9 shrink-0 place-items-center rounded-md"
                    aria-hidden="true"
                  >
                    <Glyph name="clock" size={16} />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="flex items-center gap-2">
                      <span class="text-text truncate font-medium">
                        {t('versions.version', { version: v.version })}
                      </span>
                      {v.isCurrent ? (
                        <span class="bg-selected text-primary-text rounded-full px-2 py-0.5 text-xs font-semibold">
                          {t('versions.current')}
                        </span>
                      ) : null}
                    </span>
                    {/* En el teléfono, la fecha y debajo autor y tamaño: en una línea truncada a
                        137 px se perdían los dos. */}
                    <span class="sytue-meta mt-0.5 block truncate tabular-nums">
                      <span class="max-md:block max-md:truncate">
                        {versionDate(locale, v.createdAt) || '—'}
                      </span>
                      <span class="max-md:block max-md:truncate">
                        <span class="max-md:hidden"> · </span>
                        {v.author ? `${v.author} · ` : ''}
                        {formatBytes(locale, v.sizeBytes)}
                      </span>
                    </span>
                  </span>
                  <span class="flex shrink-0 items-center gap-1">
                    {/* Descarga por <a> (attachment del BFF), como el resto del explorador. */}
                    <a
                      href={downloadHref(item, v)}
                      download={item.name}
                      rel="noopener"
                      aria-label={t('versions.downloadAria', { version: v.version })}
                      title={t('versions.downloadTitle')}
                      class={cx(
                        'text-text-muted hover:bg-bg-subtle hover:text-text active:bg-surface-sunken sytue-focus-ring-inner rounded-control flex h-8 w-8 items-center justify-center transition-colors duration-150 pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)]',
                      )}
                    >
                      <Glyph name="download" size={16} />
                    </a>
                    {!v.isCurrent && confirming !== v.version ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={restoring !== null}
                        onClick={() => setConfirming(v.version)}
                      >
                        {t('versions.restore')}
                      </Button>
                    ) : null}
                  </span>
                </div>

                {/* Confirmación EN LA FILA (no un modal encima de otro) con la
                    consecuencia real: qué contenido pasa a estar vivo y qué NO
                    se pierde. Cada variante es una FRASE ENTERA del catálogo: en
                    alemán el verbo se va al final y trocearla la rompería. */}
                {confirming === v.version ? (
                  <div class="border-border bg-bg-subtle mt-2.5 rounded-md border p-3">
                    <p class="text-text text-sm leading-relaxed">
                      {v.createdAt
                        ? t('versions.confirmDated', {
                            name: item.name,
                            version: v.version,
                            date: versionDate(locale, v.createdAt),
                          })
                        : t('versions.confirm', { name: item.name, version: v.version })}
                    </p>
                    <p class="text-text-muted mt-1 text-sm leading-relaxed">
                      {currentVersion !== undefined
                        ? t('versions.confirmKeepCurrent', { version: currentVersion })
                        : t('versions.confirmKeep')}
                    </p>
                    <div class="mt-3 flex items-center gap-2">
                      <Button
                        size="sm"
                        loading={restoring === v.version}
                        onClick={() => void onRestore(v)}
                      >
                        {t('versions.confirmCta')}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={restoring !== null}
                        onClick={() => setConfirming(null)}
                      >
                        {t('versions.cancel')}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
          {nextCursor !== null ? (
            <div class="flex justify-center pt-3">
              <Button
                variant="ghost"
                size="sm"
                loading={loadingMore}
                onClick={() => void load(nextCursor)}
              >
                {t('versions.loadMore')}
              </Button>
            </div>
          ) : null}
        </>
      )}
    </Modal>
  );
}
