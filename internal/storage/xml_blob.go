package storage

import (
	"encoding/xml"
	"time"

	"techpedia/internal/document"
)

type xmlDocument struct {
	XMLName    xml.Name `xml:"document"`
	Version    string   `xml:"version,attr"`
	WID        string   `xml:"wid,omitempty"`
	Locale     string   `xml:"locale"`
	Group      string   `xml:"group"`
	Slug       string   `xml:"slug"`
	Title      string   `xml:"title"`
	Source     string   `xml:"source"`
	HTML       string   `xml:"html"`
	Author     string   `xml:"author"`
	Summary    string   `xml:"summary,omitempty"`
	EditPolicy string   `xml:"edit_policy,omitempty"`
	Revision   string   `xml:"revision"`
	CreatedAt  string   `xml:"created_at"`
	UpdatedAt  string   `xml:"updated_at"`
}

func marshalDocumentXML(doc document.Document) ([]byte, error) {
	doc = document.NormalizeDocument(doc)
	blob := xmlDocument{
		Version:    "3",
		WID:        doc.ID.String(),
		Locale:     doc.Locale,
		Group:      doc.Group,
		Slug:       doc.Slug,
		Title:      doc.Title,
		Source:     doc.Source,
		HTML:       doc.HTML,
		Author:     doc.Author,
		Summary:    doc.Summary,
		EditPolicy: string(doc.EditPolicy),
		Revision:   doc.Revision,
		CreatedAt:  doc.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:  doc.UpdatedAt.Format(time.RFC3339Nano),
	}

	return xml.MarshalIndent(blob, "", "  ")
}

func unmarshalDocumentXML(data []byte) (document.Document, error) {
	var blob xmlDocument
	if err := xml.Unmarshal(data, &blob); err != nil {
		return document.Document{}, err
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, blob.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, blob.UpdatedAt)
	var wid document.WID
	if blob.WID != "" {
		parsed, err := document.ParseWID(blob.WID)
		if err != nil {
			return document.Document{}, err
		}
		wid = parsed
	}
	locale := document.NormalizeLocale(blob.Locale)
	group := blob.Group
	if group == "" {
		group = blob.Slug
	}

	return document.NormalizeDocument(document.Document{
		ID:         wid,
		Locale:     locale,
		Group:      group,
		Slug:       blob.Slug,
		Title:      blob.Title,
		Source:     blob.Source,
		HTML:       blob.HTML,
		Author:     blob.Author,
		Summary:    blob.Summary,
		EditPolicy: document.EditPolicy(blob.EditPolicy),
		Revision:   blob.Revision,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}), nil
}
