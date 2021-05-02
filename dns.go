package ohren

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"io"
	"log"
	"net"
)

type DnsResponder struct {
	Addresses []net.IP
	TTL       uint32
}

const defaultTTL = 60 * 5

func (d DnsResponder) Respond(conn net.Conn) (RequestDetails, error) {
	var length uint16
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	m := make([]byte, length)
	if _, err := io.ReadFull(conn, m); err != nil {
		return nil, err
	}

	req := new(dns.Msg)
	err := req.Unpack(m)
	if err != nil {
		return nil, err
	}

	resp := new(dns.Msg)
	resp = resp.SetReply(req)

	if req.Opcode != dns.OpcodeQuery {
		return nil, err
	}

	answersByHost := make(map[string]uint32)
	const (
		AAnswered   uint32 = 1 << iota
		AAAAAnswered uint32 = 1 << iota
	)

	for _, question := range req.Question {
		log.Println(question)
		if question.Qclass == dns.ClassINET {
			switch question.Qtype {
			case dns.TypeA:
				if 0 == answersByHost[question.Name] & AAnswered {
					log.Println("adding A answers")
					resp.Answer = append(resp.Answer, d.getARecords(question.Name)...)
					answersByHost[question.Name] |= AAnswered
				}
			case dns.TypeAAAA:
				if 0 == answersByHost[question.Name] & AAAAAnswered {
					resp.Answer = append(resp.Answer, d.getAAAARecord(question.Name)...)
					answersByHost[question.Name] |= AAAAAnswered
				}
			}
		}
	}

	respBytes, err := resp.Pack()
	if len(respBytes) > dns.MaxMsgSize {
		return nil, errors.New("response too big")
	}
	// Add length
	lengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBytes, uint16(resp.Len()))
	respBytes = append(lengthBytes, respBytes...)
	log.Println(respBytes)
	if err != nil {
		return nil, fmt.Errorf("error packing dns: %s", err)
	}
	log.Printf("Writing response message: %s\n", resp)

	//respBytes = append([]byte{0x00}, respBytes...)

	_, err = conn.Write(respBytes)
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(answersByHost))
	for host := range answersByHost {
		hosts = append(hosts, host)
	}

	return DnsRequestDetails{
		RequestedHosts: hosts,
		Request:        req,
		Response:       resp,
	}, nil
}

func (d DnsResponder) getARecords(name string) (records []dns.RR) {
	for _, ip := range d.Addresses {
		ip = ip.To4()
		if ip == nil {
			continue
		}
		records = append(records, &dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			A: ip,
		})
	}
	return
}


func (d DnsResponder) getAAAARecord(name string) (records []dns.RR) {
	for _, ip := range d.Addresses {
		if len(ip) != net.IPv6len {
			continue
		}
		records = append(records, &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    d.ttl(),
			},
			AAAA: ip,
		})
	}
	return
}


func (d DnsResponder) ttl() uint32 {
	if d.TTL == 0 {
		return defaultTTL
	} else {
		return d.TTL
	}
}
