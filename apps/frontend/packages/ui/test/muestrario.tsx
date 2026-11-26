// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/**
 * # El muestrario de `@sytue/ui`
 *
 * Todas las piezas que exporta `src/index.ts`, por familias y con sus variantes y estados
 * uno al lado del otro, para revisarlas a ojo y para compararlas con el muestrario de
 * Fyne (`apps/fyne/pantalla/muestrario.go`) lado a lado. Es el criterio de salida de F0
 * del roadmap: el catálogo renderiza todos los componentes en todos los temas.
 *
 * Los ejes del tema —2 colores × 4 modos × 2 formas— NO van aquí: son atributos de
 * `<html>`, y este módulo pinta lo mismo con cualquiera de ellos. Los pone la página
 * (`muestrario-pagina.tsx`, con los mandos de `useTheme`) o el test, que recorre las
 * 16 combinaciones (`muestrario.test.tsx`).
 *
 * El guardián de `muestrario.test.tsx` exige que cada componente exportado salga aquí
 * como `<Nombre` (o en su lista de exclusiones, con el motivo): uno nuevo que nadie
 * enseña es uno que nadie revisa en oscuro ni en carril.
 *
 * Importa SOLO de `../src/index.js`: lo que se enseña es la API pública, no un módulo
 * interno que las apps no pueden usar. Los textos son de muestra y van tal cual, como
 * los del banco de menús: esto no es una app de la suite ni lo ve nadie fuera de quien
 * revisa (lint:i18n no mira `test/`).
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  AdminShell,
  AppGrid,
  AppShell,
  Avatar,
  AvatarPicker,
  Badge,
  BarChart,
  BarraMovil,
  BarraUnicaContext,
  BotonFlotante,
  BulkActionBar,
  Button,
  CARD_TONES,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardIcon,
  CardTitle,
  Checkbox,
  ChoiceCard,
  ConfirmDialog,
  CopyField,
  DataFlow,
  DialogosDelEditor,
  Dropdown,
  EmptyState,
  FieldGroup,
  FieldHint,
  Glifo,
  Icon,
  IconButton,
  Illustration,
  IndicadorRefrescoTactil,
  Kbd,
  InlineHelp,
  Input,
  LanguageSelect,
  List,
  ListItem,
  MenuItem,
  MenuSeparator,
  Meter,
  Migas,
  Modal,
  OrgMark,
  PaginaNoEncontrada,
  PaletaDeComandos,
  Popover,
  ProductIcon,
  ProgressBar,
  Radio,
  ReadOnlyBar,
  Resaltado,
  ReviewSummary,
  SectionHeader,
  Select,
  SegmentedControl,
  SelectorDeColor,
  SettingsSection,
  SettingsShell,
  ShortcutsModal,
  Sidebar,
  SidebarAction,
  SidebarItem,
  SidebarSection,
  SinBarraUnica,
  Skeleton,
  SkeletonRows,
  SkipLink,
  Spinner,
  Stat,
  Stepper,
  Switch,
  Table,
  Tabs,
  Tag,
  Textarea,
  ToastProvider,
  Toolbar,
  Tooltip,
  TopBar,
  TopBarButton,
  VersionList,
  VirtualList,
  Wizard,
  confirmarEnEditor,
  useToast,
} from '../src/index.js';
import type {
  BadgeTone,
  ButtonSize,
  ButtonVariant,
  IconButtonTone,
  IconName,
  OpcionDePaleta,
  SortDirection,
} from '../src/index.js';

/** Los ejes del tema que recorre el muestrario (ver `src/theme/init.ts`). */
export const MODOS = ['light', 'dark', 'translucent', 'system'] as const;
/** «El de cada app» no es UN color: se revisa en las apps, no en el catálogo. */
export const COLORES = ['petroleo', 'cobalto'] as const;
export const FORMAS = ['marco', 'carril'] as const;

export interface Combinacion {
  modo: (typeof MODOS)[number];
  color: (typeof COLORES)[number];
  forma: (typeof FORMAS)[number];
}

/** Las 2 × 4 × 2 = 16, en el orden en que se recorren. */
export const COMBINACIONES: readonly Combinacion[] = COLORES.flatMap((color) =>
  MODOS.flatMap((modo) => FORMAS.map((forma) => ({ modo, color, forma }))),
);

/** El nombre de una combinación: el de su captura y el de su `?` en la página. */
export function nombreDe({ modo, color, forma }: Combinacion): string {
  return `${color}-${modo}-${forma}`;
}

/**
 * Una familia del muestrario. `capa` dice que lo suyo se abre ENCIMA de la página (un
 * diálogo, la paleta): sale cerrada con su botón y `abierta` la monta ya abierta, que es
 * como la recorre el test y lo que pide `?abrir=` en la página.
 */
export interface Familia {
  clave: string;
  titulo: string;
  capa?: boolean;
  /** `cual` elige UNA de las capas de la familia (`?abrir=dialogos:paleta`); sin él, la primera. */
  Pintar: (props: { abierta: boolean; cual?: string | undefined }) => JSX.Element;
}

const nada = (): void => {};

/** Una fila con rótulo: las variantes de una pieza, una al lado de la otra. */
function Fila({ rotulo, children }: { rotulo: string; children: ComponentChildren }) {
  return (
    <div class="flex flex-col gap-2">
      <p class="sytue-overline text-text-subtle">{rotulo}</p>
      <div class="flex flex-wrap items-center gap-3">{children}</div>
    </div>
  );
}

