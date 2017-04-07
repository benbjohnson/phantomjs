package phantomjs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/middlemost/phantomjs"
)

// Ensure process can create a webpage.
func TestProcess_WebPage(t *testing.T) {
	ctx := context.Background()

	// Serve web page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer srv.Close()

	// Start process.
	p := phantomjs.NewProcess()
	if err := p.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Create page.
	page := p.CreateWebPage(ctx)
	defer page.Close()

	// Open a web page.
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	} else if content := page.Content(); content != `<html><head></head><body>OK</body></html>` {
		t.Fatalf("unexpected content: %q", content)
	}
}
