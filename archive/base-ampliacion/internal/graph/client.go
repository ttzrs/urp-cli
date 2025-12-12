package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Client struct {
	driver neo4j.DriverWithContext
	dbName string
}

func NewClient(uri, username, password string) (*Client, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create driver: %w", err)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		return nil, fmt.Errorf("failed to verify connectivity: %w", err)
	}

	return &Client{
		driver: driver,
		dbName: "memgraph", 
	}, nil
}

func (c *Client) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

func (c *Client) RunQuery(ctx context.Context, query string, params map[string]any) (neo4j.ResultWithContext, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	return session.Run(ctx, query, params)
}
