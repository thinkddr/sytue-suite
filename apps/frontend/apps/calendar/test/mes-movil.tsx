// SPDX-License-Identifier: Apache-2.0
// Banco sin servidor del mes compacto del teléfono (Calendar). Lo compila
// test/e2e/compilar-mes-movil.mjs y lo fotografía test/e2e/mes-movil.mjs: AppShell real
// con la barra móvil del mes, el cajón con su bloque de vista y el MesCompacto con los
// mismos datos que el test de Fyne (la captura del dueño: noviembre de 2026, hoy el 6).
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { I18nProvider } from '@sytue/i18n/preact';
import { AppShell, Sidebar } from '@sytue/ui';
import { useT } from '../src/i18n/index.js';
import type { GridEvent } from '../src/components/CalendarGrid.js';
import { addDays, addMonthsClamped, compactTitle, type ViewMode } from '../src/components/dates.js';
import {
  AccionesBarraCompacta,
  BloqueVistaMovil,
  MesCompacto,
} from '../src/components/MesCompacto.js';
import { ReminderCenter, type EstadoRecordatorios } from '../src/components/ReminderCenter.js';

const HOY = new Date(2026, 10, 6, 19, 36);
const AZUL = '#3d8fd6';

function ev(dia: number, hora: number, summary: string): GridEvent {
  const d = (h: number) =>
    `202611${String(dia).padStart(2, '0')}T${String(h).padStart(2, '0')}0000`;
  return {
    href: `/cal/${dia}-${hora}.ics`,
    uid: `${dia}-${hora}`,
    summary,
    start: d(hora),
    end: d(hora + 1),
    ical: '',
    color: AZUL,
  };
}

// Un punto el 2, 3, 6 (Tenis 14:00–15:00), 10, 13, 17, 19, 20, 21, 23, 24, 26, 27 y 30;
// dos el 9 y el 16.
const EVENTOS: GridEvent[] = [
  ...[2, 3, 10, 13, 17, 19, 20, 21, 23, 24, 26, 27, 30].map((d) => ev(d, 10, 'Reunión')),
  ev(6, 14, 'Tenis'),
  ev(9, 9, 'Dentista'),
  ev(9, 18, 'Cena'),
  ev(16, 9, 'Revisión'),
  ev(16, 17, 'Clase'),
];

const RECORDATORIOS: EstadoRecordatorios = {
  lista: [
    ['r1', 'Tenis', '2026-11-06T13:00:00Z'],
    ['r2', 'Dentista', '2026-11-09T08:00:00Z'],
  ].map(([id, summary, dtstart]) => ({
    id,
    event_id: id,
    uid: id,
    summary,
    dtstart,
    trigger_at: dtstart,
    action: 'DISPLAY',
  })),
  cargando: false,
  error: false,
  ocupado: null,
  permiso: 'granted',
  descartar: async () => {},
  irA: () => {},
  pedirPermiso: async () => {},
};

function Banco() {
  const t = useT('es');
  const [cursor, setCursor] = useState(new Date(2026, 10, 6));
  const [plegado, setPlegado] = useState(false);
  const [mode, setMode] = useState<ViewMode>('month');
  const pendientes = RECORDATORIOS.lista.length + 1;
  return (
    <AppShell
      topbar={null}
      sidebar={
        <Sidebar>
          <BloqueVistaMovil
            t={t}
            mode={mode}
            onMode={setMode}
            onBuscar={() => {}}
            recordatorios={
              <ReminderCenter t={t} locale="es" estado={RECORDATORIOS} disparador="fila" />
            }
            invitaciones={1}
            onInvitaciones={() => {}}
          />
        </Sidebar>
      }
      mobileBarCenter={
        <h1 class="text-md truncate font-semibold">{compactTitle('es', cursor, HOY)}</h1>
      }
      mobileBarActions={
        <AccionesBarraCompacta
          t={t}
          locale="es"
          hoy={HOY}
          cursor={cursor}
          offline={false}
          onHoy={() => setCursor(HOY)}
          onCrear={() => {}}
        />
      }
      mobileNavNotice={t('nav.menuPending', { count: pendientes })}
    >
      <div data-banco-mes class="h-full overflow-auto">
        <MesCompacto
          t={t}
          locale="es"
          cursor={cursor}
          hoy={HOY}
          events={EVENTOS}
          plegado={plegado}
          onPlegar={() => setPlegado((v) => !v)}
          cargando={false}
          onElegir={setCursor}
          onMoverMes={(d) =>
            setCursor((c) => (plegado ? addDays(c, 7 * d) : addMonthsClamped(c, d)))
          }
          onSelectEvent={() => {}}
        />
      </div>
    </AppShell>
  );
}

const raiz = document.createElement('div');
document.body.appendChild(raiz);
render(
  <I18nProvider locale="es">
    <Banco />
  </I18nProvider>,
  raiz,
);
