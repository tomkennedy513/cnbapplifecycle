package credhub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	api "code.cloudfoundry.org/credhub-cli/credhub"
)

type platformOptions struct {
	CredhubURI string `json:"credhub-uri"`
}

func InterpolateServiceRefsFromVcapServices(maxConnectionAttempts int, retryDelay time.Duration) error {
	if os.Getenv("CREDHUB_SKIP_INTERPOLATION") != "" {
		return nil
	}
	if !strings.Contains(os.Getenv("VCAP_SERVICES"), `"credhub-ref"`) {
		return nil
	}

	ch, err := newCredhubClient()
	if err != nil {
		return fmt.Errorf("unable to set up credhub client: %v", err)
	}

	var interpolatedServices string
	for attempt := 1; attempt <= maxConnectionAttempts; attempt++ {
		interpolatedServices, err = ch.InterpolateString(os.Getenv("VCAP_SERVICES"))
		if err == nil {
			break
		}
		fmt.Printf("Failed on attempt %v out of %v: Unable to interpolate credhub references: %v\n", attempt, maxConnectionAttempts, err)
		time.Sleep(retryDelay)
	}

	if err != nil {
		return fmt.Errorf("unable to interpolate credhub references: %v", err)
	}

	if err := os.Setenv("VCAP_SERVICES", interpolatedServices); err != nil {
		return fmt.Errorf("unable to update VCAP_SERVICES with interpolated credhub references: %v", err)
	}
	return nil
}

type Credentials struct {
	CredhubRef string `json:"credhub-ref"`
}

type ServiceRef struct {
	Credentials Credentials `json:"credentials"`
}

type ServicesMap map[string][]map[string]any

func InterpolateServiceRefsFromFiles(maxConnectionAttempts int, retryDelay time.Duration, bindingRoot string) error {
	if os.Getenv("CREDHUB_SKIP_INTERPOLATION") != "" {
		return nil
	}

	servicesMap, err := getServicesMap(bindingRoot)
	if err != nil {
		return fmt.Errorf("unable to get services map: %v", err)
	}

	if len(servicesMap) == 0 {
		return nil
	}

	ch, err := newCredhubClient()
	if err != nil {
		return fmt.Errorf("unable to set up credhub client: %v", err)
	}

	jsonString, err := json.Marshal(servicesMap)
	if err != nil {
		return fmt.Errorf("unable to marshal services map: %v", err)
	}

	var interpolatedServices string
	for attempt := 1; attempt <= maxConnectionAttempts; attempt++ {
		interpolatedServices, err = ch.InterpolateString(string(jsonString))
		if err == nil {
			break
		}
		fmt.Printf("Failed on attempt %v out of %v: Unable to interpolate credhub references: %v\n", attempt, maxConnectionAttempts, err)
		time.Sleep(retryDelay)
	}

	if err != nil {
		return fmt.Errorf("unable to interpolate credhub references: %v", err)
	}

	interpolatedServicesMap := ServicesMap{}
	err = json.Unmarshal([]byte(interpolatedServices), &interpolatedServicesMap)
	if err != nil {
		return fmt.Errorf("unable to unmarshal interpolated services map: %v", err)
	}

	for serviceName, serviceList := range interpolatedServicesMap {
		if len(serviceList) == 0 {
			continue
		}

		bindingDir := filepath.Join(bindingRoot, serviceName)
		serviceData := serviceList[0] // Each service should have one entry

		for key, value := range serviceData {
			filename := filepath.Join(bindingDir, key)

			var content string
			switch v := value.(type) {
			case string:
				content = v
			default:
				bytes, err := json.Marshal(v)
				if err != nil {
					return fmt.Errorf("unable to marshal value for %s/%s: %v", serviceName, key, err)
				}
				content = string(bytes)
			}

			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				return fmt.Errorf("unable to write file %s: %v", filename, err)
			}
		}

		credhubRefFile := filepath.Join(bindingDir, "credhub-ref")
		if err := os.Remove(credhubRefFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("unable to remove credhub-ref file %s: %v", credhubRefFile, err)
		}
	}

	return nil
}

func getServicesMap(bindingRoot string) (ServicesMap, error) {
	info, err := os.Stat(bindingRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to get binding root: %v", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("binding root is not a directory: %v", bindingRoot)
	}

	entries, err := os.ReadDir(bindingRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to read binding root: %v", err)
	}

	servicesMap := ServicesMap{}
	for _, entry := range entries {
		if entry.IsDir() {
			ref, err := getCredhubRef(filepath.Join(bindingRoot, entry.Name(), "credhub-ref"))
			if err != nil {
				return nil, fmt.Errorf("unable to get credhub refs for service %s: %v", entry.Name(), err)
			}
			if ref == "" {
				continue
			}
			servicesMap[entry.Name()] = []map[string]any{{"credhub-ref": ref}}
		}
	}

	return servicesMap, nil
}

func getCredhubRef(filename string) (string, error) {
	_, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("unable to get info for %s: %v", filename, err)
	}

	bytes, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("unable to read %s: %v", filename, err)
	}

	return string(bytes), nil
}

func getPlatformOptions() (platformOptions, error) {
	var platformOptions platformOptions
	platformOptionString := os.Getenv("VCAP_PLATFORM_OPTIONS")
	if platformOptionString == "" {
		return platformOptions, nil
	}

	err := json.Unmarshal([]byte(platformOptionString), &platformOptions)
	if err != nil {
		return platformOptions, err
	}

	return platformOptions, nil
}

func newCredhubClient() (*api.CredHub, error) {
	platformOptions, err := getPlatformOptions()
	if err != nil {
		return nil, fmt.Errorf("unable to get platform options: %s", err)
	}

	if platformOptions.CredhubURI == "" {
		return nil, fmt.Errorf("missing credhub URI")
	}

	instanceCertPath := os.Getenv("CF_INSTANCE_CERT")
	instanceKeyPath := os.Getenv("CF_INSTANCE_KEY")
	systemCertsPath := os.Getenv("CF_SYSTEM_CERT_PATH")

	if instanceCertPath == "" || instanceKeyPath == "" {
		return nil, fmt.Errorf("missing CF_INSTANCE_CERT and/or CF_INSTANCE_KEY")
	}
	if systemCertsPath == "" {
		return nil, fmt.Errorf("missing CF_SYSTEM_CERT_PATH")
	}

	caCerts := []string{}
	files, err := os.ReadDir(systemCertsPath)
	if err != nil {
		return nil, fmt.Errorf("can't read contents of system cert path: %v", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".crt") {
			contents, err := os.ReadFile(filepath.Join(systemCertsPath, file.Name()))
			if err != nil {
				return nil, fmt.Errorf("can't read contents of cert in system cert path: %v", err)
			}
			caCerts = append(caCerts, string(contents))
		}
	}

	return api.New(
		platformOptions.CredhubURI,
		api.ClientCert(instanceCertPath, instanceKeyPath),
		api.CaCerts(caCerts...),
	)
}
