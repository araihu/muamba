import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { runInNewContext } from "node:vm";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("landing explains Muamba and links to documentation", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<h1[^>]*>Trust remote files once\. Verify them forever\.<\/h1>/);
  assert.match(html, /href="\/docs"/);
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@latest/);
  assert.match(html, /data-muamba-workflow/);
});

test("landing uses Arai Hu color modes and Goshtoso controls", async () => {
  const [html, css] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../public/styles/site.css"),
  ]);

  assert.match(html, /<html[^>]*data-theme="araihu"/);
  assert.match(html, /prefers-color-scheme: dark/);
  assert.match(html, /window\.goshtosoStorageConsent=\{allowed:canStore\}/);
  assert.match(html, /muamba:storage-probe/);
  assert.match(html, /src="\/assets\/js\/darkmode\.js"/);
  assert.match(html, /id="muamba-color-mode"/);
  assert.match(html, /<nav[^>]*aria-label="Primary navigation"[^>]*x-data="\{\}"/);
  assert.match(html, /x-on:change="\$store\.darkMode\.toggle\(\)"/);
  assert.match(css, /--muamba-ink: var\(--color-on-surface-strong\)/);
  assert.match(css, /--muamba-accent: var\(--color-primary\)/);
  assert.doesNotMatch(css, /--muamba-accent: #[0-9a-f]+/i);
});

test("landing keeps navigation links and install command inside Goshtoso components", async () => {
  const html = await read("../app/_generated/index.html");
  const primaryCTA = html.match(/<a[^>]*data-primary-cta="true"[^>]*>/)?.[0] ?? "";
  const finalCTA = html.match(/<a[^>]*data-final-cta="true"[^>]*>/)?.[0] ?? "";

  assert.match(html, /Language-agnostic TOFU vendoring/);
  assert.doesNotMatch(html, /TOFU vendoring for Go/);
  assert.match(primaryCTA, /text-primary/);
  assert.doesNotMatch(primaryCTA, /bg-primary/);
  assert.match(finalCTA, /text-primary/);
  assert.doesNotMatch(finalCTA, /bg-primary/);
  assert.match(html, /data-install-codeblock="compact"/);
  assert.match(html, /aria-label="Copy Install Muamba code"/);
  assert.doesNotMatch(html, /class="muamba-install"/);
});

test("landing footer identifies linked Muamba and Arai Hu projects", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<a href="\/">Muamba<\/a> · an <a href="https:\/\/araihu\.com">Arai Hû<\/a> project/);
});

test("Goshtoso dark-mode runtime follows system preference and toggles persistently", async () => {
  const source = await read("../public/assets/js/darkmode.js");
  const listeners = new Map();
  const stores = new Map();
  const classes = new Set();
  const storage = new Map();
  const context = {
    document: {
      addEventListener: (name, listener) => listeners.set(name, listener),
      documentElement: {
        classList: {
          add: (name) => classes.add(name),
          remove: (name) => classes.delete(name),
        },
      },
    },
    localStorage: {
      getItem: (name) => storage.get(name) ?? null,
      setItem: (name, value) => storage.set(name, value),
      removeItem: (name) => storage.delete(name),
    },
    matchMedia: () => ({ matches: true }),
  };
  context.window = context;
  context.Alpine = {
    store(name, value) {
      if (value !== undefined) stores.set(name, value);
      return stores.get(name);
    },
  };

  runInNewContext(source, context);
  listeners.get("alpine:init")();

  const darkMode = stores.get("darkMode");
  assert.equal(darkMode.on, true);
  assert.equal(classes.has("dark"), true);
  darkMode.toggle();
  assert.equal(darkMode.on, false);
  assert.equal(classes.has("dark"), false);
  assert.equal(storage.get("darkMode"), false);
});

test("docs use Goshtoso componentdocshell and remain static", async () => {
  const html = await read("../app/_generated/docs/index.html");

  assert.match(html, /class="[^"]*component-doc-shell/);
  assert.match(html, /<h1[^>]*>Get started<\/h1>/);
  assert.match(html, /href="\/componentdocshell\/assets\/shell\.css/);
  assert.match(html, /href="\/assets\/styles\.css/);
  assert.doesNotMatch(html, /WebAssembly|wasm_exec|fetch\(["']\/api/);
});

test("docs keep the Arai Hu theme and one guide heading", async () => {
  const html = await read("../app/_generated/docs/index.html");

  assert.match(html, /&#34;theme&#34;:&#34;araihu&#34;/);
  assert.doesNotMatch(html, /id="componentdocshell-theme-trigger"/);
  assert.equal((html.match(/>Guide<\/h3>|>Guide<\/div>/g) ?? []).length, 1);
});

test("docs use explicit prose rhythm and Goshtoso inline code", async () => {
  const [html, css] = await Promise.all([
    read("../app/_generated/docs/index.html"),
    read("../public/styles/site.css"),
  ]);

  assert.match(html, /class="muamba-docs-codeblock"/);
  assert.match(html, /data-inline-code="version"/);
  assert.match(html, /class="[^"]*muamba-trust-alert[^"]*"[^>]*role="alert"/);
  assert.match(css, /\.muamba-docs-codeblock\s*\{[^}]*margin-top:\s*1rem/s);
  assert.doesNotMatch(css, /\.muamba-docs \[data-code-block\]/);
});

test("docs footer links Muamba, Arai Hu, Goshtoso, and App Shells", async () => {
  const html = await read("../app/_generated/docs/index.html");

  assert.match(html, /<a href="\/">Muamba docs<\/a>/);
  assert.match(html, /<a href="https:\/\/araihu\.com">Arai Hû<\/a>/);
  assert.match(html, /<a href="https:\/\/goshtoso\.araihu\.com">Goshtoso<\/a>/);
  assert.match(html, /<a href="https:\/\/github\.com\/araihu\/goshtoso-app-shells">App Shells<\/a>/);
});

test("generated pages have one main landmark and skip links", async () => {
  for (const path of ["../app/_generated/index.html", "../app/_generated/docs/index.html"]) {
    const html = await read(path);
    assert.equal((html.match(/<main\b/g) ?? []).length, 1, path);
    assert.match(html, /href="#main-content"|href="#hero-content"/);
  }
});

test("brand uses the project-local Muamba crate mark", async () => {
  const [landing, docs, mark, logo] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../app/_generated/docs/index.html"),
    read("../public/brand/muamba-mark.svg"),
    read("../public/brand/muamba-logo.svg"),
  ]);

  assert.match(landing, /href="\/brand\/muamba-mark\.svg"/);
  assert.match(landing, /src="\/brand\/muamba-mark\.svg"/);
  assert.doesNotMatch(landing, />M<\/span>/);
  assert.match(docs, /src="\/brand\/muamba-mark\.svg"/);
  assert.match(mark, /data-muamba-crate/);
  assert.match(logo, /data-muamba-crate/);
  assert.match(logo, />MUAMBA<\/text>/);
});
