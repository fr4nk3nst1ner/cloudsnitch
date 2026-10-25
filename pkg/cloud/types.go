package cloud

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// LogConfig holds logging configuration
type LogConfig struct {
	Level    string
	FilePath string
}

// Config holds the configuration for cloud enumeration
type Config struct {
	Platform        string
	Profile         string
	AWSAccount      string
	SubscriptionID  string
	OutputFormat    string
	OutputFile      string
	ResourceTypes   []string
	SourceProject   string   // Source GCP project ID
	TargetProject   string   // Target GCP project ID
	GCPScope        string   // Cloud Asset Inventory scope: projects/ID, folders/ID, or organizations/ID
}

// Resource represents a generic cloud resource
type Resource struct {
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	ID         string                 `json:"id"`
	Location   string                 `json:"location"`
	Properties map[string]interface{} `json:"properties"`
	Tags       map[string]string      `json:"tags,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
}

// Results holds enumeration results for all resource types
type Results struct {
	Platform  string     `json:"platform"`
	Resources []Resource `json:"resources"`
}

// Enumerator interface defines methods that must be implemented by cloud-specific enumerators
type Enumerator interface {
	Enumerate() (*Results, error)
}

// WriteOutput writes the results to the specified output format and destination
func WriteOutput(results *Results, config Config) error {
	if results == nil {
		return fmt.Errorf("no results to write")
	}

	switch config.OutputFormat {
	case "json":
		return writeJSON(results, config.OutputFile)
	case "csv":
		return writeCSV(results, config.OutputFile)
	default:
		return writeText(results, config.OutputFile)
	}
}

func writeJSON(results *Results, filepath string) error {
	data, err := json.MarshalIndent(results, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal results to JSON: %v", err)
	}

	if filepath == "" {
		fmt.Println(string(data))
		return nil
	}

	return os.WriteFile(filepath, data, 0644)
}

func writeCSV(results *Results, filepath string) error {
	headers := []string{"Type", "Name", "ID", "Location", "Properties", "Tags", "CreatedAt"}
	
	var writer *csv.Writer
	if filepath == "" {
		writer = csv.NewWriter(os.Stdout)
	} else {
		file, err := os.Create(filepath)
		if err != nil {
			return fmt.Errorf("failed to create CSV file: %v", err)
		}
		defer file.Close()
		writer = csv.NewWriter(file)
	}

	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV headers: %v", err)
	}

	for _, resource := range results.Resources {
		propertiesJSON, _ := json.Marshal(resource.Properties)
		tagsJSON, _ := json.Marshal(resource.Tags)
		
		record := []string{
			resource.Type,
			resource.Name,
			resource.ID,
			resource.Location,
			string(propertiesJSON),
			string(tagsJSON),
			resource.CreatedAt.Format(time.RFC3339),
		}
		
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %v", err)
		}
	}

	writer.Flush()
	return writer.Error()
}

func writeText(results *Results, filepath string) error {
	var output string
	output += fmt.Sprintf("Platform: %s\n", results.Platform)
	output += fmt.Sprintf("Total Resources: %d\n\n", len(results.Resources))

	for _, resource := range results.Resources {
		output += fmt.Sprintf("Type: %s\n", resource.Type)
		output += fmt.Sprintf("Name: %s\n", resource.Name)
		output += fmt.Sprintf("ID: %s\n", resource.ID)
		output += fmt.Sprintf("Location: %s\n", resource.Location)
		output += fmt.Sprintf("Created: %s\n", resource.CreatedAt.Format(time.RFC3339))
		
		if len(resource.Properties) > 0 {
			output += "Properties:\n"
			for k, v := range resource.Properties {
				output += fmt.Sprintf("  %s: %v\n", k, v)
			}
		}
		
		if len(resource.Tags) > 0 {
			output += "Tags:\n"
			for k, v := range resource.Tags {
				output += fmt.Sprintf("  %s: %s\n", k, v)
			}
		}
		
		output += strings.Repeat("-", 40) + "\n"
	}

	if filepath == "" {
		fmt.Print(output)
		return nil
	}

	return os.WriteFile(filepath, []byte(output), 0644)
} 