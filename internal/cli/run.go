package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/araihu/muamba/internal/gogen"
	"github.com/araihu/muamba/internal/lifecycle"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

const helpText = `Usage: muamba <command> [options]

Commands:
  lock [RESOURCE[/DOWNLOAD] ...]       Trust unlocked remote bytes
  sync [RESOURCE[/DOWNLOAD] ...]       Restore locked files
  verify [RESOURCE[/DOWNLOAD] ...]     Verify local files offline
  update RESOURCE --version VERSION    Trust a new grouped version
  update RESOURCE/DOWNLOAD             Re-trust one current URL
  generate-go --dir DIR --output FILE  Generate an embedded Go registry
  help

Common options:
  -f FILE                         Manifest path (default: find muamba.yaml)
  --strict                       Promote manifest warnings to errors
  --allow-http                   Allow explicitly insecure HTTP URLs
  --insecure-skip-tls-verify     Allow invalid HTTPS certificates
  --ca-file FILE                 Add trusted PEM certificates
  --timeout DURATION             Download timeout (default: 1m)
  --max-size BYTES               Maximum response size (default: 104857600)
  --target GOOS/GOARCH           Materialization target (default: runtime)
  --cache-dir DIR                Integrity cache (default: MUAMBA_CACHE_DIR or OS cache)
  --all-platforms                Verify every cached variant (verify only)

Environment:
  MUAMBA_CACHE_DIR               Default integrity cache directory
`

type commonFlags struct {
	manifestPath          string
	strict                bool
	allowHTTP             bool
	insecureSkipTLSVerify bool
	caFile                string
	timeout               time.Duration
	targetRaw             string
	target                manifest.Target
	cacheDir              string
	maxSize               optionalInt64
}

type optionalInt64 struct {
	value int64
	set   bool
}

func (value *optionalInt64) Set(raw string) error {
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("max-size must be a positive byte count")
	}
	value.value, value.set = parsed, true
	return nil
}

func (value *optionalInt64) String() string {
	if !value.set {
		return ""
	}
	return strconv.FormatInt(value.value, 10)
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		_, _ = fmt.Fprint(stdout, helpText)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, helpText)
		return 0
	case "lock", "sync", "verify":
		return runLifecycle(args[0], args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "generate-go":
		return runGenerate(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		_, _ = fmt.Fprint(stderr, helpText)
		return 2
	}
}

func runLifecycle(command string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	common := addCommonFlags(flags)
	allPlatforms := false
	if command == "verify" {
		flags.BoolVar(&allPlatforms, "all-platforms", false, "verify every cached platform variant")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := common.resolveTarget(); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", command, err)
		return 2
	}
	engine, err := newEngine(common)
	if err != nil {
		return operationError(command, err, stderr)
	}
	var report lifecycle.Report
	switch command {
	case "lock":
		report, err = engine.Lock(context.Background(), flags.Args())
	case "sync":
		report, err = engine.Sync(context.Background(), flags.Args())
	case "verify":
		report, err = engine.Verify(context.Background(), flags.Args(), allPlatforms)
	}
	emitReport(report, stdout, stderr)
	if err != nil {
		return operationError(command, err, stderr)
	}
	return 0
}

func runUpdate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		_, _ = fmt.Fprintln(stderr, "usage: muamba update RESOURCE[/DOWNLOAD] [--version VERSION] [options]")
		return 2
	}
	selector := args[0]
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(stderr)
	common := addCommonFlags(flags)
	version := flags.String("version", "", "new resource version")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if err := common.resolveTarget(); err != nil {
		_, _ = fmt.Fprintf(stderr, "update: %v\n", err)
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "update: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	parts := strings.Split(selector, "/")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] == "" {
		_, _ = fmt.Fprintf(stderr, "update: invalid selector %q\n", selector)
		return 2
	}
	if len(parts) == 1 && *version == "" {
		_, _ = fmt.Fprintln(stderr, "update: resource update requires --version")
		return 2
	}
	if len(parts) == 2 && *version != "" {
		_, _ = fmt.Fprintln(stderr, "update: single-download update does not accept --version")
		return 2
	}
	engine, err := newEngine(common)
	if err != nil {
		return operationError("update", err, stderr)
	}
	var report lifecycle.Report
	if len(parts) == 1 {
		report, err = engine.UpdateResource(context.Background(), parts[0], *version)
	} else {
		report, err = engine.UpdateDownload(context.Background(), parts[0], parts[1])
	}
	emitReport(report, stdout, stderr)
	if err != nil {
		return operationError("update", err, stderr)
	}
	return 0
}

