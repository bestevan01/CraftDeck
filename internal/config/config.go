// Package config loads CraftDeck's runtime configuration from environment
// variables so the systemd unit can configure the daemon without a config
// file (see packaging/systemd/craftdeck.service).
package config

import "os"

type Config struct {
	// ListenAddr is the address the management web UI/API listens on.
	ListenAddr string
	// DataDir holds the SQLite database, per-instance working directories,
	// and uploaded plugin/mod/loader jars.
	DataDir string
	// MasterKeyPath points to the file holding the AES-GCM key used to
	// encrypt secrets at rest (RCON passwords, DDNS API tokens).
	MasterKeyPath string
}

func Load() Config {
	return Config{
		ListenAddr:    getEnv("CRAFTDECK_LISTEN_ADDR", ":8080"),
		DataDir:       getEnv("CRAFTDECK_DATA_DIR", "/var/lib/craftdeck"),
		MasterKeyPath: getEnv("CRAFTDECK_MASTER_KEY", "/etc/craftdeck/master.key"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
