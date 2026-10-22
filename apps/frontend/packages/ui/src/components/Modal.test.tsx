/**
 * Tests de Modal — regresion del robo de foco (bug del renombrado de
 * calendarios: el modal enviaba el nombre viejo). Dos garantias:
 *  1) el foco inicial se fija SINCRONO al abrir (useLayoutEffect): nadie puede
 *     empezar a teclear antes de que el dialogo tome el foco;
 *  2) un re-render del padre con una closure onClose NUEVA (cargas de fondo,
 *     toasts) no re-ejecuta el efecto ni roba el foco del campo en uso.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { Modal } from './Modal.js';
import { Dropdown, MenuItem } from './Dropdown.js';
import { Popover } from './Popover.js';
import { catalogs } from '@sytue/i18n';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

test('al abrir, el foco entra al dialogo de forma sincrona (sin ventana de carrera)', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);

  render(
    <Modal open onClose={() => {}} title="Editar">
      <input id="campo0" />
    </Modal>,
    host,
  );
  // Sin esperar ningun frame: el layout effect ya debe haber enfocado. Y en el
  // CAMPO, no en el botón de cerrar: el diálogo arranca listo para teclear.
  expect(document.activeElement).toBe(document.getElementById('campo0'));
  render(null, host);
});

test('sin campos, el foco inicial entra en el boton de cerrar', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <Modal open onClose={() => {}} title="Aviso">
      <p>Solo texto</p>
    </Modal>,
    host,
  );
  // Se acepta la etiqueta de CUALQUIERA de los cinco idiomas: lo que este test afirma
  // es que el foco entra en el boton de cerrar, no en que idioma esta escrito. Atarlo
  // a `catalogs.en` daba por sentado que el entorno detecta ingles — cierto en la
  // maquina de quien lo escribio, falso en el runner de CI, donde fallaba sin que
  // nada del foco se hubiera roto.
  const cerrar = Object.values(catalogs).map((c) => c['ui.close']);
  expect(cerrar).toContain(
    (document.activeElement as HTMLElement | null)?.getAttribute('aria-label') ?? '',
  );
  render(null, host);
});

/** Sustituye `matchMedia` mientras dura `f`: happy-dom no sabe de dedos ni de anchos. */
function conMedios(casan: string[], f: () => void): void {
  const antes = window.matchMedia;
  window.matchMedia = ((q: string) => ({ matches: casan.includes(q), media: q })) as never;
  try {
    f();
  } finally {
    window.matchMedia = antes;
  }
}

/**
 * Da a `f` un contenedor montado y lo desmonta SIEMPRE, también si una aserción falla:
 * un Modal que se queda montado se queda con su Escape en `document` y se come el de los
 * tests que vienen detrás (Popover, Tooltip, AdminShell…).
 */
function enHost(f: (host: HTMLElement) => void): void {
  const host = document.body.appendChild(document.createElement('div'));
  try {
    f(host);
  } finally {
    render(null, host);
    host.remove();
  }
}

test('en el teléfono todo diálogo es una hoja anclada abajo, centrado o lateral', () => {
  for (const placement of ['center', 'right'] as const) {
    enHost((host) => {
      render(
        <Modal open onClose={() => {}} title="Redactar" placement={placement}>
          <input id="mensaje" />
        </Modal>,
        host,
      );
      const dialogo = document.querySelector('[role="dialog"]') as HTMLDivElement;
      // Las clases responsivas se resuelven por CSS; este test fija el contrato que
      // impide volver a la tarjeta flotando a media pantalla (y, en el redactor, a la
      // franja entre sus acciones y el teclado).
      expect(dialogo.parentElement?.className).toContain('max-md:items-end');
      expect(dialogo.className).toContain('max-md:rounded-t-card');
      // El foco entra en el panel con el dedo: sin esto, el `:focus-visible` global
      // redondeaba las esquinas de abajo (el aro lo quita `.sytue-hoja-movil`).
      expect(dialogo.className).toContain('max-md:rounded-b-none');
      expect(dialogo.className).toContain('sytue-hoja-movil');
      // El asa, solo en el teléfono y fuera del árbol accesible.
      const asa = dialogo.firstElementChild as HTMLElement;
      expect(asa.getAttribute('aria-hidden')).toBe('true');
      expect(asa.className).toContain('md:hidden');
    });
  }
});

