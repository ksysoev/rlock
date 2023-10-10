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
	DefaultNameSpace = "rlock"
	// DefaultMaxInterval is the default maximum interval between attempts to acquire a lock.
	DefaultMaxInterval = 5 * time.Second
	// DefaultInitialInterval is the default initial interval between attempts to acquire a lock.
	DefaultInitialInterval = 500 * time.Millisecond
)

// releaseLock is a Lua script that releases the lock.
var releaseLock = redis.NewScript(`
if redis.call("get",KEYS[1]) == ARGV[1] then
    return redis.call("del",KEYS[1])
else
    return 0
end
`)

// refreshLock is a Lua script that refreshes the expiration time of a lock.
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
	maxInterval     time.Duration
	initialInterval time.Duration
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
		maxInterval:     DefaultMaxInterval,
		initialInterval: DefaultInitialInterval}
}

// SetNameSpace sets the namespace for locks redis keys that will be use as a prefix spearagted from key by double colon.
func (l *Locker) SetNameSpace(nameSpace string) *Locker {
	l.nameSpace = nameSpace
	return l
}

// SetMaxInterval sets the maximum interval between attempts to acquire a lock.
func (l *Locker) SetMaxInterval(maxInterval time.Duration) *Locker {
	l.maxInterval = maxInterval
	return l
}

// SetInitialInterval sets the initial interval between attempts to acquire a lock.
func (l *Locker) SetInitialInterval(initialInterval time.Duration) *Locker {
	l.initialInterval = initialInterval
	return l
}

// TryAcquire tries to acquire a lock for the given key with the specified time-to-live (TTL).
func (l *Locker) TryAcquire(key string, ttl time.Duration) (*Lock, error) {
	id := uuid.NewString()
	success, err := l.storage.SetNX(l.ctx, l.getRedisKey(key), id, ttl).Result()

	if err != nil {
		return nil, err
	}

	if !success {
		return nil, fmt.Errorf("can't acquire lock")
	}

	return newLock(key, l, id, ttl), nil
}

// Acquire tries to acquire a lock for the given key. It uses SetNX Redis command to set the key-value pair only if the key does not exist. If the lock is acquired successfully, it returns a new Lock instance with the given key, Locker instance, unique ID and time-to-live (TTL). If the lock is not acquired, it retries after a certain interval using exponential backoff algorithm until the timeout is reached. If the timeout is reached, it returns an error.
// Parameters:
// - key: string representing the key to acquire the lock for.
// - ttl: time.Duration representing the time-to-live of the lock.
// - timeout: time.Duration representing the maximum time to wait for the lock to be acquired.
// Returns:
// - *Lock: a new Lock instance if the lock is acquired successfully.
// - error: an error if the lock is not acquired.
func (l *Locker) Acquire(key string, ttl time.Duration, timeout time.Duration) (*Lock, error) {
	id := uuid.NewString()

	context, _ := context.WithTimeout(l.ctx, timeout)

	backoff := backoff.NewExponentialBackOff()
	backoff.MaxElapsedTime = l.maxInterval
	backoff.InitialInterval = l.initialInterval
	backoff.Reset()
	for {

		success, err := l.storage.SetNX(l.ctx, l.getRedisKey(key), id, ttl).Result()

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

// getRedisKey returns the Redis key for the given key.
func (l *Locker) getRedisKey(key string) string {
	return l.nameSpace + "::" + key
}

// release releases the lock for the given key.
func (l *Locker) release(lock *Lock) error {
	keys := []string{l.getRedisKey(lock.key)}
	_, err := releaseLock.Run(l.ctx, l.storage, keys, lock.id).Int()
	return err
}

// refresh is a method of the Locker struct that refreshes the expiration time of a given lock.
// If the lock was successfully refreshed, it returns boolean value is true, otherwise it is false.
// If an error occurred during the operation, the boolean value is false and the error is returned.
func (l *Locker) refresh(lock *Lock) (bool, error) {
	keys := []string{l.getRedisKey(lock.key)}
	res, err := refreshLock.Run(l.ctx, l.storage, keys, lock.id, lock.ttl.Abs().Milliseconds()).Int()

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

// TryRefresh tries to refresh the lock. if lock in redis still belongs to the current instance, it will update ttl.
// If the lock is overtaken, it returns an error.
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
