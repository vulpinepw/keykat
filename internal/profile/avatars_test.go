package profile

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

func TestAvatarsRenderValidSVG(t *testing.T) {
	seen := map[string]bool{}
	for _, color := range Palette {
		for _, a := range append(AvatarDrawings, AvatarDrawing{ID: AvatarInitials}) {
			if color == Palette[0] {
				if seen[a.ID] {
					t.Errorf("duplicate avatar id %s", a.ID)
				}
				seen[a.ID] = true
			}
			p := &Profile{Name: "Work <Main> & Co", Color: color, Avatar: a.ID, Security: Security{Mode: ModeEncrypted}}
			for _, svg := range []string{AvatarSVG(p), BadgeSVG(p)} {
				if strings.Contains(svg, "{ink}") || strings.Contains(svg, "{bg}") {
					t.Fatalf("%s: unreplaced token", a.ID)
				}
				if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
					t.Fatalf("%s/%s: invalid SVG: %v", a.ID, color, err)
				}
			}
		}
	}
}

func TestAvatarContrastAndInitials(t *testing.T) {
	if AvatarInk("#F5A524") != "#15120a" || AvatarInk("#6366F1") != "#ffffff" {
		t.Error("ink contrast choice is wrong")
	}
	cases := map[string]string{"Work": "W", "online banking": "OB", "  ": "?", "école privée": "ÉP", "a b c": "AB"}
	for name, want := range cases {
		if got := Initials(name); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
	svg := AvatarSVG(&Profile{Name: "<x> &y", Color: "#000000", Avatar: AvatarInitials})
	if strings.Contains(svg, "<x>") || strings.Contains(svg, "&y") || !strings.Contains(svg, ">XY</text>") {
		t.Errorf("unexpected initials markup: %s", svg)
	}
}

func TestAvatarGallery(t *testing.T) {
	out := os.Getenv("KEYCAT_AVATAR_PREVIEW")
	if out == "" {
		t.Skip("set KEYCAT_AVATAR_PREVIEW to write the gallery")
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8"><body style="margin:0;padding:24px;background:#0f1115;font:13px Segoe UI;color:#9aa3b2">`)
	b.WriteString(`<div style="display:grid;grid-template-columns:repeat(8,96px);gap:18px">`)
	colors := []string{"#F5A524", "#8B6CF6", "#2EB67D", "#4C8DF6", "#EF4444", "#64748B", "#E05AB4", "#22B8CF"}
	for i, a := range append(AvatarDrawings, AvatarDrawing{ID: AvatarInitials, Name: "Initials"}) {
		p := &Profile{Name: "Keycat Work", Color: colors[i%len(colors)], Avatar: a.ID, Security: Security{Mode: ModeEncrypted}}
		b.WriteString(`<figure style="margin:0;text-align:center"><div style="width:96px;height:96px">` + AvatarSVG(p) +
			`</div><figcaption>` + a.Name + `</figcaption></figure>`)
	}
	b.WriteString(`</div><div style="margin-top:28px;display:flex;gap:16px;align-items:center;background:#1b1f26;padding:12px">`)
	for _, a := range AvatarDrawings[:8] {
		p := &Profile{Name: "Work", Color: "#F5A524", Avatar: a.ID, Security: Security{Mode: ModeEncrypted}}
		b.WriteString(`<div style="width:20px;height:20px">` + BadgeSVG(p) + `</div>`)
	}
	b.WriteString(`</div></body>`)
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
