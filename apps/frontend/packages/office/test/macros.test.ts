/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica PURA de macros (grabar/reproducir):
 *  - normalizeSteps: descarta comandos no reproducibles (undo/redo/presentar),
 *    conserva orden y valores, y devuelve copias limpias (sin aliasing).
 *  - replaySteps: despacha cada paso EN ORDEN por el sink inyectado.
 *  - serializeMacros/parseMacros: ida y vuelta + parseo tolerante (JSON basura,
 *    entradas y pasos mal formados descartados) en la frontera de persistencia.
 *  - macrosKey: clave por usuario, y NINGUNA sin sesión (no hay clave «anon»).
 *  - validarPaso/validarMacro/reproducir: la reproducción valida cada paso contra el
 *    registro de comandos, el tipo de documento, el permiso y el valor, y no despacha
 *    nada si uno falla.
 *  - Límites: pasos por macro, macros por usuario, tamaño de un valor.
 * Sin DOM ni localStorage (bun corre sin navegador). El pegamento de interfaz
 * vive en ribbon/MacrosGroup.tsx. Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/macros.test.ts
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  MAX_MACROS,
  MAX_PASOS,
  MAX_VALOR,
  loadMacros,
  macrosKey,
  normalizeSteps,
  parseMacros,
  replaySteps,
  reproducir,
  saveMacros,
  serializeMacros,
  validarMacro,
  validarPaso,
} from '../src/editor/macros';
import type { EntornoMacro, Macro, MacroStep, Rechazo } from '../src/editor/macros';

test('normalizeSteps: descarta undo/redo/presentar y conserva el resto en orden', () => {
  const raw: MacroStep[] = [
    { editor: 'document', cmd: 'bold' },
    { editor: 'document', cmd: 'undo' },
    { editor: 'document', cmd: 'color', value: '#DC2626' },
    { editor: 'document', cmd: 'redo' },
    { editor: 'presentation', cmd: 'presentar' },
    { editor: 'spreadsheet', cmd: 'align', value: 'center' },
  ];
  expect(normalizeSteps(raw)).toEqual([
    { editor: 'document', cmd: 'bold' },
    { editor: 'document', cmd: 'color', value: '#DC2626' },
    { editor: 'spreadsheet', cmd: 'align', value: 'center' },
  ]);
});

test('normalizeSteps: copia limpia (no aliasa los pasos de entrada)', () => {
  const raw: MacroStep[] = [{ editor: 'document', cmd: 'bold' }];
  const out = normalizeSteps(raw);
  expect(out[0]).not.toBe(raw[0]);
  expect(out[0]).toEqual(raw[0]);
});

test('replaySteps: despacha cada paso EN ORDEN', () => {
  const seen: MacroStep[] = [];
  const steps: MacroStep[] = [
    { editor: 'spreadsheet', cmd: 'bold' },
    { editor: 'spreadsheet', cmd: 'numfmt', value: 'percent' },
    { editor: 'spreadsheet', cmd: 'align', value: 'right' },
  ];
  replaySteps(steps, (s) => seen.push(s));
  expect(seen).toEqual(steps);
});

test('serializeMacros/parseMacros: ida y vuelta preserva las macros', () => {
  const macros: Macro[] = [
    {
      id: 'a',
      name: 'Encabezado',
      steps: [
        { editor: 'document', cmd: 'heading', value: 'h1' },
        { editor: 'document', cmd: 'bold' },
      ],
    },
    { id: 'b', name: 'Centrar', steps: [{ editor: 'spreadsheet', cmd: 'align', value: 'center' }] },
  ];
  expect(parseMacros(serializeMacros(macros))).toEqual(macros);
});

test('parseMacros: tolerante — JSON basura o no-array → []', () => {
  expect(parseMacros(null)).toEqual([]);
  expect(parseMacros('')).toEqual([]);
  expect(parseMacros('{no json')).toEqual([]);
  expect(parseMacros('{"a":1}')).toEqual([]); // objeto, no array
});

