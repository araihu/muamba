# Muamba v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and publish Muamba v1 as a pinned Go tool that locks, synchronizes, verifies, updates, and package-embeds grouped remote downloads from `muamba.yaml`.

**Architecture:** A thin `cmd/muamba` delegates to focused internal packages for YAML documents, integrity, safe paths, HTTP transport, lifecycle transactions, and deterministic Go generation. Lifecycle commands stage remote bytes before mutation, while `verify` and `generate-go --check` provide offline CI gates. Codex Spark creates only Task 1 boilerplate; this session reviews it before functional slices begin.

**Tech Stack:** Go 1.26.5, standard library, `go.yaml.in/yaml/v3 v3.0.5`, `github.com/gofrs/flock v0.13.0`, GitHub Actions.

## Global Constraints

- Module path is `github.com/araihu/muamba`; repository license is MIT.
- Root schema is exactly `schema: 1`; unknown YAML keys fail validation.
- Resource and download names are strict kebab-case; each resource has one required opaque version and one or more keyed downloads.
- `${version}` is the only interpolation token and works in download URL and path.
- HTTPS is default; HTTP and disabled TLS verification require separate explicit flags.
- Integrity accepts SHA-256/384/512 SRI Base64 and prefixed hexadecimal; new trust defaults to SHA-384 SRI.
- `lock` and `update` are the only trust-changing commands.
- `verify` is offline/read-only; `sync` never changes the manifest or broadly prunes.
- Go generation is package-scoped, deterministic, formatted with `gofmt`, and verifies embedded bytes before output.
- Base Muamba has no metadata, overlay, attribution, or Goshtoso-specific behavior.
- Use TDD for every functional task and commit after every independently passing task.

---

## File map

```text
go.mod                                  module, Go version, pinned dependencies
go.sum                                  dependency checksums
LICENSE                                 MIT license
README.md                               install, manifest, command examples
AGENTS.md                               repository-local development rules
.gitignore                              temporary binaries and coverage output
.github/workflows/ci.yml                test, race, vet, generation drift gates
cmd/muamba/main.go                      process entrypoint
internal/cli/run.go                     subcommand/flag parsing and exit behavior
internal/cli/run_test.go                CLI contract tests
internal/manifest/model.go              typed manifest and resolved download model
internal/manifest/document.go           YAML load/find/marshal and node-preserving edits
internal/manifest/document_test.go      parsing and comment-preservation tests
internal/manifest/validate.go           schema, names, interpolation, version warnings
internal/manifest/validate_test.go      validation matrix
internal/manifest/select.go             resource/download selector resolution
internal/manifest/select_test.go        selector tests
internal/integrity/integrity.go         digest parsing, computing, formatting, comparison
internal/integrity/integrity_test.go    SRI and hex tests
internal/safepath/path.go               root containment and symlink checks
internal/safepath/path_test.go          traversal/collision/symlink tests
internal/transport/client.go            HTTPS policy, TLS, redirects, timeout, size limit
internal/transport/client_test.go       HTTP/TLS server tests
internal/lifecycle/engine.go            shared engine, report, warnings, staging primitives
internal/lifecycle/verify.go            offline verification
internal/lifecycle/verify_test.go       verify tests
internal/lifecycle/lock_sync.go         first trust and locked materialization
internal/lifecycle/lock_sync_test.go    lock/sync tests
internal/lifecycle/update.go            grouped and single-download retrust/cleanup
internal/lifecycle/update_test.go       transaction and cleanup tests
internal/gogen/generate.go              package discovery, registry rendering, check mode
internal/gogen/generate_test.go         golden and compile fixture tests
internal/gogen/testdata/want.go         stable generated output
internal/testrepo/testrepo.go            temporary repository fixture helpers
```

### Task 1: Repository and CLI boilerplate with Codex Spark

**Files:**
- Create: `go.mod`
- Create: `LICENSE`
- Create: `README.md`
- Create: `AGENTS.md`
- Create: `.gitignore`
- Create: `.github/workflows/ci.yml`
- Create: `cmd/muamba/main.go`
- Create: `internal/cli/run.go`
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Produces: `cli.Run(args []string, stdout, stderr io.Writer) int`
- Produces: executable `go run ./cmd/muamba help`
- Consumes: no earlier task

- [ ] **Step 1: Create and push the public repository**

Run:

```bash
gh repo create araihu/muamba --public --source=. --remote=origin --push \
  --description "TOFU vendoring and integrity locks for remote assets"
```

