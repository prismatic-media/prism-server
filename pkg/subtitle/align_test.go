package subtitle

import (
	"math"
	"testing"
)

func TestParseSubtitleBlocks(t *testing.T) {
	vttContent := `WEBVTT

1
00:00:01.500 --> 00:00:04.200 align:start
Hello, <i>world</i>!

NOTE some comment

2
00:00:05.100 --> 00:00:08.000
This is a test.
`
	blocks, err := ParseSubtitleBlocks(vttContent)
	if err != nil {
		t.Fatalf("ParseSubtitleBlocks failed: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	if blocks[0].Text != "Hello, <i>world</i>!" {
		t.Errorf("expected text 'Hello, <i>world</i>!', got %q", blocks[0].Text)
	}
	if math.Abs(blocks[0].Start-1.5) > 0.001 {
		t.Errorf("expected start 1.5, got %v", blocks[0].Start)
	}
	if math.Abs(blocks[0].End-4.2) > 0.001 {
		t.Errorf("expected end 4.2, got %v", blocks[0].End)
	}
}

func TestAlignSubtitles(t *testing.T) {
	refBlocks := []SubtitleBlock{
		{Start: 10.0, End: 12.0, Text: "Welcome to the show today"},
		{Start: 15.0, End: 18.0, Text: "We will demonstrate automated synchronization"},
	}

	// Subtitle blocks offset by +5.0 seconds (i.e. start is early/late)
	// If ref has 10.0 and sub has 5.0, offset is ref - sub = 5.0 seconds.
	subBlocks := []SubtitleBlock{
		{Start: 5.0, End: 7.0, Text: "Welcome to the show today"},
		{Start: 10.0, End: 13.0, Text: "We will demonstrate automated synchronization"},
	}

	offset, score := AlignSubtitles(refBlocks, subBlocks)

	// Since ref start = 10.0 and sub start = 5.0, the offset diff should be +5.0 seconds.
	if math.Abs(offset-5.0) > 0.1 {
		t.Errorf("expected offset around 5.0, got %v", offset)
	}

	if score < 95.0 {
		t.Errorf("expected high similarity score, got %v", score)
	}
}

func TestShiftVTT(t *testing.T) {
	vttContent := `WEBVTT

00:00:01.500 --> 00:00:04.200 align:start
Hello, world!
`
	shifted, err := ShiftVTT(vttContent, 2.5)
	if err != nil {
		t.Fatalf("ShiftVTT failed: %v", err)
	}

	if !tContains(shifted, "00:00:04.000 --> 00:00:06.700 align:start") {
		t.Errorf("expected shifted timestamps, got:\n%s", shifted)
	}
}

func tContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringsContains(s, sub))
}

func stringsContains(s, sub string) bool {
	// A simple search
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
