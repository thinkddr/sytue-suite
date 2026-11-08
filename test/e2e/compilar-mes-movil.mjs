// SPDX-License-Identifier: Apache-2.0
// Compila los dos bancos del mes compacto del teléfono de Calendar (componentes reales,
// sin servidor): el de los componentes y el de la app entera. Los conduce
// test/e2e/mes-movil.mjs.
import { fileURLToPath } from 'node:url';
import { compilarBanco } from './lib/compilar-banco.mjs';

const banco = (f) =>
  fileURLToPath(new URL(`../../apps/frontend/apps/calendar/test/${f}`, import.meta.url));
await compilarBanco(banco('mes-movil.tsx'), process.env.MES_MOVIL_JS || '/tmp/mes-movil.js');
await compilarBanco(
  banco('mes-movil-app.tsx'),
  process.env.MES_MOVIL_APP_JS || '/tmp/mes-movil-app.js',
);
