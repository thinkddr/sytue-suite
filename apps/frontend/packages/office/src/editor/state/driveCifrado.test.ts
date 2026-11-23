/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Guardar en Drive» con el Drive en la bóveda (ADR 0086): el fichero del documento
 * se cifra en el navegador y va al Drive de la bóveda; al servicio de Docs solo le
 * llega el ancla. El repositorio es el de verdad (`PrivateDriveRepository`) sobre el
 * almacén sintético de sus propias pruebas, como en Photos.
 */
import { afterEach, expect, test } from 'bun:test';
import { DriveClient } from '@sytue/api-clients';
import {
  decodeStorageScopeHeader,
  storageScopeHeader,
  type MatrixStorageScope,
} from '@sytue/config/matrix';
import { PrivateDriveError, type PrivateDriveRepository } from '@sytue/offline/matrix';
import { PqVaultError, type PqVault, type PqVaultOpenOptions } from '@sytue/offline/vault';
import { collectPrivateDriveFile } from '@sytue/offline/vault/drive';
import {
  privateDriveWriteFixture,
  room,
  scope,
} from '../../../../offline/src/matrix/private-drive-fixture.js';
import type { ExportFormat } from '../types';
import { anclaDeBoveda, esAnclaDeBoveda, leerAncla, sePuedeAnclarEnBoveda } from './anclaDrive';
import {
  CARPETA_DOCUMENTOS,
  FormatoSinVuelta,
  abrirDriveCifrado,
  driveEnBoveda,
  guardarDocumentoEnBoveda,
  guardarEnDriveCifrado,
  motivoDriveCifrado,
  type DriveCifradoAbierto,
} from './driveCifrado';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

const UNO = '0b5e7a1c-3f2d-4e6b-9a8c-1d2e3f4a5b6c';

test('las dos anclas no se confunden, y solo se sustituye la de la bóveda', () => {
  expect(leerAncla(`drive:${UNO}`)).toEqual({ donde: 'servidor', id: UNO });
  expect(leerAncla(`vault:${UNO}`)).toEqual({ donde: 'boveda', id: UNO });
  expect(leerAncla('')).toBeNull();
  expect(leerAncla('vault:../x')).toBeNull();
  expect(leerAncla(`vault:${UNO.toUpperCase()}`)).toBeNull();
  expect(leerAncla(`otra:${UNO}`)).toBeNull();
  expect(anclaDeBoveda(UNO)).toBe(`vault:${UNO}`);
  expect(() => anclaDeBoveda('raiz')).toThrow();
  expect(esAnclaDeBoveda(`vault:${UNO}`)).toBe(true);
  expect(esAnclaDeBoveda(`drive:${UNO}`)).toBe(false);
  expect(sePuedeAnclarEnBoveda('')).toBe(true);
  expect(sePuedeAnclarEnBoveda(`vault:${UNO}`)).toBe(true);
  expect(sePuedeAnclarEnBoveda(`drive:${UNO}`)).toBe(false);
});

/** El texto en claro que sale de «exportar», y qué formatos se pidieron. */
function exportador(textoEnClaro: string) {
  const pedidos: ExportFormat[] = [];
  const exportar = (formato: ExportFormat) => {
    pedidos.push(formato);
    return Promise.resolve(
      new Blob([new TextEncoder().encode(`${textoEnClaro} (${formato})`)], {
        type: formato === 'odf' ? 'application/vnd.oasis.opendocument.text' : 'application/x-doc',
      }),
    );
  };
  return { pedidos, exportar };
}

async function leer(repositorio: PrivateDriveRepository, id: string): Promise<string> {
  const fila = await repositorio.file(id);
  const blob = await collectPrivateDriveFile({
    size: fila.size_bytes,
    maximum: 1 << 20,
    type: 'application/octet-stream',
    run: (sink) => repositorio.download(id, sink),
  });
  return blob.text();
}

const texto = (fila: Record<string, unknown>, clave: string) => String(fila[clave] ?? '');

async function hijosDe(repositorio: PrivateDriveRepository, nombre: string) {
  const raiz = texto(await repositorio.root(), 'id');
  const carpeta = (await repositorio.children(raiz)).folders.find(
    (c) => texto(c, 'name') === nombre,
  );
  if (!carpeta) return [];
  return (await repositorio.children(texto(carpeta, 'id'))).files;
}

