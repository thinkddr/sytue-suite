// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

/** Espera el movimiento real del cajón y dos pinturas, sin cambiar su geometría. */
export async function esperarCajones(page, { limiteMs = 2_000 } = {}) {
  let reloj;
  try {
    await Promise.race([
      page.evaluate(async () => {
        const cajones = () => [...document.querySelectorAll('aside.sytue-drawer-in')];
        const pendientes = () =>
          cajones()
            .flatMap((el) => el.getAnimations())
            .filter((a) => a.pending || a.playState !== 'finished');
        do {
          await Promise.all(pendientes().map((a) => a.finished));
          await new Promise((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(resolve)),
          );
        } while (pendientes().length > 0);
      }),
      new Promise((_, reject) => {
        reloj = setTimeout(
          () => reject(new Error(`El cajón no terminó sus animaciones en ${limiteMs} ms`)),
          limiteMs,
        );
      }),
    ]);
  } finally {
    clearTimeout(reloj);
  }
}
