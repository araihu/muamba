package transport

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPRequiresExplicitAllowance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer server.Close()
	client, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err == nil || !strings.Contains(err.Error(), "HTTP requires") {
		t.Fatalf("Fetch error = %v", err)
	}
	client, err = New(Options{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := client.Fetch(context.Background(), server.URL, &out, 0); err != nil || out.String() != "ok" {
		t.Fatalf("Fetch = %q, %v", out.String(), err)
	}
}

func TestTLSRequiresTrustOrExplicitBypass(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secure")) }))
	defer server.Close()
	client, _ := New(Options{})
	if _, err := client.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err == nil {
		t.Fatal("expected untrusted certificate error")
	}

	certPath := filepath.Join(t.TempDir(), "ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(certPath, cert, 0o600); err != nil {
		t.Fatal(err)
	}
	trusted, err := New(Options{CAFile: certPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trusted.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err != nil {
		t.Fatal(err)
	}
	insecure, err := New(Options{InsecureSkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insecure.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err != nil {
		t.Fatal(err)
	}
}

func TestFetchEnforcesSizeAndHidesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("SECRET RESPONSE BODY"))
			return
		}
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()
	client, _ := New(Options{AllowHTTP: true, MaxBytes: 4})
	if _, err := client.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("size error = %v", err)
	}
	_, err := client.Fetch(context.Background(), server.URL+"/error", &bytes.Buffer{}, 0)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("status error = %v", err)
	}
}

func TestFetchHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer server.Close()
	client, _ := New(Options{AllowHTTP: true, Timeout: 10 * time.Millisecond})
	if _, err := client.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 0); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestFetchLimitsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var next int
		_, _ = fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/"), "%d", &next)
		http.Redirect(w, r, fmt.Sprintf("/%d", next+1), http.StatusFound)
	}))
	defer server.Close()
	client, _ := New(Options{AllowHTTP: true})
	if _, err := client.Fetch(context.Background(), server.URL+"/0", &bytes.Buffer{}, 0); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestFetchUsesPerRequestSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()
	client, err := New(Options{AllowHTTP: true, MaxBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Fetch(context.Background(), server.URL, &bytes.Buffer{}, 4); err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("Fetch limit error = %v", err)
	}
	var out bytes.Buffer
	if _, err := client.Fetch(context.Background(), server.URL, &out, 5); err != nil || out.String() != "12345" {
		t.Fatalf("Fetch = %q, %v", out.String(), err)
	}
	strict, err := New(Options{AllowHTTP: true, MaxBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	var loosened bytes.Buffer
	if _, err := strict.Fetch(context.Background(), server.URL, &loosened, 5); err != nil || loosened.String() != "12345" {
		t.Fatalf("per-request limit did not override client limit: %q, %v", loosened.String(), err)
	}
}

func TestFetchUsesDefaultLimitWhenBothLimitsAreUnset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("complete response"))
	}))
	defer server.Close()
	client, err := New(Options{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := client.Fetch(context.Background(), server.URL, &out, 0); err != nil || out.String() != "complete response" {
		t.Fatalf("Fetch = %q, %v", out.String(), err)
	}
}

func TestFetchHandlesMaximumSizeLimitWithoutOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("complete response"))
	}))
	defer server.Close()
	client, err := New(Options{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := client.Fetch(context.Background(), server.URL, &out, math.MaxInt64); err != nil || out.String() != "complete response" {
		t.Fatalf("Fetch = %q, %v", out.String(), err)
	}
}
