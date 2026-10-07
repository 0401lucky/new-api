// Package groupload tracks admitted model work, independently of pricing mode.
package groupload

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const leaseTTL = 120 * time.Second

var ErrUnavailable = errors.New("group load unavailable")

var memory = struct {
	sync.Mutex
	groups map[string]map[string]time.Time
	closed map[string]time.Time
}{groups: make(map[string]map[string]time.Time), closed: make(map[string]time.Time)}

// Redis TIME keeps instances on the same clock. Register + prune + count is
// atomic, so the returned count includes exactly this admission.
var registerScript = redis.NewScript(`
local now = tonumber(redis.call('TIME')[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
if ARGV[1] ~= '' and redis.call('EXISTS', KEYS[2]) == 0 then
  redis.call('ZADD', KEYS[1], now + tonumber(ARGV[2]), ARGV[1])
end
local last = redis.call('ZRANGE', KEYS[1], -1, -1, 'WITHSCORES')
if #last > 0 then redis.call('EXPIREAT', KEYS[1], tonumber(last[2]) + 1) end
return redis.call('ZCARD', KEYS[1])
`)

func key(group string) string { return fmt.Sprintf("group-load:%x", sha256.Sum256([]byte(group))) }

func Register(group, slot string, ttl time.Duration) (int64, error) {
	if common.RedisEnabled {
		if common.RDB == nil {
			return 0, errors.New("Redis is not initialized")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return registerScript.Run(ctx, common.RDB, []string{key(group), key(group) + ":closed:" + slot}, slot, max(1, int64(ttl/time.Second))).Int64()
	}
	memory.Lock()
	defer memory.Unlock()
	now := time.Now()
	for id, expiry := range memory.closed {
		if !expiry.After(now) {
			delete(memory.closed, id)
		}
	}
	slots := memory.groups[group]
	for id, expiry := range slots {
		if !expiry.After(now) {
			delete(slots, id)
		}
	}
	if slot != "" && !memory.closed[group+":"+slot].After(now) {
		if slots == nil {
			slots = make(map[string]time.Time)
			memory.groups[group] = slots
		}
		slots[slot] = now.Add(ttl)
	}
	count := int64(len(slots))
	if count == 0 {
		delete(memory.groups, group)
	}
	return count, nil
}

func Count(group string) (int64, error) { return Register(group, "", 0) }

func Release(group, slot string) error {
	if common.RedisEnabled {
		if common.RDB == nil {
			return errors.New("Redis is not initialized")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := common.RDB.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key(group)+":closed:"+slot, "1", leaseTTL)
			pipe.ZRem(ctx, key(group), slot)
			return nil
		})
		return err
	}
	memory.Lock()
	defer memory.Unlock()
	delete(memory.groups[group], slot)
	memory.closed[group+":"+slot] = time.Now().Add(leaseTTL)
	if len(memory.groups[group]) == 0 {
		delete(memory.groups, group)
	}
	return nil
}

type Lease struct {
	Group   string
	Slot    string
	done    chan struct{}
	stopped chan struct{}
	once    sync.Once
	// Durable ownership moves to the persisted task; Close then only stops
	// the HTTP heartbeat. Task completion or expiry removes the slot.
	Durable bool
}

func Acquire(group string) (*Lease, int64, error) {
	l := &Lease{Group: group, Slot: common.GetUUID(), done: make(chan struct{}), stopped: make(chan struct{})}
	count, err := Register(group, l.Slot, leaseTTL)
	if err != nil {
		return nil, 0, err
	}
	go func() {
		defer close(l.stopped)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-l.done:
				return
			case <-ticker.C:
				if _, err := Register(l.Group, l.Slot, leaseTTL); err != nil {
					common.SysError("group load heartbeat: " + err.Error())
				}
			}
		}
	}()
	return l, count, nil
}

func (l *Lease) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		close(l.done)
		<-l.stopped
		if !l.Durable {
			if err := Release(l.Group, l.Slot); err != nil {
				common.SysError("release group load: " + err.Error())
			}
		}
	})
}

func (l *Lease) Transfer(ttl time.Duration) error {
	if l == nil {
		return nil
	}
	// Stop the short HTTP heartbeat before extending the durable lease.
	l.Durable = true
	l.Close()
	_, err := Register(l.Group, l.Slot, ttl)
	return err
}
