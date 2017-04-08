package phantomjs_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

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

// Ensure process can set and retrieve cookies.
func TestWebPage_Cookies(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Test data.
	cookies := []*http.Cookie{
		{
			Domain:   ".example1.com",
			HttpOnly: true,
			Name:     "NAME1",
			Path:     "/",
			Secure:   true,
			Value:    "VALUE1",
		},
		{
			Domain:   ".example2.com",
			Expires:  time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC),
			HttpOnly: false,
			Name:     "NAME2",
			Path:     "/path",
			Secure:   false,
			Value:    "VALUE2",
		},
	}

	// Set the cookies.
	page.SetCookies(cookies)

	// Cookie with expiration should have string version set on return.
	cookies[1].RawExpires = "Thu, 02 Jan 2020 03:04:05 GMT"

	// Retrieve and verify the cookies.
	if other := page.Cookies(); len(other) != 2 {
		t.Fatalf("unexpected cookie count: %d", len(other))
	} else if !reflect.DeepEqual(other[0], cookies[0]) {
		t.Fatalf("unexpected cookie(0): %#v", other[0])
	} else if !reflect.DeepEqual(other[1], cookies[1]) {
		t.Fatalf("unexpected cookie(1): %#v\n%#v", other[1], cookies[1])
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
