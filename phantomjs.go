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
	p.mustDoJSON("POST", "/webpage/create", nil, &resp)
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
		if err := json.NewDecoder(httpResponse.Body).Decode(resp); err != nil {
			panic(err)
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
	p.ref.process.mustDoJSON("POST", "/webpage/open", req, &resp)

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
	p.ref.process.mustDoJSON("POST", "/webpage/can_go_back", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// CanGoForward returns true if the page can be navigated forward.
func (p *WebPage) CanGoForward() bool {
	var resp struct {
		Value bool `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/can_go_forward", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// ClipRect returns the clipping rectangle used when rendering.
// Returns nil if no clipping rectangle is set.
func (p *WebPage) ClipRect() Rect {
	var resp struct {
		Value rectJSON `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/clip_rect", map[string]interface{}{"ref": p.ref.id}, &resp)
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
	p.ref.process.mustDoJSON("POST", "/webpage/set_clip_rect", req, nil)
}

// Content returns content of the webpage enclosed in an HTML/XML element.
func (p *WebPage) Content() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/content", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetContent sets the content of the webpage.
func (p *WebPage) SetContent(content string) {
	p.ref.process.mustDoJSON("POST", "/webpage/set_content", map[string]interface{}{"ref": p.ref.id, "content": content}, nil)
}

// Cookies returns a list of cookies visible to the current URL.
func (p *WebPage) Cookies() []*http.Cookie {
	var resp struct {
		Value []cookieJSON `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/cookies", map[string]interface{}{"ref": p.ref.id}, &resp)

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
	p.ref.process.mustDoJSON("POST", "/webpage/set_cookies", req, nil)
}

// CustomHeaders returns a list of additional headers sent with the web page.
func (p *WebPage) CustomHeaders() http.Header {
	var resp struct {
		Value map[string]string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/custom_headers", map[string]interface{}{"ref": p.ref.id}, &resp)

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
	p.ref.process.mustDoJSON("POST", "/webpage/set_custom_headers", req, nil)
}

// FocusedFrameName returns the name of the currently focused frame.
func (p *WebPage) FocusedFrameName() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/focused_frame_name", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameContent returns the content of the current frame.
func (p *WebPage) FrameContent() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_content", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetFrameContent sets the content of the current frame.
func (p *WebPage) SetFrameContent(content string) {
	p.ref.process.mustDoJSON("POST", "/webpage/set_frame_content", map[string]interface{}{"ref": p.ref.id, "content": content}, nil)
}

// FrameName returns the name of the current frame.
func (p *WebPage) FrameName() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_name", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FramePlainText returns the plain text representation of the current frame content.
func (p *WebPage) FramePlainText() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_plain_text", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameTitle returns the title of the current frame.
func (p *WebPage) FrameTitle() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_title", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameURL returns the URL of the current frame.
func (p *WebPage) FrameURL() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_url", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameCount returns the total number of frames.
func (p *WebPage) FrameCount() int {
	var resp struct {
		Value int `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_count", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// FrameNames returns an list of frame names.
func (p *WebPage) FrameNames() []string {
	var resp struct {
		Value []string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/frame_names", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// LibraryPath returns the path used by InjectJS() to resolve scripts.
// Initially it is set to Process.Path().
func (p *WebPage) LibraryPath() string {
	var resp struct {
		Value string `json:"value"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/library_path", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Value
}

// SetLibraryPath sets the library path used by InjectJS().
func (p *WebPage) SetLibraryPath(path string) {
	p.ref.process.mustDoJSON("POST", "/webpage/set_library_path", map[string]interface{}{"ref": p.ref.id, "path": path}, nil)
}

func (p *WebPage) NavigationLocked() string {
	panic("TODO")
}

func (p *WebPage) OfflineStoragePath() string {
	panic("TODO")
}

func (p *WebPage) OfflineStorageQuota() string {
	panic("TODO")
}

func (p *WebPage) OwnsPages() string {
	panic("TODO")
}

func (p *WebPage) PagesWindowName() string {
	panic("TODO")
}

func (p *WebPage) Pages() string {
	panic("TODO")
}

func (p *WebPage) PaperSize() string {
	panic("TODO")
}

func (p *WebPage) PlainText() string {
	panic("TODO")
}

func (p *WebPage) ScrollPosition() string {
	panic("TODO")
}

func (p *WebPage) Settings() string {
	panic("TODO")
}

func (p *WebPage) Title() string {
	panic("TODO")
}

func (p *WebPage) Url() string {
	panic("TODO")
}

func (p *WebPage) ViewportSize() string {
	panic("TODO")
}

func (p *WebPage) WindowName() string {
	panic("TODO")
}

func (p *WebPage) ZoomFactor() string {
	panic("TODO")
}

func (p *WebPage) AddCookie() {
	panic("TODO")
}

func (p *WebPage) ChildFramesCount() {
	panic("TODO")
}

func (p *WebPage) ChildFramesName() {
	panic("TODO")
}

func (p *WebPage) ClearCookies() {
	panic("TODO")
}

// Close releases the web page and its resources.
func (p *WebPage) Close() {
	p.ref.process.mustDoJSON("POST", "/webpage/close", map[string]interface{}{"ref": p.ref.id}, nil)
}

func (p *WebPage) CurrentFrameName() {
	panic("TODO")
}

func (p *WebPage) DeleteCookie() {
	panic("TODO")
}

func (p *WebPage) EvaluateAsync() {
	panic("TODO")
}

func (p *WebPage) EvaluateJavaScript() {
	panic("TODO")
}

func (p *WebPage) Evaluate() {
	panic("TODO")
}

func (p *WebPage) GetPage() {
	panic("TODO")
}

func (p *WebPage) GoBack() {
	panic("TODO")
}

func (p *WebPage) GoForward() {
	panic("TODO")
}

func (p *WebPage) Go() {
	panic("TODO")
}

func (p *WebPage) IncludeJs() {
	panic("TODO")
}

func (p *WebPage) InjectJs() {
	panic("TODO")
}

func (p *WebPage) OpenUrl() {
	panic("TODO")
}

func (p *WebPage) Release() {
	panic("TODO")
}

func (p *WebPage) Reload() {
	panic("TODO")
}

func (p *WebPage) RenderBase64() {
	panic("TODO")
}

func (p *WebPage) RenderBuffer() {
	panic("TODO")
}

func (p *WebPage) Render() {
	panic("TODO")
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
	p.ref.process.mustDoJSON("POST", "/webpage/switch_to_frame_name", map[string]interface{}{"ref": p.ref.id, "name": name}, nil)
}

// SwitchToFramePosition changes focus to a frame at the given position.
func (p *WebPage) SwitchToFramePosition(pos int) {
	p.ref.process.mustDoJSON("POST", "/webpage/switch_to_frame_position", map[string]interface{}{"ref": p.ref.id, "position": pos}, nil)
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
			case '/webpage/can_go_back': return handleWebpageCanGoBack(request, response);
			case '/webpage/can_go_forward': return handleWebpageCanGoForward(request, response);
			case '/webpage/clip_rect': return handleWebpageClipRect(request, response);
			case '/webpage/set_clip_rect': return handleWebpageSetClipRect(request, response);
			case '/webpage/cookies': return handleWebpageCookies(request, response);
			case '/webpage/set_cookies': return handleWebpageSetCookies(request, response);
			case '/webpage/custom_headers': return handleWebpageCustomHeaders(request, response);
			case '/webpage/set_custom_headers': return handleWebpageSetCustomHeaders(request, response);
			case '/webpage/create': return handleWebpageCreate(request, response);
			case '/webpage/content': return handleWebpageContent(request, response);
			case '/webpage/set_content': return handleWebpageSetContent(request, response);
			case '/webpage/focused_frame_name': return handleWebpageFocusedFrameName(request, response);
			case '/webpage/frame_content': return handleWebpageFrameContent(request, response);
			case '/webpage/set_frame_content': return handleWebpageSetFrameContent(request, response);
			case '/webpage/frame_name': return handleWebpageFrameName(request, response);
			case '/webpage/frame_plain_text': return handleWebpageFramePlainText(request, response);
			case '/webpage/frame_title': return handleWebpageFrameTitle(request, response);
			case '/webpage/frame_url': return handleWebpageFrameURL(request, response);
			case '/webpage/frame_count': return handleWebpageFrameCount(request, response);
			case '/webpage/frame_names': return handleWebpageFrameNames(request, response);
			case '/webpage/library_path': return handleWebpageLibraryPath(request, response);
			case '/webpage/set_library_path': return handleWebpageSetLibraryPath(request, response);
			case '/webpage/switch_to_frame_name': return handleWebpageSwitchToFrameName(request, response);
			case '/webpage/switch_to_frame_position': return handleWebpageSwitchToFramePosition(request, response);
			case '/webpage/open': return handleWebpageOpen(request, response);
			case '/webpage/close': return handleWebpageClose(request, response);
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
	var msg = JSON.parse(request.post)
	var page = ref(msg.ref)
	page.close()
	delete(refs, msg.ref)
	response.statusCode = 200;
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
	refID++;
	refs[refID] = value;
	return {id: refID.toString()};
}

// Returns a reference object by ID.
function ref(id) {
	return refs[id];
}
`
