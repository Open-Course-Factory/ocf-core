package dto

// The lexicon travels as one document rather than a set of entries.
//
// It is a table with a shape: entries point at parents, and names have to line
// up across languages. Saving it piecemeal means every intermediate state has a
// dangling parent or a half-renamed room, and the checks would have to be
// suspended for exactly as long as an editor takes to finish — which is the
// window in which a broken one reaches a learner.

// LexiconEntryInput is one object and what each language calls it.
type LexiconEntryInput struct {
	Key       string            `json:"key" binding:"required"`
	ParentKey string            `json:"parent_key,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Names     map[string]string `json:"names"`
}

// LexiconEntryOutput mirrors the input, so an editor round-trips what it reads.
type LexiconEntryOutput struct {
	Key       string            `json:"key"`
	ParentKey string            `json:"parent_key,omitempty"`
	Kind      string            `json:"kind"`
	Names     map[string]string `json:"names"`
}

// LexiconDocumentOutput is the whole vocabulary plus what is wrong with it.
//
// Problems come back with the document rather than from a second endpoint: they
// are a property of what was just read, and fetching them separately invites an
// editor showing one state while judging another.
type LexiconDocumentOutput struct {
	Entries  []LexiconEntryOutput `json:"entries"`
	Problems []string             `json:"problems"`
}

// ReplaceLexiconInput is the whole document, as sent.
type ReplaceLexiconInput struct {
	Entries []LexiconEntryInput `json:"entries"`
}
