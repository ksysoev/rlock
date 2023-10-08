// Package rlock provides a distributed locking mechanism using Redis.
package rlock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var releaseLock = redis.NewScript(`
if redis.call("get",KEYS[1]) == ARGV[1] then
    return redis.call("del",KEYS[1])
else
    return 0
end
`)

// ReleaseSignal is a type alias for a string that represents a signal to release a lock.
type ReleaseSignal string

// Locker is a struct that represents a distributed lock manager.
type Locker struct {
	storage   *redis.Client
	nameSpace string
	ctx       context.Context
}

// Lock is a struct that represents a distributed lock.
type Lock struct {
	id       string
	isLocked bool
	key      string
	locker   *Locker
}

// NewLocker creates a new Locker instance.
func NewLocker(ctx context.Context, storage *redis.Client) *Locker {
	return &Locker{ctx: ctx, storage: storage}
}

// TryAcquire tries to acquire a lock for the given key with the specified time-to-live (TTL).
func (l *Locker) TryAcquire(key string, ttl time.Duration) (*Lock, error) {
	id := uuid.NewString()
	success, err := l.storage.SetNX(l.ctx, l.nameSpace+key, id, ttl).Result()

	if err != nil {
		return nil, err
	}

	if !success {
		return nil, fmt.Errorf("can't acquire lock")
	}

	return newLock(key, l, id), nil
}

// release releases the lock for the given key.
func (l *Locker) release(key string, id string) error {
	_, err := releaseLock.Run(l.ctx, l.storage, []string{l.nameSpace + key}, id).Int()
	return err
}

// newLock creates a new Lock instance.
func newLock(key string, locker *Locker, id string) *Lock {
	return &Lock{isLocked: true, key: key, locker: locker, id: id}
}

// Release releases the lock.
// If the lock is already released, it returns an error.
func (l *Lock) Release() error {
	if l.isLocked {
		err := l.locker.release(l.key, l.id)

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
