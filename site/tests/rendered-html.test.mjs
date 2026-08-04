import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("landing explains Muamba and links to documentation", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<h1[^>]*>Choose each source URL\. Lock the first bytes fetched\.<\/h1>/);
  assert.match(html, /href="\/docs"/);
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@v0\.0\.3/);
  assert.doesNotMatch(html, /go get -tool[^<\n]*@latest/);
  assert.match(html, /href="https:\/\/github\.com\/araihu\/muamba\/releases\/latest"/);
  assert.match(html, /Download release/);
  assert.match(html, /data-muamba-workflow/);
});

test("public installation offers standalone releases and pinned Go tools", async () => {
  const [landing, docs, readme] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../app/_generated/docs/index.html"),
    read("../../README.md"),
  ]);

  for (const text of [landing, docs, readme]) {
    assert.match(text, /https:\/\/github\.com\/araihu\/muamba\/releases\/latest/);
  }
  assert.match(landing, /Prebuilt releases need no Go installation\./);
  assert.match(docs, /Prebuilt archives need no Go installation\./);
  assert.match(readme, /Prebuilt archives require no Go installation\./);
  assert.match(readme, /muamba version/);
  assert.match(readme, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@v0\.0\.3/);
});

test("public routes expose complete route-specific social metadata", async () => {
  const routes = [
    {
      html: await read("../app/_generated/index.html"),
      title: "Muamba · Lock first-use bytes",
      description: "Review source URLs, accept the first bytes fetched, and reject later changes with SHA-384 locks.",
      canonical: "https://muamba.araihu.com/",
    },
    {
      html: await read("../app/_generated/docs/index.html"),
      title: "Muamba docs · Get started",
      description: "Review a source URL, lock the first bytes fetched, and verify later copies offline.",
      canonical: "https://muamba.araihu.com/docs",
    },
  ];
  const image = "https://muamba.araihu.com/og-v2.png";
  const alt = "Muamba flow from reviewed source to locked bytes and offline build.";

  for (const route of routes) {
    const required = [
      `<title>${route.title}</title>`,
      `<meta name="description" content="${route.description}">`,
      `<link rel="canonical" href="${route.canonical}">`,
      `<meta property="og:url" content="${route.canonical}">`,
      `<meta property="og:type" content="website">`,
      `<meta property="og:title" content="${route.title}">`,
      `<meta property="og:description" content="${route.description}">`,
      `<meta property="og:site_name" content="Muamba">`,
      `<meta property="og:image" content="${image}">`,
      `<meta property="og:image:type" content="image/png">`,
      `<meta property="og:image:width" content="1280">`,
      `<meta property="og:image:height" content="640">`,
      `<meta property="og:image:alt" content="${alt}">`,
      `<meta name="twitter:card" content="summary_large_image">`,
      `<meta name="twitter:title" content="${route.title}">`,
      `<meta name="twitter:description" content="${route.description}">`,
      `<meta name="twitter:image" content="${image}">`,
      `<meta name="twitter:image:alt" content="${alt}">`,
    ];
    for (const tag of required) {
      assert.equal(route.html.split(tag).length - 1, 1, `${route.canonical}: ${tag}`);
    }
  }
});

test("social preview image has validated share dimensions and size", async () => {
  const image = await readFile(new URL("../public/og-v2.png", import.meta.url));

  assert.ok(image.subarray(0, 8).equals(Buffer.from([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  ])));
  assert.equal(image.subarray(12, 16).toString("ascii"), "IHDR");
  assert.equal(image.readUInt32BE(16), 1280);
  assert.equal(image.readUInt32BE(20), 640);
  assert.ok(image.byteLength < 1_000_000, `social preview is ${image.byteLength} bytes`);
});

test("public copy states the trust contract directly", async () => {
  const [landing, docs, readme] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../app/_generated/docs/index.html"),
    read("../../README.md"),
  ]);

  assert.match(landing, /Choose each source URL\. Lock the first bytes fetched\./);
  assert.match(landing, /Running lock accepts the first response and records its SHA-384 digest\./);
  assert.match(docs, /The digest detects later changes; it does not authenticate the publisher or content\./);
  assert.match(readme, /Running `lock`\s+accepts the first response returned by each reviewed URL and records its/);
  assert.match(readme, /cmd\/muamba@v0\.0\.3/);

  const retiredCopy = /Review remote files once|You choose the sources and bytes to trust|verified (?:files|bytes)|A small workflow with a hard boundary|Remote convenience, local certainty|Muamba never decides|It is aimed at|—/i;
  assert.doesNotMatch(`${landing}\n${docs}\n${readme}`, retiredCopy);
});

test("public copy distinguishes materialized verification from cache verification and sync order", async () => {
  const [landing, docs] = await Promise.all([
    read("../app/_generated/index.html"),
    read("../app/_generated/docs/index.html"),
  ]);

  for (const html of [landing, docs]) {
    assert.match(html, /verify --all-platforms/);
    assert.match(html, /Sync checks the destination first, then the cache, then the network\./);
  }
  assert.match(docs, /Default verify checks materialized files/);
  assert.doesNotMatch(`${landing}\n${docs}`, /Verification reads committed files and integrity cache blobs|Sync uses verified cache first/);
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
  assert.match(html, /class="muamba-install-code"[^>]*>.*?data-code-block.*?data-density="compact"/s);
  assert.doesNotMatch(html, /aria-label="Copy (Lock|Verify|Sync) code"/);
  assert.doesNotMatch(html, /class="muamba-install"/);
});

test("landing header exposes the current release and icon controls", async () => {
  const html = await read("../app/_generated/index.html");
  const navigation = html.match(/<nav class="landing-shell__navigation".*?<\/nav>/)?.[0] ?? "";

  assert.match(html, /href="https:\/\/github\.com\/araihu\/muamba\/releases\/tag\/v0\.0\.3"/);
  assert.match(html, />v0\.0\.3<\/a>/);
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
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@v0\.0\.3/);
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
  assert.equal((html.match(/data-density="compact"/g) ?? []).length, 4);
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
