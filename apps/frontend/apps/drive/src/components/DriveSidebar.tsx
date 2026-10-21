/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DriveSidebar — navegacion lateral de Sytue Drive (fondo de marca, item activo
 * con acento). Secciones canonicas de un gestor de archivos. Visual; la
 * navegacion entre secciones se conecta a medida que se implementan.
 *
 * El idioma llega por props desde la isla raiz (Shell), que a su vez lo recibe
 * del layout SSR: aqui no se detecta nada (docs/CONVENTIONS.md §13.1).
 */
import { useEffect, useState } from 'preact/hooks';
import { Sidebar, SidebarSection, SidebarItem, Meter, formatBytes } from '@sytue/ui';
import { appUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import { ImportData } from '@sytue/shell';
import { useT, type T } from '../i18n';
import { Glyph } from './glyphs';
import { CloudsSidebar, type Cloud } from './PersonalClouds';
import type { GlyphName } from './glyphs';

/**
 * Asistente «Conectar tus dispositivos» del portal: ya trae la dirección WebDAV con
 * botón de copiar y los pasos por sistema operativo. Se ENLAZA en vez de duplicarlo
 * aquí (una sola verdad; el portal es quien conoce los endpoints reales del tenant).
 *
 * `appForKey('one').href` es el dominio DESNUDO (sin el prefijo `/one`): vale para
 * la raíz, que Traefik redirige, pero no para una ruta profunda pegada detrás
 * —era el mismo 404 que el del Launcher del portal—. `appUrl` compone con el
 * prefijo real.
 */
const CONNECT_DEVICES_URL = appUrl('one', 'dispositivos');

export type Section = 'my' | 'documents' | 'team' | 'shared' | 'recent' | 'favorites' | 'trash';

/** Secciones de la sidebar sin backend todavía — se muestran deshabilitadas. */
export const UNIMPLEMENTED_SECTIONS: ReadonlySet<Section> = new Set<Section>();

/**
 * StorageMeter — barra de uso real de almacenamiento (usado/total, %). Lee la
 * cuota del BFF (/api/quota → GetQuota del servicio Go). Sobria (hairline, relleno
 * sólido, sin degradado); la barra expone role=progressbar y el texto el valor.
 * Si la cuota no carga, el pie se muestra sin barra (no estorba).
 */
function StorageMeter({ locale, t }: { locale: Locale; t: T }) {
  const [quota, setQuota] = useState<{
    usedBytes: number;
    limitBytes: number;
    externalStorage?: boolean;
  } | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let alive = true;
    fetch('/api/quota', { credentials: 'include' })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error('quota'))))
      .then((data) => {
        if (alive)
          setQuota(data as { usedBytes: number; limitBytes: number; externalStorage?: boolean });
      })
      .catch(() => {
        if (alive) setFailed(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  if (failed) return null;
  // EL MEDIDOR COMPARTIDO, y no una copia. El de aquí pintaba SIEMPRE del mismo
  // gris: al 99 % avisaba igual que al 3 %, así que el único indicio de estar a
  // punto de quedarse sin sitio era leer el porcentaje —color como portador
  // único de información, WCAG 1.4.1—. `Meter` cambia de rampa al 80 % y al 95 %
  // y además lo DICE con palabras, que es lo que ya hacía Sytue Mail.
  return (
    <div class="px-1.5 py-1">
      <Meter
        value={quota?.usedBytes ?? 0}
        max={quota?.limitBytes ?? 0}
        label={t('storage.title')}
        valueText={
          quota
            ? t('storage.usage', {
                used: formatBytes(locale, quota.usedBytes),
                total: formatBytes(locale, quota.limitBytes),
              })
            : t('storage.loading')
        }
        onChrome
      />
      {/* Con bucket propio conectado, lo nuevo no gasta este espacio: se dice
          junto al medidor, que es donde se mira cuando se acaba el sitio. */}
      {quota?.externalStorage ? (
        <p class="text-sidebar-fg-muted mt-1 text-xs leading-relaxed">
          {t('storage.externalNote')}
        </p>
      ) : null}
    </div>
  );
}

/** Las etiquetas son CLAVES del catálogo, no texto: se traducen al pintar. */
const ITEMS: { key: Section; label: Parameters<T>[0]; glyph: GlyphName }[] = [
  { key: 'my', label: 'nav.myDrive', glyph: 'folder' },
  // Va justo tras «Mi unidad»: un documento de Docs no vive en ninguna carpeta, así que
  // sin esta entrada lo que creas desde Drive desaparece de la vista.
  { key: 'documents', label: 'nav.documents', glyph: 'file' },
  { key: 'team', label: 'nav.teamDrives', glyph: 'building' },
  { key: 'shared', label: 'nav.sharedWithMe', glyph: 'users' },
  { key: 'recent', label: 'nav.recent', glyph: 'clock' },
  { key: 'favorites', label: 'nav.favorites', glyph: 'star' },
  { key: 'trash', label: 'nav.trash', glyph: 'trash' },
];

export function DriveSidebar({
  active,
  onSelect,
  locale,
  cloud,
  onCloud,
}: {
  active: Section;
  onSelect: (s: Section) => void;
  locale: Locale;
  /** La nube personal abierta, si hay una: entonces ninguna sección va activa. */
  cloud: string | null;
  onCloud: (c: Cloud) => void;
}) {
  const t = useT(locale);
  return (
    <Sidebar
      // En <768px la pinta el AdminShell como drawer: ocultarla a secas dejaba
      // sin acceso a Papelera, Compartidos, Unidades de equipo y Documentos.
      // Sin cabecera con isotipo y «Drive»: el selector de apps de la barra
      // superior ya dice en qué app estás, y la barra lateral lo repetía justo
      // debajo (informado el 2026-09-09). Lo mismo se ha quitado en Rooms, Admin
      // y Cloud.
      footer={
        <div class="space-y-1">
          {/* «Trae tus archivos»: el motor de migración existía entero y solo tenía
              puerta en la consola de administración. Quien abre Drive vacío no va a
              pedirle a su administración que le migre los archivos. Se pinta solo si
              la organización tiene conectado un origen. */}
          <ImportData domains={['drive']} />
          {/* Abrir los archivos desde el Finder/Explorador: el paso a paso vive en
              el portal (una sola verdad), aquí solo el camino para llegar. */}
          <a
            href={CONNECT_DEVICES_URL}
            class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring sytue-focus-ring-chrome flex items-center gap-2 rounded-md px-1.5 py-1.5 text-sm transition-colors duration-150"
          >
            <Glyph name="folder" size={18} class="shrink-0" />
            {/* Dos líneas y no `truncate`, como `SidebarItem`: «Ver estos archivos en tu
                ordenador» no cabía entero en búlgaro, griego, irlandés, maltés,
                neerlandés ni portugués. */}
            <span class="line-clamp-2 min-w-0 break-words">{t('nav.connectDevices')}</span>
          </a>
          <StorageMeter locale={locale} t={t} />
          <p class="text-sidebar-fg-muted px-1.5 text-xs leading-relaxed">{t('storage.euNote')}</p>
        </div>
      }
    >
      <SidebarSection title={t('nav.locations')}>
        {ITEMS.map((it) => (
          <SidebarItem
            key={it.key}
            label={t(it.label)}
            icon={<Glyph name={it.glyph} size={18} />}
            active={!cloud && active === it.key}
            disabled={UNIMPLEMENTED_SECTIONS.has(it.key)}
            onClick={() => {
              if (!UNIMPLEMENTED_SECTIONS.has(it.key)) onSelect(it.key);
            }}
          />
        ))}
      </SidebarSection>
      {/* Nubes personales, almacén de la organización y la ÚNICA entrada «Añadir
          almacenamiento» (antes había además un enlace aparte en el pie). */}
      <CloudsSidebar t={t} active={cloud} onSelect={onCloud} />
    </Sidebar>
  );
}
