package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/repository"
)

// ── Multi-episode NFO fixture ─────────────────────────────────────────

const multiEpisodeNFOFixture = `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>First Episode</title>
    <season>1</season>
    <episode>1</episode>
    <plot>The first episode of the season.</plot>
    <aired>2022-01-01</aired>
</episodedetails>
<episodedetails>
    <title>Second Episode</title>
    <season>1</season>
    <episode>2</episode>
    <plot>The second episode of the season.</plot>
    <aired>2022-01-08</aired>
</episodedetails>`

// ── Episode NFO with no season/episode numbers ────────────────────────

const episodeNFOFixtureNoSeasonEpisode = `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>Special Episode</title>
    <plot>A special episode without season/episode numbers.</plot>
    <aired>2022-06-15</aired>
</episodedetails>`

// ── Season 00 episode NFO ────────────────────────────────────────────

const episodeNFOFixtureSeason00 = `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>Season Zero Special</title>
    <season>0</season>
    <episode>1</episode>
    <plot>A season 0 special episode.</plot>
    <aired>2022-06-15</aired>
</episodedetails>`

// ── Test: multi-episode NFO splitting ─────────────────────────────────

func TestParseEpisodeNFOs_MultiEpisodeFile(t *testing.T) {
	// Write the multi-episode NFO to a temp file and parse via ParseEpisodeNFOs
	dir := t.TempDir()
	nfoPath := filepath.Join(dir, "S1E1.nfo")
	writeFileHelper(t, nfoPath, multiEpisodeNFOFixture)

	f, err := os.Open(nfoPath)
	if err != nil {
		t.Fatalf("failed to open NFO: %v", err)
	}
	defer f.Close()

	episodes, err := ParseEpisodeNFOs(f)
	if err != nil {
		t.Fatalf("ParseEpisodeNFOs failed: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}
	if episodes[0].Episode != 1 || episodes[1].Episode != 2 {
		t.Errorf("episode numbers wrong: %d, %d", episodes[0].Episode, episodes[1].Episode)
	}
	if episodes[0].Title == episodes[1].Title {
		t.Errorf("both episodes have the same title %q — parser likely returned duplicates of the first block", episodes[0].Title)
	}
	if episodes[0].Title != "First Episode" {
		t.Errorf("first episode title = %q, want %q", episodes[0].Title, "First Episode")
	}
	if episodes[1].Title != "Second Episode" {
		t.Errorf("second episode title = %q, want %q", episodes[1].Title, "Second Episode")
	}
}

// ── Test: missing season/episode numbers ──────────────────────────────

func TestImport_MissingSeasonEpisodeNumbers(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "ShowNoNum (2022)")
	writeFileHelper(t, filepath.Join(showDir, "tvshow.nfo"), showNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "special.nfo"), episodeNFOFixtureNoSeasonEpisode)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "special.mkv"), "")

	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, dir)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	importer.SetLibraryID(lib.ID)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatalf("ImportRequest failed: %v", err)
	}
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	// The episode should be imported with season=0 and episode=0 (defaults)
	var season, episode int
	err = db.QueryRow(`SELECT season, episode FROM media_items WHERE library_id = ? AND type = 'episode'`, lib.ID).Scan(&season, &episode)
	if err != nil {
		t.Fatalf("failed to query episode: %v", err)
	}
	// Default values when NFO has no <season>/<episode> tags
	if season != 0 {
		t.Errorf("expected season=0 for missing season, got %d", season)
	}
	if episode != 0 {
		t.Errorf("expected episode=0 for missing episode, got %d", episode)
	}
}

// ── Test: Season 00 specials import correctly ──────────────────────────

func TestImport_SpecialEpisode_Season00(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "ShowSpecial (2022)")
	writeFileHelper(t, filepath.Join(showDir, "tvshow.nfo"), showNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 00", "S00E01.nfo"), episodeNFOFixtureSeason00)
	writeFileHelper(t, filepath.Join(showDir, "Season 00", "S00E01.mkv"), "")

	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, dir)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	importer.SetLibraryID(lib.ID)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatalf("ImportRequest failed: %v", err)
	}
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	// Season 0 is valid — should be imported as season=0, NOT treated as "missing"
	var season, episode int
	err = db.QueryRow(`SELECT season, episode FROM media_items WHERE library_id = ? AND type = 'episode'`, lib.ID).Scan(&season, &episode)
	if err != nil {
		t.Fatalf("failed to query episode: %v", err)
	}
	if season != 0 {
		t.Errorf("expected season=0 for Season 00 special, got %d", season)
	}
	if episode != 1 {
		t.Errorf("expected episode=1 for S00E01, got %d", episode)
	}
}

// ── Test: episode backdrop falls back to thumbnail ────────────────────

func TestImport_EpisodeBackdropFallback(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "ShowBackdrop (2022)")
	writeFileHelper(t, filepath.Join(showDir, "tvshow.nfo"), showNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "S1E1.nfo"), episodeNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "S1E1.mkv"), "")
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "S1E1-thumb.jpg"), "fake-jpg-bytes")

	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, dir)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	importer.SetLibraryID(lib.ID)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatalf("ImportRequest failed: %v", err)
	}
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	// The episode's BackdropPath should fall back to the thumbnail (S1E1-thumb.jpg)
	var backdropPath string
	err = db.QueryRow(`SELECT backdrop_path FROM media_items WHERE library_id = ? AND type = 'episode'`, lib.ID).Scan(&backdropPath)
	if err != nil {
		t.Fatalf("failed to query backdrop_path: %v", err)
	}
	if backdropPath == "" {
		t.Error("expected backdrop_path to be set (should fall back to thumbnail)")
	} else if filepath.Base(backdropPath) != "S1E1-thumb.jpg" {
		t.Errorf("expected backdrop_path to be S1E1-thumb.jpg, got %s", filepath.Base(backdropPath))
	}
}
