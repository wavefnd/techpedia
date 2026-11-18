package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/discussion"
	"techpedia/internal/document"
)

const (
	discussionTopicPrefix = "discussion:v1:topic:"
	discussionPagePrefix  = "discussion:v1:page:"
	discussionReplyPrefix = "discussion:v1:reply:"
)

func (s *BadgerDocumentStore) CreateDiscussionTopic(ctx context.Context, topic discussion.Topic) (discussion.Topic, error) {
	value, err := discussion.NormalizeTopic(topic)
	if err != nil {
		return discussion.Topic{}, err
	}
	now := time.Now().UTC()
	value.CreatedAt = now
	value.UpdatedAt = now
	value.ReplyCount = 0
	encoded, err := json.Marshal(value)
	if err != nil {
		return discussion.Topic{}, err
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if _, err := txn.Get([]byte(discussionTopicKey(value.ID))); err == nil {
			return discussion.ErrInvalid
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := txn.Set([]byte(discussionTopicKey(value.ID)), encoded); err != nil {
			return err
		}
		return txn.Set([]byte(discussionPageIndexKey(value)), []byte(value.ID))
	})
	return value, err
}

func (s *BadgerDocumentStore) ListDiscussionTopics(ctx context.Context, locale string, slug string, limit int) ([]discussion.Topic, error) {
	locale = document.NormalizeLocale(locale)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	prefix := discussionPageIndexPrefix(locale, slug)
	topics := make([]discussion.Topic, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()
		seek := append([]byte(prefix), 0xff)
		for it.Seek(seek); it.ValidForPrefix([]byte(prefix)) && len(topics) < limit; it.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			var topicID string
			if err := it.Item().Value(func(value []byte) error {
				topicID = string(value)
				return nil
			}); err != nil {
				return err
			}
			topic, err := discussionTopicFromTxn(txn, topicID)
			if errors.Is(err, discussion.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			topics = append(topics, topic)
		}
		return nil
	})
	return topics, err
}

func (s *BadgerDocumentStore) ListAllDiscussionTopics(ctx context.Context, query string, limit int) ([]discussion.Topic, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	query = strings.ToLower(strings.TrimSpace(query))
	topics := make([]discussion.Topic, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(discussionTopicPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek([]byte(discussionTopicPrefix)); it.ValidForPrefix([]byte(discussionTopicPrefix)); it.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			var topic discussion.Topic
			if err := it.Item().Value(func(value []byte) error { return json.Unmarshal(value, &topic) }); err != nil {
				return err
			}
			if query != "" && !strings.Contains(strings.ToLower(topic.Subject+"\n"+topic.Body+"\n"+topic.Author+"\n"+topic.PageTitle), query) {
				continue
			}
			topics = append(topics, topic)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(topics, func(i, j int) bool { return topics[i].UpdatedAt.After(topics[j].UpdatedAt) })
	if len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

func (s *BadgerDocumentStore) GetDiscussionThread(ctx context.Context, topicID string) (discussion.Thread, error) {
	var thread discussion.Thread
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		topic, err := discussionTopicFromTxn(txn, topicID)
		if err != nil {
			return err
		}
		thread.Topic = topic
		prefix := []byte(discussionReplyTopicPrefix(topicID))
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var reply discussion.Reply
			if err := it.Item().Value(func(value []byte) error { return json.Unmarshal(value, &reply) }); err != nil {
				return err
			}
			thread.Replies = append(thread.Replies, reply)
		}
		return nil
	})
	return thread, err
}

func (s *BadgerDocumentStore) AddDiscussionReply(ctx context.Context, reply discussion.Reply) (discussion.Reply, error) {
	value, err := discussion.NormalizeReply(reply)
	if err != nil {
		return discussion.Reply{}, err
	}
	value.CreatedAt = time.Now().UTC()
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		topic, err := discussionTopicFromTxn(txn, value.TopicID)
		if err != nil {
			return err
		}
		if !topic.ArchivedAt.IsZero() {
			return discussion.ErrArchived
		}
		encodedReply, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(discussionReplyKey(value)), encodedReply); err != nil {
			return err
		}
		topic.ReplyCount++
		topic.UpdatedAt = value.CreatedAt
		encodedTopic, err := json.Marshal(topic)
		if err != nil {
			return err
		}
		return txn.Set([]byte(discussionTopicKey(topic.ID)), encodedTopic)
	})
	return value, err
}

