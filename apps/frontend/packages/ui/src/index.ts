/**
 * @sytue/ui — design system de Sytue One (Preact 10).
 *
 * Componentes canonicos Mattermost-moderno x Sytue, definidos UNA sola vez.
 * Las apps los consumen; no crean variantes ni UI ad-hoc (docs/design.md §11).
 * Importar los estilos una vez por app: `import "@sytue/ui/styles.css";`.
 */

// Tema
export {
  ThemeProvider,
  useTheme,
  themeInitScript,
  themeColorSwatch,
  DEFAULT_COLOR,
  DEFAULT_FORMA,
  THEME_COLORS,
  THEME_FORMAS,
} from './theme/ThemeProvider.js';
export { SUFIJO_ICONOS, basePathScript, swRegisterScript, withBase } from './basePath.js';
export type {
  ThemeColor,
  ThemeForma,
  ThemeMode,
  ResolvedTheme,
  ThemeProviderProps,
} from './theme/ThemeProvider.js';

// Primitivas y utilidades
export { cx } from './lib/cx.js';
export type { ClassValue } from './lib/cx.js';

export { saneaHtmlDeDocumento } from './lib/saneaHtml.js';
// ¿Esta tecla es de la app o de quien está escribiendo? La comprobación estaba
// escrita tres veces (Mail, Calendar, el visor de PDF de Docs) y ninguna miraba
// lo mismo. Ver el módulo: las tres partes tapan agujeros distintos.
export {
  esAtajoDeApp,
  enCampoDeTexto,
  haySuperposicionAbierta,
  esMac,
  atajoDelSistema,
  useAtajo,
} from './lib/atajos.js';
// Tamaños de fichero con el separador decimal del idioma. Estaba copiado siete
// veces, cuatro de ellas con `toFixed` (que siempre pone el punto inglés).
export { formatBytes } from './lib/formatBytes.js';
export { useNavDrawer, DRAWER_FOCUSABLE } from './lib/useNavDrawer.js';
export { useMobileBack } from './lib/useMobileBack.js';
export { useEnHoja, CONSULTA_HOJA } from './lib/useEnHoja.js';
// La fila de barra que en móvil se desplaza en vez de envolver, y el aviso de
// que queda contenido. Lo usan las barras de toda la suite; ver el módulo para
// por qué va siempre acompañado de `data-scroll-x`.
export { FILA_UNICA_EN_MOVIL, MASCARA_SCROLL_X, useScrollRestante } from './lib/scrollX.js';
// Acota un panel flotante a la ventana. Lo usan Dropdown y Popover; queda
// exportado para el panel que aún no puede migrar a ellos (ReminderCenter de
// Calendar), que si no se sale por la izquierda y no hay forma de alcanzarlo.
export { useFitInViewport } from './lib/useFitInViewport.js';
export { limitesViewport, observarViewport } from './lib/viewport.js';
// Sondeo que se para con la pestaña de fondo: nadie mira, nadie pregunta.
export { useAhora, useSondeo } from './lib/useSondeo.js';
// Ref con el último valor recibido, escrita al confirmar y no durante el render.
export { useUltimo } from './lib/useUltimo.js';
export { useTablist } from './lib/useTablist.js';
export type { Tablist } from './lib/useTablist.js';
// Tinta: entrada de dibujo a mano alzada (lápiz, dedo, ratón) con rechazo de la
// palma, presión/inclinación y muestras agrupadas. Vive aquí para que PDF,
// Slides y Docs anoten con el mismo cimiento en vez de escribirlo tres veces.
// Límite de accesibilidad documentado en el propio módulo.
export { useInk, createInkTracker } from './lib/ink.js';
export type {
  InkPoint,
  InkStroke,
  InkSink,
  InkOptions,
  InkOrigin,
  InkTracker,
  InkPointerType,
  InkPointerSample,
} from './lib/ink.js';

// Deshacer y rehacer
export type { Historial } from './lib/historial.js';
export {
  TOPE,
  nuevoHistorial,
  anotar,
  deshacer,
  rehacer,
  sePuedeDeshacer,
  sePuedeRehacer,
} from './lib/historial.js';

// Arrastre, redimensión y navegación en lienzos
export {
  clamp,
  calcularArrastre,
  calcularRedimension,
  calcularEnvolvente,
  calcularLimitesArrastre,
  calcularPasoTeclado,
} from './lib/arrastre.js';
export type {
  Coordenadas,
  Dimensiones,
  Rectangulo,
  RectanguloConId,
  Limites,
  TiradorRedimension,
} from './lib/arrastre.js';

// Hook de puntero con captura
export { usePointerDrag } from './lib/usePointerDrag.js';
export type { DragState, DragMoveEvent, UsePointerDragOptions } from './lib/usePointerDrag.js';

export { Icon } from './components/Icon.js';
export type { IconName, IconProps } from './components/Icon.js';
export { Glifo, grosorDeGlifo, puntos, subtrazosCerrados } from './components/Glifo.js';
export type { GlifoProps } from './components/Glifo.js';

