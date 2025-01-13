package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	defaultHALURL  = "iofog"
	defaultPort    = "54331"
	deprovisionURL = "http://iofog:54321/v2/deprovision"
	defaultPeriod  = 60 // Default to 1 minute if PERIOD is not set
	saltFile       = "id/salt-key"
	hwidFile       = "id/hw-id"
)

type HardwareData struct {
	Lscpu   map[string]interface{} `json:"lscpu"`
	Lspci   map[string]interface{} `json:"lspci"`
	Lsusb   map[string]interface{} `json:"lsusb"`
	Lshw    map[string]interface{} `json:"lshw"`
	CpuInfo map[string]interface{} `json:"cpuinfo"`
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func fetchEndpoint(url string) (interface{}, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", url, err)
	}

	return data, nil
}

func collectHardwareData(baseURL string) (*HardwareData, error) {
	endpoints := []string{"lscpu", "lspci", "lsusb", "lshw", "proc/cpuinfo"}
	data := &HardwareData{}

	for _, endpoint := range endpoints {
		url := fmt.Sprintf("http://%s:%s/hal/hwc/%s", baseURL, defaultPort, endpoint)
		result, err := fetchEndpoint(url)
		if err != nil {
			return nil, err
		}

		switch endpoint {
		case "lscpu":
			data.Lscpu = parseToMap(result)
		case "lspci":
			data.Lspci = parseToMap(result)
		case "lsusb":
			data.Lsusb = parseToMap(result)
		case "lshw":
			data.Lshw = parseToMap(result)
		case "proc/cpuinfo":
			data.CpuInfo = parseToMap(result)
		}
	}

	return data, nil
}

func parseToMap(data interface{}) map[string]interface{} {
	if resultMap, ok := data.(map[string]interface{}); ok {
		return resultMap
	}
	return map[string]interface{}{"data": data}
}

func generateSalt() (string, error) {
	salt := make([]byte, 16) // 16-byte salt
	_, err := rand.Read(salt)
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

func saveToFile(filename, data string) error {
	return ioutil.WriteFile(filename, []byte(data), 0600)
}

func loadFromFile(filename string) (string, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
}

func calculateSaltedHash(data *HardwareData) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to serialize hardware data: %w", err)
	}

	salt, err := loadFromFile(saltFile)
	if err != nil {
		log.Println("Salt not found, generating new one.")
		salt, err = generateSalt()
		if err != nil {
			return "", fmt.Errorf("failed to generate salt: %w", err)
		}
		if err := saveToFile(saltFile, salt); err != nil {
			return "", fmt.Errorf("failed to save salt to file: %w", err)
		}
	}

	saltedData := append([]byte(salt), jsonData...)
	hash := sha256.Sum256(saltedData)
	return fmt.Sprintf("%x", hash), nil
}

func loadAuthToken() (string, error) {
	token, err := ioutil.ReadFile("local-api")
	if err != nil {
		return "", fmt.Errorf("failed to read local-api file: %w", err)
	}
	return string(bytes.TrimSpace(token)), nil
}

func deprovisionDevice(authToken string) error {
	req, err := http.NewRequest(http.MethodDelete, deprovisionURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create DELETE request: %w", err)
	}
	req.Header.Set("Authorization", authToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send DELETE request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response status: %d", resp.StatusCode)
	}

	return nil
}

func main() {
	halURL := getEnv("HAL_URL", defaultHALURL)
	periodEnv := getEnv("PERIOD", strconv.Itoa(defaultPeriod))
	period, err := strconv.Atoi(periodEnv)
	if err != nil || period <= 0 {
		log.Printf("Invalid PERIOD value, using default: %d seconds", defaultPeriod)
		period = defaultPeriod
	}

	initialHdID, err := loadFromFile(hwidFile)
	if err != nil {
		log.Println("HWID not found, will calculate on first run.")
	}

	for {
		hardwareData, err := collectHardwareData(halURL)
		if err != nil {
			log.Printf("Error collecting hardware data: %v", err)
			continue
		}

		hwID, err := calculateSaltedHash(hardwareData)
		if err != nil {
			log.Printf("Error calculating hardware hash: %v", err)
			continue
		}
		log.Printf("Calculated hardware hash: %s", hwID)

		if initialHdID == "" {
			initialHdID = hwID
			if err := saveToFile(hwidFile, hwID); err != nil {
				log.Printf("Error saving HWID to file: %v", err)
			}
			log.Println("Initial hardware ID set.")
			continue
		}

		if hwID != initialHdID {
			authToken, err := loadAuthToken()
			if err != nil {
				log.Printf("Error loading auth token: %v", err)
				continue
			}

			if err := deprovisionDevice(authToken); err != nil {
				log.Printf("Error deprovisioning device: %v", err)
				continue
			}

			log.Println("Device deprovisioned due to hardware changes.")
			break
		}

		log.Println("Hardware configuration unchanged.")
		time.Sleep(time.Duration(period) * time.Second) // Periodic check interval
	}
}
