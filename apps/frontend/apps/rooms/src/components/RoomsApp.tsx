/* SPDX-License-Identifier: Apache-2.0 */
import type { ComponentProps } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import { ChatApp } from './ChatApp.js';
import { MatrixRoomsGate } from './MatrixRoomsGate.js';
export function RoomsApp(props: ComponentProps<typeof ChatApp>) {
  return (
    <I18nProvider locale={props.locale}>
      <ToastProvider key={JSON.stringify([props.user.id, props.user.tenant])}>
        <MatrixRoomsGate actor={props.user.id} locale={props.locale}>
          <ChatApp {...props} />
        </MatrixRoomsGate>
      </ToastProvider>
    </I18nProvider>
  );
}
