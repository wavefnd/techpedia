package wiki

import (
	"context"
	"errors"
	"sort"

	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

type HistoryStore interface {
	GetRevision(context.Context, string, string, string) (document.Document, error)
	ListRevisions(context.Context, string, string, int) ([]document.RevisionSummary, error)
}

var ErrRestrictedSource = errors.New("administrator-only source is not available through MCP")

func (r *Reader) History(ctx context.Context, locale, name string, limit int) ([]document.RevisionSummary, error) {
	doc, err := r.Page(ctx, locale, name)
	if err != nil {
		return nil, err
	}
	if doc.EditPolicy == document.EditPolicyAdmins {
		return nil, ErrRestrictedSource
	}
	s, ok := r.store.(HistoryStore)
	if !ok {
		return nil, errors.New("revision reading unavailable")
	}
	return s.ListRevisions(ctx, doc.Locale, doc.Slug, max(1, min(limit, 100)))
}
func (r *Reader) Revision(ctx context.Context, locale, name, revision string) (document.Document, error) {
	if !changeproposal.Revision(revision) {
		return document.Document{}, changeproposal.ErrInvalid
	}
	current, err := r.Page(ctx, locale, name)
	if err != nil {
		return document.Document{}, err
	}
	if current.EditPolicy == document.EditPolicyAdmins {
		return document.Document{}, ErrRestrictedSource
	}
	s, ok := r.store.(HistoryStore)
	if !ok {
		return document.Document{}, errors.New("revision reading unavailable")
	}
	doc, err := s.GetRevision(ctx, current.Locale, current.Slug, revision)
	if err != nil {
		return doc, err
	}
	if doc.EditPolicy == document.EditPolicyAdmins {
		return document.Document{}, ErrRestrictedSource
	}
	return doc, nil
}

type TranslationStore interface {
	GetItemByWikiPage(context.Context, document.WID) (techdata.Item, error)
	GetLatestByWID(context.Context, document.WID) (document.Document, error)
}

func (r *Reader) Translations(ctx context.Context, locale, name string) ([]document.Document, error) {
	doc, err := r.Page(ctx, locale, name)
	if err != nil {
		return nil, err
	}
	s, ok := r.store.(TranslationStore)
	if !ok {
		return nil, errors.New("translation reading unavailable")
	}
	item, err := s.GetItemByWikiPage(ctx, doc.ID)
	if errors.Is(err, techdata.ErrItemNotFound) {
		return []document.Document{doc}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []document.Document{}
	for locale, wid := range item.WikiPages {
		if _, err := r.ActiveEdition(ctx, locale); err != nil {
			continue
		}
		v, err := s.GetLatestByWID(ctx, wid)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Locale < out[j].Locale })
	return out, nil
}
