package ohren

import (
	"crypto/tls"
	"log"
	"net"
)

type TlsResponder struct {
	Config *tls.Config
	PlainResponder Responder
}

func (t TlsResponder) Respond(conn net.Conn) (RequestDetails, error) {
	var err error
	tlsConn := tls.Server(conn, t.Config)
	err = tlsConn.Handshake()
	if err != nil {
		return nil, err
	}
	defer func(tlsConn *tls.Conn) {
		err := tlsConn.Close()
		if err != nil {
			log.Println(err)
		}
	}(tlsConn)
	return t.PlainResponder.Respond(tlsConn)
}
