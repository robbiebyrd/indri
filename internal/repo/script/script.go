package script

import (
	"encoding/json"
	"github.com/robbiebyrd/indri/internal/models"
	"log"
	"os"
)

func Get(configFilePath string) *models.Script {
	jsonData, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	c := models.Script{}

	err = json.Unmarshal(jsonData, &c)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	return &c
}
