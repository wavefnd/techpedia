package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
	"techpedia/internal/translation"
)

type fixedInterfaceTranslator struct {
	err error
}

func (f fixedInterfaceTranslator) TranslateInterface(_ context.Context, _ string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	messages := make(map[string]string, len(translation.InterfaceMessages()))
	for _, seed := range translation.InterfaceMessages() {
		messages[seed.Key] = "translated " + seed.Key
	}
	return messages, nil
}

func TestProposalWaitsForAdministratorApprovalBeforeTranslation(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionEdit})
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	srv.interfaceTranslator = fixedInterfaceTranslator{}

	req := httptest.NewRequest(http.MethodPost, "/api/editor/languages", strings.NewReader(`{"tag":"fr"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("propose edition: %d: %s", rec.Code, rec.Body.String())
	}
	var response editionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode proposal response: %v", err)
	}
	if response.InterfaceTranslationStatus != "" || response.InterfaceTranslationFilled != 0 {
		t.Fatalf("proposal translated before approval: %#v", response)
	}
	stored, err := store.GetEdition(context.Background(), "fr")
	if err != nil || stored.Status != edition.StatusPending || len(stored.InterfaceMessages) != 0 {
		t.Fatalf("unexpected pending proposal: %#v, %v", stored, err)
	}
}

func TestApprovalCreatesEditableInterfaceTranslationDraft(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionAdmin})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "fr", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	srv.interfaceTranslator = fixedInterfaceTranslator{}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/approve", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve edition: %d: %s", rec.Code, rec.Body.String())
	}
	var response editionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if response.Status != edition.StatusApproved || response.InterfaceTranslationStatus != "complete" || response.InterfaceTranslationFilled != len(translation.InterfaceMessages()) {
		t.Fatalf("unexpected approval translation status: %#v", response)
	}
	stored, err := store.GetEdition(context.Background(), "fr")
	if err != nil || stored.InterfaceMessages["menu"] != "translated menu" {
		t.Fatalf("translated draft was not stored: %#v, %v", stored, err)
	}
}

func TestApprovalSurvivesInterfaceTranslationFailure(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionAdmin})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "de", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	srv.interfaceTranslator = fixedInterfaceTranslator{err: translation.ErrTranslationUnavailable}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/languages/de/approve", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"interface_translation_status":"failed"`) {
		t.Fatalf("translation failure blocked edition approval: %d: %s", rec.Code, rec.Body.String())
	}
	stored, err := store.GetEdition(context.Background(), "de")
	if err != nil || stored.Status != edition.StatusApproved {
		t.Fatalf("edition was not approved after translation failure: %#v, %v", stored, err)
	}
}

func TestAdminCanFillOnlyBlankInterfaceTranslations(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionAdmin})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{
		Tag: "fr", Direction: "ltr", Status: edition.StatusPending,
	}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	if _, err := store.ApproveEdition(context.Background(), "fr", "translation_operator", time.Now()); err != nil {
		t.Fatalf("approve edition: %v", err)
	}
	if _, err := store.UpdateInterfaceMessages(context.Background(), "fr", map[string]string{"menu": "Menu personnalisé"}); err != nil {
		t.Fatalf("store administrator translation: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	srv.interfaceTranslator = fixedInterfaceTranslator{}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/interface/translate", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("translate interface: %d: %s", rec.Code, rec.Body.String())
	}
	stored, err := store.GetEdition(context.Background(), "fr")
	if err != nil {
		t.Fatalf("load translated edition: %v", err)
	}
	if stored.InterfaceMessages["menu"] != "Menu personnalisé" || stored.InterfaceMessages["close"] != "translated close" {
		t.Fatalf("automatic translation overwrote an administrator value: %#v", stored.InterfaceMessages)
	}
}

func TestAdminCanRejectPendingLanguageProposal(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionAdmin})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "fr", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/languages/fr", strings.NewReader(`{"confirmation":"fr"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reject edition: %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetEdition(context.Background(), "fr"); !errors.Is(err, edition.ErrNotFound) {
		t.Fatalf("rejected proposal remains stored: %v", err)
	}
}

func TestLanguageProposalReviewRequiresAdministrator(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionEdit})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "fr", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())

	approveReq := httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/approve", nil)
	approveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	approveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusForbidden {
		t.Fatalf("editor approval status = %d, want 403", approveRec.Code)
	}

	rejectReq := httptest.NewRequest(http.MethodDelete, "/api/admin/languages/fr", strings.NewReader(`{"confirmation":"fr"}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rejectRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusForbidden {
		t.Fatalf("editor rejection status = %d, want 403", rejectRec.Code)
	}
}

func TestLanguageProposalRejectionRequiresMatchingConfirmation(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionAdmin})
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "fr", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/languages/fr", strings.NewReader(`{"confirmation":"de"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched confirmation status = %d, want 400", rec.Code)
	}
	if _, err := store.GetEdition(context.Background(), "fr"); err != nil {
		t.Fatalf("proposal was removed after mismatched confirmation: %v", err)
	}
}

func TestLanguageProposalRejectsCompositeTag(t *testing.T) {
	store, accounts, token := openEditionTranslationTestState(t, []auth.Permission{auth.PermissionEdit})
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	req := httptest.NewRequest(http.MethodPost, "/api/editor/languages", strings.NewReader(`{"tag":"zh-Hans"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("composite language tag status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestInterfaceTranslationRetryRequiresAuthentication(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/interface/translate", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous translation retry status = %d, want 401", rec.Code)
	}
}

func openEditionTranslationTestState(t *testing.T, permissions []auth.Permission) (*storage.BadgerDocumentStore, *auth.Store, string) {
	t.Helper()
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open account store: %v", err)
	}
	t.Cleanup(func() { _ = accounts.Close() })
	if _, err := accounts.Register(context.Background(), auth.Registration{
		Username: "translation_operator", Password: "long-translation-password", Permissions: permissions,
	}); err != nil {
		t.Fatalf("register translation operator: %v", err)
	}
	_, token, _, err := accounts.Login(context.Background(), "translation_operator", "long-translation-password")
	if err != nil {
		t.Fatalf("login translation operator: %v", err)
	}
	return store, accounts, token
}

var _ translation.InterfaceTranslator = fixedInterfaceTranslator{}
