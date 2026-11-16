/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { Modal } from './Modal.js';
import { DURACION_DESHACER, MAX_AVISOS_VISIBLES, ToastProvider, useToast } from './Toast.js';

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

// --- DS-03: pausa al pasar o enfocar, tope de tres con cola y regiones vivas fijas ---

const espera = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/** Entrega al test el `notify`/`dismiss` del proveedor en el que se monta. */
function Toma({ recibir }: { recibir: (api: ReturnType<typeof useToast>) => void }) {
  const api = useToast();
  useEffect(() => recibir(api), [api, recibir]);
  return null;
}

/** Monta un proveedor y devuelve su `notify`/`dismiss` para dirigirlo desde el test. */
async function montar(): Promise<{
  api: ReturnType<typeof useToast>;
  tarjetas: () => HTMLElement[];
  desmontar: () => Promise<void>;
}> {
  const host = document.body.appendChild(document.createElement('div'));
  let api!: ReturnType<typeof useToast>;
  const recibir = (x: ReturnType<typeof useToast>) => {
    api = x;
  };
  await act(async () =>
    render(
      <ToastProvider>
        <Toma recibir={recibir} />
      </ToastProvider>,
      host,
    ),
  );
  return {
    api,
    tarjetas: () => [...document.querySelectorAll<HTMLElement>('[data-toast]')],
    desmontar: async () => {
      await act(async () => render(null, host));
      host.remove();
    },
  };
}

// VISTO EN ROJO: sin `retener`, el aviso de 80 ms se va con el puntero encima.
test('con el puntero encima el aviso no se va, y al salir se va cuando le toca', async () => {
  const { api, tarjetas, desmontar } = await montar();
  try {
    await act(async () => {
      api.notify({ tone: 'success', title: 'Guardado', duration: 80 });
    });
    const tarjeta = tarjetas()[0]!;
    await act(async () => {
      tarjeta.dispatchEvent(new PointerEvent('pointerenter'));
    });
    await act(() => espera(200));
    expect(tarjetas()).toHaveLength(1);

    await act(async () => {
      tarjeta.dispatchEvent(new PointerEvent('pointerleave'));
    });
    // Le quedaba casi todo: no se va de golpe al salir, sino cuando se le acaba.
    expect(tarjetas()).toHaveLength(1);
    await act(() => espera(200));
    expect(tarjetas()).toHaveLength(0);
  } finally {
    await desmontar();
  }
});

// Un toque en el móvil no deja el aviso parado para siempre: al levantar el dedo llega
// `pointerleave` (el `mouseleave` emulado no llegaría hasta tocar otra cosa).
test('un toque en el aviso no lo deja parado al levantar el dedo', async () => {
  const { api, tarjetas, desmontar } = await montar();
  try {
    await act(async () => {
      api.notify({ tone: 'success', title: 'Guardado', duration: 80 });
    });
    const tarjeta = tarjetas()[0]!;
    await act(async () => {
      tarjeta.dispatchEvent(new PointerEvent('pointerenter', { pointerType: 'touch' }));
      tarjeta.dispatchEvent(new MouseEvent('mouseenter'));
      tarjeta.dispatchEvent(new PointerEvent('pointerleave', { pointerType: 'touch' }));
    });
    await act(() => espera(200));
    expect(tarjetas()).toHaveLength(0);
  } finally {
    await desmontar();
  }
});

// VISTO EN ROJO: sin `onFocusIn`, tabular hasta «Deshacer» no impide que se vaya.
test('con el foco dentro el aviso no se va, y se reanuda al sacarlo', async () => {
  const { api, tarjetas, desmontar } = await montar();
  try {
    await act(async () => {
      api.notify({
        tone: 'success',
        title: 'Movido a la papelera',
        duration: 80,
        action: { label: 'Deshacer', onClick: () => {} },
      });
    });
    const deshacer = [...tarjetas()[0]!.querySelectorAll('button')].find(
      (b) => b.textContent === 'Deshacer',
    )!;
    await act(async () => deshacer.focus());
    await act(() => espera(200));
    expect(tarjetas()).toHaveLength(1);

    await act(async () => deshacer.blur());
    await act(() => espera(200));
    expect(tarjetas()).toHaveLength(0);
  } finally {
    await desmontar();
  }
});

// VISTO EN ROJO: sin el tope, los cuatro avisos se pintan a la vez.
test('el cuarto aviso espera en cola y entra cuando se va uno', async () => {
  const { api, tarjetas, desmontar } = await montar();
  const titulos = () => tarjetas().map((t) => t.querySelector('p')?.textContent);
  try {
    await act(async () => {
      for (const n of ['1', '2', '3', '4']) api.notify({ tone: 'info', title: n });
    });
    expect(MAX_AVISOS_VISIBLES).toBe(3);
    expect(titulos()).toEqual(['1', '2', '3']);

    await act(async () =>
      tarjetas()[0]!.querySelector<HTMLButtonElement>('button[aria-label]')!.click(),
    );
    expect(titulos()).toEqual(['2', '3', '4']);
  } finally {
    await desmontar();
  }
});

