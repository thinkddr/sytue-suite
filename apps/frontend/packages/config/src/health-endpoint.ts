import type { APIRoute } from 'astro';

/*
 * Salud + VERSIÓN de esta superficie.
 *
 * Devolvía `ok` en texto plano, que basta para la sonda de Kubernetes pero deja
 * al panel de plataforma sin saber qué imagen corre en cada una de las once
 * superficies web. Ahora contesta la misma forma que los servicios Go
 * (`{status, version, commit}`), así que quien pregunta no necesita saber si al
 * otro lado hay Go o Astro.
 *
 * DE DÓNDE SALE LA VERSIÓN, y por qué en ese orden:
 *
 *  1. `process.env` — lo que el Dockerfile horneó en la imagen que ESTÁ
 *     corriendo. Es la respuesta verdadera a «qué hay desplegado», y la única
 *     que distingue un despliegue a medias de uno completo.
 *  2. `import.meta.env.PUBLIC_*` — el valor congelado en el build. Es el único
 *     que le queda a `www`, que es estático (`output: 'static'`): su HTML se
 *     genera en el build y después no hay servidor que lea ninguna variable.
 *     Sin este segundo escalón, la única superficie pública de la suite sería
 *     también la única que no sabe decir su versión.
 *
 * Vacío y no «desconocido» si no hay ninguno: quien lo pinta ya distingue «no
 * contestó» de «contestó sin versión», y rellenarlo aquí con un texto le quitaría
 * esa diferencia.
 */
// `import.meta.env` con cast: este paquete es la configuración compartida y no
// arrastra los tipos de Astro, que son los que lo declaran. El acceso es válido
// —Vite lo sustituye literalmente en el build—, solo que TypeScript no lo sabe
// aquí; arrastrar `astro/client` a este paquete para una línea sería peor.
const horneado = (import.meta as unknown as { env?: Record<string, string | undefined> }).env ?? {};

const release = process.env.SYTUE_RELEASE || horneado.PUBLIC_SYTUE_RELEASE || '';
const commit = process.env.SYTUE_COMMIT || horneado.PUBLIC_SYTUE_COMMIT || '';

/*
 * La versión y el commit SOLO hacia dentro. Medido contra producción el
 * 2026-08-13: `curl https://drive.sytue.eu/healthz` devolvía, sin autenticación
 * de ningún tipo, `{"status":"ok","version":"v2.2.1","commit":"ee717df9…"}`.
 * Decirle a cualquiera qué versión exacta corre es regalarle la lista de fallos
 * conocidos que puede probar, y el commit además señala el código concreto.
 *
 * Quien pregunta de dentro sigue viéndolo, que es quien lo necesita: la sonda
 * del kubelet llama a la IP del Pod y Sytue ID pregunta por la URL interna del
 * componente, ninguno de los dos a través de Traefik — y es Traefik quien añade
 * X-Forwarded-For. Así que su presencia distingue «viene de internet» sin
 * mantener ninguna lista de redes.
 */
export const GET: APIRoute = ({ request }) => {
  const deFuera = request.headers.get('x-forwarded-for') !== null;
  const cuerpo = deFuera ? { status: 'ok' } : { status: 'ok', version: release, commit };
  return new Response(JSON.stringify(cuerpo) + '\n', {
    status: 200,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
};
