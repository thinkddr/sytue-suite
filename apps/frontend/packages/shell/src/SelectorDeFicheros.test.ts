import { describe, expect, it } from 'bun:test';
import { SelectorDeFicheros } from './SelectorDeFicheros.js';

describe('SelectorDeFicheros', () => {
  it('se exporta correctamente como componente Preact', () => {
    expect(typeof SelectorDeFicheros).toBe('function');
  });
});
