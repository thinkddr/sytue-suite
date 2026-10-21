// Configuración del harness e2e de Sytue One (@playwright/test).
// Requiere la pila dev corriendo (Traefik + mkcert): https://*.sytue.localhost.
import { defineConfig } from '@playwright/test';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/** Estado de sesión del admin dev (lo escribe tests/auth.setup.ts). */
export const STORAGE_STATE = fileURLToPath(new URL('.auth/admin.json', import.meta.url));

// Proyecto 'chrome' opcional: solo si hay Chrome instalado Y se pide con PW_CHROME=1.
const useChrome = process.env.PW_CHROME === '1' && existsSync('/Applications/Google Chrome.app');

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 2,
  retries: 1,
  timeout: 45_000,
  reporter: [['list'], ['json', { outputFile: 'results.json' }]],
  use: {
    ignoreHTTPSErrors: true,
    viewport: { width: 1440, height: 900 },
    baseURL: 'https://one.sytue.localhost',
    // La suite entera afirma sobre texto EN ESPAÑOL («Salir», «Tú», «Nueva reunión»).
    // Sin fijar el idioma, Chromium pide `en-US`, el middleware lo negocia y sirve la
    // interfaz en inglés: los tests fallan por el idioma y el informe parece decir que
    // el producto está roto. El idioma es parte del entorno de la prueba, no del test.
    locale: 'es-ES',
    extraHTTPHeaders: { 'Accept-Language': 'es-ES,es;q=0.9' },
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts$/ },
    {
      name: 'chromium',
      dependencies: ['setup'],
      use: {
        storageState: STORAGE_STATE,
        launchOptions: {
          // Cámara y micrófono SINTÉTICOS: sin ellos, en headless no hay dispositivos y
          // quien entra en una sala de Meet lo hace «sin publicar», así que LiveKit no ve
          // una sesión real y el ciclo de la llamada (participante entra/sale → sala
          // terminada → aviso al canal) no llega a dispararse. El segundo flag acepta el
          // permiso automáticamente para que no aparezca el diálogo del navegador.
          args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream'],
        },
      },
    },
    ...(useChrome
      ? [
          {
            name: 'chrome',
            dependencies: ['setup'],
            use: { channel: 'chrome', storageState: STORAGE_STATE },
          },
        ]
      : []),
  ],
});
