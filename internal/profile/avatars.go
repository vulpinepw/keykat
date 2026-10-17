package profile

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type AvatarDrawing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SVG  string `json:"svg"`
}

const AvatarInitials = "initials"

const DefaultAvatar = "cat"

var AvatarDrawings = []AvatarDrawing{
	{"cat", "Cat", `<path fill="{ink}" d="M24 18c2 0 3.5.3 4.6 1l4.4-5c1.4-1.2 2.4-.4 2.4 1.4l.2 5.6c2 2.5 2.4 5.9 1.6 8.7-1.2 5.2-6.7 8.3-13.2 8.3s-12-3.1-13.2-8.3c-.8-2.8-.4-6.2 1.6-8.7l.2-5.6c0-1.8 1-2.6 2.4-1.4l4.4 5c1.1-.7 2.6-1 4.6-1z"/><ellipse cx="19.5" cy="27.4" rx="1.9" ry="2.5" fill="{bg}"/><ellipse cx="28.5" cy="27.4" rx="1.9" ry="2.5" fill="{bg}"/><path fill="{bg}" d="M22.6 31.2h2.8L24 32.8z"/>`},
	{"fox", "Fox", `<path fill="{ink}" d="M24 38.5c-3-2-7.6-5.9-10.5-10.3-1.6-2.4-2.1-5.2-2-8l.3-8.2c.1-1.7 1.3-2.2 2.5-1.2l5.5 5c2.7-.8 5.7-.8 8.4 0l5.5-5c1.2-1 2.4-.5 2.5 1.2l.3 8.2c.1 2.8-.4 5.6-2 8-2.9 4.4-7.5 8.3-10.5 10.3z"/><path fill="{bg}" d="M16.8 24.4q2.3-2.6 4.6 0-2.3 1.9-4.6 0zM26.6 24.4q2.3-2.6 4.6 0-2.3 1.9-4.6 0zM22.4 32.4h3.2L24 34.3z"/>`},
	{"owl", "Owl", `<path fill="{ink}" d="M12 13.5l5.2 3.8c2-.9 4.3-1.3 6.8-1.3s4.8.4 6.8 1.3l5.2-3.8v13c0 7.4-4.8 12.5-12 12.5S12 33.9 12 26.5z"/><circle cx="19" cy="25" r="4.4" fill="{bg}"/><circle cx="29" cy="25" r="4.4" fill="{bg}"/><circle cx="19" cy="25" r="1.9" fill="{ink}"/><circle cx="29" cy="25" r="1.9" fill="{ink}"/><path fill="{bg}" d="M24 29.4l1.7 2-1.7 2.2-1.7-2.2z"/>`},
	{"bear", "Bear", `<circle cx="15.5" cy="16.5" r="4.6" fill="{ink}"/><circle cx="32.5" cy="16.5" r="4.6" fill="{ink}"/><circle cx="24" cy="26" r="12" fill="{ink}"/><ellipse cx="24" cy="30.5" rx="5.4" ry="4.2" fill="{bg}"/><circle cx="19.4" cy="23.4" r="1.5" fill="{bg}"/><circle cx="28.6" cy="23.4" r="1.5" fill="{bg}"/><ellipse cx="24" cy="29.2" rx="2" ry="1.4" fill="{ink}"/>`},
	{"rabbit", "Rabbit", `<path fill="{ink}" d="M18.6 21.5c-1.6-3.2-2.6-7-2.4-10.2.1-2.2 1.9-2.9 3.2-1.3 1.9 2.4 2.9 6.6 3 10.7zM29.4 21.5c1.6-3.2 2.6-7 2.4-10.2-.1-2.2-1.9-2.9-3.2-1.3-1.9 2.4-2.9 6.6-3 10.7z"/><ellipse cx="24" cy="29" rx="10.5" ry="9" fill="{ink}"/><circle cx="20" cy="27.5" r="1.4" fill="{bg}"/><circle cx="28" cy="27.5" r="1.4" fill="{bg}"/><path fill="{bg}" d="M22.7 31.2h2.6L24 32.6z"/>`},
	{"paw", "Paw", `<ellipse cx="24" cy="29.5" rx="6.6" ry="5.6" fill="{ink}"/><ellipse cx="15.4" cy="22.6" rx="2.6" ry="3.3" fill="{ink}"/><ellipse cx="20.4" cy="16.8" rx="2.6" ry="3.4" fill="{ink}"/><ellipse cx="27.6" cy="16.8" rx="2.6" ry="3.4" fill="{ink}"/><ellipse cx="32.6" cy="22.6" rx="2.6" ry="3.3" fill="{ink}"/>`},
	{"bird", "Bird", `<path fill="{ink}" d="M11.5 29.4c3.6.2 6.5-1 8.7-3.6 1.5-4.6 5.4-7.6 9.9-7.6 1.6 0 3 .4 4.2 1.1l3.4-.9-1.8 2.9c.5 1 .8 2.2.8 3.4 0 6.6-5.6 11.7-12.8 11.7-5.4 0-10-2.9-12.4-7z"/><circle cx="31" cy="22.6" r="1.4" fill="{bg}"/><path fill="{bg}" d="M21.4 30.6c2.8.2 5.3-.8 7.1-2.8-.4 3.3-3.2 5.7-6.8 5.9z"/>`},
	{"fish", "Fish", `<path fill="{ink}" d="M10.5 24c3.6-5.4 9-8 14.6-8 4.6 0 8.3 2.6 10.4 5.6l3.5-4.1v13l-3.5-4.1c-2.1 3-5.8 5.6-10.4 5.6-5.6 0-11-2.6-14.6-8z"/><circle cx="18.2" cy="22.8" r="1.6" fill="{bg}"/><path fill="none" stroke="{bg}" stroke-width="1.8" stroke-linecap="round" d="M24.5 19.6c1.8 2.9 1.8 5.9 0 8.8"/>`},
	{"key", "Key", `<circle cx="16.5" cy="24" r="7" fill="{ink}"/><circle cx="16.5" cy="24" r="2.7" fill="{bg}"/><path fill="{ink}" d="M21.5 21.8H37v4.4h-2.6v3.6H31v-3.6h-2.2v2.4h-3.3v-2.4h-4z"/>`},
	{"shield", "Shield", `<path fill="{ink}" d="M24 10.5l11 4v8.2c0 7-4.6 12.2-11 14.8-6.4-2.6-11-7.8-11-14.8v-8.2z"/><path fill="none" stroke="{bg}" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round" d="M19 24.2l3.6 3.6 6.6-7"/>`},
	{"moon", "Moon", `<circle cx="22" cy="25.5" r="11" fill="{ink}"/><circle cx="28.6" cy="20" r="9.2" fill="{bg}"/><path fill="{ink}" d="M33.5 11.5l1 2.6 2.6 1-2.6 1-1 2.6-1-2.6-2.6-1 2.6-1z"/>`},
	{"sun", "Sun", `<circle cx="24" cy="24" r="6.6" fill="{ink}"/><path fill="none" stroke="{ink}" stroke-width="2.8" stroke-linecap="round" d="M24 11v3.4M24 33.6V37M11 24h3.4M33.6 24H37M14.8 14.8l2.4 2.4M30.8 30.8l2.4 2.4M14.8 33.2l2.4-2.4M30.8 17.2l2.4-2.4"/>`},
	{"star", "Star", `<path fill="{ink}" d="M24 11.5l3.8 8.1 8.7 1-6.4 6 1.7 8.7L24 31l-7.8 4.3 1.7-8.7-6.4-6 8.7-1z"/>`},
	{"bolt", "Lightning", `<path fill="{ink}" d="M26.6 10.5L14.5 26.6h8.1L20.6 37.5l12.1-16.1h-8.1z"/>`},
	{"flame", "Flame", `<path fill="{ink}" d="M24 37.5c-6 0-10-4-10-9.4 0-4.6 3-7.6 5.2-10.2.6 2.4 1.8 3.6 3.2 4.2-.2-4.6 1.8-8.6 5.6-11.6.2 4.4 2.2 7 4.4 9.6 1.6 1.9 2.6 4.4 2.6 7.6 0 5.8-4.4 9.8-11 9.8z"/><path fill="{bg}" d="M24 34.6c-2.6 0-4.4-1.8-4.4-4.3 0-2.5 1.8-4 3.2-5.6.2 1.4.9 2.2 1.8 2.6.2-1.9.8-3.3 2-4.4.4 2.4 1.8 3.6 1.8 6.4 0 3.2-1.8 5.3-4.4 5.3z"/>`},
	{"leaf", "Leaf", `<path fill="{ink}" d="M14 34.5c-1.6-9.6 3.4-19.4 21.2-21.2 1 15.4-6.6 22.8-17.6 22z"/><path fill="none" stroke="{bg}" stroke-width="2" stroke-linecap="round" d="M15.6 33.4c4.2-6 8.6-10.5 14.6-14.6"/>`},
	{"mountain", "Mountain", `<path fill="{ink}" d="M8.5 35L20 16.5l6.2 10 3.6-5.6L39.5 35z"/><path fill="{bg}" d="M20 16.5l4.4 7.1-2.1-1.2-2.1 1.8-2.2-1.8-2.4 1.2z"/>`},
	{"heart", "Heart", `<path fill="{ink}" d="M24 36.5s-12-7-12-15.2c0-3.9 2.9-6.8 6.4-6.8 2.4 0 4.4 1.3 5.6 3.2 1.2-1.9 3.2-3.2 5.6-3.2 3.5 0 6.4 2.9 6.4 6.8 0 8.2-12 15.2-12 15.2z"/>`},
	{"ghost", "Ghost", `<path fill="{ink}" d="M13 36.5v-14c0-6.1 4.9-11 11-11s11 4.9 11 11v14l-3.7-2.6-3.6 2.6-3.7-2.6-3.7 2.6-3.6-2.6z"/><ellipse cx="20" cy="22.6" rx="1.8" ry="2.4" fill="{bg}"/><ellipse cx="28" cy="22.6" rx="1.8" ry="2.4" fill="{bg}"/>`},
	{"briefcase", "Work", `<path fill="{ink}" d="M19 15.5a2.5 2.5 0 0 1 2.5-2.5h5a2.5 2.5 0 0 1 2.5 2.5V17h-3v-1.2h-4V17h-3z"/><rect x="11" y="17" width="26" height="18" rx="3.2" fill="{ink}"/><path fill="{bg}" d="M11 25h26v2.2H11z"/><rect x="21.5" y="23.4" width="5" height="5.4" rx="1.2" fill="{ink}"/>`},
	{"house", "Home", `<path fill="{ink}" d="M24 11.5l13.4 11.2c.8.7.3 2-.8 2H34v10.3c0 1.1-.9 2-2 2H16c-1.1 0-2-.9-2-2V24.7h-2.6c-1.1 0-1.6-1.3-.8-2z"/><rect x="21" y="27" width="6" height="10" rx="1.2" fill="{bg}"/>`},
	{"bank", "Bank", `<path fill="{ink}" d="M24 10.5l13 6.6v2.6H11v-2.6zM12 21.5h3.6v9.6H12zM18.8 21.5h3.6v9.6h-3.6zM25.6 21.5h3.6v9.6h-3.6zM32.4 21.5H36v9.6h-3.6z"/><rect x="11" y="32.6" width="26" height="3.6" rx="1" fill="{ink}"/>`},
	{"cart", "Shopping", `<path fill="none" stroke="{ink}" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round" d="M10.5 13h3.8l3.8 16.4h14.8"/><path fill="{ink}" d="M16.4 17.2h20c1 0 1.6.9 1.3 1.8l-2.4 8.2c-.3.9-1.1 1.6-2.1 1.6H18.6z"/><circle cx="20" cy="34" r="2.5" fill="{ink}"/><circle cx="31" cy="34" r="2.5" fill="{ink}"/>`},
	{"gamepad", "Games", `<path fill="{ink}" d="M17 17h14c4 0 6.5 3 7.2 7.4l1 6.2c.5 3.2-3.2 5-5.4 2.6L30.5 30h-13l-3.3 3.2c-2.2 2.4-5.9.6-5.4-2.6l1-6.2C10.5 20 13 17 17 17z"/><path fill="{bg}" d="M16.2 21.4h2.2v2.2h2.2v2.2h-2.2V28h-2.2v-2.2H14v-2.2h2.2z"/><circle cx="30.4" cy="22.6" r="1.6" fill="{bg}"/><circle cx="33.6" cy="25.8" r="1.6" fill="{bg}"/>`},
	{"code", "Code", `<path fill="none" stroke="{ink}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" d="M18.5 16.5L11 24l7.5 7.5M29.5 16.5L37 24l-7.5 7.5M26.4 13.5l-4.8 21"/>`},
	{"headphones", "Music", `<path fill="none" stroke="{ink}" stroke-width="3" stroke-linecap="round" d="M12.5 28.5v-4.5a11.5 11.5 0 0 1 23 0v4.5"/><rect x="10" y="25.5" width="7" height="11" rx="2.6" fill="{ink}"/><rect x="31" y="25.5" width="7" height="11" rx="2.6" fill="{ink}"/>`},
	{"book", "Study", `<path fill="{ink}" d="M11.5 14.4c4.3-.9 8.4-.3 11.3 2v19.6c-2.9-2.3-7-2.9-11.3-2zM36.5 14.4c-4.3-.9-8.4-.3-11.3 2v19.6c2.9-2.3 7-2.9 11.3-2z"/>`},
	{"globe", "Travel", `<circle cx="24" cy="24" r="12" fill="none" stroke="{ink}" stroke-width="2.6"/><path fill="none" stroke="{ink}" stroke-width="2.6" d="M12 24h24M24 12c3.2 3.4 4.8 7.4 4.8 12s-1.6 8.6-4.8 12c-3.2-3.4-4.8-7.4-4.8-12s1.6-8.6 4.8-12z"/>`},
}

