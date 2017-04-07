package phantomjs_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/middlemost/phantomjs"
)

// Ensure web page can return whether it can navigate forward.
func TestWebPage_CanGoForward(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()
	if page.CanGoForward() {
		t.Fatal("expected false")
	}
}

// Ensure process can return if the page can navigate back.
func TestWebPage_CanGoBack(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()
	if page.CanGoBack() {
		t.Fatal("expected false")
	}
}

// Ensure process can set and retrieve the clipping rectangle.
func TestWebPage_ClipRect(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Clipping rectangle should be empty initially.
	if v := page.ClipRect(); v != (phantomjs.Rect{}) {
		t.Fatalf("expected empty rect: %#v", v)
	}

	// Set a rectangle.
	rect := phantomjs.Rect{Top: 1, Left: 2, Width: 3, Height: 4}
	page.SetClipRect(rect)
	if v := page.ClipRect(); !reflect.DeepEqual(v, rect) {
		t.Fatalf("unexpected value: %#v", v)
	}
}

// Ensure web page can return its contents.
func TestWebPage_Content(t *testing.T) {
	// Serve web page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer srv.Close()

	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	} else if content := page.Content(); content != `<html><head></head><body>OK</body></html>` {
		t.Fatalf("unexpected content: %q", content)
	}
}

// Process is a test wrapper for phantomjs.Process.
type Process struct {
	*phantomjs.Process
}

// NewProcess returns a new, open Process.
func NewProcess() *Process {
	return &Process{Process: phantomjs.NewProcess()}
}

// MustOpenNewProcess returns a new, open Process. Panic on error.
func MustOpenNewProcess() *Process {
	p := NewProcess()
	if err := p.Open(); err != nil {
		panic(err)
	}
	return p
}

// MustClose closes the process. Panic on error.
func (p *Process) MustClose() {
	if err := p.Close(); err != nil {
		panic(err)
	}
}