// Catalogo de apps (app switcher)
export {
  suiteApps,
  APP_SWITCHER_ORDER,
  CATALOGO,
  WORKHUB,
  appForKey,
  appsPresentables,
  hrefDe,
  puedeAdministrar,
  CLAVE_ADMIN,
} from './lib/apps.js';
export type { SuiteApp, EntradaCatalogo, SuiteV4, Disponibilidad } from './lib/apps.js';

// Assets de marca
export { logos } from './assets/index.js';
export type { LogoKey } from './assets/index.js';
export { productIcons, productIconsSm } from './assets/icons.js';
export { ProductIcon, PRODUCT_ICON_SM_BELOW } from './components/ProductIcon.js';
export type { ProductIconProps } from './components/ProductIcon.js';
export { illustrations } from './assets/illustrations.js';
export type { IllustrationKey } from './assets/illustrations.js';
// El mapa de arriba entrega URLs, y una URL dentro de un <img> es un documento
// SVG aparte: NO hereda las variables CSS del tema, así que el line-art sale sin
// color. Para los spots que se tiñen, este componente los pinta en línea.
export { Illustration } from './components/Illustration.js';
export type { IllustrationProps, SpotName } from './components/Illustration.js';

// Componentes
export { Button } from './components/Button.js';
export type { ButtonProps, ButtonVariant, ButtonSize } from './components/Button.js';
export { IconButton } from './components/IconButton.js';
export type { IconButtonProps, IconButtonSize, IconButtonTone } from './components/IconButton.js';
export { Spinner } from './components/Spinner.js';
export type { SpinnerProps } from './components/Spinner.js';
export { Skeleton } from './components/Skeleton.js';
export type { SkeletonProps } from './components/Skeleton.js';
// Cuota consumida (disco, asientos, aforo): avisa al 80 % y al 95 %, y lo dice
// con palabras además de con color.
export { Meter } from './components/Meter.js';
export type { MeterProps } from './components/Meter.js';
// Progreso de algo que está pasando (una subida): sin los avisos del medidor.
export { ProgressBar } from './components/ProgressBar.js';
export type { ProgressBarProps } from './components/ProgressBar.js';
export { BarChart } from './components/BarChart.js';
export type { BarChartProps, BarChartPoint } from './components/BarChart.js';
export {
  Card,
  CardHeader,
  CardTitle,
  CardBody,
  CardFooter,
  CardIcon,
  CARD_TONES,
  cardTone,
} from './components/Card.js';
export type { CardProps, CardIconProps, CardTone } from './components/Card.js';
export { Badge } from './components/Badge.js';
export { ReadOnlyBar } from './components/ReadOnlyBar.js';
export type { ReadOnlyBarProps, ReadOnlyBarAction } from './components/ReadOnlyBar.js';
export type { BadgeProps, BadgeTone } from './components/Badge.js';
export { ConfirmDialog } from './components/ConfirmDialog.js';
export { useConfirm } from './components/useConfirm.js';
export { usePrompt } from './components/usePrompt.js';
export {
  DialogosDelEditor,
  confirmarEnEditor,
  preguntarEnEditor,
} from './components/dialogosImperativos.js';
export type { PreguntarOpciones } from './components/usePrompt.js';
export type { ConfirmarOpciones } from './components/useConfirm.js';
export type { ConfirmDialogProps } from './components/ConfirmDialog.js';
export { Avatar } from './components/Avatar.js';
export type { AvatarProps, AvatarSize } from './components/Avatar.js';
export { AvatarPicker } from './components/AvatarPicker.js';
export type { AvatarPickerProps, AvatarPickerLabels } from './components/AvatarPicker.js';
export {
  fileToAvatarDataUri,
  presetAvatarDataUri,
  reducirImagen,
  ImageError,
  PRESET_AVATAR_COLORS,
  MAX_PHOTO_BYTES,
} from './lib/avatarImage.js';
export type { ImageErrorCode, ReducirImagenOpciones, ImagenReducida } from './lib/avatarImage.js';
export { OrgMark, colorDeOrg } from './components/OrgMark.js';
export type { OrgMarkProps } from './components/OrgMark.js';
export {
  FieldGroup,
  describedBy,
  Input,
  Textarea,
  Select,
  Checkbox,
  Radio,
  Switch,
} from './components/Form.js';
export type {
  FieldGroupProps,
  InputProps,
  InputSize,
  TextareaProps,
  SelectProps,
  ChoiceProps,
  SwitchProps,
} from './components/Form.js';
export { LanguageSelect } from './components/LanguageSelect.js';
export type { LanguageSelectProps } from './components/LanguageSelect.js';
export { ShortcutsModal } from './components/ShortcutsModal.js';
export type { AtajoDeTeclado, GrupoDeAtajos } from './components/ShortcutsModal.js';
export { PaletaDeComandos } from './components/PaletaDeComandos.js';
export type { OpcionDePaleta, PaletaDeComandosProps } from './components/PaletaDeComandos.js';
export { Modal } from './components/Modal.js';
// Selector de elementos enfocables: lo comparten el Modal y cualquier otra
// superficie que atrape el foco (el visor de presentación de Docs).
export { FOCUSABLE } from './components/Modal.js';
export type { ModalProps, ModalSize } from './components/Modal.js';
// Barra de herramientas: el rol y el foco roving que promete, juntos y de una pieza.
export { Toolbar } from './components/Toolbar.js';
export type { ToolbarProps } from './components/Toolbar.js';
// La barra que sale al marcar varios: recuento anunciado, una sola parada de
// tabulación y Escape para soltar la selección.
export { BulkActionBar } from './components/BulkActionBar.js';
export type { BulkActionBarProps } from './components/BulkActionBar.js';
export { Dropdown, MenuItem, MenuSeparator } from './components/Dropdown.js';
export {
  BOTON_DE_BARRA,
  CAMPO_DE_BARRA,
  CAMPO_DE_BARRA_BASE,
  SEPARADOR_DE_BARRA,
  BARRA_DE_EDICION,
} from './lib/cromoDeEditor.js';
export type { DropdownProps, MenuItemProps } from './components/Dropdown.js';
export { Popover } from './components/Popover.js';
export type { PopoverProps } from './components/Popover.js';
export { Tooltip } from './components/Tooltip.js';
export type { TooltipProps } from './components/Tooltip.js';
export { Tabs } from './components/Tabs.js';
export type { TabsProps, TabItem } from './components/Tabs.js';
export { List, ListItem } from './components/List.js';
export type { ListProps, ListItemProps } from './components/List.js';
export { VirtualList } from './components/VirtualList.js';
export type { VirtualListProps, VirtualListHandle } from './components/VirtualList.js';
export { Table } from './components/Table.js';
export type { TableProps, TableColumn, SortDirection } from './components/Table.js';
export { EmptyState } from './components/EmptyState.js';
export type { EmptyStateProps } from './components/EmptyState.js';
export { PaginaNoEncontrada } from './components/PaginaNoEncontrada.js';
export type { PaginaNoEncontradaProps } from './components/PaginaNoEncontrada.js';
export { ToastProvider, useToast } from './components/Toast.js';
export type { ToastInput, ToastTone } from './components/Toast.js';

