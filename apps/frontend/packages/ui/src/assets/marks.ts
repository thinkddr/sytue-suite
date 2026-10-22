/**
 * @sytue/ui/marks — la MARCA DE APLICACIÓN de cada producto.
 *
 * Es lo que identifica al producto en la pestaña, en el dock y en la pantalla de
 * inicio: el badge de la casa con el color del producto y su glifo, generado por
 * `scripts/gen-marks.sh` en `branding/marks/`.
 *
 * POR QUÉ EXISTE ESTE FICHERO. Había DOS familias y el conmutador de aplicaciones
 * usaba la vieja. `branding/icons/` no se toca desde que nació Sytue Notepads (06/09
 * 21:15) y las marcas se rehicieron enteras esa misma noche: primero el logotipo
 * «sy:» por color, después el glifo del producto en vez del monograma, luego el
 * bocadillo de Rooms y por último el centrado óptico. El favicon y el icono de
 * escritorio salen de esa familia nueva; el conmutador seguía con la anterior, así
 * que el mismo producto se veía de dos maneras según dónde se mirara.
 *
 * `chat` y `meet` NO tienen marca, y no es un olvido: son hosts legados que solo
 * devuelven un 301 al canónico. Quien los pida se queda con el icono de la familia
 * anterior, que para un redirector es más que suficiente.
 */
import type { ProductKey } from '@sytue/config';

import calendar from './marks/calendar.svg?url';
import cast from './marks/cast.svg?url';
import cloud from './marks/cloud.svg?url';
import console_ from './marks/console.svg?url';
import contacts from './marks/contacts.svg?url';
import docs from './marks/docs.svg?url';
import drive from './marks/drive.svg?url';
import erp from './marks/erp.svg?url';
import hub from './marks/hub.svg?url';
import id from './marks/id.svg?url';
import mail from './marks/mail.svg?url';
import notes from './marks/notes.svg?url';
import photos from './marks/photos.svg?url';
import one from './marks/one.svg?url';
import pages from './marks/pages.svg?url';
import bases from './marks/bases.svg?url';
import rooms from './marks/rooms.svg?url';
import sheets from './marks/sheets.svg?url';
import slides from './marks/slides.svg?url';
import pdf from './marks/pdf.svg?url';

/**
 * Parcial a propósito: la ausencia de `chat` y `meet` es la que hace que el
 * consumidor tenga que decidir qué pone en su lugar, en vez de recibir una cadena
 * vacía y pintar un hueco.
 */
export const productMarks: Partial<Record<ProductKey, string>> = {
  calendar,
  cast,
  cloud,
  console: console_,
  contacts,
  docs,
  drive,
  erp,
  hub,
  id,
  mail,
  notes,
  one,
  pages,
  bases,
  pdf,
  photos,
  rooms,
  sheets,
  slides,
};
