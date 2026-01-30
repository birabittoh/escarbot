package webui

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

const envFilePath = ".env"

// UpdateEnvVar updates or creates a key-value pair in the .env file
func UpdateEnvVar(key, value string) error {
	envVars := make(map[string]string)

	// Read existing .env file if it exists
	file, err := os.Open(envFilePath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				envVars[parts[0]] = parts[1]
			}
		}
	}

	// Update or add the new key-value pair
	envVars[key] = value

	// Write back to .env file
	outputFile, err := os.Create(envFilePath)
	if err != nil {
		return fmt.Errorf("error creating .env file: %v", err)
	}
	defer outputFile.Close()

	writer := bufio.NewWriter(outputFile)
	for k, v := range envVars {
		_, err := writer.WriteString(fmt.Sprintf("%s=%s\n", k, v))
		if err != nil {
			return fmt.Errorf("error writing to .env file: %v", err)
		}
	}
	writer.Flush()

	// Also update the environment variable for the current process
	os.Setenv(key, value)

	log.Printf("Updated .env: %s=%s", key, value)
	return nil
}

// UpdateBoolEnvVar updates a boolean environment variable
func UpdateBoolEnvVar(key string, value bool) error {
	strValue := "false"
	if value {
		strValue = "true"
	}
	return UpdateEnvVar(key, strValue)
}