test('parseMacros: descarta entradas y pasos mal formados', () => {
  const json = JSON.stringify([
    { id: 'ok', name: 'Buena', steps: [{ editor: 'document', cmd: 'bold', value: 7 }] }, // value no-string → se ignora el value
    { id: 'x', name: 'Sin pasos', steps: [] }, // sin pasos → descartada
    { name: 'Sin id', steps: [{ editor: 'document', cmd: 'bold' }] }, // sin id → descartada
    { id: 'y', name: 'Editor malo', steps: [{ editor: 'otro', cmd: 'bold' }] }, // editor inválido → paso fuera → sin pasos → descartada
    'no-un-objeto',
  ]);
  expect(parseMacros(json)).toEqual([
    { id: 'ok', name: 'Buena', steps: [{ editor: 'document', cmd: 'bold' }] },
  ]);
});

test('macrosKey: clave por usuario; sin sesión no hay clave («anon» ya no existe)', () => {
  expect(macrosKey('sub-123')).toBe('sytue:docs:macros:sub-123');
  expect(macrosKey()).toBeNull();
  expect(macrosKey('')).toBeNull();
});

test('loadMacros/saveMacros: sin sesión no tocan localStorage', () => {
  const guardado: string[] = [];
  // Happy DOM registra un getter sin setter; conservar también su descriptor.
  const antes = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: () => {
        guardado.push('get');
        return '[]';
      },
      setItem: () => {
        guardado.push('set');
      },
    },
  });
  try {
    saveMacros([{ id: 'a', name: 'A', steps: [{ editor: 'document', cmd: 'bold' }] }]);
    expect(loadMacros()).toEqual([]);
    expect(guardado).toEqual([]); // ni lee ni escribe
    saveMacros([], 'sub-1');
    expect(guardado).toEqual(['set']);
  } finally {
    if (antes) Object.defineProperty(globalThis, 'localStorage', antes);
    else Reflect.deleteProperty(globalThis, 'localStorage');
  }
});

/* ---------- Validación de la reproducción ---------- */

const ORIGENES = { propio: 'https://docs.sytue.eu', drive: ['https://drive.sytue.eu'] };
const entorno = (parche: Partial<EntornoMacro> = {}): EntornoMacro => ({
  kind: 'document',
  soloLectura: false,
  origenes: ORIGENES,
  ...parche,
});
const doc = (cmd: string, value?: string): MacroStep =>
  value === undefined ? { editor: 'document', cmd } : { editor: 'document', cmd, value };

