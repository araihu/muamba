import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("landing explains Muamba and links to documentation", async () => {
  const html = await read("../app/_generated/index.html");

  assert.match(html, /<h1[^>]*>Trust remote files once\. Verify them forever\.<\/h1>/);
  assert.match(html, /href="\/docs"/);
  assert.match(html, /go get -tool github\.com\/araihu\/muamba\/cmd\/muamba@latest/);
  assert.match(html, /data-muamba-workflow/);
});

test("docs use Goshtoso componentdocshell and remain static", async () => {
  const html = await read("../app/_generated/docs/index.html");

  assert.match(html, /class="[^"]*component-doc-shell/);
  assert.match(html, /<h1[^>]*>Get started<\/h1>/);
  assert.match(html, /href="\/componentdocshell\/assets\/shell\.css/);
  assert.match(html, /href="\/assets\/styles\.css/);
  assert.doesNotMatch(html, /WebAssembly|wasm_exec|fetch\(["']\/api/);
});

test("generated pages have one main landmark and skip links", async () => {
  for (const path of ["../app/_generated/index.html", "../app/_generated/docs/index.html"]) {
    const html = await read(path);
    assert.equal((html.match(/<main\b/g) ?? []).length, 1, path);
    assert.match(html, /href="#main-content"|href="#hero-content"/);
  }
});
