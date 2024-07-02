package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	AppConfig AppConfig       `json:"appConfig"`
	ZoneList  map[string]Zone `json:"zoneList"`
}

type AppConfig struct {
	TZ         string `json:"tz"`
	DebugLevel string `json:"debugLevel"`
}

type Zone struct {
	FriendlyName string         `json:"friendlyName"`
	Location     string         `json:"location"`
	Schedule     []ScheduleItem `json:"schedule"`
}

type ScheduleItem struct {
	StartTime       string `json:"startTime"`
	DurationMinutes int    `json:"durationMinutes"`
	Weekdays        uint8  `json:"weekdays"`
	Completed       bool   `json:"completed"`
}

func LoadConfig(file string) (*Config, error) {
	var cfg Config

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}