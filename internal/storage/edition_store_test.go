package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"techpedia/internal/edition"
)

func TestLanguageEditionLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	values, err := store.ListEditions(ctx)
	if err != nil {
		t.Fatalf("list default editions: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected a fresh store without seeded editions, got %#v", values)
	}

	proposed, err := store.ProposeEdition(ctx, edition.Edition{
		Tag: "pt", Name: "Portuguese", NativeName: "Português",
		Direction: "ltr", RequestedBy: "editor", Status: edition.StatusPending,
	})
	if err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	if proposed.Tag != "pt" || proposed.Active() {
		t.Fatalf("unexpected proposed edition: %#v", proposed)
	}
	if _, err := store.ProposeEdition(ctx, proposed); !errors.Is(err, edition.ErrAlreadyExists) {
		t.Fatalf("expected duplicate proposal conflict, got %v", err)
	}
	approved, err := store.ApproveEdition(ctx, "pt", "administrator", time.Now())
	if err != nil || approved.Status != edition.StatusApproved || approved.ApprovedBy != "administrator" {
		t.Fatalf("approve edition: value=%#v err=%v", approved, err)
	}

	messages := make(map[string]string, len(edition.InterfaceMessageKeys()))
	for _, key := range edition.InterfaceMessageKeys() {
		messages[key] = "configured " + key
	}
	updated, err := store.UpdateInterfaceMessages(ctx, "pt", messages)
	if err != nil || updated.InterfaceMessages["navigation"] == "" {
		t.Fatalf("update interface messages: value=%#v err=%v", updated, err)
	}
	navigation := []edition.NavigationSection{{Title: "Explorar", Items: []edition.NavigationItem{{Label: "Página principal", Target: "Página principal"}}}}
	updated, err = store.UpdateNavigation(ctx, "pt", navigation)
	if err != nil || len(updated.Navigation) != 1 || updated.Navigation[0].Items[0].Target != "Página principal" {
		t.Fatalf("update navigation: value=%#v err=%v", updated, err)
	}
	updated, err = store.UpdatePortal(ctx, "pt", 10, []string{"BR", "PT"})
	if err != nil || updated.PortalOrder != 10 || len(updated.CountryCodes) != 2 {
		t.Fatalf("update portal metadata: value=%#v err=%v", updated, err)
	}

	active, err := store.ActivateEdition(ctx, "pt", "Página principal", time.Now())
	if err != nil {
		t.Fatalf("activate edition: %v", err)
	}
	if !active.Active() || active.MainPageName != "Página principal" {
		t.Fatalf("unexpected active edition: %#v", active)
	}
	messages["navigation"] = "Explorar"
	updated, err = store.UpdateInterfaceMessages(ctx, "pt", messages)
	if err != nil || updated.InterfaceMessages["navigation"] != "Explorar" {
		t.Fatalf("update interface messages: value=%#v err=%v", updated, err)
	}
}

func TestLanguageEditionsUseAdministratorPortalOrder(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, tag := range []string{"en", "ko", "fr"} {
		if _, err := store.ProposeEdition(ctx, edition.Edition{Tag: tag, Direction: "ltr", Status: edition.StatusPending}); err != nil {
			t.Fatalf("propose %s edition: %v", tag, err)
		}
	}
	if _, err := store.UpdatePortal(ctx, "ko", 20, []string{"KR"}); err != nil {
		t.Fatalf("set Korean portal order: %v", err)
	}
	if _, err := store.UpdatePortal(ctx, "fr", 10, []string{"FR"}); err != nil {
		t.Fatalf("set French portal order: %v", err)
	}
	values, err := store.ListEditions(ctx)
	if err != nil {
		t.Fatalf("list editions: %v", err)
	}
	if len(values) != 3 || values[0].Tag != "fr" || values[1].Tag != "ko" || values[2].Tag != "en" {
		t.Fatalf("unexpected portal order: %#v", values)
	}
}

func TestFillInterfaceMessagesPreservesAdministratorValues(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ProposeEdition(ctx, edition.Edition{
		Tag: "fr", Direction: "ltr", Status: edition.StatusPending,
	}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	if _, err := store.ApproveEdition(ctx, "fr", "administrator", time.Now()); err != nil {
		t.Fatalf("approve edition: %v", err)
	}
	if _, err := store.UpdateInterfaceMessages(ctx, "fr", map[string]string{"menu": "Menu personnalisé"}); err != nil {
		t.Fatalf("store administrator interface value: %v", err)
	}

	updated, filled, err := store.FillInterfaceMessages(ctx, "fr", map[string]string{
		"menu":  "Generated menu",
		"close": "Fermer",
	})
	if err != nil {
		t.Fatalf("fill interface messages: %v", err)
	}
	if filled != 1 {
		t.Fatalf("filled = %d, want 1", filled)
	}
	if updated.InterfaceMessages["menu"] != "Menu personnalisé" || updated.InterfaceMessages["close"] != "Fermer" {
		t.Fatalf("unexpected merged interface messages: %#v", updated.InterfaceMessages)
	}
}

func TestPendingLanguageEditionCanBeRejected(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ProposeEdition(ctx, edition.Edition{Tag: "de", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	rejected, err := store.RejectEdition(ctx, "de")
	if err != nil || rejected.Tag != "de" || rejected.Status != edition.StatusPending {
		t.Fatalf("reject edition: value=%#v err=%v", rejected, err)
	}
	if _, err := store.GetEdition(ctx, "de"); !errors.Is(err, edition.ErrNotFound) {
		t.Fatalf("rejected proposal remains stored: %v", err)
	}
}

func TestPendingEditionCannotBeConfiguredOrActivatedBeforeApproval(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ProposeEdition(ctx, edition.Edition{Tag: "de", Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose edition: %v", err)
	}
	if _, err := store.UpdateInterfaceMessages(ctx, "de", map[string]string{"menu": "Menü"}); !errors.Is(err, edition.ErrInvalid) {
		t.Fatalf("pending interface update error = %v, want ErrInvalid", err)
	}
	if _, err := store.ActivateEdition(ctx, "de", "Hauptseite", time.Now()); !errors.Is(err, edition.ErrInvalid) {
		t.Fatalf("pending activation error = %v, want ErrInvalid", err)
	}
}
