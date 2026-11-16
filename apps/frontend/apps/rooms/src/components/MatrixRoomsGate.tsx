/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren } from 'preact';
import { useMemo } from 'preact/hooks';
import { MatrixDeviceGate, type MatrixGateHost } from '@sytue/shell/matrix';
import type { Locale } from '@sytue/i18n';
import { clearDraftMemory } from '../lib/drafts.js';
import { clearMatrixMessages } from '../client/matrix/messages.js';
import {
  matrixBootstrap,
  beginMatrixSso,
  roomsMatrixSessionAdapter,
} from '../client/matrix/auth.js';
import { setRoomsMatrix } from '../client/matrix/client.js';
export function MatrixRoomsGate({
  actor,
  tenant,
  locale,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  children: ComponentChildren;
}) {
  const host = useMemo<MatrixGateHost>(
    () => ({
      bootstrap: matrixBootstrap,
      begin: beginMatrixSso,
      session: roomsMatrixSessionAdapter,
      allowPeers: true,
      activate: (client) => {
        setRoomsMatrix(client);
        window.dispatchEvent(new Event('sytue-matrix-verified'));
        // Resume authenticated historical session migration on this authorized device.
        void client.backfillRoomSessionProvenance().catch(() => undefined);
      },
      detach: () => {
        setRoomsMatrix(null);
        clearMatrixMessages();
        clearDraftMemory();
      },
    }),
    [],
  );
  return (
    <MatrixDeviceGate actor={actor} tenant={tenant} locale={locale} host={host}>
      {children}
    </MatrixDeviceGate>
  );
}
