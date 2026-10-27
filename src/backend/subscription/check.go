package subscription

import (
	"strconv"
	"strings"
)

// Check finds the servers this sing-box cannot build and gives them their reason. The
// servers whose ID has a verdict in known take it: a server is built once, when it first
// comes. The others are built all at once, as the outbounds of one configuration
// (checkAll, engine.Check), and one by one by their links (checkOne, engine.CheckLink) only
// when that fails, to find which. It returns the verdicts of every server it had, for the
// next time.
func Check(servers []Server, known map[string]string, checkAll func(configuration []byte) error, checkOne func(link string) error) map[string]string {
	verdicts := make(map[string]string)
	var fresh []int
	for index := range servers {
		server := &servers[index]
		if server.ID == "" {
			continue
		}
		if reason, found := known[server.ID]; found {
			server.Reason = reason
			verdicts[server.ID] = reason
			continue
		}
		if server.Usable() && server.outbound != nil {
			fresh = append(fresh, index)
		}
	}
	if len(fresh) == 0 {
		return verdicts
	}
	var configuration strings.Builder
	configuration.WriteString(`{"log":{"disabled":true},"outbounds":[`)
	for position, index := range fresh {
		if position > 0 {
			configuration.WriteByte(',')
		}
		// the outbound is an object: the tag goes in first
		configuration.WriteString(`{"tag":"check-` + strconv.Itoa(position) + `"`)
		if rest := servers[index].outbound[1:]; rest[0] != '}' {
			configuration.WriteByte(',')
			configuration.Write(rest)
		} else {
			configuration.WriteByte('}')
		}
	}
	configuration.WriteString("]}")
	if checkAll([]byte(configuration.String())) != nil {
		for _, index := range fresh {
			if err := checkOne(servers[index].Link); err != nil {
				servers[index].Reason = Refusal(err)
			}
		}
	}
	for _, index := range fresh {
		verdicts[servers[index].ID] = servers[index].Reason
	}
	return verdicts
}

// Refusal is the reason a link is refused for, without the advice of the engine to install
// its own packages, which are not those of podkop.
func Refusal(err error) string {
	message, _, _ := strings.Cut(err.Error(), ", install podkop-engine-full")
	return message
}
