/**
 * Tests de DriveClient sobre los caminos que MUEVEN BYTES.
 *
 * `copy()`: lo que se guarda es que la copia ocurra EN EL SERVIDOR. Antes descargaba el
 * fichero y lo volvía a subir —con un comentario que decía que el servicio no exponía
 * copia nativa, cuando sí—, así que los bytes cruzaban la red dos veces y la resubida
 * contaba cuota que no se estaba ocupando.
 *
 * `openDownload()`: lo que se guarda es que el cuerpo llegue SIN LEER, para que quien solo
 * lo reenvía (el BFF de Drive) encadene streams en vez de materializar el fichero entero
 * en memoria.
 *
 * En ambos se comprueban las PETICIONES y la forma de la respuesta, no el contenido: el
 * defecto no era un valor mal devuelto, era el camino que tomaba para llegar al mismo sitio.
 *
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { DriveClient } from './index.js';

interface Llamada {
  url: string;
  method: string;
  body?: string | undefined;
}

/** Bytes que devuelve el doble en /content (el valor da igual; el tamaño se comprueba). */
const CONTENIDO = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);

/**
 * Cuerpo de /content como un stream que DELATA a quien lo lee: `pull` solo se invoca
 * cuando alguien tira de los bytes.
 *
 * Es la única señal fiable de que el cliente no materializó el fichero: comprobar
 * `bodyUsed`/`body` en la respuesta devuelta NO sirve, porque envolver el resultado en
 * `new Response(await res.blob())` vuelve a dar `bodyUsed === false` y un stream nuevo
 * —justo el defecto que se quiere impedir—.
 */
function cuerpoDelator(): { stream: ReadableStream<Uint8Array>; leido: () => boolean } {
  let leido = false;
  const stream = new ReadableStream<Uint8Array>(
    {
      pull(c) {
        leido = true;
        c.enqueue(CONTENIDO);
        c.close();
      },
    },
    // highWaterMark 0 es imprescindible: con el valor por defecto (1) el stream se
    // rellena solo al construirlo y `pull` saltaría sin que nadie hubiera leído —el
    // delator delataría siempre y el test no distinguiría nada—.
    { highWaterMark: 0 },
  );
  return { stream, leido: () => leido };
}

/**
 * DriveClient con un fetch de mentira que responde lo justo para resolver la ruta
 * `/origen.bin` y registra cada petición.
 */
