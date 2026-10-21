// SPDX-License-Identifier: Apache-2.0
// Copyright (c) Sytue. Licensed under Apache-2.0.

// Ejecutar en CI o en el efímero de Scaleway: captura el diseño YA renderizado.
import { chromium, type Response } from "playwright";
import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

function argumentsFrom(argv: string[]) {
  const allowed = new Set([
    "url",
    "out",
    "width",
    "height",
    "scale",
    "state",
    "source-hash",
    "selector",
    "ready-selector",
    "analysis",
  ]);
  const values = new Map<string, string>();
  for (let i = 0; i < argv.length; i += 2) {
    const flag = argv[i];
    const value = argv[i + 1];
    if (
      !flag?.startsWith("--") ||
      !allowed.has(flag.slice(2)) ||
      !value ||
      value.startsWith("--")
    ) {
      throw new Error(`Argumento inválido: ${flag ?? "(vacío)"}`);
    }
    const key = flag.slice(2);
    if (values.has(key)) throw new Error(`Argumento duplicado: ${flag}`);
    values.set(key, value);
  }
  function required(key: string) {
    const value = values.get(key);
    if (!value) throw new Error(`Falta --${key}`);
    return value;
  }
  function dimension(key: string, fallback: number) {
    const raw = values.get(key) ?? String(fallback);
    if (!/^\d+$/.test(raw)) throw new Error(`--${key} debe ser un entero`);
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value < 1 || value > 16_384) {
      throw new Error(`--${key} debe estar entre 1 y 16384`);
    }
    return value;
  }
  const url = new URL(required("url"));
  if (!["http:", "https:"].includes(url.protocol))
    throw new Error("--url debe usar HTTP o HTTPS");
  const sourceHash = required("source-hash");
  if (!/^[a-f0-9]{64}$/.test(sourceHash))
    throw new Error("--source-hash debe ser el SHA-256 del manifiesto");
  const state = values.get("state") ?? "default";
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(state))
    throw new Error("--state debe ser un nombre de estado");
  const scale = dimension("scale", 1);
  if (scale !== 1 && scale !== 2) throw new Error("--scale solo admite 1 o 2");
  return {
    url: url.href,
    out: resolve(required("out")),
    width: dimension("width", 1440),
    height: dimension("height", 900),
    scale,
    state,
    sourceHash,
    selector: values.get("selector") ?? "#fyne-root",
    readySelector: values.get("ready-selector"),
    analysis: values.get("analysis"),
  };
}

interface BitmapResource {
  name: string;
  hash: string;
  mediaType: "image/png" | "image/jpeg";
  width: number;
  height: number;
  srcs: string[];
}

async function bitmapResources(
  analysisPath: string | undefined,
  sourceHash: string,
): Promise<BitmapResource[]> {
  if (!analysisPath) return [];
  const analysis = JSON.parse(
    await readFile(resolve(analysisPath), "utf8"),
  ) as {
    sourceHash?: unknown;
    program?: { resources?: unknown };
  };
  if (analysis.sourceHash !== sourceHash)
    throw new Error(
      "--analysis y --source-hash pertenecen a fuentes distintas",
    );
  const resources = analysis.program?.resources ?? [];
  if (!Array.isArray(resources))
    throw new Error("El análisis debe contener program.resources como lista");
  return resources.map((resource: unknown) => {
    if (!resource || typeof resource !== "object")
      throw new Error("Recurso de análisis inválido");
    const value = resource as Record<string, unknown>;
    if (
      typeof value.name !== "string" ||
      !value.name ||
      typeof value.hash !== "string" ||
      !/^[a-f0-9]{64}$/.test(value.hash) ||
      !["image/png", "image/jpeg"].includes(String(value.mediaType)) ||
      typeof value.width !== "number" ||
      !Number.isSafeInteger(value.width) ||
      value.width < 1 ||
      value.width > 16_384 ||
      typeof value.height !== "number" ||
      !Number.isSafeInteger(value.height) ||
      value.height < 1 ||
      value.height > 16_384 ||
      value.width * value.height > 64_000_000 ||
      !Array.isArray(value.srcs) ||
      value.srcs.length === 0 ||
      value.srcs.some((src: unknown) => typeof src !== "string" || !src)
    ) {
      throw new Error(
        "El análisis contiene un recurso bitmap sin tipo, hash, dimensión o src válido",
      );
    }
    return value as unknown as BitmapResource;
  });
}

