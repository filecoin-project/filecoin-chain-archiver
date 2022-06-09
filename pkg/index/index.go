package index

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/ipfs/go-log/v2"
)

var (
	expiryLength = 5 * time.Minute
)

var logger = log.Logger("filecoin-chain-archiver/pkg/index-resolver")

type IndexS3Resolver struct {
	client s3ClientInterface
	bucket string
}

type s3ClientInterface interface {
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
}

type Resolver interface {
	Resolve(context.Context, string) (string, error)
}

func NewIndexS3Resolver(client s3ClientInterface, bucket string) *IndexS3Resolver {
	return &IndexS3Resolver{
		client: client,
		bucket: bucket,
	}
}

func (i *IndexS3Resolver) Resolve(ctx context.Context, obj string) (string, error) {
	objInfo, err := i.client.StatObject(ctx, i.bucket, obj, minio.StatObjectOptions{})
	if err != nil {
		return "", err
	}

	if v, ok := objInfo.Metadata["X-Amz-Website-Redirect-Location"]; ok {
		return v[0], nil
	}

	return "", fmt.Errorf("failed to resolve link")
}

type cacheMetadata struct {
	value  string
	expiry time.Time
}

type CachedResolver struct {
	resolver Resolver

	cacheMu sync.Mutex
	cache   map[string]cacheMetadata
}

func NewCachedResolver(resolver Resolver) *CachedResolver {
	return &CachedResolver{
		resolver: resolver,
		cache:    make(map[string]cacheMetadata),
	}
}

func (i *CachedResolver) read(obj string) (string, bool) {
	i.cacheMu.Lock()
	defer i.cacheMu.Unlock()
	if v, ok := i.cache[obj]; ok {
		if time.Now().Before(v.expiry) {
			return v.value, true
		}
	}

	return "", false
}

func (i *CachedResolver) set(obj, value string, expiry time.Time) {
	i.cacheMu.Lock()
	defer i.cacheMu.Unlock()
	i.cache[obj] = cacheMetadata{
		expiry: expiry,
		value:  value,
	}
}

func (i *CachedResolver) Resolve(ctx context.Context, obj string) (string, error) {
	if v, ok := i.read(obj); ok {
		logger.Debugw("cache hit")
		return v, nil
	}

	value, err := i.resolver.Resolve(ctx, obj)
	if err != nil {
		return "", err
	}

	logger.Debugw("cache miss")
	i.set(obj, value, time.Now().Add(expiryLength))

	return value, nil
}
