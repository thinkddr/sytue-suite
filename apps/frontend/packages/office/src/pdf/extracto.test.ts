import { describe, expect, it } from 'bun:test';
import { extractoLegible, sinCapaDeTexto } from './extracto.js';

describe('extractoLegible', () => {
  it('deja pasar texto real, con tildes y números', () => {
    expect(extractoLegible('Impreso de matrícula · Curso 2026/2027, 1er apellido')).not.toBeNull();
  });
  it('descarta la capa de texto con la tabla de caracteres rota', () => {
    // Lo que daba el PDF del formulario de matrícula, tal cual.
    expect(extractoLegible(",035(62 '( 0$75Ê&8/$ &,&/2 '( *5$'2 0(',2 *5$'2 6")).toBeNull();
  });
  it('descarta lo vacío o casi vacío', () => {
    expect(extractoLegible('')).toBeNull();
    expect(extractoLegible(' a ')).toBeNull();
    expect(extractoLegible(undefined)).toBeNull();
  });
});

describe('sinCapaDeTexto', () => {
  // Abrir ya no espera a leer el texto de todas las páginas: de algunas no se sabe.
  it('una página sin leer no convierte el documento en un escaneo', () => {
    expect(sinCapaDeTexto([{ extracto: 'Contrato' }, {}, {}])).toBe(false);
    expect(sinCapaDeTexto([{ extracto: '' }, {}, {}])).toBe(true);
    expect(sinCapaDeTexto([{ extracto: '' }, { extracto: 'Anexo' }])).toBe(false);
  });
  it('sin ninguna leída no se afirma nada', () => {
    expect(sinCapaDeTexto([{}, {}])).toBe(false);
    expect(sinCapaDeTexto([])).toBe(false);
  });
});
