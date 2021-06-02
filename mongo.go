package service

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"time"
)

// NewMongo - initialize mongo-driver client. Also pinging mongo.
func NewMongo(ctx context.Context, host string) (*mongo.Client, error) {
	if host == "" {
		host = "mongodb://localhost:27017"
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(host))
	if err != nil {
		return nil, err
	}

	ctx_, _ := context.WithTimeout(ctx, 5*time.Second)
	err = client.Connect(ctx_)
	if err != nil {
		return nil, err
	}

	ctx_, _ = context.WithTimeout(ctx, 3*time.Second)
	err = client.Ping(ctx_, readpref.Primary())
	if err != nil {
		return nil, err
	}

	return client, nil
}
