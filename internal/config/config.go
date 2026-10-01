package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Worker   WorkerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Kafka    KafkaConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type WorkerConfig struct {
	Workers   int
	QueueSize int
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type RedisConfig struct {
	Addr       string
	Password   string
	DB         int
	ProductTTL time.Duration
}

type KafkaConfig struct {
	Brokers []string
}

func Load() *Config {
	kafkaBrokersRaw := getEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := strings.Split(kafkaBrokersRaw, ",")

	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", ":8080"),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Worker: WorkerConfig{
			Workers:   getEnvAsInt("WORKER_COUNT", 3),
			QueueSize: getEnvAsInt("WORKER_QUEUE_SIZE", 20),
		},
		Database: DatabaseConfig{
			DSN:             getEnv("DATABASE_URL", "postgres://commerce_user:commerce_password@localhost:5432/digital_commerce?sslmode=disable"),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: 15 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		Redis: RedisConfig{
			Addr:       getEnv("REDIS_ADDR", "localhost:6379"),
			Password:   getEnv("REDIS_PASSWORD", ""),
			DB:         getEnvAsInt("REDIS_DB", 0),
			ProductTTL: 10 * time.Minute,
		},
		Kafka: KafkaConfig{
			Brokers: brokers,
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}
