package backup

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectInfo is one remote object listing entry.
type ObjectInfo struct {
	Key     string
	Size    int64
	ModTime time.Time
}

// ObjectStore is the remote-destination seam (consumer-side). minioStore is
// the production implementation; tests inject fakes. Kept deliberately
// small: put, list, delete, ping is all a backup destination needs.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// CredentialResolver mirrors the daemon's env-then-vault secret resolution.
type CredentialResolver func(name string) (string, bool)

// ObjectKey is the remote object name for an archive created at t:
// <prefix>/heka-backup-<stamp>.zip.
func (s S3Config) ObjectKey(name string) string {
	return path.Join(s.Prefix, name)
}

// BuildStore constructs the S3 client for this destination, resolving
// credentials through the vault-backed resolver. Missing credentials are an
// error — the S3 config itself is not secret, the keys are.
func (s S3Config) BuildStore(resolver CredentialResolver) (ObjectStore, error) {
	accessKey, _ := resolver(SecretS3AccessKeyID)
	secretKey, _ := resolver(SecretS3SecretAccessKey)
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3: credentials missing from the vault — store %s and %s as secrets first",
			SecretS3AccessKeyID, SecretS3SecretAccessKey)
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	client, err := minio.New(s.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: s.UseSSL,
		Region: s.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: client: %w", err)
	}
	return &minioStore{client: client, bucket: s.Bucket, secure: s.UseSSL}, nil
}

// TestConnection verifies config + credentials end to end: the bucket must
// exist and a probe object must round-trip (write then delete), which also
// surfaces missing write permissions that a bare BucketExists hides.
func (s S3Config) TestConnection(ctx context.Context, resolver CredentialResolver) error {
	store, err := s.BuildStore(resolver)
	if err != nil {
		return err
	}
	if err := store.Ping(ctx); err != nil {
		return err
	}
	probeKey := s.ObjectKey(".heka-probe")
	probe := strings.NewReader("heka connectivity probe")
	if err := store.Put(ctx, probeKey, probe, int64(probe.Len())); err != nil {
		return fmt.Errorf("s3: write probe failed: %w", err)
	}
	if err := store.Delete(ctx, probeKey); err != nil {
		return fmt.Errorf("s3: probe cleanup failed (backup upload will still work): %w", err)
	}
	return nil
}

// minioStore adapts minio-go to ObjectStore. minio-go speaks the AWS SigV4
// API against any S3-compatible endpoint (AWS, Cloudflare R2 — endpoint
// https://<account>.r2.cloudflarestorage.com with region "auto", Backblaze
// B2, MinIO) and defaults to path-style addressing, which R2 requires.
type minioStore struct {
	client *minio.Client
	bucket string
	secure bool
}

// friendlyS3Error rewrites opaque redirect responses into actionable hints.
// Plain HTTP against remote endpoints (Cloudflare in particular) answers a
// bare 301 HTML page, which otherwise surfaces as unreadable markup.
func friendlyS3Error(err error, secure bool) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "301") || strings.Contains(msg, "Moved Permanently") {
		if secure {
			return fmt.Errorf("s3: the endpoint redirected the request (301) even over HTTPS — the host must be exactly the storage endpoint (e.g. <account>.r2.cloudflarestorage.com, no bucket name, no path, no scheme): %w", err)
		}
		return fmt.Errorf("s3: the endpoint redirected the request (301) — plain HTTP is not accepted; enable \"Use HTTPS\": %w", err)
	}
	return err
}

func (m *minioStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, r, size,
		minio.PutObjectOptions{ContentType: "application/zip"})
	return friendlyS3Error(err, m.secure)
}

func (m *minioStore) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	for obj := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, friendlyS3Error(obj.Err, m.secure)
		}
		out = append(out, ObjectInfo{Key: obj.Key, Size: obj.Size, ModTime: obj.LastModified})
	}
	return out, nil
}

func (m *minioStore) Delete(ctx context.Context, key string) error {
	return friendlyS3Error(m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}), m.secure)
}

func (m *minioStore) Ping(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return friendlyS3Error(err, m.secure)
	}
	if !exists {
		return fmt.Errorf("s3: bucket %q does not exist (or credentials lack access)", m.bucket)
	}
	return nil
}

// PruneRemote keeps only the newest keep backup archives under prefix,
// deleting older heka-backup-*.zip objects. Foreign objects sharing the
// prefix are never touched. Returns the number of removals.
func PruneRemote(ctx context.Context, store ObjectStore, prefix string, keep int) (int, error) {
	if keep < 1 {
		return 0, nil
	}
	objects, err := store.List(ctx, prefix)
	if err != nil {
		return 0, err
	}
	var archives []ObjectInfo
	for _, o := range objects {
		name := path.Base(o.Key)
		if strings.HasPrefix(name, "heka-backup-") && strings.HasSuffix(name, ".zip") {
			archives = append(archives, o)
		}
	}
	sort.Slice(archives, func(i, j int) bool {
		if archives[i].ModTime.Equal(archives[j].ModTime) {
			return archives[i].Key > archives[j].Key // stable tie-break: newer stamp
		}
		return archives[i].ModTime.After(archives[j].ModTime)
	})
	removed := 0
	for _, o := range archives[min(keep, len(archives)):] {
		if err := store.Delete(ctx, o.Key); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
