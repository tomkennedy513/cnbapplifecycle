package databaseuri

import (
	"encoding/json"
	"net/url"
)

var schemes = map[string]string{
	"mysql":      "mysql2",
	"mysql2":     "",
	"postgres":   "",
	"postgresql": "postgres",
}

func GetDatabaseUriFromVcapServices(services []byte) (string, error) {
	if len(services) == 0 {
		return "", nil
	}

	data := map[string][]struct {
		Credentials struct {
			Uri string `json:"uri"`
		} `json:"credentials"`
	}{}
	if err := json.Unmarshal(services, &data); err != nil {
		return "", err
	}

	for _, bindingSlices := range data {
		for _, binding := range bindingSlices {
			if binding.Credentials.Uri != "" {
				if uri, err := parseDatabaseUri(binding.Credentials.Uri); err == nil && uri != "" {
					return uri, nil
				}
			}
		}
	}
	return "", nil
}

func parseDatabaseUri(service_uri string) (string, error) {
	uri, err := url.Parse(service_uri)
	if err != nil {
		return "", err
	}

	if val, ok := schemes[uri.Scheme]; ok  {
		if val != "" {
			uri.Scheme = val
		}

		return uri.String(), nil
	}

	return "", nil
	
}
