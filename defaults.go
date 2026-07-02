package llmextractor

// Tuning defaults for the registered extractor and New's zero-option form.
const (
	// defaultThreshold drops low-confidence candidates.
	defaultThreshold = 0.6
	// defaultMax bounds candidates recorded per Consolidate (cost guard).
	defaultMax = 8
)
