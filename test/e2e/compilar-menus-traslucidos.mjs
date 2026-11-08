// SPDX-License-Identifier: Apache-2.0
// Solo CI: compila el banco con componentes reales y sin servidor de Astro/Vite.
// El modo ?nav=app/erp incluye Sidebar/AppShell y el cajón que vive en el propio nav.
import { fileURLToPath } from 'node:url';
import { compilarBanco } from './lib/compilar-banco.mjs';

await compilarBanco(
  fileURLToPath(
    new URL('../../apps/frontend/packages/ui/test/menus-traslucidos.tsx', import.meta.url),
  ),
  process.env.MENUS_JS || '/tmp/menus-traslucidos.js',
);