Expected: public repository created, `origin` uses SSH, and the design/plan commit is present on `main`.

- [ ] **Step 2: Dispatch Codex Spark with boilerplate-only boundaries**

Use model `gpt-5.3-codex-spark` with high reasoning and this exact prompt:

```text
Bootstrap github.com/araihu/muamba from the approved design and implementation
plan already committed in this repository. Create only Task 1 files: go.mod,
LICENSE, README.md, AGENTS.md, .gitignore, .github/workflows/ci.yml,
cmd/muamba/main.go, internal/cli/run.go, and internal/cli/run_test.go. Use Go
1.26.5 and the standard library only for this task. Implement cli.Run(args,
stdout, stderr) with help listing lock, sync, verify, update, and generate-go;
functional subcommands may return a concise "not implemented" error. Do not
implement manifest, transport, lifecycle, integrity, safe-path, or Go-generation
logic. Do not modify docs/superpowers. Run gofmt, go test ./..., and go vet ./....
Do not commit; report changed files and test results.
```

- [ ] **Step 3: Review Spark output against exact module baseline**

`go.mod` must contain:

```go
module github.com/araihu/muamba

go 1.26.5
```

Reject dependencies, generated code, lifecycle implementation, or changes to design/plan files. Task 2 adds YAML when first imported; Task 5 adds flock when first imported.

- [ ] **Step 4: Verify the CLI test captures the initial public contract**

```go
func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) code = %d, stderr = %q", code, stderr.String())
	}
	for _, command := range []string{"lock", "sync", "verify", "update", "generate-go"} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("help missing %q", command)
		}
	}
}
```

- [ ] **Step 5: Run boilerplate gates**

Run: `go mod tidy && go test ./... && go vet ./...`

Expected: all commands exit 0; `go.mod` has no dependencies yet.

- [ ] **Step 6: Commit boilerplate**

```bash
git add go.mod LICENSE README.md AGENTS.md .gitignore .github/workflows/ci.yml cmd/muamba internal/cli
git commit -m "chore: bootstrap muamba CLI"
```

### Task 2: Manifest document, interpolation, validation, and selectors

**Files:**
- Create: `internal/manifest/model.go`
- Create: `internal/manifest/document.go`
- Create: `internal/manifest/document_test.go`
- Create: `internal/manifest/validate.go`
- Create: `internal/manifest/validate_test.go`
- Create: `internal/manifest/select.go`
- Create: `internal/manifest/select_test.go`

**Interfaces:**
- Produces: `manifest.Find(startDir, explicit string) (string, error)`
- Produces: `manifest.Load(path string) (*Document, error)`
- Produces: `(*Document).Validate(strict bool) ([]Warning, error)`
- Produces: `(*Document).Select(selectors []string) ([]Selection, error)`
- Produces: `(*Document).SetIntegrity(resource, download, value string) error`
- Produces: `(*Document).SetVersion(resource, value string) error`
- Produces: `(*Document).Marshal() ([]byte, error)`
- Produces: resolved `Selection{ResourceName, DownloadName, Version, URL, Path, Integrity string}`
- Consumes: YAML v3 nodes for comment-preserving edits

- [ ] **Step 1: Write failing model and load tests**

Cover a grouped manifest, missing `schema`, `schema: 2`, unknown root/resource/download keys, empty resources/downloads, and `Find` walking from `assets/js` to the nearest parent `muamba.yaml`.

```go
func TestLoadGroupedManifest(t *testing.T) {
	doc := loadFixture(t, `schema: 1
resources:
  alpine:
    version: "3.14.9"
    downloads:
      core-js:
        url: https://cdn.example/alpine@${version}/alpine.js
        path: assets/alpine/${version}/alpine.js
`)
	got := doc.Manifest.Resources["alpine"].Downloads["core-js"]
	if got.URL != "https://cdn.example/alpine@${version}/alpine.js" {
		t.Fatalf("URL = %q", got.URL)
	}
}
```

- [ ] **Step 2: Run manifest load tests and confirm failure**

Run: `go test ./internal/manifest -run 'TestLoad|TestFind' -count=1`

Expected: FAIL because `Load` and `Find` do not exist.

- [ ] **Step 3: Implement typed decoding plus preserved YAML document**

Run `go get go.yaml.in/yaml/v3@v3.0.5`. Use `yaml.Node` for the source tree and decode into structs with `KnownFields(true)`. `Document` stores absolute manifest path, manifest directory, root node, and typed `Manifest`.

