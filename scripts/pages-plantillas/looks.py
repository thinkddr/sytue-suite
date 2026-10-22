# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
"""Aspectos (paleta, tipografía, radios) que comparten las plantillas. Cada paleta pasa AA al generar."""

from lib import comprobar_paleta


def _c(background, surface, text, muted, primary, on_primary, border, inverse, on_inverse):
    return dict(background=background, surface=surface, text=text, muted=muted, primary=primary,
                on_primary=on_primary, border=border, inverse=inverse, on_inverse=on_inverse)


# Fuentes del catálogo: inter figtree source-serif-4 lora playfair-display dm-sans work-sans manrope
# ibm-plex-sans noto-sans noto-serif montserrat.
LOOKS = {
    # Cálidos y de oficio
    "terracota": dict(colores=_c("#fdfaf5", "#f4ece1", "#2a1d17", "#6b5546", "#9a3412", "#ffffff", "#e4d6c5", "#2b1e18", "#fbf3ea"),
                      fuentes=("playfair-display", "source-serif-4"), radio=(4, 8, 12)),
    "barberia": dict(colores=_c("#14130f", "#1f1d18", "#f4efe6", "#b9b2a4", "#d4a85a", "#14130f", "#3a362d", "#0b0a08", "#f4efe6"),
                     fuentes=("montserrat", "work-sans"), peso=800, mayus="uppercase", ls=0.04, h1=(48, 38, 30), radio=(0, 2, 4)),
    "salvia": dict(colores=_c("#fbfcf8", "#eef2e7", "#1d2a1f", "#52614f", "#3f6b3a", "#ffffff", "#d9e1cf", "#1f3322", "#f1f5ea"),
                   fuentes=("lora", "manrope"), radio=(6, 10, 16)),
    "rosa": dict(colores=_c("#fffafa", "#fbeeee", "#2d1b1f", "#6f5258", "#a3304f", "#ffffff", "#f0d9dc", "#3a1c25", "#fff1f3"),
                 fuentes=("playfair-display", "dm-sans"), radio=(8, 14, 22)),
    "marino": dict(colores=_c("#ffffff", "#f1f5f9", "#0f1f33", "#4a5b70", "#1d4e89", "#ffffff", "#d8e0ea", "#0f2744", "#eef4fb"),
                   fuentes=("ibm-plex-sans", "ibm-plex-sans"), radio=(2, 4, 8), ancho=1160),
    "burdeos": dict(colores=_c("#fdfbf8", "#f5eeea", "#26181a", "#675256", "#7a1f2e", "#ffffff", "#e6d8d3", "#2a1217", "#faf0ee"),
                    fuentes=("playfair-display", "source-serif-4"), radio=(2, 4, 8)),
    "pizarra": dict(colores=_c("#f8fafc", "#eaeff5", "#10202f", "#455567", "#0b6e8a", "#ffffff", "#d3dce6", "#0f2a3a", "#ecf5f9"),
                    fuentes=("dm-sans", "figtree"), radio=(6, 10, 14)),
    "mostaza": dict(colores=_c("#fffdf6", "#f8f0d6", "#241f0f", "#625a3d", "#8a5a00", "#ffffff", "#ebe0b8", "#2a2410", "#fdf6dc"),
                    fuentes=("montserrat", "figtree"), peso=700, radio=(4, 8, 12)),
    "menta": dict(colores=_c("#f9fdfc", "#e8f5f1", "#10261f", "#42605a", "#0c7a64", "#ffffff", "#cfe6df", "#0e2e27", "#eaf8f4"),
                  fuentes=("manrope", "manrope"), radio=(8, 12, 18)),
    "ciruela": dict(colores=_c("#fdfafd", "#f3ebf4", "#26172a", "#64506a", "#7a2f8a", "#ffffff", "#e3d5e6", "#2a1530", "#f8eefa"),
                    fuentes=("lora", "dm-sans"), radio=(6, 12, 18)),
    "grafito": dict(colores=_c("#f7f7f5", "#ebebe7", "#1b1b1a", "#555550", "#b03a0a", "#ffffff", "#d9d9d3", "#1b1b1a", "#f4f4f0"),
                    fuentes=("montserrat", "work-sans"), peso=800, mayus="uppercase", ls=0.03, h1=(50, 40, 32), radio=(2, 4, 6)),
    "celeste": dict(colores=_c("#fbfdff", "#e9f3fb", "#0d2236", "#476079", "#0a6ebd", "#ffffff", "#d3e4f2", "#0c2a45", "#eaf4fd"),
                    fuentes=("figtree", "figtree"), radio=(8, 12, 18)),
    "arena": dict(colores=_c("#fbf8f1", "#f1eadb", "#2a2417", "#665c46", "#1f5f7a", "#ffffff", "#e0d6bf", "#1d3a46", "#f1f7f9"),
                  fuentes=("lora", "source-serif-4"), radio=(2, 6, 10)),
    "coral": dict(colores=_c("#fffaf8", "#fdece6", "#2d1a14", "#6e5148", "#b3391d", "#ffffff", "#f1d8cf", "#3a1a12", "#fff0ea"),
                  fuentes=("dm-sans", "work-sans"), radio=(10, 16, 24)),
    "oliva": dict(colores=_c("#fbfbf4", "#eeeed9", "#232612", "#59593a", "#5c6b12", "#ffffff", "#dcdcbc", "#262a10", "#f6f7e4"),
                  fuentes=("noto-serif", "noto-sans"), radio=(4, 8, 12)),
    "noche": dict(colores=_c("#0f1419", "#18202a", "#eef2f6", "#a9b4c0", "#5fb3d9", "#0b1218", "#2c3745", "#070b0f", "#eef2f6"),
                  fuentes=("montserrat", "inter"), peso=700, radio=(4, 8, 12)),
    "vino": dict(colores=_c("#161012", "#211819", "#f6ece8", "#c5b3ad", "#e0a458", "#161012", "#3b2b2d", "#0d0809", "#f6ece8"),
                 fuentes=("playfair-display", "source-serif-4"), radio=(2, 4, 8)),
    "lavanda": dict(colores=_c("#fbfaff", "#eeebfa", "#1c1833", "#545078", "#4b3fb0", "#ffffff", "#dad6f0", "#1f1a4a", "#f1efff"),
                    fuentes=("dm-sans", "figtree"), radio=(8, 14, 22)),
    "tinta": dict(colores=_c("#ffffff", "#f3f3f3", "#111111", "#505050", "#0b57d0", "#ffffff", "#dddddd", "#111111", "#f5f5f5"),
                  fuentes=("inter", "inter"), radio=(2, 6, 10), ancho=1080),
    "campo": dict(colores=_c("#fcfbf5", "#f0efdf", "#1f2a14", "#55603e", "#2f6b2f", "#ffffff", "#dcdcc4", "#1f3a1c", "#f1f6e8"),
                  fuentes=("lora", "work-sans"), radio=(4, 8, 14)),
    "ladrillo": dict(colores=_c("#fdf9f6", "#f4e9e1", "#2b1c14", "#6a5242", "#a0391a", "#ffffff", "#e5d3c7", "#33190f", "#fceee6"),
                     fuentes=("montserrat", "work-sans"), peso=700, radio=(2, 4, 6)),
    "cobalto": dict(colores=_c("#fbfcff", "#e8eefc", "#0e1a3d", "#46547c", "#2740c4", "#ffffff", "#d3dcf3", "#0e1a4a", "#eef2ff"),
                    fuentes=("manrope", "inter"), radio=(6, 10, 16)),
    "durazno": dict(colores=_c("#fffaf5", "#fdeedd", "#2e1d10", "#705540", "#b25200", "#ffffff", "#f2dcc4", "#3b1f0c", "#fff1e3"),
                    fuentes=("dm-sans", "figtree"), radio=(8, 14, 20)),
    "bosque": dict(colores=_c("#f9fbf8", "#e6efe6", "#12261a", "#46604f", "#1c5a3a", "#ffffff", "#cfdfd2", "#10301f", "#e9f5ee"),
                   fuentes=("lora", "manrope"), radio=(4, 8, 12)),
    "pizarra-oscura": dict(colores=_c("#10151c", "#19212b", "#f1f4f8", "#a8b3c1", "#f0b84a", "#10151c", "#2b3541", "#080b10", "#f1f4f8"),
                           fuentes=("montserrat", "work-sans"), peso=800, mayus="uppercase", ls=0.03, h1=(48, 38, 30), radio=(2, 4, 8)),
}


def comprobar():
    malos = []
    for k, v in LOOKS.items():
        malos += comprobar_paleta(k, v["colores"])
    return malos


if __name__ == "__main__":
    m = comprobar()
    print("\n".join(m) or "todas las paletas pasan AA")
