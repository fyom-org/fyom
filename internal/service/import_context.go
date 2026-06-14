package service

// ImportEntityKind represents the semantic type of a media entity during classification.
type ImportEntityKind string

const (
	ImportEntityUnknown ImportEntityKind = "unknown"
	ImportEntityMovie   ImportEntityKind = "movie"
	ImportEntityShow    ImportEntityKind = "show"
	ImportEntitySeason  ImportEntityKind = "season"
	ImportEntityEpisode ImportEntityKind = "episode"
	ImportEntityExtra   ImportEntityKind = "extra"
)

// ImportNodeKind represents the type of a filesystem node.
type ImportNodeKind string

const (
	ImportNodeDir      ImportNodeKind = "dir"
	ImportNodeVideo    ImportNodeKind = "video"
	ImportNodeNFO      ImportNodeKind = "nfo"
	ImportNodeImage    ImportNodeKind = "image"
	ImportNodeSubtitle ImportNodeKind = "subtitle"
	ImportNodeOther    ImportNodeKind = "other"
)

// ImportEvidence records why a classification decision was made.
type ImportEvidence struct {
	Rule    string
	Weight  int
	Message string
}

// PathClaim records which candidate owns a given filesystem path.
type PathClaim struct {
	CandidateID string
	Kind        ImportEntityKind
	Path        string
}

// WalkContext carries semantic context during recursive directory traversal.
// It ensures that child nodes inherit parent semantics (e.g., a season directory
// inside a show root knows its show ID and season number).
type WalkContext struct {
	ScanID        string
	LibraryID     string
	SourceRoot    string
	CurrentPath   string
	Depth         int

	ParentKind    ImportEntityKind
	ShowRootPath  string
	ShowID        string

	SeasonRootPath string
	SeasonNumber   *int

	// ClaimedPaths tracks which paths have already been consumed by a candidate,
	// preventing the same file from being claimed by multiple candidates.
	ClaimedPaths map[string]PathClaim
}

// ImportCandidate represents a classified media entity before it is persisted.
type ImportCandidate struct {
	ID            string
	LibraryID     string
	Kind          ImportEntityKind

	RootPath      string
	PrimaryPath   string
	NFOPath       string

	ParentID      string
	ShowID        string
	SeasonNumber  *int
	EpisodeNumber *int

	Confidence    int
	Evidence      []ImportEvidence
}

// FSNode represents a node in the filesystem snapshot tree.
type FSNode struct {
	Path     string
	Name     string
	Kind     ImportNodeKind
	IsDir    bool
	Size     int64
	Children []*FSNode
}

// ClassificationResult holds the output of the classification phase.
type ClassificationResult struct {
	Candidates   []ImportCandidate
	Rejected     []RejectedItem
	Unknown      []ImportCandidate
	ScannedDirs  int
}

// RejectedItem records a path that was explicitly rejected during classification.
type RejectedItem struct {
	Path   string
	Type   ImportEntityKind
	Reason string
}

// ReconcileAction represents the action taken for a candidate during reconciliation.
type ReconcileAction string

const (
	ReconcileCreate ReconcileAction = "create"
	ReconcileUpdate ReconcileAction = "update"
	ReconcileSkip   ReconcileAction = "skip"
	ReconcileReject ReconcileAction = "reject"
)

// ResolvedItem records the outcome of reconciling a single candidate.
type ResolvedItem struct {
	CandidateID string
	MediaID     string
	Kind        ImportEntityKind
	Action      ReconcileAction
	RootPath    string
	PrimaryPath string
}

// ReconcileResult holds the output of the reconciliation phase.
type ReconcileResult struct {
	Accepted        []ResolvedItem
	Rejected        []RejectedItem
	Unknown         []ImportCandidate
	TotalCandidates int
	DoneCandidates  int
}
