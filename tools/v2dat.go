/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package tools

import (
	"bufio"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/netlist"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/v2data"
	"github.com/spf13/cobra"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

func newUnpackDomainCmd() *cobra.Command {
	var ourDir string
	c := &cobra.Command{
		Use:   "unpack-domain [-o output_dir] geosite.dat[:tag1[,tag2]...]",
		Args:  cobra.ExactArgs(1),
		Short: "Unpack v2ray domain data file to text files.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := UnpackDomainDAT(args[0], ourDir); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
	c.Flags().StringVarP(&ourDir, "out", "o", "", "output dir")
	return c
}

func newUnpackIPCmd() *cobra.Command {
	var ourDir string
	c := &cobra.Command{
		Use:   "unpack-ip [-o output_dir] geoip.dat[:tag1[,tag2]...]",
		Args:  cobra.ExactArgs(1),
		Short: "Unpack v2ray ip data file to text files.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := UnpackIPDAT(args[0], ourDir); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
	c.Flags().StringVarP(&ourDir, "out", "o", "", "output dir")
	return c
}

func splitTags(s string) (string, []string) {
	file, tags, ok := strings.Cut(s, ":")
	if ok {
		return file, strings.Split(tags, ",")
	}
	return s, nil
}

func splitAttrs(s string) (string, map[string]struct{}) {
	tag, attrs, ok := strings.Cut(s, "@")
	if ok {
		m := make(map[string]struct{})
		for _, attr := range strings.Split(attrs, "@") {
			m[attr] = struct{}{}
		}
		return tag, m
	}
	return s, nil
}

// filterAttrs filter entries that do not have any of given attrs.
// If no attr was given, filterAttrs returns in.
func filterAttrs(in []*v2data.Domain, attrs map[string]struct{}) []*v2data.Domain {
	if len(attrs) == 0 {
		return in
	}
	out := make([]*v2data.Domain, 0)
	for _, d := range in {
		hasAttr := false
		for _, attr := range d.Attribute {
			if _, ok := attrs[attr.Key]; ok {
				hasAttr = true
				break
			}
		}
		if !hasAttr {
			continue
		}
		out = append(out, d)
	}
	return out
}

func UnpackDomainDAT(in, outDir string) error {
	filePath, suffixes := splitTags(in)
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	geoSiteList, err := domain.LoadGeoSiteList(b)
	if err != nil {
		return err
	}

	entries := make(map[string][]*v2data.Domain)
	for _, geoSite := range geoSiteList.GetEntry() {
		tag := strings.ToLower(geoSite.GetCountryCode())
		entries[tag] = geoSite.GetDomain()
	}

	save := func(suffix string, data []*v2data.Domain) error {
		file := fmt.Sprintf("%s_%s.txt", fileName(in), suffix)
		if len(outDir) > 0 {
			file = filepath.Join(outDir, file)
		}
		mlog.S().Infof("writing data %s to %s", suffix, file)
		return convertV2DomainToTextFile(data, file)
	}

	if len(suffixes) > 0 {
		for _, suffix := range suffixes {
			tag, attrs := splitAttrs(suffix)
			entry, ok := entries[tag]
			if !ok {
				return fmt.Errorf("cannot find entry %s", tag)
			}
			entry = filterAttrs(entry, attrs)
			if err := save(suffix, entry); err != nil {
				return fmt.Errorf("failed to save %s, %w", suffix, err)
			}
		}
	} else { // If tag is omitted, unpack all tags.
		for tag, domains := range entries {
			if err := save(tag, domains); err != nil {
				return fmt.Errorf("failed to save %s, %w", tag, err)
			}
		}
	}
	return nil
}

func fileName(f string) string {
	f = filepath.Base(f)
	if i := strings.LastIndexByte(f, '.'); i == -1 {
		return f
	} else {
		return f[:i]
	}
}

func convertV2DomainToTextFile(domain []*v2data.Domain, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return convertV2DomainToText(domain, f)
}

func convertV2DomainToText(domain []*v2data.Domain, w io.Writer) error {
	bw := bufio.NewWriter(w)
	for _, r := range domain {
		var prefix string
		switch r.Type {
		case v2data.Domain_Plain:
			prefix = "keyword:"
		case v2data.Domain_Regex:
			prefix = "regexp:"
		case v2data.Domain_Domain:
			prefix = ""
		case v2data.Domain_Full:
			prefix = "full:"
		default:
			return fmt.Errorf("invalid domain type %d", r.Type)
		}
		if _, err := bw.WriteString(prefix); err != nil {
			return err
		}
		if _, err := bw.WriteString(r.Value); err != nil {
			return err
		}
		if _, err := bw.WriteRune('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func UnpackIPDAT(in, ourDir string) error {
	filePath, wantTags := splitTags(in)
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	geoIPList, err := netlist.LoadGeoIPListFromDAT(b)
	if err != nil {
		return err
	}

	entries := make(map[string]*v2data.GeoIP)
	var wantEntries map[string]*v2data.GeoIP
	for _, geoSite := range geoIPList.GetEntry() {
		tag := strings.ToLower(geoSite.GetCountryCode())
		entries[tag] = geoSite
	}

	if len(wantTags) > 0 {
		wantEntries = make(map[string]*v2data.GeoIP)
		for _, tag := range wantTags {
			entry, ok := entries[tag]
			if !ok {
				return fmt.Errorf("cannot find entry %s", tag)
			}
			wantEntries[tag] = entry
		}
	} else {
		wantEntries = entries
	}

	for tag, ipList := range wantEntries {
		file := fmt.Sprintf("%s_%s.txt", fileName(in), tag)
		if len(ourDir) > 0 {
			file = filepath.Join(ourDir, file)
		}
		mlog.S().Infof("saving %s ip to %s", tag, file)
		err := convertV2CidrToTextFile(ipList.GetCidr(), file)
		if err != nil {
			return err
		}
	}

	return nil
}

func convertV2CidrToTextFile(cidr []*v2data.CIDR, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return convertV2CidrToText(cidr, f)
}

func convertV2CidrToText(cidr []*v2data.CIDR, w io.Writer) error {
	bw := bufio.NewWriter(w)
	for i, record := range cidr {
		ip, ok := netip.AddrFromSlice(record.Ip)
		if !ok {
			return fmt.Errorf("invalid ip at index #%d, %s", i, record.Ip)
		}
		prefix, err := ip.Prefix(int(record.Prefix))
		if !ok {
			return fmt.Errorf("invalid prefix at index #%d, %w", i, err)
		}

		if _, err := bw.WriteString(prefix.String()); err != nil {
			return err
		}
		if _, err := bw.WriteRune('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}