- [ ] **Step 4: Write failing validation/interpolation tests**

Test strict kebab-case with `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`, required opaque version, `${version}` expansion in URL/path, unknown token errors, exact case-sensitive URL-version warning, strict failure, duplicate resolved paths, absolute paths, and `..` traversal.

- [ ] **Step 5: Implement validation and resolved selections**

Use these concrete types:

```go
type Warning struct {
	Resource string
	Download string
	Message  string
}

type Selection struct {
	ResourceName string
	DownloadName string
	Version      string
	URL          string
	Path         string
	Integrity    string
}
```

Sort resources and downloads lexicographically before returning selections or warnings.

- [ ] **Step 6: Write and implement selector tests**

Cover no selector, `alpine`, `alpine/core-js`, duplicates, unknown resource, unknown download, and malformed selectors with more than one slash.

- [ ] **Step 7: Write and implement YAML mutation preservation tests**

Start with comments beside `version`, `url`, and `path`; call `SetVersion` and `SetIntegrity`; marshal; assert comments and map order remain while only target scalars change.

- [ ] **Step 8: Run focused and package tests**

Run: `go test ./internal/manifest -count=1`

Expected: PASS.

- [ ] **Step 9: Commit manifest slice**

```bash
git add internal/manifest
git commit -m "feat: parse grouped muamba manifests"
```

### Task 3: Integrity formats and safe destination paths

**Files:**
- Create: `internal/integrity/integrity.go`
- Create: `internal/integrity/integrity_test.go`
- Create: `internal/safepath/path.go`
- Create: `internal/safepath/path_test.go`

**Interfaces:**
- Produces: `integrity.Parse(value string) (Digest, error)`
- Produces: `integrity.Compute(reader io.Reader, algorithm crypto.Hash) ([]byte, error)`
- Produces: `integrity.Verify(reader io.Reader, expected Digest) (actual []byte, err error)`
- Produces: `integrity.FormatSRI(algorithm crypto.Hash, sum []byte) string`
- Produces: `safepath.Resolve(root, relative string) (string, error)`
- Produces: `safepath.ValidateUnique(root string, selections []manifest.Selection) error`

- [ ] **Step 1: Write failing integrity table tests**

Use `abc` vectors for SHA-256, SHA-384, and SHA-512 in both Base64 SRI and lowercase/uppercase hex. Reject bare hex, malformed Base64, incorrect digest lengths, unsupported `md5`, and whitespace-separated multiple digests.

- [ ] **Step 2: Run integrity tests and confirm failure**

Run: `go test ./internal/integrity -count=1`

Expected: FAIL because the package is absent.

- [ ] **Step 3: Implement digest parsing and constant-time verification**

Map only SHA-256/384/512 to `crypto.Hash`; require exact digest length; use `subtle.ConstantTimeCompare`; format new trust as `sha384-` plus standard Base64.

- [ ] **Step 4: Write failing path tests**

Cover clean nested path, absolute path, `../escape`, duplicate resolved paths, a symlinked parent escaping root, and a symlink remaining inside root.

- [ ] **Step 5: Implement safe path resolution**

Resolve the nearest existing parent with `filepath.EvalSymlinks`, append nonexistent suffix components, and compare using `filepath.Rel`. Reject `.` paths and any result equal to or outside root.

- [ ] **Step 6: Run focused tests**

Run: `go test ./internal/integrity ./internal/safepath -count=1`

Expected: PASS.

- [ ] **Step 7: Commit integrity and path slice**

```bash
git add internal/integrity internal/safepath
git commit -m "feat: verify digests and safe paths"
```

### Task 4: Safe HTTP transport

**Files:**
- Create: `internal/transport/client.go`
- Create: `internal/transport/client_test.go`

**Interfaces:**
- Produces: `transport.Options{AllowHTTP bool, InsecureSkipTLSVerify bool, CAFile string, Timeout time.Duration, MaxBytes int64}`
- Produces: `transport.New(options Options) (*Client, error)`
- Produces: `(*Client).Fetch(ctx context.Context, rawURL string, destination io.Writer) (int64, error)`
- Consumes: expanded URL from `manifest.Selection`

- [ ] **Step 1: Write failing HTTPS/HTTP policy tests**

Use `httptest.NewServer` and `httptest.NewTLSServer`. Assert HTTPS succeeds with supplied CA, HTTP fails by default, HTTP succeeds with `AllowHTTP`, invalid TLS fails, and invalid TLS succeeds only with `InsecureSkipTLSVerify`.

