// Package api is the protocol of the socket the running backend listens on: the types of
// its answers, the server and the client. The commands of the user interface (ctl)
// ask the backend through it; what they need to know of what runs, the backend tells
// (Facts), and they do not work it out again from the settings.
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/itdoginfo/podkop/src/backend/logbuf"
	E "github.com/sagernet/sing/common/exceptions"
)

// SocketPath is where the running backend listens.
const SocketPath = "/var/run/podkop/ctl.sock"

// ProtocolVersion is the version of this protocol, told in Status. The command line and
// the backend are one program, but between the upgrade of the package and the restart of
// the service the backend that runs is the one before: the version tells why an answer is
// not what the command expects.
const ProtocolVersion = 1

// The files of the directory of the configuration (supervisor.Options.ConfigDir). The
// configuration that runs is there while it runs, and as the last one once it stops,
// until the next one runs; the last configuration sing-box refused is there until the
// next settings are read.
const (
	RunningConfig  = "config.json"
	LastConfig     = "config.last.json"
	RejectedConfig = "config.rejected.json"
)

// Result is what a boot or a change of the settings came to.
type Result struct {
	// OK tells whether the router is now in the state the settings ask for.
	OK bool `json:"ok"`
	// Message says what was done, or why it was not.
	Message string `json:"message"`
}

// Status is the state of the backend.
type Status struct {
	// Protocol is the ProtocolVersion of the backend.
	Protocol int `json:"protocol"`
	// Running tells whether sing-box runs and the router is set up around it.
	Running bool `json:"running"`
	// Since is when the configuration that runs was started.
	Since time.Time `json:"since"`
	// Last is what the last boot or change came to.
	Last Result `json:"last"`
	// LastAt is when that was.
	LastAt time.Time `json:"last_at"`
}

// Facts are what the commands need to know of what runs.
type Facts struct {
	// ClashAPI is where the Clash API of the configuration that runs is reached from the
	// router (host:port), and ClashSecret its key; empty when nothing runs.
	ClashAPI    string `json:"clash_api"`
	ClashSecret string `json:"clash_secret"`
}

// ListsUpdate is what an update of the lists asked for from outside came to.
type ListsUpdate struct {
	// OK tells whether every list that is downloaded was downloaded.
	OK bool `json:"ok"`
	// Lines say what was done, a line a kind of list.
	Lines []string `json:"lines"`
}

// ListState is what is known of one rule set of the configuration that runs.
type ListState struct {
	Tag string `json:"tag"`
	// Kind is who fills the rule set: "remote" — sing-box downloads it from URL, in
	// Format (a list written as plain text is "text"); "file" — the backend writes the
	// file at Path from the settings or a local list.
	Kind   string `json:"kind"`
	URL    string `json:"url,omitempty"`
	Format string `json:"format,omitempty"`
	Path   string `json:"path,omitempty"`
	// LastUpdated is when sing-box last downloaded a remote rule set or found it
	// unchanged at its server; none before the first download.
	LastUpdated *time.Time `json:"last_updated,omitempty"`
}

// ProbeResult is what the backend saw of a check name.
type ProbeResult struct {
	// DNS tells that a DNS question for the name was answered from FakeIP.
	DNS bool `json:"dns"`
	// Connection tells that a connection to the name came in through the transparent
	// proxy inbound: past the firewall mark and the routing table.
	Connection bool `json:"connection"`
	// Client is the address the connection came from.
	Client string `json:"client,omitempty"`
	// Address is the FakeIP address of the name.
	Address string `json:"address,omitempty"`
}

// LogPage is a part of the log: lines, the number to ask with for what comes after, and
// the run of the backend the numbers belong to.
type LogPage struct {
	Run   string   `json:"run"`
	Next  uint64   `json:"next"`
	Lines []string `json:"lines"`
}

// Backend is the running backend as the socket shows it.
type Backend interface {
	Status() Status
	// Facts tells what the commands need to know of what runs.
	Facts() Facts
	// Request applies the settings and returns what that came to.
	Request(ctx context.Context) Result
	// UpdateLists downloads every list now and returns what that came to.
	UpdateLists(ctx context.Context) ListsUpdate
	// ListStatus tells what is known of each list of what runs.
	ListStatus(ctx context.Context) []ListState
	// RestoreRouting puts back what the network took and tells what it did.
	RestoreRouting(ctx context.Context) string
	// Probe tells what was seen of the check name of a label, waiting for it.
	Probe(ctx context.Context, label string, wait time.Duration) ProbeResult
	// ProbeSelf checks the router itself with a check name.
	ProbeSelf(ctx context.Context) ProbeResult
}