func runGenerate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("generate-go", flag.ContinueOnError)
	flags.SetOutput(stderr)
	common := addCommonFlags(flags)
	dir := flags.String("dir", "", "package directory relative to the manifest")
	output := flags.String("output", "", "generated filename inside the package directory")
	packageName := flags.String("package", "", "Go package name (inferred when possible)")
	check := flags.Bool("check", false, "fail if generated output is stale")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := common.resolveTarget(); err != nil {
		_, _ = fmt.Fprintf(stderr, "generate-go: %v\n", err)
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "generate-go: unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	if *dir == "" || *output == "" {
		_, _ = fmt.Fprintln(stderr, "generate-go: --dir and --output are required")
		return 2
	}
	path, err := findManifest(common.manifestPath)
	if err != nil {
		return operationError("generate-go", err, stderr)
	}
	document, err := manifest.Load(path)
	if err != nil {
		return operationError("generate-go", err, stderr)
	}
	warnings, err := document.Validate(common.strict)
	if err != nil {
		return operationError("generate-go", err, stderr)
	}
	emitWarnings(warnings, stderr)
	err = gogen.Generate(document, gogen.Options{
		Dir: *dir, Output: *output, Package: *packageName, Check: *check, Strict: common.strict, Target: common.target,
	})
	if err != nil {
		return operationError("generate-go", err, stderr)
	}
	if !*check {
		_, _ = fmt.Fprintf(stdout, "generated %s/%s\n", strings.TrimSuffix(*dir, "/"), *output)
	}
	return 0
}

func addCommonFlags(flags *flag.FlagSet) *commonFlags {
	common := &commonFlags{}
	flags.StringVar(&common.manifestPath, "f", "", "manifest path")
	flags.BoolVar(&common.strict, "strict", false, "promote manifest warnings to errors")
	flags.BoolVar(&common.allowHTTP, "allow-http", false, "allow HTTP URLs")
	flags.BoolVar(&common.insecureSkipTLSVerify, "insecure-skip-tls-verify", false, "skip TLS certificate verification")
	flags.StringVar(&common.caFile, "ca-file", "", "trusted PEM certificate file")
	flags.DurationVar(&common.timeout, "timeout", time.Minute, "download timeout")
	flags.Var(&common.maxSize, "max-size", "maximum response size in bytes")
	flags.StringVar(&common.targetRaw, "target", "", "materialization target GOOS/GOARCH")
	flags.StringVar(&common.cacheDir, "cache-dir", "", "integrity cache directory")
	return common
}

func (common *commonFlags) resolveTarget() error {
	if common.targetRaw == "" {
		common.target = manifest.RuntimeTarget()
		return nil
	}
	target, err := manifest.ParseTarget(common.targetRaw)
	if err != nil {
		return err
	}
	common.target = target
	return nil
}

func newEngine(common *commonFlags) (*lifecycle.Engine, error) {
	path, err := findManifest(common.manifestPath)
	if err != nil {
		return nil, err
	}
	return lifecycle.New(path, lifecycle.Options{
		Strict:      common.strict,
		Target:      common.target,
		CacheDir:    common.cacheDir,
		MaxBytes:    common.maxSize.value,
		MaxBytesSet: common.maxSize.set,
		Transport: transport.Options{
			AllowHTTP:             common.allowHTTP,
			InsecureSkipTLSVerify: common.insecureSkipTLSVerify,
			CAFile:                common.caFile,
			Timeout:               common.timeout,
		},
	})
}

func findManifest(explicit string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return manifest.Find(cwd, explicit)
}

func emitReport(report lifecycle.Report, stdout, stderr io.Writer) {
	emitWarnings(report.Warnings, stderr)
	for _, selection := range report.Changed {
		_, _ = fmt.Fprintf(stdout, "changed %s\n", selection)
	}
	for _, selection := range report.Verified {
		_, _ = fmt.Fprintf(stdout, "verified %s\n", selection)
	}
}

func emitWarnings(warnings []manifest.Warning, stderr io.Writer) {
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(stderr, "warning: %s/%s: %s\n", warning.Resource, warning.Download, warning.Message)
	}
}

func operationError(command string, err error, stderr io.Writer) int {
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", command, err)
	return 1
}
