# Muamba site surface brief

- Task: explain Muamba's TOFU vendoring model, earn trust, get a Go developer to a verified first manifest.
- Register: precise, calm, toolmaker. Security claims stay concrete; no fear copy.
- Priority: product promise, install command, three-step workflow, integrity guarantees, docs path.
- Landing direction: Goshtoso landing anatomy—quiet top bar, split hero with real product artifact, sequential proof sections, restrained borders, no decorative cards.
- Docs direction: Goshtoso `componentdocshell` with Muamba-owned navigation and content; static pre-rendered HTML with local runtime assets.
- Responsive rule: one navigation trigger at mobile widths, readable code, no horizontal page scroll.
- Runtime boundary: no API and no WebAssembly. Add WASM only if future docs must parse user-provided manifests in-browser.
- Brand boundary: project-local crate mark and wordmark under `site/public/brand`; no Arai Hû asset-pipeline integration.

## Goshtoso consumer snags

- Goshtoso v0.1.3 had no semantic inline-code primitive for prose. Muamba
  required `components/inlinecode`; the new public component now owns theme
  tokens, escaping, stable identity, and consumer root hooks.
- `codeblock.CodeBlock` has no compact density option in the pinned API. Keep
  the component's semantics and copy behavior, then scope density and rhythm
  through a consumer wrapper until Goshtoso exposes that option.
