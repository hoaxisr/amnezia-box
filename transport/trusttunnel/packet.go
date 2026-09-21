package trusttunnel

import (
	"encoding/binary"
	"math"
	"net"
	"net/netip"

	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"
)

func truncateAppName(name string) string {
	if len(name) > math.MaxUint8 {
		name = name[:math.MaxUint8]
	}
	return name
}

type udpConn struct {
	httpConn
	resolveFunc     func(fqdn string) (netip.Addr, error)
	readWaitOptions N.ReadWaitOptions
}

const (
	packetAddressPortLen       = 16 + 2
	packetLengthLen            = 4
	packetAppNameLengthLen     = 1
	clientPacketHeaderBaseLen  = packetLengthLen + packetAddressPortLen + packetAddressPortLen + packetAppNameLengthLen
	serverPacketHeaderFixedLen = packetLengthLen + packetAddressPortLen + packetAddressPortLen
)

func clientPacketHeaderLen(nameLen int) int {
	return clientPacketHeaderBaseLen + nameLen
}

func packetWriteBuffer(payload []byte, frontHeadroom int) *buf.Buffer {
	buffer := buf.NewSize(frontHeadroom + len(payload))
	buffer.Resize(frontHeadroom, 0)
	common.Must1(buffer.Write(payload))
	return buffer
}

func (c *udpConn) InitializeReadWaiter(options N.ReadWaitOptions) (needCopy bool) {
	c.readWaitOptions = options
	return false
}

var (
	_ N.NetPacketConn    = (*clientUDPConn)(nil)
	_ N.FrontHeadroom    = (*clientUDPConn)(nil)
	_ N.PacketReadWaiter = (*clientUDPConn)(nil)
)

type clientUDPConn struct {
	udpConn
	appName string
}

func (c *clientUDPConn) FrontHeadroom() int {
	return clientPacketHeaderLen(len(c.appName))
}

func (c *clientUDPConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer = c.readWaitOptions.NewPacketBuffer()
	destination, err = c.ReadPacket(buffer)
	if err != nil {
		buffer.Release()
		return nil, M.Socksaddr{}, err
	}
	c.readWaitOptions.PostReturn(buffer)
	return buffer, destination, nil
}

func (c *clientUDPConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = c.waitCreated()
	if err != nil {
		return M.Socksaddr{}, err
	}
	return c.readPacketFromServer(buffer)
}

func (c *clientUDPConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := c.ReadPacket(buffer)
	if err != nil {
		return 0, nil, err
	}
	return buffer.Len(), destination.UDPAddr(), nil
}

func (c *clientUDPConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return c.writePacketToServer(buffer, destination)
}

func (c *clientUDPConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = c.WritePacket(packetWriteBuffer(p, clientPacketHeaderLen(len(c.appName))), M.SocksaddrFromNet(addr))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *clientUDPConn) readPacketFromServer(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	header := buf.NewSize(4 + 16 + 2 + 16 + 2)
	defer header.Release()
	_, err = header.ReadFullFrom(c.body, header.Cap())
	if err != nil {
		err = c.wrapError(err)
		return
	}
	var length uint32
	common.Must(binary.Read(header, binary.BigEndian, &length))
	var sourceAddressBuffer [16]byte
	common.Must1(header.Read(sourceAddressBuffer[:]))
	destination.Addr = parse16BytesIP(sourceAddressBuffer)
	common.Must(binary.Read(header, binary.BigEndian, &destination.Port))
	common.Must(rw.SkipN(header, 16+2)) // To local address:port
	payloadLen := int(length) - (16 + 2 + 16 + 2)
	if payloadLen < 0 {
		return M.Socksaddr{}, E.New("invalid udp length: ", length)
	}
	_, err = buffer.ReadFullFrom(c.body, payloadLen)
	err = c.wrapError(err)
	return
}

func (c *clientUDPConn) writePacketToServer(buffer *buf.Buffer, source M.Socksaddr) error {
	defer buffer.Release()
	if !source.IsIP() {
		if c.resolveFunc == nil {
			return E.New("write to without resolveFunc")
		}
		ip, err := c.resolveFunc(source.Fqdn)
		if err != nil {
			return err
		}
		source.Addr = ip
	}
	payloadLen := buffer.Len()
	headerLen := clientPacketHeaderBaseLen + len(c.appName)
	lengthField := uint32(packetAddressPortLen + packetAddressPortLen + packetAppNameLengthLen + len(c.appName) + payloadLen)
	destinationAddress := buildPaddingIP(source.Addr)

	var (
		header         *buf.Buffer
		headerInBuffer bool
	)
	if buffer.Start() >= headerLen {
		headerBytes := buffer.ExtendHeader(headerLen)
		header = buf.With(headerBytes)
		headerInBuffer = true
	} else {
		header = buf.NewSize(headerLen)
		defer header.Release()
	}
	common.Must(binary.Write(header, binary.BigEndian, lengthField))
	common.Must(header.WriteZeroN(16 + 2)) // Source address:port (unknown)
	common.Must1(header.Write(destinationAddress[:]))
	common.Must(binary.Write(header, binary.BigEndian, source.Port))
	common.Must(binary.Write(header, binary.BigEndian, uint8(len(c.appName))))
	common.Must1(header.WriteString(c.appName))
	if !headerInBuffer {
		_, err := c.writer.Write(header.Bytes())
		if err != nil {
			return c.wrapError(err)
		}
	}
	_, err := c.writer.Write(buffer.Bytes())
	if err != nil {
		return c.wrapError(err)
	}
	if c.flusher != nil {
		c.flusher.Flush()
	}
	return nil
}

