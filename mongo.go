package service

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"time"
)

// Init - initialize mongo-driver client. Also pinging mongo.
func NewMongo(ctx context.Context, host string) (*mongo.Client, error) {
	if host == "" {
		host = "mongodb://localhost:27017"
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(host))
	if err != nil {
		return nil, errors.Wrap(err, "new client error")
	}

	ctx_, _ := context.WithTimeout(ctx, 5*time.Second)
	err = client.Connect(ctx_)
	if err != nil {
		return nil, errors.Wrap(err, "connect error")
	}

	ctx_, _ = context.WithTimeout(ctx, 3*time.Second)
	err = client.Ping(ctx_, readpref.Primary())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("ping error %s", host))
	}

	return client, nil
}
