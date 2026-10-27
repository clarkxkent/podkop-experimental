// Package subscription reads what a provider lists at the address of a subscription: proxy
// links, one a line, the whole often in base64. It downloads the list (Download), takes it
// apart into servers (Parse), finds the servers this sing-box cannot use (Check), keeps the
// last list that was taken (Store) and tells the provider which device asks (ReadDevice).
//
// The address and the links are secrets: the token in the address is the account, a link
// holds the keys of a server. No message of this package repeats them.
package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	E "github.com/sagernet/sing/common/exceptions"
)

// Server is a server a subscription lists.
type Server struct {
	// Name is what the provider calls it: the part of the link after "#".
	Name string `json:"name"`
	// Link is the proxy link, a secret.
	Link string `json:"link"`
	// ID tells servers apart whatever the provider names them: a hash of the outbound the
	// link decodes to, without its name. A link that cannot be decoded has none.
	ID string `json:"id,omitempty"`
	// Reason tells why the server cannot be used; empty when it can.
	Reason string `json:"reason,omitempty"`
	// Notes are what the link asks for and the outbound cannot do.
	Notes []string `json:"notes,omitempty"`

	// outbound is the decoded outbound without its tag, for Check
	outbound []byte
}

// Usable tells whether sing-box can use the server.
func (s *Server) Usable() bool {
	return s.Reason == ""
}

// Decoder turns a proxy link into an outbound, a JSON object whose tag is the name of the
// link, and the warnings about it (engine.DecodeLink).
type Decoder func(link string) (outbound []byte, warnings []string, err error)

// ErrPage is a web page in place of a list: as a rule the page an ISP shows for a site it
// blocks. Another way to the provider may get the list.
var ErrPage = E.New("the provider sent a web page, not a list of servers")

// Parse takes the body of an answer of a provider apart into its servers, in its order. A
// line that is not a proxy link is skipped: the comments some providers put in (#…) and the
// lines for other programs (happ://…). Links that cannot be decoded are servers with a
// reason; a body with no link at all is refused.
func Parse(body []byte, decode Decoder) ([]Server, error) {
	text := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
	if text == "" {
		return nil, E.New("the provider sent no servers")
	}
	if !strings.Contains(text, "://") {
		if decoded, isBase64 := decodeBase64(text); isBase64 {
			text = strings.TrimSpace(strings.TrimPrefix(decoded, "\ufeff"))
		}
	}
	switch {
	case strings.HasPrefix(text, "<"):
		return nil, ErrPage
	case strings.HasPrefix(text, "{"), strings.HasPrefix(text, "["):
		return nil, E.New("the provider sent a configuration in JSON, not a list of links: other formats are not read yet")
	case isClashYAML(text):
		return nil, E.New("the provider sent a configuration of Clash, not a list of links: other formats are not read yet")
	}
	var servers []Server
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		scheme, _, isLink := strings.Cut(line, "://")
		if !isLink || strings.ContainsAny(scheme, " \t") || strings.EqualFold(scheme, "happ") {
			continue
		}
		servers = append(servers, parseLink(line, len(servers)+1, decode))
	}
	if len(servers) == 0 {
		return nil, E.New("the provider sent no proxy links")
	}
	return servers, nil
}

// decodeBase64 reads a body in base64, with or without padding, of either alphabet, broken
// into lines or not. It tells false when the body is not base64 of text.
func decodeBase64(text string) (string, bool) {
	compact := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, text)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(compact)
		if err == nil && utf8.Valid(decoded) {
			return string(decoded), true
		}
	}
	return "", false
}

// isClashYAML tells a configuration of Clash by its list of proxies.
func isClashYAML(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "proxies:") {
			return true
		}
	}
	return false
}

func parseLink(link string, number int, decode Decoder) Server {
	server := Server{Link: link}
	content, warnings, err := decode(link)
	if err != nil {
		server.Name = cleanName(fragmentOf(link), number)
		server.Reason = err.Error()
		return server
	}
	var fields map[string]any
	err = json.Unmarshal(content, &fields)
	if err != nil {
		server.Name = cleanName(fragmentOf(link), number)
		server.Reason = "the outbound of the link is not a JSON object"
		return server
	}
	name, _ := fields["tag"].(string)
	delete(fields, "tag")
	// the keys of a map are written in order: the same outbound is the same bytes
	canonical, err := json.Marshal(fields)
	if err != nil {
		server.Name = cleanName(name, number)
		server.Reason = "the outbound of the link cannot be encoded"
		return server
	}
	sum := sha256.Sum256(canonical)
	server.ID = hex.EncodeToString(sum[:12])
	server.Name = cleanName(name, number)
	server.Notes = warnings
	server.outbound = canonical
	return server
}

// fragmentOf is the name a link that cannot be decoded has: the part after "#".
func fragmentOf(link string) string {
	at := strings.LastIndexByte(link, '#')
	if at < 0 {
		return ""
	}
	fragment := link[at+1:]
	if unescaped, err := url.PathUnescape(fragment); err == nil {
		return unescaped
	}
	return fragment
}

// cleanName is the name of a server as it is shown: on one line, without what Remnawave adds
// after the name for some clients (?serverDescription=…), or "server N" when it has none.
func cleanName(name string, number int) string {
	name, _, _ = strings.Cut(name, "?serverDescription=")
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "server " + strconv.Itoa(number)
	}
	return name
}
