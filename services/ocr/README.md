<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue OCR

Optical character recognition for Sytue PDF. It turns the image of a
scanned page into words with their positions; it does **not** touch PDF: that is done by Sytue
Docs, which writes the invisible text layer ([ADR-0034](../../docs/decisions.md#0034)).

<a id="por-qué-es-un-servicio-y-no-una-biblioteca"></a>
## Why it is a service and not a library

The engine is **Tesseract 5** (Apache-2.0, self-hosted). It is a C binary with
about 15 MB of language data per language. Putting it inside the Sytue
Docs image would mean:

- that **every Docs replica** carries ~100 MB of data that only one
  feature in every thousand requests uses;
- CGO in the Docs binary, which today is static on distroless;
- and that scaling OCR (which is what consumes CPU) would force scaling the editor.

As a separate service, Docs remains a static binary and OCR scales on
its own, which is exactly what you should do with something that burns CPU in bursts.

<a id="qué-no-hace"></a>
## What it does NOT do

- **It stores nothing.** The image comes in, the text goes out and it is forgotten. No
  database, no disk, no content logging.
- **It does not go out to the internet.** There are no outbound calls at all: the model is in the
  image. A scanned document never leaves the cluster.
- **It does not talk to the browser.** It is ClusterIP, with no ingress. Only Docs calls it.

## API

    POST /api/v1/recognize     multipart: file=<imagen PNG/JPEG>, idiomas=spa+eng
    → { "ancho": 2480, "alto": 3508, "palabras": [ {texto, x, y, ancho, alto, confianza} ] }

Coordinates are in **image pixels**, with the origin at the top left (which
is how Tesseract returns them). Docs is the one that converts them to the PDF coordinate system, since it
is the one that knows where the image is placed on the page.

The credential is the **user's JWT**, which Docs forwards. No shared secret
and no trust in the network: an OCR request is attributable to whoever made it.

<a id="idiomas"></a>
## Languages

The five product languages (`spa`, `eng`, `fra`, `deu`, `ita`) are shipped in the image. The
request asks for whichever it wants using Tesseract syntax (`spa+eng`), and the
service **rejects** the ones it does not have instead of recognizing in the wrong
language, which gives a plausible but wrong result.
