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

var refreshLock = redis.NewScript(`
local k, i, t = KEYS[1], ARGV[1], ARGV[2]
if redis.call("EXISTS", k) == 0 then 
	redis.call("SET",k, i, "PX", t)
	return 1
elseif (redis.call("GET",k) == i) then
	return redis.call("PEXPIRE",k, t)
else
    return 0
end
`)

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

func (l *Locker) refresh(key string, id string, duration time.Duration) (bool, error) {
	res, err := refreshLock.Run(l.ctx, l.storage, []string{l.nameSpace + key}, id, duration.Abs().Milliseconds()).Int()

	if err != nil {
		return false, err
	}

	if res == 0 {
		return false, nil
	}

	return true, nil
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

func (l *Lock) TryRefresh(ttl time.Duration) error {
	res, err := l.locker.refresh(l.key, l.id, ttl)

	if err != nil {
		return err
	}

	l.isLocked = res

	if !res {
		return fmt.Errorf("lock is overtaken")
	}

	return nil
}

// IsLocked returns true if the lock is still held, false otherwise.
func (l *Lock) IsLocked() bool {
	return l.isLocked
}
