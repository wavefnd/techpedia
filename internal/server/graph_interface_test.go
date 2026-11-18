package server

import (
	"testing"

	"techpedia/internal/edition"
)

func TestGraphInterfaceDefaultsDoNotMutateEditionOrReplaceAdminMessages(t *testing.T) {
	for _, tag := range []string{"ko", "en", "fr"} {
		value := edition.Edition{Tag: tag, InterfaceMessages: map[string]string{"graphLayoutRunning": "관리자가 작성한 문구", "menu": "맞춤 메뉴"}}
		response := newEditionResponse(value)
		if response.InterfaceMessages["graphLayoutRunning"] != "관리자가 작성한 문구" || response.InterfaceMessages["menu"] != "맞춤 메뉴" {
			t.Fatal("administrator messages replaced")
		}
		if response.InterfaceMessages["graphLayoutStart"] == "" || response.InterfaceMessages["graphLayoutStable"] == "" {
			t.Fatal("new optional graph controls have no fallback")
		}
		if len(value.InterfaceMessages) != 2 {
			t.Fatal("response defaults were persisted into source model")
		}
	}
	value := edition.Edition{Tag: "ko"}
	response := newEditionResponse(value)
	if response.InterfaceMessages["graphLayoutPause"] != "움직임 일시정지" || value.InterfaceMessages != nil {
		t.Fatal("unexpected Korean defaults or source mutation")
	}
}