// ---------------------------------------------------------------- Botones

const VARIANTES: ButtonVariant[] = ['primary', 'secondary', 'ghost', 'danger'];
const TAMANOS: ButtonSize[] = ['xs', 'sm', 'md', 'lg'];
const TONOS_ICONO: IconButtonTone[] = ['default', 'subtle', 'danger', 'header', 'media'];

function Botones() {
  return (
    <>
      <Fila rotulo="Variantes">
        {VARIANTES.map((v) => (
          <Button key={v} variant={v}>
            {v}
          </Button>
        ))}
      </Fila>
      <Fila rotulo="Tamaños">
        {TAMANOS.map((s) => (
          <Button key={s} size={s} variant="secondary">
            {s}
          </Button>
        ))}
      </Fila>
      <Fila rotulo="Estados">
        <Button leadingIcon={<Icon name="plus" size={16} />}>Con icono</Button>
        <Button trailingIcon={<Icon name="chevronDown" size={16} />} variant="secondary">
          Con flecha
        </Button>
        <Button loading>Guardando</Button>
        <Button disabled>Deshabilitado</Button>
        <Button href="#muestrario" variant="ghost">
          Enlace
        </Button>
        <Button iconOnly aria-label="Ajustes" variant="ghost">
          <Icon name="settings" size={18} />
        </Button>
      </Fila>
      <Fila rotulo="Botón de icono">
        {TONOS_ICONO.map((tone) => (
          <IconButton key={tone} label={`Tono ${tone}`} tone={tone}>
            <Icon name="corazon" size={18} />
          </IconButton>
        ))}
        <IconButton label="Marcado" pressed>
          <Icon name="bold" size={18} />
        </IconButton>
        <IconButton label="Deshabilitado" disabled>
          <Icon name="trash" size={18} />
        </IconButton>
      </Fila>
      <Fila rotulo="Salto al contenido (visible con el foco)">
        <SkipLink targetId="muestrario" />
      </Fila>
      <Fila rotulo="El «+» flotante de crear, solo y con menú (dentro de su marco)">
        <div class="border-border relative h-24 w-48 transform-gpu overflow-hidden rounded-lg border">
          <BotonFlotante label="Crear evento" onClick={nada} />
        </div>
        <div class="border-border relative h-24 w-48 transform-gpu overflow-hidden rounded-lg border">
          <BotonFlotante
            label="Nuevo"
            menu={
              <>
                <MenuItem icon={<Icon name="upload" size={16} />}>Subir archivos</MenuItem>
                <MenuItem icon={<Icon name="image" size={16} />}>Fotos</MenuItem>
              </>
            }
          />
        </div>
      </Fila>
    </>
  );
}

// ---------------------------------------------------------------- Iconos y marca

const ICONOS: IconName[] = [
  'search',
  'mail',
  'bell',
  'calendar',
  'users',
  'settings',
  'lock',
  'cloud',
  'trash',
  'download',
  'upload',
  'warning',
];

function Iconos() {
  return (
    <>
      <Fila rotulo="Icon">
        {ICONOS.map((name) => (
          <Icon key={name} name={name} label={name} />
        ))}
      </Fila>
      <Fila rotulo="Glifo (trazo propio)">
        <Glifo d="M4 12h16M12 4v16" label="Cruz" />
        <Glifo d="M5 5h14v14H5z" size={28} label="Cuadro" />
      </Fila>
      <Fila rotulo="Iconos de producto">
        {(['mail', 'drive', 'docs', 'calendar'] as const).map((appKey) => (
          <ProductIcon key={appKey} appKey={appKey} size={32} alt={appKey} />
        ))}
        <ProductIcon appKey="notes" size={16} alt="notes" />
      </Fila>
      <Fila rotulo="Ilustraciones">
        <Illustration name="emptyMail" size={96} />
        <Illustration name="allDone" size={96} />
        <Illustration name="errorRetry" size={96} />
      </Fila>
      <Fila rotulo="Marca de organización">
        <OrgMark slug="acme" />
        <OrgMark slug="cooperativa" size={40} />
        <OrgMark />
      </Fila>
      <Fila rotulo="Avatar">
        {(['xs', 'sm', 'md', 'lg', 'xl'] as const).map((size) => (
          <Avatar key={size} name="Ana Martín" size={size} />
        ))}
        <Avatar name="Luis Pérez" presence="online" />
        <Avatar name="Marta Gil" presence="away" />
        <Avatar name="Íker Sanz" presence="dnd" />
        <Avatar name="Sytue" logo />
      </Fila>
      <Fila rotulo="Elegir avatar">
        <AvatarPicker
          name="Ana Martín"
          hasPhoto={false}
          onPick={nada}
          labels={{
            change: 'Cambiar',
            add: 'Añadir foto',
            presets: 'Colores',
            upload: 'Subir',
            remove: 'Quitar',
            error: () => 'No se pudo leer la imagen',
          }}
        />
      </Fila>
    </>
  );
}

// ---------------------------------------------------------------- Campos

const COLORES_SELECTOR = [
  { valor: 'var(--sytue-product-mail)', nombre: 'Mail' },
  { valor: 'var(--sytue-product-drive)', nombre: 'Drive' },
  { valor: 'var(--sytue-product-docs)', nombre: 'Docs' },
  { valor: 'var(--sytue-product-calendar)', nombre: 'Calendar' },
];

