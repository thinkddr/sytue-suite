// SPDX-License-Identifier: Apache-2.0

/** Declarative, portable expressions. No JavaScript is evaluated during conversion. */
export type Expr =
  | { kind: "literal"; value: string | number | boolean | null }
  | { kind: "undefined" }
  | { kind: "name"; name: string }
  /** Only the lexical updater parameter reads pending state; closures stay fixed. */
  | { kind: "current"; name: string }
  | { kind: "get"; object: Expr; key: Expr }
  | { kind: "binary"; op: string; left: Expr; right: Expr }
  | { kind: "unary"; op: string; value: Expr }
  | { kind: "conditional"; test: Expr; yes: Expr; no: Expr }
  | { kind: "array"; items: Expr[] }
  | { kind: "object"; entries: Record<string, Expr> }
  | { kind: "template"; parts: Expr[] }
  | { kind: "call"; name: string; args: Expr[] };

export interface Handler {
  parameter?: string;
  steps: {
    kind: "set" | "call";
    name: string;
    args: Expr[];
    updater?: boolean;
  }[];
}

export type Node =
  | {
      kind: "element";
      id: string;
      tag: string;
      attrs: Record<string, Expr>;
      events: Record<string, Handler>;
      children: Node[];
      /** A compile-time local bitmap, never a network URL or interpreted DOM. */
      imageResource?: string;
    }
  | { kind: "text"; value: Expr }
  | { kind: "conditional"; test: Expr; yes: Node[]; no: Node[] }
  | {
      kind: "each";
      items: Expr;
      item: string;
      index?: string;
      children: Node[];
    }
  | {
      kind: "component";
      id: string;
      name: string;
      props: Record<string, Expr>;
      children: Node[];
    };

export interface Component {
  name: string;
  props: string[];
  /** An object parameter receives the complete supplied props under this binding. */
  propsObject?: string;
  states: { name: string; setter: string; initial: Expr }[];
  constants: { name: string; value: Expr }[];
  /** Preserve lexical initialization order, including state depending on const. */
  initializers: { kind: "state" | "constant"; name: string }[];
  body: Node[];
}

export interface Program {
  entry: string;
  components: Component[];
  sources: { path: string; hash: string }[];
  actions: string[];
  /** Inline/external CSS requires captured browser measurements in stage 01. */
  hasStyles?: boolean;
  resources?: BitmapResource[];
}

export interface BitmapResource {
  name: string;
  path: string;
  hash: string;
  mediaType: "image/png" | "image/jpeg";
  /** Original bytes encoded for deterministic, portable compiler artifacts. */
  content: string;
  width: number;
  height: number;
  /** Exact literal HTML sources; capture verifies served bytes against hash. */
  srcs: string[];
}

export interface CompileOptions {
  /** Source paths and stable component names are relative to this directory. */
  root?: string;
  /** Override Astro's default public directory without executing its config. */
  publicDir?: string;
  /** Optional package adapters: package -> exported name -> native builtin name. */
  adapters?: Record<string, Record<string, string>>;
}
