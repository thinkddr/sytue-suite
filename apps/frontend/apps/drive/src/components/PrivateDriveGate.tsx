/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentChildren } from 'preact';
import { useMemo } from 'preact/hooks';
import { DriveClient } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import { PqVaultGate } from '@sytue/shell/matrix';
import { withBase } from '@sytue/ui';
import { createPrivateDriveGate, type PrivateDriveSession } from '../client/private-drive.js';

function browserBaseURL(): string {
  return window.location.origin + withBase(import.meta.env.BASE_URL, '/').replace(/\/$/, '');
}

export function PrivateDriveGate({
  actor,
  tenant,
  locale,
  children,
}: {
  actor: string;
  tenant?: string | undefined;
  locale: Locale;
  children: (session: PrivateDriveSession, baseURL: string) => ComponentChildren;
}) {
  const gate = useMemo(
    () =>
      createPrivateDriveGate({
        actor,
        tenant,
        source: () =>
          new DriveClient({
            baseUrl: browserBaseURL(),
          }),
      }),
    [actor, tenant],
  );
  return (
    <PqVaultGate
      actor={actor}
      tenant={tenant}
      locale={locale}
      product="drive"
      basePath={import.meta.env.BASE_URL}
      prepare={gate.host.prepare}
      activate={gate.host.activate}
      close={gate.host.close}
    >
      {(vault) => children(gate.current(vault), browserBaseURL())}
    </PqVaultGate>
  );
}
