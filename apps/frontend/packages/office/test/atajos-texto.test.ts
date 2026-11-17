/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El manejador de atajos del texto editable (editor/canvas/atajos.ts), el que llaman el
 * folio, la caja de Diapos y el editor de celda: que despache lo del registro, que el
 * enlace pida su URL en vez de mandarse vacío y que la celda solo admita las marcas.
 *   cd apps/frontend && bun test packages/office/test/atajos-texto.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { describe, expect, test } from 'bun:test';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { MARCAS_DE_CELDA, atenderAtajoDeTexto } = await import('../src/editor/canvas/atajos');

type Mandado = [string, string | undefined];

const pulsar = (key: string, mods: Partial<KeyboardEvent> = {}) => {
  let consumida = false;
  const e = {
    key,
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
    ...mods,
    preventDefault: () => {
      consumida = true;
    },
  };
  return { e, consumida: () => consumida };
};

const destino = () => {
  const mandados: Mandado[] = [];
  return { mandados, exec: (c: string, v?: string) => void mandados.push([c, v]) };
};

describe('atajos del texto editable', () => {
  test('despacha el comando del registro con su valor y consume la tecla', () => {
    const d = destino();
    const p = pulsar('2', { altKey: true });
    expect(atenderAtajoDeTexto(p.e, d)).toBe(true);
    expect(p.consumida()).toBe(true);
    expect(d.mandados).toEqual([['heading', 'h2']]);
  });

  test('lo que no es suyo no se toca', () => {
    const d = destino();
    const p = pulsar('s');
    expect(atenderAtajoDeTexto(p.e, d)).toBe(false);
    expect(p.consumida()).toBe(false);
    expect(d.mandados).toEqual([]);
  });

  test('Ctrl+K pide la URL y enlaza con ella; cancelar no manda nada', async () => {
    const d = destino();
    const antes = window.prompt;
    let respuesta: string | null = 'sytue.eu';
    window.prompt = () => respuesta;
    try {
      expect(atenderAtajoDeTexto(pulsar('k').e, d)).toBe(true);
      await Promise.resolve();
      await Promise.resolve();
      expect(d.mandados).toEqual([['insertLink', 'sytue.eu']]);
      respuesta = null;
      atenderAtajoDeTexto(pulsar('k').e, d);
      await Promise.resolve();
      await Promise.resolve();
      expect(d.mandados).toHaveLength(1);
    } finally {
      window.prompt = antes;
    }
  });

  test('en el editor de celda solo valen las marcas', () => {
    const d = destino();
    expect(atenderAtajoDeTexto(pulsar('b').e, d, MARCAS_DE_CELDA)).toBe(true);
    const centrar = pulsar('E', { shiftKey: true });
    expect(atenderAtajoDeTexto(centrar.e, d, MARCAS_DE_CELDA)).toBe(false);
    expect(centrar.consumida()).toBe(false);
    expect(d.mandados).toEqual([['bold', undefined]]);
  });
});
