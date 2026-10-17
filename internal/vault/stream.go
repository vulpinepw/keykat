package vault

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	DefaultChunkSize = 1 << 20
	chunkHeaderSize  = 5
	flagLast         = 0x01
	maxChunks        = 1 << 32
)

var (
	ErrTruncated = errors.New("vault: encrypted stream is truncated")
	ErrCorrupt   = errors.New("vault: encrypted stream is malformed")
	ErrTrailing  = errors.New("vault: unexpected data after end of stream")
)

func noncePrefixSize(nonceSize int) int { return nonceSize - 5 }

func makeNonce(dst, prefix []byte, counter uint32, last bool) []byte {
	dst = append(dst[:0], prefix...)
	dst = binary.BigEndian.AppendUint32(dst, counter)
	if last {
		return append(dst, 1)
	}
	return append(dst, 0)
}

type streamWriter struct {
	w        io.Writer
	s        *suite
	prefixes [][]byte
	aad      []byte
	chunk    int
	buf      []byte
	scratch  [2][]byte
	nonce    []byte
	counter  uint64
	closed   bool
	err      error
}

func newStreamWriter(w io.Writer, s *suite, prefixes [][]byte, aad []byte, chunkSize int) *streamWriter {
	return &streamWriter{
		w:        w,
		s:        s,
		prefixes: prefixes,
		aad:      aad,
		chunk:    chunkSize,
		buf:      make([]byte, 0, chunkSize),
	}
}

func (sw *streamWriter) Write(p []byte) (int, error) {
	if sw.err != nil {
		return 0, sw.err
	}
	if sw.closed {
		return 0, errors.New("vault: write after close")
	}
	n := 0
	for len(p) > 0 {
		if len(sw.buf) == sw.chunk {
			if err := sw.flush(false); err != nil {
				sw.err = err
				return n, err
			}
		}
		k := copy(sw.buf[len(sw.buf):sw.chunk], p)
		sw.buf = sw.buf[:len(sw.buf)+k]
		p = p[k:]
		n += k
	}
	return n, nil
}

func (sw *streamWriter) flush(last bool) error {
	if sw.counter >= maxChunks {
		return errors.New("vault: stream exceeds maximum length")
	}
	data := sw.buf
	for i, l := range sw.s.layers {
		sw.nonce = makeNonce(sw.nonce, sw.prefixes[i], uint32(sw.counter), last)
		out := l.Seal(sw.scratch[i%2][:0], sw.nonce, data, sw.aad)
		sw.scratch[i%2] = out
		data = out
	}
	var hdr [chunkHeaderSize]byte
	binary.LittleEndian.PutUint32(hdr[:4], uint32(len(data)))
	if last {
		hdr[4] = flagLast
	}
	if _, err := sw.w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := sw.w.Write(data); err != nil {
		return err
	}
	zero(sw.buf)
	sw.buf = sw.buf[:0]
	sw.counter++
	return nil
}

func (sw *streamWriter) Close() error {
	if sw.err != nil {
		return sw.err
	}
	if sw.closed {
		return nil
	}
	sw.closed = true
	if err := sw.flush(true); err != nil {
		sw.err = err
		return err
	}
	return nil
}

type streamReader struct {
	r        io.Reader
	s        *suite
	prefixes [][]byte
	aad      []byte
	chunk    int
	ct       []byte
	scratch  [2][]byte
	nonce    []byte
	plain    []byte
	pos      int
	counter  uint64
	sawLast  bool
	err      error
}

func newStreamReader(r io.Reader, s *suite, prefixes [][]byte, aad []byte, chunkSize int) *streamReader {
	return &streamReader{r: r, s: s, prefixes: prefixes, aad: aad, chunk: chunkSize}
}

func (sr *streamReader) Read(p []byte) (int, error) {
	for sr.pos >= len(sr.plain) {
		if sr.err != nil {
			return 0, sr.err
		}
		if sr.sawLast {
			var one [1]byte
			if n, _ := io.ReadFull(sr.r, one[:]); n > 0 {
				sr.err = ErrTrailing
			} else {
				sr.err = io.EOF
			}
			return 0, sr.err
		}
		if err := sr.next(); err != nil {
			sr.err = err
			return 0, err
		}
	}
	n := copy(p, sr.plain[sr.pos:])
	sr.pos += n
	return n, nil
}

func (sr *streamReader) next() error {
	var hdr [chunkHeaderSize]byte
	if _, err := io.ReadFull(sr.r, hdr[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrTruncated
		}
		return err
	}
	size := int(binary.LittleEndian.Uint32(hdr[:4]))
	flags := hdr[4]
	if flags&^flagLast != 0 || size < sr.s.Overhead() || size > sr.chunk+sr.s.Overhead() {
		return ErrCorrupt
	}
	if sr.counter >= maxChunks {
		return ErrCorrupt
	}
	if cap(sr.ct) < size {
		sr.ct = make([]byte, size)
	}
	sr.ct = sr.ct[:size]
	if _, err := io.ReadFull(sr.r, sr.ct); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return ErrTruncated
		}
		return err
	}
	last := flags&flagLast != 0
	data := sr.ct
	for i := len(sr.s.layers) - 1; i >= 0; i-- {
		l := sr.s.layers[i]
		sr.nonce = makeNonce(sr.nonce, sr.prefixes[i], uint32(sr.counter), last)
		out, err := l.Open(sr.scratch[i%2][:0], sr.nonce, data, sr.aad)
		if err != nil {
			return ErrAuth
		}
		sr.scratch[i%2] = out
		data = out
	}
	if !last && len(data) != sr.chunk {
		return ErrCorrupt
	}
	sr.plain, sr.pos = data, 0
	sr.counter++
	sr.sawLast = last
	return nil
}
