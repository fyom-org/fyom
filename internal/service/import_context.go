package service

// ImportEntityKind represents the semantic type of a media entity during classification.
type ImportEntityKind string

// ImportEntityUnknown represents an unclassified media entity.
const ImportEntityUnknown ImportEntityKind = "unknown"

// ImportEntityContainer represents a grouping/container directory that is not itself a media root.
const ImportEntityContainer ImportEntityKind = "container"

// ImportEntityMovie represents a movie media entity.
const ImportEntityMovie ImportEntityKind = "movie"

const (
	// ImportEntityShow represents a TV show media entity.
	ImportEntityShow    ImportEntityKind = "show"
	// ImportEntitySeason represents a season within a TV show.
	ImportEntitySeason  ImportEntityKind = "season"
	// ImportEntityEpisode represents an episode within a season.
	ImportEntityEpisode ImportEntityKind = "episode"
	// ImportEntityExtra represents extra/supplementary media content.
	ImportEntityExtra   ImportEntityKind = "extra"
)

// ImportNodeKind represents the type of a filesystem node.
type ImportNodeKind string

const (
	// ImportNodeDir represents a directory node in the filesystem tree.
	ImportNodeDir      ImportNodeKind = "dir"
	// ImportNodeVideo represents a video file node.
	ImportNodeVideo    ImportNodeKind = "video"
	// ImportNodeNFO represents an NFO metadata file node.
	ImportNodeNFO      ImportNodeKind = "nfo"
	// ImportNodeImage represents an image file node (poster, thumbnail, etc.).
	ImportNodeImage    ImportNodeKind = "image"
	// ImportNodeSubtitle represents a subtitle file node.
	ImportNodeSubtitle ImportNodeKind = "subtitle"
	// ImportNodeOther represents any other file type not categorized above.
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
	// ReconcileCreate indicates a new media item was created during reconciliation.
	ReconcileCreate ReconcileAction = "create"
	// ReconcileUpdate indicates an existing media item was updated during reconciliation.
	ReconcileUpdate ReconcileAction = "update"
	// ReconcileSkip indicates the candidate was skipped (already up-to-date).
	ReconcileSkip   ReconcileAction = "skip"
	// ReconcileReject indicates the candidate was rejected during reconciliation.
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