function Campos() {
  const [color, setColor] = useState(COLORES_SELECTOR[0].valor);
  const [activo, setActivo] = useState(true);
  const [vista, setVista] = useState<'lista' | 'rejilla'>('lista');
  return (
    <>
      <div class="grid gap-4 sm:grid-cols-2">
        <FieldGroup label="Nombre" htmlFor="m-nombre" help="Como te verán los demás" required>
          <Input id="m-nombre" value="Ana Martín" />
        </FieldGroup>
        <FieldGroup label="Correo" htmlFor="m-correo" error="Falta la arroba">
          <Input id="m-correo" type="email" invalid value="ana.ejemplo" />
        </FieldGroup>
        <FieldGroup label="Buscar" htmlFor="m-buscar">
          <Input
            id="m-buscar"
            size="sm"
            placeholder="Buscar en el buzón"
            leadingIcon={<Icon name="search" size={16} />}
          />
        </FieldGroup>
        <FieldGroup label="Deshabilitado" htmlFor="m-off">
          <Input id="m-off" disabled value="No se puede tocar" />
        </FieldGroup>
        <FieldGroup label="Notas" htmlFor="m-notas">
          <Textarea id="m-notas" rows={3} value="Una nota de varias líneas." />
        </FieldGroup>
        <FieldGroup label="Zona horaria" htmlFor="m-zona">
          <Select id="m-zona">
            <option>Europe/Madrid</option>
            <option>Europe/Lisbon</option>
          </Select>
        </FieldGroup>
      </div>
      <Fila rotulo="Casilla, radio y conmutador">
        <Checkbox label="Marcada" checked />
        <Checkbox label="Sin marcar" />
        <Checkbox label="Deshabilitada" disabled />
        <Radio name="m-radio" label="Uno" checked />
        <Radio name="m-radio" label="Otro" />
        <Switch label="Notificaciones" checked={activo} onChange={() => setActivo((a) => !a)} />
        <Switch label="Apagado" checked={false} />
      </Fila>
      <Fila rotulo="Control segmentado">
        <SegmentedControl
          opciones={[
            { valor: 'lista', rotulo: 'Lista', icono: 'list' },
            { valor: 'rejilla', rotulo: 'Cuadrícula', icono: 'grid' },
          ]}
          valor={vista}
          onChange={setVista}
          aria-label="Vista"
        />
        <SegmentedControl
          opciones={[
            { valor: 'lista', rotulo: 'Lista', icono: 'list' },
            { valor: 'rejilla', rotulo: 'Cuadrícula', icono: 'grid' },
          ]}
          valor={vista}
          onChange={setVista}
          size="sm"
          soloIcono
          aria-label="Vista compacta"
        />
      </Fila>
      <Fila rotulo="Idioma">
        <LanguageSelect />
      </Fila>
      <Fila rotulo="Selector de color">
        <SelectorDeColor
          colores={COLORES_SELECTOR}
          valor={color}
          onChange={setColor}
          aria-label="Color del calendario"
        />
        <SelectorDeColor
          colores={COLORES_SELECTOR}
          valor={color}
          onChange={setColor}
          tamano="sm"
          aria-label="Color pequeño"
        />
      </Fila>
      <Fila rotulo="Teclas">
        <Kbd>Ctrl</Kbd>
        <Kbd>K</Kbd>
        <Kbd tone="chrome">Esc</Kbd>
      </Fila>
      <Fila rotulo="Pistas y ayuda">
        <FieldHint>Una pista neutra bajo el campo.</FieldHint>
        <FieldHint tone="success">Comprobado.</FieldHint>
        <FieldHint tone="warning">Revisa el dominio.</FieldHint>
        <FieldHint tone="danger">No se puede usar.</FieldHint>
      </Fila>
      <InlineHelp title="Qué pasa al conectar" action={<Button size="sm">Ver más</Button>}>
        Se copia la lista de contactos; nada sale de Sytue One.
      </InlineHelp>
      <InlineHelp tone="warning" title="Atención">
        Esta acción afecta a toda la organización.
      </InlineHelp>
      <CopyField label="Servidor" value="imap.sytue.example" mono />
      <CopyField label="Contraseña" value="no-la-copies" secret />
    </>
  );
}

// ---------------------------------------------------------------- Elección y pasos

const PASOS = [
  { id: 'cuenta', title: 'Cuenta' },
  { id: 'dominio', title: 'Dominio' },
  { id: 'revisar', title: 'Revisar' },
];

