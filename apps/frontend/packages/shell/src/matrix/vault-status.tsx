/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El estado de la bóveda en la barra de la suite (ADR 0086).
 *
 * La puerta (`PqVaultGate`) ya no pinta su candado fijo abajo a la derecha, que
 * tapaba «Nuevo contacto» en el teléfono, la última celda del mes de Calendar y la
 * papelera de una tarjeta de Docs. Lo publica aquí y la barra de `SytueShell` lo
 * enseña junto a la campana: abriendo (un indicador de progreso), abierta (el
 * candado) o pidiendo atención (el candado con su punto). Al pulsarlo se abren los
 * detalles de la puerta, con sus salidas (volver a entrar, vaciar, reintentar).
 *
 * La puerta y la barra se encuentran en los dos sentidos, porque unas apps montan la
 * puerta por fuera de la shell (Calendar, Notes…) y otras por dentro (Drive, Docs):
 *
 * - Por fuera: la puerta da `VaultStatusContext` y la barra lo lee.
 * - Por dentro: la barra da `VaultStatusSinkContext` y la puerta le pasa su estado.
 *
 * En los dos casos la barra lo `reclama`; la puerta solo pinta su candado flotante
 * de reserva si nadie lo ha reclamado (la shell nativa, sin barra, o una prueba).
 */
import { createContext, type JSX } from 'preact';
import { useContext, useLayoutEffect } from 'preact/hooks';
import { Icon, Spinner, TopBarButton } from '@sytue/ui';
import { useT } from '@sytue/i18n/preact';

/** `abriendo`: el núcleo y el puente trabajan; `atencion`: no abrió y hay algo que hacer. */
export type VaultIndicatorState = 'abriendo' | 'abierta' | 'atencion';

export interface VaultStatus {
  readonly estado: VaultIndicatorState;
  /** Abre los detalles de la puerta (su diálogo). */
  readonly abrirDetalles: () => void;
  /** La barra que lo enseña lo reclama mientras está montada; devuelve cómo soltarlo. */
  readonly reclamar: () => () => void;
}

/** De la puerta hacia abajo: la shell que va dentro de la puerta lo lee. */
export const VaultStatusContext = createContext<VaultStatus | null>(null);

/** De la shell hacia abajo: la puerta que va dentro de la shell le pasa su estado. */
export const VaultStatusSinkContext = createContext<((status: VaultStatus | null) => void) | null>(
  null,
);

const etiqueta = {
  abriendo: 'matrix.unlocking',
  abierta: 'matrix.title',
  atencion: 'vault.attention',
} as const;

/**
 * El candado de la barra. `compacto` es el de la barra plegada del teléfono: solo
 * sale mientras abre o si pide atención; abierta, el candado vive con las demás
 * acciones de la suite, tras la entrada.
 */
export function VaultStatusButton({
  status,
  compacto = false,
}: {
  status: VaultStatus | null;
  compacto?: boolean;
}): JSX.Element | null {
  const t = useT();
  const reclamar = status?.reclamar;
  useLayoutEffect(() => (reclamar ? reclamar() : undefined), [reclamar]);
  if (!status || (compacto && status.estado === 'abierta')) return null;
  return (
    <TopBarButton
      label={t(etiqueta[status.estado])}
      indicator={status.estado === 'atencion'}
      aria-busy={status.estado === 'abriendo' ? 'true' : undefined}
      data-sytue-boveda-estado={status.estado}
      onClick={status.abrirDetalles}
    >
      {status.estado === 'abriendo' ? <Spinner size={16} /> : <Icon name="lock" size={18} />}
    </TopBarButton>
  );
}

/** El estado que la barra tiene que enseñar: el de la puerta de fuera o el de dentro. */
export function useVaultStatusFromAbove(): VaultStatus | null {
  return useContext(VaultStatusContext);
}
