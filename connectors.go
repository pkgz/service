package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// NewMongo - initialize mongo-driver client. Also pinging mongo.
func NewMongo(ctx context.Context, host string) (*mongo.Client, error) {
	if host == "" {
		host = "mongodb://localhost:27017"
	}

	client, err := mongo.Connect(options.Client().ApplyURI(host))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	return client, nil
}

// NewRedis - initialize a redis Ring across the given hosts. Also pinging redis.
func NewRedis(ctx context.Context, host []string, password string) (*redis.Ring, error) {
	addr := make(map[string]string)
	for i, h := range host {
		addr[fmt.Sprintf("server_%d", i+1)] = h
	}
	conn := redis.NewRing(&redis.RingOptions{
		NewClient: func(opt *redis.Options) *redis.Client {
			opt.Password = password
			return redis.NewClient(opt)
		},
		Addrs: addr,
	})

	if err := conn.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping error: %w", err)
	}

	return conn, nil
}

// NewInflux - initialize an influxdb client. Also pinging influx and verifying it reports ready.
func NewInflux(ctx context.Context, host, token string, opts *influxdb2.Options) (influxdb2.Client, error) {
	client := influxdb2.NewClientWithOptions(host, token, opts)
	if _, err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping influx: %w", err)
	}

	ready, err := client.Ready(ctx)
	if err != nil {
		return nil, fmt.Errorf("influx client: %w", err)
	}
	if ready.Status == nil {
		return nil, errors.New("influx client ready status is nil")
	}
	if *ready.Status != domain.ReadyStatusReady {
		return nil, fmt.Errorf("influx client is not ready: %s", *ready.Status)
	}

	return client, nil
}
