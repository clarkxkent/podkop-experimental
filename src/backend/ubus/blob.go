package ubus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	E "github.com/sagernet/sing/common/exceptions"
)

// A blob attribute of libubox (blob.h): a header of 4 bytes, big-endian, with the extended
// flag (bit 31), the id (bits 24–30) and the length of the attribute with its header (bits
// 0–23); then the payload, padded to 4 bytes. An extended attribute is one of blobmsg
// (blobmsg.h): its payload starts with the length of a name (2 bytes), the name and a zero,
// padded to 4 bytes, and the id is the type of the value.
const (
	attrExtended = 0x80000000
	attrIDShift  = 24
	attrIDMask   = 0x7f
	attrLenMask  = 0x00ffffff
	attrHeader   = 4
)

// The types of a blobmsg value: what Method.Arguments names, and what is sent.
const (
	TypeUnspec = 0
	TypeArray  = 1
	TypeTable  = 2
	TypeString = 3
	TypeInt64  = 4
	TypeInt32  = 5
	TypeInt16  = 6
	// TypeBool is TypeInt8 of libubox: a byte
	TypeBool   = 7
	TypeDouble = 8
)

func pad(length int) int {
	return (length + 3) &^ 3
}

// writer builds blobs: an attribute is opened, filled and closed, and may hold others.
type writer struct {
	buffer []byte
}

// open starts an attribute and returns where it starts, for close.
func (w *writer) open(id int, extended bool) int {
	at := len(w.buffer)
	head := uint32(id&attrIDMask) << attrIDShift
	if extended {
		head |= attrExtended
	}
	w.buffer = binary.BigEndian.AppendUint32(w.buffer, head)
	return at
}

// close writes the length of the attribute started at at into its header, then the
// padding: the length is that of the attribute itself, the padding is not in it.
func (w *writer) close(at int) {
	length := len(w.buffer) - at
	head := binary.BigEndian.Uint32(w.buffer[at:])&^attrLenMask | uint32(length)
	binary.BigEndian.PutUint32(w.buffer[at:], head)
	for len(w.buffer)%4 != 0 {
		w.buffer = append(w.buffer, 0)
	}
}

// openNamed starts a blobmsg attribute of the type with its name.
func (w *writer) openNamed(kind int, name string) int {
	at := w.open(kind, true)
	start := len(w.buffer)
	w.buffer = binary.BigEndian.AppendUint16(w.buffer, uint16(len(name)))
	w.buffer = append(w.buffer, name...)
	w.buffer = append(w.buffer, 0)
	for (len(w.buffer)-start)%4 != 0 {
		w.buffer = append(w.buffer, 0)
	}
	return at
}

func (w *writer) putInt32(id int, value uint32) {
	at := w.open(id, false)
	w.buffer = binary.BigEndian.AppendUint32(w.buffer, value)
	w.close(at)
}

func (w *writer) putString(id int, value string) {
	at := w.open(id, false)
	w.buffer = append(w.buffer, value...)
	w.buffer = append(w.buffer, 0)
	w.close(at)
}

// putValue writes a value of encoding/json's generic form (what json.Decoder with UseNumber
// gives) as a blobmsg attribute. A null is left out: older libubox writes no JSON for it.
func (w *writer) putValue(name string, value any) error {
	switch value := value.(type) {
	case nil:
		return nil
	case map[string]any:
		at := w.openNamed(TypeTable, name)
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := w.putValue(key, value[key]); err != nil {
				return err
			}
		}
		w.close(at)
	case []any:
		at := w.openNamed(TypeArray, name)
		for _, item := range value {
			if err := w.putValue("", item); err != nil {
				return err
			}
		}
		w.close(at)
	case string:
		at := w.openNamed(TypeString, name)
		w.buffer = append(w.buffer, value...)
		w.buffer = append(w.buffer, 0)
		w.close(at)
	case bool:
		at := w.openNamed(TypeBool, name)
		if value {
			w.buffer = append(w.buffer, 1)
		} else {
			w.buffer = append(w.buffer, 0)
		}
		w.close(at)
	case json.Number:
		if number, err := value.Int64(); err == nil {
			if number >= math.MinInt32 && number <= math.MaxInt32 {
				at := w.openNamed(TypeInt32, name)
				w.buffer = binary.BigEndian.AppendUint32(w.buffer, uint32(int32(number)))
				w.close(at)
			} else {
				at := w.openNamed(TypeInt64, name)
				w.buffer = binary.BigEndian.AppendUint64(w.buffer, uint64(number))
				w.close(at)
			}
			return nil
		}
		number, err := value.Float64()
		if err != nil {
			return E.Cause(err, "number ", value.String())
		}
		at := w.openNamed(TypeDouble, name)
		w.buffer = binary.BigEndian.AppendUint64(w.buffer, math.Float64bits(number))
		w.close(at)
	default:
		return E.New("a value of type ", fmt.Sprintf("%T", value), " has no blobmsg form")
	}
	return nil
}

