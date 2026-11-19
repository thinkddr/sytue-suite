/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren, JSX } from 'preact';
import { useContext, useEffect, useMemo, useState } from 'preact/hooks';
import type { MatrixStorageProduct } from '@sytue/config/matrix';
import { prefijoDeApp } from '@sytue/config/tokens';
import type { Locale } from '@sytue/i18n';
import { MatrixBrowserAuth } from '@sytue/offline/matrix/auth';
import {
  CifradoDesactivado,
  MatrixSecurityError,
  SuiteMatrix,
  type MatrixBootstrap,
} from '@sytue/offline/matrix';
import { MatrixDeviceGate, type MatrixGateHost } from './MatrixDeviceGate.js';
import { PqVaultGate } from './PqVaultGate.js';
import { MatrixStorageFixtureContext } from './storage-fixture-context.js';

export type MatrixLegacyResolver = (signal: AbortSignal) => Promise<SuiteMatrix>;
export interface MatrixSourceGateHost extends Omit<MatrixGateHost, 'prepare'> {
  /** The domain factory decides whether a source is required using its Go plan. */
  prepare: (
    client: SuiteMatrix,
    signal: AbortSignal,
    resolveLegacy: MatrixLegacyResolver,
  ) => Promise<void>;
}

/** One recovery flow for a canonical identity and its retained source. Both gates
 * keep their original SDK stores, SSO, trust UI and draining lifecycle. */
export function MatrixSourceGate({
  actor,
  tenant,
  locale,
  host,
  legacyHost,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  host: MatrixSourceGateHost;
  legacyHost: MatrixGateHost;
  children: ComponentChildren | ((client: SuiteMatrix) => ComponentChildren);
}): JSX.Element {
  const [requested, setRequested] = useState(false);
  const [showHistorical, setShowHistorical] = useState(false);
  const [sourceGeneration, setSourceGeneration] = useState(0);
  const pair = useMemo(() => {
    let canonical: SuiteMatrix | undefined,
      historical: SuiteMatrix | undefined,
      historicalReady = false,
      retired = false,
      disposed = false,
      generation = 0;
    const waiting = new Set<{
      resolve: (client: SuiteMatrix) => void;
      reject: (error: Error) => void;
      clear: () => void;
    }>();
    const reject = () => new MatrixSecurityError('device');
    const bound = (value: MatrixBootstrap, version: 1 | 2): MatrixBootstrap => {
      const actual =
        value.storage_identity_version === undefined || value.storage_identity_version === 0
          ? 1
          : value.storage_identity_version;
      if (
        actual !== version ||
        value.actor !== actor ||
        (tenant !== undefined && value.tenant !== tenant) ||
        !value.storage_product
      )
        throw new MatrixSecurityError('identity');
      return value;
    };
    const matching = (value: SuiteMatrix) => {
      if (retired || !canonical || canonical.isClosed || value.isClosed) throw reject();
      const own = bound(canonical.bootstrap, 2),
        old = bound(value.bootstrap, 1);
      if (
        old.actor !== own.actor ||
        old.tenant !== own.tenant ||
        old.storage_product !== own.storage_product ||
        old.homeserver_url !== own.homeserver_url
      )
        throw new MatrixSecurityError('identity');
    };
    const retire = (source?: SuiteMatrix) => {
      if (retired || (source !== undefined && source !== canonical && source !== historical))
        return;
      retired = true;
      for (const item of waiting) {
        item.clear();
        item.reject(reject());
      }
      waiting.clear();
      // Close is synchronous cancellation. SDK and domain jobs retain their keys
      // until their existing closeAndDrain barriers finish; no store is forgotten.
      if (canonical && canonical !== source && !canonical.isClosed) canonical.close();
      if (historical && historical !== source && !historical.isClosed) historical.close();
    };
    const resolveLegacy: MatrixLegacyResolver = async (signal) => {
      signal.throwIfAborted();
      if (retired || !canonical || canonical.isClosed) throw reject();
      if (historical && historicalReady) {
        matching(historical);
        return historical;
      }
      setRequested(true);
      setShowHistorical(true);
      return new Promise<SuiteMatrix>((resolve, rejectPending) => {
        const abort = () => {
          waiting.delete(item);
          item.clear();
          rejectPending(reject());
        };
        const item = {
          resolve,
          reject: rejectPending,
          clear: () => signal.removeEventListener('abort', abort),
        };
        waiting.add(item);
        signal.addEventListener('abort', abort, { once: true });
        if (signal.aborted) abort();
      });
    };
    const canonicalHost: MatrixGateHost = {
      ...host,
      bootstrap: async (signal) => {
        if (disposed) throw reject();
        signal?.throwIfAborted();
        const value = bound(await host.bootstrap(signal), 2);
        signal?.throwIfAborted();
        if (disposed) throw reject();
        canonical = undefined;
        historical = undefined;
        historicalReady = false;
        retired = false;
        setRequested(false);
        setShowHistorical(false);
        setSourceGeneration(++generation);
        return value;
      },
      prepare: async (client, signal) => {
        if (retired) throw reject();
        canonical = client;
        await host.prepare(client, signal, resolveLegacy);
        signal.throwIfAborted();
        if (retired || canonical !== client || client.isClosed) throw reject();
      },
      detach: (client) => {
        retire(client);
        host.detach?.(client);
      },
      close: async (client) => {
        const old = historical;
        try {
          await host.close?.(client);
        } finally {
          if (old) await old.closeAndDrain();
        }
      },
    };
    const historicalHost: MatrixGateHost = {
      ...legacyHost,
      bootstrap: async (signal) => {
        const value = bound(await legacyHost.bootstrap(signal), 1);
        const own = canonical?.bootstrap;
        if (
          retired ||
          !own ||
          canonical?.isClosed ||
          value.actor !== own.actor ||
          value.tenant !== own.tenant ||
          value.storage_product !== own.storage_product ||
          value.homeserver_url !== own.homeserver_url
        )
          throw new MatrixSecurityError('identity');
        return value;
      },
      open: async (bootstrap, ui, session) => {
        const instance = legacyHost.open
          ? await legacyHost.open(bootstrap, ui, session)
          : await SuiteMatrix.open(bootstrap, ui, session);
        try {
          matching(instance);
          historical = instance;
          return instance;
        } catch (error) {
          await instance.closeAndDrain();
          throw error;
        }
      },
      activate: (client) => {
        matching(client);
        legacyHost.activate?.(client);
        historical = client;
        historicalReady = true;
        for (const item of waiting) {
          item.clear();
          item.resolve(client);
        }
        waiting.clear();
        setShowHistorical(false);
      },
      detach: (client) => {
        retire(client);
        legacyHost.detach?.(client);
      },
    };
    return {
      canonicalHost,
      historicalHost,
      dispose: () => {
        disposed = true;
        retire();
      },
    };
  }, [actor, tenant, host, legacyHost]);
  useEffect(() => {
    setRequested(false);
    setShowHistorical(false);
    return () => pair.dispose();
  }, [pair]);
  return (
    <>
      <div hidden={showHistorical}>
        <MatrixDeviceGate actor={actor} tenant={tenant} locale={locale} host={pair.canonicalHost}>
          {children}
        </MatrixDeviceGate>
      </div>
      {requested && (
        <div hidden={!showHistorical}>
          <MatrixDeviceGate
            key={sourceGeneration}
            actor={actor}
            tenant={tenant}
            locale={locale}
            host={pair.historicalHost}
          >
            {null}
          </MatrixDeviceGate>
        </div>
      )}
    </>
  );
}

