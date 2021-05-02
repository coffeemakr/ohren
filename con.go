package ohren

import (
	"bytes"
	"io"
	"net"
	"time"
)

type ResettableReader interface {
	io.Reader
	io.ByteReader
	Reset() error
}

type recordingReader struct {
	buffer         bytes.Buffer
	recordedReader io.Reader
	usedReader     io.Reader
}

func (r *recordingReader) Read(p []byte) (n int, err error) {
	return r.usedReader.Read(p)
}

func ReadByte(reader io.Reader) (byte, error) {
	firstByte := make([]byte, 1)
	_, err := io.ReadFull(reader, firstByte)
	if err != nil {
		return 0, err
	} else {
		return firstByte[0], nil
	}
}

func (r *recordingReader) ReadByte() (byte, error) {
	return ReadByte(r)
}

func (r *recordingReader) Reset() error {
	r.usedReader = io.MultiReader(&r.buffer, r.recordedReader)
	return nil
}

func (r *recordingReader) Bytes() []byte {
	return r.buffer.Bytes()
}

func newResettableRecordedReader(r io.Reader) (reader *recordingReader) {
	if r == nil {
		panic("reader is nil")
	}
	reader = new(recordingReader)
	reader.recordedReader = io.TeeReader(r, &reader.buffer)
	reader.usedReader = reader.recordedReader
	return reader
}

type ResetConn interface {
	net.Conn
	Reset() error
}

func newResetConn(conn net.Conn) ResetConn {
	if conn == nil {
		panic("conn is nil")
	}
	return &resetConn{
		conn: conn,
		r:    newResettableRecordedReader(conn),
	}
}

type resetConn struct {
	conn net.Conn
	r ResettableReader
}

func (r *resetConn) Reset() error {
	return r.r.Reset()
}

func (r *resetConn) Read(b []byte) (n int, err error) {
	return r.r.Read(b)
}

func (r *resetConn) Write(b []byte) (n int, err error) {
	return r.conn.Write(b)
}

func (r *resetConn) Close() error {
	return r.conn.Close()
}

func (r *resetConn) LocalAddr() net.Addr {
	return r.conn.LocalAddr()
}

func (r *resetConn) RemoteAddr() net.Addr {
	return r.conn.RemoteAddr()
}

func (r *resetConn) SetDeadline(t time.Time) error {
	return r.conn.SetDeadline(t)
}

func (r *resetConn) SetReadDeadline(t time.Time) error {
	return r.conn.SetReadDeadline(t)
}

func (r *resetConn) SetWriteDeadline(t time.Time) error {
	return r.conn.SetWriteDeadline(t)
}