function Eleccion() {
  const [plan, setPlan] = useState('equipo');
  const [paso, setPaso] = useState(1);
  return (
    <>
      <div class="grid gap-3 sm:grid-cols-3">
        <ChoiceCard
          name="m-plan"
          title="Personal"
          description="Para ti solo"
          icon="user"
          selected={plan === 'personal'}
          onSelect={() => setPlan('personal')}
        />
        <ChoiceCard
          name="m-plan"
          title="Equipo"
          description="Hasta 50 personas"
          badge={<Badge tone="primary">Recomendado</Badge>}
          icon="users"
          selected={plan === 'equipo'}
          onSelect={() => setPlan('equipo')}
        />
        <ChoiceCard
          name="m-plan"
          title="Empresa"
          description="Con su dominio"
          footnote="Pronto"
          icon="building"
          disabled
          selected={false}
          onSelect={nada}
        />
      </div>
      <Tabs
        label="Pestañas de muestra"
        items={[
          { id: 'general', label: 'General', content: <p class="text-text">Lo general.</p> },
          {
            id: 'seguridad',
            label: 'Seguridad',
            icon: <Icon name="lock" size={16} />,
            content: <p class="text-text">La seguridad.</p>,
          },
          { id: 'avanzado', label: 'Avanzado', content: <p class="text-text">Lo avanzado.</p> },
        ]}
      />
      <Stepper steps={PASOS} current={paso} onStepClick={setPaso} />
      <Wizard
        current={paso}
        onCurrentChange={setPaso}
        onCancel={nada}
        steps={PASOS.map((p) => ({
          ...p,
          description: `El paso «${p.title}» del asistente.`,
          content: <p class="text-text">Contenido de {p.title.toLowerCase()}.</p>,
        }))}
      />
    </>
  );
}

// ---------------------------------------------------------------- Estado y avisos

const TONOS_INSIGNIA: BadgeTone[] = [
  'neutral',
  'primary',
  'accent',
  'success',
  'warning',
  'danger',
  'info',
];

