package document

// LinkDocument is a derived metadata/target snapshot, not document source.
// It is internal to graph and backlink queries and is never a public DTO.
type LinkDocument struct {
	Document Document `json:"document"`
	Targets  []string `json:"targets"`
}
