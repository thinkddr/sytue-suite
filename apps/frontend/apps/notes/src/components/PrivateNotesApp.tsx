/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentProps, JSX } from 'preact';
import { MatrixStorageGate } from '@sytue/shell/matrix';
import { NotesApp } from './NotesApp.js';

export function PrivateNotesApp(props: ComponentProps<typeof NotesApp>): JSX.Element {
  return (
    <MatrixStorageGate
      actor={props.user.id}
      tenant={props.user.tenant}
      locale={props.locale}
      product="notes"
      deviceName="Sytue Notepads · Web"
    >
      <NotesApp {...props} />
    </MatrixStorageGate>
  );
}
