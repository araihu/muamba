import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("Wrangler owns Muamba production deployment", async () => {
  const [rawConfig, packageJSON, viteConfig] = await Promise.all([
    read("../wrangler.jsonc"),
    read("../package.json"),
    read("../vite.config.ts"),
  ]);
  const config = JSON.parse(rawConfig);
  const pkg = JSON.parse(packageJSON);

  assert.equal(config.name, "muamba-site");
  assert.equal(config.main, "./worker/index.ts");
  assert.equal(config.assets.directory, "dist/client");
  assert.deepEqual(config.routes, [
    { pattern: "muamba.araihu.com", custom_domain: true },
  ]);
  assert.equal(pkg.scripts.deploy, "npm run generate && npm run build && wrangler deploy");
  assert.doesNotMatch(viteConfig, /sites-vite-plugin|hosting\.json/);

  await assert.rejects(access(new URL("../.openai/hosting.json", import.meta.url)), {
    code: "ENOENT",
  });
});
