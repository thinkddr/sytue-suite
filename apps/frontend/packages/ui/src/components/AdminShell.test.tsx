/**
 * Smoke test de AdminShell: el toggle abre el drawer (aria-expanded, foco
 * dentro de la nav) y Esc lo cierra devolviendo el foco al toggle.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { AdminShell } from './AdminShell.js';
import { AppShell } from './AppShell.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

// La barra estuvo DENTRO de la hoja: quitar su filete dejaba aún un paño blanco que
// cortaba el marco. Ahora la hoja empieza después de ella, sin heredar su tinta de cromo.
for (const [nombre, carcasa] of [
  [
    'AppShell',
    <AppShell
      key="AppShell"
      topbar={<header>Suite</header>}
      sidebar={<nav>Menú</nav>}
      secondaryPanel={<p>Lista</p>}
      title="Correo"
    >
      <p>Detalle</p>
    </AppShell>,
  ],
  [
    'AdminShell',
    <AdminShell key="AdminShell" sidebar={<nav>Menú</nav>} title="Consola">
      <p>Contenido</p>
    </AdminShell>,
  ],
] as const) {
  test(`${nombre}: la barra móvil queda fuera de la hoja y de su contenido`, () => {
    const host = document.createElement('div');
    document.body.appendChild(host);
    try {
      render(carcasa, host);
      const barra = host.querySelector('.sytue-mobilebar')!;
      const hoja = host.querySelector('.sytue-sheet')!;
      const contenido = host.querySelector('#contenido')!;
      expect(barra.closest('.sytue-sheet')).toBeNull();
      expect(barra.nextElementSibling).toBe(hoja);
      expect(hoja.contains(contenido)).toBe(true);
      expect(contenido.closest('.sytue-mobilebar')).toBeNull();
      expect(barra.querySelector('button[aria-controls]')).not.toBeNull();
      // En teléfono panel y detalle ocupan por turnos la hoja completa: ninguno
      // puede borrar la esquina superior izquierda a ese ancho.
      expect(contenido.className).not.toContain('max-md:[--sytue-sheet-tl:0px]');
    } finally {
      render(null, host);
      host.remove();
    }
  });
}

test('AdminShell: toggle abre el drawer y Esc lo cierra con retorno de foco', async () => {
  const host = document.createElement('div');
  document.body.appendChild(host);

  render(
    <AdminShell
      sidebar={
        <nav aria-label="Navegación principal">
          <a href="/tenants">Organizaciones</a>
        </nav>
      }
      title="Consola"
    >
      <p>contenido</p>
    </AdminShell>,
    host,
  );
  // Preact difiere los efectos vía requestAnimationFrame (~16 ms en happy-dom),
  // así que hay que cederle el turno. Pero NO con una espera fija: con la máquina
  // cargada —el CI corre todo el workspace en paralelo— ese plazo se agota antes
  // de que el efecto llegue a correr, y el test falla sin que nada esté roto.
  // Se espera A QUE PASE lo que tiene que pasar, con un límite generoso.
  //
  // 30 s y no 5: esto sale EN CUANTO se cumple la condición, así que el límite no
  // cuesta nada en el caso bueno y solo alarga el que ya está roto. Con 5 s el
  // rojo lo ponía la máquina y no el componente. Y al agotarse hay que DECIRLO:
  // volviendo en silencio, la que fallaba era la aserción de después, que informa
  // de un DOM que no cambió sin distinguir «no ocurrió» de «no ha ocurrido AÚN».
  const hasta = async (cond: () => boolean, que: string, limite = 30_000): Promise<void> => {
    const fin = Date.now() + limite;
    while (!cond() && Date.now() < fin) await new Promise((r) => setTimeout(r, 10));
    if (!cond()) throw new Error(`no se cumplió en ${limite} ms: ${que}`);
  };

  await hasta(() => host.querySelector('button[aria-controls]') !== null, 'el toggle se pinta');
  const toggle = host.querySelector<HTMLButtonElement>('button[aria-controls]')!;
  const drawer = document.getElementById(toggle.getAttribute('aria-controls')!)!;
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
  // Cerrado: el aside lleva hidden (clase max-md:hidden) y no hay overlay.
  expect(drawer.className).toContain('max-md:hidden');

  toggle.click();
  await hasta(() => toggle.getAttribute('aria-expanded') === 'true', 'el toggle abre');
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  expect(drawer.className).not.toContain('max-md:hidden');
  // Foco dentro del drawer (primer enlace de la nav). El foco lo mueve otro
  // efecto, que puede llegar después de que `aria-expanded` ya sea true.
  await hasta(() => drawer.contains(document.activeElement), 'el foco entra en el drawer');
  expect(drawer.contains(document.activeElement)).toBe(true);

  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await hasta(() => toggle.getAttribute('aria-expanded') === 'false', 'Escape cierra');
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
  expect(drawer.className).toContain('max-md:hidden');
  // Retorno de foco al toggle.
  await hasta(() => document.activeElement === toggle, 'el foco vuelve al toggle');
  expect(document.activeElement).toBe(toggle);
});
