package edition

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusActive   Status = "active"
)

var (
	ErrNotFound      = errors.New("language edition not found")
	ErrAlreadyExists = errors.New("language edition already exists")
	ErrInvalid       = errors.New("invalid language edition")
)

var requiredInterfaceMessageKeys = []string{
	"menu", "close", "tagline", "searchLabel", "searchPlaceholder", "search", "noResults",
	"userActions", "account", "adminPanel", "logout", "login", "signup", "navigation",
	"languages", "addLanguage", "sidebarNote", "toc", "section", "document", "read",
	"discussion", "edit", "history", "contentLicense", "updated", "historyTitle",
	"currentArticle", "noSummary", "noRevisions", "selectRevision", "loadingHistory",
	"historyError", "revisionError", "editor", "cancel", "save", "saving", "loadingSource",
	"documentName", "editSummary", "summaryPlaceholder", "source", "autocomplete",
	"livePreview", "serverRendering", "saved", "sourceError", "previewError", "saveError",
	"graph", "graphAll", "graphLanguage", "graphSearch", "graphLoading", "graphEmpty",
	"graphReset", "graphDocuments", "graphLinks", "graphTruncated",
}

var optionalInterfaceMessageKeys = []string{
	"theme", "lines", "characters", "scrollTop", "scrollBottom", "backlinks", "noBacklinks",
	"graphLayoutStart", "graphLayoutPause", "graphLayoutRunning", "graphLayoutStable", "graphLayoutPaused", "graphLayoutFailed",
}

func InterfaceMessageKeys() []string {
	return append([]string(nil), requiredInterfaceMessageKeys...)
}

func OptionalInterfaceMessageKeys() []string {
	return append([]string(nil), optionalInterfaceMessageKeys...)
}

func AllInterfaceMessageKeys() []string {
	values := make([]string, 0, len(requiredInterfaceMessageKeys)+len(optionalInterfaceMessageKeys))
	values = append(values, requiredInterfaceMessageKeys...)
	values = append(values, optionalInterfaceMessageKeys...)
	return values
}

type Edition struct {
	Tag               string              `json:"tag"`
	Name              string              `json:"name"`
	NativeName        string              `json:"native_name"`
	Direction         string              `json:"direction"`
	Status            Status              `json:"status"`
	MainPageName      string              `json:"main_page_name,omitempty"`
	RequestedBy       string              `json:"requested_by,omitempty"`
	RequestedAt       time.Time           `json:"requested_at"`
	ApprovedBy        string              `json:"approved_by,omitempty"`
	ApprovedAt        time.Time           `json:"approved_at,omitempty"`
	ActivatedAt       time.Time           `json:"activated_at,omitempty"`
	InterfaceMessages map[string]string   `json:"interface_messages,omitempty"`
	Navigation        []NavigationSection `json:"navigation,omitempty"`
	PortalOrder       int                 `json:"portal_order,omitempty"`
	CountryCodes      []string            `json:"country_codes,omitempty"`
}

type NavigationSection struct {
	Title string           `json:"title"`
	Items []NavigationItem `json:"items"`
}

type NavigationItem struct {
	Label  string `json:"label"`
	Target string `json:"target"`
}

func (e Edition) Active() bool {
	return e.Status == StatusActive && strings.TrimSpace(e.MainPageName) != ""
}

func (e Edition) Configurable() bool {
	return e.Status == StatusApproved || e.Status == StatusActive
}

