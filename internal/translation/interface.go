package translation

// InterfaceMessage is an unambiguous English seed used only to create an
// editable interface draft for an administrator-approved language edition.
type InterfaceMessage struct {
	Key    string
	Source string
}

var interfaceMessages = []InterfaceMessage{
	{Key: "menu", Source: "Menu"},
	{Key: "close", Source: "Close"},
	{Key: "tagline", Source: "The computer science and software engineering encyclopedia"},
	{Key: "searchLabel", Source: "Search TechPedia"},
	{Key: "searchPlaceholder", Source: "Search documents"},
	{Key: "search", Source: "Search"},
	{Key: "noResults", Source: "No search results"},
	{Key: "userActions", Source: "User menu"},
	{Key: "account", Source: "My account"},
	{Key: "adminPanel", Source: "Administrator panel"},
	{Key: "logout", Source: "Log out"},
	{Key: "login", Source: "Log in"},
	{Key: "signup", Source: "Create account"},
	{Key: "theme", Source: "Change theme"},
	{Key: "navigation", Source: "Navigation"},
	{Key: "languages", Source: "Languages"},
	{Key: "addLanguage", Source: "Add language"},
	{Key: "sidebarNote", Source: "Explore TechPedia"},
	{Key: "toc", Source: "Contents"},
	{Key: "section", Source: "Section"},
	{Key: "document", Source: "Document"},
	{Key: "read", Source: "Read"},
	{Key: "discussion", Source: "Discussion"},
	{Key: "edit", Source: "Edit document"},
	{Key: "history", Source: "Revision history"},
	{Key: "contentLicense", Source: "Content license"},
	{Key: "updated", Source: "Last updated"},
	{Key: "historyTitle", Source: "Document revision history"},
	{Key: "currentArticle", Source: "Current document"},
	{Key: "noSummary", Source: "No edit summary"},
	{Key: "noRevisions", Source: "No revisions"},
	{Key: "selectRevision", Source: "Select a revision"},
	{Key: "loadingHistory", Source: "Loading revision history"},
	{Key: "historyError", Source: "Could not load revision history"},
	{Key: "revisionError", Source: "Could not load the revision"},
	{Key: "editor", Source: "Document editor"},
	{Key: "cancel", Source: "Cancel"},
	{Key: "save", Source: "Save document"},
	{Key: "saving", Source: "Saving document"},
	{Key: "loadingSource", Source: "Loading document source"},
	{Key: "documentName", Source: "Document name"},
	{Key: "editSummary", Source: "Edit summary"},
	{Key: "summaryPlaceholder", Source: "Describe your changes"},
	{Key: "source", Source: "Document source"},
	{Key: "autocomplete", Source: "Autocomplete"},
	{Key: "livePreview", Source: "Live preview"},
	{Key: "serverRendering", Source: "Server-rendered preview"},
	{Key: "saved", Source: "Document saved"},
	{Key: "sourceError", Source: "Could not load document source"},
	{Key: "previewError", Source: "Could not render preview"},
	{Key: "saveError", Source: "Could not save document"},
	{Key: "graph", Source: "Document graph"},
	{Key: "graphAll", Source: "All languages graph"},
	{Key: "graphLanguage", Source: "Current language graph"},
	{Key: "graphSearch", Source: "Search graph documents"},
	{Key: "graphLoading", Source: "Loading document graph"},
	{Key: "graphEmpty", Source: "No graph documents"},
	{Key: "graphReset", Source: "Reset graph view"},
	{Key: "graphDocuments", Source: "documents"},
	{Key: "graphLinks", Source: "links"},
	{Key: "graphTruncated", Source: "Only some documents are shown"},
	{Key: "lines", Source: "lines"},
	{Key: "characters", Source: "characters"},
	{Key: "scrollTop", Source: "Go to the top"},
	{Key: "scrollBottom", Source: "Go to the bottom"},
	{Key: "backlinks", Source: "Documents that link here"},
	{Key: "noBacklinks", Source: "No documents link here"},
}

func InterfaceMessages() []InterfaceMessage {
	return append(append([]InterfaceMessage(nil), interfaceMessages...), graphInterfaceMessages...)
}

var graphInterfaceMessages = []InterfaceMessage{
	{Key: "graphLayoutStart", Source: "Resume layout"},
	{Key: "graphLayoutPause", Source: "Pause layout"},
	{Key: "graphLayoutRunning", Source: "Layout in motion"},
	{Key: "graphLayoutStable", Source: "Motion has settled; using less processing"},
	{Key: "graphLayoutPaused", Source: "Layout paused"},
	{Key: "graphLayoutFailed", Source: "Animation is unavailable. Document exploration still works."},
}

// New optional controls remain usable on existing editions until an admin translates
// them. Defaults are response-only and never replace saved edition messages.
func GraphInterfaceDefaults(tag string) map[string]string {
	if tag == "ko" {
		return map[string]string{
			"graphLayoutStart": "움직임 이어가기", "graphLayoutPause": "움직임 일시정지", "graphLayoutRunning": "탐색 중",
			"graphLayoutStable": "움직임이 잦아들어 계산을 줄이고 있습니다", "graphLayoutPaused": "일시정지됨",
			"graphLayoutFailed": "움직임을 표시할 수 없습니다. 문서 탐색은 계속할 수 있습니다.",
		}
	}
	values := make(map[string]string, len(graphInterfaceMessages))
	for _, message := range graphInterfaceMessages {
		values[message.Key] = message.Source
	}
	return values
}
