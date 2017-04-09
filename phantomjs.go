package phantomjs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	// ErrInjectionFailed is returned by InjectJS when injection fails.
	ErrInjectionFailed = errors.New("injection failed")
)

// Keyboard modifiers.
const (
	ShiftKey = 0x02000000
	CtrlKey  = 0x04000000
	AltKey   = 0x08000000
	MetaKey  = 0x10000000
	Keypad   = 0x20000000
)

// Default settings.
const (
	DefaultPort    = 20202
	DefaultBinPath = "phantomjs"
)

// Process represents a PhantomJS process.
type Process struct {
	path string
	cmd  *exec.Cmd

	// Path to the 'phantomjs' binary.
	BinPath string

	// HTTP port used to communicate with phantomjs.
	Port int

	// Output from the process.
	Stdout io.Writer
	Stderr io.Writer
}

// NewProcess returns a new instance of Process.
func NewProcess() *Process {
	return &Process{
		BinPath: DefaultBinPath,
		Port:    DefaultPort,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

// Path returns a temporary path that the process is run from.
func (p *Process) Path() string {
	return p.path
}

// Open start the phantomjs process with the shim script.
func (p *Process) Open() error {
	if err := func() error {
		// Generate temporary path to run script from.
		path, err := ioutil.TempDir("", "phantomjs-")
		if err != nil {
			return err
		}
		p.path = path

		// Write shim script.
		scriptPath := filepath.Join(path, "shim.js")
		if err := ioutil.WriteFile(scriptPath, []byte(shim), 0600); err != nil {
			return err
		}

		// Start external process.
		cmd := exec.Command(p.BinPath, scriptPath)
		cmd.Dir = p.Path()
		cmd.Env = []string{fmt.Sprintf("PORT=%d", p.Port)}
		cmd.Stdout = p.Stdout
		cmd.Stderr = p.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		p.cmd = cmd

		// Wait until process is available.
		if err := p.wait(); err != nil {
			return err
		}
		return nil

	}(); err != nil {
		p.Close()
		return err
	}

	return nil
}

// Close stops the process.
func (p *Process) Close() (err error) {
	// Kill process.
	if p.cmd != nil {
		if e := p.cmd.Process.Kill(); e != nil && err == nil {
			err = e
		}
		p.cmd.Wait()
	}

	// Remove shim file.
	if p.path != "" {
		if e := os.RemoveAll(p.path); e != nil && err == nil {
			err = e
		}
	}

	return err
}

// URL returns the process' API URL.
func (p *Process) URL() string {
	return fmt.Sprintf("http://localhost:%d", p.Port)
}

// wait continually checks the process until it gets a response or times out.
func (p *Process) wait() error {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return errors.New("timeout")
		case <-ticker.C:
			if err := p.ping(); err == nil {
				return nil
			}
		}
	}
}

// ping checks the process to see if it is up.
func (p *Process) ping() error {
	// Send request.
	resp, err := http.Get(p.URL() + "/ping")
	if err != nil {
		return err
	}
	resp.Body.Close()

	// Verify successful status code.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// CreateWebPage returns a new instance of a "webpage".
func (p *Process) CreateWebPage() *WebPage {
	var resp struct {
		Ref refJSON `json:"ref"`
	}
	p.mustDoJSON("POST", "/webpage/Create", nil, &resp)
	return &WebPage{ref: newRef(p, resp.Ref.ID)}
}

// mustDoJSON sends an HTTP request to url and encodes and decodes the req/resp as JSON.
// This function will panic if it cannot communicate with the phantomjs API.
func (p *Process) mustDoJSON(method, path string, req, resp interface{}) {
	// Encode request.
	var r io.Reader
	if req != nil {
		buf, err := json.Marshal(req)
		if err != nil {
			panic(err)
		}
		r = bytes.NewReader(buf)
	}

	// Create request.
	httpRequest, err := http.NewRequest(method, p.URL()+path, r)
	if err != nil {
		panic(err)
	}

	// Send request.
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		panic(err)
	}
	defer httpResponse.Body.Close()

	// Check response code.
	if httpResponse.StatusCode == http.StatusNotFound {
		panic(fmt.Errorf("not found: %s", path))
	} else if httpResponse.StatusCode == http.StatusInternalServerError {
		body, _ := ioutil.ReadAll(httpResponse.Body)
		panic(errors.New(string(body)))
	}

	// Decode response if reference passed in.
	if resp != nil {
		if buf, err := ioutil.ReadAll(httpResponse.Body); err != nil {
			panic(err)
		} else if err := json.Unmarshal(buf, resp); err != nil {
			panic(fmt.Errorf("unmarshal error: err=%s, buffer=%s", err, buf))
		}
	}
}

// WebPage represents an object returned from "webpage.create()".
type WebPage struct {
	ref *Ref
}

