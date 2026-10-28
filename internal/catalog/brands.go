package catalog

import "strings"

// Brand resolution is catalogue data, not engine logic: the engine stores the
// raw make string and the raw IANA enterprise number and interprets neither
// (spec §6). This file maps those observations onto the catalogue's brand
// names so a family can be looked up. Badges (Savin, Lanier, Gestetner…) map
// to the engine vendor; the raw badge is kept verbatim on the match.

// brandAliases maps lower-case make strings and badge names to catalogue brands.
var brandAliases = map[string]string{
	"hp": "HP", "hewlett-packard": "HP", "hewlett packard": "HP", "hp inc.": "HP", "hp inc": "HP",
	"ricoh": "Ricoh", "savin": "Ricoh", "lanier": "Ricoh", "gestetner": "Ricoh", "nashuatec": "Ricoh", "rex-rotary": "Ricoh", "infotec": "Ricoh", "nrg": "Ricoh",
	"canon":          "Canon",
	"xerox":          "Xerox",
	"konica minolta": "Konica Minolta", "konica": "Konica Minolta", "minolta": "Konica Minolta", "develop": "Konica Minolta", "konica-minolta": "Konica Minolta",
	"kyocera": "Kyocera", "kyocera mita": "Kyocera", "kyocera document solutions": "Kyocera", "triumph-adler": "Kyocera", "utax": "Kyocera",
	"lexmark": "Lexmark",
	"sharp":   "Sharp",
	"toshiba": "Toshiba", "toshiba tec": "Toshiba",
	"brother": "Brother",
	"epson":   "Epson", "seiko epson": "Epson",
	"oki": "OKI", "oki data": "OKI", "okidata": "OKI",
	"samsung":  "Samsung",
	"fujifilm": "Fujifilm", "fuji xerox": "Fujifilm", "fujifilm business innovation": "Fujifilm",
	"riso": "RISO",
}

// brandEnterprises maps IANA private enterprise numbers (sysObjectID
// .1.3.6.1.4.1.<n>) to catalogue brands. Only numbers the research team is
// confident about are listed; an unlisted number resolves to no brand rather
// than a guess.
var brandEnterprises = map[int]string{
	11:    "HP",
	367:   "Ricoh",
	1602:  "Canon",
	253:   "Xerox",
	18334: "Konica Minolta",
	1347:  "Kyocera",
	641:   "Lexmark",
	2385:  "Sharp",
	1129:  "Toshiba",
	2435:  "Brother",
	1248:  "Epson",
	2001:  "OKI",
	236:   "Samsung",
	297:   "Fujifilm", // Fuji Xerox
}

// BrandFromMake resolves a make string or badge to a catalogue brand.
func BrandFromMake(makeStr string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(makeStr))
	if m == "" {
		return "", false
	}
	if b, ok := brandAliases[m]; ok {
		return b, true
	}
	// "HP Inc.", "RICOH COMPANY, LTD.", "Canon Inc." — try the leading words.
	words := strings.FieldsFunc(m, func(r rune) bool { return r == ' ' || r == ',' || r == '.' })
	for n := len(words); n > 0; n-- {
		if b, ok := brandAliases[strings.Join(words[:n], " ")]; ok {
			return b, true
		}
	}
	return "", false
}

// BrandFromEnterprise resolves an IANA enterprise number to a catalogue brand.
func BrandFromEnterprise(n int) (string, bool) {
	b, ok := brandEnterprises[n]
	return b, ok
}

// brandInText looks for a brand alias as a whole word in free text (the model
// string or sysDescr) — used only when make and enterprise gave nothing.
func brandInText(text string) (string, bool) {
	t := strings.ToLower(text)
	words := strings.FieldsFunc(t, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') })
	for i := range words {
		for n := 3; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			if b, ok := brandAliases[strings.Join(words[i:i+n], " ")]; ok {
				return b, true
			}
		}
	}
	return "", false
}
