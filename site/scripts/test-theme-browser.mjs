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
  "--disable-gpu",
  "--disable-extensions",
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
  const loadWithScheme = async (scheme, pathname = "/") => {
    await send("Emulation.setEmulatedMedia", {
      features: [{ name: "prefers-color-scheme", value: scheme }],
    }, sessionId);
    const loaded = waitFor("Page.loadEventFired", sessionId);
    await send("Page.navigate", { url: `${origin}${pathname}` }, sessionId);
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

  await setViewport(1280, 900);
  await loadWithScheme("light");
  const incumbentTypography = await evaluate(`(() => {
    const rootStyle = getComputedStyle(document.documentElement);
    return {
      body: getComputedStyle(document.body).fontFamily,
      heading: getComputedStyle(document.querySelector('.muamba-hero-copy h1')).fontFamily,
      bodyToken: rootStyle.getPropertyValue('--font-body').trim(),
      titleToken: rootStyle.getPropertyValue('--font-title').trim(),
    };
  })()`);
  assert.match(incumbentTypography.body, /^ui-sans-serif/);
  assert.match(incumbentTypography.heading, /^ui-sans-serif/);
  assert.match(incumbentTypography.bodyToken, /Lato/);
  assert.match(incumbentTypography.titleToken, /Lato/);

  await setViewport(1496, 849);
  await loadWithScheme("dark", "/docs");
  const docsTypography = await evaluate(`(() => {
    const section = document.querySelector('.muamba-docs section');
    const paragraphs = section.querySelectorAll(':scope > p');
    const codeblock = section.querySelector(':scope > .muamba-docs-codeblock');
    const codeFollower = codeblock.nextElementSibling;
    if (!codeFollower || codeFollower.tagName !== 'P') {
      throw new Error('no paragraph follows the code block');
    }
    const h2 = section.querySelector(':scope > h2');
    const paragraphStyle = getComputedStyle(paragraphs[0]);
    const h2Style = getComputedStyle(h2);
    const alertTitleStyle = getComputedStyle(document.querySelector('.muamba-trust-alert h3'));
    const alertBodyStyle = getComputedStyle(document.querySelector('.muamba-trust-alert p'));
    const measure = document.createElement('span');
    measure.style.cssText = 'position:absolute;visibility:hidden;width:75ch;font:' + paragraphStyle.font;
    document.body.append(measure);
    const result = {
      paragraphGap: Math.round(paragraphs[1].getBoundingClientRect().top - paragraphs[0].getBoundingClientRect().bottom),
      codeFollowGap: Math.round(codeFollower.getBoundingClientRect().top - codeblock.getBoundingClientRect().bottom),
      proseWidth: Math.round(paragraphs[0].getBoundingClientRect().width),
      maximumReadableWidth: Math.round(measure.getBoundingClientRect().width),
      fontFamily: paragraphStyle.fontFamily,
      h2LineHeightRatio: Number((parseFloat(h2Style.lineHeight) / parseFloat(h2Style.fontSize)).toFixed(2)),
      alertTitleFontSize: Math.round(parseFloat(alertTitleStyle.fontSize)),
      alertTitleMarginTop: Math.round(parseFloat(alertTitleStyle.marginTop)),
      alertTitleMarginBottom: Math.round(parseFloat(alertTitleStyle.marginBottom)),
      alertBodyFontSize: Math.round(parseFloat(alertBodyStyle.fontSize)),
      pageFits: document.documentElement.scrollWidth <= innerWidth,
    };
    measure.remove();
    return result;
  })()`);
  assert.ok(docsTypography.paragraphGap >= 16, `paragraph gap is ${docsTypography.paragraphGap}px`);
  assert.ok(docsTypography.codeFollowGap >= 16, `code-to-copy gap is ${docsTypography.codeFollowGap}px`);
  assert.ok(docsTypography.proseWidth <= docsTypography.maximumReadableWidth,
    `prose measure is ${docsTypography.proseWidth}px; 75ch is ${docsTypography.maximumReadableWidth}px`);
  assert.match(docsTypography.fontFamily, /^"Instrument Sans"/);
  assert.ok(docsTypography.h2LineHeightRatio <= 1.3,
    `h2 line-height ratio is ${docsTypography.h2LineHeightRatio}`);
  assert.deepEqual({
    titleFontSize: docsTypography.alertTitleFontSize,
    titleMarginTop: docsTypography.alertTitleMarginTop,
    titleMarginBottom: docsTypography.alertTitleMarginBottom,
    bodyFontSize: docsTypography.alertBodyFontSize,
  }, { titleFontSize: 14, titleMarginTop: 0, titleMarginBottom: 0, bodyFontSize: 14 });
  assert.equal(docsTypography.pageFits, true);

  for (const width of [375, 639, 719, 720, 959, 960, 1279, 1280]) {
    await setViewport(width, 900);
    await loadWithScheme("light", "/docs");
    const responsiveTypography = await evaluate(`(() => {
      const h1 = document.querySelector('.muamba-docs h1');
      const h2 = document.querySelector('.muamba-docs section > h2');
      const paragraph = document.querySelector('.muamba-docs section > p');
      const paragraphStyle = getComputedStyle(paragraph);
      const menuStyle = getComputedStyle(document.querySelector('.component-doc-shell__menu-button'));
      const sidebarStyle = getComputedStyle(document.querySelector('.component-doc-shell__sidebar'));
      const overflowProbe = document.createElement('p');
      const overflowCode = document.createElement('code');
      overflowCode.textContent = 'https://example.com/' + 'unbroken-segment-'.repeat(30);
      overflowProbe.append(overflowCode);
      paragraph.parentElement.append(overflowProbe);
      const measure = document.createElement('span');
      measure.style.cssText = 'position:absolute;visibility:hidden;width:45ch;font:' + paragraphStyle.font;
      document.body.append(measure);
      const content = [...document.querySelectorAll('.muamba-docs section > h2, .muamba-docs section > h3, .muamba-docs section > p, .muamba-docs-codeblock')];
      const overflowRect = overflowCode.getBoundingClientRect();
      const result = {
        h1FontSize: Math.round(parseFloat(getComputedStyle(h1).fontSize)),
        h2FontSize: Math.round(parseFloat(getComputedStyle(h2).fontSize)),
        proseWidth: Math.round(paragraph.getBoundingClientRect().width),
        minimumReadableWidth: Math.round(measure.getBoundingClientRect().width),
        menuDisplay: menuStyle.display,
        sidebarPosition: sidebarStyle.position,
        sidebarVisibility: sidebarStyle.visibility,
        inlineCodeIntact: [...document.querySelectorAll('.muamba-docs section > p > code')]
          .every((element) => element.getClientRects().length === 1),
        inlineCodeFits: overflowRect.left >= -0.5 && overflowRect.right <= innerWidth + 0.5,
        contentFits: content.every((element) => {
          const rect = element.getBoundingClientRect();
          return rect.left >= 0 && rect.right <= innerWidth + 0.5;
        }),
      };
      measure.remove();
      return result;
    })()`);
    assert.equal(responsiveTypography.contentFits, true, `docs content overflows at ${width}px`);
    assert.equal(responsiveTypography.inlineCodeIntact, true, `inline code wraps internally at ${width}px`);
    if (width < 640) {
      assert.ok(responsiveTypography.h1FontSize > responsiveTypography.h2FontSize,
        `heading hierarchy reverses at ${width}px: h1 ${responsiveTypography.h1FontSize}px, h2 ${responsiveTypography.h2FontSize}px`);
    }
    if (width === 720) {
      assert.ok(responsiveTypography.proseWidth >= responsiveTypography.minimumReadableWidth,
        `prose collapses to ${responsiveTypography.proseWidth}px at sidebar breakpoint; 45ch is ${responsiveTypography.minimumReadableWidth}px`);
    }
    if (width < 960) {
      assert.equal(responsiveTypography.sidebarVisibility, 'hidden', `closed sidebar is not hidden at ${width}px`);
      await evaluate(`document.querySelector('.component-doc-shell__menu-button').click()`);
      await evaluate(`new Promise((resolve, reject) => {
        const deadline = performance.now() + 1000;
        const check = () => {
          if (getComputedStyle(document.querySelector('.component-doc-shell__sidebar')).visibility === 'visible') {
            resolve(true);
          } else if (performance.now() >= deadline) {
            reject(new Error('sidebar did not become visible'));
          } else {
            requestAnimationFrame(check);
          }
        };
        check();
      })`);
      assert.equal(await evaluate(`getComputedStyle(document.querySelector('.component-doc-shell__sidebar')).visibility`),
        'visible', `open sidebar stays hidden at ${width}px`);
    }
    if (width >= 720 && width < 960) {
      assert.equal(responsiveTypography.menuDisplay, 'flex', `menu is hidden at ${width}px`);
      assert.equal(responsiveTypography.sidebarPosition, 'fixed', `sidebar consumes reading width at ${width}px`);
    }
    if (width === 960) {
      assert.equal(responsiveTypography.menuDisplay, 'none');
      assert.equal(responsiveTypography.sidebarPosition, 'static');
    }
    assert.equal(responsiveTypography.inlineCodeFits, true, `inline code overflows at ${width}px`);
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
  if (chrome.exitCode === null) {
    const exited = new Promise((resolve) => chrome.once("exit", resolve));
    chrome.kill("SIGTERM");
    const graceful = await Promise.race([
      exited.then(() => true),
      new Promise((resolve) => setTimeout(() => resolve(false), 2_000)),
    ]);
    if (!graceful) {
      chrome.kill("SIGKILL");
      await exited;
    }
  }
  await new Promise((resolve) => server.close(resolve));
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
