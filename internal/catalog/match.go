package catalog

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Confidence levels of a match, from strongest to weakest.
const (
	ConfidenceExact     = "exact"      // a base model identifier (or an explicit range) was found in the observations
	ConfidenceFamily    = "family"     // only a generic/wildcard identity matched, or several families tied
	ConfidenceBrandOnly = "brand-only" // the brand resolved but no family contains the model
	ConfidenceNone      = "none"       // the brand itself is unknown to the catalogue
)

// Observations are the raw inputs of a match, kept verbatim (the research
// insists that a reported name is an observation, not a key).
type Observations struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	SysDescr     string `json:"sysDescr"`
	EnterpriseID int    `json:"enterpriseId"`
}

// Match is the result of looking a device up in the catalogue.
type Match struct {
	Confidence   string              `json:"confidence"`
	Brand        string              `json:"brand,omitempty"`
	BrandBasis   string              `json:"brandBasis,omitempty"` // make | enterprise | text
	FamilyID     string              `json:"familyId,omitempty"`
	Family       *Family             `json:"family,omitempty"`
	ProfileID    string              `json:"profileId,omitempty"`
	Profile      *ManagementProfile  `json:"profile,omitempty"`
	ModelID      string              `json:"modelId,omitempty"`      // the catalogue base identifier that matched
	MatchedToken string              `json:"matchedToken,omitempty"` // the observation token it matched
	Evidence     []OperationEvidence `json:"evidence"`               // model-specific entries for ModelID
	Candidates   []string            `json:"candidates,omitempty"`   // other family ids that scored equally (ambiguity)
	Reason       string              `json:"reason"`
	Observations Observations        `json:"observations"`
}

// modelKey is a normalised base model identifier with its family.
type modelKey struct {
	familyID string
	raw      string
	key      string // upper-case, no spaces/hyphens
	// explicit numeric range "M282-M285": prefix letters + lo..hi
	rangePrefix string
	rangeLo     int
	rangeHi     int
	// trailing wildcard "M28x" (only when the x follows digits)
	wildcardStem string
}

var rangeRe = regexp.MustCompile(`^([A-Z]*)(\d+)-([A-Z]*)(\d+)$`)
var wildcardRe = regexp.MustCompile(`^([A-Z]*\d+)X$`)

func newModelKey(familyID, raw string) modelKey {
	mk := modelKey{familyID: familyID, raw: raw, key: normalise(raw)}
	// Ranges keep their hyphen for detection ("M282-M285"); the key itself has none.
	if m := rangeRe.FindStringSubmatch(strings.ToUpper(strings.ReplaceAll(raw, " ", ""))); m != nil && (m[3] == "" || m[3] == m[1]) {
		lo, _ := strconv.Atoi(m[2])
		hi, _ := strconv.Atoi(m[4])
		if lo <= hi && len(m[2]) == len(m[4]) {
			mk.rangePrefix, mk.rangeLo, mk.rangeHi = m[1], lo, hi
		}
	}
	if m := wildcardRe.FindStringSubmatch(mk.key); m != nil {
		mk.wildcardStem = m[1]
	}
	return mk
}

