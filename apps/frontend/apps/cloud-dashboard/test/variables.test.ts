/**
 * Filtro de variables, plantilla `.env.example` y orden de proyectos.
 *
 * La plantilla es lo que se sube al repositorio: lo que importa es que NO lleve
 * valores, ni siquiera los que no son secretos (que suelen ser de un entorno).
 */
import { expect, test } from 'bun:test';
import { filtrarVariables, plantillaEnvExample } from '../src/lib/variables.js';
import { ordenarProyectos } from '../src/lib/proyectos.js';
import type { EnvVar, Project } from '../src/lib/types.js';

function v(p: Partial<EnvVar> & Pick<EnvVar, 'key'>): EnvVar {
  return {
    preview: 'valor-real',
    secret: false,
    scopes: ['production', 'preview'],
    source: 'manual',
    updatedAt: '2026-11-01T00:00:00Z',
    ...p,
  };
}

const VARS = [
  v({ key: 'STRIPE_KEY', secret: true, scopes: ['production'] }),
  v({ key: 'API_URL', preview: 'https://api.ejemplo.com' }),
  v({ key: 'PREVIEW_TOKEN', scopes: ['preview'] }),
  v({ key: 'DATABASE_URL', source: 'postgres' }),
];

test('filtra por nombre sin mayúsculas y por ámbito', () => {
  expect(filtrarVariables(VARS, 'stripe', '').map((x) => x.key)).toEqual(['STRIPE_KEY']);
  expect(filtrarVariables(VARS, '', 'preview').map((x) => x.key)).toEqual([
    'API_URL',
    'PREVIEW_TOKEN',
    'DATABASE_URL',
  ]);
  expect(filtrarVariables(VARS, '', 'managed').map((x) => x.key)).toEqual(['DATABASE_URL']);
  expect(filtrarVariables(VARS, '', '')).toBe(VARS);
});

test('la plantilla lleva las claves, nunca los valores, y separa las gestionadas', () => {
  const texto = plantillaEnvExample(VARS, {
    cabecera: 'Variables de web',
    soloProduccion: 'solo producción',
    soloPreview: 'solo previews',
    gestionadas: 'Las pone un servicio',
  });
  expect(texto).not.toContain('valor-real');
  expect(texto).not.toContain('api.ejemplo.com');
  expect(texto).toBe(
    [
      '# Variables de web',
      '',
      'API_URL=',
      'PREVIEW_TOKEN= # solo previews',
      'STRIPE_KEY= # solo producción',
      '',
      '# Las pone un servicio',
      'DATABASE_URL=',
      '',
    ].join('\n'),
  );
});

test('los proyectos se ordenan con la colación del idioma y los pausados pueden ir arriba', () => {
  const p = (name: string, createdAt: string, paused = false): Project =>
    ({ slug: name.toLowerCase(), name, createdAt, paused }) as Project;
  const lista = [
    p('Zeta', '2026-01-03T00:00:00Z'),
    p('Árbol', '2026-01-01T00:00:00Z', true),
    p('beta', '2026-01-02T00:00:00Z'),
  ];
  expect(ordenarProyectos(lista, 'name', 'es').map((x) => x.name)).toEqual([
    'Árbol',
    'beta',
    'Zeta',
  ]);
  expect(ordenarProyectos(lista, 'recent', 'es').map((x) => x.name)).toEqual([
    'Zeta',
    'beta',
    'Árbol',
  ]);
  expect(ordenarProyectos(lista, 'paused', 'es').map((x) => x.name)).toEqual([
    'Árbol',
    'beta',
    'Zeta',
  ]);
  // No reordena la lista que recibe.
  expect(lista[0]!.name).toBe('Zeta');
});
