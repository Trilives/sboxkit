package app

import "testing"

func TestClampOffsetKeepsSelectionVisible(t *testing.T) {
	tests := []struct {
		name     string
		selected int
		offset   int
		visible  int
		total    int
		want     int
	}{
		{name: "above window", selected: 1, offset: 3, visible: 5, total: 20, want: 1},
		{name: "below window", selected: 9, offset: 3, visible: 5, total: 20, want: 5},
		{name: "clamps to max", selected: 19, offset: 30, visible: 5, total: 20, want: 15},
		{name: "short list", selected: 2, offset: 2, visible: 10, total: 3, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampOffset(tt.selected, tt.offset, tt.visible, tt.total)
			if got != tt.want {
				t.Fatalf("clampOffset() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTruncateKeepsStableWidth(t *testing.T) {
	got := truncate("abcdefghijklmnopqrstuvwxyz", 10)
	if got != "abcdefg..." {
		t.Fatalf("truncate() = %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("short truncate() = %q", got)
	}
}
