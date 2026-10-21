/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * Qué productos levanta ESTA instalación, preguntado al Hub —la misma fuente
 * que usa el conmutador de aplicaciones— y no adivinado por un 404.
 *
 * El perfil piloto no despliega Sytue DevX (CLOUD_ENABLED=false), así que sus
 * veintiuna rutas respondían 404 en cada barrido nocturno y los dos barridos
 * salían en rojo por un producto que no existe aquí a propósito. Un 404 en la
 * raíz de una app tampoco valía como señal: el ingress comodín manda los hosts
 * sin enrutar a Calendar y contesta 200.
 *
 * El token sale del BFF del portal con la cookie de la sesión de auditoría
 * (`/one/auth/token`, igual que hace la shell). Si no hay sesión aún, si el Hub
 * no contesta o si la lista viene vacía, se devuelve `null` y el que llama mide
 * TODO, como antes: no medir nunca puede ser la salida cómoda.
 *
 * Y SE DICE EN VOZ ALTA POR QUÉ. Caer a «medir todo» es lo correcto, pero
 * hacerlo en silencio convierte un fallo de esta consulta en lo que parece un
 * fallo de producto: el 2026-09-11 el barrido de escritorio salió en rojo con
 * «31 rutas no se pudieron medir», veinticinco de ellas de Sytue DevX —que el
 * piloto no despliega a propósito—, y en el log no había ni una línea que
 * dijera que la detección había fallado. Un respaldo que no se anuncia parece
 * el camino normal.
 */

let avisado = false;

/**
 * Deja constancia de por qué no se pudo saber, y cae a «medir todo».
 *
 * UNA SOLA VEZ: quien llama reintenta por cada aplicación —la sesión aparece a
 * mitad del barrido—, y veinte líneas iguales taparían el resto del informe.
 */
function sinSaber(motivo) {
  if (avisado) return null;
  avisado = true;
  console.log(`  ⚠ no se sabe qué productos levanta esta instalación (${motivo}).`);
  console.log(`     Se mide TODO, así que las rutas de un producto ausente saldrán como no medidas.`);
  return null;
}

/** Productos que una instalación puede no levantar, y con qué clave los nombra el Hub. */
const OPCIONALES = { cloud: "cloud", pages: "pages" };

export async function appsDesplegadas(page, dominio) {
  try {
    // EL HOST DE LA SESIÓN, NO `one.<dominio>`. La sesión vive en el host de la
    // ORGANIZACIÓN (`corporate.sytue.eu`), y las cookies no cruzan de host: pedir
    // el token a `one.<dominio>` devolvía 401 SIEMPRE. Y como el 401 cae a «no se
    // sabe», el barrido medía TODO —incluida Sytue DevX, que este perfil no
    // despliega—, y sus 21 pantallas salían como hallazgos en cada ejecución.
    //
    // Medido el 2026-09-12 con la misma sesión:
    //   corporate.sytue.eu/one/auth/token → 200
    //   one.sytue.eu/one/auth/token       → 401
    //
    // Se navega primero y se toma el origen de donde se acabe: el portal redirige
    // al host de la organización, así que eso es lo que hay que preguntar. Es el
    // mismo criterio que ya usan los guiones de login de test/e2e.
    await page.goto(`https://one.${dominio}/one/`, { waitUntil: "domcontentloaded" });
    const base = new URL(page.url()).origin;
    const tok = await page.request.get(`${base}/one/auth/token`);
    if (!tok.ok()) return sinSaber(`el BFF de ${base} devolvió ${tok.status()} al pedir el token`);
    const { accessToken } = await tok.json();
    if (!accessToken) return sinSaber("el BFF del portal no devolvió token");
    const res = await page.request.get(`https://hub.${dominio}/api/v1/apps`, {
      headers: { authorization: `Bearer ${accessToken}` },
    });
    if (!res.ok()) return sinSaber(`el Hub devolvió ${res.status()} en /api/v1/apps`);
    const { apps } = await res.json();
    if (!Array.isArray(apps) || !apps.length) return sinSaber("el Hub devolvió la lista vacía");
    return new Set(apps);
  } catch (e) {
    return sinSaber(`no se pudo preguntar: ${e}`);
  }
}

/** `false` solo para un producto OPCIONAL que el Hub dice que no está; sin lista, todo se mide. */
export function desplegada(app, conjunto) {
  if (!conjunto) return true;
  const producto = OPCIONALES[app];
  return !producto || conjunto.has(producto);
}
