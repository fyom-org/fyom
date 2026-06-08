package model

import "encoding/xml"

// NFO Movie metadata — maps the most common tags found in tinyMediaManager / Kodi NFO files.
type NFOMovie struct {
	XMLName   xml.Name `xml:"movie"`
	Title     string   `xml:"title"`
	SortTitle string   `xml:"sorttitle,omitempty"`
	Year      int      `xml:"year,omitempty"`
	Plot      string   `xml:"plot,omitempty"`
	Overview  string   `xml:"overview,omitempty"`
	Rating    float64  `xml:"rating,omitempty"`
	Runtime   int      `xml:"runtime,omitempty"` // minutes
	Studio    string   `xml:"studio,omitempty"`
	MPAARating string  `xml:"mpaa,omitempty"`
	Set       string   `xml:"set,omitempty"`
}

// NFO TVShow metadata — parent record for a series.
type NFOTVShow struct {
	XMLName  xml.Name `xml:"tvshow"`
	Title    string   `xml:"title"`
	Plot     string   `xml:"plot,omitempty"`
	Overview string   `xml:"overview,omitempty"`
	Rating   float64  `xml:"rating,omitempty"`
	Studio   string   `xml:"studio,omitempty"`
}

// NFO Episode metadata — child record that links to a TVShow via ShowTitle.
type NFOEpisode struct {
	XMLName    xml.Name `xml:"episodedetails"`
	Title      string   `xml:"title"`
	ShowTitle  string   `xml:"showtitle,omitempty"`
	Season     int      `xml:"season,omitempty"`
	Episode    int      `xml:"episode,omitempty"`
	Plot       string   `xml:"plot,omitempty"`
	Overview   string   `xml:"overview,omitempty"`
	Rating     float64  `xml:"rating,omitempty"`
	FirstAired string   `xml:"firstaired,omitempty"`
	Runtime    int      `xml:"runtime,omitempty"`
	Aired      string   `xml:"aired,omitempty"`
}
