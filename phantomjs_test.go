package phantomjs_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

// Ensure process can set and retrieve whether the page owns other opened pages.
func TestWebPage_OwnsPages(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set value & verify it changed.
	page.SetOwnsPages(true)
	if !page.OwnsPages() {
		t.Fatal("expected true")
	}
}

// Ensure process can retrieve a list of window names opened by the page.
func TestWebPage_PageWindowNames(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set content to open windows.
	page.SetOwnsPages(true)
	page.SetContent(`<html><body><a id="link" target="win1" href="/win1.html">CLICK ME</a></body></html>`)

	// Click the link.
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)

	// Retrieve a list of window names.
	if names := page.PageWindowNames(); !reflect.DeepEqual(names, []string{"win1"}) {
		t.Fatalf("unexpected names: %+v", names)
	}
}

// Ensure process can retrieve a list of owned web pages.
func TestWebPage_Pages(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><body><a id="link" target="win1" href="/win1.html">CLICK ME</a></body></html>`))
		case "/win1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Open root page.
	page.SetOwnsPages(true)
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	}

	// Click the link.
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)

	// Retrieve a list of window names.
	if pages := page.Pages(); len(pages) != 1 {
		t.Fatalf("unexpected count: %d", len(pages))
	} else if u := pages[0].URL(); u != srv.URL+`/win1.html` {
		t.Fatalf("unexpected url: %s", u)
	} else if name := pages[0].WindowName(); name != `win1` {
		t.Fatalf("unexpected window name: %s", name)
	}
}

// Ensure process can set and retrieve the sizing options used for printing.
func TestWebPage_PaperSize(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Ensure initial size is the zero value.
	t.Run("Initial", func(t *testing.T) {
		page := p.CreateWebPage()
		defer page.Close()

		if sz := page.PaperSize(); !reflect.DeepEqual(sz, phantomjs.PaperSize{}) {
			t.Fatalf("unexpected size: %#v", sz)
		}
	})

	// Ensure width/height can be set.
	t.Run("WidthHeight", func(t *testing.T) {
		page := p.CreateWebPage()
		defer page.Close()

		sz := phantomjs.PaperSize{Width: "5in", Height: "10in"}
		page.SetPaperSize(sz)
		if other := page.PaperSize(); !reflect.DeepEqual(other, sz) {
			t.Fatalf("unexpected size: %#v", other)
		}
	})

	// Ensure format can be set.
	t.Run("Format", func(t *testing.T) {
		page := p.CreateWebPage()
		defer page.Close()

		sz := phantomjs.PaperSize{Format: "A4"}
		page.SetPaperSize(sz)
		if other := page.PaperSize(); !reflect.DeepEqual(other, sz) {
			t.Fatalf("unexpected size: %#v", other)
		}
	})

	// Ensure orientation can be set.
	t.Run("Orientation", func(t *testing.T) {
		page := p.CreateWebPage()
		defer page.Close()

		sz := phantomjs.PaperSize{Orientation: "landscape"}
		page.SetPaperSize(sz)
		if other := page.PaperSize(); !reflect.DeepEqual(other, sz) {
			t.Fatalf("unexpected size: %#v", other)
		}
	})

	// Ensure margins can be set.
	t.Run("Margin", func(t *testing.T) {
		page := p.CreateWebPage()
		defer page.Close()

		sz := phantomjs.PaperSize{
			Margin: &phantomjs.PaperSizeMargin{
				Top:    "1in",
				Bottom: "2in",
				Left:   "3in",
				Right:  "4in",
			},
		}
		page.SetPaperSize(sz)
		if other := page.PaperSize(); !reflect.DeepEqual(other, sz) {
			t.Fatalf("unexpected size: %#v", other)
		}
	})
}

// Ensure process can retrieve the plain text representation of a page.
func TestWebPage_PlainText(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set content & verify plain text.
	page.SetContent(`<html><body>FOO</body></html>`)
	if v := page.PlainText(); v != `FOO` {
		t.Fatalf("unexpected plain text: %s", v)
	}
}

// Ensure process can set and retrieve the scroll position of the page.
func TestWebPage_ScrollPosition(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set and verify position.
	pos := phantomjs.Position{Top: 10, Left: 20}
	page.SetScrollPosition(pos)
	if other := page.ScrollPosition(); !reflect.DeepEqual(other, pos) {
		t.Fatalf("unexpected position: %#v", pos)
	}
}

// Ensure process can set and retrieve page settings.
func TestWebPage_Settings(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set and verify settings.
	settings := phantomjs.WebPageSettings{
		JavascriptEnabled:             true,
		LoadImages:                    true,
		LocalToRemoteURLAccessEnabled: true,
		UserAgent:                     "Mozilla/5.0",
		Username:                      "susy",
		Password:                      "pass",
		XSSAuditingEnabled:            true,
		WebSecurityEnabled:            true,
		ResourceTimeout:               10 * time.Second,
	}
	page.SetSettings(settings)
	if other := page.Settings(); !reflect.DeepEqual(other, settings) {
		t.Fatalf("unexpected settings: %#v", other)
	}
}

// Ensure process can retrieve the title of a page.
func TestWebPage_Title(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set & verify title.
	page.SetContent(`<html><head><title>FOO</title></head><body>BAR</body></html>`)
	if v := page.Title(); v != `FOO` {
		t.Fatalf("unexpected plain text: %s", v)
	}
}

// Ensure process can set and retrieve the viewport size.
func TestWebPage_ViewportSize(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set and verify size.
	page.SetViewportSize(100, 200)
	if w, h := page.ViewportSize(); w != 100 || h != 200 {
		t.Fatalf("unexpected size: w=%d, h=%d", w, h)
	}
}

// Ensure process can set and retrieve the zoom factor on the page.
func TestWebPage_ZoomFactor(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set factor & verify it changed.
	page.SetZoomFactor(2.5)
	if v := page.ZoomFactor(); v != 2.5 {
		t.Fatalf("unexpected zoom factor: %f", v)
	}
}

// Ensure process can add a cookie to the page.
func TestWebPage_AddCookie(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Test data.
	cookie := &http.Cookie{
		Domain:   ".example1.com",
		HttpOnly: true,
		Name:     "NAME1",
		Path:     "/",
		Secure:   true,
		Value:    "VALUE1",
	}

	// Add the cookie.
	if !page.AddCookie(cookie) {
		t.Fatal("could not add cookie")
	}

	// Retrieve and verify the cookies.
	if other := page.Cookies(); len(other) != 1 {
		t.Fatalf("unexpected cookie count: %d", len(other))
	} else if !reflect.DeepEqual(other[0], cookie) {
		t.Fatalf("unexpected cookie(0): %#v", other)
	}
}

// Ensure process can clear all cookies on the page.
func TestWebPage_ClearCookies(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Add a cookie.
	if !page.AddCookie(&http.Cookie{Domain: ".example1.com", Name: "NAME1", Path: "/", Value: "VALUE1"}) {
		t.Fatal("could not add cookie")
	} else if cookies := page.Cookies(); len(cookies) != 1 {
		t.Fatalf("unexpected cookie count: %d", len(cookies))
	}

	// Clear cookies and verify they are gone.
	page.ClearCookies()
	if cookies := page.Cookies(); len(cookies) != 0 {
		t.Fatalf("unexpected cookie count: %d", len(cookies))
	}
}

// Ensure process can delete a single cookie on the page.
func TestWebPage_DeleteCookie(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Add a cookies.
	if !page.AddCookie(&http.Cookie{Domain: ".example1.com", Name: "NAME1", Path: "/", Value: "VALUE1"}) {
		t.Fatal("could not add cookie")
	} else if !page.AddCookie(&http.Cookie{Domain: ".example1.com", Name: "NAME2", Path: "/", Value: "VALUE2"}) {
		t.Fatal("could not add cookie")
	} else if cookies := page.Cookies(); len(cookies) != 2 {
		t.Fatalf("unexpected cookie count: %d", len(cookies))
	}

	// Delete first cookie.
	if !page.DeleteCookie("NAME1") {
		t.Fatal("could not delete cookie")
	} else if cookies := page.Cookies(); len(cookies) != 1 {
		t.Fatalf("unexpected cookie count: %d", len(cookies))
	} else if cookies[0].Name != "NAME2" {
		t.Fatalf("unexpected cookie(0) name: %s", cookies[0].Name)
	}
}

// Ensure process can execute JavaScript asynchronously.
// This function relies on time so it is inherently flakey.
func TestWebPage_EvaluateAsync(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Execute after one second.
	page.EvaluateAsync(`function() { window.testValue = "OK" }`, 1*time.Second)

	// Value should not be set immediately.
	if value := page.EvaluateJavaScript(`function() { return window.testValue }`); value != nil {
		t.Fatalf("unexpected value: %#v", value)
	}

	// Wait a bit.
	time.Sleep(2 * time.Second)

	// Value should hopefully be set now.
	if value := page.EvaluateJavaScript(`function() { return window.testValue }`); value != "OK" {
		t.Fatalf("unexpected value: %#v", value)
	}
}

// Ensure process can execute JavaScript in the context of a web page.
func TestWebPage_Evaluate(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()
	page.SetContent(`<html><head><title>FOO</title></head><body>BAR</body></html>`)

	// Retrieve title.
	if value := page.EvaluateJavaScript(`function() { return document.title }`); value != "FOO" {
		t.Fatalf("unexpected value: %#v", value)
	}
}

// Ensure process can retrieve a page by window name.
func TestWebPage_Page(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Set content to open windows.
	page.SetOwnsPages(true)
	page.SetContent(`<html><body><a id="link" target="win1" href="/win1.html">CLICK ME</a></body></html>`)

	// Click the link.
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)

	// Retrieve a window by name.
	if childPage := page.Page("win1"); childPage == nil || childPage.WindowName() != "win1" {
		t.Fatalf("unexpected page: %#v", childPage)
	}

	// Non-existent pages should return nil.
	if childPage := page.Page("bad_page"); childPage != nil {
		t.Fatalf("expected nil page: %#v", childPage)
	}
}

// Ensure process can moves forward and back in history.
func TestWebPage_GoBackForward(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><body><a id="link" href="/page1.html">CLICK ME</a></body></html>`))
		case "/page1.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Open root page.
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	}

	// Click the link and verify location.
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)
	if u := page.URL(); u != srv.URL+"/page1.html" {
		t.Fatalf("unexpected page: %s", u)
	}

	// Navigate back & verify location.
	page.GoBack()
	if u := page.URL(); u != srv.URL+"/" {
		t.Fatalf("unexpected page: %s", u)
	}

	// Navigate forward & verify location.
	page.GoForward()
	if u := page.URL(); u != srv.URL+"/page1.html" {
		t.Fatalf("unexpected page: %s", u)
	}
}