describe('validarPaso', () => {
  test('un paso bueno pasa', () => {
    expect(validarPaso(doc('bold'), entorno())).toBeNull();
    expect(validarPaso(doc('color', '#DC2626'), entorno())).toBeNull();
    expect(validarPaso(doc('font', 'Georgia'), entorno())).toBeNull();
  });

  test('un comando que el registro no conoce se rechaza', () => {
    expect(validarPaso(doc('formatC:'), entorno())).toBe('comando');
    expect(validarPaso(doc('__proto__'), entorno())).toBe('comando');
    expect(validarPaso(doc('toString'), entorno())).toBe('comando');
  });

  test('un comando de otro tipo de documento se rechaza', () => {
    // `sort` es de la hoja: en un folio no existe aunque el registro lo conozca.
    expect(validarPaso({ editor: 'spreadsheet', cmd: 'sort', value: 'asc' }, entorno())).toBe(
      'tipo',
    );
    // Y un paso de hoja no vale ni en un registro que lo admita si el documento abierto es otro.
    expect(
      validarPaso({ editor: 'spreadsheet', cmd: 'bold' }, entorno({ kind: 'presentation' })),
    ).toBe('tipo');
    expect(
      validarPaso({ editor: 'spreadsheet', cmd: 'bold' }, entorno({ kind: 'spreadsheet' })),
    ).toBeNull();
  });

  test('no reproduce en solo lectura lo que edita', () => {
    const lector = entorno({ soloLectura: true });
    expect(validarPaso(doc('bold'), lector)).toBe('soloLectura');
    expect(validarPaso(doc('heading', 'h1'), lector)).toBe('soloLectura');
    // Cortar es `copy` con valor: el registro lo da por lectura y mueve celdas.
    const hoja = entorno({ kind: 'spreadsheet', soloLectura: true });
    expect(validarPaso({ editor: 'spreadsheet', cmd: 'copy', value: 'cut' }, hoja)).toBe(
      'soloLectura',
    );
    expect(validarPaso({ editor: 'spreadsheet', cmd: 'copy' }, hoja)).toBeNull();
  });

  test('nunca reproduce deshacer, presentar ni los de contraseña', () => {
    for (const cmd of [
      'undo',
      'redo',
      'presentar',
      'protect',
      'unprotect',
      'protectbook',
      'unprotectbook',
      'extdata',
    ]) {
      expect(validarPaso({ editor: 'spreadsheet', cmd }, entorno({ kind: 'spreadsheet' }))).toBe(
        'excluido',
      );
    }
  });

  describe('valores', () => {
    const malo = (cmd: string, value: string | undefined): void => {
      expect(validarPaso(doc(cmd, value), entorno())).toBe('valor');
    };

    test('la fuente ha de ser de la lista', () => {
      malo('font', 'Comic Sans MS');
      malo('font', 'Arial; background:url(//x)');
      malo('font', undefined);
    });

    test('talla, interlineado y bloque, de los que ofrece la cinta', () => {
      malo('size', '99');
      malo('lineHeight', '3');
      malo('heading', 'script');
      expect(validarPaso(doc('size', '5'), entorno())).toBeNull();
      expect(validarPaso(doc('lineHeight', '1.15'), entorno())).toBeNull();
      expect(validarPaso(doc('heading', 'blockquote'), entorno())).toBeNull();
    });

    test('colores: hex válido; «transparent» solo donde quita el resaltado', () => {
      malo('color', 'red');
      malo('color', 'url(javascript:alert(1))');
      malo('color', '#12');
      malo('color', 'transparent');
      malo('highlight', 'expression(1)');
      expect(validarPaso(doc('highlight', 'transparent'), entorno())).toBeNull();
      expect(validarPaso(doc('color', '#abc'), entorno())).toBeNull();
    });

    test('enlaces: sin esquemas peligrosos', () => {
      malo('insertLink', 'javascript:alert(1)');
      malo('insertLink', 'JaVaScRiPt:alert(1)');
      malo('insertLink', 'data:text/html,<b>x</b>');
      malo('insertLink', 'https://a b');
      expect(validarPaso(doc('insertLink', 'https://sytue.eu'), entorno())).toBeNull();
      expect(validarPaso(doc('insertLink', 'mailto:a@b.eu'), entorno())).toBeNull();
      expect(validarPaso(doc('insertLink', 'sytue.eu'), entorno())).toBeNull();
    });

    test('imágenes: data:, blob: y ruta propia o de Drive; nada de terceros', () => {
      const png = 'data:image/png;base64,iVBORw0KGgo=';
      expect(validarPaso(doc('insertImage', png), entorno())).toBeNull();
      expect(
        validarPaso(doc('insertImage', 'blob:https://docs.sytue.eu/abc-123'), entorno()),
      ).toBeNull();
      expect(validarPaso(doc('insertImage', '/api/images/1'), entorno())).toBeNull();
      expect(validarPaso(doc('insertImage', 'https://docs.sytue.eu/x.png'), entorno())).toBeNull();
      expect(
        validarPaso(doc('insertImage', 'https://drive.sytue.eu/files/1'), entorno()),
      ).toBeNull();
      malo('insertImage', 'https://tracker.example/pixel.gif');
      malo('insertImage', '//tracker.example/pixel.gif');
      malo('insertImage', 'blob:https://otro.example/abc');
      malo('insertImage', 'data:image/svg+xml;base64,PHN2Zz48L3N2Zz4='); // SVG es código
      malo('insertImage', 'data:text/html;base64,PGI+');
      malo('insertImage', 'javascript:alert(1)');
      malo('insertImage', 'https://docs.sytue.eu.evil.example/x.png');
      malo('insertImage', '');
    });

    test('el medio de una diapositiva se valida por su `src`', () => {
      const pres = entorno({ kind: 'presentation' });
      const paso = (src: string): MacroStep => ({
        editor: 'presentation',
        cmd: 'media',
        value: JSON.stringify({ src, mediaType: 'video' }),
      });
      expect(validarPaso(paso('data:video/mp4;base64,AAAA'), pres)).toBeNull();
      expect(validarPaso(paso('https://cdn.example/v.mp4'), pres)).toBe('valor');
      expect(validarPaso({ editor: 'presentation', cmd: 'media', value: 'no json' }, pres)).toBe(
        'valor',
      );
    });

    test('tabla: FxC; alineación y fondo de celda', () => {
      malo('insertTable', '3x4<script>');
      malo('insertTable', '999x999');
      malo('tableCellValign', 'bottom;position:fixed');
      expect(validarPaso(doc('insertTable', '3x4'), entorno())).toBeNull();
    });

    test('JSON de la hoja: parseable, sin colores falsos y no desmesurado', () => {
      const hoja = entorno({ kind: 'spreadsheet' });
      const style = (v: string): MacroStep => ({ editor: 'spreadsheet', cmd: 'style', value: v });
      expect(validarPaso(style('{"color":"#112233","fill":"#fff"}'), hoja)).toBeNull();
      expect(validarPaso(style('{"color":"url(javascript:1)"}'), hoja)).toBe('valor');
      expect(validarPaso(style('{"cond":{"bgColor":"red"}}'), hoja)).toBe('valor');
      expect(validarPaso(style('{no json'), hoja)).toBe('valor');
      expect(validarPaso(style('[1,2]'), hoja)).toBeNull();
      expect(validarPaso(style('"cadena"'), hoja)).toBe('valor');
      expect(validarPaso(style('{"a":'.repeat(20) + '1' + '}'.repeat(20)), hoja)).toBe('valor'); // hondura
      expect(validarPaso(style(JSON.stringify({ x: 'a'.repeat(MAX_VALOR) })), hoja)).toBe('valor');
    });

    test('enumeraciones de la hoja', () => {
      const hoja = entorno({ kind: 'spreadsheet' });
      const ok = (cmd: string, value?: string): string | null =>
        validarPaso(
          value === undefined
            ? { editor: 'spreadsheet', cmd }
            : { editor: 'spreadsheet', cmd, value },
          hoja,
        );
      expect(ok('sort', 'asc')).toBeNull();
      expect(ok('sort', 'sideways')).toBe('valor');
      expect(ok('freeze', '2,1')).toBeNull();
      expect(ok('freeze', '2;1')).toBe('valor');
      expect(ok('border', 'outer')).toBeNull();
      expect(ok('border', 'dashed')).toBe('valor');
      expect(ok('decimals', '+1')).toBeNull();
      expect(ok('numfmt', 'percent')).toBeNull();
      expect(ok('numfmt', 'custom:0.00 "€"')).toBeNull();
      expect(ok('condmove', '3,-1')).toBeNull();
      expect(ok('condmove', '3,7')).toBe('valor');
      expect(ok('paste', 'values')).toBeNull();
      expect(ok('paste', 'formulas')).toBe('valor');
    });
  });
});