func (s *BadgerDocumentStore) ArchiveDiscussionTopic(ctx context.Context, topicID string, archivedBy string) (discussion.Topic, error) {
	archivedBy = strings.TrimSpace(archivedBy)
	if archivedBy == "" {
		return discussion.Topic{}, discussion.ErrInvalid
	}
	var topic discussion.Topic
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		value, err := discussionTopicFromTxn(txn, topicID)
		if err != nil {
			return err
		}
		if !value.ArchivedAt.IsZero() {
			topic = value
			return nil
		}
		now := time.Now().UTC()
		value.ArchivedAt = now
		value.ArchivedBy = archivedBy
		value.UpdatedAt = now
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(discussionTopicKey(value.ID)), encoded); err != nil {
			return err
		}
		topic = value
		return nil
	})
	return topic, err
}

func (s *BadgerDocumentStore) DeleteDiscussionTopic(ctx context.Context, topicID string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		topic, err := discussionTopicFromTxn(txn, topicID)
		if err != nil {
			return err
		}
		if err := txn.Delete([]byte(discussionTopicKey(topic.ID))); err != nil {
			return err
		}
		if err := txn.Delete([]byte(discussionPageIndexKey(topic))); err != nil {
			return err
		}
		return deleteKeysByPrefix(txn, discussionReplyTopicPrefix(topic.ID))
	})
}

func (s *BadgerDocumentStore) DeleteDiscussionReply(ctx context.Context, topicID string, replyID string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		topic, err := discussionTopicFromTxn(txn, topicID)
		if err != nil {
			return err
		}
		prefix := []byte(discussionReplyTopicPrefix(topicID))
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		var key []byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var reply discussion.Reply
			if err := it.Item().Value(func(value []byte) error { return json.Unmarshal(value, &reply) }); err != nil {
				return err
			}
			if reply.ID == replyID {
				key = it.Item().KeyCopy(nil)
				break
			}
		}
		if len(key) == 0 {
			return discussion.ErrNotFound
		}
		if err := txn.Delete(key); err != nil {
			return err
		}
		if topic.ReplyCount > 0 {
			topic.ReplyCount--
		}
		encoded, err := json.Marshal(topic)
		if err != nil {
			return err
		}
		return txn.Set([]byte(discussionTopicKey(topic.ID)), encoded)
	})
}

func discussionTopicFromTxn(txn *badger.Txn, topicID string) (discussion.Topic, error) {
	item, err := txn.Get([]byte(discussionTopicKey(topicID)))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return discussion.Topic{}, discussion.ErrNotFound
	}
	if err != nil {
		return discussion.Topic{}, err
	}
	var topic discussion.Topic
	if err := item.Value(func(value []byte) error { return json.Unmarshal(value, &topic) }); err != nil {
		return discussion.Topic{}, err
	}
	return topic, nil
}

func discussionTopicKey(topicID string) string {
	return discussionTopicPrefix + strings.TrimSpace(topicID)
}

func discussionPageIndexPrefix(locale string, slug string) string {
	return discussionPagePrefix + document.NormalizeLocale(locale) + ":" + slug + ":"
}

func discussionPageIndexKey(topic discussion.Topic) string {
	return discussionPageIndexPrefix(topic.Locale, topic.Slug) + fmt.Sprintf("%020d:%s", topic.CreatedAt.UnixNano(), topic.ID)
}

func discussionReplyTopicPrefix(topicID string) string {
	return discussionReplyPrefix + strings.TrimSpace(topicID) + ":"
}

func discussionReplyKey(reply discussion.Reply) string {
	return discussionReplyTopicPrefix(reply.TopicID) + fmt.Sprintf("%020d:%s", reply.CreatedAt.UnixNano(), reply.ID)
}

func deleteKeysByPrefix(txn *badger.Txn, prefixValue string) error {
	prefix := []byte(prefixValue)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	keys := make([][]byte, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		keys = append(keys, it.Item().KeyCopy(nil))
	}
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func contextDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
