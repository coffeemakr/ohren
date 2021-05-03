package ohren

import (
	"context"
	"net"
	"time"
)

type UdpListener struct {
	Addr        net.Addr
	Responder   *DnsResponder
	Timeout     time.Duration
	WorkerCount int
}

func (u UdpListener) Record(records chan Record) error {
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(context.Background(), "udp", u.Addr.String())
	if err != nil {
		return err
	}
	udpConn := pc.(*net.UDPConn)
	defer pc.Close()
	for {
		udpConn.RemoteAddr()
		records <- ProcessConnection(udpConn, u.Timeout, u.Responder)
	}
}