test('en el teléfono el pie pone sus acciones a todo el ancho, de dos en dos', () => {
  enHost((host) => {
    render(
      <Modal
        open
        onClose={() => {}}
        title="Editar evento"
        footer={
          <>
            <button>Eliminar</button>
            <button>Cancelar</button>
            <button>Guardar</button>
          </>
        }
      >
        <p>Texto</p>
      </Modal>,
      host,
    );
    const pie = document.querySelector('[role="dialog"]')!.lastElementChild as HTMLElement;
    // Las acciones son HIJOS del pie (sin envoltorio: lo vigila pies-al-comun.test.ts).
    expect(pie.children.length).toBe(3);
    // El reparto se resuelve por CSS; este test fija el contrato medido con Playwright
    // en 390×844: media fila cada una, la primera de un número impar sola arriba y
    // 44 px de alto. En escritorio sigue el `justify-end` de siempre.
    expect(pie.className).toContain('max-md:[&>*]:flex-[1_1_calc(50%_-_0.25rem)]');
    expect(pie.className).toContain('max-md:[&>*:first-child:nth-last-child(odd)]:basis-full');
    expect(pie.className).toContain('max-md:[&>button]:min-h-11');
    expect(pie.className).toContain('justify-end');
  });
});

test('con el dedo, abrir no enfoca el campo: el teclado no tapa la hoja', () => {
  enHost((host) => {
    conMedios(['(pointer: coarse)'], () => {
      render(
        <Modal open onClose={() => {}} title="Nuevo evento">
          <input id="titulo-evento" />
        </Modal>,
        host,
      );
    });
    // El foco va al panel, que el lector de pantalla anuncia como el diálogo.
    expect(document.activeElement?.getAttribute('role')).toBe('dialog');
  });
});

test('bajar la hoja por el asa la cierra; un tirón corto, no', () => {
  let cierres = 0;
  const ui = (dismissable: boolean) => (
    <Modal open onClose={() => cierres++} title="Compartir" dismissable={dismissable}>
      <p>Texto</p>
    </Modal>
  );
  // El tiempo del gesto se fija a mano: con el reloj real, tres eventos seguidos
  // tardan 0 ms o unas décimas según la máquina, y un tirón corto pasaba a ser un
  // golpe rápido (que sí cierra) solo en el runner.
  const tirar = (zona: Element, hasta: number, ms = 1000) => {
    const ev = (tipo: string, y: number, t: number) => {
      const e = new PointerEvent(tipo, {
        bubbles: true,
        clientY: y,
        pointerType: 'touch',
        pointerId: 1,
      });
      Object.defineProperty(e, 'timeStamp', { value: t });
      zona.dispatchEvent(e);
    };
    ev('pointerdown', 100, 0);
    ev('pointermove', hasta, ms);
    ev('pointerup', hasta, ms);
  };
  enHost((host) => {
    conMedios(['(width < 48rem)'], () => {
      render(ui(true), host);
      const asa = document.querySelector('[role="dialog"]')!.firstElementChild!;
      tirar(asa, 140);
      expect(cierres).toBe(0);
      tirar(asa, 300);
      expect(cierres).toBe(1);
      // El mismo tirón corto, pero rápido, sí cierra: es el golpe de las hojas del sistema.
      tirar(asa, 140, 20);
      expect(cierres).toBe(2);
      // Sin cierre por clic fuera, ni asa ni arrastre: la cabecera no la baja.
      render(ui(false), host);
      const cabecera = document.querySelector('[role="dialog"] h2')!.parentElement!;
      expect(cabecera.previousElementSibling).toBeNull();
      tirar(cabecera, 400);
      expect(cierres).toBe(2);
    });
  });
});

test('los filetes de cabecera y pie solo aparecen cuando el cuerpo se desplaza', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  const ui = () => (
    <Modal open onClose={() => {}} title="Actividad" footer={<button>Cerrar</button>}>
      <p>Una línea</p>
    </Modal>
  );
  const dialogo = () => document.querySelector('[role="dialog"]')!;
  const bandas = () => {
    const cab = dialogo().querySelector('h2')!.parentElement!;
    return [cab.className, dialogo().lastElementChild!.className];
  };
  // El estado se fija en un efecto y Preact repinta en la siguiente vuelta.
  const vuelta = () => new Promise((r) => setTimeout(r, 0));
  try {
    render(ui(), host);
    await vuelta();
    for (const c of bandas()) expect(c).toContain('border-transparent');
    // Un cuerpo de 900 en un hueco de 300: se desplaza.
    const cuerpo = dialogo().querySelector('h2')!.parentElement!.nextElementSibling!;
    Object.defineProperty(cuerpo, 'scrollHeight', { configurable: true, get: () => 900 });
    Object.defineProperty(cuerpo, 'clientHeight', { configurable: true, get: () => 300 });
    render(ui(), host);
    await vuelta();
    for (const c of bandas()) expect(c).toContain('border-border-subtle');
  } finally {
    // Siempre: un Modal que se queda montado se queda también con su Escape en
    // `document`, y se come el de los tests que vienen detrás.
    render(null, host);
    host.remove();
  }
});

