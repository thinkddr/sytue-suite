/**
 * PanelOpcionesBusqueda — «Opciones de búsqueda», junto al buscador de la lista.
 *
 * El servicio entiende `from:`, `to:`, `subject:`, `has:attachment`, `is:unread`,
 * `after:` y `before:`, pero solo lo decía un `title` en inglés técnico, que no llega ni
 * a la pantalla táctil ni al lector de pantalla. Este panel pregunta en lenguaje llano
 * (De, Para, Asunto, Contiene, Desde, Hasta, Con adjuntos, Sin leer), escribe la
 * consulta con `componerConsulta` y lanza la búsqueda. Al abrirse con una consulta ya
 * escrita la desmonta en campos (`desmontarConsulta`), así que se puede afinar lo que
 * ya se buscaba sin volver a teclearlo. Es `panelOpcionesBusqueda` de Fyne
 * (apps/fyne/apps/mail/consulta.go).
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Popover } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { Glyph } from './glyphs.js';
import {
  OPCIONES_BUSQUEDA_VACIAS,
  componerConsulta,
  desmontarConsulta,
  type OpcionesBusqueda,
} from './utils.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  /** Lo escrito ahora en el buscador: el panel se abre con ello desmontado. */
  query: string;
  /** Escribe la consulta compuesta en el buscador, que es quien busca. */
  onBuscar: (consulta: string) => void;
}

export function PanelOpcionesBusqueda({ locale, query, onBuscar }: Props): JSX.Element {
  const t = useT(locale);
  return (
    <Popover
      label={t('list.searchOptions')}
      align="end"
      trigger={(props) => (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          iconOnly
          aria-label={t('list.searchOptions')}
          title={t('list.searchOptions')}
          leadingIcon={<Glyph name="filter" size={16} />}
          {...props}
        />
      )}
    >
      {(cerrar) => (
        <Formulario
          locale={locale}
          query={query}
          onBuscar={(q) => {
            onBuscar(q);
            cerrar();
          }}
        />
      )}
    </Popover>
  );
}

/** Los campos de texto, en el orden en que se leen. */
const CAMPOS_TEXTO = [
  ['de', 'list.searchFrom'],
  ['para', 'list.searchTo'],
  ['asunto', 'list.searchSubject'],
  ['contiene', 'list.searchWords'],
] as const;

/**
 * El formulario vive solo mientras el panel está abierto: cada apertura vuelve a
 * desmontar la consulta de ese momento en vez de enseñar lo que quedó la vez anterior.
 */
function Formulario({ locale, query, onBuscar }: Props): JSX.Element {
  const t = useT(locale);
  const [o, setO] = useState<OpcionesBusqueda>(() => desmontarConsulta(query));
  const poner = <K extends keyof OpcionesBusqueda>(k: K, v: OpcionesBusqueda[K]): void =>
    setO((prev) => ({ ...prev, [k]: v }));
  return (
    <form
      class="flex w-72 max-w-full flex-col gap-3"
      onSubmit={(e: Event) => {
        e.preventDefault();
        onBuscar(componerConsulta(o));
      }}
    >
      {CAMPOS_TEXTO.map(([k, clave]) => (
        <FieldGroup key={k} label={t(clave)} htmlFor={`busqueda-${k}`}>
          <Input
            id={`busqueda-${k}`}
            type="text"
            autocomplete="off"
            value={o[k]}
            onInput={(e) => poner(k, e.currentTarget.value)}
          />
        </FieldGroup>
      ))}
      <div class="grid grid-cols-2 gap-2">
        <FieldGroup label={t('list.searchSince')} htmlFor="busqueda-desde" class="min-w-0">
          <Input
            id="busqueda-desde"
            type="date"
            value={o.desde}
            max={o.hasta || undefined}
            onInput={(e) => poner('desde', e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup label={t('list.searchUntil')} htmlFor="busqueda-hasta" class="min-w-0">
          <Input
            id="busqueda-hasta"
            type="date"
            value={o.hasta}
            min={o.desde || undefined}
            onInput={(e) => poner('hasta', e.currentTarget.value)}
          />
        </FieldGroup>
      </div>
      <div class="flex flex-col gap-2">
        <Checkbox
          label={t('filter.attachments')}
          checked={o.conAdjuntos}
          onChange={(e) => poner('conAdjuntos', e.currentTarget.checked)}
        />
        <Checkbox
          label={t('filter.unread')}
          checked={o.sinLeer}
          onChange={(e) => poner('sinLeer', e.currentTarget.checked)}
        />
      </div>
      <div class="flex justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setO(OPCIONES_BUSQUEDA_VACIAS)}
        >
          {t('list.searchReset')}
        </Button>
        <Button type="submit" variant="primary" size="sm">
          {t('list.searchApply')}
        </Button>
      </div>
    </form>
  );
}
