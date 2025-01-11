package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"github.com/stealthrocket/net/wasip1"
)

const (
	defaultHALURL = "iofog"
	defaultPort   = "54331"
	deprovisionURL = "http://iofog:54321/v2/deprovision"
	defaultPeriod = 60 // Default to 10 minutes if PERIOD is not set
)

type HardwareData struct {
	Lscpu   map[string]interface{} `json:"lscpu"`
	Lspci   map[string]interface{} `json:"lspci"`
	Lsusb   map[string]interface{} `json:"lsusb"`
	Lshw    map[string]interface{} `json:"lshw"`
	CpuInfo map[string]interface{} `json:"cpuinfo"`
}

var salt string // Global variable to hold the salt in memory

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
			if resultMap, ok := result.(map[string]interface{}); ok {
				data.Lscpu = resultMap
			} else {
				data.Lscpu = map[string]interface{}{"data": result}
			}
		case "lspci":
			if resultMap, ok := result.(map[string]interface{}); ok {
				data.Lspci = resultMap
			} else {
				data.Lspci = map[string]interface{}{"data": result}
			}
		case "lsusb":
			if resultMap, ok := result.(map[string]interface{}); ok {
				data.Lsusb = resultMap
			} else {
				data.Lsusb = map[string]interface{}{"data": result}
			}
		case "lshw":
			if resultMap, ok := result.(map[string]interface{}); ok {
				data.Lshw = resultMap
			} else {
				data.Lshw = map[string]interface{}{"data": result}
			}
		case "proc/cpuinfo":
			if resultMap, ok := result.(map[string]interface{}); ok {
				data.CpuInfo = resultMap
			} else {
				data.CpuInfo = map[string]interface{}{"data": result}
			}
		}
	}

	return data, nil
}

// Generate a random salt
func generateSalt() (string, error) {
	salt := make([]byte, 16) // 16-byte salt
	_, err := rand.Read(salt)
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

// Calculate the salted hash of the hardware data
func calculateSaltedHash(data *HardwareData) (string, error) {
	// Marshal the hardware data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to serialize hardware data: %w", err)
	}

	// If salt is empty, generate a new one
	if salt == "" {
		var err error
		salt, err = generateSalt()
		if err != nil {
			return "", fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// Combine the salt and hardware data
	saltedData := append([]byte(salt), jsonData...)

	// Calculate the SHA256 hash of the salted data
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

func init() {
	// Initialize WASI-compatible transport
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.DialContext = wasip1.DialContext
	}
}


func main() {
	halURL := getEnv("HAL_URL", defaultHALURL)
	periodEnv := getEnv("PERIOD", strconv.Itoa(defaultPeriod))
	period, err := strconv.Atoi(periodEnv)
	if err != nil || period <= 0 {
		log.Printf("Invalid PERIOD value, using default: %d seconds", defaultPeriod)
		period = defaultPeriod
	}

	var initialHdID string

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
