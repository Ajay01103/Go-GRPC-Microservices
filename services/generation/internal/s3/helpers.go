package s3

import "bytes"

// bytesReader wraps a byte slice in an io.Reader for PutObject.
func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
