package keychain

import (
	"crypto/rand"
	"math"
	"math/big"
	"strings"
)

type GeneratorOptions struct {
	Length         int  `json:"length"`
	Lower          bool `json:"lower"`
	Upper          bool `json:"upper"`
	Digits         bool `json:"digits"`
	Symbols        bool `json:"symbols"`
	AvoidAmbiguous bool `json:"avoidAmbiguous"`
}

func DefaultGenerator() GeneratorOptions {
	return GeneratorOptions{Length: 20, Lower: true, Upper: true, Digits: true, Symbols: true, AvoidAmbiguous: true}
}

func (o *GeneratorOptions) normalize() {
	if o.Length < 8 {
		o.Length = 8
	}
	if o.Length > 128 {
		o.Length = 128
	}
	if !o.Lower && !o.Upper && !o.Digits && !o.Symbols {
		o.Lower, o.Digits = true, true
	}
}

func (o GeneratorOptions) sets() []string {
	lower, upper, digits := "abcdefghijklmnopqrstuvwxyz", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "0123456789"
	symbols := "!@#$%^&*-_=+?.:;~"
	if o.AvoidAmbiguous {
		lower = strings.NewReplacer("l", "", "o", "").Replace(lower)
		upper = strings.NewReplacer("I", "", "O", "").Replace(upper)
		digits = strings.NewReplacer("0", "", "1", "").Replace(digits)
	}
	var out []string
	if o.Lower {
		out = append(out, lower)
	}
	if o.Upper {
		out = append(out, upper)
	}
	if o.Digits {
		out = append(out, digits)
	}
	if o.Symbols {
		out = append(out, symbols)
	}
	return out
}

func randIndex(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic("keychain: system random source failed: " + err.Error())
	}
	return int(v.Int64())
}

func GeneratePassword(o GeneratorOptions) string {
	o.normalize()
	sets := o.sets()
	all := strings.Join(sets, "")
	out := make([]byte, 0, o.Length)
	for _, set := range sets {
		out = append(out, set[randIndex(len(set))])
	}
	for len(out) < o.Length {
		out = append(out, all[randIndex(len(all))])
	}
	for i := len(out) - 1; i > 0; i-- {
		j := randIndex(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func (o GeneratorOptions) Entropy() int {
	o.normalize()
	return int(float64(o.Length) * math.Log2(float64(len(strings.Join(o.sets(), "")))))
}
