// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// LO QUE iOS HACE ZOOM AL ENFOCARLO.
//
// Safari amplía la página al enfocar un campo cuya letra mide MENOS DE 16 PÍXELES,
// y no la devuelve al salir: se queda ampliada, la maqueta se sale de la pantalla
// y a partir de ahí escribir un correo es una pelea. No hay forma de desactivarlo
// que no sea `maximum-scale=1`, que además de no funcionar en iOS moderno rompe
// el zoom a quien lo necesita para leer.
//
// LA TRAMPA DE ESTA SUITE: la raíz está en 14px, así que `text-base` —1rem— son
// 14 y NO 16. Una clase que suena a «tamaño normal» dispara el zoom igual. Por
// eso esto mide PÍXELES CALCULADOS y no clases.
//
// Uso:  bun run zoom-tactil.mjs           (necesita .auth/prod-audit.json)
//
// ponytail: guion de verificación; no forma parte de la suite de specs.
const RUTAS = {
  mail: "https://mail.sytue.eu/mail",
  notes: "https://notes.sytue.eu/notes",
  rooms: "https://rooms.sytue.eu/rooms",
  docus: "https://docus.sytue.eu/docus",
  contacts: "https://contacts.sytue.eu/contacts",
  calendar: "https://calendar.sytue.eu/calendar",
  drive: "https://drive.sytue.eu/drive",
};
const SEL =
  "input:not([type=checkbox]):not([type=radio]):not([type=range]),textarea,select,[contenteditable=true],[contenteditable='']";
const b = await chromium.launch();
for (const [app, url] of Object.entries(RUTAS)) {
  const c = await b.newContext({
    ...devices["iPhone 14 Pro"],
    storageState: ".auth/prod-audit.json",
    ignoreHTTPSErrors: true,
  });
  const p = await c.newPage();
  try {
    await p.goto(url, { waitUntil: "domcontentloaded" });
    await p.waitForTimeout(5000);
    // Abrir el redactor/creador si la app lo tiene: es donde vive el editor.
    await p.evaluate(() => {
      const b = [...document.querySelectorAll("button,a")].find((e) =>
        /redactar|compose|nueva nota|new note|nuevo|new |crear|create/i.test(
          e.textContent || e.getAttribute("aria-label") || "",
        ),
      );
      b?.click();
    });
    await p.waitForTimeout(2500);
    const raiz = await p.evaluate(
      () => getComputedStyle(document.documentElement).fontSize,
    );
    const r = await p.evaluate(
      (sel) =>
        [...document.querySelectorAll(sel)]
          .filter((e) => {
            const q = e.getBoundingClientRect();
            return q.width > 4 && q.height > 4;
          })
          .map((e) => ({
            que:
              e.tagName.toLowerCase() +
              (e.getAttribute("contenteditable") !== null
                ? "[contenteditable]"
                : "") +
              (e.type ? `[${e.type}]` : ""),
            px: parseFloat(getComputedStyle(e).fontSize),
            etiqueta: (
              e.getAttribute("aria-label") ||
              e.getAttribute("placeholder") ||
              e.getAttribute("name") ||
              ""
            ).slice(0, 30),
          })),
      SEL,
    );
    const malos = r.filter((e) => e.px < 16);
    console.log(
      `${app.padEnd(9)} raíz ${raiz}  ${r.length} enfocables, ${malos.length} con ZOOM`,
    );
    for (const e of malos)
      console.log(
        `    ZOOM ${String(e.px).padStart(6)}px  ${e.que.padEnd(24)} ${e.etiqueta}`,
      );
  } catch (e) {
    console.log(
      `${app.padEnd(9)} FALLO ${String(e).split("\n")[0].slice(0, 60)}`,
    );
  }
  await c.close();
}
await b.close();
