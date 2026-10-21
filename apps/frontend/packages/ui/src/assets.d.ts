/** Declaraciones de modulos de assets para imports tipados (Astro/Vite). */
declare module '*.svg?url' {
  const url: string;
  export default url;
}
declare module '*.svg' {
  const url: string;
  export default url;
}
/** El SVG como TEXTO, para poder teñirlo antes de pintarlo (ver OrgMark). */
declare module '*.svg?raw' {
  const source: string;
  export default source;
}
