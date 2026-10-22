package ubus

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The bytes libubox writes for blobmsg_add_string(b, "a", "b"), blobmsg_add_u32(b, "n", 7)
// and blobmsg_add_u8(b, "t", 1), worked out from blob.h and blobmsg.h: the header (extended
// flag, type, length without the padding), the name's length, the name and its zero padded
// to 4, the value, the padding.
func TestBlobmsgBytes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		bytes []byte
	}{
		{"a", "b", []byte{0x83, 0, 0, 0x0a, 0, 1, 'a', 0, 'b', 0, 0, 0}},
		{"n", json.Number("7"), []byte{0x85, 0, 0, 0x0c, 0, 1, 'n', 0, 0, 0, 0, 7}},
		{"t", true, []byte{0x87, 0, 0, 0x09, 0, 1, 't', 0, 1, 0, 0, 0}},
		{"big", json.Number("5000000000"), []byte{0x84, 0, 0, 0x14, 0, 3, 'b', 'i', 'g', 0, 0, 0, 0, 0, 0, 0x01, 0x2a, 0x05, 0xf2, 0}},
		{"", []any{}, []byte{0x81, 0, 0, 0x08, 0, 0, 0, 0}},
	}
	for _, c := range cases {
		w := &writer{}
		require.NoError(t, w.putValue(c.name, c.value))
		require.Equal(t, c.bytes, w.buffer, c.name)
	}
}

func TestBlobmsgRoundTrip(t *testing.T) {
	source := `{"s":"text","n":-5,"big":5000000000,"f":1.5,"yes":true,"no":false,
		"t":{"k":[1,"two",false,{"deep":"x"}],"empty":{}},"list":[],"name with spaces":"ok"}`
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	var value map[string]any
	require.NoError(t, decoder.Decode(&value))

	w := &writer{}
	for key, item := range value {
		require.NoError(t, w.putValue(key, item))
	}
	table, err := readTable(w.buffer)
	require.NoError(t, err)

	require.Equal(t, map[string]any{
		"s": "text", "n": int64(-5), "big": int64(5000000000), "f": 1.5, "yes": true, "no": false,
		"t":    map[string]any{"k": []any{int64(1), "two", false, map[string]any{"deep": "x"}}, "empty": map[string]any{}},
		"list": []any{}, "name with spaces": "ok",
	}, table)
}

func TestBlobBroken(t *testing.T) {
	w := &writer{}
	require.NoError(t, w.putValue("a", "text"))
	whole := w.buffer
	// cut anywhere before the end of the attribute (its padding may be missing)
	for cut := 1; cut < int(binary.BigEndian.Uint32(whole)&attrLenMask); cut++ {
		_, err := readTable(whole[:cut])
		require.Error(t, err, "cut at %d", cut)
	}
	// a length beyond the data
	broken := append([]byte(nil), whole...)
	broken[3] = 0xff
	_, err := readTable(broken)
	require.Error(t, err)
	// a value of no form in blobmsg
	require.Error(t, (&writer{}).putValue("x", struct{}{}))
}
