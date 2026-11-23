/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Subir fotos con el Drive en la bóveda (ADR 0086): cifradas en el navegador, a
 * «Fotos» del Drive de la bóveda, sin pisar ninguna y sin importar ni congelar nada
 * desde la galería. El repositorio es el de verdad (`PrivateDriveRepository`) sobre
 * el almacén sintético de sus propias pruebas.
 */
import { afterEach, expect, test } from 'bun:test';
import { DriveClient } from '@sytue/api-clients';
import {
  decodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import type { DriveCifrado, EstadoDriveCifrado } from './driveCifrado.js';
import { PrivateDriveError } from '@sytue/offline/matrix';
import { PqVaultError, type PqVault, type PqVaultOpenOptions } from '@sytue/offline/vault';
import {
  privateDriveWriteFixture,
  room,
  scope,
} from '../../../../packages/offline/src/matrix/private-drive-fixture.js';
import {
  AperturaDriveCifrado,
  MiniaturasCifradas,
  abrirDriveCifrado,
  carpetaFotos,
  destinoCifrado,
  driveEnBoveda,
  fotosCifradas,
  leerFotoCifrada,
  motivoDriveCifrado,
  nombreLibre,
} from './driveCifrado.js';
import { subirFotos } from './subida.js';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

const jpeg = (nombre: string, texto: string) =>
  new File([new TextEncoder().encode(texto)], nombre, { type: 'image/jpeg' });

test('el nombre libre es el de Go: «nombre (2).ext» y siguientes', () => {
  const cogidos = new Set(['IMG_0001.jpg', 'IMG_0001 (2).jpg', 'sin-extension']);
  expect(nombreLibre('playa.jpg', cogidos)).toBe('playa.jpg');
  expect(nombreLibre('IMG_0001.jpg', cogidos)).toBe('IMG_0001 (3).jpg');
  expect(nombreLibre('sin-extension', cogidos)).toBe('sin-extension (2)');
  expect(nombreLibre('.oculto', new Set(['.oculto']))).toBe('.oculto (2)');
});

test('sube cifrado a «Fotos», sin pisar la que tiene el mismo nombre, y se lee de vuelta', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    // Sin «Fotos», listar no la crea.
    expect(await fotosCifradas(repositorio)).toEqual([]);
    expect(await carpetaFotos(repositorio, false)).toBeNull();

    const destino = destinoCifrado({ repositorio, signal: new AbortController().signal });
    expect(await destino.subir(jpeg('IMG_0001.jpg', 'FOTO EN CLARO UNO'))).toBe('IMG_0001.jpg');
    expect(await destino.subir(jpeg('IMG_0001.jpg', 'FOTO EN CLARO DOS'))).toBe('IMG_0001 (2).jpg');

    const lista = await fotosCifradas(repositorio);
    expect(lista.map((x) => x.nombre).sort()).toEqual(['IMG_0001 (2).jpg', 'IMG_0001.jpg']);
    expect(lista.every((x) => x.tipo === 'image/jpeg')).toBe(true);
    const segunda = lista.find((x) => x.nombre === 'IMG_0001 (2).jpg')!;
    const blob = await leerFotoCifrada(repositorio, segunda);
    expect(blob.type).toBe('image/jpeg');
    expect(await blob.text()).toBe('FOTO EN CLARO DOS');

    // Al servidor solo llega texto cifrado.
    for (const cifrado of f.blobs.values())
      expect(new TextDecoder().decode(cifrado)).not.toContain('FOTO EN CLARO');

    // Un destino nuevo (otra sesión) ve los nombres que ya hay.
    const otro = destinoCifrado({ repositorio, signal: new AbortController().signal });
    expect(await otro.subir(jpeg('IMG_0001.jpg', 'TRES'))).toBe('IMG_0001 (3).jpg');
  } finally {
    await repositorio.close();
    await f.close();
  }
});

/** Una bóveda sintética sobre el almacén de la prueba, como la de Drive web. */
async function bovedaDePrueba(estado?: Record<string, unknown>) {
  const f = await privateDriveWriteFixture();
  const peticiones: { url: string; metodo: string }[] = [];
  const abiertas: PqVaultOpenOptions[] = [];
  let cerrada = false;
  const vault = {
    scope: { ...scope },
    vaultId: room,
    get isClosed() {
      return cerrada;
    },
    storage: () => f.repository(),
    close: () => {
      cerrada = true;
    },
    closeAndDrain: async () => {
      cerrada = true;
    },
  } as unknown as PqVault;
  const fuente = () =>
    new DriveClient({
      baseUrl: 'https://photos.example/photos/api/drive-cifrado',
      fetchImpl: Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: Parameters<typeof fetch>[1]) => {
          peticiones.push({ url: String(url), metodo: init?.method ?? 'GET' });
          expect(
            decodeStorageScopeHeader(new Headers(init?.headers).get(storageScopeHeader) ?? ''),
          ).toEqual(scope as MatrixStorageScope);
          return Response.json(estado ?? f.state);
        },
        { preconnect: () => {} },
      ),
    });
  const abrir = (o: PqVaultOpenOptions) => {
    abiertas.push(o);
    return Promise.resolve(vault);
  };
  return { f, peticiones, abiertas, abrir, fuente, cerrada: () => cerrada };
}

