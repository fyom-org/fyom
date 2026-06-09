package model

import "encoding/xml"

// NFOMovie represents the movie metadata mapped from tinyMediaManager / Kodi NFO files.
type NFOMovie struct {
	XMLName    xml.Name    `xml:"movie"`
	Title      string      `xml:"title"`
	SortTitle  string      `xml:"sorttitle,omitempty"`
	Year       int         `xml:"year,omitempty"`
	Plot       string      `xml:"plot,omitempty"`
	Overview   string      `xml:"overview,omitempty"`
	Rating     float64     `xml:"rating,omitempty"`
	Runtime    int         `xml:"runtime,omitempty"` // minutes
	Studio     string      `xml:"studio,omitempty"`
	MPAARating string      `xml:"mpaa,omitempty"`
	Set        string      `xml:"set,omitempty"`
	Thumbs     []NFOThumb  `xml:"thumb"`
	Fanart     *NFOFanart  `xml:"fanart,omitempty"`
}

// NFOTVShow represents the parent metadata record for a TV series.
type NFOTVShow struct {
	XMLName  xml.Name    `xml:"tvshow"`
	Title    string      `xml:"title"`
	Year     int         `xml:"year,omitempty"`
	Plot     string      `xml:"plot,omitempty"`
	Overview string      `xml:"overview,omitempty"`
	Rating   float64     `xml:"rating,omitempty"`
	Runtime  int         `xml:"runtime,omitempty"`
	Studio   string      `xml:"studio,omitempty"`
	Thumbs   []NFOThumb  `xml:"thumb"`
	Fanart   *NFOFanart  `xml:"fanart,omitempty"`
}

// NFOEpisode represents the episode metadata record, linked to an NFOTVShow via ShowTitle.
type NFOEpisode struct {
	XMLName    xml.Name `xml:"episodedetails"`
	Title      string   `xml:"title"`
	ShowTitle  string   `xml:"showtitle,omitempty"`
	Season     int      `xml:"season,omitempty"`
	Episode    int      `xml:"episode,omitempty"`
	Plot       string   `xml:"plot,omitempty"`
	Overview   string   `xml:"overview,omitempty"`
	Rating     float64  `xml:"rating,omitempty"`
	Runtime    int      `xml:"runtime,omitempty"`
	FirstAired string   `xml:"firstaired,omitempty"`
	Aired      string   `xml:"aired,omitempty"`
	Thumbs     []NFOThumb `xml:"thumb"`
}

// NFOThumb represents a <thumb> element which may have an aspect attribute.
type NFOThumb struct {
	Aspect string `xml:"aspect,attr,omitempty"`
	Path   string `xml:",chardata"`
}

// NFOFanart represents a <fanart> element containing <thumb> children.
type NFOFanart struct {
	Thumbs []NFOThumb `xml:"thumb"`
}
