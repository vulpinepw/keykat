package identity

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/mail"
	"strings"
	"time"
)

type Identity struct {
	Country     string    `json:"country"`
	CountryName string    `json:"countryName"`
	Gender      string    `json:"gender"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	FullName    string    `json:"fullName"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	Phone       string    `json:"phone"`
	PhoneLocal  string    `json:"phoneLocal"`
	Birthday    string    `json:"birthday"`
	Street      string    `json:"street"`
	City        string    `json:"city"`
	Region      string    `json:"region"`
	RegionCode  string    `json:"regionCode"`
	PostalCode  string    `json:"postalCode"`
	Company     string    `json:"company"`
	JobTitle    string    `json:"jobTitle"`
	Generated   time.Time `json:"generated"`
}

type Settings struct {
	Country     string `json:"country"`
	EmailMode   string `json:"emailMode"`
	EmailBase   string `json:"emailBase,omitempty"`
	EmailDomain string `json:"emailDomain,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{Country: "US", EmailMode: "placeholder"}
}

func Countries() [][2]string {
	return [][2]string{{"US", "United States"}, {"GB", "United Kingdom"}, {"CA", "Canada"}, {"FR", "France"}, {"any", "Any of these"}}
}

func (s *Settings) Validate() error {
	if s.Country == "" {
		s.Country = "US"
	}
	if _, ok := countries[s.Country]; !ok && s.Country != "any" {
		return fmt.Errorf("unsupported country %q", s.Country)
	}
	switch s.EmailMode {
	case "", "placeholder":
		s.EmailMode = "placeholder"
	case "alias":
		addr, err := mail.ParseAddress(s.EmailBase)
		if err != nil || addr.Address != s.EmailBase || !strings.Contains(s.EmailBase, "@") {
			return errors.New("enter the e-mail address to create aliases of, like you@example.org")
		}
	case "domain":
		d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s.EmailDomain)), "@")
		if d == "" || strings.ContainsAny(d, " @/\\") || !strings.Contains(d, ".") {
			return errors.New("enter a domain you control, like example.org")
		}
		s.EmailDomain = d
	default:
		return fmt.Errorf("unknown e-mail mode %q", s.EmailMode)
	}
	return nil
}

func newRand() *rand.Rand {
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		panic(err)
	}
	return rand.New(rand.NewChaCha8(seed))
}

func pick[T any](r *rand.Rand, list []T) T { return list[r.IntN(len(list))] }

var foldAccents = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "æ", "ae",
	"ç", "c", "è", "e", "é", "e", "ê", "e", "ë", "e", "ì", "i", "í", "i", "î", "i", "ï", "i",
	"ñ", "n", "ò", "o", "ó", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o", "œ", "oe",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y", "ß", "ss",
)

func ascii(s string) string {
	var b strings.Builder
	for _, r := range foldAccents.Replace(strings.ToLower(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func Generate(s Settings) Identity {
	r := newRand()
	code := s.Country
	if code == "any" || countries[code] == nil {
		codes := []string{"US", "GB", "CA", "FR"}
		code = pick(r, codes)
	}
	c := countries[code]
	id := Identity{Country: code, CountryName: c.name, Generated: time.Now().UTC()}
	if r.IntN(2) == 0 {
		id.Gender, id.FirstName = "female", pick(r, c.female)
	} else {
		id.Gender, id.FirstName = "male", pick(r, c.male)
	}
	id.LastName = pick(r, c.last)
	id.FullName = id.FirstName + " " + id.LastName

	first, last := ascii(id.FirstName), ascii(id.LastName)
	if first == "" {
		first = "alex"
	}
	if last == "" {
		last = "lee"
	}
	n := r.IntN(90) + 10
	switch r.IntN(3) {
	case 0:
		id.Username = fmt.Sprintf("%s%s%d", first, last[:1], n)
	case 1:
		id.Username = fmt.Sprintf("%s_%s%d", first, last, n)
	default:
		id.Username = fmt.Sprintf("%s.%s%d", first[:1], last, n)
	}
	switch s.EmailMode {
	case "alias":
		at := strings.LastIndex(s.EmailBase, "@")
		local, domain := s.EmailBase[:at], s.EmailBase[at+1:]
		if i := strings.Index(local, "+"); i >= 0 {
			local = local[:i]
		}
		id.Email = fmt.Sprintf("%s+%s%d@%s", local, first, n, domain)
	case "domain":
		id.Email = fmt.Sprintf("%s.%s%d@%s", first, last, n, s.EmailDomain)
	default:
		id.Email = fmt.Sprintf("%s.%s%d@example.com", first, last, n)
	}

	city := pick(r, c.cities)
	id.City, id.Region, id.RegionCode = city.name, city.region, city.regionCode
	id.Street = c.street(r)
	id.PostalCode = c.postal(r, city)
	id.Phone, id.PhoneLocal = c.phone(r, city)

	now := time.Now()
	age := r.IntN(35) + 24
	day := time.Date(now.Year()-age, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, r.IntN(365))
	id.Birthday = day.Format("2006-01-02")

	id.Company = pick(r, companies)
	id.JobTitle = pick(r, jobTitles)
	return id
}

type city struct {
	name, region, regionCode string
	postal                   string
	area                     string
}

type country struct {
	name               string
	male, female, last []string
	cities             []city
	street             func(*rand.Rand) string
	postal             func(*rand.Rand, city) string
	phone              func(*rand.Rand, city) (intl, local string)
}

var countries = map[string]*country{
	"US": {
		name:   "United States",
		male:   usMale,
		female: usFemale,
		last:   usLast,
		cities: []city{
			{"Springfield", "Illinois", "IL", "627", "217"}, {"Portland", "Oregon", "OR", "972", "503"},
			{"Austin", "Texas", "TX", "787", "512"}, {"Denver", "Colorado", "CO", "802", "303"},
			{"Columbus", "Ohio", "OH", "432", "614"}, {"Madison", "Wisconsin", "WI", "537", "608"},
			{"Raleigh", "North Carolina", "NC", "276", "919"}, {"Sacramento", "California", "CA", "958", "916"},
			{"Phoenix", "Arizona", "AZ", "850", "602"}, {"Nashville", "Tennessee", "TN", "372", "615"},
			{"Richmond", "Virginia", "VA", "232", "804"}, {"Boise", "Idaho", "ID", "837", "208"},
			{"Albany", "New York", "NY", "122", "518"}, {"Tampa", "Florida", "FL", "336", "813"},
			{"Omaha", "Nebraska", "NE", "681", "402"}, {"Spokane", "Washington", "WA", "992", "509"},
			{"Salt Lake City", "Utah", "UT", "841", "801"}, {"Pittsburgh", "Pennsylvania", "PA", "152", "412"},
			{"Minneapolis", "Minnesota", "MN", "554", "612"}, {"Louisville", "Kentucky", "KY", "402", "502"},
		},
		street: func(r *rand.Rand) string {
			return fmt.Sprintf("%d %s %s", r.IntN(9800)+100, pick(r, usStreets), pick(r, []string{"Street", "Avenue", "Road", "Drive", "Lane", "Court", "Way", "Boulevard", "Place"}))
		},
		postal: func(r *rand.Rand, c city) string { return fmt.Sprintf("%s%02d", c.postal, r.IntN(100)) },
		phone: func(r *rand.Rand, c city) (string, string) {
			line := fmt.Sprintf("555-01%02d", r.IntN(100))
			return fmt.Sprintf("+1 %s %s", c.area, line), fmt.Sprintf("(%s) %s", c.area, line)
		},
	},
	"CA": {
		name:   "Canada",
		male:   usMale,
		female: usFemale,
		last:   append(append([]string{}, usLast[:30]...), "Tremblay", "Gagnon", "Roy", "Côté", "Bouchard", "Gauthier", "Morin", "Lavoie", "Fortin", "Gagné"),
		cities: []city{
			{"Toronto", "Ontario", "ON", "M4", "416"}, {"Ottawa", "Ontario", "ON", "K1", "613"},
			{"Vancouver", "British Columbia", "BC", "V6", "604"}, {"Victoria", "British Columbia", "BC", "V8", "250"},
			{"Montréal", "Quebec", "QC", "H2", "514"}, {"Québec", "Quebec", "QC", "G1", "418"},
			{"Calgary", "Alberta", "AB", "T2", "403"}, {"Edmonton", "Alberta", "AB", "T5", "780"},
			{"Winnipeg", "Manitoba", "MB", "R3", "204"}, {"Halifax", "Nova Scotia", "NS", "B3", "902"},
		},
		street: func(r *rand.Rand) string {
			return fmt.Sprintf("%d %s %s", r.IntN(2400)+10, pick(r, usStreets), pick(r, []string{"Street", "Avenue", "Road", "Drive", "Crescent", "Boulevard"}))
		},
		postal: func(r *rand.Rand, c city) string {
			const letters = "ABCEGHJKLMNPRSTVWXYZ"
			return fmt.Sprintf("%s%c %d%c%d", c.postal, letters[r.IntN(len(letters))], r.IntN(10), letters[r.IntN(len(letters))], r.IntN(10))
		},
		phone: func(r *rand.Rand, c city) (string, string) {
			line := fmt.Sprintf("555-01%02d", r.IntN(100))
			return fmt.Sprintf("+1 %s %s", c.area, line), fmt.Sprintf("(%s) %s", c.area, line)
		},
	},
	"GB": {
		name:   "United Kingdom",
		male:   gbMale,
		female: gbFemale,
		last:   gbLast,
		cities: []city{
			{"Leeds", "West Yorkshire", "", "LS", ""}, {"Bristol", "Bristol", "", "BS", ""},
			{"Manchester", "Greater Manchester", "", "M", ""}, {"Birmingham", "West Midlands", "", "B", ""},
			{"Sheffield", "South Yorkshire", "", "S", ""}, {"Nottingham", "Nottinghamshire", "", "NG", ""},
			{"Liverpool", "Merseyside", "", "L", ""}, {"Cardiff", "Cardiff", "", "CF", ""},
			{"Edinburgh", "City of Edinburgh", "", "EH", ""}, {"Glasgow", "Glasgow City", "", "G", ""},
			{"Brighton", "East Sussex", "", "BN", ""}, {"Oxford", "Oxfordshire", "", "OX", ""},
			{"Cambridge", "Cambridgeshire", "", "CB", ""}, {"York", "North Yorkshire", "", "YO", ""},
			{"Norwich", "Norfolk", "", "NR", ""}, {"Exeter", "Devon", "", "EX", ""},
		},
		street: func(r *rand.Rand) string {
			return fmt.Sprintf("%d %s %s", r.IntN(180)+1, pick(r, gbStreets), pick(r, []string{"Road", "Street", "Lane", "Avenue", "Close", "Drive", "Gardens", "Way"}))
		},
		postal: func(r *rand.Rand, c city) string {
			const letters = "ABDEFGHJLNPQRSTUWXYZ"
			return fmt.Sprintf("%s%d %d%c%c", c.postal, r.IntN(19)+1, r.IntN(10), letters[r.IntN(len(letters))], letters[r.IntN(len(letters))])
		},
		phone: func(r *rand.Rand, c city) (string, string) {
			n := r.IntN(1000)
			return fmt.Sprintf("+44 7700 900%03d", n), fmt.Sprintf("07700 900%03d", n)
		},
	},
	"FR": {
		name:   "France",
		male:   frMale,
		female: frFemale,
		last:   frLast,
		cities: []city{
			{"Paris", "Île-de-France", "", "750", ""}, {"Lyon", "Auvergne-Rhône-Alpes", "", "690", ""},
			{"Marseille", "Provence-Alpes-Côte d'Azur", "", "130", ""}, {"Toulouse", "Occitanie", "", "310", ""},
			{"Nantes", "Pays de la Loire", "", "440", ""}, {"Bordeaux", "Nouvelle-Aquitaine", "", "330", ""},
			{"Lille", "Hauts-de-France", "", "590", ""}, {"Strasbourg", "Grand Est", "", "670", ""},
			{"Montpellier", "Occitanie", "", "340", ""}, {"Rennes", "Bretagne", "", "350", ""},
			{"Grenoble", "Auvergne-Rhône-Alpes", "", "380", ""}, {"Dijon", "Bourgogne-Franche-Comté", "", "210", ""},
		},
		street: func(r *rand.Rand) string { return fmt.Sprintf("%d %s", r.IntN(120)+1, pick(r, frStreets)) },
		postal: func(r *rand.Rand, c city) string {
			if c.name == "Paris" {
				return fmt.Sprintf("750%02d", r.IntN(20)+1)
			}
			return fmt.Sprintf("%s%02d", c.postal, r.IntN(10)*10)
		},
		phone: func(r *rand.Rand, c city) (string, string) {
			a, b := r.IntN(100), r.IntN(100)
			return fmt.Sprintf("+33 6 39 98 %02d %02d", a, b), fmt.Sprintf("06 39 98 %02d %02d", a, b)
		},
	},
}
