package ohren

import (
	"bufio"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)


func readLine(r io.Reader) ([]byte, error) {
	bufferedReader := bufio.NewReaderSize(io.LimitReader(r, 2048), 2048)
	return bufferedReader.ReadBytes('\n')
}

// Responder reads the reader fully and responds with an appropriate response
type Responder interface {
	Respond(conn net.Conn) (RequestDetails, error)
}

type Listener interface {
	Record( chan Record) error
}

type ResponderFunc func(conn net.Conn) (RequestDetails, error)

func (f ResponderFunc) Respond(conn net.Conn) (RequestDetails, error) {
	return f(conn)
}

func mustGetPort(addr net.Addr) int {
	_, rawPort, err := net.SplitHostPort(addr.String())
	if err != nil {
		panic(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		panic(err)
	}
	return port
}

type TcpListener struct {
	Listener  net.Listener
	Responder Responder
	Timeout   time.Duration
	WorkerCount int
}

func ProcessConnection(conn net.Conn, timeout time.Duration, responder Responder) Record {
	log.Printf("processing connection: %s", conn.RemoteAddr())
	record := new(RecordedConnection)
	record.StartTime = time.Now()
	record.SetLocalAddress(conn.LocalAddr())
	record.SetRemoteAddress(conn.RemoteAddr())
	details, err := responder.Respond(conn)
	if err != nil {
		log.Printf("error responding: %s\n", err)
	}

	record.EndTime = time.Now()

	record.Details = details
	return record
}

func (h TcpListener) handleConnection(connections chan net.Conn, out chan Record) {
	for conn := range connections {
		out <- ProcessConnection(conn, h.Timeout, h.Responder)
		if err := conn.Close(); err != nil {
			log.Printf("failed to close connection: %s", err)
		} else {
			log.Printf("connection from %s to %s closed", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}

func (h TcpListener) Record(out chan Record) error {
	var err error
	var con net.Conn
	var conChannel = make(chan net.Conn)
	var wg = new(sync.WaitGroup)

	for i := 0; i < h.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			h.handleConnection(conChannel, out)
			wg.Done()
		}()
	}

	for {
		con, err = h.Listener.Accept()
		if err != nil {
			close(conChannel)
			wg.Wait()
			return err
		}
		conChannel <- con
	}
}
