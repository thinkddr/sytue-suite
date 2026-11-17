// Política del sonido de aviso de mensajes (D-ROOM-07). Ejecutar: `bun test`.
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import {
  CLAVE_SONIDO,
  SONIDO_POR_DEFECTO,
  conSeparacion,
  debeSonar,
  guardarPreferenciasSonido,
  leerPreferenciasSonido,
  type ContextoSonido,
} from './sonidoAviso.js';

const canal = { kind: 'channel' };
const directo = { kind: 'direct' };

/** Mensaje ajeno, en otro canal, con la pestaña de fondo y el ajuste por defecto. */
function ctx(cambios: Partial<ContextoSonido> = {}): ContextoSonido {
  return {
    personal: false,
    canalActivo: false,
    ventanaConFoco: false,
    bloqueado: false,
    silencio: false,
    prefs: { ...SONIDO_POR_DEFECTO },
    ...cambios,
  };
}

describe('por defecto', () => {
  test('suena un mensaje directo', () => {
    expect(debeSonar({}, directo, ctx())).toBe(true);
  });

  test('suena una mención o palabra clave en un canal', () => {
    expect(debeSonar({}, canal, ctx({ personal: true }))).toBe(true);
  });

  test('calla un mensaje cualquiera de canal, aunque el canal pida «Todas»', () => {
    expect(debeSonar({}, canal, ctx())).toBe(false);
    expect(debeSonar({}, { ...canal, notifyLevel: 'all' }, ctx())).toBe(false);
  });
});

describe('ajustable', () => {
  test('apagar «directos y menciones» los calla', () => {
    const prefs = { directos: false, resto: false };
    expect(debeSonar({}, directo, ctx({ prefs }))).toBe(false);
    expect(debeSonar({}, canal, ctx({ personal: true, prefs }))).toBe(false);
  });

  test('encender «resto» hace sonar los mensajes de canal', () => {
    expect(debeSonar({}, canal, ctx({ prefs: { directos: true, resto: true } }))).toBe(true);
  });

  test('«resto» encendido no despierta a un canal en «Solo menciones»', () => {
    const prefs = { directos: true, resto: true };
    expect(debeSonar({}, { ...canal, notifyLevel: 'mentions' }, ctx({ prefs }))).toBe(false);
    expect(
      debeSonar({}, { ...canal, notifyLevel: 'mentions' }, ctx({ prefs, personal: true })),
    ).toBe(true);
  });

  test('un directo puesto en «Solo menciones» solo suena si te mencionan', () => {
    expect(debeSonar({}, { ...directo, notifyLevel: 'mentions' }, ctx())).toBe(false);
  });
});

describe('calla siempre', () => {
  test('lo propio, lo bloqueado y en «No molestar» o pausa', () => {
    expect(debeSonar({ mine: true }, directo, ctx())).toBe(false);
    expect(debeSonar({}, directo, ctx({ bloqueado: true }))).toBe(false);
    expect(debeSonar({}, directo, ctx({ silencio: true }))).toBe(false);
  });

  test('la tarjeta de llamada, que ya tiene su timbre', () => {
    expect(debeSonar({ type: 'call' }, directo, ctx())).toBe(false);
    expect(debeSonar({ type: 'user' }, directo, ctx())).toBe(true);
  });

  test('un canal silenciado o sin canal conocido', () => {
    expect(debeSonar({}, { ...directo, muted: true }, ctx({ personal: true }))).toBe(false);
    expect(debeSonar({}, { ...canal, notifyLevel: 'none' }, ctx({ personal: true }))).toBe(false);
    expect(debeSonar({}, undefined, ctx({ personal: true }))).toBe(false);
  });

  test('lo que ya estás viendo: canal abierto con la pestaña al frente', () => {
    expect(debeSonar({}, directo, ctx({ canalActivo: true, ventanaConFoco: true }))).toBe(false);
    // Con la pestaña de fondo, el canal abierto no se está mirando: suena.
    expect(debeSonar({}, directo, ctx({ canalActivo: true }))).toBe(true);
    // Al frente pero en otro canal: suena.
    expect(debeSonar({}, directo, ctx({ ventanaConFoco: true }))).toBe(true);
  });
});

describe('separación mínima', () => {
  test('una ráfaga suena una vez y vuelve a sonar pasado el margen', () => {
    let reloj = 1000;
    let veces = 0;
    const tocar = conSeparacion(
      () => veces++,
      () => reloj,
      2000,
    );
    expect(tocar()).toBe(true);
    reloj += 500;
    expect(tocar()).toBe(false);
    reloj += 1499;
    expect(tocar()).toBe(false);
    reloj += 1;
    expect(tocar()).toBe(true);
    expect(veces).toBe(2);
  });
});

describe('ajuste de este navegador', () => {
  const real = (globalThis as { localStorage?: Storage }).localStorage;
  let datos: Map<string, string>;
  beforeEach(() => {
    datos = new Map();
    (globalThis as { localStorage?: unknown }).localStorage = {
      getItem: (k: string) => datos.get(k) ?? null,
      setItem: (k: string, v: string) => void datos.set(k, v),
      removeItem: (k: string) => void datos.delete(k),
    };
  });
  afterEach(() => {
    (globalThis as { localStorage?: unknown }).localStorage = real;
  });

  test('sin nada guardado, directos sí y resto no', () => {
    expect(leerPreferenciasSonido()).toEqual({ directos: true, resto: false });
  });

  test('lo guardado se relee', () => {
    guardarPreferenciasSonido({ directos: false, resto: true });
    expect(leerPreferenciasSonido()).toEqual({ directos: false, resto: true });
  });

  test('basura o campos sueltos caen al defecto campo a campo', () => {
    datos.set(CLAVE_SONIDO, '{roto');
    expect(leerPreferenciasSonido()).toEqual({ directos: true, resto: false });
    datos.set(CLAVE_SONIDO, JSON.stringify({ resto: true, directos: 'sí' }));
    expect(leerPreferenciasSonido()).toEqual({ directos: true, resto: true });
  });

  test('sin almacenamiento no revienta', () => {
    (globalThis as { localStorage?: unknown }).localStorage = {
      getItem: () => {
        throw new Error('privado');
      },
      setItem: () => {
        throw new Error('privado');
      },
    };
    expect(() => guardarPreferenciasSonido({ directos: false, resto: false })).not.toThrow();
    expect(leerPreferenciasSonido()).toEqual({ directos: true, resto: false });
  });
});
