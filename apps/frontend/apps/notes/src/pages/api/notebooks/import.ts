/**
 * POST /api/notebooks/import  — crea un cuaderno desde un `.onex`, Markdown
 *   (`.md` o `.zip`) o Evernote (`.enex`); `?name=` es el nombre del fichero.
 * POST /api/notebooks/import?preview=1 — lo lee y lo devuelve SIN crear nada.
 *
 * La vista previa comparte ruta con la importación a propósito: si aceptara
 * ficheros que la otra rechaza, sería una vista previa de algo que luego no se
 * puede importar.
 */
import type { APIRoute } from 'astro';
import { createNotebookImportHandler } from '../../../lib/notebookImport.js';
import { conCliente, fallo, json } from '../../../lib/notes.js';

export const prerender = false;

export const POST: APIRoute = createNotebookImportHandler({ conCliente, fallo, json });
