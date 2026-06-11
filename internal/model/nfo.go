package model

import "encoding/xml"

// NFOUniqueID maps a provider name to its external ID.
// Kodi format: <uniqueid type="imdb" default="true">tt0816692</uniqueid>
type NFOUniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

// NFOActor represents a cast member.
// Kodi format: <actor><name>...</name><role>...</role><order>1</order><thumb>...</thumb></actor>
type NFOActor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Order int    `xml:"order"`
	Thumb string `xml:"thumb"`
}

// NFOThumb represents a thumb element with aspect attribute.
// Kodi format: <thumb aspect="poster" preview="...">...</thumb>
// Season thumbs add season and type attributes:
// <thumb season="1" type="season" aspect="poster">...</thumb>
type NFOThumb struct {
	Aspect  string `xml:"aspect,attr"`
	Preview string `xml:"preview,attr"`
	Season  string `xml:"season,attr"`
	Type    string `xml:"type,attr"`
	Spoof   string `xml:"spoof,attr"`
	Cache   string `xml:"cache,attr"`
	URL     string `xml:",chardata"`
}

// NFOFanart contains multiple background images.
type NFOFanart struct {
	Thumbs []NFOThumb `xml:"thumb"`
}

// NFORatingValue represents a single rating from a named source.
// Kodi format:
//
//	<rating name="imdb" max="10" default="true">
//	  <value>7.100000</value>
//	  <votes>111025</votes>
	//	</rating>
//
// NOTE: value and votes are CHILD ELEMENTS, not attributes!
type NFORatingValue struct {
	Name    string  `xml:"name,attr"`
	Max     float64 `xml:"max,attr"`
	Default string  `xml:"default,attr"`
	Value   float64 `xml:"value"`
	Votes   int     `xml:"votes"`
}

// NFORatings wraps multiple named ratings.
type NFORatings struct {
	Rating []NFORatingValue `xml:"rating"`
}

// NFOSet represents a movie collection/set.
// Kodi format: <set><name>Marvel Cinematic Universe</name><overview>...</overview></set>
type NFOSet struct {
	Name     string `xml:"name"`
	Overview string `xml:"overview"`
}

// NFOStreamDetails contains technical media info from <fileinfo>.
type NFOStreamDetails struct {
	Video     NFOVideo      `xml:"video"`
	Audios    []NFOAudio    `xml:"audio"`
	Subtitles []NFOSubtitle `xml:"subtitle"`
}

// NFOVideo contains video stream metadata.
type NFOVideo struct {
	Codec             string  `xml:"codec"`
	Aspect            float64 `xml:"aspect"`
	Width             int     `xml:"width"`
	Height            int     `xml:"height"`
	DurationInSeconds int     `xml:"durationinseconds"`
	StereoMode        string  `xml:"stereomode"`
	HDRType           string  `xml:"hdrtype"`
}

// NFOAudio contains audio stream metadata.
type NFOAudio struct {
	Codec    string `xml:"codec"`
	Language string `xml:"language"`
	Channels int    `xml:"channels"`
}

// NFOSubtitle contains subtitle stream metadata.
type NFOSubtitle struct {
	Language string `xml:"language"`
}

// NFOFileInfo wraps stream details.
type NFOFileInfo struct {
	StreamDetails NFOStreamDetails `xml:"streamdetails"`
}

