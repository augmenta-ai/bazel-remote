package s3proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/buchgr/bazel-remote/v2/cache"
	"github.com/buchgr/bazel-remote/v2/cache/disk/casblob"
	"github.com/buchgr/bazel-remote/v2/utils/backendproxy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type s3Cache struct {
	s3Client         *s3.Client
	prefix           string
	bucket           string
	uploadQueue      chan<- backendproxy.UploadReq
	accessLogger     cache.Logger
	errorLogger      cache.Logger
	v2mode           bool
	updateTimestamps bool
	objectKey        func(hash string, kind cache.EntryKind) string
}

var (
	cacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bazel_remote_s3_cache_hits",
		Help: "The total number of s3 backend cache hits",
	})
	cacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bazel_remote_s3_cache_misses",
		Help: "The total number of s3 backend cache misses",
	})
)

// Used in place of minio's verbose "NoSuchKey" error.
var errNotFound = errors.New("NOT FOUND")

// New returns a new instance of the S3-API based cache
func New(
	Endpoint string,
	Bucket string,
	Prefix string,
	DisableSSL bool,
	UpdateTimestamps bool,
	Region string,

	storageMode string, accessLogger cache.Logger,
	errorLogger cache.Logger, numUploaders, maxQueuedUploads int) cache.Proxy {

	fmt.Println("Using S3 backend with AWS SDK.")

	ctx := context.Background()

	// Load AWS configuration using the default credential chain
	// This supports: environment variables, shared credentials file, IAM roles, SSO, etc.
	var opts []func(*config.LoadOptions) error

	if Region != "" {
		opts = append(opts, config.WithRegion(Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create S3 client
	s3ClientOpts := func(o *s3.Options) {
		if Endpoint != "" {
			o.BaseEndpoint = aws.String(Endpoint)
			// For custom endpoints, use path-style addressing
			o.UsePathStyle = true
		}
		if DisableSSL {
			// Note: AWS SDK v2 doesn't have a direct DisableSSL option
			// Custom endpoints should use http:// scheme
			log.Println("Warning: disable_ssl is deprecated. Use http:// in endpoint instead.")
		}
	}

	s3Client := s3.NewFromConfig(cfg, s3ClientOpts)

	if storageMode != "zstd" && storageMode != "uncompressed" {
		log.Fatalf("Unsupported storage mode for the s3proxy backend: %q, must be one of \"zstd\" or \"uncompressed\"",
			storageMode)
	}

	c := &s3Cache{
		s3Client:         s3Client,
		prefix:           Prefix,
		bucket:           Bucket,
		accessLogger:     accessLogger,
		errorLogger:      errorLogger,
		v2mode:           storageMode == "zstd",
		updateTimestamps: UpdateTimestamps,
	}

	if c.v2mode {
		c.objectKey = func(hash string, kind cache.EntryKind) string {
			return objectKeyV2(c.prefix, hash, kind)
		}
	} else {
		c.objectKey = func(hash string, kind cache.EntryKind) string {
			return objectKeyV1(c.prefix, hash, kind)
		}
	}

	c.uploadQueue = backendproxy.StartUploaders(c, numUploaders, maxQueuedUploads)

	return c
}

func objectKeyV2(prefix string, hash string, kind cache.EntryKind) string {
	var baseKey string
	if kind == cache.CAS {
		// Use "cas.v2" to distinguish new from old format blobs.
		baseKey = path.Join("cas.v2", hash[:2], hash)
	} else {
		baseKey = path.Join(kind.String(), hash[:2], hash)
	}

	if prefix == "" {
		return baseKey
	}

	return path.Join(prefix, baseKey)
}

func objectKeyV1(prefix string, hash string, kind cache.EntryKind) string {
	if prefix == "" {
		return path.Join(kind.String(), hash[:2], hash)
	}

	return path.Join(prefix, kind.String(), hash[:2], hash)
}

// Helper function for logging responses
func logResponse(log cache.Logger, method, bucket, key string, err error) {
	status := "OK"
	if err != nil {
		status = err.Error()
	}

	log.Printf("S3 %s %s %s %s", method, bucket, key, status)
}

func (c *s3Cache) UploadFile(item backendproxy.UploadReq) {
	ctx := context.Background()
	
	contentType := "application/octet-stream"
	
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(c.objectKey(item.Hash, item.Kind)),
		Body:        item.Rc,
		ContentType: aws.String(contentType),
	})

	logResponse(c.accessLogger, "UPLOAD", c.bucket, c.objectKey(item.Hash, item.Kind), err)

	_ = item.Rc.Close()
}

func (c *s3Cache) Put(ctx context.Context, kind cache.EntryKind, hash string, logicalSize int64, sizeOnDisk int64, rc io.ReadCloser) {
	if c.uploadQueue == nil {
		_ = rc.Close()
		return
	}

	select {
	case c.uploadQueue <- backendproxy.UploadReq{
		Hash:        hash,
		LogicalSize: logicalSize,
		SizeOnDisk:  sizeOnDisk,
		Kind:        kind,
		Rc:          rc,
	}:
	default:
		c.errorLogger.Printf("too many uploads queued\n")
		_ = rc.Close()
	}
}

func (c *s3Cache) UpdateModificationTimestamp(ctx context.Context, bucket string, object string) {
	// Copy the object to itself to update the modification timestamp
	copySource := fmt.Sprintf("%s/%s", bucket, object)
	
	_, err := c.s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(object),
		CopySource: aws.String(copySource),
	})

	logResponse(c.accessLogger, "COPY", bucket, object, err)
}

func (c *s3Cache) Get(ctx context.Context, kind cache.EntryKind, hash string, _ int64) (io.ReadCloser, int64, error) {

	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.objectKey(hash, kind)),
	})
	
	if err != nil {
		// Check if error is NoSuchKey
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "NoSuchKey" {
				cacheMisses.Inc()
				logResponse(c.accessLogger, "DOWNLOAD", c.bucket, c.objectKey(hash, kind), errNotFound)
				return nil, -1, nil
			}
		}
		cacheMisses.Inc()
		logResponse(c.accessLogger, "DOWNLOAD", c.bucket, c.objectKey(hash, kind), err)
		return nil, -1, err
	}
	cacheHits.Inc()

	if c.updateTimestamps {
		c.UpdateModificationTimestamp(ctx, c.bucket, c.objectKey(hash, kind))
	}

	logResponse(c.accessLogger, "DOWNLOAD", c.bucket, c.objectKey(hash, kind), nil)

	if kind == cache.CAS && c.v2mode {
		return casblob.ExtractLogicalSize(result.Body)
	}

	size := int64(-1)
	if result.ContentLength != nil {
		size = *result.ContentLength
	}

	return result.Body, size, nil
}

func (c *s3Cache) Contains(ctx context.Context, kind cache.EntryKind, hash string, _ int64) (bool, int64) {
	size := int64(-1)
	exists := false

	result, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.objectKey(hash, kind)),
	})

	exists = (err == nil)
	if err != nil {
		err = errNotFound
	} else if kind != cache.CAS || !c.v2mode {
		if result.ContentLength != nil {
			size = *result.ContentLength
		}
	}

	logResponse(c.accessLogger, "CONTAINS", c.bucket, c.objectKey(hash, kind), err)

	return exists, size
}
