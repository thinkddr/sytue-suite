/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentProps } from 'preact';
import { ChatApp } from './ChatApp.js';
import { MatrixRoomsGate } from './MatrixRoomsGate.js';
export function RoomsApp(props: ComponentProps<typeof ChatApp>) {
  return (
    <MatrixRoomsGate actor={props.user.id} locale={props.locale}>
      <ChatApp {...props} />
    </MatrixRoomsGate>
  );
}