// Configuración guiada (asistentes paso a paso + páginas de ajustes).
// El usuario no lee documentación: el flujo le explica qué pasa, qué necesita y
// dónde conseguirlo, valida en vivo y resume antes de aplicar.
export { Wizard, Stepper } from './components/Wizard.js';
export type { WizardProps, WizardStep, StepperProps, WizardLabels } from './components/Wizard.js';
export { SettingsShell, SettingsSection } from './components/SettingsShell.js';
export type { SettingsShellProps, SettingsSectionProps } from './components/SettingsShell.js';
export { ChoiceCard } from './components/ChoiceCard.js';
export type { ChoiceCardProps } from './components/ChoiceCard.js';
export { FieldHint, InlineHelp } from './components/FieldHint.js';
export type { FieldHintProps, InlineHelpProps, HintTone } from './components/FieldHint.js';
export { ReviewSummary } from './components/ReviewSummary.js';
export type { ReviewSummaryProps, ReviewItem } from './components/ReviewSummary.js';
// «Qué pasa con los datos» al conectar algo con un tercero: entra, sale o las dos
// cosas. Va en TODOS los asistentes de integración, dicho igual en toda la suite.
export { DataFlow, dataFlowMode } from './components/DataFlow.js';
export type { DataFlowProps, DataCategory, DataFlowMode } from './components/DataFlow.js';

// Layout / shell visual
export {
  Sidebar,
  SidebarSection,
  SidebarItem,
  SidebarAction,
  SIDEBAR_ACTION_CLASS,
} from './components/Sidebar.js';
export { MobileFab } from './components/MobileFab.js';
export type { MobileFabProps } from './components/MobileFab.js';
export type {
  SidebarProps,
  SidebarSectionProps,
  SidebarItemProps,
  SidebarActionProps,
} from './components/Sidebar.js';
export { TopBar, TopBarButton } from './components/TopBar.js';
export type { TopBarProps, TopBarButtonProps } from './components/TopBar.js';
export { AppGrid } from './components/AppGrid.js';
export type { AppGridProps } from './components/AppGrid.js';
export { AppShell } from './components/AppShell.js';
export type { AppShellProps } from './components/AppShell.js';
export { AdminShell } from './components/AdminShell.js';
export type { AdminShellProps } from './components/AdminShell.js';
export { SkipLink } from './components/SkipLink.js';
export type { SkipLinkProps } from './components/SkipLink.js';
export { SectionHeader } from './components/SectionHeader.js';
export type { SectionHeaderProps } from './components/SectionHeader.js';