var (
	_ N.NetPacketConn    = (*serverUDPConn)(nil)
	_ N.FrontHeadroom    = (*serverUDPConn)(nil)
	_ N.PacketReadWaiter = (*serverUDPConn)(nil)
)

type serverUDPConn struct {
	udpConn
}

func (s *serverUDPConn) FrontHeadroom() int {
	return serverPacketHeaderFixedLen
}

func (s *serverUDPConn) WaitReadPacket() (buffer *buf.Buffer, destination M.Socksaddr, err error) {
	buffer = s.readWaitOptions.NewPacketBuffer()
	destination, err = s.ReadPacket(buffer)
	if err != nil {
		buffer.Release()
		return nil, M.Socksaddr{}, err
	}
	s.readWaitOptions.PostReturn(buffer)
	return buffer, destination, nil
}

func (s *serverUDPConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = s.waitCreated()
	if err != nil {
		return M.Socksaddr{}, err
	}
	return s.readPacketFromClient(buffer)
}

func (s *serverUDPConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	buffer := buf.With(p)
	destination, err := s.ReadPacket(buffer)
	if err != nil {
		return 0, nil, err
	}
	return buffer.Len(), destination.UDPAddr(), nil
}

func (s *serverUDPConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return s.writePacketToClient(buffer, destination)
}

func (s *serverUDPConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = s.WritePacket(packetWriteBuffer(p, serverPacketHeaderFixedLen), M.SocksaddrFromNet(addr))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *serverUDPConn) readPacketFromClient(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	header := buf.NewSize(4 + 16 + 2 + 16 + 2 + 1)
	defer header.Release()
	_, err = header.ReadFullFrom(s.body, header.Cap())
	if err != nil {
		err = s.wrapError(err)
		return
	}
	var length uint32
	common.Must(binary.Read(header, binary.BigEndian, &length))
	var sourceAddressBuffer [16]byte
	common.Must1(header.Read(sourceAddressBuffer[:]))
	var sourcePort uint16
	common.Must(binary.Read(header, binary.BigEndian, &sourcePort))
	_ = sourcePort
	var destinationAddressBuffer [16]byte
	common.Must1(header.Read(destinationAddressBuffer[:]))
	destination.Addr = parse16BytesIP(destinationAddressBuffer)
	common.Must(binary.Read(header, binary.BigEndian, &destination.Port))
	var appNameLen uint8
	common.Must(binary.Read(header, binary.BigEndian, &appNameLen))
	if appNameLen > 0 {
		err = rw.SkipN(s.body, int(appNameLen))
		if err != nil {
			err = s.wrapError(err)
			return M.Socksaddr{}, err
		}
	}
	payloadLen := int(length) - (16 + 2 + 16 + 2 + 1 + int(appNameLen))
	if payloadLen < 0 {
		return M.Socksaddr{}, E.New("invalid udp length: ", length)
	}
	_, err = buffer.ReadFullFrom(s.body, payloadLen)
	err = s.wrapError(err)
	return
}

func (s *serverUDPConn) writePacketToClient(buffer *buf.Buffer, source M.Socksaddr) error {
	defer buffer.Release()
	if !source.IsIP() {
		return E.New("only support IP")
	}
	payloadLen := buffer.Len()
	headerLen := serverPacketHeaderFixedLen
	lengthField := uint32(packetAddressPortLen + packetAddressPortLen + payloadLen)
	sourceAddress := buildPaddingIP(source.Addr)
	var destinationAddress [16]byte
	var destinationPort uint16
	var (
		header         *buf.Buffer
		headerInBuffer bool
	)
	if buffer.Start() >= headerLen {
		headerBytes := buffer.ExtendHeader(headerLen)
		header = buf.With(headerBytes)
		headerInBuffer = true
	} else {
		header = buf.NewSize(headerLen)
		defer header.Release()
	}
	common.Must(binary.Write(header, binary.BigEndian, lengthField))
	common.Must1(header.Write(sourceAddress[:]))
	common.Must(binary.Write(header, binary.BigEndian, source.Port))
	common.Must1(header.Write(destinationAddress[:]))
	common.Must(binary.Write(header, binary.BigEndian, destinationPort))
	if !headerInBuffer {
		_, err := s.writer.Write(header.Bytes())
		if err != nil {
			return s.wrapError(err)
		}
	}
	_, err := s.writer.Write(buffer.Bytes())
	if err != nil {
		return s.wrapError(err)
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}
