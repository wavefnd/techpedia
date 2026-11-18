package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	badger "github.com/dgraph-io/badger/v4"
	"golang.org/x/crypto/argon2"
)

const (
	PermissionEdit     Permission = "edit"
	PermissionTechdata Permission = "techdata"
	PermissionAdmin    Permission = "admin"
	PermissionOwner    Permission = "owner"
)

const sessionLifetime = 30 * 24 * time.Hour

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRegistration = errors.New("invalid registration")
	ErrUsernameTaken       = errors.New("username already exists")
	ErrSessionNotFound     = errors.New("session not found")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidPermissions  = errors.New("invalid regular user permissions")
	ErrOwnerImmutable      = errors.New("owner permissions are immutable")
	ErrOwnerAlreadyExists  = errors.New("a different bootstrap owner already exists")
	ErrInvalidProfile      = errors.New("invalid user profile")
	ErrBusy                = errors.New("authentication temporarily busy")
	usernamePattern        = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
)

type Permission string

type User struct {
	ID           string       `json:"id"`
	Username     string       `json:"username"`
	DisplayName  string       `json:"display_name"`
	Bio          string       `json:"bio,omitempty"`
	Permissions  []Permission `json:"permissions"`
	PasswordSalt []byte       `json:"password_salt,omitempty"`
	PasswordHash []byte       `json:"password_hash,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type Session struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Registration struct {
	Username              string
	DisplayName           string
	Password              string
	Permissions           []Permission
	UseRegularPermissions bool
}

type ProfileUpdate struct {
	DisplayName string
	Bio         string
}

type Backend interface {
	Register(ctx context.Context, registration Registration) (User, error)
	Login(ctx context.Context, username string, password string) (User, string, time.Time, error)
	Authenticate(ctx context.Context, token string) (User, error)
	Logout(ctx context.Context, token string) error
	GetProfile(ctx context.Context, username string) (User, error)
	UpdateProfile(ctx context.Context, userID string, profile ProfileUpdate) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	SetPermissions(ctx context.Context, userID string, permissions []Permission) (User, error)
	RegularUserPermissions(ctx context.Context) ([]Permission, error)
	SetRegularUserPermissions(ctx context.Context, permissions []Permission) (int, error)
	EnsureBootstrapAdmin(ctx context.Context, username string, password string) error
	Close() error
}

type Store struct {
	db           *badger.DB
	passwordWork passwordWorkBudget
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	opts := badger.DefaultOptions(path)
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.ensureRegularUserPolicy(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Register(ctx context.Context, registration Registration) (User, error) {
	username := normalizeUsername(registration.Username)
	displayName := strings.TrimSpace(registration.DisplayName)
	if !usernamePattern.MatchString(username) || len(registration.Password) < 10 {
		return User{}, ErrInvalidRegistration
	}
	if displayName == "" {
		displayName = username
	}
	if len([]rune(displayName)) > 40 {
		return User{}, ErrInvalidRegistration
	}
	if _, err := s.getUserByUsername(ctx, username); err == nil {
		return User{}, ErrUsernameTaken
	} else if !errors.Is(err, ErrUserNotFound) {
		return User{}, err
	}
	release, err := s.passwordWork.acquire(ctx)
	if err != nil {
		return User{}, err
	}
	defer release()

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	user := User{
		Username:     username,
		DisplayName:  displayName,
		Permissions:  normalizeManagedPermissions(registration.Permissions),
		PasswordSalt: salt,
		PasswordHash: hashPassword(registration.Password, salt),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		if registration.UseRegularPermissions {
			permissions, err := regularUserPermissionsFromTxn(txn)
			if err != nil {
				return err
			}
			user.Permissions = permissions
		}
		if _, err := txn.Get([]byte(usernameKey(username))); err == nil {
			return ErrUsernameTaken
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		next, err := nextSequence(txn, userSequenceKey)
		if err != nil {
			return err
		}
		user.ID = fmt.Sprintf("U%d", next)
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(userKey(user.ID)), data); err != nil {
			return err
		}
		return txn.Set([]byte(usernameKey(username)), []byte(user.ID))
	})
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

func (s *Store) Login(ctx context.Context, username string, password string) (User, string, time.Time, error) {
	user, err := s.getUserByUsername(ctx, username)
	if err != nil || len(user.PasswordSalt) == 0 || len(user.PasswordHash) == 0 {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}
	release, err := s.passwordWork.acquire(ctx)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	defer release()
	actual := hashPassword(password, user.PasswordSalt)
	if subtle.ConstantTimeCompare(actual, user.PasswordHash) != 1 {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return User{}, "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expiresAt := time.Now().UTC().Add(sessionLifetime)
	sessionData, err := json.Marshal(Session{UserID: user.ID, ExpiresAt: expiresAt})
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	if err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		return txn.Set([]byte(sessionKey(token)), sessionData)
	}); err != nil {
		return User{}, "", time.Time{}, err
	}
	return publicUser(user), token, expiresAt, nil
}

func (s *Store) Authenticate(ctx context.Context, token string) (User, error) {
	if strings.TrimSpace(token) == "" {
		return User{}, ErrSessionNotFound
	}
	var session Session
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		return readJSON(txn, sessionKey(token), &session, ErrSessionNotFound)
	})
	if err != nil {
		return User{}, err
	}
	if !session.ExpiresAt.After(time.Now().UTC()) {
		_ = s.Logout(ctx, token)
		return User{}, ErrSessionNotFound
	}
	user, err := s.getUser(ctx, session.UserID)
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		err := txn.Delete([]byte(sessionKey(token)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
}

func (s *Store) GetProfile(ctx context.Context, username string) (User, error) {
	username = normalizeUsername(username)
	if !usernamePattern.MatchString(username) {
		return User{}, ErrUserNotFound
	}
	user, err := s.getUserByUsername(ctx, username)
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

func (s *Store) UpdateProfile(ctx context.Context, userID string, profile ProfileUpdate) (User, error) {
	displayName := strings.TrimSpace(profile.DisplayName)
	bio := strings.TrimSpace(strings.ReplaceAll(profile.Bio, "\r\n", "\n"))
	if displayName == "" || len([]rune(displayName)) > 40 || len([]rune(bio)) > 1000 || hasInvalidProfileControl(bio) {
		return User{}, ErrInvalidProfile
	}

	var user User
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		if err := readJSON(txn, userKey(userID), &user, ErrUserNotFound); err != nil {
			return err
		}
		user.DisplayName = displayName
		user.Bio = bio
		user.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return txn.Set([]byte(userKey(user.ID)), data)
	})
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	users := make([]User, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(userPrefix)
		opts.PrefetchValues = true
		iterator := txn.NewIterator(opts)
		defer iterator.Close()
		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			if err := contextError(ctx); err != nil {
				return err
			}
			var user User
			if err := iterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &user) }); err != nil {
				return err
			}
			users = append(users, publicUser(user))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.Before(users[j].CreatedAt) })
	return users, nil
}

func (s *Store) SetPermissions(ctx context.Context, userID string, permissions []Permission) (User, error) {
	var user User
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		if err := readJSON(txn, userKey(userID), &user, ErrUserNotFound); err != nil {
			return err
		}
		if user.Has(PermissionOwner) || containsPermission(permissions, PermissionOwner) {
			return ErrOwnerImmutable
		}
		user.Permissions = normalizePermissions(permissions)
		user.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}
		return txn.Set([]byte(userKey(user.ID)), data)
	})
	if err != nil {
		return User{}, err
	}
	return publicUser(user), nil
}

func (s *Store) RegularUserPermissions(ctx context.Context) ([]Permission, error) {
	var permissions []Permission
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		var err error
		permissions, err = regularUserPermissionsFromTxn(txn)
		return err
	})
	return permissions, err
}

func (s *Store) SetRegularUserPermissions(ctx context.Context, permissions []Permission) (int, error) {
	if !validRegularUserPermissions(permissions) {
		return 0, ErrInvalidPermissions
	}
	permissions = normalizePermissions(permissions)
	updated := 0
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		var err error
		updated, err = setRegularUserPermissionsTxn(ctx, txn, permissions)
		return err
	})
	return updated, err
}

func (s *Store) ensureRegularUserPolicy() error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(regularPermissionsKey)); err == nil {
			return nil
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		_, err := setRegularUserPermissionsTxn(context.Background(), txn, []Permission{})
		return err
	})
}

func (s *Store) EnsureBootstrapAdmin(ctx context.Context, username string, password string) error {
	username = normalizeUsername(username)
	if username == "" && password == "" {
		return nil
	}
	if username == "" {
		return errors.New("bootstrap admin username is required")
	}
	user, err := s.getUserByUsername(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		if password == "" {
			return errors.New("bootstrap admin password is required to create the owner account")
		}
		user, err = s.Register(ctx, Registration{
			Username:    username,
			DisplayName: username,
			Password:    password,
			Permissions: []Permission{PermissionEdit, PermissionTechdata, PermissionAdmin},
		})
	}
	if err != nil {
		return err
	}
	return s.ensureOwner(ctx, user.ID)
}

func (s *Store) ensureOwner(ctx context.Context, userID string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		if item, err := txn.Get([]byte(ownerKey)); err == nil {
			var ownerID string
			if err := item.Value(func(value []byte) error {
				ownerID = string(value)
				return nil
			}); err != nil {
				return err
			}
			if ownerID != userID {
				return ErrOwnerAlreadyExists
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		var user User
		if err := readJSON(txn, userKey(userID), &user, ErrUserNotFound); err != nil {
			return err
		}
		if !user.Has(PermissionOwner) {
			user.Permissions = normalizePermissions(append(user.Permissions, PermissionOwner))
			user.UpdatedAt = time.Now().UTC()
			data, err := json.Marshal(user)
			if err != nil {
				return err
			}
			if err := txn.Set([]byte(userKey(user.ID)), data); err != nil {
				return err
			}
		}
		return txn.Set([]byte(ownerKey), []byte(userID))
	})
}

func (s *Store) getUserByUsername(ctx context.Context, username string) (User, error) {
	username = normalizeUsername(username)
	var userID string
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(usernameKey(username)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			userID = string(value)
			return nil
		})
	})
	if err != nil {
		return User{}, err
	}
	return s.getUser(ctx, userID)
}

func (s *Store) getUser(ctx context.Context, userID string) (User, error) {
	var user User
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		return readJSON(txn, userKey(userID), &user, ErrUserNotFound)
	})
	return user, err
}

func (u User) Has(permission Permission) bool {
	for _, current := range u.Permissions {
		if current == permission || (permission != PermissionOwner && (current == PermissionAdmin || current == PermissionOwner)) {
			return true
		}
	}
	return false
}

func publicUser(user User) User {
	user.PasswordSalt = nil
	user.PasswordHash = nil
	if user.Permissions == nil {
		user.Permissions = []Permission{}
	}
	return user
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func hasInvalidProfileControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return true
		}
	}
	return false
}

func normalizePermissions(values []Permission) []Permission {
	seen := make(map[Permission]struct{}, len(values))
	permissions := make([]Permission, 0, len(values))
	for _, permission := range values {
		switch permission {
		case PermissionEdit, PermissionTechdata, PermissionAdmin, PermissionOwner:
			if _, ok := seen[permission]; !ok {
				seen[permission] = struct{}{}
				permissions = append(permissions, permission)
			}
		}
	}
	sort.Slice(permissions, func(i, j int) bool { return permissions[i] < permissions[j] })
	return permissions
}

func normalizeManagedPermissions(values []Permission) []Permission {
	permissions := make([]Permission, 0, len(values))
	for _, permission := range values {
		if permission != PermissionOwner {
			permissions = append(permissions, permission)
		}
	}
	return normalizePermissions(permissions)
}

func containsPermission(permissions []Permission, expected Permission) bool {
	for _, permission := range permissions {
		if permission == expected {
			return true
		}
	}
	return false
}

func validRegularUserPermissions(values []Permission) bool {
	for _, permission := range values {
		if permission != PermissionEdit && permission != PermissionTechdata {
			return false
		}
	}
	return true
}

func regularUserPermissionsFromTxn(txn *badger.Txn) ([]Permission, error) {
	var permissions []Permission
	err := readJSON(txn, regularPermissionsKey, &permissions, nil)
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		permissions = []Permission{}
	}
	return normalizePermissions(permissions), nil
}

func setRegularUserPermissionsTxn(ctx context.Context, txn *badger.Txn, permissions []Permission) (int, error) {
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return 0, err
	}
	if err := txn.Set([]byte(regularPermissionsKey), encoded); err != nil {
		return 0, err
	}

	updated := 0
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(userPrefix)
	iterator := txn.NewIterator(opts)
	defer iterator.Close()
	for iterator.Rewind(); iterator.Valid(); iterator.Next() {
		if err := contextError(ctx); err != nil {
			return 0, err
		}
		var user User
		if err := iterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &user) }); err != nil {
			return 0, err
		}
		if user.Has(PermissionAdmin) {
			continue
		}
		user.Permissions = append([]Permission{}, permissions...)
		user.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(user)
		if err != nil {
			return 0, err
		}
		if err := txn.Set([]byte(userKey(user.ID)), data); err != nil {
			return 0, err
		}
		updated++
	}
	return updated, nil
}

func hashPassword(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
}

func sessionKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return sessionPrefix + base64.RawURLEncoding.EncodeToString(hash[:])
}

func readJSON(txn *badger.Txn, key string, target any, notFound error) error {
	item, err := txn.Get([]byte(key))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return notFound
	}
	if err != nil {
		return err
	}
	return item.Value(func(value []byte) error { return json.Unmarshal(value, target) })
}

func nextSequence(txn *badger.Txn, key string) (uint64, error) {
	var current uint64
	item, err := txn.Get([]byte(key))
	if err == nil {
		if err := item.Value(func(value []byte) error {
			if len(value) != 8 {
				return errors.New("invalid auth sequence")
			}
			current = binary.BigEndian.Uint64(value)
			return nil
		}); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}
	current++
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, current)
	if err := txn.Set([]byte(key), value); err != nil {
		return 0, err
	}
	return current, nil
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

const (
	userPrefix            = "auth:v1:user:"
	usernamePrefix        = "auth:v1:username:"
	sessionPrefix         = "auth:v1:session:"
	userSequenceKey       = "auth:v1:sequence:user"
	regularPermissionsKey = "auth:v1:policy:regular-permissions"
	ownerKey              = "auth:v1:owner"
)

func userKey(id string) string           { return userPrefix + id }
func usernameKey(username string) string { return usernamePrefix + username }
