/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren } from 'preact';
import { useMemo } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { prefijoDeApp } from '@sytue/config/tokens';
import { MatrixBrowserAuth } from '@sytue/offline/matrix/auth';
import { MatrixSecurityError, type SuiteMatrix } from '@sytue/offline/matrix';
import {
  MatrixSourceGate,
  type MatrixGateHost,
  type MatrixSourceGateHost,
} from '@sytue/shell/matrix';
import { PrivateMailClient } from '../client/privateMail.js';

/** Presenters belong to the captured application, never a mutable global account. */
export function PrivateMailGate({
  actor,
  tenant,
  locale,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  children: (client: PrivateMailClient) => ComponentChildren;
}) {
  const gate = useMemo(() => {
    const basePath = prefijoDeApp('mail');
    if (!basePath) throw new MatrixSecurityError('unsupported');
    const canonical = new MatrixBrowserAuth({
      storageProduct: 'mail',
      identityVersion: 2,
      deviceName: 'Sytue Mail · Web',
    });
    const historical = new MatrixBrowserAuth({
      storageProduct: 'mail',
      identityVersion: 1,
      deviceName: 'Sytue Mail · Web',
    });
    const entries = new Map<
      SuiteMatrix,
      { job: Promise<PrivateMailClient>; value?: PrivateMailClient; detached: boolean }
    >();
    const host: MatrixSourceGateHost = {
      bootstrap: canonical.refreshBootstrap,
      begin: (value) => canonical.begin(value),
      session: canonical,
      prepare: async (suite, signal, resolveLegacy) => {
        if (
          suite.bootstrap.actor !== actor ||
          (tenant !== undefined && suite.bootstrap.tenant !== tenant) ||
          suite.bootstrap.storage_product !== 'mail' ||
          suite.bootstrap.storage_identity_version !== 2
        )
          throw new MatrixSecurityError('identity');
        signal.throwIfAborted();
        let entry = entries.get(suite);
        if (!entry) {
          const job = suite
            .openMailPrivate({ basePath, resolveLegacy }, signal)
            .then((opened) => new PrivateMailClient(opened));
          entry = { job, detached: false };
          entries.set(suite, entry);
        }
        let value: PrivateMailClient;
        try {
          value = await entry.job;
        } catch (error) {
          if (entries.get(suite) === entry) entries.delete(suite);
          throw error;
        }
        if (entry.detached || entries.get(suite) !== entry || signal.aborted || suite.isClosed) {
          await value.closeAndDrain();
          throw new MatrixSecurityError('device');
        }
        entry.value = value;
      },
      detach: (suite) => {
        const entry = entries.get(suite);
        if (entry) {
          entry.detached = true;
          entry.value?.detach();
        }
      },
      close: async (suite) => {
        const entry = entries.get(suite);
        if (!entry) return;
        entries.delete(suite);
        entry.detached = true;
        entry.value?.detach();
        let value: PrivateMailClient;
        try {
          value = entry.value ?? (await entry.job);
        } catch {
          return;
        }
        await value.closeAndDrain();
      },
    };
    const legacyHost: MatrixGateHost = {
      bootstrap: historical.refreshBootstrap,
      begin: (value) => historical.begin(value),
      session: historical,
    };
    const current = (suite: SuiteMatrix): PrivateMailClient => {
      const entry = entries.get(suite);
      if (!entry?.value || entry.detached || suite.isClosed)
        throw new MatrixSecurityError('device');
      return entry.value;
    };
    return { host, legacyHost, current };
  }, [actor, tenant]);
  return (
    <MatrixSourceGate
      actor={actor}
      tenant={tenant}
      locale={locale}
      host={gate.host}
      legacyHost={gate.legacyHost}
    >
      {(suite) => children(gate.current(suite))}
    </MatrixSourceGate>
  );
}
