// Package r2store wraps Cloudflare R2's S3-compatible API (via the
// official AWS SDK v2 S3 client — R2 accepts standard SigV4-signed S3
// requests, Cloudflare's own docs recommend this exact client) for the
// small set of operations pdlife.app needs: upload and delete one object
// at a time. Not a general-purpose S3 client.
package r2store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	s3      *s3.Client
	bucket  string
	cdnBase string
}

// Config bundles the R2 credentials for one bucket. Endpoint/AccessKeyID/
// SecretAccessKey/Bucket are always required; New returns an error naming
// what's missing rather than guessing. CDNBase is optional — it's only
// needed by Upload's returned URL, and buckets with no public domain in
// front of them (e.g. the private backups bucket used by cmd/db_backup,
// see BackupConfigFromEnv) never set it.
type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	CDNBase         string
}

func (c Config) missing() []string {
	var m []string
	if strings.TrimSpace(c.Endpoint) == "" {
		m = append(m, "Endpoint")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		m = append(m, "AccessKeyID")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		m = append(m, "SecretAccessKey")
	}
	if strings.TrimSpace(c.Bucket) == "" {
		m = append(m, "Bucket")
	}
	return m
}

// New builds a Client, or an error listing exactly which Config fields
// were empty.
func New(cfg Config) (*Client, error) {
	if missing := cfg.missing(); len(missing) > 0 {
		return nil, fmt.Errorf("r2store: missing config: %s", strings.Join(missing, ", "))
	}

	client := s3.New(s3.Options{
		Region:       "auto", // Cloudflare R2's SigV4 region is always "auto"
		BaseEndpoint: aws.String(strings.TrimSuffix(cfg.Endpoint, "/")),
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})

	return &Client{
		s3:      client,
		bucket:  cfg.Bucket,
		cdnBase: strings.TrimSuffix(cfg.CDNBase, "/"),
	}, nil
}

// Upload PUTs data at key and returns the public CDN URL
// (cdnBase + "/" + key), with a cache-busting "?v=" query param. The CDN
// in front of R2 caches by full URL including the query string (observed
// max-age=14400s / Cf-Cache-Status HIT in production), but object keys
// here are deterministic (pdlife/news/{source}/{externalID}.png) and get
// overwritten in place by the "regenerate" flow — without a cache-buster,
// the CDN edge keeps serving the pre-regenerate bytes at the same URL for
// up to 4 hours after a successful regenerate.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("r2store: upload %s: %w", key, err)
	}
	return fmt.Sprintf("%s/%s?v=%d", c.cdnBase, key, time.Now().Unix()), nil
}

// Delete removes the object at key. A not-found response is not treated
// as an error — deleting an already-gone (or never-uploaded) object is a
// no-op, which matters for the "regenerate" flow's best-effort cleanup of
// the previous image before uploading a new one.
func (c *Client) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("r2store: delete %s: %w", key, err)
	}
	return nil
}

// KeyFromURL extracts the object key from a previously-stored CDN URL
// (the inverse of Upload's returned URL), for Delete calls during
// regenerate. Strips any "?v=..." cache-buster before matching. Returns
// "" if url doesn't look like one of ours.
func (c *Client) KeyFromURL(url string) string {
	if i := strings.IndexByte(url, '?'); i >= 0 {
		url = url[:i]
	}
	prefix := c.cdnBase + "/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	return strings.TrimPrefix(url, prefix)
}

// ObjectInfo is one object returned by ListObjects.
type ObjectInfo struct {
	Key          string
	LastModified time.Time
}

// ListObjects returns every object under the given key prefix. Only
// cmd/db_backup needs this so far (to find R2-side backups older than its
// retention window) — Upload/Delete/KeyFromURL don't need it, so it's kept
// separate rather than folded into New's initial feature set.
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var objects []ObjectInfo
	var continuationToken *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("r2store: list objects %s: %w", prefix, err)
		}
		for _, obj := range out.Contents {
			if obj.Key == nil || obj.LastModified == nil {
				continue
			}
			objects = append(objects, ObjectInfo{Key: *obj.Key, LastModified: *obj.LastModified})
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		continuationToken = out.NextContinuationToken
	}
	return objects, nil
}

// ConfigFromEnv reads the R2_* env vars into a Config — shared by every
// public-media R2 caller in this codebase (news feature images, editorial
// article media) so the five var names live in exactly one place. This
// bucket is fronted by cdn.pdlife.app, so anything uploaded through it is
// publicly reachable — never use this for private data (see
// BackupConfigFromEnv).
func ConfigFromEnv() Config {
	return Config{
		Endpoint:        os.Getenv("R2_ENDPOINT"),
		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		Bucket:          os.Getenv("R2_BUCKET"),
		CDNBase:         os.Getenv("R2_CDN_BASE"),
	}
}

// BackupConfigFromEnv reads config for the private database-backups
// bucket used by cmd/db_backup — a separate bucket from ConfigFromEnv's,
// with no CDN/custom domain in front of it, since a full DB dump
// (password hashes, patient health data) must never be publicly
// reachable the way news images intentionally are. R2_BACKUP_ENDPOINT/
// ACCESS_KEY_ID/SECRET_ACCESS_KEY fall back to the main R2_* vars if
// unset, since R2 endpoints and access keys are normally account-wide in
// Cloudflare (one key can cover multiple buckets) — only R2_BACKUP_BUCKET
// has no fallback, since it must always name a distinct, non-public
// bucket. CDNBase is deliberately left empty.
func BackupConfigFromEnv() Config {
	return Config{
		Endpoint:        firstNonEmpty(os.Getenv("R2_BACKUP_ENDPOINT"), os.Getenv("R2_ENDPOINT")),
		AccessKeyID:     firstNonEmpty(os.Getenv("R2_BACKUP_ACCESS_KEY_ID"), os.Getenv("R2_ACCESS_KEY_ID")),
		SecretAccessKey: firstNonEmpty(os.Getenv("R2_BACKUP_SECRET_ACCESS_KEY"), os.Getenv("R2_SECRET_ACCESS_KEY")),
		Bucket:          os.Getenv("R2_BACKUP_BUCKET"),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
