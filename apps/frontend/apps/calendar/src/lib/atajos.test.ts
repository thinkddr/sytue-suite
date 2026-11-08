/**
 * Los atajos de Calendar salen de UNA tabla: lo que hace cada tecla y lo que enseña
 * la hoja de «?». Aquí se vigila que «?» abre la hoja, que las de siempre siguen
 * haciendo lo suyo y que la hoja no promete una tecla que el oyente no atiende.
 */
import { describe, expect, test } from 'bun:test';
import { accionDeTecla, gruposDeAtajos } from './atajos.js';
import { useT } from '../i18n/index.js';

describe('atajos de Calendar', () => {
  test('«?» abre la hoja de atajos', () => {
    expect(accionDeTecla('?')).toBe('help');
  });

  test('las teclas de siempre, en mayúscula o minúscula', () => {
    expect(accionDeTecla('t')).toBe('today');
    expect(accionDeTecla('M')).toBe('month');
    expect(accionDeTecla('j')).toBe('next');
    expect(accionDeTecla('k')).toBe('prev');
    expect(accionDeTecla('c')).toBe('create');
    expect(accionDeTecla('/')).toBe('search');
    expect(accionDeTecla('x')).toBeNull();
    expect(accionDeTecla('Enter')).toBeNull();
  });

  test('la hoja enseña cada tecla que el oyente atiende, traducida', () => {
    const t = useT('es');
    const grupos = gruposDeAtajos(t);
    expect(grupos.map((g) => g.titulo)).toEqual(['Moverse', 'Vistas', 'Acciones']);
    const teclas = grupos.flatMap((g) => g.atajos.flatMap((a) => a.teclas));
    for (const k of teclas) expect(accionDeTecla(k)).not.toBeNull();
    expect(teclas).toContain('?');
    const ayuda = grupos.flatMap((g) => g.atajos).find((a) => a.teclas.includes('?'));
    expect(ayuda?.descripcion).toBe('Ver esta lista');
  });
});
