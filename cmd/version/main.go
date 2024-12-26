package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Project struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Authors      []string          `json:"authors"`
	License      string            `json:"license"`
	Homepage     string            `json:"homepage"`
	Repository   Repository        `json:"repository"`
	Dependencies map[string]string `json:"dependencies"`
	Scripts      map[string]string `json:"scripts"`
}

type Repository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func main() {
	bumpType := flag.String("bump", "", "Version bump type: major, minor, or patch")
	flag.Parse()

	if *bumpType == "" {
		fmt.Println("Please specify a bump type: major, minor, or patch")
		os.Exit(1)
	}

	// Read project.json
	data, err := os.ReadFile("project.json")
	if err != nil {
		fmt.Printf("Error reading project.json: %v\n", err)
		os.Exit(1)
	}

	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		fmt.Printf("Error parsing project.json: %v\n", err)
		os.Exit(1)
	}

	// Parse current version
	parts := strings.Split(project.Version, ".")
	if len(parts) != 3 {
		fmt.Println("Invalid version format in project.json")
		os.Exit(1)
	}

	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	// Bump version
	switch *bumpType {
	case "major":
		major = fmt.Sprintf("%d", atoi(major)+1)
		minor = "0"
		patch = "0"
	case "minor":
		minor = fmt.Sprintf("%d", atoi(minor)+1)
		patch = "0"
	case "patch":
		patch = fmt.Sprintf("%d", atoi(patch)+1)
	default:
		fmt.Printf("Invalid bump type: %s\n", *bumpType)
		os.Exit(1)
	}

	// Update version
	project.Version = fmt.Sprintf("%s.%s.%s", major, minor, patch)

	// Write back to project.json
	updatedData, err := json.MarshalIndent(project, "", "    ")
	if err != nil {
		fmt.Printf("Error encoding project data: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile("project.json", updatedData, 0600); err != nil {
		fmt.Printf("Error writing project.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Version bumped to %s\n", project.Version)
}

func atoi(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		fmt.Printf("Error parsing number: %v\n", err)
		os.Exit(1)
	}
	return n
}
