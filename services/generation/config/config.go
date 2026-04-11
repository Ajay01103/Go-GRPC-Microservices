package config

import "github.com/spf13/viper"

type Config struct {
	DBURL                   string `mapstructure:"GENERATION_DB_URL"`
	GRPCPort                string `mapstructure:"GENERATION_GRPC_PORT"`
	JWTSecret               string `mapstructure:"JWT_SECRET"`
	CORSOrigin              string `mapstructure:"GENERATION_CORS_ORIGIN"`
	RedisURL                string `mapstructure:"GENERATION_REDIS_URL"`
	TTSQueueChannel         string `mapstructure:"GENERATION_TTS_QUEUE_CHANNEL"`
	TTSResultsChannelPrefix string `mapstructure:"GENERATION_TTS_RESULTS_CHANNEL_PREFIX"`
	TTSEndpoint             string `mapstructure:"GENERATION_TTS_ENDPOINT"`
	TTSAPIKey               string `mapstructure:"GENERATION_TTS_API_KEY"`
	AudioBaseURL            string `mapstructure:"GENERATION_AUDIO_BASE_URL"`
}

func Load() (Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("../..")
	_ = viper.ReadInConfig()

	viper.AutomaticEnv()

	viper.SetDefault("GENERATION_GRPC_PORT", "50053")
	viper.SetDefault("GENERATION_CORS_ORIGIN", "*")
	viper.SetDefault("GENERATION_TTS_QUEUE_CHANNEL", "tts:jobs")
	viper.SetDefault("GENERATION_TTS_RESULTS_CHANNEL_PREFIX", "tts:results:")
	viper.SetDefault("GENERATION_TTS_ENDPOINT", "")
	viper.SetDefault("GENERATION_AUDIO_BASE_URL", "/api/audio")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