// Ensure process can move by relative index.
func TestWebPage_Go(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><body><a id="link" href="/page1.html">CLICK ME</a></body></html>`))
		case "/page1.html":
			w.Write([]byte(`<html><body><a id="link" href="/page2.html">CLICK ME</a></body></html>`))
		case "/page2.html":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Open root page.
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	}

	// Click the links on two pages and verify location.
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)
	page.EvaluateJavaScript(`function() { document.body.querySelector("#link").click() }`)
	if u := page.URL(); u != srv.URL+"/page2.html" {
		t.Fatalf("unexpected page: %s", u)
	}

	// Navigate back & verify location.
	page.Go(-2)
	if u := page.URL(); u != srv.URL+"/" {
		t.Fatalf("unexpected page: %s", u)
	}

	// Navigate forward & verify location.
	page.Go(1)
	if u := page.URL(); u != srv.URL+"/page1.html" {
		t.Fatalf("unexpected page: %s", u)
	}
}

// Ensure process include external scripts.
func TestWebPage_IncludeJS(t *testing.T) {
	// Mock external HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`<html><body>FOO</body></html>`))
		case "/script.js":
			w.Write([]byte(`window.testValue = 'INCLUDED'`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Open root page.
	if err := page.Open(srv.URL); err != nil {
		t.Fatal(err)
	}

	// Include external script.
	page.IncludeJS(srv.URL + "/script.js")

	// Verify that script ran.
	if v := page.Evaluate(`function() { return window.testValue }`); v != "INCLUDED" {
		t.Fatalf("unexpected test value: %#v", v)
	}
}

// Ensure process include local scripts.
func TestWebPage_InjectJS(t *testing.T) {
	p := MustOpenNewProcess()
	defer p.MustClose()

	page := p.CreateWebPage()
	defer page.Close()

	// Write local script.
	if err := ioutil.WriteFile(filepath.Join(p.Path(), "script.js"), []byte(`window.testValue = 'INCLUDED'`), 0600); err != nil {
		t.Fatal(err)
	}

	// Include local script.
	if err := page.InjectJS("script.js"); err != nil {
		t.Fatal(err)
	}

	// Verify that script ran.
	if v := page.Evaluate(`function() { return window.testValue }`); v != "INCLUDED" {
		t.Fatalf("unexpected test value: %#v", v)
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

// Ensure web page can reload a web page.
func TestWebPage_Reload(t *testing.T) {
	// Serve web page.
	var counter int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		fmt.Fprintf(w, "<html><head></head><body>%d</body></html>", counter)
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

	// First time the counter should be 1.
	if content := page.Content(); content != `<html><head></head><body>1</body></html>` {
		t.Fatalf("unexpected content: %q", content)
	}

	// Reload the page and the counter should increment.
	page.Reload()
	if content := page.Content(); content != `<html><head></head><body>2</body></html>` {
		t.Fatalf("unexpected content: %q", content)
	}
}

// Ensure web page can render to a base64 string.
func TestWebPage_RenderBase64(t *testing.T) {
	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	page.SetContent(`<html><head></head><body>TEST</body></html>`)
	page.SetViewportSize(100, 200)

	// Render page.
	data := page.RenderBase64("png")

	// Decode data.
	buf, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}

	// Parse image and verify dimensions.
	img, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	} else if bounds := img.Bounds(); bounds.Max.X != 100 || bounds.Max.Y != 200 {
		t.Fatalf("unexpected image dimesions: %dx%d", bounds.Max.X, bounds.Max.Y)
	}
}

// Ensure web page can render to a file.
func TestWebPage_Render(t *testing.T) {
	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	page.SetContent(`<html><head></head><body>TEST</body></html>`)
	page.SetViewportSize(100, 200)

	// Render page.
	filename := filepath.Join(p.Path(), "test.png")
	page.Render(filename, "png", 100)

	// Read file.
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	// Parse image and verify dimensions.
	img, err := png.Decode(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	} else if bounds := img.Bounds(); bounds.Max.X != 100 || bounds.Max.Y != 200 {
		t.Fatalf("unexpected image dimesions: %dx%d", bounds.Max.X, bounds.Max.Y)
	}
}

// Ensure web page can receive mouse events.
func TestWebPage_SendMouseEvent(t *testing.T) {
	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	page.SetContent(`<html><head><script>window.onclick = function(e) { window.testX = e.x; window.testY = e.y; window.testButton = e.button }</script></head><body></body></html>`)

	// Send mouse event.
	page.SendMouseEvent("click", 100, 200, "middle")

	// Verify test variables.
	if x := page.Evaluate(`function() { return window.testX }`); x != float64(100) {
		t.Fatalf("unexpected x: %d", x)
	} else if y := page.Evaluate(`function() { return window.testY }`); y != float64(200) {
		t.Fatalf("unexpected y: %d", y)
	} else if button := page.Evaluate(`function() { return window.testButton }`); button != float64(1) {
		t.Fatalf("unexpected button: %d", button)
	}
}

// Ensure web page can receive keyboard events.
func TestWebPage_SendKeyboardEvent(t *testing.T) {
	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	page.SetContent(`<html><head><script>document.onkeydown = function(e) { window.testKey = e.keyCode; window.testAlt = e.altKey; window.testCtrl = e.ctrlKey; window.testMeta = e.metaKey; window.testShift = e.shiftKey;  }</script></head><body></body></html>`)

	// Send event.
	page.SendKeyboardEvent("keydown", "A", phantomjs.AltKey|phantomjs.CtrlKey|phantomjs.MetaKey|phantomjs.ShiftKey)

	// Verify test variables.
	if key := page.Evaluate(`function() { return window.testKey }`); key != float64(65) {
		t.Fatalf("unexpected key: %s", key)
	} else if altKey := page.Evaluate(`function() { return window.testAlt }`); altKey != true {
		t.Fatalf("unexpected alt key: %v", altKey)
	} else if ctrlKey := page.Evaluate(`function() { return window.testCtrl }`); ctrlKey != true {
		t.Fatalf("unexpected ctrl key: %v", ctrlKey)
	} else if metaKey := page.Evaluate(`function() { return window.testMeta }`); metaKey != true {
		t.Fatalf("unexpected meta key: %v", metaKey)
	} else if shiftKey := page.Evaluate(`function() { return window.testShift }`); shiftKey != true {
		t.Fatalf("unexpected shift key: %v", shiftKey)
	}
}

// Ensure web page can set content and URL at the same time.
func TestWebPage_SetContentAndURL(t *testing.T) {
	// Start process.
	p := MustOpenNewProcess()
	defer p.MustClose()

	// Create & open page.
	page := p.CreateWebPage()
	defer page.Close()
	page.SetContentAndURL(`<html><body>FOO</body></html>`, "http://google.com")

	// Verify content & URL.
	if content := page.Content(); content != `<html><head></head><body>FOO</body></html>` {
		t.Fatalf("unexpected content: %s", content)
	} else if u := page.URL(); u != `http://google.com/` {
		t.Fatalf("unexpected URL: %s", u)
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
