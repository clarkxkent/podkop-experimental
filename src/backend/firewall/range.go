package firewall

import (
	"encoding/binary"
	"net/netip"
)

// Range is a range of IPv4 destinations, both ends included: the first address in the high
// 32 bits, the last in the low ones, as the address sets of sing-box keep them
// (cidrset.Set.Ranges4), so that the ranges of a list come here without a copy. Eight bytes
// and no pointer: a plan keeps those of every list for the life of a run, where a
// netip.Prefix took 32 bytes with a pointer the collector scans.
type Range = uint64

// RangeOf is the range of an IPv4 prefix; there is none of an IPv6 one.
func RangeOf(prefix netip.Prefix) (Range, bool) {
	if !prefix.Addr().Is4() {
		return 0, false
	}
	prefix = prefix.Masked()
	address := prefix.Addr().As4()
	from := binary.BigEndian.Uint32(address[:])
	return Range(from)<<32 | Range(from|uint32(uint64(1)<<(32-prefix.Bits())-1)), true
}

// RangesOf are the ranges of the IPv4 prefixes.
func RangesOf(prefixes []netip.Prefix) []Range {
	ranges := make([]Range, 0, len(prefixes))
	for _, prefix := range prefixes {
		if item, ok := RangeOf(prefix); ok {
			ranges = append(ranges, item)
		}
	}
	return ranges
}

// RangeString writes a range as a prefix when it is one, as "first-last" otherwise.
func RangeString(item Range) string {
	from, to := uint32(item>>32), uint32(item)
	size := uint64(to) - uint64(from) + 1
	if size&(size-1) == 0 && from&uint32(size-1) == 0 {
		bits := 32
		for value := size; value > 1; value >>= 1 {
			bits--
		}
		return netip.PrefixFrom(address4(from), bits).String()
	}
	return address4(from).String() + "-" + address4(to).String()
}

// RangeStrings writes ranges as RangeString does.
func RangeStrings(ranges []Range) []string {
	texts := make([]string, len(ranges))
	for index, item := range ranges {
		texts[index] = RangeString(item)
	}
	return texts
}

func address4(value uint32) netip.Addr {
	var address [4]byte
	binary.BigEndian.PutUint32(address[:], value)
	return netip.AddrFrom4(address)
}
