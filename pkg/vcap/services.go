package vcap

import (
	"encoding/json"
	"maps"
	"net/url"
	"os"
	"path/filepath"
)

const (
	databaseUrlEnv        = "DATABASE_URL"
	serviceBindingRootEnv = "SERVICE_BINDING_ROOT"
	vcapServicesEnv       = "VCAP_SERVICES"
)

var (
	schemes = map[string]string{
		"mysql":      "mysql2",
		"mysql2":     "",
		"postgres":   "",
		"postgresql": "postgres",
	}
)

type vcapServiceBinding struct {
	Name        string         `json:"name"`
	Label       string         `json:"label"`
	Credentials map[string]any `json:"credentials"`
}

func TranslateVcapServices(bindingRoot string) error {
	vcapServices := os.Getenv(vcapServicesEnv)
	if vcapServices == "" || vcapServices == "{}" {
		return nil
	}

	if err := os.Setenv(serviceBindingRootEnv, bindingRoot); err != nil {
		return err
	}

	var bindingsMap map[string][]vcapServiceBinding
	if err := json.Unmarshal([]byte(vcapServices), &bindingsMap); err != nil {
		return err
	}

	if err := os.MkdirAll(bindingRoot, 0755); err != nil {
		return err
	}

	for bindingsSlice := range maps.Values(bindingsMap) {
		for _, binding := range bindingsSlice {
			if err := writeBinding(bindingRoot, binding); err != nil {
				return err
			}

			if uri, ok := binding.Credentials["uri"]; ok && os.Getenv(databaseUrlEnv) == "" {
				if err := os.Setenv(databaseUrlEnv, parseUri(uri)); err != nil {
					return err
				}
			}
		}

	}

	return nil
}

func writeBinding(bindingRoot string, binding vcapServiceBinding) error {
	bindingDir := filepath.Join(bindingRoot, binding.Name)
	if err := os.MkdirAll(bindingDir, 0755); err != nil {
		return err
	}

	for name, value := range binding.Credentials {
		if err := writeCredentialFile(filepath.Join(bindingDir, name), value); err != nil {
			return err
		}
	}

	return nil
}

func writeCredentialFile(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	asString, err := stringify(value)
	if err != nil {
		return err
	}

	_, err = file.WriteString(asString)
	return err
}

func stringify(in any) (string, error) {
	switch t := in.(type) {
	case string:
		return t, nil
	default:
		asJson, err := json.Marshal(t)
		if err != nil {
			return "", nil
		}

		return string(asJson), nil

	}

}

func parseUri(uri any) string {
	asString, ok := uri.(string)
	if !ok {
		return ""
	}

	if parsed, err := url.Parse(asString); err == nil {
		if val, ok := schemes[parsed.Scheme]; ok {
			if val != "" {
				parsed.Scheme = val
			}
			return parsed.String()
		}
	}
	return ""
}