// NFOMovie represents the movie metadata mapped from tinyMediaManager / Kodi NFO files.
type NFOMovie struct {
	XMLName       xml.Name      `xml:"movie"`
	Title         string        `xml:"title"`
	OriginalTitle string        `xml:"originaltitle"`
	SortTitle     string        `xml:"sorttitle,omitempty"`
	Year          int           `xml:"year,omitempty"`
	Premiered     string        `xml:"premiered,omitempty"`
	Plot          string        `xml:"plot,omitempty"`
	Outline       string        `xml:"outline,omitempty"`
	Tagline       string        `xml:"tagline,omitempty"`
	Rating        float64       `xml:"rating,omitempty"`
	Ratings       NFORatings    `xml:"ratings"`
	UserRating    float64       `xml:"userrating,omitempty"`
	Runtime       int           `xml:"runtime,omitempty"` // minutes
	MPAA          string        `xml:"mpaa,omitempty"`
	Playcount     int           `xml:"playcount,omitempty"`
	LastPlayed    string        `xml:"lastplayed,omitempty"`
	Genres        []string      `xml:"genre"`
	Countries     []string      `xml:"country"`
	Studios       []string      `xml:"studio"`
	Tags          []string      `xml:"tag"`
	Credits       []string      `xml:"credits"`
	Directors     []string      `xml:"director"`
	Actors        []NFOActor    `xml:"actor"`
	Set           *NFOSet       `xml:"set"`
	Thumbs        []NFOThumb    `xml:"thumb"`
	Fanart        *NFOFanart    `xml:"fanart,omitempty"`
	UniqueIDs     []NFOUniqueID `xml:"uniqueid"`
	ID            string        `xml:"id,omitempty"`
	FileInfo      NFOFileInfo   `xml:"fileinfo"`
	Trailer       string        `xml:"trailer,omitempty"`
	FileName      string        `xml:"-"`
}

// NFOTVShow represents the parent metadata record for a TV series.
type NFOTVShow struct {
	XMLName       xml.Name      `xml:"tvshow"`
	Title         string        `xml:"title"`
	OriginalTitle string        `xml:"originaltitle"`
	ShowTitle     string        `xml:"showtitle"`
	SortTitle     string        `xml:"sorttitle,omitempty"`
	Year          int           `xml:"year,omitempty"`
	Premiered     string        `xml:"premiered,omitempty"`
	Plot          string        `xml:"plot,omitempty"`
	Outline       string        `xml:"outline,omitempty"`
	Rating        float64       `xml:"rating,omitempty"`
	Ratings       NFORatings    `xml:"ratings"`
	UserRating    float64       `xml:"userrating,omitempty"`
	Runtime       int           `xml:"runtime,omitempty"`
	MPAA          string        `xml:"mpaa,omitempty"`
	Status        string        `xml:"status,omitempty"`
	Genres        []string      `xml:"genre"`
	Studios       []string      `xml:"studio"`
	Tags          []string      `xml:"tag"`
	Actors        []NFOActor    `xml:"actor"`
	Thumbs        []NFOThumb    `xml:"thumb"`
	Fanart        *NFOFanart    `xml:"fanart,omitempty"`
	UniqueIDs     []NFOUniqueID `xml:"uniqueid"`
	ID            string        `xml:"id,omitempty"`
	FileName      string        `xml:"-"`
}

// NFOEpisode represents the episode metadata record, linked to an NFOTVShow via ShowTitle.
type NFOEpisode struct {
	XMLName        xml.Name      `xml:"episodedetails"`
	Title          string        `xml:"title"`
	ShowTitle      string        `xml:"showtitle,omitempty"`
	Season         int           `xml:"season,omitempty"`
	Episode        int           `xml:"episode,omitempty"`
	DisplaySeason  int           `xml:"displayseason,omitempty"`
	DisplayEpisode int           `xml:"displayepisode,omitempty"`
	Plot           string        `xml:"plot,omitempty"`
	Outline        string        `xml:"outline,omitempty"`
	Rating         float64       `xml:"rating,omitempty"`
	Ratings        NFORatings    `xml:"ratings"`
	UserRating     float64       `xml:"userrating,omitempty"`
	Runtime        int           `xml:"runtime,omitempty"`
	MPAA           string        `xml:"mpaa,omitempty"`
	Aired          string        `xml:"aired,omitempty"`
	Premiered      string        `xml:"premiered,omitempty"`
	Genres         []string      `xml:"genre"`
	Studios        []string      `xml:"studio"`
	Credits        []string      `xml:"credits"`
	Directors      []string      `xml:"director"`
	Actors         []NFOActor    `xml:"actor"`
	Thumbs         []NFOThumb    `xml:"thumb"`
	UniqueIDs      []NFOUniqueID `xml:"uniqueid"`
	ID             string        `xml:"id,omitempty"`
	FileInfo       NFOFileInfo   `xml:"fileinfo"`
	FileName       string        `xml:"-"`
}
