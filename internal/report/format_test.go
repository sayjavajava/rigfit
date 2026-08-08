// SPDX-License-Identifier: GPL-3.0-only

package report

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1_000, "1.0 KB"},
		{1_500, "1.5 KB"},
		{1_000_000, "1.0 MB"},
		{4_294_967_296, "4.3 GB"},
		{1_000_000_000_000, "1.0 TB"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.bytes); got != c.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}