- [ ] **Step 2: Run transport tests and confirm failure**

Run: `go test ./internal/transport -count=1`

Expected: FAIL because `New` and `Fetch` do not exist.

- [ ] **Step 3: Implement client construction and scheme checks**

Clone a dedicated `http.Transport`, use TLS minimum 1.2, load PEM certificates into system roots for `CAFile`, set a 60-second default timeout, and reject schemes other than HTTPS or explicitly allowed HTTP.

- [ ] **Step 4: Write failing redirect and bounds tests**

Cover eleven redirects, HTTPS-to-HTTP downgrade, a response larger than 100 MiB using a lowered test option, a context timeout, non-200 status, and an error string that excludes response body text.

- [ ] **Step 5: Implement redirect revalidation and bounded copy**

Use `CheckRedirect` to enforce ten redirects and call the same scheme policy on every target. Read through `io.LimitReader(max+1)` and fail without committing when byte count exceeds the limit.

- [ ] **Step 6: Run transport tests**

Run: `go test ./internal/transport -count=1`

Expected: PASS.

- [ ] **Step 7: Commit transport slice**

```bash
git add internal/transport
git commit -m "feat: add safe download transport"
```

### Task 5: Offline verification and synchronized materialization

**Files:**
- Create: `internal/testrepo/testrepo.go`
- Create: `internal/lifecycle/engine.go`
- Create: `internal/lifecycle/verify.go`
- Create: `internal/lifecycle/verify_test.go`
- Create: `internal/lifecycle/lock_sync.go`
- Create: `internal/lifecycle/lock_sync_test.go`

**Interfaces:**
- Produces: `lifecycle.New(manifestPath string, options Options) (*Engine, error)`
- Produces: `(*Engine).Verify(ctx context.Context, selectors []string) (Report, error)`
- Produces: `(*Engine).Lock(ctx context.Context, selectors []string) (Report, error)`
- Produces: `(*Engine).Sync(ctx context.Context, selectors []string) (Report, error)`
- Produces: `Report{Changed []string, Verified []string, Warnings []manifest.Warning}`
- Consumes: manifest, integrity, safe-path, transport packages

- [ ] **Step 1: Build temporary repository helper and failing verify tests**

The helper writes a manifest and arbitrary files under `t.TempDir()`. Verify tests cover locked match, missing file, mismatched file, unlocked selection, selector filtering, warnings, and strict URL-version failure. Inject a fetcher that panics if `Verify` attempts network access.

- [ ] **Step 2: Run verify tests and confirm failure**

Run: `go test ./internal/lifecycle -run TestVerify -count=1`

Expected: FAIL because the lifecycle engine is absent.

- [ ] **Step 3: Implement engine loading and offline verify**

`New` loads and validates once. `Verify` resolves selectors, requires integrity, hashes files through `integrity.Verify`, and returns sorted report entries. It performs no mutation and does not acquire the mutation lock.

- [ ] **Step 4: Write failing first-lock tests**

Serve two deterministic files from an HTTP test server enabled through `AllowHTTP`. Assert `Lock` downloads only missing-integrity entries, writes SHA-384 SRI, preserves comments, uses atomic target replacement, and leaves already locked downloads untouched.

- [ ] **Step 5: Implement manifest-scoped locking and first trust**

Run `go get github.com/gofrs/flock@v0.13.0`. Hash the canonical manifest path with SHA-256 and create a stable lock path under `os.TempDir()/muamba-locks/`; never delete the lock file after unlock. Use `flock.New`, `TryLockContext`, and deferred unlock. Stage each download beside its target, calculate SHA-384, stage a node-preserving manifest, then commit files and manifest.

- [ ] **Step 6: Write failing sync repair tests**

Cover missing target, corrupted target, remote matching lock, remote not matching lock, unlocked download, no manifest mutation, no deletion of unrelated files, and no visible target replacement after remote mismatch.

- [ ] **Step 7: Implement sync**

Skip already matching files. For a missing/mismatched file, stage remote bytes, verify committed digest, then atomically replace only that target. Never call `SetIntegrity` or remove paths.

- [ ] **Step 8: Run lifecycle tests and race test**

Run: `go test ./internal/lifecycle -count=1 && go test -race ./internal/lifecycle -count=1`

Expected: PASS.

- [ ] **Step 9: Commit lifecycle base**

```bash
git add internal/testrepo internal/lifecycle
git commit -m "feat: lock and synchronize resources"
```

