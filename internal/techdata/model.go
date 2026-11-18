package techdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"techpedia/internal/document"
)

var (
	ErrItemNotFound       = errors.New("techdata item not found")
	ErrLocaleOccupied     = errors.New("techdata locale already has a wiki page")
	ErrWikiPageLinked     = errors.New("wiki page already belongs to another techdata item")
	ErrItemInUse          = errors.New("techdata item is still in use")
	ErrPotentialDuplicate = errors.New("potential duplicate techdata item")
	ErrInvalidValue       = errors.New("invalid techdata value")
	ErrUnsupportedLocale  = errors.New("unsupported locale")
)

type CreateItemOptions struct {
	ConfirmDistinct bool
}

type DuplicateItemError struct {
	Candidates []Item
}

func (e *DuplicateItemError) Error() string {
	return ErrPotentialDuplicate.Error()
}

func (e *DuplicateItemError) Unwrap() error {
	return ErrPotentialDuplicate
}

type Item struct {
	ID           TID                     `json:"id"`
	Labels       map[string]string       `json:"labels"`
	Descriptions map[string]string       `json:"descriptions"`
	Aliases      map[string][]string     `json:"aliases"`
	WikiPages    map[string]document.WID `json:"wiki_pages"`
	Statements   []Statement             `json:"statements"`
	Version      uint64                  `json:"version"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

type Property struct {
	ID           PID                 `json:"id"`
	DataType     DataType            `json:"data_type"`
	Labels       map[string]string   `json:"labels"`
	Descriptions map[string]string   `json:"descriptions"`
	Aliases      map[string][]string `json:"aliases"`
	Version      uint64              `json:"version"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type DataType string

const (
	DataTypeItem     DataType = "item"
	DataTypeString   DataType = "string"
	DataTypeQuantity DataType = "quantity"
	DataTypeTime     DataType = "time"
	DataTypeURL      DataType = "url"
)

type Rank string

const (
	RankPreferred  Rank = "preferred"
	RankNormal     Rank = "normal"
	RankDeprecated Rank = "deprecated"
)

type Value struct {
	Type   DataType `json:"type"`
	Item   TID      `json:"item,omitempty"`
	Text   string   `json:"text,omitempty"`
	Amount string   `json:"amount,omitempty"`
	Unit   TID      `json:"unit,omitempty"`
}

type Snak struct {
	Property PID   `json:"property"`
	Value    Value `json:"value"`
}

type Reference struct {
	Snaks []Snak `json:"snaks"`
}

type Statement struct {
	Property   PID         `json:"property"`
	Value      Value       `json:"value"`
	Rank       Rank        `json:"rank"`
	Qualifiers []Snak      `json:"qualifiers"`
	References []Reference `json:"references"`
}

type Store interface {
	CreateItem(ctx context.Context, item Item, options ...CreateItemOptions) (Item, error)
	ListItems(ctx context.Context) ([]Item, error)
	GetItem(ctx context.Context, id TID) (Item, error)
	DeleteItem(ctx context.Context, id TID) (Item, error)
	GetItemByWikiPage(ctx context.Context, wid document.WID) (Item, error)
	AttachWikiPage(ctx context.Context, id TID, locale string, wid document.WID, label string) (Item, error)
	DetachWikiPage(ctx context.Context, id TID, locale string) (Item, error)
}

func NormalizeIdentityName(value string) string {
	value = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
	var result strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("+#*-", char) {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func NormalizeItem(item Item) (Item, error) {
	var err error
	item.Labels, err = normalizeLocalizedText(item.Labels)
	if err != nil {
		return Item{}, err
	}
	item.Descriptions, err = normalizeLocalizedText(item.Descriptions)
	if err != nil {
		return Item{}, err
	}
	item.Aliases, err = normalizeAliases(item.Aliases)
	if err != nil {
		return Item{}, err
	}
	if item.WikiPages == nil {
		item.WikiPages = map[string]document.WID{}
	}
	seenWikiPages := make(map[document.WID]struct{}, len(item.WikiPages))
	for locale, wid := range item.WikiPages {
		if !document.IsSupportedLocale(locale) {
			return Item{}, fmt.Errorf("%w: %s", ErrUnsupportedLocale, locale)
		}
		if err := wid.Validate(); err != nil {
			return Item{}, err
		}
		if _, exists := seenWikiPages[wid]; exists {
			return Item{}, techdataWikiPageLinkedError(wid)
		}
		seenWikiPages[wid] = struct{}{}
	}
	if item.Statements == nil {
		item.Statements = []Statement{}
	}
	for _, statement := range item.Statements {
		if err := statement.Validate(); err != nil {
			return Item{}, err
		}
	}
	return item, nil
}

func techdataWikiPageLinkedError(wid document.WID) error {
	return fmt.Errorf("%w: %s is linked more than once", ErrWikiPageLinked, wid)
}

func (statement Statement) Validate() error {
	if err := statement.Property.Validate(); err != nil {
		return err
	}
	if statement.Rank != "" && statement.Rank != RankPreferred && statement.Rank != RankNormal && statement.Rank != RankDeprecated {
		return fmt.Errorf("%w: unknown rank %q", ErrInvalidValue, statement.Rank)
	}
	if err := statement.Value.Validate(); err != nil {
		return err
	}
	for _, qualifier := range statement.Qualifiers {
		if err := qualifier.Validate(); err != nil {
			return err
		}
	}
	for _, reference := range statement.References {
		for _, snak := range reference.Snaks {
			if err := snak.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (snak Snak) Validate() error {
	if err := snak.Property.Validate(); err != nil {
		return err
	}
	return snak.Value.Validate()
}

func (value Value) Validate() error {
	switch value.Type {
	case DataTypeItem:
		return value.Item.Validate()
	case DataTypeString, DataTypeTime, DataTypeURL:
		if strings.TrimSpace(value.Text) == "" {
			return fmt.Errorf("%w: %s value is empty", ErrInvalidValue, value.Type)
		}
		return nil
	case DataTypeQuantity:
		if strings.TrimSpace(value.Amount) == "" {
			return fmt.Errorf("%w: quantity amount is empty", ErrInvalidValue)
		}
		if value.Unit != "" {
			return value.Unit.Validate()
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidValue, value.Type)
	}
}

func normalizeLocalizedText(values map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for locale, value := range values {
		if !document.IsSupportedLocale(locale) {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedLocale, locale)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			result[locale] = value
		}
	}
	return result, nil
}

func normalizeAliases(values map[string][]string) (map[string][]string, error) {
	result := make(map[string][]string, len(values))
	for locale, aliases := range values {
		if !document.IsSupportedLocale(locale) {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedLocale, locale)
		}
		seen := map[string]struct{}{}
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			key := strings.ToLower(alias)
			if alias == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result[locale] = append(result[locale], alias)
		}
	}
	return result, nil
}