/*
 * Los VECTORES COMPARTIDOS con la app nativa (`test/vectores/macros.json`): los mismos
 * casos los exige `apps/nucleo/office/macros_test.go` a la validación de Go. Si uno se
 * cambia, se cambia en los dos lados: son una sola regla escrita dos veces.
 */
describe('vectores compartidos con la app nativa', () => {
  interface Caso {
    nombre: string;
    kind: EntornoMacro['kind'];
    editor: MacroStep['editor'];
    cmd: string;
    valor?: string;
    soloLectura: boolean;
    esperado: Rechazo | null;
  }
  const doc2 = JSON.parse(
    readFileSync(new URL('./vectores/macros.json', import.meta.url), 'utf8'),
  ) as { origenes: EntornoMacro['origenes']; casos: Caso[] };

  test('hay casos que comprobar', () => {
    expect(doc2.casos.length).toBeGreaterThan(60);
  });

  for (const caso of doc2.casos) {
    test(caso.nombre, () => {
      const paso: MacroStep =
        caso.valor === undefined
          ? { editor: caso.editor, cmd: caso.cmd }
          : { editor: caso.editor, cmd: caso.cmd, value: caso.valor };
      const e: EntornoMacro = {
        kind: caso.kind,
        soloLectura: caso.soloLectura,
        origenes: doc2.origenes,
      };
      expect(validarPaso(paso, e)).toBe(caso.esperado);
    });
  }
});

