/**
 * El analizador de .env: lo único con ramas de verdad del importador.
 *
 * Importa variables de producción, así que lo que importa aquí no es que acierte
 * con el caso bonito, sino que NO se invente ninguna: una línea rara se ignora y
 * se cuenta, nunca se guarda a medias.
 */
import { expect, test } from 'bun:test';
import { parseDotenv } from '../src/components/EnvImport.js';

test('lee lo que trae un .env de verdad', () => {
  const { vars, skipped } = parseDotenv(
    [
      '# comentario',
      '',
      'DATABASE_URL=postgres://u:p@host:5432/db',
      'export API_KEY="abc123"',
      "TOKEN='con espacios y = dentro'",
      '  SPACED = valor  ',
    ].join('\n'),
  );
  expect(vars).toEqual([
    { key: 'DATABASE_URL', value: 'postgres://u:p@host:5432/db' },
    { key: 'API_KEY', value: 'abc123' },
    { key: 'TOKEN', value: 'con espacios y = dentro' },
    { key: 'SPACED', value: 'valor' },
  ]);
  expect(skipped).toBe(0);
});

test('ignora y CUENTA lo que no es una asignación usable', () => {
  const { vars, skipped } = parseDotenv(
    ['sin_igual', '=sin_clave', '9MAL=x', 'VACIA=', 'BIEN=1'].join('\n'),
  );
  expect(vars).toEqual([{ key: 'BIEN', value: '1' }]);
  // Las cuatro anteriores: se dicen, no desaparecen en silencio.
  expect(skipped).toBe(4);
});

test('la última repetida gana, como cualquier cargador de .env', () => {
  const { vars } = parseDotenv('A=1\nA=2\nB=3');
  expect(vars).toEqual([
    { key: 'A', value: '2' },
    { key: 'B', value: '3' },
  ]);
});

test('no expande variables ni interpreta escapes: el valor viaja tal cual', () => {
  const { vars } = parseDotenv('A=$OTRA\nB=linea\\ncon\\nbarras');
  expect(vars).toEqual([
    { key: 'A', value: '$OTRA' },
    { key: 'B', value: 'linea\\ncon\\nbarras' },
  ]);
});
