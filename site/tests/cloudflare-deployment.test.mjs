import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("Wrangler owns an assets-only Muamba production deployment", async () => {
  const [rawConfig, packageJSON, buildScript] = await Promise.all([
    read("../wrangler.jsonc"),
    read("../package.json"),
    read("../scripts/build-static.mjs"),
  ]);
  const config = JSON.parse(rawConfig);
  const pkg = JSON.parse(packageJSON);

  assert.equal(config.name, "muamba-site");
  assert.equal(config.main, undefined);
  assert.equal(config.assets.directory, "dist");
  assert.equal(config.assets.html_handling, "drop-trailing-slash");
  assert.equal(config.images, undefined);
  assert.deepEqual(config.routes, [
    { pattern: "muamba.araihu.com", custom_domain: true },
  ]);
  assert.equal(pkg.scripts.deploy, "npm run generate && npm run build && wrangler deploy");
  assert.match(buildScript, /app\/_generated\/docs\/index\.html/);
  assert.doesNotMatch(JSON.stringify(pkg), /vinext|next|react|vite/i);

  for (const path of ["../worker/index.ts", "../app/route.ts", "../app/docs/route.ts", "../.openai/hosting.json"]) {
    await assert.rejects(access(new URL(path, import.meta.url)), { code: "ENOENT" });
  }
});

test("static build contains both HTML routes and local assets", async () => {
  const [landing, docs, styles] = await Promise.all([
    read("../dist/index.html"),
    read("../dist/docs/index.html"),
    read("../dist/styles/site.css"),
  ]);

  assert.match(landing, /<title>Muamba/);
  assert.match(docs, /<title>Muamba docs/);
  assert.match(styles, /\.muamba-docs-codeblock/);
});
