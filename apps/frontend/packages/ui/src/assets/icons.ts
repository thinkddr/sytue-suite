/**
 * @sytue/ui/icons — iconos de producto Sytue.
 *
 * SVG en branding/icons/ (mismo grid geometrico 24/40). Importados como URL
 * para el AppGrid/Launcher y etiquetas de producto. El orden canonico del app
 * switcher lo fija branding/index.json (appSwitcher.order).
 */
import one from './icons/one.svg?url';
import id from './icons/id.svg?url';
import drive from './icons/drive.svg?url';
import docs from './icons/docs.svg?url';
import sheets from './icons/sheets.svg?url';
import slides from './icons/slides.svg?url';
// CertiPDF no tiene icono aparte: su marca hace de icono a cualquier tamaño.
import pdf from './marks/pdf.svg?url';
import mail from './icons/mail.svg?url';
import calendar from './icons/calendar.svg?url';
import contacts from './icons/contacts.svg?url';
import notes from './icons/notes.svg?url';
import photos from './icons/photos.svg?url';
import chat from './icons/chat.svg?url';
import meet from './icons/meet.svg?url';
import rooms from './icons/rooms.svg?url';
import cloud from './icons/cloud.svg?url';
import console_ from './icons/console.svg?url';
import hub from './icons/hub.svg?url';
import erp from './icons/erp.svg?url';
import cast from './icons/cast.svg?url';
import pages from './icons/pages.svg?url';
import bases from './icons/bases.svg?url';

import oneSm from './icons/one-sm.svg?url';
import idSm from './icons/id-sm.svg?url';
import driveSm from './icons/drive-sm.svg?url';
import docsSm from './icons/docs-sm.svg?url';
import sheetsSm from './icons/sheets-sm.svg?url';
import slidesSm from './icons/slides-sm.svg?url';
import mailSm from './icons/mail-sm.svg?url';
import calendarSm from './icons/calendar-sm.svg?url';
import contactsSm from './icons/contacts-sm.svg?url';
import notesSm from './icons/notes-sm.svg?url';
import photosSm from './icons/photos-sm.svg?url';
import chatSm from './icons/chat-sm.svg?url';
import roomsSm from './icons/rooms-sm.svg?url';
import cloudSm from './icons/cloud-sm.svg?url';
import console_Sm from './icons/console-sm.svg?url';
import hubSm from './icons/hub-sm.svg?url';
import erpSm from './icons/erp-sm.svg?url';
import castSm from './icons/cast-sm.svg?url';
import pagesSm from './icons/pages-sm.svg?url';
import basesSm from './icons/bases-sm.svg?url';

import type { ProductKey } from '@sytue/config';

export const productIcons: Record<ProductKey, string> = {
  one,
  id,
  drive,
  docs,
  sheets,
  slides,
  pdf,
  mail,
  calendar,
  contacts,
  notes,
  photos,
  chat,
  meet,
  rooms,
  cloud,
  console: console_,
  hub,
  erp,
  cast,
  pages,
  bases,
};

/**
 * Variante de TAMAÑO ÓPTICO para 15-20 px: mismo dibujo con trazo 2.1 en vez de
 * 1.7 y, donde hacía falta, con el detalle podado. Con 1.7 el glifo no llega a
 * blanco a 16 px (0,68 px de trazo): la auditoría midió 2,88:1 de contraste en
 * mail, por debajo del mínimo 3:1 de WCAG 1.4.11 para objetos gráficos. Mismo
 * criterio que los tamaños ópticos de SF Symbols y Material.
 *
 * `meet` no tiene variante (producto retirado de la UI, unificado en Rooms):
 * reutiliza la de rooms para no dejar el Record incompleto.
 */
export const productIconsSm: Record<ProductKey, string> = {
  one: oneSm,
  id: idSm,
  drive: driveSm,
  docs: docsSm,
  sheets: sheetsSm,
  slides: slidesSm,
  pdf,
  mail: mailSm,
  calendar: calendarSm,
  contacts: contactsSm,
  notes: notesSm,
  photos: photosSm,
  chat: chatSm,
  meet: roomsSm,
  rooms: roomsSm,
  cloud: cloudSm,
  console: console_Sm,
  hub: hubSm,
  erp: erpSm,
  cast: castSm,
  pages: pagesSm,
  bases: basesSm,
};
