/* SPDX-License-Identifier: Apache-2.0 */
import type { JSX } from 'preact';
import { useContext, useEffect, useMemo, useState } from 'preact/hooks';
import { createTranslate, type Locale } from '@sytue/i18n';
import { Button, Checkbox, Modal } from '@sytue/ui';
import {
  isSourceReleaseProduct,
  reauthenticationUrl,
  rememberPendingRelease,
  sourceReleaseClient,
  takePendingRelease,
  type FrozenSource,
  type SourceReleaseFetch,
} from './source-release.js';
import { MatrixStorageFixtureContext } from './storage-fixture-context.js';

export interface SourceReleaseNavigation {
  reload: () => void;
  assign: (url: string) => void;
  here: () => string;
}
const browserNavigation: SourceReleaseNavigation = {
  reload: () => window.location.reload(),
  assign: (url) => window.location.assign(url),
  here: () => window.location.pathname + window.location.search,
};

type Status = 'idle' | 'busy' | 'failed';

/**
 * La salida de quien ya no puede abrir lo que una app nativa pasó a Matrix
 * (ADR 0086, «soltar la fuente»). En la web estas apps leen y escriben por la
 * API; si la fuente del servidor se congeló, aquí se ve la copia de entonces.
 * Este aviso lo dice y ofrece volver a ella, con casilla de confirmación y
 * reautenticación. Sin fuente congelada no pinta nada.
 */
export function SourceReleaseNotice({
  actor,
  tenant,
  locale,
  product,
  basePath,
  fetchImpl,
  navigation = browserNavigation,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  product: string;
  basePath: string;
  fetchImpl?: SourceReleaseFetch;
  navigation?: SourceReleaseNavigation;
}): JSX.Element | null {
  const t = useMemo(() => createTranslate(locale), [locale]);
  // Los bancos de desarrollo (MatrixStorageFixture) no tienen servidor detrás.
  const fixture = useContext(MatrixStorageFixtureContext);
  const owner = useMemo(
    () =>
      !fixture && tenant && isSourceReleaseProduct(product) ? { actor, tenant, product } : null,
    [fixture, actor, tenant, product],
  );
  const client = useMemo(
    () =>
      owner
        ? sourceReleaseClient({ ...owner, basePath, ...(fetchImpl ? { fetchImpl } : {}) })
        : null,
    [owner, basePath, fetchImpl],
  );
  const [source, setSource] = useState<FrozenSource | null>(null);
  const [open, setOpen] = useState(false);
  const [understood, setUnderstood] = useState(false);
  const [status, setStatus] = useState<Status>('idle');

  const run = async (epoch: string, signal?: AbortSignal) => {
    if (!client || !owner) return;
    setStatus('busy');
    const outcome = await client.release(epoch, signal);
    if (signal?.aborted) return;
    switch (outcome) {
      case 'released':
        navigation.reload();
        return;
      case 'reauth':
        rememberPendingRelease(owner, epoch);
        navigation.assign(reauthenticationUrl(basePath, navigation.here()));
        return;
      case 'stale': {
        // Otra pestaña o dispositivo ya la soltó, o se volvió a congelar.
        const now = await client.frozen(signal).catch(() => undefined);
        if (now === null) {
          navigation.reload();
          return;
        }
        if (now) setSource(now);
        setStatus('failed');
        return;
      }
      case 'failed':
        setStatus('failed');
    }
  };

  useEffect(() => {
    if (!client || !owner) return;
    const controller = new AbortController();
    const pending = takePendingRelease(owner);
    void client.frozen(controller.signal).then(
      (frozen) => {
        if (controller.signal.aborted) return;
        setSource(frozen);
        // De vuelta de Sytue ID: la persona ya confirmó esta misma época.
        if (frozen && pending === frozen.epoch) void run(frozen.epoch, controller.signal);
      },
      () => {
        // Sin poder leer el modo no se ofrece nada: la app sigue como estaba.
      },
    );
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run se rehace en cada render.
  }, [client, owner]);

  if (!source) return null;
  return (
    <>
      <div
        role="status"
        class="border-warning/40 bg-warning/10 text-text flex flex-col gap-2 border-b px-4 py-2.5 text-sm sm:flex-row sm:items-center"
      >
        <p class="flex-1">{t('vault.releaseNotice')}</p>
        <Button
          variant="secondary"
          disabled={status === 'busy'}
          onClick={() => {
            setUnderstood(false);
            setOpen(true);
          }}
        >
          {t('vault.release')}
        </Button>
      </div>
      {status === 'failed' && !open ? (
        <p class="text-danger-text mx-auto w-full max-w-3xl px-3 text-sm" role="alert">
          {t('vault.releaseFailed')}
        </p>
      ) : null}
      <Modal open={open} onClose={() => setOpen(false)} title={t('vault.releaseTitle')}>
        <div class="flex flex-col gap-4">
          <p class="text-text-muted text-sm">{t('vault.releaseBody')}</p>
          <p class="text-text-muted text-sm">{t('vault.releaseSignIn')}</p>
          <Checkbox
            checked={understood}
            onChange={(event) => setUnderstood(event.currentTarget.checked)}
            label={t('vault.releaseUnderstood')}
          />
          {status === 'failed' ? (
            <p class="text-danger-text text-sm" role="alert">
              {t('vault.releaseFailed')}
            </p>
          ) : null}
          <Button
            variant="danger"
            disabled={!understood || status === 'busy'}
            onClick={() => {
              if (!understood) return;
              void run(source.epoch);
            }}
          >
            {t('vault.release')}
          </Button>
        </div>
      </Modal>
    </>
  );
}
