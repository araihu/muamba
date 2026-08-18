import assert from "node:assert/strict";
import { access, mkdtemp, readFile, rm, stat } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { extname, join, normalize } from "node:path";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const dist = fileURLToPath(new URL("../dist/", import.meta.url));
const storageDisabled = process.env.MUAMBA_DISABLE_STORAGE === "1";
const chromeCandidates = [
  process.env.CHROME_BIN,
  "/opt/homebrew/bin/chromium",
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  "/Applications/Chromium.app/Contents/MacOS/Chromium",
  "/usr/bin/google-chrome",
  "/usr/bin/chromium",
  "/usr/bin/chromium-browser",
].filter(Boolean);

async function findChrome() {
  for (const candidate of chromeCandidates) {
    try {
      await access(candidate);
      return candidate;
    } catch {}
  }
  throw new Error("Chrome or Chromium not found; set CHROME_BIN");
}

const mime = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
};

const server = createServer(async (request, response) => {
  try {
    const pathname = decodeURIComponent(new URL(request.url, "http://localhost").pathname);
    const route = pathname === "/" ? "/index.html" : pathname === "/docs" ? "/docs/index.html" : pathname;
    const relative = normalize(route).replace(/^[/\\]+/, "");
    const filename = join(dist, relative);
    const metadata = await stat(filename);
    if (!metadata.isFile() || !filename.startsWith(dist)) throw new Error("not found");
    response.writeHead(200, { "content-type": mime[extname(filename)] ?? "application/octet-stream" });
    response.end(await readFile(filename));
  } catch {
    response.writeHead(404);
    response.end();
  }
});

await new Promise((resolve, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolve);
});

const address = server.address();
const origin = `http://127.0.0.1:${address.port}`;
const profile = await mkdtemp(join(tmpdir(), "muamba-theme-browser-"));
const chrome = spawn(await findChrome(), [
  "--headless=new",
  "--disable-features=MacAppCodeSignClone",
  "--disable-gpu",
  "--no-default-browser-check",
  "--no-first-run",
  "--remote-debugging-port=0",
  `--user-data-dir=${profile}`,
  "about:blank",
], { stdio: ["ignore", "ignore", "pipe"] });

