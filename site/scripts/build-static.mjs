import { copyFile, cp, mkdir, rm } from "node:fs/promises";

const root = new URL("../", import.meta.url);
const dist = new URL("dist/", root);

await rm(dist, { force: true, recursive: true });
await cp(new URL("public/", root), dist, { recursive: true });
await mkdir(new URL("docs/", dist), { recursive: true });
await copyFile(new URL("app/_generated/index.html", root), new URL("index.html", dist));
await copyFile(
  new URL("app/_generated/docs/index.html", root),
  new URL("docs/index.html", dist),
);
