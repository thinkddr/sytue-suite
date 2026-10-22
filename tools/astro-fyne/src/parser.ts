// SPDX-License-Identifier: Apache-2.0

import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import {
  basename,
  dirname,
  extname,
  isAbsolute,
  relative,
  resolve,
} from "node:path";
import ts from "typescript";
import { decodeHTML, decodeHTMLAttribute } from "entities";
import type {
  CompileOptions,
  Component,
  BitmapResource,
  Expr,
  Handler,
  Node,
  Program,
} from "./ir.ts";
import { loadBitmap } from "./resources.ts";

interface AstroNode {
  type: string;
  name?: string;
  value?: string;
  attributes?: { name: string; kind: string; value: string }[];
  children?: AstroNode[];
  position?: { start: { line: number; column: number } };
}

interface AstroCompiler {
  parse(
    source: string,
    options: { position: boolean },
  ): Promise<{
    ast: AstroNode;
    diagnostics: {
      severity: number;
      text: string;
      location?: { line: number; column: number };
    }[];
  }>;
}

interface Import {
  from: string;
  exported: string;
}

interface Definition {
  node: ts.FunctionDeclaration | ts.ArrowFunction | ts.FunctionExpression;
  name: string;
}

interface Source {
  path: string;
  text: string;
  ts: ts.SourceFile;
  imports: Map<string, Import>;
  definitions: Map<string, Definition>;
  exports: Map<string, string>;
  globals: ts.VariableDeclaration[];
  astro?: AstroNode;
}

interface Scope {
  source: Source;
  component: Component;
  names: Set<string>;
  /** Includes declarations not initialized yet, so calls respect lexical shadowing. */
  bindings: Set<string>;
  /** List parameters that shadow the component's enclosing lexical bindings. */
  listShadows: Set<string>;
  /** Only explicit props can authorize an ordinary host callback call. */
  hostBindings: Set<string>;
  setters: Map<string, string>;
  handlers: Map<string, ts.ArrowFunction | ts.FunctionExpression>;
  substitutions: Map<string, Expr>;
  location?: { line: number; column: number };
}

const TAGS = new Set([
  "div",
  "main",
  "section",
  "header",
  "footer",
  "nav",
  "article",
  "form",
  "label",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "p",
  "span",
  "strong",
  "button",
  "a",
  "input",
  "textarea",
  "ul",
  "ol",
  "li",
  "img",
]);
const BUILTINS = new Set(["Button", "Input", "Card", "CardBody", "CardTitle"]);
const PURE_CALLS = new Set(["String", "Number", "Boolean", "t"]);
const BINARY = new Set([
  "+",
  "-",
  "*",
  "/",
  "%",
  "===",
  "!==",
  "<",
  "<=",
  ">",
  ">=",
  "&&",
  "||",
  "??",
]);
const literal = (value: string | number | boolean | null): Expr => ({
  kind: "literal",
  value,
});
const hash = (text: string): string =>
  createHash("sha256").update(text).digest("hex");
const compare = (left: string, right: string): number =>
  left < right ? -1 : left > right ? 1 : 0;

/** A rejected construct always includes the original source location. */
export class ConversionError extends Error {
  constructor(path: string, line: number, column: number, message: string) {
    super(`${path}:${line}:${column}: ${message}`);
    this.name = "ConversionError";
  }
}

/** Compile a default (or explicitly named) Astro/Preact entry without executing its code. */
export async function compile(
  path: string,
  exported?: string,
  options: CompileOptions = {},
): Promise<Program> {
  return new Compiler(options).compile(resolve(path), exported);
}

class Compiler {
  private sources = new Map<string, Source>();
  private components = new Map<string, Component>();
  private active = new Set<string>();
  private actions = new Set<string>();
  private ids = new Map<string, Set<string>>();
  private nextID = 0;
  private root = "";
  private entrySource = "";
  private styles = new Map<string, string>();
  private hasStyles = false;
  private resources = new Map<string, BitmapResource>();
  private jsxReferences = new Map<string, string>();

  constructor(private readonly options: CompileOptions) {}

  async compile(path: string, exported?: string): Promise<Program> {
    this.root = resolve(this.options.root ?? dirname(path));
    this.entrySource = path;
    const source = await this.source(path);
    let choice = exported;
    if (!choice) {
      choice = source.astro
        ? "default"
        : source.exports.has("default")
          ? "default"
          : undefined;
      if (!choice) {
        const candidates = [...source.exports.keys()].filter((name) =>
          /^[A-Z]/.test(name),
        );
        if (candidates.length === 1) choice = candidates[0];
      }
    }
    if (!choice)
      this.fail(
        source,
        source.ts,
        "Elige un componente exportado; la entrada es ambigua.",
      );
    const entry = await this.component(source, choice);
    return {
      entry,
      components: [...this.components.values()].sort((a, b) =>
        compare(a.name, b.name),
      ),
      sources: [
        ...[...this.sources.values()].map((s) => ({
          path: s.path,
          text: s.text,
        })),
        ...[...this.styles].map(([path, text]) => ({ path, text })),
      ]
        .map((s) => ({ path: this.sourcePath(s.path), hash: hash(s.text) }))
        .concat(
          [...this.resources.values()].map(({ path, hash }) => ({
            path,
            hash,
          })),
        )
        .sort((a, b) => compare(a.path, b.path)),
      actions: [...this.actions].sort(),
      hasStyles: this.hasStyles,
      resources: [...this.resources.values()].sort((a, b) =>
        compare(a.path, b.path),
      ),
    };
  }

  private sourcePath(path: string): string {
    return relative(this.root, path).replaceAll("\\", "/");
  }

  private fail(
    source: Source,
    node: ts.Node,
    message: string,
    scope?: Scope,
  ): never {
    const location =
      scope?.location ??
      (() => {
        const p = source.ts.getLineAndCharacterOfPosition(
          node.getStart(source.ts),
        );
        return { line: p.line + 1, column: p.character + 1 };
      })();
    throw new ConversionError(
      this.sourcePath(source.path),
      location.line,
      location.column,
      message,
    );
  }

  private async compiler(): Promise<AstroCompiler> {
    return (await import("@astrojs/compiler")) as AstroCompiler;
  }

