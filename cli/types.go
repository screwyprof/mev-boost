package cli

import (
	"errors"
	"net/url"
	"strings"

	"github.com/flashbots/mev-boost/config/relay"
)

var errDuplicateEntry = errors.New("duplicate entry")

type relayList []relay.Entry

func (r *relayList) String() string {
	return strings.Join(relay.EntriesToStrings(*r), ",")
}

func (r *relayList) Contains(relay relay.Entry) bool {
	for _, entry := range *r {
		if relay.String() == entry.String() {
			return true
		}
	}
	return false
}

func (r *relayList) Set(value string) error {
	relay, err := relay.NewRelayEntry(value)
	if err != nil {
		return err
	}
	if r.Contains(relay) {
		return errDuplicateEntry
	}
	*r = append(*r, relay)
	return nil
}

type relayMonitorList []*url.URL

func (rm *relayMonitorList) String() string {
	relayMonitors := []string{}
	for _, relayMonitor := range *rm {
		relayMonitors = append(relayMonitors, relayMonitor.String())
	}
	return strings.Join(relayMonitors, ",")
}

func (rm *relayMonitorList) Contains(relayMonitor *url.URL) bool {
	for _, entry := range *rm {
		if relayMonitor.String() == entry.String() {
			return true
		}
	}
	return false
}

func (rm *relayMonitorList) Set(value string) error {
	relayMonitor, err := url.Parse(value)
	if err != nil {
		return err
	}
	if rm.Contains(relayMonitor) {
		return errDuplicateEntry
	}
	*rm = append(*rm, relayMonitor)
	return nil
}
