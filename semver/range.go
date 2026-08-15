package semver

import (
	"fmt"
	"strings"
)

// op is a primitive comparison operator applied to a comparator's version.
type op int

const (
	opEQ op = iota
	opLT
	opLE
	opGT
	opGE
)

// comparator is a single primitive version constraint such as ">=1.2.3".
type comparator struct {
	op      op
	version Version
}

// Range is a version range: a disjunction of comparator sets, where each set
// is a conjunction (AND) of comparators.
type Range struct {
	sets [][]comparator
}

// ParseRange parses a range expression. The empty string is equivalent to "*".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "*"
	}
	r := Range{}
	for _, part := range strings.Split(s, "||") {
		comps, err := parseComparatorSet(strings.TrimSpace(part))
		if err != nil {
			return r, err
		}
		r.sets = append(r.sets, comps)
	}
	return r, nil
}

func parseComparatorSet(s string) ([]comparator, error) {
	if s == "" {
		s = "*"
	}
	tokens := strings.Fields(s)
	var comps []comparator
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "-" {
			return nil, fmt.Errorf("invalid hyphen range")
		}
		if i+2 < len(tokens) && tokens[i+1] == "-" {
			c, err := hyphenRange(tokens[i], tokens[i+2])
			if err != nil {
				return nil, err
			}
			comps = append(comps, c...)
			i += 2
			continue
		}
		c, err := parseComparator(tok)
		if err != nil {
			return nil, err
		}
		comps = append(comps, c...)
	}
	if len(comps) == 0 {
		comps = []comparator{{op: opGE, version: Version{}}}
	}
	return comps, nil
}

func parseComparator(tok string) ([]comparator, error) {
	switch tok {
	case "", "*", "x", "X":
		return []comparator{{op: opGE, version: Version{}}}, nil
	}
	if strings.HasPrefix(tok, "^") {
		return parseCaret(tok[1:])
	}
	if strings.HasPrefix(tok, "~") {
		return parseTilde(tok[1:])
	}
	for _, opStr := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(tok, opStr) {
			return parseOperator(opStr, tok[len(opStr):])
		}
	}
	return parseBare(tok)
}

func parseOperator(opStr, rest string) ([]comparator, error) {
	p, err := parsePartial(rest)
	if err != nil {
		return nil, err
	}
	full := p.majorSet && p.minorSet && p.patchSet
	switch opStr {
	case ">=":
		v := partialVersion(p)
		if !p.majorSet {
			v = Version{}
		}
		return []comparator{{op: opGE, version: v}}, nil
	case "<=":
		if !p.majorSet {
			return []comparator{{op: opGE, version: Version{}}}, nil
		}
		if full {
			return []comparator{{op: opLE, version: partialVersion(p)}}, nil
		}
		maj, min, pat := incrementPartial(p)
		return []comparator{{op: opLT, version: Version{Major: maj, Minor: min, Patch: pat}}}, nil
	case ">":
		if !p.majorSet {
			return []comparator{{op: opLT, version: Version{}}}, nil // >* matches nothing
		}
		if full {
			return []comparator{{op: opGT, version: partialVersion(p)}}, nil
		}
		maj, min, pat := incrementPartial(p)
		return []comparator{{op: opGE, version: Version{Major: maj, Minor: min, Patch: pat}}}, nil
	case "<":
		if !p.majorSet {
			return []comparator{{op: opLT, version: Version{}}}, nil
		}
		return []comparator{{op: opLT, version: partialVersion(p)}}, nil
	case "=":
		return parseBareFromPartial(p), nil
	}
	return nil, fmt.Errorf("unknown operator %q", opStr)
}

func parseBare(tok string) ([]comparator, error) {
	p, err := parsePartial(tok)
	if err != nil {
		return nil, err
	}
	return parseBareFromPartial(p), nil
}

func parseBareFromPartial(p partial) []comparator {
	switch {
	case !p.majorSet:
		return []comparator{{op: opGE, version: Version{}}}
	case p.majorSet && p.minorSet && p.patchSet:
		return []comparator{{op: opEQ, version: partialVersion(p)}}
	case !p.minorSet:
		return []comparator{
			{op: opGE, version: Version{Major: p.major}},
			{op: opLT, version: Version{Major: p.major + 1}},
		}
	default: // minor set, patch wild
		return []comparator{
			{op: opGE, version: Version{Major: p.major, Minor: p.minor}},
			{op: opLT, version: Version{Major: p.major, Minor: p.minor + 1}},
		}
	}
}

