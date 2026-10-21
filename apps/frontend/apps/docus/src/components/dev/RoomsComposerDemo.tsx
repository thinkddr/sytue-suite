/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El compositor REAL de Rooms en el banco DEV existente, sin sesión ni backend.
 * Los laterales reproducen el canal de 412px de una ventana de 1024px.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Composer } from '../../../../rooms/src/components/Composer';
import { roomsT } from '../../../../rooms/src/i18n';

export function RoomsComposerDemo({
  paneles,
  hilo,
}: {
  paneles: boolean;
  hilo: boolean;
}): JSX.Element {
  const [enviado, setEnviado] = useState('');
  return (
    <main class="flex h-dvh min-w-0">
      {paneles ? <aside aria-label="Sidebar" class="w-[260px] shrink-0" /> : null}
      <section data-demo-composer class="flex min-w-0 flex-1 flex-col">
        <div class="min-h-0 flex-1" />
        <p data-demo-result aria-live="polite" class="sr-only">
          {enviado}
        </p>
        <Composer
          channelName="general"
          channelId="fixture"
          advanced={!hilo}
          ariaLabel="Message"
          locale="en"
          t={roomsT('en')}
          onSend={(body) => setEnviado(body)}
        />
      </section>
      {paneles ? <aside aria-label="People" class="w-[352px] shrink-0" /> : null}
    </main>
  );
}