test('re-render del padre con onClose nuevo NO roba el foco del campo', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);

  const ui = (onClose: () => void) => (
    <Modal open onClose={onClose} title="Editar">
      <input id="campo" />
    </Modal>
  );

  render(
    ui(() => {}),
    host,
  );

  // El usuario entra al campo y escribe.
  const campo = document.getElementById('campo') as HTMLInputElement;
  campo.focus();
  campo.value = 'nombre nuevo';
  expect(document.activeElement).toBe(campo);

  // Re-render del padre (misma prop open, closure onClose NUEVA).
  render(
    ui(() => {}),
    host,
  );

  // El foco sigue en el campo: el texto del usuario no se pierde.
  expect(document.activeElement).toBe(campo);
  expect((document.getElementById('campo') as HTMLInputElement).value).toBe('nombre nuevo');

  render(null, host);
});

test('Escape invoca la onClose MAS RECIENTE (no la capturada al abrir)', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);

  const llamadas: string[] = [];
  const ui = (etiqueta: string) => (
    <Modal open onClose={() => llamadas.push(etiqueta)} title="Editar">
      <input id="campo2" />
    </Modal>
  );

  render(ui('primera'), host);
  render(ui('segunda'), host);

  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  expect(llamadas).toEqual(['segunda']);

  render(null, host);
});

test('el panel derecho conserva el borrador y devuelve el foco al cerrar', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const trigger = document.createElement('button');
  document.body.appendChild(trigger);
  trigger.focus();
  let cerrado = false;
  render(
    <Modal
      open
      placement="right"
      title="Nuevo producto"
      onClose={() => {
        cerrado = true;
      }}
    >
      <input id="producto-panel" />
    </Modal>,
    host,
  );
  const campo = document.getElementById('producto-panel') as HTMLInputElement;
  expect(document.activeElement).toBe(campo);
  campo.value = 'Producto nuevo';
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  expect(cerrado).toBe(true);
  expect(campo.value).toBe('Producto nuevo');
  render(null, host);
  expect(document.activeElement).toBe(trigger);
  host.remove();
  trigger.remove();
});

for (const capa of ['menu', 'popover'] as const) {
  test(`Escape cierra primero el ${capa} de un modal y conserva el formulario`, async () => {
    const host = document.body.appendChild(document.createElement('div'));
    let cierres = 0;
    const titulo = `Edición ${capa}`;
    const etiquetaHija = `Opciones ${capa}`;
    function Edicion() {
      const [open, setOpen] = useState(true);
      const [borrador, setBorrador] = useState('Original');
      return (
        <Modal
          open={open}
          placement="right"
          title={titulo}
          onClose={() => {
            cierres++;
            setOpen(false);
          }}
        >
          <input
            id={`borrador-${capa}`}
            value={borrador}
            onInput={(e) => setBorrador(e.currentTarget.value)}
          />
          {capa === 'menu' ? (
            <Dropdown
              label={etiquetaHija}
              trigger={(props) => (
                <button id={`abrir-${capa}`} {...props}>
                  Opciones
                </button>
              )}
            >
              <MenuItem onClick={() => {}}>Acción</MenuItem>
            </Dropdown>
          ) : (
            <Popover
              label={etiquetaHija}
              trigger={(props) => (
                <button id={`abrir-${capa}`} {...props}>
                  Opciones
                </button>
              )}
            >
              <input aria-label="Campo contextual" />
            </Popover>
          )}
        </Modal>
      );
    }
    try {
      render(<Edicion />, host);
      const campo = document.getElementById(`borrador-${capa}`) as HTMLInputElement;
      campo.value = 'Borrador sin guardar';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
      await new Promise((r) => setTimeout(r, 20));
      document.getElementById(`abrir-${capa}`)!.click();
      await new Promise((r) => setTimeout(r, 60));
      const hija = () => document.querySelector(`[aria-label="${etiquetaHija}"]`);
      const modal = () => document.querySelector(`[role="dialog"][aria-label="${titulo}"]`);
      expect(hija()).not.toBeNull();
      expect(hija()?.contains(document.activeElement)).toBe(true);

      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
      );
      await new Promise((r) => setTimeout(r, 60));
      expect(hija()).toBeNull();
      expect(modal()).not.toBeNull();
      expect(cierres).toBe(0);
      expect(campo.value).toBe('Borrador sin guardar');
      expect(document.activeElement).toBe(document.getElementById(`abrir-${capa}`));

      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
      );
      await new Promise((r) => setTimeout(r, 60));
      expect(modal()).toBeNull();
      expect(cierres).toBe(1);
    } finally {
      render(null, host);
      host.remove();
    }
  });
}
