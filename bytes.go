package sox

import (
	"errors"
	"io"
)

var errInvalidSeek = errors.New("invalid seek")

// bytesReader wraps []byte to implement io.ReadSeeker
type bytesReader struct {
	data []byte
	pos  int64
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += int64(n)
	return n, nil
}

func (r *bytesReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = int64(len(r.data)) + offset
	default:
		return 0, errInvalidSeek
	}
	if abs < 0 {
		return 0, errInvalidSeek
	}
	r.pos = abs
	return abs, nil
}
