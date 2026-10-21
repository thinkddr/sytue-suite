// Contrato de la página de descargas: NUNCA enlazar a un fichero que no existe.
//
// Las plataformas no llegan a la vez —macOS necesita compilarse en un Mac y hasta
// que ese runner exista solo hay Windows y Linux—, así que el build hornea la
// lista de las publicadas. Si este filtro se pierde, la web enseña un botón de
// macOS que da 404 y nada falla: por eso está escrito aquí.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';

test('solo se enlazan las plataformas publicadas', async () => {
  process.env.PUBLIC_SYTUE_DOWNLOADS_BASE = 'https://descargas.ejemplo/latest';
  process.env.PUBLIC_SYTUE_DOWNLOADS_PLATFORMS = 'windows,linux';
  // Import dinámico: el módulo lee el entorno al cargarse.
  const { desktopFor } = await import('./src/lib/downloads.js');

  const porId = Object.fromEntries(desktopFor('sytue-one').map((d) => [d.id, d.href]));
  expect(porId.windows).toBe(
    'https://descargas.ejemplo/latest/sytue-one-windows-amd64-installer.exe',
  );
  expect(porId.linux).toBe('https://descargas.ejemplo/latest/sytue-one-linux-amd64.tar.xz');
  // macOS no está publicada: sin href, la tarjeta dice «próximamente».
  expect(porId.macos).toBe('');
});