function clienteEspia(): {
  client: DriveClient;
  llamadas: Llamada[];
  /** true en cuanto ALGUIEN lee el cuerpo de /content. */
  cuerpoLeido: () => boolean;
} {
  const llamadas: Llamada[] = [];
  const cuerpo = cuerpoDelator();
  const json = (body: unknown): Response =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });

  const raiz = { id: 'carpeta-raiz', name: '', is_root: true };
  const fichero = { id: 'file-1', name: 'origen.bin', content_type: 'application/octet-stream' };

  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString();
    llamadas.push({
      url,
      method: init?.method ?? 'GET',
      body: typeof init?.body === 'string' ? init.body : undefined,
    });
    // El orden importa: /folders/{id}/children también contiene '/folders/'.
    if (url.includes('/children')) return json({ folders: [], files: [fichero] });
    if (url.includes('/folders/root')) return json(raiz);
    // Como el servicio Go: la URL pública la compone él, con SU host público.
    if (url.endsWith('/shares'))
      return json({
        share: { id: 's1', kind: 'link', token: 'tk', permission: 'read' },
        url: 'https://drive.publico/public/tk',
      });
    if (url.includes('/copy'))
      return json({ ...fichero, id: 'file-2', name: 'Copia de origen.bin' });
    // Como el servicio Go: pedir un fichero por id trae su RUTA resuelta, que él
    // añade aditivamente (los listados de carpeta no la llevan). Por el final
    // para no cazar /files/file-1/content ni /files/file-1/copy.
    if (url.endsWith('/files/file-1')) return json({ ...fichero, path: '/Documentos/origen.bin' });
    // El contenido, como lo emite el servicio Go: cuerpo en streaming y el tamaño
    // exacto en Content-Length (sacado del metadato, no de bufferizar).
    if (url.includes('/content'))
      return new Response(cuerpo.stream, {
        status: 200,
        headers: {
          'content-type': 'application/octet-stream',
          'content-length': String(CONTENIDO.byteLength),
        },
      });
    return json({});
  }) as unknown as typeof fetch;

  return {
    client: new DriveClient({
      baseUrl: 'https://drive.test',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
    llamadas,
    cuerpoLeido: cuerpo.leido,
  };
}

test('copy usa la copia del SERVIDOR y no descarga ni resube los bytes', async () => {
  const { client, llamadas } = clienteEspia();
  await client.copy('/origen.bin', '/destino.bin');

  // La copia nativa: un POST a /files/{id}/copy con la carpeta destino.
  const copia = llamadas.find((l) => l.url.includes('/copy'));
  expect(copia?.method).toBe('POST');
  expect(copia?.body).toContain('dest_folder_id');

  // Y NADA de mover bytes: ni descarga del contenido ni subida.
  expect(llamadas.some((l) => l.url.includes('/content'))).toBe(false);
  expect(llamadas.some((l) => l.method === 'PUT')).toBe(false);
});

test('el nombre pedido se respeta: el servicio nombra la copia a su manera', async () => {
  const { client, llamadas } = clienteEspia();
  await client.copy('/origen.bin', '/destino.bin');

  // El doble responde «Copia de origen.bin», así que hace falta un PATCH que lo deje en
  // el nombre que pidió quien llamó. Sin él, `copy(a, b)` no cumpliría lo que promete.
  const renombrado = llamadas.find((l) => l.method === 'PATCH');
  expect(renombrado?.body).toContain('destino.bin');
});

test('copiar una carpeta se rechaza en el cliente, sin ir al servidor', async () => {
  const { client } = clienteEspia();
  // El doble solo conoce ficheros: una ruta que no resuelve a fichero debe fallar.
  await expect(client.copy('/no-existe/', '/x.bin')).rejects.toThrow();
});

test('openDownload entrega el cuerpo SIN LEER, para poder transmitirlo', async () => {
  const { client, cuerpoLeido } = clienteEspia();
  const res = await client.openDownload('/origen.bin');

  // Lo que muerde: si el cliente materializa el fichero (un .blob()/.arrayBuffer() por
  // dentro), el stream se habrá pulsado y el pico de memoria del BFF vuelve a ser O(tamaño).
  expect(cuerpoLeido()).toBe(false);
  expect(res.body).toBeInstanceOf(ReadableStream);

  // Y el tamaño viaja en la cabecera: el BFF lo propaga sin tener que contar bytes
  // (el cliente nativo lo exige para aplicar su límite offline de 50 MiB).
  expect(res.headers.get('content-length')).toBe(String(CONTENIDO.byteLength));

  // El cuerpo sigue entero para quien SÍ lo quiera leer.
  expect((await res.blob()).size).toBe(CONTENIDO.byteLength);
  expect(cuerpoLeido()).toBe(true);
});

test('download() sigue dando los bytes enteros (manipular un PDF los necesita)', async () => {
  const { client } = clienteEspia();
  const blob = await client.download('/origen.bin');
  expect(blob.size).toBe(CONTENIDO.byteLength);
});

test('openDownload rechaza una carpeta sin pedir contenido al servidor', async () => {
  const { client, llamadas } = clienteEspia();
  await expect(client.openDownload('/no-existe/')).rejects.toThrow();
  expect(llamadas.some((l) => l.url.includes('/content'))).toBe(false);
});

test('statById conserva la RUTA que resuelve el servicio, no la raíz', async () => {
  const { client } = clienteEspia();
  const meta = await client.statById('file-1');

  // «Abrir en Docus/Calcus/Diapos» descarga por esta ruta (open-from-drive →
  // openDownload) y «guardar en Drive» sube por ella: colgada de la raíz, un
  // fichero en subcarpeta no se encontraba y la apertura volvía a la lista de
  // documentos con un error. Solo abría el que vivía en la raíz.
  expect(meta.path).toBe('/Documentos/origen.bin');
  expect(meta.name).toBe('origen.bin');
});

test('el enlace público es el que devuelve el servicio, no el host por el que se le llama', async () => {
  // El BFF llama por la URL INTERNA del clúster: recomponer el enlace con ella lo
  // dejaba apuntando a un host que desde fuera no existe.
  const { client } = clienteEspia();
  const share = await client.createShare({ path: '/origen.bin' });
  expect(share.url).toBe('https://drive.publico/public/tk');
});
