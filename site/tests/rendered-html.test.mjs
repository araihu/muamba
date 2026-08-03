import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("landing explains Muamba and links to documentation", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<h1[^>]*>Trust remote files once\. Verify them forever\.<\/h1>/);
  assert.match(html, /href="\/docs"/);
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@v0\.0\.2/);
  assert.doesNotMatch(html, /go get -tool[^<\n]*@latest/);
  assert.match(html, /data-muamba-workflow/);
});

test("landing uses Arai Hu color modes and Goshtoso controls", async () => {
  const [html, css] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../public/styles/site.css"),
  ]);

  assert.match(html, /<html[^>]*data-theme="araihu"/);
  assert.match(html, /prefers-color-scheme: dark/);
  assert.match(html, /href="\/landingshell\/assets\/shell\.css\?v=/);
  assert.match(html, /src="\/landingshell\/assets\/shell\.js\?v=/);
  assert.match(html, /x-data="landingShell\(/);
  assert.doesNotMatch(html, /src="\/scripts\/theme\.js"/);
  assert.doesNotMatch(html, /src="\/assets\/js\/darkmode\.js"/);
  assert.match(html, /id="landingshell-dark-mode"/);
  assert.match(html, /x-bind:aria-label="dark \? 'Switch to light mode' : 'Switch to dark mode'"/);
  assert.match(html, /x-on:click="toggleDark\(\)"/);
  assert.match(html, /aria-label="Source repository"/);
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
  assert.match(html, /class="muamba-install-code" aria-label="Install Muamba"/);
  assert.match(html, /aria-label="Copy Install Muamba code"/);
  assert.doesNotMatch(html, /aria-label="Copy (Lock|Verify|Sync) code"/);
  assert.doesNotMatch(html, /muamba-codeblock-compact/);
  assert.doesNotMatch(html, /class="muamba-install"/);
});

test("landing header exposes the current release and icon controls", async () => {
  const html = await read("../app/_generated/index.html");
  const navigation = html.match(/<nav class="landing-shell__navigation".*?<\/nav>/)?.[0] ?? "";

  assert.match(html, /href="https:\/\/github\.com\/araihu\/muamba\/releases\/tag\/v0\.0\.2"/);
  assert.match(html, />v0\.0\.2<\/a>/);
  assert.match(html, /<button[^>]*id="landingshell-dark-mode"[^>]*aria-label="Switch to dark mode"/);
  assert.match(navigation, /<a[^>]*aria-label="Source repository"/);
  assert.doesNotMatch(navigation, />GitHub<\/a>/);
  assert.doesNotMatch(navigation, />Dark mode<\/label>/);
});

test("landing footer identifies linked Muamba and Arai Hu projects", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<strong>Muamba<\/strong>/);
  assert.match(html, /an <a href="https:\/\/araihu\.com"[^>]*>Arai Hû<\/a> project/);
  assert.match(html, /<nav class="landing-shell__footer-links" aria-label="Footer navigation">/);
  assert.match(html, /<a href="\/docs">Docs<\/a>/);
  assert.match(html, /<a href="https:\/\/github\.com\/araihu\/muamba"[^>]*>GitHub<\/a>/);
});

test("landing shell owns guarded dark-mode behavior", async () => {
  const source = await read("../public/landingshell/assets/shell.js");

  assert.match(source, /Alpine\.data\("landingShell"/);
  assert.match(source, /root\.classList\.toggle\("dark", this\.dark\)/);
  assert.match(source, /try \{ window\.localStorage\.setItem\("darkMode", String\(this\.dark\)\); \} catch/);
});

test("docs use Goshtoso componentdocshell and remain static", async () => {
  const html = await read("../app/_generated/docs/index.html");

  assert.match(html, /class="[^"]*component-doc-shell/);
  assert.match(html, /<h1[^>]*>Get started<\/h1>/);
  assert.match(html, /href="\/componentdocshell\/assets\/shell\.css/);
  assert.match(html, /href="\/assets\/styles\.css/);
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@v0\.0\.2/);
  assert.doesNotMatch(html, /go get -tool[^<\n]*@latest/);
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
