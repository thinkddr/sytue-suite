import { describe, expect, it, beforeEach } from 'bun:test';
import {
  obtenerFondoLienzo,
  guardarFondoLienzo,
  estilosFondoLienzo,
  CLAVE_STORAGE_FONDO,
  FONDO_POR_DEFECTO,
  type FondoLienzo,
} from './fondoLienzo.js';

class MockStorage implements Storage {
  private store = new Map<string, string>();
  get length() {
    return this.store.size;
  }
  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null;
  }
  getItem(key: string): string | null {
    return this.store.get(key) ?? null;
  }
  setItem(key: string, value: string): void {
    this.store.set(key, String(value));
  }
  removeItem(key: string): void {
    this.store.delete(key);
  }
  clear(): void {
    this.store.clear();
  }
}

describe('fondoLienzo', () => {
  beforeEach(() => {
    (globalThis as unknown as { localStorage: Storage }).localStorage = new MockStorage();
  });

  it('devuelve el valor por defecto crema si no hay nada guardado', () => {
    expect(obtenerFondoLienzo()).toBe(FONDO_POR_DEFECTO);
    expect(FONDO_POR_DEFECTO).toBe('crema');
  });

  it('guarda y recupera opciones válidas en localStorage', () => {
    const opciones: FondoLienzo[] = ['blanco', 'oscuro', 'damero', 'crema'];
    for (const op of opciones) {
      guardarFondoLienzo(op);
      expect(localStorage.getItem(CLAVE_STORAGE_FONDO)).toBe(op);
      expect(obtenerFondoLienzo()).toBe(op);
    }
  });

  it('devuelve crema ante valores corruptos o desconocidos en localStorage', () => {
    localStorage.setItem(CLAVE_STORAGE_FONDO, 'rojo-fosforito');
    expect(obtenerFondoLienzo()).toBe('crema');

    localStorage.setItem(CLAVE_STORAGE_FONDO, '');
    expect(obtenerFondoLienzo()).toBe('crema');
  });

  it('proporciona estilos y colores diferenciados para cada modo', () => {
    const crema = estilosFondoLienzo('crema');
    expect(crema.style.backgroundColor).toBe('#faf9f6');

    const blanco = estilosFondoLienzo('blanco');
    expect(blanco.style.backgroundColor).toBe('#ffffff');

    const oscuro = estilosFondoLienzo('oscuro');
    expect(oscuro.style.backgroundColor).toBe('#131619');

    const damero = estilosFondoLienzo('damero');
    expect(damero.style.backgroundImage).toContain('repeating-conic-gradient');
    expect(damero.style.backgroundSize).toBe('16px 16px');
  });
});