// Open opens a URL.
func (p *WebPage) Open(url string) error {
	req := map[string]interface{}{
		"ref": p.ref.id,
		"url": url,
	}
	var resp struct {
		Status string `json:"status"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Open", req, &resp)

	if resp.Status != "success" {
		return errors.New("failed")
	}
	return nil
}

// CanGoBack returns true if the page can be navigated back.
func (p *WebPage) CanGoBack() bool {
	var resp struct {
		Value bool `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/CanGoBack", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// CanGoForward returns true if the page can be navigated forward.
func (p *WebPage) CanGoForward() bool {
	var resp struct {
		Value bool `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/CanGoForward", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// ClipRect returns the clipping rectangle used when rendering.
// Returns nil if no clipping rectangle is set.
func (p *WebPage) ClipRect() Rect {
	var resp struct {
		Value rectJSON `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/ClipRect", map[string]interface{}{"ref": p.ref.id}, &resp)
	return Rect{
		Top:    resp.Value.Top,
		Left:   resp.Value.Left,
		Width:  resp.Value.Width,
		Height: resp.Value.Height,
	}
}

// SetClipRect sets the clipping rectangle used when rendering.
// Set to nil to render the entire webpage.
func (p *WebPage) SetClipRect(rect Rect) {
	req := map[string]interface{}{
		"ref": p.ref.id,
		"rect": rectJSON{
			Top:    rect.Top,
			Left:   rect.Left,
			Width:  rect.Width,
			Height: rect.Height,
		},
	}
	p.ref.process.mustDoJSON("POST", "/webpage/SetClipRect", req, nil)
}

// Content returns content of the webpage enclosed in an HTML/XML element.
func (p *WebPage) Content() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Content", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetContent sets the content of the webpage.
func (p *WebPage) SetContent(content string) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetContent", map[string]interface{}{"ref": p.ref.id, "content": content}, nil)
}

// Cookies returns a list of cookies visible to the current URL.
func (p *WebPage) Cookies() []*http.Cookie {
	var resp struct {
		Value []cookieJSON `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Cookies", map[string]interface{}{"ref": p.ref.id}, &resp)

	a := make([]*http.Cookie, len(resp.Value))
	for i := range resp.Value {
		a[i] = decodeCookieJSON(resp.Value[i])
	}
	return a
}

// SetCookies sets a list of cookies visible to the current URL.
func (p *WebPage) SetCookies(cookies []*http.Cookie) {
	a := make([]cookieJSON, len(cookies))
	for i := range cookies {
		a[i] = encodeCookieJSON(cookies[i])
	}
	req := map[string]interface{}{"ref": p.ref.id, "cookies": a}
	p.ref.process.mustDoJSON("POST", "/webpage/SetCookies", req, nil)
}

// CustomHeaders returns a list of additional headers sent with the web page.
func (p *WebPage) CustomHeaders() http.Header {
	var resp struct {
		Value map[string]string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/CustomHeaders", map[string]interface{}{"ref": p.ref.id}, &resp)

	// Convert to a header object.
	hdr := make(http.Header)
	for key, value := range resp.Value {
		hdr.Set(key, value)
	}
	return hdr
}

// SetCustomHeaders sets a list of additional headers sent with the web page.
//
// This function does not support multiple headers with the same name. Only
// the first value for a header key will be used.
func (p *WebPage) SetCustomHeaders(header http.Header) {
	m := make(map[string]string)
	for key := range header {
		m[key] = header.Get(key)
	}
	req := map[string]interface{}{"ref": p.ref.id, "headers": m}
	p.ref.process.mustDoJSON("POST", "/webpage/SetCustomHeaders", req, nil)
}

// FocusedFrameName returns the name of the currently focused frame.
func (p *WebPage) FocusedFrameName() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FocusedFrameName", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameContent returns the content of the current frame.
func (p *WebPage) FrameContent() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameContent", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetFrameContent sets the content of the current frame.
func (p *WebPage) SetFrameContent(content string) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetFrameContent", map[string]interface{}{"ref": p.ref.id, "content": content}, nil)
}

// FrameName returns the name of the current frame.
func (p *WebPage) FrameName() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameName", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FramePlainText returns the plain text representation of the current frame content.
func (p *WebPage) FramePlainText() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FramePlainText", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameTitle returns the title of the current frame.
func (p *WebPage) FrameTitle() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameTitle", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameURL returns the URL of the current frame.
func (p *WebPage) FrameURL() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameURL", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameCount returns the total number of frames.
func (p *WebPage) FrameCount() int {
	var resp struct {
		Value int `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameCount", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameNames returns an list of frame names.
func (p *WebPage) FrameNames() []string {
	var resp struct {
		Value []string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/FrameNames", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// LibraryPath returns the path used by InjectJS() to resolve scripts.
// Initially it is set to Process.Path().
func (p *WebPage) LibraryPath() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/LibraryPath", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetLibraryPath sets the library path used by InjectJS().
func (p *WebPage) SetLibraryPath(path string) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetLibraryPath", map[string]interface{}{"ref": p.ref.id, "path": path}, nil)
}

// NavigationLocked returns true if the navigation away from the page is disabled.
func (p *WebPage) NavigationLocked() bool {
	var resp struct {
		Value bool `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/NavigationLocked", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetNavigationLocked sets whether navigation away from the page should be disabled.
func (p *WebPage) SetNavigationLocked(value bool) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetNavigationLocked", map[string]interface{}{"ref": p.ref.id, "value": value}, nil)
}

// OfflineStoragePath returns the path used by offline storage.
func (p *WebPage) OfflineStoragePath() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/OfflineStoragePath", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// OfflineStorageQuota returns the number of bytes that can be used for offline storage.
func (p *WebPage) OfflineStorageQuota() int {
	var resp struct {
		Value int `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/OfflineStorageQuota", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// OwnsPages returns true if this page owns pages opened in other windows.
func (p *WebPage) OwnsPages() bool {
	var resp struct {
		Value bool `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/OwnsPages", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetOwnsPages sets whether this page owns pages opened in other windows.
func (p *WebPage) SetOwnsPages(v bool) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetOwnsPages", map[string]interface{}{"ref": p.ref.id, "value": v}, nil)
}

// PageWindowNames returns an list of owned window names.
func (p *WebPage) PageWindowNames() []string {
	var resp struct {
		Value []string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/PageWindowNames", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// Pages returns a list of owned pages.
func (p *WebPage) Pages() []*WebPage {
	var resp struct {
		Refs []refJSON `json:"refs"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Pages", map[string]interface{}{"ref": p.ref.id}, &resp)

	// Convert reference IDs to web pages.
	a := make([]*WebPage, len(resp.Refs))
	for i, ref := range resp.Refs {
		a[i] = &WebPage{ref: newRef(p.ref.process, ref.ID)}
	}
	return a
}

// PaperSize returns the size of the web page when rendered as a PDF.
func (p *WebPage) PaperSize() PaperSize {
	var resp struct {
		Value paperSizeJSON `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/PaperSize", map[string]interface{}{"ref": p.ref.id}, &resp)
	return decodePaperSizeJSON(resp.Value)
}

// SetPaperSize sets the size of the web page when rendered as a PDF.
func (p *WebPage) SetPaperSize(size PaperSize) {
	req := map[string]interface{}{"ref": p.ref.id, "size": encodePaperSizeJSON(size)}
	p.ref.process.mustDoJSON("POST", "/webpage/SetPaperSize", req, nil)
}

// PlainText returns the plain text representation of the page.
func (p *WebPage) PlainText() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/PlainText", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// ScrollPosition returns the current scroll position of the page.
func (p *WebPage) ScrollPosition() Position {
	var resp struct {
		Top  int `json:"top"`
		Left int `json:"left"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/ScrollPosition", map[string]interface{}{"ref": p.ref.id}, &resp)
	return Position{Top: resp.Top, Left: resp.Left}
}

// SetScrollPosition sets the current scroll position of the page.
func (p *WebPage) SetScrollPosition(pos Position) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetScrollPosition", map[string]interface{}{"ref": p.ref.id, "top": pos.Top, "left": pos.Left}, nil)
}

// Settings returns the settings used on the web page.
func (p *WebPage) Settings() WebPageSettings {
	var resp struct {
		Settings webPageSettingsJSON `json:"settings"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Settings", map[string]interface{}{"ref": p.ref.id}, &resp)
	return WebPageSettings{
		JavascriptEnabled:             resp.Settings.JavascriptEnabled,
		LoadImages:                    resp.Settings.LoadImages,
		LocalToRemoteURLAccessEnabled: resp.Settings.LocalToRemoteURLAccessEnabled,
		UserAgent:                     resp.Settings.UserAgent,
		Username:                      resp.Settings.Username,
		Password:                      resp.Settings.Password,
		XSSAuditingEnabled:            resp.Settings.XSSAuditingEnabled,
		WebSecurityEnabled:            resp.Settings.WebSecurityEnabled,
		ResourceTimeout:               time.Duration(resp.Settings.ResourceTimeout) * time.Millisecond,
	}
}

// SetSettings sets various settings on the web page.
//
// The settings apply only during the initial call to the page.open function.
// Subsequent modification of the settings object will not have any impact.
func (p *WebPage) SetSettings(settings WebPageSettings) {
	req := map[string]interface{}{
		"ref": p.ref.id,
		"settings": webPageSettingsJSON{
			JavascriptEnabled:             settings.JavascriptEnabled,
			LoadImages:                    settings.LoadImages,
			LocalToRemoteURLAccessEnabled: settings.LocalToRemoteURLAccessEnabled,
			UserAgent:                     settings.UserAgent,
			Username:                      settings.Username,
			Password:                      settings.Password,
			XSSAuditingEnabled:            settings.XSSAuditingEnabled,
			WebSecurityEnabled:            settings.WebSecurityEnabled,
			ResourceTimeout:               int(settings.ResourceTimeout / time.Millisecond),
		},
	}
	p.ref.process.mustDoJSON("POST", "/webpage/SetSettings", req, nil)
}

// Title returns the title of the web page.
func (p *WebPage) Title() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Title", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// URL returns the current URL of the web page.
func (p *WebPage) URL() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/URL", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// ViewportSize returns the size of the viewport on the browser.
func (p *WebPage) ViewportSize() (width, height int) {
	var resp struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/ViewportSize", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Width, resp.Height
}

// SetViewportSize sets the size of the viewport.
func (p *WebPage) SetViewportSize(width, height int) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetViewportSize", map[string]interface{}{"ref": p.ref.id, "width": width, "height": height}, nil)
}

// WindowName returns the window name of the web page.
func (p *WebPage) WindowName() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/WindowName", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// ZoomFactor returns zoom factor when rendering the page.
func (p *WebPage) ZoomFactor() float64 {
	var resp struct {
		Value float64 `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/ZoomFactor", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetZoomFactor sets the zoom factor when rendering the page.
func (p *WebPage) SetZoomFactor(factor float64) {
	p.ref.process.mustDoJSON("POST", "/webpage/SetZoomFactor", map[string]interface{}{"ref": p.ref.id, "value": factor}, nil)
}

// AddCookie adds a cookie to the page.
// Returns true if the cookie was successfully added.
func (p *WebPage) AddCookie(cookie *http.Cookie) bool {
	var resp struct {
		ReturnValue bool `json:"returnValue"`
	}
	req := map[string]interface{}{"ref": p.ref.id, "cookie": encodeCookieJSON(cookie)}
	p.ref.process.mustDoJSON("POST", "/webpage/AddCookie", req, &resp)
	return resp.ReturnValue
}

// ClearCookies deletes all cookies visible to the current URL.
func (p *WebPage) ClearCookies() {
	p.ref.process.mustDoJSON("POST", "/webpage/ClearCookies", map[string]interface{}{"ref": p.ref.id}, nil)
}

// Close releases the web page and its resources.
func (p *WebPage) Close() {
	p.ref.process.mustDoJSON("POST", "/webpage/Close", map[string]interface{}{"ref": p.ref.id}, nil)
}

// DeleteCookie removes a cookie with a matching name.
// Returns true if the cookie was successfully deleted.
func (p *WebPage) DeleteCookie(name string) bool {
	var resp struct {
		ReturnValue bool `json:"returnValue"`
	}
	req := map[string]interface{}{"ref": p.ref.id, "name": name}
	p.ref.process.mustDoJSON("POST", "/webpage/DeleteCookie", req, &resp)
	return resp.ReturnValue
}

// EvaluateAsync executes a JavaScript function and returns immediately.
// Execution is delayed by delay. No value is returned.
func (p *WebPage) EvaluateAsync(script string, delay time.Duration) {
	p.ref.process.mustDoJSON("POST", "/webpage/EvaluateAsync", map[string]interface{}{"ref": p.ref.id, "script": script, "delay": int(delay / time.Millisecond)}, nil)
}

// EvaluateJavaScript executes a JavaScript function.
// Returns the value returned by the function.
func (p *WebPage) EvaluateJavaScript(script string) interface{} {
	var resp struct {
		ReturnValue interface{} `json:"returnValue"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/EvaluateJavaScript", map[string]interface{}{"ref": p.ref.id, "script": script}, &resp)
	return resp.ReturnValue
}

// Evaluate executes a JavaScript function in the context of the web page.
// Returns the value returned by the function.
func (p *WebPage) Evaluate(script string) interface{} {
	var resp struct {
		ReturnValue interface{} `json:"returnValue"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Evaluate", map[string]interface{}{"ref": p.ref.id, "script": script}, &resp)
	return resp.ReturnValue
}

// Page returns an owned page by window name.
// Returns nil if the page cannot be found.
func (p *WebPage) Page(name string) *WebPage {
	var resp struct {
		Ref refJSON `json:"ref"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/Page", map[string]interface{}{"ref": p.ref.id, "name": name}, &resp)
	if resp.Ref.ID == "" {
		return nil
	}
	return &WebPage{ref: newRef(p.ref.process, resp.Ref.ID)}
}

// GoBack navigates back to the previous page.
func (p *WebPage) GoBack() {
	p.ref.process.mustDoJSON("POST", "/webpage/GoBack", map[string]interface{}{"ref": p.ref.id}, nil)
}

// GoForward navigates to the next page.
func (p *WebPage) GoForward() {
	p.ref.process.mustDoJSON("POST", "/webpage/GoForward", map[string]interface{}{"ref": p.ref.id}, nil)
}

// Go navigates to the page in history by relative offset.
// A positive index moves forward, a negative index moves backwards.
func (p *WebPage) Go(index int) {
	p.ref.process.mustDoJSON("POST", "/webpage/Go", map[string]interface{}{"ref": p.ref.id, "index": index}, nil)
}

// IncludeJS includes an external script from url.
// Returns after the script has been loaded.
func (p *WebPage) IncludeJS(url string) {
	p.ref.process.mustDoJSON("POST", "/webpage/IncludeJS", map[string]interface{}{"ref": p.ref.id, "url": url}, nil)
}

// InjectJS injects an external script from the local filesystem.
//
// The script will be loaded from the Process.Path() directory. If it cannot be
// found then it is loaded from the library path.
func (p *WebPage) InjectJS(filename string) error {
	var resp struct {
		ReturnValue bool `json:"returnValue"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/InjectJS", map[string]interface{}{"ref": p.ref.id, "filename": filename}, &resp)
	if !resp.ReturnValue {
		return ErrInjectionFailed
	}
	return nil
}

func (p *WebPage) Reload() {
	p.ref.process.mustDoJSON("POST", "/webpage/Reload", map[string]interface{}{"ref": p.ref.id}, nil)
}

// RenderBase64 renders the web page to a base64 encoded string.
func (p *WebPage) RenderBase64(format string) string {
	var resp struct {
		ReturnValue string `json:"returnValue"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/RenderBase64", map[string]interface{}{"ref": p.ref.id, "format": format}, &resp)
	return resp.ReturnValue
}

// Render renders the web page to a file with the given format and quality settings.
// This supports the "PDF", "PNG", "JPEG", "BMP", "PPM", and "GIF" formats.
func (p *WebPage) Render(filename, format string, quality int) {
	req := map[string]interface{}{"ref": p.ref.id, "filename": filename, "format": format, "quality": quality}
	p.ref.process.mustDoJSON("POST", "/webpage/Render", req, nil)
}

// SendMouseEvent sends a mouse event as if it came from the user.
// It is not a synthetic event.
//
// The eventType can be "mouseup", "mousedown", "mousemove", "doubleclick",
// or "click". The mouseX and mouseY specify the position of the mouse on the
// screen. The button argument specifies the mouse button clicked (e.g. "left").
func (p *WebPage) SendMouseEvent(eventType string, mouseX, mouseY int, button string) {
	p.ref.process.mustDoJSON("POST", "/webpage/SendMouseEvent", map[string]interface{}{"ref": p.ref.id, "eventType": eventType, "mouseX": mouseX, "mouseY": mouseY, "button": button}, nil)
}

// SendKeyboardEvent sends a keyboard event as if it came from the user.
// It is not a synthetic event.
//
// The eventType can be "keyup", "keypress", or "keydown".
//
// The key argument is a string or a key listed here:
// https://github.com/ariya/phantomjs/commit/cab2635e66d74b7e665c44400b8b20a8f225153a
//
// Keyboard modifiers can be joined together using the bitwise OR operator.
func (p *WebPage) SendKeyboardEvent(eventType string, key string, modifier int) {
	p.ref.process.mustDoJSON("POST", "/webpage/SendKeyboardEvent", map[string]interface{}{"ref": p.ref.id, "eventType": eventType, "key": key, "modifier": modifier}, nil)
}

func (p *WebPage) SendEvent() {
	panic("TODO")
}

func (p *WebPage) SetContentAndURL() {
	panic("TODO")
}

func (p *WebPage) Stop() {
	panic("TODO")
}

func (p *WebPage) SwitchToChildFrame() {
	panic("TODO")
}

func (p *WebPage) SwitchToFocusedFrame() {
	panic("TODO")
}

// SwitchToFrameName changes focus to the named frame.
func (p *WebPage) SwitchToFrameName(name string) {
	p.ref.process.mustDoJSON("POST", "/webpage/SwitchToFrameName", map[string]interface{}{"ref": p.ref.id, "name": name}, nil)
}

// SwitchToFramePosition changes focus to a frame at the given position.
func (p *WebPage) SwitchToFramePosition(pos int) {
	p.ref.process.mustDoJSON("POST", "/webpage/SwitchToFramePosition", map[string]interface{}{"ref": p.ref.id, "position": pos}, nil)
}

func (p *WebPage) SwitchToMainFrame() {
	panic("TODO")
}

func (p *WebPage) SwitchToParentFrame() {
	panic("TODO")
}

func (p *WebPage) UploadFile() {
	panic("TODO")
}

// OpenWebPageSettings represents the settings object passed to WebPage.Open().
type OpenWebPageSettings struct {
	Method string `json:"method"`
}

// Ref represents a reference to an object in phantomjs.
type Ref struct {
	process *Process
	id      string
}

// newRef returns a new instance of a referenced object within the process.
func newRef(p *Process, id string) *Ref {
	return &Ref{process: p, id: id}
}

// ID returns the reference identifier.
func (r *Ref) ID() string {
	return r.id
}

// refJSON is a struct for encoding refs as JSON.
type refJSON struct {
	ID string `json:"id"`
}

// Rect represents a rectangle used by WebPage.ClipRect().
type Rect struct {
	Top    int
	Left   int
	Width  int
	Height int
}

// rectJSON is a struct for encoding rects as JSON.
type rectJSON struct {
	Top    int `json:"top"`
	Left   int `json:"left"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// cookieJSON is a struct for encoding http.Cookie objects as JSON.
type cookieJSON struct {
	Domain   string `json:"domain"`
	Expires  string `json:"expires"`
	Expiry   int    `json:"expiry"`
	HttpOnly bool   `json:"httponly"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	Value    string `json:"value"`
}

func encodeCookieJSON(v *http.Cookie) cookieJSON {
	out := cookieJSON{
		Domain:   v.Domain,
		HttpOnly: v.HttpOnly,
		Name:     v.Name,
		Path:     v.Path,
		Secure:   v.Secure,
		Value:    v.Value,
	}

	if !v.Expires.IsZero() {
		out.Expires = v.Expires.UTC().Format(http.TimeFormat)
	}
	return out
}

func decodeCookieJSON(v cookieJSON) *http.Cookie {
	out := &http.Cookie{
		Domain:     v.Domain,
		RawExpires: v.Expires,
		HttpOnly:   v.HttpOnly,
		Name:       v.Name,
		Path:       v.Path,
		Secure:     v.Secure,
		Value:      v.Value,
	}

	if v.Expires != "" {
		expires, err := time.Parse(http.TimeFormat, v.Expires)
		if err != nil {
			panic(err)
		}
		out.Expires = expires
		out.RawExpires = v.Expires
	}

	return out
}

// PaperSize represents the size of a webpage when rendered as a PDF.
//
// Units can be specified in "mm", "cm", "in", or "px".
// If no unit is specified then "px" is used.
type PaperSize struct {
	// Dimensions of the paper.
	// This can also be specified via Format.
	Width  string
	Height string

	// Supported formats: "A3", "A4", "A5", "Legal", "Letter", "Tabloid".
	Format string

	// Margins around the paper.
	Margin *PaperSizeMargin

	// Supported orientations: "portrait", "landscape".
	Orientation string
}

// PaperSizeMargin represents the margins around the paper.
type PaperSizeMargin struct {
	Top    string
	Bottom string
	Left   string
	Right  string
}

type paperSizeJSON struct {
	Width       string               `json:"width,omitempty"`
	Height      string               `json:"height,omitempty"`
	Format      string               `json:"format,omitempty"`
	Margin      *paperSizeMarginJSON `json:"margin,omitempty"`
	Orientation string               `json:"orientation,omitempty"`
}

type paperSizeMarginJSON struct {
	Top    string `json:"top,omitempty"`
	Bottom string `json:"bottom,omitempty"`
	Left   string `json:"left,omitempty"`
	Right  string `json:"right,omitempty"`
}

func encodePaperSizeJSON(v PaperSize) paperSizeJSON {
	out := paperSizeJSON{
		Width:       v.Width,
		Height:      v.Height,
		Format:      v.Format,
		Orientation: v.Orientation,
	}
	if v.Margin != nil {
		out.Margin = &paperSizeMarginJSON{
			Top:    v.Margin.Top,
			Bottom: v.Margin.Bottom,
			Left:   v.Margin.Left,
			Right:  v.Margin.Right,
		}
	}
	return out
}

func decodePaperSizeJSON(v paperSizeJSON) PaperSize {
	out := PaperSize{
		Width:       v.Width,
		Height:      v.Height,
		Format:      v.Format,
		Orientation: v.Orientation,
	}
	if v.Margin != nil {
		out.Margin = &PaperSizeMargin{
			Top:    v.Margin.Top,
			Bottom: v.Margin.Bottom,
			Left:   v.Margin.Left,
			Right:  v.Margin.Right,
		}
	}
	return out
}

// Position represents a coordinate on the page, in pixels.
type Position struct {
	Top  int
	Left int
}

// WebPageSettings represents various settings on a web page.
type WebPageSettings struct {
	JavascriptEnabled             bool
	LoadImages                    bool
	LocalToRemoteURLAccessEnabled bool
	UserAgent                     string
	Username                      string
	Password                      string
	XSSAuditingEnabled            bool
	WebSecurityEnabled            bool
	ResourceTimeout               time.Duration
}

type webPageSettingsJSON struct {
	JavascriptEnabled             bool   `json:"javascriptEnabled"`
	LoadImages                    bool   `json:"loadImages"`
	LocalToRemoteURLAccessEnabled bool   `json:"localToRemoteUrlAccessEnabled"`
	UserAgent                     string `json:"userAgent"`
	Username                      string `json:"username"`
	Password                      string `json:"password"`
	XSSAuditingEnabled            bool   `json:"XSSAuditingEnabled"`
	WebSecurityEnabled            bool   `json:"webSecurityEnabled"`
	ResourceTimeout               int    `json:"resourceTimeout"`
}

// shim is the included javascript used to communicate with PhantomJS.
const shim = `
var system = require("system")
var webpage = require('webpage');
var webserver = require('webserver');

/*
 * HTTP API
 */

// Serves RPC API.
var server = webserver.create();
server.listen(system.env["PORT"], function(request, response) {
	try {
		switch (request.url) {
			case '/ping': return handlePing(request, response);
			case '/webpage/CanGoBack': return handleWebpageCanGoBack(request, response);
			case '/webpage/CanGoForward': return handleWebpageCanGoForward(request, response);
			case '/webpage/ClipRect': return handleWebpageClipRect(request, response);
			case '/webpage/SetClipRect': return handleWebpageSetClipRect(request, response);
			case '/webpage/Cookies': return handleWebpageCookies(request, response);
			case '/webpage/SetCookies': return handleWebpageSetCookies(request, response);
			case '/webpage/CustomHeaders': return handleWebpageCustomHeaders(request, response);
			case '/webpage/SetCustomHeaders': return handleWebpageSetCustomHeaders(request, response);
			case '/webpage/Create': return handleWebpageCreate(request, response);
			case '/webpage/Content': return handleWebpageContent(request, response);
			case '/webpage/SetContent': return handleWebpageSetContent(request, response);
			case '/webpage/FocusedFrameName': return handleWebpageFocusedFrameName(request, response);
			case '/webpage/FrameContent': return handleWebpageFrameContent(request, response);
			case '/webpage/SetFrameContent': return handleWebpageSetFrameContent(request, response);
			case '/webpage/FrameName': return handleWebpageFrameName(request, response);
			case '/webpage/FramePlainText': return handleWebpageFramePlainText(request, response);
			case '/webpage/FrameTitle': return handleWebpageFrameTitle(request, response);
			case '/webpage/FrameURL': return handleWebpageFrameURL(request, response);
			case '/webpage/FrameCount': return handleWebpageFrameCount(request, response);
			case '/webpage/FrameNames': return handleWebpageFrameNames(request, response);
			case '/webpage/LibraryPath': return handleWebpageLibraryPath(request, response);
			case '/webpage/SetLibraryPath': return handleWebpageSetLibraryPath(request, response);
			case '/webpage/NavigationLocked': return handleWebpageNavigationLocked(request, response);
			case '/webpage/SetNavigationLocked': return handleWebpageSetNavigationLocked(request, response);
			case '/webpage/OfflineStoragePath': return handleWebpageOfflineStoragePath(request, response);
			case '/webpage/OfflineStorageQuota': return handleWebpageOfflineStorageQuota(request, response);
			case '/webpage/OwnsPages': return handleWebpageOwnsPages(request, response);
			case '/webpage/SetOwnsPages': return handleWebpageSetOwnsPages(request, response);
			case '/webpage/PageWindowNames': return handleWebpagePageWindowNames(request, response);
			case '/webpage/Pages': return handleWebpagePages(request, response);
			case '/webpage/PaperSize': return handleWebpagePaperSize(request, response);
			case '/webpage/SetPaperSize': return handleWebpageSetPaperSize(request, response);
			case '/webpage/PlainText': return handleWebpagePlainText(request, response);
			case '/webpage/ScrollPosition': return handleWebpageScrollPosition(request, response);
			case '/webpage/SetScrollPosition': return handleWebpageSetScrollPosition(request, response);
			case '/webpage/Settings': return handleWebpageSettings(request, response);
			case '/webpage/SetSettings': return handleWebpageSetSettings(request, response);
			case '/webpage/Title': return handleWebpageTitle(request, response);
			case '/webpage/URL': return handleWebpageURL(request, response);
			case '/webpage/ViewportSize': return handleWebpageViewportSize(request, response);
			case '/webpage/SetViewportSize': return handleWebpageSetViewportSize(request, response);
			case '/webpage/WindowName': return handleWebpageWindowName(request, response);
			case '/webpage/ZoomFactor': return handleWebpageZoomFactor(request, response);
			case '/webpage/SetZoomFactor': return handleWebpageSetZoomFactor(request, response);

			case '/webpage/AddCookie': return handleWebpageAddCookie(request, response);
			case '/webpage/ClearCookies': return handleWebpageClearCookies(request, response);
			case '/webpage/DeleteCookie': return handleWebpageDeleteCookie(request, response);
			case '/webpage/SwitchToFrameName': return handleWebpageSwitchToFrameName(request, response);
			case '/webpage/SwitchToFramePosition': return handleWebpageSwitchToFramePosition(request, response);
			case '/webpage/Open': return handleWebpageOpen(request, response);
			case '/webpage/Close': return handleWebpageClose(request, response);
			case '/webpage/EvaluateAsync': return handleWebpageEvaluateAsync(request, response);
			case '/webpage/EvaluateJavaScript': return handleWebpageEvaluateJavaScript(request, response);
			case '/webpage/Evaluate': return handleWebpageEvaluate(request, response);
			case '/webpage/Page': return handleWebpagePage(request, response);
			case '/webpage/GoBack': return handleWebpageGoBack(request, response);
			case '/webpage/GoForward': return handleWebpageGoForward(request, response);
			case '/webpage/Go': return handleWebpageGo(request, response);
			case '/webpage/IncludeJS': return handleWebpageIncludeJS(request, response);
			case '/webpage/InjectJS': return handleWebpageInjectJS(request, response);
			case '/webpage/Reload': return handleWebpageReload(request, response);
			case '/webpage/RenderBase64': return handleWebpageRenderBase64(request, response);
			case '/webpage/Render': return handleWebpageRender(request, response);
			case '/webpage/SendMouseEvent': return handleWebpageSendMouseEvent(request, response);
			case '/webpage/SendKeyboardEvent': return handleWebpageSendKeyboardEvent(request, response);
			default: return handleNotFound(request, response);
		}
	} catch(e) {
		response.statusCode = 500;
		response.write(request.url + ": " + e.message);
		response.closeGracefully();
	}
});

function handlePing(request, response) {
	response.statusCode = 200;
	response.write('ok');
	response.closeGracefully();
}

function handleWebpageCanGoBack(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.canGoBack}));
	response.closeGracefully();
}

function handleWebpageCanGoForward(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.canGoForward}));
	response.closeGracefully();
}

function handleWebpageClipRect(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.clipRect}));
	response.closeGracefully();
}

function handleWebpageSetClipRect(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.clipRect = msg.rect;
	response.closeGracefully();
}

function handleWebpageCookies(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.cookies}));
	response.closeGracefully();
}

function handleWebpageSetCookies(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.cookies = msg.cookies;
	response.closeGracefully();
}

function handleWebpageCustomHeaders(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.customHeaders}));
	response.closeGracefully();
}