### Task 6: Explicit grouped and single-download updates

**Files:**
- Create: `internal/lifecycle/update.go`
- Create: `internal/lifecycle/update_test.go`
- Modify: `internal/lifecycle/engine.go`

**Interfaces:**
- Produces: `(*Engine).UpdateResource(ctx context.Context, resource, version string) (Report, error)`
- Produces: `(*Engine).UpdateDownload(ctx context.Context, resource, download string) (Report, error)`
- Consumes: lifecycle staging and manifest mutation primitives from Task 5

- [ ] **Step 1: Write failing grouped update success test**

Create one resource with JavaScript, CSS, and license downloads under a versioned path. Update from `1.0.0` to `1.1.0`; assert every URL/path expands with the new version, every new digest is SHA-384 SRI, manifest comments survive, and verified old files/directories are removed after commit.

- [ ] **Step 2: Write failing grouped transaction tests**

Make the second of three downloads return 500. Assert version, integrities, old files, and destination tree remain unchanged. Add a test where an old file is locally modified; preflight must fail before network access or mutation.

- [ ] **Step 3: Implement resource update staging and commit**

Clone typed/node manifest state in memory, set candidate version, validate all expanded destinations, preflight old cleanup hashes, stage all remote files, compute all digests, stage manifest bytes, atomically replace new files, atomically replace manifest, then clean verified old paths. On ordinary commit failure, restore backups and return an error.

- [ ] **Step 4: Write failing single-download retrust test**

Keep resource version unchanged, change bytes at the same URL, call `UpdateDownload`, and assert only selected file and integrity change.

- [ ] **Step 5: Implement single-download update**

Reuse staging and mutation lock. Do not alter resource version or sibling downloads.

- [ ] **Step 6: Add concurrent mutation test**

Hold the manifest flock in one goroutine/process and assert a second mutation times out with a diagnostic naming the manifest, without network or file changes.

- [ ] **Step 7: Run update and lifecycle tests**

Run: `go test ./internal/lifecycle -count=1 && go test -race ./internal/lifecycle -count=1`

Expected: PASS.

- [ ] **Step 8: Commit update slice**

```bash
git add internal/lifecycle
git commit -m "feat: update grouped resource versions"
```

### Task 7: Deterministic package-scoped Go generation

**Files:**
- Create: `internal/gogen/generate.go`
- Create: `internal/gogen/generate_test.go`
- Create: `internal/gogen/testdata/want.go`

**Interfaces:**
- Produces: `gogen.Options{Dir, Output, Package string, Check, Strict bool}`
- Produces: `gogen.Generate(document *manifest.Document, options Options) error`
- Consumes: manifest validation, selection, safe paths, and integrity verification

- [ ] **Step 1: Write failing selection and package-discovery tests**

Create downloads under `assets/` and `site/static/`. Generate for `assets`; assert only `assets` downloads appear. Infer package from an existing non-test `.go` file. Require explicit package for an empty directory. Reject output outside `--dir` and a selection containing no downloads.

- [ ] **Step 2: Run generation tests and confirm failure**

Run: `go test ./internal/gogen -count=1`

Expected: FAIL because `Generate` does not exist.

- [ ] **Step 3: Implement deterministic renderer**

Render private `embed.FS`, private resource/download constants, `MuambaResource`, `MuambaDownload`, `MuambaResources`, `MuambaResourceByName`, and `MuambaOpen`. Sort resources, downloads, constants, and explicit embed paths. Convert kebab-case by uppercasing only the first ASCII letter of each segment (`core-js` becomes `CoreJs`). Reject generated identifier collisions.

- [ ] **Step 4: Verify caller-owned registry slices**

Generated `MuambaResources` must deep-copy the outer resource slice and each `Downloads` slice. Add a test that mutates one result and confirms a later result is unchanged.

- [ ] **Step 5: Add golden and `--check` tests**

Compare formatted output with `testdata/want.go`; prove normal mode writes atomically; prove check mode passes for identical output and fails without mutation for stale output.

- [ ] **Step 6: Add compile-and-open fixture test**

Create a temporary Go module/package, copy two locked files, generate code, write a test calling `MuambaOpen`, then run `go test ./...` inside the fixture and require exit 0.

- [ ] **Step 7: Run generation tests**

Run: `go test ./internal/gogen -count=1`

Expected: PASS.

- [ ] **Step 8: Commit generation slice**

```bash
git add internal/gogen
git commit -m "feat: generate embedded Go registries"
```

