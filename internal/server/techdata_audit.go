package server

import (
	"net/http"
	"sort"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

type adminTechdataAuditResponse struct {
	Documents          int                              `json:"documents"`
	LinkedDocuments    int                              `json:"linked_documents"`
	UnlinkedDocuments  []adminDocumentSummary           `json:"unlinked_documents"`
	MetadataCandidates []adminTechdataMetadataCandidate `json:"metadata_candidates"`
	NameCandidates     []adminTechdataNameCandidate     `json:"name_candidates"`
}

type adminTechdataMetadataCandidate struct {
	Page          adminDocumentSummary `json:"page"`
	TID           string               `json:"tid"`
	Label         string               `json:"label"`
	LinkedLocales []string             `json:"linked_locales"`
}

type adminTechdataNameCandidate struct {
	Name  string                 `json:"name"`
	Pages []adminDocumentSummary `json:"pages"`
}

type techdataMetadataMatch struct {
	TID           string
	Label         string
	LinkedLocales []string
}

func (s *Server) handleAdminTechdataAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	documents := make([]document.Document, 0)
	for _, value := range editions {
		if !value.Active() {
			continue
		}
		localeDocuments, err := s.adminDocumentsForLocale(r.Context(), value.Tag)
		if err != nil {
			http.Error(w, "failed to load documents", http.StatusInternalServerError)
			return
		}
		for _, doc := range localeDocuments {
			if doc.ID != "" {
				documents = append(documents, doc)
			}
		}
	}
	items, err := s.store.ListItems(r.Context())
	if err != nil {
		http.Error(w, "failed to load techdata items", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, buildTechdataAudit(documents, items))
}

func buildTechdataAudit(documents []document.Document, items []techdata.Item) adminTechdataAuditResponse {
	response := adminTechdataAuditResponse{
		Documents:          len(documents),
		UnlinkedDocuments:  []adminDocumentSummary{},
		MetadataCandidates: []adminTechdataMetadataCandidate{},
		NameCandidates:     []adminTechdataNameCandidate{},
	}
	linked := make(map[document.WID]string)
	metadata := make(map[string]map[string][]techdataMetadataMatch)
	for _, item := range items {
		locales := sortedWikiPageLocales(item.WikiPages)
		for _, wid := range item.WikiPages {
			linked[wid] = item.ID.String()
		}
		metadataLocales := make(map[string]struct{}, len(item.Labels)+len(item.Aliases))
		for locale := range item.Labels {
			metadataLocales[locale] = struct{}{}
		}
		for locale := range item.Aliases {
			metadataLocales[locale] = struct{}{}
		}
		for locale := range metadataLocales {
			if item.WikiPages[locale] != "" {
				continue
			}
			label := item.Labels[locale]
			if label == "" && len(item.Aliases[locale]) > 0 {
				label = item.Aliases[locale][0]
			}
			values := append([]string{label}, item.Aliases[locale]...)
			for _, value := range values {
				key := normalizeAuditName(value)
				if key == "" {
					continue
				}
				if metadata[locale] == nil {
					metadata[locale] = make(map[string][]techdataMetadataMatch)
				}
				metadata[locale][key] = append(metadata[locale][key], techdataMetadataMatch{
					TID: item.ID.String(), Label: label, LinkedLocales: locales,
				})
			}
		}
	}

	nameGroups := make(map[string][]adminDocumentSummary)
	for _, doc := range documents {
		if _, exists := linked[doc.ID]; exists {
			response.LinkedDocuments++
			continue
		}
		page := newAdminDocumentSummary(doc, "")
		response.UnlinkedDocuments = append(response.UnlinkedDocuments, page)
		key := normalizeAuditName(doc.Title)
		nameGroups[key] = append(nameGroups[key], page)
		seenItems := make(map[string]struct{})
		for _, match := range metadata[doc.Locale][key] {
			if _, exists := seenItems[match.TID]; exists {
				continue
			}
			seenItems[match.TID] = struct{}{}
			response.MetadataCandidates = append(response.MetadataCandidates, adminTechdataMetadataCandidate{
				Page: page, TID: match.TID, Label: match.Label, LinkedLocales: match.LinkedLocales,
			})
		}
	}

	for _, pages := range nameGroups {
		locales := make(map[string]struct{})
		for _, page := range pages {
			locales[page.Locale] = struct{}{}
		}
		if len(pages) < 2 || len(locales) < 2 {
			continue
		}
		sort.Slice(pages, func(i, j int) bool {
			return pages[i].Locale < pages[j].Locale || (pages[i].Locale == pages[j].Locale && pages[i].Name < pages[j].Name)
		})
		response.NameCandidates = append(response.NameCandidates, adminTechdataNameCandidate{Name: pages[0].Name, Pages: pages})
	}
	sort.Slice(response.UnlinkedDocuments, func(i, j int) bool {
		return response.UnlinkedDocuments[i].UpdatedAt > response.UnlinkedDocuments[j].UpdatedAt
	})
	sort.Slice(response.MetadataCandidates, func(i, j int) bool {
		left, right := response.MetadataCandidates[i], response.MetadataCandidates[j]
		return left.Page.Name < right.Page.Name || (left.Page.Name == right.Page.Name && left.Page.Locale < right.Page.Locale)
	})
	sort.Slice(response.NameCandidates, func(i, j int) bool { return response.NameCandidates[i].Name < response.NameCandidates[j].Name })
	return response
}

func sortedWikiPageLocales(values map[string]document.WID) []string {
	locales := make([]string, 0, len(values))
	for locale := range values {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return locales
}

func normalizeAuditName(value string) string {
	return techdata.NormalizeIdentityName(value)
}
