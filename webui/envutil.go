package webui

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

const envFilePath = ".env"

// needsQuoting returns true if the value contains special characters
func needsQuoting(value string) bool {
	return strings.ContainsAny(value, " \t\n\r\"'`$\\")
}

// quoteValue wraps a value in double quotes and escapes inner quotes
func quoteValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	return `"` + escaped + `"`
}

// unquoteValue removes quotes and unescapes a value
func unquoteValue(value string) string {
	if len(value) < 2 {
		return value
	}
	// Check if quoted with double or single quotes
	if (value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'') {
		unquoted := value[1 : len(value)-1]
		// Unescape common sequences
		unquoted = strings.ReplaceAll(unquoted, `\n`, "\n")
		unquoted = strings.ReplaceAll(unquoted, `\r`, "\r")
		unquoted = strings.ReplaceAll(unquoted, `\"`, `"`)
		unquoted = strings.ReplaceAll(unquoted, `\\`, `\`)
		return unquoted
	}
	return value
}

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
				k := parts[0]
				v := unquoteValue(parts[1])
				envVars[k] = v
			}
		}
	}

	// Update or add the new key-value pair
	envVars[key] = value

	// Get all keys and sort them alphabetically
	var keys []string
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Write back to .env file
	outputFile, err := os.Create(envFilePath)
	if err != nil {
		return fmt.Errorf("error creating .env file: %v", err)
	}
	defer outputFile.Close()

	writer := bufio.NewWriter(outputFile)
	for _, k := range keys {
		v := envVars[k]
		// Quote values that need it
		if needsQuoting(v) {
			v = quoteValue(v)
		}
		_, err := writer.WriteString(fmt.Sprintf("%s=%s\n", k, v))
		if err != nil {
			return fmt.Errorf("error writing to .env file: %v", err)
		}
	}
	writer.Flush()

	// Also update the environment variable for the current process
	os.Setenv(key, value)

	log.Printf("Updated .env: %s", key)
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