try {
  const browserEndpoint = await new Promise((resolve, reject) => {
    let stderr = "";
    const timeout = setTimeout(() => reject(new Error(`Chrome startup timeout: ${stderr}`)), 10_000);
    chrome.stderr.setEncoding("utf8");
    chrome.stderr.on("data", (chunk) => {
      stderr += chunk;
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    });
    chrome.once("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`Chrome exited before DevTools was ready: ${code}\n${stderr}`));
    });
  });

  const socket = new WebSocket(browserEndpoint);
  await new Promise((resolve, reject) => {
    socket.addEventListener("open", resolve, { once: true });
    socket.addEventListener("error", reject, { once: true });
  });

  let sequence = 0;
  const pending = new Map();
  const events = [];
  socket.addEventListener("message", ({ data }) => {
    const message = JSON.parse(data);
    if (message.id) {
      const waiter = pending.get(message.id);
      pending.delete(message.id);
      if (message.error) waiter.reject(new Error(message.error.message));
      else waiter.resolve(message.result);
      return;
    }
    events.push(message);
  });

  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++sequence;
    pending.set(id, { resolve, reject });
    socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
  });
  const waitFor = async (method, sessionId) => {
    for (;;) {
      const index = events.findIndex((event) => event.method === method && event.sessionId === sessionId);
      if (index >= 0) return events.splice(index, 1)[0];
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
  };

  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  await send("Page.enable", {}, sessionId);
  await send("Runtime.enable", {}, sessionId);
  await send("Log.enable", {}, sessionId);
  if (storageDisabled) {
    await send("Page.addScriptToEvaluateOnNewDocument", {
      source: `Object.defineProperty(window, "localStorage", {
        configurable: true,
        get() { throw new DOMException("storage denied", "SecurityError") }
      })`,
    }, sessionId);
  }

  const evaluate = async (expression) => {
    const result = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true }, sessionId);
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text);
    return result.result.value;
  };
  const state = () => evaluate(`(() => ({
    dark: document.documentElement.classList.contains("dark"),
    label: document.querySelector("#landingshell-dark-mode").getAttribute("aria-label"),
    store: Alpine.$data(document.documentElement).dark,
    saved: (() => { try { return localStorage.getItem("darkMode") } catch { return "unavailable" } })()
  }))()`);
  const loadWithScheme = async (scheme) => {
    await send("Emulation.setEmulatedMedia", {
      features: [{ name: "prefers-color-scheme", value: scheme }],
    }, sessionId);
    const loaded = waitFor("Page.loadEventFired", sessionId);
    await send("Page.navigate", { url: `${origin}/` }, sessionId);
    await loaded;
    await new Promise((resolve) => setTimeout(resolve, 150));
  };

  const setViewport = (width, height) => send("Emulation.setDeviceMetricsOverride", {
    width,
    height,
    deviceScaleFactor: 1,
    mobile: false,
  }, sessionId);

  await loadWithScheme("dark");
  if (storageDisabled) {
    assert.deepEqual(await state(), { dark: true, label: "Switch to light mode", store: true, saved: "unavailable" });
    await evaluate(`document.querySelector("#landingshell-dark-mode").click()`);
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.deepEqual(await state(), { dark: false, label: "Switch to dark mode", store: false, saved: "unavailable" });

    await loadWithScheme("light");
    assert.deepEqual(await state(), { dark: false, label: "Switch to dark mode", store: false, saved: "unavailable" });
    await evaluate(`document.querySelector("#landingshell-dark-mode").click()`);
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.deepEqual(await state(), { dark: true, label: "Switch to light mode", store: true, saved: "unavailable" });
  } else {
    assert.deepEqual(await state(), { dark: true, label: "Switch to light mode", store: true, saved: null });
    await evaluate(`document.querySelector("#landingshell-dark-mode").click()`);
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.deepEqual(await state(), { dark: false, label: "Switch to dark mode", store: false, saved: "false" });

    await evaluate("localStorage.clear()");
    await loadWithScheme("light");
    assert.deepEqual(await state(), { dark: false, label: "Switch to dark mode", store: false, saved: null });
    await evaluate(`document.querySelector("#landingshell-dark-mode").click()`);
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.deepEqual(await state(), { dark: true, label: "Switch to light mode", store: true, saved: "true" });

    await setViewport(880, 781);
    await loadWithScheme("light");
    assert.deepEqual(await evaluate(`(() => {
      const hero = document.querySelector(".muamba-hero-grid");
      const code = document.querySelector(".muamba-install-code .codeblock");
      return {
        columns: getComputedStyle(hero).gridTemplateColumns.split(" ").length,
        commandFits: code.scrollWidth <= code.clientWidth,
        pageFits: document.documentElement.scrollWidth <= innerWidth,
      };
    })()`), { columns: 1, commandFits: true, pageFits: true });

    await setViewport(1280, 900);
    await loadWithScheme("light");
    assert.equal(await evaluate(`getComputedStyle(document.querySelector(".muamba-hero-grid")).gridTemplateColumns.split(" ").length`), 2);
  }

  assert.deepEqual(
    events.filter((event) => event.method === "Runtime.exceptionThrown" ||
      (event.method === "Log.entryAdded" && event.params?.entry?.level === "error")),
    [],
    "browser page emitted an exception or error log",
  );

  socket.close();
  console.log(storageDisabled
    ? "theme browser test passed with localStorage throwing SecurityError"
    : "theme browser test passed for dark and light system preferences");
} finally {
  chrome.kill("SIGTERM");
  server.close();
  await rm(profile, { force: true, recursive: true });
}

if (!storageDisabled) {
  const denied = spawn(process.execPath, [fileURLToPath(import.meta.url)], {
    env: { ...process.env, MUAMBA_DISABLE_STORAGE: "1" },
    stdio: "inherit",
  });
  const code = await new Promise((resolve) => denied.once("exit", resolve));
  assert.equal(code, 0, "storage-disabled browser test failed");
}
