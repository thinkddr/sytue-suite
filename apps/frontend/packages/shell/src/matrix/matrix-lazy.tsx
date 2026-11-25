/* SPDX-License-Identifier: Apache-2.0 */
/**
 * La rama de Matrix de `MatrixSourceGate.tsx`, bajada al usarla.
 *
 * Las apps de la bóveda (ADR 0086) importan `MatrixStorageGate`, que vive junto a la
 * puerta de Matrix. Con `MatrixDeviceGate` y `SuiteMatrix` importados de forma
 * estática, cada app bajaba matrix-js-sdk (~1 MB, ~211 KB en brotli) en su JS inicial
 * aunque nunca abriera Matrix. Aquí la puerta de dispositivos es la misma, con las
 * mismas props, pero su módulo (`matrix-branch.ts`) solo se pide cuando la rama de
 * Matrix se pinta de verdad.
 *
 * Mientras baja se pinta lo mismo que la puerta antes de su bootstrap (el `<main>`
 * vacío), también en el servidor. Si el trozo no llega, el mismo aviso y el mismo
 * «Reintentar» que la puerta enseña cuando falla.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { createTranslate } from '@sytue/i18n';
import { Button } from '@sytue/ui';
import type { MatrixDeviceGate as Gate } from './MatrixDeviceGate.js';
import type * as Branch from './matrix-branch.js';

type MatrixBranch = typeof Branch;
type GateProps = Parameters<typeof Gate>[0];
type Open = MatrixBranch['SuiteMatrix']['open'];

let branch: MatrixBranch | undefined;
let loading: Promise<MatrixBranch> | undefined;

/** Baja la rama una sola vez; si falla, el siguiente intento la vuelve a pedir. */
export function loadMatrixBranch(): Promise<MatrixBranch> {
  loading ??= import('./matrix-branch.js').then(
    (module) => (branch = module),
    (error: unknown) => {
      loading = undefined;
      throw error;
    },
  );
  return loading;
}

/** `SuiteMatrix.open`, con el SDK bajado antes. */
export async function openSuiteMatrix(...args: Parameters<Open>): ReturnType<Open> {
  const { SuiteMatrix } = await loadMatrixBranch();
  return SuiteMatrix.open(...args);
}

/** `MatrixDeviceGate` con las mismas props; el módulo baja al montarse. */
export function MatrixDeviceGate(props: GateProps): JSX.Element {
  const [loaded, setLoaded] = useState<MatrixBranch | undefined>(branch);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (loaded) return;
    let current = true;
    setFailed(false);
    loadMatrixBranch().then(
      (module) => current && setLoaded(module),
      () => current && setFailed(true),
    );
    return () => {
      current = false;
    };
  }, [loaded, attempt]);
  if (loaded) return <loaded.MatrixDeviceGate {...props} />;
  const t = props.host.translate ?? createTranslate(props.locale);
  return (
    <main class="mx-auto flex w-full max-w-xl flex-col p-6">
      {failed ? (
        <div class="flex flex-col gap-4">
          <p class="text-danger-text text-sm" role="alert">
            {t('matrix.failed')}
          </p>
          <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
            {t('action.retry')}
          </Button>
        </div>
      ) : null}
    </main>
  );
}
