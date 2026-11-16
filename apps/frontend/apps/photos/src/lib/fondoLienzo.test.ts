import { describe, expect, it, beforeEach } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  obtenerFondoLienzo,
  guardarFondoLienzo,
  estilosFondoLienzo,
  clasesMuestraFondo,
  FONDOS_DISPONIBLES,
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
    expect(crema.style.backgroundColor).toBe('var(--sytue-neutral-100)');
    expect(crema.className).toBe('bg-neutral-100');

    const blanco = estilosFondoLienzo('blanco');
    expect(blanco.style.backgroundColor).toBe('var(--sytue-neutral-0)');

    const oscuro = estilosFondoLienzo('oscuro');
    expect(oscuro.style.backgroundColor).toBe('var(--sytue-neutral-900)');

    const damero = estilosFondoLienzo('damero');
    expect(damero.style.backgroundImage).toContain('repeating-conic-gradient');
    expect(damero.style.backgroundImage).toContain('var(--sytue-neutral-200)');
    expect(damero.style.backgroundSize).toBe('16px 16px');
  });

  // La paleta antigua (el crema `#faf9f6`, el grafito `#131619`) y los grises por
  // defecto de Tailwind no siguen al color de la suite: todo sale de los neutros.
  const SIN_PALETA = /#[0-9a-f]{3,8}\b|rgba?\(|\b(gray|slate|zinc)-\d|bg-white\b/i;

  it('pinta cada fondo y su muestra solo con tokens de la paleta', () => {
    for (const fondo of FONDOS_DISPONIBLES) {
      const { className, style } = estilosFondoLienzo(fondo);
      expect(className).not.toMatch(SIN_PALETA);
      for (const valor of Object.values(style)) expect(valor).not.toMatch(SIN_PALETA);
      expect(style.backgroundColor).toMatch(/^var\(--sytue-neutral-\d+\)$/);

      const muestra = clasesMuestraFondo(fondo);
      expect(muestra).not.toMatch(SIN_PALETA);
      expect(muestra).toContain(className);
      // El borde, también un neutro (sin escribir la clase entera: el guardián de
      // tokens de `lint:design` leería aquí un token a medias).
      const borde = muestra.split(' ').find((c) => c.startsWith('border-'));
      expect(borde).toMatch(/^border-[a-z]+-\d+$/);
      expect(borde).toContain('neutral');
    }
  });

  it('la muestra no se copia en el visor ni en el editor', () => {
    for (const fichero of ['../components/PhotosApp.tsx', '../components/Editor.tsx']) {
      const fuente = readFileSync(new URL(fichero, import.meta.url), 'utf8');
      expect(fuente).not.toMatch(/#faf9f6|#131619|#d8d3c6|#cbd5e1/i);
      expect(fuente).not.toContain("['crema', 'blanco', 'oscuro', 'damero']");
      expect(fuente).toContain('<MuestraFondo');
    }
  });
});
