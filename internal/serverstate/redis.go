package serverstate

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// redisStore implements Store backed by a Redis instance.
type redisStore struct {
	client redis.UniversalClient
	key    string
	ctx    context.Context
}

const redisKey = "nfrx:state"

// NewRedisStore connects to the given Redis URL and returns a Store.
// The underlying key is initialized to a default state if it does not exist.
func NewRedisStore(addr string) (*redisStore, error) {
	opts, err := parseRedisURL(addr)
	if err != nil {
		return nil, err
	}
	c := redis.NewUniversalClient(opts)
	rs := &redisStore{client: c, key: redisKey, ctx: context.Background()}
	if err := c.Ping(rs.ctx).Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(State{Status: "not_ready"})
	_ = c.SetNX(rs.ctx, rs.key, b, 0).Err()
	return rs, nil
}

// parseRedisURL parses addr into UniversalOptions supporting single, cluster,
// and sentinel Redis deployments. If no scheme is present, addr is treated as
// a plain host:port string.
func parseRedisURL(addr string) (*redis.UniversalOptions, error) {
	if !strings.Contains(addr, "://") {
		return &redis.UniversalOptions{Addrs: []string{addr}}, nil
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	opts := &redis.UniversalOptions{}
	if u.User != nil {
		opts.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			opts.Password = pw
		}
	}
	opts.Addrs = strings.Split(u.Host, ",")

	q := u.Query()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	switch u.Scheme {
	case "redis", "rediss":
		if u.Path != "" && u.Path != "/" {
			if db, err := strconv.Atoi(strings.TrimPrefix(u.Path, "/")); err == nil {
				opts.DB = db
			} else {
				return nil, fmt.Errorf("redis: invalid db: %v", err)
			}
		} else if dbStr := q.Get("db"); dbStr != "" {
			if db, err := strconv.Atoi(dbStr); err == nil {
				opts.DB = db
			} else {
				return nil, fmt.Errorf("redis: invalid db: %v", err)
			}
		}
		if u.Scheme == "rediss" {
			opts.TLSConfig = tlsCfg
		}
	case "redis-sentinel", "rediss-sentinel":
		opts.MasterName = strings.TrimPrefix(u.Path, "/")
		if dbStr := q.Get("db"); dbStr != "" {
			if db, err := strconv.Atoi(dbStr); err == nil {
				opts.DB = db
			} else {
				return nil, fmt.Errorf("redis: invalid db: %v", err)
			}
		}
		if v := q.Get("sentinel_username"); v != "" {
			opts.SentinelUsername = v
		}
		if v := q.Get("sentinel_password"); v != "" {
			opts.SentinelPassword = v
		}
		if u.Scheme == "rediss-sentinel" {
			opts.TLSConfig = tlsCfg
		}
	default:
		return nil, fmt.Errorf("redis: invalid URL scheme: %s", u.Scheme)
	}

	return opts, nil
}

func (r *redisStore) Load() State {
	b, err := r.client.Get(r.ctx, r.key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return State{Status: "not_ready"}
		}
		return State{Status: "unknown"}
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{Status: "unknown"}
	}
	return st
}

func (r *redisStore) Store(s State) {
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = r.client.Set(r.ctx, r.key, b, 0).Err()
}
