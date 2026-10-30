package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServerPort     string
	DBDSN          string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	CookieHashKey  []byte
	CookieBlockKey []byte
	BcryptCost     int
	TokenExpiry    time.Duration
}

func (c *Config) Validate() error {
	if c.DBDSN == "" {
		return fmt.Errorf("database DSN must be configured")
	}

	if len(c.CookieHashKey) != 64 {
		return fmt.Errorf("COOKIE_HASH_KEY must contain 64 bytes")
	}
	if len(c.CookieBlockKey) != 32 {
		return fmt.Errorf("COOKIE_BLOCK_KEY must contain 32 bytes")
	}

	return nil
}

func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:    getEnv("SERVER_PORT", "8080"),
		DBDSN:         getEnv("DB_DSN", "heartrate.db"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),
		BcryptCost:    getEnvAsInt("BCRYPT_COST", 10),
		TokenExpiry:   24 * time.Hour,
	}

	hashKey, err := decodeHexKey(getEnv("COOKIE_HASH_KEY", ""), 64)
	if err != nil {
		return nil, fmt.Errorf("COOKIE_HASH_KEY: %w", err)
	}
	cfg.CookieHashKey = hashKey

	blockKey, err := decodeHexKey(getEnv("COOKIE_BLOCK_KEY", ""), 32)
	if err != nil {
		return nil, fmt.Errorf("COOKIE_BLOCK_KEY: %w", err)
	}
	cfg.CookieBlockKey = blockKey

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	strValue := getEnv(key, "")
	if strValue == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(strValue)
	if err != nil {
		return defaultValue
	}
	return value
}

func decodeHexKey(hexKey string, expectedBytes int) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	if len(key) != expectedBytes {
		return nil, fmt.Errorf("must be a %d-character hex string", expectedBytes*2)
	}
	return key, nil
}