/**
 * Puerta compartida de almacenamiento cifrado para los productos de la suite.
 *
 * ADR 0086: las pantallas de producto usan la bóveda post-cuántica directa
 * (`PqVaultGate`, `age-MLKEM768-X25519-v1`) sin Synapse/Matrix. Solo se conserva
 * la vía Matrix si un llamador explícito pide `(client: SuiteMatrix)`.
 */
export function MatrixStorageGate({
  actor,
  tenant,
  locale,
  product,
  basePath,
  deviceName,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  product: MatrixStorageProduct;
  basePath?: string | undefined;
  deviceName?: string | undefined;
  children: ComponentChildren | ((client: SuiteMatrix) => ComponentChildren);
}): JSX.Element {
  if (typeof children !== 'function') {
    return (
      <PqVaultGate
        actor={actor}
        tenant={tenant}
        locale={locale}
        product={product}
        basePath={basePath}
      >
        {children}
      </PqVaultGate>
    );
  }
  return (
    <MatrixStorageFunctionGate
      actor={actor}
      tenant={tenant}
      locale={locale}
      product={product}
      basePath={basePath}
      deviceName={deviceName}
    >
      {children}
    </MatrixStorageFunctionGate>
  );
}

function MatrixStorageFunctionGate({
  actor,
  tenant,
  locale,
  product,
  basePath,
  deviceName,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  product: MatrixStorageProduct;
  basePath?: string | undefined;
  deviceName?: string | undefined;
  children: ComponentChildren | ((client: SuiteMatrix) => ComponentChildren);
}): JSX.Element {
  // Development benches only (MatrixStorageFixture): a synthetic homeserver half.
  const fixture = useContext(MatrixStorageFixtureContext);
  const host = useMemo(() => {
    const resolvedBasePath = basePath ?? prefijoDeApp(product === 'hub' ? 'one' : product) ?? '/';
    const label = deviceName ?? `Sytue ${product} · Web`;
    const canonical = new MatrixBrowserAuth({
      storageProduct: product,
      identityVersion: 2,
      deviceName: label,
    });
    const synthetic = fixture?.({ actor, tenant, product });
    const deviceHost: MatrixGateHost = {
      bootstrap: canonical.refreshBootstrap,
      begin: (value) => canonical.begin(value),
      session: canonical,
      ...synthetic,
      prepare: async (suite, signal) => {
        if (
          suite.bootstrap.actor !== actor ||
          (tenant !== undefined && suite.bootstrap.tenant !== tenant) ||
          suite.bootstrap.storage_product !== product ||
          suite.bootstrap.storage_identity_version !== 2
        )
          throw new MatrixSecurityError('identity');
        await suite.prepareDeviceStorage({ basePath: resolvedBasePath }, signal);
      },
    };
    return deviceHost;
  }, [actor, tenant, product, basePath, deviceName, fixture]);
  const [cifrado, setCifrado] = useState<'comprobando' | 'activado' | 'desactivado'>(
    fixture ? 'activado' : 'comprobando',
  );
  useEffect(() => {
    if (fixture) return;
    let vigente = true;
    host.bootstrap().then(
      () => vigente && setCifrado('activado'),
      (error: unknown) =>
        vigente && setCifrado(error instanceof CifradoDesactivado ? 'desactivado' : 'activado'),
    );
    return () => {
      vigente = false;
    };
  }, [host, fixture]);
  if (cifrado === 'comprobando') return <></>;
  return (
    <MatrixDeviceGate actor={actor} tenant={tenant} locale={locale} host={host}>
      {children}
    </MatrixDeviceGate>
  );
}
