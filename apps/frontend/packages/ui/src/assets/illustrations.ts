/**
 * @sytue/ui/illustrations — ilustraciones de servicio Sytue.
 *
 * SVG line-art en branding/illustrations/ + og/ (fuente de verdad, vendorizados
 * por scripts/sync-branding.sh). Importados como URL para empty states
 * (EmptyState), hero de www y og/favicon maskable. Claves camelCase alineadas
 * con branding/index.json (illustrations + og). Los empty states/spots usan
 * roles var(--sytue-*) y respetan claro/oscuro; hero y og son superficie de
 * marca fija. Ver brand-guidelines.md §9.
 */
import emptyMail from './illustrations/empty-mail.svg?url';
import emptyDrive from './illustrations/empty-drive.svg?url';
import emptyCalendar from './illustrations/empty-calendar.svg?url';
import emptyChat from './illustrations/empty-chat.svg?url';
import emptyContacts from './illustrations/empty-contacts.svg?url';
import emptyNotes from './illustrations/empty-notes.svg?url';
import emptySearch from './illustrations/empty-search.svg?url';
import emptyPhotos from './illustrations/empty-photos.svg?url';
import errorRetry from './illustrations/error-retry.svg?url';
import allDone from './illustrations/all-done.svg?url';
import heroSuite from './illustrations/hero-suite.svg?url';
import spotSovereignty from './illustrations/spot-sovereignty.svg?url';
import spotUpload from './illustrations/spot-upload.svg?url';
import ogImage from './illustrations/og-image.svg?url';
import faviconMaskable from './illustrations/favicon-maskable.svg?url';

export const illustrations = {
  emptyMail,
  emptyDrive,
  emptyCalendar,
  emptyChat,
  emptyContacts,
  emptyNotes,
  emptySearch,
  emptyPhotos,
  errorRetry,
  allDone,
  heroSuite,
  spotSovereignty,
  spotUpload,
  ogImage,
  faviconMaskable,
} as const;

export type IllustrationKey = keyof typeof illustrations;
