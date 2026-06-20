package service

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Test 6: parse episode NFO, title comes from <title> ──

func TestNFOParser_ParseEpisodeNFO_UsesEpisodeTitleAndMetadata(t *testing.T) {
	dir := t.TempDir()
	nfoPath := filepath.Join(dir, "S01E01.nfo")
	nfoContent := `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>孤独的转机</title>
    <season>1</season>
    <episode>1</episode>
    <plot>后藤独虽然性格内向，却靠自学练出一手好吉他。</plot>
    <rating>7.4</rating>
    <aired>2022-10-09</aired>
    <year>2022</year>
    <runtime>24</runtime>
    <director>斋藤圭一郎</director>
    <writer>吉田惠里香</writer>
    <credits>吉田惠里香</credits>
    <genre>Drama</genre>
    <studio>CloverWorks</studio>
    <actor>
        <name>青山吉能</name>
        <role>Hitori Gotoh</role>
        <type>Actor</type>
        <sortorder>0</sortorder>
    </actor>
    <uniqueid type="imdb">tt21253480</uniqueid>
    <uniqueid type="tvdb">8900312</uniqueid>
    <uniqueid type="tmdb">12345</uniqueid>
</episodedetails>`

	if err := os.WriteFile(nfoPath, []byte(nfoContent), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(nfoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	episodes, err := ParseEpisodeNFOs(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) == 0 {
		t.Fatal("no episodes parsed")
	}

	ep := episodes[0]

	// Title from NFO
	if ep.Title != "孤独的转机" {
		t.Errorf("title = %q, expected %q", ep.Title, "孤独的转机")
	}

	// Overview (plot)
	if ep.Plot == "" {
		t.Error("plot should not be empty")
	}

	// Aired
	if ep.Aired != "2022-10-09" {
		t.Errorf("aired = %q, expected %q", ep.Aired, "2022-10-09")
	}

	// Rating
	if ep.Rating != 7.4 {
		t.Errorf("rating = %f, expected %f", ep.Rating, 7.4)
	}

	// Runtime
	if ep.Runtime != 24 {
		t.Errorf("runtime = %d, expected %d", ep.Runtime, 24)
	}

	// Directors
	if len(ep.Directors) == 0 {
		t.Error("directors should not be empty")
	} else if ep.Directors[0] != "斋藤圭一郎" {
		t.Errorf("director = %q, expected %q", ep.Directors[0], "斋藤圭一郎")
	}

	// Writers (credits)
	if len(ep.Credits) == 0 {
		t.Error("credits should not be empty")
	} else if ep.Credits[0] != "吉田惠里香" {
		t.Errorf("credits = %q, expected %q", ep.Credits[0], "吉田惠里香")
	}

	// Actors
	if len(ep.Actors) == 0 {
		t.Error("actors should not be empty")
	} else {
		if ep.Actors[0].Name != "青山吉能" {
			t.Errorf("actor name = %q, expected %q", ep.Actors[0].Name, "青山吉能")
		}
	}

	// UniqueIDs - verify correct mapping
	if len(ep.UniqueIDs) < 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(ep.UniqueIDs))
	}
	idMap := make(map[string]string)
	for _, id := range ep.UniqueIDs {
		idMap[id.Type] = id.Value
	}
	if idMap["imdb"] != "tt21253480" {
		t.Errorf("imdb = %q, expected %q", idMap["imdb"], "tt21253480")
	}
	if idMap["tvdb"] != "8900312" {
		t.Errorf("tvdb = %q, expected %q", idMap["tvdb"], "8900312")
	}
	if idMap["tmdb"] != "12345" {
		t.Errorf("tmdb = %q, expected %q", idMap["tmdb"], "12345")
	}
}

// ── Test 7: movie unique_ids map correctly (imdb/tmdb/tvdb) ──

func TestNFOParser_ParseMovieNFO_MapsIMDBTMDBTVDBCorrectly(t *testing.T) {
	dir := t.TempDir()
	nfoPath := filepath.Join(dir, "movie.nfo")
	nfoContent := `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>Test Movie</title>
    <year>2020</year>
    <plot>A test movie.</plot>
    <rating>7.5</rating>
    <uniqueid type="imdb">tt0000001</uniqueid>
    <uniqueid type="tmdb">12345</uniqueid>
    <uniqueid type="tvdb">67890</uniqueid>
    <imdbid>tt0000001</imdbid>
    <tmdbid>12345</tmdbid>
    <tvdbid>67890</tvdbid>
</movie>`

	if err := os.WriteFile(nfoPath, []byte(nfoContent), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(nfoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	movie, err := ParseMovieNFO(f)
	if err != nil {
		t.Fatal(err)
	}

	// Verify unique IDs are mapped correctly
	if len(movie.UniqueIDs) < 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(movie.UniqueIDs))
	}
	idMap := make(map[string]string)
	for _, id := range movie.UniqueIDs {
		idMap[id.Type] = id.Value
	}

	if idMap["imdb"] != "tt0000001" {
		t.Errorf("imdb = %q, expected %q", idMap["imdb"], "tt0000001")
	}
	if idMap["tmdb"] != "12345" {
		t.Errorf("tmdb = %q, expected %q", idMap["tmdb"], "12345")
	}
	if idMap["tvdb"] != "67890" {
		t.Errorf("tvdb = %q, expected %q", idMap["tvdb"], "67890")
	}

	// IMDB must NOT end up in TVDB
	if idMap["imdb"] == idMap["tvdb"] {
		t.Error("IMDB and TVDB should not have the same value")
	}

	// Also verify old-format fields are parsed
	if movie.ImdbID != "tt0000001" {
		t.Errorf("ImdbID = %q, expected %q", movie.ImdbID, "tt0000001")
	}
	if movie.TmdbID != "12345" {
		t.Errorf("TmdbID = %q, expected %q", movie.TmdbID, "12345")
	}
	if movie.TvdbID != "67890" {
		t.Errorf("TvdbID = %q, expected %q", movie.TvdbID, "67890")
	}
}
