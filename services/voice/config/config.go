package config

import "github.com/spf13/viper"

// Config holds all configuration values for the voice service.
type Config struct {
	DBUrl       string `mapstructure:"VOICE_DB_URL"`
	GRPCPort    string `mapstructure:"VOICE_GRPC_PORT"`
	JWTSecret   string `mapstructure:"JWT_SECRET"`
	RedisURL    string `mapstructure:"VOICE_REDIS_URL"`
	CORSOrigin  string `mapstructure:"VOICE_CORS_ORIGIN"`
	S3Endpoint  string `mapstructure:"S3_ENDPOINT"`
	S3Region    string `mapstructure:"S3_REGION"`
	S3Bucket    string `mapstructure:"S3_BUCKET"`
	S3AccessKey string `mapstructure:"S3_ACCESS_KEY_ID"`
	S3SecretKey string `mapstructure:"S3_SECRET_ACCESS_KEY"`
}

// Load reads configuration from a .env file or environment variables.
func Load() (Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("../..")
	_ = viper.ReadInConfig()

	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("VOICE_GRPC_PORT", "50052")
	viper.SetDefault("VOICE_CORS_ORIGIN", "*")
	viper.SetDefault("S3_REGION", "us-west-004")
	viper.SetDefault("S3_ENDPOINT", "https://s3.us-west-004.backblazeb2.com")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
