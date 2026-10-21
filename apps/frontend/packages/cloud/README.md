# @sytue/cloud

El SDK de las apps que corren en **Sytue DevX**. Guardar y leer datos sin SQL,
sin cadena de conexión y **sin configurar nada**.

```js
import { sytue } from '@sytue/cloud';

const posts = await sytue.table('posts').newest().limit(10).all();
await sytue.table('posts').add({ titulo: 'Hola mundo' });
```

Eso es todo el arranque. No hay URL que copiar del panel, ni clave que pegar en
un `.env`, ni fichero de configuración.

## Por qué no hay que configurar nada

En cualquier otro servicio parecido, el primer paso es «copia la URL y la clave
del panel y pégalas en tu proyecto». Ese paso es donde se atasca quien empieza —y
donde se filtra la primera credencial, porque acaba en un commit—.

En Sytue DevX no hace falta: cuando despliegas, el control-plane **escribe las
credenciales de tus servicios en el entorno de tu app**. El SDK las lee. Por eso
`sytue` ya viene listo nada más importarlo.

> Si acabas de crear la base de datos y tu app ya estaba corriendo, **vuelve a
> desplegar**: las credenciales se inyectan al desplegar, no antes. El SDK te lo
> dice con esas palabras si te pasa.

## La base de datos

Una tabla se usa con los verbos de siempre:

```js
const t = sytue.table('posts');

await t.all(); // todas las filas
await t.find(3); // una por su id (null si no está)
await t.add({ titulo: 'Hola' }); // la crea y te devuelve la fila CON su id
await t.update(3, { titulo: 'Otro' });
await t.remove(3); // true si existía
await t.count(); // cuántas hay
```

Y se filtra encadenando:

```js
await sytue
  .table('posts')
  .where({ publicado: true }) // igualdad; `null` pregunta por nulo
  .search('titulo', 'verano') // busca la palabra, sin distinguir mayúsculas
  .newest() // de más nuevo a más viejo (mira `created_at`)
  .limit(10)
  .all();

await sytue.table('posts').where({ autor: 'ana' }).first(); // uno o null
```

Si tu columna identificadora no se llama `id`:

```js
await sytue.table('usuarios', { key: 'email' }).find('ana@ejemplo.com');
```

### Lo que el SDK no te deja hacer

`update` y `remove` **exigen el identificador de la fila**. No existe la forma de
actualizar o vaciar una tabla entera de un descuido, que es el accidente más caro
—y más frecuente— de quien está aprendiendo. Si de verdad necesitas una operación
masiva, es SQL desde el panel, donde se ve lo que va a pasar.

### Encadenar no arrastra nada

Cada método devuelve una consulta **nueva**. Puedes guardar una base y reutilizarla
sin que se le peguen los filtros de la vez anterior:

```js
const mios = sytue.table('posts').where({ autor: 'ana' });
await mios.where({ publicado: true }).all(); // ana + publicados
await mios.all(); // solo ana
```

## Errores que dicen qué hacer

Es la razón de ser del paquete. En vez de `error: relation "postss" does not
exist`:

> No existe la tabla «postss». Revisa el nombre o créala desde el panel
> (Base de datos → SQL).

En vez de un `403` sin más:

> Sin permiso para «posts». Suele ser una política de seguridad de fila (RLS) que
> no deja ver ni tocar estas filas; revísala en el panel (Base de datos → Tablas).

Los mensajes vienen en **inglés, español, francés, alemán e italiano**. El idioma
sale de `SYTUE_LANG` o `LANG` (que tu contenedor ya trae) y se puede fijar a mano:

```js
import { createClient } from '@sytue/cloud';
const sytue = createClient({ language: 'fr' });
```

La respuesta cruda del servidor sigue estando ahí para quien la quiera:

```js
try {
  await sytue.table('posts').add({});
} catch (e) {
  console.log(e.message); // el mensaje útil
  console.log(e.detail); // { code: '23502', message: '...' }
}
```

## Saber dónde estás

```js
sytue.hasDatabase; // ¿hay base de datos conectada?
sytue.project; // slug del proyecto, como en el panel
sytue.isPreview; // true en un despliegue de vista previa (una rama, un PR)
```

`isPreview` es útil para no mandar correos de verdad ni cobrar de verdad desde la
vista previa de un pull request.

## Desarrollar en tu ordenador

Fuera de Sytue DevX no hay entorno que leer, así que hay que decírselo. Es el
único caso en el que se pasan argumentos:

```js
import { createClient } from '@sytue/cloud';

const sytue = createClient({
  restUrl: process.env.POSTGREST_URL,
  restToken: process.env.PGRST_JWT,
});
```

Esos dos valores los ves en el panel del proyecto. Apunta a la base de una **vista
previa**, no a la de producción.

## Requisitos

Node 18+, Bun o Deno. **Cero dependencias**: solo usa `fetch`.

## Lo que aún no cubre

Ficheros (Storage) y sesión de usuario (Auth) todavía no tienen API en este
paquete; se usan por sus variables de entorno (`S3_*`, `OIDC_*`). Están en la
lista de lo siguiente.

---

Para **crear proyectos y desplegar** desde una herramienta —no desde la app— el
paquete es `@sytue/api-clients`, que habla con el control-plane.