function handleWebpageSetCustomHeaders(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.customHeaders = msg.headers;
	response.closeGracefully();
}

function handleWebpageCreate(request, response) {
	var ref = createRef(webpage.create());
	response.statusCode = 200;
	response.write(JSON.stringify({ref: ref}));
	response.closeGracefully();
}

function handleWebpageOpen(request, response) {
	var msg = JSON.parse(request.post)
	var page = ref(msg.ref)
	page.open(msg.url, function(status) {
		response.write(JSON.stringify({status: status}));
		response.closeGracefully();
	})
}

function handleWebpageContent(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.content}));
	response.closeGracefully();
}

function handleWebpageSetContent(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.content = msg.content;
	response.closeGracefully();
}

function handleWebpageFocusedFrameName(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.focusedFrameName}));
	response.closeGracefully();
}

function handleWebpageFrameContent(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.frameContent}));
	response.closeGracefully();
}

function handleWebpageSetFrameContent(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.frameContent = msg.content;
	response.closeGracefully();
}

function handleWebpageFrameName(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.frameName}));
	response.closeGracefully();
}

function handleWebpageFramePlainText(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.framePlainText}));
	response.closeGracefully();
}

function handleWebpageFrameTitle(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.frameTitle}));
	response.closeGracefully();
}

function handleWebpageFrameURL(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.frameUrl}));
	response.closeGracefully();
}

