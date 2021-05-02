package ohren

import (
	"bytes"
	"fmt"
	"github.com/miekg/dns"
	"net"
	"net/http"
	"strconv"
	"time"
)

type RequestType string

const (
	RequestTypeHttp = RequestType("HTTP connection")
	RequestTypeDNS  = RequestType("DNS request")
)

type RequestDetails interface {
	Type() RequestType
	Describe() string
	Hosts() []string
}

type HttpRequestDetails struct {
	Request  *http.Request
	Response *http.Response
}

func (d HttpRequestDetails) Type() RequestType {
	return RequestTypeHttp
}

func (d HttpRequestDetails) Hosts() []string {
	return []string{d.Request.Host}
}

func (d HttpRequestDetails) Describe() string {
	buffer := new(bytes.Buffer)
	buffer.WriteString("> Request:\n")
	d.Request.Write(buffer)
	buffer.WriteString("\n> Response:\n")
	d.Response.Write(buffer)
	return buffer.String()
}

type DnsRequestDetails struct {
	RequestedHosts []string
	Request        *dns.Msg
	Response       *dns.Msg
}

func (d DnsRequestDetails) Type() RequestType {
	return RequestTypeDNS
}

func (d DnsRequestDetails) Describe() string {
	return fmt.Sprintf("> Request: \n%s\n\n>Response:\n%s", d.Request.String(), d.Response.String())
}

func (d DnsRequestDetails) Hosts() []string {
	return d.RequestedHosts
}

type RecordedConnection struct {
	RemotePort    int
	RemoteAddress string
	LocalPort     int
	LocalAddress  string
	StartTime     time.Time
	EndTime       time.Time
	Details       RequestDetails
	Error         error
}

func (c *RecordedConnection) SetLocalAddress(addr net.Addr) {
	var err error
	var port string
	c.LocalAddress, port, err = net.SplitHostPort(addr.String())
	if err != nil {
		panic(err)
	}
	c.LocalPort, err = strconv.Atoi(port)
	if err != nil {
		panic(err)
	}
	return
}

func (c *RecordedConnection) SetRemoteAddress(addr net.Addr) {
	var err error
	var port string
	c.RemoteAddress, port, err = net.SplitHostPort(addr.String())
	if err != nil {
		panic(err)
	}
	c.RemotePort, err = strconv.Atoi(port)
	if err != nil {
		panic(err)
	}
	return
}

type Record *RecordedConnection
