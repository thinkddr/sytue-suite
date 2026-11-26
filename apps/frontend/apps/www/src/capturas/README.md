<a id="capturas-de-producto"></a>

# Product screenshots

The images in this folder appear on the `/product/<key>` pages, **by naming
convention**. There is no list to maintain anywhere:

| File                            | Where it appears                                        |
| ------------------------------- | ------------------------------------------------------- |
| `<key>.<idioma>.webp`           | Below the page headline, before the four bullet points. |
| `<key>-funciones.<idioma>.webp` | After the long list of features.                        |

`<key>` is the product key (`mail`, `drive`, `notes`, `photos`…), the same one as
in `PRODUCT_KEYS` in `src/lib/product-content.ts`. `<idioma>` (language) is one of
the five in `LOCALES` (`packages/i18n`). A product without a screenshot shows the
usual page: no empty gap and no broken image.

**The language goes in the name because the screen is translated too.** Showing
the Spanish screenshot on the German page leaves half the page in one language and
the image illustrating it in another. If the one for a language is missing, the
page falls back to the default language's screenshot rather than going without an image.

What is **not** translated is the account content —email subjects, note
titles—: it is what is inside the demo mailbox and it stays in Spanish in all
five. What changes is the interface, which is what the page is there to show.

<a id="cómo-se-hacen"></a>

## How they are made

**1440×900** (16/10 aspect ratio, the one `Captura.astro` reserves), one pass per
language and with a demo account, never with anyone's data. They are generated
with Playwright against the deployed suite and converted to WebP:

```
magick captura.png -quality 82 -define webp:method=6 <key>.webp
```

That they are **real screenshots** is not a detail: the page promises what the
product does today, and an image retouched with features that do not exist turns
that promise into misleading advertising.

<a id="lo-que-no-se-publica"></a>

## What is not published

Before shooting, the script **blurs** anything that identifies someone:
email addresses, IP addresses and the `@usuario` (`@username`). It does not touch
the domains reserved for documentation (RFC 2606 and 6761: `.example`, `.test`,
`.invalid`, `example.com`…), which are precisely the demo data the page is there
to show: covering `carla.benet@ejemplo.test` leaves the screen blurry without
protecting anyone.

It is done with CSS and **before** the shot, not by cropping the PNG afterwards: a
coordinate-based crop goes out of alignment as soon as the design changes, and then
the data is left exposed without anyone noticing.

Even so, **look at them before committing**. The script covers what it knows how to
recognize; a person's name, the subject of a real email or whatever is written inside
a document are not recognized, and there is no safety net there.