function handleWebpageFrameCount(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.framesCount}));
	response.closeGracefully();
}

function handleWebpageFrameNames(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.framesName}));
	response.closeGracefully();
}

function handleWebpageLibraryPath(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.libraryPath}));
	response.closeGracefully();
}

function handleWebpageSetLibraryPath(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.libraryPath = msg.path;
	response.closeGracefully();
}

function handleWebpageNavigationLocked(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.navigationLocked}));
	response.closeGracefully();
}

function handleWebpageSetNavigationLocked(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.navigationLocked = msg.value;
	response.closeGracefully();
}

function handleWebpageOfflineStoragePath(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.offlineStoragePath}));
	response.closeGracefully();
}

function handleWebpageOfflineStorageQuota(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.offlineStorageQuota}));
	response.closeGracefully();
}

function handleWebpageOwnsPages(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.ownsPages}));
	response.closeGracefully();
}

function handleWebpageSetOwnsPages(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.ownsPages = msg.value;
	response.closeGracefully();
}

function handleWebpagePageWindowNames(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.pagesWindowName}));
	response.closeGracefully();
}

function handleWebpagePages(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	var refs = page.pages.map(function(p) { return createRef(p); })
	response.write(JSON.stringify({refs: refs}));
	response.closeGracefully();
}

