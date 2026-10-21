// Detecta texto castellano incrustado en el código: se carga cada app pidiendo
// inglés y se busca en lo RENDERIZADO. Es la única comprobación que ha
// encontrado estos defectos de verdad — un grep sobre el fuente falla en las
// tres formas que más se usan:
//
//   1. `label="Congelar paneles"`         (atributo)
//   2. `>Todo el día<`                    (texto entre etiquetas)
//   3. un rótulo solo en su línea de JSX  (la más común, y la que se escapó)
//
// Y no ve nada de lo que sale de una constante compartida: los nombres de
// producto del conmutador de aplicaciones estaban fijados en español en
// `@sytue/ui` —con las traducciones ya escritas y sin usar en `@sytue/i18n`—,
// así que las once apps enseñaban «Correo electrónico» a cualquier usuario.
//
// Sirve para cualquier idioma; se pide inglés porque es la lengua base y la que
// cubre el catálogo entero.
import { expect, test } from '@playwright/test';
import { APPS } from './helpers';

/** Palabras que no existen en inglés: si aparecen pidiendo inglés, están en duro. */
const CASTELLANO =
  /\b(Hoy|Crear|Buscar|Guardar|Cancelar|Eliminar|Nuevo|Nueva|Enviar|Ajustes|Invitaciones|Correo|Archivos|Documentos|Carpeta|Compartir|Volver|Cerrar|Añadir|Editar|Borrar|Nombre|Descripción|Configuración|Usuarios|Miembros|Contraseña|Idioma|Ayuda|Aceptar|Rechazar|Pendiente|Semana|Día|Contactos|Calendario|electrónico|aplicaciones|Insertar|Tabla|Columna|Fila|Celda|Texto|Fuente|Tamaño|Color|Estilo|Formato|Filtro|Datos|Vista|Página|Diapositiva|Hoja|Gráfico|Imagen|Forma|Consulta|Valor|Rango|Tipo|Campo|Condición|Función|Notas|Índice|Leyendas|Referencias|Comentario|Ventana|Proteger|Herramientas|Celdas|Elementos|Enlace|Formulario|Multimedia|Conectores)\b/g;

test.describe('sin castellano en duro', () => {
  test.use({
    locale: 'en-GB',
    extraHTTPHeaders: { 'accept-language': 'en-GB,en;q=0.9' },
  });

  for (const [nombre, url] of Object.entries(APPS)) {
    test(`${nombre} se ve entera en inglés`, async ({ page }) => {
      const res = await page.goto(url, { waitUntil: 'domcontentloaded' });
      // Un 400 aquí suele ser el IdP rechazando el `client_id`: en un entorno de
      // desarrollo recién sembrado falta el registro OAuth de esa app, y no hay
      // forma de mirar su interfaz hasta arreglarlo.
      expect(res?.status(), `${nombre} no carga (¿cliente OAuth sin registrar?)`).toBeLessThan(400);
      await page.waitForTimeout(3_000); // que hidraten las islas

      // El idioma negociado tiene que llegar al <html>: el portal lo tenía fijo
      // a «es» y era el único de los diez, así que declaraba castellano a los
      // lectores de pantalla hiciera lo que hiciera la sesión.
      expect(await page.evaluate(() => document.documentElement.lang)).toBe('en');

      const texto = await page.evaluate(() => document.body.innerText);
      const encontrado = [...new Set(texto.match(CASTELLANO) ?? [])];
      expect(encontrado, `${nombre} muestra castellano pidiendo inglés`).toEqual([]);
    });
  }

  // La portada de Docs es la lista de documentos: el editor, que es donde estaba
  // el grueso del problema, no se abre ahí. Cada tipo tiene su propia cinta.
  for (const kind of ['document', 'spreadsheet', 'presentation'] as const) {
    test(`el editor de ${kind} se ve entero en inglés`, async ({ page }) => {
      test.slow(); // tres editores, y cada uno recorre todas sus pestañas
      const res = await page.request.post(`${APPS.docs}api/create`, {
        data: { kind, title: `e2e-i18n-${kind}` },
      });
      expect(res.status(), await res.text()).toBe(201);
      const { id } = (await res.json()) as { id: string };

      await page.goto(`${APPS.docs}edit?id=${id}`);
      await expect(page.getByRole('tablist', { name: /cinta|ribbon|band/i })).toBeVisible({
        timeout: 60_000,
      });

      const visto = new Set<string>();
      for (const pestana of await page.getByRole('tab').all()) {
        await pestana.click().catch(() => {});
        await page.waitForTimeout(400);
        // Se miran también aria-label y title: media cinta son botones de icono,
        // y ahí es donde el castellano sobrevive a cualquier revisión visual.
        const texto = await page.evaluate(() => {
          const rotulos = [...document.querySelectorAll('[aria-label],[title]')].map(
            (e) => e.getAttribute('aria-label') ?? e.getAttribute('title') ?? '',
          );
          return `${document.body.innerText}\n${rotulos.join('\n')}`;
        });
        for (const hallazgo of texto.match(CASTELLANO) ?? []) visto.add(hallazgo);
      }
      expect([...visto], `la cinta de ${kind} muestra castellano`).toEqual([]);
    });
  }
});
