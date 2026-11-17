# @sytue/cloud

The SDK for apps that run on **Sytue DevX**. Store and read data with no SQL,
no connection string and **nothing to configure**.

```js
import { sytue } from '@sytue/cloud';

const posts = await sytue.table('posts').newest().limit(10).all();
await sytue.table('posts').add({ titulo: 'Hola mundo' });
```

That is the whole setup. There is no URL to copy from the dashboard, no key to paste
into a `.env`, and no configuration file.

<a id="por-qué-no-hay-que-configurar-nada"></a>

## Why there is nothing to configure

In any other similar service, the first step is “copy the URL and the key from
the dashboard and paste them into your project”. That step is where beginners get
stuck —and where the first credential leaks, because it ends up in a commit—.

On Sytue DevX it is not needed: when you deploy, the control plane **writes your
services' credentials into your app's environment**. The SDK reads them. That is why
`sytue` is ready as soon as you import it.

> If you have just created the database and your app was already running, **deploy
> again**: credentials are injected on deploy, not before. The SDK tells you so in
> those very words if it happens to you.

<a id="la-base-de-datos"></a>

## The database

A table is used with the usual verbs:

```js
const t = sytue.table('posts');

await t.all(); // todas las filas
await t.find(3); // una por su id (null si no está)
await t.add({ titulo: 'Hola' }); // la crea y te devuelve la fila CON su id
await t.update(3, { titulo: 'Otro' });
await t.remove(3); // true si existía
await t.count(); // cuántas hay
```

And you filter by chaining:

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

If your identifier column is not called `id`:

```js
await sytue.table('usuarios', { key: 'email' }).find('ana@ejemplo.com');
```

<a id="lo-que-el-sdk-no-te-deja-hacer"></a>

### What the SDK does not let you do

`update` and `remove` **require the row identifier**. There is no way to update
or empty an entire table by accident, which is the most expensive —and most
frequent— accident for someone who is learning. If you really need a bulk
operation, it is SQL from the dashboard, where you can see what is going to happen.

<a id="encadenar-no-arrastra-nada"></a>

### Chaining carries nothing over

Each method returns a **new** query. You can store a base query and reuse it
without the filters from the previous time sticking to it:

```js
const mios = sytue.table('posts').where({ autor: 'ana' });
await mios.where({ publicado: true }).all(); // ana + publicados
await mios.all(); // solo ana
```

<a id="errores-que-dicen-qué-hacer"></a>

## Errors that say what to do

This is the package's reason for being. Instead of `error: relation "postss" does not
exist`:

> There is no table called “postss”. Check the name or create it from the dashboard
> (Database → SQL).

Instead of a bare `403`:

> No permission on “posts”. This is usually a row-level security (RLS) policy hiding
> or blocking these rows; review it in the dashboard (Database → Tables).

The messages come in **English, Spanish, French, German and Italian**. The language
comes from `SYTUE_LANG` or `LANG` (which your container already has) and can be set
by hand:

```js
import { createClient } from '@sytue/cloud';
const sytue = createClient({ language: 'fr' });
```

The raw server response is still there for anyone who wants it:

```js
try {
  await sytue.table('posts').add({});
} catch (e) {
  console.log(e.message); // el mensaje útil
  console.log(e.detail); // { code: '23502', message: '...' }
}
```

<a id="saber-dónde-estás"></a>

## Knowing where you are

```js
sytue.hasDatabase; // ¿hay base de datos conectada?
sytue.project; // slug del proyecto, como en el panel
sytue.isPreview; // true en un despliegue de vista previa (una rama, un PR)
```

`isPreview` is useful to avoid sending real emails or charging for real from the
preview of a pull request.

<a id="desarrollar-en-tu-ordenador"></a>

## Developing on your computer

Outside Sytue DevX there is no environment to read, so you have to tell it. It is the
only case in which arguments are passed:

```js
import { createClient } from '@sytue/cloud';

const sytue = createClient({
  restUrl: process.env.POSTGREST_URL,
  restToken: process.env.PGRST_JWT,
});
```

You can see those two values in the project dashboard. Point it at the database of a
**preview**, not the production one.

<a id="requisitos"></a>

## Requirements

Node 18+, Bun or Deno. **Zero dependencies**: it only uses `fetch`.

<a id="lo-que-aún-no-cubre"></a>

## What it does not cover yet

Files (Storage) and user sessions (Auth) do not have an API in this package yet;
they are used through their environment variables (`S3_*`, `OIDC_*`). They are on
the list of what comes next.

---

To **create projects and deploy** from a tool —not from the app— the
package is `@sytue/api-clients`, which talks to the control plane.
