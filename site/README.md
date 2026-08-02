# Muamba site

Static landing and documentation site for [Muamba](https://github.com/araihu/muamba), published at [muamba.araihu.com](https://muamba.araihu.com).

Go + templ pre-render the pages with Goshtoso. The docs route uses `goshtoso-app-shells/componentdocshell`. Vinext packages the generated HTML and local embedded assets for Sites; the deployed routes return constant documents and use no API or WebAssembly.

## Development

```bash
npm install
npm run generate
npm run dev
```

## Verification

```bash
go test ./...
npm test
```

Edit `internal/pages/*.templ`, run `templ generate`, then run `npm run generate`. Do not hand-edit `*_templ.go` or `app/_generated/*`.