describe('validarMacro / reproducir', () => {
  const buena: MacroStep[] = [doc('bold'), doc('color', '#DC2626'), doc('heading', 'h2')];

  test('una macro buena valida', () => {
    expect(validarMacro(buena, entorno())).toEqual({ ok: true });
  });

  test('un solo paso malo la rechaza ENTERA y dice cuál', () => {
    const mala = [...buena, doc('font', 'Papyrus'), doc('italic')];
    expect(validarMacro(mala, entorno())).toEqual({ ok: false, motivo: 'valor', paso: 3 });
  });

  test('reproducir no despacha NADA si un paso falla (ni los buenos de antes)', () => {
    const visto: MacroStep[] = [];
    const v = reproducir([...buena, doc('insertLink', 'javascript:alert(1)')], entorno(), (s) =>
      visto.push(s),
    );
    expect(v).toEqual({ ok: false, motivo: 'valor', paso: 3 });
    expect(visto).toEqual([]);
  });

  test('reproducir despacha en orden cuando todo valida', () => {
    const visto: MacroStep[] = [];
    expect(reproducir(buena, entorno(), (s) => visto.push(s))).toEqual({ ok: true });
    expect(visto).toEqual(buena);
  });

  test('en solo lectura no despacha nada', () => {
    const visto: MacroStep[] = [];
    const v = reproducir(buena, entorno({ soloLectura: true }), (s) => visto.push(s));
    expect(v).toEqual({ ok: false, motivo: 'soloLectura', paso: 0 });
    expect(visto).toEqual([]);
  });

  test('un techo de pasos', () => {
    const larga = Array.from({ length: MAX_PASOS + 1 }, () => doc('bold'));
    expect(validarMacro(larga, entorno())).toEqual({
      ok: false,
      motivo: 'limite',
      paso: MAX_PASOS,
    });
    expect(validarMacro(larga.slice(0, MAX_PASOS), entorno())).toEqual({ ok: true });
  });
});

describe('lo guardado no se fía', () => {
  test('parseMacros descarta comandos que el registro no conoce y los de contraseña', () => {
    const json = JSON.stringify([
      {
        id: 'a',
        name: 'Mezcla',
        steps: [
          { editor: 'document', cmd: 'bold' },
          { editor: 'document', cmd: 'constructor' },
          { editor: 'spreadsheet', cmd: 'unprotect', value: 'mi-clave' },
          { editor: 'spreadsheet', cmd: 'protect', value: '{"password":"x"}' },
        ],
      },
    ]);
    expect(parseMacros(json)).toEqual([
      { id: 'a', name: 'Mezcla', steps: [{ editor: 'document', cmd: 'bold' }] },
    ]);
  });

  test('parseMacros recorta macros, pasos y nombre a los techos', () => {
    const pasos = Array.from({ length: MAX_PASOS + 50 }, () => ({
      editor: 'document',
      cmd: 'bold',
    }));
    const macros = Array.from({ length: MAX_MACROS + 5 }, (_, i) => ({
      id: `m${i}`,
      name: 'x'.repeat(500),
      steps: pasos,
    }));
    const out = parseMacros(JSON.stringify(macros));
    expect(out).toHaveLength(MAX_MACROS);
    expect(out[0]!.steps).toHaveLength(MAX_PASOS);
    expect(out[0]!.name.length).toBeLessThanOrEqual(80);
  });

  test('parseMacros descarta un valor desmesurado', () => {
    const json = JSON.stringify([
      {
        id: 'a',
        name: 'Imagen',
        steps: [
          { editor: 'document', cmd: 'bold' },
          { editor: 'document', cmd: 'insertImage', value: 'x'.repeat(MAX_VALOR + 1) },
        ],
      },
    ]);
    expect(parseMacros(json)[0]!.steps).toEqual([{ editor: 'document', cmd: 'bold' }]);
  });

  test('normalizeSteps descarta lo que no está en el registro y los valores grandes', () => {
    expect(
      normalizeSteps([
        doc('bold'),
        doc('inventado'),
        doc('insertImage', 'x'.repeat(MAX_VALOR + 1)),
        { editor: 'spreadsheet', cmd: 'unprotect', value: 'pw' },
      ]),
    ).toEqual([doc('bold')]);
  });
});
