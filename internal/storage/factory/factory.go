// Package factory 根据配置构造存储后端实例
package factory

import (
	"encoding/json"
	"fmt"

	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/storage/alipan"
	"github.com/musicflow/musicflow/internal/storage/local"
	"github.com/musicflow/musicflow/internal/storage/onedrive"
	s3storage "github.com/musicflow/musicflow/internal/storage/s3"
	sftpstorage "github.com/musicflow/musicflow/internal/storage/sftp"
	"github.com/musicflow/musicflow/internal/storage/tianyi"
	"github.com/musicflow/musicflow/internal/storage/webdav"
	"github.com/musicflow/musicflow/internal/storage/yun139"
	"go.uber.org/zap"
)

// TargetSpec 构造存储后端所需的规格
type TargetSpec struct {
	ID     string
	Name   string
	Type   storage.StorageType
	Config []byte
	Log    *zap.Logger
}

// Build 根据存储目标配置构造后端实例
func Build(spec TargetSpec) (storage.Backend, error) {
	switch spec.Type {
	case storage.StorageLocal:
		var cfg struct {
			BasePath string `json:"base_path"`
		}
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse local config: %w", err)
		}
		if cfg.BasePath == "" {
			return nil, fmt.Errorf("local storage missing base_path")
		}
		return local.New(spec.ID, spec.Name, cfg.BasePath)

	case storage.StorageWebDAV:
		var cfg webdav.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse webdav config: %w", err)
		}
		return webdav.New(spec.ID, spec.Name, cfg), nil

	case storage.StorageSFTP:
		var cfg sftpstorage.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse sftp config: %w", err)
		}
		return sftpstorage.New(spec.ID, spec.Name, cfg), nil

	case storage.StorageS3:
		var cfg s3storage.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse s3 config: %w", err)
		}
		return s3storage.New(spec.ID, spec.Name, cfg), nil

	case storage.StorageAlipan:
		var cfg alipan.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse alipan config: %w", err)
		}
		return alipan.New(spec.ID, spec.Name, cfg, spec.Log), nil

	case storage.StorageOneDrive:
		var cfg onedrive.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse onedrive config: %w", err)
		}
		return onedrive.New(spec.ID, spec.Name, cfg, spec.Log), nil

	case storage.StorageYun139:
		var cfg yun139.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse yun139 config: %w", err)
		}
		return yun139.New(spec.ID, spec.Name, cfg, spec.Log), nil

	case storage.StorageTianyi:
		var cfg tianyi.Config
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("parse tianyi config: %w", err)
		}
		return tianyi.New(spec.ID, spec.Name, cfg, spec.Log), nil

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", spec.Type)
	}
}
