package identity

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	phoneRe = map[string]*regexp.Regexp{
		"US": regexp.MustCompile(`^\+1 \d{3} 555-01\d\d$`),
		"CA": regexp.MustCompile(`^\+1 \d{3} 555-01\d\d$`),
		"GB": regexp.MustCompile(`^\+44 7700 900\d{3}$`),
		"FR": regexp.MustCompile(`^\+33 6 39 98 \d\d \d\d$`),
	}
	postalRe = map[string]*regexp.Regexp{
		"US": regexp.MustCompile(`^\d{5}$`),
		"CA": regexp.MustCompile(`^[A-Z]\d[A-Z] \d[A-Z]\d$`),
		"GB": regexp.MustCompile(`^[A-Z]{1,2}\d{1,2} \d[A-Z]{2}$`),
		"FR": regexp.MustCompile(`^\d{5}$`),
	}
	emailRe = regexp.MustCompile(`^[a-z0-9._+]+@[a-z0-9.-]+\.[a-z]+$`)
)

func TestGenerateAllCountries(t *testing.T) {
	for _, code := range []string{"US", "CA", "GB", "FR"} {
		for i := 0; i < 200; i++ {
			id := Generate(Settings{Country: code, EmailMode: "placeholder"})
			if id.Country != code {
				t.Fatalf("country %s, want %s", id.Country, code)
			}
			for name, v := range map[string]string{
				"first": id.FirstName, "last": id.LastName, "street": id.Street, "city": id.City,
				"username": id.Username, "company": id.Company, "job": id.JobTitle,
			} {
				if strings.TrimSpace(v) == "" {
					t.Fatalf("%s: empty %s", code, name)
				}
			}
			if !phoneRe[code].MatchString(id.Phone) {
				t.Fatalf("%s: phone %q is not in the fictional range", code, id.Phone)
			}
			if !postalRe[code].MatchString(id.PostalCode) {
				t.Fatalf("%s: postal code %q malformed", code, id.PostalCode)
			}
			if !emailRe.MatchString(id.Email) || !strings.HasSuffix(id.Email, "@example.com") {
				t.Fatalf("%s: placeholder email %q", code, id.Email)
			}
			bday, err := time.Parse("2006-01-02", id.Birthday)
			if err != nil {
				t.Fatal(err)
			}
			age := time.Since(bday).Hours() / 24 / 365.25
			if age < 23 || age > 60 {
				t.Fatalf("age %.1f out of range", age)
			}
		}
	}
}

func TestEmailModes(t *testing.T) {
	alias := Settings{Country: "US", EmailMode: "alias", EmailBase: "me+old@mail.example.org"}
	if err := alias.Validate(); err != nil {
		t.Fatal(err)
	}
	id := Generate(alias)
	if !strings.HasPrefix(id.Email, "me+") || !strings.HasSuffix(id.Email, "@mail.example.org") {
		t.Errorf("alias email %q", id.Email)
	}
	domain := Settings{Country: "FR", EmailMode: "domain", EmailDomain: "@Catch.Example.NET"}
	if err := domain.Validate(); err != nil {
		t.Fatal(err)
	}
	id = Generate(domain)
	if !strings.HasSuffix(id.Email, "@catch.example.net") || !emailRe.MatchString(id.Email) {
		t.Errorf("domain email %q (accents must be folded)", id.Email)
	}
	for _, bad := range []Settings{
		{Country: "ZZ"},
		{Country: "US", EmailMode: "alias", EmailBase: "not an email"},
		{Country: "US", EmailMode: "domain", EmailDomain: "no-dot"},
		{Country: "US", EmailMode: "carrier-pigeon"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("accepted invalid settings %+v", bad)
		}
	}
}

func TestIdentitiesDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := Generate(Settings{Country: "any", EmailMode: "placeholder"})
		seen[id.Email+id.Phone] = true
	}
	if len(seen) < 45 {
		t.Errorf("only %d distinct identities out of 50", len(seen))
	}
}

func TestAsciiFolding(t *testing.T) {
	if got := ascii("Léa Lefèvre-Côté"); got != "lealefevrecote" {
		t.Errorf("ascii = %q", got)
	}
}