  private async source(path: string): Promise<Source> {
    const previous = this.sources.get(path);
    if (previous) return previous;
    if (![".astro", ".tsx", ".ts", ".jsx", ".js"].includes(extname(path))) {
      throw new ConversionError(
        this.sourcePath(path),
        1,
        1,
        "La fuente debe ser Astro, TSX o JSX.",
      );
    }
    const text = await readFile(path, "utf8");
    let script = text;
    let astro: AstroNode | undefined;
    if (extname(path) === ".astro") {
      const parsed = await (
        await this.compiler()
      ).parse(text, { position: true });
      const error = parsed.diagnostics.find((d) => d.severity === 1);
      if (error) {
        throw new ConversionError(
          this.sourcePath(path),
          error.location?.line ?? 1,
          error.location?.column ?? 1,
          error.text,
        );
      }
      astro = parsed.ast;
      // Keep frontmatter line numbers aligned with the original file.
      script = (astro.children ?? [])
        .filter((n) => n.type === "frontmatter")
        .map((n) => "\n".repeat(n.position?.start.line ?? 1) + (n.value ?? ""))
        .join("\n");
    }
    const ast = ts.createSourceFile(
      path,
      script,
      ts.ScriptTarget.Latest,
      true,
      astro ? ts.ScriptKind.TS : ts.ScriptKind.TSX,
    );
    const source: Source = {
      path,
      text,
      ts: ast,
      astro,
      imports: new Map(),
      definitions: new Map(),
      exports: new Map(),
      globals: [],
    };
    this.sources.set(path, source);
    const diagnostics = (
      ast as ts.SourceFile & { parseDiagnostics: ts.DiagnosticWithLocation[] }
    ).parseDiagnostics;
    if (diagnostics.length) {
      const diagnostic = diagnostics[0]!;
      const p = ast.getLineAndCharacterOfPosition(diagnostic.start ?? 0);
      throw new ConversionError(
        this.sourcePath(path),
        p.line + 1,
        p.character + 1,
        ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n"),
      );
    }
    for (const statement of ast.statements) {
      if (ts.isImportDeclaration(statement)) {
        if (!ts.isStringLiteral(statement.moduleSpecifier))
          this.fail(source, statement, "Import inválido.");
        const from = statement.moduleSpecifier.text;
        const clause = statement.importClause;
        if (from.endsWith(".css") && from.startsWith(".")) {
          if (clause)
            this.fail(
              source,
              statement,
              "CSS Modules requieren un adaptador; importa CSS estático sin bindings.",
            );
          await this.stylesheet(resolve(dirname(source.path), from));
          continue;
        }
        if (!clause)
          this.fail(
            source,
            statement,
            `Import con efectos laterales no soportado: ${from}.`,
          );
        if (clause.isTypeOnly) continue;
        if (clause.name)
          source.imports.set(clause.name.text, { from, exported: "default" });
        if (clause.namedBindings) {
          if (!ts.isNamedImports(clause.namedBindings)) {
            this.fail(
              source,
              statement,
              "Usa imports nombrados; los namespaces no son convertibles.",
            );
          }
          for (const specifier of clause.namedBindings.elements) {
            if (!specifier.isTypeOnly) {
              source.imports.set(specifier.name.text, {
                from,
                exported: specifier.propertyName?.text ?? specifier.name.text,
              });
            }
          }
        }
      } else if (ts.isFunctionDeclaration(statement)) {
        const name = statement.name?.text ?? this.fileName(path);
        source.definitions.set(name, { name, node: statement });
        if (this.hasModifier(statement, ts.SyntaxKind.ExportKeyword))
          source.exports.set(name, name);
        if (this.hasModifier(statement, ts.SyntaxKind.DefaultKeyword))
          source.exports.set("default", name);
      } else if (ts.isVariableStatement(statement)) {
        if (!(statement.declarationList.flags & ts.NodeFlags.Const)) {
          this.fail(
            source,
            statement,
            "Solo se admiten constantes y useState; let/var exige lógica nativa.",
          );
        }
        for (const declaration of statement.declarationList.declarations) {
          const init =
            declaration.initializer && this.unwrap(declaration.initializer);
          if (
            ts.isIdentifier(declaration.name) &&
            init &&
            (ts.isArrowFunction(init) || ts.isFunctionExpression(init))
          ) {
            source.definitions.set(declaration.name.text, {
              name: declaration.name.text,
              node: init,
            });
            if (this.hasModifier(statement, ts.SyntaxKind.ExportKeyword)) {
              source.exports.set(declaration.name.text, declaration.name.text);
            }
          } else {
            source.globals.push(declaration);
          }
        }
      } else if (ts.isExportAssignment(statement)) {
        const expression = this.unwrap(statement.expression);
        if (ts.isIdentifier(expression))
          source.exports.set("default", expression.text);
        else if (
          ts.isArrowFunction(expression) ||
          ts.isFunctionExpression(expression)
        ) {
          const name = this.fileName(path);
          source.definitions.set(name, { name, node: expression });
          source.exports.set("default", name);
        } else
          this.fail(
            source,
            statement,
            "El export default debe ser un componente.",
          );
      } else if (ts.isExportDeclaration(statement)) {
        if (statement.isTypeOnly) continue;
        if (
          statement.moduleSpecifier ||
          !statement.exportClause ||
          !ts.isNamedExports(statement.exportClause)
        ) {
          this.fail(
            source,
            statement,
            "Reexportar componentes requiere importar su fuente explícitamente.",
          );
        }
        for (const item of statement.exportClause.elements) {
          source.exports.set(
            item.name.text,
            item.propertyName?.text ?? item.name.text,
          );
        }
      } else if (
        !ts.isInterfaceDeclaration(statement) &&
        !ts.isTypeAliasDeclaration(statement) &&
        !ts.isEmptyStatement(statement)
      ) {
        this.fail(
          source,
          statement,
          "Código imperativo o SSR no soportado: separa datos, autenticación y efectos en el host nativo.",
        );
      }
    }
    return source;
  }

  private fileName(path: string): string {
    return (
      basename(path, extname(path)).replace(/[^a-zA-Z0-9]/g, "_") || "Pagina"
    );
  }