async function capture() {
  const args = argumentsFrom(process.argv.slice(2));
  const resources = await bitmapResources(args.analysis, args.sourceHash);
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({
      viewport: { width: args.width, height: args.height },
      deviceScaleFactor: args.scale,
      locale: "es-ES",
      colorScheme: "light",
      reducedMotion: "reduce",
    });
    page.setDefaultTimeout(30_000);
    const pageErrors: string[] = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    const imageResponses = new Map<string, Response[]>();
    page.on("response", (imageResponse) => {
      if (imageResponse.request().resourceType() !== "image") return;
      const responses = imageResponses.get(imageResponse.url()) ?? [];
      responses.push(imageResponse);
      imageResponses.set(imageResponse.url(), responses);
    });
    const response = await page.goto(args.url, {
      waitUntil: "domcontentloaded",
    });
    if (!response?.ok())
      throw new Error(
        `La página responde ${response?.status() ?? "sin respuesta"}`,
      );
    await page.locator(args.selector).waitFor({ state: "visible" });
    if ((await page.locator(args.selector).count()) !== 1)
      throw new Error("--selector debe señalar una sola raíz");
    if (args.readySelector)
      await page.locator(args.readySelector).waitFor({ state: "visible" });
    await page.evaluate(async () => {
      await Promise.race([
        document.fonts.ready,
        new Promise<never>((_, reject) =>
          setTimeout(
            () => reject(new Error("Las fuentes no terminan de cargar")),
            15_000,
          ),
        ),
      ]);
      for (const image of document.images) {
        if (!image.complete) await image.decode();
        if (image.naturalWidth === 0)
          throw new Error(
            `Imagen sin cargar: ${image.currentSrc || image.src}`,
          );
      }
      await new Promise<void>((done) =>
        requestAnimationFrame(() => requestAnimationFrame(() => done())),
      );
    });

    const images = await page.evaluate((selector) => {
      const root = document.querySelector(selector)!;
      const nodes = [root, ...root.querySelectorAll("img")];
      return nodes
        .filter(
          (element): element is HTMLImageElement =>
            element instanceof HTMLImageElement,
        )
        .map((image) => ({
          id: image.id,
          src: image.getAttribute("src") ?? "",
          currentSrc: image.currentSrc,
          width: image.naturalWidth,
          height: image.naturalHeight,
          responsive:
            image.hasAttribute("srcset") ||
            image.hasAttribute("sizes") ||
            image.parentElement?.tagName === "PICTURE",
        }));
    }, args.selector);
    const verifiedImages: Record<
      string,
      {
        name: string;
        src: string;
        currentSrc: string;
        hash: string;
        width: number;
        height: number;
      }
    > = Object.create(null);
    for (const image of images) {
      if (!args.analysis)
        throw new Error(
          "Las imágenes requieren --analysis con el manifiesto del compilador",
        );
      if (!image.id || Object.hasOwn(verifiedImages, image.id))
        throw new Error("Una imagen necesita id explícito y único");
      if (image.responsive)
        throw new Error(
          `${image.id}: srcset, sizes y picture requieren un adaptador de recursos responsive`,
        );
      if (!image.src || /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(image.src))
        throw new Error(
          `${image.id}: solo se admiten imágenes locales del compilador`,
        );
      const matches = resources.filter((resource) =>
        resource.srcs.includes(image.src),
      );
      if (matches.length !== 1)
        throw new Error(
          `${image.id}: el src no corresponde a un único recurso compilado`,
        );
      const resource = matches[0]!;
      const expectedURL = new URL(image.src, args.url);
      if (
        expectedURL.origin !== new URL(args.url).origin ||
        image.currentSrc !== expectedURL.href
      )
        throw new Error(
          `${image.id}: el navegador seleccionó otra URL de imagen`,
        );
      if (image.width !== resource.width || image.height !== resource.height)
        throw new Error(
          `${image.id}: las dimensiones naturales no coinciden con el recurso compilado`,
        );
      const responses = imageResponses.get(image.currentSrc) ?? [];
      if (responses.length !== 1)
        throw new Error(
          `${image.id}: la captura requiere una única respuesta verificable de imagen`,
        );
      const imageResponse = responses[0]!;
      if (imageResponse.request().redirectedFrom())
        throw new Error(`${image.id}: no se admiten redirecciones de imagen`);
      if (!imageResponse.ok())
        throw new Error(
          `${image.id}: el recurso responde ${imageResponse.status()}`,
        );
      const mediaType = imageResponse
        .headers()
        ["content-type"]?.split(";")[0]
        ?.trim();
      if (mediaType !== resource.mediaType)
        throw new Error(
          `${image.id}: el tipo MIME no coincide con el recurso compilado`,
        );
      const bytes = await imageResponse.body();
      if (bytes.length > 20 * 1024 * 1024)
        throw new Error(`${image.id}: el recurso supera 20 MiB`);
      if (createHash("sha256").update(bytes).digest("hex") !== resource.hash)
        throw new Error(
          `${image.id}: el hash de la imagen cargada no coincide con el compilador`,
        );
      verifiedImages[image.id] = {
        name: resource.name,
        src: image.src,
        currentSrc: image.currentSrc,
        hash: resource.hash,
        width: resource.width,
        height: resource.height,
      };
    }

    const measure = () =>
      page.evaluate(
        ({ selector, sourceHash, state, verifiedImages }) => {
          const root = document.querySelector(selector);
          if (!(root instanceof HTMLElement))
            throw new Error("La raíz debe ser un elemento HTML");
          if (
            root.dataset.fyneSourceHash &&
            root.dataset.fyneSourceHash !== sourceHash
          ) {
            throw new Error(
              "La página y el manifiesto tienen distinto sourceHash",
            );
          }
          if (root.dataset.fyneState && root.dataset.fyneState !== state) {
            throw new Error("La página y la captura tienen distinto estado");
          }
          if (
            root
              .getAnimations({ subtree: true })
              .some((animation) => animation.playState === "running")
          ) {
            throw new Error(
              "La captura requiere un estado estable sin animaciones en curso",
            );
          }
          const elements = [root, ...root.querySelectorAll("*")];
          const nodes: Record<
            string,
            Record<string, string | number | boolean>
          > = Object.create(null);
          const tokens: Record<string, string> = Object.create(null);
          const rootCSS = getComputedStyle(root);
          for (const property of rootCSS) {
            if (!property.startsWith("--")) continue;
            const value = rootCSS.getPropertyValue(property).trim();
            if (value) tokens[property] = value;
          }
          const allIDs = new Map<string, number>();
          for (const element of document.querySelectorAll("[id]")) {
            allIDs.set(element.id, (allIDs.get(element.id) ?? 0) + 1);
          }
          const px = (
            value: string,
            description: string,
            allowNormal = false,
          ) => {
            if (allowNormal && value === "normal") return 0;
            if (!/^-?(?:\d+(?:\.\d+)?|\.\d+)px$/.test(value)) {
              throw new Error(
                `${description}: se requiere una medida en px, recibido ${value}`,
              );
            }
            return Number.parseFloat(value);
          };
          const zero = (value: string) => value === "normal" || value === "0px";
          for (const element of elements) {
            if (!(element instanceof HTMLElement))
              throw new Error(
                "SVG, canvas y elementos no HTML requieren un adaptador nativo",
              );
            const id = element.id;
            if (!id)
              throw new Error(
                `Falta id explícito en <${element.tagName.toLowerCase()}>`,
              );
            if (allIDs.get(id) !== 1) throw new Error(`id duplicado: ${id}`);
            const css = getComputedStyle(element);
            const fail = (property: string, value: string) => {
              throw new Error(`${id}: CSS no soportado: ${property}: ${value}`);
            };
            const forbidden: [string, string, string[]][] = [
              ["transform", css.transform, ["none"]],
              ["translate", css.translate, ["none"]],
              ["rotate", css.rotate, ["none"]],
              ["scale", css.scale, ["none"]],
              ["box-shadow", css.boxShadow, ["none"]],
              ["text-shadow", css.textShadow, ["none"]],
              ["background-image", css.backgroundImage, ["none"]],
              ["filter", css.filter, ["none"]],
              ["backdrop-filter", css.backdropFilter, ["none"]],
              ["mask-image", css.maskImage, ["none"]],
              ["clip-path", css.clipPath, ["none"]],
              ["mix-blend-mode", css.mixBlendMode, ["normal"]],
              ["writing-mode", css.writingMode, ["horizontal-tb"]],
              ["z-index", css.zIndex, ["auto"]],
              ["text-decoration-line", css.textDecorationLine, ["none"]],
            ];
            for (const [property, value, permitted] of forbidden) {
              if (!permitted.includes(value)) fail(property, value);
            }
            if (css.opacity !== "1") fail("opacity", css.opacity);
            if (css.visibility !== "visible")
              fail("visibility", css.visibility);
            if (
              !["block", "inline", "flex", "inline-flex", "none"].includes(
                css.display,
              )
            )
              fail("display", css.display);
            for (const pseudo of ["::before", "::after"]) {
              const content = getComputedStyle(element, pseudo).content;
              if (content !== "none" && content !== "normal")
                fail(`${pseudo} content`, content);
            }
            if (element.shadowRoot)
              fail("shadow-root", "requiere un adaptador");
            if (
              ["CANVAS", "VIDEO", "IFRAME", "AUDIO", "PICTURE"].includes(
                element.tagName,
              )
            )
              fail("elemento", element.tagName);
            if (element.getClientRects().length > 1)
              fail(
                "fragmentación en varias líneas",
                "requiere un nodo por fragmento",
              );
            if (
              element.scrollWidth > element.clientWidth + 1 ||
              element.scrollHeight > element.clientHeight + 1
            ) {
              fail("overflow", "contenido fuera del rectángulo medido");
            }
            const borders = [
              css.borderTopWidth,
              css.borderRightWidth,
              css.borderBottomWidth,
              css.borderLeftWidth,
            ];
            const borderColors = [
              css.borderTopColor,
              css.borderRightColor,
              css.borderBottomColor,
              css.borderLeftColor,
            ];
            const borderStyles = [
              css.borderTopStyle,
              css.borderRightStyle,
              css.borderBottomStyle,
              css.borderLeftStyle,
            ];
            if (
              new Set(borders).size !== 1 ||
              (Number.parseFloat(css.borderTopWidth) > 0 &&
                (new Set(borderColors).size !== 1 ||
                  borderStyles.some((style) => style !== "solid")))
            ) {
              fail("border", "se requiere un borde sólido uniforme");
            }
            const radii = [
              css.borderTopLeftRadius,
              css.borderTopRightRadius,
              css.borderBottomRightRadius,
              css.borderBottomLeftRadius,
            ];
            if (new Set(radii).size !== 1)
              fail("border-radius", "se requiere un radio uniforme");
            if (element instanceof HTMLImageElement) {
              const verified = verifiedImages[id];
              if (
                !verified ||
                element.getAttribute("src") !== verified.src ||
                element.currentSrc !== verified.currentSrc ||
                element.naturalWidth !== verified.width ||
                element.naturalHeight !== verified.height
              ) {
                fail(
                  "img src",
                  "la imagen cambió respecto al recurso verificado",
                );
              }
              if (css.objectFit !== "fill") fail("object-fit", css.objectFit);
              if (css.objectPosition !== "50% 50%")
                fail("object-position", css.objectPosition);
              if (
                Number.parseFloat(css.borderTopWidth) !== 0 ||
                Number.parseFloat(css.borderTopLeftRadius) !== 0 ||
                [
                  css.paddingTop,
                  css.paddingRight,
                  css.paddingBottom,
                  css.paddingLeft,
                ].some((value) => Number.parseFloat(value) !== 0)
              ) {
                fail(
                  "img box",
                  "bordes, radios y padding necesitan un adaptador de contenido y recorte",
                );
              }
            }
            const hasText =
              [...element.childNodes].some(
                (node) =>
                  node.nodeType === Node.TEXT_NODE && node.textContent?.trim(),
              ) ||
              element instanceof HTMLInputElement ||
              element instanceof HTMLTextAreaElement;
            let fontFamily = "";
            let fontStyle = "normal";
            let fontWeight = 400;
            let fontSize = 0;
            let lineHeight = 0;
            let textAlign = "left";
            let whiteSpace = "normal";
            if (hasText) {
              fontFamily = (css.fontFamily.split(",")[0] ?? "")
                .trim()
                .replace(/^['"]|['"]$/g, "");
              if (!fontFamily) fail("font-family", css.fontFamily);
              fontStyle = css.fontStyle;
              fontWeight = Number(css.fontWeight);
              if (!Number.isFinite(fontWeight))
                fail("font-weight", css.fontWeight);
              const faceLoaded = [...document.fonts].some((face) => {
                const family = face.family.replace(/^['"]|['"]$/g, "");
                const weights = face.weight
                  .split(/\s+/)
                  .map((weight) =>
                    weight === "normal"
                      ? 400
                      : weight === "bold"
                        ? 700
                        : Number(weight),
                  );
                const minimumWeight = weights[0];
                const maximumWeight = weights[weights.length - 1];
                return (
                  family === fontFamily &&
                  face.status === "loaded" &&
                  face.style === fontStyle &&
                  weights.every(Number.isFinite) &&
                  minimumWeight !== undefined &&
                  maximumWeight !== undefined &&
                  fontWeight >= minimumWeight &&
                  fontWeight <= maximumWeight
                );
              });
              if (!faceLoaded)
                fail(
                  "font-face",
                  `${fontFamily} ${fontWeight} ${fontStyle} no tiene una cara web cargada`,
                );
              if (!zero(css.letterSpacing))
                fail("letter-spacing", css.letterSpacing);
              if (!zero(css.wordSpacing)) fail("word-spacing", css.wordSpacing);
              if (css.textTransform !== "none")
                fail("text-transform", css.textTransform);
              if (css.textIndent !== "0px") fail("text-indent", css.textIndent);
              if (!["left", "center", "right", "start"].includes(css.textAlign))
                fail("text-align", css.textAlign);
              if (!["normal", "nowrap"].includes(css.whiteSpace))
                fail("white-space", css.whiteSpace);
              if (css.direction !== "ltr") fail("direction", css.direction);
              textAlign = css.textAlign === "start" ? "left" : css.textAlign;
              whiteSpace = css.whiteSpace;
              fontSize = px(css.fontSize, `${id} font-size`);
              lineHeight = px(css.lineHeight, `${id} line-height`);
              if (
                (element instanceof HTMLInputElement ||
                  element instanceof HTMLTextAreaElement) &&
                element.placeholder
              ) {
                const placeholder = getComputedStyle(element, "::placeholder");
                for (const property of [
                  "color",
                  "fontFamily",
                  "fontSize",
                  "fontStyle",
                  "fontWeight",
                  "letterSpacing",
                  "opacity",
                ] as const) {
                  if (placeholder[property] !== css[property])
                    fail(`::placeholder ${property}`, placeholder[property]);
                }
              }
            }
            const rect = element.getBoundingClientRect();
            const parentRect =
              element === root
                ? { left: 0, top: 0 }
                : element.parentElement!.getBoundingClientRect();
            const rowGap = px(css.rowGap, `${id} row-gap`, true);
            const columnGap = px(css.columnGap, `${id} column-gap`, true);
            if (rowGap !== columnGap) fail("gap", `${rowGap}px ${columnGap}px`);
            nodes[id] = {
              x: rect.left - parentRect.left,
              y: rect.top - parentRect.top,
              width: rect.width,
              height: rect.height,
              paddingTop: px(css.paddingTop, `${id} padding-top`),
              paddingRight: px(css.paddingRight, `${id} padding-right`),
              paddingBottom: px(css.paddingBottom, `${id} padding-bottom`),
              paddingLeft: px(css.paddingLeft, `${id} padding-left`),
              gap: rowGap,
              direction: css.flexDirection,
              background: css.backgroundColor,
              color: css.color,
              borderColor: css.borderTopColor,
              borderWidth: px(css.borderTopWidth, `${id} border-width`),
              radius: px(css.borderTopLeftRadius, `${id} border-radius`),
              fontFamily,
              fontStyle,
              fontWeight,
              fontSize,
              lineHeight,
              textAlign,
              whiteSpace,
              display: css.display,
              opacity: 1,
              measured: true,
            };
          }
          return { nodes, tokens, html: root.outerHTML };
        },
        {
          selector: args.selector,
          sourceHash: args.sourceHash,
          state: args.state,
          verifiedImages,
        },
      );

    const before = await measure();
    const screenshot = await page.screenshot({
      fullPage: false,
      animations: "disabled",
      caret: "hide",
      scale: "device",
    });
    const after = await measure();
    if (JSON.stringify(before) !== JSON.stringify(after))
      throw new Error("La página cambió durante la captura");
    for (const image of Object.values(verifiedImages)) {
      if (imageResponses.get(image.currentSrc)?.length !== 1)
        throw new Error("Una imagen se volvió a cargar durante la captura");
    }
    if (pageErrors.length)
      throw new Error(`Errores de JavaScript: ${pageErrors.join("; ")}`);
    await mkdir(args.out, { recursive: true });
    await writeFile(resolve(args.out, "web.png"), screenshot);
    await writeFile(
      resolve(args.out, "measurements.json"),
      `${JSON.stringify(
        {
          schema: 1,
          sourceHash: args.sourceHash,
          state: args.state,
          selector: args.selector,
          url: args.url,
          viewport: {
            width: args.width,
            height: args.height,
            scale: args.scale,
          },
          domHash: createHash("sha256").update(before.html).digest("hex"),
          screenshotHash: createHash("sha256").update(screenshot).digest("hex"),
          tokens: before.tokens,
          resources: verifiedImages,
          nodes: before.nodes,
        },
        null,
        2,
      )}\n`,
    );
    console.log(
      `Captura ${args.state}: ${Object.keys(before.nodes).length} nodos, ${args.width}×${args.height} @${args.scale}`,
    );
  } finally {
    await browser.close();
  }
}

capture().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
