// Package okfproto bridges binder's in-memory okf.Bundle / okf.Concept domain
// model and the vendor-neutral github.com/ghchinoy/okf-proto Protobuf wire
// representation (okf.v1.Bundle) plus FileDescriptorSet ingestion.
//
// It keeps okf.Bundle and pkg/binder free of protobuf types while giving Go SDK
// callers a zero-boilerplate bridge between binder's capabilities (validate,
// review, graph, project, enrich) and okf-proto's binary (.binpb), ProtoJSON,
// and FileDescriptorSet pipelines.
package okfproto

import (
	"fmt"
	"sort"
	"strings"
	"time"

	okfv1pb "github.com/ghchinoy/okf-proto/gen/go/okf/v1"
	protobundle "github.com/ghchinoy/okf-proto/pkg/bundle"
	"github.com/ghchinoy/okf-proto/pkg/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/ghchinoy/binder/internal/okfrules"
	"github.com/ghchinoy/binder/pkg/okf"
)

// ToProto converts a binder *okf.Bundle into a canonical *okfv1pb.Bundle
// Protobuf message. today (YYYY-MM-DD) is used to populate DerivedTrust.stale.
func ToProto(b *okf.Bundle, today string) (*okfv1pb.Bundle, error) {
	if b == nil {
		return nil, fmt.Errorf("okfproto: nil okf.Bundle")
	}
	ver := string(b.OKFVersion)
	if ver == "" {
		ver = string(okf.DefaultSpecVersion)
	}
	out := &okfv1pb.Bundle{
		OkfVersion: ver,
	}

	concepts := append([]*okf.Concept(nil), b.Concepts...)
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].ID < concepts[j].ID })

	for _, c := range concepts {
		pc, err := ConceptToProto(c, today)
		if err != nil {
			return nil, fmt.Errorf("okfproto: concept %s: %w", c.ID, err)
		}
		out.Concepts = append(out.Concepts, pc)
	}
	return out, nil
}

// ConceptToProto converts a single binder *okf.Concept into an *okfv1pb.Concept.
func ConceptToProto(c *okf.Concept, today string) (*okfv1pb.Concept, error) {
	if c == nil {
		return nil, fmt.Errorf("okfproto: nil okf.Concept")
	}
	fmMap := map[string]any{}
	var keyOrder []string
	if c.Frontmatter != nil {
		keyOrder = c.Frontmatter.Keys()
		for _, k := range keyOrder {
			v, _ := c.Frontmatter.Get(k)
			fmMap[k] = v
		}
	}
	st, err := structpb.NewStruct(fmMap)
	if err != nil {
		return nil, fmt.Errorf("building frontmatter Struct: %w", err)
	}

	var gen *okfv1pb.Actorstamp
	if c.Trust.Generated != nil {
		gen = &okfv1pb.Actorstamp{
			By: c.Trust.Generated.By,
			At: c.Trust.Generated.At,
		}
	}
	var verified []*okfv1pb.Actorstamp
	for _, v := range c.Trust.Verified {
		verified = append(verified, &okfv1pb.Actorstamp{By: v.By, At: v.At})
	}
	var sources []*okfv1pb.Source
	for _, s := range c.Trust.Sources {
		sources = append(sources, &okfv1pb.Source{
			Id:           s.ID,
			Resource:     s.Resource,
			Title:        s.Title,
			Author:       s.Author,
			UsageCount:   s.UsageCount,
			LastModified: s.LastModified,
		})
	}
	var uw *okfv1pb.DateRange
	if c.Trust.UsageWindow != nil {
		uw = &okfv1pb.DateRange{
			From: c.Trust.UsageWindow.From,
			To:   c.Trust.UsageWindow.To,
		}
	}
	var links []*okfv1pb.Link
	for _, l := range c.Links {
		links = append(links, &okfv1pb.Link{
			RawTarget: l.RawTarget,
			TargetId:  l.TargetID,
			Text:      l.Text,
			Resolved:  l.Resolved,
		})
	}

	var tags []string
	if c.Frontmatter != nil {
		if rawTags, ok := c.Frontmatter.Get("tags"); ok {
			switch tv := rawTags.(type) {
			case []any:
				for _, item := range tv {
					if s := strings.TrimSpace(okfrules.AsString(item)); s != "" {
						tags = append(tags, s)
					}
				}
			case []string:
				tags = append(tags, tv...)
			}
		}
	}

	getStr := func(key string) string {
		if c.Frontmatter == nil {
			return ""
		}
		v, _ := c.Frontmatter.Get(key)
		return okfrules.AsString(v)
	}

	rel := c.RelPath
	if rel == "" && c.ID != "" {
		rel = c.ID + ".md"
	}

	var origFM []byte
	if len(c.OriginalFrontmatter) > 0 {
		origFM = append([]byte(nil), c.OriginalFrontmatter...)
	}

	return &okfv1pb.Concept{
		Id:          c.ID,
		RelPath:     rel,
		Type:        c.Type,
		Title:       getStr("title"),
		Description: getStr("description"),
		Resource:    getStr("resource"),
		Tags:        tags,
		Sources:     sources,
		UsageWindow: uw,
		Generated:   gen,
		Verified:    verified,
		Status:      c.Trust.Status,
		StaleAfter:  c.Trust.StaleAfter,
		Derived: &okfv1pb.DerivedTrust{
			Tier:     string(okfrules.TrustTier(c)),
			Stale:    okfrules.IsStale(c, today),
			Attested: c.Trust.Attested,
		},
		Links:               links,
		Body:                c.Body,
		Frontmatter:         st,
		FrontmatterKeyOrder: keyOrder,
		OriginalFrontmatter: origFM,
	}, nil
}

