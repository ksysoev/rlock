package rlock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ReleaseSignal string

type RedisStore interface {
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) *redis.BoolCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Locker struct {
	storage   RedisStore
	nameSpace string
	release   chan ReleaseSignal
	ctx       context.Context
}

type Lock struct {
	isLocked bool
	key      string
	release  chan ReleaseSignal
}

func NewLocker(ctx context.Context, storage RedisStore) *Locker {
	release := make(chan ReleaseSignal)
	locker := &Locker{ctx: ctx, storage: storage, release: release}

	go func(locker *Locker) {
		for releaseLock := range locker.release {
			storage.Del(locker.ctx, locker.nameSpace+string(releaseLock))
		}
	}(locker)

	return &Locker{storage: storage, release: release}
}

func (l *Locker) TryAcquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	success, err := l.storage.SetNX(ctx, l.nameSpace+key, "1", ttl).Result()

	if err != nil {
		return nil, err
	}

	if !success {
		return nil, fmt.Errorf("can't acquire lock")
	}

	return newLock(key, l.release), nil
}

func newLock(key string, release chan ReleaseSignal) *Lock {
	return &Lock{isLocked: true, key: key, release: release}
}

func (l *Lock) Release() error {
	if l.isLocked {
		l.release <- ReleaseSignal(l.key)
		return nil
	}

	return fmt.Errorf("lock is alread released")
}

func (l *Lock) IsLocked() bool {
	return l.isLocked
}
