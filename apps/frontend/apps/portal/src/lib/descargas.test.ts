// La ruta de descarga tiene que llevar el prefijo de la aplicación.
//
// El portal se sirve con `base: '/one'`. Sin el prefijo, el enlace se sale de la
// app: en el host de organización nadie enruta `/api/descargas/…` y Traefik
// contesta un 404 en texto plano. Pasó, y no lo cazó nada porque solo se ve
// entrando a la página y pulsando el botón.
import { expect, test } from 'bun:test';
import {
  arquitecturaDe,
  clasificar,
  equipoDelUA,
  hrefDe,
  sirveEn,
  productoDe,
  resolverEndpointDescargas,
  esInterno,
} from './descargas.js';

test('lleva el prefijo de la aplicación', () => {
  expect(hrefDe('SytueOne-macos-universal.dmg', '/one')).toBe(
    '/one/api/descargas/SytueOne-macos-universal.dmg',
  );
});

test('sin prefijo, la ruta se queda en la raíz', () => {
  expect(hrefDe('SytueOne-macos-universal.dmg', '/')).toBe(
    '/api/descargas/SytueOne-macos-universal.dmg',
  );
});

test('el nombre va escapado: un espacio no puede partir la URL', () => {
  expect(hrefDe('Sytue One.dmg', '/one')).toBe('/one/api/descargas/Sytue%20One.dmg');
});

test('resolverEndpointDescargas: privado siempre usa el endpoint interno', () => {
  const interno = 'http://seaweedfs.sytue-data.svc.cluster.local:8333';
  expect(
    resolverEndpointDescargas(false, {
      SYTUE_DOWNLOADS_ENDPOINT: interno,
      SYTUE_DOWNLOADS_PUBLIC_ENDPOINT: 'https://downloads.sytue.eu',
    }),
  ).toBe(interno);
});

test('resolverEndpointDescargas: público con endpoint público configurado lo usa', () => {
  const publico = 'https://downloads.sytue.eu';
  expect(
    resolverEndpointDescargas(true, {
      SYTUE_DOWNLOADS_ENDPOINT: 'http://seaweedfs.sytue-data.svc.cluster.local:8333',
      SYTUE_DOWNLOADS_PUBLIC_ENDPOINT: publico,
    }),
  ).toBe(publico);
});

test('resolverEndpointDescargas: público sin endpoint público o con cadena vacía cae al interno y NUNCA a vacío', () => {
  const interno = 'http://seaweedfs.sytue-data.svc.cluster.local:8333';
  // SYTUE_DOWNLOADS_PUBLIC_ENDPOINT undefined
  expect(
    resolverEndpointDescargas(true, {
      SYTUE_DOWNLOADS_ENDPOINT: interno,
    }),
  ).toBe(interno);

  // SYTUE_DOWNLOADS_PUBLIC_ENDPOINT "" (cadena vacía inyectada por Helm cuando no hay valor)
  expect(
    resolverEndpointDescargas(true, {
      SYTUE_DOWNLOADS_ENDPOINT: interno,
      SYTUE_DOWNLOADS_PUBLIC_ENDPOINT: '',
    }),
  ).toBe(interno);

  // Espacios en blanco
  expect(
    resolverEndpointDescargas(true, {
      SYTUE_DOWNLOADS_ENDPOINT: interno,
      SYTUE_DOWNLOADS_PUBLIC_ENDPOINT: '   ',
    }),
  ).toBe(interno);
});

test('esInterno: detecta hosts internos de cluster, IPs privadas y evita redirección rota', () => {
  expect(
    esInterno(
      'http://seaweedfs.sytue-data.svc.cluster.local:8333/sytue-eu-descargas/latest/SytueDrive-macos-universal.dmg',
    ),
  ).toBe(true);
  expect(esInterno('http://localhost:8333/test.dmg')).toBe(true);
  expect(esInterno('http://127.0.0.1:8333/test.dmg')).toBe(true);
  expect(esInterno('http://10.42.0.15:8333/test.dmg')).toBe(true);
  expect(esInterno('http://192.168.1.37:8333/test.dmg')).toBe(true);
  expect(esInterno('https://s3.eu-local-1.amazonaws.com/test.dmg')).toBe(true);

  // Endpoints públicos legítimos no son internos
  expect(esInterno('https://s3.fr-par.scw.cloud/sytue-prod-downloads/latest/test.dmg')).toBe(false);
  expect(esInterno('https://downloads.sytue.eu/latest/test.dmg')).toBe(false);
});

// Los nombres que publica apps/fyne/scripts/empaquetar.sh: `sytue-<app>-<plataforma>…`,
// Linux en .tar.xz y, en Windows, el instalador aparte del .exe suelto.
test('clasificar: reconoce lo que empaqueta Fyne y deja fuera el .exe suelto', () => {
  expect(clasificar('sytue-calcus-linux-amd64.tar.xz')).toBe('linux');
  expect(clasificar('sytue-calcus-windows-amd64-installer.exe')).toBe('windows');
  expect(clasificar('sytue-calcus-macos-universal.dmg')).toBe('macos');
  expect(clasificar('sytue-calcus-windows-amd64.exe')).toBeNull();
});

