package mdns

import (
	"net"
	"os"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
)

type fakeFinder struct {
	control.InterfaceFinder
	interfaces []control.Interface
}

func (f *fakeFinder) Interfaces() []control.Interface { return f.interfaces }

func (f *fakeFinder) ByName(name string) (*control.Interface, error) {
	for i := range f.interfaces {
		if f.interfaces[i].Name == name {
			return &f.interfaces[i], nil
		}
	}
	return nil, os.ErrNotExist
}

type fakeNetworkManager struct {
	adapter.NetworkManager
	finder control.InterfaceFinder
}

func (m *fakeNetworkManager) InterfaceFinder() control.InterfaceFinder { return m.finder }

func interfaceNamesOf(interfaces []control.Interface) []string {
	names := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		names = append(names, iface.Name)
	}
	return names
}

// A query multicast into a tun that routes back into sing-box loops forever,
// so automatic selection skips point-to-point links; an explicit name wins.
func TestFetchInterfacesSkipsPointToPointUnlessNamed(t *testing.T) {
	up := net.FlagUp | net.FlagMulticast
	finder := &fakeFinder{interfaces: []control.Interface{
		{Name: "br0", Flags: up},
		{Name: "t2s0", Flags: up | net.FlagPointToPoint},
		{Name: "lo", Flags: up | net.FlagLoopback},
		{Name: "nwg0", Flags: net.FlagUp | net.FlagPointToPoint},
	}}
	transport := &Transport{
		logger:         logger.NOP(),
		networkManager: &fakeNetworkManager{finder: finder},
	}
	interfaces, err := transport.fetchInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if names := interfaceNamesOf(interfaces); len(names) != 1 || names[0] != "br0" {
		t.Errorf("automatic selection = %v, want [br0]", names)
	}

	transport.interfaceNames = []string{"t2s0"}
	interfaces, err = transport.fetchInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if names := interfaceNamesOf(interfaces); len(names) != 1 || names[0] != "t2s0" {
		t.Errorf("explicit selection = %v, want [t2s0]", names)
	}
}