function handleWebpagePaperSize(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.paperSize}));
	response.closeGracefully();
}

function handleWebpageSetPaperSize(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.paperSize = msg.size;
	response.closeGracefully();
}

function handleWebpagePlainText(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.plainText}));
	response.closeGracefully();
}

function handleWebpageScrollPosition(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	var pos = page.scrollPosition;
	response.write(JSON.stringify({top: pos.top, left: pos.left}));
	response.closeGracefully();
}

function handleWebpageSetScrollPosition(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.scrollPosition = {top: msg.top, left: msg.left};
	response.closeGracefully();
}

function handleWebpageSettings(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({settings: page.settings}));
	response.closeGracefully();
}

function handleWebpageSetSettings(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.settings = msg.settings;
	response.closeGracefully();
}

function handleWebpageTitle(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.title}));
	response.closeGracefully();
}

function handleWebpageURL(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.url}));
	response.closeGracefully();
}

function handleWebpageViewportSize(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	var viewport = page.viewportSize;
	response.write(JSON.stringify({width: viewport.width, height: viewport.height}));
	response.closeGracefully();
}

function handleWebpageSetViewportSize(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.viewportSize = {width: msg.width, height: msg.height};
	response.closeGracefully();
}

function handleWebpageWindowName(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.windowName}));
	response.closeGracefully();
}

