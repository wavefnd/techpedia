package storage

import (
	"strings"
	"testing"

	"techpedia/internal/document"
)

func TestDocumentEditPolicyRoundTripsThroughXML(t *testing.T) {
	data, err := marshalDocumentXML(document.Document{
		ID:         "W12",
		Locale:     "ko",
		Slug:       "Protected",
		Title:      "Protected",
		EditPolicy: document.EditPolicyAdmins,
	})
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	if !strings.Contains(string(data), "<edit_policy>admins</edit_policy>") {
		t.Fatalf("XML omitted edit policy: %s", data)
	}
	if !strings.Contains(string(data), `version="3"`) || !strings.Contains(string(data), `<wid>W12</wid>`) {
		t.Fatalf("protected document did not use XML v3 identity: %s", data)
	}

	decoded, err := unmarshalDocumentXML(data)
	if err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	if decoded.EditPolicy != document.EditPolicyAdmins {
		t.Fatalf("unexpected edit policy %q", decoded.EditPolicy)
	}
	if decoded.ID != "W12" {
		t.Fatalf("unexpected WID %q", decoded.ID)
	}
}

func TestLegacyDocumentXMLDefaultsToEditorPolicy(t *testing.T) {
	data := []byte(`<document version="1"><locale>ko</locale><slug>Legacy</slug><title>Legacy</title></document>`)
	decoded, err := unmarshalDocumentXML(data)
	if err != nil {
		t.Fatalf("unmarshal legacy document: %v", err)
	}
	if decoded.EditPolicy != document.EditPolicyEditors {
		t.Fatalf("legacy document policy = %q, want %q", decoded.EditPolicy, document.EditPolicyEditors)
	}
}
