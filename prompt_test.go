package llmextractor

import "testing"

func TestExtractionPrompt_CarriesJournalAndTranscript(t *testing.T) {
	p := extractionPrompt("JOURNAL-MARKER", "TRANSCRIPT-MARKER")
	if p.Content == "" {
		t.Fatal("empty prompt content")
	}
	for _, want := range []string{"JOURNAL-MARKER", "TRANSCRIPT-MARKER", "JSON array", "private", "confidence"} {
		if !contains(p.Content, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