// normalise upper-cases an identifier and removes spaces and hyphens so
// "IM C3010", "IM-C3010" and "IMC3010" compare equal.
func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r == ' ' || r == '-' || r == '_' || r == '/' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// tokens derives candidate model tokens from the observations: each
// alphanumeric word, each pair of adjacent words joined ("IM" "C3010" ->
// "IMC3010"), and the trailing digit run of long alpha prefixes
// ("STUDIO6529A" -> "6529A"). Order is kept so the first hit is the leftmost.
func tokens(texts ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		if len(t) >= 3 && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, text := range texts {
		words := strings.FieldsFunc(strings.ToUpper(text), func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
		})
		for i, w := range words {
			add(w)
			if i+1 < len(words) {
				add(w + words[i+1])
			}
			// long alpha prefix glued to a model number: e-STUDIO6529A, TASKalfa2554ci
			j := 0
			for j < len(w) && w[j] >= 'A' && w[j] <= 'Z' {
				j++
			}
			if j >= 3 && j < len(w) {
				add(w[j:])
			}
		}
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// score levels for a token/key pair; higher is better. 0 = no match.
const (
	scoreNone   = 0
	scoreFamily = 1 // generic prefix ("E877" for E87740), wildcard, or short-stem
	scoreExact  = 2 // identifier equal, equal plus a letter-only bundle suffix, or within an explicit range
)

// matchKey scores one observation token against one catalogue key.
func matchKey(tok string, mk modelKey) int {
	if tok == mk.key {
		return scoreExact
	}
	// Explicit range "M282-M285": token "M283" or "M283FDW".
	if mk.rangePrefix != "" || mk.rangeHi > 0 {
		if strings.HasPrefix(tok, mk.rangePrefix) {
			rest := tok[len(mk.rangePrefix):]
			digits := rest
			for i, r := range rest {
				if r < '0' || r > '9' {
					digits = rest[:i]
					break
				}
			}
			if n, err := strconv.Atoi(digits); err == nil && len(digits) == len(strconv.Itoa(mk.rangeLo)) && n >= mk.rangeLo && n <= mk.rangeHi {
				if suffix := rest[len(digits):]; suffix == "" || (isLetters(suffix) && len(suffix) <= 4) {
					return scoreExact
				}
			}
		}
		return scoreNone
	}
	// Wildcard "M28x": token "M283FDW" (the char after the stem must be a digit).
	if mk.wildcardStem != "" && strings.HasPrefix(tok, mk.wildcardStem) && len(tok) > len(mk.wildcardStem) && tok[len(mk.wildcardStem)] >= '0' && tok[len(mk.wildcardStem)] <= '9' {
		return scoreFamily
	}
	// Base identifier plus a letter-only bundle suffix: E87770Z, C3730I, CX730DE.
	// Purely numeric identifiers (Canon "4751", HP "5700") still need a letter suffix, never more digits.
	if strings.HasPrefix(tok, mk.key) {
		suffix := tok[len(mk.key):]
		if isLetters(suffix) && len(suffix) <= 4 {
			return scoreExact
		}
		return scoreNone
	}
	// Generic identity shorter than the base id: "E877" / "E877DN" for E87740 (remainder <= 2 chars).
	stem := strings.TrimRight(tok, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if len(stem) >= 4 && !isDigits(stem) && strings.HasPrefix(mk.key, stem) && len(mk.key)-len(stem) <= 2 {
		return scoreFamily
	}
	return scoreNone
}

// Match finds the best catalogue family for a device's observations. It never
// guesses across brands: an unknown make with an unknown enterprise number is
// "none", a known brand with no family containing the model is "brand-only".
func (c *Catalogue) Match(makeStr, model, sysDescr string, enterpriseID int) (*Match, bool) {
	m := &Match{Confidence: ConfidenceNone, Evidence: []OperationEvidence{}, Observations: Observations{Make: makeStr, Model: model, SysDescr: sysDescr, EnterpriseID: enterpriseID}}
	if b, ok := BrandFromMake(makeStr); ok {
		m.Brand, m.BrandBasis = b, "make"
	} else if b, ok := BrandFromEnterprise(enterpriseID); ok {
		m.Brand, m.BrandBasis = b, "enterprise"
	} else if b, ok := brandInText(model + " " + sysDescr); ok {
		m.Brand, m.BrandBasis = b, "text"
	}
	if m.Brand == "" {
		m.Reason = "make and enterprise number are not in the catalogue"
		return m, false
	}
	m.Confidence = ConfidenceBrandOnly
	m.Reason = "brand resolved from " + m.BrandBasis + "; no catalogue family contains this model"

	type hit struct {
		familyID string
		score    int
		ids      int // number of distinct base ids matched
		modelID  string
		token    string
		order    int // leftmost token index
	}
	hits := map[string]*hit{}
	toks := tokens(model, sysDescr)
	for _, mk := range c.brandKeys[m.Brand] {
		counted := false
		for i, tok := range toks {
			sc := matchKey(tok, mk)
			if sc == scoreNone {
				continue
			}
			h := hits[mk.familyID]
			if h == nil {
				h = &hit{familyID: mk.familyID, order: i}
				hits[mk.familyID] = h
			}
			if !counted {
				h.ids++
				counted = true
			}
			if sc > h.score || (sc == h.score && i < h.order) {
				h.score, h.modelID, h.token, h.order = sc, mk.raw, tok, i
			}
		}
	}
	if len(hits) == 0 {
		return m, false
	}
	list := make([]*hit, 0, len(hits))
	for _, h := range hits {
		list = append(list, h)
	}
	prefixBonus := func(h *hit) int {
		f := c.byID[h.familyID]
		if f.ModelPrefix != "" && strings.Contains(normalise(model+" "+sysDescr), normalise(f.ModelPrefix)) {
			return 1
		}
		return 0
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.order != b.order {
			return a.order < b.order
		}
		if pa, pb := prefixBonus(a), prefixBonus(b); pa != pb {
			return pa > pb
		}
		if a.ids != b.ids {
			return a.ids > b.ids
		}
		return a.familyID < b.familyID
	})
	best := list[0]
	tied := false
	for _, h := range list[1:] {
		if h.score == best.score && h.order == best.order && prefixBonus(h) == prefixBonus(best) && h.ids == best.ids {
			m.Candidates = append(m.Candidates, h.familyID)
			tied = true
		}
	}
	f := c.byID[best.familyID]
	m.FamilyID, m.Family = f.ID, f
	m.ProfileID = f.ManagementProfile
	if p, ok := c.Profile(f.ManagementProfile); ok {
		m.Profile = p
	}
	m.ModelID, m.MatchedToken = best.modelID, best.token
	m.Evidence = c.EvidenceForModel(m.Brand, best.modelID)
	if m.Evidence == nil {
		m.Evidence = []OperationEvidence{}
	}
	switch {
	case tied:
		m.Confidence = ConfidenceFamily
		m.Reason = "several families match equally (" + strings.Join(append([]string{f.ID}, m.Candidates...), ", ") + "); the first is shown"
	case best.score == scoreExact:
		m.Confidence = ConfidenceExact
		m.Reason = "base identifier " + best.modelID + " found as " + best.token
	default:
		m.Confidence = ConfidenceFamily
		m.Reason = "generic identity " + best.token + " maps to " + best.modelID + " in " + f.ID + "; exact base identifier not observed"
	}
	return m, true
}
