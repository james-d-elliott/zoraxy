package database

import (
	"encoding/json"
	"fmt"
	"os"
)

type BootstrapConfig struct {
	Tables map[string]BootstrapTableConfig `json:"tables"`
}

type BootstrapTableConfig struct {
	Values map[string]any `json:"values"`
}

func Bootstrap(db *Database, path string) (err error) {
	if db == nil || len(path) == 0 {
		return fmt.Errorf("database and config path must be specified")
	}

	config := &BootstrapConfig{}

	f, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}

	defer f.Close()

	decoder := json.NewDecoder(f)

	if err = decoder.Decode(config); err != nil {
		return fmt.Errorf("failed to decode config file: %w", err)
	}

	for tableName, table := range config.Tables {
		if err = db.NewTable(tableName); err != nil {
			return fmt.Errorf("error creating table '%s': %w", tableName, err)
		}

		for key, value := range table.Values {
			switch v := value.(type) {
			case float64:
				// The 'encoding/json' lib treats all numbers as float64 by default, this works around this issue.
				if err = db.Write(tableName, key, int(v)); err != nil {
					return fmt.Errorf("error writing to table '%s' with key '%s' and value '%v': %w", tableName, key, value, err)
				}
			default:
				if err = db.Write(tableName, key, value); err != nil {
					return fmt.Errorf("error writing to table '%s' with key '%s' and value '%v': %w", tableName, key, value, err)
				}
			}
		}
	}

	return nil
}
