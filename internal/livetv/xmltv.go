package livetv

import (
	"encoding/xml"
	"io"
	"strings"
	"time"
)

// GuideChannel is an XMLTV <channel>.
type GuideChannel struct {
	ID, Name, Icon string
}

// Program is an XMLTV <programme>.
type Program struct {
	Channel     string
	Start, Stop time.Time
	Title       string
	SubTitle    string
	Description string
	Category    string
	Icon        string
	Episode     string
}

type xmlProgramme struct {
	Start    string `xml:"start,attr"`
	Stop     string `xml:"stop,attr"`
	Channel  string `xml:"channel,attr"`
	Title    []text `xml:"title"`
	SubTitle []text `xml:"sub-title"`
	Desc     []text `xml:"desc"`
	Category []text `xml:"category"`
	Icon     struct {
		Src string `xml:"src,attr"`
	} `xml:"icon"`
	Episode []struct {
		System string `xml:"system,attr"`
		Value  string `xml:",chardata"`
	} `xml:"episode-num"`
}

type text struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

func pick(list []text, lang string) string {
	for _, t := range list {
		if lang != "" && strings.EqualFold(t.Lang, lang) {
			return strings.TrimSpace(t.Value)
		}
	}
	if len(list) > 0 {
		return strings.TrimSpace(list[0].Value)
	}
	return ""
}

// ParseXMLTVTime parses "20260109120000 +0100" style timestamps.
func ParseXMLTVTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"20060102150405 -0700", "20060102150405-0700", "20060102150405", "200601021504 -0700", "200601021504"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ParseXMLTV streams an XMLTV document, calling onChannel/onProgram for each
// entry within [from, to). lang selects the preferred language (e.g. "it").
func ParseXMLTV(r io.Reader, lang string, from, to time.Time, onChannel func(GuideChannel), onProgram func(Program)) error {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "channel":
			var c struct {
				ID    string `xml:"id,attr"`
				Names []text `xml:"display-name"`
				Icon  struct {
					Src string `xml:"src,attr"`
				} `xml:"icon"`
			}
			if err := dec.DecodeElement(&c, &se); err != nil {
				return err
			}
			onChannel(GuideChannel{ID: c.ID, Name: pick(c.Names, lang), Icon: c.Icon.Src})
		case "programme":
			var p xmlProgramme
			if err := dec.DecodeElement(&p, &se); err != nil {
				return err
			}
			start, ok1 := ParseXMLTVTime(p.Start)
			stop, ok2 := ParseXMLTVTime(p.Stop)
			if !ok1 {
				continue
			}
			if !ok2 {
				stop = start.Add(30 * time.Minute)
			}
			if stop.Before(from) || !start.Before(to) {
				continue
			}
			ep := ""
			for _, e := range p.Episode {
				if e.System == "onscreen" || ep == "" {
					ep = strings.TrimSpace(e.Value)
				}
			}
			onProgram(Program{Channel: p.Channel, Start: start, Stop: stop, Title: pick(p.Title, lang), SubTitle: pick(p.SubTitle, lang),
				Description: pick(p.Desc, lang), Category: pick(p.Category, lang), Icon: p.Icon.Src, Episode: ep})
		}
	}
}
