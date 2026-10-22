// Regresión de los inicios reales, integrada en accesibilidad-office.mjs y su servidor DEV.
// No basta con contar scrollbars: un gesto sobre Recientes debe mover también Crear y h1.
const PRODUCTOS = [
  { id: "docus", kind: "document" },
  { id: "calcus", kind: "spreadsheet" },
  { id: "diapos", kind: "presentation" },
  { id: "certipdf", kind: "pdf" },
];
const TOTAL_DOCS = 240;
const TOTAL_PDFS = 80;
const json = (route, data) =>
  route.fulfill({
    contentType: "application/json",
    body: JSON.stringify(data),
  });
const nombre = (n, pdf) =>
  `Reciente de prueba ${String(n).padStart(4, "0")}${pdf ? ".pdf" : ""}`;

async function simularInicio(page, producto) {
  const cursores = new Set();
  await page.route(/\/auth\/(session|token)(\?|$)/, (route) =>
    json(route, {
      authenticated: true,
      accessToken: "demo",
      expiresAt: Date.now() + 3600000,
      user: { id: "demo-user", name: "Demo", tenant: "prueba" },
    }),
  );
  // La Shell real consulta el Hub del mismo origen del banco, nunca un servicio externo.
  await page.route(/\/api\/v1\//, (route) =>
    route.request().url().includes("/stream")
      ? route.fulfill({ status: 204, body: "" })
      : json(route, { notifications: [], unread: 0, apps: [], items: [] }),
  );
  await page.route(
    /\/api\/(documents|templates|themes|pdf-recientes|pdf-firmas)(\?|$)/,
    (route) => {
      const url = new URL(route.request().url());
      const endpoint = url.pathname.split("/").pop();
      if (endpoint === "documents") {
        const cursor = url.searchParams.get("cursor") || "0";
        const inicio = Number(cursor);
        const limite = Number(url.searchParams.get("limit"));
        if (
          url.searchParams.get("kind") !== producto.kind ||
          limite !== 100 ||
          !Number.isInteger(inicio)
        ) {
          throw new Error(`paginación inesperada: ${url.search}`);
        }
        cursores.add(cursor);
        const fin = Math.min(inicio + limite, TOTAL_DOCS);
        return json(route, {
          documents: Array.from({ length: fin - inicio }, (_, i) => ({
            id: `reciente-${inicio + i + 1}`,
            title: nombre(inicio + i + 1, false),
            kind: producto.kind,
            ownerId: "demo-user",
            createdAt: "2026-10-16T10:00:00Z",
            updatedAt: "2026-10-16T10:00:00Z",
            version: 1,
            starred: false,
          })),
          ...(fin < TOTAL_DOCS ? { nextCursor: String(fin) } : {}),
        });
      }
      if (endpoint === "pdf-recientes")
        return json(route, {
          items: Array.from({ length: TOTAL_PDFS }, (_, i) => ({
            name: nombre(i + 1, true),
            path: `/PDF/${nombre(i + 1, true)}`,
            modifiedAt: "2026-10-16T10:00:00Z",
            size: 120000,
          })),
        });
      if (endpoint === "pdf-firmas")
        return json(route, { sent: [], received: [] });
      return json(route, { templates: [], org_templates: [], themes: [] });
    },
  );
  return cursores;
}

/** Marca el scroller de la página, encontrado desde el título, no desde la lista. */
async function localizarScroll(page) {
  return page.locator("main h1").evaluate((titulo) => {
    for (let el = titulo.parentElement; el; el = el.parentElement) {
      if (
        /auto|scroll/.test(getComputedStyle(el).overflowY) &&
        el.clientHeight > 0
      ) {
        el.dataset.inicioScroll = "";
        return true;
      }
    }
    return false;
  });
}

async function comprobarDocumento(page) {
  const { alto, visible } = await page.evaluate(() => ({
    alto: document.documentElement.scrollHeight,
    visible: innerHeight,
  }));
  if (alto > visible + 1) {
    throw new Error(
      `el documento añade otro scroll vertical: ${alto} > ${visible}`,
    );
  }
}

