package rlock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ReleaseSignal string

type RedisStore interface {
	SetNX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Locker struct {
	storage   RedisStore
	nameSpace string
	ctx       context.Context
}

type Lock struct {
	isLocked bool
	key      string
	locker   *Locker
}

func NewLocker(ctx context.Context, storage RedisStore) *Locker {
	return &Locker{ctx: ctx, storage: storage}
}

func (l *Locker) TryAcquire(key string, ttl time.Duration) (*Lock, error) {
	success, err := l.storage.SetNX(l.ctx, l.nameSpace+key, "1", ttl).Result()

	if err != nil {
		return nil, err
	}

	if !success {
		return nil, fmt.Errorf("can't acquire lock")
	}

	return newLock(key, l), nil
}

func (l *Locker) release(key string) error {
	_, err := l.storage.Del(l.ctx, l.nameSpace+key).Result()
	return err
}

func newLock(key string, locker *Locker) *Lock {
	return &Lock{isLocked: true, key: key, locker: locker}
}

func (l *Lock) Release() error {
	if l.isLocked {
		err := l.locker.release(l.key)

		if err != nil {
			return err
		}

		l.isLocked = false
		return nil
	}

	return fmt.Errorf("lock is alread released")
}

func (l *Lock) IsLocked() bool {
	return l.isLocked
}