func Normalize(value Edition) (Edition, error) {
	profile, ok := LanguageProfileFor(value.Tag)
	if !ok {
		return Edition{}, ErrInvalid
	}
	value.Tag = profile.Tag
	value.Name = profile.Name
	value.NativeName = profile.NativeName
	value.Direction = profile.Direction
	value.MainPageName = strings.TrimSpace(value.MainPageName)
	value.RequestedBy = strings.TrimSpace(value.RequestedBy)
	value.ApprovedBy = strings.TrimSpace(value.ApprovedBy)
	if len([]rune(value.Name)) > 80 || len([]rune(value.NativeName)) > 80 || len([]rune(value.MainPageName)) > 120 {
		return Edition{}, ErrInvalid
	}
	if value.Status != StatusPending && value.Status != StatusApproved && value.Status != StatusActive {
		return Edition{}, ErrInvalid
	}
	if value.Status == StatusPending {
		value.ApprovedBy = ""
		value.ApprovedAt = time.Time{}
	}
	if value.Status != StatusActive {
		value.MainPageName = ""
		value.ActivatedAt = time.Time{}
	}
	if value.Status == StatusActive && value.MainPageName == "" {
		return Edition{}, ErrInvalid
	}
	if len(value.InterfaceMessages) > 100 {
		return Edition{}, ErrInvalid
	}
	messages := make(map[string]string, len(value.InterfaceMessages))
	for key, message := range value.InterfaceMessages {
		key = strings.TrimSpace(key)
		message = strings.TrimSpace(message)
		if !validMessageKey(key) || len([]rune(message)) > 240 {
			return Edition{}, ErrInvalid
		}
		if message != "" {
			messages[key] = message
		}
	}
	value.InterfaceMessages = messages
	if len(value.Navigation) > 8 {
		return Edition{}, ErrInvalid
	}
	navigation := make([]NavigationSection, 0, len(value.Navigation))
	for _, section := range value.Navigation {
		section.Title = strings.TrimSpace(section.Title)
		if section.Title == "" || len([]rune(section.Title)) > 80 || len(section.Items) == 0 || len(section.Items) > 16 {
			return Edition{}, ErrInvalid
		}
		items := make([]NavigationItem, 0, len(section.Items))
		for _, item := range section.Items {
			item.Label = strings.TrimSpace(item.Label)
			item.Target = strings.TrimSpace(item.Target)
			if item.Label == "" || item.Target == "" || len([]rune(item.Label)) > 120 || len([]rune(item.Target)) > 120 {
				return Edition{}, ErrInvalid
			}
			items = append(items, item)
		}
		section.Items = items
		navigation = append(navigation, section)
	}
	value.Navigation = navigation
	if value.PortalOrder < 0 || value.PortalOrder > 100000 || len(value.CountryCodes) > 300 {
		return Edition{}, ErrInvalid
	}
	countries := make([]string, 0, len(value.CountryCodes))
	seenCountries := make(map[string]struct{}, len(value.CountryCodes))
	for _, code := range value.CountryCodes {
		code = strings.ToUpper(strings.TrimSpace(code))
		if !ValidCountryCode(code) {
			return Edition{}, ErrInvalid
		}
		if _, exists := seenCountries[code]; exists {
			continue
		}
		seenCountries[code] = struct{}{}
		countries = append(countries, code)
	}
	value.CountryCodes = countries
	if value.Status == StatusActive {
		if len(value.Navigation) == 0 {
			return Edition{}, ErrInvalid
		}
		for _, key := range requiredInterfaceMessageKeys {
			if value.InterfaceMessages[key] == "" {
				return Edition{}, ErrInvalid
			}
		}
	}
	return value, nil
}

func validMessageKey(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, current := range value[1:] {
		if (current < 'a' || current > 'z') && (current < 'A' || current > 'Z') && (current < '0' || current > '9') {
			return false
		}
	}
	return len(value) <= 48
}

type Store interface {
	ListEditions(ctx context.Context) ([]Edition, error)
	GetEdition(ctx context.Context, tag string) (Edition, error)
	ProposeEdition(ctx context.Context, value Edition) (Edition, error)
	ApproveEdition(ctx context.Context, tag string, approvedBy string, approvedAt time.Time) (Edition, error)
	RejectEdition(ctx context.Context, tag string) (Edition, error)
	ActivateEdition(ctx context.Context, tag string, mainPageName string, activatedAt time.Time) (Edition, error)
	UpdateInterfaceMessages(ctx context.Context, tag string, messages map[string]string) (Edition, error)
	FillInterfaceMessages(ctx context.Context, tag string, messages map[string]string) (Edition, int, error)
	UpdateNavigation(ctx context.Context, tag string, navigation []NavigationSection) (Edition, error)
	UpdatePortal(ctx context.Context, tag string, portalOrder int, countryCodes []string) (Edition, error)
}
