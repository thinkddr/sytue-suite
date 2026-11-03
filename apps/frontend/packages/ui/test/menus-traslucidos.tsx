// SPDX-License-Identifier: Apache-2.0
// Banco sin servidor de la cascada de superficies. Lo compila SOLO el runner de CI.
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import { AppGrid } from '../src/components/AppGrid.js';
import { Card } from '../src/components/Card.js';
import { Dropdown, MenuItem } from '../src/components/Dropdown.js';
import { Select } from '../src/components/Form.js';
import { Popover } from '../src/components/Popover.js';
import { Tooltip } from '../src/components/Tooltip.js';
import { Modal } from '../src/components/Modal.js';
import { ToastProvider, useToast } from '../src/components/Toast.js';
import { AppShell } from '../src/components/AppShell.js';
import { Sidebar, SidebarItem, SidebarSection } from '../src/components/Sidebar.js';
import { TopBar } from '../src/components/TopBar.js';
import { suiteApps } from '../src/lib/apps.js';
import { useNavDrawer } from '../src/lib/useNavDrawer.js';

/** Mismo Sidebar en flujo, en el cajón de AppShell y como nav modal del ERP. */
function NavegacionDeMuestra({ erp }: { erp: boolean }) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState('cuaderno');
  useNavDrawer(erp && open, 'fixture-erp-nav', 'fixture-erp-toggle', () => setOpen(false), {
    desktopDrawer: true,
  });
  const sidebar = (
    <Sidebar
      id="fixture-erp-nav"
      class={erp ? `sytue-nav-drawer ${open ? 'sytue-drawer-in' : 'hidden'}` : undefined}
      header={<p class="text-sidebar-fg font-semibold">Cuadernos</p>}
      footer={
        <p class="text-sidebar-fg-muted" data-nav-footer>
          Conectado
        </p>
      }
    >
      <SidebarSection title="Biblioteca">
        {[
          { id: 'cuaderno', label: 'Mi cuaderno' },
          { id: 'archivo', label: 'Archivo' },
        ].map(({ id, label }) => (
          <SidebarItem
            key={id}
            label={label}
            active={active === id}
            onClick={() => {
              setActive(id);
              if (erp) setOpen(false);
            }}
          />
        ))}
      </SidebarSection>
    </Sidebar>
  );
  const topbar = <TopBar rail class="shrink-0" brand={<span>Sytue</span>} />;
  const contenido = (
    <div class="p-4">
      <h1>Contenido de la aplicación</h1>
    </div>
  );
  return (
    <div data-nav-fixture>
      {erp ? (
        <div class="sytue-frame flex h-[100dvh] flex-col">
          {/* Sólo geometría del nav propio; color, selección y foco son del producto. */}
          <style>
            {
              '#fixture-erp-nav{height:auto;position:fixed;top:var(--spacing-topbar);z-index:var(--z-modal)}'
            }
          </style>
          {topbar}
          <div class="sytue-safe-workspace flex min-h-0 flex-1">
            {sidebar}
            <main class="sytue-sheet flex-1">
              <button
                id="fixture-erp-toggle"
                type="button"
                aria-controls="fixture-erp-nav"
                aria-expanded={open}
                onClick={() => setOpen(true)}
              >
                Abrir navegación
              </button>
              {contenido}
            </main>
          </div>
        </div>
      ) : (
        <AppShell topbar={topbar} sidebar={sidebar} title="Cuadernos">
          {contenido}
        </AppShell>
      )}
    </div>
  );
}

function DialogoDeMuestra() {
  const [open, setOpen] = useState(false);
  const toast = useToast();
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Abrir formulario
      </button>
      <button
        type="button"
        onClick={() => toast.notify({ tone: 'danger', title: 'La solicitud falló', duration: 0 })}
      >
        Mostrar error persistente
      </button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Formulario de prueba"
        footer={
          <button type="button" onClick={() => setOpen(false)}>
            Guardar formulario
          </button>
        }
      >
        <p class="text-text">Configuración de la cuenta</p>
        <p class="text-text-muted">Los campos deben leerse sobre un panel opaco.</p>
        <label class="text-text">
          Nombre
          <input name="name" defaultValue="Trabajo" />
        </label>
      </Modal>
    </>
  );
}

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
    <ToastProvider>
      {new URL(location.href).searchParams.get('nav') ? (
        <NavegacionDeMuestra erp={new URL(location.href).searchParams.get('nav') === 'erp'} />
      ) : (
        <main style={{ display: 'flex', gap: '24px', padding: '24px' }}>
          <DialogoDeMuestra />
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
      )}
    </ToastProvider>
  </I18nProvider>,
  document.body,
);
