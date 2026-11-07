/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { DURACION_DESHACER, ToastProvider, useToast } from './Toast.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

// El aviso con «Deshacer» es UNO para toda la suite (antes erp, drive y contactos tenían
// cada uno su copia): se retira a los 10 s, el doble que uno de serie. Espejo de
// `TestElAvisoConDeshacerDuraDiezSegundos` en `apps/fyne/ui`.
//
// VISTO EN ROJO: con DURACION_DESHACER a 5000, el aviso con «Deshacer» se va a la vez
// que el de serie.
test('el aviso con «Deshacer» se retira a los diez segundos, el de serie a los cinco', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  // Al montarse avisa dos veces: uno de serie y uno con «Deshacer».
  function Avisa() {
    const { notify } = useToast();
    useEffect(() => {
      notify({ tone: 'success', title: 'Guardado' });
      notify({
        tone: 'success',
        title: 'Movido a la papelera',
        duration: DURACION_DESHACER,
        action: { label: 'Deshacer', onClick: () => {} },
      });
    }, [notify]);
    return null;
  }
  const original = globalThis.setTimeout;
  const plazos: number[] = [];
  // Se apuntan los plazos y no se dispara ninguno: lo que se mira es cuándo se retiraría.
  globalThis.setTimeout = ((_f: () => void, ms?: number) => {
    plazos.push(ms ?? 0);
    return original(() => {}, 0);
  }) as typeof setTimeout;
  try {
    await act(async () =>
      render(
        <ToastProvider>
          <Avisa />
        </ToastProvider>,
        host,
      ),
    );
    expect(plazos.filter((ms) => ms >= 1000)).toEqual([5000, 10_000]);
  } finally {
    globalThis.setTimeout = original;
    await act(async () => render(null, host));
    host.remove();
  }
});
