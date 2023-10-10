# rlock
rlock is a Redis-based distributed locking library for Go. It provides a simple API for acquiring and releasing locks, and uses Redis to ensure that locks are properly distributed across multiple processes.

## Installation

To install rlock, use `go get`:

```sh
go get github.com/ksysoev/rlock
```

## Usage

Here's an example of how to use rlock to acquire and release a lock:

```golang
redisClient := redis.NewClient(getRedisOptions())
l := rlock.NewLocker(context.Background(), redisClient)

lock, err := l.TryAcquire("my-lock", 1*time.Second)
if err != nil {
    fmt.Println("Unable to acquire lock:", err)
}

// It'll wait untill lock will be relesed or timeout is reach. 
lock1, err := l.Acquire("my-lock", 1*time.Second, 2*time.Second)
if err != nil {
    fmt.Println("Unable to acquire lock:", err)
}
defer lock1.Release()
```

## Contributing

Contributions to rlock are welcome!

## License

rlock is licensed under the MIT License
