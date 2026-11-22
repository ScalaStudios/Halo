package remote

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func TestRecorderAppliesSharedLimitToResizeAndOutput(t *testing.T) {
	r := newRecorder(80, 24, "Test")
	r.output(bytes.Repeat([]byte("x"), recordingLimit-1024))
	for i := 0; i < 1024; i++ {
		r.resize(120, 40)
	}
	data, limitReached := r.close()
	if !limitReached {
		t.Fatal("resize events did not exhaust the recording budget")
	}
	stream, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	recording, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(recording) > recordingLimit || !bytes.Contains(recording, []byte(`"r","120x40"`)) {
		t.Fatalf("expected a bounded recording with resize events, got %d bytes", len(recording))
	}
}
