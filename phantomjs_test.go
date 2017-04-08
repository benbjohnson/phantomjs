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

// Ensure process can set and retrieve custom headers.
func TestWebPage_CustomHeaders(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Test data.
	hdr := make(http.Header)
	hdr.Set("FOO", "BAR")
	hdr.Set("BAZ", "BAT")

	// Set the headers.
	page.SetCustomHeaders(hdr)

	// Retrieve and verify the headers.
	if other := page.CustomHeaders(); !reflect.DeepEqual(other, hdr) {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can return the name of the currently focused frame.
func TestWebPage_FocusedFrameName(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FRAME 1</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body><input autofocus/></body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Retrieve the focused frame.
	if other := page.FocusedFrameName(); other != "FRAME2" {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can set and retrieve frame content.
func TestWebPage_FrameContent(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Switch to frame and update content.
	page.SwitchToFrameName("FRAME2")
	page.SetFrameContent(`<html><body>NEW CONTENT</body></html>`)

	if other := page.FrameContent(); other != `<html><head></head><body>NEW CONTENT</body></html>` {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can retrieve the current frame name.
func TestWebPage_FrameName(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Switch to frame and retrieve name.
	page.SwitchToFrameName("FRAME2")
	if other := page.FrameName(); other != `FRAME2` {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can retrieve frame content as plain text.
func TestWebPage_FramePlainText(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Switch to frame and update content.
	page.SwitchToFrameName("FRAME2")
	if other := page.FramePlainText(); other != `BAR` {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can retrieve the frame title.
func TestWebPage_FrameTitle(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><head><title>TEST TITLE</title><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Switch to frame and verify title.
	page.SwitchToFrameName("FRAME2")
	if other := page.FrameTitle(); other != `TEST TITLE` {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can retrieve the frame URL.
func TestWebPage_FrameURL(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Switch to frame and verify title.
	page.SwitchToFramePosition(1)
	if other := page.FrameURL(); other != srv.URL+`/frame2.html` {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure web page can retrieve the total frame count.
func TestWebPage_FrameCount(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Verify frame count.
	if n := page.FrameCount(); n != 2 {
		t.Fatalf("unexpected value: %#v", n)
	}
}

// Ensure web page can retrieve a list of frame names.
func TestWebPage_FrameNames(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><frameset rows="*,*"><frame name="FRAME1" src="/frame1.html"/><frame name="FRAME2" src="/frame2.html"/></frameset></html>`))
		case "/frame1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/frame2.html":
			w.Write([]byte(`<html><body>BAR</body></html>`))
		default:
			http.NotFound(w, r)
		}
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
	}

	// Verify frame count.
	if other := page.FrameNames(); !reflect.DeepEqual(other, []string{"FRAME1", "FRAME2"}) {
		t.Fatalf("unexpected value: %#v", other)
	}
}

// Ensure process can set and retrieve the library path.
func TestWebPage_LibraryPath(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Verify initial path is equal to process path.
	if v := page.LibraryPath(); v != p.Path() {
		t.Fatalf("unexpected path: %s", v)
	}

	// Set the library path & verify it changed.
	page.SetLibraryPath("/tmp")
	if v := page.LibraryPath(); v != `/tmp` {
		t.Fatalf("unexpected path: %s", v)
	}
}

// Ensure process can set and retrieve whether the navigation is locked.
func TestWebPage_NavigationLocked(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set the navigation lock & verify it changed.
	page.SetNavigationLocked(true)
	if !page.NavigationLocked() {
		t.Fatal("expected navigation locked")
	}
}

// Ensure process can retrieve the offline storage path.
func TestWebPage_OfflineStoragePath(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Retrieve storage path and ensure it's not blank.
	if v := page.OfflineStoragePath(); v == `` {
		t.Fatal("expected path")
	}
}

// Ensure process can set and retrieve the offline storage quota.
func TestWebPage_OfflineStorageQuota(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Retrieve storage quota and ensure it's non-zero.
	if v := page.OfflineStorageQuota(); v == 0 {
		t.Fatal("expected quota")
	}
}

// Ensure web page can open a URL.
func TestWebPage_Open(t *testing.T) {
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
