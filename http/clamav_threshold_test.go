package fbhttp

import "testing"

func TestShouldBypassAutomaticClamAV(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want bool
	}{
		{name: "below threshold", size: clamAVAutomaticScanMaxBytes - 1, want: false},
		{name: "at threshold", size: clamAVAutomaticScanMaxBytes, want: false},
		{name: "above threshold", size: clamAVAutomaticScanMaxBytes + 1, want: true},
		{name: "2GB file", size: 2_000_000_000, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldBypassAutomaticClamAV(tt.size); got != tt.want {
				t.Fatalf("shouldBypassAutomaticClamAV(%d) = %t, want %t", tt.size, got, tt.want)
			}
		})
	}
}
