/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GESTIONAR UN CAMPO DESDE SU CABECERA: el propietario tiene en cada cabecera un menú ▾
 * (renombrar, la fórmula si la hay, borrar) y una «+» al final; quien no lo es no ve
 * nada de eso. Los diálogos dejan el error del servicio dentro y no envían lo que no
 * vale: un nombre igual, una fórmula con un nombre que no existe, un campo que usa
 * una fórmula.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { BasesField, BasesRecord, BasesTable } from '@sytue/api-clients/bases';
import { Rejilla, type AccionCampo } from './Rejilla.js';
import {
  DialogoBorrarCampo,
  DialogoFormulaCampo,
  DialogoOpcionesCampo,
  DialogoRenombrarCampo,
} from './Dialogos.js';

// Solo aquí, y se retira al acabar: el resto de la suite corre sin DOM.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});
afterEach(() => {
  render(null, document.body);
  document.body.innerHTML = '';
});

const t = ((k: string, v?: Record<string, unknown>) =>
  v ? `${k}:${Object.values(v).join(',')}` : k) as never;

const campo = (id: string, name: string, kind: BasesField['kind'], extra = {}): BasesField => ({
  id,
  table_id: 't1',
  name,
  kind,
  position: 0,
  ...extra,
});

const CAMPOS = [
  campo('pre', 'Precio', 'number'),
  campo('nom', 'Nombre', 'text'),
  campo('tot', 'Total', 'formula', { formula: '{pre} * 2', result: 'number' }),
];
const TABLAS: BasesTable[] = [
  { id: 't1', base_id: 'b', name: 'Pedidos', position: 0, fields: CAMPOS },
];
const REGISTRO: BasesRecord = {
  id: 'r1',
  table_id: 't1',
  data: { pre: 3, nom: 'Ana', tot: 6 },
  links: {},
  rev: 1,
  created_by: '',
  updated_by: '',
  created_at: '',
  updated_at: '',
};

function montarRejilla(propietario = true) {
  const acciones: [AccionCampo, string][] = [];
  const nuevos: number[] = [];
  act(() => {
    render(
      <Rejilla
        t={t}
        nombreTabla="Pedidos"
        campos={CAMPOS}
        registros={[REGISTRO]}
        puedeEditar
        etiquetas={{}}
        onCambiar={async () => null}
        onElegirVarios={() => undefined}
        onAdjuntos={() => undefined}
        onAbrir={() => undefined}
        onNuevo={() => undefined}
        desde={0}
        hayAntes={false}
        onAntes={() => undefined}
        hayMas={false}
        onMas={() => undefined}
        {...(propietario
          ? {
              onCampo: (a: AccionCampo, c: BasesField) => acciones.push([a, c.id]),
              onNuevoCampo: () => nuevos.push(1),
            }
          : {})}
      />,
      document.body,
    );
  });
  return { acciones, nuevos };
}

const botonPorNombre = (nombre: string) =>
  document.querySelector<HTMLButtonElement>(`button[aria-label="${nombre}"]`);

const botonPorTexto = (texto: string) =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

const itemsDelMenu = () =>
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].map((b) =>
    b.textContent?.trim(),
  );

