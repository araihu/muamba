# Muamba site surface brief

- Task: explain Muamba's TOFU vendoring model, earn trust, get a Go developer to a verified first manifest.
- Register: precise, calm, toolmaker. Security claims stay concrete; no fear copy.
- Priority: product promise, install command, three-step workflow, integrity guarantees, docs path.
- Landing direction: Goshtoso landing anatomy—quiet top bar, split hero with real product artifact, sequential proof sections, restrained borders, no decorative cards.
- Docs direction: Goshtoso `componentdocshell` with Muamba-owned navigation and content; static pre-rendered HTML with local runtime assets.
- Responsive rule: one navigation trigger at mobile widths, readable code, no horizontal page scroll.
- Runtime boundary: no API and no WebAssembly. Add WASM only if future docs must parse user-provided manifests in-browser.
- Deployment boundary: publish only pre-rendered HTML and local files through Cloudflare Static Assets. Do not add an application Worker, SSR/RSC route, or image-optimization endpoint.
- Brand boundary: project-local crate mark and wordmark under `site/public/brand`; no Arai Hû asset-pipeline integration.

## Goshtoso consumer snags

- Goshtoso v0.1.3 had no semantic inline-code primitive for prose. Muamba
  required `components/inlinecode`; the new public component now owns theme
  tokens, escaping, stable identity, and consumer root hooks.
- `codeblock.CodeBlock` has no compact density option in the pinned API. Keep
  the component's semantics and copy behavior, then scope density and rhythm
  through a consumer wrapper until Goshtoso exposes that option.
- Goshtoso's standalone dark-mode store assumes that `localStorage` is
  available when no consent adapter exists. Muamba publishes a capability
  probe through `goshtosoStorageConsent`; unavailable storage falls back to
  system preference and a session-only toggle instead of breaking Alpine init.
