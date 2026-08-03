# Muamba site

Static landing and documentation site for [Muamba](https://github.com/araihu/muamba), published at [muamba.araihu.com](https://muamba.araihu.com).

Go + templ pre-render the pages with Goshtoso. The docs route uses `goshtoso-app-shells/componentdocshell`. A small Node build copies those generated documents and local assets into `dist/`; Wrangler publishes that directory as Cloudflare Static Assets. There is no application Worker, API, server rendering, React runtime, or WebAssembly.

## Development

```bash
npm install
npm run dev
```

## Verification

```bash
go test ./...
npm test
```

Edit `internal/pages/*.templ`, run `templ generate`, then run `npm run generate`. Do not hand-edit `*_templ.go` or `app/_generated/*`.

## Deployment

Global Wrangler owns the assets-only deployment and the `muamba.araihu.com` custom domain:

```bash
npm run deploy
```
