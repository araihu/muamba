package manifest

import "testing"

func TestSelect(t *testing.T) {
	body := validManifest + `  htmx:
    version: "2.0.8"
    downloads:
      core-js:
        url: https://cdn.example/htmx@${version}/htmx.js
        path: assets/htmx/${version}/htmx.js
`
	doc, err := Load(writeManifest(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(false); err != nil {
		t.Fatal(err)
	}
	all, err := doc.Select(nil)
	if err != nil || len(all) != 2 || all[0].ResourceName != "alpine" || all[1].ResourceName != "htmx" {
		t.Fatalf("Select(all) = %#v, %v", all, err)
	}
	one, err := doc.Select([]string{"htmx/core-js"})
	if err != nil || len(one) != 1 || one[0].ResourceName != "htmx" {
		t.Fatalf("Select(one) = %#v, %v", one, err)
	}
	if _, err := doc.Select([]string{"missing"}); err == nil {
		t.Fatal("expected unknown resource error")
	}
	if _, err := doc.Select([]string{"alpine/missing"}); err == nil {
		t.Fatal("expected unknown download error")
	}
}