function Estado() {
  return (
    <>
      <Fila rotulo="Insignias">
        {TONOS_INSIGNIA.map((tone) => (
          <Badge key={tone} tone={tone}>
            {tone}
          </Badge>
        ))}
        <Badge tone="primary" pill>
          Píldora
        </Badge>
        <Badge tone="danger" count>
          12
        </Badge>
      </Fila>
      <Fila rotulo="Etiquetas">
        <Tag>Sin punto</Tag>
        <Tag dot="var(--sytue-product-mail)">Trabajo</Tag>
        <Tag dot="var(--sytue-product-drive)" tone="muted" size="xs">
          Archivo
        </Tag>
        <Tag onRemove={nada} removeLabel="Quitar «Viajes»">
          Viajes
        </Tag>
      </Fila>
      <Fila rotulo="Carga">
        <Spinner label="Cargando" />
        <Spinner size={28} />
        <Skeleton width="12rem" height="1rem" />
        <Skeleton circle width="2.5rem" height="2.5rem" />
        <IndicadorRefrescoTactil estado={{ recorrido: 48, actualizando: false }} />
        <IndicadorRefrescoTactil estado={{ recorrido: 0, actualizando: true }} />
      </Fila>
      <div class="grid gap-4 sm:grid-cols-2">
        <SkeletonRows rows={3} lead="avatar" lines={2} meta label="Cargando la lista" />
        <SkeletonRows rows={3} lead="tile" />
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <Meter label="Disco" value={4} max={10} />
        <Meter label="Disco casi lleno" value={8.5} max={10} />
        <Meter label="Disco lleno" value={9.7} max={10} />
        <ProgressBar label="Subiendo" value={0.4} />
        <ProgressBar label="Preparando" />
        <ProgressBar label="Falló" value={0.7} tone="danger" />
      </div>
      <Fila rotulo="Resaltado">
        <p class="text-text">
          <Resaltado texto="Reunión de presupuesto del jueves" consulta="presu" />
        </p>
      </Fila>
      <BarChart
        seriesLabel="Mensajes por día"
        tableHeaders={['Día', 'Mensajes']}
        height={120}
        points={[
          { label: 'L', value: 12 },
          { label: 'M', value: 30 },
          { label: 'X', value: 18 },
          { label: 'J', value: 42 },
          { label: 'V', value: 25 },
        ]}
      />
      <BarChart seriesLabel="Sin datos" points={[]} emptyLabel="Todavía no hay datos" />
      <ReadOnlyBar
        message="Solo lectura: lo compartieron contigo sin permiso de edición."
        action={{ label: 'Pedir acceso', onClick: nada }}
        secondaryAction={{ label: 'Hacer una copia', onClick: nada }}
      />
      <div class="grid gap-4 sm:grid-cols-3">
        <EmptyState
          title="Bandeja vacía"
          description="Aquí llegarán los mensajes."
          spot="emptyMail"
          action={<Button size="sm">Redactar</Button>}
        />
        <EmptyState
          title="No se pudo cargar"
          description="Vuelve a intentarlo en un momento."
          tone="error"
          icon="warning"
          action={
            <Button size="sm" variant="secondary">
              Reintentar
            </Button>
          }
        />
        <EmptyState title="Sin resultados" icon="search" compact />
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Tarjetas y resúmenes

function Tarjetas() {
  return (
    <>
      <div class="grid gap-3 sm:grid-cols-4">
        <Stat label="Usuarios activos" value="1.284" hint="+12 % este mes" />
        <Stat label="Copias al día" value="98 %" tone="success" />
        <Stat label="Cuota usada" value="87 %" tone="warning" />
        <Stat label="Errores" value={3} tone="danger" surface="plain" />
      </div>
      <SectionHeader
        title="Cabecera de sección"
        subtitle="Con icono, subtítulo y acciones"
        icon={<Icon name="album" size={18} />}
        actions={<Button size="sm">Nueva</Button>}
        level={3}
      />
      <div class="grid gap-3 sm:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Neutra</CardTitle>
          </CardHeader>
          <CardBody>
            <p class="text-text">El cuerpo de la tarjeta.</p>
          </CardBody>
          <CardFooter>
            <Button size="sm" variant="secondary">
              Acción
            </Button>
          </CardFooter>
        </Card>
        <Card raised>
          <CardBody>
            <p class="text-text">Elevada</p>
          </CardBody>
        </Card>
        <Card tone="mint" fill="solid">
          <CardBody>
            <p>Sólida</p>
          </CardBody>
        </Card>
      </div>
      <Fila rotulo="Tonos">
        {CARD_TONES.map((tone) => (
          <Card key={tone} tone={tone} sectionLabel={tone}>
            <CardBody class="flex items-center gap-2">
              <CardIcon tone={tone}>
                <Icon name="corazon" size={16} />
              </CardIcon>
              <span>{tone}</span>
            </CardBody>
          </Card>
        ))}
      </Fila>
      <ReviewSummary
        editLabel="Cambiar"
        items={[
          { label: 'Dominio', value: 'sytue.example', mono: true, onEdit: nada },
          { label: 'Personas', value: '12' },
        ]}
      />
      <DataFlow partner="Slack" incoming={['messages', 'people']} outgoing={['files']} />
      <DataFlow compact incoming={['events']} />
      <VersionList
        currentLabel="Actual"
        versionLabel={(v) => `Versión ${v.version}`}
        versions={[
          { version: 3, isCurrent: true, sizeBytes: 24_000, author: 'Ana Martín' },
          { version: 2, isCurrent: false, sizeBytes: 21_500, author: 'Luis Pérez' },
        ]}
        actions={() => (
          <Button size="xs" variant="ghost">
            Restaurar
          </Button>
        )}
      />
    </>
  );
}

// ---------------------------------------------------------------- Listas y tablas

interface Fichero {
  id: string;
  nombre: string;
  tamano: string;
}

const FICHEROS: Fichero[] = [
  { id: 'a', nombre: 'Presupuesto 2027.xlsx', tamano: '24 KB' },
  { id: 'b', nombre: 'Acta de la reunión.docx', tamano: '12 KB' },
  { id: 'c', nombre: 'Logo.svg', tamano: '3 KB' },
];

const FILAS_VIRTUALES = Array.from({ length: 200 }, (_, i) => `Fila virtual ${i + 1}`);

function Listas() {
  const [orden, setOrden] = useState<SortDirection>('asc');
  const [marcados, setMarcados] = useState<ReadonlySet<string>>(() => new Set(['b']));
  const alternar = (clave: string) =>
    setMarcados((prev) => {
      const sig = new Set(prev);
      if (!sig.delete(clave)) sig.add(clave);
      return sig;
    });
  return (
    <>
      <div class="grid gap-4 sm:grid-cols-2">
        <List selectable label="Bandeja de muestra">
          <ListItem
            index={0}
            leading={<Avatar name="Ana Martín" size="sm" />}
            title="Ana Martín"
            subtitle="Presupuesto del jueves"
            meta="10:24"
            unread
          />
          <ListItem
            index={1}
            leading={<Avatar name="Luis Pérez" size="sm" />}
            title="Luis Pérez"
            subtitle="Acta de la reunión"
            meta="Ayer"
            selected
          />
          <ListItem index={2} title="Sin interacción" interactive={false} />
        </List>
        <VirtualList
          class="border-border h-40 rounded-lg border"
          aria-label="Lista virtual de muestra"
          items={FILAS_VIRTUALES}
          estimateSize={32}
          renderItem={(fila) => <div class="text-text px-3 py-1.5">{fila}</div>}
        />
      </div>
      <Table<Fichero>
        caption="Ficheros de muestra"
        rows={orden === 'asc' ? FICHEROS : [...FICHEROS].reverse()}
        rowKey={(f) => f.id}
        sortKey="nombre"
        sortDirection={orden}
        onSort={() => setOrden((o) => (o === 'asc' ? 'desc' : 'asc'))}
        selectedKeys={marcados}
        onToggleRow={alternar}
        columns={[
          { key: 'nombre', header: 'Nombre', cell: (f) => f.nombre, sortable: true },
          { key: 'tamano', header: 'Tamaño', cell: (f) => f.tamano, align: 'end' },
        ]}
      />
      <Toolbar aria-label="Formato" class="flex gap-1">
        <IconButton label="Negrita">
          <Icon name="bold" size={18} />
        </IconButton>
        <IconButton label="Cursiva">
          <Icon name="italic" size={18} />
        </IconButton>
        <IconButton label="Enlace">
          <Icon name="link" size={18} />
        </IconButton>
      </Toolbar>
      <BulkActionBar
        count={marcados.size}
        countLabel={`${marcados.size} seleccionados`}
        onClear={nada}
      >
        <Button size="sm" variant="secondary" leadingIcon={<Icon name="download" size={16} />}>
          Descargar
        </Button>
        <Button size="sm" variant="danger">
          Eliminar
        </Button>
      </BulkActionBar>
    </>
  );
}

// ---------------------------------------------------------------- Menús y globos

function Menus({ abierta }: { abierta: boolean }) {
  const [popover, setPopover] = useState(abierta);
  return (
    <Fila rotulo="Menú, panel y ayuda">
      <Dropdown
        label="Acciones"
        trigger={(props) => (
          <Button
            variant="secondary"
            trailingIcon={<Icon name="chevronDown" size={16} />}
            {...props}
          >
            Acciones
          </Button>
        )}
      >
        <MenuItem icon={<Icon name="copies" size={16} />}>Duplicar</MenuItem>
        <MenuItem icon={<Icon name="download" size={16} />}>Descargar</MenuItem>
        <MenuSeparator />
        <MenuItem danger icon={<Icon name="trash" size={16} />}>
          Eliminar
        </MenuItem>
      </Dropdown>
      <Popover
        label="Opciones de vista"
        open={popover}
        onOpenChange={setPopover}
        trigger={(props) => (
          <Button variant="ghost" {...props}>
            Opciones
          </Button>
        )}
      >
        <p class="text-text">Panel con lo que se quiera dentro.</p>
      </Popover>
      <Tooltip label="Una ayuda breve">
        <Button variant="ghost">Pasa por encima</Button>
      </Tooltip>
    </Fila>
  );
}

// ---------------------------------------------------------------- Navegación

function Navegacion() {
  const [activa, setActiva] = useState('entrada');
  return (
    <>
      <TopBar
        brand={<span class="text-text font-semibold">Sytue</span>}
        center={<Input size="sm" aria-label="Buscar" placeholder="Buscar" />}
        actions={
          <>
            <TopBarButton label="Avisos" indicator>
              <Icon name="bell" size={18} />
            </TopBarButton>
            <Avatar name="Ana Martín" size="sm" />
          </>
        }
      />
      <div class="grid gap-4 sm:grid-cols-[16rem_1fr]">
        <div class="border-border h-80 overflow-hidden rounded-lg border">
          <Sidebar
            header={<p class="text-sidebar-fg font-semibold">Correo</p>}
            footer={
              <SidebarAction
                label="Nueva carpeta"
                icon={<Icon name="plus" size={16} />}
                onClick={nada}
              />
            }
          >
            <SidebarSection title="Buzones">
              {[
                { id: 'entrada', label: 'Entrada', icon: 'mail' as const, count: 4 },
                { id: 'enviados', label: 'Enviados', icon: 'arrowUpRight' as const },
                { id: 'papelera', label: 'Papelera', icon: 'trash' as const },
              ].map((b) => (
                <SidebarItem
                  key={b.id}
                  label={b.label}
                  icon={<Icon name={b.icon} size={16} />}
                  active={activa === b.id}
                  {...(b.count ? { count: b.count } : {})}
                  onClick={() => setActiva(b.id)}
                />
              ))}
            </SidebarSection>
            <SidebarSection title="Plegable" collapsible defaultOpen={false}>
              <SidebarItem label="Dentro" />
            </SidebarSection>
            <SidebarSection title="Deshabilitado">
              <SidebarItem label="No disponible" disabled />
            </SidebarSection>
          </Sidebar>
        </div>
        <div class="flex flex-col gap-3">
          {/* Seis niveles: la ruta del explorador, que se pliega en «…» si no cabe. */}
          <Migas
            label="Ruta"
            migas={['Mi unidad', 'Proyectos', '2026', 'Cliente', 'Entregas', 'Final'].map(
              (nombre) => ({ clave: nombre, nombre, onIr: nada }),
            )}
          />
          <BarraMovil
            menu={{
              id: 'm-menu',
              controla: 'm-nav',
              abierto: false,
              alPulsar: nada,
              aviso: 'Hay novedades',
            }}
            titulo="Barra del teléfono"
            acciones={
              <SinBarraUnica>
                <IconButton label="Buscar">
                  <Icon name="search" size={18} />
                </IconButton>
              </SinBarraUnica>
            }
          />
          {/* Dentro de la barra única de la suite, lo de `SinBarraUnica` no sale: la
              barra de la app sube a la topbar. */}
          <BarraUnicaContext.Provider value={{ destinos: null }}>
            <SinBarraUnica>
              <p class="text-text">Esto no debería verse.</p>
            </SinBarraUnica>
          </BarraUnicaContext.Provider>
          <div class="border-border h-64 overflow-auto rounded-lg border">
            <AppGrid current="mail" />
          </div>
        </div>
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Armazones

/**
 * Los armazones miden la ventana (`100dvh`) y fijan sus cajones: dentro de un marco con
 * `transform` lo fijo se coloca respecto al marco, y `*:h-full!` le baja la altura a la
 * del marco. Así se ven enteros sin tapar el resto del muestrario.
 */
const MARCO =
  'border-border relative h-[26rem] transform-gpu overflow-hidden rounded-lg border *:h-full!';

function Armazones() {
  return (
    <>
      <div class={MARCO}>
        <AppShell
          title="Bandeja"
          topbar={<TopBar brand={<span class="text-text font-semibold">Sytue Mail</span>} />}
          sidebar={
            <Sidebar>
              <SidebarSection title="Buzones">
                <SidebarItem label="Entrada" active />
                <SidebarItem label="Enviados" />
              </SidebarSection>
            </Sidebar>
          }
        >
          <div class="p-4">
            <SectionHeader title="Contenido de la app" level={2} />
          </div>
        </AppShell>
      </div>
      <div class={MARCO}>
        <AdminShell
          title="Administración"
          sidebar={
            <Sidebar>
              <SidebarItem label="Personas" active />
              <SidebarItem label="Dominios" />
            </Sidebar>
          }
        >
          <SettingsShell
            title="Ajustes de la organización"
            description="Lo que ven todas las personas de la organización."
            indice={[
              { id: 'general', rotulo: 'General' },
              { id: 'seguridad', rotulo: 'Seguridad' },
            ]}
          >
            <SettingsSection id="general" title="General" description="Nombre y logotipo.">
              <Input aria-label="Nombre de la organización" value="Cooperativa" />
            </SettingsSection>
            <SettingsSection id="seguridad" title="Seguridad" plain>
              <Switch label="Exigir doble factor" checked />
            </SettingsSection>
          </SettingsShell>
        </AdminShell>
      </div>
      <div class={MARCO}>
        <PaginaNoEncontrada
          overline="Error 404"
          title="Esta página no existe"
          description="Puede que el enlace esté mal escrito o que ya no exista."
          action="Volver al inicio"
          href="#muestrario"
        />
      </div>
    </>
  );
}

// ---------------------------------------------------------------- Diálogos

/** El botón que abre una capa y la capa, abierta desde el principio si se pide. */
function useAbierta(inicial: boolean): [boolean, () => void, () => void] {
  const [abierta, setAbierta] = useState(inicial);
  return [abierta, () => setAbierta(true), () => setAbierta(false)];
}

function DialogoModal({ abierta }: { abierta: boolean }) {
  const [open, abrir, cerrar] = useAbierta(abierta);
  return (
    <>
      <Button onClick={abrir}>Abrir diálogo</Button>
      <Modal
        open={open}
        onClose={cerrar}
        title="Diálogo de muestra"
        footer={
          <>
            <Button variant="ghost" onClick={cerrar}>
              Cancelar
            </Button>
            <Button onClick={cerrar}>Guardar</Button>
          </>
        }
      >
        <FieldGroup label="Nombre" htmlFor="m-modal-nombre">
          <Input id="m-modal-nombre" value="Ana Martín" />
        </FieldGroup>
      </Modal>
    </>
  );
}

function DialogoConfirmar({ abierta }: { abierta: boolean }) {
  const [open, abrir, cerrar] = useAbierta(abierta);
  return (
    <>
      <Button variant="danger" onClick={abrir}>
        Eliminar…
      </Button>
      <ConfirmDialog
        open={open}
        onClose={cerrar}
        onConfirm={cerrar}
        title="¿Eliminar «Presupuesto 2027»?"
        confirmLabel="Eliminar"
        cancelLabel="Cancelar"
        tone="danger"
      >
        <p class="text-text">Irá a la papelera durante 30 días.</p>
      </ConfirmDialog>
    </>
  );
}

function DialogoAtajos({ abierta }: { abierta: boolean }) {
  const [open, abrir, cerrar] = useAbierta(abierta);
  return (
    <>
      <Button variant="secondary" onClick={abrir}>
        Atajos
      </Button>
      <ShortcutsModal
        open={open}
        onClose={cerrar}
        title="Atajos de muestra"
        groups={[
          {
            titulo: 'General',
            atajos: [
              { teclas: ['?'], descripcion: 'Ver los atajos' },
              { teclas: ['Ctrl', 'K'], descripcion: 'Buscar' },
            ],
          },
          {
            // Un segundo grupo con una combinación más ancha: las descripciones de los
            // dos tienen que caer en la misma vertical.
            titulo: 'Edición',
            atajos: [
              { teclas: ['Ctrl', 'Mayús', 'Z'], descripcion: 'Rehacer' },
              { teclas: ['Ctrl', 'Alt', 'Mayús', 'V'], descripcion: 'Pegar sin formato' },
            ],
          },
        ]}
      />
    </>
  );
}

const OPCIONES_PALETA: OpcionDePaleta[] = [
  { id: 'nuevo', rotulo: 'Nuevo mensaje', grupo: 'Acciones', icono: 'mail', pista: 'C' },
  { id: 'buscar', rotulo: 'Buscar', grupo: 'Acciones', icono: 'search' },
  { id: 'ajustes', rotulo: 'Ajustes', grupo: 'Ir a', icono: 'settings' },
];

function DialogoPaleta({ abierta }: { abierta: boolean }) {
  const [open, abrir, cerrar] = useAbierta(abierta);
  const [consulta, setConsulta] = useState('');
  return (
    <>
      <Button variant="secondary" onClick={abrir}>
        Paleta de comandos
      </Button>
      <PaletaDeComandos
        open={open}
        onClose={cerrar}
        etiqueta="Paleta de comandos"
        rotuloCampo="Buscar un comando"
        placeholder="Escribe un comando"
        rotuloResultados="Comandos"
        sinResultados={(c) => `Nada para «${c}»`}
        consulta={consulta}
        onConsulta={setConsulta}
        opciones={OPCIONES_PALETA.filter((o) =>
          o.rotulo.toLowerCase().includes(consulta.toLowerCase()),
        )}
        onElegir={cerrar}
      />
    </>
  );
}

const CONFIRMAR_EDITOR = {
  titulo: 'Diálogo del editor',
  mensaje: '¿Seguir con la acción?',
  confirmLabel: 'Seguir',
};

/** El diálogo imperativo del editor: `confirmarEnEditor` lo abre desde una función suelta. */
function DialogoDelEditor({ abierta }: { abierta: boolean }) {
  useEffect(() => {
    if (abierta) void confirmarEnEditor(CONFIRMAR_EDITOR);
  }, [abierta]);
  return (
    <>
      <DialogosDelEditor />
      <Button variant="ghost" onClick={() => void confirmarEnEditor(CONFIRMAR_EDITOR)}>
        Confirmar desde el editor
      </Button>
    </>
  );
}

function lanzarAvisos(notify: ReturnType<typeof useToast>['notify']): void {
  notify({ tone: 'success', title: 'Guardado', description: 'Los cambios están a salvo.' });
  notify({ tone: 'info', title: 'Sincronizando' });
  notify({ tone: 'warning', title: 'Queda poco espacio' });
  notify({
    tone: 'danger',
    title: 'No se pudo enviar',
    action: { label: 'Reintentar', onClick: nada },
  });
}

/**
 * Los cuatro tonos de aviso; con `abierta`, salen ya al montar. Se ven tres: el cuarto
 * espera en cola a que se vaya uno, que es el tope de la pila y también se revisa.
 */
function Avisos({ abierta }: { abierta: boolean }) {
  const { notify } = useToast();
  const lanzar = () => lanzarAvisos(notify);
  // `notify` es estable (useCallback del proveedor): esto corre una vez al montar.
  useEffect(() => {
    if (abierta) lanzarAvisos(notify);
  }, [abierta, notify]);
  return (
    <Button variant="secondary" onClick={lanzar}>
      Lanzar avisos
    </Button>
  );
}

/**
 * Las capas de «dialogos» que se abren una a una con `abrir=dialogos:<capa>`: son modales
 * y solo cabe una a la vez, pero cada una tiene que verse (y probarse) ABIERTA, no solo su
 * botón. `dialogos` a secas abre la primera con los avisos.
 */
export const CAPAS_DE_DIALOGOS = ['modal', 'confirmar', 'atajos', 'paleta', 'editor'] as const;

function Dialogos({ abierta, cual }: { abierta: boolean; cual?: string | undefined }) {
  const es = (capa: (typeof CAPAS_DE_DIALOGOS)[number]) =>
    abierta && (cual ?? CAPAS_DE_DIALOGOS[0]) === capa;
  return (
    <Fila rotulo="Capas sobre la página">
      <DialogoModal abierta={es('modal')} />
      <DialogoConfirmar abierta={es('confirmar')} />
      <DialogoAtajos abierta={es('atajos')} />
      <DialogoPaleta abierta={es('paleta')} />
      <DialogoDelEditor abierta={es('editor')} />
      <Avisos abierta={abierta && cual === undefined} />
    </Fila>
  );
}

/**
 * Las familias, en el orden de la página. Las de capa (`capa: true`) son las que solo se
 * ven abiertas: el test las recorre abiertas y la página las abre con `?abrir=<clave>`.
 */
export const FAMILIAS: readonly Familia[] = [
  { clave: 'botones', titulo: 'Botones', Pintar: Botones },
  { clave: 'iconos', titulo: 'Iconos, marca y avatar', Pintar: Iconos },
  { clave: 'campos', titulo: 'Campos y ayuda', Pintar: Campos },
  { clave: 'eleccion', titulo: 'Elección, pestañas y pasos', Pintar: Eleccion },
  { clave: 'estado', titulo: 'Estado, carga y avisos en línea', Pintar: Estado },
  { clave: 'tarjetas', titulo: 'Tarjetas y resúmenes', Pintar: Tarjetas },
  { clave: 'listas', titulo: 'Listas, tablas y barras', Pintar: Listas },
  { clave: 'menus', titulo: 'Menús y globos', capa: true, Pintar: Menus },
  { clave: 'navegacion', titulo: 'Navegación', Pintar: Navegacion },
  { clave: 'armazones', titulo: 'Armazones', Pintar: Armazones },
  { clave: 'dialogos', titulo: 'Diálogos y avisos', capa: true, Pintar: Dialogos },
];

export interface MuestrarioProps {
  /** Las familias que se montan; por defecto, todas. */
  familias?: readonly string[] | undefined;
  /**
   * La familia de capa que sale abierta (`?abrir=` en la página), y tras `:` cuál de sus
   * capas (`dialogos:paleta`, ver `CAPAS_DE_DIALOGOS`).
   */
  abrir?: string | undefined;
}

/**
 * El muestrario entero. Lleva su `ToastProvider` porque los avisos son una de sus
 * familias; el `I18nProvider` lo pone quien lo monta (la página o el test), con el
 * idioma que quiera revisar.
 */
export function Muestrario({ familias, abrir }: MuestrarioProps): JSX.Element {
  const visibles = familias ? FAMILIAS.filter((f) => familias.includes(f.clave)) : FAMILIAS;
  const [familiaAbierta, cual] = abrir?.split(':') ?? [];
  return (
    <ToastProvider>
      <main id="muestrario" class="bg-bg text-text flex flex-col gap-10 p-4 sm:p-6">
        {visibles.map(({ clave, titulo, Pintar }) => (
          <section
            key={clave}
            data-familia={clave}
            aria-labelledby={`familia-${clave}`}
            class="flex flex-col gap-4"
          >
            <h2 id={`familia-${clave}`} class="sytue-h3 text-text">
              {titulo}
            </h2>
            <Pintar abierta={familiaAbierta === clave} cual={cual} />
          </section>
        ))}
      </main>
    </ToastProvider>
  );
}
