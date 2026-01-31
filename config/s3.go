package config

// S3CloudStorageConfig stores the configuration of an S3 API proxy backend.
// Authentication is handled automatically by the AWS SDK using the standard
// credential chain (environment variables, shared credentials, IAM roles, SSO, etc.)
type S3CloudStorageConfig struct {
	Endpoint         string `yaml:"endpoint"`
	Bucket           string `yaml:"bucket"`
	Prefix           string `yaml:"prefix"`
	DisableSSL       bool   `yaml:"disable_ssl"`
	UpdateTimestamps bool   `yaml:"update_timestamps"`
	Region           string `yaml:"region"`
}
