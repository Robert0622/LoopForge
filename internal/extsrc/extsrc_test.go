package extsrc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	got := Split(" a.json, ,https://x/y\n b ")
	want := []string{"a.json", "https://x/y", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Split = %q, want %q", got, want)
	}
	if Split("  ") != nil {
		t.Fatal("blank input should yield no sources")
	}
}

func TestLoadDirectoryFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"b.json": "2", "a.json": "1", "c.txt": "x", ".hidden.json": "h"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	docs, err := Load(context.Background(), dir, ".json")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || string(docs[0].Data) != "1" || string(docs[1].Data) != "2" {
		t.Fatalf("docs = %+v", docs)
	}
}

func TestLoadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	docs, err := Load(context.Background(), srv.URL+"/catalog.json")
	if err != nil || len(docs) != 1 || string(docs[0].Data) != `{"items":[]}` {
		t.Fatalf("docs=%+v err=%v", docs, err)
	}
	if _, err := LoadAll(context.Background(), []string{srv.URL + "/missing"}); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("missing URL error = %v", err)
	}
}

func TestLoadRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(path, make([]byte, MaxDocumentBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), path); err == nil {
		t.Fatal("expected size error")
	}
}

func TestJSONList(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want int
	}{
		"array":   {`[{"id":"a"},{"id":"b"}]`, 2},
		"wrapped": {`{"items":[{"id":"a"}]}`, 1},
		"single":  {`{"id":"a"}`, 1},
	} {
		list, err := JSONList([]byte(tc.in), "items")
		if err != nil || len(list) != tc.want {
			t.Fatalf("%s: len=%d err=%v", name, len(list), err)
		}
	}
	if _, err := JSONList([]byte("  "), "items"); err == nil {
		t.Fatal("empty document should fail")
	}
	if _, err := JSONList([]byte(`{"items":{}}`), "items"); err == nil {
		t.Fatal("non-array items should fail")
	}
}