test('abre solo la bóveda que ya existe y con su recibo, leyendo el estado sin importar nada', async () => {
  const b = await bovedaDePrueba();
  try {
    const drive = await abrirDriveCifrado({
      actor: scope.subject,
      tenant: scope.tenant,
      abrir: b.abrir,
      fuente: b.fuente,
      base: '/photos/api/drive-cifrado',
    });
    expect(b.abiertas).toHaveLength(1);
    expect(b.abiertas[0]).toMatchObject({
      product: 'drive',
      existing: true,
      basePath: '/photos/api/drive-cifrado',
    });
    // Solo el estado de la fuente: ni empezar la migración ni leer filas en claro.
    expect(b.peticiones).toEqual([
      {
        url: 'https://photos.example/photos/api/drive-cifrado/api/matrix/storage/migration',
        metodo: 'GET',
      },
    ]);
    expect(await carpetaFotos(drive.repositorio, false)).toBeNull();
    await drive.cerrar();
    expect(b.cerrada()).toBe(true);
  } finally {
    await b.f.close();
  }
});

test('sin el recibo de la migración no abre: «pendiente», y lo deja todo cerrado', async () => {
  const b = await bovedaDePrueba();
  const sinRecibo = await bovedaDePrueba({
    ...b.f.state,
    epoch: '87654321-4321-4321-8321-cba987654321',
  });
  try {
    const fallo = await abrirDriveCifrado({
      actor: scope.subject,
      tenant: scope.tenant,
      abrir: sinRecibo.abrir,
      fuente: sinRecibo.fuente,
    }).catch((e: unknown) => e);
    expect(fallo).toBeInstanceOf(PrivateDriveError);
    expect(motivoDriveCifrado(fallo)).toBe('pendiente');
    expect(sinRecibo.cerrada()).toBe(true);
    expect(sinRecibo.peticiones.every((p) => p.metodo === 'GET')).toBe(true);
  } finally {
    await b.f.close();
    await sinRecibo.f.close();
  }
});

test('cada fallo de la bóveda tiene su motivo', () => {
  expect(motivoDriveCifrado(new PqVaultError('signin'))).toBe('entrar');
  expect(motivoDriveCifrado(new PqVaultError('missing'))).toBe('pendiente');
  expect(motivoDriveCifrado(new PrivateDriveError('not_ready'))).toBe('pendiente');
  expect(motivoDriveCifrado(new PqVaultError('wrong'))).toBe('fallo');
  expect(motivoDriveCifrado(new Error('x'))).toBe('fallo');
});

test('saber si el Drive está en la bóveda no pide claves y, ante la duda, dice que no', async () => {
  const estado = (mode: string) => () => ({
    privateStorageState: () => Promise.resolve({ mode } as never),
  });
  expect(await driveEnBoveda('u', 'acme', estado('frozen'))).toBe(true);
  expect(await driveEnBoveda('u', 'acme', estado('private'))).toBe(true);
  expect(await driveEnBoveda('u', 'acme', estado('legacy'))).toBe(false);
  expect(await driveEnBoveda('u', undefined, estado('frozen'))).toBe(false);
  const roto = () => ({ privateStorageState: () => Promise.reject(new Error('red')) });
  expect(await driveEnBoveda('u', 'acme', roto)).toBe(false);
});

function driveEnBovedaPorElBff(): string[] {
  const rutas: string[] = [];
  globalThis.fetch = (async (url: Parameters<typeof fetch>[0]) => {
    rutas.push(String(url));
    return Response.json(
      { error: 'drive_private_vault', message: 'El Drive de esta persona está en la bóveda.' },
      { status: 409 },
    );
  }) as unknown as typeof fetch;
  return rutas;
}

