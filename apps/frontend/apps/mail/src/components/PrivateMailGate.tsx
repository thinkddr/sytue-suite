/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import { prefijoDeApp } from '@sytue/config/tokens';
import { MatrixBrowserAuth } from '@sytue/offline/matrix/auth';
import { MatrixSecurityError, type SuiteMatrix } from '@sytue/offline/matrix';
import {
  PqVaultGate,
  MatrixSourceGate,
  type MatrixGateHost,
  type MatrixSourceGateHost,
} from '@sytue/shell/matrix';
import { PqVault, type PqVaultOpenOptions } from '@sytue/offline/vault';
import { PrivateMailClient } from '../client/privateMail.js';
import { createVaultMailGate } from '../client/privateMailVault.js';
import {
  almacenDePestana,
  destinoCorreo,
  elegirAlmacenCorreo,
  olvidarTraspaso,
  pedirTraspaso,
  quiereSeguirConMatrix,
  seguirConMatrix,
  traspasoPedido,
  type DestinoCorreo,
} from '../client/almacenCorreo.js';
import { PasarABoveda, TraspasoBoveda } from './TraspasoBoveda.js';

interface GateProps {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  children: (client: PrivateMailClient) => ComponentChildren;
}

/**
 * La puerta del correo privado. Con `vault` (el conmutador de despliegue
 * `SYTUE_MAIL_BOVEDA`, ADR 0087), quien no tiene correo en Matrix lo abre en la
 * bóveda; a quien lo tiene en Matrix se le ofrece pasarlo a la bóveda (vaciándolo)
 * o seguir por ahora con Matrix. Sin el conmutador, siempre Matrix.
 */
export function PrivateMailGate(props: GateProps & { vault?: boolean | undefined }) {
  const [almacen, setAlmacen] = useState<DestinoCorreo | null>(props.vault ? null : 'matrix');
  useEffect(() => {
    if (!props.vault) {
      setAlmacen('matrix');
      return;
    }
    const controller = new AbortController();
    setAlmacen(null);
    void elegirAlmacenCorreo({
      basePath: prefijoDeApp('mail') ?? '',
      actor: props.actor,
      tenant: props.tenant,
      signal: controller.signal,
    })
      .then((elegido) => {
        if (controller.signal.aborted) return;
        const pestana = almacenDePestana();
        const destino = destinoCorreo(
          elegido,
          traspasoPedido(pestana, props.actor, props.tenant),
          quiereSeguirConMatrix(pestana, props.actor, props.tenant),
        );
        if (destino === 'boveda') olvidarTraspaso(pestana, props.actor, props.tenant);
        setAlmacen(destino);
      })
      .catch(() => {});
    return () => controller.abort();
  }, [props.vault, props.actor, props.tenant]);
  if (almacen === 'boveda') return <VaultMailGate {...props} />;
  if (almacen === 'traspaso') return <VaultMailGate {...props} traspaso />;
  if (almacen === 'elegir')
    return (
      <PasarABoveda
        locale={props.locale}
        onConfirm={() => {
          pedirTraspaso(almacenDePestana(), props.actor, props.tenant);
          setAlmacen('traspaso');
        }}
        onKeep={() => {
          seguirConMatrix(almacenDePestana(), props.actor, props.tenant);
          setAlmacen('matrix');
        }}
      />
    );
  if (almacen === 'matrix')
    return (
      <MatrixMailGate
        {...props}
        onTraspaso={
          props.vault
            ? () => {
                pedirTraspaso(almacenDePestana(), props.actor, props.tenant);
                setAlmacen('traspaso');
              }
            : undefined
        }
      />
    );
  return null;
}

/**
 * La puerta de la bóveda. Con `traspaso` (ADR 0087) abre una bóveda NUEVA y
 * registra su clave con la marca de traspaso; en cuanto el correo abre, la marca
 * de la pestaña se olvida y las siguientes visitas ya eligen la bóveda.
 */
function VaultMailGate({
  actor,
  tenant,
  locale,
  children,
  traspaso,
}: GateProps & { traspaso?: boolean | undefined }) {
  const gate = useMemo(() => {
    const basePath = prefijoDeApp('mail');
    if (!basePath) throw new MatrixSecurityError('unsupported');
    return createVaultMailGate({ actor, tenant, basePath });
  }, [actor, tenant]);
  const nueva = useMemo(
    () =>
      traspaso
        ? (options: PqVaultOpenOptions) => PqVault.reset({ ...options, handover: true })
        : undefined,
    [traspaso],
  );
  const prepare = useMemo(
    () =>
      traspaso
        ? async (vault: PqVault, signal: AbortSignal) => {
            await gate.host.prepare(vault, signal);
            olvidarTraspaso(almacenDePestana(), actor, tenant);
          }
        : gate.host.prepare,
    [gate, traspaso, actor, tenant],
  );
  return (
    <PqVaultGate
      actor={actor}
      tenant={tenant}
      locale={locale}
      product="mail"
      prepare={prepare}
      detach={gate.host.detach}
      close={gate.host.close}
      {...(nueva ? { openVault: nueva, resetVault: nueva } : {})}
    >
      {(vault) => children(gate.current(vault))}
    </PqVaultGate>
  );
}

/** Presenters belong to the captured application, never a mutable global account. */
function MatrixMailGate({
  actor,
  tenant,
  locale,
  children,
  onTraspaso,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  children: (client: PrivateMailClient) => ComponentChildren;
  /** Solo con el conmutador de la bóveda: la salida para quien no puede abrir. */
  onTraspaso?: (() => void) | undefined;
}) {
  // La salida se ofrece mientras el correo de Matrix no está abierto: a quien lo
  // abre no se le pone delante un «vaciar».
  const [abierto, setAbierto] = useState(false);
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
        setAbierto(true);
      },
      detach: (suite) => {
        const entry = entries.get(suite);
        if (entry) {
          entry.detached = true;
          entry.value?.detach();
        }
        setAbierto(false);
      },
      close: async (suite) => {
        const entry = entries.get(suite);
        setAbierto(false);
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
    <>
      <MatrixSourceGate
        actor={actor}
        tenant={tenant}
        locale={locale}
        host={gate.host}
        legacyHost={gate.legacyHost}
      >
        {(suite) => children(gate.current(suite))}
      </MatrixSourceGate>
      {onTraspaso && !abierto ? <TraspasoBoveda locale={locale} onConfirm={onTraspaso} /> : null}
    </>
  );
}
