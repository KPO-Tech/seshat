package pdfsmart

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// A page read from its text layer says nothing about its pictures. Rather than let a reader believe the page is only what
// it says, the text ends with one marker for each picture worth one: "[Image 2 on page 4: Figure 2. Residual learning]",
// or "[Image 2 on page 4]" when no caption could be attached. The marker is the handle a host can use later, with a
// multimodal model, to ask for the page itself (its number) and for the picture on it (its number on the page).
const (
	// A picture narrower or shorter than this, in pixels, is an icon, a bullet or a rule.
	markerImageMinSide = 100
	// A picture used on this many pages or more is a logo or a background, not a figure.
	repeatedPictureMinPages = 3
	// A caption longer than this, in characters, is cut.
	maxCaptionRunes = 160
)

// Picture is a picture of a page that its text does not say anything about.
type Picture struct {
	// Number counts the pictures of the page from 1, in the order the file lists them (which is not always the order on the
	// page). With the page number it names the picture for good, whatever pages were read.
	Number int
	// Title is the caption found in the text of the page ("Figure 2. ..."), or empty when there is none to attach.
	Title string
}

// ImageMarker writes the marker of a picture in the text of a page.
func ImageMarker(page int, picture Picture) string {
	if picture.Title == "" {
		return fmt.Sprintf("[Image %d on page %d]", picture.Number, page)
	}
	return fmt.Sprintf("[Image %d on page %d: %s]", picture.Number, page, picture.Title)
}

// withImageMarkers appends a marker for each of count pictures to the text of a page. A caption is attached to a picture only
// when the page has as many captions as pictures, in which case they are taken in order: with fewer pictures than captions
// (a figure drawn as vectors has a caption and no picture) which caption belongs to which picture is a guess, and a wrong
// title is worse than none.
func withImageMarkers(text string, page, count int) (string, []Picture) {
	if count <= 0 {
		return text, nil
	}
	captions := captionsIn(text)
	if len(captions) != count {
		captions = nil
	}
	pictures := make([]Picture, count)
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(text, "\n"))
	for i := range pictures {
		pictures[i].Number = i + 1
		if captions != nil {
			pictures[i].Title = captions[i]
		}
		sb.WriteString("\n\n")
		sb.WriteString(ImageMarker(page, pictures[i]))
	}
	return sb.String(), pictures
}

var captionStart = regexp.MustCompile(`(?i)^(figure|fig\.?|image|chart|diagram|photo|illustration|figura|abbildung|abb\.|schéma|schema|graphique)\s*\d+`)

// captionsIn finds the lines of a text that start like the caption of a figure ("Figure 3.", "Fig. 3:", "Figura 3 -").
func captionsIn(text string) []string {
	var captions []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "#*_> ")
		if !captionStart.MatchString(line) {
			continue
		}
		line = strings.Join(strings.Fields(strings.NewReplacer("*", "", "_", "", "[", "(", "]", ")").Replace(line)), " ")
		if utf8.RuneCountInString(line) > maxCaptionRunes {
			line = string([]rune(line)[:maxCaptionRunes]) + "…"
		}
		captions = append(captions, line)
	}
	return captions
}

var markerLine = regexp.MustCompile(`^\[Image \d+ on page \d+(: .*)?\]$`)

// HasText says whether the page gave any text besides the markers of its pictures: a page that is only its markers has
// not been read, and a host that tells the pages it could not read from the others must not count it as read.
func (p PageResult) HasText() bool {
	for _, line := range strings.Split(p.Text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !markerLine.MatchString(line) {
			return true
		}
	}
	return false
}