### Task 8: Wire complete CLI and end-to-end behavior

**Files:**
- Modify: `internal/cli/run.go`
- Modify: `internal/cli/run_test.go`
- Modify: `cmd/muamba/main.go`
- Create: `internal/cli/e2e_test.go`

**Interfaces:**
- Consumes: all lifecycle and generation interfaces
- Produces: final `muamba lock|sync|verify|update|generate-go` CLI

- [ ] **Step 1: Write failing flag and exit-code tests**

Cover global `-f`, `--strict`, `--allow-http`, `--insecure-skip-tls-verify`, `--ca-file`, `--timeout`, and `--max-size`. Cover `update RESOURCE --version X`, `update RESOURCE/DOWNLOAD`, and `generate-go --dir --output --package --check`. Unknown flags/subcommands return exit 2; operational failures return exit 1; success returns 0.

- [ ] **Step 2: Implement subcommand parsing with standard `flag.FlagSet`**

Keep `Run` responsible only for parsing, dependency construction, concise reports, and exit codes. Do not put manifest, transport, or lifecycle logic into `internal/cli`.

- [ ] **Step 3: Write end-to-end CLI test**

Start a local HTTP server, write an unlocked grouped manifest, then call `Run` through lock, verify, corrupt, sync, update one download, generate Go, and generation check. Assert manifest/file contents after every command and ensure stderr never contains served body text.

- [ ] **Step 4: Implement concise output contract**

Print one stable line per changed or verified `resource/download`; warnings go to stderr with `warning:` prefix; failures contain command and exact resource/download context.

- [ ] **Step 5: Run all local gates**

Run: `go test ./... && go test -race ./... && go vet ./...`

Expected: PASS.

- [ ] **Step 6: Commit complete CLI**

```bash
git add cmd/muamba internal/cli
git commit -m "feat: expose muamba lifecycle CLI"
```

### Task 9: Documentation, CI, and public repository verification

**Files:**
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `.github/workflows/ci.yml`
- Create: `examples/web-assets/muamba.yaml`
- Create: `examples/web-assets/assets/doc.go`
- Create: `examples/web-assets/assets/muamba_gen.go`

**Interfaces:**
- Consumes: final CLI and generated API
- Produces: reproducible public onboarding and CI gates

- [ ] **Step 1: Expand README with exact v1 workflow**

Document tool pinning, grouped downloads, explicit lock, sync, offline verify, version update, strict mode, transport opt-ins, package-scoped generation, integrity formats, and non-goals. Use commands that run against `examples/web-assets`.

- [ ] **Step 2: Add a locked HTTPS example and generated package**

Use small immutable public JS/CSS/license URLs containing exact declared versions. Run Muamba to lock and generate committed example output. Record no Goshtoso metadata or overlay fields.

- [ ] **Step 3: Finalize AGENTS and CI gates**

CI must run:

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/muamba verify --strict -f examples/web-assets/muamba.yaml
go run ./cmd/muamba generate-go --strict --check -f examples/web-assets/muamba.yaml --dir examples/web-assets/assets --output muamba_gen.go
```

- [ ] **Step 4: Run fresh-clone-equivalent verification**

Run `git clean` only in a disposable clone or temporary worktree, then run all CI commands with no local tool cache assumptions. Confirm generated example compiles and verification performs no network access.

- [ ] **Step 5: Review changed files and secrets**

Run: `git diff --check && git status --short && git grep -nE '(gho_|github_pat_|Authorization:|BEGIN .*PRIVATE KEY)' -- . ':!docs/superpowers'`

Expected: no whitespace errors, unintended files, or credentials.

- [ ] **Step 6: Commit documentation and CI**

```bash
git add README.md AGENTS.md .github/workflows/ci.yml examples
git commit -m "docs: add muamba v1 workflow"
```

- [ ] **Step 7: Push and verify GitHub Actions**

Run:

```bash
git push -u origin main
gh run list --repo araihu/muamba --limit 5
```

Wait for the pushed workflow to finish. Require every job green before reporting implementation complete.

## Final acceptance

- [ ] `go test ./...` passes.
- [ ] `go test -race ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] Example `verify --strict` passes offline.
- [ ] Example `generate-go --check --strict` passes.
- [ ] Git working tree is clean.
- [ ] Public `https://github.com/araihu/muamba` exists with `main` pushed.
- [ ] GitHub Actions for final commit is green.
- [ ] No Goshtoso overlay or attribution implementation entered Muamba.