test('sin ancla: un fichero nuevo y cifrado en «Sytue Docs», y el dueño ancla el documento a él', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    const abrir = (): Promise<DriveCifradoAbierto> =>
      Promise.resolve({
        repositorio,
        signal: new AbortController().signal,
        cerrar: () => Promise.resolve(),
      });
    const anclas: [string, string][] = [];
    const primero = exportador('ACTA EN CLARO UNO');
    const r = await guardarDocumentoEnBoveda({
      ancla: '',
      esDueno: true,
      titulo: 'Acta',
      kind: 'document',
      abrir,
      exportar: primero.exportar,
      anclar: (nueva, antes) => {
        anclas.push([nueva, antes]);
        return Promise.resolve(true);
      },
    });
    expect(primero.pedidos).toEqual(['odf']);
    expect(r.nombre).toBe('Acta.odt');
    expect(r.nuevo).toBe(true);
    expect(esAnclaDeBoveda(r.ancla)).toBe(true);
    expect(anclas).toEqual([[r.ancla, '']]);
    expect(r.anclaDelDocumento).toBe(r.ancla);
    const id = leerAncla(r.ancla)!.id;
    expect(await leer(repositorio, id)).toBe('ACTA EN CLARO UNO (odf)');
    expect((await hijosDe(repositorio, CARPETA_DOCUMENTOS)).map((x) => texto(x, 'name'))).toEqual([
      'Acta.odt',
    ]);

    // Guardar otra vez con el ancla: versión nueva del MISMO fichero, sin volver a anclar.
    const segundo = exportador('ACTA EN CLARO DOS');
    const otra = await guardarDocumentoEnBoveda({
      ancla: r.ancla,
      esDueno: true,
      titulo: 'Acta renombrada',
      kind: 'document',
      abrir,
      exportar: segundo.exportar,
      anclar: () => Promise.reject(new Error('no debe anclar')),
    });
    expect(otra).toMatchObject({ nombre: 'Acta.odt', ancla: r.ancla, nuevo: false });
    expect(segundo.pedidos).toEqual(['odf']);
    expect(await leer(repositorio, id)).toBe('ACTA EN CLARO DOS (odf)');
    expect((await repositorio.file(id)).version).toBe(2);
    expect(await hijosDe(repositorio, CARPETA_DOCUMENTOS)).toHaveLength(1);

    // Al servidor solo llega texto cifrado.
    for (const cifrado of f.blobs.values())
      expect(new TextDecoder().decode(cifrado)).not.toContain('EN CLARO');
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('un nombre ya cogido en «Sytue Docs» no se pisa', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    const base = { repositorio, titulo: 'Ventas', kind: 'spreadsheet' as const };
    const a = await guardarEnDriveCifrado({
      ...base,
      ancla: '',
      exportar: exportador('A').exportar,
    });
    const b = await guardarEnDriveCifrado({
      ...base,
      ancla: '',
      exportar: exportador('B').exportar,
    });
    expect([a.nombre, b.nombre]).toEqual(['Ventas.ods', 'Ventas (2).ods']);
    expect(a.ancla).not.toBe(b.ancla);
    expect(await leer(repositorio, leerAncla(a.ancla)!.id)).toBe('A (odf)');
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('«drive:<id>» que pasó a la bóveda con su id: versión nueva de ese fichero, en su formato y sin cambiar el ancla', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    const raiz = texto(await repositorio.root(), 'id');
    const trabajo = texto(await repositorio.createFolder(raiz, 'Trabajo'), 'id');
    const fila = await repositorio.upload(
      trabajo,
      'Informe.docx',
      'application/x-doc',
      new Blob(['viejo']),
    );
    const ancla = `drive:${texto(fila, 'id')}`;
    const e = exportador('INFORME NUEVO');
    const r = await guardarDocumentoEnBoveda({
      ancla,
      esDueno: true,
      titulo: 'Otro título',
      kind: 'document',
      abrir: () =>
        Promise.resolve({
          repositorio,
          signal: new AbortController().signal,
          cerrar: () => Promise.resolve(),
        }),
      exportar: e.exportar,
      anclar: () => Promise.reject(new Error('no debe anclar')),
    });
    expect(e.pedidos).toEqual(['ooxml']);
    expect(r).toMatchObject({
      nombre: 'Informe.docx',
      ancla,
      nuevo: false,
      anclaDelDocumento: ancla,
    });
    expect(await leer(repositorio, texto(fila, 'id'))).toBe('INFORME NUEVO (ooxml)');
    expect(await hijosDe(repositorio, CARPETA_DOCUMENTOS)).toEqual([]);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('«drive:<id>» que no está en la bóveda: fichero nuevo, pero el ancla del servidor no se sustituye', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    const ancla = `drive:${UNO}`;
    const r = await guardarDocumentoEnBoveda({
      ancla,
      esDueno: true,
      titulo: 'Acta',
      kind: 'presentation',
      abrir: () =>
        Promise.resolve({
          repositorio,
          signal: new AbortController().signal,
          cerrar: () => Promise.resolve(),
        }),
      exportar: exportador('X').exportar,
      anclar: () => Promise.reject(new Error('no debe anclar')),
    });
    expect(r.nombre).toBe('Acta.odp');
    expect(r.nuevo).toBe(true);
    expect(r.anclaDelDocumento).toBe(ancla);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('quien no es el dueño guarda una copia en SU Drive cifrado, sin anclar el documento', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  let cerrada = false;
  try {
    await repositorio.initialize(f.state);
    const r = await guardarDocumentoEnBoveda({
      ancla: '',
      esDueno: false,
      titulo: 'Acta',
      kind: 'document',
      abrir: () =>
        Promise.resolve({
          repositorio,
          signal: new AbortController().signal,
          cerrar: () => {
            cerrada = true;
            return Promise.resolve();
          },
        }),
      exportar: exportador('X').exportar,
      anclar: () => Promise.reject(new Error('no debe anclar')),
    });
    expect(r.anclaDelDocumento).toBe('');
    expect(cerrada).toBe(true);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('un fichero anclado sin ida y vuelta (.pdf) no se reescribe, y se suelta la bóveda', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  let cerrada = false;
  try {
    await repositorio.initialize(f.state);
    const raiz = texto(await repositorio.root(), 'id');
    const fila = await repositorio.upload(raiz, 'Plano.pdf', 'application/pdf', new Blob(['%PDF']));
    const e = exportador('X');
    const fallo = await guardarDocumentoEnBoveda({
      ancla: `vault:${texto(fila, 'id')}`,
      esDueno: true,
      titulo: 'Plano',
      kind: 'document',
      abrir: () =>
        Promise.resolve({
          repositorio,
          signal: new AbortController().signal,
          cerrar: () => {
            cerrada = true;
            return Promise.resolve();
          },
        }),
      exportar: e.exportar,
      anclar: () => Promise.resolve(true),
    }).catch((x: unknown) => x);
    expect(fallo).toBeInstanceOf(FormatoSinVuelta);
    expect(motivoDriveCifrado(fallo)).toBe('formato');
    expect(e.pedidos).toEqual([]);
    expect(cerrada).toBe(true);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('si el servicio no deja anclar, lo guardado se queda y el documento sigue como estaba', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    const r = await guardarDocumentoEnBoveda({
      ancla: '',
      esDueno: true,
      titulo: 'Acta',
      kind: 'document',
      abrir: () =>
        Promise.resolve({
          repositorio,
          signal: new AbortController().signal,
          cerrar: () => Promise.resolve(),
        }),
      exportar: exportador('X').exportar,
      anclar: () => Promise.reject(new Error('sin red')),
    });
    expect(r.nuevo).toBe(true);
    expect(r.anclaDelDocumento).toBe('');
    expect(await hijosDe(repositorio, CARPETA_DOCUMENTOS)).toHaveLength(1);
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
      baseUrl: 'https://docus.example/api/drive-cifrado',
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

test('abre solo la bóveda de Drive que ya existe, con su recibo, sin importar nada', async () => {
  const b = await bovedaDePrueba();
  try {
    const drive = await abrirDriveCifrado({
      actor: scope.subject,
      tenant: scope.tenant,
      abrir: b.abrir,
      fuente: b.fuente,
      base: '/api/drive-cifrado',
    });
    expect(b.abiertas).toHaveLength(1);
    expect(b.abiertas[0]).toMatchObject({
      product: 'drive',
      existing: true,
      basePath: '/api/drive-cifrado',
    });
    expect(b.peticiones).toEqual([
      {
        url: 'https://docus.example/api/drive-cifrado/api/matrix/storage/migration',
        metodo: 'GET',
      },
    ]);
    const r = await guardarEnDriveCifrado({
      repositorio: drive.repositorio,
      ancla: '',
      titulo: 'Acta',
      kind: 'document',
      exportar: exportador('ACTA').exportar,
      signal: drive.signal,
    });
    expect(r.nombre).toBe('Acta.odt');
    // Lo que se subió es texto cifrado, por la puerta de bloques de la bóveda.
    for (const cifrado of b.f.blobs.values())
      expect(new TextDecoder().decode(cifrado)).not.toContain('ACTA');
    await drive.cerrar();
    expect(b.cerrada()).toBe(true);
  } finally {
    await b.f.close();
  }
});

test('sin el recibo de la migración no abre: «pendiente», y lo deja cerrado', async () => {
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
  } finally {
    await b.f.close();
    await sinRecibo.f.close();
  }
});

test('cada fallo de la bóveda tiene su motivo', () => {
  expect(motivoDriveCifrado(new PqVaultError('signin'))).toBe('entrar');
  expect(motivoDriveCifrado(new PqVaultError('missing'))).toBe('pendiente');
  expect(motivoDriveCifrado(new PrivateDriveError('not_ready'))).toBe('pendiente');
  expect(motivoDriveCifrado(new FormatoSinVuelta())).toBe('formato');
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