function handleWebpageZoomFactor(request, response) {
	var page = ref(JSON.parse(request.post).ref);
	response.write(JSON.stringify({value: page.zoomFactor}));
	response.closeGracefully();
}

function handleWebpageSetZoomFactor(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.zoomFactor = msg.value;
	response.closeGracefully();
}


function handleWebpageAddCookie(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.addCookie(msg.cookie);
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpageClearCookies(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.clearCookies();
	response.closeGracefully();
}

function handleWebpageDeleteCookie(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.deleteCookie(msg.name);
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpageSwitchToFrameName(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.switchToFrame(msg.name);
	response.closeGracefully();
}

function handleWebpageSwitchToFramePosition(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.switchToFrame(msg.position);
	response.closeGracefully();
}

function handleWebpageClose(request, response) {
	var msg = JSON.parse(request.post);

	// Close page.
	var page = ref(msg.ref);
	page.close();
	delete(refs, msg.ref);

	// Close and dereference owned pages.
	for (var i = 0; i < page.pages.length; i++) {
		page.pages[i].close();
		deleteRef(page.pages[i]);
	}

	response.statusCode = 200;
	response.closeGracefully();
}

function handleWebpageEvaluateAsync(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.evaluateAsync(msg.script, msg.delay);
	response.closeGracefully();
}

function handleWebpageEvaluateJavaScript(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.evaluateJavaScript(msg.script);
	response.statusCode = 200;
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpageEvaluate(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.evaluate(msg.script);
	response.statusCode = 200;
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpagePage(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var p = page.getPage(msg.name);

	response.statusCode = 200;
	if (p === null) {
		response.write(JSON.stringify({}));
	} else {
		response.write(JSON.stringify({ref: createRef(p)}));
	}
	response.closeGracefully();
}

function handleWebpageGoBack(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.goBack();
	response.closeGracefully();
}

function handleWebpageGoForward(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.goForward();
	response.closeGracefully();
}

function handleWebpageGo(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.go(msg.index);
	response.closeGracefully();
}

function handleWebpageIncludeJS(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.includeJs(msg.url, function() {
		response.closeGracefully();
	});
}

function handleWebpageInjectJS(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.injectJs(msg.filename);
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpageReload(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.reload();
	response.closeGracefully();
}

function handleWebpageRenderBase64(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	var returnValue = page.renderBase64(msg.format);
	response.write(JSON.stringify({returnValue: returnValue}));
	response.closeGracefully();
}

function handleWebpageRender(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.render(msg.filename, {format: msg.format, quality: msg.quality});
	response.closeGracefully();
}

function handleWebpageSendMouseEvent(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.sendEvent(msg.eventType, msg.mouseX, msg.mouseY, msg.button);
	response.closeGracefully();
}

function handleWebpageSendKeyboardEvent(request, response) {
	var msg = JSON.parse(request.post);
	var page = ref(msg.ref);
	page.sendEvent(msg.eventType, msg.key, null, null, msg.modifier);
	response.closeGracefully();
}


function handleNotFound(request, response) {
	response.statusCode = 404;
	response.write('not found');
	response.closeGracefully();
}


/*
 * REFS
 */

// Holds references to remote objects.
var refID = 0;
var refs = {};

// Adds an object to the reference map and a ref object.
function createRef(value) {
	// Return existing reference, if one exists.
	for (var key in refs) {
		if (refs.hasOwnProperty(key)) {
			if (refs[key] === value) {
				return key
			}
		}
	}

	// Generate a new id for new references.
	refID++;
	refs[refID.toString()] = value;
	return {id: refID.toString()};
}

// Removes a reference to a value, if any.
function deleteRef(value) {
	for (var key in refs) {
		if (refs.hasOwnProperty(key)) {
			if (refs[key] === value) {
				delete(refs, key);
			}
		}
	}
}

// Returns a reference object by ID.
function ref(id) {
	return refs[id];
}
`