  private async stylesheet(path: string): Promise<void> {
    if (this.styles.has(path)) return;
    const text = await readFile(path, "utf8");
    this.styles.set(path, text);
    this.hasStyles = true;
    // Preserve local stylesheet dependency hashes. This is dependency discovery,
    // not a CSS interpreter: rendering remains the browser's responsibility.
    for (const match of text.matchAll(
      /@import\s+(?:url\(\s*)?['"]([^'"]+)['"]/g,
    )) {
      if (match[1]!.startsWith("."))
        await this.stylesheet(resolve(dirname(path), match[1]!));
    }
  }

  private hasModifier(node: ts.Node, kind: ts.SyntaxKind): boolean {
    return (
      ts.canHaveModifiers(node) &&
      !!ts.getModifiers(node)?.some((m) => m.kind === kind)
    );
  }

  private async component(source: Source, exported: string): Promise<string> {
    const local = source.astro
      ? this.fileName(source.path)
      : source.exports.get(exported);
    if (!local)
      this.fail(source, source.ts, `El componente ${exported} no se exporta.`);
    const name = `${local}_${hash(this.sourcePath(source.path)).slice(0, 8)}`;
    if (this.active.has(name))
      this.fail(
        source,
        source.ts,
        `Import circular o componente recursivo: ${local}.`,
      );
    if (this.components.has(name)) return name;
    const component: Component = {
      name,
      props: [],
      states: [],
      constants: [],
      initializers: [],
      body: [],
    };
    const scope: Scope = {
      source,
      component,
      names: new Set(),
      bindings: new Set(
        source.globals.flatMap((declaration) =>
          this.bindingNames(declaration.name),
        ),
      ),
      listShadows: new Set(),
      hostBindings: new Set(),
      setters: new Map(),
      handlers: new Map(),
      substitutions: new Map(),
    };
    this.active.add(name);
    this.ids.set(name, new Set());
    if (source.astro) {
      // Astro.props is the only ambient Astro object permitted in a declarative
      // page. Astro.url, request, locals and SSR calls are intentionally rejected.
      for (const declaration of source.globals)
        this.declare(declaration, scope, true);
      for (const statement of source.ts.statements) {
        if (ts.isFunctionDeclaration(statement)) {
          this.fail(
            source,
            statement,
            "Las funciones del frontmatter requieren un binding nativo.",
          );
        }
      }
      component.body = await this.astroChildren(
        source.astro.children ?? [],
        scope,
      );
    } else {
      const definition = source.definitions.get(local);
      if (!definition)
        this.fail(
          source,
          source.ts,
          `${local} no es una función o arrow component.`,
        );
      if (this.hasModifier(definition.node, ts.SyntaxKind.AsyncKeyword)) {
        this.fail(
          source,
          definition.node,
          "Los componentes async/SSR requieren datos del host nativo.",
        );
      }
      for (const declaration of source.globals)
        this.declare(declaration, scope, true);
      this.props(definition.node.parameters, scope);
      const body = definition.node.body;
      if (!body) this.fail(source, definition.node, "Componente sin cuerpo.");
      if (ts.isBlock(body)) {
        for (const statement of body.statements)
          if (ts.isVariableStatement(statement))
            for (const declaration of statement.declarationList.declarations)
              for (const binding of this.bindingNames(declaration.name))
                scope.bindings.add(binding);
        let returned = false;
        for (const statement of body.statements) {
          if (returned)
            this.fail(
              source,
              statement,
              "Código después de return no soportado.",
            );
          if (ts.isVariableStatement(statement)) {
            if (!(statement.declarationList.flags & ts.NodeFlags.Const)) {
              this.fail(
                source,
                statement,
                "Usa const o useState; las mutaciones imperativas no son convertibles.",
              );
            }
            for (const declaration of statement.declarationList.declarations) {
              this.declare(declaration, scope);
            }
          } else if (ts.isReturnStatement(statement) && statement.expression) {
            component.body = await this.render(statement.expression, scope);
            returned = true;
          } else {
            this.fail(
              source,
              statement,
              "El cuerpo admite constantes, useState y return JSX; otros efectos requieren bindings nativos.",
            );
          }
        }
        if (!returned)
          this.fail(source, body, "Componente sin return declarativo.");
      } else component.body = await this.render(body, scope);
    }
    this.components.set(name, component);
    this.active.delete(name);
    return name;
  }

  private props(
    parameters: ts.NodeArray<ts.ParameterDeclaration>,
    scope: Scope,
  ): void {
    if (parameters.length > 1)
      this.fail(
        scope.source,
        parameters[1]!,
        "Un componente solo recibe props.",
      );
    const parameter = parameters[0];
    if (!parameter) return;
    if (parameter.dotDotDotToken || parameter.initializer) {
      this.fail(
        scope.source,
        parameter,
        "Parámetro rest o props con valor por defecto no soportado.",
      );
    }
    if (ts.isIdentifier(parameter.name)) {
      scope.names.add(parameter.name.text);
      scope.component.propsObject = parameter.name.text;
      return;
    }
    if (!ts.isObjectBindingPattern(parameter.name)) {
      this.fail(
        scope.source,
        parameter,
        "Las props deben ser un objeto o un destructuring de objeto.",
      );
    }
    for (const binding of parameter.name.elements) {
      if (
        binding.dotDotDotToken ||
        binding.propertyName ||
        binding.initializer ||
        !ts.isIdentifier(binding.name)
      ) {
        this.fail(
          scope.source,
          binding,
          "Props renombradas, rest o defaults requieren un binding explícito.",
        );
      }
      scope.component.props.push(binding.name.text);
      scope.names.add(binding.name.text);
      scope.hostBindings.add(binding.name.text);
    }
  }

  private bindingNames(name: ts.BindingName): string[] {
    if (ts.isIdentifier(name)) return [name.text];
    return name.elements.flatMap((element) =>
      ts.isBindingElement(element) ? this.bindingNames(element.name) : [],
    );
  }

  private declare(
    declaration: ts.VariableDeclaration,
    scope: Scope,
    global = false,
  ): void {
    if (!declaration.initializer)
      this.fail(scope.source, declaration, "Constante sin valor.", scope);
    const expression = this.unwrap(declaration.initializer);
    if (ts.isArrayBindingPattern(declaration.name)) {
      const elements = declaration.name.elements;
      if (
        global ||
        elements.length !== 2 ||
        !ts.isBindingElement(elements[0]!) ||
        !ts.isBindingElement(elements[1]!) ||
        !ts.isIdentifier(elements[0]!.name) ||
        !ts.isIdentifier(elements[1]!.name) ||
        elements.some(
          (e) =>
            ts.isBindingElement(e) &&
            (e.initializer || e.dotDotDotToken || e.propertyName),
        )
      ) {
        this.fail(
          scope.source,
          declaration,
          "Solo se admite el tuple [estado, setter] de useState.",
          scope,
        );
      }
      if (
        !ts.isCallExpression(expression) ||
        !ts.isIdentifier(expression.expression) ||
        scope.source.imports.get(expression.expression.text)?.exported !==
          "useState" ||
        !["preact/hooks", "preact/compat"].includes(
          scope.source.imports.get(expression.expression.text)!.from,
        ) ||
        expression.arguments.length !== 1
      ) {
        this.fail(
          scope.source,
          declaration,
          "Estado no soportado: usa useState con un valor declarativo.",
          scope,
        );
      }
      const name = elements[0]!.name.text;
      const setter = elements[1]!.name.text;
      const initial = this.expr(expression.arguments[0]!, scope);
      scope.component.states.push({ name, setter, initial });
      scope.component.initializers.push({ kind: "state", name });
      scope.names.add(name);
      scope.setters.set(setter, name);
      scope.hostBindings.delete(name);
      scope.hostBindings.delete(setter);
      return;
    }
    if (
      ts.isObjectBindingPattern(declaration.name) &&
      scope.source.astro &&
      ts.isPropertyAccessExpression(expression) &&
      expression.expression.getText() === "Astro" &&
      expression.name.text === "props"
    ) {
      this.props(
        ts.factory.createNodeArray([
          ts.factory.createParameterDeclaration(
            undefined,
            undefined,
            declaration.name,
          ),
        ]),
        scope,
      );
      return;
    }
    if (!ts.isIdentifier(declaration.name)) {
      this.fail(
        scope.source,
        declaration,
        "Destructuring no soportado fuera de props y useState.",
        scope,
      );
    }
    const name = declaration.name.text;
    if (ts.isArrowFunction(expression) || ts.isFunctionExpression(expression)) {
      if (global)
        this.fail(
          scope.source,
          declaration,
          "Una función global requiere un binding nativo.",
          scope,
        );
      scope.handlers.set(name, expression);
      scope.hostBindings.delete(name);
      return;
    }
    if (
      scope.source.astro &&
      name === "prerender" &&
      ts.isLiteralExpression(expression)
    )
      return;
    if (
      scope.source.astro &&
      name === "prerender" &&
      [ts.SyntaxKind.TrueKeyword, ts.SyntaxKind.FalseKeyword].includes(
        expression.kind,
      )
    )
      return;
    const value = this.expr(expression, scope);
    if (global && this.hasHostCall(value))
      this.fail(
        scope.source,
        declaration,
        "Una constante de módulo con llamadas al host requiere un ciclo de vida de inicialización nativo; no se reejecuta al renderizar en stage 01.",
        scope,
      );
    scope.component.constants.push({ name, value });
    scope.component.initializers.push({ kind: "constant", name });
    scope.names.add(name);
    scope.hostBindings.delete(name);
  }

  private hasHostCall(value: Expr): boolean {
    switch (value.kind) {
      case "call":
        return (
          !["String", "Number", "Boolean"].includes(value.name) ||
          value.args.some((argument) => this.hasHostCall(argument))
        );
      case "get":
        return this.hasHostCall(value.object) || this.hasHostCall(value.key);
      case "binary":
        return this.hasHostCall(value.left) || this.hasHostCall(value.right);
      case "unary":
        return this.hasHostCall(value.value);
      case "conditional":
        return (
          this.hasHostCall(value.test) ||
          this.hasHostCall(value.yes) ||
          this.hasHostCall(value.no)
        );
      case "array":
        return value.items.some((item) => this.hasHostCall(item));
      case "object":
        return Object.values(value.entries).some((item) =>
          this.hasHostCall(item),
        );
      case "template":
        return value.parts.some((part) => this.hasHostCall(part));
      default:
        return false;
    }
  }

  private isHostAction(name: string, scope: Scope): boolean {
    if (scope.hostBindings.has(name)) return true;
    return (
      name === "t" &&
      !scope.names.has(name) &&
      !scope.bindings.has(name) &&
      !scope.handlers.has(name) &&
      !scope.setters.has(name) &&
      !scope.source.definitions.has(name) &&
      !scope.substitutions.has(name)
    );
  }

  private unwrap(expression: ts.Expression): ts.Expression {
    while (
      ts.isParenthesizedExpression(expression) ||
      ts.isAsExpression(expression) ||
      ts.isTypeAssertionExpression(expression) ||
      ts.isNonNullExpression(expression) ||
      ts.isSatisfiesExpression(expression)
    )
      expression = expression.expression;
    return expression;
  }

  private isAmbientUndefined(node: ts.Node, scope: Scope): boolean {
    if (!ts.isIdentifier(node) || node.text !== "undefined") return false;
    if (
      scope.names.has(node.text) ||
      scope.bindings.has(node.text) ||
      scope.substitutions.has(node.text) ||
      scope.handlers.has(node.text) ||
      scope.setters.has(node.text) ||
      scope.source.imports.has(node.text) ||
      scope.source.definitions.has(node.text)
    )
      this.fail(
        scope.source,
        node,
        "undefined sombreado por un binding léxico; renómbralo o proporciona un adaptador nativo explícito.",
        scope,
      );
    return true;
  }

  private expr(expression: ts.Expression, scope: Scope): Expr {
    const node = this.unwrap(expression);
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node))
      return literal(node.text);
    if (ts.isNumericLiteral(node)) {
      const value = Number(node.text);
      if (!Number.isFinite(value))
        this.fail(
          scope.source,
          node,
          "Un literal numérico debe ser finito; Infinity no es convertible a Go.",
          scope,
        );
      return literal(value);
    }
    if (node.kind === ts.SyntaxKind.TrueKeyword) return literal(true);
    if (node.kind === ts.SyntaxKind.FalseKeyword) return literal(false);
    if (node.kind === ts.SyntaxKind.NullKeyword) return literal(null);
    if (ts.isIdentifier(node)) {
      if (this.isAmbientUndefined(node, scope)) return { kind: "undefined" };
      const substituted = scope.substitutions.get(node.text);
      if (substituted) return substituted;
      if (!scope.names.has(node.text)) {
        this.fail(
          scope.source,
          node,
          `Binding desconocido: ${node.text}. Decláralo en props o en const.`,
          scope,
        );
      }
      return { kind: "name", name: node.text };
    }
    if (ts.isPropertyAccessExpression(node)) {
      if (node.questionDotToken)
        this.fail(
          scope.source,
          node,
          "El acceso opcional requiere un binding explícito en stage 01.",
          scope,
        );
      if (
        ts.isIdentifier(node.expression) &&
        node.expression.text === "Astro" &&
        node.name.text !== "props"
      ) {
        this.fail(
          scope.source,
          node,
          `Astro.${node.name.text} depende de SSR; proporciona datos por props.`,
          scope,
        );
      }
      if (
        ts.isIdentifier(node.expression) &&
        node.expression.text === "Astro" &&
        node.name.text === "props"
      ) {
        scope.component.propsObject ??= "__astroProps";
        scope.names.add(scope.component.propsObject);
        return { kind: "name", name: scope.component.propsObject };
      }
      return {
        kind: "get",
        object: this.expr(node.expression, scope),
        key: literal(node.name.text),
      };
    }
    if (ts.isElementAccessExpression(node)) {
      if (node.questionDotToken)
        this.fail(
          scope.source,
          node,
          "El acceso opcional requiere un binding explícito en stage 01.",
          scope,
        );
      return {
        kind: "get",
        object: this.expr(node.expression, scope),
        key: this.expr(node.argumentExpression, scope),
      };
    }
    if (ts.isBinaryExpression(node)) {
      const op = node.operatorToken.getText();
      if (!BINARY.has(op))
        this.fail(scope.source, node, `Operador no soportado: ${op}.`, scope);
      return {
        kind: "binary",
        op,
        left: this.expr(node.left, scope),
        right: this.expr(node.right, scope),
      };
    }
    if (ts.isPrefixUnaryExpression(node)) {
      const op = ts.tokenToString(node.operator);
      if (!op || !["!", "+", "-"].includes(op))
        this.fail(
          scope.source,
          node,
          "Mutación u operador unario no soportado.",
          scope,
        );
      return { kind: "unary", op, value: this.expr(node.operand, scope) };
    }
    if (ts.isConditionalExpression(node)) {
      return {
        kind: "conditional",
        test: this.expr(node.condition, scope),
        yes: this.expr(node.whenTrue, scope),
        no: this.expr(node.whenFalse, scope),
      };
    }
    if (ts.isArrayLiteralExpression(node)) {
      if (
        node.elements.some(
          (e) => ts.isSpreadElement(e) || ts.isOmittedExpression(e),
        )
      ) {
        this.fail(
          scope.source,
          node,
          "Arrays dispersos o con spread no soportados.",
          scope,
        );
      }
      return {
        kind: "array",
        items: node.elements.map((e) => this.expr(e, scope)),
      };
    }
    if (ts.isObjectLiteralExpression(node)) {
      const entries: Record<string, Expr> = {};
      for (const property of node.properties) {
        if (ts.isShorthandPropertyAssignment(property)) {
          entries[property.name.text] = this.expr(property.name, scope);
        } else if (
          ts.isPropertyAssignment(property) &&
          (ts.isIdentifier(property.name) ||
            ts.isStringLiteral(property.name) ||
            ts.isNumericLiteral(property.name))
        ) {
          entries[property.name.text] = this.expr(property.initializer, scope);
        } else
          this.fail(
            scope.source,
            property,
            "Propiedad dinámica, método o spread no soportado.",
            scope,
          );
      }
      return { kind: "object", entries };
    }
    if (ts.isTemplateExpression(node)) {
      const parts: Expr[] = [literal(node.head.text)];
      for (const span of node.templateSpans)
        parts.push(
          this.expr(span.expression, scope),
          literal(span.literal.text),
        );
      return { kind: "template", parts };
    }
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      PURE_CALLS.has(node.expression.text)
    ) {
      const name = node.expression.text;
      const shadowed =
        scope.names.has(name) ||
        scope.bindings.has(name) ||
        scope.handlers.has(name) ||
        scope.setters.has(name) ||
        scope.source.definitions.has(name) ||
        scope.source.imports.has(name) ||
        scope.substitutions.has(name);
      if (name !== "t" && shadowed)
        this.fail(
          scope.source,
          node,
          `Builtin ${name} sombreado por un binding de la fuente; su llamada requiere un adaptador nativo explícito.`,
          scope,
        );
      if (
        name === "t" &&
        (((scope.names.has(name) || scope.bindings.has(name)) &&
          !scope.hostBindings.has(name)) ||
          scope.handlers.has(name) ||
          scope.setters.has(name) ||
          scope.source.definitions.has(name) ||
          scope.substitutions.has(name))
      )
        this.fail(
          scope.source,
          node,
          "t sombreado por un binding local; la traducción requiere el binding host explícito.",
          scope,
        );
      if (node.questionDotToken || node.arguments.some(ts.isSpreadElement)) {
        this.fail(
          scope.source,
          node,
          "Llamadas opcionales o con spread no soportadas.",
          scope,
        );
      }
      if (node.expression.text !== "t" && node.arguments.length !== 1) {
        this.fail(
          scope.source,
          node,
          `${node.expression.text} necesita exactamente un argumento.`,
          scope,
        );
      }
      if (node.expression.text === "t") this.actions.add("t");
      return {
        kind: "call",
        name: node.expression.text,
        args: node.arguments.map((a) => this.expr(a, scope)),
      };
    }
    this.fail(
      scope.source,
      node,
      `Expresión ${ts.SyntaxKind[node.kind]} no convertible; usa un binding nativo para hooks, DOM, red o funciones.`,
      scope,
    );
  }

  private async render(
    expression: ts.Expression,
    scope: Scope,
  ): Promise<Node[]> {
    const node = this.unwrap(expression);
    if (
      node.kind === ts.SyntaxKind.NullKeyword ||
      node.kind === ts.SyntaxKind.FalseKeyword ||
      node.kind === ts.SyntaxKind.TrueKeyword ||
      this.isAmbientUndefined(node, scope)
    )
      return [];
    if (ts.isJsxElement(node))
      return [await this.jsx(node.openingElement, node.children, scope)];
    if (ts.isJsxSelfClosingElement(node))
      return [await this.jsx(node, [], scope)];
    if (ts.isJsxFragment(node)) return this.jsxChildren(node.children, scope);
    if (ts.isConditionalExpression(node)) {
      return [
        {
          kind: "conditional",
          test: this.expr(node.condition, scope),
          yes: await this.render(node.whenTrue, scope),
          no: await this.render(node.whenFalse, scope),
        },
      ];
    }
    if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
    ) {
      return [
        {
          kind: "conditional",
          test: this.expr(node.left, scope),
          yes: await this.render(node.right, scope),
          no: [{ kind: "text", value: this.expr(node.left, scope) }],
        },
      ];
    }
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === "map"
    ) {
      if (
        node.arguments.length !== 1 ||
        !ts.isArrowFunction(node.arguments[0]!)
      ) {
        this.fail(
          scope.source,
          node,
          "map solo admite un callback arrow declarativo.",
          scope,
        );
      }
      const callback = node.arguments[0]!;
      if (this.hasModifier(callback, ts.SyntaxKind.AsyncKeyword))
        this.fail(
          scope.source,
          callback,
          "map async devuelve promesas; requiere un ciclo de vida asíncrono nativo explícito.",
          scope,
        );
      if (
        callback.parameters.length < 1 ||
        callback.parameters.length > 2 ||
        callback.parameters.some(
          (p) => !ts.isIdentifier(p.name) || p.initializer || p.dotDotDotToken,
        )
      ) {
        this.fail(
          scope.source,
          callback,
          "map necesita item y opcionalmente index, sin destructuring.",
          scope,
        );
      }
      const item = (callback.parameters[0]!.name as ts.Identifier).text;
      const index = (callback.parameters[1]?.name as ts.Identifier | undefined)
        ?.text;
      const childScope = {
        ...scope,
        names: new Set(scope.names),
        hostBindings: new Set(scope.hostBindings),
        setters: new Map(scope.setters),
        handlers: new Map(scope.handlers),
        listShadows: new Set(scope.listShadows),
      };
      for (const parameter of index ? [item, index] : [item]) {
        if (
          scope.bindings.has(parameter) ||
          scope.component.props.includes(parameter) ||
          scope.component.propsObject === parameter ||
          scope.source.imports.has(parameter) ||
          scope.source.definitions.has(parameter)
        )
          childScope.listShadows.add(parameter);
        childScope.names.add(parameter);
        childScope.hostBindings.delete(parameter);
        childScope.setters.delete(parameter);
        childScope.handlers.delete(parameter);
      }
      let body: ts.Expression;
      if (ts.isBlock(callback.body)) {
        if (
          callback.body.statements.length !== 1 ||
          !ts.isReturnStatement(callback.body.statements[0]!) ||
          !callback.body.statements[0]!.expression
        ) {
          this.fail(
            scope.source,
            callback,
            "El callback de map solo puede devolver JSX.",
            scope,
          );
        }
        body = callback.body.statements[0]!.expression;
      } else body = callback.body;
      return [
        {
          kind: "each",
          items: this.expr(node.expression.expression, scope),
          item,
          index,
          children: await this.render(body, childScope),
        },
      ];
    }
    return [{ kind: "text", value: this.expr(node, scope) }];
  }

  private async jsxChildren(
    children: readonly ts.JsxChild[],
    scope: Scope,
  ): Promise<Node[]> {
    const nodes: Node[] = [];
    for (const child of children) {
      if (ts.isJsxText(child)) {
        const text = this.decodeJSX(this.jsxText(child.text), scope, child);
        if (text) nodes.push({ kind: "text", value: literal(text) });
      } else if (ts.isJsxExpression(child)) {
        if (child.dotDotDotToken)
          this.fail(
            scope.source,
            child,
            "Spread de hijos no soportado.",
            scope,
          );
        if (child.expression)
          nodes.push(...(await this.render(child.expression, scope)));
      } else nodes.push(...(await this.render(child, scope)));
    }
    return nodes;
  }

  private jsxText(text: string): string {
    // JSX trims indentation and blank lines, but preserves meaningful spaces on
    // a line. Do not trim each token: <span>Hello </span>{name} needs that space.
    const lines = text.replaceAll("\r", "").split("\n");
    return lines
      .map((line, index) => {
        let value = line.replaceAll("\t", " ");
        if (index !== 0) value = value.replace(/^ +/, "");
        if (index !== lines.length - 1) value = value.replace(/ +$/, "");
        return value;
      })
      .filter((line) => line.length > 0)
      .join(" ");
  }

  private decodeJSX(text: string, scope: Scope, node: ts.Node): string {
    // JSX follows its HTML4 reference table and numeric rules, not HTML5's
    // replacement-character/Windows-1252 recovery. Reuse our existing public
    // TypeScript dependency as the oracle; the emitted JavaScript is only parsed.
    // This also preserves unknown and semicolonless references without a copied
    // entity table or evaluating any JavaScript from the user's source.
    return text.replace(
      /&(?:#[xX][\da-fA-F]+|#\d+|[A-Za-z]\w*);/g,
      (reference) => {
        const cached = this.jsxReferences.get(reference);
        if (cached !== undefined) return cached;
        const compiled = ts.transpileModule(
          `const value = <p>${reference}</p>;`,
          {
            fileName: "entity.tsx",
            compilerOptions: {
              target: ts.ScriptTarget.ESNext,
              module: ts.ModuleKind.ESNext,
              jsx: ts.JsxEmit.React,
              jsxFactory: "__astroFyneEntity",
            },
          },
        );
        const output = ts.createSourceFile(
          "entity.js",
          compiled.outputText,
          ts.ScriptTarget.ESNext,
          true,
          ts.ScriptKind.JS,
        );
        const value = output.statements.find(ts.isVariableStatement)
          ?.declarationList.declarations[0]?.initializer;
        if (!value || !ts.isCallExpression(value))
          this.fail(
            scope.source,
            node,
            `No se puede decodificar la referencia JSX ${reference}.`,
            scope,
          );
        const decoded = value.arguments[2];
        if (!decoded || !ts.isStringLiteral(decoded))
          this.fail(
            scope.source,
            node,
            `TypeScript no produjo una cadena para ${reference}.`,
            scope,
          );
        this.jsxReferences.set(reference, decoded.text);
        return decoded.text;
      },
    );
  }

  private async jsx(
    opening: ts.JsxOpeningElement | ts.JsxSelfClosingElement,
    children: readonly ts.JsxChild[],
    scope: Scope,
  ): Promise<Node> {
    const tag = opening.tagName.getText();
    const attrs: Record<string, Expr> = {};
    const events: Record<string, Handler> = {};
    for (const attribute of opening.attributes.properties) {
      if (!ts.isJsxAttribute(attribute))
        this.fail(
          scope.source,
          attribute,
          "Spread de atributos no soportado.",
          scope,
        );
      const name = attribute.name.getText();
      if (name === "key")
        this.fail(
          scope.source,
          attribute,
          "key requiere identidad y ciclo de vida de componentes; no se aproxima en stage 01.",
          scope,
        );
      if (Object.hasOwn(attrs, name) || Object.hasOwn(events, name)) {
        this.fail(
          scope.source,
          attribute,
          `Atributo duplicado: ${name}.`,
          scope,
        );
      }
      if (/^on[A-Z]/.test(name)) {
        if (
          !attribute.initializer ||
          !ts.isJsxExpression(attribute.initializer) ||
          !attribute.initializer.expression
        ) {
          this.fail(
            scope.source,
            attribute,
            "Un evento necesita un handler declarativo.",
            scope,
          );
        }
        events[name] = this.handler(attribute.initializer.expression, scope);
      } else if (/^on[a-z]/.test(name)) {
        this.fail(
          scope.source,
          attribute,
          "Usa eventos Preact (onClick, onInput); scripts HTML no son convertibles.",
          scope,
        );
      } else {
        if (name === "dangerouslySetInnerHTML" || name === "ref") {
          this.fail(
            scope.source,
            attribute,
            `${name} depende del DOM y requiere un widget nativo.`,
            scope,
          );
        }
        if (!attribute.initializer) attrs[name] = literal(true);
        else if (ts.isStringLiteral(attribute.initializer))
          attrs[name] = literal(
            this.decodeJSX(attribute.initializer.text, scope, attribute),
          );
        else if (
          ts.isJsxExpression(attribute.initializer) &&
          attribute.initializer.expression
        ) {
          attrs[name] = this.expr(attribute.initializer.expression, scope);
        } else
          this.fail(scope.source, attribute, `Atributo vacío: ${name}.`, scope);
      }
    }
    return this.element(
      tag,
      attrs,
      events,
      await this.jsxChildren(children, scope),
      scope,
      opening,
    );
  }

  private async element(
    tag: string,
    attrs: Record<string, Expr>,
    events: Record<string, Handler>,
    children: Node[],
    scope: Scope,
    node: ts.Node,
  ): Promise<Node> {
    if (TAGS.has(tag)) {
      let id: string;
      const declared = attrs.id;
      if (declared?.kind === "literal" && typeof declared.value === "string") {
        id = declared.value;
        if (!id)
          this.fail(
            scope.source,
            node,
            "Un id explícito no puede estar vacío.",
            scope,
          );
        const used = this.ids.get(scope.component.name)!;
        if (used.has(id))
          this.fail(scope.source, node, `Id duplicado: ${id}.`, scope);
        used.add(id);
      } else id = `${scope.component.name}_n${++this.nextID}`;
      let imageResource: string | undefined;
      if (tag === "img") {
        const src = attrs.src;
        if (src?.kind !== "literal" || typeof src.value !== "string")
          this.fail(
            scope.source,
            node,
            "img src necesita una ruta local literal PNG/JPEG.",
            scope,
          );
        if (children.length || Object.keys(events).length)
          this.fail(
            scope.source,
            node,
            "img no admite hijos ni eventos en stage 01.",
            scope,
          );
        for (const attribute of [
          "srcSet",
          "srcset",
          "sizes",
          "useMap",
          "usemap",
          "isMap",
          "ismap",
          "crossOrigin",
          "crossorigin",
          "referrerPolicy",
          "referrerpolicy",
          "loading",
          "decoding",
          "fetchpriority",
          "fetchPriority",
        ]) {
          if (attrs[attribute])
            this.fail(
              scope.source,
              node,
              `img ${attribute} requiere un contrato nativo explícito.`,
              scope,
            );
        }
        try {
          const bitmap = await loadBitmap(
            src.value,
            scope.source.path,
            this.root,
            this.options.publicDir
              ? resolve(this.root, this.options.publicDir)
              : undefined,
            this.entrySource,
          );
          const previous = this.resources.get(bitmap.path);
          if (previous) {
            if (previous.hash !== bitmap.hash)
              throw new Error(
                `El recurso bitmap cambió durante la conversión: ${bitmap.path}`,
              );
            if (!previous.srcs.includes(src.value))
              previous.srcs.push(src.value);
            previous.srcs.sort(compare);
            imageResource = previous.name;
          } else {
            this.resources.set(bitmap.path, bitmap);
            imageResource = bitmap.name;
          }
        } catch (error) {
          this.fail(
            scope.source,
            node,
            error instanceof Error ? error.message : String(error),
            scope,
          );
        }
      }
      return {
        kind: "element",
        id,
        tag,
        attrs,
        events,
        children,
        ...(imageResource ? { imageResource } : {}),
      };
    }
    const imported = scope.source.imports.get(tag);
    const adapted =
      imported && this.options.adapters?.[imported.from]?.[imported.exported];
    if (adapted && BUILTINS.has(adapted)) {
      // Keep builtins as component nodes; event handlers remain on an equivalent
      // semantic element so the IR never needs to encode functions as props.
      const nativeTag =
        adapted === "Button"
          ? "button"
          : adapted === "Input"
            ? "input"
            : adapted === "CardTitle"
              ? "h3"
              : "div";
      if (Object.keys(events).length) {
        attrs["data-native-component"] = literal(adapted);
        return this.element(nativeTag, attrs, events, children, scope, node);
      }
      return {
        kind: "component",
        id: `${scope.component.name}_c${++this.nextID}`,
        name: `$ui.${adapted}`,
        props: attrs,
        children,
      };
    }
    if (
      imported &&
      !imported.from.startsWith(".") &&
      !isAbsolute(imported.from)
    ) {
      this.fail(
        scope.source,
        node,
        `Componente externo sin adaptador: ${imported.from}/${imported.exported}.`,
        scope,
      );
    }
    if (Object.keys(events).length) {
      this.fail(
        scope.source,
        node,
        "Pasar callbacks a componentes requiere un binding nativo explícito.",
        scope,
      );
    }
    if (tag.includes(".") || /^[a-z]/.test(tag)) {
      this.fail(
        scope.source,
        node,
        `Elemento o componente no soportado: ${tag}.`,
        scope,
      );
    }
    let source = scope.source;
    let exported = tag;
    if (imported) {
      source = await this.relativeSource(imported.from, scope);
      exported = imported.exported;
    } else if (source.definitions.has(tag)) {
      // A local component does not have to be exported; temporarily resolve its
      // lexical name without changing the source or rewriting imports.
      source.exports.set(tag, tag);
    } else
      this.fail(scope.source, node, `Componente desconocido: ${tag}.`, scope);
    if (children.length)
      this.fail(
        scope.source,
        node,
        "Hijos de componentes requieren un contrato nativo explícito; no se omiten durante la conversión.",
        scope,
      );
    const id = `${scope.component.name}_c${++this.nextID}`;
    return {
      kind: "component",
      id,
      name: await this.component(source, exported),
      props: attrs,
      children,
    };
  }

  private async relativeSource(
    specifier: string,
    scope: Scope,
  ): Promise<Source> {
    const base = resolve(dirname(scope.source.path), specifier);
    const extension = extname(base);
    const candidates = extension
      ? [
          base,
          ...([".js", ".jsx"].includes(extension)
            ? [
                base.slice(0, -extension.length) + ".tsx",
                base.slice(0, -extension.length) + ".ts",
              ]
            : []),
        ]
      : [
          base + ".tsx",
          base + ".astro",
          base + ".jsx",
          base + ".ts",
          base + ".js",
          resolve(base, "index.tsx"),
          resolve(base, "index.astro"),
        ];
    for (const candidate of candidates) {
      try {
        await readFile(candidate, "utf8");
      } catch {
        continue;
      }
      return this.source(candidate);
    }
    this.fail(
      scope.source,
      scope.source.ts,
      `No se encuentra la fuente del import ${specifier}.`,
      scope,
    );
  }

  private handler(expression: ts.Expression, scope: Scope): Handler {
    let node = this.unwrap(expression);
    if (ts.isIdentifier(node)) {
      const known = scope.handlers.get(node.text);
      if (known) {
        if (scope.listShadows.size)
          this.fail(
            scope.source,
            node,
            `El handler nombrado ${node.text} se usa bajo map con bindings externos sombreados (${[...scope.listShadows].sort().join(", ")}); requiere capturas lexicales cualificadas.`,
            scope,
          );
        node = known;
      } else if (scope.setters.has(node.text)) {
        this.fail(
          scope.source,
          node,
          "Envuelve el setter en e => setter(e.currentTarget.value).",
          scope,
        );
      } else if (this.isHostAction(node.text, scope)) {
        this.actions.add(node.text);
        return { steps: [{ kind: "call", name: node.text, args: [] }] };
      } else if (scope.names.has(node.text)) {
        this.fail(
          scope.source,
          node,
          `Handler ${node.text} no declarado en props; estados, constantes y bindings locales no autorizan acciones host.`,
          scope,
        );
      } else
        this.fail(
          scope.source,
          node,
          `Handler desconocido: ${node.text}.`,
          scope,
        );
    }
    if (!ts.isArrowFunction(node) && !ts.isFunctionExpression(node)) {
      this.fail(
        scope.source,
        node,
        "El handler debe ser una arrow o callback declarado.",
        scope,
      );
    }
    if (
      node.parameters.length > 1 ||
      node.parameters.some(
        (p) => !ts.isIdentifier(p.name) || p.initializer || p.dotDotDotToken,
      ) ||
      this.hasModifier(node, ts.SyntaxKind.AsyncKeyword)
    ) {
      this.fail(
        scope.source,
        node,
        "Handler async o con parámetros complejos no soportado.",
        scope,
      );
    }
    const parameter = (node.parameters[0]?.name as ts.Identifier | undefined)
      ?.text;
    if (parameter && scope.setters.has(parameter))
      this.fail(
        scope.source,
        node,
        `El parámetro de evento ${parameter} sombrea un setter; requiere un binding nativo explícito.`,
        scope,
      );
    const eventScope = {
      ...scope,
      names: new Set(scope.names),
      hostBindings: new Set(scope.hostBindings),
    };
    if (parameter) eventScope.names.add(parameter);
    if (parameter) eventScope.hostBindings.delete(parameter);
    const expressions: ts.Expression[] = [];
    if (ts.isBlock(node.body)) {
      let returned = false;
      for (const statement of node.body.statements) {
        if (returned)
          this.fail(
            scope.source,
            statement,
            "Código después de return en un handler no soportado.",
            scope,
          );
        if (ts.isExpressionStatement(statement))
          expressions.push(statement.expression);
        else if (ts.isReturnStatement(statement) && statement.expression) {
          expressions.push(statement.expression);
          returned = true;
        } else
          this.fail(
            scope.source,
            statement,
            "El handler solo puede llamar a acciones o setters.",
            scope,
          );
      }
    } else expressions.push(node.body);
    const steps: Handler["steps"] = [];
    for (const expression of expressions) {
      const call = this.unwrap(expression);
      if (
        !ts.isCallExpression(call) ||
        !ts.isIdentifier(call.expression) ||
        call.questionDotToken ||
        call.arguments.some(ts.isSpreadElement)
      ) {
        this.fail(
          scope.source,
          call,
          "El handler solo puede llamar a acciones o setters nombrados.",
          scope,
        );
      }
      const name = call.expression.text;
      const state = scope.setters.get(name);
      if (state) {
        if (call.arguments.length !== 1)
          this.fail(scope.source, call, "Un setter necesita un valor.", scope);
        let value = this.unwrap(call.arguments[0]!);
        let setterScope = eventScope;
        let updater: true | undefined;
        if (ts.isArrowFunction(value)) {
          if (
            value.parameters.length !== 1 ||
            !ts.isIdentifier(value.parameters[0]!.name) ||
            value.parameters[0]!.initializer ||
            value.parameters[0]!.dotDotDotToken ||
            ts.isBlock(value.body) ||
            this.hasModifier(value, ts.SyntaxKind.AsyncKeyword)
          ) {
            this.fail(
              scope.source,
              value,
              "El updater del setter debe ser valor => expresión.",
              scope,
            );
          }
          setterScope = {
            ...eventScope,
            substitutions: new Map(eventScope.substitutions),
            hostBindings: new Set(eventScope.hostBindings),
          };
          setterScope.hostBindings.delete(value.parameters[0]!.name.text);
          setterScope.substitutions.set(value.parameters[0]!.name.text, {
            kind: "current",
            name: state,
          });
          value = this.unwrap(value.body);
          updater = true;
        }
        steps.push({
          kind: "set",
          name: state,
          args: [this.expr(value, setterScope)],
          ...(updater ? { updater } : {}),
        });
      } else {
        if (!this.isHostAction(name, eventScope)) {
          this.fail(
            scope.source,
            call,
            `Acción ${name} no declarada en props.`,
            scope,
          );
        }
        this.actions.add(name);
        steps.push({
          kind: "call",
          name,
          args: call.arguments.map((a) => this.expr(a, eventScope)),
        });
      }
    }
    return { parameter, steps };
  }

  private parseExpression(text: string, scope: Scope): ts.Expression {
    const script = ts.createSourceFile(
      scope.source.path,
      `const __value = (${text});`,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TSX,
    );
    const diagnostics = (
      script as ts.SourceFile & {
        parseDiagnostics: ts.DiagnosticWithLocation[];
      }
    ).parseDiagnostics;
    if (diagnostics.length)
      this.fail(
        scope.source,
        scope.source.ts,
        ts.flattenDiagnosticMessageText(diagnostics[0]!.messageText, "\n"),
        scope,
      );
    const statement = script.statements[0];
    if (
      !statement ||
      !ts.isVariableStatement(statement) ||
      !statement.declarationList.declarations[0]?.initializer
    ) {
      this.fail(
        scope.source,
        scope.source.ts,
        "Expresión Astro inválida.",
        scope,
      );
    }
    return statement.declarationList.declarations[0]!.initializer!;
  }

  private async astroChildren(
    children: AstroNode[],
    scope: Scope,
  ): Promise<Node[]> {
    const nodes: Node[] = [];
    for (const child of children) {
      if (["frontmatter", "comment", "doctype"].includes(child.type)) continue;
      const located = {
        ...scope,
        location: child.position?.start ?? scope.location,
      };
      if (child.type === "element" && child.name === "style") {
        this.hasStyles = true;
        continue;
      }
      if (child.type === "text") {
        const text = decodeHTML(this.jsxText(child.value ?? ""));
        if (text) nodes.push({ kind: "text", value: literal(text) });
      } else if (child.type === "expression") {
        nodes.push(
          ...(await this.render(
            this.parseExpression(
              (child.children ?? []).map((n) => this.serialize(n)).join(""),
              located,
            ),
            located,
          )),
        );
      } else if (child.type === "fragment") {
        nodes.push(
          ...(await this.astroChildren(child.children ?? [], located)),
        );
      } else if (
        ["element", "component", "custom-element"].includes(child.type)
      ) {
        const attrs: Record<string, Expr> = {};
        const events: Record<string, Handler> = {};
        for (const attribute of child.attributes ?? []) {
          const name = attribute.name;
          if (name === "key")
            this.fail(
              scope.source,
              scope.source.ts,
              "key requiere identidad y ciclo de vida de componentes; no se aproxima en stage 01.",
              located,
            );
          if (name.startsWith("client:")) {
            if (
              ![
                "client:load",
                "client:idle",
                "client:visible",
                "client:only",
                "client:media",
              ].includes(name)
            ) {
              this.fail(
                scope.source,
                scope.source.ts,
                `Directiva de hidratación desconocida: ${name}.`,
                located,
              );
            }
            continue; // Native rendering is immediate; no hydration boundary exists.
          }
          if (Object.hasOwn(attrs, name) || Object.hasOwn(events, name)) {
            this.fail(
              scope.source,
              scope.source.ts,
              `Atributo duplicado: ${name}.`,
              located,
            );
          }
          if (
            name.includes(":") ||
            name === "set:html" ||
            name === "set:text"
          ) {
            this.fail(
              scope.source,
              scope.source.ts,
              `Directiva Astro no soportada: ${name}.`,
              located,
            );
          }
          if (/^on[a-z]/.test(name))
            this.fail(
              scope.source,
              scope.source.ts,
              "Scripts de eventos HTML no son convertibles; usa un componente Preact.",
              located,
            );
          if (/^on[A-Z]/.test(name)) {
            if (attribute.kind !== "expression")
              this.fail(
                scope.source,
                scope.source.ts,
                "Un evento necesita un handler declarativo.",
                located,
              );
            events[name] = this.handler(
              this.parseExpression(attribute.value, located),
              located,
            );
          } else if (attribute.kind === "quoted")
            attrs[name] = literal(decodeHTMLAttribute(attribute.value));
          else if (attribute.kind === "empty") attrs[name] = literal(true);
          else if (
            attribute.kind === "expression" ||
            attribute.kind === "shorthand"
          ) {
            attrs[name] = this.expr(
              this.parseExpression(attribute.value || name, located),
              located,
            );
          } else
            this.fail(
              scope.source,
              scope.source.ts,
              `Atributo Astro ${attribute.kind} no soportado; elimina spread o template-literal.`,
              located,
            );
        }
        nodes.push(
          await this.element(
            child.name ?? "",
            attrs,
            events,
            await this.astroChildren(child.children ?? [], located),
            located,
            scope.source.ts,
          ),
        );
      } else
        this.fail(
          scope.source,
          scope.source.ts,
          `Nodo Astro no soportado: ${child.type}.`,
          located,
        );
    }
    return nodes;
  }

  private serialize(node: AstroNode): string {
    if (node.type === "text") return node.value ?? "";
    if (node.type === "expression")
      return `{${(node.children ?? []).map((n) => this.serialize(n)).join("")}}`;
    if (node.type === "comment") return "{/* comentario */}";
    if (node.type === "fragment")
      return `<>${(node.children ?? []).map((n) => this.serialize(n)).join("")}</>`;
    const attributes = (node.attributes ?? [])
      .map((a) => {
        if (a.kind === "empty") return a.name;
        if (a.kind === "quoted") return `${a.name}=${JSON.stringify(a.value)}`;
        if (a.kind === "spread") return `{...${a.value}}`;
        return `${a.name}={${a.value || a.name}}`;
      })
      .join(" ");
    const opening = `<${node.name}${attributes ? " " + attributes : ""}`;
    if (!node.children?.length) return `${opening} />`;
    return `${opening}>${node.children.map((n) => this.serialize(n)).join("")}</${node.name}>`;
  }
}
