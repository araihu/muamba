package otel

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

func TestLogValueRoundTripWithUnifiedAttributes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value attribute.Value
	}{
		{"empty", attribute.Value{}},
		{"bool", attribute.BoolValue(true)},
		{"int", attribute.Int64Value(42)},
		{"float", attribute.Float64Value(1.5)},
		{"string", attribute.StringValue("build output")},
		{"bytes", attribute.ByteSliceValue([]byte{0, 0xff, 0x42})},
		{"bool-array", attribute.BoolSliceValue([]bool{true, false})},
		{"int-array", attribute.Int64SliceValue([]int64{1, 2})},
		{"float-array", attribute.Float64SliceValue([]float64{1.5, 2.5})},
		{"string-array", attribute.StringSliceValue([]string{"a", "b"})},
		{"mixed-array", attribute.SliceValue(attribute.StringValue("a"), attribute.Int64Value(2))},
		{"nested-map", attribute.MapValue(
			attribute.KeyValue{Key: "bytes", Value: attribute.ByteSliceValue([]byte{0xff})},
			attribute.KeyValue{Key: "nested", Value: attribute.MapValue(attribute.String("key", "value"))},
		)},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire := LogValueToPB(test.value)
			if wire.GetStringValue() == "INVALID" {
				t.Fatal("value was lost during conversion")
			}
			if test.value.Type() == attribute.BYTESLICE && !bytes.Equal(wire.GetBytesValue(), test.value.AsByteSlice()) {
				t.Fatal("binary body was not preserved")
			}
			if !proto.Equal(wire, LogValueToPB(LogValueFromPB(wire))) {
				t.Fatalf("OTLP round trip changed %s", test.name)
			}
		})
	}
	if LogValueFromPB(nil).Type() != attribute.EMPTY {
		t.Fatal("missing body must remain empty")
	}
}

func TestStdioAndStructuredLogsSurviveExportAndReexport(t *testing.T) {
	t.Parallel()
	exporter := &recordExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", "moco-build"))),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx := WithLoggerProvider(trace.ContextWithSpanContext(t.Context(), span), provider)
	streams := SpanStdio(ctx, "build", attribute.String("build.id", "recovery"))
	if _, err := io.WriteString(streams.Stdout, "stdout"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(streams.Stderr, "stderr"); err != nil {
		t.Fatal(err)
	}
	if err := streams.Close(); err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(ctx, "build").(*Writer)
	writer.Emit(attribute.MapValue(attribute.String("stage", "check")),
		attribute.KeyValue{Key: "details", Value: attribute.SliceValue(attribute.StringValue("ok"), attribute.BoolValue(true))})
	records := exporter.snapshot()
	if len(records) != 5 {
		t.Fatalf("got %d records, want output, EOF and structured records", len(records))
	}
	for i, body := range []string{"stdout", "stderr", "", ""} {
		if records[i].Body().AsString() != body {
			t.Fatalf("stdio body %d changed", i)
		}
		attrs := map[string]attribute.Value{}
		records[i].WalkAttributes(func(kv attribute.KeyValue) bool { attrs[string(kv.Key)] = kv.Value; return true })
		if attrs[StdioStreamAttr].AsInt64() != int64(i%2+1) || attrs[StdioEOFAttr].AsBool() != (i >= 2) {
			t.Fatalf("stdio stream or EOF marker %d changed", i)
		}
		if attrs["build.id"].AsString() != "recovery" {
			t.Fatal("build attribute was lost")
		}
	}
	replayed := &recordExporter{}
	if err := ReexportLogsFromPB(t.Context(), replayed, &collogspb.ExportLogsServiceRequest{ResourceLogs: LogsToPB(records)}); err != nil {
		t.Fatal(err)
	}
	recovered := replayed.snapshot()
	if len(recovered) != len(records) {
		t.Fatal("reexport dropped records")
	}
	for i := range records {
		if !proto.Equal(logRecord(records[i]), logRecord(recovered[i])) {
			t.Fatalf("reexport changed record %d", i)
		}
		if recovered[i].TraceID() != span.TraceID() || recovered[i].SpanID() != span.SpanID() {
			t.Fatal("trace context was lost")
		}
		if recovered[i].Resource().Equivalent() != records[i].Resource().Equivalent() || recovered[i].InstrumentationScope().Name != "build" {
			t.Fatal("resource or instrumentation scope was lost")
		}
	}
}

type recordExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *recordExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

func (*recordExporter) Shutdown(context.Context) error   { return nil }
func (*recordExporter) ForceFlush(context.Context) error { return nil }

func (e *recordExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]sdklog.Record, len(e.records))
	for i := range e.records {
		result[i] = e.records[i].Clone()
	}
	return result
}
