// Package rlock provides a distributed locking mechanism using Redis.
package rlock

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReleaseSignal is a type alias for a string that represents a signal to release a lock.
type ReleaseSignal string

// RedisStore is an interface that defines the Redis commands used by the Locker.
type RedisStore interface {
	// SetNX sets the value of a key, only if the key does not exist.
	// It returns true if the key was set, false otherwise.
	SetNX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd
	// Del deletes one or more keys.
	// It returns the number of keys that were deleted.
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Locker is a struct that represents a distributed lock manager.
type Locker struct {
	storage   RedisStore
	nameSpace string
	ctx       context.Context
}

// Lock is a struct that represents a distributed lock.
type Lock struct {
	isLocked bool
	key      string
	locker   *Locker
}

// NewLocker creates a new Locker instance.
func NewLocker(ctx context.Context, storage RedisStore) *Locker {
	return &Locker{ctx: ctx, storage: storage}
}

// TryAcquire tries to acquire a lock for the given key with the specified time-to-live (TTL).
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

// release releases the lock for the given key.
func (l *Locker) release(key string) error {
	_, err := l.storage.Del(l.ctx, l.nameSpace+key).Result()
	return err
}

// newLock creates a new Lock instance.
func newLock(key string, locker *Locker) *Lock {
	return &Lock{isLocked: true, key: key, locker: locker}
}

// Release releases the lock.
// If the lock is already released, it returns an error.
func (l *Lock) Release() error {
	if l.isLocked {
		err := l.locker.release(l.key)

		if err != nil {
			return err
		}

		l.isLocked = false
		return nil
	}

	return fmt.Errorf("lock is already released")
}

// IsLocked returns true if the lock is still held, false otherwise.
func (l *Lock) IsLocked() bool {
	return l.isLocked
}
