package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const discoveryHost = "discovery.minecraftservices.com"
const discoveryPath = "/minecraft/client"

var discoveryClient = &http.Client{Timeout: 15 * time.Second}

func serveDiscovery(w http.ResponseWriter, r *http.Request, base string) error {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return nil
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://"+discoveryHost+discoveryPath, nil)
	if err != nil {
		return err
	}
	resp, err := discoveryClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, http.StatusText(resp.StatusCode), resp.StatusCode)
		return nil
	}
	var doc map[string]any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	services, ok := doc["discovery"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing discovery services")
	}
	for _, value := range services {
		service, _ := value.(map[string]any)
		endpoints, _ := service["endpoints"].(map[string]any)
		for _, value := range endpoints {
			endpoint, _ := value.(map[string]any)
			uri, _ := endpoint["uri"].(string)
			for prefix, host := range hosts {
				if path, ok := strings.CutPrefix(uri, "https://"+host+"/"); ok {
					endpoint["uri"] = base + "/" + prefix + "/" + path
					break
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		return json.NewEncoder(w).Encode(doc)
	}
	return nil
}