// FromProto converts an *okfv1pb.Bundle into a binder *okf.Bundle.
func FromProto(pb *okfv1pb.Bundle, root string) (*okf.Bundle, error) {
	if pb == nil {
		return nil, fmt.Errorf("okfproto: nil okfv1pb.Bundle")
	}
	ver := okf.SpecVersion(pb.GetOkfVersion())
	if ver == "" {
		ver = okf.DefaultSpecVersion
	}
	b := &okf.Bundle{
		Root:       root,
		OKFVersion: ver,
	}
	for _, pc := range pb.GetConcepts() {
		c := ConceptFromProto(pc)
		b.Concepts = append(b.Concepts, c)
	}
	sort.Slice(b.Concepts, func(i, j int) bool { return b.Concepts[i].RelPath < b.Concepts[j].RelPath })
	return b, nil
}

// ConceptFromProto converts an *okfv1pb.Concept into a binder *okf.Concept,
// preserving frontmatter key order, OriginalFrontmatter bytes, and re-projecting
// TrustSignals via binder's authoritative okfrules.ProjectTrust rule.
func ConceptFromProto(pc *okfv1pb.Concept) *okf.Concept {
	fm := okfrules.NewOrderedMap()
	rawMap := map[string]any{}
	if pc.GetFrontmatter() != nil {
		rawMap = pc.GetFrontmatter().AsMap()
	}
	seen := map[string]bool{}
	for _, k := range pc.GetFrontmatterKeyOrder() {
		if v, ok := rawMap[k]; ok {
			fm.Set(k, v)
			seen[k] = true
		}
	}
	var extra []string
	for k := range rawMap {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		fm.Set(k, rawMap[k])
	}

	// Ensure typed top-level scalars are present if Frontmatter Struct was omitted.
	setIfAbsent := func(k, v string) {
		if v != "" && !fm.Has(k) {
			fm.Set(k, v)
		}
	}
	setIfAbsent("type", pc.GetType())
	setIfAbsent("title", pc.GetTitle())
	setIfAbsent("description", pc.GetDescription())
	setIfAbsent("resource", pc.GetResource())
	setIfAbsent("status", pc.GetStatus())
	setIfAbsent("stale_after", pc.GetStaleAfter())

	typ := pc.GetType()
	if typ == "" {
		typ = okfrules.AsString(rawMap["type"])
	}
	rel := pc.GetRelPath()
	if rel == "" && pc.GetId() != "" {
		rel = pc.GetId() + ".md"
	}

	var links []okf.Link
	for _, l := range pc.GetLinks() {
		links = append(links, okf.Link{
			RawTarget: l.GetRawTarget(),
			TargetID:  l.GetTargetId(),
			Text:      l.GetText(),
			Resolved:  l.GetResolved(),
		})
	}

	var origFM []byte
	if len(pc.GetOriginalFrontmatter()) > 0 {
		origFM = append([]byte(nil), pc.GetOriginalFrontmatter()...)
	}

	return &okf.Concept{
		ID:                  pc.GetId(),
		RelPath:             rel,
		Type:                typ,
		Frontmatter:         fm,
		Body:                pc.GetBody(),
		OriginalFrontmatter: origFM,
		Links:               links,
		Trust:               okfrules.ProjectTrust(fm, typ),
	}
}

// FromDescriptorSet converts a compiled Protobuf FileDescriptorSet directly into
// a binder *okf.Bundle (plus the full *okfv1pb.Bundle carrying reserved index.md
// files), ready for `graph.Build`, `graph.Project`, `review.Review`, or writing
// to disk via WriteBundle.
func FromDescriptorSet(fds *descriptorpb.FileDescriptorSet, root string, now time.Time, producer string) (*okf.Bundle, *okfv1pb.Bundle, error) {
	pb, err := descriptor.FromDescriptorSet(fds, descriptor.Options{
		Producer: producer,
		Now:      now,
	})
	if err != nil {
		return nil, nil, err
	}
	b, err := FromProto(pb, root)
	if err != nil {
		return nil, nil, err
	}
	return b, pb, nil
}

// WriteBundle writes an *okfv1pb.Bundle to outDir using okf-proto's importer.
func WriteBundle(pb *okfv1pb.Bundle, outDir string) error {
	return protobundle.Import(pb, outDir)
}
