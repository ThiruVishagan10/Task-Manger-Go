package main

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	Port string
}

func loadConfig() Config {
	loadDotEnv(".env")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{Port: port}
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
