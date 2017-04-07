package phantomjs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
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

// Open start the phantomjs process with the shim script.
func (p *Process) Open(ctx context.Context) error {
	// Write shim to a temporary file.
	f, err := ioutil.TempFile("", "phantomjs-")
	if err != nil {
		return err
	} else if _, err := f.WriteString(shim); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	} else if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	p.path = f.Name()

	// Start external process.
	cmd := exec.Command(p.BinPath, p.path)
	cmd.Env = []string{fmt.Sprintf("PORT=%d", p.Port)}
	cmd.Stdout = p.Stdout
	cmd.Stderr = p.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd

	// Wait until process is available.
	if err := p.wait(ctx); err != nil {
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
		if e := p.cmd.Wait(); e != nil && err == nil {
			err = e
		}
	}

	// Remove shim file.
	if p.path != "" {
		if e := os.Remove(p.path); e != nil && err == nil {
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
func (p *Process) wait(ctx context.Context) error {
	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.New("timeout")
		case <-ticker.C:
			if err := p.ping(ctx); err == nil {
				return nil
			}
		}
	}
}

// ping checks the process to see if it is up.
func (p *Process) ping(ctx context.Context) error {
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
func (p *Process) CreateWebPage(ctx context.Context) *WebPage {
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
		panic(errors.New("not found"))
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

// Close releases the web page and its resources.
func (p *WebPage) Close() {
	p.ref.process.mustDoJSON("POST", "/webpage/close", map[string]interface{}{"ref": p.ref.id}, nil)
}

// Content returns content of the webpage enclosed in an HTML/XML element.
func (p *WebPage) Content() string {
	var resp struct {
		Content string `json:"content"`
	}
	p.ref.process.mustDoJSON("POST", "/webpage/content", map[string]interface{}{"ref": p.ref.id}, &resp)
	return resp.Content
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
			case '/webpage/create': return handleWebpageCreate(request, response);
			case '/webpage/open': return handleWebpageOpen(request, response);
			case '/webpage/content': return handleWebpageContent(request, response);
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
	var page = ref(JSON.parse(request.post).ref)
	response.write(JSON.stringify({content: page.content}));
	response.closeGracefully();
}

function handleWebpageClose(request, response) {
	var msg = JSON.parse(request.post)
	var page = ref(msg.ref)
	page.close()
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