func parseCaret(rest string) ([]comparator, error) {
	p, err := parsePartial(rest)
	if err != nil {
		return nil, err
	}
	lo := partialVersion(p)
	switch {
	case !p.majorSet:
		return []comparator{{op: opGE, version: Version{}}}, nil
	case p.major > 0:
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Major: p.major + 1}},
		}, nil
	case !p.minorSet: // ^0
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Major: 1}},
		}, nil
	case p.minor > 0: // ^0.2.x
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Minor: p.minor + 1}},
		}, nil
	case !p.patchSet: // ^0.0
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Minor: 1}},
		}, nil
	default: // ^0.0.x
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Patch: p.patch + 1}},
		}, nil
	}
}

func parseTilde(rest string) ([]comparator, error) {
	p, err := parsePartial(rest)
	if err != nil {
		return nil, err
	}
	lo := partialVersion(p)
	switch {
	case !p.majorSet:
		return []comparator{{op: opGE, version: Version{}}}, nil
	case !p.minorSet: // ~1
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Major: p.major + 1}},
		}, nil
	default: // ~1.2 or ~1.2.3
		return []comparator{
			{op: opGE, version: lo},
			{op: opLT, version: Version{Major: p.major, Minor: p.minor + 1}},
		}, nil
	}
}

func hyphenRange(loStr, hiStr string) ([]comparator, error) {
	lo, err := parsePartial(loStr)
	if err != nil {
		return nil, err
	}
	hi, err := parsePartial(hiStr)
	if err != nil {
		return nil, err
	}
	var comps []comparator
	if lo.majorSet {
		comps = append(comps, comparator{op: opGE, version: partialVersion(lo)})
	} else {
		comps = append(comps, comparator{op: opGE, version: Version{}})
	}
	switch {
	case !hi.majorSet: // no upper bound
	case hi.majorSet && hi.minorSet && hi.patchSet:
		comps = append(comps, comparator{op: opLE, version: partialVersion(hi)})
	case hi.minorSet:
		comps = append(comps, comparator{op: opLT, version: Version{Major: hi.major, Minor: hi.minor + 1}})
	default: // only major
		comps = append(comps, comparator{op: opLT, version: Version{Major: hi.major + 1}})
	}
	return comps, nil
}

// partial is a version that may omit components or use wildcards (x/X/*),
// used while parsing range comparators.
type partial struct {
	majorSet bool
	major    uint64
	minorSet bool
	minor    uint64
	patchSet bool
	patch    uint64
	pre      []string
}

func parsePartial(s string) (partial, error) {
	var p partial
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i] // build metadata is ignored in ranges
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		preStr := s[i+1:]
		s = s[:i]
		ids := strings.Split(preStr, ".")
		for _, id := range ids {
			if !isValidIdentifier(id, true) {
				return p, fmt.Errorf("invalid prerelease identifier %q", id)
			}
		}
		p.pre = ids
	}
	if s == "" || s == "*" {
		return p, nil
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return p, fmt.Errorf("too many version parts in %q", s)
	}
	for i, part := range parts {
		if part == "" || part == "x" || part == "X" || part == "*" {
			break
		}
		n, err := parseCorePart(part)
		if err != nil {
			return p, fmt.Errorf("invalid version part %q: %w", part, err)
		}
		switch i {
		case 0:
			p.majorSet = true
			p.major = n
		case 1:
			p.minorSet = true
			p.minor = n
		case 2:
			p.patchSet = true
			p.patch = n
		}
	}
	return p, nil
}

// incrementPartial increments the last explicitly-specified component, zeroing
// the rest. Used to turn ">" / "<=" on a partial into a closed lower/upper bound.
func incrementPartial(p partial) (uint64, uint64, uint64) {
	switch {
	case !p.minorSet:
		return p.major + 1, 0, 0
	case !p.patchSet:
		return p.major, p.minor + 1, 0
	default:
		return p.major, p.minor, p.patch + 1
	}
}

func partialVersion(p partial) Version {
	return Version{Major: p.major, Minor: p.minor, Patch: p.patch, Prerelease: p.pre}
}

// Satisfies reports whether v satisfies the range.
func (r Range) Satisfies(v Version) bool {
	for _, set := range r.sets {
		if matchesSet(v, set) {
			return true
		}
	}
	return false
}

func matchesSet(v Version, comps []comparator) bool {
	for _, c := range comps {
		if !c.matches(v) {
			return false
		}
	}
	// Prerelease filtering: a prerelease version only satisfies a set when at
	// least one comparator in the set targets the same major.minor.patch and
	// itself carries a prerelease tag.
	if len(v.Prerelease) > 0 {
		ok := false
		for _, c := range comps {
			if len(c.version.Prerelease) > 0 &&
				c.version.Major == v.Major &&
				c.version.Minor == v.Minor {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (c comparator) matches(v Version) bool {
	switch c.op {
	case opEQ:
		return v.Compare(c.version) == 0
	case opLT:
		return v.Compare(c.version) < 0
	case opLE:
		return v.Compare(c.version) <= 0
	case opGT:
		return v.Compare(c.version) > 0
	case opGE:
		return v.Compare(c.version) >= 0
	}
	return false
}
