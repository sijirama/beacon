package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb *redis.Client
}

func NewStore(redisURL string) (*Store, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &Store{rdb: redis.NewClient(opt)}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

func (s *Store) Close() error {
	return s.rdb.Close()
}

// --- Magic links ---
//
// magic:<token> -> email   with TTL = MagicLinkTTL
// Single-use: deleted on verify.

func magicKey(token string) string { return "magic:" + token }

func (s *Store) CreateMagicLink(ctx context.Context, email string, ttl time.Duration) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, magicKey(token), strings.ToLower(email), ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) ConsumeMagicLink(ctx context.Context, token string) (string, error) {
	email, err := s.rdb.Get(ctx, magicKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrInvalidMagicLink
	}
	if err != nil {
		return "", err
	}
	if err := s.rdb.Del(ctx, magicKey(token)).Err(); err != nil {
		return "", err
	}
	return email, nil
}

// --- Sessions ---
//
// session:<sid> -> email   with TTL = SessionTTL (sliding)

func sessionKey(sid string) string { return "session:" + sid }

func (s *Store) CreateSession(ctx context.Context, email string, ttl time.Duration) (string, error) {
	sid, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, sessionKey(sid), strings.ToLower(email), ttl).Err(); err != nil {
		return "", err
	}
	return sid, nil
}

func (s *Store) GetSession(ctx context.Context, sid string, slidingTTL time.Duration) (string, error) {
	email, err := s.rdb.Get(ctx, sessionKey(sid)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", err
	}
	_ = s.rdb.Expire(ctx, sessionKey(sid), slidingTTL).Err()
	return email, nil
}

func (s *Store) DeleteSession(ctx context.Context, sid string) error {
	return s.rdb.Del(ctx, sessionKey(sid)).Err()
}

// --- helpers ---

var (
	ErrInvalidMagicLink = errors.New("magic link invalid or expired")
	ErrSessionNotFound  = errors.New("session not found")
)

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