test('productoDe: el nombre de marca sale del catálogo, no de «Sytue» a secas', () => {
  expect(productoDe('sytue-calcus-windows-amd64-installer.exe')).toBe('Sytue Calcus');
  expect(productoDe('sytue-hub-linux-amd64.tar.xz')).toBe('Sytue Hub');
  expect(productoDe('sytue-notes-macos-universal.dmg')).toBe('Sytue Notepads');
  expect(productoDe('sytue-certipdf-macos-universal.dmg')).toBe('Sytue CertiPDF');
  expect(productoDe('SytueOne-macos-universal.dmg')).toBe('Sytue One');
  // Los bundles de escritorio que publica empaquetar.sh: la caja, no una de sus apps.
  // «one» es en el catálogo la clave del Hub, así que va por nombre, no por clave.
  expect(productoDe('sytue-one-linux-amd64.tar.xz')).toBe('Sytue One');
  expect(productoDe('sytue-drive-windows-amd64-installer.exe')).toBe('Sytue Drive');
});

// GUARDIÁN de «para tu sistema». Los seis ficheros son los de la v3.6.23, tal cual los
// publicó release-desktop. Medido en producción el 2026-09-25: un Linux ARM64 recibía
// destacado el `linux-amd64`, que no arranca en su procesador.
const V3_6_23 = [
  'sytue-drive-linux-amd64.tar.xz',
  'sytue-drive-macos-universal.dmg',
  'sytue-drive-windows-amd64-installer.exe',
  'sytue-one-linux-amd64.tar.xz',
  'sytue-one-macos-universal.dmg',
  'sytue-one-windows-amd64-installer.exe',
].map((fichero) => ({
  fichero,
  plataforma: clasificar(fichero)!,
  arquitectura: arquitecturaDe(fichero),
}));

const UA = {
  windows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  windowsArm:
    'Mozilla/5.0 (Windows NT 10.0; ARM64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  // Apple silicon e Intel dicen lo mismo: «Intel Mac OS X».
  mac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15',
  linux:
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  linuxArm:
    'Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  iphone:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Mobile/15E148 Safari/604.1',
  android:
    'Mozilla/5.0 (Linux; Android 16; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36',
};

const paraTi = (ua: string) => {
  const e = equipoDelUA(ua);
  return V3_6_23.filter((i) => sirveEn(i, e.plataforma, e.arquitectura)).map((i) => i.fichero);
};

test('arquitecturaDe: lee el procesador del nombre que da empaquetar.sh', () => {
  expect(arquitecturaDe('sytue-one-linux-amd64.tar.xz')).toBe('amd64');
  expect(arquitecturaDe('sytue-one-linux-arm64.tar.xz')).toBe('arm64');
  expect(arquitecturaDe('sytue-one-windows-amd64-installer.exe')).toBe('amd64');
  expect(arquitecturaDe('sytue-one-macos-universal.dmg')).toBe('universal');
  expect(arquitecturaDe('SytueOne.dmg')).toBeNull();
});

test('para tu sistema: cada equipo recibe SOLO lo que arranca en él', () => {
  expect(paraTi(UA.windows)).toEqual([
    'sytue-drive-windows-amd64-installer.exe',
    'sytue-one-windows-amd64-installer.exe',
  ]);
  // Windows en ARM emula x64: el instalador amd64 le sirve.
  expect(paraTi(UA.windowsArm)).toEqual(paraTi(UA.windows));
  expect(paraTi(UA.mac)).toEqual([
    'sytue-drive-macos-universal.dmg',
    'sytue-one-macos-universal.dmg',
  ]);
  expect(paraTi(UA.linux)).toEqual([
    'sytue-drive-linux-amd64.tar.xz',
    'sytue-one-linux-amd64.tar.xz',
  ]);
  // No hay build ARM64 de Linux: nada se destaca, en vez de un binario que no arranca.
  expect(paraTi(UA.linuxArm)).toEqual([]);
  // El iPhone dice «like Mac OS X»: antes recibía el .dmg destacado.
  expect(paraTi(UA.iphone)).toEqual([]);
  expect(paraTi(UA.android)).toEqual([]);
});

test('un build ARM64 de Linux, cuando exista, se destaca en ARM y no en x86_64', () => {
  const arm = { plataforma: 'linux' as const, arquitectura: 'arm64' as const };
  expect(sirveEn(arm, 'linux', 'arm64')).toBe(true);
  expect(sirveEn(arm, 'linux', 'amd64')).toBe(false);
  // Sin saber el procesador, no se descarta nada.
  expect(sirveEn(arm, 'linux', null)).toBe(true);
});

// GUARDIÁN: la web y el Hub (services/hub/internal/instaladores, `TestLaWebYElHubAceptanLoMismo`)
// ofrecen los mismos ficheros. Los dos pasan estos vectores; si uno cambia su regla sin el
// otro, un instalador sale en la web y no en las apps nativas, o al revés.
test('los nombres de instalador: los mismos que acepta el Hub', async () => {
  const vectores: { nombre: string; plataforma: string | null; arquitectura: string | null }[] =
    await Bun.file(new URL('./nombres-instaladores.json', import.meta.url)).json();
  expect(vectores.length).toBeGreaterThan(0);
  for (const v of vectores) {
    expect({ nombre: v.nombre, plataforma: clasificar(v.nombre) }).toEqual({
      nombre: v.nombre,
      plataforma: v.plataforma as ReturnType<typeof clasificar>,
    });
    if (v.plataforma) expect(arquitecturaDe(v.nombre)).toBe(v.arquitectura as never);
  }
});