// El de la cola no gasta su tiempo esperando: su reloj arranca al entrar.
test('el reloj del aviso en cola no corre hasta que entra', async () => {
  const { api, tarjetas, desmontar } = await montar();
  try {
    let ids: string[] = [];
    await act(async () => {
      ids = ['1', '2', '3'].map((n) => api.notify({ tone: 'danger', title: n }));
      api.notify({ tone: 'success', title: '4', duration: 80 });
    });
    await act(() => espera(200));
    await act(async () => api.dismiss(ids[0]!));
    expect(tarjetas().map((t) => t.querySelector('p')?.textContent)).toEqual(['2', '3', '4']);
    await act(() => espera(200));
    expect(tarjetas().map((t) => t.querySelector('p')?.textContent)).toEqual(['2', '3']);
  } finally {
    await desmontar();
  }
});

// VISTO EN ROJO: con la región viva creada a la vez que el aviso, no existe antes.
test('las regiones vivas están en body antes del primer aviso y el aviso entra en ellas', async () => {
  const { api, desmontar } = await montar();
  const region = (live: string) =>
    document.body.querySelector<HTMLElement>(`:scope > [aria-live="${live}"]`);
  try {
    expect(region('polite')?.getAttribute('role')).toBe('status');
    expect(region('assertive')?.getAttribute('role')).toBe('alert');
    expect(region('polite')?.textContent).toBe('');
    expect(region('assertive')?.textContent).toBe('');

    await act(async () => {
      api.notify({ tone: 'success', title: 'Guardado', description: 'Todo al día.' });
      api.notify({ tone: 'danger', title: 'No se ha podido enviar' });
    });
    expect(region('polite')?.textContent).toBe('Guardado Todo al día.');
    expect(region('assertive')?.textContent).toBe('No se ha podido enviar');
    // La tarjeta no es otra región viva: el aviso se leería dos veces.
    expect(document.querySelectorAll('[data-toast][aria-live], [data-toast][role]')).toHaveLength(
      0,
    );
  } finally {
    await desmontar();
  }
});

// VISTO EN ROJO: si advertencia y peligro pidieran reloj, el plazo saldría aquí.
test('advertencia y peligro siguen sin irse solos, también al salir de la cola', async () => {
  const original = globalThis.setTimeout;
  const plazos: number[] = [];
  globalThis.setTimeout = ((f: () => void, ms?: number) => {
    plazos.push(ms ?? 0);
    return original(f, ms);
  }) as typeof setTimeout;
  const { api, tarjetas, desmontar } = await montar();
  try {
    let primero = '';
    await act(async () => {
      primero = api.notify({ tone: 'info', title: 'Sincronizando', duration: 0 });
      api.notify({ tone: 'warning', title: 'Sin conexión' });
      api.notify({ tone: 'danger', title: 'Error 1' });
      api.notify({ tone: 'danger', title: 'Error 2' });
    });
    await act(async () => api.dismiss(primero));
    expect(tarjetas().map((t) => t.dataset.toast)).toEqual(['warning', 'danger', 'danger']);
    expect(plazos.filter((ms) => ms >= 1000)).toEqual([]);
  } finally {
    globalThis.setTimeout = original;
    await desmontar();
  }
});

// `aria-modal` deja fuera del árbol accesible lo que hay en `body`: con un diálogo
// abierto, el aviso se anuncia en las regiones de SU hueco, que ya estaban al abrirlo.
test('con un diálogo abierto el aviso se anuncia dentro del diálogo', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  let api!: ReturnType<typeof useToast>;
  const recibir = (x: ReturnType<typeof useToast>) => {
    api = x;
  };
  try {
    await act(async () =>
      render(
        <ToastProvider>
          <Toma recibir={recibir} />
          <Modal open title="Exportar" onClose={() => {}}>
            <p>Cuerpo</p>
          </Modal>
        </ToastProvider>,
        host,
      ),
    );
    await act(() => espera(0));
    const dialogo = document.querySelector<HTMLElement>('[role="dialog"]')!;
    const enDialogo = () => dialogo.querySelector('[aria-live="assertive"]');
    expect(enDialogo()?.textContent).toBe('');

    await act(async () => {
      api.notify({ tone: 'danger', title: 'No se ha podido exportar' });
    });
    expect(enDialogo()?.textContent).toBe('No se ha podido exportar');
    expect(document.body.querySelector(':scope > [aria-live="assertive"]')?.textContent).toBe('');
  } finally {
    await act(async () => render(null, host));
    host.remove();
  }
});