async function puntoSobreRecientes(page, enlace) {
  await enlace.evaluate((el) => {
    const scroll = document.querySelector("[data-inicio-scroll]");
    scroll.scrollTop +=
      el.getBoundingClientRect().top -
      scroll.getBoundingClientRect().top -
      scroll.clientHeight * 0.55;
  });
  // TanStack actualiza la ventana al recibir scroll: esperar dos frames, no un retardo fijo.
  await page.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  );
  return enlace.evaluate((el) => {
    const r = el.getBoundingClientRect();
    const scroll = document
      .querySelector("[data-inicio-scroll]")
      .getBoundingClientRect();
    const x = r.left + r.width / 2;
    const y = Math.min(r.bottom - 16, scroll.bottom - 24, innerHeight - 24);
    if (
      y < Math.max(r.top, scroll.top) ||
      !el.contains(document.elementFromPoint(x, y))
    ) {
      throw new Error("el reciente no se puede alcanzar dentro de la hoja");
    }
    return { x, y };
  });
}

async function gesto(page, cdp, punto, distancia = 180, preventFling = true) {
  if (cdp) {
    try {
      await cdp.send("Input.synthesizeScrollGesture", {
        x: Math.round(punto.x),
        y: Math.round(punto.y),
        yDistance: -distancia,
        speed: preventFling
          ? Math.max(800, Math.min(Math.round(distancia * 4), 6000))
          : 20000,
        gestureSourceType: "touch",
        preventFling,
      });
    } catch {
      await page.mouse.move(punto.x, punto.y);
      await page.mouse.wheel(0, distancia);
    }
  } else {
    await page.mouse.move(punto.x, punto.y);
    await page.mouse.wheel(0, distancia);
  }
}

async function gestoUnificado(page, cdp, enlace) {
  const punto = await puntoSobreRecientes(page, enlace);
  const antes = await page.evaluate(() => ({
    titulo: document.querySelector("main h1").getBoundingClientRect().top,
    crear: document
      .querySelector(
        'section[aria-labelledby="new-heading"],section[aria-labelledby="pdf-new-heading"]',
      )
      .getBoundingClientRect().top,
  }));
  await gesto(page, cdp, punto);
  const movido = await page
    .waitForFunction(
      (antes) =>
        document.querySelector("main h1").getBoundingClientRect().top <
          antes.titulo - 30 &&
        document
          .querySelector(
            'section[aria-labelledby="new-heading"],section[aria-labelledby="pdf-new-heading"]',
          )
          .getBoundingClientRect().top <
          antes.crear - 30,
      antes,
      { timeout: 3000 },
    )
    .then(() => true)
    .catch(() => false);
  if (movido) return true;
  if (cdp) {
    await page.mouse.move(punto.x, punto.y);
    await page.mouse.wheel(0, 180);
    return page
      .waitForFunction(
        (antes) =>
          document.querySelector("main h1").getBoundingClientRect().top <
            antes.titulo - 30 &&
          document
            .querySelector(
              'section[aria-labelledby="new-heading"],section[aria-labelledby="pdf-new-heading"]',
            )
            .getBoundingClientRect().top <
            antes.crear - 30,
        antes,
        { timeout: 3000 },
      )
      .then(() => true)
      .catch(() => false);
  }
  return false;
}

async function recorrerCatalogo(page, cdp, producto, cursores) {
  const pdf = producto.kind === "pdf";
  const ultimo = `main a:has([title="${nombre(pdf ? TOTAL_PDFS : TOTAL_DOCS, pdf)}"])`;
  // Cada gesto llega al final de la página traída. La carga incremental aumenta el
  // recorrido y obliga a un nuevo gesto: no se manipula scrollTop para paginar.
  for (let i = 0; i < 8; i++) {
    const scroll = await page.locator("[data-inicio-scroll]").evaluate((el) => {
      const r = el.getBoundingClientRect();
      return {
        x: r.left + r.width / 2,
        y: r.top + Math.max(el.clientHeight - 40, 160),
        distance: el.scrollHeight,
      };
    });
    await gesto(
      page,
      cdp,
      scroll,
      cdp ? Math.min(scroll.distance, 400) : scroll.distance,
      false,
    );
    if (cdp) {
      await page.mouse.move(scroll.x, scroll.y);
      await page.mouse.wheel(0, scroll.distance);
    }
    try {
      // `visible` solo exige una caja: las filas de overscan ya la tienen fuera
      // del viewport. Esperar la geometría final deja asentarse también la rueda.
      await page.waitForFunction(
        (selector) => {
          const el = document.querySelector(selector);
          if (!el) return false;
          const r = el.getBoundingClientRect();
          const s = document
            .querySelector("[data-inicio-scroll]")
            .getBoundingClientRect();
          const seguro =
            parseFloat(
              getComputedStyle(document.documentElement).getPropertyValue(
                "--sytue-safe-bottom",
              ),
            ) || 0;
          return (
            r.top >= s.top - 1 &&
            r.bottom <= Math.min(s.bottom, innerHeight - seguro) + 1 &&
            el.contains(
              document.elementFromPoint(r.left + r.width / 2, r.bottom - 8),
            )
          );
        },
        ultimo,
        { timeout: 1200 },
      );
      if (!pdf && !["0", "100", "200"].every((cursor) => cursores.has(cursor)))
        throw new Error("no se recorrieron las tres páginas de documentos");
      const nodos = await page
        .locator('main a[href*="/edit?id=reciente-"]')
        .count();
      if (!pdf && nodos >= TOTAL_DOCS / 2)
        throw new Error(
          `${nodos}/${TOTAL_DOCS} tarjetas montadas: se perdió la virtualización`,
        );
      return;
    } catch (error) {
      if (!String(error).includes("Timeout")) throw error;
    }
  }
  throw new Error(
    `no se alcanza completo el último reciente (${cursores.size} páginas cargadas)`,
  );
}

