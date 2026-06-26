package subtitle

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// SubtitleBlock represents a single subtitle entry with timestamps and cleaned text.
type SubtitleBlock struct {
	Start float64 // in seconds
	End   float64 // in seconds
	Text  string
}

var reFormatting = regexp.MustCompile(`<[^>]*>`)
var rePunct = regexp.MustCompile(`[^\p{L}\p{N}\s]`)

func cleanText(text string) string {
	// Strip HTML/WebVTT formatting tags
	text = reFormatting.ReplaceAllString(text, "")
	text = strings.ToLower(text)
	// Strip punctuation and special characters
	text = rePunct.ReplaceAllString(text, "")
	// Normalize spaces
	words := strings.Fields(text)
	return strings.Join(words, " ")
}

func parseTimestamp(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".") // convert SRT comma to decimal
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid time format: %q", s)
	}
	var hrs, mins, secs float64
	var err error
	if len(parts) == 3 {
		hrs, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		mins, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		secs, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
	} else if len(parts) == 2 {
		mins, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		secs, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
	}
	return hrs*3600 + mins*60 + secs, nil
}

func formatTimestamp(t float64) string {
	if t < 0 {
		t = 0
	}
	hrs := int(t) / 3600
	mins := (int(t) % 3600) / 60
	secs := int(t) % 60
	ms := int(math.Round((t - math.Floor(t)) * 1000))
	if ms >= 1000 {
		ms = 999
	}
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hrs, mins, secs, ms)
}

// ParseSubtitleBlocks parses raw WebVTT or SRT content into a slice of SubtitleBlocks.
func ParseSubtitleBlocks(content string) ([]SubtitleBlock, error) {
	var blocks []SubtitleBlock
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	var currentBlock *SubtitleBlock

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "-->") {
			times := strings.Split(line, "-->")
			if len(times) != 2 {
				continue
			}

			endPart := strings.TrimSpace(times[1])
			endFields := strings.Fields(endPart)
			var endTimeStr string
			if len(endFields) > 0 {
				endTimeStr = endFields[0]
			} else {
				endTimeStr = endPart
			}

			start, err := parseTimestamp(times[0])
			if err != nil {
				continue
			}
			end, err := parseTimestamp(endTimeStr)
			if err != nil {
				continue
			}

			if currentBlock != nil {
				blocks = append(blocks, *currentBlock)
			}
			currentBlock = &SubtitleBlock{
				Start: start,
				End:   end,
			}
		} else if currentBlock != nil {
			if line == "" {
				blocks = append(blocks, *currentBlock)
				currentBlock = nil
			} else {
				// Avoid adding sequence numbers or styling/NOTE tags as text
				if strings.HasPrefix(line, "NOTE") || strings.HasPrefix(line, "STYLE") {
					continue
				}
				if currentBlock.Text == "" {
					currentBlock.Text = line
				} else {
					currentBlock.Text += " " + line
				}
			}
		}
	}
	if currentBlock != nil {
		blocks = append(blocks, *currentBlock)
	}

	// Filter out blocks with empty text
	var result []SubtitleBlock
	for _, b := range blocks {
		b.Text = strings.TrimSpace(b.Text)
		if b.Text != "" {
			result = append(result, b)
		}
	}

	return result, nil
}

