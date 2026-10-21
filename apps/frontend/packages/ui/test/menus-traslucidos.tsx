// SPDX-License-Identifier: Apache-2.0
// Banco sin servidor de la cascada de superficies. Lo compila SOLO el runner de CI.
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { AppGrid } from '../src/components/AppGrid.js';
import { Card } from '../src/components/Card.js';
import { Dropdown, MenuItem } from '../src/components/Dropdown.js';
import { Select } from '../src/components/Form.js';
import { Popover } from '../src/components/Popover.js';
import { Tooltip } from '../src/components/Tooltip.js';
import { suiteApps } from '../src/lib/apps.js';

function Opciones({ contexto }: { contexto: string }) {
  const tinta = contexto === 'cromo' ? 'text-on-chrome' : 'text-text';
  return (
    <div data-contexto={contexto} style={{ padding: '20px' }}>
      <h2 class={tinta}>{contexto}</h2>
      <AppGrid
        apps={suiteApps.filter((app) => ['cast', 'mail'].includes(app.key))}
        current="cast"
      />
      <Select aria-label={`Idioma de ${contexto}`}>
        <optgroup label="Idiomas">
          <option value="es">Español</option>
          <option value="bloqueada" disabled>
            No disponible
          </option>
          <option value="de">Deutsch</option>
        </optgroup>
        <optgroup label="Grupo no disponible" disabled>
          <option value="grupo-bloqueado">Idioma del grupo</option>
        </optgroup>
        <option value="en">English</option>
      </Select>
      <Tooltip label={`Ayuda de ${contexto}`} placement="bottom">
        <button type="button" class={tinta}>
          Ayuda de {contexto}
        </button>
      </Tooltip>
      <Dropdown
        label={`Acciones de ${contexto}`}
        trigger={(props) => (
          <button class={tinta} {...props}>
            Acciones de {contexto}
          </button>
        )}
      >
        <MenuItem>Editar</MenuItem>
        <MenuItem danger>Eliminar</MenuItem>
      </Dropdown>
      <Popover
        label={`Opciones de ${contexto}`}
        trigger={(props) => (
          <button class={tinta} {...props}>
            Opciones de {contexto}
          </button>
        )}
      >
        <p class="text-text">Opciones de la aplicación</p>
        <p class="text-primary-text">Más información</p>
        <p class="sytue-meta">Descripción de la opción</p>
      </Popover>
    </div>
  );
}

render(
  <I18nProvider locale="es">
    <main style={{ display: 'flex', gap: '24px', padding: '24px' }}>
      <section class="bg-surface">
        <Opciones contexto="contenido" />
      </section>
      <section class="bg-sidebar">
        <Opciones contexto="cromo" />
      </section>
      <Card tone="slate">
        <Opciones contexto="tarjeta" />
      </Card>
    </main>
  </I18nProvider>,
  document.body,
);
