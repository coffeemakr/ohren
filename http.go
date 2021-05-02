package ohren

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
)

var (
	http1Regex = regexp.MustCompile("HTTP/(1|1.1)\r?\n?$")
	http2Regex = regexp.MustCompile("HTTP/2\r?\n?$")
)

type HttpType string

const (
	HttpTypeUnknown HttpType = "Unknown"
	HttpTypePlain1           = "HTTP"
	HttpTypeTls              = "HTTP+TLS"
)

const defaultHttpResponse = `<html>
  <head>
    <title>An Example Page</title>
  </head>
  <body>
    <p>Hello World, this is a very simple HTML document.</p>
  </body>
</html>
` // Thanks wikipedia

var DefaultHtmlResponder = &HtmlResponder{
	ContentType:     "text/html; charset=utf-8",
	ResponseContent: defaultHttpResponse,
}

var DefaultHttpResponder = &MultiHttpResponder{
	HttpResponder: DefaultHtmlResponder,
}

func getHttpProtocol(head []byte) int {
	switch {
	case http1Regex.Match(head):
		return 1
	case http2Regex.Match(head):
		return 2
	default:
		return 0
	}
}

type HtmlResponder struct {
	ResponseContent string
	ContentType     string
}

func (r *HtmlResponder) Respond(conn net.Conn) (RequestDetails, error) {
	var err error
	var request *http.Request
	request, err = http.ReadRequest(bufio.NewReaderSize(conn, 2048))
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(io.Discard, request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %s", err)
	}
	bodyBytes := []byte(r.ResponseContent)
	response := &http.Response{
		StatusCode:   200,
		Status:       "OK",
		Close:        true,
		Uncompressed: true,
		Proto:        request.Proto,
		ProtoMajor:   request.ProtoMajor,
		ProtoMinor:   request.ProtoMinor,
		Header: http.Header{
			"Content-Type": []string{r.ContentType},
		},
		ContentLength: int64(len(bodyBytes)),
		Body:          ioutil.NopCloser(bytes.NewReader(bodyBytes)),
	}

	err = response.Write(conn)
	return &HttpRequestDetails{
		Request:  request,
		Response: response,
	}, err
}

func detectHttpProtocol(reader io.Reader) (HttpType, error) {
	firstByte, err := ReadByte(reader)
	if err != nil {
		log.Printf("failed to read first byte: %s\n", err)
		return HttpTypeUnknown, err
	}
	log.Printf("First byte is %d\n", firstByte)

	if firstByte == 22 {
		// TLS connection
		return HttpTypeTls, nil
	} else if firstByte <= 127 {
		head, err := readLine(reader)
		head = append([]byte{firstByte}, head...)
		if err != nil && err != io.EOF {
		} else if err == nil {
			httpProtocol := getHttpProtocol(head)
			if httpProtocol != 0 {
				if httpProtocol == 1 {
					return HttpTypePlain1, nil
				}
			}
		} else {
			log.Println("no newline")
		}
	}
	return HttpTypeUnknown, nil
}

type MultiHttpResponder struct {
	HttpResponder  Responder
	HttpsResponder Responder
}

func (m MultiHttpResponder) WithTlsConfig(config *tls.Config) *MultiHttpResponder {
	return &MultiHttpResponder{
		HttpResponder:  m.HttpResponder,
		HttpsResponder: &TlsResponder{
			Config:         config,
			PlainResponder: m.HttpResponder,
		},
	}
}

func (m MultiHttpResponder) Respond(conn net.Conn) (RequestDetails, error) {
	resetConn := newResetConn(conn)
	var responder Responder
	protocol, err := detectHttpProtocol(resetConn)
	if err != nil {
		return nil, err
	}
	err = resetConn.Reset()
	if err != nil {
		return nil, err
	}
	switch protocol {
	case HttpTypePlain1:
		responder = m.HttpResponder
	case HttpTypeTls:
		responder = m.HttpsResponder
	}
	if responder == nil {
		return nil, fmt.Errorf("no responder for protocol: %s", protocol)
	}
	return responder.Respond(resetConn)
}
