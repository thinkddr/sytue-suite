<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue OCR

Reconocimiento óptico de caracteres para Sytue PDF. Convierte la imagen de una
página escaneada en palabras con su posición; **no** toca PDF: eso lo hace Sytue
Docs, que escribe la capa de texto invisible ([ADR-0034](../../docs/decisions.md#0034)).

## Por qué es un servicio y no una biblioteca

El motor es **Tesseract 5** (Apache-2.0, autoalojado). Es un binario en C con
unos 15 MB de datos de idioma por lengua. Meterlo dentro de la imagen de Sytue
Docs significaría:

- que **cada réplica de Docs** cargue con ~100 MB de datos que solo usa una
  función de cada mil peticiones;
- CGO en el binario de Docs, que hoy es estático sobre distroless;
- y que escalar el OCR —que es lo que consume CPU— obligue a escalar el editor.

Como servicio aparte, Docs sigue siendo un binario estático y el OCR escala por
su cuenta, que es justo lo que hay que hacer con lo que gasta CPU a ráfagas.

## Qué NO hace

- **No guarda nada.** La imagen entra, sale el texto y se olvida. Sin base de
  datos, sin disco, sin registro de contenido.
- **No sale a internet.** No hay ninguna llamada saliente: el modelo está en la
  imagen. Un documento escaneado nunca abandona el clúster.
- **No habla con el navegador.** Es ClusterIP, sin ingress. Solo lo llama Docs.

## API

    POST /api/v1/recognize     multipart: file=<imagen PNG/JPEG>, idiomas=spa+eng
    → { "ancho": 2480, "alto": 3508, "palabras": [ {texto, x, y, ancho, alto, confianza} ] }

Las coordenadas van en **píxeles de la imagen**, con origen arriba-izquierda —que
es como las da Tesseract—. Quien las convierte al sistema del PDF es Docs, que es
quien sabe dónde está colocada la imagen en la página.

La credencial es el **JWT del usuario**, que Docs reenvía. Ni secreto compartido
ni confianza en la red: una petición de OCR es atribuible a quien la pidió.

## Idiomas

Los cinco de producto (`spa`, `eng`, `fra`, `deu`, `ita`) van en la imagen. La
petición pide los que quiera con la sintaxis de Tesseract (`spa+eng`), y el
servicio **rechaza** los que no tenga en vez de reconocer en el idioma
equivocado, que da un resultado plausible y mal.