var avatarIndex = func() map[string]AvatarDrawing {
	m := map[string]AvatarDrawing{}
	for _, a := range AvatarDrawings {
		m[a.ID] = a
	}
	return m
}()

func ValidAvatar(id string) bool {
	_, ok := avatarIndex[id]
	return ok || id == AvatarInitials
}

func AvatarInk(color string) string {
	n, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if err != nil {
		return "#ffffff"
	}
	lin := func(c uint64) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin((n>>16)&255) + 0.7152*lin((n>>8)&255) + 0.0722*lin(n&255)
	if l > 0.36 {
		return "#15120a"
	}
	return "#ffffff"
}

func Initials(name string) string {
	var out []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s)
}

func avatarInner(p *Profile) string {
	color := p.Color
	if !colorRe.MatchString(color) {
		color = Palette[0]
	}
	ink := AvatarInk(color)
	if a, ok := avatarIndex[p.Avatar]; ok {
		return strings.NewReplacer("{ink}", ink, "{bg}", color).Replace(a.SVG)
	}
	initials := Initials(p.Name)
	size := 20
	if len([]rune(initials)) > 1 {
		size = 17
	}
	return fmt.Sprintf(`<text x="24" y="25" text-anchor="middle" dominant-baseline="middle" font-family="Segoe UI Variable Display, Segoe UI, system-ui, sans-serif" font-weight="700" font-size="%d" fill="%s">%s</text>`,
		size, ink, xmlEscape(initials))
}

func AvatarSVG(p *Profile) string {
	color := p.Color
	if !colorRe.MatchString(color) {
		color = Palette[0]
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">` +
		`<circle cx="24" cy="24" r="24" fill="` + color + `"/>` + avatarInner(p) + `</svg>`
}

func BadgeSVG(p *Profile) string {
	return AvatarSVG(p)
}