// attr is one attribute read from a blob.
type attr struct {
	id       int
	extended bool
	// payload is what follows the header, without the padding
	payload []byte
}

// readAttrs reads the attributes that follow one another in data, the payload of the
// attribute that holds them.
func readAttrs(data []byte) ([]attr, error) {
	var attrs []attr
	for len(data) > 0 {
		if len(data) < attrHeader {
			return nil, E.New("a blob attribute cut short")
		}
		head := binary.BigEndian.Uint32(data)
		length := int(head & attrLenMask)
		if length < attrHeader || length > len(data) {
			return nil, E.New("a blob attribute of length ", length, " in ", len(data), " bytes")
		}
		attrs = append(attrs, attr{
			id:       int(head>>attrIDShift) & attrIDMask,
			extended: head&attrExtended != 0,
			payload:  data[attrHeader:length],
		})
		next := pad(length)
		if next > len(data) {
			next = len(data)
		}
		data = data[next:]
	}
	return attrs, nil
}

// named splits the payload of a blobmsg attribute into its name and its value.
func (a attr) named() (string, []byte, error) {
	if len(a.payload) < 2 {
		return "", nil, E.New("a blobmsg attribute without its name")
	}
	length := int(binary.BigEndian.Uint16(a.payload))
	header := pad(2 + length + 1)
	if header > len(a.payload) {
		return "", nil, E.New("a blobmsg name of length ", length, " in ", len(a.payload), " bytes")
	}
	return string(a.payload[2 : 2+length]), a.payload[header:], nil
}

// readValue reads the value of a blobmsg attribute: a table into a map, an array into a
// slice, a string, an integer into int64, a byte into bool, a double into float64.
func readValue(kind int, data []byte) (any, error) {
	switch kind {
	case TypeTable:
		return readTable(data)
	case TypeArray:
		attrs, err := readAttrs(data)
		if err != nil {
			return nil, err
		}
		list := make([]any, 0, len(attrs))
		for _, item := range attrs {
			_, value, err := item.named()
			if err != nil {
				return nil, err
			}
			decoded, err := readValue(item.id, value)
			if err != nil {
				return nil, err
			}
			list = append(list, decoded)
		}
		return list, nil
	case TypeString:
		if len(data) == 0 || data[len(data)-1] != 0 {
			return nil, E.New("a blobmsg string without its zero")
		}
		return string(data[:len(data)-1]), nil
	case TypeInt64, TypeDouble:
		if len(data) < 8 {
			return nil, E.New("a blobmsg number of ", len(data), " bytes")
		}
		if kind == TypeDouble {
			return math.Float64frombits(binary.BigEndian.Uint64(data)), nil
		}
		return int64(binary.BigEndian.Uint64(data)), nil
	case TypeInt32:
		if len(data) < 4 {
			return nil, E.New("a blobmsg number of ", len(data), " bytes")
		}
		return int64(int32(binary.BigEndian.Uint32(data))), nil
	case TypeInt16:
		if len(data) < 2 {
			return nil, E.New("a blobmsg number of ", len(data), " bytes")
		}
		return int64(int16(binary.BigEndian.Uint16(data))), nil
	case TypeBool:
		if len(data) < 1 {
			return nil, E.New("a blobmsg byte of no bytes")
		}
		return data[0] != 0, nil
	case TypeUnspec:
		return nil, nil
	}
	return nil, E.New("a blobmsg value of type ", kind)
}

// readTable reads the blobmsg attributes of data as the fields of a table.
func readTable(data []byte) (map[string]any, error) {
	attrs, err := readAttrs(data)
	if err != nil {
		return nil, err
	}
	table := make(map[string]any, len(attrs))
	for _, field := range attrs {
		if !field.extended {
			return nil, E.New("a field of a table that is not blobmsg")
		}
		name, value, err := field.named()
		if err != nil {
			return nil, err
		}
		table[name], err = readValue(field.id, value)
		if err != nil {
			return nil, E.Cause(err, "field ", name)
		}
	}
	return table, nil
}
