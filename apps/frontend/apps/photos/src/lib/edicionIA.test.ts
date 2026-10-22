/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { interpretarEdicionIA, convertirEdicionEnPasos } from './edicionIA.js';
import { validaReceta, validaPaso } from '../editor/receta.js';

describe('edicionIA - interpretarEdicionIA', () => {
  test('interpreta JSON puro con ajustes de color', () => {
    const raw = JSON.stringify({
      explicacion: 'Aumento de calidez y contraste para simular atardecer.',
      ajustes: {
        exposicion: 0.35,
        contraste: 0.2,
        saturacion: 0.15,
        temperatura: 0.4,
      },
    });

    const res = interpretarEdicionIA(raw);
    expect(res).not.toBeNull();
    expect(res?.explicacion).toBe('Aumento de calidez y contraste para simular atardecer.');
    expect(res?.ajustes.exposicion).toBe(0.35);
    expect(res?.ajustes.contraste).toBe(0.2);
    expect(res?.ajustes.saturacion).toBe(0.15);
    expect(res?.ajustes.temperatura).toBe(0.4);
    expect(res?.recorte).toBeUndefined();
  });

  test('interpreta respuesta envuelta en bloques de código markdown y preámbulo', () => {
    const raw = `
Hola, aquí tienes la edición que me pediste para esta fotografía:
\`\`\`json
{
  "explicacion": "Blanco y negro dramático con negros profundos y altas luces bien definidas.",
  "ajustes": {
    "exposicion": 0.1,
    "contraste": 0.5,
    "saturacion": -1.0,
    "temperatura": 0
  }
}
\`\`\`
Espero que te guste el resultado.
`;

    const res = interpretarEdicionIA(raw);
    expect(res).not.toBeNull();
    expect(res?.explicacion).toContain('Blanco y negro');
    expect(res?.ajustes.saturacion).toBe(-1);
    expect(res?.ajustes.contraste).toBe(0.5);
  });

  test('interpreta y acota valores que exceden los rangos fotográficos físicos', () => {
    const raw = JSON.stringify({
      explicacion: 'Valores extremos',
      ajustes: {
        exposicion: 99.5, // máx 5
        contraste: -4.2, // mín -1
        saturacion: 2.0, // máx 1
        temperatura: -3.5, // mín -1
      },
    });

    const res = interpretarEdicionIA(raw);
    expect(res).not.toBeNull();
    expect(res?.ajustes.exposicion).toBe(5);
    expect(res?.ajustes.contraste).toBe(-1);
    expect(res?.ajustes.saturacion).toBe(1);
    expect(res?.ajustes.temperatura).toBe(-1);
  });

  test('interpreta recorte armónico válido', () => {
    const raw = JSON.stringify({
      explicacion: 'Reencuadre centrado según la regla de los tercios.',
      ajustes: {
        contraste: 0.1,
      },
      recorte: {
        x: 0.1,
        y: 0.05,
        ancho: 0.8,
        alto: 0.8,
      },
    });

    const res = interpretarEdicionIA(raw);
    expect(res).not.toBeNull();
    expect(res?.recorte).toEqual({
      x: 0.1,
      y: 0.05,
      ancho: 0.8,
      alto: 0.8,
    });
  });

  test('rechaza respuesta sin explicación o sin ajustes válidos', () => {
    expect(interpretarEdicionIA('{}')).toBeNull();
    expect(interpretarEdicionIA('{"explicacion": ""}')).toBeNull();
    expect(interpretarEdicionIA('{"explicacion": "sin ajustes", "ajustes": {}}')).toBeNull();
    expect(interpretarEdicionIA('Esto no es un json')).toBeNull();
  });
});

describe('edicionIA - convertirEdicionEnPasos', () => {
  test('convierte ajustes a pasos y valida contra el motor de recetas', () => {
    const edicion = {
      explicacion: 'Edición cinematográfica',
      ajustes: {
        exposicion: 0.25,
        contraste: 0.3,
        saturacion: -0.1,
        temperatura: 0.15,
      },
      recorte: {
        x: 0.1,
        y: 0.1,
        ancho: 0.8,
        alto: 0.8,
      },
    };

    const pasos = convertirEdicionEnPasos(edicion);
    expect(pasos.length).toBe(5);

    // Todos los pasos deben ser válidos individualmente
    for (const p of pasos) {
      expect(validaPaso(p)).toBeNull();
    }

    // La receta resultante debe ser válida
    const receta = { version: 1, pasos };
    expect(validaReceta(receta)).toBeNull();
  });

  test('omite ajustes con valor 0 para no ensuciar la receta', () => {
    const edicion = {
      explicacion: 'Solo temperatura',
      ajustes: {
        exposicion: 0,
        contraste: 0,
        saturacion: 0,
        temperatura: 0.35,
      },
    };

    const pasos = convertirEdicionEnPasos(edicion);
    expect(pasos.length).toBe(1);
    expect(pasos[0]?.tipo).toBe('temperatura');
    expect(pasos[0]?.params?.valor).toBe(0.35);
  });
});
