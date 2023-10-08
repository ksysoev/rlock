package rlock

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func getRedisOptions() *redis.Options {
	TestRedisHost := os.Getenv("TEST_REDIS_HOST")
	if TestRedisHost == "" {
		TestRedisHost = "localhost"
	}

	TestRedisPort := os.Getenv("TEST_REDIS_PORT")
	if TestRedisPort == "" {
		TestRedisPort = "6379"
	}

	return &redis.Options{Addr: fmt.Sprintf("%s:%s", TestRedisHost, TestRedisPort)}
}

func TestTryAcquireSuccess(t *testing.T) {
	redisClient := redis.NewClient(getRedisOptions())

	l := NewLocker(context.Background(), redisClient)

	lock, err := l.TryAcquire("TestTryAcquireSuccess", 1*time.Second)
	defer lock.Release()

	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	if !lock.isLocked {
		t.Error("Expected to get locked lock, but it's not")
	}
}

func TestTryAcquireLockAlredyLocked(t *testing.T) {
	redisClient := redis.NewClient(getRedisOptions())

	l := NewLocker(context.Background(), redisClient)

	lock, err := l.TryAcquire("TestTryAcquireLockAlredyLocked", 1*time.Second)
	defer lock.Release()

	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	_, err = l.TryAcquire("TestTryAcquireLockAlredyLocked", 1*time.Second)

	if err == nil {
		t.Error("Expected to get error, but got nil")
	} else if err.Error() != "can't acquire lock" {
		t.Error("Expected to get error with message 'can't acquire lock', but got: ", err)
	}
}

func TestReleseLock(t *testing.T) {
	redisClient := redis.NewClient(getRedisOptions())

	l := NewLocker(context.Background(), redisClient)

	lock, err := l.TryAcquire("TestReleseLock", 1*time.Second)

	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	err = lock.Release()
	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	_, err = l.TryAcquire("TestReleseLock", 1*time.Second)

	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}
}

func TestRefreshLock(t *testing.T) {
	redisClient := redis.NewClient(getRedisOptions())

	l := NewLocker(context.Background(), redisClient)

	lock, err := l.TryAcquire("TestRefreshLock", 1*time.Second)

	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	err = lock.Release()
	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	err = lock.Refresh(1 * time.Second)
	if err != nil {
		t.Error("Expected to get no error, but got: ", err)
	}

	_, err = l.TryAcquire("TestRefreshLock", 1*time.Second)

	if err == nil {
		t.Error("Expected to get error, but got nil")
	} else if err.Error() != "can't acquire lock" {
		t.Error("Expected to get error with message 'can't acquire lock', but got: ", err)
	}
}
