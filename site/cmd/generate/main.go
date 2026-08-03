package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"

	"github.com/a-h/templ"
	shellassets "github.com/araihu/goshtoso-app-shells/componentdocshell/assets"
	landingassets "github.com/araihu/goshtoso-app-shells/landingshell/assets"
	"github.com/araihu/goshtoso/assets"
	"github.com/araihu/muamba/site/internal/pages"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := render("app/_generated/index.html", pages.LandingPage()); err != nil {
		return err
	}
	if err := render("app/_generated/docs/index.html", pages.DocsPage()); err != nil {
		return err
	}

	manifest := assets.DefaultRuntimeManifest()
	if err := extract(assets.Handler(), "public", manifest.Stylesheet.LocalURL); err != nil {
		return err
	}
	for _, dependency := range manifest.Dependencies {
		if dependency.Enabled {
			if err := extract(assets.Handler(), "public", dependency.LocalURL); err != nil {
				return err
			}
		}
	}
	for _, assetURL := range []string{
		shellassets.StylesheetURL(""),
		shellassets.ScriptURL(""),
		shellassets.AraiHuThemeURL(""),
	} {
		if err := extract(shellassets.Handler(), "public", assetURL); err != nil {
			return err
		}
	}
	for _, assetURL := range []string{
		landingassets.StylesheetURL(""),
		landingassets.ScriptURL(""),
	} {
		if err := extract(landingassets.Handler(), "public", assetURL); err != nil {
			return err
		}
	}
	return nil
}
func render(name string, component templ.Component) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := component.Render(context.Background(), file); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	return nil
}

func extract(handler http.Handler, root, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	request := httptest.NewRequest(http.MethodGet, parsed.Path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("extract %s: status %d", parsed.Path, recorder.Code)
	}
	destination := filepath.Join(root, filepath.FromSlash(parsed.Path))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, recorder.Body.Bytes(), 0o644)
}
