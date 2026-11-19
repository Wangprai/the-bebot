package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

func NewMongoDB(ctx context.Context, uri string, databaseName string) (*MongoDB, error) {
	clientOptions := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(50).
		SetServerSelectionTimeout(5 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect to mongodb: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}

	db := client.Database(databaseName)

	return &MongoDB{
		Client:   client,
		Database: db,
	}, nil
}

func (m *MongoDB) Close(ctx context.Context) error {
	if m == nil || m.Client == nil {
		return nil
	}

	return m.Client.Disconnect(ctx)
}
