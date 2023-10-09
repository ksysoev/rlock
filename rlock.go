// Package rlock provides a distributed locking mechanism using Redis.
package rlock

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// DefaultNameSpace is the default namespace for locks.
	DefaultNameSpace       = "rlock:"
	DefaultMaxInterval     = 5 * time.Second
	DefaultInitialInterval = 500 * time.Millisecond
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
	storage         *redis.Client
	nameSpace       string
	ctx             context.Context
	MaxInterval     time.Duration
	InitialInterval time.Duration
}

// Lock is a struct that represents a distributed lock.
type Lock struct {
	id       string
	isLocked bool
	key      string
	locker   *Locker
	ttl      time.Duration
}

// NewLocker creates a new Locker instance.
func NewLocker(ctx context.Context, storage *redis.Client) *Locker {
	return &Locker{
		ctx:             ctx,
		storage:         storage,
		MaxInterval:     DefaultMaxInterval,
		InitialInterval: DefaultInitialInterval}
}

func (l *Locker) SetNameSpace(nameSpace string) *Locker {
	l.nameSpace = nameSpace
	return l
}

func (l *Locker) SetMaxInterval(maxInterval time.Duration) *Locker {
	l.MaxInterval = maxInterval
	return l
}

func (l *Locker) SetInitialInterval(initialInterval time.Duration) *Locker {
	l.InitialInterval = initialInterval
	return l
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

	return newLock(key, l, id, ttl), nil
}

func (l *Locker) Acquire(key string, ttl time.Duration, timeout time.Duration) (*Lock, error) {
	id := uuid.NewString()

	context, _ := context.WithTimeout(l.ctx, timeout)

	backoff := backoff.NewExponentialBackOff()
	backoff.MaxElapsedTime = l.MaxInterval
	backoff.InitialInterval = l.InitialInterval
	backoff.Reset()
	for {

		success, err := l.storage.SetNX(l.ctx, l.nameSpace+key, id, ttl).Result()

		if err != nil {
			return nil, err
		}

		if success {
			return newLock(key, l, id, ttl), nil
		}

		intr := backoff.NextBackOff()

		select {
		case <-context.Done():
			return nil, fmt.Errorf("can't acquire lock")
		case <-time.After(intr):
			continue
		}
	}
}

// release releases the lock for the given key.
func (l *Locker) release(lock *Lock) error {
	_, err := releaseLock.Run(l.ctx, l.storage, []string{l.nameSpace + lock.key}, lock.id).Int()
	return err
}

func (l *Locker) refresh(lock *Lock) (bool, error) {
	res, err := refreshLock.Run(l.ctx, l.storage, []string{l.nameSpace + lock.key}, lock.id, lock.ttl.Abs().Milliseconds()).Int()

	if err != nil {
		return false, err
	}

	if res == 0 {
		return false, nil
	}

	return true, nil
}

// newLock creates a new Lock instance.
func newLock(key string, locker *Locker, id string, ttl time.Duration) *Lock {
	return &Lock{isLocked: true, key: key, locker: locker, id: id, ttl: ttl}
}

// Release releases the lock.
// If the lock is already released, it returns an error.
func (l *Lock) Release() error {
	if l.isLocked {
		err := l.locker.release(l)

		if err != nil {
			return err
		}

		l.isLocked = false
		return nil
	}

	return fmt.Errorf("lock is already released")
}

func (l *Lock) TryRefresh() error {
	res, err := l.locker.refresh(l)

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
