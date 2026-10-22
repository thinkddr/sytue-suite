# Capturas de producto

Las imágenes de esta carpeta salen en las fichas `/product/<key>`, **por
convención de nombre**. No hay lista que mantener en ninguna parte:

| Fichero                         | Dónde sale                                               |
| ------------------------------- | -------------------------------------------------------- |
| `<key>.<idioma>.webp`           | Bajo el titular de la ficha, antes de los cuatro puntos. |
| `<key>-funciones.<idioma>.webp` | Después de la lista larga de funciones.                  |

`<key>` es la clave del producto (`mail`, `drive`, `notes`, `photos`…), la misma
de `PRODUCT_KEYS` en `src/lib/product-content.ts`. `<idioma>` es uno de los cinco
de `LOCALES` (`packages/i18n`). Un producto sin captura enseña la ficha de
siempre: no hay hueco vacío ni imagen rota.

**El idioma va en el nombre porque la pantalla también está traducida.** Enseñar
la captura en español dentro de la ficha alemana deja media página en un idioma y
la imagen que la ilustra en otro. Si falta la de un idioma, la ficha cae a la del
idioma por defecto antes que quedarse sin imagen.

Lo que **no** se traduce es el contenido de la cuenta —los asuntos de los correos,
los títulos de las notas—: es lo que hay dentro del buzón de demostración y sigue
en español en las cinco. Lo que cambia es la interfaz, que es lo que la ficha
viene a enseñar.

## Cómo se hacen

**1440×900** (proporción 16/10, la que reserva `Captura.astro`), una pasada por
idioma y con una cuenta de demostración, nunca con datos de nadie. Se generan con
Playwright contra la suite desplegada y se convierten a WebP:

```
magick captura.png -quality 82 -define webp:method=6 <key>.webp
```

Que sean **capturas de verdad** no es un detalle: la ficha promete lo que el
producto hace hoy, y una imagen retocada con funciones que no existen convierte
esa promesa en publicidad engañosa.

## Lo que no se publica

Antes de disparar, el guion **desenfoca** lo que identifica a alguien:
direcciones de correo, direcciones IP y el `@usuario`. No toca los dominios reservados para
documentación (RFC 2606 y 6761: `.example`, `.test`, `.invalid`,
`example.com`…), que son justamente los datos de demostración que la ficha viene
a enseñar: tapar `carla.benet@ejemplo.test` deja la pantalla borrosa sin
proteger a nadie.

Va con CSS y **antes** del disparo, no recortando el PNG después: un recorte por
coordenadas se desalinea en cuanto cambia el diseño, y entonces el dato queda al
aire sin que nadie se entere.

Aun así, **míralas antes de commitear**. El guion tapa lo que sabe reconocer; un
nombre propio, el asunto de un correo de verdad o lo que ponga dentro de un
documento no los reconoce, y ahí no hay red que te salve.
