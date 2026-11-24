# Muamba compatibility adaptation

This directory preserves the source and Apache-2.0 license of
[`github.com/dagger/otel-go` v1.43.0](https://github.com/dagger/otel-go/tree/v1.43.0).
The original module checksum is
`h1:AYCnAamWmxtSxigWPTgC+8EWqiWPcDZEegh8y05gdJ8=`.
This is a local compatibility adaptation of the Dagger SDK's
telemetry bridge; the OpenTelemetry libraries themselves remain upstream modules.

The v1.43.0 bridge used `log.Value` and `log.KeyValue`. This adaptation uses
`attribute.Value` and `attribute.KeyValue` and preserves Muamba's log-body
and OTLP record round-trip behavior with the stable Logs API.

Local changes are limited to:

- `logging.go`: use unified attribute types for stdio and structured log bodies.
- `transform.go`: migrate OTLP conversion and preserve empty bodies, byte values,
  homogeneous arrays, mixed arrays and maps with the unified value model.
- `go.mod`/`go.sum`: select OpenTelemetry core, SDK, trace and Logs v1.47.0,
  OTLP log exporters v0.23.0, and x/net v0.60.0.
- `compat_test.go`: check value round trips, stdio/EOF records, structured logs,
  trace context, resources, and OTLP export/reexport through the new SDK.

All other Go source files are unchanged from the published version. No generated
Dagger client files are patched. `.dagger/go.mod` selects this module through a
local replacement, and the Dagger check includes it in its isolated SDK source.
The required module check runs its tests alongside the SDK contract tests.

Before removing the local replacement, run the compatibility tests against the
candidate upstream bridge. Check empty and missing log bodies and record round
trips, then run the complete `dagger call check`.