export async function pruebasDeInicio(
  navegador,
  { BASE, OUT, SOLO, IDIOMA, falla, ok },
) {
  for (const producto of PRODUCTOS.filter(
    (p) =>
      !SOLO.length ||
      SOLO.includes(p.id) ||
      (p.id === "certipdf" && SOLO.includes("portada")),
  )) {
    for (const forma of ["marco", "carril"]) {
      for (const width of [1280, 360]) {
        const movil = width === 360;
        const id = `${producto.id}-inicio-${forma}-${width}`;
        const contexto = await navegador.newContext({
          viewport: { width, height: 800 },
          hasTouch: movil,
          isMobile: movil,
          locale: IDIOMA,
        });
        await contexto.addInitScript((forma) => {
          localStorage.setItem("sytue-forma", forma);
          localStorage.setItem("sytue-theme", "light");
        }, forma);
        const page = await contexto.newPage();
        const cdp = movil ? await contexto.newCDPSession(page) : null;
        if (cdp) {
          await cdp.send("Emulation.setTouchEmulationEnabled", {
            enabled: true,
            maxTouchPoints: 1,
          });
        }
        try {
          const cursores = await simularInicio(page, producto);
          const ruta =
            producto.kind === "pdf"
              ? "/dev/pdf?portada=1&shell=1"
              : `/dev/inicio?kind=${producto.kind}`;
          await page.goto(`${BASE}${ruta}&lang=${IDIOMA}`, { timeout: 120000 });
          await page.locator("main h1").waitFor({ timeout: 60000 });
          if (movil)
            await page.addStyleTag({
              content: ":root { --sytue-safe-bottom: 20px !important; }",
            });
          const primero = page.locator(
            `main a:has([title="${nombre(1, producto.kind === "pdf")}"])`,
          );
          await primero.waitFor({ timeout: 30000 });
          await comprobarDocumento(page);
          if (!(await localizarScroll(page)))
            throw new Error("el título no pertenece a una página desplazable");
          if (!(await gestoUnificado(page, cdp, primero)))
            throw new Error(
              "desplazar Recientes deja inmóviles la cabecera y Crear",
            );
          await recorrerCatalogo(page, cdp, producto, cursores);
          await page.screenshot({ path: `${OUT}/${id}-final.png` });
          if (movil && forma === "marco") {
            await page.setViewportSize({ width: 800, height: 360 });
            await comprobarDocumento(page);
            await recorrerCatalogo(page, cdp, producto, cursores);
            await page.screenshot({ path: `${OUT}/${id}-girado.png` });
          }
          // Control negativo: la MISMA medición debe rechazar Recientes aislado.
          if (producto.id === "docus" && forma === "marco" && !movil) {
            await page.locator("[data-inicio-scroll]").evaluate((el) => {
              el.scrollTop = 0;
            });
            await page
              .locator('section[aria-labelledby="docs-heading"]')
              .evaluate((el) => {
                Object.assign(el.style, {
                  height: "240px",
                  maxHeight: "240px",
                  overflowY: "auto",
                  flex: "none",
                });
              });
            await primero.waitFor();
            if (await gestoUnificado(page, cdp, primero))
              throw new Error(
                "el control negativo no detecta el scroll aislado",
              );
          }
          ok(
            id,
            `${movil ? "gesto táctil CDP" : "rueda"} desplaza toda la página; último reciente accesible${producto.kind === "pdf" ? "" : ", tres páginas y DOM virtualizado"}`,
          );
        } catch (error) {
          falla("scroll", id, String(error.message).slice(0, 220));
          await page
            .screenshot({ path: `${OUT}/${id}-error.png` })
            .catch(() => undefined);
        } finally {
          await contexto.close();
        }
      }
    }
  }
}