// AlignSubtitles aligns uploaded subtitles to reference subtitles.
// Returns the calculated sync offset (seconds) and the similarity score percentage (0-100).
func AlignSubtitles(refBlocks, subBlocks []SubtitleBlock) (offset float64, similarity float64) {
	if len(refBlocks) == 0 || len(subBlocks) == 0 {
		return 0.0, 0.0
	}

	type wordOccurrence struct {
		time float64
	}
	refWords := make(map[string][]wordOccurrence)
	subWords := make(map[string][]wordOccurrence)

	for _, block := range refBlocks {
		cleaned := cleanText(block.Text)
		words := strings.Fields(cleaned)
		midTime := (block.Start + block.End) / 2.0
		seen := make(map[string]struct{})
		for _, w := range words {
			if len(w) >= 3 && !isStopWord(w) {
				if _, ok := seen[w]; !ok {
					seen[w] = struct{}{}
					refWords[w] = append(refWords[w], wordOccurrence{time: midTime})
				}
			}
		}
	}

	for _, block := range subBlocks {
		cleaned := cleanText(block.Text)
		words := strings.Fields(cleaned)
		midTime := (block.Start + block.End) / 2.0
		seen := make(map[string]struct{})
		for _, w := range words {
			if len(w) >= 3 && !isStopWord(w) {
				if _, ok := seen[w]; !ok {
					seen[w] = struct{}{}
					subWords[w] = append(subWords[w], wordOccurrence{time: midTime})
				}
			}
		}
	}

	// Accumulate delta times in a histogram with a bin width of 1.0 second
	binWidth := 1.0
	histogram := make(map[int][]float64)

	for w, refOccs := range refWords {
		subOccs, ok := subWords[w]
		if !ok {
			continue
		}
		for _, ro := range refOccs {
			for _, so := range subOccs {
				diff := ro.time - so.time
				// Alignments are usually within +/- 300 seconds (5 minutes)
				if diff >= -300.0 && diff <= 300.0 {
					bin := int(math.Round(diff / binWidth))
					histogram[bin] = append(histogram[bin], diff)
				}
			}
		}
	}

	bestBin := 0
	maxCount := 0
	for bin, diffs := range histogram {
		if len(diffs) > maxCount {
			maxCount = len(diffs)
			bestBin = bin
		}
	}

	bestOffset := 0.0
	if maxCount > 0 {
		sum := 0.0
		for _, v := range histogram[bestBin] {
			sum += v
		}
		bestOffset = sum / float64(len(histogram[bestBin]))
	}

	// Calculate similarity score as a global content overlap metric (Cosine Similarity)
	// entirely independent of timestamps/timing.
	var refAllWords []string
	for _, b := range refBlocks {
		cleaned := cleanText(b.Text)
		for _, w := range strings.Fields(cleaned) {
			if len(w) >= 3 && !isStopWord(w) {
				refAllWords = append(refAllWords, w)
			}
		}
	}

	var subAllWords []string
	for _, b := range subBlocks {
		cleaned := cleanText(b.Text)
		for _, w := range strings.Fields(cleaned) {
			if len(w) >= 3 && !isStopWord(w) {
				subAllWords = append(subAllWords, w)
			}
		}
	}

	refFreq := make(map[string]float64)
	subFreq := make(map[string]float64)
	for _, w := range refAllWords {
		refFreq[w]++
	}
	for _, w := range subAllWords {
		subFreq[w]++
	}

	dotProduct := 0.0
	refMagSq := 0.0
	subMagSq := 0.0

	for w, fRef := range refFreq {
		refMagSq += fRef * fRef
		if fSub, ok := subFreq[w]; ok {
			dotProduct += fRef * fSub
		}
	}
	for _, fSub := range subFreq {
		subMagSq += fSub * fSub
	}

	simScore := 0.0
	if refMagSq > 0 && subMagSq > 0 {
		simScore = (dotProduct / (math.Sqrt(refMagSq) * math.Sqrt(subMagSq))) * 100.0
	}

	return bestOffset, simScore
}

// ShiftVTT shifts all timestamps in a WebVTT file by the specified offset (in seconds).
func ShiftVTT(content string, offset float64) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.Contains(line, "-->") {
			parts := strings.Split(line, "-->")
			if len(parts) != 2 {
				continue
			}
			startStr := strings.TrimSpace(parts[0])
			endPart := strings.TrimSpace(parts[1])

			endFields := strings.Fields(endPart)
			if len(endFields) == 0 {
				continue
			}
			endStr := endFields[0]
			settings := ""
			if len(endFields) > 1 {
				settings = " " + strings.Join(endFields[1:], " ")
			}

			start, err := parseTimestamp(startStr)
			if err != nil {
				return "", fmt.Errorf("parsing start time on line %d: %w", i+1, err)
			}
			end, err := parseTimestamp(endStr)
			if err != nil {
				return "", fmt.Errorf("parsing end time on line %d: %w", i+1, err)
			}

			newStart := start + offset
			newEnd := end + offset

			lines[i] = fmt.Sprintf("%s --> %s%s", formatTimestamp(newStart), formatTimestamp(newEnd), settings)
		}
	}
	return strings.Join(lines, "\n"), nil
}


var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "a": {}, "of": {}, "to": {}, "in": {}, "is": {}, "you": {}, "that": {}, "it": {},
	"he": {}, "was": {}, "for": {}, "on": {}, "are": {}, "as": {}, "with": {}, "his": {}, "they": {}, "i": {},
	"at": {}, "be": {}, "this": {}, "have": {}, "from": {}, "or": {}, "had": {}, "by": {}, "but": {}, "not": {},
	"what": {}, "all": {}, "were": {}, "we": {}, "when": {}, "your": {}, "can": {}, "said": {}, "there": {}, "use": {},
	"an": {}, "each": {}, "which": {}, "she": {}, "do": {}, "how": {}, "their": {}, "if": {}, "will": {}, "up": {},
	"other": {}, "about": {}, "out": {}, "many": {}, "then": {}, "them": {}, "these": {}, "so": {}, "some": {}, "her": {},
	"would": {}, "make": {}, "like": {}, "him": {}, "into": {}, "has": {}, "look": {}, "two": {}, "more": {}, "write": {},
	"go": {}, "see": {}, "number": {}, "no": {}, "way": {}, "could": {}, "people": {}, "my": {}, "than": {}, "first": {},
	"water": {}, "been": {}, "call": {}, "who": {}, "oil": {}, "its": {}, "now": {}, "find": {},
	// Spanish common words
	"que": {}, "el": {}, "la": {}, "en": {}, "y": {}, "un": {}, "una": {}, "es": {}, "por": {}, "para": {}, "con": {},
	"los": {}, "las": {}, "del": {}, "al": {}, "lo": {}, "como": {}, "mas": {}, "o": {}, "pero": {}, "sus": {},
}

func isStopWord(w string) bool {
	_, ok := stopWords[w]
	return ok
}
