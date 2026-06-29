package files

import (
	"testing"

	"github.com/gabriel-vasile/mimetype"
)

func TestAllowedFor(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantOK  bool
		wantExt string
	}{
		{"png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, true, ".png"},
		{"plain text (e.g. EICAR)", []byte("X5O!P%@AP[4 plain ascii text"), true, ".txt"},
		{"windows exe", []byte{0x4D, 0x5A, 0x90, 0x00, 0x03}, false, ""},
	}
	for _, c := range cases {
		m := mimetype.Detect(c.data)
		_, ext, ok := allowedFor(m)
		if ok != c.wantOK {
			t.Errorf("%s: allowed=%v want %v (sniffed %s)", c.name, ok, c.wantOK, m.String())
		}
		if ok && ext != c.wantExt {
			t.Errorf("%s: ext=%q want %q", c.name, ext, c.wantExt)
		}
	}
}