function escribir(el: HTMLInputElement | HTMLTextAreaElement, valor: string): void {
  act(() => {
    el.value = valor;
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

test('el propietario tiene un menú en cada cabecera y una «+» al final', () => {
  const { acciones, nuevos } = montarRejilla();
  const cabeceras = document.querySelectorAll('thead th');
  // Fila, los tres campos y la «+».
  expect(cabeceras.length).toBe(5);
  expect(document.querySelector('table')?.getAttribute('aria-colcount')).toBe('5');
  for (const c of CAMPOS) expect(botonPorNombre(`field.actions:${c.name}`)).not.toBeNull();

  // Un campo corriente: renombrar, su valor por defecto y borrar, sin fórmula.
  act(() => botonPorNombre('field.actions:Nombre')!.click());
  expect(itemsDelMenu()).toEqual(['field.rename', 'field.default', 'field.delete']);
  act(() => botonPorTexto('field.default')!.click());
  expect(acciones.pop()).toEqual(['defecto', 'nom']);
  act(() => botonPorNombre('field.actions:Nombre')!.click());
  act(() => botonPorTexto('field.rename')!.click());
  expect(acciones).toEqual([['renombrar', 'nom']]);
  expect(document.querySelector('[role="menu"]')).toBeNull();

  // Una fórmula también se edita; y borrar avisa a quien la gestiona.
  act(() => botonPorNombre('field.actions:Total')!.click());
  expect(itemsDelMenu()).toEqual(['field.rename', 'field.editFormula', 'field.delete']);
  act(() => botonPorTexto('field.editFormula')!.click());
  act(() => botonPorNombre('field.actions:Precio')!.click());
  act(() => botonPorTexto('field.delete')!.click());
  expect(acciones).toEqual([
    ['renombrar', 'nom'],
    ['formula', 'tot'],
    ['borrar', 'pre'],
  ]);

  act(() => botonPorNombre('field.add')!.click());
  expect(nuevos).toEqual([1]);
});

test('Escape cierra el menú y devuelve el foco al ▾ de la cabecera', () => {
  montarRejilla();
  const disparador = botonPorNombre('field.actions:Precio')!;
  disparador.focus();
  act(() => disparador.click());
  expect(document.querySelector('[role="menu"]')).not.toBeNull();
  act(() => {
    (document.activeElement ?? document.body).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
  });
  expect(document.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(disparador);
});

test('quien no es propietario no ve el menú ni la «+»', () => {
  montarRejilla(false);
  expect(document.querySelectorAll('thead th').length).toBe(4);
  expect(botonPorNombre('field.actions:Precio')).toBeNull();
  expect(botonPorNombre('field.add')).toBeNull();
  expect(document.querySelector('table')?.getAttribute('aria-colcount')).toBe('4');
});

test('renombrar no envía el mismo nombre y enseña el error del servicio', () => {
  const guardados: string[] = [];
  act(() => {
    render(
      <DialogoRenombrarCampo
        t={t}
        campo={CAMPOS[1]!}
        error="err.invalid"
        ocupado={false}
        onGuardar={(n) => guardados.push(n)}
        onCerrar={() => undefined}
      />,
      document.body,
    );
  });
  const entrada = document.getElementById('renombrar-campo') as HTMLInputElement;
  expect(entrada.value).toBe('Nombre');
  expect(botonPorTexto('common.save')!.disabled).toBe(true);
  escribir(entrada, '  Cliente ');
  act(() => botonPorTexto('common.save')!.click());
  expect(guardados).toEqual(['Cliente']);
  expect(document.querySelector('[role="alert"]')?.textContent).toBe('err.invalid');
});

test('la fórmula se edita con nombres y se envía con ids', () => {
  const guardadas: string[] = [];
  act(() => {
    render(
      <DialogoFormulaCampo
        t={t}
        campo={CAMPOS[2]!}
        campos={CAMPOS}
        tablas={TABLAS}
        error=""
        ocupado={false}
        onGuardar={(f) => guardadas.push(f)}
        onCerrar={() => undefined}
      />,
      document.body,
    );
  });
  const area = document.getElementById('formula-campo') as HTMLTextAreaElement;
  expect(area.value).toBe('{Precio} * 2');
  expect(botonPorTexto('common.save')!.disabled).toBe(true);
  escribir(area, '{Nada} * 3');
  expect(document.body.textContent).toContain('field.formulaUnknown');
  expect(botonPorTexto('common.save')!.disabled).toBe(true);
  escribir(area, '{precio} * 3');
  act(() => botonPorTexto('common.save')!.click());
  expect(guardadas).toEqual(['{pre} * 3']);
});

test('borrar un campo avisa de sus datos; si lo usa una fórmula, no se deja', () => {
  const borrados: number[] = [];
  const montar = (c: BasesField, error = '') =>
    act(() => {
      render(
        <DialogoBorrarCampo
          t={t}
          campo={c}
          tablas={TABLAS}
          error={error}
          ocupado={false}
          onBorrar={() => borrados.push(1)}
          onCerrar={() => undefined}
        />,
        document.body,
      );
    });
  montar(CAMPOS[1]!, 'err.generic');
  expect(document.body.textContent).toContain('field.deleteConfirm:Nombre');
  expect(document.querySelector('[role="alert"]')?.textContent).toBe('err.generic');
  act(() => botonPorTexto('common.delete')!.click());
  expect(borrados).toEqual([1]);

  render(null, document.body);
  montar(CAMPOS[0]!);
  expect(document.body.textContent).toContain('field.deleteBlocked:Precio,Total');
  expect(botonPorTexto('common.delete')!.disabled).toBe(true);
});

const ESTADO = campo('est', 'Estado', 'select', { choices: ['Nuevo', 'Hecho'] });

function montarConMover() {
  const movidos: [string, string][] = [];
  const acciones: [AccionCampo, string][] = [];
  const campos = [...CAMPOS, ESTADO];
  act(() => {
    render(
      <Rejilla
        t={t}
        nombreTabla="Pedidos"
        campos={campos}
        registros={[{ ...REGISTRO, data: { ...REGISTRO.data, est: 'Viejo' } }]}
        puedeEditar
        etiquetas={{}}
        onCambiar={async () => null}
        onElegirVarios={() => undefined}
        onAdjuntos={() => undefined}
        onAbrir={() => undefined}
        onNuevo={() => undefined}
        desde={0}
        hayAntes={false}
        onAntes={() => undefined}
        hayMas={false}
        onMas={() => undefined}
        onCampo={(a, c) => acciones.push([a, c.id])}
        onMoverCampo={(c, d) => movidos.push([c.id, d.id])}
      />,
      document.body,
    );
  });
  return { movidos, acciones };
}

test('un campo de selección edita sus opciones y cualquiera se mueve desde el menú', () => {
  const { movidos, acciones } = montarConMover();
  act(() => botonPorNombre('field.actions:Estado')!.click());
  expect(itemsDelMenu()).toEqual([
    'field.rename',
    'field.editChoices',
    'field.default',
    'field.moveLeft',
    'field.moveRight',
    'field.delete',
  ]);
  // El último no se mueve más a la derecha.
  expect(botonPorTexto('field.moveRight')!.disabled).toBe(true);
  act(() => botonPorTexto('field.editChoices')!.click());
  act(() => botonPorNombre('field.actions:Estado')!.click());
  act(() => botonPorTexto('field.moveLeft')!.click());
  expect(acciones).toEqual([['opciones', 'est']]);
  expect(movidos).toEqual([['est', 'tot']]);
});

test('Alt+flechas en el ▾ de la cabecera mueven la columna; sin Alt, nada', () => {
  const { movidos } = montarConMover();
  const ir = (tecla: string, alt = true) =>
    act(() => {
      botonPorNombre('field.actions:Nombre')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: tecla, altKey: alt, bubbles: true }),
      );
    });
  ir('ArrowRight');
  ir('ArrowLeft');
  ir('ArrowRight', false);
  expect(movidos).toEqual([
    ['nom', 'tot'],
    ['nom', 'pre'],
  ]);
  expect(botonPorNombre('field.actions:Nombre')!.getAttribute('aria-keyshortcuts')).toBe(
    'Alt+ArrowLeft Alt+ArrowRight',
  );
  expect(document.querySelector('[role="status"]')?.textContent).toBe('field.moved:Nombre,1');
});

test('arrastrar una cabecera sobre otra mueve el campo a su sitio', () => {
  const { movidos } = montarConMover();
  const th = (id: string) => document.querySelector<HTMLElement>(`th[data-campo="${id}"]`)!;
  expect(th('pre').getAttribute('draggable')).toBe('true');
  // Preact escucha `dragstart` si el elemento tiene `ondragstart`; happy-dom no lo
  // tiene, y entonces el nombre va tal cual («DragStart»). Se manda el que escucha.
  const NOMBRES: Record<string, string> = {
    dragstart: 'DragStart',
    dragover: 'DragOver',
    drop: 'Drop',
  };
  const evento = (tipo: string) => {
    const nombre = `on${tipo}` in th('pre') ? tipo : NOMBRES[tipo]!;
    const e = new Event(nombre, { bubbles: true, cancelable: true });
    Object.defineProperty(e, 'dataTransfer', {
      value: { setData: () => undefined, effectAllowed: '' },
    });
    return e;
  };
  act(() => {
    th('est').dispatchEvent(evento('dragstart'));
  });
  act(() => {
    th('pre').dispatchEvent(evento('dragover'));
  });
  act(() => {
    th('pre').dispatchEvent(evento('drop'));
  });
  expect(movidos).toEqual([['est', 'pre']]);
});

test('sin onMoverCampo la cabecera no se arrastra ni ofrece moverla', () => {
  montarRejilla();
  expect(document.querySelector('th[draggable="true"]')).toBeNull();
  act(() => botonPorNombre('field.actions:Nombre')!.click());
  expect(itemsDelMenu()).not.toContain('field.moveLeft');
});

test('una opción retirada se conserva en la celda, en gris y dicha con texto', () => {
  montarConMover();
  const retirada = document.querySelector('[data-retirada]');
  expect(retirada?.textContent).toContain('Viejo');
  expect(retirada?.textContent).toContain('field.retiredOption');
});

test('editar las opciones avisa de las que se retiran y envía la lista limpia', () => {
  const guardadas: string[][] = [];
  act(() => {
    render(
      <DialogoOpcionesCampo
        t={t}
        campo={ESTADO}
        error=""
        ocupado={false}
        onGuardar={(o) => guardadas.push(o)}
        onCerrar={() => undefined}
      />,
      document.body,
    );
  });
  const area = document.getElementById('opciones-campo') as HTMLTextAreaElement;
  expect(area.value).toBe('Nuevo\nHecho');
  expect(botonPorTexto('common.save')!.disabled).toBe(true);
  escribir(area, ' Hecho \nUrgente\n\nUrgente');
  expect(document.body.textContent).toContain('field.choicesRetiring:Nuevo');
  act(() => botonPorTexto('common.save')!.click());
  expect(guardadas).toEqual([['Hecho', 'Urgente']]);
  escribir(area, '  ');
  expect(botonPorTexto('common.save')!.disabled).toBe(true);
});
