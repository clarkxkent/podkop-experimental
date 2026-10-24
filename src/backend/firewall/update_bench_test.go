package firewall

import (
	"net/netip"
	"testing"
)

// What a change of one address costs besides the kernel when a list of 100000 subnets is
// in the set: the two lists merged and their difference worked out (the update merges only
// the next one: the previous plan keeps its ranges). Run with -cpuprofile
// and -memprofile to see where it goes.
func BenchmarkOneAddressMore(b *testing.B) {
	var previous []Range
	for index := 0; index < 100000; index++ {
		item, _ := RangeOf(netip.PrefixFrom(netip.AddrFrom4([4]byte{11 + byte(index/32768), byte(index / 128), byte(index%128) * 2, 0}), 24))
		previous = append(previous, item)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		one, _ := RangeOf(netip.MustParsePrefix("100.71.99.8/32"))
		next := append(append([]Range{}, previous...), one)
		gone, came := rangesDiffer(merge(previous), merge(next))
		if len(gone) != 0 || len(came) != 1 {
			b.Fatal(len(gone), len(came))
		}
		_ = intervals(came)
	}
}
