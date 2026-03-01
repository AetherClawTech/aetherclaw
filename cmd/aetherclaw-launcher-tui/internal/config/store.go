package configstore

import (
	"errors"
	"os"
	"path/filepath"

	aetherclawconfig "github.com/AetherClawTech/aetherclaw/pkg/config"
)

const (
	configDirName  = ".aetherclaw"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*aetherclawconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return aetherclawconfig.LoadConfig(path)
}

func Save(cfg *aetherclawconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return aetherclawconfig.SaveConfig(path, cfg)
}
