//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package arbitrary

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
	"github.com/miekg/dns"
	"io"
	"os"
	"strconv"
	"strings"
)

type Arbitrary struct {
	tcMatcher map[tc]*domain.MixMatcher
}

// tc is a combination of dns type and class.
type tc struct {
	t uint16
	c uint16
}

func NewArbitrary() *Arbitrary {
	return &Arbitrary{tcMatcher: make(map[tc]*domain.MixMatcher)}
}

func (a *Arbitrary) Match(q *dns.Question) []RR {
	domainMatcher := a.tcMatcher[tc{t: q.Qtype, c: q.Qclass}]
	if domainMatcher == nil {
		return nil
	}

	v, ok := domainMatcher.Match(q.Name)
	if !ok {
		return nil
	}
	return v.(*appendableRR).rrs
}

var errInvalidRecordLength = errors.New("invalid record length")

func (a *Arbitrary) BatchLoad(ss []string) error {
	for _, s := range ss {
		if strings.HasPrefix(s, "ext:") {
			if err := a.LoadFromFile(s[4:]); err != nil {
				return err
			}
		} else {
			if err := a.LoadFromText(s); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Arbitrary) LoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	err = a.LoadFromReader(f)
	if err != nil {
		return fmt.Errorf("failed to load from file: %w", err)
	}
	return nil
}

func (a *Arbitrary) LoadFromReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	line := 0
	for scanner.Scan() {
		line++
		s := scanner.Text()
		s = utils.RemoveComment(s, "#")
		if len(s) == 0 {
			continue
		}

		if err := a.LoadFromText(s); err != nil {
			return fmt.Errorf("invalid record ar line #%d: %w", line, err)
		}
	}
	return scanner.Err()
}

// LoadFromText
// Text format: pattern qclass qtype section rr...
func (a *Arbitrary) LoadFromText(s string) error {
	ss := utils.SplitLine(s)
	if len(ss) < 4 {
		return errInvalidRecordLength
	}

	pattern := ss[0]
	qclass := ss[1]
	qtype := ss[2]
	section := ss[3]
	rs := ss[4:]

	tc := tc{}
	if c, ok := parseClass(qclass); ok {
		tc.c = c
	} else {
		return fmt.Errorf("invalid qclass: %s", qclass)
	}
	if t, ok := parseType(qtype); ok {
		tc.t = t
	} else {
		return fmt.Errorf("invalid qtype: %s", qtype)
	}

	sec, ok := parseSection(section)
	if !ok {
		return fmt.Errorf("invalid section: %s", qtype)
	}

	rr, err := dns.NewRR(strings.Join(rs, " "))
	if err != nil {
		return fmt.Errorf("invalid rr: %w", err)
	}

	domainMatcher := a.tcMatcher[tc]
	if domainMatcher == nil { // lazy init
		domainMatcher = domain.NewMixMatcher()
		domainMatcher.SetPattenTypeMap(domain.MixMatcherStrToPatternTypeDefaultFull)
		a.tcMatcher[tc] = domainMatcher
	}

	arr := &appendableRR{rrs: []RR{{Section: sec, RR: rr}}}

	if err := domainMatcher.Add(pattern, arr); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	return nil
}

type appendableRR struct {
	rrs []RR
}

type Section uint8

const (
	SectionAnswer Section = iota
	SectionNs     Section = iota
	SectionExtra  Section = iota
)

type RR struct {
	Section
	RR dns.RR
}

func NewMsgFromRR(rrs []RR) *dns.Msg {
	m := new(dns.Msg)
	for _, rr := range rrs {
		cp := dns.Copy(rr.RR)
		switch rr.Section {
		case SectionAnswer:
			m.Answer = append(m.Answer, cp)
		case SectionNs:
			m.Ns = append(m.Ns, cp)
		case SectionExtra:
			m.Extra = append(m.Extra, cp)
		}
	}
	return m
}

var strToSection = map[string]Section{
	"ANSWER": SectionAnswer,
	"NS":     SectionNs,
	"EXTRA":  SectionExtra,
}

func (a *appendableRR) Append(v interface{}) {
	newRR, ok := v.(*appendableRR)
	if !ok {
		return
	}
	a.rrs = append(a.rrs, newRR.rrs...)
}

func parseSection(s string) (Section, bool) {
	sec, ok := strToSection[s]
	return sec, ok
}

func parseType(s string) (uint16, bool) {
	if u, ok := dns.StringToType[s]; ok {
		return u, true
	}
	return strToUint16(s)
}

func parseClass(s string) (uint16, bool) {
	if u, ok := dns.StringToClass[s]; ok {
		return u, true
	}
	return strToUint16(s)
}

func strToUint16(s string) (uint16, bool) {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	if i < 0 || i > int(^uint16(0)) {
		return 0, false
	}
	return uint16(i), true
}