test('con el 409 de la bóveda, esa foto y las siguientes van cifradas y no al índice', async () => {
  const rutas = driveEnBovedaPorElBff();
  const subidas: string[] = [];
  let aperturas = 0;
  const resultado = await subirFotos([jpeg('a.jpg', 'a'), jpeg('b.jpg', 'b')], () => {}, {
    cifrado: () => {
      aperturas++;
      return Promise.resolve({
        subir: (f: File) => {
          subidas.push(f.name);
          return Promise.resolve();
        },
      });
    },
  });
  expect(resultado).toEqual({ fotos: [], fallos: [], cifradas: ['a.jpg', 'b.jpg'] });
  expect(subidas).toEqual(['a.jpg', 'b.jpg']);
  expect(aperturas).toBe(1);
  // Una sola vez al BFF: tras el 409 no se vuelve a intentar en claro, ni se indexa.
  expect(rutas).toHaveLength(1);
  expect(rutas[0]).toContain('/api/drive-subir');
});

test('si la bóveda no abre, todas fallan con su motivo y no se reintenta abrirla', async () => {
  driveEnBovedaPorElBff();
  let aperturas = 0;
  const resultado = await subirFotos([jpeg('a.jpg', 'a'), jpeg('b.jpg', 'b')], () => {}, {
    cifrado: () => {
      aperturas++;
      return Promise.reject(new PqVaultError('signin'));
    },
  });
  expect(resultado.fallos).toEqual(['a.jpg', 'b.jpg']);
  expect(resultado.cifradas).toEqual([]);
  expect(resultado.motivoCifrado).toBe('entrar');
  expect(aperturas).toBe(1);
});

function driveFalso() {
  let cerrado = 0;
  const drive = {
    repositorio: {},
    signal: new AbortController().signal,
    cerrar: () => {
      cerrado++;
      return Promise.resolve();
    },
  } as unknown as DriveCifrado;
  return { drive, cerrado: () => cerrado };
}

test('una sola apertura a la vez, la misma para todos, y cerrar la suelta', async () => {
  const { drive, cerrado } = driveFalso();
  const estados: EstadoDriveCifrado['fase'][] = [];
  let aperturas = 0;
  const apertura = new AperturaDriveCifrado(
    () => {
      aperturas++;
      return Promise.resolve(drive);
    },
    (e) => estados.push(e.fase),
  );
  const [a, b] = await Promise.all([apertura.abrir(), apertura.abrir()]);
  expect(a).toBe(drive);
  expect(b).toBe(drive);
  expect(await apertura.abrir()).toBe(drive);
  expect(aperturas).toBe(1);
  expect(estados).toEqual(['abriendo', 'abierto']);
  await apertura.cerrar();
  expect(cerrado()).toBe(1);
  await expect(apertura.abrir()).rejects.toThrow('drive_private_closed');
});

test('lo que termina de abrirse después de cerrar se suelta, sin avisar a nadie', async () => {
  const { drive, cerrado } = driveFalso();
  let soltar!: (d: DriveCifrado) => void;
  const estados: EstadoDriveCifrado['fase'][] = [];
  const apertura = new AperturaDriveCifrado(
    () => new Promise<DriveCifrado>((r) => (soltar = r)),
    (e) => estados.push(e.fase),
  );
  const pendiente = apertura.abrir().catch((e: unknown) => e);
  await apertura.cerrar();
  soltar(drive);
  expect(await pendiente).toBeInstanceOf(PrivateDriveError);
  expect(cerrado()).toBe(1);
  expect(estados).toEqual(['abriendo']);
});

test('un fallo al abrir deja su motivo y se puede volver a intentar', async () => {
  const { drive } = driveFalso();
  const estados: EstadoDriveCifrado[] = [];
  let vez = 0;
  const apertura = new AperturaDriveCifrado(
    () => (++vez === 1 ? Promise.reject(new PqVaultError('signin')) : Promise.resolve(drive)),
    (e) => estados.push(e),
  );
  await expect(apertura.abrir()).rejects.toBeInstanceOf(PqVaultError);
  expect(estados.at(-1)).toEqual({ fase: 'fallo', motivo: 'entrar' });
  expect(await apertura.abrir()).toBe(drive);
  expect(estados.at(-1)).toEqual({ fase: 'abierto', drive });
});

test('las miniaturas descifradas se sueltan al cerrarse su Drive, y después no se guardan más', () => {
  const vida = new AbortController();
  const miniaturas = new MiniaturasCifradas(vida.signal);
  miniaturas.poner('a', new Blob(['x'], { type: 'image/jpeg' }));
  expect(miniaturas.tiene('a')).toBe(true);
  expect(Object.keys(miniaturas.todas())).toEqual(['a']);
  vida.abort();
  expect(miniaturas.todas()).toEqual({});
  miniaturas.poner('b', new Blob(['y']));
  expect(miniaturas.tiene('b')).toBe(false);
});
