package profile

const BrowserConfigFile = "keycat-profile.json"

type BrowserConfig struct {
	Version        int               `json:"version"`
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Color          string            `json:"color"`
	Avatar         string            `json:"avatar"`
	AvatarSVG      string            `json:"avatarSvg"`
	BadgeSVG       string            `json:"badgeSvg"`
	Encryption     EncryptionSummary `json:"encryption"`
	Privacy        Privacy           `json:"privacy"`
	Appearance     Appearance        `json:"appearance"`
	CookieBehavior int               `json:"cookieBehavior"`
	ResetPrefs     []string          `json:"resetPrefs"`
	Summary        []SummaryItem     `json:"summary"`
}

func NewBrowserConfig(p *Profile, ps PrefSet) BrowserConfig {
	return BrowserConfig{
		Version:        1,
		ID:             p.ID,
		Name:           p.Name,
		Color:          p.Color,
		Avatar:         p.Avatar,
		AvatarSVG:      AvatarSVG(p),
		BadgeSVG:       BadgeSVG(p),
		Encryption:     p.Encryption(),
		Privacy:        p.Privacy,
		Appearance:     p.Appearance,
		CookieBehavior: CookieBehavior(p.Privacy.Cookies),
		ResetPrefs:     ps.Reset,
		Summary:        Summary(p),
	}
}