// Serve listens on the socket until the context ends. The socket is for root only: who
// can write to it can make the backend apply the settings, and Facts has the key of the
// Clash API.
func Serve(ctx context.Context, path string, backend Backend, logs *logbuf.Buffer) error {
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return err
	}
	// a socket left by a backend that was killed
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return E.Cause(err, "listen on ", path)
	}
	_ = os.Chmod(path, 0o600)
	answer := func(writer http.ResponseWriter, value any) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(value)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(writer http.ResponseWriter, request *http.Request) {
		status := backend.Status()
		status.Protocol = ProtocolVersion
		answer(writer, status)
	})
	mux.HandleFunc("GET /facts", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.Facts())
	})
	mux.HandleFunc("GET /logs", func(writer http.ResponseWriter, request *http.Request) {
		since, _ := strconv.ParseUint(request.URL.Query().Get("since"), 10, 64)
		lines, next := logs.Since(since)
		page := LogPage{Run: logs.Run(), Next: next, Lines: make([]string, len(lines))}
		for index, line := range lines {
			page.Lines[index] = line.Text()
		}
		answer(writer, page)
	})
	mux.HandleFunc("POST /apply", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.Request(request.Context()))
	})
	mux.HandleFunc("POST /lists/update", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.UpdateLists(request.Context()))
	})
	mux.HandleFunc("GET /lists", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.ListStatus(request.Context()))
	})
	mux.HandleFunc("POST /routing/restore", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.RestoreRouting(request.Context()))
	})
	mux.HandleFunc("GET /probe", func(writer http.ResponseWriter, request *http.Request) {
		wait, _ := strconv.Atoi(request.URL.Query().Get("wait"))
		answer(writer, backend.Probe(request.Context(), request.URL.Query().Get("label"), time.Duration(wait)*time.Millisecond))
	})
	mux.HandleFunc("GET /probe/self", func(writer http.ResponseWriter, request *http.Request) {
		answer(writer, backend.ProbeSelf(request.Context()))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = server.Close()
		_ = os.Remove(path)
	}()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Client asks a running backend through its socket.
type Client struct {
	Path string
}

func (c Client) call(method string, path string, timeout time.Duration, result any) error {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", c.Path)
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, "http://backend"+path, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return E.New("the backend answers ", response.Status)
	}
	return json.NewDecoder(response.Body).Decode(result)
}

// Status returns the state of the backend; an error when none runs.
func (c Client) Status() (*Status, error) {
	status := new(Status)
	err := c.call(http.MethodGet, "/status", 3*time.Second, status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// Facts returns what the commands need to know of what runs; an error when no backend
// runs.
func (c Client) Facts() (*Facts, error) {
	facts := new(Facts)
	err := c.call(http.MethodGet, "/facts", 3*time.Second, facts)
	if err != nil {
		return nil, err
	}
	return facts, nil
}

// Logs returns the lines of the log after the given number, and the number to ask with next.
func (c Client) Logs(since uint64) (*LogPage, error) {
	page := new(LogPage)
	err := c.call(http.MethodGet, "/logs?since="+strconv.FormatUint(since, 10), 5*time.Second, page)
	if err != nil {
		return nil, err
	}
	return page, nil
}

// Apply makes the backend apply the settings and returns what that came to.
func (c Client) Apply() (*Result, error) {
	result := new(Result)
	err := c.call(http.MethodPost, "/apply", 90*time.Second, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateLists makes the backend download every list now and returns what that came to.
func (c Client) UpdateLists() (*ListsUpdate, error) {
	result := new(ListsUpdate)
	err := c.call(http.MethodPost, "/lists/update", 6*time.Minute, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RestoreRouting asks the backend to put back what the network took, and returns what it
// did.
func (c Client) RestoreRouting() (string, error) {
	var result string
	err := c.call(http.MethodPost, "/routing/restore", 10*time.Second, &result)
	return result, err
}

// Lists returns what the backend knows of each list of what runs.
func (c Client) Lists() ([]ListState, error) {
	var states []ListState
	err := c.call(http.MethodGet, "/lists", 10*time.Second, &states)
	if err != nil {
		return nil, err
	}
	return states, nil
}

// Probe asks the backend what it saw of the check name of a label; the backend waits for
// the connection for the given time, its usual one when that is zero.
func (c Client) Probe(label string, wait time.Duration) (*ProbeResult, error) {
	result := new(ProbeResult)
	path := "/probe?label=" + url.QueryEscape(label) + "&wait=" + strconv.FormatInt(wait.Milliseconds(), 10)
	err := c.call(http.MethodGet, path, wait+15*time.Second, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ProbeSelf makes the backend check the router itself with a check name.
func (c Client) ProbeSelf() (*ProbeResult, error) {
	result := new(ProbeResult)
	err := c.call(http.MethodGet, "/probe/self", 15*time.Second, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
