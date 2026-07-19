package awg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"testing"

	"github.com/amnezia-vpn/amneziawg-go/device"
	awgTun "github.com/amnezia-vpn/amneziawg-go/tun"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/metadata"
)

// fakeTun is a minimal tunAdapter used to exercise returnDeviceWrapper without
// a real (network or system) tun underneath.
type fakeTun struct {
	started bool
	closed  chan struct{}
	mu      sync.Mutex
	written [][]byte // packets observed via Write
}

func newFakeTun() *fakeTun {
	return &fakeTun{closed: make(chan struct{})}
}

func (f *fakeTun) Start() error { f.started = true; return nil }
func (f *fakeTun) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}
func (f *fakeTun) File() *os.File              { return nil }
func (f *fakeTun) MTU() (int, error)           { return 1408, nil }
func (f *fakeTun) Name() (string, error)       { return "fake", nil }
func (f *fakeTun) Events() <-chan awgTun.Event { return nil }
func (f *fakeTun) BatchSize() int              { return 1 }

func (f *fakeTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	<-f.closed
	return 0, errors.New("closed")
}

func (f *fakeTun) Write(bufs [][]byte, offset int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range bufs {
		packet := make([]byte, len(b)-offset)
		copy(packet, b[offset:])
		f.written = append(f.written, packet)
	}
	return len(bufs), nil
}

func (f *fakeTun) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTun) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("not implemented")
}

// fakeReturn is a minimal tun.Return used to test the return-path in
// returnDeviceWrapper.Write.
type fakeReturn struct {
	headroom int
	consume  bool // if true, ReturnPackets consumes everything it is given
}

func (r *fakeReturn) ReturnHeadroom() int { return r.headroom }
func (r *fakeReturn) ReturnPackets(packets [][]byte) [][]byte {
	if r.consume {
		return nil
	}
	return packets
}

var _ tun.Return = (*fakeReturn)(nil)

func TestReturnDeviceWriteNoReturnPathFallsThrough(t *testing.T) {
	fake := newFakeTun()
	rd := newReturnDevice(fake)
	bufs := [][]byte{append(make([]byte, 16), []byte{1, 2, 3}...)}
	n, err := rd.Write(bufs, 16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 packet written, got %d", n)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.written) != 1 {
		t.Fatalf("expected the real tun to observe the packet, got %d", len(fake.written))
	}
}

func TestReturnDeviceWriteConsumedByReturnPath(t *testing.T) {
	fake := newFakeTun()
	rd := newReturnDevice(fake)
	rd.state.Store(&returnPathState{returnPath: &fakeReturn{headroom: 4, consume: true}, headroom: 4})

	bufs := [][]byte{append(make([]byte, 16), []byte{1, 2, 3}...)}
	_, err := rd.Write(bufs, 16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.written) != 0 {
		t.Fatal("expected the packet to be consumed by the return path, not delivered to the real tun")
	}
}

func TestDeviceAttachDetachReturn(t *testing.T) {
	fake := newFakeTun()
	d := &Device{returnDevice: newReturnDevice(fake)}

	rp := &fakeReturn{headroom: 4}
	if err := d.AttachReturn(rp); err != nil {
		t.Fatalf("unexpected error attaching: %v", err)
	}
	// re-attaching the same return path is a no-op
	if err := d.AttachReturn(rp); err != nil {
		t.Fatalf("unexpected error re-attaching the same path: %v", err)
	}
	other := &fakeReturn{headroom: 4}
	if err := d.AttachReturn(other); err == nil {
		t.Fatal("expected an error attaching a second return path")
	}
	if err := d.DetachReturn(rp); err != nil {
		t.Fatalf("unexpected error detaching: %v", err)
	}
	if err := d.AttachReturn(other); err != nil {
		t.Fatalf("unexpected error attaching after detach: %v", err)
	}
}

func TestDeviceAttachReturnHeadroomTooLarge(t *testing.T) {
	fake := newFakeTun()
	d := &Device{returnDevice: newReturnDevice(fake)}
	if err := d.AttachReturn(&fakeReturn{headroom: device.MessageTransportOffsetContent + 1}); err == nil {
		t.Fatal("expected an error for a return path headroom exceeding availability")
	}
}

func TestDeviceWritePacketsRequiresStarted(t *testing.T) {
	fake := newFakeTun()
	d := &Device{returnDevice: newReturnDevice(fake)}
	if err := d.WritePackets([][]byte{{1, 2, 3}}); err == nil {
		t.Fatal("expected an error writing packets before the device is started")
	}
}

func TestDevicePortAddressesAndMTU(t *testing.T) {
	v4 := netip.MustParsePrefix("10.0.0.2/32")
	v6 := netip.MustParsePrefix("fd00::2/128")
	d := &Device{address: []netip.Prefix{v4, v6}, mtu: 1408}
	gotV4, gotV6 := d.PortAddresses()
	if gotV4 != v4.Addr() || gotV6 != v6.Addr() {
		t.Fatalf("unexpected addresses: v4=%v v6=%v", gotV4, gotV6)
	}
	if d.PortMTU() != 1408 {
		t.Fatalf("unexpected MTU: %d", d.PortMTU())
	}
}

// TestDeviceCloseDoesNotDoubleCloseTun reproduces the review's CRITICAL 1:
// a real awgDevice on a networkTun already closes the underlying tun in
// awgDevice.Close(); Device.Close() must not close it a second time (which
// panicked with "close of closed channel" on the netstack tun).
func TestDeviceCloseDoesNotDoubleCloseTun(t *testing.T) {
	realTun, err := newNetworkTun([]netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")}, 1408)
	if err != nil {
		t.Fatalf("create network tun: %v", err)
	}
	rd := newReturnDevice(realTun)
	logger := &device.Logger{
		Verbosef: func(string, ...any) {},
		Errorf:   func(string, ...any) {},
	}
	awgDev := device.NewDevice(rd, newBind(context.Background(), nil), logger)

	d := &Device{
		awgDevice:    awgDev,
		tun:          realTun,
		returnDevice: rd,
	}
	d.started.Store(true)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Device.Close panicked (double close of tun): %v", r)
		}
	}()
	if err := d.Close(); err != nil {
		t.Fatalf("unexpected error closing device: %v", err)
	}
}
